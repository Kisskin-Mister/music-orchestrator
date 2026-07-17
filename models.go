package main

import "time"

type Capabilities struct {
	Search               bool `json:"search"`
	Metadata             bool `json:"metadata"`
	EmbedPlayback        bool `json:"embed_playback"`
	OfficialStreamURL    bool `json:"official_stream_url"`
	RawAudioStream       bool `json:"raw_audio_stream"`
	PersistentCache      bool `json:"persistent_cache"`
	OfflinePlayback      bool `json:"offline_playback"`
	PublicDeploymentSafe bool `json:"public_deployment_safe"`
}

type Policy struct {
	DownloadAllowed     bool     `json:"download_allowed"`
	CacheAllowed        bool     `json:"cache_allowed"`
	RequiresAttribution bool     `json:"requires_attribution"`
	Notes               []string `json:"notes"`
}

type Provider struct {
	ID           string       `json:"id"`
	Name         string       `json:"name"`
	Kind         string       `json:"kind"`
	Configured   bool         `json:"configured"`
	Enabled      bool         `json:"enabled"`
	RiskLevel    string       `json:"risk_level"`
	Capabilities Capabilities `json:"capabilities"`
	Policy       Policy       `json:"policy"`
}

type Track struct {
	ID              string       `json:"id"`
	ProviderID      string       `json:"provider_id"`
	ProviderTrackID string       `json:"provider_track_id"`
	Title           string       `json:"title"`
	Artist          string       `json:"artist,omitempty"`
	Album           string       `json:"album,omitempty"`
	DurationSeconds int          `json:"duration_seconds,omitempty"`
	ArtworkURL      string       `json:"artwork_url,omitempty"`
	SourceURL       string       `json:"source_url,omitempty"`
	Attribution     string       `json:"attribution,omitempty"`
	Capabilities    Capabilities `json:"capabilities"`
	Policy          Policy       `json:"policy"`
}

type SearchResponse struct {
	Query  string  `json:"query"`
	Limit  int     `json:"limit"`
	Offset int     `json:"offset"`
	Total  int     `json:"total"`
	Items  []Track `json:"items"`
}

type Playback struct {
	TrackID          string       `json:"track_id"`
	ProviderID       string       `json:"provider_id"`
	PlaybackType     string       `json:"playback_type"`
	StreamURL        *string      `json:"stream_url"`
	EmbedURL         *string      `json:"embed_url"`
	ExpiresInSeconds *int         `json:"expires_in_seconds"`
	Attribution      string       `json:"attribution,omitempty"`
	Capabilities     Capabilities `json:"capabilities"`
	Policy           Policy       `json:"policy"`
}

type Job struct {
	ID        string         `json:"id"`
	Type      string         `json:"type"`
	Status    string         `json:"status"`
	TrackID   string         `json:"track_id,omitempty"`
	Payload   map[string]any `json:"payload"`
	Result    map[string]any `json:"result,omitempty"`
	Error     string         `json:"error,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

type Favorite struct {
	TrackID   string    `json:"track_id"`
	CreatedAt time.Time `json:"created_at"`
}
type Playlist struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Tracks      []PlaylistTrack `json:"tracks"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}
type PlaylistTrack struct {
	TrackID  string    `json:"track_id"`
	Position int       `json:"position"`
	AddedAt  time.Time `json:"added_at"`
}

type ErrorBody struct {
	Error ErrorDetail `json:"error"`
}
type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}
