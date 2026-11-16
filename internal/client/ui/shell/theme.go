package shell

import (
	"fmt"
	"strings"
	"time"

	"gioui.org/layout"
	"gioui.org/op"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
	libtheme "github.com/ryoku-dev/ryoku/docs/lithic/library/go/theme"
	"github.com/ryoku-dev/ryotunes/internal/client/state"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
)

func clientDensity(preferences state.ClientPreferences) libtheme.Density {
	if preferences.Density == "compact" {
		return libtheme.Compact
	}
	return libtheme.Comfortable
}

// applyClientTheme brings the theme up to date on the window's goroutine: it
// swaps in the desktop reading the watcher left (theme.NewFor on the same
// shaper, which also carries the desktop's reduced motion) and the density.
func (r *clientRuntime) applyClientTheme(preferences state.ClientPreferences) *look.Theme {
	r.themeMu.Lock()
	if r.pendingDesktop != nil {
		r.theme.SetDesktop(*r.pendingDesktop)
		r.pendingDesktop = nil
	}
	r.theme.Density = clientDensity(preferences)
	theme := r.theme
	r.themeMu.Unlock()
	return theme
}

// desktopRetry is how often a desktop reading the window has not taken yet
// asks again for a frame, and desktopRetryFor how long it keeps asking: Gio
// drops a window Invalidate that lands while a frame is under way.
const (
	desktopRetry    = 16 * time.Millisecond
	desktopRetryFor = 2 * time.Second
)

// offerDesktop leaves a new desktop reading for the window's goroutine
// (applyClientTheme) and asks for the frame that takes it. It is called on
// the watcher's goroutine. Until a frame has taken the reading it asks
// again every desktopRetry, for at most desktopRetryFor, so a reading that
// arrived while a frame was being drawn is not left waiting for the next
// unrelated frame; once taken, nothing more is asked.
func (r *clientRuntime) offerDesktop(desktop palette.Desktop) {
	r.themeMu.Lock()
	r.pendingDesktop = &desktop
	retrying := r.desktopRetrying
	r.desktopRetrying = true
	r.themeMu.Unlock()
	r.hub.Invalidate()
	if retrying {
		return
	}
	go func() {
		ticker := time.NewTicker(desktopRetry)
		defer ticker.Stop()
		deadline := time.Now().Add(desktopRetryFor)
		for {
			select {
			case <-r.ctx.Done():
				r.themeMu.Lock()
				r.desktopRetrying = false
				r.themeMu.Unlock()
				return
			case now := <-ticker.C:
				r.themeMu.Lock()
				pending := r.pendingDesktop != nil && now.Before(deadline)
				if !pending {
					r.desktopRetrying = false
				}
				r.themeMu.Unlock()
				if !pending {
					return
				}
				r.hub.Invalidate()
			}
		}
	}()
}

// themeFrameEnd asks for the next frame when a desktop reading arrived while
// this frame was being drawn, so it is worn on the very next frame. Call it
// last in a frame; a frame at rest asks for nothing.
func (r *clientRuntime) themeFrameEnd(gtx layout.Context) {
	r.themeMu.Lock()
	pending := r.pendingDesktop != nil
	r.themeMu.Unlock()
	if pending {
		gtx.Execute(op.InvalidateCmd{})
	}
}

// parseMode checks --mode, which forces the mode as LITHIC_MODE does for
// demo renders and tests.
func parseMode(value string) error {
	switch value {
	case "", "follow", "dark", "light":
		return nil
	default:
		return fmt.Errorf("invalid palette mode %q: use follow, dark, or light", value)
	}
}

// reconcileProvider lays Ryotunes' music colours over the desktop's roles:
// the provider source, then the cover tint when the listener asks for it,
// then the accent.
func (s *uiShell) reconcileProvider(gtx layout.Context, e *env.Env) {
	snapshot := e.Store.Snapshot()
	if snapshot.Provider != e.Th.Src.Provider {
		e.Th.SetProvider(gtx, snapshot.Provider)
	}

	roles := e.Th.LoadedRoles()
	targetRoles := roles
	targetAccent := look.AccentFromDesktop(roles)
	setScheme := true
	switch snapshot.Client.AccentMode() {
	case state.AccentSource:
		targetAccent = look.AccentFromSource(snapshot.Provider, roles)
	case state.AccentArt:
		if snapshot.Playback.Now != nil && snapshot.Playback.Now.Thumbnail != nil {
			thumbnail := strings.TrimSpace(*snapshot.Playback.Now.Thumbnail)
			if thumbnail != "" && e.Art != nil {
				coverPalette, ready := e.Art.Palette(thumbnail)
				if !ready {
					setScheme = false
				} else {
					targetRoles, targetAccent = e.Th.ArtScheme(coverPalette, snapshot.Client.CoverTintOn())
				}
			}
		}
	}
	if setScheme {
		e.Th.SetScheme(gtx, targetRoles, targetAccent)
	}
	e.Th.Animate(gtx)
}
