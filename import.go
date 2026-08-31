package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Importing a local music collection.
//
// Files are catalogued where they already live rather than copied: a library of
// thousands of tracks is tens of gigabytes, and duplicating it to serve it would
// be a poor trade for a machine that is often a Raspberry Pi with an SD card.
//
// The scan root is not free-form. A handler that walked any absolute path the
// caller supplied would be an arbitrary file-read primitive for anyone holding
// an admin session, so the root is declared once in configuration
// (APP_IMPORT_ROOT) and every candidate path must resolve inside it *after*
// symlinks are followed — a symlink pointing at /etc is exactly how this kind
// of check gets bypassed.
const importRootEnv = "APP_IMPORT_ROOT"

// Extensions we accept. `.m4p` is deliberately absent: those files carry
// FairPlay DRM, and decoding them means circumventing it. They are reported to
// the user with an explanation instead of being silently skipped.
var importableExtensions = map[string]bool{
	".mp3": true, ".m4a": true, ".flac": true, ".wav": true,
	".aac": true, ".ogg": true, ".opus": true, ".wma": true, ".aiff": true, ".alac": true,
}

const drmExtension = ".m4p"

// maxProbeFiles caps a single scan so one request cannot pin the server for
// hours; the response says when the cap was hit so the caller can continue.
const maxProbeFiles = 20000

type ImportResult struct {
	Scanned   int            `json:"scanned"`
	Imported  int            `json:"imported"`
	Duplicate int            `json:"duplicate"`
	Skipped   []ImportSkip   `json:"skipped,omitempty"`
	Elapsed   string         `json:"elapsed"`
	Truncated bool           `json:"truncated,omitempty"`
	Counts    map[string]int `json:"counts,omitempty"`
}

type ImportSkip struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

// resolveImportPath validates that `requested` stays within `root`.
//
// Both sides are resolved through EvalSymlinks before comparison, and the
// prefix test uses a trailing separator so that "/musicOTHER" cannot pass as a
// child of "/music".
func resolveImportPath(root, requested string) (string, error) {
	if root == "" {
		return "", fmt.Errorf("import root is not configured (%s)", importRootEnv)
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("import root is unavailable: %w", err)
	}
	target := requested
	if target == "" {
		target = realRoot
	} else if !filepath.IsAbs(target) {
		target = filepath.Join(realRoot, target)
	}
	realTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		return "", fmt.Errorf("path is unavailable: %w", err)
	}
	if realTarget != realRoot && !strings.HasPrefix(realTarget, realRoot+string(os.PathSeparator)) {
		return "", fmt.Errorf("path is outside the import root")
	}
	return realTarget, nil
}

// fileFingerprint identifies a file by size plus a hash of its head and tail.
//
// Hashing whole files would mean reading the entire library from disk on every
// scan. Two distinct audio files sharing a size, first 64 KB and last 64 KB is
// not a case worth the extra I/O.
func fileFingerprint(path string, size int64) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	const window = 64 << 10
	h := sha256.New()
	fmt.Fprintf(h, "%d:", size)
	head := make([]byte, min64(window, size))
	if _, err := io.ReadFull(f, head); err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return "", err
	}
	h.Write(head)
	if size > window*2 {
		if _, err := f.Seek(-window, io.SeekEnd); err != nil {
			return "", err
		}
		tail := make([]byte, window)
		if _, err := io.ReadFull(f, tail); err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
			return "", err
		}
		h.Write(tail)
	}
	return hex.EncodeToString(h.Sum(nil))[:32], nil
}

func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

type probedTags struct {
	Format struct {
		Duration string            `json:"duration"`
		Tags     map[string]string `json:"tags"`
	} `json:"format"`
}

// probeTags reads metadata with ffprobe, which the project already depends on
// for downloads — no extra library, and it understands every container we accept.
func probeTags(ctx context.Context, path string) (title, artist, album string, duration int) {
	cmd := exec.CommandContext(ctx, "ffprobe", "-v", "quiet", "-print_format", "json",
		"-show_format", path)
	out, err := cmd.Output()
	if err != nil {
		return "", "", "", 0
	}
	var probed probedTags
	if json.Unmarshal(out, &probed) != nil {
		return "", "", "", 0
	}
	// ffprobe casing varies by container: TITLE in FLAC, title in MP4.
	lookup := func(keys ...string) string {
		for key, value := range probed.Format.Tags {
			for _, want := range keys {
				if strings.EqualFold(key, want) && strings.TrimSpace(value) != "" {
					return strings.TrimSpace(value)
				}
			}
		}
		return ""
	}
	if seconds, err := strconv.ParseFloat(probed.Format.Duration, 64); err == nil {
		duration = int(seconds)
	}
	return lookup("title"), lookup("artist", "album_artist"), lookup("album"), duration
}

// scanImportRoot walks the tree and turns audio files into tracks.
func (a *App) scanImportRoot(ctx context.Context, dir string) (ImportResult, []LocalFile) {
	result := ImportResult{Counts: map[string]int{}}
	found := []LocalFile{}
	start := time.Now()

	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil // an unreadable subtree must not abort the whole scan
		}
		if len(found) >= maxProbeFiles {
			result.Truncated = true
			return fs.SkipAll
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext == drmExtension {
			result.Skipped = append(result.Skipped, ImportSkip{
				Path:   filepath.Base(path),
				Reason: "Файл защищён DRM (FairPlay). Перекачайте трек из Apple Music — с 2009 года покупки идут без защиты.",
			})
			return nil
		}
		if !importableExtensions[ext] {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		result.Scanned++
		result.Counts[strings.TrimPrefix(ext, ".")]++

		fingerprint, err := fileFingerprint(path, info.Size())
		if err != nil {
			result.Skipped = append(result.Skipped, ImportSkip{Path: filepath.Base(path), Reason: "не удалось прочитать файл"})
			return nil
		}
		title, artist, album, duration := probeTags(ctx, path)
		if title == "" {
			// Nothing usable in the tags: the file name is a better label than
			// an empty row in the library.
			title = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		}
		found = append(found, LocalFile{
			TrackID: "local:" + fingerprint,
			Path:    path,
			Size:    info.Size(),
			Track: Track{
				ID: "local:" + fingerprint, ProviderID: "local", ProviderTrackID: fingerprint,
				Title: title, Artist: artist, Album: album, DurationSeconds: duration,
				SourceURL: "/v1/local/" + fingerprint, Attribution: "Локальный файл",
				Capabilities: localCaps(), Policy: localPolicy(),
			},
		})
		return nil
	})
	result.Elapsed = time.Since(start).Round(time.Millisecond).String()
	return result, found
}

func (a *App) importScan(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdmin(w, r) {
		return
	}
	var req struct {
		Path string `json:"path"`
	}
	_ = decodeJSON(r, &req)

	root, err := resolveImportPath(os.Getenv(importRootEnv), req.Path)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Minute)
	defer cancel()
	result, files := a.scanImportRoot(ctx, root)

	ownerID := userIDFromRequest(r)
	imported, duplicate, err := a.store.BulkImportLocalFiles(ownerID, files)
	if err != nil {
		slog.Warn("import failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Не удалось сохранить импортированные треки")
		return
	}
	result.Imported, result.Duplicate = imported, duplicate
	slog.Info("import finished", "scanned", result.Scanned, "imported", imported, "duplicate", duplicate, "elapsed", result.Elapsed)
	writeJSON(w, http.StatusOK, result)
}

// serveLocalFile streams an imported file. The path comes from the database
// rather than the request, but it is re-validated against the import root
// anyway: the root may have changed since the scan, and a stored path must
// never become a way out of it.
// resolveLocalFile проверяет, что файл лежит в одной из двух разрешённых папок.
//
// Их именно две, и раньше проверялась только первая: APP_IMPORT_ROOT — папка на
// сервере, которую сканирует импорт по пути, а uploadDir — куда складывает
// кнопка «Импортировать». В типовой установке APP_IMPORT_ROOT не задан вообще,
// поэтому каждый загруженный через кнопку трек получал 403.
func (a *App) resolveLocalFile(path string) (string, error) {
	var lastErr error
	for _, root := range []string{os.Getenv(importRootEnv), a.uploadDir()} {
		safe, err := resolveImportPath(root, path)
		if err == nil {
			return safe, nil
		}
		lastErr = err
	}
	return "", lastErr
}

func (a *App) serveLocalFile(w http.ResponseWriter, r *http.Request) {
	fingerprint := r.PathValue("fingerprint")
	path, ok := a.store.LocalFilePath(a.optionalUserIDFromRequest(r), "local:"+fingerprint)
	if !ok {
		writeError(w, http.StatusNotFound, "Файл не найден")
		return
	}
	safe, err := a.resolveLocalFile(path)
	if err != nil {
		writeError(w, http.StatusForbidden, "Файл вне разрешённой папки")
		return
	}
	http.ServeFile(w, r, safe)
}

// --- Загрузка файлов из браузера ----------------------------------------

// maxUploadFileSize bounds a single file. A lossless album track runs to tens of
// megabytes; anything past this is not music someone meant to upload.
const maxUploadFileSize = 512 << 20

// safeUploadName strips everything except the base name.
//
// The filename arrives from the browser, and with `webkitdirectory` it carries
// the relative path ("Artist/Album/01.mp3"). Joining that onto a directory would
// let a crafted client write outside it, so only the final component survives
// and anything still suspicious is rejected.
func safeUploadName(name string) (string, bool) {
	// Windows clients may send backslashes, which filepath.Base does not treat
	// as separators on Unix; normalise first so the base name is really the base.
	base := filepath.Base(strings.ReplaceAll(name, `\`, "/"))
	if base == "." || base == ".." || base == string(os.PathSeparator) || strings.TrimSpace(base) == "" {
		return "", false
	}
	if strings.ContainsAny(base, `/\`) || strings.Contains(base, "..") {
		return "", false
	}
	return base, true
}

// uploadDir is where browser uploads land: inside the media root the server
// already owns, so no extra configuration is needed for the common case.
func (a *App) uploadDir() string { return filepath.Join(a.cfg.MediaRoot, "imported") }

// importUpload accepts files or a whole folder from the browser.
//
// It reads the multipart body as a stream rather than through ParseMultipartForm:
// a dropped folder can be gigabytes, and buffering that in memory would take the
// server down on the small machines this project targets.
func (a *App) importUpload(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdmin(w, r) {
		return
	}
	reader, err := r.MultipartReader()
	if err != nil {
		writeError(w, http.StatusBadRequest, "Ожидалась multipart-загрузка")
		return
	}
	if err := os.MkdirAll(a.uploadDir(), 0o755); err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось создать папку для загрузок")
		return
	}

	result := ImportResult{Counts: map[string]int{}}
	files := []LocalFile{}
	start := time.Now()

	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			writeError(w, http.StatusBadRequest, "Загрузка прервалась: "+err.Error())
			return
		}
		if part.FormName() != "files" || part.FileName() == "" {
			_ = part.Close()
			continue
		}
		original := part.FileName()
		name, ok := safeUploadName(original)
		if !ok {
			result.Skipped = append(result.Skipped, ImportSkip{Path: original, Reason: "недопустимое имя файла"})
			_ = part.Close()
			continue
		}
		ext := strings.ToLower(filepath.Ext(name))
		if ext == drmExtension {
			result.Skipped = append(result.Skipped, ImportSkip{
				Path:   name,
				Reason: "Файл защищён DRM (FairPlay). Перекачайте трек из Apple Music — с 2009 года покупки идут без защиты.",
			})
			_ = part.Close()
			continue
		}
		if !importableExtensions[ext] {
			_ = part.Close()
			continue // не аудио — молча пропускаем, их в папке бывает много
		}

		stored, size, err := a.storeUpload(part, name)
		_ = part.Close()
		if err != nil {
			result.Skipped = append(result.Skipped, ImportSkip{Path: name, Reason: err.Error()})
			continue
		}
		result.Scanned++
		result.Counts[strings.TrimPrefix(ext, ".")]++

		fingerprint, err := fileFingerprint(stored, size)
		if err != nil {
			result.Skipped = append(result.Skipped, ImportSkip{Path: name, Reason: "не удалось прочитать файл"})
			continue
		}
		title, artist, album, duration := probeTags(r.Context(), stored)
		if title == "" {
			title = strings.TrimSuffix(name, filepath.Ext(name))
		}
		files = append(files, LocalFile{
			TrackID: "local:" + fingerprint, Path: stored, Size: size,
			Track: Track{
				ID: "local:" + fingerprint, ProviderID: "local", ProviderTrackID: fingerprint,
				Title: title, Artist: artist, Album: album, DurationSeconds: duration,
				SourceURL: "/v1/local/" + fingerprint, Attribution: "Загружено",
				Capabilities: localCaps(), Policy: localPolicy(),
			},
		})
	}

	imported, duplicate, err := a.store.BulkImportLocalFiles(userIDFromRequest(r), files)
	if err != nil {
		slog.Warn("upload import failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Не удалось сохранить загруженные треки")
		return
	}
	result.Imported, result.Duplicate = imported, duplicate
	result.Elapsed = time.Since(start).Round(time.Millisecond).String()
	slog.Info("upload finished", "scanned", result.Scanned, "imported", imported, "duplicate", duplicate)
	writeJSON(w, http.StatusOK, result)
}

// storeUpload streams one part to disk, refusing anything over the size cap
// without having buffered it first.
func (a *App) storeUpload(part io.Reader, name string) (path string, size int64, err error) {
	path = filepath.Join(a.uploadDir(), name)
	// A repeat upload of the same name gets a suffix rather than overwriting
	// someone's existing file; duplicates are caught later by fingerprint.
	for i := 1; fileExists(path); i++ {
		ext := filepath.Ext(name)
		path = filepath.Join(a.uploadDir(), fmt.Sprintf("%s (%d)%s", strings.TrimSuffix(name, ext), i, ext))
	}
	dst, err := os.Create(path)
	if err != nil {
		return "", 0, fmt.Errorf("не удалось сохранить файл")
	}
	defer dst.Close()
	size, err = io.Copy(dst, io.LimitReader(part, maxUploadFileSize+1))
	if err != nil {
		_ = os.Remove(path)
		return "", 0, fmt.Errorf("ошибка чтения")
	}
	if size > maxUploadFileSize {
		_ = os.Remove(path)
		return "", 0, fmt.Errorf("файл больше %d МБ", maxUploadFileSize>>20)
	}
	return path, size, nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
