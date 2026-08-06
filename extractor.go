package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Resolving a stream URL costs a cold yt-dlp process (seconds). The CDN links
// stay valid for hours, so caching them across a listening session turns
// replay/seek/next into instant hits. An hour matches how long the links
// actually live; the five minutes this used to be expired mid-track, so seeking
// past 5:00 paid for a fresh yt-dlp run. A link that does expire early is
// recovered by the 403 re-resolve in the stream handler rather than by the TTL.
const streamCacheTTL = 1 * time.Hour

type streamCacheEntry struct {
	target  StreamTarget
	expires time.Time
}

// A search is a cold yt-dlp process — fork plus Python startup plus the network
// round trip, seconds per call on a Pi — and yt-dlp has no search pagination, so
// every page re-runs the query from the top. Caching the result set per
// provider+query makes a repeated query instant, which is what a listener
// typing, backing out and re-searching actually does. Five minutes is long
// enough to cover that loop — users rarely need sub-day freshness on search.
const searchCacheTTL = 24 * time.Hour

// searchCacheMaxEntries bounds the map so a long-lived process cannot grow it
// without limit. Expired entries are swept first; the cap only bites when a
// burst of distinct queries all arrive inside one TTL.
const searchCacheMaxEntries = 256

// searchCacheEntry records the depth it was fetched at, because a request for a
// deeper page than the entry was built from has to miss: yt-dlp was never asked
// for those results, so serving the short list would end pagination early.
type searchCacheEntry struct {
	items   []Track
	limit   int
	more    bool
	expires time.Time
}

// StreamTarget is one resolved audio source plus what the stream handler needs
// to decide how to serve it. SoundCloud dropped progressive downloads: every
// format it offers today is an HLS playlist, which no <audio> element outside
// Safari can play and which has no byte length to range over. Carrying the
// protocol and codec here is what lets the handler pick the proxy path for a
// plain file and the remux path for a playlist.
type StreamTarget struct {
	URL     string
	Headers map[string]string
	HLS     bool
	ACodec  string
	Ext     string
	// Duration is the track length in seconds as yt-dlp reports it. A remuxed
	// playlist is served before its byte length is known, so this is the only
	// thing the client can build a scrubber from until the file is complete.
	Duration float64
}

// streamFlight is one in-progress resolve. yt-dlp is expensive enough that two
// listeners starting the same cold track must not each pay for a process: the
// second waits on the first's result. Warm-on-play makes this the normal case —
// /v1/playback starts a resolve that /v1/stream arrives on top of a moment
// later.
type streamFlight struct {
	done   chan struct{}
	target StreamTarget
	err    error
}

// isHLS is true for a manifest rather than a file. yt-dlp names the protocol
// m3u8/m3u8_native, but a bare URL check covers formats that report none.
func isHLS(protocol, rawURL string) bool {
	if strings.HasPrefix(protocol, "m3u8") {
		return true
	}
	if before, _, _ := strings.Cut(rawURL, "?"); strings.HasSuffix(before, ".m3u8") {
		return true
	}
	return false
}

type Extractor struct {
	cfg         Config
	streamCache sync.Map // cacheKey -> streamCacheEntry
	streamFlow  sync.Map // cacheKey -> *streamFlight
	searchMu    sync.Mutex
	searchCache map[string]searchCacheEntry // providerID+query -> searchCacheEntry
}

func NewExtractor(cfg Config) *Extractor {
	return &Extractor{cfg: cfg, searchCache: map[string]searchCacheEntry{}}
}

type ytdlpInfo struct {
	ID          string            `json:"id"`
	Title       string            `json:"title"`
	Uploader    string            `json:"uploader"`
	Channel     string            `json:"channel"`
	Artist      string            `json:"artist"`
	Album       string            `json:"album"`
	Track       string            `json:"track"`
	Categories  []string          `json:"categories"`
	Duration    float64           `json:"duration"`
	WebpageURL  string            `json:"webpage_url"`
	OriginalURL string            `json:"original_url"`
	Thumbnail   string            `json:"thumbnail"`
	URL         string            `json:"url"`
	ACodec      string            `json:"acodec"`
	Ext         string            `json:"ext"`
	Protocol    string            `json:"protocol"`
	IEKey       string            `json:"ie_key"`
	Type        string            `json:"_type"`
	IsLive      bool              `json:"is_live"`
	LiveStatus  string            `json:"live_status"`
	Entries     []ytdlpInfo       `json:"entries"`
	Formats     []ytdlpFormat     `json:"formats"`
	HTTPHeaders map[string]string `json:"http_headers"`
}

type ytdlpFormat struct {
	URL         string            `json:"url"`
	HTTPHeaders map[string]string `json:"http_headers"`
	ACodec      string            `json:"acodec"`
	VCodec      string            `json:"vcodec"`
	Ext         string            `json:"ext"`
	ABR         float64           `json:"abr"`
	TBR         float64           `json:"tbr"`
	Protocol    string            `json:"protocol"`
}

// Search returns the playable tracks for one page and whether the provider
// still had results past the ones it was asked for. That second value is the
// only reliable "keep scrolling" signal: the count of tracks handed back cannot
// carry it, because filtering live streams, channels and playlist rows out of
// a full response routinely leaves fewer tracks than were requested, which is
// indistinguishable from having reached the end of the results.
func (e *Extractor) Search(providerID, query string, limit int) ([]Track, bool, error) {
	if !e.cfg.EnableRiskyExtractors {
		return nil, false, fmt.Errorf("extractors disabled")
	}
	if limit <= 0 {
		limit = 10
	}
	cacheKey := providerID + "\x00" + query
	if items, more, ok := e.cachedSearch(cacheKey, limit); ok {
		return items, more, nil
	}
	searchLimit := limit
	if providerID == "youtube_stream" && searchLimit < 12 {
		searchLimit = 12
	}
	spec := "ytsearch" + fmt.Sprint(searchLimit) + ":" + query
	if providerID == "soundcloud_stream" {
		spec = "scsearch" + fmt.Sprint(searchLimit) + ":" + query
	}
	info, err := e.dump(spec, e.cfg.ExtractorTimeout, providerID == "youtube_stream")
	if err != nil {
		slog.Warn("yt-dlp search failed", "provider", providerID, "error", err)
		return nil, false, err
	}
	entries := info.Entries
	if len(entries) == 0 {
		entries = []ytdlpInfo{info}
	}
	// A search that comes back as full as it was asked to be was cut off by the
	// limit, not by running out of hits, so deeper pages exist. Guessing "more"
	// wrongly costs one extra request that returns nothing; guessing "done"
	// wrongly strands the listener on page one, so lean towards more.
	more := len(entries) >= searchLimit
	if providerID == "youtube_stream" {
		sort.SliceStable(entries, func(i, j int) bool {
			return youtubeAudioScore(entries[i], query) > youtubeAudioScore(entries[j], query)
		})
	}
	out := []Track{}
	for _, it := range entries {
		if len(out) >= limit {
			break
		}
		if it.Title == "" || it.IsLive || it.LiveStatus == "is_live" || it.LiveStatus == "is_upcoming" {
			continue
		}
		pid := it.ID
		source := first(it.WebpageURL, it.OriginalURL, it.URL)
		if providerID == "youtube_stream" && !isYouTubeSearchVideo(it, pid, source) {
			continue
		}
		if providerID == "soundcloud_stream" {
			if source == "" {
				source = soundcloudFallbackURL(it)
			}
			if source == "" {
				continue
			}
			pid = scIDFromURL(source)
		}
		if pid == "" {
			continue
		}
		out = append(out, e.toTrack(providerID, pid, it, source))
	}
	// Filling the page while entries were still unread is the other way to know
	// the results continue, and it catches the case where yt-dlp was asked for
	// more than the caller wanted via the YouTube minimum above.
	if len(out) >= limit && len(entries) > len(out) {
		more = true
	}
	e.storeSearch(cacheKey, limit, out, more)
	return out, more, nil
}

// cachedSearch returns a copy of a live entry that was fetched at least as deep
// as limit. The copy matters: callers annotate the tracks they get back in
// place, and that must not write through into the cached slice.
func (e *Extractor) cachedSearch(key string, limit int) ([]Track, bool, bool) {
	e.searchMu.Lock()
	defer e.searchMu.Unlock()
	entry, ok := e.searchCache[key]
	if !ok || entry.limit < limit || time.Now().After(entry.expires) {
		return nil, false, false
	}
	items, more := entry.items, entry.more
	if len(items) > limit {
		// Serving a prefix of a deeper entry: whatever was trimmed is itself
		// proof that the results continue past this page.
		items, more = items[:limit], true
	}
	return append([]Track(nil), items...), more, true
}

func (e *Extractor) storeSearch(key string, limit int, items []Track, more bool) {
	e.searchMu.Lock()
	defer e.searchMu.Unlock()
	if e.searchCache == nil {
		e.searchCache = map[string]searchCacheEntry{}
	}
	now := time.Now()
	for k, v := range e.searchCache {
		if now.After(v.expires) {
			delete(e.searchCache, k)
		}
	}
	// Nothing expired and the map is full: evict whatever is closest to expiring
	// so the query in hand still gets a slot.
	if len(e.searchCache) >= searchCacheMaxEntries {
		evictKey, evictAt := "", time.Time{}
		for k, v := range e.searchCache {
			if evictKey == "" || v.expires.Before(evictAt) {
				evictKey, evictAt = k, v.expires
			}
		}
		delete(e.searchCache, evictKey)
	}
	e.searchCache[key] = searchCacheEntry{items: append([]Track(nil), items...), limit: limit, more: more, expires: now.Add(searchCacheTTL)}
}

// scsearch entries occasionally arrive without webpage_url/original_url/url —
// partially hydrated results. Skipping those dropped every SoundCloud hit, so
// rebuild the canonical API URL from the numeric track id instead: yt-dlp
// resolves that form, and it passes the host allowlist in scURLFromID.
func soundcloudFallbackURL(it ytdlpInfo) string {
	id := strings.TrimSpace(it.ID)
	if id == "" || !isDigits(id) {
		return ""
	}
	return "https://api.soundcloud.com/tracks/" + id
}

func isDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func (e *Extractor) Resolve(providerID, pid string) (Track, error) {
	u, err := e.urlFor(providerID, pid)
	if err != nil {
		return Track{}, err
	}
	info, err := e.dump(u, e.cfg.ExtractorTimeout, false)
	if err != nil {
		return Track{}, err
	}
	return e.toTrack(providerID, choosePID(providerID, pid, info), info, first(info.WebpageURL, info.OriginalURL, u)), nil
}

// StreamSource returns the CDN URL plus the headers yt-dlp says that URL
// expects. The headers matter: googlevideo bindssome responses to the client
// that negotiated them, so replacing the User-Agent with our own can turn a
// working link into a 403.
func (e *Extractor) StreamSource(providerID, pid string) (string, map[string]string, error) {
	target, err := e.StreamTarget(providerID, pid)
	if err != nil {
		return "", nil, err
	}
	return target.URL, target.Headers, nil
}

// StreamTarget resolves the audio source and reports how it is packaged. One
// yt-dlp process runs per track at a time; concurrent callers share its result.
func (e *Extractor) StreamTarget(providerID, pid string) (StreamTarget, error) {
	cacheKey := providerID + ":" + pid
	if target, ok := e.cachedStream(cacheKey); ok {
		return target, nil
	}

	flight := &streamFlight{done: make(chan struct{})}
	if existing, loaded := e.streamFlow.LoadOrStore(cacheKey, flight); loaded {
		leader := existing.(*streamFlight)
		<-leader.done
		return leader.target, leader.err
	}
	defer func() {
		// Dropped before the waiters are released, and after resolveStream has
		// already filled the cache, so a caller arriving in between finds the
		// finished entry rather than starting a second process.
		e.streamFlow.Delete(cacheKey)
		close(flight.done)
	}()
	flight.target, flight.err = e.resolveStream(providerID, pid, cacheKey)
	return flight.target, flight.err
}

func (e *Extractor) cachedStream(cacheKey string) (StreamTarget, bool) {
	cached, ok := e.streamCache.Load(cacheKey)
	if !ok {
		return StreamTarget{}, false
	}
	entry, ok := cached.(streamCacheEntry)
	if !ok {
		return StreamTarget{}, false
	}
	if time.Now().Before(entry.expires) {
		return entry.target, true
	}
	e.streamCache.Delete(cacheKey)
	return StreamTarget{}, false
}

func (e *Extractor) resolveStream(providerID, pid, cacheKey string) (StreamTarget, error) {
	u, err := e.urlFor(providerID, pid)
	if err != nil {
		return StreamTarget{}, err
	}
	info, err := e.dump(u, e.cfg.ExtractorTimeout, false)
	if err != nil {
		return StreamTarget{}, err
	}
	// The format list is scored before yt-dlp's own pick is considered. Left to
	// itself yt-dlp maximises bitrate, which on SoundCloud means the 160k AAC
	// playlist even for tracks that also publish a plain 128k MP3 — a remux the
	// listener waits on instead of a file the proxy streams in one round trip.
	// scoreFormat knows which of those this backend can actually serve fast.
	formats := info.Formats
	sort.SliceStable(formats, func(i, j int) bool { return scoreFormat(formats[i]) > scoreFormat(formats[j]) })
	for _, f := range formats {
		if f.URL != "" && f.VCodec == "none" && f.ACodec != "none" {
			return e.cacheStream(cacheKey, targetFor(f, info)), nil
		}
	}
	for _, f := range formats {
		if f.URL != "" && f.ACodec != "none" {
			return e.cacheStream(cacheKey, targetFor(f, info)), nil
		}
	}
	// Nothing scoreable: fall back to whatever yt-dlp resolved. The codec and
	// extension come along so the remux can copy the audio rather than guess.
	if info.URL != "" {
		return e.cacheStream(cacheKey, StreamTarget{
			URL:      info.URL,
			Headers:  info.HTTPHeaders,
			HLS:      isHLS(info.Protocol, info.URL),
			ACodec:   info.ACodec,
			Ext:      info.Ext,
			Duration: info.Duration,
		}), nil
	}
	return StreamTarget{}, fmt.Errorf("no playable audio URL")
}

func targetFor(f ytdlpFormat, info ytdlpInfo) StreamTarget {
	return StreamTarget{
		URL:      f.URL,
		Headers:  headersOr(f.HTTPHeaders, info.HTTPHeaders),
		HLS:      isHLS(f.Protocol, f.URL),
		ACodec:   f.ACodec,
		Ext:      f.Ext,
		Duration: info.Duration,
	}
}

// InvalidateStream drops a cached CDN link that the upstream has since
// rejected, so the next StreamTarget call resolves a fresh one.
func (e *Extractor) InvalidateStream(providerID, pid string) {
	e.streamCache.Delete(providerID + ":" + pid)
}

func (e *Extractor) cacheStream(cacheKey string, target StreamTarget) StreamTarget {
	e.streamCache.Store(cacheKey, streamCacheEntry{target: target, expires: time.Now().Add(streamCacheTTL)})
	return target
}

func headersOr(primary, fallback map[string]string) map[string]string {
	if len(primary) > 0 {
		return primary
	}
	return fallback
}

func scoreFormat(f ytdlpFormat) float64 {
	s := f.ABR
	if s == 0 {
		s = f.TBR
	}
	if f.VCodec == "none" {
		s += 10000
	}
	// AVPlayer does not decode YouTube's usual WebM/Opus bestaudio (iOS
	// reports AVFoundation -11828). Prefer broadly playable AAC/M4A, then MP3,
	// while retaining the old bitrate/audio-only ordering as a fallback.
	if f.Ext == "m4a" || strings.HasPrefix(f.ACodec, "mp4a") || f.ACodec == "aac" {
		s += 1_000_000
	} else if f.Ext == "mp3" || f.ACodec == "mp3" {
		s += 900_000
	} else if f.Ext == "webm" || f.ACodec == "opus" {
		s -= 100_000
	}
	// An HLS playlist has to be remuxed before it can be played. SoundCloud
	// offers a progressive URL alongside the playlist for many tracks; taking it
	// keeps playback on the fast proxy path.
	if isHLS(f.Protocol, f.URL) {
		s -= 500_000
		// Among playlists the AAC preference above flips. An MP3 playlist
		// remuxes into a bare stream of frames the handler can serve while
		// ffmpeg is still writing it, whereas AAC has to be repacked into a
		// container, and the MP4 one a player wants cannot be read until the
		// remux finishes. See containerFor.
		if f.Ext == "mp3" || f.ACodec == "mp3" {
			s += 200_000
		}
	}
	return s
}

func youtubeAudioScore(it ytdlpInfo, query string) int {
	score := 0
	if isYouTubeAudioTopic(it) {
		score += 1000
	}
	if hasCategory(it, "Music") {
		score += 100
	}
	if it.Track != "" {
		score += 250
	}
	if it.Album != "" {
		score += 80
	}
	if strings.Contains(strings.ToLower(first(it.Uploader, it.Channel)), " - topic") {
		score += 180
	}
	if it.Duration > 45 && it.Duration < 600 {
		score += 50
	}
	if strings.Contains(strings.ToLower(it.Title), "official video") || strings.Contains(strings.ToLower(it.Title), "music video") || strings.Contains(strings.ToLower(it.Title), "клип") {
		score -= 300
	}
	q := strings.ToLower(query)
	if it.Track != "" && strings.Contains(q, strings.ToLower(it.Track)) {
		score += 120
	}
	return score
}

func isYouTubeAudioTopic(it ytdlpInfo) bool {
	if it.Track != "" && it.Artist != "" && hasCategory(it, "Music") {
		return true
	}
	return strings.Contains(strings.ToLower(first(it.Uploader, it.Channel)), " - topic")
}

func hasCategory(it ytdlpInfo, category string) bool {
	for _, c := range it.Categories {
		if strings.EqualFold(c, category) {
			return true
		}
	}
	return false
}

func (e *Extractor) Download(providerID, pid, format, mediaRoot string) (string, int64, error) {
	if format == "" {
		format = "mp3"
	}
	u, err := e.urlFor(providerID, pid)
	if err != nil {
		return "", 0, err
	}
	if err := os.MkdirAll(mediaRoot, 0755); err != nil {
		return "", 0, err
	}
	stem := safeStem(providerID, pid)
	tmpl := filepath.Join(mediaRoot, stem+".%(ext)s")
	ctx, cancel := context.WithTimeout(context.Background(), e.cfg.DownloadTimeout)
	defer cancel()
	args := append(e.jsRuntimeArgs(), "--no-playlist", "-x", "--audio-format", format, "-o", tmpl, u)
	cmd := exec.CommandContext(ctx, e.cfg.YTDLPBinary, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", 0, fmt.Errorf("yt-dlp download failed: %w: %s", err, stderr.String())
	}
	matches, _ := filepath.Glob(filepath.Join(mediaRoot, stem+".*"))
	if len(matches) == 0 {
		return "", 0, fmt.Errorf("download completed but file not found")
	}
	sort.Strings(matches)
	fi, err := os.Stat(matches[0])
	if err != nil {
		return "", 0, err
	}
	return filepath.Base(matches[0]), fi.Size(), nil
}

// jsRuntimeArgs enables a JavaScript runtime for every yt-dlp invocation.
// Without one yt-dlp warns on each run and falls back to clients that cannot
// extract some SoundCloud HLS and YouTube formats at all.
func (e *Extractor) jsRuntimeArgs() []string {
	rt := strings.TrimSpace(e.cfg.YTDLPJSRuntimes)
	if rt == "" {
		return nil
	}
	return []string{"--js-runtimes", rt}
}

func (e *Extractor) dump(spec string, timeoutDur time.Duration, flatPlaylist bool) (ytdlpInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeoutDur)
	defer cancel()
	args := append([]string{"--no-update", "--retries", "1", "--socket-timeout", "20"}, e.jsRuntimeArgs()...)
	if flatPlaylist {
		args = append(args, "--flat-playlist")
	}
	args = append(args, "--dump-single-json", "--skip-download", spec)
	cmd := exec.CommandContext(ctx, e.cfg.YTDLPBinary, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return ytdlpInfo{}, fmt.Errorf("yt-dlp failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	var info ytdlpInfo
	if err := json.Unmarshal(stdout.Bytes(), &info); err == nil && (info.Title != "" || len(info.Entries) > 0 || info.ID != "") {
		return info, nil
	}
	// Some older wrappers/scripts print one JSON object per line.
	s := bufio.NewScanner(bytes.NewReader(stdout.Bytes()))
	entries := []ytdlpInfo{}
	for s.Scan() {
		line := bytes.TrimSpace(s.Bytes())
		if len(line) == 0 {
			continue
		}
		var it ytdlpInfo
		if err := json.Unmarshal(line, &it); err == nil && (it.Title != "" || it.ID != "") {
			entries = append(entries, it)
		}
	}
	if len(entries) > 0 {
		return ytdlpInfo{Entries: entries}, nil
	}
	return ytdlpInfo{}, fmt.Errorf("cannot parse yt-dlp JSON")
}

func isYouTubeSearchVideo(it ytdlpInfo, pid, source string) bool {
	if it.IEKey != "" && it.IEKey != "Youtube" {
		return false
	}
	if strings.HasPrefix(pid, "UC") || strings.Contains(source, "/channel/") || strings.Contains(source, "/playlist?") {
		return false
	}
	return true
}

func (e *Extractor) toTrack(providerID, pid string, it ytdlpInfo, source string) Track {
	pr := e.provider(providerID)
	artwork := first(it.Thumbnail)
	if artwork == "" && providerID == "youtube_stream" && isSafeYouTubeVideoID(pid) {
		artwork = youtubeThumbnailURL(pid)
	}
	return Track{
		ID:              providerID + ":" + pid,
		ProviderID:      providerID,
		ProviderTrackID: pid,
		Title:           first(it.Track, it.Title, pid),
		Artist:          first(it.Artist, it.Uploader, it.Channel),
		Album:           it.Album,
		DurationSeconds: int(it.Duration),
		ArtworkURL:      artwork,
		SourceURL:       source,
		Attribution:     pr.Name,
		Official:        isYouTubeAudioTopic(it),
		Capabilities:    pr.Capabilities,
		Policy:          pr.Policy,
	}
}

func youtubeThumbnailURL(videoID string) string {
	// hqdefault exists for every normal YouTube video, unlike maxresdefault.
	return "https://i.ytimg.com/vi/" + videoID + "/hqdefault.jpg"
}

func isSafeYouTubeVideoID(id string) bool {
	if id == "" {
		return false
	}
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			continue
		}
		return false
	}
	return true
}

func (e *Extractor) provider(id string) Provider {
	return (&ProviderService{cfg: e.cfg}).extractorProvider(id, map[string]string{"youtube_stream": "YouTube Stream", "soundcloud_stream": "SoundCloud Stream"}[id], map[string]string{"youtube_stream": "youtube", "soundcloud_stream": "soundcloud"}[id])
}

func (e *Extractor) urlFor(providerID, pid string) (string, error) {
	switch providerID {
	case "youtube_stream":
		return ytURL(pid), nil
	case "soundcloud_stream":
		return scURLFromID(pid)
	default:
		return "", fmt.Errorf("unsupported extractor provider %s", providerID)
	}
}

func choosePID(providerID, fallback string, info ytdlpInfo) string {
	if providerID == "soundcloud_stream" {
		return fallback
	}
	if info.ID != "" {
		return info.ID
	}
	return fallback
}

func first(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
