package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Store struct {
	mu   sync.Mutex
	path string
	data persisted
}
type persisted struct {
	Favorites map[string]Favorite `json:"favorites"`
	Playlists map[string]Playlist `json:"playlists"`
	Jobs      map[string]Job      `json:"jobs"`
}

func NewStore(path string) (*Store, error) {
	s := &Store{path: path, data: persisted{Favorites: map[string]Favorite{}, Playlists: map[string]Playlist{}, Jobs: map[string]Job{}}}
	b, err := os.ReadFile(path)
	if err == nil && len(b) > 0 {
		if err := json.Unmarshal(b, &s.data); err != nil {
			return nil, err
		}
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
func (s *Store) AddFavorite(trackID string) (Favorite, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f := Favorite{TrackID: trackID, CreatedAt: time.Now().UTC()}
	s.data.Favorites[trackID] = f
	return f, s.saveLocked()
}
func (s *Store) ListFavorites() []Favorite {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Favorite, 0, len(s.data.Favorites))
	for _, f := range s.data.Favorites {
		out = append(out, f)
	}
	return out
}
func (s *Store) DeleteFavorite(trackID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data.Favorites, trackID)
	return s.saveLocked()
}
func (s *Store) CreatePlaylist(name, desc string) (Playlist, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	p := Playlist{ID: newID("pl"), Name: name, Description: desc, Tracks: []PlaylistTrack{}, CreatedAt: now, UpdatedAt: now}
	s.data.Playlists[p.ID] = p
	return p, s.saveLocked()
}
func (s *Store) ListPlaylists() []Playlist {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Playlist, 0, len(s.data.Playlists))
	for _, p := range s.data.Playlists {
		out = append(out, p)
	}
	return out
}
func (s *Store) GetPlaylist(id string) (Playlist, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.data.Playlists[id]
	return p, ok
}
func (s *Store) AddPlaylistTrack(id, trackID string) (Playlist, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.data.Playlists[id]
	if !ok {
		return Playlist{}, errors.New("playlist not found")
	}
	p.Tracks = append(p.Tracks, PlaylistTrack{TrackID: trackID, Position: len(p.Tracks), AddedAt: time.Now().UTC()})
	p.UpdatedAt = time.Now().UTC()
	s.data.Playlists[id] = p
	return p, s.saveLocked()
}
func (s *Store) SaveJob(j Job) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	j.UpdatedAt = time.Now().UTC()
	s.data.Jobs[j.ID] = j
	return s.saveLocked()
}
func (s *Store) ListJobs() []Job {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Job, 0, len(s.data.Jobs))
	for _, j := range s.data.Jobs {
		out = append(out, j)
	}
	return out
}
func (s *Store) GetJob(id string) (Job, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, ok := s.data.Jobs[id]
	return j, ok
}
