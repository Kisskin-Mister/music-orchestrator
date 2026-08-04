package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type contextKey string

const userIDContextKey contextKey = "user_id"

type App struct {
	cfg Config
	// baseCfg is the env-derived config. Stored overrides are re-applied on top
	// of it, so clearing an override restores the .env value rather than
	// whatever happened to be active.
	baseCfg   Config
	store     *Store
	providers *ProviderService
	sessions  *sessionStore
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
	effective := st.StoredSettings().apply(cfg)
	a := &App{cfg: effective, baseCfg: cfg, store: st, providers: NewProviderService(effective), sessions: newSessionStore(), mux: http.NewServeMux()}
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
	a.mux.HandleFunc("GET /v1/auth/session", a.authSessionInfo)
	a.mux.HandleFunc("POST /v1/auth/register", a.authRegister)
	a.mux.HandleFunc("POST /v1/auth/login", a.authLogin)
	a.mux.HandleFunc("POST /v1/auth/verify", a.authVerify)
	a.mux.HandleFunc("POST /v1/auth/logout", a.authLogout)
	a.mux.HandleFunc("GET /v1/auth/me", a.auth(a.authMe))
	a.mux.HandleFunc("PATCH /v1/account", a.auth(a.updateAccount))
	a.mux.HandleFunc("GET /v1/settings", a.auth(a.getSettings))
	a.mux.HandleFunc("PATCH /v1/settings", a.auth(a.updateSettings))
	a.mux.HandleFunc("GET /v1/users", a.auth(a.listUsers))
	a.mux.HandleFunc("POST /v1/users", a.auth(a.createUser))
	a.mux.HandleFunc("PATCH /v1/users/{user_id}", a.auth(a.updateUser))
	a.mux.HandleFunc("DELETE /v1/users/{user_id}", a.auth(a.deleteUser))
	a.mux.HandleFunc("GET /v1/search", a.search)
	a.mux.HandleFunc("GET /v1/tracks/{track_id}", a.track)
	a.mux.HandleFunc("GET /v1/playback/{track_id}", a.playback)
	a.mux.HandleFunc("GET /v1/stream/{track_id}", a.stream)
	a.mux.HandleFunc("GET /v1/downloads", a.auth(a.listDownloads))
	a.mux.HandleFunc("POST /v1/downloads", a.auth(a.download))
	a.mux.HandleFunc("DELETE /v1/downloads/{track_id}", a.auth(a.deleteDownload))
	a.mux.HandleFunc("GET /media/{filename}", a.media)
	a.mux.HandleFunc("GET /v1/artwork", a.artwork)
	a.mux.HandleFunc("POST /v1/favorites", a.auth(a.addFavorite))
	a.mux.HandleFunc("GET /v1/favorites", a.auth(a.listFavorites))
	a.mux.HandleFunc("DELETE /v1/favorites/{track_id}", a.auth(a.deleteFavorite))
	a.mux.HandleFunc("POST /v1/playlists", a.auth(a.createPlaylist))
	a.mux.HandleFunc("GET /v1/playlists", a.auth(a.listPlaylists))
	a.mux.HandleFunc("GET /v1/playlists/{playlist_id}", a.auth(a.getPlaylist))
	a.mux.HandleFunc("PATCH /v1/playlists/{playlist_id}", a.auth(a.updatePlaylist))
	a.mux.HandleFunc("POST /v1/playlists/{playlist_id}/cover", a.auth(a.uploadPlaylistCover))
	a.mux.HandleFunc("DELETE /v1/playlists/{playlist_id}", a.auth(a.deletePlaylist))
	a.mux.HandleFunc("POST /v1/playlists/{playlist_id}/tracks", a.auth(a.addPlaylistTrack))
	a.mux.HandleFunc("DELETE /v1/playlists/{playlist_id}/tracks/{track_id}", a.auth(a.removePlaylistTrack))
	a.mux.HandleFunc("GET /v1/jobs", a.auth(a.listJobs))
	a.mux.HandleFunc("GET /v1/jobs/{job_id}", a.auth(a.getJob))
	a.mux.HandleFunc("GET /", a.web)
}

func (a *App) withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		for _, allowed := range a.cfg.CORSOrigins {
			if origin == allowed || allowed == "*" {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Vary", "Origin")
				break
			}
		}
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-API-Key")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *App) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, authType, ok := a.currentIdentity(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "Invalid or missing credentials (X-API-Key or session)")
			return
		}
		ctx := context.WithValue(r.Context(), userIDContextKey, userID)
		ctx = context.WithValue(ctx, authTypeContextKey, authType)
		next(w, r.WithContext(ctx))
	}
}

func apiKeyUserID(key string) string {
	sum := sha256.Sum256([]byte(key))
	return "api_" + hex.EncodeToString(sum[:])[:24]
}

func userIDFromRequest(r *http.Request) string {
	if userID, ok := r.Context().Value(userIDContextKey).(string); ok && userID != "" {
		return userID
	}
	return "anonymous"
}

func (a *App) optionalUserIDFromRequest(r *http.Request) string {
	if userID, _, ok := a.currentIdentity(r); ok {
		return userID
	}
	return "anonymous"
}

func publicFavorite(f Favorite) Favorite { f.OwnerID = ""; return f }
func publicPlaylist(p Playlist) Playlist { p.OwnerID = ""; return p }
func publicJob(j Job) Job                { j.OwnerID = ""; return j }

func (a *App) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "service": "music-orchestrator", "runtime": "go", "environment": a.cfg.Environment, "risky_extractors_enabled": a.cfg.EnableRiskyExtractors})
}
func (a *App) openapi(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, OpenAPISchema())
}
func (a *App) listProviders(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, a.providers.Providers())
}

func (a *App) authMe(w http.ResponseWriter, r *http.Request) {
	username, role, _, _, _ := a.store.AccountByID(userIDFromRequest(r))
	writeJSON(w, http.StatusOK, map[string]any{"id": userIDFromRequest(r), "username": username, "role": role, "auth_type": authTypeFromRequest(r)})
}

func (a *App) requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	_, role, _, _, ok := a.store.AccountByID(userIDFromRequest(r))
	if !ok || role != "admin" {
		writeErrorCode(w, http.StatusForbidden, "admin_required", "Admin access required")
		return false
	}
	return true
}

func (a *App) updateAccount(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdmin(w, r) {
		return
	}
	var req struct {
		Username   string  `json:"username"`
		Password   string  `json:"password"`
		TOTPSecret *string `json:"totp_secret"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	username := strings.TrimSpace(req.Username)
	if username == "" {
		writeError(w, http.StatusBadRequest, "Username is required")
		return
	}
	passwordHash := ""
	if req.Password != "" {
		if len(req.Password) < passwordMinLength {
			writeError(w, http.StatusBadRequest, "Password must be at least 10 characters")
			return
		}
		var err error
		passwordHash, err = hashPassword(req.Password)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Failed to hash password")
			return
		}
	}
	totp := ""
	if req.TOTPSecret != nil {
		totp = normalizeTOTPSecret(*req.TOTPSecret)
	} else if _, _, _, existingTOTP, ok := a.store.AccountByID(userIDFromRequest(r)); ok {
		totp = existingTOTP
	}
	if !validTOTPSecret(totp) {
		writeErrorCode(w, http.StatusBadRequest, "invalid_totp_secret", "TOTP secret must be valid base32")
		return
	}
	owner, found, err := a.store.UpdateOwnerAccount(userIDFromRequest(r), username, passwordHash, totp)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to save account")
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "Account not found")
		return
	}
	writeJSON(w, http.StatusOK, sessionInfoResponse{Authenticated: true, UserID: owner.ID, Username: owner.Username, Role: "admin", AuthType: "session", LoginEnabled: true, TOTPEnabled: owner.TOTPSecret != ""})
}

func (a *App) listUsers(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdmin(w, r) {
		return
	}
	writeJSON(w, http.StatusOK, a.store.ListUsers())
}

func (a *App) createUser(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdmin(w, r) {
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	username := strings.TrimSpace(req.Username)
	if username == "" {
		writeError(w, http.StatusBadRequest, "Username is required")
		return
	}
	if len(req.Password) < passwordMinLength {
		writeError(w, http.StatusBadRequest, "Password must be at least 10 characters")
		return
	}
	h, err := hashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to hash password")
		return
	}
	u, err := a.store.CreateUser(username, h)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to save user")
		return
	}
	writeJSON(w, http.StatusCreated, u)
}

func (a *App) updateUser(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdmin(w, r) {
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	passwordHash := ""
	if req.Password != "" {
		if len(req.Password) < passwordMinLength {
			writeError(w, http.StatusBadRequest, "Password must be at least 10 characters")
			return
		}
		var err error
		passwordHash, err = hashPassword(req.Password)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Failed to hash password")
			return
		}
	}
	u, found, err := a.store.UpdateUser(r.PathValue("user_id"), req.Username, passwordHash)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to save user")
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "User not found")
		return
	}
	writeJSON(w, http.StatusOK, u)
}

func (a *App) deleteUser(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdmin(w, r) {
		return
	}
	if !a.store.DeleteUser(r.PathValue("user_id")) {
		writeError(w, http.StatusNotFound, "User not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
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
	items = a.annotateTracks(a.optionalUserIDFromRequest(r), items)
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

func (a *App) annotateTracks(ownerID string, items []Track) []Track {
	for i := range items {
		items[i] = a.annotateTrack(ownerID, items[i])
	}
	return items
}

func (a *App) annotateTrack(ownerID string, t Track) Track {
	if strings.Contains(strings.ToLower(t.Artist), " - topic") {
		t.Official = true
	}
	if j, ok := a.store.FindSuccessfulDownload(ownerID, t.ProviderID, t.ProviderTrackID); ok {
		if mediaURL, ok := j.Result["media_url"].(string); ok && mediaURL != "" {
			t.Downloaded = true
			t.DownloadMediaURL = mediaURL
		}
	}
	return t
}

func (a *App) track(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("track_id")
	t, ok := a.providers.Track(id)
	if !ok {
		writeError(w, http.StatusNotFound, "Track not found")
		return
	}
	writeJSON(w, http.StatusOK, a.annotateTrack(a.optionalUserIDFromRequest(r), t))
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
		if j, ok := a.store.FindSuccessfulDownload(a.optionalUserIDFromRequest(r), providerID, pid); ok {
			if mediaURL, ok := j.Result["media_url"].(string); ok && mediaURL != "" {
				pb.PlaybackType = "local_cached_stream"
				pb.StreamURL = &mediaURL
				pb.Attribution = pr.Name + " · saved copy"
				writeJSON(w, http.StatusOK, pb)
				return
			}
		}
		if !a.cfg.EnableRiskyExtractors {
			writeJSON(w, http.StatusOK, pb)
			return
		}
		// Devices play through this backend instead of opening a short-lived CDN
		// URL directly. Besides respecting the host's proxy settings, this lets us
		// choose an iOS-compatible M4A/AAC format in one place.
		stream := "/v1/stream/" + id
		pb.PlaybackType = "extractor_stream"
		pb.StreamURL = &stream
		pb.Attribution = pr.Name
	}
	writeJSON(w, http.StatusOK, pb)
}

func (a *App) stream(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("track_id")
	providerID, pid, ok := splitTrackID(id)
	if !ok || (providerID != "youtube_stream" && providerID != "soundcloud_stream") {
		writeError(w, http.StatusBadRequest, "Invalid extractor track id")
		return
	}
	if !a.cfg.EnableRiskyExtractors {
		writeError(w, http.StatusForbidden, "risky extractors are disabled")
		return
	}

	target, err := a.providers.extractor.StreamURL(providerID, pid)
	if err != nil {
		slog.Warn("stream resolve failed", "track", id, "error", err)
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	upstream, err := http.NewRequestWithContext(r.Context(), http.MethodGet, target, nil)
	if err != nil {
		writeError(w, http.StatusBadGateway, "Cannot build upstream stream request")
		return
	}
	for _, name := range []string{"Range", "If-Range", "If-None-Match", "If-Modified-Since"} {
		if value := r.Header.Get(name); value != "" {
			upstream.Header.Set(name, value)
		}
	}
	upstream.Header.Set("Accept", "audio/*,*/*;q=0.8")
	upstream.Header.Set("User-Agent", "MusicOrchestrator/1.0")

	resp, err := http.DefaultClient.Do(upstream)
	if err != nil {
		slog.Warn("stream proxy failed", "track", id, "error", err)
		writeError(w, http.StatusBadGateway, "Audio source is unavailable")
		return
	}
	defer resp.Body.Close()
	for _, name := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges", "ETag", "Last-Modified", "Cache-Control"} {
		if value := resp.Header.Get(name); value != "" {
			w.Header().Set(name, value)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func (a *App) download(w http.ResponseWriter, r *http.Request) {
	ownerID := userIDFromRequest(r)
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
		_ = a.store.SaveJob(ownerID, job)
		writeJSON(w, http.StatusAccepted, publicJob(job))
		return
	}
	if !a.cfg.EnableRiskyExtractors {
		job.Status = "blocked_by_policy"
		job.Error = "risky extractors are disabled"
		_ = a.store.SaveJob(ownerID, job)
		writeJSON(w, http.StatusAccepted, publicJob(job))
		return
	}
	if cached, ok := a.store.FindSuccessfulDownload(ownerID, providerID, pid); ok {
		if cached.Payload == nil {
			cached.Payload = map[string]any{}
		}
		cached.Payload["cached"] = true
		writeJSON(w, http.StatusOK, cached)
		return
	}
	if t, ok := a.providers.Track(req.TrackID); ok {
		t = a.annotateTrack(ownerID, t)
		job.Payload["track"] = sanitizeTrackForStorage(t)
	}
	filename, size, err := a.providers.extractor.Download(providerID, pid, req.Format, a.cfg.MediaRoot)
	if err != nil {
		job.Status = "failed"
		job.Error = err.Error()
		_ = a.store.SaveJob(ownerID, job)
		writeJSON(w, http.StatusAccepted, publicJob(job))
		return
	}
	mediaURL := "/media/" + filename
	if a.cfg.PublicMediaBaseURL != "" {
		mediaURL = a.cfg.PublicMediaBaseURL + mediaURL
	}
	job.Status = "succeeded"
	job.Result = map[string]any{"provider_id": providerID, "provider_track_id": pid, "media_url": mediaURL, "bytes_written": size}
	if track, ok := job.Payload["track"]; ok {
		job.Result["track"] = track
	}
	_ = a.store.SaveJob(ownerID, job)
	writeJSON(w, http.StatusAccepted, publicJob(job))
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

func (a *App) listDownloads(w http.ResponseWriter, r *http.Request) {
	ownerID := userIDFromRequest(r)
	jobs := a.store.SuccessfulDownloads(ownerID)
	for i := range jobs {
		jobs[i] = a.enrichDownloadJob(jobs[i])
	}
	for i := range jobs {
		jobs[i] = publicJob(jobs[i])
	}
	writeJSON(w, http.StatusOK, jobs)
}

func (a *App) enrichDownloadJob(j Job) Job {
	if j.Result == nil {
		j.Result = map[string]any{}
	}
	if _, ok := j.Result["track"]; ok {
		return j
	}
	if t, ok := a.providers.Track(j.TrackID); ok {
		t = a.annotateTrack(j.OwnerID, t)
		j.Result["track"] = sanitizeTrackForStorage(t)
	}
	return j
}

func (a *App) deleteDownload(w http.ResponseWriter, r *http.Request) {
	trackID := r.PathValue("track_id")
	jobs := a.store.DeleteDownloadsByTrack(userIDFromRequest(r), trackID)
	for _, job := range jobs {
		if job.Result == nil {
			continue
		}
		if mediaURL, ok := job.Result["media_url"].(string); ok && strings.HasPrefix(mediaURL, "/media/") {
			_ = os.Remove(filepath.Join(a.cfg.MediaRoot, filepath.Base(mediaURL)))
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) addFavorite(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TrackID string `json:"track_id"`
	}
	if err := decodeJSON(r, &req); err != nil || req.TrackID == "" {
		writeError(w, 400, "Invalid favorite payload")
		return
	}
	t, ok := a.providers.Track(req.TrackID)
	if !ok {
		writeError(w, 404, "Track not found")
		return
	}
	f, err := a.store.AddFavorite(userIDFromRequest(r), a.annotateTrack(userIDFromRequest(r), t))
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	writeJSON(w, 201, publicFavorite(f))
}
func (a *App) listFavorites(w http.ResponseWriter, r *http.Request) {
	ownerID := userIDFromRequest(r)
	items := a.store.ListFavorites(ownerID)
	for i := range items {
		if items[i].Track != nil {
			continue
		}
		if t, ok := a.providers.Track(items[i].TrackID); ok {
			t = a.annotateTrack(ownerID, t)
			items[i].Track = &t
		}
	}
	for i := range items {
		items[i] = publicFavorite(items[i])
	}
	writeJSON(w, 200, items)
}
func (a *App) deleteFavorite(w http.ResponseWriter, r *http.Request) {
	_ = a.store.DeleteFavorite(userIDFromRequest(r), r.PathValue("track_id"))
	w.WriteHeader(204)
}
func (a *App) createPlaylist(w http.ResponseWriter, r *http.Request) {
	var req struct{ Name, Description string }
	if err := decodeJSON(r, &req); err != nil || strings.TrimSpace(req.Name) == "" {
		writeError(w, 400, "Invalid playlist payload")
		return
	}
	p, err := a.store.CreatePlaylist(userIDFromRequest(r), req.Name, req.Description)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	writeJSON(w, 201, publicPlaylist(p))
}
func (a *App) listPlaylists(w http.ResponseWriter, r *http.Request) {
	playlists := a.store.ListPlaylists(userIDFromRequest(r))
	for i := range playlists {
		playlists[i] = publicPlaylist(playlists[i])
	}
	writeJSON(w, 200, playlists)
}
func (a *App) getPlaylist(w http.ResponseWriter, r *http.Request) {
	p, ok := a.store.GetPlaylist(userIDFromRequest(r), r.PathValue("playlist_id"))
	if !ok {
		writeError(w, 404, "Playlist not found")
		return
	}
	writeJSON(w, 200, publicPlaylist(p))
}
func (a *App) updatePlaylist(w http.ResponseWriter, r *http.Request) {
	var req PlaylistUpdate
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, 400, "Invalid playlist payload")
		return
	}
	p, found, err := a.store.UpdatePlaylist(userIDFromRequest(r), r.PathValue("playlist_id"), req)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	if !found {
		writeError(w, 404, "Playlist not found")
		return
	}
	writeJSON(w, 200, publicPlaylist(p))
}
func (a *App) deletePlaylist(w http.ResponseWriter, r *http.Request) {
	if !a.store.DeletePlaylist(userIDFromRequest(r), r.PathValue("playlist_id")) {
		writeError(w, 404, "Playlist not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (a *App) addPlaylistTrack(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TrackID string `json:"track_id"`
	}
	if err := decodeJSON(r, &req); err != nil || req.TrackID == "" {
		writeError(w, 400, "Invalid playlist track payload")
		return
	}
	track, ok := a.providers.Track(req.TrackID)
	if !ok {
		writeError(w, 404, "Track not found")
		return
	}
	track = a.annotateTrack(userIDFromRequest(r), track)
	p, added, err := a.store.AddPlaylistTrack(userIDFromRequest(r), r.PathValue("playlist_id"), track)
	if err != nil {
		writeError(w, 404, err.Error())
		return
	}
	if added {
		writeJSON(w, 201, publicPlaylist(p))
		return
	}
	writeJSON(w, 200, publicPlaylist(p))
}
func (a *App) removePlaylistTrack(w http.ResponseWriter, r *http.Request) {
	p, ok, err := a.store.RemovePlaylistTrack(userIDFromRequest(r), r.PathValue("playlist_id"), r.PathValue("track_id"))
	if err != nil || !ok {
		writeError(w, 404, "Playlist track not found")
		return
	}
	writeJSON(w, 200, publicPlaylist(p))
}
func (a *App) listJobs(w http.ResponseWriter, r *http.Request) {
	jobs := a.store.ListJobs(userIDFromRequest(r))
	for i := range jobs {
		jobs[i] = publicJob(jobs[i])
	}
	writeJSON(w, 200, jobs)
}
func (a *App) getJob(w http.ResponseWriter, r *http.Request) {
	j, ok := a.store.GetJob(userIDFromRequest(r), r.PathValue("job_id"))
	if !ok {
		writeError(w, 404, "Job not found")
		return
	}
	writeJSON(w, 200, publicJob(j))
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
