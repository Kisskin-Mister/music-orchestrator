package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func playlistRequest(t *testing.T, app *App, method, path, body, key string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRecorder()
	app.ServeHTTP(r, authReq(method, path, body, key))
	return r
}

func createTestPlaylist(t *testing.T, app *App, key, name string) Playlist {
	t.Helper()
	r := playlistRequest(t, app, "POST", "/v1/playlists", `{"name":`+strconvQuote(name)+`,"description":"old"}`, key)
	if r.Code != http.StatusCreated {
		t.Fatalf("create playlist %d: %s", r.Code, r.Body.String())
	}
	var p Playlist
	if err := json.Unmarshal(r.Body.Bytes(), &p); err != nil {
		t.Fatal(err)
	}
	return p
}

func readPlaylist(t *testing.T, r *httptest.ResponseRecorder) Playlist {
	t.Helper()
	var p Playlist
	if err := json.Unmarshal(r.Body.Bytes(), &p); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestPlaylistUpdateRenameDescriptionCover(t *testing.T) {
	app := testApp(t, false)
	p := createTestPlaylist(t, app, "test-key", "Mix")

	r := playlistRequest(t, app, "PATCH", "/v1/playlists/"+p.ID, `{"name":"Renamed","description":"new desc","cover_url":"https://img.example/cover.jpg"}`, "test-key")
	if r.Code != 200 {
		t.Fatalf("update playlist %d: %s", r.Code, r.Body.String())
	}
	updated := readPlaylist(t, r)
	if updated.Name != "Renamed" || updated.Description != "new desc" || updated.CoverURL != "https://img.example/cover.jpg" {
		t.Fatalf("update not applied: %#v", updated)
	}
	if updated.UpdatedAt.Before(p.UpdatedAt) {
		t.Fatalf("updated_at must move forward: %v vs %v", updated.UpdatedAt, p.UpdatedAt)
	}

	// Partial update keeps untouched fields.
	r = playlistRequest(t, app, "PATCH", "/v1/playlists/"+p.ID, `{"name":"Only Name"}`, "test-key")
	if r.Code != 200 {
		t.Fatalf("partial update %d: %s", r.Code, r.Body.String())
	}
	updated = readPlaylist(t, r)
	if updated.Name != "Only Name" || updated.Description != "new desc" || updated.CoverURL != "https://img.example/cover.jpg" {
		t.Fatalf("partial update wiped fields: %#v", updated)
	}

	// Empty name is rejected.
	r = playlistRequest(t, app, "PATCH", "/v1/playlists/"+p.ID, `{"name":"  "}`, "test-key")
	if r.Code != http.StatusBadRequest {
		t.Fatalf("empty name must be 400, got %d", r.Code)
	}

	// Other users cannot touch the playlist.
	r = playlistRequest(t, app, "PATCH", "/v1/playlists/"+p.ID, `{"name":"Hacked"}`, "other-key")
	if r.Code != http.StatusNotFound {
		t.Fatalf("foreign update must be 404, got %d", r.Code)
	}

	// Auth is required.
	r = httptest.NewRecorder()
	app.ServeHTTP(r, httptest.NewRequest("PATCH", "/v1/playlists/"+p.ID, nil))
	if r.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated update must be 401, got %d", r.Code)
	}

	// Unknown playlist.
	r = playlistRequest(t, app, "PATCH", "/v1/playlists/pl_missing", `{"name":"x"}`, "test-key")
	if r.Code != http.StatusNotFound {
		t.Fatalf("missing playlist must be 404, got %d", r.Code)
	}
}

func TestPlaylistRemoveTrackFlow(t *testing.T) {
	app := testApp(t, false)
	p := createTestPlaylist(t, app, "test-key", "Mix")

	r := playlistRequest(t, app, "POST", "/v1/playlists/"+p.ID+"/tracks", `{"track_id":"local:seed-1"}`, "test-key")
	if r.Code != http.StatusCreated {
		t.Fatalf("add track %d: %s", r.Code, r.Body.String())
	}
	p = readPlaylist(t, r)
	if p.TrackCount != 1 || p.DurationSeconds == 0 {
		t.Fatalf("expected one track with duration: %#v", p)
	}

	r = playlistRequest(t, app, "DELETE", "/v1/playlists/"+p.ID+"/tracks/local:seed-1", "", "test-key")
	if r.Code != 200 {
		t.Fatalf("remove track %d: %s", r.Code, r.Body.String())
	}
	p = readPlaylist(t, r)
	if p.TrackCount != 0 || len(p.Tracks) != 0 || p.DurationSeconds != 0 {
		t.Fatalf("track not removed / aggregates stale: %#v", p)
	}

	// Removing an absent track is a consistent 404.
	r = playlistRequest(t, app, "DELETE", "/v1/playlists/"+p.ID+"/tracks/local:seed-1", "", "test-key")
	if r.Code != http.StatusNotFound {
		t.Fatalf("absent track removal must be 404, got %d", r.Code)
	}

	// Foreign and missing playlists are 404 too.
	r = playlistRequest(t, app, "DELETE", "/v1/playlists/"+p.ID+"/tracks/local:seed-1", "", "other-key")
	if r.Code != http.StatusNotFound {
		t.Fatalf("foreign playlist must be 404, got %d", r.Code)
	}
	r = playlistRequest(t, app, "DELETE", "/v1/playlists/pl_missing/tracks/local:seed-1", "", "test-key")
	if r.Code != http.StatusNotFound {
		t.Fatalf("missing playlist must be 404, got %d", r.Code)
	}
}

func TestPlaylistDeleteFlow(t *testing.T) {
	app := testApp(t, false)
	p := createTestPlaylist(t, app, "test-key", "Mix")

	r := playlistRequest(t, app, "DELETE", "/v1/playlists/"+p.ID, "", "other-key")
	if r.Code != http.StatusNotFound {
		t.Fatalf("foreign delete must be 404, got %d", r.Code)
	}

	r = playlistRequest(t, app, "DELETE", "/v1/playlists/"+p.ID, "", "test-key")
	if r.Code != http.StatusNoContent {
		t.Fatalf("delete playlist %d: %s", r.Code, r.Body.String())
	}

	r = playlistRequest(t, app, "GET", "/v1/playlists/"+p.ID, "", "test-key")
	if r.Code != http.StatusNotFound {
		t.Fatalf("deleted playlist must be 404, got %d", r.Code)
	}

	r = playlistRequest(t, app, "DELETE", "/v1/playlists/"+p.ID, "", "test-key")
	if r.Code != http.StatusNotFound {
		t.Fatalf("second delete must be 404, got %d", r.Code)
	}

	// The deletion must persist in the store file.
	st, err := NewStore(app.cfg.StorePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := st.GetPlaylist(apiKeyUserID("test-key"), p.ID); ok {
		t.Fatal("playlist must be gone from the persisted store")
	}
}
