package main

import (
	"context"
	"fmt"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// stubFFmpeg stands in for a remux: it appends to its output file in steps with
// a pause between them, which is what makes the file readable long before it is
// complete. Every run records itself in a marker file so a test can tell how
// many processes were started.
func stubFFmpeg(t *testing.T, steps int, pause time.Duration, marker string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ffmpeg-stub")
	script := fmt.Sprintf(`#!/usr/bin/env bash
out="${!#}"
echo run >> %q
: > "$out"
for i in $(seq 1 %d); do
  printf 'chunk-%%02d;' "$i" >> "$out"
  sleep %s
done
`, marker, steps, fmt.Sprintf("%.2f", pause.Seconds()))
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func stubOutput(steps int) string {
	var b strings.Builder
	for i := 1; i <= steps; i++ {
		fmt.Fprintf(&b, "chunk-%02d;", i)
	}
	return b.String()
}

func hlsTestCache(t *testing.T, ffmpeg string) *hlsCache {
	t.Helper()
	return newHLSCache(Config{MediaRoot: t.TempDir(), FFmpegBinary: ffmpeg, HLSRemuxTimeout: 10 * time.Second})
}

func mp3Target(url string) StreamTarget {
	return StreamTarget{URL: url, HLS: true, ACodec: "mp3", Ext: "mp3", Duration: 212}
}

func runCount(t *testing.T, marker string) int {
	t.Helper()
	data, err := os.ReadFile(marker)
	if os.IsNotExist(err) {
		return 0
	}
	if err != nil {
		t.Fatal(err)
	}
	return len(strings.Fields(string(data)))
}

// The whole progressive path rests on this: at the end of what has been written
// so far the reader must wait for the writer instead of reporting the end of
// the audio.
func TestFollowerReader(t *testing.T) {
	path := filepath.Join(t.TempDir(), "track.mp3")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	chunks := []string{"first;", "second;", "third;"}
	go func() {
		for _, chunk := range chunks {
			time.Sleep(60 * time.Millisecond)
			if _, err := f.WriteString(chunk); err != nil {
				done <- err
				return
			}
		}
		_ = f.Close()
		done <- nil
	}()

	fr := &followerReader{path: path, done: done, poll: 10 * time.Millisecond}
	defer fr.Close()
	started := time.Now()
	got, err := io.ReadAll(fr)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if want := strings.Join(chunks, ""); string(got) != want {
		t.Fatalf("read %q, want %q", got, want)
	}
	// A reader that treated the first EOF as the end would have returned after
	// the first chunk, well inside the writer's schedule.
	if elapsed := time.Since(started); elapsed < 150*time.Millisecond {
		t.Fatalf("returned after %s — it stopped at EOF instead of following the writer", elapsed)
	}
}

// A file that does not exist yet is the normal case: the handler attaches
// before ffmpeg has created its output.
func TestFollowerReaderWaitsForFileToAppear(t *testing.T) {
	path := filepath.Join(t.TempDir(), "late.mp3")
	done := make(chan error, 1)
	go func() {
		time.Sleep(80 * time.Millisecond)
		if err := os.WriteFile(path, []byte("late-bytes"), 0o644); err != nil {
			done <- err
			return
		}
		done <- nil
	}()

	fr := &followerReader{path: path, done: done, poll: 10 * time.Millisecond}
	defer fr.Close()
	got, err := io.ReadAll(fr)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != "late-bytes" {
		t.Fatalf("read %q, want %q", got, "late-bytes")
	}
}

// A failed remux has to reach the reader as an error. Reporting EOF instead
// would hand the listener a silently truncated track.
func TestFollowerReaderSurfacesRemuxFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "broken.mp3")
	if err := os.WriteFile(path, []byte("partial"), 0o644); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	done <- fmt.Errorf("ffmpeg remux failed")

	fr := &followerReader{path: path, done: done, poll: 10 * time.Millisecond}
	defer fr.Close()
	if _, err := io.ReadAll(fr); err == nil {
		t.Fatal("expected the remux failure to surface, got a clean end of file")
	}
}

// A complete cache file is served straight from disk: no ffmpeg, no waiting.
func TestMaterializeProgressiveCacheHit(t *testing.T) {
	// An unusable binary proves nothing was executed.
	cache := hlsTestCache(t, filepath.Join(t.TempDir(), "ffmpeg-does-not-exist"))
	target := mp3Target("https://cdn.example/playlist.m3u8")
	final := cache.pathFor("soundcloud_stream:cached", containerFor(target))
	if err := os.MkdirAll(filepath.Dir(final), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(final, []byte("already-remuxed"), 0o644); err != nil {
		t.Fatal(err)
	}

	path, container, done := cache.materializeProgressive("soundcloud_stream:cached", target)
	if path != final {
		t.Fatalf("path = %q, want the finished cache file %q", path, final)
	}
	if container.ext != "mp3" {
		t.Fatalf("container = %q, want mp3", container.ext)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("cache hit reported %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cache hit did not report completion immediately")
	}
}

// Two listeners on the same cold track share one ffmpeg and one .partial.
func TestMaterializeProgressiveConcurrent(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "runs")
	cache := hlsTestCache(t, stubFFmpeg(t, 5, 100*time.Millisecond, marker))
	target := mp3Target("https://cdn.example/playlist.m3u8")

	firstPath, _, firstDone := cache.materializeProgressive("soundcloud_stream:shared", target)
	time.Sleep(150 * time.Millisecond) // the second listener arrives mid-remux
	secondPath, _, secondDone := cache.materializeProgressive("soundcloud_stream:shared", target)

	if firstPath != secondPath {
		t.Fatalf("readers attached to different files: %q and %q", firstPath, secondPath)
	}
	if !strings.HasSuffix(firstPath, ".partial") {
		t.Fatalf("expected the growing file, got %q", firstPath)
	}
	for i, done := range []<-chan error{firstDone, secondDone} {
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("reader %d reported %v", i, err)
			}
		case <-time.After(10 * time.Second):
			t.Fatalf("reader %d never heard the remux finish", i)
		}
	}
	if n := runCount(t, marker); n != 1 {
		t.Fatalf("started %d ffmpeg processes for one track, want 1", n)
	}

	// And the finished file is where the next request will look for it.
	final := cache.pathFor("soundcloud_stream:shared", containerFor(target))
	data, err := os.ReadFile(final)
	if err != nil {
		t.Fatalf("finished file: %v", err)
	}
	if string(data) != stubOutput(5) {
		t.Fatalf("finished file holds %q, want %q", data, stubOutput(5))
	}
}

// The listener closing the tab must not kill the remux: the bytes downloaded so
// far are worth keeping, and other listeners may be reading the same file.
func TestHLSDetachFromRequestContext(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "runs")
	cfg := Config{MediaRoot: t.TempDir(), FFmpegBinary: stubFFmpeg(t, 6, 100*time.Millisecond, marker), HLSRemuxTimeout: 10 * time.Second}
	app := &App{cfg: cfg, hls: newHLSCache(cfg)}
	target := mp3Target("https://cdn.example/playlist.m3u8")
	trackID := "soundcloud_stream:detach"

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest("GET", "/v1/stream/"+trackID, nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	go func() {
		time.Sleep(200 * time.Millisecond)
		cancel()
	}()
	app.serveHLS(rec, req, trackID, target)
	cancel()

	final := app.hls.pathFor(trackID, containerFor(target))
	deadline := time.Now().Add(10 * time.Second)
	for {
		if data, err := os.ReadFile(final); err == nil {
			if string(data) != stubOutput(6) {
				t.Fatalf("remux left %q, want the complete %q", data, stubOutput(6))
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the abandoned remux never finished — it was killed with the request")
		}
		time.Sleep(50 * time.Millisecond)
	}
	if n := runCount(t, marker); n != 1 {
		t.Fatalf("started %d ffmpeg processes, want 1", n)
	}
}

// Packaging has to be readable while it is being written. MP4 with +faststart
// is not: the moov atom is moved to the front after the last sample is muxed.
func TestContainerForPicksProgressiveFormats(t *testing.T) {
	for _, tc := range []struct {
		name        string
		target      StreamTarget
		ext         string
		contentType string
	}{
		{"mp3 copy", StreamTarget{ACodec: "mp3", Ext: "mp3"}, "mp3", "audio/mpeg"},
		{"aac to adts", StreamTarget{ACodec: "mp4a.40.2", Ext: "m4a"}, "aac", "audio/aac"},
		{"opus transcoded", StreamTarget{ACodec: "opus", Ext: "webm"}, "mp3", "audio/mpeg"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			container := containerFor(tc.target)
			if container.ext != tc.ext || container.contentType != tc.contentType {
				t.Fatalf("got %s/%s, want %s/%s", container.ext, container.contentType, tc.ext, tc.contentType)
			}
			if strings.Contains(strings.Join(container.args, " "), "faststart") {
				t.Fatal("faststart cannot be streamed while ffmpeg is still writing")
			}
		})
	}
}

// Among HLS formats the AAC-first preference flips, because an MP3 playlist
// remuxes into something that streams as it is written.
func TestScoreFormatPrefersMP3PlaylistOverAAC(t *testing.T) {
	mp3 := ytdlpFormat{URL: "https://cdn.example/mp3.m3u8", Protocol: "m3u8_native", ACodec: "mp3", Ext: "mp3", ABR: 128}
	aac := ytdlpFormat{URL: "https://cdn.example/aac.m3u8", Protocol: "m3u8_native", ACodec: "mp4a.40.2", Ext: "m4a", ABR: 128}
	if scoreFormat(mp3) <= scoreFormat(aac) {
		t.Fatalf("mp3 playlist scored %v, aac %v — aac would win", scoreFormat(mp3), scoreFormat(aac))
	}
	// Outside HLS the iOS-compatible ordering must be untouched.
	plainMP3 := ytdlpFormat{URL: "https://cdn.example/a.mp3", ACodec: "mp3", Ext: "mp3", ABR: 128, VCodec: "none"}
	plainM4A := ytdlpFormat{URL: "https://cdn.example/a.m4a", ACodec: "mp4a.40.2", Ext: "m4a", ABR: 128, VCodec: "none"}
	if scoreFormat(plainM4A) <= scoreFormat(plainMP3) {
		t.Fatal("progressive m4a must still outrank progressive mp3")
	}
}
