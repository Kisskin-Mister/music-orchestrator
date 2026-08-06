package main

import (
	"testing"
	"time"
)

func TestSearchCacheServesEqualOrShallowerPages(t *testing.T) {
	e := NewExtractor(Config{})
	e.storeSearch("youtube_stream\x00lofi", 3, []Track{{ID: "a"}, {ID: "b"}, {ID: "c"}}, false)

	items, more, ok := e.cachedSearch("youtube_stream\x00lofi", 3)
	if !ok || len(items) != 3 {
		t.Fatalf("same-depth lookup should hit, got %d items ok=%v", len(items), ok)
	}
	if more {
		t.Fatal("an exhausted entry must not claim more results")
	}
	// A shallower page is a prefix of a deeper one, so it can be served — and
	// the tracks trimmed off it prove the results continue.
	items, more, ok = e.cachedSearch("youtube_stream\x00lofi", 2)
	if !ok || len(items) != 2 || items[1].ID != "b" {
		t.Fatalf("shallower lookup should be truncated, got %#v ok=%v", items, ok)
	}
	if !more {
		t.Fatal("a truncated entry has results past the page")
	}
	// A deeper page must miss: yt-dlp was never asked for those results, and
	// serving the short list would end pagination a page early.
	if _, _, ok := e.cachedSearch("youtube_stream\x00lofi", 4); ok {
		t.Fatal("deeper page must not be served from a shallower entry")
	}
	// The query and the provider both have to match.
	if _, _, ok := e.cachedSearch("soundcloud_stream\x00lofi", 3); ok {
		t.Fatal("another provider must not read this entry")
	}
}

func TestSearchCacheExpiresAndCopies(t *testing.T) {
	e := NewExtractor(Config{})
	e.storeSearch("youtube_stream\x00lofi", 1, []Track{{ID: "a"}}, false)

	// Callers annotate the tracks they get back in place (download state), so a
	// handed-out slice must never alias the cached one.
	items, _, _ := e.cachedSearch("youtube_stream\x00lofi", 1)
	items[0].Title = "mutated"
	again, _, _ := e.cachedSearch("youtube_stream\x00lofi", 1)
	if again[0].Title != "" {
		t.Fatalf("caller mutation leaked into the cache: %#v", again[0])
	}

	e.searchMu.Lock()
	entry := e.searchCache["youtube_stream\x00lofi"]
	entry.expires = time.Now().Add(-time.Second)
	e.searchCache["youtube_stream\x00lofi"] = entry
	e.searchMu.Unlock()
	if _, _, ok := e.cachedSearch("youtube_stream\x00lofi", 1); ok {
		t.Fatal("an expired entry must not be served")
	}
}

func TestJSRuntimeArgs(t *testing.T) {
	// yt-dlp enables only deno by default and warns on every run without a JS
	// runtime; node is what the host actually has.
	got := (&Extractor{cfg: Config{YTDLPJSRuntimes: "node"}}).jsRuntimeArgs()
	if len(got) != 2 || got[0] != "--js-runtimes" || got[1] != "node" {
		t.Fatalf("want the runtime flag, got %#v", got)
	}
	// Empty config means an older yt-dlp that would reject the flag outright.
	if got := (&Extractor{cfg: Config{YTDLPJSRuntimes: "  "}}).jsRuntimeArgs(); got != nil {
		t.Fatalf("blank runtime should drop the flag, got %#v", got)
	}
}

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

// SoundCloud publishes the same track as an HLS playlist and, often, as a
// progressive file. The playlist costs a 10-30s ffmpeg remux before the first
// note, so the plain file has to win even when the playlist carries the codec
// we otherwise prefer.
func TestScoreFormatPrefersProgressiveOverHLS(t *testing.T) {
	hls := ytdlpFormat{URL: "https://cdn.example/a.m3u8", Protocol: "m3u8_native", ACodec: "mp4a.40.2", Ext: "m4a", ABR: 128}
	progressive := ytdlpFormat{URL: "https://cdn.example/a.mp3", Protocol: "http", ACodec: "mp3", Ext: "mp3", ABR: 128}
	if scoreFormat(progressive) <= scoreFormat(hls) {
		t.Fatalf("progressive %v should outrank HLS %v", scoreFormat(progressive), scoreFormat(hls))
	}
	// Between two playlists the codec preference still decides.
	hlsOpus := ytdlpFormat{URL: "https://cdn.example/b.m3u8", Protocol: "m3u8_native", ACodec: "opus", Ext: "webm", ABR: 128}
	if scoreFormat(hls) <= scoreFormat(hlsOpus) {
		t.Fatal("AAC playlist should still outrank an Opus playlist")
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
