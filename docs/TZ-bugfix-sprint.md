# Music Orchestrator — Bug Fix & Enhancement TZ

## Context

Music Orchestrator is a self-hosted music app with:
- **Go backend** (main.go, extractor.go, providers.go, store.go, auth.go)
- **React/TS frontend** (frontend/frontend/src/ — Vite + TanStack Query + Zustand + Tailwind)
- **Flutter mobile** (mobile/ — Dart, Android/iOS/Web)
- **yt-dlp** integration for YouTube + SoundCloud search/stream/download

Deployed on Raspberry Pi at `music.mibombopussiclat.ru`.
Desktop gets React, mobile User-Agent gets Flutter web (already handled by Traefik).

## Bugs to Fix (ordered by priority)

### BUG 1: Search Race Condition (CRITICAL)
**File:** `frontend/frontend/src/features/search/SearchPage.tsx` lines 98-104, 162-170
**Problem:** When typing in search, after debounce (350ms), results from a DIFFERENT query briefly appear. The debounce fires `setQuery(next)` but if a previous fetch is still in-flight, TanStack Query's `result.data` may contain stale data from the old query.
**Fix:** Strengthen the `hasFreshResult` check: also hide results when `isFetching` is true and the query changed. Show loading skeleton instead of stale results.

### BUG 2: YouTube 2x Duration / Silence (CRITICAL)
**File:** `frontend/frontend/src/features/search/SearchPage.tsx` lines 579-590 (updateBuffered)
**File:** `frontend/frontend/src/store/player.ts` lines 6-14 (correctedDurationSeconds)
**Problem:** Audio element reports ~2x duration because upstream CDN returns Content-Length for video+audio container. After real track ends, audio plays silence until wrong duration.
**Fix:** In `updateBuffered`, prefer `currentTrack.duration_seconds` from provider metadata over `audio.duration` when they differ by >30%. Also add `onEnded` handler to auto-advance.

### BUG 3: iOS Media Session Missing Info (HIGH)
**File:** `frontend/frontend/src/features/search/SearchPage.tsx` lines 550-557
**Problem:** `navigator.mediaSession.metadata` artwork uses raw external URL (YouTube/SoundCloud) which iOS can't fetch due to CORS. Missing `sizes` and `type` properties.
**Fix:** Use `artworkURL()` proxy (already imported at line 7) + add `sizes: '512x512'` and `type: 'image/jpeg'`.

### BUG 4: Playlist — No Add Track Button (HIGH)
**File:** `frontend/frontend/src/features/search/SearchPage.tsx` lines 319-373 (PlaylistCard)
**Problem:** When viewing a playlist, you can REMOVE tracks but there's no UI to ADD new tracks.
**Fix:** Add an "Добавить трек" button at the bottom of the playlist track list that opens a search/track picker. Use existing `addPlaylistTrack` mutation and `useSearch` hook.

### BUG 5: Desktop Nav Bar Disappears on Scroll (MEDIUM)
**File:** `frontend/frontend/src/features/search/SearchPage.tsx` line 133
**Problem:** Nav aside uses `lg:static` which overrides `xl:sticky`. Between lg and xl breakpoints, nav scrolls away.
**Fix:** Change `lg:static lg:inset-auto` to `lg:sticky lg:top-5`.

### BUG 6: SoundCloud Not Working (HIGH)
**File:** `extractor.go` lines 56-110 (Search function)
**Problem:** When `webpage_url` is empty in yt-dlp results, ALL SoundCloud tracks are skipped (lines 98-101).
**Fix:** Add fallback: when `source` is empty, try `info.URL` or construct URL from entry data.

### BUG 7: YouTube Slow Loading (HIGH)
**File:** `providers.go` lines 56-68 (sequential search loop)
**File:** `extractor.go` lines 263-299 (dump — cold yt-dlp process)
**Problem:** YouTube and SoundCloud searches run SEQUENTIALLY. Stream URLs are re-resolved on every play.
**Fix:**
1. Run provider searches in PARALLEL using goroutines + sync.WaitGroup in `providers.go` `Search()`
2. Cache resolved stream URLs for 60 seconds using sync.Map in `extractor.go`

### BUG 8: No Infinite Scroll Pagination (MEDIUM)
**File:** `frontend/frontend/src/features/search/SearchPage.tsx` lines 13, 68, 142, 185
**Problem:** Manual "Ещё 20" button. Re-fetches ALL results from offset 0 each time.
**Fix:** Replace with IntersectionObserver sentinel element. Show spinner at bottom while loading.

## Deployment
After all fixes:
1. `go test ./...` — must pass
2. `cd frontend/frontend && npm test -- --run` — must pass
3. `cd frontend/frontend && npm run build` — must succeed
4. Build backend: `go build -trimpath -ldflags='-s -w' -o bin/music-orchestrator .`
5. Restart: `systemctl --user restart music-orchestrator-backend-test.service music-orchestrator-frontend-test.service`

## DO NOT touch
- auth.go, auth_password.go, store.go — work fine
- mobile/ — Flutter app is separate
- .env, .env.example, deployment files
