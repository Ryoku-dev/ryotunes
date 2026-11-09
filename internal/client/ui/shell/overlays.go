package shell

import (
	"context"
	"image"
	"strings"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"

	"github.com/ryoku-dev/ryotunes/internal/client/lithic"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

type overlayState struct {
	placards       lithic.Placards
	confirm        lithic.Confirm
	confirmRequest *confirmRequest
	menu           lithic.Popover
	palette        lithic.Dialog
	paletteSearch  lithic.Search
	paletteIndex   int
	paletteWasOpen bool
	paletteButtons map[string]*lithic.Button
}

type paletteItem struct {
	key, label, icon string
	run              func()
}

func (s *uiShell) layoutOverlays(gtx layout.Context, e *env.Env) {
	s.layoutItemMenu(gtx, e)
	s.layoutPalette(gtx, e)
	s.layoutConfirm(gtx, e)
	s.overlays.placards.Layout(gtx, e.Th)
}

func (s *uiShell) layoutItemMenu(gtx layout.Context, e *env.Env) {
	if s.menuTarget == nil && !s.overlays.menu.Open {
		return
	}
	point := s.menuPoint
	menuWidth := min(gtx.Constraints.Max.X, gtx.Dp(320))
	menuHeight := min(gtx.Constraints.Max.Y, gtx.Dp(480))
	point.X = max(0, min(point.X, gtx.Constraints.Max.X-menuWidth))
	point.Y = max(0, min(point.Y, gtx.Constraints.Max.Y-menuHeight))
	offset := op.Offset(point).Push(gtx.Ops)
	menuGTX := gtx
	menuGTX.Constraints.Min = image.Point{}
	menuGTX.Constraints.Max = image.Pt(max(1, gtx.Constraints.Max.X-point.X), max(1, gtx.Constraints.Max.Y-point.Y))
	s.overlays.menu.Layout(menuGTX,
		func(layout.Context) layout.Dimensions { return layout.Dimensions{Size: image.Pt(1, 1)} },
		func(gtx layout.Context) layout.Dimensions {
			if s.menuTarget == nil {
				return layout.Dimensions{}
			}
			return s.itemMenu.Layout(gtx, e, *s.menuTarget)
		},
	)
	offset.Pop()
	if s.itemMenu.Done() || !s.overlays.menu.Open {
		s.overlays.menu.Open = false
		s.menuTarget = nil
	}
}

func (s *uiShell) layoutConfirm(gtx layout.Context, e *env.Env) {
	request := s.overlays.confirmRequest
	if request == nil {
		return
	}
	if s.overlays.confirm.Layout(gtx, e.Th, request.title, request.body, request.action, request.required) {
		run := request.run
		s.overlays.confirmRequest = nil
		if run != nil {
			run()
		}
	}
	if s.overlays.confirm.Closed() {
		s.overlays.confirmRequest = nil
	}
}

func (s *uiShell) layoutPalette(gtx layout.Context, e *env.Env) {
	open := e.Nav.(*navigator).paletteOpen()
	if open && !s.overlays.paletteWasOpen {
		s.overlays.palette.Open = true
		s.overlays.palette.Width = 620
		s.overlays.paletteSearch.SetText("")
		s.overlays.paletteIndex = 0
		s.overlays.paletteSearch.Focus(gtx)
	}
	s.overlays.paletteWasOpen = open
	if !open && !s.overlays.palette.Open {
		return
	}

	body := func(gtx layout.Context) layout.Dimensions {
		for {
			event, ok := gtx.Event(
				key.Filter{Focus: &s.overlays.paletteSearch.Editor, Name: key.NameUpArrow},
				key.Filter{Focus: &s.overlays.paletteSearch.Editor, Name: key.NameDownArrow},
				key.Filter{Focus: &s.overlays.paletteSearch.Editor, Name: key.NameEnter},
				key.Filter{Focus: &s.overlays.paletteSearch.Editor, Name: key.NameEscape},
			)
			if !ok {
				break
			}
			keyEvent, ok := event.(key.Event)
			if !ok || keyEvent.State != key.Press {
				continue
			}
			items := s.paletteItems(e)
			switch keyEvent.Name {
			case key.NameUpArrow:
				if len(items) > 0 {
					s.overlays.paletteIndex = (s.overlays.paletteIndex - 1 + len(items)) % len(items)
				}
			case key.NameDownArrow:
				if len(items) > 0 {
					s.overlays.paletteIndex = (s.overlays.paletteIndex + 1) % len(items)
				}
			case key.NameEnter:
				if len(items) > 0 {
					items[min(s.overlays.paletteIndex, len(items)-1)].run()
					s.overlays.palette.Open = false
					e.Nav.(*navigator).closePalette()
				}
			case key.NameEscape:
				s.overlays.palette.Open = false
				e.Nav.(*navigator).closePalette()
			}
		}

		searchGTX := gtx
		searchGTX.Constraints.Min = image.Point{}
		searchGTX.Constraints.Max.Y = gtx.Dp(44)
		searchDims := s.overlays.paletteSearch.Layout(searchGTX, e.Th, "Search routes, actions and queue…", "Ctrl K")
		items := s.paletteItems(e)
		if len(items) == 0 {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(layout.Context) layout.Dimensions { return searchDims }),
				layout.Rigid(layout.Spacer{Height: 21}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return lithic.Label{Style: e.Th.T.Body, Color: e.Th.R.Fg3}.Layout(gtx, e.Th, "Nothing quick for that.")
				}),
			)
		}
		if s.overlays.paletteIndex >= len(items) {
			s.overlays.paletteIndex = len(items) - 1
		}
		children := []layout.FlexChild{
			layout.Rigid(func(layout.Context) layout.Dimensions { return searchDims }),
			layout.Rigid(layout.Spacer{Height: 13}.Layout),
		}
		for index, item := range items {
			index, item := index, item
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				button := s.paletteButton(item.key)
				button.Small = true
				kind := lithic.Quiet
				if index == s.overlays.paletteIndex {
					kind = lithic.On
				}
				dims := button.Layout(gtx, e.Th, kind, lithic.Icon(item.icon), item.label)
				if button.Clicked(gtx) {
					item.run()
					s.overlays.palette.Open = false
					e.Nav.(*navigator).closePalette()
				}
				return dims
			}))
			if index != len(items)-1 {
				children = append(children, layout.Rigid(layout.Spacer{Height: 4}.Layout))
			}
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	}
	s.overlays.palette.Layout(gtx, e.Th, "Command palette", body)
	if !s.overlays.palette.Open && e.Nav.(*navigator).paletteOpen() {
		e.Nav.(*navigator).closePalette()
	}
}

func (s *uiShell) paletteButton(key string) *lithic.Button {
	if s.overlays.paletteButtons == nil {
		s.overlays.paletteButtons = make(map[string]*lithic.Button)
	}
	button := s.overlays.paletteButtons[key]
	if button == nil {
		button = new(lithic.Button)
		s.overlays.paletteButtons[key] = button
	}
	return button
}

func (s *uiShell) paletteItems(e *env.Env) []paletteItem {
	query := strings.ToLower(strings.TrimSpace(s.overlays.paletteSearch.Text()))
	items := []paletteItem{
		{key: "route-home", label: "Home", icon: "home", run: func() { e.Nav.(*navigator).root(env.Route{Kind: env.RouteHome}) }},
		{key: "route-search", label: "Search", icon: "search", run: func() { e.Nav.(*navigator).root(env.Route{Kind: env.RouteSearch}) }},
		{key: "route-library", label: "Library", icon: "library", run: func() { e.Nav.(*navigator).root(env.Route{Kind: env.RouteLibrary}) }},
		{key: "route-radio", label: "Radio", icon: "radio", run: func() { e.Nav.(*navigator).root(env.Route{Kind: env.RouteRadio}) }},
		{key: "route-downloads", label: "Downloads", icon: "download", run: func() { e.Nav.(*navigator).root(env.Route{Kind: env.RouteDownloads}) }},
		{key: "route-settings", label: "Settings", icon: "settings", run: func() { e.Nav.(*navigator).root(env.Route{Kind: env.RouteSettings}) }},
		{key: "play-pause", label: "Play or pause", icon: "play", run: func() { s.callNull(e, protocol.MethodTogglePause) }},
		{key: "previous", label: "Previous track", icon: "skip_previous", run: func() { s.callNull(e, protocol.MethodPrevTrack) }},
		{key: "next", label: "Next track", icon: "skip_next", run: func() { s.callNull(e, protocol.MethodNextTrack) }},
		{key: "shuffle", label: "Toggle shuffle", icon: "shuffle", run: func() { s.callNull(e, protocol.MethodToggleShuffle) }},
		{key: "repeat", label: "Change repeat mode", icon: "repeat", run: func() { s.cycleRepeat(e) }},
		{key: "panel-queue", label: "Toggle queue", icon: "queue", run: func() { e.Nav.TogglePanel(env.PanelQueue) }},
		{key: "panel-lyrics", label: "Toggle lyrics", icon: "lyrics", run: func() { e.Nav.TogglePanel(env.PanelLyrics) }},
		{key: "mini", label: "Toggle mini player", icon: "album", run: e.Nav.ToggleMini},
		{key: "settings-appearance", label: "Open appearance settings", icon: "tune", run: func() { e.Nav.(*navigator).root(env.Route{Kind: env.RouteSettings, Params: "appearance"}) }},
		{key: "settings-playback", label: "Open playback settings", icon: "play", run: func() { e.Nav.(*navigator).root(env.Route{Kind: env.RouteSettings, Params: "playback"}) }},
		{key: "settings-providers", label: "Open provider settings", icon: "person", run: func() { e.Nav.(*navigator).root(env.Route{Kind: env.RouteSettings, Params: "providers"}) }},
		{key: "settings-downloads", label: "Open download settings", icon: "download", run: func() { e.Nav.(*navigator).root(env.Route{Kind: env.RouteSettings, Params: "downloads"}) }},
		{key: "settings-integrations", label: "Open integration settings", icon: "settings", run: func() { e.Nav.(*navigator).root(env.Route{Kind: env.RouteSettings, Params: "integrations"}) }},
		{key: "settings-data", label: "Open data and storage settings", icon: "library", run: func() { e.Nav.(*navigator).root(env.Route{Kind: env.RouteSettings, Params: "data"}) }},
		{key: "settings-local", label: "Open local music settings", icon: "album", run: func() { e.Nav.(*navigator).root(env.Route{Kind: env.RouteSettings, Params: "local"}) }},
		{key: "settings-about", label: "Open about settings", icon: "info", run: func() { e.Nav.(*navigator).root(env.Route{Kind: env.RouteSettings, Params: "about"}) }},
	}
	snapshot := e.Store.Snapshot()
	for index, track := range snapshot.Queue.Items {
		index, track := index, track
		items = append(items, paletteItem{
			key: "queue-" + track.VideoID, label: track.Title + " · " + track.Artists, icon: "play",
			run: func() { s.call(e, protocol.MethodPlayIndex, protocol.IndexParams{Index: index}) },
		})
	}
	filtered := make([]paletteItem, 0, min(12, len(items)))
	for _, item := range items {
		if query != "" && !strings.Contains(strings.ToLower(item.label), query) {
			continue
		}
		filtered = append(filtered, item)
		if len(filtered) == 12 {
			break
		}
	}
	return filtered
}

func (s *uiShell) callNull(e *env.Env, method string) {
	s.call(e, method, protocol.Null{})
}

func (s *uiShell) call(e *env.Env, method string, params any) {
	e.Go(func(ctx context.Context) {
		if err := e.Conn.Call(ctx, method, params, nil); err != nil {
			e.Toast(err.Error(), env.ToastError)
		}
	})
}

func (s *uiShell) cycleRepeat(e *env.Env) {
	mode := e.Store.Snapshot().Queue.Repeat
	next := model.RepeatAll
	switch mode {
	case model.RepeatAll:
		next = model.RepeatOne
	case model.RepeatOne:
		next = model.RepeatOff
	}
	s.call(e, protocol.MethodSetRepeat, protocol.SetRepeatParams{Mode: next})
}
