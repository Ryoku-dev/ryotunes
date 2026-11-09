package shell

import (
	"fmt"
	"strings"

	"gioui.org/layout"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/anim"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
	libtheme "github.com/ryoku-dev/ryoku/docs/lithic/library/go/theme"
	"github.com/ryoku-dev/ryotunes/internal/client/state"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
)

func (r *clientRuntime) modeValue(preferences state.ClientPreferences) string {
	if r.mode != "" {
		return r.mode
	}
	switch preferences.Appearance {
	case "dark", "light":
		return preferences.Appearance
	default:
		return "follow"
	}
}

func clientDensity(preferences state.ClientPreferences) libtheme.Density {
	if preferences.Density == "compact" {
		return libtheme.Compact
	}
	return libtheme.Comfortable
}

func (r *clientRuntime) applyClientTheme(preferences state.ClientPreferences) *look.Theme {
	appearance, file := r.modeValue(preferences), preferences.PaletteFile
	var roles *palette.Roles
	if appearance != r.paletteAppearance || file != r.paletteFile {
		resolved := look.Roles(appearance, file)
		roles = &resolved
	}

	r.themeMu.Lock()
	if roles != nil {
		r.theme.SetRoles(*roles)
		r.paletteAppearance, r.paletteFile = appearance, file
		r.pendingRoles = nil
	} else if r.pendingRoles != nil {
		r.theme.SetRoles(*r.pendingRoles)
		r.pendingRoles = nil
	}
	r.theme.Density = clientDensity(preferences)
	theme := r.theme
	r.themeMu.Unlock()
	anim.SetReduce(preferences.ReduceMotion)
	return theme
}

func parseMode(value string) error {
	switch value {
	case "", "follow", "dark", "light":
		return nil
	default:
		return fmt.Errorf("invalid palette mode %q: use follow, dark, or light", value)
	}
}

func (s *uiShell) reconcileProvider(gtx layout.Context, e *env.Env) {
	snapshot := e.Store.Snapshot()
	if snapshot.Provider != e.Th.Src.Provider {
		e.Th.SetProvider(gtx, snapshot.Provider)
	}

	roles := e.Th.LoadedRoles()
	target := look.AccentFromDesktop(roles)
	setAccent := true
	switch snapshot.Client.AccentMode() {
	case state.AccentSource:
		target = look.AccentFromSource(snapshot.Provider, roles)
	case state.AccentArt:
		if snapshot.Playback.Now != nil && snapshot.Playback.Now.Thumbnail != nil {
			thumbnail := strings.TrimSpace(*snapshot.Playback.Now.Thumbnail)
			if thumbnail != "" && e.Art != nil {
				sample, ready := e.Art.Sample(thumbnail)
				if !ready {
					setAccent = false
				} else if sample.A != 0 {
					target = e.Th.ArtAccent(sample)
				}
			}
		}
	}
	if setAccent {
		e.Th.SetAccent(gtx, target)
	}

	preset := snapshot.Client.CornerPreset()
	look.WatchDesktopCorner(s.runtime.ctx, preset == state.CornersDesktop, s.runtime.hub.Invalidate)
	corner := look.DefaultCornerDp
	if fixed, ok := state.CornerPresetDp(preset); ok {
		corner = fixed
	} else {
		switch preset {
		case state.CornersCustom:
			corner = min(max(snapshot.Client.CornerDp, 0), state.CornerMaxDp)
		case state.CornersDesktop:
			corner, _ = look.DesktopCorner()
		}
	}
	e.Th.SetCorner(gtx, unit.Dp(corner))
	e.Th.Animate(gtx)
}
