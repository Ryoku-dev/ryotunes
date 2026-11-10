package shell

import (
	gioapp "gioui.org/app"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/layout"

	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

var globalKeyFilters = []event.Filter{
	key.Filter{Name: "K", Required: key.ModShortcut},
	key.Filter{Name: "/"},
	key.Filter{Name: key.NameSpace},
	key.Filter{Name: key.NameLeftArrow, Required: key.ModShortcut},
	key.Filter{Name: key.NameRightArrow, Required: key.ModShortcut},
	key.Filter{Name: key.NameLeftArrow, Required: key.ModAlt},
	key.Filter{Name: key.NameRightArrow, Required: key.ModAlt},
	key.Filter{Name: "F", Required: key.ModShortcut},
	key.Filter{Name: "L", Required: key.ModShortcut},
	key.Filter{Name: "R", Required: key.ModShortcut},
	key.Filter{Name: "B", Required: key.ModShortcut},
	key.Filter{Name: ",", Required: key.ModShortcut},
	key.Filter{Name: "Q", Required: key.ModShortcut},
	key.Filter{Name: "F11"},
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
			e.Nav.FocusSearch()
		case pressed.Name == "/" && pressed.Modifiers == 0:
			e.Nav.FocusSearch()
		case pressed.Name == key.NameSpace && pressed.Modifiers == 0:
			if !s.topBar.SearchFocused(gtx) && s.confirmReq == nil {
				s.callNull(e, protocol.MethodTogglePause)
			}
		case pressed.Name == key.NameLeftArrow && pressed.Modifiers == key.ModShortcut:
			s.callNull(e, protocol.MethodPrevTrack)
		case pressed.Name == key.NameRightArrow && pressed.Modifiers == key.ModShortcut:
			s.callNull(e, protocol.MethodNextTrack)
		case pressed.Name == key.NameLeftArrow && pressed.Modifiers == key.ModAlt:
			e.Nav.Back()
		case pressed.Name == key.NameRightArrow && pressed.Modifiers == key.ModAlt:
			e.Nav.Forward()
		case pressed.Name == "F" && pressed.Modifiers == key.ModShortcut:
			if page, ok := s.page(e.Nav.Current()).(env.Filterer); ok {
				page.FocusFilter(gtx)
			}
		case pressed.Name == "L" && pressed.Modifiers == key.ModShortcut:
			s.toggleLike(e)
		case pressed.Name == "R" && pressed.Modifiers == key.ModShortcut:
			if page, ok := s.page(e.Nav.Current()).(env.Refresher); ok {
				page.Refresh(e)
			}
		case pressed.Name == "B" && pressed.Modifiers == key.ModShortcut:
			s.toggleSidebar(e)
		case pressed.Name == "," && pressed.Modifiers == key.ModShortcut:
			e.Nav.Push(env.Route{Kind: env.RouteSettings, Title: "Settings"})
		case pressed.Name == "Q" && pressed.Modifiers == key.ModShortcut:
			s.quitWindows()
		case pressed.Name == "F11" && pressed.Modifiers == 0:
			s.toggleFullscreen()
		case pressed.Name == key.NameEscape:
			s.escape(gtx, e)
		}
	}
}

func (s *uiShell) toggleSidebar(e *env.Env) {
	if s.currentMini {
		return
	}
	e.Nav.ToggleSidebar()
}

func (s *uiShell) escape(gtx layout.Context, e *env.Env) {
	if s.confirmReq != nil {
		s.confirm.Dialog.Dismiss(gtx)
		s.confirmReq = nil
		return
	}
	if tag, ok := e.Overlays.Top(); ok {
		e.Overlays.Close(tag)
		return
	}
	if s.nav.CloseSidebarSheet() {
		return
	}
	if e.Nav.Panel() != env.PanelNone && !s.lastDocked {
		e.Nav.OpenPanel(env.PanelNone)
		return
	}
	if s.topBar.SearchFocused(gtx) {
		return
	}
	if e.Nav.NowPlaying() {
		e.Nav.ToggleNowPlaying()
		return
	}
	if page, ok := s.page(e.Nav.Current()).(env.Leaver); ok && !s.currentMini {
		page.Leave(e)
	}
}

func (s *uiShell) toggleFullscreen() {
	s.windowMu.Lock()
	window := s.window
	mode := s.config.Mode
	s.windowMu.Unlock()
	if window == nil {
		return
	}
	if mode == gioapp.Fullscreen {
		window.Option(gioapp.Windowed.Option())
	} else {
		window.Option(gioapp.Fullscreen.Option())
	}
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
