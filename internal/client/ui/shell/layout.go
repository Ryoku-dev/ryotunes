package shell

import (
	"image"
	"image/color"
	"time"

	gioapp "gioui.org/app"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/anim"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/chrome"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/sheet"
)

// The sheet inside the page pane, in dp, as the Hub sets it (Hub.qml:929-982): the
// masthead 28 in and 4 down, 34 tall; the colophon 28 in, 26 tall, 6 off the foot and 20
// short of the right edge; the page's stage between them.
const (
	mastheadX      unit.Dp = 28
	mastheadY      unit.Dp = 4
	mastheadHeight unit.Dp = 34
	mastheadEnd    unit.Dp = 10
	colophonHeight unit.Dp = 26
	colophonFoot   unit.Dp = 6
	colophonEnd    unit.Dp = 20
	// sheetMinimum is the pane height under which the page keeps the whole pane
	sheetMinimum unit.Dp = 240
	// mastheadFacts is the pane width under which the masthead leaves the provider out
	mastheadFacts unit.Dp = 720
)

// layout lays the window out as the Hub's one paper: the window wears the side plane, the
// sidebar's tree and the top bar stand bare on it, and the page, the docked panel and the
// player bar are panes drawn by their hairlines (cut 16 at the top, 7 at the foot).
func (s *uiShell) layout(gtx layout.Context, e *env.Env, widthDP float32) layout.Dimensions {
	size := gtx.Constraints.Max
	planes := e.Th.Planes()
	paint.Fill(gtx.Ops, planes.Side)
	class := look.ClassFor(widthDP)
	e.Th.Class = class
	s.nav.setSidebarClass(class)
	s.reconcileProvider(gtx, e)

	collapsedTarget := class == look.Expanded && e.Nav.SidebarCollapsed()
	panel := e.Nav.Panel()
	dockEligible := panelDockEligible(widthDP, class, collapsedTarget)
	panel = s.reconcileFramePanel(class, panel, dockEligible)
	docked := panel != env.PanelNone && dockEligible && !e.Nav.NowPlaying()

	s.panelMotion.Duration = e.Th.Motion.HubUnfold
	s.panelMotion.Ease = anim.EaseOutCubic
	dockTarget := float32(0)
	if docked {
		dockTarget = 1
	}
	s.panelMotion.Set(gtx, dockTarget)
	panelProgress := s.panelMotion.Value(gtx)

	gap := gtx.Dp(look.Gap)
	margin := gap
	topHeight := min(size.Y, gtx.Dp(look.TopBar))
	playerHeight := gtx.Dp(look.PlayerBar)
	bottomNavHeight := 0
	if class == look.Compact {
		playerHeight = gtx.Dp(look.PlayerCompact)
		bottomNavHeight = gtx.Dp(look.BottomNav)
	}
	snapshot := e.Store.Snapshot()
	playerProgress := s.playerBarProgress(
		gtx, e.Th.Motion.HubUnfold, snapshot.Provider != "", snapshot.Playback.Now != nil,
	)
	playerY, playerBottom, panesBottom := resolvePlayerGeometry(
		size.Y, topHeight, margin, gap, playerHeight, bottomNavHeight, class == look.Compact, playerProgress,
	)
	s.overlayBottom = panesBottom
	if snapshot.Playback.Now == nil {
		s.overlays.Close(&s.soundTag)
	}

	sidebarWidth, sidebarProgress := 0, float32(0)
	switch class {
	case look.Expanded:
		target := float32(0)
		if collapsedTarget {
			target = 1
		}
		s.sidebarMotion.Duration = e.Th.Motion.Move
		s.sidebarMotion.Ease = anim.EaseOutCubic
		if !s.sidebarMotionReady {
			s.sidebarMotion.Set(gtx, target)
			s.sidebarMotionReady = true
		}
		s.sidebarMotion.Set(gtx, target)
		sidebarProgress = s.sidebarMotion.Value(gtx)
		expanded, rail := gtx.Dp(look.SidebarExpanded), gtx.Dp(look.SidebarMedium)
		sidebarWidth = expanded + int(float32(rail-expanded)*sidebarProgress+.5)
	case look.Medium:
		sidebarWidth, sidebarProgress = gtx.Dp(look.SidebarMedium), 1
	}

	panelWidth := int(float32(gtx.Dp(look.Panel))*panelProgress + .5)
	rects := resolveFrameRects(size, topHeight, panesBottom, margin, gap, sidebarWidth, panelWidth)
	contentRect, panelRect := rects.Content, rects.Panel
	paneTop, paneBottom := gtx.Dp(e.Th.Cut.Large), gtx.Dp(e.Th.Cut.Mid)

	// the open Settings page hangs its sections in the tree while the tree is unfolded and
	// tall enough to hold them; otherwise the page keeps its own section chips
	s.sidebar.SetOutline(s.settingsOutline(gtx, e, class == look.Expanded && !collapsedTarget && sidebarProgress == 0, rects.Sidebar.Dy()))
	if sidebarWidth > 0 {
		layoutAt(gtx, rects.Sidebar.Min, rects.Sidebar.Size(), func(gtx layout.Context) layout.Dimensions {
			return s.sidebar.Layout(gtx, e, class, sidebarProgress)
		})
		s.sidebar.ConsumeNavigation()
	}
	if e.Nav.NowPlaying() {
		contentRect = image.Rect(rects.Content.Min.X, topHeight, size.X-margin, panesBottom)
	}
	s.layoutPane(gtx, e, contentRect, paneTop, paneBottom, func(gtx layout.Context) layout.Dimensions {
		return s.layoutContent(gtx, e)
	})
	if panelWidth > 0 && !e.Nav.NowPlaying() {
		opacity := paint.PushOpacity(gtx.Ops, panelProgress)
		s.layoutPane(gtx, e, panelRect, paneTop, paneBottom, func(gtx layout.Context) layout.Dimensions {
			return s.rightPanel.Layout(gtx, e)
		})
		opacity.Pop()
	}
	sheetOpen := panel != env.PanelNone && !docked && !e.Nav.NowPlaying()
	s.layoutPanelSheet(gtx, e, rects.Content, class, sheetOpen)

	if playerBottom > playerY {
		playerRects := resolvePlayerReveal(size.X, margin, playerY, playerBottom, playerHeight)
		reveal := clip.Rect(playerRects.Visible).Push(gtx.Ops)
		s.player.SetTop(playerY)
		s.layoutPane(gtx, e, playerRects.Full, paneTop, paneBottom, func(gtx layout.Context) layout.Dimensions {
			if class == look.Compact {
				return s.player.LayoutCompact(gtx, e)
			}
			return s.player.Layout(gtx, e)
		})
		reveal.Pop()
	}
	if s.overlays.IsOpen(&s.soundTag) {
		if anchor, ok := s.player.SoundAnchor(); ok {
			s.overlays.Reanchor(&s.soundTag, anchor)
		}
	} else {
		s.player.ResetSoundAnchor()
	}
	if class == look.Compact {
		layoutAt(gtx, image.Pt(0, size.Y-bottomNavHeight), image.Pt(size.X, bottomNavHeight), func(gtx layout.Context) layout.Dimensions {
			return s.bottomNav.Layout(gtx, e)
		})
	}

	frame := chrome.Frame{
		Class: class, Sidebar: rects.SidebarColumn, SidebarProgress: sidebarProgress,
		SidebarSheet: s.nav.SidebarSheetOpen(), Content: rects.Content,
		Decorated: s.config.Decorated, Maximized: s.config.Mode == gioapp.Maximized,
	}
	layoutAt(gtx, image.Point{}, image.Pt(size.X, topHeight), func(gtx layout.Context) layout.Dimensions {
		return s.topBar.Layout(gtx, e, frame)
	})
	if action, ok := s.topBar.WindowAction(); ok {
		s.performWindowAction(action)
	}
	s.layoutSidebarSheet(gtx, e, class == look.Medium)
	return layout.Dimensions{Size: size}
}

// settingsOutlinePage is the Settings page's outline, read structurally so the shell
// needs no part of the page's package beyond its constructor: whether the tree shows it,
// its section names and the one being read, and a way to go to one.
type settingsOutlinePage interface {
	SetOutline(shown bool)
	chrome.SettingsOutline
}

// settingsOutline hands the tree the open Settings page's outline while the tree is
// unfolded and height pixels tall enough to hold it beside its playlists, nil otherwise,
// and tells the page whether the outline shows: folded by the reader or with no room,
// the page shows its own section chips instead.
func (s *uiShell) settingsOutline(gtx layout.Context, e *env.Env, unfolded bool, height int) chrome.SettingsOutline {
	route := e.Nav.Current()
	if route.Kind != env.RouteSettings || e.Nav.NowPlaying() {
		return nil
	}
	page, ok := s.page(route).(settingsOutlinePage)
	if !ok {
		return nil
	}
	available := unfolded && s.sidebar.OutlineFits(gtx, height)
	page.SetOutline(available && s.sidebar.OutlineOpen())
	if !available {
		return nil
	}
	return page
}

func (s *uiShell) playerBarProgress(gtx layout.Context, duration time.Duration, snapshotReady, loaded bool) float32 {
	target := float32(0)
	if loaded {
		target = 1
	}
	s.playerMotion.Duration = duration
	s.playerMotion.Ease = anim.EaseOutCubic
	if !s.playerMotionReady {
		if !snapshotReady {
			return 0
		}
		s.playerMotion.Set(gtx, target)
		s.playerMotionReady = true
	} else {
		s.playerMotion.Set(gtx, target)
	}
	return s.playerMotion.Value(gtx)
}

func resolvePlayerGeometry(sizeY, top, margin, gap, playerHeight, bottomNavHeight int, compact bool, progress float32) (int, int, int) {
	progress = min(max(progress, 0), 1)
	visibleHeight := int(float32(playerHeight)*progress + .5)
	visibleGap := int(float32(gap)*progress + .5)
	bottom := sizeY - margin
	if compact {
		bottom = sizeY - bottomNavHeight - margin
	}
	bottom = max(top, bottom)
	playerTop := max(top, bottom-visibleHeight)
	panesBottom := max(top, playerTop-visibleGap)
	return playerTop, bottom, panesBottom
}

type playerReveal struct {
	Visible image.Rectangle
	Full    image.Rectangle
}

func resolvePlayerReveal(sizeX, margin, top, bottom, height int) playerReveal {
	return playerReveal{
		Visible: image.Rect(margin, top, sizeX-margin, bottom),
		Full:    image.Rect(margin, top, sizeX-margin, top+height),
	}
}

type frameRects struct {
	Sidebar       image.Rectangle
	Content       image.Rectangle
	Panel         image.Rectangle
	SidebarColumn int
}

// resolveFrameRects places the sidebar's column at the window's edge, the full height
// over the panes (its head shares the top bar's height), and the page pane right after
// it, as the Hub's pane stands at the sidebar's width; with no sidebar the pane keeps the
// window margin.
func resolveFrameRects(size image.Point, top, bottom, margin, gap, sidebarWidth, panelWidth int) frameRects {
	contentX := margin
	if sidebarWidth > 0 {
		contentX = sidebarWidth
	}
	panelGap := 0
	if panelWidth > 0 {
		panelGap = gap
	}
	contentRight := max(contentX, size.X-margin-panelWidth-panelGap)
	return frameRects{
		Sidebar:       image.Rect(0, 0, sidebarWidth, bottom),
		Content:       image.Rect(contentX, top, contentRight, bottom),
		Panel:         image.Rect(size.X-margin-panelWidth, top, size.X-margin, bottom),
		SidebarColumn: sidebarWidth,
	}
}

func panelDockEligible(widthDP float32, class look.Class, collapsed bool) bool {
	sidebar := float32(0)
	switch class {
	case look.Expanded:
		sidebar = float32(look.SidebarExpanded)
		if collapsed {
			sidebar = float32(look.SidebarMedium)
		}
	case look.Medium:
		sidebar = float32(look.SidebarMedium)
	default:
		return false
	}
	return widthDP-sidebar-float32(look.Panel)-3*float32(look.Gap) >= float32(look.ContentMinWithPanel)
}

func (s *uiShell) reconcileFramePanel(class look.Class, panel env.Panel, dockEligible bool) env.Panel {
	if !s.classReady {
		s.lastClass, s.lastDocked, s.classReady = class, panel != env.PanelNone && dockEligible, true
		return panel
	}
	if class == look.Compact && s.lastClass != look.Compact && s.lastDocked && panel != env.PanelNone {
		s.rememberedPanel = panel
		s.nav.OpenPanel(env.PanelNone)
		panel = env.PanelNone
	}
	if class != look.Compact && s.lastClass == look.Compact && dockEligible && panel == env.PanelNone && s.rememberedPanel != env.PanelNone {
		panel = s.rememberedPanel
		s.rememberedPanel = env.PanelNone
		s.nav.OpenPanel(panel)
	}
	s.lastClass = class
	s.lastDocked = panel != env.PanelNone && dockEligible
	return panel
}

// mastheadPage is a page that tells its masthead the part of its place it shows, that
// part's gloss (ungated) and how far it is read (negative when it does not scroll).
type mastheadPage interface {
	Masthead() (part, partGloss string, progress float32)
}

// numberedMastheadPage is a page that also numbers its part, as Settings numbers the
// section being read ("05.4").
type numberedMastheadPage interface {
	Masthead() (number, part, partGloss string, progress float32)
}

// pageSheet is what the masthead and colophon read for the route: its place, with the
// page's own part and number when it gives them, and how far it is read.
func pageSheet(route env.Route, page env.Page, provider string) (sheet.Page, float32) {
	place, part := sheet.ForRoute(route.Kind)
	gloss, progress := "", float32(-1)
	switch page := page.(type) {
	case mastheadPage:
		ownPart, ownGloss, ownProgress := page.Masthead()
		if ownPart != "" {
			part, gloss = ownPart, ownGloss
		}
		progress = ownProgress
	case numberedMastheadPage:
		number, ownPart, ownGloss, ownProgress := page.Masthead()
		if number != "" {
			place.Number = number
		}
		if ownPart != "" {
			part, gloss = ownPart, ownGloss
		}
		progress = ownProgress
	}
	return sheet.Page{Place: place, Part: part, PartGloss: gloss, Provider: provider}, progress
}

// mastheadProvider is the provider a route draws from, for the masthead's facts; pages
// that draw from none (Downloads, Settings, the equalizer) name none.
func mastheadProvider(route env.Route, e *env.Env) string {
	switch route.Kind {
	case env.RouteHome, env.RouteLibrary, env.RouteSearch, env.RouteAlbum, env.RouteArtist, env.RoutePlaylist, env.RouteList:
		return look.ProviderName(e.Th.Src.Provider)
	default:
		return ""
	}
}

// layoutContent draws the page pane's sheet: the registration sheet under the page, the
// masthead over it, the colophon at its foot, and the page on the stage between, rising
// in as it changes (fade 240, 14 dp from the right over 300, OutCubic). Now playing takes
// the whole pane for its cover and draws its own head.
func (s *uiShell) layoutContent(gtx layout.Context, e *env.Env) layout.Dimensions {
	size := gtx.Constraints.Max
	route := e.Nav.Current()
	if e.Nav.NowPlaying() {
		now := e.Store.Snapshot().Playback.Now
		trackID := ""
		if now != nil {
			trackID = now.VideoID
		}
		return s.pageRise.Layout(gtx, "\x00now-playing\x1e"+trackID, func(gtx layout.Context) layout.Dimensions {
			return s.nowPlaying.Layout(gtx, e)
		})
	}
	page := s.page(route)
	key := routeKey(route) + "\x1e" + string(e.Th.Src.Provider)
	if size.Y < gtx.Dp(sheetMinimum) {
		return s.pageRise.Layout(gtx, key, func(gtx layout.Context) layout.Dimensions {
			return page.Layout(gtx, e)
		})
	}
	kit.RegistrationSheet(gtx, e.Th.Theme)

	stageTop := gtx.Dp(mastheadY) + gtx.Dp(mastheadHeight)
	colophonTop := size.Y - gtx.Dp(colophonFoot) - gtx.Dp(colophonHeight)
	layoutAt(gtx, image.Pt(0, stageTop), image.Pt(size.X, max(0, colophonTop-stageTop)), func(gtx layout.Context) layout.Dimensions {
		return s.pageRise.Layout(gtx, key, func(gtx layout.Context) layout.Dimensions {
			return page.Layout(gtx, e)
		})
	})

	provider := mastheadProvider(route, e)
	if size.X < gtx.Dp(mastheadFacts) {
		// a narrow pane keeps its place and clock; the provider shows on the player
		provider = ""
	}
	head, progress := pageSheet(route, page, provider)
	x := gtx.Dp(mastheadX)
	layoutAt(gtx, image.Pt(x, gtx.Dp(mastheadY)), image.Pt(max(0, size.X-x-gtx.Dp(mastheadEnd)), gtx.Dp(mastheadHeight)), func(gtx layout.Context) layout.Dimensions {
		return s.masthead.Layout(gtx, e.Th, head, progress)
	})
	layoutAt(gtx, image.Pt(x, colophonTop), image.Pt(max(0, size.X-x-gtx.Dp(colophonEnd)), gtx.Dp(colophonHeight)), func(gtx layout.Context) layout.Dimensions {
		return s.colophon.Layout(gtx, e.Th, head.Place.ID)
	})
	return layout.Dimensions{Size: size}
}

// layoutPanelSheet draws the queue, lyrics or Listen Together panel as a drawer over the
// page when it cannot dock: from the right, the whole width in the compact class, over a
// scrim that puts it away when pressed.
func (s *uiShell) layoutPanelSheet(gtx layout.Context, e *env.Env, content image.Rectangle, class look.Class, open bool) {
	if content.Empty() {
		return
	}
	extent := min(gtx.Dp(look.Panel), content.Dx())
	if class == look.Compact {
		extent = content.Dx()
	}
	layoutAt(gtx, content.Min, content.Size(), func(gtx layout.Context) layout.Dimensions {
		return s.panelDrawer.Layout(gtx, e.Th, open, look.DrawerRight, extent, func(gtx layout.Context) layout.Dimensions {
			bounds := image.Rectangle{Max: gtx.Constraints.Max}
			paneTop := gtx.Dp(e.Th.Cut.Large)
			paintPlane(gtx, e.Th, bounds, paneTop, gtx.Dp(e.Th.Cut.Mid), e.Th.Planes().Pane, sheetStroke(e.Th))
			return clipped(gtx, e.Th, bounds, paneTop, gtx.Dp(e.Th.Cut.Mid), func(gtx layout.Context) layout.Dimensions {
				return s.rightPanel.Layout(gtx, e)
			})
		})
	})
	if s.panelDrawer.Dismissed() {
		e.Nav.OpenPanel(env.PanelNone)
		gtx.Execute(op.InvalidateCmd{})
	}
}

// layoutSidebarSheet draws the medium class's tree as a drawer from the window's left
// edge over everything, as the Hub's narrow drawer: 272 wide on the window's paper under
// a scrim; choosing a place or pressing the scrim puts it away.
func (s *uiShell) layoutSidebarSheet(gtx layout.Context, e *env.Env, medium bool) {
	open := medium && s.nav.SidebarSheetOpen()
	size := gtx.Constraints.Max
	layer := op.Record(gtx.Ops)
	extent := min(gtx.Dp(look.SidebarExpanded), size.X)
	s.sidebarDrawer.Layout(gtx, e.Th, open, look.DrawerLeft, extent, func(gtx layout.Context) layout.Dimensions {
		bounds := image.Rectangle{Max: gtx.Constraints.Max}
		paint.FillShape(gtx.Ops, e.Th.Planes().Side, clip.Rect(bounds).Op())
		paint.FillShape(gtx.Ops, e.Th.R.Vein, clip.Rect(image.Rect(bounds.Max.X-max(1, gtx.Dp(1)), 0, bounds.Max.X, bounds.Max.Y)).Op())
		return s.sidebarSheet.Layout(gtx, e, look.Expanded, 0)
	})
	if s.sidebarSheet.ConsumeNavigation() || s.sidebarDrawer.Dismissed() {
		s.nav.CloseSidebarSheet()
		gtx.Execute(op.InvalidateCmd{})
	}
	op.Defer(gtx.Ops, layer.Stop())
}

// sheetStroke is a drawer's edge: the design's pane stroke, or the vein where the design
// draws planes by their fill alone, so a drawer over a scrim keeps its edge.
func sheetStroke(th *look.Theme) color.NRGBA {
	if stroke := th.Planes().PaneStroke; stroke.A > 0 {
		return stroke
	}
	return th.R.Vein
}

// layoutPane draws a pane at rectangle: the design's pane plane with its stroke, cut top
// at the top corners and bottom at the foot (round in the round designs), body clipped to
// its shape.
func (s *uiShell) layoutPane(gtx layout.Context, e *env.Env, rectangle image.Rectangle, top, bottom int, body layout.Widget) {
	if rectangle.Empty() {
		return
	}
	planes := e.Th.Planes()
	layoutAt(gtx, rectangle.Min, rectangle.Size(), func(gtx layout.Context) layout.Dimensions {
		bounds := image.Rectangle{Max: gtx.Constraints.Max}
		paintPlane(gtx, e.Th, bounds, top, bottom, planes.Pane, planes.PaneStroke)
		return clipped(gtx, e.Th, bounds, top, bottom, body)
	})
}

// paintPlane fills bounds with fill and strokes it 1 px inside with stroke (none when
// transparent), cut top at the top corners and bottom at the foot in the theme's design.
func paintPlane(gtx layout.Context, th *look.Theme, bounds image.Rectangle, top, bottom int, fill, stroke color.NRGBA) {
	top, bottom = paneCuts(bounds, top, bottom)
	kit.FillCorners(gtx, th.Theme, bounds, top, top, bottom, bottom, fill)
	if stroke.A > 0 {
		kit.StrokeCorners(gtx, th.Theme, bounds, top, top, bottom, bottom, max(1, gtx.Dp(1)), stroke)
	}
}

// clipped lays body out clipped to the pane's shape.
func clipped(gtx layout.Context, th *look.Theme, bounds image.Rectangle, top, bottom int, body layout.Widget) layout.Dimensions {
	top, bottom = paneCuts(bounds, top, bottom)
	stack := clip.Outline{Path: kit.Shape(gtx, th.Theme, bounds, top, top, bottom, bottom)}.Op().Push(gtx.Ops)
	dimensions := body(gtx)
	stack.Pop()
	return dimensions
}

// paneCuts drops the cuts on a pane too small to carry them.
func paneCuts(bounds image.Rectangle, top, bottom int) (int, int) {
	if bounds.Dx() < 2*max(top, bottom) || bounds.Dy() < top+bottom {
		return 0, 0
	}
	return top, bottom
}

func layoutAt(gtx layout.Context, at, size image.Point, widget layout.Widget) layout.Dimensions {
	size.X = max(0, size.X)
	size.Y = max(0, size.Y)
	offset := op.Offset(at).Push(gtx.Ops)
	area := clip.Rect(image.Rectangle{Max: size}).Push(gtx.Ops)
	child := gtx
	child.Constraints = layout.Exact(size)
	widget(child)
	area.Pop()
	offset.Pop()
	return layout.Dimensions{Size: size}
}
