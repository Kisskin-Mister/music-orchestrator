# bugfix-sprint-v0.3.1 - Product Requirements Document (PRD)

## Requirements Description

### Background
- **Business Problem**: Music Orchestrator v0.3.0 has 5 critical bugs that break core playback and discovery UX. YouTube tracks display double the actual duration, YouTube streaming takes 5-10 seconds to start, SoundCloud tracks don't play at all, users cannot add tracks to playlists from the playlist screen, and search results are capped at 50 with no way to load more.
- **Target Users**: Single user (self-hosted). Primary device: iPhone (Safari). Secondary: desktop browser. Flutter mobile app + React web app.
- **Value Proposition**: Fix all 5 bugs so the app works as a reliable daily music player. Without these fixes, the app is unusable for its primary purpose.

### Feature Overview
- **Core Features**:
  1. Correct duration display for all tracks (YouTube, SoundCloud, local)
  2. Fast YouTube streaming (<3s to first audio byte)
  3. Working SoundCloud playback via HLS remux
  4. "Add to playlist" button accessible from playlist detail screen
  5. Infinite scroll on search results (up to 200 results)
- **Feature Boundaries**:
  - In scope: All 5 bugs, version bump, tests, deploy
  - Out of scope: New features, UI redesign, auth changes, settings changes, new providers
- **User Scenarios**:
  - User searches "kanye stronger" → sees results → plays track → duration shows 3:12 (not 6:24) → audio starts within 3 seconds
  - User searches "drake" on SoundCloud → sees results → plays track → audio plays
  - User opens a playlist → taps "+" → searches for track → adds it → sees it in playlist
  - User searches "lofi" → scrolls past first 20 → sees loading indicator → sees 40 results → continues scrolling

### Detailed Requirements

#### Requirement 1: Fix x2 Duration Display

**Background**: YouTube's CDN serves audio muxed with video. When the browser's `<audio>` element (or just_audio on Flutter) reads the response, it reports the full container duration (~2x the audio-only duration). The backend also omits `duration_seconds` from JSON when the value is 0, preventing the frontend from applying its correction heuristic.

**User Interaction**:
1. User plays a YouTube track
2. Player shows correct duration (e.g., 3:12)
3. Progress bar moves linearly from 0:00 to 3:12
4. Track auto-advances at 3:12 (not at 6:24)

**Input/Output**:
- Input: `GET /v1/search?q=test` returns tracks with `duration_seconds`
- Input: `<audio>` element's `loadedmetadata` event reports `duration`
- Output: Player displays `min(audio.duration, provider.duration * 1.3)` when provider > 0, or `audio.duration / 2` when provider is 0 and audio.duration > 300s

**Data Validation**:
- `duration_seconds` must always be present in JSON (never omitted), even when 0
- Frontend must handle `duration_seconds: 0` as "unknown" and fall back to audio element duration
- If audio element duration > 300s AND provider duration is 0/missing → halve audio duration as heuristic

**Edge Cases**:
- Podcast/long-form content (>30 min): do NOT halve. Only apply heuristic when audio duration is within 2x-3x of a typical song (120-600s)
- Live streams: `duration_seconds` is 0, audio element reports Infinity → show "Live" badge, no progress bar
- Downloaded tracks: use file metadata duration, not provider metadata

#### Requirement 2: Fast YouTube Streaming

**Background**: The current stream handler (`main.go:432`) makes 2 HTTP round-trips to YouTube CDN: first `probeUpstream` (Range: bytes=0-0) to learn Content-Length, then `copyRange` in 4MB chunks. Each round-trip adds 2-5s latency. For a 5-minute song (~5MB), this means 5-10s before the user hears anything.

**User Interaction**:
1. User taps play on a YouTube track
2. Buffering indicator appears
3. Audio starts within 3 seconds
4. Scrubbing works (seek forward/backward)

**Input/Output**:
- Input: `GET /v1/stream/youtube_stream:VIDEO_ID` from browser
- Output: Proxied audio stream with proper `Content-Length`, `Accept-Ranges: bytes`, `Content-Type: audio/mp4`

**Data Validation**:
- Upstream response must include `Content-Length` (YouTube CDN always does)
- If upstream returns 403/429 → return 502 to client with error message
- If upstream returns redirect → follow it (YouTube CDN often redirects)

**Edge Cases**:
- User seeks to middle of track: forward `Range` header to upstream, return 206 Partial Content
- User pauses and resumes: upstream URL may have expired (TTL ~6h) → re-resolve via yt-dlp if 403
- Concurrent plays of same track: share the upstream connection (HTTP/2 multiplexing or connection pooling)

#### Requirement 3: SoundCloud Playback

**Background**: SoundCloud now serves audio only as HLS playlists (m3u8). The backend has `hls.go` which remuxes HLS to a normal file via ffmpeg, but the integration needs verification. yt-dlp must be able to resolve SoundCloud URLs, and ffmpeg must be available at `/usr/bin/ffmpeg`.

**User Interaction**:
1. User enables SoundCloud source in search
2. User searches for a track
3. User taps play
4. First play takes 10-30s (ffmpeg remux), subsequent plays are instant (cached)
5. Audio plays with seek support

**Input/Output**:
- Input: `GET /v1/search?q=test&providers=soundcloud_stream` → returns tracks
- Input: `GET /v1/stream/soundcloud_stream:TRACK_ID` → triggers HLS remux, returns audio file
- Output: `Content-Type: audio/mpeg` or `audio/mp4`, with `Accept-Ranges: bytes`

**Data Validation**:
- yt-dlp must resolve SoundCloud URLs: `yt-dlp --dump-json "https://soundcloud.com/artist/track"` must succeed
- ffmpeg must be available: `which ffmpeg` returns `/usr/bin/ffmpeg`
- Remuxed file must be >0 bytes
- Cache hit: subsequent requests serve from disk without re-remuxing

**Edge Cases**:
- SoundCloud URL not resolvable by yt-dlp → return 502 with "SoundCloud track unavailable"
- ffmpeg remux timeout (>60s) → return 502
- Concurrent remux of same track → mutex prevents duplicate ffmpeg processes (already implemented in `hls.go`)
- Disk full → return 502, log error

#### Requirement 4: Add Track to Playlist from Playlist Screen

**Background**: Currently, to add a track to a playlist, the user must go to Search, find the track, open its context menu, and select "Add to playlist". There is no way to add a track directly from the playlist detail screen.

**User Interaction**:
1. User opens a playlist
2. User sees a "+" button (FAB on Flutter, button in header on React)
3. User taps "+"
4. A search dialog/screen appears
5. User types track name → results appear
6. User taps "+" next to a track → track is added to playlist
7. Dialog closes, playlist refreshes and shows the new track

**Input/Output**:
- Input: `POST /v1/playlists/{playlist_id}/tracks` with `{"track_id": "youtube_stream:VIDEO_ID"}`
- Output: Updated playlist with new track
- Search: `GET /v1/search?q=...` (reuses existing search endpoint)

**Data Validation**:
- Track must exist in backend (search returns it)
- Playlist must exist
- Duplicate track → silently succeed (idempotent)
- Track from any provider (YouTube, SoundCloud, local) can be added

**Edge Cases**:
- Search returns no results → show "Ничего не найдено" message
- Network error during add → show error snackbar, keep dialog open
- Adding track to playlist while offline → queue for sync (future, not in this sprint)

#### Requirement 5: Infinite Scroll on Search Results

**Background**: Backend caps search at 50 results (`main.go:322`: `if limit < 1 || limit > 50`). Frontend loads 20 results and stops. For popular queries, there are many more results available but the user cannot see them.

**User Interaction**:
1. User searches "lofi"
2. First 20 results appear
3. User scrolls down
4. Loading indicator appears at bottom
5. Next 20 results load and append
6. Repeat until all results loaded or cap reached (200)
7. "Все результаты загружены" message at bottom

**Input/Output**:
- Input: `GET /v1/search?q=lofi&limit=20&offset=0` → first page
- Input: `GET /v1/search?q=lofi&limit=20&offset=20` → second page
- Output: `SearchResponse` with `total` field indicating total available results

**Data Validation**:
- `offset` must be >= 0
- `limit` must be 1-200
- `total` must accurately reflect total results across all pages
- If `offset >= total` → return empty items array

**Edge Cases**:
- User types new search while previous page is loading → cancel previous request, start new search
- Backend returns `total=0` → show empty state
- Network error during load more → show retry button at bottom
- Very fast scrolling → debounce scroll events (300ms)

---

## Design Decisions

### Technical Approach

**Architecture**: No architecture changes. Fix existing code in place.

**Key Components**:
1. `models.go` — Track struct (remove omitempty)
2. `main.go` — stream handler (replace probeUpstream with direct proxy), search handler (raise limit cap)
3. `extractor.go` — stream cache TTL (increase to 5 min)
4. `hls.go` — add logging, verify ffmpeg path
5. `frontend/frontend/src/store/player.ts` — duration correction heuristic
6. `frontend/frontend/src/features/search/SearchPage.tsx` — infinite scroll, playlist add button
7. `mobile/lib/state/player_controller.dart` — duration correction heuristic
8. `mobile/lib/screens/search_screen.dart` — infinite scroll
9. `mobile/lib/screens/playlist_detail_screen.dart` — add track FAB

**Data Storage**: No schema changes. Cache files in `media/stream-cache/` (already exists).

**Interface Design**: No API changes. Existing endpoints support all required functionality (`offset`/`limit` params already exist on search, `POST /v1/playlists/{id}/tracks` already exists).

### Constraints
- **Performance**: Time-to-first-audio-byte < 3s for YouTube. Search response < 2s.
- **Compatibility**: Must work on iPhone Safari (primary), Chrome, Firefox. Flutter on iOS.
- **Security**: No auth changes. Existing session/cookie auth applies to all endpoints.
- **Scalability**: Single user, single Raspberry Pi. No concurrent user concerns.

### Risk Assessment
- **Technical Risks**:
  - Removing `probeUpstream` may break CDNs that don't send Content-Length → keep as fallback
  - Raising search limit to 200 may increase yt-dlp memory usage → monitor on Pi
  - HLS remux may fail if ffmpeg not installed → verify before deploy
- **Dependency Risks**:
  - yt-dlp may stop supporting SoundCloud → keep yt-dlp updated
  - YouTube CDN may change response format → error handling covers this
- **Schedule Risks**:
  - All fixes are independent → can be done in parallel
  - No external dependencies → no blocking

---

## Acceptance Criteria

### Functional Acceptance
- [ ] YouTube track plays with correct duration (±5s of actual length)
- [ ] YouTube track starts playing within 3 seconds of tap
- [ ] SoundCloud track plays (first play within 30s, subsequent plays instant)
- [ ] Playlist detail screen has "+" button that opens track search
- [ ] Search results load in pages of 20, up to 200 total
- [ ] Duration never shows 2x actual length
- [ ] Scrubbing works on both YouTube and SoundCloud tracks
- [ ] Error messages shown for failed playback (not silent failures)

### Quality Standards
- [ ] `go test ./...` passes
- [ ] `cd frontend/frontend && npm test -- --run` passes
- [ ] `cd frontend/frontend && npm run build` succeeds
- [ ] No new lint warnings
- [ ] All new code has error handling

### User Acceptance
- [ ] Play a YouTube track → hear audio within 3s → duration correct
- [ ] Play a SoundCloud track → hear audio within 30s → can scrub
- [ ] Open playlist → tap "+" → search → add track → see it in list
- [ ] Search "lofi" → scroll → see more results load → scroll to bottom

---

## Execution Phases

### Phase 1: Fix Duration (BUG 1)
**Goal**: Duration displays correctly for all tracks
- [ ] Remove `omitempty` from `DurationSeconds` in `models.go`
- [ ] Add duration correction heuristic to `player.ts` (halve when provider=0, audio>300s)
- [ ] Add same heuristic to `player_controller.dart`
- [ ] Test: `curl /v1/search?q=test | jq '.items[0].duration_seconds'` returns number
- [ ] Test: Play YouTube track, verify duration matches
- **Deliverables**: Correct duration on all platforms
- **Time**: 30 minutes

### Phase 2: Fast Streaming (BUG 2)
**Goal**: YouTube playback starts in <3s
- [ ] Replace `probeUpstream` with direct streaming proxy in `main.go`
- [ ] Forward client's `Range` header to upstream
- [ ] Copy upstream response headers (Content-Length, Content-Type, Accept-Ranges)
- [ ] Increase stream cache TTL from 60s to 300s in `extractor.go`
- [ ] Test: `curl -w '%{time_starttransfer}' -o /dev/null /v1/stream/youtube_stream:ID` < 3s
- **Deliverables**: Fast YouTube streaming
- **Time**: 1 hour

### Phase 3: SoundCloud (BUG 3)
**Goal**: SoundCloud tracks play
- [ ] Verify ffmpeg: `which ffmpeg` returns `/usr/bin/ffmpeg`
- [ ] Verify yt-dlp: `yt-dlp --dump-json "https://soundcloud.com/humbvit/c418-sweden"`
- [ ] Add logging to `hls.go:materialize()` — log track ID, remux time, output size
- [ ] Test: `curl /v1/search?q=test&providers=soundcloud_stream` returns results
- [ ] Test: Play SoundCloud track, verify audio plays
- **Deliverables**: Working SoundCloud playback
- **Time**: 1 hour

### Phase 4: Playlist UX (BUG 4)
**Goal**: Users can add tracks to playlists from playlist screen
- [ ] Flutter: Add FAB to `PlaylistDetailScreen` that opens track search dialog
- [ ] React: Add "Add track" button to `PlaylistCard` that opens inline search
- [ ] Search dialog: text input → search API → results with "+" button → `POST /v1/playlists/{id}/tracks`
- [ ] After add: refresh playlist, show new track
- [ ] Test: Open playlist → "+" → search → add → see track in list
- **Deliverables**: Add-to-playlist from playlist screen
- **Time**: 1.5 hours

### Phase 5: Infinite Scroll (BUG 5)
**Goal**: Search results load in pages
- [ ] Backend: Raise limit cap from 50 to 200 in `main.go`
- [ ] React: Add `IntersectionObserver` on last track → load next page via `offset`
- [ ] Flutter: Add `ScrollController` listener → `library.loadMoreResults()`
- [ ] Both: Show loading indicator at bottom, stop when `items.length >= total`
- [ ] Test: Search "lofi" → scroll → see 20, 40, 60... results
- **Deliverables**: Infinite scroll on search
- **Time**: 1 hour

### Phase 6: Deploy
**Goal**: All fixes live
- [ ] `go test ./...`
- [ ] `cd frontend/frontend && npm test -- --run && npm run build`
- [ ] `go build -o bin/music-orchestrator .`
- [ ] Restart backend: `APP_ADDR=:18080 APP_ENABLE_RISKY_EXTRACTORS=true ./bin/music-orchestrator`
- [ ] Restart frontend: `npx vite preview --host 0.0.0.0 --port 5173`
- [ ] Rebuild Flutter container: `cd mobile && docker build -f Dockerfile.flutter -t music-flutter-web:latest .`
- [ ] Restart Flutter container: `docker stop music-flutter-web && docker rm music-flutter-web && docker run -d --name music-flutter-web --restart unless-stopped -p 172.22.0.1:5174:80 music-flutter-web:latest`
- [ ] `git add -A && git commit -m "fix: v0.3.1 — duration, streaming, soundcloud, playlist UX, infinite scroll"`
- [ ] `git push github go-main`
- **Deliverables**: v0.3.1 deployed and running
- **Time**: 30 minutes

---

**Document Version**: 1.0
**Created**: 2026-08-06
**Clarification Rounds**: 0 (all bugs have clear reproduction steps and root causes identified in code)
**Quality Score**: 92/100
