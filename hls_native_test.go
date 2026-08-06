package main

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// hlsFixture is a CDN standing in for SoundCloud: it serves whatever playlists
// a test registers plus numbered segments, and records what was asked for.
type hlsFixture struct {
	server    *httptest.Server
	playlists map[string]string // path -> body

	mu        sync.Mutex
	requests  []string
	headers   http.Header
	inFlight  int
	peak      int
	segmentMS time.Duration // per-segment delay, to make downloads overlap
	// fail maps a segment path to the number of times it should fail before
	// answering normally. Set to a large number for a segment that never works.
	fail map[string]int
	// hold blocks every segment until it is closed, for cancellation tests.
	hold chan struct{}
}

func newHLSFixture(t *testing.T) *hlsFixture {
	t.Helper()
	f := &hlsFixture{playlists: map[string]string{}, fail: map[string]int{}}
	f.server = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.server.Close)
	return f
}

func (f *hlsFixture) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.requests = append(f.requests, r.URL.Path)
	f.headers = r.Header.Clone()
	body, isPlaylist := f.playlists[r.URL.Path]
	remaining := f.fail[r.URL.Path]
	if remaining > 0 {
		f.fail[r.URL.Path] = remaining - 1
	}
	delay, hold := f.segmentMS, f.hold
	if !isPlaylist {
		f.inFlight++
		if f.inFlight > f.peak {
			f.peak = f.inFlight
		}
	}
	f.mu.Unlock()

	if isPlaylist {
		if remaining > 0 {
			http.Error(w, "playlist unavailable", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		_, _ = w.Write([]byte(body))
		return
	}

	defer func() {
		f.mu.Lock()
		f.inFlight--
		f.mu.Unlock()
	}()
	if hold != nil {
		select {
		case <-hold:
		case <-r.Context().Done():
			return
		}
	}
	if delay > 0 {
		select {
		case <-time.After(delay):
		case <-r.Context().Done():
			return
		}
	}
	if remaining > 0 {
		http.Error(w, "segment unavailable", http.StatusInternalServerError)
		return
	}
	// The body names the segment, so the concatenation is readable in a failure
	// message and any reordering is obvious.
	_, _ = w.Write([]byte(segmentBody(strings.TrimPrefix(r.URL.Path, "/"))))
}

func segmentBody(name string) string { return "<" + name + ">" }

func (f *hlsFixture) url(path string) string { return f.server.URL + path }

func (f *hlsFixture) requestCount(path string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, req := range f.requests {
		if req == path {
			n++
		}
	}
	return n
}

func (f *hlsFixture) peakConcurrency() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.peak
}

// mediaPlaylist builds a playlist of n segments named seg-00.mp3 upwards,
// relative to the playlist's own directory the way SoundCloud writes them.
func mediaPlaylist(prefix string, n int) (body string, names []string) {
	var b strings.Builder
	b.WriteString("#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-TARGETDURATION:10\n")
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("%sseg-%02d.mp3", prefix, i)
		fmt.Fprintf(&b, "#EXTINF:9.98,\n%s\n", name)
		names = append(names, name)
	}
	b.WriteString("#EXT-X-ENDLIST\n")
	return b.String(), names
}

func expectedAudio(names []string) string {
	var b strings.Builder
	for _, name := range names {
		b.WriteString(segmentBody(name))
	}
	return b.String()
}

// A media playlist is the common case: SoundCloud hands out one directly.
// Relative entries resolve against the playlist URL, absolute ones stand.
func TestParseHLSPlaylistMediaPlaylist(t *testing.T) {
	f := newHLSFixture(t)
	f.playlists["/tracks/1/playlist.m3u8"] = "#EXTM3U\n" +
		"#EXT-X-VERSION:3\n" +
		"#EXTINF:10,\nseg-00.mp3\n" +
		"# a comment that is not a tag\n" +
		"#EXTINF:10,\n../shared/seg-01.mp3\n" +
		"#EXTINF:10,\nhttps://other.example/seg-02.mp3\n" +
		"#EXT-X-ENDLIST\n"

	segments, err := parseHLSPlaylist(context.Background(), f.url("/tracks/1/playlist.m3u8"), nil)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	want := []string{
		f.url("/tracks/1/seg-00.mp3"),
		f.url("/tracks/shared/seg-01.mp3"),
		"https://other.example/seg-02.mp3",
	}
	if len(segments) != len(want) {
		t.Fatalf("got %d segments (%v), want %d", len(segments), segments, len(want))
	}
	for i := range want {
		if segments[i] != want[i] {
			t.Fatalf("segment %d = %q, want %q", i, segments[i], want[i])
		}
	}
}

// A master playlist has to be followed, and the audio-only rendition is the one
// worth following: a video variant would drag a whole muxed stream down for its
// audio track.
func TestParseHLSPlaylistFollowsMasterToAudioOnly(t *testing.T) {
	f := newHLSFixture(t)
	body, names := mediaPlaylist("", 3)
	f.playlists["/audio/hi/playlist.m3u8"] = body
	f.playlists["/master.m3u8"] = "#EXTM3U\n" +
		"#EXT-X-STREAM-INF:BANDWIDTH=2000000,RESOLUTION=1280x720,CODECS=\"avc1.640028,mp4a.40.2\"\n" +
		"/video/hi.m3u8\n" +
		"#EXT-X-STREAM-INF:BANDWIDTH=64000,CODECS=\"mp4a.40.2\"\n" +
		"/audio/lo/playlist.m3u8\n" +
		"#EXT-X-STREAM-INF:BANDWIDTH=128000,CODECS=\"mp4a.40.2\"\n" +
		"/audio/hi/playlist.m3u8\n"

	segments, err := parseHLSPlaylist(context.Background(), f.url("/master.m3u8"), nil)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	// Highest bitrate among the audio-only variants, and its segments resolve
	// against its own directory rather than the master's.
	if len(segments) != len(names) {
		t.Fatalf("got %d segments (%v), want %d", len(segments), segments, len(names))
	}
	if segments[0] != f.url("/audio/hi/seg-00.mp3") {
		t.Fatalf("first segment = %q, want the audio variant's %q", segments[0], f.url("/audio/hi/seg-00.mp3"))
	}
	if f.requestCount("/video/hi.m3u8") != 0 {
		t.Fatal("the video variant was fetched — a muxed stream would have been downloaded for its audio")
	}
}

// An EXT-X-MEDIA rendition is the other way a master names its audio track.
func TestParseHLSPlaylistFollowsAudioRendition(t *testing.T) {
	f := newHLSFixture(t)
	body, _ := mediaPlaylist("", 2)
	f.playlists["/audio/playlist.m3u8"] = body
	f.playlists["/master.m3u8"] = "#EXTM3U\n" +
		"#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID=\"aud\",NAME=\"Main, English\",URI=\"/audio/playlist.m3u8\"\n" +
		"#EXT-X-STREAM-INF:BANDWIDTH=3000000,RESOLUTION=1920x1080,AUDIO=\"aud\"\n" +
		"/video/hi.m3u8\n"

	segments, err := parseHLSPlaylist(context.Background(), f.url("/master.m3u8"), nil)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(segments) != 2 || segments[0] != f.url("/audio/seg-00.mp3") {
		t.Fatalf("segments = %v, want the audio rendition's", segments)
	}
}

// Anything that cannot be turned into a plain concatenation must be reported,
// not guessed at: the caller falls back to ffmpeg, which can handle it.
func TestParseHLSPlaylistDeclinesUnsupportedPlaylists(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		want string
	}{
		{
			"encrypted",
			"#EXTM3U\n#EXT-X-KEY:METHOD=AES-128,URI=\"key.bin\"\n#EXTINF:10,\nseg-00.mp3\n",
			"encrypted",
		},
		{
			"fragmented mp4",
			"#EXTM3U\n#EXT-X-MAP:URI=\"init.mp4\"\n#EXTINF:10,\nseg-00.m4s\n",
			"EXT-X-MAP",
		},
		{
			"byte range segments",
			"#EXTM3U\n#EXTINF:10,\n#EXT-X-BYTERANGE:75232@0\nwhole.mp3\n",
			"byte-range",
		},
		{
			"empty playlist",
			"#EXTM3U\n#EXT-X-VERSION:3\n",
			"neither segments nor variants",
		},
		{
			"not a playlist at all",
			"<html>404</html>",
			"not an m3u8 playlist",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newHLSFixture(t)
			f.playlists["/playlist.m3u8"] = tc.body
			_, err := parseHLSPlaylist(context.Background(), f.url("/playlist.m3u8"), nil)
			if err == nil {
				t.Fatal("expected the parser to decline, got a segment list")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q does not mention %q", err, tc.want)
			}
		})
	}

	// METHOD=NONE is not encryption, it is the tag that turns it off.
	f := newHLSFixture(t)
	f.playlists["/playlist.m3u8"] = "#EXTM3U\n#EXT-X-KEY:METHOD=NONE\n#EXTINF:10,\nseg-00.mp3\n"
	if _, err := parseHLSPlaylist(context.Background(), f.url("/playlist.m3u8"), nil); err != nil {
		t.Fatalf("METHOD=NONE was treated as encryption: %v", err)
	}
}

// A playlist that answers 404 or 500 is a parse failure like any other.
func TestParseHLSPlaylistSurfacesHTTPFailure(t *testing.T) {
	f := newHLSFixture(t)
	if _, err := parseHLSPlaylist(context.Background(), f.url("/missing.m3u8"), nil); err == nil {
		t.Fatal("expected an error for a playlist that does not exist")
	}
}

// yt-dlp's headers carry the CDN's cookie and user agent; without them
// SoundCloud answers 403.
func TestParseHLSPlaylistSendsHeaders(t *testing.T) {
	f := newHLSFixture(t)
	body, _ := mediaPlaylist("", 1)
	f.playlists["/playlist.m3u8"] = body

	headers := map[string]string{"Authorization": "OAuth token", "User-Agent": "TestAgent/1.0"}
	if _, err := parseHLSPlaylist(context.Background(), f.url("/playlist.m3u8"), headers); err != nil {
		t.Fatalf("parse: %v", err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if got := f.headers.Get("Authorization"); got != "OAuth token" {
		t.Fatalf("Authorization = %q, want the resolved one", got)
	}
	if got := f.headers.Get("User-Agent"); got != "TestAgent/1.0" {
		t.Fatalf("User-Agent = %q, want the resolved one", got)
	}
}

// The download is parallel, but the file is not: audio written out of order is
// noise. Segments here finish in reverse, which is what a naive "write as they
// arrive" implementation gets wrong.
func TestDownloadHLSSegmentsWritesInPlaylistOrder(t *testing.T) {
	const count = 12
	// The earlier the segment, the longer it takes, so the last one is ready
	// first and the writer has to hold everything back for segment 0.
	reversed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/")
		var idx int
		_, _ = fmt.Sscanf(name, "seg-%d.mp3", &idx)
		time.Sleep(time.Duration(count-idx) * 15 * time.Millisecond)
		_, _ = w.Write([]byte(segmentBody(name)))
	}))
	defer reversed.Close()

	var (
		segments []string
		names    []string
	)
	for i := 0; i < count; i++ {
		name := fmt.Sprintf("seg-%02d.mp3", i)
		names = append(names, name)
		segments = append(segments, reversed.URL+"/"+name)
	}

	var out bytes.Buffer
	written, err := downloadHLSSegments(context.Background(), &out, segments, nil)
	if err != nil {
		t.Fatalf("download: %v", err)
	}
	if want := expectedAudio(names); out.String() != want {
		t.Fatalf("wrote %q, want playlist order %q", out.String(), want)
	}
	if written != int64(out.Len()) {
		t.Fatalf("reported %d bytes, wrote %d", written, out.Len())
	}
}

// The whole point of the native path: segments are fetched at the same time
// instead of one round trip after another.
func TestDownloadHLSSegmentsDownloadsInParallel(t *testing.T) {
	f := newHLSFixture(t)
	const (
		count = 12
		delay = 120 * time.Millisecond
	)
	f.mu.Lock()
	f.segmentMS = delay
	f.mu.Unlock()

	var segments []string
	for i := 0; i < count; i++ {
		segments = append(segments, f.url(fmt.Sprintf("/seg-%02d.mp3", i)))
	}

	started := time.Now()
	var out bytes.Buffer
	if _, err := downloadHLSSegments(context.Background(), &out, segments, nil); err != nil {
		t.Fatalf("download: %v", err)
	}
	elapsed := time.Since(started)

	// Sequential would be count*delay; six at a time is a quarter of that even
	// with a generous allowance for a loaded machine.
	if limit := count * delay / 2; elapsed > limit {
		t.Fatalf("took %s for %d segments of %s — that is sequential, not parallel", elapsed, count, delay)
	}
	if peak := f.peakConcurrency(); peak < 2 {
		t.Fatalf("peak concurrency was %d — segments were fetched one at a time", peak)
	}
	if peak := f.peakConcurrency(); peak > hlsSegmentWorkers {
		t.Fatalf("peak concurrency was %d, more than the %d workers", peak, hlsSegmentWorkers)
	}
}

// A gap in the middle of a track is worse than a failure the caller can fall
// back from, so one dead segment fails the download.
func TestDownloadHLSSegmentsFailsOnDeadSegment(t *testing.T) {
	f := newHLSFixture(t)
	var segments []string
	for i := 0; i < 6; i++ {
		segments = append(segments, f.url(fmt.Sprintf("/seg-%02d.mp3", i)))
	}
	f.mu.Lock()
	f.fail["/seg-03.mp3"] = 1000 // never recovers
	f.mu.Unlock()

	var out bytes.Buffer
	_, err := downloadHLSSegments(context.Background(), &out, segments, nil)
	if err == nil {
		t.Fatal("expected the download to fail, got a complete file")
	}
	if !strings.Contains(err.Error(), "segment 4/6") {
		t.Fatalf("error %q does not say which segment failed", err)
	}
}

// A single hiccup should not cost the listener the track.
func TestDownloadHLSSegmentsRetriesOnce(t *testing.T) {
	f := newHLSFixture(t)
	var (
		segments []string
		names    []string
	)
	for i := 0; i < 4; i++ {
		name := fmt.Sprintf("seg-%02d.mp3", i)
		names = append(names, name)
		segments = append(segments, f.url("/"+name))
	}
	f.mu.Lock()
	f.fail["/seg-02.mp3"] = 1
	f.mu.Unlock()

	var out bytes.Buffer
	if _, err := downloadHLSSegments(context.Background(), &out, segments, nil); err != nil {
		t.Fatalf("a single failing segment was not retried: %v", err)
	}
	if want := expectedAudio(names); out.String() != want {
		t.Fatalf("wrote %q, want %q", out.String(), want)
	}
	if n := f.requestCount("/seg-02.mp3"); n != 2 {
		t.Fatalf("segment fetched %d times, want one retry", n)
	}
}

// A cancelled download has to stop, and stop everything: the remux timeout
// expiring must not leave six workers pulling segments for a track nobody is
// listening to.
func TestDownloadHLSSegmentsStopsOnCancel(t *testing.T) {
	f := newHLSFixture(t)
	f.mu.Lock()
	f.hold = make(chan struct{})
	f.mu.Unlock()

	var segments []string
	for i := 0; i < 40; i++ {
		segments = append(segments, f.url(fmt.Sprintf("/seg-%02d.mp3", i)))
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(80 * time.Millisecond)
		cancel()
	}()

	done := make(chan error, 1)
	var out bytes.Buffer
	go func() {
		_, err := downloadHLSSegments(ctx, &out, segments, nil)
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("a cancelled download reported success")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a cancelled download did not return — the fetchers are still running")
	}
	cancel()
	// Only the segments already in flight were requested; nothing kept going.
	f.mu.Lock()
	before := len(f.requests)
	f.mu.Unlock()
	time.Sleep(200 * time.Millisecond)
	f.mu.Lock()
	after := len(f.requests)
	f.mu.Unlock()
	if after != before {
		t.Fatalf("%d more segments were requested after cancellation", after-before)
	}
}

// The native path exists for the formats ffmpeg would have copied. Everything
// else has to go to ffmpeg, because concatenated Opus segments are not an MP3.
func TestCanUseNativeHLS(t *testing.T) {
	for _, tc := range []struct {
		name   string
		target StreamTarget
		want   bool
	}{
		{"mp3 playlist", StreamTarget{HLS: true, ACodec: "mp3", Ext: "mp3"}, true},
		{"aac playlist", StreamTarget{HLS: true, ACodec: "mp4a.40.2", Ext: "m4a"}, true},
		{"bare aac", StreamTarget{HLS: true, ACodec: "aac", Ext: "aac"}, true},
		{"mp4 container", StreamTarget{HLS: true, ACodec: "", Ext: "mp4"}, true},
		{"opus needs transcoding", StreamTarget{HLS: true, ACodec: "opus", Ext: "webm"}, false},
		{"vorbis needs transcoding", StreamTarget{HLS: true, ACodec: "vorbis", Ext: "webm"}, false},
		{"plain file is not a playlist", StreamTarget{HLS: false, ACodec: "mp3", Ext: "mp3"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := canUseNativeHLS(tc.target); got != tc.want {
				t.Fatalf("canUseNativeHLS = %v, want %v", got, tc.want)
			}
			// Whatever the native path accepts, ffmpeg would have copied: the two
			// must not disagree about which formats are transcoded.
			if tc.want {
				args := strings.Join(containerFor(tc.target).args, " ")
				if !strings.Contains(args, "-c:a copy") {
					t.Fatalf("ffmpeg would have transcoded this format (%s), so a raw concatenation is wrong", args)
				}
			}
		})
	}
}

// End to end through the cache: a copyable playlist is downloaded without
// ffmpeg at all, which is proved by there being no usable ffmpeg to run.
func TestRemuxUsesNativeDownloaderInsteadOfFFmpeg(t *testing.T) {
	f := newHLSFixture(t)
	body, names := mediaPlaylist("", 8)
	f.playlists["/playlist.m3u8"] = body

	cache := hlsTestCache(t, filepath.Join(t.TempDir(), "ffmpeg-does-not-exist"))
	target := mp3Target(f.url("/playlist.m3u8"))
	trackID := "soundcloud_stream:native"

	path, container, done := cache.materializeProgressive(trackID, target)
	if !strings.HasSuffix(path, ".partial") {
		t.Fatalf("expected a growing file, got %q", path)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("native download failed: %v", err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("native download never finished")
	}

	final := cache.pathFor(trackID, container)
	data, err := os.ReadFile(final)
	if err != nil {
		t.Fatalf("finished file: %v", err)
	}
	if want := expectedAudio(names); string(data) != want {
		t.Fatalf("cache file holds %q, want the segments in order %q", data, want)
	}
}

// The listener must not wait for the last segment. The native downloader writes
// each segment as it becomes the next one, so a follower reader hears the start
// of the track while the rest is still downloading.
func TestRemuxNativeIsProgressive(t *testing.T) {
	f := newHLSFixture(t)
	body, names := mediaPlaylist("", 8)
	f.playlists["/playlist.m3u8"] = body
	f.mu.Lock()
	f.segmentMS = 120 * time.Millisecond
	f.mu.Unlock()

	cache := hlsTestCache(t, filepath.Join(t.TempDir(), "ffmpeg-does-not-exist"))
	target := mp3Target(f.url("/playlist.m3u8"))
	trackID := "soundcloud_stream:progressive"

	started := time.Now()
	path, container, done := cache.materializeProgressive(trackID, target)
	fr := &followerReader{path: path, fallback: cache.pathFor(trackID, container), done: done, poll: 10 * time.Millisecond}
	defer fr.Close()

	buf := make([]byte, len(segmentBody(names[0])))
	if _, err := readFullFollower(fr, buf); err != nil {
		t.Fatalf("first segment: %v", err)
	}
	ttfb := time.Since(started)
	if string(buf) != segmentBody(names[0]) {
		t.Fatalf("first bytes = %q, want %q", buf, segmentBody(names[0]))
	}
	// Two rounds of six parallel segments take about 240ms; anything near the
	// full download means the reader waited for the whole file.
	if ttfb > time.Second {
		t.Fatalf("first byte took %s — the download was not progressive", ttfb)
	}
}

func readFullFollower(fr *followerReader, buf []byte) (int, error) {
	read := 0
	for read < len(buf) {
		n, err := fr.Read(buf[read:])
		read += n
		if err != nil {
			return read, err
		}
	}
	return read, nil
}

// ffmpeg is still the fallback, and the fallback has to be reachable: a
// playlist the native parser declines must not fail the play.
func TestRemuxFallsBackToFFmpegForUnsupportedPlaylist(t *testing.T) {
	f := newHLSFixture(t)
	f.playlists["/playlist.m3u8"] = "#EXTM3U\n#EXT-X-KEY:METHOD=AES-128,URI=\"k\"\n#EXTINF:10,\nseg-00.mp3\n"

	marker := filepath.Join(t.TempDir(), "runs")
	cache := hlsTestCache(t, stubFFmpeg(t, 4, 20*time.Millisecond, marker))
	target := mp3Target(f.url("/playlist.m3u8"))
	trackID := "soundcloud_stream:encrypted"

	_, container, done := cache.materializeProgressive(trackID, target)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("fallback failed: %v", err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("fallback never finished")
	}
	if n := runCount(t, marker); n != 1 {
		t.Fatalf("ffmpeg ran %d times, want 1 — the native path did not fall back", n)
	}
	data, err := os.ReadFile(cache.pathFor(trackID, container))
	if err != nil {
		t.Fatalf("finished file: %v", err)
	}
	if string(data) != stubOutput(4) {
		t.Fatalf("finished file holds %q, want ffmpeg's %q", data, stubOutput(4))
	}
}

// Transcoding never reaches the native path at all — no playlist is fetched,
// because ffmpeg has to re-encode the audio anyway.
func TestRemuxUsesFFmpegForTranscodedFormats(t *testing.T) {
	f := newHLSFixture(t)
	body, _ := mediaPlaylist("", 3)
	f.playlists["/playlist.m3u8"] = body

	marker := filepath.Join(t.TempDir(), "runs")
	cache := hlsTestCache(t, stubFFmpeg(t, 3, 20*time.Millisecond, marker))
	target := StreamTarget{URL: f.url("/playlist.m3u8"), HLS: true, ACodec: "opus", Ext: "webm", Duration: 100}

	_, _, done := cache.materializeProgressive("soundcloud_stream:opus", target)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("transcode failed: %v", err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("transcode never finished")
	}
	if n := runCount(t, marker); n != 1 {
		t.Fatalf("ffmpeg ran %d times, want 1", n)
	}
	if f.requestCount("/playlist.m3u8") != 0 {
		t.Fatal("the native path fetched a playlist it cannot copy")
	}
}

// A download that dies partway cannot silently become a cache file: the
// truncated .partial has to go, and the readers have to hear about it.
func TestRemuxNativeReportsMidDownloadFailure(t *testing.T) {
	f := newHLSFixture(t)
	body, _ := mediaPlaylist("", 30)
	f.playlists["/playlist.m3u8"] = body
	f.mu.Lock()
	// Late enough that the first segments are already written, so falling back
	// to ffmpeg would rewrite bytes a listener has heard.
	f.fail["/seg-20.mp3"] = 1000
	f.mu.Unlock()

	cache := hlsTestCache(t, filepath.Join(t.TempDir(), "ffmpeg-does-not-exist"))
	target := mp3Target(f.url("/playlist.m3u8"))
	trackID := "soundcloud_stream:broken"

	tmp, container, done := cache.materializeProgressive(trackID, target)
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("a download that lost a segment reported success")
		}
	case <-time.After(20 * time.Second):
		t.Fatal("the failing download never finished")
	}
	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Fatal("the truncated .partial was left behind and will be mistaken for a cache file")
	}
	if _, err := os.Stat(cache.pathFor(trackID, container)); !os.IsNotExist(err) {
		t.Fatal("a failed download was published as a finished cache file")
	}
}

// Two listeners on one cold track still share a single download.
func TestRemuxNativeSharedBetweenListeners(t *testing.T) {
	f := newHLSFixture(t)
	body, names := mediaPlaylist("", 6)
	f.playlists["/playlist.m3u8"] = body
	f.mu.Lock()
	f.segmentMS = 60 * time.Millisecond
	f.mu.Unlock()

	cache := hlsTestCache(t, filepath.Join(t.TempDir(), "ffmpeg-does-not-exist"))
	target := mp3Target(f.url("/playlist.m3u8"))
	trackID := "soundcloud_stream:shared-native"

	firstPath, container, firstDone := cache.materializeProgressive(trackID, target)
	secondPath, _, secondDone := cache.materializeProgressive(trackID, target)
	if firstPath != secondPath {
		t.Fatalf("listeners attached to different files: %q and %q", firstPath, secondPath)
	}
	for i, done := range []<-chan error{firstDone, secondDone} {
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("listener %d reported %v", i, err)
			}
		case <-time.After(20 * time.Second):
			t.Fatalf("listener %d never heard the download finish", i)
		}
	}
	if n := f.requestCount("/seg-00.mp3"); n != 1 {
		t.Fatalf("segment fetched %d times — the download was not shared", n)
	}
	data, err := os.ReadFile(cache.pathFor(trackID, container))
	if err != nil {
		t.Fatalf("finished file: %v", err)
	}
	if want := expectedAudio(names); string(data) != want {
		t.Fatalf("cache file holds %q, want %q", data, want)
	}
}

// hlsAttrs has to survive the quoted commas real playlists contain.
func TestHLSAttrs(t *testing.T) {
	attrs := hlsAttrs(`BANDWIDTH=128000,CODECS="mp4a.40.2,avc1.4d401f",NAME="Main, English",TYPE=AUDIO`)
	for name, want := range map[string]string{
		"BANDWIDTH": "128000",
		"CODECS":    "mp4a.40.2,avc1.4d401f",
		"NAME":      "Main, English",
		"TYPE":      "AUDIO",
	} {
		if got := attrs[name]; got != want {
			t.Fatalf("%s = %q, want %q", name, got, want)
		}
	}
}

// The degenerate playlists: one segment, and none at all.
func TestDownloadHLSSegmentsSingleSegment(t *testing.T) {
	f := newHLSFixture(t)
	var out bytes.Buffer
	n, err := downloadHLSSegments(context.Background(), &out, []string{f.url("/seg-00.mp3")}, nil)
	if err != nil {
		t.Fatalf("download: %v", err)
	}
	if out.String() != segmentBody("seg-00.mp3") || n != int64(out.Len()) {
		t.Fatalf("wrote %q (%d bytes), want %q", out.String(), n, segmentBody("seg-00.mp3"))
	}
	if _, err := downloadHLSSegments(context.Background(), &out, nil, nil); err == nil {
		t.Fatal("an empty playlist reported success")
	}
}
