# TZ: Tier 3 — Native HLS Parser + YouTube Warm-up

## Context

SoundCloud serves audio only as HLS (m3u8 playlists). Current flow:
1. yt-dlp resolves playlist URL (~2-4s)
2. ffmpeg downloads all HLS segments sequentially, remuxes to MP3 (~6-30s)
3. followerReader serves the .partial progressively

Tier 3 eliminates ffmpeg for HLS by parsing the playlist in Go and downloading segments in parallel.

YouTube streaming is slow because the first play from cold starts a yt-dlp resolve (~2-4s). Warm-on-play helps but only if the user clicked play before the stream request.

## Task 1: Native HLS Downloader (hls_native.go)

Create `hls_native.go` with a native Go HLS segment downloader that replaces ffmpeg for SoundCloud HLS.

### HLS Playlist Parser

```go
// parseHLSPlaylist fetches an m3u8 URL and returns segment URLs in order.
// Handles both master playlists (pick highest bitrate audio variant)
// and media playlists (direct segment list).
func parseHLSPlaylist(ctx context.Context, url string, headers map[string]string) ([]string, error)
```

Steps:
1. Fetch m3u8 URL with headers
2. If `#EXT-X-STREAM-INF` exists → it's a master playlist, pick the first audio-only variant (or highest bitrate), fetch that m3u8
3. Parse `#EXTINF` + segment URLs (relative to base URL)
4. Return ordered segment URLs

### Parallel Segment Downloader

```go
// downloadHLSSegments downloads segments in parallel and writes them in order to w.
// Returns total bytes written.
//
// Concurrency: 6 goroutines downloading segments, ordered writer ensures
// segments are written in playlist order even if downloaded out of order.
func downloadHLSSegments(ctx context.Context, w io.Writer, segments []string, headers map[string]string) (int64, error)
```

Architecture:
- **Fetcher pool**: N goroutines (default 6) pulling from a segment channel
- **Ordered writer**: each segment gets a sequence number; writer blocks until the next expected segment arrives
- **Buffer**: each segment is downloaded into a bytes.Buffer, then sent to the ordered writer
- **Error handling**: one failed segment aborts the whole download

### Integration with hlsCache

Modify `hlsCache.remux()` to use native downloader when the source is SoundCloud (or any HLS with MP3/AAC segments):

```go
func (c *hlsCache) remux(job *hlsJob, trackID, path string, target StreamTarget) {
    // If native HLS is possible (MP3 or AAC codec, no transcode needed)
    if canUseNativeHLS(target) {
        c.remuxNative(job, trackID, path, target)
        return
    }
    // Fallback to ffmpeg for transcoding (Opus, etc.)
    c.remuxFFmpeg(job, trackID, path, target)
}
```

`canUseNativeHLS` returns true when codec is mp3, aac, mp4a, or m4a — anything that doesn't need transcoding.

For AAC segments: write raw ADTS frames directly (no container needed, same as ffmpeg -f adts).

For MP3 segments: write raw MP3 frames directly (same as ffmpeg -f mp3).

### Expected improvement

| Scenario | Current (ffmpeg) | Native HLS |
|---|---|---|
| Cold (3.5MB MP3) | ~6s | **~1.5s** (6 parallel segments) |
| Cold (8MB AAC) | ~15s | **~3s** |
| Warm (cache hit) | 0.09s | 0.09s (unchanged) |

The key win: ffmpeg downloads segments sequentially (one at a time). Native downloader fetches 6 in parallel.

## Task 2: YouTube Streaming Warm-up Enhancement

Current warm-on-play fires from `/v1/playback` but only warms the HLS cache (SoundCloud). YouTube doesn't benefit because it doesn't use HLS.

### Add YouTube CDN pre-fetch

In `warmStream()`, for non-HLS targets (YouTube), pre-fetch the first chunk and store it in a short-lived cache:

```go
func (a *App) warmStream(providerID, pid string) {
    target, err := a.providers.extractor.StreamTarget(providerID, pid)
    if err != nil { return }
    
    if target.HLS {
        // existing: warm HLS cache
        a.hls.materialize(context.Background(), trackID, target)
    } else {
        // NEW: pre-fetch first 1MB from CDN to warm the connection
        a.prefetchFirstChunk(target)
    }
}
```

This way when `/v1/stream` arrives, the CDN connection is already warm and the first chunk is cached.

## Task 3: Tests

Add `hls_native_test.go`:
- Test m3u8 master playlist parsing (mock HTTP server)
- Test m3u8 media playlist parsing
- Test parallel download with ordered writing
- Test error propagation (one segment fails)
- Test cancellation (context cancelled mid-download)

## Files to create/modify

| File | Action |
|---|---|
| `hls_native.go` | **CREATE** — HLS parser + parallel downloader |
| `hls_native_test.go` | **CREATE** — tests |
| `hls.go` | **MODIFY** — integrate native path into remux() |
| `main.go` | **MODIFY** — enhance warmStream() for YouTube |

## Constraints

- Keep ffmpeg as fallback for transcoding (Opus → MP3)
- Don't change the progressive streaming / followerReader logic — it works great
- Don't change the stream cache or search cache
- All existing tests must pass
- New tests must pass with `go test -race ./...`

## Verification

```bash
go build ./...
go test -race ./...
curl -s http://localhost:18080/health
# Play a SoundCloud track — should start within 2s instead of 6-30s
# Play a YouTube track — should start within 1-2s
```
