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
	s.reconcileProvider(gtx, e)

	panel := e.Nav.Panel()
	dockEligible := panelDockEligible(widthDP, class)
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

	sidebarPaneWidth := 0
	if class == look.Expanded {
		sidebarPaneWidth = gtx.Dp(look.SidebarExpanded)
	} else if class == look.Medium {
		sidebarPaneWidth = gtx.Dp(look.SidebarMedium)
	}
	sidebarColumn := 0
	contentX := margin
	if sidebarPaneWidth > 0 {
		sidebarColumn = margin + sidebarPaneWidth
		contentX = sidebarColumn + gap
	}

	panelWidth := int(float32(gtx.Dp(look.Panel)) * panelProgress)
	panelGap := 0
	if panelWidth > 0 {
		panelGap = gap
	}
	contentRight := max(contentX, size.X-margin-panelWidth-panelGap)
	contentRect := image.Rect(contentX, topHeight, contentRight, panesBottom)
	frameContentRect := contentRect
	sidebarRect := image.Rect(margin, topHeight, margin+sidebarPaneWidth, panesBottom)
	panelRect := image.Rect(size.X-margin-panelWidth, topHeight, size.X-margin, panesBottom)

	if e.Nav.NowPlaying() {
		contentRect = image.Rect(margin, topHeight, size.X-margin, panesBottom)
		s.layoutPaneRect(gtx, contentRect, gtx.Dp(e.Th.Cut.Large), e.Th.R.Stone2, func(gtx layout.Context) layout.Dimensions {
			return s.layoutContent(gtx, e)
		})
	} else {
		if sidebarPaneWidth > 0 {
			s.layoutPaneRect(gtx, sidebarRect, gtx.Dp(e.Th.Cut.Large), e.Th.R.Stone1, func(gtx layout.Context) layout.Dimensions {
				return s.sidebar.Layout(gtx, e, class)
			})
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
			_ = s.sheetMotion.Value(gtx)
		}
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
		Class: class, Sidebar: sidebarColumn, Content: frameContentRect,
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

func (s *uiShell) reconcileProvider(gtx layout.Context, e *env.Env) {
	provider := e.Store.Snapshot().Provider
	if provider != e.Th.Src.Provider {
		e.Th.SetProvider(gtx, provider)
	}
	e.Th.Animate(gtx)
}

func panelDockEligible(widthDP float32, class look.Class) bool {
	sidebar := float32(0)
	switch class {
	case look.Expanded:
		sidebar = float32(look.SidebarExpanded)
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
