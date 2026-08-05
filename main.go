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
	hls       *hlsCache
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
	a := &App{cfg: effective, baseCfg: cfg, store: st, providers: NewProviderService(effective), sessions: newSessionStore(), hls: newHLSCache(effective), mux: http.NewServeMux()}
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

// upstreamChunkSize is why this proxy exists at all.
//
// googlevideo throttles a single long-lived connection: the first megabyte
// arrives at full speed and then throughput collapses to near zero, so a track
// plays for a couple of minutes and goes silent while the seek bar still shows
// the full length. Requesting the same file as a sequence of byte ranges resets
// that limit on every request — measured on a 6 MB track, one connection stalled
// at 850 KB while 2 MB ranges each completed in about a second.
//
// 4 MB balances the number of round trips against how much is re-fetched when a
// listener seeks away mid-chunk.
const upstreamChunkSize int64 = 4 << 20

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

	target, err := a.providers.extractor.StreamTarget(providerID, pid)
	if err != nil {
		slog.Warn("stream resolve failed", "track", id, "error", err)
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	if target.HLS {
		trackID := providerID + ":" + pid
		a.serveHLS(w, r, trackID, target)
		return
	}

	total, contentType, err := a.probeUpstream(r.Context(), target.URL, target.Headers)
	if err != nil {
		slog.Warn("stream probe failed", "track", id, "error", err)
		writeError(w, http.StatusBadGateway, "Audio source is unavailable")
		return
	}

	start, end, partial, ok := parseRange(r.Header.Get("Range"), total)
	if !ok {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", total))
		writeError(w, http.StatusRequestedRangeNotSatisfiable, "Invalid range")
		return
	}

	if contentType == "" {
		contentType = "audio/mpeg"
	}
	w.Header().Set("Content-Type", contentType)
	// Accept-Ranges is what lets the client seek at all; without it browsers
	// treat the stream as a live feed and disable the scrubber.
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Content-Length", strconv.FormatInt(end-start+1, 10))
	if partial {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, total))
		w.WriteHeader(http.StatusPartialContent)
	} else {
		w.WriteHeader(http.StatusOK)
	}
	if r.Method == http.MethodHead {
		return
	}

	flusher, _ := w.(http.Flusher)
	for offset := start; offset <= end; {
		chunkEnd := offset + upstreamChunkSize - 1
		if chunkEnd > end {
			chunkEnd = end
		}
		n, err := a.copyRange(r.Context(), w, target.URL, target.Headers, offset, chunkEnd)
		if n > 0 && flusher != nil {
			flusher.Flush()
		}
		if err != nil {
			// The client hanging up mid-track is normal (skip, close), so it is
			// not worth a warning; anything else means the listener just heard
			// the audio cut out and we want that in the log.
			if r.Context().Err() == nil {
				slog.Warn("stream chunk failed", "track", id, "offset", offset, "error", err)
			}
			return
		}
		offset += n
	}
}

// probeUpstream asks for a single byte to learn the full length from
// Content-Range, which is the only reliable way to size a CDN response that
// refuses to answer HEAD.
func (a *App) probeUpstream(ctx context.Context, target string, headers map[string]string) (int64, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return 0, "", err
	}
	applyUpstreamHeaders(req, headers)
	req.Header.Set("Range", "bytes=0-0")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusPartialContent {
		return 0, "", fmt.Errorf("upstream does not support ranges: %s", resp.Status)
	}
	_, _, total, ok := parseContentRange(resp.Header.Get("Content-Range"))
	if !ok || total <= 0 {
		return 0, "", fmt.Errorf("upstream did not report a size")
	}
	return total, resp.Header.Get("Content-Type"), nil
}

// copyRange fetches one byte range and returns how many bytes reached the client.
func (a *App) copyRange(ctx context.Context, w io.Writer, target string, headers map[string]string, start, end int64) (int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return 0, err
	}
	applyUpstreamHeaders(req, headers)
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusPartialContent && resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("upstream returned %s", resp.Status)
	}
	n, err := io.Copy(w, resp.Body)
	if err != nil {
		return n, err
	}
	if n == 0 {
		return 0, fmt.Errorf("upstream returned an empty range")
	}
	return n, nil
}

// applyUpstreamHeaders forwards what yt-dlp prescribed for this URL. Overriding
// the User-Agent with our own is enough to get a 403 from googlevideo.
func applyUpstreamHeaders(req *http.Request, headers map[string]string) {
	for name, value := range headers {
		if strings.EqualFold(name, "Range") || strings.EqualFold(name, "Host") {
			continue
		}
		req.Header.Set(name, value)
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", "Mozilla/5.0")
	}
}

// parseRange handles the single-range form browsers actually send.
// A missing header means the whole file.
func parseRange(header string, total int64) (start, end int64, partial, ok bool) {
	if header == "" {
		return 0, total - 1, false, true
	}
	spec, found := strings.CutPrefix(strings.TrimSpace(header), "bytes=")
	if !found || strings.Contains(spec, ",") {
		return 0, 0, false, false
	}
	fromText, toText, found := strings.Cut(spec, "-")
	if !found {
		return 0, 0, false, false
	}
	switch {
	case fromText == "":
		// "bytes=-500": the trailing 500 bytes.
		length, err := strconv.ParseInt(toText, 10, 64)
		if err != nil || length <= 0 {
			return 0, 0, false, false
		}
		if length > total {
			length = total
		}
		return total - length, total - 1, true, true
	default:
		start, err := strconv.ParseInt(fromText, 10, 64)
		if err != nil || start < 0 || start >= total {
			return 0, 0, false, false
		}
		end = total - 1
		if toText != "" {
			parsed, err := strconv.ParseInt(toText, 10, 64)
			if err != nil || parsed < start {
				return 0, 0, false, false
			}
			if parsed < end {
				end = parsed
			}
		}
		return start, end, true, true
	}
}

func parseContentRange(header string) (start, end, total int64, ok bool) {
	spec, found := strings.CutPrefix(strings.TrimSpace(header), "bytes ")
	if !found {
		return 0, 0, 0, false
	}
	rangePart, totalPart, found := strings.Cut(spec, "/")
	if !found {
		return 0, 0, 0, false
	}
	total, err := strconv.ParseInt(totalPart, 10, 64)
	if err != nil {
		return 0, 0, 0, false
	}
	fromText, toText, found := strings.Cut(rangePart, "-")
	if !found {
		return 0, 0, total, true
	}
	start, _ = strconv.ParseInt(fromText, 10, 64)
	end, _ = strconv.ParseInt(toText, 10, 64)
	return start, end, total, true
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
