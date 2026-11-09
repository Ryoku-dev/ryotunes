package browsercookies

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStoreKindPathFiltering(t *testing.T) {
	tests := []struct {
		path  string
		name  string
		depth int
		kind  Kind
		ok    bool
	}{
		{"/home/me/.mozilla/firefox/a/cookies.sqlite", "cookies.sqlite", 3, Gecko, true},
		{"/home/me/.mozilla/a/b/c/d/e/cookies.sqlite", "cookies.sqlite", 5, Gecko, false},
		{"/home/me/.config/google-chrome/Default/Cookies", "Cookies", 3, Chromium, true},
		{"/home/me/.config/chromium/Profile 1/Network/Cookies", "Cookies", 4, Chromium, true},
		{"/home/me/.config/random/Profile 1/Cookies", "Cookies", 3, Chromium, false},
		{"/home/me/.config/google-chrome/Default/Cookies", "Cookies", 6, Chromium, false},
	}
	for _, test := range tests {
		kind, ok := storeKind(test.path, test.name, test.depth)
		if ok != test.ok || ok && kind != test.kind {
			t.Fatalf("storeKind(%q): got (%v,%v), want (%v,%v)", test.path, kind, ok, test.kind, test.ok)
		}
	}
}

func TestWalkStoresRefusesSymlinkDirectories(t *testing.T) {
	root := t.TempDir()
	real := filepath.Join(root, "real", "Default")
	if err := os.MkdirAll(real, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(real, "Cookies"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "real"), filepath.Join(root, "linked")); err != nil {
		t.Fatal(err)
	}
	var found []string
	walkStores(root, 0, 5, func(path string, _ os.DirEntry, _ int) { found = append(found, path) })
	if len(found) != 1 || found[0] != filepath.Join(real, "Cookies") {
		t.Fatalf("walked through symlink: %v", found)
	}
}
