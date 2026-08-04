package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestArtworkHostAllowed(t *testing.T) {
	allowed := []string{
		"i.ytimg.com",
		"ytimg.com",
		"i1.sndcdn.com",
		"yt3.ggpht.com",
		"i.ytimg.com:443",
	}
	for _, host := range allowed {
		if !artworkHostAllowed(host) {
			t.Errorf("expected %q to be allowed", host)
		}
	}
	// The SSRF cases: internal addresses, and lookalike domains that merely
	// contain an allowed name rather than ending with it.
	blocked := []string{
		"127.0.0.1",
		"localhost",
		"169.254.169.254",
		"192.168.1.10:8080",
		"evil.com",
		"ytimg.com.evil.com",
		"notytimg.com",
		"",
	}
	for _, host := range blocked {
		if artworkHostAllowed(host) {
			t.Errorf("expected %q to be blocked", host)
		}
	}
}

func TestArtworkRejectsDisallowedTargets(t *testing.T) {
	app := testApp(t, false)
	cases := []struct {
		name string
		url  string
		want int
	}{
		{"missing url", "/v1/artwork", http.StatusBadRequest},
		{"not absolute", "/v1/artwork?url=/etc/passwd", http.StatusBadRequest},
		{"file scheme", "/v1/artwork?url=file:///etc/passwd", http.StatusBadRequest},
		{"internal host", "/v1/artwork?url=http://127.0.0.1:18080/health", http.StatusForbidden},
		{"metadata host", "/v1/artwork?url=http://169.254.169.254/latest/meta-data/", http.StatusForbidden},
		{"lookalike host", "/v1/artwork?url=https://ytimg.com.evil.com/a.jpg", http.StatusForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.url, nil))
			if rec.Code != tc.want {
				t.Fatalf("got status %d, want %d (body: %s)", rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}
