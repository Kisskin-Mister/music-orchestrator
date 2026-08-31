package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The import root is the whole security boundary here: without it, an admin
// session would be an arbitrary file-read primitive.
func TestImportPathStaysInsideRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "music"), 0o755); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, escape := range []string{
		"../",
		"../../etc",
		filepath.Join(root, "..", filepath.Base(outside)),
		outside,
		"music/../../..",
	} {
		if got, err := resolveImportPath(root, escape); err == nil {
			t.Errorf("escape %q was accepted and resolved to %q", escape, got)
		}
	}

	// A real subdirectory must still work.
	if _, err := resolveImportPath(root, "music"); err != nil {
		t.Fatalf("legitimate subdirectory rejected: %v", err)
	}
}

// A symlink pointing outside is the classic bypass: the string looks like a
// child of the root until you follow it.
func TestImportRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	link := filepath.Join(root, "escape")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if got, err := resolveImportPath(root, link); err == nil {
		t.Fatalf("symlink escape accepted, resolved to %q", got)
	}
}

func TestImportRequiresConfiguredRoot(t *testing.T) {
	if _, err := resolveImportPath("", "/anything"); err == nil {
		t.Fatal("scanning must be refused when no import root is configured")
	}
}

// DRM-protected files are reported rather than silently ignored, so the user
// learns why a chunk of their iTunes library did not appear.
func TestImportReportsDRMFiles(t *testing.T) {
	root := t.TempDir()
	write := func(name string, body []byte) {
		if err := os.WriteFile(filepath.Join(root, name), body, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("protected.m4p", []byte("drm"))
	write("plain.mp3", make([]byte, 4096))
	write("notes.txt", []byte("ignore me"))

	app := testApp(t, false)
	result, files := app.scanImportRoot(t.Context(), root)

	if result.Scanned != 1 || len(files) != 1 {
		t.Fatalf("scanned %d files, catalogued %d — expected exactly the mp3", result.Scanned, len(files))
	}
	if len(result.Skipped) != 1 || filepath.Ext(result.Skipped[0].Path) != ".m4p" {
		t.Fatalf("expected the .m4p to be reported, got %+v", result.Skipped)
	}
}

// Re-running a scan must not duplicate the library.
func TestImportIsIdempotent(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "song.mp3"), make([]byte, 8192), 0o600); err != nil {
		t.Fatal(err)
	}
	app := testApp(t, false)
	_, files := app.scanImportRoot(t.Context(), root)

	imported, duplicate, err := app.store.BulkImportLocalFiles("owner", files)
	if err != nil || imported != 1 || duplicate != 0 {
		t.Fatalf("first import: %d imported, %d duplicate, err %v", imported, duplicate, err)
	}
	imported, duplicate, err = app.store.BulkImportLocalFiles("owner", files)
	if err != nil || imported != 0 || duplicate != 1 {
		t.Fatalf("second import must be a no-op: %d imported, %d duplicate, err %v", imported, duplicate, err)
	}
	if got := app.store.ListFavorites("owner"); len(got) != 1 {
		t.Fatalf("library holds %d tracks after two scans, want 1", len(got))
	}
}

// One account must not be able to read another's imported files.
func TestLocalFileIsScopedToOwner(t *testing.T) {
	app := testApp(t, false)
	files := []LocalFile{{TrackID: "local:abc", Path: "/tmp/a.mp3", Track: Track{ID: "local:abc", Title: "A"}}}
	if _, _, err := app.store.BulkImportLocalFiles("alice", files); err != nil {
		t.Fatal(err)
	}
	if _, ok := app.store.LocalFilePath("bob", "local:abc"); ok {
		t.Fatal("bob can resolve alice's imported file")
	}
	if _, ok := app.store.LocalFilePath("alice", "local:abc"); !ok {
		t.Fatal("alice cannot resolve her own file")
	}
}

// The browser sends a relative path as the filename when a whole folder is
// picked ("Artist/Album/01.mp3"). The security property is not that odd input
// gets rejected — it is that whatever comes back can only name a file directly
// inside the upload directory.
func TestSafeUploadNameCannotEscapeDirectory(t *testing.T) {
	for _, in := range []string{
		"Artist/Album/01.mp3",
		`Artist\Album\01.mp3`,
		"song.mp3",
		"Альбом/трек.mp3",
		"../../etc/passwd",
		"../secret.mp3",
		"/etc/shadow",
	} {
		got, ok := safeUploadName(in)
		if !ok {
			continue // rejecting outright is also a safe outcome
		}
		if strings.ContainsAny(got, `/\`) || strings.Contains(got, "..") {
			t.Errorf("safeUploadName(%q) = %q — still contains a path", in, got)
		}
		// The decisive check: joining the result must stay inside the directory.
		dir := "/srv/uploads"
		if joined := filepath.Join(dir, got); !strings.HasPrefix(joined, dir+"/") {
			t.Errorf("safeUploadName(%q) = %q escapes to %q", in, got, joined)
		}
	}

	// Names that resolve to nothing usable must be refused.
	for _, bad := range []string{"..", ".", "", "   ", "/"} {
		if got, ok := safeUploadName(bad); ok {
			t.Errorf("safeUploadName(%q) accepted as %q", bad, got)
		}
	}

	// A normal nested path keeps the real file name.
	if got, _ := safeUploadName("Artist/Album/01.mp3"); got != "01.mp3" {
		t.Errorf("nested path lost its file name: %q", got)
	}
}

// Загруженный кнопкой импорта трек должен играть.
//
// Кнопка кладёт файлы в media/imported, а не в APP_IMPORT_ROOT — та папка нужна
// для сканирования серверной директории и в типовой установке вообще не задана.
// Проверяем весь путь до звука: /v1/playback отдаёт ссылку, /v1/local отдаёт байты.
func TestUploadedTrackPlays(t *testing.T) {
	app := testApp(t, false)
	owner := apiKeyUserID("test-key")
	if err := os.MkdirAll(app.uploadDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(app.uploadDir(), "song.mp3")
	if err := os.WriteFile(path, []byte("ID3 audio bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	files := []LocalFile{{
		TrackID: "local:abc", Path: path, Size: 15,
		Track: Track{ID: "local:abc", ProviderID: "local", ProviderTrackID: "abc",
			Title: "Загруженный трек", SourceURL: "/v1/local/abc",
			Capabilities: localCaps(), Policy: localPolicy()},
	}}
	if _, _, err := app.store.BulkImportLocalFiles(owner, files); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	app.mux.ServeHTTP(rec, authReq("GET", "/v1/playback/local:abc", "", "test-key"))
	if rec.Code != http.StatusOK {
		t.Fatalf("playback: HTTP %d — %s", rec.Code, rec.Body.String())
	}
	var pb struct {
		StreamURL    string `json:"stream_url"`
		PlaybackType string `json:"playback_type"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &pb); err != nil {
		t.Fatal(err)
	}
	if pb.StreamURL != "/v1/local/abc" {
		t.Fatalf("playback ссылается на %q вместо загруженного файла (playback_type=%q)", pb.StreamURL, pb.PlaybackType)
	}

	rec = httptest.NewRecorder()
	app.mux.ServeHTTP(rec, authReq("GET", pb.StreamURL, "", "test-key"))
	if rec.Code != http.StatusOK {
		t.Fatalf("отдача файла: HTTP %d — %s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "ID3 audio bytes" {
		t.Fatalf("отдано %q", rec.Body.String())
	}
}
