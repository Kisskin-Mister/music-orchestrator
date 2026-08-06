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
