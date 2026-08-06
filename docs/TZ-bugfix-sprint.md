# TZ v0.3.1 — Bugfix Sprint: x2 Duration, Slow Stream, SoundCloud, Playlist UX, Infinite Scroll

## Context
Music Orchestrator — self-hosted music aggregator (Go backend + React web + Flutter mobile).
Version 0.3.0 deployed. User reports 5 critical bugs that must be fixed in v0.3.1.

**Working directory**: `/home/kisskin/music-orchestrator` (symlink to `/mnt/ssd/projects/music-orchestrator`)
**Branch**: `go-main`
**Deploy target**: Raspberry Pi, systemd services

---

## BUG 1: x2 Duration (YouTube tracks show double length)

### Root Cause
`models.go:41`:
```go
DurationSeconds  int          `json:"duration_seconds,omitempty"`
```
`omitempty` drops the field when value is 0. Frontend receives `undefined`, cannot apply `OVERREPORTED_DURATION_RATIO` correction, audio element reports ~2x duration from Content-Length header.

### Fix
1. **Backend** (`models.go`): Remove `omitempty` from `DurationSeconds`:
   ```go
   DurationSeconds  int          `json:"duration_seconds"`
   ```
2. **Frontend** (`frontend/frontend/src/store/player.ts`): In `correctedDurationSeconds()`, when `provider === undefined` and `mediaSeconds` is available, check if `mediaSeconds` looks like 2x a reasonable track length (>300s for most music). If so, halve it. This is a safety net for when backend sends 0.
3. **Flutter** (`mobile/lib/state/player_controller.dart`): Same logic in `duration` getter — when `ds` is null/0 but `raw` is suspiciously large, apply halving heuristic.

### Verification
- `curl http://127.0.0.1:18080/v1/search?q=test | jq '.items[0].duration_seconds'` — must return a number, never null
- Play a YouTube track — duration must match actual song length (±5s)

---

## BUG 2: Slow YouTube Streaming (5-10s delay before playback)

### Root Cause
`main.go:457` — `probeUpstream()` sends `Range: bytes=0-0` to YouTube CDN to learn Content-Length. This adds 2-5s latency per track. Then `copyRange()` does chunked proxying in 4MB slices.

### Fix
1. **Remove `probeUpstream` for non-HLS streams**: YouTube CDN responses include `Content-Length` in the initial response. Instead of probing, stream the entire response directly.
2. **Implement streaming proxy**: For non-HLS streams, pipe the upstream response directly to the client:
   ```go
   func (a *App) streamDirect(w http.ResponseWriter, r *http.Request, target StreamTarget) {
       req, err := http.NewRequestWithContext(r.Context(), "GET", target.URL, nil)
       applyUpstreamHeaders(req, target.Headers)
       // Forward client's Range header if present
       if r.Header.Get("Range") != "" {
           req.Header.Set("Range", r.Header.Get("Range"))
       }
       resp, err := http.DefaultClient.Do(req)
       // Copy headers from upstream response
       for k, v := range resp.Header {
           w.Header()[k] = v
       }
       w.WriteHeader(resp.StatusCode)
       io.Copy(w, resp.Body)
   }
   ```
3. **Keep `probeUpstream` only as fallback** for CDNs that don't send Content-Length.

### Verification
- Time from click to first audio byte: must be <3s for YouTube (currently 5-10s)
- `curl -w '%{time_starttransfer}' -o /dev/null http://127.0.0.1:18080/v1/stream/youtube_stream:VIDEO_ID`

---

## BUG 3: SoundCloud Doesn't Play

### Root Cause
HLS remux code exists (`hls.go`) but yt-dlp may fail to resolve SoundCloud URLs. Need to verify the full chain.

### Fix
1. **Diagnose first**: Run `yt-dlp --dump-json "https://soundcloud.com/artist/track"` and check if it returns valid data
2. If yt-dlp works: check that `extractor.go` properly passes SoundCloud URLs through
3. If yt-dlp fails: update yt-dlp or add SoundCloud-specific workarounds
4. **Ensure HLS remux is called**: Add logging to `hls.go:materialize()` to confirm it's being invoked
5. **Test with a known-good SoundCloud URL**: e.g., `https://soundcloud.com/humbvit/c418-sweden`

### Verification
- `curl http://127.0.0.1:18080/v1/search?q=test&providers=soundcloud_stream` — must return results
- Play a SoundCloud track — must start within 10s

---

## BUG 4: No "+" Button to Add Track in Playlist Detail

### Root Cause
`PlaylistDetailScreen` shows tracks but has no way to add new tracks. User must go to Search, find track, use context menu.

### Fix
1. **Flutter** (`mobile/lib/screens/playlist_detail_screen.dart`): Add a FloatingActionButton that opens a track search/picker dialog
2. **React** (`frontend/frontend/src/features/search/SearchPage.tsx`): In `PlaylistCard` component, add "Add track" button that opens inline search
3. Track picker should:
   - Show search input
   - Search across enabled providers
   - Show results with "+" button per track
   - Call `POST /v1/playlists/{id}/tracks` to add

### Verification
- Open a playlist → see "+" button → tap → search dialog opens → search "test" → see results → tap "+" → track added to playlist

---

## BUG 5: No Infinite Scroll / Pagination on Search

### Root Cause
Backend (`main.go:322`): `if limit < 1 || limit > 50` — hardcap at 50 results.
Frontend: `PAGE_SIZE = 20`, loads 20 results, no scroll listener.

### Fix
1. **Backend**: Raise cap to 200: `if limit < 1 || limit > 200`
2. **Frontend React** (`SearchPage.tsx`): Add `IntersectionObserver` on last track element → load next page via `offset` param
3. **Flutter** (`search_screen.dart`): Add `ScrollController` listener → when near bottom, call `library.loadMoreResults()`
4. **Both**: Show loading indicator at bottom while fetching next page
5. **Both**: Stop loading when `items.length >= total`

### Verification
- Search "music" → see 20 results → scroll down → see "Loading..." → see 40 results → scroll more → up to 200

---

## Additional Tasks

### Performance Metrics
Add timing middleware to track:
- `/v1/search` response time
- `/v1/stream/{id}` time-to-first-byte
- yt-dlp process duration
Log these as structured JSON for monitoring.

### Code Audit
Review and fix:
- Error handling consistency (some endpoints return plain text, others JSON)
- Memory leaks in stream caching (check TTL expiration)
- Race conditions in HLS cache (concurrent downloads of same track)

### Version Bump
- `frontend/frontend/package.json`: `"version": "0.3.1"`
- Flutter `pubspec.yaml`: `version: 0.3.1+1`
- Commit: `fix: v0.3.1 — duration, streaming, soundcloud, playlist UX, infinite scroll`

### Deploy
After all fixes:
1. `go test ./...`
2. `cd frontend/frontend && npm test -- --run && npm run build`
3. `go build -o bin/music-orchestrator .`
4. Restart backend: `pkill -f music-orchestrator; cd /home/kisskin/music-orchestrator && APP_ADDR=:18080 APP_ENABLE_RISKY_EXTRACTORS=true ./bin/music-orchestrator &`
5. Restart frontend: `pkill -f 'vite preview'; cd frontend/frontend && npx vite preview --host 0.0.0.0 --port 5173 &`
6. `git push github go-main`

---

## Constraints
- Do NOT break existing functionality
- All Go tests must pass
- All frontend tests must pass
- Frontend must build without errors
- SoundCloud HLS remux must work (ffmpeg is installed at `/usr/bin/ffmpeg`)
- Backend runs on Raspberry Pi (ARM64) — keep memory usage reasonable
- Do NOT change auth, user management, or settings code
- Do NOT modify `.env` or deployment configs

## Files to Modify
- `models.go` — remove omitempty from DurationSeconds
- `main.go` — stream handler, search limit cap, performance metrics
- `extractor.go` — SoundCloud URL resolution
- `hls.go` — logging for HLS remux
- `frontend/frontend/src/store/player.ts` — duration correction
- `frontend/frontend/src/features/search/SearchPage.tsx` — infinite scroll, playlist add
- `frontend/frontend/package.json` — version bump
- `mobile/lib/state/player_controller.dart` — duration correction
- `mobile/lib/screens/search_screen.dart` — infinite scroll
- `mobile/lib/screens/playlist_detail_screen.dart` — add track button
- `mobile/pubspec.yaml` — version bump
