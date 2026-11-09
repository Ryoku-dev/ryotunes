package chrome

import (
	"image"

	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/text"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
)

// TopBar owns the persistent title, navigation, search, provider, and window controls.
type TopBar struct {
	move       kit.Click
	back       kit.IconPlaque
	forward    kit.IconPlaque
	searchOpen kit.IconPlaque
	maximize   kit.IconPlaque
	close      kit.IconPlaque

	search    topSearch
	providers providerSwitcher
	discord   discordToggle

	compactSearch bool
	pending       system.Action
	hasPending    bool
	maximized     bool
}

// FocusSearch opens the compact field when needed and gives it keyboard focus.
func (top *TopBar) FocusSearch(gtx layout.Context) {
	top.compactSearch = true
	top.search.focus(gtx)
}

// SearchFocused reports whether the shared search editor owns keyboard focus.
func (top *TopBar) SearchFocused(gtx layout.Context) bool {
	return top.search.focused(gtx)
}

// WindowAction returns and clears a window action requested by top-bar chrome.
func (top *TopBar) WindowAction() (system.Action, bool) {
	action, ok := top.pending, top.hasPending
	top.pending, top.hasPending = 0, false
	return action, ok
}

func (top *TopBar) request(action system.Action) {
	top.pending, top.hasPending = action, true
}

func topBarAt(gtx layout.Context, rectangle image.Rectangle, widget layout.Widget) layout.Dimensions {
	if rectangle.Dx() <= 0 || rectangle.Dy() <= 0 {
		return layout.Dimensions{}
	}
	offset := op.Offset(rectangle.Min).Push(gtx.Ops)
	child := gtx
	child.Constraints = layout.Exact(rectangle.Size())
	dimensions := widget(child)
	offset.Pop()
	return dimensions
}

func (top *TopBar) layoutMoveRegion(gtx layout.Context, rectangle image.Rectangle) {
	if rectangle.Dx() <= 0 || rectangle.Dy() <= 0 {
		return
	}
	offset := op.Offset(rectangle.Min).Push(gtx.Ops)
	child := gtx
	child.Constraints = layout.Exact(rectangle.Size())
	top.move.Layout(child, func(gtx layout.Context) layout.Dimensions {
		area := clip.Rect(image.Rectangle{Max: rectangle.Size()}).Push(gtx.Ops)
		system.ActionInputOp(system.ActionMove).Add(gtx.Ops)
		area.Pop()
		return layout.Dimensions{Size: rectangle.Size()}
	})
	offset.Pop()
}

func (top *TopBar) handle(gtx layout.Context, e *env.Env, frame Frame) {
	for {
		click, ok := top.move.Update(gtx)
		if !ok {
			break
		}
		if click.NumClicks >= 2 && !frame.Decorated {
			if frame.Maximized {
				top.request(system.ActionUnmaximize)
			} else {
				top.request(system.ActionMaximize)
			}
		}
	}
	acted := false
	if top.back.Clicked(gtx) && e.Nav.CanBack() {
		e.Nav.Back()
		acted = true
	}
	if top.forward.Clicked(gtx) && e.Nav.CanForward() {
		e.Nav.Forward()
		acted = true
	}
	if top.searchOpen.Clicked(gtx) {
		top.compactSearch = true
		top.search.focus(gtx)
		acted = true
	}
	if top.maximize.Clicked(gtx) {
		if frame.Maximized {
			top.request(system.ActionUnmaximize)
		} else {
			top.request(system.ActionMaximize)
		}
		acted = true
	}
	if top.close.Clicked(gtx) {
		top.request(system.ActionClose)
		acted = true
	}
	if acted {
		gtx.Execute(op.InvalidateCmd{})
	}
}

func (top *TopBar) layoutBrand(gtx layout.Context, e *env.Env, frame Frame, height int) {
	markSize := gtx.Dp(24)
	switch frame.Class {
	case look.Expanded:
		topBarAt(gtx, image.Rect(gtx.Dp(21), gtx.Dp(16), gtx.Dp(21)+markSize, gtx.Dp(16)+markSize), look.RyokuMark(24))
		titleX := gtx.Dp(21) + markSize + gtx.Dp(8)
		right := frame.Sidebar
		if right <= titleX {
			right = max(titleX, frame.Content.Min.X)
		}
		topBarAt(gtx, image.Rect(titleX, gtx.Dp(16), right, gtx.Dp(40)), func(gtx layout.Context) layout.Dimensions {
			return kit.Label{Style: e.Th.T.Title, Color: e.Th.R.Fg, MaxLines: 1, Align: text.Start}.Layout(gtx, e.Th.Theme, "Ryotunes")
		})
	case look.Medium:
		column := frame.Sidebar
		if column <= 0 {
			column = gtx.Dp(80)
		}
		x := gtx.Dp(8) + max(0, (column-gtx.Dp(8)-markSize)/2)
		topBarAt(gtx, image.Rect(x, gtx.Dp(16), x+markSize, gtx.Dp(16)+markSize), look.RyokuMark(24))
	default:
		topBarAt(gtx, image.Rect(gtx.Dp(13), gtx.Dp(16), gtx.Dp(13)+markSize, gtx.Dp(16)+markSize), look.RyokuMark(24))
	}
}

func (top *TopBar) layoutNavigation(gtx layout.Context, e *env.Env, frame Frame, start int) int {
	if frame.Class == look.Compact {
		if top.compactSearch {
			return start
		}
		top.searchOpen.Kind, top.searchOpen.Icon, top.searchOpen.Description, top.searchOpen.Small = kit.PlaqueQuiet, look.Icon.Search, "Search", true
		topBarAt(gtx, image.Rect(start, gtx.Dp(12), start+gtx.Dp(32), gtx.Dp(44)), func(gtx layout.Context) layout.Dimensions {
			return top.searchOpen.Layout(gtx, e.Th.Theme)
		})
		return start + gtx.Dp(32)
	}

	canBack, canForward := e.Nav.CanBack(), e.Nav.CanForward()
	top.back.Kind, top.back.Icon, top.back.Description, top.back.Small = kit.PlaqueQuiet, look.Icon.Back, "Back", true
	top.forward.Kind, top.forward.Icon, top.forward.Description, top.forward.Small = kit.PlaqueQuiet, look.Icon.Forward, "Forward", true
	topBarAt(gtx, image.Rect(start, gtx.Dp(12), start+gtx.Dp(32), gtx.Dp(44)), func(gtx layout.Context) layout.Dimensions {
		if !canBack {
			gtx = gtx.Disabled()
		}
		return top.back.Layout(gtx, e.Th.Theme)
	})
	start += gtx.Dp(36)
	topBarAt(gtx, image.Rect(start, gtx.Dp(12), start+gtx.Dp(32), gtx.Dp(44)), func(gtx layout.Context) layout.Dimensions {
		if !canForward {
			gtx = gtx.Disabled()
		}
		return top.forward.Layout(gtx, e.Th.Theme)
	})
	return start + gtx.Dp(32)
}

func (top *TopBar) layoutWindowControls(gtx layout.Context, e *env.Env, frame Frame, start int) {
	if frame.Decorated {
		return
	}
	y := gtx.Dp(12)
	size, gap := gtx.Dp(32), gtx.Dp(4)
	maximizeIcon, maximizeDescription := look.Icon.Maximize, "Maximize"
	if frame.Maximized {
		maximizeIcon, maximizeDescription = look.Icon.Restore, "Restore"
	}
	top.maximize.Kind, top.maximize.Icon, top.maximize.Description, top.maximize.Small = kit.PlaqueQuiet, maximizeIcon, maximizeDescription, true
	top.close.Kind, top.close.Icon, top.close.Description, top.close.Small = kit.PlaqueQuiet, look.Icon.Close, "Close", true
	buttons := []*kit.IconPlaque{&top.maximize, &top.close}
	for index, button := range buttons {
		x := start + index*(size+gap)
		topBarAt(gtx, image.Rect(x, y, x+size, y+size), func(gtx layout.Context) layout.Dimensions {
			return button.Layout(gtx, e.Th.Theme)
		})
	}
}

// Layout draws the 56 dp full-window top strip on the shell's ground plane.
func (top *TopBar) Layout(gtx layout.Context, e *env.Env, frame Frame) layout.Dimensions {
	if e == nil || e.Th == nil || e.Nav == nil || e.Store == nil {
		return layout.Dimensions{}
	}
	top.maximized = frame.Maximized

	width := gtx.Constraints.Max.X
	height := gtx.Dp(look.TopBar)
	size := image.Pt(width, height)
	gtx.Constraints = layout.Exact(size)
	area := clip.Rect(image.Rectangle{Max: size}).Push(gtx.Ops)

	top.layoutBrand(gtx, e, frame, height)
	controlWidth := 0
	if !frame.Decorated {
		controlWidth = gtx.Dp(68)
	}
	controlsX := width - gtx.Dp(13) - controlWidth
	trailingRight := width - gtx.Dp(13)
	if !frame.Decorated {
		trailingRight = controlsX - gtx.Dp(13)
	}
	providerWidth := gtx.Dp(180)
	if frame.Class == look.Compact {
		providerWidth = gtx.Dp(40)
	}
	providerX := trailingRight - providerWidth
	discordX := providerX - gtx.Dp(13) - gtx.Dp(32)

	providerAnchor := image.Rect(providerX, gtx.Dp(8), providerX+providerWidth, gtx.Dp(48))
	if frame.Class == look.Compact {
		topBarAt(gtx, providerAnchor, func(gtx layout.Context) layout.Dimensions {
			return top.providers.layoutCompact(gtx, e, providerAnchor)
		})
	} else {
		topBarAt(gtx, providerAnchor, func(gtx layout.Context) layout.Dimensions {
			return top.providers.layoutExpanded(gtx, e)
		})
	}
	topBarAt(gtx, image.Rect(discordX, gtx.Dp(12), discordX+gtx.Dp(32), gtx.Dp(44)), func(gtx layout.Context) layout.Dimensions {
		return top.discord.layout(gtx, e)
	})

	leading := frame.Content.Min.X
	if leading <= 0 {
		leading = frame.Sidebar + gtx.Dp(8)
	}
	if frame.Class == look.Compact {
		leading = gtx.Dp(50)
	}
	navigationEnd := top.layoutNavigation(gtx, e, frame, leading)
	searchX := navigationEnd + gtx.Dp(13)
	if frame.Class == look.Compact {
		searchX = leading
	}
	searchRight := min(searchX+gtx.Dp(520), discordX-gtx.Dp(13))
	if frame.Class == look.Compact && !top.compactSearch {
		searchRight = searchX
	}
	if searchRight > searchX {
		anchor := image.Rect(searchX, gtx.Dp(6), searchRight, gtx.Dp(50))
		topBarAt(gtx, anchor, func(gtx layout.Context) layout.Dimensions {
			return top.search.layout(gtx, e, anchor)
		})
	}
	if frame.Class == look.Compact && top.compactSearch && !top.search.active && topSearchQuery(top.search.field.Text()) == "" {
		top.compactSearch = false
	}

	top.layoutWindowControls(gtx, e, frame, controlsX)

	top.layoutMoveRegion(gtx, image.Rect(0, 0, leading, height))
	if searchX > navigationEnd {
		top.layoutMoveRegion(gtx, image.Rect(navigationEnd, 0, searchX, height))
	}
	interactiveEnd := max(navigationEnd, searchRight)
	top.layoutMoveRegion(gtx, image.Rect(interactiveEnd, 0, discordX, height))
	top.layoutMoveRegion(gtx, image.Rect(discordX+gtx.Dp(32), 0, providerX, height))
	controlsStart, controlsEnd := width, width
	if !frame.Decorated {
		controlsStart = controlsX
		controlsEnd = width - gtx.Dp(13)
	}
	top.layoutMoveRegion(gtx, image.Rect(providerX+providerWidth, 0, controlsStart, height))
	top.layoutMoveRegion(gtx, image.Rect(controlsEnd, 0, width, height))
	top.handle(gtx, e, frame)

	area.Pop()
	return layout.Dimensions{Size: size}
}
