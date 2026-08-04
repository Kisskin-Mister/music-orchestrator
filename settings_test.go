package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// adminSession registers the first account, which the store makes the owner/admin.
func adminSession(t *testing.T, app *App) string {
	t.Helper()
	body := `{"username":"owner","password":"owner-password-123"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK && rec.Code != http.StatusCreated {
		t.Fatalf("register failed: %d %s", rec.Code, rec.Body.String())
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookieName {
			return c.Value
		}
	}
	t.Fatal("no session cookie returned")
	return ""
}

func patchSettings(t *testing.T, app *App, session, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPatch, "/v1/settings", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: session})
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	return rec
}

func TestSettingsNeverExposeSecretsOrExecutablePath(t *testing.T) {
	app := testApp(t, false)
	session := adminSession(t, app)
	app.cfg.YouTubeAPIKey = "super-secret-key"
	app.cfg.NavidromeToken = "navidrome-secret"

	req := httptest.NewRequest(http.MethodGet, "/v1/settings", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: session})
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	raw := rec.Body.String()
	if strings.Contains(raw, "super-secret-key") || strings.Contains(raw, "navidrome-secret") {
		t.Fatalf("settings response leaked a secret: %s", raw)
	}

	var view settingsView
	if err := json.Unmarshal([]byte(raw), &view); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !view.YouTubeAPIKeySet || !view.NavidromeTokenSet {
		t.Fatal("expected secrets to be reported as set")
	}
}

// The yt-dlp path is a program the server executes; accepting it over the API
// would turn an admin session into arbitrary code execution on the host.
func TestSettingsCannotChangeExecutablePath(t *testing.T) {
	app := testApp(t, false)
	session := adminSession(t, app)
	before := app.cfg.YTDLPBinary

	rec := patchSettings(t, app, session, `{"yt_dlp_binary":"/tmp/evil.sh","read_only":{"yt_dlp_binary":"/tmp/evil.sh"}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if app.cfg.YTDLPBinary != before {
		t.Fatalf("yt-dlp path changed via API: %q -> %q", before, app.cfg.YTDLPBinary)
	}
}

// A blank secret means "keep the current one" — otherwise rendering an empty
// password field and saving any other setting would silently wipe a working key.
func TestSettingsBlankSecretKeepsStoredValue(t *testing.T) {
	app := testApp(t, false)
	session := adminSession(t, app)

	if rec := patchSettings(t, app, session, `{"youtube_api_key":"real-key"}`); rec.Code != http.StatusOK {
		t.Fatalf("set key: %d %s", rec.Code, rec.Body.String())
	}
	if app.cfg.YouTubeAPIKey != "real-key" {
		t.Fatalf("key not applied, got %q", app.cfg.YouTubeAPIKey)
	}
	if rec := patchSettings(t, app, session, `{"youtube_api_key":"  "}`); rec.Code != http.StatusOK {
		t.Fatalf("blank patch: %d %s", rec.Code, rec.Body.String())
	}
	if app.cfg.YouTubeAPIKey != "real-key" {
		t.Fatalf("blank value wiped the stored secret, got %q", app.cfg.YouTubeAPIKey)
	}
}

func TestSettingsRejectOutOfRangeTimeouts(t *testing.T) {
	app := testApp(t, false)
	session := adminSession(t, app)
	for _, body := range []string{
		`{"extractor_timeout_seconds":1}`,
		`{"extractor_timeout_seconds":9999}`,
		`{"download_timeout_seconds":1}`,
		`{"session_ttl_hours":0}`,
	} {
		if rec := patchSettings(t, app, session, body); rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for %s, got %d", body, rec.Code)
		}
	}
}

// Toggling extractors must reach the provider service, which holds its own
// copy of the config — a mutation that only updated App.cfg would leave
// /v1/providers reporting the old state.
func TestSettingsToggleReachesProviders(t *testing.T) {
	app := testApp(t, false)
	session := adminSession(t, app)

	if rec := patchSettings(t, app, session, `{"enable_risky_extractors":true}`); rec.Code != http.StatusOK {
		t.Fatalf("enable: %d %s", rec.Code, rec.Body.String())
	}
	req := httptest.NewRequest(http.MethodGet, "/v1/providers", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	var providers []Provider
	if err := json.Unmarshal(rec.Body.Bytes(), &providers); err != nil {
		t.Fatalf("decode providers: %v", err)
	}
	for _, p := range providers {
		if p.ID == "youtube_stream" && !p.Enabled {
			t.Fatal("provider service still reports youtube_stream disabled after enabling extractors")
		}
	}
}

func TestSettingsRequireAdmin(t *testing.T) {
	app := testApp(t, false)
	req := httptest.NewRequest(http.MethodGet, "/v1/settings", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without credentials, got %d", rec.Code)
	}
}
