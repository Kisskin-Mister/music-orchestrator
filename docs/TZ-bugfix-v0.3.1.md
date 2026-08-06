# TZ v0.3.1 — Bugfix Sprint

## Context
Music Orchestrator v0.3.0 has 5 critical bugs that make the app barely usable:
- YouTube tracks show ~2x actual duration
- YouTube streaming is extremely slow (10+ seconds to start)
- SoundCloud doesn't play at all
- Can't add tracks to existing playlists
- Search stops loading after 50 results

Backend: Go, runs on 0.0.0.0:18080
Frontend: React + Vite, runs on 0.0.0.0:5173
Flutter: Docker container on 172.22.0.1:5174

---

## BUG 1: x2 Duration — YouTube tracks show ~double actual duration

### Root cause
Two-part failure:

**Part A — Backend drops duration=0 via `omitempty`:**
- `models.go:41` — `DurationSeconds int json:"duration_seconds,omitempty"` 
- When yt-dlp returns `duration: 0` for a track, the JSON field is omitted entirely
- Frontend receives `undefined` for duration_seconds

**Part B — Frontend correction requires positive provider duration:**
- `player.ts:12-22` — `correctedDurationSeconds(providerSeconds, mediaSeconds)` 
- When `providerSeconds` is `undefined` or `0`, it returns `mediaSeconds` raw (the doubled value)
- The `OVERREPORTED_DURATION_RATIO = 1.3` check never fires
- `SearchPage.tsx:666` — `updateBuffered()` has same issue: `providerDuration > 0` guard fails

### Fix
1. **Backend**: Remove `omitempty` from `DurationSeconds` in `models.go` — always send the field, even if 0
2. **Frontend player.ts**: When `providerSeconds` is `undefined`/`0` AND `mediaSeconds > 120`, apply a heuristic: use `mediaSeconds * 0.52` as a fallback (YouTube containers are typically ~1.92x the real audio)
3. **Frontend SearchPage.tsx**: In `updateBuffered()`, when `providerDuration` is 0, still check if `mediaDuration` seems overreported (> 120s for a track that was expected to be shorter) and apply the same heuristic
4. Add unit tests for edge cases: `correctedDurationSeconds(0, 380)`, `correctedDurationSeconds(undefined, 380)`

---

## BUG 2: Slow YouTube Streaming — 10+ seconds to start playing

### Root cause
**Double yt-dlp invocation + unnecessary probe:**

1. `extractor.go:215` — `StreamTarget()` calls `e.dump()` which spawns a NEW yt-dlp process (cold start ~3-8s on Pi)
2. `main.go:457` — After StreamTarget returns, `probeUpstream()` sends `Range: bytes=0-0` to CDN for Content-Length — another network round-trip (~1-3s)
3. YouTube's signed CDN URLs may expire/throttle between resolve and probe
4. Stream cache TTL is only 60s — too short for a listening session

### Fix
1. **Eliminate `probeUpstream` for non-HLS streams**: When `StreamTarget` returns a direct URL, skip the probe. Instead, forward the first byte-range request directly to upstream and extract Content-Length from its response headers. This saves one full round-trip.
2. **Increase stream cache TTL**: Change `streamCacheTTL` from `60 * time.Second` to `300 * time.Second` (5 minutes). YouTube CDN URLs stay valid for ~6 minutes.
3. **Lazy content-type detection**: If `contentType` is unknown, default to `audio/mpeg` (or infer from the URL extension) and let the first upstream response confirm/correct it. Don't block on a probe.
4. **Pre-warm on search**: When a user searches and results come back, optionally resolve the first 2-3 stream URLs in the background so clicking play is instant.

---

## BUG 3: SoundCloud Doesn't Play

### Root cause
The HLS remux code (`hls.go`) is correctly integrated but the running backend binary may be stale OR ffmpeg may not be handling SoundCloud's specific HLS format.

**Diagnostic steps (you MUST verify these before fixing):**
1. Run `which ffmpeg && ffmpeg -version` — confirm ffmpeg is available
2. Run `curl -s 'http://127.0.0.1:18080/v1/search?q=drake&providers=soundcloud_stream&limit=1'` — get a track ID
3. Run `curl -s -o /dev/null -w '%{http_code} %{size_download}' 'http://127.0.0.1:18080/v1/stream/<track_id>'` — check if it returns 200 with data
4. If it returns 502, check `/tmp/music-backend.log` for the actual error
5. If ffmpeg is the issue, check if SoundCloud's HLS uses codecs ffmpeg can handle

### Fix (after diagnosis)
- If ffmpeg not found: install it (`apt install ffmpeg`) or ensure `APP_FFMPEG_BINARY` is set correctly
- If codec issue: `hls.go:57-68` — the `containerFor()` function handles AAC→MP4, MP3→MP3, else→transcode to MP3. Verify SoundCloud's actual codec matches.
- If yt-dlp extraction fails: update yt-dlp (`pip install -U yt-dlp`) and test manually
- Add better error logging in `serveHLS()` — log the actual ffmpeg command and stderr on failure

---

## BUG 4: No + Button to Add Tracks in Playlist View

### Root cause
The "Добавить трек" button EXISTS but is at the BOTTOM of the playlist card (`SearchPage.tsx:398-400`), below all tracks. For playlists with many tracks, users never see it.

Additionally, there is NO `+` button on individual track rows within a playlist — only heart and trash buttons.

### Fix
1. **Move "Добавить трек" button to the playlist card header** — next to the play/edit/delete buttons (`SearchPage.tsx:385-389`). It should be visible without scrolling.
2. **Add a floating + button for playlist detail view** — similar to the floating + for creating playlists (`SearchPage.tsx:150`), but for adding tracks to the current playlist. Position it above the mini player.
3. **In the PlaylistTrackPicker** (`SearchPage.tsx:408-442`): make sure the search actually works and results are addable with one tap.

---

## BUG 5: No Infinite Scroll — Search stops after 50 results

### Root cause
Backend hard-caps at 50 items:
- `main.go:322-324` — `if limit < 1 || limit > 50 { limit = 20 }`

Frontend IntersectionObserver fires endlessly:
- `searchLimit` goes to 60, 80, 100... but backend always returns max 50
- Observer sees `canLoadMore = true` (50 < total), fires again, same 50 returned
- Infinite loop of identical API calls

### Fix
1. **Backend**: Raise cap from 50 to 200 in `main.go:322`: `if limit < 1 || limit > 200 { limit = 20 }`
2. **Frontend**: Add a guard: if the API returned fewer items than requested, stop the observer:
   ```ts
   const canLoadMore = visibleTracks.length > 0 && visibleTracks.length < visibleTotal && visibleTracks.length >= searchLimit;
   ```
   This stops the loop when the backend returns less than we asked for (hit its cap).
3. **Frontend**: Add a "Показано X из Y — Загрузить ещё" button as fallback for when IntersectionObserver doesn't trigger (e.g., desktop with tall viewport).

---

## ADDITIONAL TASKS

### Performance Metrics
After fixing the above, measure and log:
- yt-dlp cold start time (first search)
- yt-dlp cached hit time (second search within cache TTL)
- Stream resolve time (first play)
- Stream cached resolve time (second play within 5 min)
- HLS remux time (first SoundCloud play)
- Frontend build size

Write results to `docs/PERFORMANCE-v0.3.1.md`.

### Code Audit
Review the full codebase for:
- Error handling gaps (unhandled errors that silently fail)
- Memory leaks (sync.Map entries that never expire, goroutine leaks)
- Race conditions (concurrent map access without sync)
- Security issues (path traversal in media serving, missing auth checks)
- Missing input validation

Fix any critical issues found. Log non-critical ones as TODO comments.

### Version Bump
1. `frontend/frontend/package.json` → `"version": "0.3.1"`
2. `mobile/lib/screens/settings_screen.dart` → `Music Orchestrator v0.3.1`
3. `mobile/pubspec.yaml` → bump version if applicable

### Testing
1. `go test ./...` — must pass
2. `cd frontend/frontend && npm test -- --run` — must pass
3. `cd frontend/frontend && npm run build` — must pass
4. Manual smoke test: search for "kanye west" on YouTube, play first result, verify duration is correct
5. Manual smoke test: search for "drake" on SoundCloud, play first result, verify it plays
6. Manual smoke test: open a playlist with 5+ tracks, verify "Добавить трек" button is visible without scrolling
7. Manual smoke test: search for something with 100+ results, verify infinite scroll loads past 50

### Deploy
1. Build backend: `cd /home/kisskin/music-orchestrator && go build -o bin/music-orchestrator .`
2. Restart backend: kill old process, start with `APP_ADDR=:18080 APP_ENABLE_RISKY_EXTRACTORS=true ./bin/music-orchestrator`
3. Build frontend: `cd frontend/frontend && npm run build && npx vite preview --host 0.0.0.0 --port 5173 &`
4. Rebuild Flutter container: `cd mobile && docker build -f Dockerfile.flutter -t music-flutter-web:latest . && docker stop music-flutter-web && docker rm music-flutter-web && docker run -d --name music-flutter-web --restart unless-stopped -p 172.22.0.1:5174:80 music-flutter-web:latest`
5. Commit and push: `git add -A && git commit -m "fix: v0.3.1 — duration, streaming, soundcloud, playlist, infinite scroll" && git push github go-main`

### CONSTRAINTS
- Do NOT break working features
- Do NOT touch auth, sessions, or user management code
- Do NOT change the Flutter app's visual design (it IS the reference)
- All changes must compile and pass tests before committing
- Log all significant decisions as comments in the code
