package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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
// The playlist is remuxed into a normal file and served from disk after that.
// Remuxing rather than transcoding keeps this cheap enough for a Pi — the
// segments already carry AAC or MP3, so ffmpeg only rewrites the container —
// and a real file on disk restores the two things the proxy path gave us for
// free: byte ranges (so the scrubber works) and an exact duration in the
// header (so the player does not have to guess one from the bitrate).
//
// The remux itself is not waited for. The writer — the native segment
// downloader in hls_native.go where the audio can be copied, ffmpeg where it
// has to be transcoded — appends to a .partial file and the handler reads from
// that same file, blocking at EOF until more arrives, so the listener hears the
// first HLS segment about a second in instead of waiting the 10-30s a whole
// playlist takes to download.
type hlsCache struct {
	dir     string
	ffmpeg  string
	timeout time.Duration

	mu   sync.Mutex
	jobs map[string]*hlsJob // final cache path -> remux in flight
}

// hlsJob is one running ffmpeg. It is shared: every listener that arrives while
// the remux is in flight reads the same .partial rather than starting a second
// process over the same output path.
type hlsJob struct {
	tmp       string
	container hlsContainer
	done      chan struct{} // closed when the remux has finished or failed
	err       error         // valid once done is closed
}

// result hands out a fresh single-use channel per caller, because a job has
// many readers and a value sent on a shared channel would only reach one.
func (j *hlsJob) result() <-chan error {
	ch := make(chan error, 1)
	go func() {
		<-j.done
		ch <- j.err
		close(ch)
	}()
	return ch
}

// settled is the already-finished form of the same channel, for a cache hit or
// a failure that never got as far as starting ffmpeg.
func settled(err error) <-chan error {
	ch := make(chan error, 1)
	ch <- err
	close(ch)
	return ch
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
		jobs:    map[string]*hlsJob{},
	}
}

// hlsContainer is the output packaging for one source codec.
type hlsContainer struct {
	ext         string
	contentType string
	// args are the ffmpeg output flags, including the codec choice.
	args []string
}

// containerFor keeps the audio bytes untouched wherever possible, and picks
// packaging that can be read while it is still being written.
//
// MP3 is the ideal case and the one SoundCloud usually offers (scoreFormat
// prefers an MP3 playlist over an AAC one for exactly this reason): a bare MP3
// is a stream of self-describing frames, so a reader can start on the first one
// ffmpeg emits. AAC goes into ADTS for the same reason — the old MP4 output
// needed +faststart, which rewrites the file at the end to move the moov atom
// in front of the audio, and nothing can be played out of that file until the
// rewrite lands. Anything else (Opus in a playlist, say) is transcoded, because
// the point of this path is to hand the client something it can decode.
func containerFor(target StreamTarget) hlsContainer {
	codec := strings.ToLower(target.ACodec)
	ext := strings.ToLower(target.Ext)
	switch {
	case codec == "mp3", ext == "mp3":
		return hlsContainer{ext: "mp3", contentType: "audio/mpeg", args: []string{"-c:a", "copy", "-f", "mp3"}}
	case strings.HasPrefix(codec, "mp4a"), codec == "aac", ext == "m4a", ext == "mp4":
		return hlsContainer{ext: "aac", contentType: "audio/aac", args: []string{"-c:a", "copy", "-f", "adts"}}
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

// materializeProgressive returns a path that can be read immediately, the
// packaging it will hold, and a channel that reports how the remux ended.
//
// The path is the finished cache file on a hit and the growing .partial
// otherwise; a reader tells the two apart by comparing against pathFor, and
// followerReader copes with the .partial being renamed out from under it while
// it reads.
func (c *hlsCache) materializeProgressive(trackID string, target StreamTarget) (string, hlsContainer, <-chan error) {
	container := containerFor(target)
	path := c.pathFor(trackID, container)

	if info, err := os.Stat(path); err == nil && info.Size() > 0 {
		slog.Debug("hls cache hit", "track", trackID, "bytes", info.Size(), "container", container.ext)
		return path, container, settled(nil)
	}

	c.mu.Lock()
	if job, ok := c.jobs[path]; ok {
		c.mu.Unlock()
		return job.tmp, job.container, job.result()
	}
	if err := os.MkdirAll(c.dir, 0755); err != nil {
		c.mu.Unlock()
		return "", container, settled(err)
	}
	// A .partial with no job behind it was left by a process that died mid
	// remux, so nothing will ever finish it. Only the jobs map means "someone
	// is writing this"; the file on its own does not.
	tmp := path + ".partial"
	_ = os.Remove(tmp)
	job := &hlsJob{tmp: tmp, container: container, done: make(chan struct{})}
	c.jobs[path] = job
	c.mu.Unlock()

	go c.remux(job, trackID, path, target)
	return tmp, container, job.result()
}

// remux fills the job's .partial, by whichever route can do it.
//
// A stream copy does not need ffmpeg at all — the segments already hold the
// output bytes — and the native downloader fetches six of them at once instead
// of one after another, which is most of the wait on a cold SoundCloud track.
// It declines anything it cannot do byte-for-byte, and everything that has to
// be transcoded never reaches it, so ffmpeg stays the general case.
func (c *hlsCache) remux(job *hlsJob, trackID, path string, target StreamTarget) {
	if canUseNativeHLS(target) && c.remuxNative(job, trackID, path, target) {
		return
	}
	c.remuxFFmpeg(job, trackID, path, target)
}

// remuxFFmpeg runs ffmpeg to completion on a context of its own. Detaching from
// the request context is deliberate: a listener who closes the tab five seconds
// in used to kill the ffmpeg every other listener was waiting on and delete the
// partial file, so the next play started from nothing again.
func (c *hlsCache) remuxFFmpeg(job *hlsJob, trackID, path string, target StreamTarget) {
	runCtx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	args := []string{"-hide_banner", "-loglevel", "error", "-y"}
	if headers := ffmpegHeaders(target.Headers); headers != "" {
		args = append(args, "-headers", headers)
	}
	args = append(args, "-i", target.URL, "-vn")
	args = append(args, job.container.args...)
	args = append(args, job.tmp)

	slog.Info("hls remux started", "track", trackID, "container", job.container.ext, "codec", target.ACodec, "timeout", c.timeout)
	started := time.Now()
	cmd := exec.CommandContext(runCtx, c.ffmpeg, args...)
	var stderr strings.Builder
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err == nil {
		var info os.FileInfo
		if info, err = os.Stat(job.tmp); err == nil {
			if info.Size() == 0 {
				err = fmt.Errorf("ffmpeg produced an empty file")
			} else if err = os.Rename(job.tmp, path); err == nil {
				slog.Info("hls remuxed", "track", trackID, "bytes", info.Size(), "container", job.container.ext, "elapsed", time.Since(started))
			}
		}
	} else if runCtx.Err() != nil {
		slog.Warn("hls remux timed out", "track", trackID, "elapsed", time.Since(started), "timeout", c.timeout)
		err = fmt.Errorf("ffmpeg remux timed out after %s", c.timeout)
	} else {
		err = fmt.Errorf("ffmpeg remux failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}

	c.finish(job, trackID, path, err)
}

// finish delivers a job's result to everyone waiting on it and takes it out of
// the jobs map. Both the native and the ffmpeg path end here, and every path
// that ends a job must go through it: a job whose done channel is never closed
// leaves its readers waiting forever.
func (c *hlsCache) finish(job *hlsJob, trackID, path string, err error) {
	if err != nil {
		// A truncated file must never be mistaken for a finished one by the
		// next request, and it is no use to the readers still attached either.
		_ = os.Remove(job.tmp)
		slog.Warn("hls remux failed", "track", trackID, "error", err)
	}

	c.mu.Lock()
	delete(c.jobs, path)
	c.mu.Unlock()

	job.err = err
	close(job.done)
}

// materialize waits for a complete local copy. Playback warming uses it; the
// stream handler does not, because waiting is the thing it exists to avoid.
func (c *hlsCache) materialize(ctx context.Context, trackID string, target StreamTarget) (string, hlsContainer, error) {
	_, container, done := c.materializeProgressive(trackID, target)
	select {
	case err := <-done:
		if err != nil {
			return "", container, err
		}
		return c.pathFor(trackID, container), container, nil
	case <-ctx.Done():
		return "", container, ctx.Err()
	}
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

// followerPollInterval is how long a reader that has caught up with ffmpeg
// sleeps before looking for more bytes. Short enough to be inaudible between
// segments, long enough not to spin a Pi core for the length of a track.
const followerPollInterval = 100 * time.Millisecond

// followerReader reads a file that is still being written, in the manner of
// tail -f: at EOF it waits for either more bytes or the remux to end, instead
// of reporting the end of the audio.
//
// It opens lazily and falls back to the finished cache path, because ffmpeg
// renames the .partial into place the moment it is done and a reader that
// arrived a moment too late would otherwise find nothing to open. A reader that
// opened before the rename keeps its descriptor, which follows the file.
type followerReader struct {
	ctx      context.Context
	path     string        // the .partial being written
	fallback string        // the finished cache path it is renamed to
	done     <-chan error  // remux result, delivered once
	poll     time.Duration // 0 means followerPollInterval

	f        *os.File
	pos      int64
	finished bool
}

func (fr *followerReader) open() error {
	f, err := os.Open(fr.path)
	if err != nil && os.IsNotExist(err) && fr.fallback != "" {
		f, err = os.Open(fr.fallback)
	}
	if err != nil {
		return err
	}
	fr.f = f
	return nil
}

func (fr *followerReader) Close() error {
	if fr.f == nil {
		return nil
	}
	return fr.f.Close()
}

func (fr *followerReader) Read(p []byte) (int, error) {
	for {
		if fr.f == nil {
			if err := fr.open(); err != nil && !os.IsNotExist(err) {
				return 0, err
			}
		}
		if fr.f != nil {
			// ReadAt rather than Read: a plain Read that returned io.EOF once
			// would keep returning it, since the file offset is already at the
			// end that ffmpeg is about to move.
			n, err := fr.f.ReadAt(p, fr.pos)
			if n > 0 {
				fr.pos += int64(n)
				return n, nil
			}
			if err != nil && err != io.EOF {
				return 0, err
			}
		}
		// Caught up with the writer, or the file does not exist yet.
		if fr.finished {
			if fr.f == nil {
				return 0, fmt.Errorf("remux produced no output")
			}
			return 0, io.EOF
		}
		if err := fr.wait(); err != nil {
			return 0, err
		}
	}
}

// wait blocks until there is a reason to look at the file again. A remux that
// ends does not end the read: ffmpeg's last bytes may still be unread, so the
// caller loops once more and only then reports EOF.
func (fr *followerReader) wait() error {
	ctx := fr.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	poll := fr.poll
	if poll <= 0 {
		poll = followerPollInterval
	}
	timer := time.NewTimer(poll)
	defer timer.Stop()
	select {
	case err := <-fr.done:
		fr.finished = true
		return err
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// serveHLS streams the remuxed audio, starting before the remux has finished.
//
// A finished file goes through http.ServeContent, which brings ranges,
// conditional requests and an exact Content-Length. A file still being written
// is streamed progressively instead: length is unknown until ffmpeg stops, so
// the response is chunked and carries the duration in X-Content-Duration for
// players that want a scrubber before the file is complete.
func (a *App) serveHLS(w http.ResponseWriter, r *http.Request, trackID string, target StreamTarget) {
	providerID, _, _ := splitTrackID(trackID)
	message := unavailableMessage(providerID)

	path, container, done := a.hls.materializeProgressive(trackID, target)
	final := a.hls.pathFor(trackID, container)
	if path == "" {
		if err := <-done; err != nil {
			slog.Warn("hls cache unusable", "track", trackID, "error", err)
		}
		writeError(w, http.StatusBadGateway, message)
		return
	}
	if path == final {
		a.serveHLSFile(w, r, final, container, message)
		return
	}

	// A seek cannot be answered progressively: bytes=N- has to be measured
	// against a total length that only exists once ffmpeg has stopped. Playback
	// from the start is what needs to be instant, so a mid-track request waits
	// for the remux and is then served the complete file with real ranges.
	if start, _, ok := parseRangeHint(r.Header.Get("Range")); ok && start > 0 {
		select {
		case err := <-done:
			if err != nil {
				writeError(w, http.StatusBadGateway, message)
				return
			}
		case <-r.Context().Done():
			return
		}
		a.serveHLSFile(w, r, final, container, message)
		return
	}

	fr := &followerReader{ctx: r.Context(), path: path, fallback: final, done: done}
	defer fr.Close()

	// The first read is what decides between an error page and audio, so the
	// headers cannot go out before it returns.
	buf := make([]byte, 32*1024)
	n, err := fr.Read(buf)
	if err != nil && n == 0 {
		if r.Context().Err() != nil {
			return
		}
		slog.Warn("hls progressive read failed", "track", trackID, "error", err)
		writeError(w, http.StatusBadGateway, message)
		return
	}

	w.Header().Set("Content-Type", container.contentType)
	w.Header().Set("Accept-Ranges", "bytes")
	if target.Duration > 0 {
		// No Content-Length is guessed from the duration: the real bitrate of a
		// stream copy is whatever SoundCloud encoded, and a length that is off
		// by a few percent either truncates the audio or hangs the player at
		// the end. X-Content-Duration gives the scrubber its range without
		// claiming a byte count we do not have.
		w.Header().Set("X-Content-Duration", strconv.FormatFloat(target.Duration, 'f', 3, 64))
	}
	flusher, _ := w.(http.Flusher)

	for {
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
		if err != nil {
			if err != io.EOF {
				slog.Warn("hls progressive stream ended early", "track", trackID, "error", err)
			}
			return
		}
		if r.Context().Err() != nil {
			return
		}
		n, err = fr.Read(buf)
	}
}

// serveHLSFile is the finished-file path, unchanged from before progressive
// streaming existed: ServeContent supplies ranges, conditional requests and
// Content-Length without further work here.
func (a *App) serveHLSFile(w http.ResponseWriter, r *http.Request, path string, container hlsContainer, message string) {
	file, err := os.Open(path)
	if err != nil {
		slog.Warn("hls cache read failed", "path", path, "error", err)
		writeError(w, http.StatusBadGateway, message)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || info.Size() == 0 {
		writeError(w, http.StatusBadGateway, message)
		return
	}
	w.Header().Set("Content-Type", container.contentType)
	w.Header().Set("Accept-Ranges", "bytes")
	http.ServeContent(w, r, filepath.Base(path), info.ModTime(), file)
}
