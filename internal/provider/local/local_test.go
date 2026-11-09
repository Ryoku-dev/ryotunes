package local

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func TestIDRoundTrips(t *testing.T) {
	t.Parallel()
	path := "/music/Artist - Song.flac"
	if got, ok := PathFromID(SongID(path)); !ok || got != path {
		t.Fatalf("PathFromID(SongID()) = %q, %v", got, ok)
	}
	key := "artist--album"
	if got, ok := AlbumKeyFromID(AlbumID(key)); !ok || got != key {
		t.Fatalf("AlbumKeyFromID(AlbumID()) = %q, %v", got, ok)
	}
	artist := "Tyler, The Creator"
	if got, ok := ArtistNameFromID(ArtistID(artist)); !ok || got != artist {
		t.Fatalf("ArtistNameFromID(ArtistID()) = %q, %v", got, ok)
	}
	if _, ok := PathFromID("LOCALALBUM:x"); ok {
		t.Fatal("album ID was accepted as a song ID")
	}
}

func TestCanonicalMusicFolderErrors(t *testing.T) {
	file := filepath.Join(t.TempDir(), "song.mp3")
	if err := os.WriteFile(file, []byte("audio"), 0o600); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		path string
		want string
	}{
		{"", "Choose a music folder."},
		{filepath.Join(t.TempDir(), "missing"), "That music folder no longer exists or cannot be accessed."},
		{file, "Choose a directory, not a file."},
		{string(filepath.Separator), "For safety, choose a music folder instead of the filesystem root."},
	}
	for _, test := range cases {
		if _, err := canonicalMusicFolder(test.path); err == nil || err.Error() != test.want {
			t.Errorf("canonicalMusicFolder(%q) error = %v, want %q", test.path, err, test.want)
		}
	}
}

func TestWalkAudioFilteringRules(t *testing.T) {
	root := t.TempDir()
	write := func(path string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("audio"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(root, ".hidden.mp3"))
	write(filepath.Join(root, "UPPER.FLAC"))
	write(filepath.Join(root, "notes.txt"))

	deep := root
	for depth := 1; depth <= maxDepth; depth++ {
		deep = filepath.Join(deep, "d")
	}
	write(filepath.Join(deep, "depth-ten.ogg"))
	tooDeep := filepath.Join(deep, "eleven")
	write(filepath.Join(tooDeep, "depth-eleven.mp3"))

	external := t.TempDir()
	write(filepath.Join(external, "linked.opus"))
	if err := os.Symlink(external, filepath.Join(root, "linked")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(root, filepath.Join(root, "cycle")); err != nil {
		t.Fatal(err)
	}

	var names []string
	err := walkAudio(context.Background(), root, 0, make(map[string]struct{}), func(path string, _ os.FileInfo) error {
		names = append(names, filepath.Base(path))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(names)
	want := []string{".hidden.mp3", "UPPER.FLAC", "depth-ten.ogg", "linked.opus"}
	if len(names) != len(want) {
		t.Fatalf("walked %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("walked %v, want %v", names, want)
		}
	}
}

func TestFilenameAndAlbumKeys(t *testing.T) {
	t.Parallel()
	artist, title := splitFilename("Artist - Song - Live")
	if artist != "Artist" || title != "Song - Live" {
		t.Fatalf("splitFilename = %q, %q", artist, title)
	}
	if albumKey("Daft Punk", "Discovery / Deluxe") != "daft-punk--discovery---deluxe" {
		t.Fatal("album key did not preserve the stable sanitization contract")
	}
	if folderKey("/a/Downloads", "Downloads") == folderKey("/b/Downloads", "Downloads") {
		t.Fatal("same-named folders must have distinct album keys")
	}
}
