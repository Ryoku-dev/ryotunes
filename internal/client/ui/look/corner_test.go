package look

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func resetDesktopCornerState(t *testing.T) {
	t.Helper()
	desktopCorners.mu.Lock()
	cancel := desktopCorners.cancel
	desktopCorners.base = ""
	desktopCorners.path = ""
	desktopCorners.dp = DefaultCornerDp
	desktopCorners.ok = false
	desktopCorners.loaded = false
	desktopCorners.selected = false
	desktopCorners.cancel = nil
	desktopCorners.watchSeq++
	desktopCorners.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	t.Cleanup(func() { WatchDesktopCorner(context.Background(), false, nil) })
}

func waitDesktopCorner(t *testing.T, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if dp, ok := DesktopCorner(); ok && dp == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	dp, ok := DesktopCorner()
	t.Fatalf("DesktopCorner = %d, %v; want %d, true", dp, ok, want)
}

func writeDesktopCornerFile(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "desktop.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestReadDesktopCornerOverridePrecedence(t *testing.T) {
	path := writeDesktopCornerFile(t, `{
		"desktop": {
			"appearance": {"rounding": 12},
			"appOverrides": [
				{"class": "other", "rounding": 4},
				{"class": "Ryotunes", "rounding": -1},
				{"class": "ryotunes", "rounding": 28},
				{"class": "ryotunes", "rounding": 3}
			]
		}
	}`)
	if dp, ok := readDesktopCorner(path); !ok || dp != 28 {
		t.Fatalf("readDesktopCorner = %d, %v; want 28, true", dp, ok)
	}
}

func TestReadDesktopCornerClampsAndFallsBack(t *testing.T) {
	t.Run("clamp override", func(t *testing.T) {
		path := writeDesktopCornerFile(t, `{"desktop":{"appOverrides":[{"class":"ryotunes","rounding":80}],"appearance":{"rounding":6}}}`)
		if dp, ok := readDesktopCorner(path); !ok || dp != 32 {
			t.Fatalf("readDesktopCorner = %d, %v; want 32, true", dp, ok)
		}
	})
	t.Run("negative override uses appearance", func(t *testing.T) {
		path := writeDesktopCornerFile(t, `{"desktop":{"appOverrides":[{"class":"ryotunes","rounding":-1}],"appearance":{"rounding":18}}}`)
		if dp, ok := readDesktopCorner(path); !ok || dp != 18 {
			t.Fatalf("readDesktopCorner = %d, %v; want 18, true", dp, ok)
		}
	})
	t.Run("missing key", func(t *testing.T) {
		path := writeDesktopCornerFile(t, `{"desktop":{"appearance":{}}}`)
		if dp, ok := readDesktopCorner(path); ok || dp != DefaultCornerDp {
			t.Fatalf("readDesktopCorner = %d, %v; want %d, false", dp, ok, DefaultCornerDp)
		}
	})
	t.Run("missing file", func(t *testing.T) {
		if dp, ok := readDesktopCorner(filepath.Join(t.TempDir(), "missing.json")); ok || dp != DefaultCornerDp {
			t.Fatalf("readDesktopCorner = %d, %v; want %d, false", dp, ok, DefaultCornerDp)
		}
	})
}

func TestWatchDesktopCornerRefreshesCachedValue(t *testing.T) {
	resetDesktopCornerState(t)
	config := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", config)
	directory := filepath.Join(config, "ryoku")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "desktop.json")
	if err := os.WriteFile(path, []byte(`{"desktop":{"appearance":{"rounding":9}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if dp, ok := DesktopCorner(); !ok || dp != 9 {
		t.Fatalf("initial DesktopCorner = %d, %v; want 9, true", dp, ok)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	changed := make(chan struct{}, 1)
	WatchDesktopCorner(ctx, true, func() {
		select {
		case changed <- struct{}{}:
		default:
		}
	})
	select {
	case <-changed:
	case <-time.After(2 * time.Second):
		t.Fatal("desktop corner watcher did not arm")
	}
	if err := os.WriteFile(path, []byte(`{"desktop":{"appearance":{"rounding":21}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	select {
	case <-changed:
	case <-time.After(2 * time.Second):
		t.Fatal("desktop corner watcher did not invalidate")
	}
	waitDesktopCorner(t, 21)

	WatchDesktopCorner(ctx, false, nil)
	if err := os.WriteFile(path, []byte(`{"desktop":{"appearance":{"rounding":27}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	WatchDesktopCorner(ctx, true, nil)
	waitDesktopCorner(t, 27)
}

func TestWatchDesktopCornerWaitsForMissingDirectoryWithoutRearming(t *testing.T) {
	resetDesktopCornerState(t)
	config := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", config)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	changed := make(chan struct{}, 2)
	invalidate := func() {
		select {
		case changed <- struct{}{}:
		default:
		}
	}
	WatchDesktopCorner(ctx, true, invalidate)
	desktopCorners.mu.Lock()
	sequence := desktopCorners.watchSeq
	desktopCorners.mu.Unlock()
	for range 10_000 {
		WatchDesktopCorner(ctx, true, invalidate)
	}
	desktopCorners.mu.Lock()
	gotSequence := desktopCorners.watchSeq
	running := desktopCorners.cancel != nil
	desktopCorners.mu.Unlock()
	if gotSequence != sequence || !running {
		t.Fatalf("watch sequence = %d, running=%v; want %d, true", gotSequence, running, sequence)
	}
	select {
	case <-changed:
	case <-time.After(2 * time.Second):
		t.Fatal("ancestor watcher did not arm")
	}
	directory := filepath.Join(config, "ryoku")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "desktop.json")
	if err := os.WriteFile(path, []byte(`{"desktop":{"appearance":{"rounding":18}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	waitDesktopCorner(t, 18)
}
