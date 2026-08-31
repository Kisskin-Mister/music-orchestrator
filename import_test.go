package main

import (
	"os"
	"path/filepath"
	"testing"
)

// The import root is the whole security boundary here: without it, an admin
// session would be an arbitrary file-read primitive.
func TestImportPathStaysInsideRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "music"), 0o755); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, escape := range []string{
		"../",
		"../../etc",
		filepath.Join(root, "..", filepath.Base(outside)),
		outside,
		"music/../../..",
	} {
		if got, err := resolveImportPath(root, escape); err == nil {
			t.Errorf("escape %q was accepted and resolved to %q", escape, got)
		}
	}

	// A real subdirectory must still work.
	if _, err := resolveImportPath(root, "music"); err != nil {
		t.Fatalf("legitimate subdirectory rejected: %v", err)
	}
}

// A symlink pointing outside is the classic bypass: the string looks like a
// child of the root until you follow it.
func TestImportRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	link := filepath.Join(root, "escape")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if got, err := resolveImportPath(root, link); err == nil {
		t.Fatalf("symlink escape accepted, resolved to %q", got)
	}
}

func TestImportRequiresConfiguredRoot(t *testing.T) {
	if _, err := resolveImportPath("", "/anything"); err == nil {
		t.Fatal("scanning must be refused when no import root is configured")
	}
}

// DRM-protected files are reported rather than silently ignored, so the user
// learns why a chunk of their iTunes library did not appear.
func TestImportReportsDRMFiles(t *testing.T) {
	root := t.TempDir()
	write := func(name string, body []byte) {
		if err := os.WriteFile(filepath.Join(root, name), body, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("protected.m4p", []byte("drm"))
	write("plain.mp3", make([]byte, 4096))
	write("notes.txt", []byte("ignore me"))

	app := testApp(t, false)
	result, files := app.scanImportRoot(t.Context(), root)

	if result.Scanned != 1 || len(files) != 1 {
		t.Fatalf("scanned %d files, catalogued %d — expected exactly the mp3", result.Scanned, len(files))
	}
	if len(result.Skipped) != 1 || filepath.Ext(result.Skipped[0].Path) != ".m4p" {
		t.Fatalf("expected the .m4p to be reported, got %+v", result.Skipped)
	}
}

// Re-running a scan must not duplicate the library.
func TestImportIsIdempotent(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "song.mp3"), make([]byte, 8192), 0o600); err != nil {
		t.Fatal(err)
	}
	app := testApp(t, false)
	_, files := app.scanImportRoot(t.Context(), root)

	imported, duplicate, err := app.store.BulkImportLocalFiles("owner", files)
	if err != nil || imported != 1 || duplicate != 0 {
		t.Fatalf("first import: %d imported, %d duplicate, err %v", imported, duplicate, err)
	}
	imported, duplicate, err = app.store.BulkImportLocalFiles("owner", files)
	if err != nil || imported != 0 || duplicate != 1 {
		t.Fatalf("second import must be a no-op: %d imported, %d duplicate, err %v", imported, duplicate, err)
	}
	if got := app.store.ListFavorites("owner"); len(got) != 1 {
		t.Fatalf("library holds %d tracks after two scans, want 1", len(got))
	}
}

// One account must not be able to read another's imported files.
func TestLocalFileIsScopedToOwner(t *testing.T) {
	app := testApp(t, false)
	files := []LocalFile{{TrackID: "local:abc", Path: "/tmp/a.mp3", Track: Track{ID: "local:abc", Title: "A"}}}
	if _, _, err := app.store.BulkImportLocalFiles("alice", files); err != nil {
		t.Fatal(err)
	}
	if _, ok := app.store.LocalFilePath("bob", "local:abc"); ok {
		t.Fatal("bob can resolve alice's imported file")
	}
	if _, ok := app.store.LocalFilePath("alice", "local:abc"); !ok {
		t.Fatal("alice cannot resolve her own file")
	}
}

// The browser sends a relative path as the filename when a whole folder is
// picked ("Artist/Album/01.mp3"). Joining that onto a directory would let a
// crafted client write anywhere, so only the base name may survive.
func TestSafeUploadNameStripsPaths(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"Artist/Album/01.mp3", "01.mp3"},
		{`Artist\Album\01.mp3`, "01.mp3"},
		{"song.mp3", "song.mp3"},
		{"Альбом/трек.mp3", "трек.mp3"},
	} {
		got, ok := safeUploadName(c.in)
		if !ok || got != c.want {
			t.Errorf("safeUploadName(%q) = %q,%v — want %q", c.in, got, ok, c.want)
		}
	}
	for _, bad := range []string{"../../etc/passwd", "..", ".", "", "   ", "../secret.mp3"} {
		if got, ok := safeUploadName(bad); ok {
			t.Errorf("safeUploadName(%q) accepted, resolved to %q", bad, got)
		}
	}
}
