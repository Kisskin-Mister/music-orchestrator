package main

import (
	"fmt"
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
