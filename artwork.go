package main

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// artworkHostAllowlist limits which hosts the server will fetch on a client's
// behalf. Without it, GET /v1/artwork?url=... would be an open SSRF proxy into
// the server's own network (cloud metadata endpoints, LAN services, ...).
// Suffix match, so only these CDNs and their subdomains are reachable.
var artworkHostAllowlist = []string{
	"ytimg.com",      // YouTube thumbnails
	"ggpht.com",      // YouTube channel art
	"sndcdn.com",     // SoundCloud artwork
	"soundcloud.com", // SoundCloud (redirects to sndcdn)
}

const (
	artworkMaxBytes = 5 << 20 // 5 MiB: thumbnails are ~20-200 KiB; this only stops abuse.
	artworkTimeout  = 10 * time.Second
	artworkCacheTTL = "public, max-age=86400"
)

func artworkHostAllowed(host string) bool {
	host = strings.ToLower(host)
	if i := strings.IndexByte(host, ':'); i >= 0 {
		host = host[:i]
	}
	for _, allowed := range artworkHostAllowlist {
		if host == allowed || strings.HasSuffix(host, "."+allowed) {
			return true
		}
	}
	return false
}

// artwork proxies remote cover images through this server.
//
// Two reasons this exists instead of pointing clients straight at the CDN:
// the clients may run where the CDN is unreachable (restricted network, a
// device that does not share the host's proxy), and proxying keeps the user's
// IP off third-party CDNs, which matches the self-hosted intent of the project.
func (a *App) artwork(w http.ResponseWriter, r *http.Request) {
	raw := r.URL.Query().Get("url")
	if raw == "" {
		writeErrorCode(w, http.StatusBadRequest, "missing_url", "Query parameter 'url' is required")
		return
	}
	target, err := url.Parse(raw)
	if err != nil || (target.Scheme != "https" && target.Scheme != "http") {
		writeErrorCode(w, http.StatusBadRequest, "invalid_url", "Artwork URL must be an absolute http(s) URL")
		return
	}
	if !artworkHostAllowed(target.Host) {
		writeErrorCode(w, http.StatusForbidden, "host_not_allowed", "Artwork host is not allowed")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), artworkTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		writeErrorCode(w, http.StatusBadGateway, "artwork_fetch_failed", "Cannot build artwork request")
		return
	}
	resp, err := artworkClient.Do(req)
	if err != nil {
		writeErrorCode(w, http.StatusBadGateway, "artwork_fetch_failed", "Cannot fetch artwork")
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		writeErrorCode(w, http.StatusBadGateway, "artwork_fetch_failed", "Artwork source returned an error")
		return
	}
	contentType := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "image/") {
		writeErrorCode(w, http.StatusBadGateway, "artwork_not_image", "Artwork source did not return an image")
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", artworkCacheTTL)
	_, _ = io.Copy(w, io.LimitReader(resp.Body, artworkMaxBytes))
}

// Redirects are followed only while every hop stays inside the allowlist,
// so a permitted host cannot bounce the request to an internal address.
var artworkClient = &http.Client{
	Timeout: artworkTimeout,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return http.ErrUseLastResponse
		}
		if !artworkHostAllowed(req.URL.Host) {
			return http.ErrUseLastResponse
		}
		return nil
	},
}
