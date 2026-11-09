package look

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"
	"github.com/ryoku-dev/ryotunes/internal/client/state"
)

// DefaultCornerDp is the Lithic Chisel cut used when no desktop preference exists.
const DefaultCornerDp = 16

type desktopCornerState struct {
	mu       sync.Mutex
	base     string
	path     string
	dp       int
	ok       bool
	loaded   bool
	selected bool
	cancel   context.CancelFunc
	watchSeq uint64
}

var desktopCorners = desktopCornerState{dp: DefaultCornerDp}

// DesktopCorner returns the cached Ryoku window corner setting. Missing or
// unusable settings return the Chisel default with ok false.
func DesktopCorner() (dp int, ok bool) {
	desktopCorners.mu.Lock()
	if desktopCorners.loaded || desktopCorners.selected {
		dp, ok = desktopCorners.dp, desktopCorners.ok
		desktopCorners.mu.Unlock()
		return dp, ok
	}
	path := ensureDesktopCornerPathLocked()
	desktopCorners.mu.Unlock()

	dp, ok = readDesktopCorner(path)
	desktopCorners.mu.Lock()
	if desktopCorners.path == path {
		desktopCorners.dp, desktopCorners.ok, desktopCorners.loaded = dp, ok, true
	}
	desktopCorners.mu.Unlock()
	return dp, ok
}

// WatchDesktopCorner arms an inotify-backed directory watch only while the
// Desktop preset is selected. Repeated calls with the same state are no-ops.
func WatchDesktopCorner(ctx context.Context, selected bool, invalidate func()) {
	desktopCorners.mu.Lock()
	if !selected {
		desktopCorners.selected = false
		cancel := desktopCorners.cancel
		if cancel != nil {
			desktopCorners.cancel = nil
			desktopCorners.watchSeq++
		}
		desktopCorners.mu.Unlock()
		if cancel != nil {
			cancel()
		}
		return
	}
	if desktopCorners.selected && desktopCorners.cancel != nil {
		desktopCorners.mu.Unlock()
		return
	}
	desktopCorners.selected = true
	path := ensureDesktopCornerPathLocked()
	watchCtx, cancel := context.WithCancel(ctx)
	desktopCorners.cancel = cancel
	desktopCorners.watchSeq++
	sequence := desktopCorners.watchSeq
	desktopCorners.mu.Unlock()
	go watchDesktopCorner(watchCtx, path, sequence, invalidate)
}

func ensureDesktopCornerPathLocked() string {
	if desktopCorners.path != "" {
		return desktopCorners.path
	}
	desktopCorners.base = desktopCornerBase()
	desktopCorners.path = filepath.Join(desktopCorners.base, "ryoku", "desktop.json")
	return desktopCorners.path
}

func desktopCornerBase() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base != "" && filepath.IsAbs(base) {
		return base
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".config"
	}
	return filepath.Join(home, ".config")
}

type desktopCornerDocument struct {
	Desktop struct {
		AppOverrides []struct {
			Class    string `json:"class"`
			Rounding *int   `json:"rounding"`
		} `json:"appOverrides"`
		Appearance struct {
			Rounding *int `json:"rounding"`
		} `json:"appearance"`
	} `json:"desktop"`
}

func readDesktopCorner(path string) (int, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return DefaultCornerDp, false
	}
	var document desktopCornerDocument
	if json.Unmarshal(data, &document) != nil {
		return DefaultCornerDp, false
	}
	for _, override := range document.Desktop.AppOverrides {
		if !strings.EqualFold(strings.TrimSpace(override.Class), "ryotunes") || override.Rounding == nil || *override.Rounding < 0 {
			continue
		}
		return min(*override.Rounding, state.CornerMaxDp), true
	}
	if rounding := document.Desktop.Appearance.Rounding; rounding != nil && *rounding >= 0 {
		return min(*rounding, state.CornerMaxDp), true
	}
	return DefaultCornerDp, false
}

func watchDesktopCorner(ctx context.Context, path string, sequence uint64, invalidate func()) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		clearDesktopCornerWatcher(sequence)
		return
	}
	defer func() {
		_ = watcher.Close()
		clearDesktopCornerWatcher(sequence)
	}()

	watched := ""
	arm := func() bool {
		directory := nearestExistingDirectory(filepath.Dir(path))
		if directory == "" {
			return false
		}
		if directory == watched {
			return true
		}
		if watched != "" {
			_ = watcher.Remove(watched)
		}
		if err := watcher.Add(directory); err != nil {
			watched = ""
			return false
		}
		watched = directory
		return true
	}
	if !arm() {
		return
	}
	refreshDesktopCorner(path, invalidate, true)

	for {
		select {
		case <-ctx.Done():
			return
		case _, open := <-watcher.Errors:
			if !open {
				return
			}
		case event, open := <-watcher.Events:
			if !open {
				return
			}
			if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename|fsnotify.Remove|fsnotify.Chmod) == 0 {
				continue
			}
			name := filepath.Clean(event.Name)
			target := filepath.Clean(path)
			targetDir := filepath.Dir(target)
			previous := watched
			if !arm() {
				continue
			}
			if name == target || name == targetDir || watched != previous {
				refreshDesktopCorner(path, invalidate, false)
			}
		}
	}
}

func nearestExistingDirectory(path string) string {
	for {
		info, err := os.Stat(path)
		if err == nil && info.IsDir() {
			return path
		}
		parent := filepath.Dir(path)
		if parent == path {
			return ""
		}
		path = parent
	}
}

func refreshDesktopCorner(path string, invalidate func(), force bool) {
	dp, ok := readDesktopCorner(path)
	desktopCorners.mu.Lock()
	changed := !desktopCorners.loaded || desktopCorners.dp != dp || desktopCorners.ok != ok
	desktopCorners.dp, desktopCorners.ok, desktopCorners.loaded = dp, ok, true
	desktopCorners.mu.Unlock()
	if invalidate != nil && (force || changed) {
		invalidate()
	}
}

func clearDesktopCornerWatcher(sequence uint64) {
	desktopCorners.mu.Lock()
	if desktopCorners.watchSeq != sequence {
		desktopCorners.mu.Unlock()
		return
	}
	cancel := desktopCorners.cancel
	desktopCorners.cancel = nil
	desktopCorners.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}
