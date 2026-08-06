package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// SoundCloud now publishes audio only as HLS: every format yt-dlp reports is an
// m3u8 playlist. A playlist cannot go down the byte-range proxy (there is no
// length to range over) and no browser except Safari will decode one from an
// <audio> src, which is why SoundCloud playback returned 502 while search kept
// working.
//
// The playlist is remuxed once into a normal file and served from disk after
// that. Remuxing rather than transcoding keeps this cheap enough for a Pi — the
// segments already carry AAC or MP3, so ffmpeg only rewrites the container —
// and a real file on disk restores the two things the proxy path gave us for
// free: byte ranges (so the scrubber works) and an exact duration in the
// header (so the player does not have to guess one from the bitrate).
type hlsCache struct {
	dir      string
	ffmpeg   string
	timeout  time.Duration
	inflight sync.Map // cache path -> *sync.Mutex
}

func newHLSCache(cfg Config) *hlsCache {
	timeout := cfg.HLSRemuxTimeout
	if timeout <= 0 {
		timeout = cfg.DownloadTimeout
	}
	return &hlsCache{
		dir:     filepath.Join(cfg.MediaRoot, "stream-cache"),
		ffmpeg:  cfg.FFmpegBinary,
		timeout: timeout,
	}
}

// hlsContainer is the output packaging for one source codec.
type hlsContainer struct {
	ext         string
	contentType string
	// args are the ffmpeg output flags, including the codec choice.
	args []string
}

// containerFor keeps the audio bytes untouched wherever possible. AAC goes into
// MP4 with faststart so the moov atom sits before the audio and a player knows
// the duration from the first read; MP3 needs no container at all. Anything else
// (Opus in an HLS playlist, say) is transcoded, because the point of this path
// is to hand the client something it can definitely decode.
func containerFor(target StreamTarget) hlsContainer {
	codec := strings.ToLower(target.ACodec)
	ext := strings.ToLower(target.Ext)
	switch {
	case strings.HasPrefix(codec, "mp4a"), codec == "aac", ext == "m4a", ext == "mp4":
		return hlsContainer{ext: "m4a", contentType: "audio/mp4", args: []string{"-c:a", "copy", "-movflags", "+faststart", "-f", "mp4"}}
	case codec == "mp3", ext == "mp3":
		return hlsContainer{ext: "mp3", contentType: "audio/mpeg", args: []string{"-c:a", "copy", "-f", "mp3"}}
	default:
		return hlsContainer{ext: "mp3", contentType: "audio/mpeg", args: []string{"-c:a", "libmp3lame", "-b:a", "192k", "-f", "mp3"}}
	}
}

// pathFor keys on the track id rather than the CDN URL: SoundCloud's signed
// playlist URLs expire and change on every resolve, so keying on them would
// rebuild the same track on every play.
func (c *hlsCache) pathFor(trackID string, container hlsContainer) string {
	sum := sha256.Sum256([]byte(trackID))
	return filepath.Join(c.dir, hex.EncodeToString(sum[:])+"."+container.ext)
}

func (c *hlsCache) lockFor(path string) *sync.Mutex {
	value, _ := c.inflight.LoadOrStore(path, &sync.Mutex{})
	return value.(*sync.Mutex)
}

// materialize returns the path of a complete local copy, building it if needed.
func (c *hlsCache) materialize(ctx context.Context, trackID string, target StreamTarget) (string, hlsContainer, error) {
	container := containerFor(target)
	path := c.pathFor(trackID, container)

	// Two listeners hitting the same cold track must not run two ffmpeg
	// processes over the same output path. The second one waits and then finds
	// the file already there.
	lock := c.lockFor(path)
	lock.Lock()
	defer lock.Unlock()

	if info, err := os.Stat(path); err == nil && info.Size() > 0 {
		slog.Debug("hls cache hit", "track", trackID, "bytes", info.Size(), "container", container.ext)
		return path, container, nil
	}
	if err := os.MkdirAll(c.dir, 0755); err != nil {
		return "", container, err
	}

	// ffmpeg is told to write a temp file that is renamed into place only on
	// success: a cancelled or crashed remux must never leave a truncated file
	// that the next request mistakes for a finished one.
	tmp := path + ".partial"
	defer os.Remove(tmp)

	runCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	args := []string{"-hide_banner", "-loglevel", "error", "-y"}
	if headers := ffmpegHeaders(target.Headers); headers != "" {
		args = append(args, "-headers", headers)
	}
	args = append(args, "-i", target.URL, "-vn")
	args = append(args, container.args...)
	args = append(args, tmp)

	slog.Info("hls remux started", "track", trackID, "container", container.ext, "codec", target.ACodec, "timeout", c.timeout)
	started := time.Now()
	cmd := exec.CommandContext(runCtx, c.ffmpeg, args...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		// A cancelled runCtx with a live parent context means ffmpeg hit the
		// remux deadline rather than the listener closing the tab, and the
		// distinction is the difference between a bug report and a skip.
		if runCtx.Err() != nil && ctx.Err() == nil {
			slog.Warn("hls remux timed out", "track", trackID, "elapsed", time.Since(started), "timeout", c.timeout)
			return "", container, fmt.Errorf("ffmpeg remux timed out after %s", c.timeout)
		}
		return "", container, fmt.Errorf("ffmpeg remux failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	info, err := os.Stat(tmp)
	if err != nil {
		return "", container, err
	}
	if info.Size() == 0 {
		return "", container, fmt.Errorf("ffmpeg produced an empty file")
	}
	if err := os.Rename(tmp, path); err != nil {
		return "", container, err
	}
	slog.Info("hls remuxed", "track", trackID, "bytes", info.Size(), "container", container.ext, "elapsed", time.Since(started))
	return path, container, nil
}

// ffmpegHeaders renders the headers yt-dlp prescribed in the CRLF-delimited
// form ffmpeg's -headers flag expects. Range and Host are dropped for the same
// reason the byte-range proxy drops them: ffmpeg sets its own.
func ffmpegHeaders(headers map[string]string) string {
	var b strings.Builder
	for name, value := range headers {
		if strings.EqualFold(name, "Range") || strings.EqualFold(name, "Host") {
			continue
		}
		if strings.ContainsAny(name, "\r\n") || strings.ContainsAny(value, "\r\n") {
			continue
		}
		fmt.Fprintf(&b, "%s: %s\r\n", name, value)
	}
	return b.String()
}

// serveHLS hands the remuxed file to http.ServeContent, which supplies range
// support, conditional requests and Content-Length without further work here.
func (a *App) serveHLS(w http.ResponseWriter, r *http.Request, trackID string, target StreamTarget) {
	providerID, _, _ := splitTrackID(trackID)
	message := unavailableMessage(providerID)
	path, container, err := a.hls.materialize(r.Context(), trackID, target)
	if err != nil {
		if r.Context().Err() != nil {
			return
		}
		slog.Warn("hls remux failed", "track", trackID, "error", err)
		writeError(w, http.StatusBadGateway, message)
		return
	}
	file, err := os.Open(path)
	if err != nil {
		slog.Warn("hls cache read failed", "track", trackID, "error", err)
		writeError(w, http.StatusBadGateway, message)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		writeError(w, http.StatusBadGateway, message)
		return
	}
	w.Header().Set("Content-Type", container.contentType)
	w.Header().Set("Accept-Ranges", "bytes")
	http.ServeContent(w, r, filepath.Base(path), info.ModTime(), file)
}
