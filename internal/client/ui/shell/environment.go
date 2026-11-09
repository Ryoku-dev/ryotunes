package shell

import (
	"context"
	"image"
	"strings"
	"time"

	"gioui.org/layout"

	"github.com/ryoku-dev/ryotunes/internal/client/state"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/model"
)

func (s *uiShell) environment(now time.Time, theme *look.Theme) *env.Env {
	snapshot := s.runtime.store.Snapshot()
	lowResource := strings.EqualFold(strings.TrimSpace(snapshot.Settings[string(model.SettingLowResourceMode)]), "true")
	motion := !snapshot.Client.ReduceMotion && !lowResource
	style := snapshot.Client.Style()
	spectrumInstalled := s.runtime.spectrum != nil && s.runtime.spectrum.Installed()
	scopeInstalled := s.runtime.scope != nil && s.runtime.scope.Installed()
	visualizerInstalled := spectrumInstalled
	if style == state.VisualizerOscilloscope || style == state.VisualizerVectorscope {
		visualizerInstalled = scopeInstalled
	}
	fx := env.Fx{
		Visualizer: snapshot.Client.VisualizerOn() && motion && visualizerInstalled,
		Wave:       snapshot.Client.WaveOn() && motion,
		Style:      style,
	}
	playing := snapshot.Playback.Now != nil && !snapshot.Playback.Paused
	spectrumStyle := style == state.VisualizerBars || style == state.VisualizerCurve
	if s.runtime.spectrum != nil {
		s.runtime.spectrum.SetActive(playing && fx.Visualizer && spectrumStyle)
	}
	if s.runtime.scope != nil {
		s.runtime.scope.SetActive(playing && fx.Visualizer && !spectrumStyle)
	}
	e := &env.Env{
		Th: theme, Overlays: &s.overlays, Store: s.runtime.store, Conn: s.runtime.client,
		Nav: s.nav, Art: s.runtime.art, Spectrum: s.runtime.spectrum, Scope: s.runtime.scope, Fx: fx, Now: now,
	}
	e.Toast = func(message string, _ env.ToastKind) {
		s.posts.Post(func() { s.placards.Push(time.Now(), message, "") })
	}
	e.Go = func(fn func(context.Context)) {
		if fn == nil {
			return
		}
		go func() {
			fn(s.runtime.ctx)
			s.runtime.hub.Invalidate()
		}()
	}
	e.Confirm = func(title, body, action, required string, run func()) {
		s.posts.Post(func() {
			s.confirmReq = &confirmRequest{title: title, body: body, action: action, required: required, run: run}
			s.confirm.Dialog.Open = true
		})
	}
	e.Menu = func(target env.MenuTarget) {
		s.posts.Post(func() {
			if target.Context == "add-to-playlist" && target.Track != nil {
				s.itemMenu.OpenAdd(*target.Track)
				return
			}
			s.itemMenu.OpenAt(e, target, e.Overlays.Press())
		})
	}
	s.nav.onSound = func(anchor image.Rectangle) {
		e.Overlays.Open(&s.soundTag, anchor, look.Placement{
			Prefer: look.Above, Align: look.End, Width: 340,
		}, func(gtx layout.Context) layout.Dimensions {
			return s.sound.Layout(gtx, e)
		})
	}
	act.OpenAuthURLs(e)
	return e
}
