package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ffmpeg is the wrong tool for the HLS playlists SoundCloud serves. It fetches
// segments one at a time — a 3.5 MB MP3 arrives as 30-odd sequential GETs, each
// paying a full round trip to the CDN before the next one starts — and for a
// stream copy that is all it does: the segments already hold exactly the bytes
// the output file needs.
//
// So for anything that would have been copied rather than transcoded, the
// playlist is parsed here and its segments are downloaded six at a time and
// concatenated in playlist order. The result is byte-for-byte what "ffmpeg -c:a
// copy -f mp3" (or "-f adts") would have written, minus the serialisation.
//
// Transcoding still belongs to ffmpeg, and so does every playlist shape this
// parser does not fully understand: remuxNative reports whether it took the
// job, and remux falls back whenever it did not.

const (
	// hlsSegmentWorkers is how many segments are fetched at once. Six keeps a
	// Pi's uplink busy without opening so many connections that the CDN starts
	// throttling them individually.
	hlsSegmentWorkers = 6
	// hlsSegmentLookahead is how many finished-but-unwritten segments may sit in
	// memory beyond the ones being fetched. It bounds memory to roughly
	// (workers+lookahead) segments — a few megabytes — while still letting a
	// fast worker start the next segment before the writer catches up.
	hlsSegmentLookahead = 4
	// hlsSegmentAttempts includes the first try. A single retry covers the
	// occasional CDN hiccup; anything more and the listener is better served by
	// a clean failure than by a stall.
	hlsSegmentAttempts  = 2
	hlsSegmentRetryWait = 150 * time.Millisecond
	// hlsPlaylistRedirects bounds master -> media -> ... chasing.
	hlsPlaylistRedirects = 3
	hlsPlaylistMaxBytes  = 4 << 20
	hlsSegmentMaxBytes   = 64 << 20
	hlsMaxSegments       = 20000
)

// canUseNativeHLS reports whether the segments can go to disk untouched. It
// mirrors the copy branches of containerFor: those are exactly the cases where
// ffmpeg would have rewritten the container and nothing else, and MP3 frames
// and ADTS frames are both self-describing, so a plain concatenation of
// segments is a valid file.
func canUseNativeHLS(target StreamTarget) bool {
	if !target.HLS {
		return false
	}
	codec := strings.ToLower(target.ACodec)
	ext := strings.ToLower(target.Ext)
	switch {
	case codec == "mp3", ext == "mp3":
		return true
	case strings.HasPrefix(codec, "mp4a"), codec == "aac", ext == "m4a", ext == "mp4":
		return true
	default:
		return false
	}
}

// parseHLSPlaylist fetches an m3u8 URL and returns its segment URLs in playlist
// order, absolute and ready to GET.
//
// A master playlist (one that lists variants rather than segments) is followed:
// the audio-only rendition wins, and among equals the highest bitrate. Shapes
// that cannot be concatenated — encryption, fragmented MP4 with an init
// segment, byte-range segments — are reported as errors rather than guessed at,
// because the caller has a working ffmpeg to fall back to.
func parseHLSPlaylist(ctx context.Context, rawURL string, headers map[string]string) ([]string, error) {
	current := rawURL
	for depth := 0; depth < hlsPlaylistRedirects; depth++ {
		body, base, err := fetchHLSPlaylist(ctx, current, headers)
		if err != nil {
			return nil, err
		}
		segments, variants, err := parseHLSLines(string(body), base)
		if err != nil {
			return nil, err
		}
		if len(segments) > 0 {
			if len(segments) > hlsMaxSegments {
				return nil, fmt.Errorf("playlist has %d segments, more than the %d supported", len(segments), hlsMaxSegments)
			}
			return segments, nil
		}
		if len(variants) == 0 {
			return nil, fmt.Errorf("playlist has neither segments nor variants")
		}
		current = pickHLSVariant(variants).url
	}
	return nil, fmt.Errorf("playlist nested more than %d levels deep", hlsPlaylistRedirects)
}

// hlsVariant is one entry of a master playlist.
type hlsVariant struct {
	url       string
	bandwidth int64
	audioOnly bool
}

// parseHLSLines splits one playlist body into segments and variants; a playlist
// is one or the other, never both.
func parseHLSLines(body string, base *url.URL) ([]string, []hlsVariant, error) {
	var (
		segments []string
		variants []hlsVariant
		pending  *hlsVariant // set by #EXT-X-STREAM-INF, claimed by the next URI line
	)
	for _, raw := range strings.Split(body, "\n") {
		line := strings.TrimSpace(strings.TrimSuffix(raw, "\r"))
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			tag, value, _ := strings.Cut(line, ":")
			switch tag {
			case "#EXT-X-KEY", "#EXT-X-SESSION-KEY":
				if method := hlsAttrs(value)["METHOD"]; method != "" && !strings.EqualFold(method, "NONE") {
					return nil, nil, fmt.Errorf("playlist is encrypted (%s)", method)
				}
			case "#EXT-X-MAP":
				// An init segment means fragmented MP4: the segments are moof/mdat
				// boxes that mean nothing without it, so they cannot simply be
				// concatenated into a bare stream.
				return nil, nil, fmt.Errorf("playlist uses an EXT-X-MAP init segment")
			case "#EXT-X-BYTERANGE":
				return nil, nil, fmt.Errorf("playlist uses byte-range segments")
			case "#EXT-X-STREAM-INF":
				attrs := hlsAttrs(value)
				bandwidth, _ := strconv.ParseInt(attrs["BANDWIDTH"], 10, 64)
				pending = &hlsVariant{
					bandwidth: bandwidth,
					audioOnly: attrs["RESOLUTION"] == "" && !hlsCodecsHaveVideo(attrs["CODECS"]),
				}
			case "#EXT-X-MEDIA":
				attrs := hlsAttrs(value)
				if strings.EqualFold(attrs["TYPE"], "AUDIO") && attrs["URI"] != "" {
					resolved, err := resolveHLSRef(base, attrs["URI"])
					if err != nil {
						return nil, nil, err
					}
					variants = append(variants, hlsVariant{url: resolved, audioOnly: true})
				}
			}
			continue
		}
		resolved, err := resolveHLSRef(base, line)
		if err != nil {
			return nil, nil, err
		}
		if pending != nil {
			pending.url = resolved
			variants = append(variants, *pending)
			pending = nil
			continue
		}
		segments = append(segments, resolved)
	}
	return segments, variants, nil
}

// pickHLSVariant prefers an audio-only rendition — a video variant would drag
// down a whole muxed stream for the sake of its audio track — and takes the
// highest bitrate among the candidates left.
func pickHLSVariant(variants []hlsVariant) hlsVariant {
	best := variants[0]
	for _, v := range variants[1:] {
		switch {
		case v.audioOnly != best.audioOnly:
			if v.audioOnly {
				best = v
			}
		case v.bandwidth > best.bandwidth:
			best = v
		}
	}
	return best
}

// hlsCodecsHaveVideo reports whether a CODECS attribute names a video codec.
// Audio-only variants are usually marked by the absence of RESOLUTION, but not
// every packager writes one.
func hlsCodecsHaveVideo(codecs string) bool {
	for _, codec := range strings.Split(codecs, ",") {
		codec = strings.ToLower(strings.TrimSpace(codec))
		for _, video := range []string{"avc1", "avc3", "hvc1", "hev1", "vp8", "vp9", "vp09", "av01", "dvh1"} {
			if strings.HasPrefix(codec, video) {
				return true
			}
		}
	}
	return false
}

// hlsAttrs parses the comma-separated NAME=VALUE list an m3u8 tag carries.
// Values may be quoted and a quoted value may itself contain commas, which is
// why this is not a plain Split on ",".
func hlsAttrs(value string) map[string]string {
	attrs := map[string]string{}
	var (
		field  strings.Builder
		quoted bool
	)
	flush := func() {
		name, val, found := strings.Cut(field.String(), "=")
		field.Reset()
		if !found {
			return
		}
		attrs[strings.ToUpper(strings.TrimSpace(name))] = strings.Trim(strings.TrimSpace(val), `"`)
	}
	for _, r := range value {
		switch {
		case r == '"':
			quoted = !quoted
			field.WriteRune(r)
		case r == ',' && !quoted:
			flush()
		default:
			field.WriteRune(r)
		}
	}
	flush()
	return attrs
}

// resolveHLSRef turns a playlist entry into an absolute URL. Segment lines are
// usually relative, and relative to the playlist's own final URL — after
// redirects, which is why the base comes from the response rather than from the
// URL that was requested.
func resolveHLSRef(base *url.URL, ref string) (string, error) {
	parsed, err := url.Parse(ref)
	if err != nil {
		return "", fmt.Errorf("unusable playlist entry %q: %w", ref, err)
	}
	if base == nil {
		if !parsed.IsAbs() {
			return "", fmt.Errorf("relative playlist entry %q with no base URL", ref)
		}
		return parsed.String(), nil
	}
	return base.ResolveReference(parsed).String(), nil
}

func fetchHLSPlaylist(ctx context.Context, rawURL string, headers map[string]string) ([]byte, *url.URL, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, nil, err
	}
	applyUpstreamHeaders(req, headers)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer discardResponse(resp)
	if resp.StatusCode != http.StatusOK {
		return nil, nil, upstreamStatusError{code: resp.StatusCode, status: resp.Status}
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, hlsPlaylistMaxBytes))
	if err != nil {
		return nil, nil, err
	}
	if !strings.Contains(string(body), "#EXTM3U") {
		return nil, nil, fmt.Errorf("%s is not an m3u8 playlist", rawURL)
	}
	base := resp.Request.URL
	if base == nil {
		base, _ = url.Parse(rawURL)
	}
	return body, base, nil
}

// segmentResult is one downloaded segment on its way to the writer.
type segmentResult struct {
	data []byte
	err  error
}

// downloadHLSSegments downloads segments in parallel and writes them to w in
// playlist order, returning the number of bytes written.
//
// The ordering is what makes this usable as a progressive source: a fetcher
// pool works on whatever segment is next, but each one hands its bytes to a
// single writer through its own slot, and the writer walks the slots in index
// order. A segment that arrives early waits; one that arrives late holds up
// only the writer, not the other fetchers.
//
// Fetchers take a token from a fixed pool before downloading and the writer
// returns one after each segment is written, which is what stops six eager
// workers from pulling an entire track into memory ahead of the disk.
//
// One failed segment fails the whole download: a gap in the middle of a track
// is worse than an error the caller can fall back from.
func downloadHLSSegments(ctx context.Context, w io.Writer, segments []string, headers map[string]string) (int64, error) {
	if len(segments) == 0 {
		return 0, fmt.Errorf("playlist has no segments")
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	workers := hlsSegmentWorkers
	if workers > len(segments) {
		workers = len(segments)
	}
	results := make([]chan segmentResult, len(segments))
	for i := range results {
		results[i] = make(chan segmentResult, 1)
	}
	slots := make(chan struct{}, workers+hlsSegmentLookahead)
	for i := 0; i < cap(slots); i++ {
		slots <- struct{}{}
	}

	var (
		next atomic.Int64
		wg   sync.WaitGroup
	)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				idx := int(next.Add(1) - 1)
				if idx >= len(segments) {
					return
				}
				select {
				case <-slots:
				case <-ctx.Done():
					return
				}
				data, err := fetchHLSSegment(ctx, segments[idx], headers)
				// The slot is buffered, so this never blocks and a worker whose
				// segment nobody is waiting for yet moves straight on.
				results[idx] <- segmentResult{data: data, err: err}
				if err != nil {
					return
				}
			}
		}()
	}

	var (
		written int64
		err     error
	)
	for i := range segments {
		select {
		case res := <-results[i]:
			if res.err != nil {
				err = fmt.Errorf("segment %d/%d: %w", i+1, len(segments), res.err)
				break
			}
			n, werr := w.Write(res.data)
			written += int64(n)
			if werr != nil {
				err = werr
			}
		case <-ctx.Done():
			err = ctx.Err()
		}
		if err != nil {
			break
		}
		// Hand the slot back so a fetcher can read one more segment ahead.
		select {
		case slots <- struct{}{}:
		default:
		}
	}

	// Unblock every fetcher still waiting on a slot or on the CDN before
	// returning, so no goroutine outlives the download.
	cancel()
	wg.Wait()
	return written, err
}

func fetchHLSSegment(ctx context.Context, rawURL string, headers map[string]string) ([]byte, error) {
	var last error
	for attempt := 0; attempt < hlsSegmentAttempts; attempt++ {
		if attempt > 0 {
			timer := time.NewTimer(hlsSegmentRetryWait)
			select {
			case <-timer.C:
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			}
			timer.Stop()
		}
		data, err := fetchHLSSegmentOnce(ctx, rawURL, headers)
		if err == nil {
			return data, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		last = err
	}
	return nil, last
}

func fetchHLSSegmentOnce(ctx context.Context, rawURL string, headers map[string]string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	applyUpstreamHeaders(req, headers)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer discardResponse(resp)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return nil, upstreamStatusError{code: resp.StatusCode, status: resp.Status}
	}
	var buf bytes.Buffer
	if resp.ContentLength > 0 && resp.ContentLength <= hlsSegmentMaxBytes {
		buf.Grow(int(resp.ContentLength))
	}
	if _, err := io.Copy(&buf, io.LimitReader(resp.Body, hlsSegmentMaxBytes)); err != nil {
		return nil, err
	}
	if buf.Len() == 0 {
		return nil, fmt.Errorf("empty segment")
	}
	return buf.Bytes(), nil
}

// remuxNative downloads the playlist into job.tmp without ffmpeg and reports
// whether it took the job.
//
// false means nothing was written and the job is untouched — no result
// delivered, no entry removed from the jobs map — so remux can hand the same
// job to ffmpeg as if the native attempt had never happened. Once a byte has
// reached a reader, falling back is no longer an option: ffmpeg would rewrite
// the file from the start and the readers following it would hear the opening
// of the track twice.
func (c *hlsCache) remuxNative(job *hlsJob, trackID, path string, target StreamTarget) bool {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()
	started := time.Now()

	segments, err := parseHLSPlaylist(ctx, target.URL, target.Headers)
	if err != nil {
		slog.Info("hls native parse declined, using ffmpeg", "track", trackID, "error", err)
		return false
	}

	slog.Info("hls native download started", "track", trackID, "segments", len(segments), "container", job.container.ext, "codec", target.ACodec)
	file, err := os.Create(job.tmp)
	if err != nil {
		slog.Warn("hls native output unusable, using ffmpeg", "track", trackID, "error", err)
		return false
	}
	written, err := downloadHLSSegments(ctx, file, segments, target.Headers)
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}

	if err != nil || written == 0 {
		if err == nil {
			err = fmt.Errorf("playlist produced no audio")
		}
		if written == 0 {
			// Nothing has been served, so ffmpeg can still start from scratch —
			// an expired signature or a segment layout this parser got wrong
			// costs a retry, not a failed play.
			_ = os.Remove(job.tmp)
			slog.Info("hls native download declined, using ffmpeg", "track", trackID, "error", err, "elapsed", time.Since(started))
			return false
		}
		c.finish(job, trackID, path, fmt.Errorf("hls native download failed after %d bytes: %w", written, err))
		return true
	}

	if err := os.Rename(job.tmp, path); err != nil {
		c.finish(job, trackID, path, err)
		return true
	}
	slog.Info("hls native download finished", "track", trackID, "bytes", written, "segments", len(segments), "container", job.container.ext, "elapsed", time.Since(started))
	c.finish(job, trackID, path, nil)
	return true
}
