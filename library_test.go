package main

import (
	"fmt"
	"testing"
	"time"
)

func seedLibrary(t *testing.T, app *App, n int) {
	t.Helper()
	files := make([]LocalFile, n)
	for i := range files {
		id := fmt.Sprintf("local:t%05d", i)
		// Альбом принадлежит одному исполнителю, как в жизни: группировка идёт
		// по паре, потому что одноимённые альбомы у разных артистов — не один
		// альбом.
		album := i % 120
		files[i] = LocalFile{TrackID: id, Path: fmt.Sprintf("/m/%d.mp3", i), Track: Track{
			ID: id, ProviderID: "local", Title: fmt.Sprintf("Song %d", i),
			Artist: fmt.Sprintf("Artist %d", album%50), Album: fmt.Sprintf("Album %d", album),
			DurationSeconds: 180,
		}}
	}
	if _, _, err := app.store.BulkImportLocalFiles("owner", files); err != nil {
		t.Fatal(err)
	}
}

// The library must stay responsive at the scale the importer now makes easy.
func TestLibraryPagingStaysFast(t *testing.T) {
	app := testApp(t, false)
	seedLibrary(t, app, 5000)

	start := time.Now()
	page := app.store.LibraryTracks("owner", "", "", 60, 0)
	elapsed := time.Since(start)

	if page.Total != 5000 {
		t.Fatalf("total = %d, want 5000", page.Total)
	}
	if len(page.Tracks) != 60 {
		t.Fatalf("page returned %d tracks, want 60 — the whole library must not be sent at once", len(page.Tracks))
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("first page took %v", elapsed)
	}
	t.Logf("страница из 60 по 5000 трекам: %v", elapsed.Round(time.Microsecond))
}

func TestLibraryGroupsAndFacets(t *testing.T) {
	app := testApp(t, false)
	seedLibrary(t, app, 600)

	if artists := app.store.LibraryArtists("owner", ""); len(artists) != 50 {
		t.Fatalf("artists = %d, want 50", len(artists))
	}
	if albums := app.store.LibraryAlbums("owner", ""); len(albums) != 120 {
		t.Fatalf("albums = %d, want 120", len(albums))
	}
	page := app.store.LibraryTracks("owner", "", "", 10, 0)
	if page.Sources["local"] != 600 {
		t.Fatalf("source facet = %v, want local:600", page.Sources)
	}
}

// Selecting one source must not empty the other chips, or there is no way back.
func TestLibraryFacetsIgnoreOwnSourceFilter(t *testing.T) {
	app := testApp(t, false)
	seedLibrary(t, app, 20)
	if _, err := app.store.AddFavorite("owner", Track{ID: "youtube_stream:x", ProviderID: "youtube_stream", Title: "Remote"}); err != nil {
		t.Fatal(err)
	}
	page := app.store.LibraryTracks("owner", "", "local", 10, 0)
	if page.Total != 20 {
		t.Fatalf("filtered total = %d, want 20", page.Total)
	}
	if page.Sources["youtube_stream"] != 1 {
		t.Fatalf("other sources disappeared from facets: %v", page.Sources)
	}
}

func TestLibrarySearchNarrowsResults(t *testing.T) {
	app := testApp(t, false)
	seedLibrary(t, app, 300)
	page := app.store.LibraryTracks("owner", "Song 42", "", 60, 0)
	if page.Total == 0 || page.Total >= 300 {
		t.Fatalf("search matched %d of 300 — expected a narrowed set", page.Total)
	}
}
