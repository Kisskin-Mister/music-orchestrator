package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	sessionCookieName  = "mo_session"
	authTypeContextKey = contextKey("auth_type")
)

// authSession is an in-memory owner session. authenticated=false marks a
// pending session that passed the password step but still needs TOTP.
type authSession struct {
	userID        string
	authenticated bool
	expiresAt     time.Time
}

type sessionStore struct {
	mu       sync.Mutex
	sessions map[string]authSession
}

func newSessionStore() *sessionStore { return &sessionStore{sessions: map[string]authSession{}} }

func (s *sessionStore) create(userID string, authenticated bool, ttl time.Duration) string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	token := hex.EncodeToString(b[:])
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[token] = authSession{userID: userID, authenticated: authenticated, expiresAt: time.Now().Add(ttl)}
	return token
}

func (s *sessionStore) get(token string) (authSession, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[token]
	if !ok {
		return authSession{}, false
	}
	if time.Now().After(sess.expiresAt) {
		delete(s.sessions, token)
		return authSession{}, false
	}
	return sess, true
}

func (s *sessionStore) authenticate(token string, ttl time.Duration) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[token]
	if !ok || time.Now().After(sess.expiresAt) {
		return false
	}
	sess.authenticated = true
	sess.expiresAt = time.Now().Add(ttl)
	s.sessions[token] = sess
	return true
}

func (s *sessionStore) delete(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, token)
}

// totpCodeAt implements RFC 6238 (SHA-1, 30s step, 6 digits) using stdlib only.
func totpCodeAt(secret string, step int64) (string, error) {
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(strings.TrimSpace(secret)))
	if err != nil {
		return "", fmt.Errorf("invalid TOTP secret encoding: %w", err)
	}
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], uint64(step))
	mac := hmac.New(sha1.New, key)
	_, _ = mac.Write(buf[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	code := (binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff) % 1_000_000
	return fmt.Sprintf("%06d", code), nil
}

func totpValid(secret, code string, now time.Time) bool {
	code = strings.TrimSpace(code)
	if len(code) != 6 {
		return false
	}
	step := now.Unix() / 30
	for _, drift := range []int64{0, -1, 1} {
		expected, err := totpCodeAt(secret, step+drift)
		if err != nil {
			return false
		}
		if subtle.ConstantTimeCompare([]byte(expected), []byte(code)) == 1 {
			return true
		}
	}
	return false
}

func normalizeTOTPSecret(secret string) string {
	return strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(secret), " ", ""))
}

func validTOTPSecret(secret string) bool {
	if secret == "" {
		return true
	}
	_, err := totpCodeAt(secret, 0)
	return err == nil
}

func (a *App) sessionTTL() time.Duration {
	hours := a.cfg.SessionTTLHours
	if hours < 1 {
		hours = 72
	}
	return time.Duration(hours) * time.Hour
}

func (a *App) setSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   a.cfg.SecureCookies,
		MaxAge:   int(a.sessionTTL().Seconds()),
	})
}

func (a *App) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   a.cfg.SecureCookies,
		MaxAge:   -1,
	})
}

func (a *App) sessionFromRequest(r *http.Request) (string, authSession, bool) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || cookie.Value == "" {
		return "", authSession{}, false
	}
	sess, ok := a.sessions.get(cookie.Value)
	return cookie.Value, sess, ok
}

// currentIdentity resolves the caller either by API key or by session cookie.
func (a *App) currentIdentity(r *http.Request) (userID, authType string, ok bool) {
	if key := r.Header.Get("X-API-Key"); key != "" && a.cfg.APIKeys[key] {
		return apiKeyUserID(key), "api_key", true
	}
	if _, sess, found := a.sessionFromRequest(r); found && sess.authenticated {
		return sess.userID, "session", true
	}
	return "", "", false
}

type sessionInfoResponse struct {
	Authenticated bool   `json:"authenticated"`
	UserID        string `json:"user_id,omitempty"`
	Username      string `json:"username,omitempty"`
	AuthType      string `json:"auth_type,omitempty"`
	Role          string `json:"role,omitempty"`
	SetupRequired bool   `json:"setup_required"`
	TOTPRequired  bool   `json:"totp_required"`
	TOTPEnabled   bool   `json:"totp_enabled"`
	LoginEnabled  bool   `json:"login_enabled"`
}

func (a *App) sessionInfo(r *http.Request) sessionInfoResponse {
	owner, hasOwner := a.store.Owner()
	resp := sessionInfoResponse{SetupRequired: !hasOwner, LoginEnabled: hasOwner}
	if hasOwner {
		resp.TOTPEnabled = owner.TOTPSecret != ""
	}
	if userID, authType, ok := a.currentIdentity(r); ok {
		resp.Authenticated = true
		resp.UserID = userID
		resp.AuthType = authType
		if username, role, _, _, found := a.store.AccountByID(userID); found {
			resp.Username = username
			resp.Role = role
		} else if hasOwner && userID == owner.ID {
			resp.Username = owner.Username
			resp.Role = "admin"
		}
		return resp
	}
	if _, sess, found := a.sessionFromRequest(r); found && !sess.authenticated {
		resp.TOTPRequired = true
		if hasOwner && sess.userID == owner.ID {
			resp.Username = owner.Username
		}
	}
	return resp
}

func (a *App) authSessionInfo(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, a.sessionInfo(r))
}

func (a *App) authRegister(w http.ResponseWriter, r *http.Request) {
	if _, exists := a.store.Owner(); exists {
		writeErrorCode(w, http.StatusConflict, "setup_already_completed", "Owner account already exists")
		return
	}
	var req struct {
		Username   string `json:"username"`
		Password   string `json:"password"`
		TOTPSecret string `json:"totp_secret"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	username := strings.TrimSpace(req.Username)
	if username == "" {
		writeError(w, http.StatusBadRequest, "Username is required")
		return
	}
	if len(req.Password) < passwordMinLength {
		writeError(w, http.StatusBadRequest, "Password must be at least 10 characters")
		return
	}
	totpSecret := normalizeTOTPSecret(req.TOTPSecret)
	if !validTOTPSecret(totpSecret) {
		writeErrorCode(w, http.StatusBadRequest, "invalid_totp_secret", "TOTP secret must be valid base32")
		return
	}
	passwordHash, err := hashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to hash password")
		return
	}
	owner := Owner{ID: newID("owner"), Username: username, PasswordHash: passwordHash, TOTPSecret: totpSecret, CreatedAt: time.Now().UTC()}
	created, err := a.store.CreateOwner(owner)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to save owner")
		return
	}
	if !created {
		writeErrorCode(w, http.StatusConflict, "setup_already_completed", "Owner account already exists")
		return
	}
	a.setSessionCookie(w, a.sessions.create(owner.ID, true, a.sessionTTL()))
	writeJSON(w, http.StatusOK, sessionInfoResponse{Authenticated: true, UserID: owner.ID, Username: owner.Username, AuthType: "session", Role: "admin", LoginEnabled: true, TOTPEnabled: owner.TOTPSecret != ""})
}

func (a *App) authLogin(w http.ResponseWriter, r *http.Request) {
	if _, hasOwner := a.store.Owner(); !hasOwner {
		writeErrorCode(w, http.StatusForbidden, "setup_required", "Owner account is not registered yet")
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	id, username, role, passwordHash, totpSecret, ok := a.store.AccountByUsername(req.Username)
	if !ok || !verifyPassword(req.Password, passwordHash) {
		writeError(w, http.StatusUnauthorized, "Invalid username or password")
		return
	}
	if totpSecret != "" {
		a.setSessionCookie(w, a.sessions.create(id, false, a.sessionTTL()))
		writeJSON(w, http.StatusOK, sessionInfoResponse{Username: username, Role: role, LoginEnabled: true, TOTPRequired: true, TOTPEnabled: true})
		return
	}
	a.setSessionCookie(w, a.sessions.create(id, true, a.sessionTTL()))
	writeJSON(w, http.StatusOK, sessionInfoResponse{Authenticated: true, UserID: id, Username: username, AuthType: "session", Role: role, LoginEnabled: true, TOTPEnabled: totpSecret != ""})
}

func (a *App) authVerify(w http.ResponseWriter, r *http.Request) {
	owner, hasOwner := a.store.Owner()
	if !hasOwner || owner.TOTPSecret == "" {
		writeError(w, http.StatusUnauthorized, "No TOTP challenge is active")
		return
	}
	token, sess, ok := a.sessionFromRequest(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "No pending login session")
		return
	}
	if sess.authenticated {
		writeJSON(w, http.StatusOK, sessionInfoResponse{Authenticated: true, UserID: sess.userID, Username: owner.Username, AuthType: "session", Role: "admin", LoginEnabled: true, TOTPEnabled: true})
		return
	}
	if sess.userID != owner.ID {
		writeError(w, http.StatusUnauthorized, "Invalid login session")
		return
	}
	var req struct {
		Code string `json:"code"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	if !totpValid(owner.TOTPSecret, req.Code, time.Now()) {
		writeError(w, http.StatusUnauthorized, "Invalid verification code")
		return
	}
	if !a.sessions.authenticate(token, a.sessionTTL()) {
		writeError(w, http.StatusUnauthorized, "Login session expired, sign in again")
		return
	}
	writeJSON(w, http.StatusOK, sessionInfoResponse{Authenticated: true, UserID: sess.userID, Username: owner.Username, AuthType: "session", Role: "admin", LoginEnabled: true, TOTPEnabled: true})
}

func (a *App) authLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		a.sessions.delete(cookie.Value)
	}
	a.clearSessionCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func authTypeFromRequest(r *http.Request) string {
	if authType, ok := r.Context().Value(authTypeContextKey).(string); ok {
		return authType
	}
	return ""
}
