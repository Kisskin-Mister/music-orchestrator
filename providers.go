package main

import (
	"encoding/base64"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"sync"
)

type ProviderService struct {
	cfg       Config
	extractor *Extractor
}

func NewProviderService(cfg Config) *ProviderService {
	return &ProviderService{cfg: cfg, extractor: NewExtractor(cfg)}
}

func (p *ProviderService) Providers() []Provider {
	return []Provider{youtubeOfficialProvider(), soundcloudOfficialProvider(), p.extractorProvider("youtube_stream", "YouTube Stream", "youtube"), p.extractorProvider("soundcloud_stream", "SoundCloud Stream", "soundcloud")}
}
func (p *ProviderService) byID(id string) (Provider, bool) {
	for _, pr := range p.Providers() {
		if pr.ID == id {
			return pr, true
		}
	}
	return Provider{}, false
}
func localCaps() Capabilities {
	return Capabilities{Search: true, Metadata: true, RawAudioStream: true, PersistentCache: true, OfflinePlayback: true, PublicDeploymentSafe: true}
}
func localPolicy() Policy {
	return Policy{DownloadAllowed: true, CacheAllowed: true, Notes: []string{"Local/demo media is safe to stream and cache."}}
}
func localProvider() Provider {
	return Provider{ID: "local", Name: "Local Demo Library", Kind: "local", Configured: true, Enabled: true, RiskLevel: "compliant", Capabilities: localCaps(), Policy: localPolicy()}
}
func youtubeOfficialProvider() Provider {
	return Provider{ID: "youtube_official", Name: "YouTube Official Embed", Kind: "youtube", Configured: false, Enabled: false, RiskLevel: "constrained", Capabilities: Capabilities{Search: false, Metadata: true, EmbedPlayback: true, PublicDeploymentSafe: true}, Policy: Policy{DownloadAllowed: false, CacheAllowed: false, RequiresAttribution: true, Notes: []string{"Use official embeds/API only."}}}
}
func soundcloudOfficialProvider() Provider {
	return Provider{ID: "soundcloud_official", Name: "SoundCloud Official", Kind: "soundcloud", Configured: false, Enabled: false, RiskLevel: "constrained", Capabilities: Capabilities{Search: false, Metadata: true, EmbedPlayback: true, PublicDeploymentSafe: true}, Policy: Policy{DownloadAllowed: false, CacheAllowed: false, RequiresAttribution: true, Notes: []string{"Official API credentials are optional future work."}}}
}
func (p *ProviderService) extractorProvider(id, name, kind string) Provider {
	return Provider{ID: id, Name: name, Kind: kind, Configured: p.cfg.EnableRiskyExtractors, Enabled: p.cfg.EnableRiskyExtractors, RiskLevel: "risky", Capabilities: Capabilities{Search: p.cfg.EnableRiskyExtractors, Metadata: p.cfg.EnableRiskyExtractors, RawAudioStream: p.cfg.EnableRiskyExtractors, PersistentCache: p.cfg.EnableRiskyExtractors, OfflinePlayback: p.cfg.EnableRiskyExtractors, PublicDeploymentSafe: false}, Policy: Policy{DownloadAllowed: p.cfg.EnableRiskyExtractors, CacheAllowed: p.cfg.EnableRiskyExtractors, RequiresAttribution: true, Notes: []string{"Personal self-hosted mode only. Users are responsible for source terms and law."}}}
}

func (p *ProviderService) Search(query string, providerIDs []string, limit int) []Track {
	if len(providerIDs) == 0 {
		if p.cfg.EnableRiskyExtractors {
			providerIDs = []string{"youtube_stream", "soundcloud_stream"}
		}
	}
	extractorIDs := []string{}
	for _, id := range providerIDs {
		switch id {
		case "youtube_stream", "soundcloud_stream":
			if p.cfg.EnableRiskyExtractors {
				extractorIDs = append(extractorIDs, id)
			}
		}
	}
	// Each extractor search is a cold yt-dlp process, so running them one after
	// another made total latency the sum of both. Fan out and keep the results
	// in request order so the provider ordering the caller asked for survives.
	results := make([][]Track, len(extractorIDs))
	var wg sync.WaitGroup
	for i, id := range extractorIDs {
		wg.Add(1)
		go func(idx int, providerID string) {
			defer wg.Done()
			items, err := p.extractor.Search(providerID, query, limit)
			if err != nil {
				slog.Warn("provider search failed", "provider", providerID, "error", err)
				return
			}
			results[idx] = items
		}(i, id)
	}
	wg.Wait()
	out := []Track{}
	for _, items := range results {
		if limit > 0 && len(out) >= limit {
			break
		}
		out = append(out, items...)
	}
	if limit > 0 && len(out) > limit {
		return out[:limit]
	}
	return out
}
func localSearch(query string) []Track {
	q := strings.ToLower(query)
	seeds := []Track{localTrack("seed-1", "Demo Song One", "Local Artist", 183), localTrack("seed-2", "Demo Lofi Loop", "Local Artist", 122), localTrack("seed-3", "Demo Synthwave Night", "Local Artist", 201)}
	if q == "" {
		return seeds
	}
	out := []Track{}
	for _, t := range seeds {
		if strings.Contains(strings.ToLower(t.Title+" "+t.Artist), q) {
			out = append(out, t)
		}
	}
	return out
}
func localTrack(pid, title, artist string, dur int) Track {
	return Track{ID: "local:" + pid, ProviderID: "local", ProviderTrackID: pid, Title: title, Artist: artist, DurationSeconds: dur, SourceURL: "/media/demo-" + pid + ".mp3", Attribution: "Local demo", Capabilities: localCaps(), Policy: localPolicy()}
}
func (p *ProviderService) Track(trackID string) (Track, bool) {
	provider, pid, ok := splitTrackID(trackID)
	if !ok {
		return Track{}, false
	}
	switch provider {
	case "local":
		for _, t := range localSearch("") {
			if t.ProviderTrackID == pid {
				return t, true
			}
		}
	case "youtube_stream", "soundcloud_stream":
		if p.cfg.EnableRiskyExtractors {
			t, err := p.extractor.Resolve(provider, pid)
			return t, err == nil
		}
	}
	return Track{}, false
}
func splitTrackID(id string) (string, string, bool) {
	a, b, ok := strings.Cut(id, ":")
	return a, b, ok && a != "" && b != ""
}
func ytURL(pid string) string     { return "https://www.youtube.com/watch?v=" + url.QueryEscape(pid) }
func scIDFromURL(u string) string { return "url_" + base64.RawURLEncoding.EncodeToString([]byte(u)) }
func scURLFromID(id string) (string, error) {
	raw := id
	if strings.HasPrefix(id, "url_") {
		b, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(id, "url_"))
		if err != nil {
			return "", err
		}
		raw = string(b)
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" {
		return "", fmt.Errorf("soundcloud extractor id must be a valid URL")
	}
	host := strings.ToLower(u.Hostname())
	if host != "soundcloud.com" && !strings.HasSuffix(host, ".soundcloud.com") && host != "sndcdn.com" && !strings.HasSuffix(host, ".sndcdn.com") {
		return "", fmt.Errorf("soundcloud URL host is not allowed")
	}
	return u.String(), nil
}
