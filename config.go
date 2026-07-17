package main

import (
	"bufio"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Addr                  string
	Environment           string
	APIKeys               map[string]bool
	CORSOrigins           []string
	StorePath             string
	MediaRoot             string
	PublicMediaBaseURL    string
	EnableRiskyExtractors bool
	YTDLPBinary           string
	ExtractorTimeout      time.Duration
	DownloadTimeout       time.Duration
	YouTubeAPIKey         string
	SoundCloudClientID    string
	NavidromeBaseURL      string
	NavidromeUsername     string
	NavidromeToken        string
}

func LoadConfig() Config {
	loadDotEnv(".env")
	cfg := Config{
		Addr:                  env("APP_ADDR", ":8080"),
		Environment:           env("APP_ENVIRONMENT", "local"),
		StorePath:             env("APP_STORE_PATH", "./data/store.json"),
		MediaRoot:             env("APP_MEDIA_ROOT", "./data/media"),
		PublicMediaBaseURL:    strings.TrimRight(env("APP_PUBLIC_MEDIA_BASE_URL", ""), "/"),
		EnableRiskyExtractors: envBool("APP_ENABLE_RISKY_EXTRACTORS", false),
		YTDLPBinary:           env("APP_YT_DLP_BINARY", "yt-dlp"),
		ExtractorTimeout:      time.Duration(envInt("APP_EXTRACTOR_TIMEOUT_SECONDS", 30)) * time.Second,
		DownloadTimeout:       time.Duration(envInt("APP_DOWNLOAD_TIMEOUT_SECONDS", 600)) * time.Second,
		YouTubeAPIKey:         env("APP_YOUTUBE_API_KEY", ""),
		SoundCloudClientID:    env("APP_SOUNDCLOUD_CLIENT_ID", ""),
		NavidromeBaseURL:      env("APP_NAVIDROME_BASE_URL", ""),
		NavidromeUsername:     env("APP_NAVIDROME_USERNAME", ""),
		NavidromeToken:        env("APP_NAVIDROME_TOKEN", ""),
	}
	cfg.APIKeys = map[string]bool{}
	for _, key := range strings.Split(env("APP_API_KEYS", "change-me-local-dev-key"), ",") {
		key = strings.TrimSpace(key)
		if key != "" {
			cfg.APIKeys[key] = true
		}
	}
	cfg.CORSOrigins = splitCSV(env("APP_CORS_ORIGINS", "http://localhost:5173,http://localhost:3000"))
	if len(cfg.APIKeys) == 0 {
		slog.Warn("APP_API_KEYS is empty; protected endpoints will reject all requests")
	}
	return cfg
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.Trim(strings.TrimSpace(v), "\"'")
		if k != "" && os.Getenv(k) == "" {
			_ = os.Setenv(k, v)
		}
	}
}
func envBool(k string, def bool) bool {
	if v := os.Getenv(k); v != "" {
		b, err := strconv.ParseBool(v)
		if err == nil {
			return b
		}
	}
	return def
}
func envInt(k string, def int) int {
	if v := os.Getenv(k); v != "" {
		i, err := strconv.Atoi(v)
		if err == nil {
			return i
		}
	}
	return def
}
func splitCSV(v string) []string {
	out := []string{}
	for _, p := range strings.Split(v, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
