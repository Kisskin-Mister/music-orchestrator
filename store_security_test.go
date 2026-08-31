package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// Every statement is parameterised, but the FTS5 MATCH argument is a
// mini-language of its own: quotes, `*`, `^` and `NEAR` all mean something.
// Unescaped input there could change what the query matches, so it is stripped.
func TestFTSQueryStripsOperators(t *testing.T) {
	for _, in := range []string{
		`" OR 1=1 --`,
		`title:secret`,
		`^admin`,
		`a* NEAR/5 b`,
		`(x)`,
	} {
		got := ftsQuery(in)
		for _, bad := range []string{`":`, `*"`, `^`, `(`, `)`} {
			if strings.Contains(strings.ReplaceAll(got, `"*`, ""), bad) {
				t.Errorf("ftsQuery(%q) = %q — leaked operator %q", in, got, bad)
			}
		}
	}
}

// A crafted search string must return nothing rather than everything, and must
// never take the query down.
func TestSearchRejectsInjection(t *testing.T) {
	st, err := NewStore(filepath.Join(t.TempDir(), "store.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err := st.AddFavorite("owner", Track{ID: "local:1", Title: "Secret Song", Artist: "Artist"}); err != nil {
		t.Fatal(err)
	}
	for _, probe := range []string{`" OR "" = "`, `'; DROP TABLE favorites; --`, `*`} {
		if got := st.SearchLibrary("owner", probe, 50); len(got) != 0 {
			t.Errorf("probe %q returned %d rows, expected none", probe, len(got))
		}
	}
	// The table must still be there and searchable.
	if got := st.SearchLibrary("owner", "secret", 50); len(got) != 1 {
		t.Fatalf("legitimate search returned %d rows, want 1", len(got))
	}
}

// Owner scoping must hold in SQL, not only in the handler.
func TestFavoritesAreScopedToOwner(t *testing.T) {
	st, err := NewStore(filepath.Join(t.TempDir(), "store.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err := st.AddFavorite("alice", Track{ID: "local:1", Title: "Alice track"}); err != nil {
		t.Fatal(err)
	}
	if got := st.ListFavorites("bob"); len(got) != 0 {
		t.Fatalf("bob sees %d of alice's favorites", len(got))
	}
	if got := st.SearchLibrary("bob", "Alice", 50); len(got) != 0 {
		t.Fatalf("bob found %d of alice's tracks via search", len(got))
	}
}
