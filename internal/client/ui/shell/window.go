package shell

import (
	"sync"
	"time"

	gioapp "gioui.org/app"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/anim"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryotunes/internal/client/state"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/chrome"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/widgets"
)

type postQueue struct {
	mu         sync.Mutex
	pending    []func()
	invalidate func()
}

func (queue *postQueue) Post(fn func()) {
	if fn == nil {
		return
	}
	queue.mu.Lock()
	queue.pending = append(queue.pending, fn)
	queue.mu.Unlock()
	if queue.invalidate != nil {
		queue.invalidate()
	}
}

func (queue *postQueue) Drain() {
	queue.mu.Lock()
	pending := queue.pending
	queue.pending = nil
	queue.mu.Unlock()
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
	posts   postQueue
	nav     *navigator
	pages   map[string]env.Page
	config  gioapp.Config

	topBar       chrome.TopBar
	sidebar      chrome.Sidebar
	sidebarSheet chrome.Sidebar
	bottomNav    chrome.BottomNav
	player       chrome.PlayerBar
	sound        chrome.Sound
	rightPanel   chrome.RightPanel
	nowPlaying   chrome.NowPlaying
	mini         chrome.Mini

	overlays            look.Overlays
	placards            kit.Placard
	confirm             kit.Confirm
	confirmReq          *confirmRequest
	itemMenu            widgets.ItemMenu
	soundTag            struct{}
	sheetOutside        struct{}
	panelMotion         anim.Tween
	sheetMotion         anim.Tween
	sheetReady          bool
	sidebarMotion       anim.Tween
	sidebarMotionReady  bool
	sidebarSheetMotion  anim.Tween
	sidebarSheetReady   bool
	sidebarSheetOutside struct{}
	contentSwap         look.Swap
	transitionWidgets   map[string]layout.Widget

	classReady      bool
	lastClass       look.Class
	lastDocked      bool
	rememberedPanel env.Panel

	prefsMu   sync.Mutex
	prefsSave chan struct{}

	windowMu    sync.Mutex
	window      *gioapp.Window
	currentMini bool
	nextWindow  *gioapp.Window
	nextMini    bool
	quitting    bool
	mainWidth   int
	mainHeight  int
}

func newShell(runtime *clientRuntime, initial env.Route, panel env.Panel, mini bool, width, height int) *uiShell {
	shell := &uiShell{
		runtime: runtime, pages: make(map[string]env.Page), prefsSave: make(chan struct{}, 1),
		mainWidth: width, mainHeight: height,
	}
	shell.posts.invalidate = runtime.hub.Invalidate
	shell.nav = newNavigator(initial, panel, mini, runtime.hub.Invalidate)
	shell.nav.collapsed = runtime.store.Snapshot().Client.SidebarCollapsed
	shell.nav.onChange = shell.persistNavigation
	shell.nav.onMini = shell.requestWindow
	go shell.preferenceWriter()
	return shell
}

func (runtime *clientRuntime) runWindows(options Options) error {
	preferences := runtime.store.Snapshot().Client
	initial := initialRoute(preferences.Route, options)
	panel := env.PanelNone
	if look.ClassFor(effectiveInitialWidth(options.Width, preferences)) != look.Compact && preferences.PanelOpen {
		panel = parsePanel(preferences.PanelTab)
	}
	if options.Panel != "" {
		panel = parsePanel(options.Panel)
	}
	mini := options.Mini || preferences.MiniOpen

	shell := newShell(runtime, initial, panel, mini, options.Width, options.Height)
	if panelOpensNowPlaying(options.Panel) {
		shell.nav.nowPlaying = true
	}
	shell.persistNavigation()
	unsubscribe := runtime.store.Subscribe(runtime.hub.Invalidate)
	defer unsubscribe()

	window := shell.newWindow(mini)
	go func() {
		select {
		case <-runtime.quit:
			shell.quitWindows()
		case <-runtime.ctx.Done():
		}
	}()
	for {
		if err := shell.runWindow(window, mini); err != nil {
			shell.flushPreferences()
			return err
		}
		shell.windowMu.Lock()
		next := shell.nextWindow
		nextMini := shell.nextMini
		quitting := shell.quitting
		shell.nextWindow = nil
		shell.window = nil
		shell.windowMu.Unlock()
		if quitting && next != nil {
			next.Perform(system.ActionClose)
			next = nil
		}
		if next == nil {
			shell.setMiniPreference(false)
			shell.flushPreferences()
			return nil
		}
		window, mini = next, nextMini
	}
}

func (s *uiShell) newWindow(mini bool) *gioapp.Window {
	window := new(gioapp.Window)
	if mini {
		preferences := s.runtime.store.Snapshot().Client
		scale := clientScale(preferences)
		window.Option(
			gioapp.Title("Ryotunes Mini"),
			gioapp.Size(unit.Dp(520*scale), unit.Dp(176*scale)),
			gioapp.Decorated(false),
		)
		return window
	}
	preferences := s.runtime.store.Snapshot().Client
	width, height := preferences.WindowWidth, preferences.WindowHeight
	if s.mainWidth > 0 && s.mainHeight > 0 {
		width, height = s.mainWidth, s.mainHeight
		s.mainWidth, s.mainHeight = 0, 0
	}
	window.Option(
		gioapp.Title("Ryotunes"),
		gioapp.Size(unit.Dp(width), unit.Dp(height)),
		gioapp.MinSize(unit.Dp(480), unit.Dp(560)),
		gioapp.Decorated(!preferences.Decorations),
	)
	return window
}

func (s *uiShell) runWindow(window *gioapp.Window, mini bool) error {
	s.windowMu.Lock()
	s.window, s.currentMini = window, mini
	s.config = gioapp.Config{}
	quitting := s.quitting
	s.windowMu.Unlock()
	if quitting {
		window.Perform(system.ActionClose)
	}
	removeWindow := s.runtime.hub.Add(window.Invalidate)
	defer removeWindow()

	var operations op.Ops
	minimumSet := !mini
	for {
		raw := window.Event()
		switch event := raw.(type) {
		case gioapp.ConfigEvent:
			s.config = event.Config
			if !minimumSet {
				// Gio pads the first minimum with a 40 dp fallback title bar until
				// decorations are settled, and niri sizes the mini from that first one
				minimumSet = true
				scale := clientScale(s.runtime.store.Snapshot().Client)
				window.Option(gioapp.MinSize(unit.Dp(300*scale), unit.Dp(160*scale)))
			}
		case gioapp.FrameEvent:
			if !mini {
				s.persistWindowSize(s.config, event.Metric)
			}
			s.posts.Drain()
			if s.runtime.store.ConsumeShow() {
				if mini {
					s.requestWindow(false)
				} else {
					window.Option(gioapp.Windowed.Option())
					window.Perform(system.ActionRaise)
				}
			}
			if mini {
				s.frameMini(event, &operations)
			} else {
				s.frameMain(event, &operations)
			}
			event.Frame(&operations)
		case gioapp.DestroyEvent:
			return event.Err
		}
	}
}

func (s *uiShell) frameMain(frame gioapp.FrameEvent, operations *op.Ops) {
	gtx := gioapp.NewContext(operations, frame)
	preferences := s.runtime.store.Snapshot().Client
	scaleMetric(&gtx, preferences)
	windowWidthDP := float32(gtx.Metric.PxToDp(gtx.Constraints.Max.X))
	theme := s.runtime.applyClientTheme(preferences)
	e := s.environment(gtx.Now, theme)
	act.PendingClipboard(gtx)
	if s.nav.takeSearchFocus() {
		s.topBar.FocusSearch(gtx)
	}
	s.layout(gtx, e, windowWidthDP)
	s.handleKeys(gtx, e)
	s.layoutOverlays(gtx, e)
}

func (s *uiShell) frameMini(frame gioapp.FrameEvent, operations *op.Ops) {
	gtx := gioapp.NewContext(operations, frame)
	preferences := s.runtime.store.Snapshot().Client
	scaleMetric(&gtx, preferences)
	theme := s.runtime.applyClientTheme(preferences)
	e := s.environment(gtx.Now, theme)
	act.PendingClipboard(gtx)
	s.reconcileProvider(gtx, e)
	paint.Fill(gtx.Ops, theme.R.Ground)
	s.mini.Layout(gtx, e)
	if action, ok := s.mini.WindowAction(); ok {
		s.performWindowAction(action)
	}
	s.handleKeys(gtx, e)
	s.layoutOverlays(gtx, e)
}

func clientScale(preferences state.ClientPreferences) float32 {
	scale := float32(preferences.ZoomPercent) / 100
	if scale <= 0 {
		return 1
	}
	return scale
}

func effectiveInitialWidth(width int, preferences state.ClientPreferences) float32 {
	return float32(width) / clientScale(preferences)
}

func scaleMetric(gtx *layout.Context, preferences state.ClientPreferences) {
	scale := clientScale(preferences)
	gtx.Metric.PxPerDp *= scale
	gtx.Metric.PxPerSp *= scale
}

func (s *uiShell) requestWindow(mini bool) {
	s.windowMu.Lock()
	if s.quitting {
		s.windowMu.Unlock()
		return
	}
	if s.window == nil {
		s.windowMu.Unlock()
		return
	}
	if s.currentMini == mini {
		s.window.Perform(system.ActionRaise)
		s.windowMu.Unlock()
		return
	}
	if s.nextWindow != nil {
		s.windowMu.Unlock()
		return
	}
	s.nextMini = mini
	s.nextWindow = s.newWindow(mini)
	current := s.window
	s.windowMu.Unlock()
	s.setMiniPreference(mini)
	current.Perform(system.ActionClose)
}

func (s *uiShell) quitWindows() {
	s.windowMu.Lock()
	s.quitting = true
	current, next := s.window, s.nextWindow
	s.nextWindow = nil
	s.windowMu.Unlock()
	if next != nil && next != current {
		next.Perform(system.ActionClose)
	}
	if current != nil {
		current.Perform(system.ActionClose)
	}
}

func (s *uiShell) setMiniPreference(mini bool) {
	s.nav.mu.Lock()
	s.nav.mini = mini
	s.nav.mu.Unlock()
	s.persistNavigation()
}

func (s *uiShell) performWindowAction(action system.Action) {
	s.windowMu.Lock()
	window := s.window
	s.windowMu.Unlock()
	if window == nil {
		return
	}
	// gio's wayland Perform only knows Close, the window modes have to go through Option
	remaining := action
	if action&system.ActionMaximize != 0 {
		window.Option(gioapp.Maximized.Option())
		remaining &^= system.ActionMaximize
	}
	if action&system.ActionUnmaximize != 0 {
		window.Option(gioapp.Windowed.Option())
		remaining &^= system.ActionUnmaximize
	}
	if action&system.ActionFullscreen != 0 {
		window.Option(gioapp.Fullscreen.Option())
		remaining &^= system.ActionFullscreen
	}
	if remaining != 0 {
		window.Perform(remaining)
	}
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
	if panel != env.PanelNone {
		preferences.PanelTab = string(panel)
	}
	preferences.MiniOpen = s.nav.miniOpen()
	preferences.SidebarCollapsed = s.nav.SidebarCollapsed()
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
