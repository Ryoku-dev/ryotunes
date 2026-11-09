package lithic

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
)

const paletteCoalesce = 150 * time.Millisecond

// Watch follows Ryoku's palette files and an optional app palette override.
func Watch(ctx context.Context, mode func() Mode, onChange func(Roles), paletteFile ...string) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer watcher.Close()

	configHome := xdgHome("XDG_CONFIG_HOME", ".config")
	cacheHome := xdgHome("XDG_CACHE_HOME", ".cache")
	override := paletteOverride(paletteFile)
	targets := map[string]struct{}{
		filepath.Join(configHome, "ryoku", "shell.json"): {},
		filepath.Join(cacheHome, "ryoku", "colors.json"): {},
		filepath.Join(configHome, "ryoku", "theme.json"): {},
	}
	if override != "" {
		targets[override] = struct{}{}
	}
	directories := make(map[string]struct{}, len(targets))
	refreshDirectories := func() error {
		for directory := range directories {
			if info, statErr := os.Stat(directory); statErr != nil || !info.IsDir() {
				_ = watcher.Remove(directory)
				delete(directories, directory)
			}
		}
		for target := range targets {
			directory := nearestExistingDirectory(filepath.Dir(target))
			if _, exists := directories[directory]; exists {
				continue
			}
			if err := watcher.Add(directory); err != nil {
				return err
			}
			directories[directory] = struct{}{}
		}
		return nil
	}
	if err := refreshDirectories(); err != nil {
		return err
	}

	load := func() Roles {
		selected := Follow
		if mode != nil {
			selected = mode()
		}
		return FromPalette(LoadPalette(selected, override))
	}
	current := load()
	var timer *time.Timer
	var fire <-chan time.Time
	reset := func() {
		if timer == nil {
			timer = time.NewTimer(paletteCoalesce)
		} else {
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(paletteCoalesce)
		}
		fire = timer.C
	}
	defer func() {
		if timer != nil {
			timer.Stop()
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			if err != nil {
				return err
			}
		case _, ok := <-watcher.Events:
			if !ok {
				return nil
			}
			if err := refreshDirectories(); err != nil {
				return err
			}
			reset()
		case <-fire:
			fire = nil
			next := load()
			if next != current {
				current = next
				if onChange != nil {
					onChange(next)
				}
			}
		}
	}
}

func nearestExistingDirectory(path string) string {
	for {
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			return path
		}
		parent := filepath.Dir(path)
		if parent == path {
			return path
		}
		path = parent
	}
}
