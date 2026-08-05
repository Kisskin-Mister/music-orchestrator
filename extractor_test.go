package main

import "testing"

func TestSoundcloudFallbackURL(t *testing.T) {
	// Entries that carry no webpage_url/original_url/url used to be dropped, which
	// silently emptied every SoundCloud search. A numeric id is enough to rebuild
	// a URL that yt-dlp resolves and scURLFromID accepts.
	got := soundcloudFallbackURL(ytdlpInfo{ID: "123456789"})
	if got != "https://api.soundcloud.com/tracks/123456789" {
		t.Fatalf("numeric id should rebuild the API URL, got %q", got)
	}
	if _, err := scURLFromID(scIDFromURL(got)); err != nil {
		t.Fatalf("rebuilt URL must survive the host allowlist: %v", err)
	}
	for _, id := range []string{"", "  ", "not-numeric", "12ab34"} {
		if got := soundcloudFallbackURL(ytdlpInfo{ID: id}); got != "" {
			t.Fatalf("id %q is not a track id, expected no fallback, got %q", id, got)
		}
	}
}

func TestStreamSourceCacheServesWithinTTL(t *testing.T) {
	e := NewExtractor(Config{})
	e.cacheStream("youtube_stream:abc", StreamTarget{URL: "https://cdn.example/a", Headers: map[string]string{"User-Agent": "x"}})
	url, headers, err := e.StreamSource("youtube_stream", "abc")
	if err != nil {
		t.Fatalf("cached entry should be returned without invoking yt-dlp: %v", err)
	}
	if url != "https://cdn.example/a" || headers["User-Agent"] != "x" {
		t.Fatalf("cache returned %q %v", url, headers)
	}
	// A different track must not read another track's cached link.
	if _, _, err := e.StreamSource("youtube_stream", "other"); err == nil {
		t.Fatal("uncached track should not resolve from the cache")
	}
}
