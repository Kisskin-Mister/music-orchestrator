package main

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const playlistCoverMaxBytes = 8 << 20

func playlistCoverExtension(data []byte) (string, bool) {
	switch http.DetectContentType(data) {
	case "image/jpeg":
		return ".jpg", true
	case "image/png":
		return ".png", true
	case "image/webp":
		return ".webp", true
	case "image/gif":
		return ".gif", true
	}
	if len(data) >= 12 && bytes.Equal(data[4:8], []byte("ftyp")) {
		brand := string(data[8:12])
		if brand == "heic" || brand == "heix" || brand == "hevc" || brand == "hevx" || brand == "mif1" {
			return ".heic", true
		}
	}
	return "", false
}

func (a *App) uploadPlaylistCover(w http.ResponseWriter, r *http.Request) {
	ownerID := userIDFromRequest(r)
	playlistID := r.PathValue("playlist_id")
	current, ok := a.store.GetPlaylist(ownerID, playlistID)
	if !ok {
		writeError(w, http.StatusNotFound, "Playlist not found")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, playlistCoverMaxBytes+(1<<20))
	if err := r.ParseMultipartForm(playlistCoverMaxBytes + (1 << 20)); err != nil {
		writeError(w, http.StatusBadRequest, "Cover must be an image up to 8 MiB")
		return
	}
	file, _, err := r.FormFile("cover")
	if err != nil {
		writeError(w, http.StatusBadRequest, "Multipart field 'cover' is required")
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, playlistCoverMaxBytes+1))
	if err != nil || len(data) == 0 || len(data) > playlistCoverMaxBytes {
		writeError(w, http.StatusBadRequest, "Cover must be an image up to 8 MiB")
		return
	}
	ext, ok := playlistCoverExtension(data)
	if !ok {
		writeError(w, http.StatusBadRequest, "Supported cover formats: JPEG, PNG, WebP, GIF or HEIC")
		return
	}

	tmp, err := os.CreateTemp(a.cfg.MediaRoot, ".playlist-cover-*")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Cannot store playlist cover")
		return
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err = tmp.Write(data); err != nil {
		_ = tmp.Close()
		writeError(w, http.StatusInternalServerError, "Cannot store playlist cover")
		return
	}
	if err = tmp.Close(); err != nil {
		writeError(w, http.StatusInternalServerError, "Cannot store playlist cover")
		return
	}
	filename := safeStem("playlist-cover", playlistID) + ext
	path := filepath.Join(a.cfg.MediaRoot, filename)
	if err = os.Rename(tmpName, path); err != nil {
		writeError(w, http.StatusInternalServerError, "Cannot store playlist cover")
		return
	}

	coverURL := "/media/" + filename
	updated, found, err := a.store.UpdatePlaylist(ownerID, playlistID, PlaylistUpdate{CoverURL: &coverURL})
	if err != nil || !found {
		_ = os.Remove(path)
		writeError(w, http.StatusInternalServerError, "Cannot update playlist cover")
		return
	}
	if strings.HasPrefix(current.CoverURL, "/media/playlist-cover-") && current.CoverURL != coverURL {
		_ = os.Remove(filepath.Join(a.cfg.MediaRoot, filepath.Base(current.CoverURL)))
	}
	writeJSON(w, http.StatusOK, publicPlaylist(updated))
}
