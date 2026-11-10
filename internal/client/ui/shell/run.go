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

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
	"github.com/ryoku-dev/ryotunes/internal/client/art"
	"github.com/ryoku-dev/ryotunes/internal/client/conn"
	"github.com/ryoku-dev/ryotunes/internal/client/spectrum"
	"github.com/ryoku-dev/ryotunes/internal/client/state"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/demo"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

const applicationID = "ryotunes"

func init() {
	gioapp.ID = applicationID
}

// Options configures the initial windows, route, palette mode, demo daemon, and PNG capture.
type Options struct {
	Demo          bool
	DemoPlaying   bool
	DemoEmpty     bool
	PNG           string
	Width, Height int
	Route         string
	Query         string
	Panel         string
	Mode          string
	Mini          bool
}

// Run owns the Ryotunes client until its windows close or a PNG has been written.
func Run(options Options) error {
	if err := parseMode(options.Mode); err != nil {
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
	spectrum *spectrum.Feed
	scope    *spectrum.Scope
	hub      invalidationHub
	themeMu  sync.Mutex
	theme    *look.Theme
	mode     string
	tempDir  string
	serveErr chan error
	quit     chan struct{}
	quitOnce sync.Once

	paletteAppearance string
	paletteFile       string
	pendingRoles      *palette.Roles
}

func startRuntime(options Options) (*clientRuntime, error) {
	ctx, cancel := context.WithCancel(context.Background())
	runtime := &clientRuntime{
		ctx: ctx, cancel: cancel, mode: options.Mode,
		serveErr: make(chan error, 1), quit: make(chan struct{}),
	}

	socketPath := protocol.SocketPath()
	if options.Demo {
		tempDir, err := os.MkdirTemp("", "ryotunes-demo-")
		if err != nil {
			cancel()
			return nil, fmt.Errorf("create demo directory: %w", err)
		}
		runtime.tempDir = tempDir
		socketPath = filepath.Join(tempDir, "ryotunesd.sock")
		go func() { runtime.serveErr <- demo.Serve(ctx, socketPath, options.DemoPlaying, options.DemoEmpty) }()
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
	// Covers are optional, the window is not: without a cache directory the
	// art cache runs in memory only.
	cacheDir := ""
	if options.Demo {
		cacheDir = filepath.Join(runtime.tempDir, "cache", "art")
	} else if options.PNG != "" {
		if tempDir, err := os.MkdirTemp("", "ryotunes-png-"); err == nil {
			runtime.tempDir = tempDir
			cacheDir = filepath.Join(tempDir, "cache", "art")
		}
	} else if root, err := os.UserCacheDir(); err == nil {
		cacheDir = filepath.Join(root, "ryotunes", "art")
	}
	runtime.art = art.New(art.Options{CacheDir: cacheDir, Invalidate: runtime.hub.Invalidate})
	if options.Demo {
		runtime.spectrum = spectrum.NewStatic(demo.SpectrumLevels())
		runtime.scope = spectrum.NewStaticScope(demo.ScopeSamples())
	} else {
		runtime.spectrum = spectrum.New(runtime.hub.Invalidate)
		runtime.scope = spectrum.NewScope(runtime.hub.Invalidate)
	}
	snapshot := runtime.store.Snapshot()
	preferences := snapshot.Client
	appearance := runtime.modeValue(preferences)
	density := clientDensity(preferences)
	runtime.theme = look.NewTheme(look.Roles(appearance, preferences.PaletteFile), density, snapshot.Provider)
	runtime.paletteAppearance, runtime.paletteFile = appearance, preferences.PaletteFile

	go look.Watch(runtime.ctx, func() (string, string) {
		preferences := runtime.store.Snapshot().Client
		return runtime.modeValue(preferences), preferences.PaletteFile
	}, func(roles palette.Roles) {
		runtime.themeMu.Lock()
		copied := roles
		runtime.pendingRoles = &copied
		runtime.themeMu.Unlock()
		runtime.hub.Invalidate()
	})
	go runtime.drain()
	return runtime, nil
}

func (r *clientRuntime) Close() {
	r.cancel()
	if r.client != nil {
		r.client.Close()
	}
	if r.spectrum != nil {
		r.spectrum.Close()
	}
	if r.scope != nil {
		r.scope.Close()
	}
	if r.art != nil {
		r.art.Close()
	}
	if r.tempDir != "" {
		_ = os.RemoveAll(r.tempDir)
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
			// select picks at random, and the snapshot an event follows has to land first
			select {
			case snapshot, ok := <-r.client.Snapshots():
				if ok {
					r.store.ApplySnapshot(snapshot)
				}
			default:
			}
			if event.Event == protocol.EventQuit {
				r.quitOnce.Do(func() { close(r.quit) })
				return
			}
			_ = r.store.Apply(event.Event, event.Data)
		}
	}
}
