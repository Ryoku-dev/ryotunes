package shell

import (
	"context"
	"image"
	"sync"
	"time"

	gioapp "gioui.org/app"
	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryotunes/internal/client/lithic"
	"github.com/ryoku-dev/ryotunes/internal/client/state"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/chrome"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/widgets"
)

type postQueue struct {
	mu         sync.Mutex
	pending    []func()
	invalidate func()
}

func (q *postQueue) Post(fn func()) {
	if fn == nil {
		return
	}
	q.mu.Lock()
	q.pending = append(q.pending, fn)
	q.mu.Unlock()
	if q.invalidate != nil {
		q.invalidate()
	}
}

func (q *postQueue) Drain() {
	q.mu.Lock()
	pending := q.pending
	q.pending = nil
	q.mu.Unlock()
	for _, fn := range pending {
		fn()
	}
}

type confirmRequest struct {
	title, body, action, required string
	run                           func()
}

type uiShell struct {
	runtime *clientRuntime
	window  *gioapp.Window
	posts   postQueue
	nav     *navigator
	pages   map[string]env.Page
	config  gioapp.Config

	player     chrome.PlayerBar
	queue      chrome.QueuePanel
	lyrics     chrome.LyricsPanel
	nowPlaying chrome.NowPlaying
	listen     chrome.ListenPanel
	mini       chrome.Mini

	rail       rail
	title      titleBar
	panelClose lithic.IconButton
	decor      decorations
	overlays   overlayState
	itemMenu   widgets.ItemMenu
	menuTarget *env.MenuTarget
	menuPoint  image.Point
	pointerTag int
	pointer    image.Point

	prefsMu   sync.Mutex
	prefsSave chan struct{}
	miniMu    sync.Mutex
	miniWin   *gioapp.Window
	closed    bool
}

func newShell(runtime *clientRuntime, window *gioapp.Window, initial env.Route, panel env.Panel, mini bool) *uiShell {
	shell := &uiShell{runtime: runtime, window: window, pages: make(map[string]env.Page), prefsSave: make(chan struct{}, 1)}
	shell.posts.invalidate = runtime.hub.Invalidate
	shell.nav = newNavigator(initial, panel, mini, runtime.hub.Invalidate)
	shell.nav.onChange = shell.persistNavigation
	shell.nav.onMini = shell.setMini
	shell.decor.window = window
	if window != nil {
		go shell.preferenceWriter()
	}
	return shell
}

func (r *clientRuntime) runWindows(options Options) error {
	preferences := r.store.Snapshot().Client
	initial := parseRoute(preferences.Route)
	if options.Route != "" {
		initial = parseRoute(options.Route)
	}
	panel := env.PanelNone
	if options.Width >= compactWidth && preferences.PanelOpen {
		panel = parsePanel(preferences.PanelTab)
	}
	if options.Panel != "" {
		panel = parsePanel(options.Panel)
	}
	mini := options.Mini || preferences.MiniOpen

	var window gioapp.Window
	window.Option(
		gioapp.Title("Ryotunes"),
		gioapp.Size(unit.Dp(options.Width), unit.Dp(options.Height)),
		gioapp.MinSize(unit.Dp(640), unit.Dp(520)),
		gioapp.Decorated(false),
	)
	removeWindow := r.hub.Add(window.Invalidate)
	defer removeWindow()
	unsubscribe := r.store.Subscribe(r.hub.Invalidate)
	defer unsubscribe()

	shell := newShell(r, &window, initial, panel, mini)
	shell.persistNavigation()
	if mini {
		shell.setMini(true)
	}
	return shell.runMain()
}

func (s *uiShell) runMain() error {
	var operations op.Ops
	for {
		event := s.window.Event()
		switch event := event.(type) {
		case gioapp.ConfigEvent:
			s.config = event.Config
		case gioapp.FrameEvent:
			// the config carries pixels; only a frame knows the metric to turn them into dp
			s.persistWindowSize(s.config, event.Metric)
			s.posts.Drain()
			s.frame(event, &operations)
			event.Frame(&operations)
		case gioapp.DestroyEvent:
			s.miniMu.Lock()
			s.closed = true
			if s.miniWin != nil {
				s.miniWin.Perform(system.ActionClose)
			}
			s.miniMu.Unlock()
			s.flushPreferences()
			return event.Err
		}
	}
}

func (s *uiShell) frame(frame gioapp.FrameEvent, operations *op.Ops) {
	gtx := gioapp.NewContext(operations, frame)
	windowWidthDP := float32(gtx.Metric.PxToDp(gtx.Constraints.Max.X))
	preferences := s.runtime.store.Snapshot().Client
	scale := float32(preferences.ZoomPercent) / 100
	if scale <= 0 {
		scale = 1
	}
	gtx.Metric.PxPerDp *= scale
	gtx.Metric.PxPerSp *= scale
	th := s.runtime.applyClientTheme(preferences)

	e := s.environment(gtx.Now, th)
	act.PendingClipboard(gtx)
	paint.Fill(gtx.Ops, th.R.Ground)
	s.trackPointer(gtx)
	s.layout(gtx, e, windowWidthDP)
	s.layoutOverlays(gtx, e)
	s.handleKeys(gtx, e)
	if s.runtime.store.ConsumeShow() {
		s.window.Option(gioapp.Windowed.Option())
		s.window.Perform(system.ActionRaise)
	}
}

func (s *uiShell) environment(now time.Time, theme *lithic.Theme) *env.Env {
	return &env.Env{
		Th: theme, Store: s.runtime.store, Conn: s.runtime.client, Nav: s.nav, Art: s.runtime.art, Now: now,
		Toast: func(message string, kind env.ToastKind) {
			s.posts.Post(func() { s.overlays.placards.Push(time.Now(), message, "") })
		},
		Go: func(fn func(context.Context)) {
			if fn == nil {
				return
			}
			go func() {
				fn(s.runtime.ctx)
				s.runtime.hub.Invalidate()
			}()
		},
		Confirm: func(title, body, action, required string, run func()) {
			s.posts.Post(func() {
				s.overlays.confirmRequest = &confirmRequest{title: title, body: body, action: action, required: required, run: run}
				s.overlays.confirm.Dialog.Open = true
			})
		},
		Menu: func(target env.MenuTarget) {
			s.posts.Post(func() {
				targetCopy := target
				s.menuTarget = &targetCopy
				s.menuPoint = s.pointer
				s.overlays.menu.Open = true
			})
		},
	}
}

func (s *uiShell) trackPointer(gtx layout.Context) {
	for {
		event, ok := gtx.Event(pointer.Filter{Target: &s.pointerTag, Kinds: pointer.Move | pointer.Drag | pointer.Press})
		if !ok {
			break
		}
		if event, ok := event.(pointer.Event); ok {
			s.pointer = event.Position.Round()
		}
	}
	area := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
	event.Op(gtx.Ops, &s.pointerTag)
	area.Pop()
}

func (s *uiShell) persistWindowSize(config gioapp.Config, metric unit.Metric) {
	if config.Size.X <= 0 || config.Size.Y <= 0 || config.Mode != gioapp.Windowed {
		return
	}
	preferences := s.runtime.store.Snapshot().Client
	width := int(metric.PxToDp(config.Size.X) + .5)
	height := int(metric.PxToDp(config.Size.Y) + .5)
	if preferences.WindowWidth == width && preferences.WindowHeight == height {
		return
	}
	preferences.WindowWidth, preferences.WindowHeight = width, height
	s.runtime.store.SetClientPreferences(preferences)
	s.savePreferences()
}

func (s *uiShell) persistNavigation() {
	preferences := s.runtime.store.Snapshot().Client
	preferences.Route = persistedRoute(s.nav.Current())
	panel := s.nav.Panel()
	preferences.PanelOpen = panel != env.PanelNone
	if panel == env.PanelQueue || panel == env.PanelLyrics {
		preferences.PanelTab = string(panel)
	}
	preferences.MiniOpen = s.nav.miniOpen()
	s.runtime.store.SetClientPreferences(preferences)
	s.savePreferences()
}
func (s *uiShell) savePreferences() {
	select {
	case s.prefsSave <- struct{}{}:
	default:
	}
}

func (s *uiShell) preferenceWriter() {
	for {
		select {
		case <-s.runtime.ctx.Done():
			return
		case <-s.prefsSave:
		}
		timer := time.NewTimer(75 * time.Millisecond)
		select {
		case <-s.runtime.ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		for {
			select {
			case <-s.prefsSave:
				continue
			default:
			}
			break
		}
		s.prefsMu.Lock()
		_ = s.runtime.store.SaveClientPreferences()
		s.prefsMu.Unlock()
	}
}

func (s *uiShell) flushPreferences() {
	s.prefsMu.Lock()
	defer s.prefsMu.Unlock()
	_ = s.runtime.store.SaveClientPreferences()
}

func (r *clientRuntime) applyClientTheme(preferences state.ClientPreferences) *lithic.Theme {
	r.syncPalettePreferences(preferences)
	density := lithic.Comfortable
	if preferences.Density == "compact" {
		density = lithic.Compact
	}
	r.themeMu.Lock()
	if r.theme.Density != density {
		theme := lithic.New(r.theme.R, lithic.Shaper())
		theme.Density = density
		r.theme = theme
	}
	theme := r.theme
	r.themeMu.Unlock()
	lithic.SetReduce(preferences.ReduceMotion)
	return theme
}

func (s *uiShell) setMini(open bool) {
	s.miniMu.Lock()
	defer s.miniMu.Unlock()
	if s.closed {
		return
	}
	if open {
		if s.miniWin != nil {
			s.miniWin.Perform(system.ActionRaise)
			return
		}
		window := new(gioapp.Window)
		window.Option(gioapp.Title("Ryotunes Mini"), gioapp.Size(360, 420), gioapp.MinSize(300, 320), gioapp.Decorated(false))
		s.miniWin = window
		remove := s.runtime.hub.Add(window.Invalidate)
		go s.runMini(window, remove)
		s.window.Option(gioapp.Minimized.Option())
		return
	}
	if s.miniWin != nil {
		s.miniWin.Perform(system.ActionClose)
	} else {
		s.restoreMain()
	}
}

func (s *uiShell) runMini(window *gioapp.Window, remove func()) {
	defer remove()
	var operations op.Ops
	for {
		event := window.Event()
		switch event := event.(type) {
		case gioapp.FrameEvent:
			gtx := gioapp.NewContext(&operations, event)
			preferences := s.runtime.store.Snapshot().Client
			scale := float32(preferences.ZoomPercent) / 100
			if scale <= 0 {
				scale = 1
			}
			gtx.Metric.PxPerDp *= scale
			gtx.Metric.PxPerSp *= scale
			th := s.runtime.applyClientTheme(preferences)
			e := s.environment(gtx.Now, th)
			act.PendingClipboard(gtx)
			paint.Fill(gtx.Ops, th.R.Ground)
			s.mini.Layout(gtx, e)
			event.Frame(&operations)
		case gioapp.DestroyEvent:
			s.miniMu.Lock()
			if s.miniWin == window {
				s.miniWin = nil
			}
			s.nav.mu.Lock()
			s.nav.mini = false
			s.nav.mu.Unlock()
			s.miniMu.Unlock()
			s.persistNavigation()
			s.restoreMain()
			return
		}
	}
}

func (s *uiShell) restoreMain() {
	s.window.Option(gioapp.Windowed.Option())
	s.window.Perform(system.ActionRaise)
	s.runtime.hub.Invalidate()
}

func layoutAt(gtx layout.Context, at, size image.Point, widget layout.Widget) layout.Dimensions {
	size.X = max(0, size.X)
	size.Y = max(0, size.Y)
	offset := op.Offset(at).Push(gtx.Ops)
	area := clip.Rect{Max: size}.Push(gtx.Ops)
	child := gtx
	child.Constraints = layout.Exact(size)
	widget(child)
	area.Pop()
	offset.Pop()
	return layout.Dimensions{Size: size}
}
