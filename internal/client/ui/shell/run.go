package shell

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	gioapp "gioui.org/app"

	"github.com/ryoku-dev/ryotunes/internal/client/art"
	"github.com/ryoku-dev/ryotunes/internal/client/conn"
	"github.com/ryoku-dev/ryotunes/internal/client/lithic"
	"github.com/ryoku-dev/ryotunes/internal/client/state"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/demo"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

const applicationID = "ryotunes"

func init() {
	gioapp.ID = applicationID
}

// Options configures the initial windows, route, palette mode, demo daemon, and PNG capture.
type Options struct {
	Demo          bool
	PNG           string
	Width, Height int
	Route         string
	Panel         string
	Mode          string
	Mini          bool
}

// Run owns the Ryotunes client until its windows close or a PNG has been written.
func Run(options Options) error {
	if _, err := parseMode(options.Mode); err != nil {
		return err
	}

	runtime, err := startRuntime(options)
	if err != nil {
		return err
	}
	defer runtime.Close()
	if options.PNG != "" && options.Mini {
		if options.Width <= 0 {
			options.Width = 360
		}
		if options.Height <= 0 {
			options.Height = 420
		}
	}
	preferences := runtime.store.Snapshot().Client
	if options.Width <= 0 {
		options.Width = preferences.WindowWidth
		if options.Width <= 0 {
			options.Width = 1280
		}
	}
	if options.Height <= 0 {
		options.Height = preferences.WindowHeight
		if options.Height <= 0 {
			options.Height = 800
		}
	}

	if options.PNG != "" {
		return runtime.renderPNG(options)
	}
	return runtime.runWindows(options)
}

type invalidationHub struct {
	mu      sync.Mutex
	next    uint64
	targets map[uint64]func()
}

func (h *invalidationHub) Add(target func()) func() {
	if target == nil {
		return func() {}
	}
	h.mu.Lock()
	if h.targets == nil {
		h.targets = make(map[uint64]func())
	}
	h.next++
	id := h.next
	h.targets[id] = target
	h.mu.Unlock()
	return func() {
		h.mu.Lock()
		delete(h.targets, id)
		h.mu.Unlock()
	}
}

func (h *invalidationHub) Invalidate() {
	h.mu.Lock()
	targets := make([]func(), 0, len(h.targets))
	for _, target := range h.targets {
		targets = append(targets, target)
	}
	h.mu.Unlock()
	for _, target := range targets {
		target()
	}
}

type clientRuntime struct {
	ctx      context.Context
	cancel   context.CancelFunc
	client   *conn.Client
	store    *state.Store
	art      *art.Cache
	hub      invalidationHub
	themeMu  sync.RWMutex
	theme    *lithic.Theme
	mode     string
	tempDir  string
	serveErr chan error

	paletteMu     sync.Mutex
	paletteCancel context.CancelFunc
	paletteMode   lithic.Mode
	paletteFile   string
	paletteReady  bool
}

func startRuntime(options Options) (*clientRuntime, error) {
	ctx, cancel := context.WithCancel(context.Background())
	runtime := &clientRuntime{ctx: ctx, cancel: cancel, mode: options.Mode, serveErr: make(chan error, 1)}

	socketPath := protocol.SocketPath()
	if options.Demo {
		tempDir, err := os.MkdirTemp("", "ryotunes-demo-")
		if err != nil {
			cancel()
			return nil, fmt.Errorf("create demo directory: %w", err)
		}
		runtime.tempDir = tempDir
		socketPath = filepath.Join(tempDir, "ryotunesd.sock")
		go func() { runtime.serveErr <- demo.Serve(ctx, socketPath) }()
		deadline := time.Now().Add(time.Second)
		for {
			if _, err := os.Stat(socketPath); err == nil {
				break
			}
			select {
			case err := <-runtime.serveErr:
				cancel()
				_ = os.RemoveAll(tempDir)
				return nil, fmt.Errorf("start demo daemon: %w", err)
			default:
			}
			if time.Now().After(deadline) {
				cancel()
				_ = os.RemoveAll(tempDir)
				return nil, errors.New("start demo daemon: socket did not appear")
			}
			time.Sleep(5 * time.Millisecond)
		}
	} else if _, err := os.Stat(socketPath); errors.Is(err, os.ErrNotExist) {
		if err := conn.LaunchContext(ctx); err != nil {
			cancel()
			return nil, fmt.Errorf("launch ryotunesd: %w", err)
		}
	}

	runtime.client = conn.NewPath(socketPath)
	runtime.store = state.NewEmpty()
	_ = runtime.store.LoadClientPreferences()
	runtime.art = art.New(art.Options{Invalidate: runtime.hub.Invalidate})
	runtime.theme = lithic.New(lithic.FromPalette(lithic.LoadPalette(lithic.Follow)), lithic.Shaper())
	runtime.syncPalettePreferences(runtime.store.Snapshot().Client)

	go runtime.drain()
	return runtime, nil
}

func (r *clientRuntime) Close() {
	r.cancel()
	if r.client != nil {
		r.client.Close()
	}
	if r.art != nil {
		r.art.Close()
	}
	if r.tempDir != "" {
		_ = os.RemoveAll(r.tempDir)
	}
}

func (r *clientRuntime) modeValue(preferences state.ClientPreferences) string {
	if r.mode != "" {
		return r.mode
	}
	switch preferences.Appearance {
	case "dark", "light":
		return preferences.Appearance
	}
	settings := r.store.Snapshot().Settings
	if value := settings["appearance"]; value == "dark" || value == "light" {
		return value
	}
	if value := settings["theme"]; value == "dark" || value == "light" {
		return value
	}
	return "follow"
}

func (r *clientRuntime) syncPalettePreferences(preferences state.ClientPreferences) {
	selected, _ := parseMode(r.modeValue(preferences))
	file := preferences.PaletteFile

	r.paletteMu.Lock()
	if r.paletteReady && r.paletteMode == selected && r.paletteFile == file {
		r.paletteMu.Unlock()
		return
	}
	if r.paletteCancel != nil {
		r.paletteCancel()
	}
	watchContext, cancel := context.WithCancel(r.ctx)
	r.paletteCancel = cancel
	r.paletteMode, r.paletteFile, r.paletteReady = selected, file, true
	r.paletteMu.Unlock()

	roles := lithic.FromPalette(lithic.LoadPalette(selected, file))
	r.themeMu.Lock()
	density := r.theme.Density
	r.theme = lithic.New(roles, lithic.Shaper())
	r.theme.Density = density
	r.themeMu.Unlock()
	r.hub.Invalidate()

	go func(mode lithic.Mode, paletteFile string) {
		_ = lithic.Watch(watchContext, func() lithic.Mode { return mode }, func(roles lithic.Roles) {
			r.paletteMu.Lock()
			current := r.paletteReady && r.paletteMode == mode && r.paletteFile == paletteFile
			r.paletteMu.Unlock()
			if !current {
				return
			}
			r.themeMu.Lock()
			density := r.theme.Density
			r.theme = lithic.New(roles, lithic.Shaper())
			r.theme.Density = density
			r.themeMu.Unlock()
			r.hub.Invalidate()
		}, paletteFile)
	}(selected, file)
}

func parseMode(value string) (lithic.Mode, error) {
	switch value {
	case "", "follow":
		return lithic.Follow, nil
	case "dark":
		return lithic.Dark, nil
	case "light":
		return lithic.Light, nil
	default:
		return lithic.Follow, fmt.Errorf("invalid palette mode %q: use follow, dark, or light", value)
	}
}

func (r *clientRuntime) drain() {
	for {
		select {
		case <-r.ctx.Done():
			return
		case snapshot, ok := <-r.client.Snapshots():
			if !ok {
				return
			}
			r.store.ApplySnapshot(snapshot)
		case event, ok := <-r.client.Events():
			if !ok {
				return
			}
			_ = r.store.Apply(event.Event, event.Data)
		}
	}
}
