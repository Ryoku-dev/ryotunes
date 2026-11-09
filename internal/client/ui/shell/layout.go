package shell

import (
	"image"

	gioapp "gioui.org/app"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"

	"github.com/ryoku-dev/ryotunes/internal/client/lithic"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
)

const (
	compactWidth  = 760
	expandedWidth = 1280
)

func (s *uiShell) layout(gtx layout.Context, e *env.Env, widthDP float32) layout.Dimensions {
	size := gtx.Constraints.Max
	playerHeight := min(size.Y, gtx.Dp(88))
	contentHeight := max(0, size.Y-playerHeight)
	layoutAt(gtx, image.Pt(0, contentHeight), image.Pt(size.X, playerHeight), func(gtx layout.Context) layout.Dimensions {
		return s.layoutPlayer(gtx, e)
	})
	// Size classes follow the window's compositor dp, not the user's content zoom.
	if widthDP < compactWidth {
		s.layoutCompact(gtx, e, contentHeight)
	} else {
		s.layoutWide(gtx, e, contentHeight, widthDP >= expandedWidth)
	}
	return layout.Dimensions{Size: size}
}

func (s *uiShell) layoutWide(gtx layout.Context, e *env.Env, height int, expanded bool) {
	width := gtx.Constraints.Max.X
	railWidth := gtx.Dp(72)
	if expanded {
		railWidth = gtx.Dp(88)
	}
	gap := gtx.Dp(8)
	top := gtx.Dp(10)
	paneHeight := max(0, height-top)
	layoutAt(gtx, image.Pt(0, top), image.Pt(railWidth, paneHeight), func(gtx layout.Context) layout.Dimensions {
		return s.layoutPane(gtx, e, false, false, func(gtx layout.Context) layout.Dimensions {
			return s.rail.Layout(gtx, e, expanded)
		})
	})

	contentX := railWidth + gap
	panel := e.Nav.Panel()
	showPanel := panel != env.PanelNone
	panelWidth := 0
	if showPanel {
		panelWidth = min(gtx.Dp(360), max(0, width-contentX-gap))
		showPanel = panelWidth > 0
	}
	workWidth := width - contentX
	if showPanel {
		workWidth -= panelWidth + gap
	}
	layoutAt(gtx, image.Pt(contentX, top), image.Pt(workWidth, paneHeight), func(gtx layout.Context) layout.Dimensions {
		return s.layoutPane(gtx, e, true, true, s.workPane(e))
	})
	if showPanel {
		panelX := contentX + workWidth + gap
		layoutAt(gtx, image.Pt(panelX, top), image.Pt(panelWidth, paneHeight), func(gtx layout.Context) layout.Dimensions {
			return s.layoutPane(gtx, e, false, false, s.panelPane(e, panel))
		})
	}
}

func (s *uiShell) layoutCompact(gtx layout.Context, e *env.Env, height int) {
	width := gtx.Constraints.Max.X
	top := gtx.Dp(10)
	gap := gtx.Dp(8)
	bottomHeight := min(gtx.Dp(64), max(0, height-top))
	paneHeight := max(0, height-top-bottomHeight-gap)
	panel := e.Nav.Panel()
	layoutAt(gtx, image.Pt(0, top), image.Pt(width, paneHeight), func(gtx layout.Context) layout.Dimensions {
		return s.layoutPane(gtx, e, true, true, s.workPane(e))
	})
	if panel != env.PanelNone {
		layoutAt(gtx, image.Pt(0, top), image.Pt(width, paneHeight), func(gtx layout.Context) layout.Dimensions {
			return s.layoutPane(gtx, e, false, true, func(gtx layout.Context) layout.Dimensions {
				return s.layoutCompactPanel(gtx, e, panel)
			})
		})
	}
	layoutAt(gtx, image.Pt(0, height-bottomHeight), image.Pt(width, bottomHeight), func(gtx layout.Context) layout.Dimensions {
		paint.Fill(gtx.Ops, e.Th.R.Stone1)
		return s.rail.LayoutBottom(gtx, e)
	})
}

func (s *uiShell) workPane(e *env.Env) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		page := s.page(e.Nav.Current())
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.Y = min(gtx.Constraints.Max.Y, gtx.Dp(56))
				gtx.Constraints.Max.Y = gtx.Constraints.Min.Y
				return s.title.Layout(gtx, e, page)
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return page.Layout(gtx, e)
			}),
		)
	}
}

func (s *uiShell) panelPane(e *env.Env, panel env.Panel) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		switch panel {
		case env.PanelLyrics:
			return s.lyrics.Layout(gtx, e)
		case env.PanelListen:
			return s.listen.Layout(gtx, e)
		case env.PanelNowPlaying:
			return s.nowPlaying.Layout(gtx, e)
		default:
			return s.queue.Layout(gtx, e)
		}
	}
}

func (s *uiShell) layoutCompactPanel(gtx layout.Context, e *env.Env, panel env.Panel) layout.Dimensions {
	dims := layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			height := min(gtx.Constraints.Max.Y, gtx.Dp(48))
			gtx.Constraints.Min.Y = height
			gtx.Constraints.Max.Y = height
			return layout.Inset{Right: 13}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.E.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return s.panelClose.Layout(gtx, e.Th, lithic.Quiet, lithic.Icon("close"), "Close panel")
				})
			})
		}),
		layout.Flexed(1, s.panelPane(e, panel)),
	)
	if s.panelClose.Clicked(gtx) {
		e.Nav.OpenPanel(env.PanelNone)
	}
	return dims
}

func (s *uiShell) layoutPlayer(gtx layout.Context, e *env.Env) layout.Dimensions {
	size := gtx.Constraints.Max
	path := e.Th.Cut.Large
	clipStack := clip.Outline{Path: cutTopPath(gtx, size, gtx.Dp(path))}.Op().Push(gtx.Ops)
	paint.Fill(gtx.Ops, e.Th.R.Stone1)
	dims := s.player.Layout(gtx, e)
	clipStack.Pop()
	return dims
}

func (s *uiShell) layoutPane(gtx layout.Context, e *env.Env, work, controls bool, body layout.Widget) layout.Dimensions {
	size := gtx.Constraints.Max
	path := cutTopPath(gtx, size, gtx.Dp(e.Th.Cut.Large))
	stack := clip.Outline{Path: path}.Op().Push(gtx.Ops)
	plane := e.Th.R.Stone1
	if work {
		plane = e.Th.R.Stone2
	}
	paint.Fill(gtx.Ops, plane)

	strip := 0
	if !s.config.Decorated && s.config.Mode != gioapp.Fullscreen {
		strip = min(size.Y, gtx.Dp(32))
		move := clip.Rect{Max: image.Pt(size.X, strip)}.Push(gtx.Ops)
		s.decor.MoveArea(gtx.Ops)
		move.Pop()
	}
	bodyHeight := max(0, size.Y-strip)
	layoutAt(gtx, image.Pt(0, strip), image.Pt(size.X, bodyHeight), body)
	if controls && strip > 0 {
		macro := op.Record(gtx.Ops)
		controlsWidth := min(size.X, gtx.Dp(116))
		layoutAt(gtx, image.Pt(size.X-controlsWidth-gtx.Dp(6), 0), image.Pt(controlsWidth, strip), func(gtx layout.Context) layout.Dimensions {
			return s.decor.Layout(gtx, e.Th, s.config.Mode)
		})
		op.Defer(gtx.Ops, macro.Stop())
	}
	stack.Pop()
	return layout.Dimensions{Size: size}
}

func cutTopPath(gtx layout.Context, size image.Point, cut int) clip.PathSpec {
	return lithic.CutPath(gtx.Ops, image.Rectangle{Max: size}, cut, cut, 0, 0)
}
