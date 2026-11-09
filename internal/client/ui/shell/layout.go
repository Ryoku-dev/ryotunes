package shell

import (
	"image"
	"image/color"

	gioapp "gioui.org/app"
	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/anim"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/chrome"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
)

func (s *uiShell) layout(gtx layout.Context, e *env.Env, widthDP float32) layout.Dimensions {
	size := gtx.Constraints.Max
	paint.Fill(gtx.Ops, e.Th.R.Ground)
	class := look.ClassFor(widthDP)
	e.Th.Class = class
	s.nav.setSidebarClass(class)
	s.reconcileProvider(gtx, e)

	collapsedTarget := class == look.Expanded && e.Nav.SidebarCollapsed()
	panel := e.Nav.Panel()
	dockEligible := panelDockEligible(widthDP, class, collapsedTarget)
	panel = s.reconcileFramePanel(class, panel, dockEligible)
	docked := panel != env.PanelNone && dockEligible && !e.Nav.NowPlaying()

	s.panelMotion.Duration = e.Th.Motion.Unfold
	s.panelMotion.Ease = anim.EaseChisel
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
	playerY := max(topHeight, size.Y-margin-playerHeight-bottomNavHeight)
	panesBottom := max(topHeight, playerY-gap)

	sidebarPaneWidth, sidebarProgress := 0, float32(0)
	switch class {
	case look.Expanded:
		target := float32(0)
		if collapsedTarget {
			target = 1
		}
		s.sidebarMotion.Duration = e.Th.Motion.Unfold
		s.sidebarMotion.Ease = anim.EaseChisel
		if !s.sidebarMotionReady {
			s.sidebarMotion.Set(gtx, target)
			s.sidebarMotionReady = true
		}
		s.sidebarMotion.Set(gtx, target)
		sidebarProgress = s.sidebarMotion.Value(gtx)
		expanded, rail := gtx.Dp(look.SidebarExpanded), gtx.Dp(look.SidebarMedium)
		sidebarPaneWidth = expanded + int(float32(rail-expanded)*sidebarProgress+.5)
	case look.Medium:
		sidebarPaneWidth, sidebarProgress = gtx.Dp(look.SidebarMedium), 1
	}

	panelWidth := int(float32(gtx.Dp(look.Panel))*panelProgress + .5)
	rects := resolveFrameRects(size, topHeight, panesBottom, margin, gap, sidebarPaneWidth, panelWidth)
	frameContentRect := rects.Content
	contentRect, sidebarRect, panelRect := rects.Content, rects.Sidebar, rects.Panel

	if e.Nav.NowPlaying() {
		contentRect = image.Rect(margin, topHeight, size.X-margin, panesBottom)
		s.layoutPaneRect(gtx, contentRect, gtx.Dp(e.Th.Cut.Large), e.Th.R.Stone2, func(gtx layout.Context) layout.Dimensions {
			return s.layoutContent(gtx, e)
		})
	} else {
		if sidebarPaneWidth > 0 {
			s.layoutPaneRect(gtx, sidebarRect, gtx.Dp(e.Th.Cut.Large), e.Th.R.Stone1, func(gtx layout.Context) layout.Dimensions {
				return s.sidebar.Layout(gtx, e, class, sidebarProgress)
			})
			s.sidebar.ConsumeNavigation()
		}
		s.layoutPaneRect(gtx, contentRect, gtx.Dp(e.Th.Cut.Large), e.Th.R.Stone2, func(gtx layout.Context) layout.Dimensions {
			return s.layoutContent(gtx, e)
		})
		if panelWidth > 0 {
			opacity := paint.PushOpacity(gtx.Ops, panelProgress)
			s.layoutPaneRect(gtx, panelRect, gtx.Dp(e.Th.Cut.Large), e.Th.R.Stone1, func(gtx layout.Context) layout.Dimensions {
				return s.rightPanel.Layout(gtx, e)
			})
			opacity.Pop()
		}
		if panel != env.PanelNone && !docked {
			s.layoutPanelSheet(gtx, e, contentRect, class)
		} else {
			s.sheetMotion.Duration = e.Th.Motion.MenuOut
			s.sheetMotion.Set(gtx, 0)
			s.sheetMotion.Value(gtx)
		}
	}
	if class == look.Medium {
		s.layoutSidebarSheet(gtx, e, frameContentRect)
	} else {
		s.sidebarSheetMotion.Duration = e.Th.Motion.MenuOut
		s.sidebarSheetMotion.Set(gtx, 0)
		s.sidebarSheetMotion.Value(gtx)
	}

	playerRect := image.Rect(margin, playerY, size.X-margin, playerY+playerHeight)
	s.layoutPaneRect(gtx, playerRect, gtx.Dp(e.Th.Cut.Large), e.Th.R.Stone1, func(gtx layout.Context) layout.Dimensions {
		if class == look.Compact {
			return s.player.LayoutCompact(gtx, e)
		}
		return s.player.Layout(gtx, e)
	})
	if class == look.Compact {
		bottomRect := image.Rect(0, size.Y-bottomNavHeight, size.X, size.Y)
		s.layoutPaneRect(gtx, bottomRect, gtx.Dp(e.Th.Cut.Large), e.Th.R.Stone1, func(gtx layout.Context) layout.Dimensions {
			return s.bottomNav.Layout(gtx, e)
		})
	}

	frame := chrome.Frame{
		Class: class, Sidebar: rects.SidebarColumn, SidebarProgress: sidebarProgress,
		SidebarSheet: s.nav.SidebarSheetOpen(), Content: frameContentRect,
		Decorated: s.config.Decorated, Maximized: s.config.Mode == gioapp.Maximized,
	}
	layoutAt(gtx, image.Point{}, image.Pt(size.X, topHeight), func(gtx layout.Context) layout.Dimensions {
		return s.topBar.Layout(gtx, e, frame)
	})
	if action, ok := s.topBar.WindowAction(); ok {
		s.performWindowAction(action)
	}
	return layout.Dimensions{Size: size}
}

type frameRects struct {
	Sidebar       image.Rectangle
	Content       image.Rectangle
	Panel         image.Rectangle
	SidebarColumn int
}

func resolveFrameRects(size image.Point, top, bottom, margin, gap, sidebarWidth, panelWidth int) frameRects {
	sidebarColumn := 0
	contentX := margin
	if sidebarWidth > 0 {
		sidebarColumn = margin + sidebarWidth
		contentX = sidebarColumn + gap
	}
	panelGap := 0
	if panelWidth > 0 {
		panelGap = gap
	}
	contentRight := max(contentX, size.X-margin-panelWidth-panelGap)
	return frameRects{
		Sidebar:       image.Rect(margin, top, margin+sidebarWidth, bottom),
		Content:       image.Rect(contentX, top, contentRight, bottom),
		Panel:         image.Rect(size.X-margin-panelWidth, top, size.X-margin, bottom),
		SidebarColumn: sidebarColumn,
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

func (s *uiShell) layoutContent(gtx layout.Context, e *env.Env) layout.Dimensions {
	route := e.Nav.Current()
	key := routeKey(route) + "\x1e" + string(e.Th.Src.Provider)
	page := s.page(route)
	widget := layout.Widget(func(gtx layout.Context) layout.Dimensions {
		return page.Layout(gtx, e)
	})
	if e.Nav.NowPlaying() {
		now := e.Store.Snapshot().Playback.Now
		trackID := ""
		if now != nil {
			trackID = now.VideoID
		}
		key = "\x00now-playing\x1e" + trackID
		widget = func(gtx layout.Context) layout.Dimensions { return s.nowPlaying.Layout(gtx, e) }
	}
	if s.transitionWidgets == nil {
		s.transitionWidgets = make(map[string]layout.Widget)
	}
	s.transitionWidgets[key] = widget
	return s.contentSwap.Layout(gtx, e.Th, key, 8, e.Th.Motion.Navigate, func(gtx layout.Context, selected string) layout.Dimensions {
		if content := s.transitionWidgets[selected]; content != nil {
			return content(gtx)
		}
		return layout.Dimensions{}
	})
}

func (s *uiShell) layoutPanelSheet(gtx layout.Context, e *env.Env, content image.Rectangle, class look.Class) {
	s.sheetMotion.Duration = e.Th.Motion.MenuIn
	s.sheetMotion.Ease = anim.EaseChisel
	if !s.sheetReady {
		s.sheetMotion.Set(gtx, 0)
		s.sheetReady = true
	}
	s.sheetMotion.Set(gtx, 1)
	progress := s.sheetMotion.Value(gtx)
	if progress <= 0 || content.Empty() {
		return
	}
	for {
		raw, ok := gtx.Event(pointer.Filter{Target: &s.sheetOutside, Kinds: pointer.Press})
		if !ok {
			break
		}
		if _, ok := raw.(pointer.Event); ok {
			e.Nav.OpenPanel(env.PanelNone)
			gtx.Execute(op.InvalidateCmd{})
		}
	}
	catcherOffset := op.Offset(content.Min).Push(gtx.Ops)
	catcher := clip.Rect(image.Rectangle{Max: content.Size()}).Push(gtx.Ops)
	event.Op(gtx.Ops, &s.sheetOutside)
	catcher.Pop()
	catcherOffset.Pop()

	width := min(gtx.Dp(look.Panel), content.Dx())
	if class == look.Compact {
		width = content.Dx()
	}
	rect := image.Rect(content.Max.X-width, content.Min.Y, content.Max.X, content.Max.Y)
	travel := int(float32(gtx.Dp(6)) * (1 - progress))
	rect = rect.Add(image.Pt(travel, 0))
	opacity := paint.PushOpacity(gtx.Ops, progress)
	s.layoutPaneRect(gtx, rect, gtx.Dp(e.Th.Cut.Large), e.Th.R.Stone4, func(gtx layout.Context) layout.Dimensions {
		return s.rightPanel.Layout(gtx, e)
	})
	opacity.Pop()
}

func (s *uiShell) layoutSidebarSheet(gtx layout.Context, e *env.Env, content image.Rectangle) {
	desired := s.nav.SidebarSheetOpen()
	s.sidebarSheetMotion.Ease = anim.EaseChisel
	if desired {
		s.sidebarSheetMotion.Duration = e.Th.Motion.MenuIn
	} else {
		s.sidebarSheetMotion.Duration = e.Th.Motion.MenuOut
	}
	if !s.sidebarSheetReady {
		s.sidebarSheetMotion.Set(gtx, 0)
		s.sidebarSheetReady = true
	}
	target := float32(0)
	if desired {
		target = 1
	}
	s.sidebarSheetMotion.Set(gtx, target)
	progress := s.sidebarSheetMotion.Value(gtx)
	if progress <= 0 || content.Empty() {
		return
	}
	for {
		raw, ok := gtx.Event(pointer.Filter{Target: &s.sidebarSheetOutside, Kinds: pointer.Press})
		if !ok {
			break
		}
		if _, ok := raw.(pointer.Event); ok && desired {
			s.nav.CloseSidebarSheet()
			desired = false
			gtx.Execute(op.InvalidateCmd{})
		}
	}

	width := min(gtx.Dp(look.SidebarExpanded), content.Dx())
	rect := image.Rect(content.Min.X, content.Min.Y, content.Min.X+width, content.Max.Y)
	layer := op.Record(gtx.Ops)
	registerOutside := func(rectangle image.Rectangle) {
		if rectangle.Empty() {
			return
		}
		area := clip.Rect(rectangle).Push(gtx.Ops)
		event.Op(gtx.Ops, &s.sidebarSheetOutside)
		area.Pop()
	}
	window := image.Rectangle{Max: gtx.Constraints.Max}
	registerOutside(image.Rect(window.Min.X, window.Min.Y, window.Max.X, rect.Min.Y))
	registerOutside(image.Rect(window.Min.X, rect.Max.Y, window.Max.X, window.Max.Y))
	registerOutside(image.Rect(window.Min.X, rect.Min.Y, rect.Min.X, rect.Max.Y))
	registerOutside(image.Rect(rect.Max.X, rect.Min.Y, window.Max.X, rect.Max.Y))
	sheetBlocker := clip.Rect(rect).Push(gtx.Ops)
	event.Op(gtx.Ops, &s.sidebarSheet)
	sheetBlocker.Pop()

	travel := gtx.Dp(6)
	if !desired {
		travel = gtx.Dp(4)
	}
	rect = rect.Add(image.Pt(-int(float32(travel)*(1-progress)+.5), 0))
	opacity := paint.PushOpacity(gtx.Ops, progress)
	s.layoutPaneRect(gtx, rect, gtx.Dp(e.Th.Cut.Large), e.Th.R.Stone1, func(gtx layout.Context) layout.Dimensions {
		return s.sidebarSheet.Layout(gtx, e, look.Expanded, 0)
	})
	opacity.Pop()
	if s.sidebarSheet.ConsumeNavigation() {
		s.nav.CloseSidebarSheet()
	}
	op.Defer(gtx.Ops, layer.Stop())
}

func (s *uiShell) layoutPaneRect(gtx layout.Context, rectangle image.Rectangle, cut int, plane color.NRGBA, body layout.Widget) {
	if rectangle.Empty() {
		return
	}
	layoutAt(gtx, rectangle.Min, rectangle.Size(), func(gtx layout.Context) layout.Dimensions {
		bounds := image.Rectangle{Max: gtx.Constraints.Max}
		if bounds.Dx() < 2*cut || bounds.Dy() < 2*cut {
			cut = 0
		}
		path := kit.CutPath(gtx.Ops, bounds, cut, cut, 0, 0)
		kit.FillCutTop(gtx.Ops, bounds, cut, plane)
		stack := clip.Outline{Path: path}.Op().Push(gtx.Ops)
		dimensions := body(gtx)
		stack.Pop()
		return dimensions
	})
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
