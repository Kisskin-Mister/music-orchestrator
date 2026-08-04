package main

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Store struct {
	mu   sync.Mutex
	path string
	data persisted
}
type persisted struct {
	Owner     *Owner              `json:"owner,omitempty"`
	Users     map[string]User     `json:"users"`
	Favorites map[string]Favorite `json:"favorites"`
	Playlists map[string]Playlist `json:"playlists"`
	Jobs      map[string]Job      `json:"jobs"`
	Settings  Settings            `json:"settings"`
}

func NewStore(path string) (*Store, error) {
	s := &Store{path: path, data: persisted{Users: map[string]User{}, Favorites: map[string]Favorite{}, Playlists: map[string]Playlist{}, Jobs: map[string]Job{}}}
	b, err := os.ReadFile(path)
	if err == nil && len(b) > 0 {
		if err := json.Unmarshal(b, &s.data); err != nil {
			return nil, err
		}
	}
	if s.data.Users == nil {
		s.data.Users = map[string]User{}
	}
	if s.data.Favorites == nil {
		s.data.Favorites = map[string]Favorite{}
	}
	if s.data.Playlists == nil {
		s.data.Playlists = map[string]Playlist{}
	}
	if s.data.Jobs == nil {
		s.data.Jobs = map[string]Job{}
	}
	return s, nil
}
func (s *Store) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0755); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	b, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(tmp, b, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *Store) visibleToOwnerLocked(recordOwnerID, viewerOwnerID string) bool {
	if recordOwnerID == viewerOwnerID {
		return true
	}
	return recordOwnerID == "" && s.data.Owner != nil && s.data.Owner.ID == viewerOwnerID
}

// Owner returns the registered owner account, if first-run setup has happened.
func (s *Store) Owner() (Owner, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data.Owner == nil {
		return Owner{}, false
	}
	return *s.data.Owner, true
}

// CreateOwner persists the owner exactly once; ok=false when setup already ran.
func (s *Store) CreateOwner(owner Owner) (ok bool, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data.Owner != nil {
		return false, nil
	}
	s.data.Owner = &owner
	return true, s.saveLocked()
}

func (s *Store) AccountByID(id string) (username, role, passwordHash, totpSecret string, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data.Owner != nil && s.data.Owner.ID == id {
		return s.data.Owner.Username, "admin", s.data.Owner.PasswordHash, s.data.Owner.TOTPSecret, true
	}
	if u, exists := s.data.Users[id]; exists {
		return u.Username, u.Role, u.PasswordHash, "", true
	}
	return "", "", "", "", false
}

func (s *Store) AccountByUsername(username string) (id, storedUsername, role, passwordHash, totpSecret string, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	username = strings.TrimSpace(username)
	if s.data.Owner != nil && subtleEqualString(s.data.Owner.Username, username) {
		return s.data.Owner.ID, s.data.Owner.Username, "admin", s.data.Owner.PasswordHash, s.data.Owner.TOTPSecret, true
	}
	for _, u := range s.data.Users {
		if subtleEqualString(u.Username, username) {
			return u.ID, u.Username, u.Role, u.PasswordHash, "", true
		}
	}
	return "", "", "", "", "", false
}

func (s *Store) UpdateOwnerAccount(id, username, passwordHash, totpSecret string) (Owner, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data.Owner == nil || s.data.Owner.ID != id {
		return Owner{}, false, nil
	}
	owner := *s.data.Owner
	if strings.TrimSpace(username) != "" {
		owner.Username = strings.TrimSpace(username)
	}
	if passwordHash != "" {
		owner.PasswordHash = passwordHash
	}
	owner.TOTPSecret = strings.TrimSpace(totpSecret)
	owner.UpdatedAt = time.Now().UTC()
	s.data.Owner = &owner
	return owner, true, s.saveLocked()
}

func (s *Store) ListUsers() []User {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]User, 0, len(s.data.Users))
	for _, u := range s.data.Users {
		u.PasswordHash = ""
		out = append(out, u)
	}
	return out
}

func (s *Store) CreateUser(username, passwordHash string) (User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	u := User{ID: newID("usr"), Username: strings.TrimSpace(username), PasswordHash: passwordHash, Role: "user", CreatedAt: now, UpdatedAt: now}
	s.data.Users[u.ID] = u
	public := u
	public.PasswordHash = ""
	return public, s.saveLocked()
}

func (s *Store) UpdateUser(id, username, passwordHash string) (User, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.data.Users[id]
	if !ok {
		return User{}, false, nil
	}
	if strings.TrimSpace(username) != "" {
		u.Username = strings.TrimSpace(username)
	}
	if passwordHash != "" {
		u.PasswordHash = passwordHash
	}
	u.Role = "user"
	u.UpdatedAt = time.Now().UTC()
	s.data.Users[id] = u
	u.PasswordHash = ""
	return u, true, s.saveLocked()
}

func (s *Store) DeleteUser(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.data.Users[id]; !ok {
		return false
	}
	delete(s.data.Users, id)
	_ = s.saveLocked()
	return true
}

func subtleEqualString(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func (s *Store) AddFavorite(ownerID string, track Track) (Favorite, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	track = sanitizeTrackForStorage(track)
	f := Favorite{TrackID: track.ID, OwnerID: ownerID, Track: &track, CreatedAt: time.Now().UTC()}
	s.data.Favorites[ownerTrackKey(ownerID, track.ID)] = f
	return f, s.saveLocked()
}
func (s *Store) ListFavorites(ownerID string) []Favorite {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Favorite, 0, len(s.data.Favorites))
	for _, f := range s.data.Favorites {
		if !s.visibleToOwnerLocked(f.OwnerID, ownerID) {
			continue
		}
		out = append(out, f)
	}
	return out
}
func (s *Store) DeleteFavorite(ownerID, trackID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data.Favorites, ownerTrackKey(ownerID, trackID))
	return s.saveLocked()
}
func (s *Store) CreatePlaylist(ownerID, name, desc string) (Playlist, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	p := Playlist{ID: newID("pl"), OwnerID: ownerID, Name: name, Description: desc, Tracks: []PlaylistTrack{}, CreatedAt: now, UpdatedAt: now}
	s.data.Playlists[p.ID] = p
	return p.withAggregates(), s.saveLocked()
}
func (s *Store) ListPlaylists(ownerID string) []Playlist {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Playlist, 0, len(s.data.Playlists))
	for _, p := range s.data.Playlists {
		if p.OwnerID != ownerID {
			continue
		}
		out = append(out, p.withAggregates())
	}
	return out
}
func (s *Store) GetPlaylist(ownerID, id string) (Playlist, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.data.Playlists[id]
	if !ok || p.OwnerID != ownerID {
		return Playlist{}, false
	}
	return p.withAggregates(), true
}
func (s *Store) AddPlaylistTrack(ownerID, id string, track Track) (Playlist, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.data.Playlists[id]
	if !ok || p.OwnerID != ownerID {
		return Playlist{}, false, errors.New("playlist not found")
	}
	track = sanitizeTrackForStorage(track)
	for _, item := range p.Tracks {
		if item.TrackID == track.ID {
			return p.withAggregates(), false, s.saveLocked()
		}
	}
	now := time.Now().UTC()
	p.Tracks = append(p.Tracks, PlaylistTrack{ID: newID("pli"), TrackID: track.ID, Track: &track, Position: len(p.Tracks), AddedAt: now})
	p.UpdatedAt = now
	s.data.Playlists[id] = p
	return p.withAggregates(), true, s.saveLocked()
}

func (s *Store) UpdatePlaylist(ownerID, id string, update PlaylistUpdate) (Playlist, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.data.Playlists[id]
	if !ok || p.OwnerID != ownerID {
		return Playlist{}, false, nil
	}
	if update.Name != nil {
		name := strings.TrimSpace(*update.Name)
		if name == "" {
			return Playlist{}, true, errors.New("playlist name is required")
		}
		p.Name = name
	}
	if update.Description != nil {
		p.Description = strings.TrimSpace(*update.Description)
	}
	if update.CoverURL != nil {
		p.CoverURL = strings.TrimSpace(*update.CoverURL)
	}
	p.UpdatedAt = time.Now().UTC()
	s.data.Playlists[id] = p
	return p.withAggregates(), true, s.saveLocked()
}

func (s *Store) DeletePlaylist(ownerID, id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.data.Playlists[id]
	if !ok || p.OwnerID != ownerID {
		return false
	}
	delete(s.data.Playlists, id)
	_ = s.saveLocked()
	return true
}

func (s *Store) RemovePlaylistTrack(ownerID, id, trackID string) (Playlist, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.data.Playlists[id]
	if !ok || p.OwnerID != ownerID {
		return Playlist{}, false, errors.New("playlist not found")
	}
	tracks := p.Tracks[:0]
	removed := false
	for _, item := range p.Tracks {
		if item.TrackID == trackID {
			removed = true
			continue
		}
		tracks = append(tracks, item)
	}
	if !removed {
		return Playlist{}, false, errors.New("track not found")
	}
	p.Tracks = tracks
	p.UpdatedAt = time.Now().UTC()
	s.data.Playlists[id] = p
	return p.withAggregates(), true, s.saveLocked()
}
func (s *Store) SaveJob(ownerID string, job Job) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	job.OwnerID = ownerID
	job.UpdatedAt = time.Now().UTC()
	s.data.Jobs[job.ID] = job
	return s.saveLocked()
}

func (s *Store) DeleteDownloadsByTrack(ownerID, trackID string) []Job {
	s.mu.Lock()
	defer s.mu.Unlock()
	removed := []Job{}
	for id, job := range s.data.Jobs {
		if s.visibleToOwnerLocked(job.OwnerID, ownerID) && job.Type == "download" && job.TrackID == trackID {
			removed = append(removed, job)
			delete(s.data.Jobs, id)
		}
	}
	_ = s.saveLocked()
	return removed
}
func (s *Store) ListJobs(ownerID string) []Job {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Job, 0, len(s.data.Jobs))
	for _, j := range s.data.Jobs {
		if !s.visibleToOwnerLocked(j.OwnerID, ownerID) {
			continue
		}
		out = append(out, j)
	}
	return out
}
func (s *Store) GetJob(ownerID, id string) (Job, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, ok := s.data.Jobs[id]
	if !ok || !s.visibleToOwnerLocked(j.OwnerID, ownerID) {
		return Job{}, false
	}
	return j, true
}

func (s *Store) FindSuccessfulDownload(ownerID, providerID, providerTrackID string) (Job, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var newest Job
	found := false
	for _, j := range s.data.Jobs {
		if !s.visibleToOwnerLocked(j.OwnerID, ownerID) || j.Type != "download" || j.Status != "succeeded" {
			continue
		}
		matches := j.TrackID == providerID+":"+providerTrackID || (j.Result != nil && j.Result["provider_id"] == providerID && j.Result["provider_track_id"] == providerTrackID)
		if matches && (!found || j.UpdatedAt.After(newest.UpdatedAt)) {
			newest = j
			found = true
		}
	}
	return newest, found
}

func (s *Store) SuccessfulDownloads(ownerID string) []Job {
	s.mu.Lock()
	defer s.mu.Unlock()
	byTrack := map[string]Job{}
	for _, j := range s.data.Jobs {
		if !s.visibleToOwnerLocked(j.OwnerID, ownerID) || j.Type != "download" || j.Status != "succeeded" || j.TrackID == "" {
			continue
		}
		prev, exists := byTrack[j.TrackID]
		if !exists || j.UpdatedAt.After(prev.UpdatedAt) {
			byTrack[j.TrackID] = j
		}
	}
	out := make([]Job, 0, len(byTrack))
	for _, j := range byTrack {
		out = append(out, j)
	}
	return out
}

func sanitizeTrackForStorage(t Track) Track {
	if t.Downloaded && t.DownloadMediaURL == "" {
		t.Downloaded = false
	}
	return t
}

func (p Playlist) withAggregates() Playlist {
	return p.withAggregatesPreservingCover()
}

func (p Playlist) withAggregatesPreservingCover() Playlist {
	p.TrackCount = len(p.Tracks)
	p.DurationSeconds = 0
	for i := range p.Tracks {
		p.Tracks[i].Position = i
		if p.Tracks[i].ID == "" {
			p.Tracks[i].ID = newID("pli")
		}
		if p.Tracks[i].Track == nil {
			continue
		}
		p.DurationSeconds += p.Tracks[i].Track.DurationSeconds
		if p.CoverURL == "" {
			p.CoverURL = p.Tracks[i].Track.ArtworkURL
		}
	}
	return p
}

func ownerTrackKey(ownerID, trackID string) string { return ownerID + "\x00" + trackID }

// MergeSettings applies a partial settings patch onto the stored overrides and
// returns the merged result. Fields left nil in the patch keep their previous
// value, so the UI can send only what the user actually changed.
func (s *Store) MergeSettings(patch Settings) (Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur := s.data.Settings
	if patch.EnableRiskyExtractors != nil {
		cur.EnableRiskyExtractors = patch.EnableRiskyExtractors
	}
	if patch.ExtractorTimeoutSeconds != nil {
		cur.ExtractorTimeoutSeconds = patch.ExtractorTimeoutSeconds
	}
	if patch.DownloadTimeoutSeconds != nil {
		cur.DownloadTimeoutSeconds = patch.DownloadTimeoutSeconds
	}
	if patch.SessionTTLHours != nil {
		cur.SessionTTLHours = patch.SessionTTLHours
	}
	if patch.SecureCookies != nil {
		cur.SecureCookies = patch.SecureCookies
	}
	if patch.PublicMediaBaseURL != nil {
		cur.PublicMediaBaseURL = patch.PublicMediaBaseURL
	}
	if patch.CORSOrigins != nil {
		cur.CORSOrigins = patch.CORSOrigins
	}
	if patch.YouTubeAPIKey != nil {
		cur.YouTubeAPIKey = patch.YouTubeAPIKey
	}
	if patch.SoundCloudClientID != nil {
		cur.SoundCloudClientID = patch.SoundCloudClientID
	}
	if patch.NavidromeBaseURL != nil {
		cur.NavidromeBaseURL = patch.NavidromeBaseURL
	}
	if patch.NavidromeUsername != nil {
		cur.NavidromeUsername = patch.NavidromeUsername
	}
	if patch.NavidromeToken != nil {
		cur.NavidromeToken = patch.NavidromeToken
	}
	s.data.Settings = cur
	return cur, s.saveLocked()
}

// StoredSettings returns the persisted overrides, applied on top of env config
// at boot so a restart does not silently revert what an admin changed.
func (s *Store) StoredSettings() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data.Settings
}
