package browsercookies

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestKindPersistsByNameAndReadsTheEarlierInteger(t *testing.T) {
	raw, err := json.Marshal(SessionSource{Kind: Chromium, Path: "/x/Cookies"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"kind":"chromium"`) {
		t.Fatalf("persisted source = %s", raw)
	}
	for stored, want := range map[string]Kind{
		`{"kind":0,"path":"/p"}`:          Gecko,
		`{"kind":1,"path":"/p"}`:          Chromium,
		`{"kind":"gecko","path":"/p"}`:    Gecko,
		`{"kind":"chromium","path":"/p"}`: Chromium,
		`{"kind":7,"path":"/p"}`:          unknownKind,
		`{"kind":"opera","path":"/p"}`:    unknownKind,
	} {
		var source SessionSource
		if err := json.Unmarshal([]byte(stored), &source); err != nil {
			t.Fatalf("%s: %v", stored, err)
		}
		if source.Kind != want {
			t.Errorf("%s decoded kind %d, want %d", stored, source.Kind, want)
		}
	}
}

func TestValidateSourceAcceptsOnlyStoresDiscoverWouldFind(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	profile := filepath.Join(home, ".config", "zen", "abc.Default (release)")
	if err := os.MkdirAll(profile, 0o700); err != nil {
		t.Fatal(err)
	}
	gecko := filepath.Join(profile, "cookies.sqlite")
	writeFile(t, gecko)
	chrome := filepath.Join(home, ".config", "google-chrome", "Default", "Cookies")
	writeFile(t, chrome)

	if store, err := ValidateSource(SessionSource{Kind: Gecko, Path: gecko}); err != nil || store.Path != gecko || store.Kind != Gecko {
		t.Fatalf("real Zen store = %#v, %v", store, err)
	}
	if _, err := ValidateSource(SessionSource{Kind: Chromium, Path: chrome}); err != nil {
		t.Fatalf("real Chrome store: %v", err)
	}

	outside := filepath.Join(t.TempDir(), "cookies.sqlite")
	writeFile(t, outside)
	linked := filepath.Join(home, ".config", "zen", "linked", "cookies.sqlite")
	if err := os.MkdirAll(filepath.Dir(linked), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, linked); err != nil {
		t.Fatal(err)
	}
	linkedDir := filepath.Join(home, ".config", "zen", "dirlink")
	if err := os.Symlink(filepath.Dir(outside), linkedDir); err != nil {
		t.Fatal(err)
	}
	fifo := filepath.Join(home, ".config", "zen", "fifo", "cookies.sqlite")
	if err := os.MkdirAll(filepath.Dir(fifo), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	notes := filepath.Join(profile, "notes.txt")
	writeFile(t, notes)

	for name, source := range map[string]SessionSource{
		"symlinked file":      {Kind: Gecko, Path: linked},
		"symlinked directory": {Kind: Gecko, Path: filepath.Join(linkedDir, "cookies.sqlite")},
		"outside every root":  {Kind: Gecko, Path: outside},
		"fifo":                {Kind: Gecko, Path: fifo},
		"wrong base name":     {Kind: Gecko, Path: notes},
		"kind does not match": {Kind: Chromium, Path: gecko},
		"unknown kind":        {Kind: unknownKind, Path: gecko},
		"relative path":       {Kind: Gecko, Path: "zen/cookies.sqlite"},
		"unclean path":        {Kind: Gecko, Path: profile + "/../" + filepath.Base(profile) + "/cookies.sqlite"},
	} {
		if _, err := ValidateSource(source); !errors.Is(err, ErrInvalidSource) {
			t.Errorf("%s: error = %v, want ErrInvalidSource", name, err)
		}
	}
	if _, err := ValidateSource(SessionSource{Kind: Gecko, Path: filepath.Join(profile, "gone", "cookies.sqlite")}); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing store error = %v, want ErrNotExist", err)
	}
}

func TestReadRefusesANonRegularFileInsteadOfBlocking(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	fifo := filepath.Join(t.TempDir(), "cookies.sqlite")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := Read(Store{Kind: Gecko, Path: fifo})
		done <- err
	}()
	select {
	case err := <-done:
		if !errors.Is(err, ErrInvalidSource) {
			t.Fatalf("FIFO read error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("reading a FIFO blocked")
	}
}

func TestCopyAvoidsARuntimeDirectoryOthersCanEnter(t *testing.T) {
	runtimeDir := t.TempDir()
	tmpDir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)
	t.Setenv("TMPDIR", tmpDir)
	shared := filepath.Join(runtimeDir, "ryotunes")
	if err := os.Mkdir(shared, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(shared, 0o755); err != nil {
		t.Fatal(err)
	}

	dir, err := makeCopyDir()
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	if filepath.Dir(dir) != tmpDir {
		t.Fatalf("copy directory %s went into a runtime directory others can enter", dir)
	}
}

func TestReadCopiesIntoRuntimeDirectoryAndStaleCopiesAreSwept(t *testing.T) {
	runtimeDir := t.TempDir()
	tmpDir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)
	t.Setenv("TMPDIR", tmpDir)
	private := filepath.Join(runtimeDir, "ryotunes")

	dir, err := makeCopyDir()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(dir) != private {
		t.Fatalf("copy directory %s is not under %s", dir, private)
	}
	if info, err := os.Stat(dir); err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("copy directory mode = %v, %v", info, err)
	}

	old := time.Now().Add(-time.Hour)
	staleRuntime := filepath.Join(private, copyPrefix+"killed")
	staleTemp := filepath.Join(tmpDir, copyPrefix+"older-build")
	for _, stale := range []string{staleRuntime, staleTemp} {
		if err := os.MkdirAll(stale, 0o700); err != nil {
			t.Fatal(err)
		}
		writeFile(t, filepath.Join(stale, "cookies.db"))
		if err := os.Chtimes(stale, old, old); err != nil {
			t.Fatal(err)
		}
	}
	unrelated := filepath.Join(tmpDir, "someone-else")
	if err := os.MkdirAll(unrelated, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(unrelated, old, old); err != nil {
		t.Fatal(err)
	}

	if err := RemoveStaleCopies(); err != nil {
		t.Fatal(err)
	}
	for _, stale := range []string{staleRuntime, staleTemp} {
		if _, err := os.Stat(stale); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("stale copy %s survived: %v", stale, err)
		}
	}
	for _, kept := range []string{dir, unrelated} {
		if _, err := os.Stat(kept); err != nil {
			t.Errorf("%s was removed: %v", kept, err)
		}
	}
}

func writeFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
}
