package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// Store persists everything the server owns.
//
// It used to be a single JSON document rewritten in full on every change, which
// made writes quadratic: importing 3 000 tracks took 19 s (6.4 ms per track,
// against 0.7 ms at 200 tracks) because each addition re-serialised the whole
// library. Rows now change individually, so cost per track stays flat and a
// library of tens of thousands is realistic.
//
// The driver is modernc.org/sqlite — a pure-Go implementation. cgo-based
// alternatives are faster, but they would break the release workflow, which
// cross-compiles a linux/arm64 binary for Raspberry Pi from an amd64 runner.
//
// Every query is parameterised. No statement is ever assembled from user input.
type Store struct {
	// Writes are serialised: SQLite allows one writer, and a mutex gives a
	// clearer failure than sporadic SQLITE_BUSY under concurrent imports.
	mu   sync.Mutex
	db   *sql.DB
	path string
}

const schema = `
PRAGMA journal_mode = WAL;
PRAGMA foreign_keys = ON;
PRAGMA busy_timeout = 5000;

CREATE TABLE IF NOT EXISTS owner (
  id            TEXT PRIMARY KEY,
  username      TEXT NOT NULL,
  password_hash TEXT NOT NULL DEFAULT '',
  totp_secret   TEXT NOT NULL DEFAULT '',
  created_at    TEXT NOT NULL,
  updated_at    TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS users (
  id            TEXT PRIMARY KEY,
  username      TEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL DEFAULT '',
  role          TEXT NOT NULL DEFAULT 'user',
  created_at    TEXT NOT NULL,
  updated_at    TEXT NOT NULL
);

-- track_json holds the full Track. The columns beside it are the ones we
-- actually filter and sort on, so they get indexes; decomposing every remaining
-- field would add migration risk without changing any query plan.
CREATE TABLE IF NOT EXISTS favorites (
  owner_id   TEXT NOT NULL,
  track_id   TEXT NOT NULL,
  track_json TEXT NOT NULL,
  created_at TEXT NOT NULL,
  PRIMARY KEY (owner_id, track_id)
);
CREATE INDEX IF NOT EXISTS idx_favorites_owner ON favorites(owner_id, created_at DESC);

CREATE TABLE IF NOT EXISTS playlists (
  id          TEXT PRIMARY KEY,
  owner_id    TEXT NOT NULL,
  name        TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  cover_url   TEXT NOT NULL DEFAULT '',
  created_at  TEXT NOT NULL,
  updated_at  TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_playlists_owner ON playlists(owner_id, created_at DESC);

CREATE TABLE IF NOT EXISTS playlist_tracks (
  id          TEXT PRIMARY KEY,
  playlist_id TEXT NOT NULL REFERENCES playlists(id) ON DELETE CASCADE,
  track_id    TEXT NOT NULL,
  track_json  TEXT NOT NULL,
  position    INTEGER NOT NULL,
  added_at    TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_playlist_tracks ON playlist_tracks(playlist_id, position);

CREATE TABLE IF NOT EXISTS jobs (
  id           TEXT PRIMARY KEY,
  owner_id     TEXT NOT NULL,
  type         TEXT NOT NULL,
  status       TEXT NOT NULL,
  track_id     TEXT NOT NULL DEFAULT '',
  payload_json TEXT NOT NULL DEFAULT '{}',
  result_json  TEXT NOT NULL DEFAULT '{}',
  error        TEXT NOT NULL DEFAULT '',
  created_at   TEXT NOT NULL,
  updated_at   TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_jobs_owner ON jobs(owner_id, created_at DESC);
-- Serves FindSuccessfulDownload and SuccessfulDownloads, the hot path when the
-- library screen asks "which of these already exist on the server?".
CREATE INDEX IF NOT EXISTS idx_jobs_download ON jobs(owner_id, type, status, track_id, updated_at DESC);

CREATE TABLE IF NOT EXISTS settings (
  id            INTEGER PRIMARY KEY CHECK (id = 1),
  settings_json TEXT NOT NULL
);

-- Full-text index over the library, so searching thousands of tracks stays
-- instant. Populated alongside favorites; unicode61 handles Cyrillic titles.
CREATE VIRTUAL TABLE IF NOT EXISTS tracks_fts USING fts5(
  track_id UNINDEXED,
  owner_id UNINDEXED,
  title,
  artist,
  album,
  tokenize = 'unicode61 remove_diacritics 2'
);
`

func NewStore(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	dbPath := strings.TrimSuffix(path, ".json") + ".db"
	db, err := sql.Open("sqlite", dbPath+"?_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	// One connection keeps writes strictly ordered and avoids SQLITE_BUSY
	// storms; reads are fast enough that pooling buys nothing here.
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("schema: %w", err)
	}
	s := &Store{db: db, path: dbPath}
	if err := s.importLegacyJSON(path); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migration from %s: %w", path, err)
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

// importLegacyJSON moves an existing store.json into SQLite once, then renames
// it aside. The original is kept rather than deleted: if the migration turns out
// to be wrong, the only copy of someone's library must still exist.
func (s *Store) importLegacyJSON(jsonPath string) error {
	raw, err := os.ReadFile(jsonPath)
	if err != nil {
		return nil // nothing to migrate
	}
	var legacy struct {
		Owner     *Owner              `json:"owner"`
		Users     map[string]User     `json:"users"`
		Favorites map[string]Favorite `json:"favorites"`
		Playlists map[string]Playlist `json:"playlists"`
		Jobs      map[string]Job      `json:"jobs"`
		Settings  Settings            `json:"settings"`
	}
	if err := json.Unmarshal(raw, &legacy); err != nil {
		return fmt.Errorf("unreadable legacy store: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if legacy.Owner != nil {
		o := *legacy.Owner
		if _, err := tx.Exec(`INSERT OR IGNORE INTO owner (id,username,password_hash,totp_secret,created_at,updated_at)
			VALUES (?,?,?,?,?,?)`, o.ID, o.Username, o.PasswordHash, o.TOTPSecret, ts(o.CreatedAt), ts(o.UpdatedAt)); err != nil {
			return err
		}
	}
	for _, u := range legacy.Users {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO users (id,username,password_hash,role,created_at,updated_at)
			VALUES (?,?,?,?,?,?)`, u.ID, u.Username, u.PasswordHash, u.Role, ts(u.CreatedAt), ts(u.UpdatedAt)); err != nil {
			return err
		}
	}
	for _, f := range legacy.Favorites {
		track := Track{ID: f.TrackID}
		if f.Track != nil {
			track = *f.Track
		}
		if err := insertFavoriteTx(tx, f.OwnerID, track, f.CreatedAt); err != nil {
			return err
		}
	}
	for _, p := range legacy.Playlists {
		if err := upsertPlaylistTx(tx, p); err != nil {
			return err
		}
	}
	for _, j := range legacy.Jobs {
		if err := upsertJobTx(tx, j); err != nil {
			return err
		}
	}
	if blob, err := json.Marshal(legacy.Settings); err == nil {
		if _, err := tx.Exec(`INSERT OR REPLACE INTO settings (id,settings_json) VALUES (1,?)`, string(blob)); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return os.Rename(jsonPath, jsonPath+".migrated")
}

func ts(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func parseTS(v string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, v)
	if err != nil {
		return time.Time{}
	}
	return t
}

// ownerFilter reproduces the original visibility rule: a viewer sees their own
// records, plus legacy rows saved with no owner when the viewer is the owner
// account. Returned as SQL so filtering happens in the database rather than by
// loading every row into memory.
func ownerFilter(column string) string {
	return fmt.Sprintf(`(%s = ? OR (%s = '' AND EXISTS (SELECT 1 FROM owner WHERE owner.id = ?)))`, column, column)
}

// --- Owner and users ---------------------------------------------------

func (s *Store) Owner() (Owner, bool) {
	var o Owner
	var created, updated string
	err := s.db.QueryRow(`SELECT id,username,password_hash,totp_secret,created_at,updated_at FROM owner LIMIT 1`).
		Scan(&o.ID, &o.Username, &o.PasswordHash, &o.TOTPSecret, &created, &updated)
	if err != nil {
		return Owner{}, false
	}
	o.CreatedAt, o.UpdatedAt = parseTS(created), parseTS(updated)
	return o, true
}

func (s *Store) CreateOwner(owner Owner) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var exists int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM owner`).Scan(&exists); err != nil {
		return false, err
	}
	if exists > 0 {
		return false, nil
	}
	_, err := s.db.Exec(`INSERT INTO owner (id,username,password_hash,totp_secret,created_at,updated_at) VALUES (?,?,?,?,?,?)`,
		owner.ID, owner.Username, owner.PasswordHash, owner.TOTPSecret, ts(owner.CreatedAt), ts(owner.UpdatedAt))
	return err == nil, err
}

func (s *Store) AccountByID(id string) (username, role, passwordHash, totpSecret string, ok bool) {
	if o, found := s.Owner(); found && o.ID == id {
		return o.Username, "admin", o.PasswordHash, o.TOTPSecret, true
	}
	err := s.db.QueryRow(`SELECT username,role,password_hash FROM users WHERE id = ?`, id).
		Scan(&username, &role, &passwordHash)
	if err != nil {
		return "", "", "", "", false
	}
	return username, role, passwordHash, "", true
}

func (s *Store) AccountByUsername(username string) (id, storedUsername, role, passwordHash, totpSecret string, ok bool) {
	if o, found := s.Owner(); found && strings.EqualFold(o.Username, username) {
		return o.ID, o.Username, "admin", o.PasswordHash, o.TOTPSecret, true
	}
	err := s.db.QueryRow(`SELECT id,username,role,password_hash FROM users WHERE username = ? COLLATE NOCASE`, username).
		Scan(&id, &storedUsername, &role, &passwordHash)
	if err != nil {
		return "", "", "", "", "", false
	}
	return id, storedUsername, role, passwordHash, "", true
}

func (s *Store) UpdateOwnerAccount(id, username, passwordHash, totpSecret string) (Owner, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	o, found := s.Owner()
	if !found || o.ID != id {
		return Owner{}, false, nil
	}
	o.Username, o.TOTPSecret, o.UpdatedAt = username, totpSecret, time.Now().UTC()
	if passwordHash != "" {
		o.PasswordHash = passwordHash
	}
	_, err := s.db.Exec(`UPDATE owner SET username=?,password_hash=?,totp_secret=?,updated_at=? WHERE id=?`,
		o.Username, o.PasswordHash, o.TOTPSecret, ts(o.UpdatedAt), o.ID)
	return o, err == nil, err
}

func (s *Store) ListUsers() []User {
	rows, err := s.db.Query(`SELECT id,username,role,created_at,updated_at FROM users ORDER BY created_at`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := []User{}
	for rows.Next() {
		var u User
		var created, updated string
		if err := rows.Scan(&u.ID, &u.Username, &u.Role, &created, &updated); err != nil {
			continue
		}
		u.CreatedAt, u.UpdatedAt = parseTS(created), parseTS(updated)
		out = append(out, u)
	}
	return out
}

func (s *Store) CreateUser(username, passwordHash string) (User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, _, _, _, _, taken := s.AccountByUsername(username); taken {
		return User{}, errors.New("username already exists")
	}
	now := time.Now().UTC()
	u := User{ID: newID("usr"), Username: username, PasswordHash: passwordHash, Role: "user", CreatedAt: now, UpdatedAt: now}
	_, err := s.db.Exec(`INSERT INTO users (id,username,password_hash,role,created_at,updated_at) VALUES (?,?,?,?,?,?)`,
		u.ID, u.Username, u.PasswordHash, u.Role, ts(now), ts(now))
	return u, err
}

func (s *Store) UpdateUser(id, username, passwordHash string) (User, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var u User
	var created string
	err := s.db.QueryRow(`SELECT id,username,role,password_hash,created_at FROM users WHERE id=?`, id).
		Scan(&u.ID, &u.Username, &u.Role, &u.PasswordHash, &created)
	if err != nil {
		return User{}, false, nil
	}
	if username != "" {
		u.Username = username
	}
	if passwordHash != "" {
		u.PasswordHash = passwordHash
	}
	u.CreatedAt, u.UpdatedAt = parseTS(created), time.Now().UTC()
	_, err = s.db.Exec(`UPDATE users SET username=?,password_hash=?,updated_at=? WHERE id=?`,
		u.Username, u.PasswordHash, ts(u.UpdatedAt), u.ID)
	return u, err == nil, err
}

func (s *Store) DeleteUser(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	res, err := s.db.Exec(`DELETE FROM users WHERE id=?`, id)
	if err != nil {
		return false
	}
	n, _ := res.RowsAffected()
	return n > 0
}

// --- Favorites ---------------------------------------------------------

func insertFavoriteTx(tx *sql.Tx, ownerID string, track Track, created time.Time) error {
	track = sanitizeTrackForStorage(track)
	blob, err := json.Marshal(track)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT OR REPLACE INTO favorites (owner_id,track_id,track_json,created_at) VALUES (?,?,?,?)`,
		ownerID, track.ID, string(blob), ts(created)); err != nil {
		return err
	}
	// Keep the search index in step with the row it describes.
	if _, err := tx.Exec(`DELETE FROM tracks_fts WHERE track_id = ? AND owner_id = ?`, track.ID, ownerID); err != nil {
		return err
	}
	_, err = tx.Exec(`INSERT INTO tracks_fts (track_id,owner_id,title,artist,album) VALUES (?,?,?,?,?)`,
		track.ID, ownerID, track.Title, track.Artist, track.Album)
	return err
}

func (s *Store) AddFavorite(ownerID string, track Track) (Favorite, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	track = sanitizeTrackForStorage(track)
	now := time.Now().UTC()
	tx, err := s.db.Begin()
	if err != nil {
		return Favorite{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := insertFavoriteTx(tx, ownerID, track, now); err != nil {
		return Favorite{}, err
	}
	if err := tx.Commit(); err != nil {
		return Favorite{}, err
	}
	return Favorite{TrackID: track.ID, OwnerID: ownerID, Track: &track, CreatedAt: now}, nil
}

func (s *Store) ListFavorites(ownerID string) []Favorite {
	rows, err := s.db.Query(`SELECT owner_id,track_id,track_json,created_at FROM favorites WHERE `+
		ownerFilter("owner_id")+` ORDER BY created_at DESC`, ownerID, ownerID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := []Favorite{}
	for rows.Next() {
		var f Favorite
		var blob, created string
		if err := rows.Scan(&f.OwnerID, &f.TrackID, &blob, &created); err != nil {
			continue
		}
		var t Track
		if json.Unmarshal([]byte(blob), &t) == nil {
			f.Track = &t
		}
		f.CreatedAt = parseTS(created)
		out = append(out, f)
	}
	return out
}

func (s *Store) DeleteFavorite(ownerID, trackID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`DELETE FROM favorites WHERE owner_id=? AND track_id=?`, ownerID, trackID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM tracks_fts WHERE owner_id=? AND track_id=?`, ownerID, trackID); err != nil {
		return err
	}
	return tx.Commit()
}

// SearchLibrary answers free-text queries from the FTS index instead of
// scanning every row, which is what makes a library of thousands navigable.
func (s *Store) SearchLibrary(ownerID, query string, limit int) []Track {
	if strings.TrimSpace(query) == "" {
		return nil
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.Query(`
		SELECT f.track_json FROM tracks_fts fts
		JOIN favorites f ON f.track_id = fts.track_id AND f.owner_id = fts.owner_id
		WHERE tracks_fts MATCH ? AND `+ownerFilter("fts.owner_id")+`
		ORDER BY rank LIMIT ?`, ftsQuery(query), ownerID, ownerID, limit)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := []Track{}
	for rows.Next() {
		var blob string
		if rows.Scan(&blob) != nil {
			continue
		}
		var t Track
		if json.Unmarshal([]byte(blob), &t) == nil {
			out = append(out, t)
		}
	}
	return out
}

// ftsQuery turns user input into a prefix search while stripping the FTS5
// operators, so a stray quote or `*` cannot change the query's meaning.
func ftsQuery(input string) string {
	var terms []string
	for _, word := range strings.Fields(input) {
		clean := strings.Map(func(r rune) rune {
			if strings.ContainsRune(`"*():^-`, r) {
				return -1
			}
			return r
		}, word)
		if clean != "" {
			terms = append(terms, `"`+clean+`"*`)
		}
	}
	return strings.Join(terms, " ")
}

// --- Playlists ---------------------------------------------------------

func upsertPlaylistTx(tx *sql.Tx, p Playlist) error {
	p = p.withAggregates()
	if _, err := tx.Exec(`INSERT OR REPLACE INTO playlists (id,owner_id,name,description,cover_url,created_at,updated_at)
		VALUES (?,?,?,?,?,?,?)`, p.ID, p.OwnerID, p.Name, p.Description, p.CoverURL, ts(p.CreatedAt), ts(p.UpdatedAt)); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM playlist_tracks WHERE playlist_id = ?`, p.ID); err != nil {
		return err
	}
	for _, pt := range p.Tracks {
		track := Track{ID: pt.TrackID}
		if pt.Track != nil {
			track = sanitizeTrackForStorage(*pt.Track)
		}
		blob, err := json.Marshal(track)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO playlist_tracks (id,playlist_id,track_id,track_json,position,added_at)
			VALUES (?,?,?,?,?,?)`, pt.ID, p.ID, pt.TrackID, string(blob), pt.Position, ts(pt.AddedAt)); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) loadPlaylist(id string) (Playlist, bool) {
	var p Playlist
	var created, updated string
	err := s.db.QueryRow(`SELECT id,owner_id,name,description,cover_url,created_at,updated_at FROM playlists WHERE id=?`, id).
		Scan(&p.ID, &p.OwnerID, &p.Name, &p.Description, &p.CoverURL, &created, &updated)
	if err != nil {
		return Playlist{}, false
	}
	p.CreatedAt, p.UpdatedAt = parseTS(created), parseTS(updated)
	rows, err := s.db.Query(`SELECT id,track_id,track_json,position,added_at FROM playlist_tracks WHERE playlist_id=? ORDER BY position`, id)
	if err != nil {
		return p.withAggregates(), true
	}
	defer rows.Close()
	for rows.Next() {
		var pt PlaylistTrack
		var blob, added string
		if err := rows.Scan(&pt.ID, &pt.TrackID, &blob, &pt.Position, &added); err != nil {
			continue
		}
		var t Track
		if json.Unmarshal([]byte(blob), &t) == nil {
			pt.Track = &t
		}
		pt.AddedAt = parseTS(added)
		p.Tracks = append(p.Tracks, pt)
	}
	return p.withAggregates(), true
}

func (s *Store) CreatePlaylist(ownerID, name, desc string) (Playlist, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	p := Playlist{ID: newID("pl"), OwnerID: ownerID, Name: name, Description: desc, CreatedAt: now, UpdatedAt: now}
	tx, err := s.db.Begin()
	if err != nil {
		return Playlist{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := upsertPlaylistTx(tx, p); err != nil {
		return Playlist{}, err
	}
	if err := tx.Commit(); err != nil {
		return Playlist{}, err
	}
	return p.withAggregates(), nil
}

func (s *Store) ListPlaylists(ownerID string) []Playlist {
	rows, err := s.db.Query(`SELECT id FROM playlists WHERE `+ownerFilter("owner_id")+` ORDER BY created_at DESC`, ownerID, ownerID)
	if err != nil {
		return nil
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	out := []Playlist{}
	for _, id := range ids {
		if p, ok := s.loadPlaylist(id); ok {
			out = append(out, p)
		}
	}
	return out
}

func (s *Store) GetPlaylist(ownerID, id string) (Playlist, bool) {
	p, ok := s.loadPlaylist(id)
	if !ok || !s.visibleTo(p.OwnerID, ownerID) {
		return Playlist{}, false
	}
	return p, true
}

func (s *Store) visibleTo(recordOwnerID, viewerOwnerID string) bool {
	if recordOwnerID == viewerOwnerID {
		return true
	}
	o, found := s.Owner()
	return recordOwnerID == "" && found && o.ID == viewerOwnerID
}

func (s *Store) AddPlaylistTrack(ownerID, id string, track Track) (Playlist, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.loadPlaylist(id)
	if !ok || p.OwnerID != ownerID {
		return Playlist{}, false, errors.New("playlist not found")
	}
	track = sanitizeTrackForStorage(track)
	for _, pt := range p.Tracks {
		// Adding the same track twice is a no-op, and the `false` tells the
		// handler to answer 200 rather than 201 Created.
		if pt.TrackID == track.ID {
			return p.withAggregates(), false, nil
		}
	}
	now := time.Now().UTC()
	p.Tracks = append(p.Tracks, PlaylistTrack{
		ID: newID("pli"), TrackID: track.ID, Track: &track,
		Position: len(p.Tracks), AddedAt: now,
	})
	p.UpdatedAt = now
	return s.savePlaylist(p)
}

func (s *Store) savePlaylist(p Playlist) (Playlist, bool, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return Playlist{}, false, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := upsertPlaylistTx(tx, p); err != nil {
		return Playlist{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return Playlist{}, false, err
	}
	return p.withAggregates(), true, nil
}

func (s *Store) UpdatePlaylist(ownerID, id string, update PlaylistUpdate) (Playlist, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.loadPlaylist(id)
	if !ok || p.OwnerID != ownerID {
		return Playlist{}, false, nil
	}
	if update.Name != nil {
		name := strings.TrimSpace(*update.Name)
		// `true` with an error means "the playlist exists but the request is
		// invalid", which the handler turns into 400 rather than 404.
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
	return s.savePlaylist(p)
}

func (s *Store) DeletePlaylist(ownerID, id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.loadPlaylist(id)
	if !ok || !s.visibleTo(p.OwnerID, ownerID) {
		return false
	}
	// playlist_tracks rows go with it via ON DELETE CASCADE.
	res, err := s.db.Exec(`DELETE FROM playlists WHERE id=?`, id)
	if err != nil {
		return false
	}
	n, _ := res.RowsAffected()
	return n > 0
}

func (s *Store) RemovePlaylistTrack(ownerID, id, trackID string) (Playlist, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.loadPlaylist(id)
	if !ok || p.OwnerID != ownerID {
		return Playlist{}, false, errors.New("playlist not found")
	}
	kept := p.Tracks[:0]
	removed := false
	for _, pt := range p.Tracks {
		if pt.TrackID == trackID {
			removed = true
			continue
		}
		kept = append(kept, pt)
	}
	if !removed {
		return Playlist{}, false, errors.New("track not found")
	}
	p.Tracks = kept
	p.UpdatedAt = time.Now().UTC()
	return s.savePlaylist(p)
}

// --- Jobs --------------------------------------------------------------

func upsertJobTx(tx *sql.Tx, j Job) error {
	payload, err := json.Marshal(orEmptyMap(j.Payload))
	if err != nil {
		return err
	}
	result, err := json.Marshal(orEmptyMap(j.Result))
	if err != nil {
		return err
	}
	_, err = tx.Exec(`INSERT OR REPLACE INTO jobs (id,owner_id,type,status,track_id,payload_json,result_json,error,created_at,updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?)`,
		j.ID, j.OwnerID, j.Type, j.Status, j.TrackID, string(payload), string(result), j.Error, ts(j.CreatedAt), ts(j.UpdatedAt))
	return err
}

func orEmptyMap(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}

func (s *Store) SaveJob(ownerID string, job Job) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	job.OwnerID = ownerID
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := upsertJobTx(tx, job); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) scanJobs(rows *sql.Rows) []Job {
	defer rows.Close()
	out := []Job{}
	for rows.Next() {
		var j Job
		var payload, result, created, updated string
		if err := rows.Scan(&j.ID, &j.OwnerID, &j.Type, &j.Status, &j.TrackID, &payload, &result, &j.Error, &created, &updated); err != nil {
			continue
		}
		_ = json.Unmarshal([]byte(payload), &j.Payload)
		_ = json.Unmarshal([]byte(result), &j.Result)
		j.CreatedAt, j.UpdatedAt = parseTS(created), parseTS(updated)
		out = append(out, j)
	}
	return out
}

const jobColumns = `id,owner_id,type,status,track_id,payload_json,result_json,error,created_at,updated_at`

func (s *Store) DeleteDownloadsByTrack(ownerID, trackID string) []Job {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`SELECT `+jobColumns+` FROM jobs WHERE `+ownerFilter("owner_id")+
		` AND type='download' AND track_id=?`, ownerID, ownerID, trackID)
	if err != nil {
		return nil
	}
	removed := s.scanJobs(rows)
	for _, j := range removed {
		if _, err := s.db.Exec(`DELETE FROM jobs WHERE id=?`, j.ID); err != nil {
			return removed
		}
	}
	return removed
}

func (s *Store) ListJobs(ownerID string) []Job {
	rows, err := s.db.Query(`SELECT `+jobColumns+` FROM jobs WHERE `+ownerFilter("owner_id")+` ORDER BY created_at DESC`, ownerID, ownerID)
	if err != nil {
		return nil
	}
	return s.scanJobs(rows)
}

func (s *Store) GetJob(ownerID, id string) (Job, bool) {
	rows, err := s.db.Query(`SELECT `+jobColumns+` FROM jobs WHERE id=? AND `+ownerFilter("owner_id"), id, ownerID, ownerID)
	if err != nil {
		return Job{}, false
	}
	jobs := s.scanJobs(rows)
	if len(jobs) == 0 {
		return Job{}, false
	}
	return jobs[0], true
}

func (s *Store) FindSuccessfulDownload(ownerID, providerID, providerTrackID string) (Job, bool) {
	trackID := providerID + ":" + providerTrackID
	rows, err := s.db.Query(`SELECT `+jobColumns+` FROM jobs WHERE `+ownerFilter("owner_id")+
		` AND type='download' AND status='succeeded' AND track_id=? ORDER BY updated_at DESC LIMIT 1`,
		ownerID, ownerID, trackID)
	if err != nil {
		return Job{}, false
	}
	jobs := s.scanJobs(rows)
	if len(jobs) == 0 {
		return Job{}, false
	}
	return jobs[0], true
}

func (s *Store) SuccessfulDownloads(ownerID string) []Job {
	// One row per track: the newest successful download wins, which is what the
	// library screen means by "this exists on the server".
	rows, err := s.db.Query(`SELECT `+jobColumns+` FROM jobs j WHERE `+ownerFilter("j.owner_id")+`
		AND j.type='download' AND j.status='succeeded' AND j.track_id <> ''
		AND j.updated_at = (SELECT MAX(k.updated_at) FROM jobs k
		                    WHERE k.track_id = j.track_id AND k.type='download' AND k.status='succeeded')
		GROUP BY j.track_id`, ownerID, ownerID)
	if err != nil {
		return nil
	}
	return s.scanJobs(rows)
}

// --- Settings ----------------------------------------------------------

func (s *Store) StoredSettings() Settings {
	var blob string
	if err := s.db.QueryRow(`SELECT settings_json FROM settings WHERE id=1`).Scan(&blob); err != nil {
		return Settings{}
	}
	var out Settings
	_ = json.Unmarshal([]byte(blob), &out)
	return out
}

func (s *Store) MergeSettings(patch Settings) (Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur := s.StoredSettings()
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
	blob, err := json.Marshal(cur)
	if err != nil {
		return cur, err
	}
	_, err = s.db.Exec(`INSERT OR REPLACE INTO settings (id,settings_json) VALUES (1,?)`, string(blob))
	return cur, err
}

// --- Shared helpers ----------------------------------------------------

func sanitizeTrackForStorage(t Track) Track {
	if t.Downloaded && t.DownloadMediaURL == "" {
		t.Downloaded = false
	}
	return t
}

func (p Playlist) withAggregates() Playlist {
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
