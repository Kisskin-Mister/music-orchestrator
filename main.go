package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type App struct {
	cfg       Config
	store     *Store
	providers *ProviderService
	mux       *http.ServeMux
}

func NewApp(cfg Config) (*App, error) {
	st, err := NewStore(cfg.StorePath)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(cfg.MediaRoot, 0755); err != nil {
		return nil, err
	}
	a := &App{cfg: cfg, store: st, providers: NewProviderService(cfg), mux: http.NewServeMux()}
	a.routes()
	return a, nil
}

func (a *App) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.withCORS(a.mux).ServeHTTP(w, r)
}

func (a *App) routes() {
	a.mux.HandleFunc("GET /health", a.health)
	a.mux.HandleFunc("GET /openapi.json", a.openapi)
	a.mux.HandleFunc("GET /v1/providers", a.listProviders)
	a.mux.HandleFunc("GET /v1/search", a.search)
	a.mux.HandleFunc("GET /v1/tracks/{track_id}", a.track)
	a.mux.HandleFunc("GET /v1/playback/{track_id}", a.playback)
	a.mux.HandleFunc("POST /v1/downloads", a.auth(a.download))
	a.mux.HandleFunc("GET /media/{filename}", a.media)
	a.mux.HandleFunc("POST /v1/favorites", a.auth(a.addFavorite))
	a.mux.HandleFunc("GET /v1/favorites", a.auth(a.listFavorites))
	a.mux.HandleFunc("DELETE /v1/favorites/{track_id}", a.auth(a.deleteFavorite))
	a.mux.HandleFunc("POST /v1/playlists", a.auth(a.createPlaylist))
	a.mux.HandleFunc("GET /v1/playlists", a.auth(a.listPlaylists))
	a.mux.HandleFunc("GET /v1/playlists/{playlist_id}", a.auth(a.getPlaylist))
	a.mux.HandleFunc("POST /v1/playlists/{playlist_id}/tracks", a.auth(a.addPlaylistTrack))
	a.mux.HandleFunc("GET /v1/jobs", a.auth(a.listJobs))
	a.mux.HandleFunc("GET /v1/jobs/{job_id}", a.auth(a.getJob))
}

func (a *App) withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		for _, allowed := range a.cfg.CORSOrigins {
			if origin == allowed || allowed == "*" {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
				break
			}
		}
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-API-Key")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *App) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("X-API-Key")
		if key == "" || !a.cfg.APIKeys[key] {
			writeError(w, http.StatusUnauthorized, "Invalid or missing X-API-Key")
			return
		}
		next(w, r)
	}
}

func (a *App) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "service": "music-orchestrator", "runtime": "go", "environment": a.cfg.Environment, "risky_extractors_enabled": a.cfg.EnableRiskyExtractors})
}
func (a *App) openapi(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, OpenAPISchema())
}
func (a *App) listProviders(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, a.providers.Providers())
}

func (a *App) search(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	limit := queryInt(r, "limit", 20)
	offset := queryInt(r, "offset", 0)
	if limit < 1 || limit > 50 {
		limit = 20
	}
	providers := splitCSV(r.URL.Query().Get("providers"))
	items := a.providers.Search(q, providers, limit+offset)
	total := len(items)
	if offset > len(items) {
		items = []Track{}
	} else {
		items = items[offset:]
	}
	if len(items) > limit {
		items = items[:limit]
	}
	writeJSON(w, http.StatusOK, SearchResponse{Query: q, Limit: limit, Offset: offset, Total: total, Items: items})
}

func (a *App) track(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("track_id")
	t, ok := a.providers.Track(id)
	if !ok {
		writeError(w, http.StatusNotFound, "Track not found")
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (a *App) playback(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("track_id")
	providerID, pid, ok := splitTrackID(id)
	if !ok {
		writeError(w, http.StatusBadRequest, "Invalid track id")
		return
	}
	pr, ok := a.providers.byID(providerID)
	if !ok {
		writeError(w, http.StatusNotFound, "Provider not found")
		return
	}
	pb := Playback{TrackID: id, ProviderID: providerID, PlaybackType: "unavailable", Capabilities: pr.Capabilities, Policy: pr.Policy}
	switch providerID {
	case "local":
		url := "/media/demo-" + pid + ".mp3"
		pb.PlaybackType = "local_stream"
		pb.StreamURL = &url
		pb.Attribution = "Local demo"
	case "youtube_official":
		embed := "https://www.youtube.com/embed/" + pid
		pb.PlaybackType = "embed"
		pb.EmbedURL = &embed
		pb.Attribution = "YouTube"
	case "youtube_stream", "soundcloud_stream":
		if !a.cfg.EnableRiskyExtractors {
			writeJSON(w, http.StatusOK, pb)
			return
		}
		stream, err := a.providers.extractor.StreamURL(providerID, pid)
		if err != nil {
			slog.Warn("stream resolve failed", "track", id, "error", err)
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
		pb.PlaybackType = "extractor_stream"
		pb.StreamURL = &stream
		pb.ExpiresInSeconds = intPtr(21600)
		pb.Attribution = pr.Name
	}
	writeJSON(w, http.StatusOK, pb)
}

func (a *App) download(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TrackID string `json:"track_id"`
		Format  string `json:"format"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	providerID, pid, ok := splitTrackID(req.TrackID)
	if !ok {
		writeError(w, http.StatusBadRequest, "Invalid track id")
		return
	}
	job := Job{ID: newID("job"), Type: "download", Status: "running", TrackID: req.TrackID, Payload: map[string]any{"format": req.Format}, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if providerID != "youtube_stream" && providerID != "soundcloud_stream" {
		job.Status = "blocked_by_policy"
		job.Error = "downloads are only supported for extractor providers"
		_ = a.store.SaveJob(job)
		writeJSON(w, http.StatusAccepted, job)
		return
	}
	if !a.cfg.EnableRiskyExtractors {
		job.Status = "blocked_by_policy"
		job.Error = "risky extractors are disabled"
		_ = a.store.SaveJob(job)
		writeJSON(w, http.StatusAccepted, job)
		return
	}
	filename, size, err := a.providers.extractor.Download(providerID, pid, req.Format, a.cfg.MediaRoot)
	if err != nil {
		job.Status = "failed"
		job.Error = err.Error()
		_ = a.store.SaveJob(job)
		writeJSON(w, http.StatusAccepted, job)
		return
	}
	mediaURL := "/media/" + filename
	if a.cfg.PublicMediaBaseURL != "" {
		mediaURL = a.cfg.PublicMediaBaseURL + mediaURL
	}
	job.Status = "succeeded"
	job.Result = map[string]any{"provider_id": providerID, "provider_track_id": pid, "media_url": mediaURL, "bytes_written": size}
	_ = a.store.SaveJob(job)
	writeJSON(w, http.StatusAccepted, job)
}

func (a *App) media(w http.ResponseWriter, r *http.Request) {
	name := filepath.Base(r.PathValue("filename"))
	path := filepath.Join(a.cfg.MediaRoot, name)
	if _, err := os.Stat(path); err != nil {
		writeError(w, http.StatusNotFound, "Media not found")
		return
	}
	http.ServeFile(w, r, path)
}

func (a *App) addFavorite(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TrackID string `json:"track_id"`
	}
	if err := decodeJSON(r, &req); err != nil || req.TrackID == "" {
		writeError(w, 400, "Invalid favorite payload")
		return
	}
	f, err := a.store.AddFavorite(req.TrackID)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	writeJSON(w, 201, f)
}
func (a *App) listFavorites(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, a.store.ListFavorites())
}
func (a *App) deleteFavorite(w http.ResponseWriter, r *http.Request) {
	_ = a.store.DeleteFavorite(r.PathValue("track_id"))
	w.WriteHeader(204)
}
func (a *App) createPlaylist(w http.ResponseWriter, r *http.Request) {
	var req struct{ Name, Description string }
	if err := decodeJSON(r, &req); err != nil || strings.TrimSpace(req.Name) == "" {
		writeError(w, 400, "Invalid playlist payload")
		return
	}
	p, err := a.store.CreatePlaylist(req.Name, req.Description)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	writeJSON(w, 201, p)
}
func (a *App) listPlaylists(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, a.store.ListPlaylists())
}
func (a *App) getPlaylist(w http.ResponseWriter, r *http.Request) {
	p, ok := a.store.GetPlaylist(r.PathValue("playlist_id"))
	if !ok {
		writeError(w, 404, "Playlist not found")
		return
	}
	writeJSON(w, 200, p)
}
func (a *App) addPlaylistTrack(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TrackID string `json:"track_id"`
	}
	if err := decodeJSON(r, &req); err != nil || req.TrackID == "" {
		writeError(w, 400, "Invalid playlist track payload")
		return
	}
	p, err := a.store.AddPlaylistTrack(r.PathValue("playlist_id"), req.TrackID)
	if err != nil {
		writeError(w, 404, err.Error())
		return
	}
	writeJSON(w, 201, p)
}
func (a *App) listJobs(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 200, a.store.ListJobs()) }
func (a *App) getJob(w http.ResponseWriter, r *http.Request) {
	j, ok := a.store.GetJob(r.PathValue("job_id"))
	if !ok {
		writeError(w, 404, "Job not found")
		return
	}
	writeJSON(w, 200, j)
}
func queryInt(r *http.Request, key string, def int) int {
	if v := r.URL.Query().Get(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return def
}

func main() {
	cfg := LoadConfig()
	app, err := NewApp(cfg)
	if err != nil {
		panic(err)
	}
	slog.Info("music orchestrator listening", "addr", cfg.Addr, "risky_extractors", cfg.EnableRiskyExtractors)
	if err := http.ListenAndServe(cfg.Addr, app); err != nil && err != http.ErrServerClosed {
		panic(fmt.Errorf("listen: %w", err))
	}
}
