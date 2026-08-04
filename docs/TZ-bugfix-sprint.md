# Music Orchestrator — Bug Fix & Enhancement TZ
# For: Claude Code (claude opus 5)
# Graph: docs/architecture-graph.html
# Date: 2026-08-04

## Context

Music Orchestrator is a self-hosted music app with:
- **Go backend** (main.go, extractor.go, providers.go, store.go, auth.go)
- **React/TS frontend** (frontend/frontend/src/ — Vite + TanStack Query + Zustand + Tailwind)
- **Flutter mobile** (mobile/ — Dart, Android/iOS/Web)
- **yt-dlp** integration for YouTube + SoundCloud search/stream/download

The app is deployed on Raspberry Pi, served via Traefik at `music.mibombopussiclat.ru`.
Desktop gets React, mobile User-Agent gets Flutter web.

## Bugs to Fix (ordered by priority)

### BUG 1: Search Race Condition (CRITICAL)
**File:** `frontend/frontend/src/features/search/SearchPage.tsx` lines 98-104, 162-170
**Problem:** When typing in search, after debounce (350ms), results from a DIFFERENT query briefly appear. The debounce fires `setQuery(next)` but if a previous fetch is still in-flight, TanStack Query's `result.data` may contain stale data from the old query for one render cycle.
**Fix:**
1. Add `enabled` guard: don't show results when `resultQuery !== query` (already partially done at line 165 but needs strengthening)
2. Use TanStack Query's `keepPreviousData: false` explicitly in useSearch
3. Add AbortController signal to the search API call so previous requests are cancelled
4. Show loading state (not stale results) when query changes but result hasn't arrived yet

### BUG 2: YouTube 2x Duration / Silence (CRITICAL)
**File:** `frontend/frontend/src/features/search/SearchPage.tsx` lines 579-590 (updateBuffered)
**File:** `frontend/frontend/src/store/player.ts` lines 6-14 (correctedDurationSeconds)
**File:** `extractor.go` lines 263-299 (dump), 45-54 (ytdlpFormat)
**Problem:** Audio element reports ~2x duration because the upstream CDN returns Content-Length for video+audio container, not audio-only. After the real 3min track ends, audio continues playing silence until the reported 6min duration.
**Fix:**
1. In `updateBuffered` (SearchPage.tsx:581), prefer `currentTrack.duration_seconds` from provider metadata over `audio.duration` when they differ significantly (>20%)
2. In `player.ts`, `correctedDurationSeconds` should use a tighter threshold and prefer provider duration for extractor streams
3. Add `onEnded` handler to audio element that calls `next()` when the track actually ends (based on real audio data, not reported duration)

### BUG 3: iOS Media Session Missing Info (HIGH)
**File:** `frontend/frontend/src/features/search/SearchPage.tsx` lines 550-557
**Problem:** `navigator.mediaSession.metadata` artwork uses raw external URL (YouTube/SoundCloud) which iOS can't fetch due to CORS. Missing `sizes` and `type` properties.
**Fix:**
```javascript
// Line 552 — change to:
artwork: currentTrack.artwork_url
  ? [{ src: artworkURL(currentTrack.artwork_url) || currentTrack.artwork_url, sizes: '512x512', type: 'image/jpeg' }]
  : []
```
Import `artworkURL` from `@/api/client` (already exported at client.ts line 26).

### BUG 4: Playlist — No Add Track Button (HIGH)
**File:** `frontend/frontend/src/features/search/SearchPage.tsx` lines 319-373 (PlaylistCard)
**Problem:** When viewing a playlist, you can REMOVE tracks but there's no UI to ADD new tracks.
**Fix:** Add an "Добавить трек" button at the bottom of the playlist track list that:
1. Opens a search/track picker overlay
2. Uses the existing `addPlaylistTrack` mutation (queries.ts)
3. Shows library tracks + allows searching for new ones
4. Reuses the same pattern as playlist creation flow (lines 282-310)

### BUG 5: Desktop Nav Bar Disappears on Scroll (MEDIUM)
**File:** `frontend/frontend/src/features/search/SearchPage.tsx` line 133
**Problem:** Nav aside uses `lg:static` which overrides `xl:sticky`. Between lg and xl breakpoints, nav scrolls away.
**Fix:** Change `lg:static lg:inset-auto` to `lg:sticky lg:top-5` (or remove lg:static entirely).

### BUG 6: SoundCloud Not Working (HIGH)
**File:** `extractor.go` lines 56-110 (Search function)
**File:** `providers.go` lines 117-133 (scIDFromURL)
**Problem:** SoundCloud search via `scsearch{limit}:{query}` may fail because:
1. yt-dlp's SoundCloud extractor may need updated client_id
2. When `webpage_url` is empty in results, ALL tracks are skipped (line 98-101)
**Fix:**
1. Add fallback in extractor.go: when `source` (webpage_url) is empty, use `info.URL` or construct URL from `info.ID` and `info.Title`
2. Add `--extractor-args "soundcloud:client_id=..."` if APP_SOUNDCLOUD_CLIENT_ID is set in config
3. Log yt-dlp stderr when SoundCloud search returns 0 results so we can diagnose
4. Test with: `yt-dlp "scsearch3:never gonna give you up" --dump-single-json --skip-download`

### BUG 7: YouTube Slow Loading (HIGH)
**File:** `providers.go` lines 56-68 (sequential search loop)
**File:** `extractor.go` lines 263-299 (dump — cold yt-dlp process)
**File:** `main.go` lines 431-500 (stream — re-resolves on every play)
**Problem:** 3 sequential blocking operations: yt-dlp cold start (5-10s), sequential provider search (2x if both enabled), stream URL re-resolution on every play.
**Fix:**
1. **Parallel providers:** In `providers.go` `Search()`, run YouTube and SoundCloud searches concurrently with `sync.WaitGroup` + goroutines, merge results
2. **Cache stream URLs:** In `extractor.go` or `main.go`, cache resolved stream URLs for 60s using a `sync.Map` with TTL
3. **Add yt-dlp flags:** `--no-check-certificates --no-warnings --flat-playlist` to reduce overhead
4. **Reduce default timeout:** Change `APP_EXTRACTOR_TIMEOUT_SECONDS` default from 30 to 15

### BUG 8: No Infinite Scroll Pagination (MEDIUM)
**File:** `frontend/frontend/src/features/search/SearchPage.tsx` lines 13, 68, 142, 185
**Problem:** Manual "Ещё 20" button. Re-fetches ALL results from offset 0 each time (O(n²) network).
**Fix:**
1. Replace "load more" button with `IntersectionObserver` sentinel element
2. Use true offset-based pagination: track `offset` state, increment by PAGE_SIZE on each load
3. Append new results to existing list instead of re-fetching everything
4. Show spinner at bottom while loading more
5. Server already supports `offset` param (main.go line 320)

Sketch:
```tsx
const [offset, setOffset] = useState(0);
const sentinelRef = useRef<HTMLDivElement>(null);
// ... IntersectionObserver triggers setOffset(n => n + PAGE_SIZE)
// useSearch(query, providers, PAGE_SIZE, offset)
// Merge results: [...prevResults, ...newResults]
```

## Desktop UI Alignment with Flutter
The React desktop UI should match Flutter's design language. Key differences to align:
- Use same color tokens (accent #b8f545, surface #0f1117, etc.)
- Match track row layout (artwork, title, artist, source icon, actions)
- Same mini player → full sheet morph animation
- Same settings layout with source toggles
- Same playlist card design with cover art

## Deployment
After all fixes:
1. Run `go test ./...` — must pass
2. Run `cd frontend/frontend && npm test -- --run` — must pass
3. Run `cd frontend/frontend && npm run build` — must succeed
4. Build backend: `go build -trimpath -ldflags='-s -w' -o bin/music-orchestrator .`
5. Restart services: `systemctl --user restart music-orchestrator-backend-test.service`
6. The frontend service runs `vite preview` which serves from `frontend/frontend/dist/`
7. Restart frontend: `systemctl --user restart music-orchestrator-frontend-test.service`
8. Verify: `curl https://music.mibombopussiclat.ru/health`

## Files to Modify (most likely)
- `frontend/frontend/src/features/search/SearchPage.tsx` — bugs 1,2,3,4,5,8
- `frontend/frontend/src/api/queries.ts` — bug 1 (abort signal), bug 8 (offset pagination)
- `frontend/frontend/src/api/client.ts` — bug 1 (abort), bug 3 (artworkURL import)
- `frontend/frontend/src/store/player.ts` — bug 2 (duration correction)
- `extractor.go` — bugs 6,7 (SoundCloud fallback, caching, parallel)
- `providers.go` — bug 7 (parallel search)
- `main.go` — bug 7 (stream cache)
- `config.go` — bug 7 (timeout default)

## DO NOT touch
- auth.go, auth_password.go — auth works fine
- store.go — data layer works fine
- mobile/ — Flutter app is separate, not part of this fix
- .env, .env.example — no config changes
- Docker/deployment files — no infra changes
