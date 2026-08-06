package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// throttlingCDN imitates googlevideo: it answers byte ranges in full, but a
// request for the whole file goes quiet after the first megabyte. That is the
// exact behaviour that made tracks play for two minutes and then fall silent.
func throttlingCDN(t *testing.T, body []byte, requests *int32) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(requests, 1)
		total := int64(len(body))
		spec := strings.TrimPrefix(r.Header.Get("Range"), "bytes=")
		if spec == "" {
			// No range: hand over a truncated prefix, like a throttled connection
			// that stops delivering partway through.
			w.Header().Set("Content-Type", "audio/mp4")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(body[:1024])
			return
		}
		var start, end int64
		if _, err := fmt.Sscanf(spec, "%d-%d", &start, &end); err != nil {
			http.Error(w, "bad range", http.StatusBadRequest)
			return
		}
		if end >= total {
			end = total - 1
		}
		w.Header().Set("Content-Type", "audio/mp4")
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, total))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(body[start : end+1])
	}))
}

func payload(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i % 251)
	}
	return b
}

// The whole point of the proxy: the listener must receive every byte even
// though a single upstream connection would have stalled.
func TestStreamProxyDeliversWholeFileDespiteThrottling(t *testing.T) {
	body := payload(10 << 20) // 10 MB — more than one chunk
	var upstreamRequests int32
	cdn := throttlingCDN(t, body, &upstreamRequests)
	defer cdn.Close()

	app := streamApp(t, cdn.URL)
	rec := httptest.NewRecorder()
	streamThrough(t, app, rec, "")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if got := rec.Body.Len(); got != len(body) {
		t.Fatalf("truncated stream: got %d bytes, want %d", got, len(body))
	}
	if !bytesEqual(rec.Body.Bytes(), body) {
		t.Fatal("delivered bytes do not match the source")
	}
	// Proof it actually chunked rather than getting lucky on one request.
	if n := atomic.LoadInt32(&upstreamRequests); n < 3 {
		t.Fatalf("expected several ranged requests, got %d", n)
	}
}

// Seeking must keep working: without Accept-Ranges and a correct Content-Range
// the scrubber goes dead.
func TestStreamProxyHonoursClientRange(t *testing.T) {
	body := payload(3 << 20)
	var requests int32
	cdn := throttlingCDN(t, body, &requests)
	defer cdn.Close()

	app := streamApp(t, cdn.URL)
	rec := httptest.NewRecorder()
	streamThrough(t, app, rec, "bytes=1048576-2097151")

	if rec.Code != http.StatusPartialContent {
		t.Fatalf("expected 206, got %d", rec.Code)
	}
	if got, want := rec.Header().Get("Content-Range"), fmt.Sprintf("bytes 1048576-2097151/%d", len(body)); got != want {
		t.Fatalf("Content-Range = %q, want %q", got, want)
	}
	if rec.Header().Get("Accept-Ranges") != "bytes" {
		t.Fatal("Accept-Ranges missing — the client would disable seeking")
	}
	if !bytesEqual(rec.Body.Bytes(), body[1048576:2097152]) {
		t.Fatal("seeked range returned the wrong bytes")
	}
}

// Time to first audio byte is one upstream round trip, not two: the old
// bytes=0-0 size probe added seconds of silence before playback started.
func TestStreamProxySkipsSizeProbe(t *testing.T) {
	body := payload(3 << 20) // fits in a single chunk
	var requests int32
	var ranges []string
	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		spec := r.Header.Get("Range")
		ranges = append(ranges, spec)
		var start, end int64
		if _, err := fmt.Sscanf(strings.TrimPrefix(spec, "bytes="), "%d-%d", &start, &end); err != nil {
			http.Error(w, "bad range", http.StatusBadRequest)
			return
		}
		if end >= int64(len(body)) {
			end = int64(len(body)) - 1
		}
		w.Header().Set("Content-Type", "audio/mp4")
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(body)))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(body[start : end+1])
	}))
	defer cdn.Close()

	app := streamApp(t, cdn.URL)
	rec := httptest.NewRecorder()
	streamThrough(t, app, rec, "")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !bytesEqual(rec.Body.Bytes(), body) {
		t.Fatal("delivered bytes do not match the source")
	}
	if got := atomic.LoadInt32(&requests); got != 1 {
		t.Fatalf("expected a single upstream request, got %d (%v)", got, ranges)
	}
	if ranges[0] == "bytes=0-0" {
		t.Fatal("first upstream request is still a size probe")
	}
	if got, want := rec.Header().Get("Content-Length"), fmt.Sprint(len(body)); got != want {
		t.Fatalf("Content-Length = %q, want %q", got, want)
	}
}

// A CDN link that expired while the listener was paused must be re-resolved
// instead of surfacing as a dead player.
func TestStreamProxyReresolvesAfterUpstream403(t *testing.T) {
	body := payload(1 << 20)
	var requests int32
	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&requests, 1) == 1 {
			http.Error(w, "expired", http.StatusForbidden)
			return
		}
		var start, end int64
		if _, err := fmt.Sscanf(strings.TrimPrefix(r.Header.Get("Range"), "bytes="), "%d-%d", &start, &end); err != nil {
			http.Error(w, "bad range", http.StatusBadRequest)
			return
		}
		if end >= int64(len(body)) {
			end = int64(len(body)) - 1
		}
		w.Header().Set("Content-Type", "audio/mp4")
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(body)))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(body[start : end+1])
	}))
	defer cdn.Close()

	app := streamApp(t, cdn.URL)
	rec := httptest.NewRecorder()
	streamThrough(t, app, rec, "")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 after re-resolve, got %d", rec.Code)
	}
	if !bytesEqual(rec.Body.Bytes(), body) {
		t.Fatal("delivered bytes do not match the source")
	}
}

// A playlist has to reach the listener while ffmpeg is still downloading it.
// Waiting for the remux is what made SoundCloud tracks take 10-30s to start.
func TestStreamHLSProgressive(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "runs")
	const steps = 6
	const pause = 150 * time.Millisecond
	app := hlsStreamApp(t, stubFFmpeg(t, steps, pause, marker))

	server := httptest.NewServer(app)
	defer server.Close()

	req, err := http.NewRequest(http.MethodGet, server.URL+"/v1/stream/youtube_stream:yt1", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Range", "bytes=0-1023")

	started := time.Now()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for a track still being remuxed, got %d", resp.StatusCode)
	}

	first := make([]byte, 8)
	if _, err := io.ReadFull(resp.Body, first); err != nil {
		t.Fatalf("first bytes: %v", err)
	}
	ttfb := time.Since(started)
	// The remux runs for roughly steps*pause; audio has to start long before it
	// ends, not after.
	if limit := steps * pause / 2; ttfb > limit {
		t.Fatalf("first byte took %s, remux takes ~%s — the response waited for the whole file", ttfb, steps*pause)
	}
	if got, want := string(first), stubOutput(steps)[:8]; got != want {
		t.Fatalf("first bytes = %q, want %q", got, want)
	}

	rest, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if got, want := string(first)+string(rest), stubOutput(steps); got != want {
		t.Fatalf("streamed %q, want the complete %q", got, want)
	}
	if resp.Header.Get("Content-Type") != "audio/mpeg" {
		t.Fatalf("Content-Type = %q, want audio/mpeg", resp.Header.Get("Content-Type"))
	}
	// The length is unknown while ffmpeg is writing, so the scrubber is fed the
	// duration instead of a guessed byte count.
	if resp.Header.Get("X-Content-Duration") == "" {
		t.Fatal("X-Content-Duration missing — the player has nothing to size the scrubber with")
	}

	// The finished file is now a cache hit, served with real ranges.
	seek, err := http.NewRequest(http.MethodGet, server.URL+"/v1/stream/youtube_stream:yt1", nil)
	if err != nil {
		t.Fatal(err)
	}
	seek.Header.Set("Range", "bytes=9-17")
	cached, err := http.DefaultClient.Do(seek)
	if err != nil {
		t.Fatal(err)
	}
	defer cached.Body.Close()
	if cached.StatusCode != http.StatusPartialContent {
		t.Fatalf("expected 206 from the cached file, got %d", cached.StatusCode)
	}
	body, err := io.ReadAll(cached.Body)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(body), stubOutput(steps)[9:18]; got != want {
		t.Fatalf("ranged read = %q, want %q", got, want)
	}
	if n := runCount(t, marker); n != 1 {
		t.Fatalf("started %d ffmpeg processes, want 1", n)
	}
}

// hlsStreamApp resolves to an m3u8 playlist, which sends the stream handler
// down the remux path with the given ffmpeg standing in for a real one.
//
// The playlist URL answers 404, so the native downloader declines it at once
// and ffmpeg gets the job — which is the path these tests are about.
func hlsStreamApp(t *testing.T, ffmpeg string) *App {
	t.Helper()
	dir := t.TempDir()
	playlist := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(playlist.Close)
	bin := filepath.Join(dir, "yt-dlp-hls-mock")
	script := "#!/usr/bin/env bash\nset -e\ncat <<'JSON'\n" +
		`{"id":"yt1","title":"Mock","duration":200,"formats":[{"url":"` + playlist.URL + "/playlist.m3u8" +
		`","protocol":"m3u8_native","acodec":"mp3","ext":"mp3","vcodec":"none","abr":128}]}` +
		"\nJSON\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	app, err := NewApp(Config{
		Addr:                  ":0",
		Environment:           "test",
		APIKeys:               map[string]bool{"test-key": true},
		CORSOrigins:           []string{"*"},
		StorePath:             filepath.Join(dir, "store.json"),
		MediaRoot:             filepath.Join(dir, "media"),
		EnableRiskyExtractors: true,
		YTDLPBinary:           bin,
		FFmpegBinary:          ffmpeg,
		ExtractorTimeout:      20 * time.Second,
		DownloadTimeout:       20 * time.Second,
		HLSRemuxTimeout:       20 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	return app
}

// Warm-on-play is the only thing standing between the client pressing play and
// a round trip to the CDN. For a plain file it fetches the head of the track,
// and the stream request that follows must play it instead of asking again.
func TestWarmStreamPrefetchesFirstChunk(t *testing.T) {
	body := payload(512 << 10) // smaller than one prefetch chunk
	var requests int32
	cdn := throttlingCDN(t, body, &requests)
	defer cdn.Close()

	app := streamApp(t, cdn.URL)
	app.warmStream("youtube_stream", "yt1")
	if got := atomic.LoadInt32(&requests); got != 1 {
		t.Fatalf("warm-up made %d upstream requests, want 1", got)
	}

	rec := httptest.NewRecorder()
	streamThrough(t, app, rec, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !bytesEqual(rec.Body.Bytes(), body) {
		t.Fatal("the prefetched chunk did not reach the listener intact")
	}
	if got := atomic.LoadInt32(&requests); got != 1 {
		t.Fatalf("upstream was asked %d times — the stream did not use the prefetched chunk", got)
	}
	if got, want := rec.Header().Get("Content-Length"), fmt.Sprint(len(body)); got != want {
		t.Fatalf("Content-Length = %q, want %q", got, want)
	}
}

// A prefetch only covers the head of the file. The rest is fetched as usual,
// and the seam between the two has to be exact.
func TestWarmStreamPrefetchIsFollowedByTheRest(t *testing.T) {
	body := payload(3 << 20) // larger than one prefetch chunk
	var requests int32
	cdn := throttlingCDN(t, body, &requests)
	defer cdn.Close()

	app := streamApp(t, cdn.URL)
	app.warmStream("youtube_stream", "yt1")

	rec := httptest.NewRecorder()
	streamThrough(t, app, rec, "")
	if !bytesEqual(rec.Body.Bytes(), body) {
		t.Fatal("the stream is not the file — the prefetched head and the rest do not line up")
	}
}

// A playlist has nothing to prefetch: it is materialised instead, and asking
// the CDN for the first megabyte of an m3u8 would cache a text file as audio.
func TestWarmStreamDoesNotPrefetchPlaylists(t *testing.T) {
	app := hlsStreamApp(t, stubFFmpeg(t, 1, 10*time.Millisecond, filepath.Join(t.TempDir(), "runs")))
	app.warmStream("youtube_stream", "yt1")

	app.prefetch.mu.Lock()
	held := len(app.prefetch.entries)
	app.prefetch.mu.Unlock()
	if held != 0 {
		t.Fatal("a playlist was stored in the prefetch cache")
	}
	// It was materialised instead, which is what the HLS cache is for.
	if _, _, err := app.hls.materialize(context.Background(), "youtube_stream:yt1", StreamTarget{HLS: true, ACodec: "mp3", Ext: "mp3"}); err != nil {
		t.Fatalf("the warm-up did not leave a materialised playlist behind: %v", err)
	}
}

// The head of a track is handed out once, and only to the URL it came from: a
// re-resolve between the warm-up and the stream request can change format, and
// half of one encoding followed by the rest of another is silence.
func TestPrefetchCacheHandsOutOnceAndChecksTheURL(t *testing.T) {
	cache := newPrefetchCache()
	entry := &prefetchEntry{url: "https://cdn.example/a.m4a", data: []byte("head"), total: 400, expires: time.Now().Add(time.Minute)}
	cache.put("youtube_stream:a", entry)

	if _, ok := cache.take("youtube_stream:a", "https://cdn.example/other.m4a"); ok {
		t.Fatal("a chunk from a different URL was handed out")
	}
	cache.put("youtube_stream:a", entry)
	if got, ok := cache.take("youtube_stream:a", entry.url); !ok || string(got.data) != "head" {
		t.Fatal("the prefetched chunk was not handed out to the request it was fetched for")
	}
	if _, ok := cache.take("youtube_stream:a", entry.url); ok {
		t.Fatal("the same chunk was handed out twice — it is streamed away on first use")
	}

	// Expired entries are as good as absent.
	cache.put("youtube_stream:b", &prefetchEntry{url: "u", data: []byte("stale"), total: 5, expires: time.Now().Add(-time.Second)})
	if _, ok := cache.take("youtube_stream:b", "u"); ok {
		t.Fatal("an expired chunk was handed out")
	}

	// And the cache cannot grow without bound as tracks are skipped.
	for i := 0; i < prefetchMaxEntries*3; i++ {
		cache.put(fmt.Sprintf("youtube_stream:%d", i), &prefetchEntry{url: "u", data: []byte("x"), total: 1, expires: time.Now().Add(time.Duration(i) * time.Minute)})
	}
	cache.mu.Lock()
	held := len(cache.entries)
	cache.mu.Unlock()
	if held > prefetchMaxEntries {
		t.Fatalf("cache holds %d entries, more than the %d ceiling", held, prefetchMaxEntries)
	}
}

// A seek is not what warm-on-play fetched, so it goes to the CDN as before.
func TestPrefetchIsNotUsedForSeeks(t *testing.T) {
	body := payload(2 << 20)
	var requests int32
	cdn := throttlingCDN(t, body, &requests)
	defer cdn.Close()

	app := streamApp(t, cdn.URL)
	app.warmStream("youtube_stream", "yt1")

	rec := httptest.NewRecorder()
	streamThrough(t, app, rec, "bytes=1048576-1048675")
	if rec.Code != http.StatusPartialContent {
		t.Fatalf("expected 206, got %d", rec.Code)
	}
	if !bytesEqual(rec.Body.Bytes(), body[1048576:1048676]) {
		t.Fatal("a seek was answered with the prefetched head of the file")
	}
}

func TestParseRangeHint(t *testing.T) {
	cases := []struct {
		header     string
		start, end int64
		ok         bool
	}{
		{"", 0, -1, true},
		{"bytes=0-99", 0, 99, true},
		{"bytes=500-", 500, -1, true},
		{"bytes=-100", 0, 0, false}, // needs the total, falls back to probing
		{"bytes=abc", 0, 0, false},
		{"bytes=0-99,200-299", 0, 0, false},
		{"bytes=99-9", 0, 0, false},
	}
	for _, c := range cases {
		start, end, ok := parseRangeHint(c.header)
		if start != c.start || end != c.end || ok != c.ok {
			t.Errorf("parseRangeHint(%q) = (%d,%d,%v), want (%d,%d,%v)",
				c.header, start, end, ok, c.start, c.end, c.ok)
		}
	}
}

func TestParseRange(t *testing.T) {
	const total = 1000
	cases := []struct {
		header      string
		start, end  int64
		partial, ok bool
	}{
		{"", 0, 999, false, true},
		{"bytes=0-99", 0, 99, true, true},
		{"bytes=500-", 500, 999, true, true},
		{"bytes=-100", 900, 999, true, true},
		{"bytes=0-99999", 0, 999, true, true},
		{"bytes=1000-", 0, 0, false, false},
		{"bytes=abc", 0, 0, false, false},
		{"bytes=0-99,200-299", 0, 0, false, false},
	}
	for _, c := range cases {
		start, end, partial, ok := parseRange(c.header, total)
		if start != c.start || end != c.end || partial != c.partial || ok != c.ok {
			t.Errorf("parseRange(%q) = (%d,%d,%v,%v), want (%d,%d,%v,%v)",
				c.header, start, end, partial, ok, c.start, c.end, c.partial, c.ok)
		}
	}
}

// streamApp builds an app whose extractor resolves to the given test CDN, so the
// real handler runs end to end without needing yt-dlp installed.
func streamApp(t *testing.T, cdnURL string) *App {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "yt-dlp-stream-mock")
	script := "#!/usr/bin/env bash\nset -e\ncat <<'JSON'\n" +
		`{"id":"yt1","title":"Mock","duration":200,"formats":[{"url":"` + cdnURL +
		`","acodec":"mp4a","vcodec":"none","abr":128,"http_headers":{"User-Agent":"TestAgent/1.0"}}]}` +
		"\nJSON\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	app, err := NewApp(Config{
		Addr:                  ":0",
		Environment:           "test",
		APIKeys:               map[string]bool{"test-key": true},
		CORSOrigins:           []string{"*"},
		StorePath:             filepath.Join(dir, "store.json"),
		MediaRoot:             filepath.Join(dir, "media"),
		EnableRiskyExtractors: true,
		YTDLPBinary:           bin,
		ExtractorTimeout:      20 * time.Second,
		DownloadTimeout:       20 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	return app
}

func streamThrough(t *testing.T, app *App, rec *httptest.ResponseRecorder, rangeHeader string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/v1/stream/youtube_stream:yt1", nil)
	if rangeHeader != "" {
		req.Header.Set("Range", rangeHeader)
	}
	app.ServeHTTP(rec, req)
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
