package main

import (
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// web serves the Flutter Web build from the same origin as the API. This keeps
// phone testing to one URL and lets session cookies work without CORS setup.
func (a *App) web(w http.ResponseWriter, r *http.Request) {
	root := strings.TrimSpace(a.cfg.WebRoot)
	if root == "" {
		http.NotFound(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/v1/") || strings.HasPrefix(r.URL.Path, "/media/") {
		http.NotFound(w, r)
		return
	}

	cleanURLPath := path.Clean("/" + r.URL.Path)
	relative := strings.TrimPrefix(cleanURLPath, "/")
	if relative == "" {
		relative = "index.html"
	}
	candidate := filepath.Join(root, filepath.FromSlash(relative))
	info, err := os.Stat(candidate)
	if err == nil && info.IsDir() {
		candidate = filepath.Join(candidate, "index.html")
		_, err = os.Stat(candidate)
	}
	if err != nil {
		// Routes such as /playlists belong to the client-side Flutter router.
		// Missing files with an extension are real 404s, not SPA routes.
		if filepath.Ext(relative) != "" {
			http.NotFound(w, r)
			return
		}
		candidate = filepath.Join(root, "index.html")
		if _, indexErr := os.Stat(candidate); indexErr != nil {
			http.NotFound(w, r)
			return
		}
	}

	if filepath.Base(candidate) == "index.html" {
		w.Header().Set("Cache-Control", "no-cache")
	}
	http.ServeFile(w, r, candidate)
}
