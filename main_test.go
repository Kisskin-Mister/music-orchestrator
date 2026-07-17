package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func testApp(t *testing.T, risky bool) *App {
	t.Helper()
	dir := t.TempDir()
	cfg := Config{Addr: ":0", Environment: "test", APIKeys: map[string]bool{"test-key": true}, CORSOrigins: []string{"*"}, StorePath: filepath.Join(dir, "store.json"), MediaRoot: filepath.Join(dir, "media"), EnableRiskyExtractors: risky, YTDLPBinary: mockYTDLP(t), ExtractorTimeout: 5_000_000_000, DownloadTimeout: 5_000_000_000}
	app, err := NewApp(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return app
}

func mockYTDLP(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "yt-dlp-mock")
	if runtime.GOOS == "windows" {
		path += ".bat"
	}
	script := `#!/usr/bin/env bash
set -e
args="$*"
if [[ "$args" == *"--dump-single-json"* ]]; then
  if [[ "$args" == *"scsearch"* ]]; then
    echo '{"entries":[{"id":"sc1","title":"Mock SoundCloud Song","uploader":"SC Artist","duration":123.4,"webpage_url":"https://soundcloud.com/a/mock-song","thumbnail":"https://img/sc.jpg"}]}'
  elif [[ "$args" == *"ytsearch"* ]]; then
    echo '{"entries":[{"id":"yt1","title":"Mock YouTube Song","uploader":"YT Artist","duration":212,"webpage_url":"https://www.youtube.com/watch?v=yt1","thumbnail":"https://img/yt.jpg"}]}'
  else
    echo '{"id":"yt1","title":"Mock Resolved","uploader":"Artist","duration":111,"webpage_url":"https://www.youtube.com/watch?v=yt1","formats":[{"url":"https://cdn.example/audio.m4a","acodec":"mp4a","vcodec":"none","abr":128}]}'
  fi
  exit 0
fi
out=""
prev=""
for a in "$@"; do
  if [[ "$prev" == "-o" ]]; then out="$a"; fi
  prev="$a"
done
file="${out//%(ext)s/mp3}"
mkdir -p "$(dirname "$file")"
printf 'mock audio' > "$file"
`
	if err := os.WriteFile(path, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestHealthProvidersAndOpenAPI(t *testing.T) {
	app := testApp(t, false)
	for _, path := range []string{"/health", "/v1/providers", "/openapi.json"} {
		r := httptest.NewRecorder()
		app.ServeHTTP(r, httptest.NewRequest("GET", path, nil))
		if r.Code != 200 {
			t.Fatalf("%s got %d", path, r.Code)
		}
	}
}

func TestSearchPlaybackAndDownload(t *testing.T) {
	app := testApp(t, true)
	r := httptest.NewRecorder()
	app.ServeHTTP(r, httptest.NewRequest("GET", "/v1/search?q=lofi&providers=youtube_stream,soundcloud_stream&limit=2", nil))
	if r.Code != 200 {
		t.Fatalf("search %d: %s", r.Code, r.Body.String())
	}
	var sr SearchResponse
	if err := json.Unmarshal(r.Body.Bytes(), &sr); err != nil {
		t.Fatal(err)
	}
	if sr.Total != 2 {
		t.Fatalf("want 2 results got %d", sr.Total)
	}

	r = httptest.NewRecorder()
	app.ServeHTTP(r, httptest.NewRequest("GET", "/v1/playback/youtube_stream:yt1", nil))
	if r.Code != 200 {
		t.Fatalf("playback %d: %s", r.Code, r.Body.String())
	}
	var pb Playback
	_ = json.Unmarshal(r.Body.Bytes(), &pb)
	if pb.PlaybackType != "extractor_stream" || pb.StreamURL == nil {
		t.Fatalf("bad playback %#v", pb)
	}

	body := strings.NewReader(`{"track_id":"youtube_stream:yt1","format":"mp3"}`)
	req := httptest.NewRequest("POST", "/v1/downloads", body)
	req.Header.Set("X-API-Key", "test-key")
	req.Header.Set("Content-Type", "application/json")
	r = httptest.NewRecorder()
	app.ServeHTTP(r, req)
	if r.Code != http.StatusAccepted {
		t.Fatalf("download %d: %s", r.Code, r.Body.String())
	}
	var job Job
	_ = json.Unmarshal(r.Body.Bytes(), &job)
	if job.Status != "succeeded" {
		t.Fatalf("bad job %#v", job)
	}
	mediaURL := fmt.Sprint(job.Result["media_url"])
	r = httptest.NewRecorder()
	app.ServeHTTP(r, httptest.NewRequest("GET", mediaURL, nil))
	if r.Code != 200 {
		t.Fatalf("media %d", r.Code)
	}
}

func TestAuthAndFavoritesPlaylists(t *testing.T) {
	app := testApp(t, false)
	r := httptest.NewRecorder()
	app.ServeHTTP(r, httptest.NewRequest("GET", "/v1/favorites", nil))
	if r.Code != 401 {
		t.Fatalf("want 401 got %d", r.Code)
	}
	req := httptest.NewRequest("POST", "/v1/favorites", strings.NewReader(`{"track_id":"local:seed-1"}`))
	req.Header.Set("X-API-Key", "test-key")
	r = httptest.NewRecorder()
	app.ServeHTTP(r, req)
	if r.Code != 201 {
		t.Fatalf("favorite %d: %s", r.Code, r.Body.String())
	}
	req = httptest.NewRequest("POST", "/v1/playlists", strings.NewReader(`{"name":"Mix"}`))
	req.Header.Set("X-API-Key", "test-key")
	r = httptest.NewRecorder()
	app.ServeHTTP(r, req)
	if r.Code != 201 {
		t.Fatalf("playlist %d: %s", r.Code, r.Body.String())
	}
}
