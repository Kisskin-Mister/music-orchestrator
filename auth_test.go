package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// RFC 6238 well-known test secret ("12345678901234567890" in base32).
const testTOTPSecret = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"

const testOwnerPassword = "correct horse battery"

func testAuthAppAt(t *testing.T, storePath string) *App {
	t.Helper()
	dir := t.TempDir()
	cfg := Config{
		Addr:                  ":0",
		Environment:           "test",
		APIKeys:               map[string]bool{"test-key": true},
		CORSOrigins:           []string{"*"},
		StorePath:             storePath,
		MediaRoot:             filepath.Join(dir, "media"),
		SessionTTLHours:       1,
		ExtractorTimeout:      5_000_000_000,
		DownloadTimeout:       5_000_000_000,
		YTDLPBinary:           mockYTDLP(t),
		EnableRiskyExtractors: false,
	}
	app, err := NewApp(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return app
}

func testAuthApp(t *testing.T) *App {
	t.Helper()
	return testAuthAppAt(t, filepath.Join(t.TempDir(), "store.json"))
}

func doJSON(t *testing.T, app *App, method, path, body string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	r := httptest.NewRecorder()
	app.ServeHTTP(r, req)
	return r
}

func sessionCookieFrom(t *testing.T, r *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, c := range r.Result().Cookies() {
		if c.Name == sessionCookieName && c.Value != "" {
			return c
		}
	}
	t.Fatalf("no %s cookie set", sessionCookieName)
	return nil
}

func currentTOTPCode(t *testing.T, secret string) string {
	t.Helper()
	code, err := totpCodeAt(secret, time.Now().Unix()/30)
	if err != nil {
		t.Fatal(err)
	}
	return code
}

func readSessionInfo(t *testing.T, r *httptest.ResponseRecorder) sessionInfoResponse {
	t.Helper()
	var info sessionInfoResponse
	if err := json.Unmarshal(r.Body.Bytes(), &info); err != nil {
		t.Fatal(err)
	}
	return info
}

func readErrorCode(t *testing.T, r *httptest.ResponseRecorder) string {
	t.Helper()
	var body ErrorBody
	if err := json.Unmarshal(r.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body.Error.Code
}

func registerOwner(t *testing.T, app *App, username, password, totpSecret string) *httptest.ResponseRecorder {
	t.Helper()
	body := `{"username":` + strconvQuote(username) + `,"password":` + strconvQuote(password) + `,"totp_secret":` + strconvQuote(totpSecret) + `}`
	return doJSON(t, app, "POST", "/v1/auth/register", body)
}

func strconvQuote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func TestFirstRunRequiresSetup(t *testing.T) {
	app := testAuthApp(t)

	r := doJSON(t, app, "GET", "/v1/auth/session", "")
	if r.Code != 200 {
		t.Fatalf("session %d", r.Code)
	}
	info := readSessionInfo(t, r)
	if info.Authenticated || !info.SetupRequired || info.LoginEnabled || info.TOTPRequired || info.TOTPEnabled {
		t.Fatalf("unexpected first-run session info: %#v", info)
	}

	// Protected endpoints must stay locked until setup + login.
	r = doJSON(t, app, "GET", "/v1/favorites", "")
	if r.Code != http.StatusUnauthorized {
		t.Fatalf("protected endpoint before setup must be 401, got %d", r.Code)
	}

	// X-API-Key fallback keeps working when a key is configured.
	req := authReq("GET", "/v1/favorites", "", "test-key")
	r = httptest.NewRecorder()
	app.ServeHTTP(r, req)
	if r.Code != 200 {
		t.Fatalf("api key fallback broken: %d", r.Code)
	}

	// Login before setup points the UI to registration.
	r = doJSON(t, app, "POST", "/v1/auth/login", `{"username":"admin","password":"whatever"}`)
	if r.Code != http.StatusForbidden {
		t.Fatalf("login before setup must be 403, got %d: %s", r.Code, r.Body.String())
	}
	if code := readErrorCode(t, r); code != "setup_required" {
		t.Fatalf("login before setup must return setup_required, got %q", code)
	}
}

func TestRegisterValidation(t *testing.T) {
	app := testAuthApp(t)

	r := registerOwner(t, app, "", testOwnerPassword, "")
	if r.Code != http.StatusBadRequest {
		t.Fatalf("empty username must be 400, got %d", r.Code)
	}

	r = registerOwner(t, app, "owner", "short", "")
	if r.Code != http.StatusBadRequest {
		t.Fatalf("short password must be 400, got %d", r.Code)
	}

	r = registerOwner(t, app, "owner", testOwnerPassword, "not base32 !!!")
	if r.Code != http.StatusBadRequest {
		t.Fatalf("invalid totp secret must be 400, got %d", r.Code)
	}

	r = doJSON(t, app, "POST", "/v1/auth/register", `{invalid`)
	if r.Code != http.StatusBadRequest {
		t.Fatalf("invalid JSON must be 400, got %d", r.Code)
	}
}

func TestRegisterThenLoginWithoutTOTP(t *testing.T) {
	app := testAuthApp(t)

	r := registerOwner(t, app, "nazar", testOwnerPassword, "")
	if r.Code != 200 {
		t.Fatalf("register %d: %s", r.Code, r.Body.String())
	}
	info := readSessionInfo(t, r)
	if !info.Authenticated || info.SetupRequired || !info.LoginEnabled || info.TOTPEnabled || info.Username != "nazar" || info.UserID == "" {
		t.Fatalf("unexpected register response: %#v", info)
	}
	cookie := sessionCookieFrom(t, r)
	if !cookie.HttpOnly {
		t.Fatal("session cookie must be HttpOnly")
	}

	// Registration is one-shot.
	r = registerOwner(t, app, "second", testOwnerPassword, "")
	if r.Code != http.StatusConflict {
		t.Fatalf("second register must be 409, got %d", r.Code)
	}
	if code := readErrorCode(t, r); code != "setup_already_completed" {
		t.Fatalf("second register must return setup_already_completed, got %q", code)
	}

	// Session info now reflects a completed setup.
	r = doJSON(t, app, "GET", "/v1/auth/session", "", cookie)
	info = readSessionInfo(t, r)
	if !info.Authenticated || info.SetupRequired || !info.LoginEnabled || info.Username != "nazar" {
		t.Fatalf("session info after register: %#v", info)
	}

	// Password must not be stored in plaintext.
	st, err := NewStore(app.cfg.StorePath)
	if err != nil {
		t.Fatal(err)
	}
	owner, ok := st.Owner()
	if !ok {
		t.Fatal("owner must be persisted")
	}
	if strings.Contains(owner.PasswordHash, testOwnerPassword) || owner.PasswordHash == "" {
		t.Fatalf("password stored unsafely: %q", owner.PasswordHash)
	}
	if !strings.HasPrefix(owner.PasswordHash, "pbkdf2-sha256$") {
		t.Fatalf("unexpected hash format: %q", owner.PasswordHash)
	}

	r = doJSON(t, app, "POST", "/v1/auth/logout", "", cookie)
	if r.Code != http.StatusNoContent {
		t.Fatalf("logout %d", r.Code)
	}

	r = doJSON(t, app, "POST", "/v1/auth/login", `{"username":"nazar","password":"wrong password"}`)
	if r.Code != http.StatusUnauthorized {
		t.Fatalf("wrong password must be 401, got %d", r.Code)
	}

	r = doJSON(t, app, "POST", "/v1/auth/login", `{"username":"nazar","password":"`+testOwnerPassword+`"}`)
	if r.Code != 200 {
		t.Fatalf("login %d: %s", r.Code, r.Body.String())
	}
	info = readSessionInfo(t, r)
	if !info.Authenticated || info.TOTPRequired || info.AuthType != "session" || info.Username != "nazar" {
		t.Fatalf("unexpected login response: %#v", info)
	}
	cookie = sessionCookieFrom(t, r)

	r = doJSON(t, app, "GET", "/v1/favorites", "", cookie)
	if r.Code != 200 {
		t.Fatalf("protected endpoint with session cookie got %d: %s", r.Code, r.Body.String())
	}

	r = doJSON(t, app, "POST", "/v1/auth/logout", "", cookie)
	if r.Code != http.StatusNoContent {
		t.Fatalf("logout %d", r.Code)
	}
	r = doJSON(t, app, "GET", "/v1/favorites", "", cookie)
	if r.Code != http.StatusUnauthorized {
		t.Fatalf("protected endpoint after logout must be 401, got %d", r.Code)
	}
}

func TestRegisterWithTOTPThenLoginRequiresVerify(t *testing.T) {
	app := testAuthApp(t)

	// Registration with TOTP signs the owner in immediately.
	r := registerOwner(t, app, "nazar", testOwnerPassword, testTOTPSecret)
	if r.Code != 200 {
		t.Fatalf("register %d: %s", r.Code, r.Body.String())
	}
	info := readSessionInfo(t, r)
	if !info.Authenticated || !info.TOTPEnabled {
		t.Fatalf("register with totp: %#v", info)
	}
	cookie := sessionCookieFrom(t, r)
	r = doJSON(t, app, "POST", "/v1/auth/logout", "", cookie)
	if r.Code != http.StatusNoContent {
		t.Fatalf("logout %d", r.Code)
	}

	// Subsequent logins stop at the TOTP step.
	r = doJSON(t, app, "POST", "/v1/auth/login", `{"username":"nazar","password":"`+testOwnerPassword+`"}`)
	if r.Code != 200 {
		t.Fatalf("login %d: %s", r.Code, r.Body.String())
	}
	info = readSessionInfo(t, r)
	if info.Authenticated || !info.TOTPRequired || !info.TOTPEnabled {
		t.Fatalf("expected totp_required pending session: %#v", info)
	}
	cookie = sessionCookieFrom(t, r)

	// Pending session must not unlock protected endpoints.
	r = doJSON(t, app, "GET", "/v1/favorites", "", cookie)
	if r.Code != http.StatusUnauthorized {
		t.Fatalf("pending 2FA session must stay 401, got %d", r.Code)
	}
	r = doJSON(t, app, "GET", "/v1/auth/session", "", cookie)
	info = readSessionInfo(t, r)
	if info.Authenticated || !info.TOTPRequired {
		t.Fatalf("pending session info wrong: %#v", info)
	}

	r = doJSON(t, app, "POST", "/v1/auth/verify", `{"code":"000000"}`, cookie)
	if r.Code != http.StatusUnauthorized {
		t.Fatalf("wrong TOTP code must be 401, got %d", r.Code)
	}

	r = doJSON(t, app, "POST", "/v1/auth/verify", `{"code":"`+currentTOTPCode(t, testTOTPSecret)+`"}`, cookie)
	if r.Code != 200 {
		t.Fatalf("verify %d: %s", r.Code, r.Body.String())
	}
	info = readSessionInfo(t, r)
	if !info.Authenticated || info.AuthType != "session" {
		t.Fatalf("verify should authenticate session: %#v", info)
	}

	r = doJSON(t, app, "GET", "/v1/favorites", "", cookie)
	if r.Code != 200 {
		t.Fatalf("protected endpoint after 2FA got %d: %s", r.Code, r.Body.String())
	}

	// Verify without any pending session must fail.
	r = doJSON(t, app, "POST", "/v1/auth/verify", `{"code":"123456"}`)
	if r.Code != http.StatusUnauthorized {
		t.Fatalf("verify without session must be 401, got %d", r.Code)
	}
}

func TestOwnerPersistsAcrossRestart(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "store.json")
	app := testAuthAppAt(t, storePath)

	r := registerOwner(t, app, "nazar", testOwnerPassword, testTOTPSecret)
	if r.Code != 200 {
		t.Fatalf("register %d: %s", r.Code, r.Body.String())
	}

	// Simulate a restart: new App over the same store file.
	restarted := testAuthAppAt(t, storePath)
	r = doJSON(t, restarted, "GET", "/v1/auth/session", "")
	info := readSessionInfo(t, r)
	if info.SetupRequired || !info.LoginEnabled || !info.TOTPEnabled {
		t.Fatalf("session info after restart: %#v", info)
	}

	r = registerOwner(t, restarted, "nazar", testOwnerPassword, "")
	if r.Code != http.StatusConflict {
		t.Fatalf("register after restart must be 409, got %d", r.Code)
	}

	r = doJSON(t, restarted, "POST", "/v1/auth/login", `{"username":"nazar","password":"`+testOwnerPassword+`"}`)
	if r.Code != 200 {
		t.Fatalf("login after restart %d: %s", r.Code, r.Body.String())
	}
	info = readSessionInfo(t, r)
	if info.Authenticated || !info.TOTPRequired {
		t.Fatalf("login after restart must require TOTP: %#v", info)
	}
	cookie := sessionCookieFrom(t, r)
	r = doJSON(t, restarted, "POST", "/v1/auth/verify", `{"code":"`+currentTOTPCode(t, testTOTPSecret)+`"}`, cookie)
	if r.Code != 200 {
		t.Fatalf("verify after restart %d: %s", r.Code, r.Body.String())
	}
}

func TestPasswordHashRoundTrip(t *testing.T) {
	encoded, err := hashPassword(testOwnerPassword)
	if err != nil {
		t.Fatal(err)
	}
	if !verifyPassword(testOwnerPassword, encoded) {
		t.Fatal("correct password must verify")
	}
	if verifyPassword("wrong password", encoded) {
		t.Fatal("wrong password must not verify")
	}
	other, err := hashPassword(testOwnerPassword)
	if err != nil {
		t.Fatal(err)
	}
	if other == encoded {
		t.Fatal("salts must differ between hashes")
	}
	if verifyPassword(testOwnerPassword, "garbage") {
		t.Fatal("malformed hash must not verify")
	}
}

func TestTOTPKnownVector(t *testing.T) {
	// RFC 6238 test vector for SHA-1, T=59s, 8-digit is 94287082; 6-digit truncation is 287082.
	code, err := totpCodeAt(testTOTPSecret, 59/30)
	if err != nil {
		t.Fatal(err)
	}
	if code != "287082" {
		t.Fatalf("TOTP vector mismatch, got %s", code)
	}
}
