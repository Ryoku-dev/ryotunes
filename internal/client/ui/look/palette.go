package look

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
)

// Roles resolves a live or caller-supplied Ryoku palette.
func Roles(appearance, file string) palette.Roles {
	mode := paletteMode(appearance)
	fallback := palette.Load(mode)
	if file == "" {
		return palette.FromPalette(fallback)
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return palette.FromPalette(fallback)
	}
	var object map[string]any
	if json.Unmarshal(data, &object) != nil {
		return palette.FromPalette(fallback)
	}
	roles := paletteFileRoles(object)
	if len(roles) == 0 {
		return palette.FromPalette(fallback)
	}
	dark := true
	forced := mode != palette.Follow
	if mode == palette.Light {
		dark = false
	} else if !forced {
		if surface, ok := roles["surface"]; ok {
			luma := (0.299*float64(surface.R) + 0.587*float64(surface.G) + 0.114*float64(surface.B)) / 255
			dark = luma <= 0.5
		}
	}
	return palette.FromPalette(palette.Palette{Roles: roles, Dark: dark, Forced: forced, Source: "wallpaper"})
}

// Watch follows the Ryoku palette and an optional caller-supplied palette file.
func Watch(ctx context.Context, current func() (appearance, file string), changed func(palette.Roles)) {
	libraryEvents := make(chan struct{}, 1)
	go func() {
		_ = palette.Watch(ctx, func() palette.Mode {
			appearance, _ := current()
			return paletteMode(appearance)
		}, func(palette.Palette) {
			select {
			case libraryEvents <- struct{}{}:
			default:
			}
		})
	}()

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		watcher = nil
	} else {
		defer watcher.Close()
	}
	watchedDir := ""
	armFile := func() {
		if watcher == nil {
			return
		}
		_, file := current()
		directory := ""
		if file != "" {
			directory = filepath.Dir(file)
		}
		if directory == watchedDir {
			return
		}
		if watchedDir != "" {
			_ = watcher.Remove(watchedDir)
		}
		watchedDir = ""
		if directory != "" && watcher.Add(directory) == nil {
			watchedDir = directory
		}
	}
	armFile()

	appearance, file := current()
	last := Roles(appearance, file)
	var fileEvents <-chan fsnotify.Event
	var fileErrors <-chan error
	if watcher != nil {
		fileEvents, fileErrors = watcher.Events, watcher.Errors
	}
	var timer *time.Timer
	var deadline <-chan time.Time
	queue := func() {
		if timer == nil {
			timer = time.NewTimer(150 * time.Millisecond)
		} else {
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(150 * time.Millisecond)
		}
		deadline = timer.C
	}

	for {
		select {
		case <-ctx.Done():
			if timer != nil {
				timer.Stop()
			}
			return
		case <-libraryEvents:
			queue()
		case <-fileEvents:
			queue()
		case <-fileErrors:
			queue()
		case <-deadline:
			deadline = nil
			armFile()
			appearance, file = current()
			next := Roles(appearance, file)
			if next != last {
				last = next
				changed(next)
			}
		}
	}
}

func paletteMode(appearance string) palette.Mode {
	switch strings.ToLower(appearance) {
	case "dark":
		return palette.Dark
	case "light":
		return palette.Light
	default:
		return palette.Follow
	}
}

func paletteFileRoles(object map[string]any) map[string]color.NRGBA {
	roles := make(map[string]color.NRGBA)
	for name, raw := range object {
		value, ok := raw.(string)
		if !ok || (len(value) != 7 && len(value) != 9) || value[0] != '#' {
			continue
		}
		bytes, err := hex.DecodeString(value[1:])
		if err != nil {
			continue
		}
		alpha := uint8(255)
		if len(bytes) == 4 {
			alpha = bytes[3]
		}
		roles[name] = color.NRGBA{R: bytes[0], G: bytes[1], B: bytes[2], A: alpha}
	}
	return roles
}
