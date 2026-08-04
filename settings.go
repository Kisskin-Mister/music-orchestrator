package main

import (
	"net/http"
	"strings"
	"time"
)

// Runtime settings let an admin change server behaviour from the UI instead of
// editing .env and restarting.
//
// Three deliberate boundaries:
//
//  1. APP_YT_DLP_BINARY is NOT editable here. It is the path of a program the
//     server executes, so exposing it would turn "admin of the web UI" into
//     "run any binary on the host". It stays an env-only, deploy-time decision.
//  2. Values that are read once at boot (listen address, store path, media
//     root, web root) are reported read-only, because changing them without a
//     restart would leave the process describing a state it is not in.
//  3. Secrets are never sent back to the client — only whether they are set.
//     Sending a blank value leaves the stored secret untouched.
//
// Overrides live in the store rather than being written back into .env: the
// file is the operator's deploy-time input, and having the process rewrite it
// makes the two sources of truth fight.
type Settings struct {
	EnableRiskyExtractors   *bool     `json:"enable_risky_extractors,omitempty"`
	ExtractorTimeoutSeconds *int      `json:"extractor_timeout_seconds,omitempty"`
	DownloadTimeoutSeconds  *int      `json:"download_timeout_seconds,omitempty"`
	SessionTTLHours         *int      `json:"session_ttl_hours,omitempty"`
	SecureCookies           *bool     `json:"secure_cookies,omitempty"`
	PublicMediaBaseURL      *string   `json:"public_media_base_url,omitempty"`
	CORSOrigins             *[]string `json:"cors_origins,omitempty"`
	YouTubeAPIKey           *string   `json:"youtube_api_key,omitempty"`
	SoundCloudClientID      *string   `json:"soundcloud_client_id,omitempty"`
	NavidromeBaseURL        *string   `json:"navidrome_base_url,omitempty"`
	NavidromeUsername       *string   `json:"navidrome_username,omitempty"`
	NavidromeToken          *string   `json:"navidrome_token,omitempty"`
}

// apply overlays stored overrides on top of the env-derived config.
func (s Settings) apply(cfg Config) Config {
	if s.EnableRiskyExtractors != nil {
		cfg.EnableRiskyExtractors = *s.EnableRiskyExtractors
	}
	if s.ExtractorTimeoutSeconds != nil && *s.ExtractorTimeoutSeconds > 0 {
		cfg.ExtractorTimeout = time.Duration(*s.ExtractorTimeoutSeconds) * time.Second
	}
	if s.DownloadTimeoutSeconds != nil && *s.DownloadTimeoutSeconds > 0 {
		cfg.DownloadTimeout = time.Duration(*s.DownloadTimeoutSeconds) * time.Second
	}
	if s.SessionTTLHours != nil && *s.SessionTTLHours > 0 {
		cfg.SessionTTLHours = *s.SessionTTLHours
	}
	if s.SecureCookies != nil {
		cfg.SecureCookies = *s.SecureCookies
	}
	if s.PublicMediaBaseURL != nil {
		cfg.PublicMediaBaseURL = strings.TrimRight(*s.PublicMediaBaseURL, "/")
	}
	if s.CORSOrigins != nil {
		cfg.CORSOrigins = *s.CORSOrigins
	}
	if s.YouTubeAPIKey != nil {
		cfg.YouTubeAPIKey = *s.YouTubeAPIKey
	}
	if s.SoundCloudClientID != nil {
		cfg.SoundCloudClientID = *s.SoundCloudClientID
	}
	if s.NavidromeBaseURL != nil {
		cfg.NavidromeBaseURL = *s.NavidromeBaseURL
	}
	if s.NavidromeUsername != nil {
		cfg.NavidromeUsername = *s.NavidromeUsername
	}
	if s.NavidromeToken != nil {
		cfg.NavidromeToken = *s.NavidromeToken
	}
	return cfg
}

// settingsView is what the UI renders: current effective values, plus enough
// context to explain why some of them cannot be edited here.
type settingsView struct {
	EnableRiskyExtractors   bool     `json:"enable_risky_extractors"`
	ExtractorTimeoutSeconds int      `json:"extractor_timeout_seconds"`
	DownloadTimeoutSeconds  int      `json:"download_timeout_seconds"`
	SessionTTLHours         int      `json:"session_ttl_hours"`
	SecureCookies           bool     `json:"secure_cookies"`
	PublicMediaBaseURL      string   `json:"public_media_base_url"`
	CORSOrigins             []string `json:"cors_origins"`

	// Secrets are reported as booleans only.
	YouTubeAPIKeySet      bool `json:"youtube_api_key_set"`
	SoundCloudClientIDSet bool `json:"soundcloud_client_id_set"`
	NavidromeTokenSet     bool `json:"navidrome_token_set"`

	NavidromeBaseURL  string `json:"navidrome_base_url"`
	NavidromeUsername string `json:"navidrome_username"`

	// Read-only: fixed at boot or deliberately env-only.
	ReadOnly readOnlySettings `json:"read_only"`
}

type readOnlySettings struct {
	Addr        string `json:"addr"`
	Environment string `json:"environment"`
	StorePath   string `json:"store_path"`
	MediaRoot   string `json:"media_root"`
	WebRoot     string `json:"web_root"`
	YTDLPBinary string `json:"yt_dlp_binary"`
	Reason      string `json:"reason"`
}

func (a *App) getSettings(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdmin(w, r) {
		return
	}
	writeJSON(w, http.StatusOK, a.settingsView())
}

func (a *App) settingsView() settingsView {
	return settingsView{
		EnableRiskyExtractors:   a.cfg.EnableRiskyExtractors,
		ExtractorTimeoutSeconds: int(a.cfg.ExtractorTimeout / time.Second),
		DownloadTimeoutSeconds:  int(a.cfg.DownloadTimeout / time.Second),
		SessionTTLHours:         a.cfg.SessionTTLHours,
		SecureCookies:           a.cfg.SecureCookies,
		PublicMediaBaseURL:      a.cfg.PublicMediaBaseURL,
		CORSOrigins:             a.cfg.CORSOrigins,
		YouTubeAPIKeySet:        a.cfg.YouTubeAPIKey != "",
		SoundCloudClientIDSet:   a.cfg.SoundCloudClientID != "",
		NavidromeTokenSet:       a.cfg.NavidromeToken != "",
		NavidromeBaseURL:        a.cfg.NavidromeBaseURL,
		NavidromeUsername:       a.cfg.NavidromeUsername,
		ReadOnly: readOnlySettings{
			Addr:        a.cfg.Addr,
			Environment: a.cfg.Environment,
			StorePath:   a.cfg.StorePath,
			MediaRoot:   a.cfg.MediaRoot,
			WebRoot:     a.cfg.WebRoot,
			YTDLPBinary: a.cfg.YTDLPBinary,
			Reason:      "Заданы при старте через .env. Путь к yt-dlp намеренно нельзя менять из интерфейса: сервер запускает этот файл как программу.",
		},
	}
}

func (a *App) updateSettings(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdmin(w, r) {
		return
	}
	var req Settings
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	// A blank secret means "leave as is" rather than "clear it", so the UI can
	// render an empty password field without wiping a working key.
	dropBlank := func(p **string) {
		if *p != nil && strings.TrimSpace(**p) == "" {
			*p = nil
		}
	}
	dropBlank(&req.YouTubeAPIKey)
	dropBlank(&req.SoundCloudClientID)
	dropBlank(&req.NavidromeToken)

	if req.ExtractorTimeoutSeconds != nil && (*req.ExtractorTimeoutSeconds < 5 || *req.ExtractorTimeoutSeconds > 600) {
		writeError(w, http.StatusBadRequest, "Таймаут поиска должен быть от 5 до 600 секунд")
		return
	}
	if req.DownloadTimeoutSeconds != nil && (*req.DownloadTimeoutSeconds < 30 || *req.DownloadTimeoutSeconds > 7200) {
		writeError(w, http.StatusBadRequest, "Таймаут загрузки должен быть от 30 до 7200 секунд")
		return
	}
	if req.SessionTTLHours != nil && (*req.SessionTTLHours < 1 || *req.SessionTTLHours > 8760) {
		writeError(w, http.StatusBadRequest, "Время жизни сессии должно быть от 1 до 8760 часов")
		return
	}

	merged, err := a.store.MergeSettings(req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось сохранить настройки")
		return
	}
	// Config is copied by value into the provider service and extractor, so
	// they are rebuilt rather than mutated — otherwise a toggled setting would
	// apply to some code paths and not others.
	a.cfg = merged.apply(a.baseCfg)
	a.providers = NewProviderService(a.cfg)
	writeJSON(w, http.StatusOK, a.settingsView())
}
