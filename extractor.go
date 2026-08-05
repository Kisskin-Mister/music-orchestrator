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
// stay valid far longer than a minute, so a short cache turns replay/seek/next
// within one listening session into an instant hit without risking a stale link.
const streamCacheTTL = 60 * time.Second

type streamCacheEntry struct {
	target  StreamTarget
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
}

func NewExtractor(cfg Config) *Extractor { return &Extractor{cfg: cfg} }

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

func (e *Extractor) Search(providerID, query string, limit int) ([]Track, error) {
	if !e.cfg.EnableRiskyExtractors {
		return nil, fmt.Errorf("extractors disabled")
	}
	if limit <= 0 {
		limit = 10
	}
	searchLimit := limit
	if providerID == "youtube_stream" && searchLimit < 12 {
		searchLimit = 12
	}
	spec := "ytsearch" + fmt.Sprint(searchLimit) + ":" + query
	if providerID == "soundcloud_stream" {
		spec = "scsearch" + fmt.Sprint(limit) + ":" + query
	}
	info, err := e.dump(spec, e.cfg.ExtractorTimeout, providerID == "youtube_stream")
	if err != nil {
		slog.Warn("yt-dlp search failed", "provider", providerID, "error", err)
		return nil, err
	}
	entries := info.Entries
	if len(entries) == 0 {
		entries = []ytdlpInfo{info}
	}
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
	return out, nil
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

// StreamTarget resolves the audio source and reports how it is packaged.
func (e *Extractor) StreamTarget(providerID, pid string) (StreamTarget, error) {
	cacheKey := providerID + ":" + pid
	if cached, ok := e.streamCache.Load(cacheKey); ok {
		if entry, ok := cached.(streamCacheEntry); ok {
			if time.Now().Before(entry.expires) {
				return entry.target, nil
			}
			e.streamCache.Delete(cacheKey)
		}
	}
	u, err := e.urlFor(providerID, pid)
	if err != nil {
		return StreamTarget{}, err
	}
	info, err := e.dump(u, e.cfg.ExtractorTimeout, false)
	if err != nil {
		return StreamTarget{}, err
	}
	if info.URL != "" {
		return e.cacheStream(cacheKey, StreamTarget{URL: info.URL, Headers: info.HTTPHeaders, HLS: isHLS("", info.URL)}), nil
	}
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
	return StreamTarget{}, fmt.Errorf("no playable audio URL")
}

func targetFor(f ytdlpFormat, info ytdlpInfo) StreamTarget {
	return StreamTarget{
		URL:     f.URL,
		Headers: headersOr(f.HTTPHeaders, info.HTTPHeaders),
		HLS:     isHLS(f.Protocol, f.URL),
		ACodec:  f.ACodec,
		Ext:     f.Ext,
	}
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
	args := []string{"--no-playlist", "-x", "--audio-format", format, "-o", tmpl, u}
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

func (e *Extractor) dump(spec string, timeoutDur time.Duration, flatPlaylist bool) (ytdlpInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeoutDur)
	defer cancel()
	args := []string{"--no-update", "--retries", "1", "--socket-timeout", "20"}
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
