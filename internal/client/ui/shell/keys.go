package shell

import (
	"fmt"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/system"
	"gioui.org/layout"

	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

var globalKeyFilters = []event.Filter{
	key.Filter{Name: "K", Required: key.ModShortcut},
	key.Filter{Name: "/"},
	key.Filter{Name: key.NameSpace},
	key.Filter{Name: key.NameLeftArrow},
	key.Filter{Name: key.NameRightArrow},
	key.Filter{Name: key.NameUpArrow},
	key.Filter{Name: key.NameDownArrow},
	key.Filter{Name: "L", Required: key.ModShortcut},
	key.Filter{Name: "R", Required: key.ModShortcut},
	key.Filter{Name: "Q", Required: key.ModShortcut},
	key.Filter{Name: "M", Required: key.ModShortcut},
	key.Filter{Name: "1", Required: key.ModShortcut},
	key.Filter{Name: "2", Required: key.ModShortcut},
	key.Filter{Name: "3", Required: key.ModShortcut},
	key.Filter{Name: "4", Required: key.ModShortcut},
	key.Filter{Name: "5", Required: key.ModShortcut},
	key.Filter{Name: "6", Required: key.ModShortcut},
	key.Filter{Name: "-", Required: key.ModShortcut},
	key.Filter{Name: "=", Required: key.ModShortcut},
	key.Filter{Name: "+", Required: key.ModShortcut, Optional: key.ModShift},
	key.Filter{Name: "0", Required: key.ModShortcut},
	key.Filter{Name: key.NameEscape},
}

func (s *uiShell) handleKeys(gtx layout.Context, e *env.Env) {
	for {
		raw, ok := gtx.Event(globalKeyFilters...)
		if !ok {
			return
		}
		pressed, ok := raw.(key.Event)
		if !ok || pressed.State != key.Press {
			continue
		}
		switch {
		case pressed.Name == "K" && pressed.Modifiers == key.ModShortcut:
			s.nav.togglePalette()
		case pressed.Name == "/" && pressed.Modifiers == 0:
			e.Nav.FocusSearch()
		case pressed.Name == key.NameSpace && pressed.Modifiers == 0:
			s.callNull(e, protocol.MethodTogglePause)
		case pressed.Name == key.NameLeftArrow && pressed.Modifiers == 0:
			s.seekBy(e, -10)
		case pressed.Name == key.NameRightArrow && pressed.Modifiers == 0:
			s.seekBy(e, 10)
		case pressed.Name == key.NameUpArrow && pressed.Modifiers == 0:
			s.volumeBy(e, 5)
		case pressed.Name == key.NameDownArrow && pressed.Modifiers == 0:
			s.volumeBy(e, -5)
		case pressed.Name == "L" && pressed.Modifiers == key.ModShortcut:
			s.toggleLike(e)
		case pressed.Name == "R" && pressed.Modifiers == key.ModShortcut:
			if page, ok := s.page(e.Nav.Current()).(env.Refresher); ok {
				page.Refresh(e)
			}
		case pressed.Name == "Q" && pressed.Modifiers == key.ModShortcut:
			s.window.Perform(system.ActionClose)
		case pressed.Name == "M" && pressed.Modifiers == key.ModShortcut:
			e.Nav.ToggleMini()
		case pressed.Modifiers == key.ModShortcut && pressed.Name >= "1" && pressed.Name <= "6":
			routes := [...]env.RouteKind{env.RouteHome, env.RouteSearch, env.RouteLibrary, env.RouteRadio, env.RouteDownloads, env.RouteSettings}
			s.nav.root(env.Route{Kind: routes[int(pressed.Name[0]-'1')]})
		case pressed.Modifiers == key.ModShortcut && pressed.Name == "-":
			s.setZoom(e, -10, false)
		case pressed.Modifiers&key.ModShortcut != 0 && (pressed.Name == "=" || pressed.Name == "+"):
			s.setZoom(e, 10, false)
		case pressed.Modifiers == key.ModShortcut && pressed.Name == "0":
			s.setZoom(e, 0, true)
		case pressed.Name == key.NameEscape:
			s.escape(e)
		}
	}
}

func (s *uiShell) escape(e *env.Env) {
	if s.overlays.menu.Open {
		s.overlays.menu.Open = false
		s.menuTarget = nil
		return
	}
	if s.nav.closePalette() {
		s.overlays.palette.Open = false
		return
	}
	if s.overlays.confirmRequest != nil {
		s.overlays.confirm.Dialog.Open = false
		s.overlays.confirmRequest = nil
		return
	}
	if e.Nav.Panel() != env.PanelNone {
		e.Nav.OpenPanel(env.PanelNone)
		return
	}
	e.Nav.Back()
}

func (s *uiShell) seekBy(e *env.Env, seconds float64) {
	position := e.Store.PositionAt(e.Now) + seconds
	position = max(0, min(position, e.Store.Snapshot().Playback.Duration))
	s.call(e, protocol.MethodSeek, protocol.SeekParams{Position: position})
}

func (s *uiShell) volumeBy(e *env.Env, amount int) {
	volume := max(0, min(100, e.Store.Snapshot().Playback.Volume+amount))
	s.call(e, protocol.MethodSetVolume, protocol.SetVolumeParams{Volume: volume})
}

func (s *uiShell) toggleLike(e *env.Env) {
	snapshot := e.Store.Snapshot()
	if snapshot.Playback.Now == nil {
		return
	}
	rating := model.RatingLike
	if snapshot.Playback.Now.Rating != nil && *snapshot.Playback.Now.Rating == model.RatingLike {
		rating = model.RatingIndifferent
	}
	s.call(e, protocol.MethodRate, protocol.RateParams{VideoID: snapshot.Playback.Now.VideoID, Rating: rating})
}

func (s *uiShell) setZoom(e *env.Env, delta int, reset bool) {
	preferences := e.Store.Snapshot().Client
	value := preferences.ZoomPercent
	if reset {
		value = 100
	} else {
		value = max(80, min(140, value+delta))
	}
	if value == preferences.ZoomPercent {
		return
	}
	preferences.ZoomPercent = value
	e.Store.SetClientPreferences(preferences)
	s.savePreferences()
	s.call(e, protocol.MethodSetSetting, protocol.SetSettingParams{Key: string(model.SettingUIScale), Value: fmt.Sprint(value)})
}
