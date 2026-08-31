package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
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
	prefetch  *prefetchCache
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
	a := &App{cfg: effective, baseCfg: cfg, store: st, providers: NewProviderService(effective), sessions: newSessionStore(), hls: newHLSCache(effective), prefetch: newPrefetchCache(), mux: http.NewServeMux()}
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
	a.mux.HandleFunc("GET /v1/library", a.auth(a.library))
	a.mux.HandleFunc("POST /v1/import/scan", a.auth(a.importScan))
	a.mux.HandleFunc("POST /v1/import/upload", a.auth(a.importUpload))
	a.mux.HandleFunc("GET /v1/local/{fingerprint}", a.serveLocalFile)
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
		writeErrorCode(w, http.StatusForbidden, "admin_required", "Нужны права администратора — войди в аккаунт владельца")
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

// maxSearchResults bounds how deep paging can go. Every page re-runs yt-dlp for
// offset+limit results, so this is as much a ceiling on the Pi's memory and
// patience as it is on the result list.
const maxSearchResults = 200

func (a *App) search(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	limit := queryInt(r, "limit", 20)
	offset := queryInt(r, "offset", 0)
	if limit < 1 {
		limit = 20
	}
	if limit > maxSearchResults {
		limit = maxSearchResults
	}
	if offset < 0 {
		offset = 0
	}
	if offset > maxSearchResults {
		offset = maxSearchResults
	}
	// Fetching one item past the page is what tells the client whether to keep
	// scrolling. The providers have no notion of a grand total — yt-dlp only
	// returns as many hits as it was asked for — so "there was one more than
	// this page" is the only honest signal, and it costs a single extra result.
	fetch := offset + limit + 1
	if fetch > maxSearchResults {
		fetch = maxSearchResults
	}
	providers := splitCSV(r.URL.Query().Get("providers"))
	items, moreAvailable := a.providers.Search(q, providers, fetch)
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
	// Counting the tracks that survived filtering cannot answer "is there more":
	// a 21-item fetch that yields 19 playable tracks looks exactly like a search
	// with only 19 hits, and reporting 19 told a client holding 19 that it had
	// everything, which killed the infinite scroll after one page. So take the
	// answer from the providers instead — either they trimmed something off this
	// page, or they reported still having results past it — and phrase the total
	// as one past what the client will be holding, which is the only claim the
	// providers can actually back. An empty next page ends the scroll.
	if moreAvailable || total > offset+len(items) {
		total = offset + len(items) + 1
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
		// Импортированный файл лежит там, куда его положил импорт, и отдаётся
		// через /v1/local. Раньше сюда безусловно подставлялся демо-путь, и
		// каждый загруженный трек молча упирался в 404.
		url := "/media/demo-" + pid + ".mp3"
		pb.Attribution = "Local demo"
		if _, found := a.store.LocalFilePath(a.optionalUserIDFromRequest(r), id); found {
			url = "/v1/local/" + pid
			pb.Attribution = "Локальный файл"
		}
		pb.PlaybackType = "local_stream"
		pb.StreamURL = &url
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
		// The client takes a second or two to read this response and open the
		// stream URL. Spending it on the resolve and the start of the remux is
		// free head start on the only part of SoundCloud playback that is slow.
		go a.warmStream(providerID, pid)
		pb.Attribution = pr.Name
	}
	writeJSON(w, http.StatusOK, pb)
}

// warmStream starts the work /v1/stream would otherwise start from cold. Both
// steps have to happen inside the goroutine: a resolve is a yt-dlp process and
// would hold up the playback response it is supposed to run behind. Nothing is
// returned because nothing waits on it — the result lands in the stream cache,
// the HLS cache and the prefetch cache, where the stream request finds it.
//
// A playlist is materialised, which is the slow case. A plain file is not
// downloaded, only opened: the first chunk is fetched and kept, so the stream
// request that follows a second later skips both the DNS/TLS handshake and the
// round trip that used to stand between the client pressing play and the first
// audio byte.
func (a *App) warmStream(providerID, pid string) {
	trackID := providerID + ":" + pid
	target, err := a.providers.extractor.StreamTarget(providerID, pid)
	if err != nil {
		slog.Debug("warm resolve failed", "track", trackID, "error", err)
		return
	}
	if !target.HLS {
		a.prefetchFirstChunk(trackID, target)
		return
	}
	if _, _, err := a.hls.materialize(context.Background(), trackID, target); err != nil {
		slog.Debug("warm remux failed", "track", trackID, "error", err)
	}
}

const (
	// prefetchChunkSize is how much of a YouTube file warm-on-play pulls down
	// in advance. A megabyte is several seconds of audio — enough to cover the
	// gap while the first ranged request of the real stream is in flight —
	// without spending a chunk's worth of bandwidth on a track the listener may
	// skip before it starts.
	prefetchChunkSize int64 = 1 << 20
	// prefetchTTL only has to span the gap between /v1/playback and /v1/stream,
	// which is however long the client takes to read one JSON response and open
	// a URL. Beyond that the CDN link itself starts going stale.
	prefetchTTL = 2 * time.Minute
	// prefetchMaxEntries bounds what the cache can hold at once. Entries are
	// single-use and short-lived, so this is a ceiling for skipped tracks
	// rather than a working set.
	prefetchMaxEntries = 4
	prefetchTimeout    = 30 * time.Second
)

// prefetchEntry is the head of one track, fetched before it was asked for.
type prefetchEntry struct {
	url         string // the CDN URL it came from, to catch a re-resolve
	data        []byte
	total       int64 // the object's full size, from Content-Range
	contentType string
	expires     time.Time
}

// prefetchCache holds those heads until the matching /v1/stream arrives.
//
// Entries are handed out once and then dropped: the stream handler streams the
// bytes straight to the client, and a listener who seeks back to the start gets
// them from the client's own buffer, not from here. That keeps the cache to a
// few megabytes without any eviction policy worth the name.
type prefetchCache struct {
	mu      sync.Mutex
	entries map[string]*prefetchEntry
}

func newPrefetchCache() *prefetchCache {
	return &prefetchCache{entries: map[string]*prefetchEntry{}}
}

func (c *prefetchCache) put(trackID string, entry *prefetchEntry) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	for id, existing := range c.entries {
		if now.After(existing.expires) {
			delete(c.entries, id)
		}
	}
	// Over the ceiling, the oldest head is the one least likely to still be
	// wanted: warm-on-play fires in the order tracks are started.
	for len(c.entries) >= prefetchMaxEntries {
		oldest := ""
		for id, existing := range c.entries {
			if oldest == "" || existing.expires.Before(c.entries[oldest].expires) {
				oldest = id
			}
		}
		delete(c.entries, oldest)
	}
	c.entries[trackID] = entry
}

// take returns the prefetched head of a track and forgets it. The URL is
// checked because a re-resolve between the warm-up and the stream request can
// hand back a different format, and half of one encoding followed by the rest
// of another is silence.
func (c *prefetchCache) take(trackID, url string) (*prefetchEntry, bool) {
	if c == nil {
		return nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[trackID]
	if !ok {
		return nil, false
	}
	delete(c.entries, trackID)
	if entry.url != url || time.Now().After(entry.expires) {
		return nil, false
	}
	return entry, true
}

// takePrefetched claims the warm-up's head of a track for a request that can
// use it. Only a request starting at byte 0 can: that is what warm-on-play
// fetched, and it is also the request whose latency the listener hears as the
// delay after pressing play. A seek is left to the CDN.
func (a *App) takePrefetched(trackID, url string, start int64) (*prefetchEntry, bool) {
	if start != 0 {
		return nil, false
	}
	entry, ok := a.prefetch.take(trackID, url)
	if !ok || entry.total <= 0 || len(entry.data) == 0 {
		return nil, false
	}
	slog.Debug("stream served a prefetched first chunk", "track", trackID, "bytes", len(entry.data))
	return entry, true
}

// prefetchFirstChunk pulls the head of a plain (non-playlist) file and parks it
// in the prefetch cache. Failures are not worth reporting: this is a head
// start, and the stream request does the whole thing itself if it is missing.
func (a *App) prefetchFirstChunk(trackID string, target StreamTarget) {
	if a.prefetch == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), prefetchTimeout)
	defer cancel()

	started := time.Now()
	resp, total, err := openUpstreamChunk(ctx, target.URL, target.Headers, 0, prefetchChunkSize-1)
	if err != nil {
		slog.Debug("warm prefetch failed", "track", trackID, "error", err)
		return
	}
	defer discardResponse(resp)
	data, err := io.ReadAll(io.LimitReader(resp.Body, prefetchChunkSize))
	if err != nil || len(data) == 0 {
		slog.Debug("warm prefetch read failed", "track", trackID, "bytes", len(data), "error", err)
		return
	}
	a.prefetch.put(trackID, &prefetchEntry{
		url:         target.URL,
		data:        data,
		total:       total,
		contentType: resp.Header.Get("Content-Type"),
		expires:     time.Now().Add(prefetchTTL),
	})
	slog.Debug("warm prefetch stored", "track", trackID, "bytes", len(data), "total", total, "elapsed", time.Since(started))
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
		writeError(w, http.StatusBadGateway, unavailableMessage(providerID))
		return
	}

	if target.HLS {
		trackID := providerID + ":" + pid
		a.serveHLS(w, r, trackID, target)
		return
	}

	// The first chunk request doubles as the size probe: its Content-Range
	// carries the full length. A separate bytes=0-0 probe used to cost an extra
	// round trip to the CDN — seconds of silence before any audio moved — and
	// bought nothing the first chunk does not already report.
	var (
		total       int64
		contentType string
		firstChunk  io.ReadCloser
	)
	rangeHeader := r.Header.Get("Range")
	if hintStart, hintEnd, ok := parseRangeHint(rangeHeader); ok {
		if entry, warm := a.takePrefetched(id, target.URL, hintStart); warm {
			// Warm-on-play already has the head of this file in memory, so the
			// listener hears it without waiting on the CDN at all.
			firstChunk, total, contentType = io.NopCloser(bytes.NewReader(entry.data)), entry.total, entry.contentType
		} else {
			chunkEnd := hintStart + upstreamChunkSize - 1
			if hintEnd >= 0 && hintEnd < chunkEnd {
				chunkEnd = hintEnd
			}
			resp, refreshed, size, err := a.openUpstream(r.Context(), providerID, pid, target, hintStart, chunkEnd)
			if err != nil {
				slog.Warn("stream open failed", "track", id, "error", err)
				writeError(w, http.StatusBadGateway, "Audio source is unavailable")
				return
			}
			firstChunk, target, total, contentType = resp.Body, refreshed, size, resp.Header.Get("Content-Type")
		}
		defer func() {
			_, _ = io.Copy(io.Discard, firstChunk)
			_ = firstChunk.Close()
		}()
	} else {
		// Suffix ranges ("bytes=-500") are relative to a size we do not know yet,
		// so those keep paying for the probe.
		total, contentType, err = a.probeUpstream(r.Context(), target.URL, target.Headers)
		if err != nil {
			slog.Warn("stream probe failed", "track", id, "error", err)
			writeError(w, http.StatusBadGateway, "Audio source is unavailable")
			return
		}
	}

	start, end, partial, ok := parseRange(rangeHeader, total)
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
	offset := start
	if firstChunk != nil {
		// Already in flight from the size probe — hand it to the listener before
		// asking the CDN for anything else.
		n, err := io.Copy(w, io.LimitReader(firstChunk, end-offset+1))
		if n > 0 && flusher != nil {
			flusher.Flush()
		}
		if err != nil {
			if r.Context().Err() == nil {
				slog.Warn("stream first chunk failed", "track", id, "offset", offset, "error", err)
			}
			return
		}
		offset += n
	}
	for offset <= end {
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

// unavailableMessage names the source the listener actually chose, so a failure
// points at the provider to retry rather than at "audio".
func unavailableMessage(providerID string) string {
	switch providerID {
	case "soundcloud_stream":
		return "SoundCloud track unavailable"
	case "youtube_stream":
		return "YouTube track unavailable"
	default:
		return "Audio source is unavailable"
	}
}

// upstreamStatusError carries the CDN's own status so the caller can tell an
// expired link (403) from a rate limit (429) or a genuinely broken source.
type upstreamStatusError struct {
	code   int
	status string
}

func (e upstreamStatusError) Error() string { return "upstream returned " + e.status }

// openUpstream fetches the first chunk and reports the object's full size,
// re-resolving the source once if the CDN link has expired. googlevideo answers
// 403 for a stale URL, which is what a listener hits after pausing for hours.
// The possibly-refreshed target is returned so the rest of the chunk loop uses
// the new URL rather than the dead one.
func (a *App) openUpstream(ctx context.Context, providerID, pid string, target StreamTarget, start, end int64) (*http.Response, StreamTarget, int64, error) {
	resp, total, err := openUpstreamChunk(ctx, target.URL, target.Headers, start, end)
	if err == nil {
		return resp, target, total, nil
	}
	var status upstreamStatusError
	if !errors.As(err, &status) || status.code != http.StatusForbidden {
		return nil, target, 0, err
	}
	a.providers.extractor.InvalidateStream(providerID, pid)
	fresh, resolveErr := a.providers.extractor.StreamTarget(providerID, pid)
	if resolveErr != nil {
		return nil, target, 0, err
	}
	resp, total, err = openUpstreamChunk(ctx, fresh.URL, fresh.Headers, start, end)
	if err != nil {
		return nil, target, 0, err
	}
	return resp, fresh, total, nil
}

// openUpstreamChunk starts one ranged request and leaves the body open for the
// caller to stream. The size comes from Content-Range, so no separate probe is
// needed. A CDN that ignores ranges answers 200; that is only usable from the
// start of the file, and its Content-Length is then the whole size.
func openUpstreamChunk(ctx context.Context, target string, headers map[string]string, start, end int64) (*http.Response, int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, 0, err
	}
	applyUpstreamHeaders(req, headers)
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	switch {
	case resp.StatusCode == http.StatusPartialContent:
		_, _, total, ok := parseContentRange(resp.Header.Get("Content-Range"))
		if !ok || total <= 0 {
			discardResponse(resp)
			return nil, 0, fmt.Errorf("upstream did not report a size")
		}
		return resp, total, nil
	case resp.StatusCode == http.StatusOK && start == 0 && resp.ContentLength > 0:
		return resp, resp.ContentLength, nil
	default:
		discardResponse(resp)
		return nil, 0, upstreamStatusError{code: resp.StatusCode, status: resp.Status}
	}
}

func discardResponse(resp *http.Response) {
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
}

// parseRangeHint reports the first byte the client asked for — and the last one
// when the client named it — without knowing the object size. That is what lets
// the first upstream request double as the size probe. Suffix ranges
// ("bytes=-500") are relative to the size, so they cannot be resolved here.
func parseRangeHint(header string) (start, end int64, ok bool) {
	trimmed := strings.TrimSpace(header)
	if trimmed == "" {
		return 0, -1, true
	}
	spec, found := strings.CutPrefix(trimmed, "bytes=")
	if !found || strings.Contains(spec, ",") {
		return 0, 0, false
	}
	fromText, toText, found := strings.Cut(spec, "-")
	if !found || fromText == "" {
		return 0, 0, false
	}
	start, err := strconv.ParseInt(fromText, 10, 64)
	if err != nil || start < 0 {
		return 0, 0, false
	}
	if toText == "" {
		return start, -1, true
	}
	end, err = strconv.ParseInt(toText, 10, 64)
	if err != nil || end < start {
		return 0, 0, false
	}
	return start, end, true
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

// library serves the media library: paged tracks, or the artist/album axes.
// Grouping and search run in SQLite so the response size stays bounded no
// matter how large the collection grows.
func (a *App) library(w http.ResponseWriter, r *http.Request) {
	ownerID := userIDFromRequest(r)
	q := r.URL.Query()
	query, source := q.Get("q"), q.Get("source")
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	if offset < 0 {
		offset = 0
	}

	switch q.Get("group") {
	case "artists":
		writeJSON(w, http.StatusOK, map[string]any{"artists": a.store.LibraryArtists(ownerID, query)})
	case "albums":
		writeJSON(w, http.StatusOK, map[string]any{"albums": a.store.LibraryAlbums(ownerID, query)})
	default:
		page := a.store.LibraryTracks(ownerID, query, source, limit, offset)
		for i := range page.Tracks {
			page.Tracks[i] = a.annotateTrack(ownerID, page.Tracks[i])
		}
		writeJSON(w, http.StatusOK, page)
	}
}
