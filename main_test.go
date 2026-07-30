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
	cfg := Config{Addr: ":0", Environment: "test", APIKeys: map[string]bool{"test-key": true, "other-key": true}, CORSOrigins: []string{"*"}, StorePath: filepath.Join(dir, "store.json"), MediaRoot: filepath.Join(dir, "media"), EnableRiskyExtractors: risky, YTDLPBinary: mockYTDLP(t), ExtractorTimeout: 5_000_000_000, DownloadTimeout: 5_000_000_000}
	app, err := NewApp(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return app
}

func authReq(method, path, body, key string) *http.Request {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("X-API-Key", key)
	req.Header.Set("Content-Type", "application/json")
	return req
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
    if [[ "$args" != *"--flat-playlist"* ]]; then
      echo 'youtube search must use --flat-playlist' >&2
      exit 23
    fi
    echo '{"entries":[{"id":"UCFl7yKfcRcFmIUbKeCA-SJQ","title":"Mock Artist Channel","uploader":"Mock Artist","duration":0,"url":"https://www.youtube.com/channel/UCFl7yKfcRcFmIUbKeCA-SJQ","ie_key":"YoutubeTab"},{"id":"yt1","title":"Mock YouTube Song","uploader":"YT Artist - Topic","duration":212,"url":"https://www.youtube.com/watch?v=yt1","ie_key":"Youtube"}]}'
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
	for _, item := range sr.Items {
		if item.ProviderID == "youtube_stream" && item.ArtworkURL != "https://i.ytimg.com/vi/yt1/hqdefault.jpg" {
			t.Fatalf("youtube search should synthesize thumbnail fallback, got %#v", item)
		}
	}

	req := httptest.NewRequest("GET", "/v1/playback/youtube_stream:yt1", nil)
	req.Header.Set("X-API-Key", "test-key")
	r = httptest.NewRecorder()
	app.ServeHTTP(r, req)
	if r.Code != 200 {
		t.Fatalf("playback %d: %s", r.Code, r.Body.String())
	}
	var pb Playback
	_ = json.Unmarshal(r.Body.Bytes(), &pb)
	if pb.PlaybackType != "extractor_stream" || pb.StreamURL == nil {
		t.Fatalf("bad playback %#v", pb)
	}

	body := strings.NewReader(`{"track_id":"youtube_stream:yt1","format":"mp3"}`)
	req = httptest.NewRequest("POST", "/v1/downloads", body)
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

	req = httptest.NewRequest("GET", "/v1/search?q=demo&providers=youtube_stream&limit=2", nil)
	req.Header.Set("X-API-Key", "test-key")
	r = httptest.NewRecorder()
	app.ServeHTTP(r, req)
	if r.Code != 200 {
		t.Fatalf("search after download %d: %s", r.Code, r.Body.String())
	}
	_ = json.Unmarshal(r.Body.Bytes(), &sr)
	if len(sr.Items) == 0 || !sr.Items[0].Downloaded || sr.Items[0].DownloadMediaURL == "" {
		t.Fatalf("download state not reflected in search: %#v", sr.Items)
	}
	for _, item := range sr.Items {
		if strings.HasPrefix(item.ProviderTrackID, "UC") || strings.Contains(item.SourceURL, "/channel/") {
			t.Fatalf("youtube flat search should skip channel results: %#v", item)
		}
	}
	if !sr.Items[0].Official {
		t.Fatalf("youtube topic track should be marked official: %#v", sr.Items[0])
	}

	body = strings.NewReader(`{"track_id":"youtube_stream:yt1","format":"mp3"}`)
	req = httptest.NewRequest("POST", "/v1/downloads", body)
	req.Header.Set("X-API-Key", "test-key")
	req.Header.Set("Content-Type", "application/json")
	r = httptest.NewRecorder()
	app.ServeHTTP(r, req)
	if r.Code != http.StatusOK {
		t.Fatalf("cached download should be 200 got %d: %s", r.Code, r.Body.String())
	}
	var cached Job
	_ = json.Unmarshal(r.Body.Bytes(), &cached)
	if cached.ID != job.ID || cached.Payload["cached"] != true {
		t.Fatalf("want cached original job got %#v", cached)
	}

	req = httptest.NewRequest("GET", "/v1/playback/youtube_stream:yt1", nil)
	req.Header.Set("X-API-Key", "test-key")
	r = httptest.NewRecorder()
	app.ServeHTTP(r, req)
	if r.Code != 200 {
		t.Fatalf("cached playback %d: %s", r.Code, r.Body.String())
	}
	_ = json.Unmarshal(r.Body.Bytes(), &pb)
	if pb.PlaybackType != "local_cached_stream" || pb.StreamURL == nil || *pb.StreamURL != mediaURL {
		t.Fatalf("playback should use saved media after download: %#v", pb)
	}

	req = httptest.NewRequest("DELETE", "/v1/downloads/youtube_stream:yt1", nil)
	req.Header.Set("X-API-Key", "test-key")
	r = httptest.NewRecorder()
	app.ServeHTTP(r, req)
	if r.Code != http.StatusNoContent {
		t.Fatalf("delete download got %d", r.Code)
	}
	r = httptest.NewRecorder()
	app.ServeHTTP(r, httptest.NewRequest("GET", mediaURL, nil))
	if r.Code != http.StatusNotFound {
		t.Fatalf("deleted media should 404 got %d", r.Code)
	}
}

func TestSoundCloudExtractorRejectsPrivateOrForeignURLs(t *testing.T) {
	if _, err := scURLFromID(scIDFromURL("http://169.254.169.254/latest/meta-data")); err == nil {
		t.Fatal("metadata service URL must be rejected")
	}
	if _, err := scURLFromID(scIDFromURL("https://evil.example/song")); err == nil {
		t.Fatal("foreign host URL must be rejected")
	}
	if u, err := scURLFromID(scIDFromURL("https://soundcloud.com/artist/song")); err != nil || u != "https://soundcloud.com/artist/song" {
		t.Fatalf("valid soundcloud URL should pass, got %q err=%v", u, err)
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
	var playlist Playlist
	if err := json.Unmarshal(r.Body.Bytes(), &playlist); err != nil {
		t.Fatal(err)
	}

	req = httptest.NewRequest("POST", "/v1/playlists/"+playlist.ID+"/tracks", strings.NewReader(`{"track_id":"local:seed-1"}`))
	req.Header.Set("X-API-Key", "test-key")
	r = httptest.NewRecorder()
	app.ServeHTTP(r, req)
	if r.Code != 201 {
		t.Fatalf("playlist track %d: %s", r.Code, r.Body.String())
	}
	if err := json.Unmarshal(r.Body.Bytes(), &playlist); err != nil {
		t.Fatal(err)
	}
	if len(playlist.Tracks) != 1 || playlist.Tracks[0].Track == nil || playlist.Tracks[0].Track.Title == "" {
		t.Fatalf("playlist should store user-friendly track snapshot: %#v", playlist.Tracks)
	}
	firstItemID := playlist.Tracks[0].ID
	if firstItemID == "" || playlist.TrackCount != 1 || playlist.DurationSeconds == 0 {
		t.Fatalf("playlist aggregates/item id missing: %#v", playlist)
	}

	req = httptest.NewRequest("POST", "/v1/playlists/"+playlist.ID+"/tracks", strings.NewReader(`{"track_id":"local:seed-1"}`))
	req.Header.Set("X-API-Key", "test-key")
	r = httptest.NewRecorder()
	app.ServeHTTP(r, req)
	if r.Code != 200 {
		t.Fatalf("duplicate playlist track should be idempotent 200 got %d: %s", r.Code, r.Body.String())
	}
	if err := json.Unmarshal(r.Body.Bytes(), &playlist); err != nil {
		t.Fatal(err)
	}
	if len(playlist.Tracks) != 1 || playlist.Tracks[0].ID != firstItemID {
		t.Fatalf("duplicate should not append: %#v", playlist.Tracks)
	}
}

func TestAPIKeysHaveIsolatedLibraryState(t *testing.T) {
	app := testApp(t, false)

	r := httptest.NewRecorder()
	app.ServeHTTP(r, authReq("POST", "/v1/favorites", `{"track_id":"local:seed-1"}`, "test-key"))
	if r.Code != http.StatusCreated {
		t.Fatalf("favorite as first user %d: %s", r.Code, r.Body.String())
	}

	r = httptest.NewRecorder()
	app.ServeHTTP(r, authReq("GET", "/v1/favorites", ``, "other-key"))
	if r.Code != http.StatusOK {
		t.Fatalf("list favorites as second user %d: %s", r.Code, r.Body.String())
	}
	var favorites []Favorite
	if err := json.Unmarshal(r.Body.Bytes(), &favorites); err != nil {
		t.Fatal(err)
	}
	if len(favorites) != 0 {
		t.Fatalf("second user must not see first user's favorites: %#v", favorites)
	}

	r = httptest.NewRecorder()
	app.ServeHTTP(r, authReq("GET", "/v1/favorites", ``, "test-key"))
	if strings.Contains(r.Body.String(), "owner_id") {
		t.Fatalf("favorites response must not leak owner_id: %s", r.Body.String())
	}

	r = httptest.NewRecorder()
	app.ServeHTTP(r, authReq("POST", "/v1/playlists", `{"name":"Private Mix"}`, "test-key"))
	if r.Code != http.StatusCreated {
		t.Fatalf("playlist as first user %d: %s", r.Code, r.Body.String())
	}
	r = httptest.NewRecorder()
	app.ServeHTTP(r, authReq("GET", "/v1/playlists", ``, "other-key"))
	if r.Code != http.StatusOK {
		t.Fatalf("list playlists as second user %d: %s", r.Code, r.Body.String())
	}
	var playlists []Playlist
	if err := json.Unmarshal(r.Body.Bytes(), &playlists); err != nil {
		t.Fatal(err)
	}
	if len(playlists) != 0 {
		t.Fatalf("second user must not see first user's playlists: %#v", playlists)
	}
}
