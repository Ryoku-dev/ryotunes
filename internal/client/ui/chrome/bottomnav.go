package chrome

import (
	"image"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/widget"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/anim"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
)

type bottomNavItem struct {
	label string
	kind  env.RouteKind
	icon  *widget.Icon
	click kit.Click
	hover look.Hover
}

// BottomNav owns the five compact navigation destinations.
type BottomNav struct {
	items     [5]bottomNavItem
	selection anim.Tween
}

func (nav *BottomNav) initialize() {
	if nav.items[0].label != "" {
		return
	}
	nav.items = [5]bottomNavItem{
		{label: "Home", kind: env.RouteHome, icon: look.Icon.Home},
		{label: "Library", kind: env.RouteLibrary, icon: look.Icon.Library},
		{label: "Radio", kind: env.RouteRadio, icon: look.Icon.Radio},
		{label: "Downloads", kind: env.RouteDownloads, icon: look.Icon.Downloads},
		{label: "Settings", kind: env.RouteSettings, icon: look.Icon.Settings},
	}
}

func bottomNavIndex(route env.Route) int {
	switch route.Kind {
	case env.RouteHome, env.RouteSearch:
		return 0
	case env.RouteLibrary, env.RouteAlbum, env.RouteArtist, env.RoutePlaylist, env.RouteList:
		return 1
	case env.RouteRadio:
		return 2
	case env.RouteDownloads:
		return 3
	case env.RouteSettings, env.RouteDiagnostics:
		return 4
	default:
		return 0
	}
}

func (nav *BottomNav) layoutItem(gtx layout.Context, e *env.Env, item *bottomNavItem, selected bool, size image.Point) layout.Dimensions {
	gtx.Constraints = layout.Exact(size)
	return item.click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		record := op.Record(gtx.Ops)
		dimensions, hover := item.hover.Layout(gtx, e.Th, func(gtx layout.Context) layout.Dimensions {
			ink := e.Th.R.Fg3
			if item.click.Hovered() {
				ink = e.Th.R.Fg2
			}
			if selected {
				ink = e.Th.R.OnSel
			}
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions { return sidebarIcon(gtx, item.icon, ink, 18) }),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return kit.Label{Style: e.Th.T.Caption, Color: ink, MaxLines: 1, Align: text.Middle}.Layout(gtx, e.Th.Theme, item.label)
					}),
				)
			})
		})
		content := record.Stop()
		area := clip.Rect(image.Rectangle{Max: dimensions.Size}).Push(gtx.Ops)
		if hover > 0 && !selected {
			kit.FillCut(gtx.Ops, image.Rectangle{Max: dimensions.Size}, gtx.Dp(6), palette.WithAlpha(e.Th.R.Stone3, hover))
		}
		content.Add(gtx.Ops)
		area.Pop()
		return dimensions
	})
}

// Layout draws the full-width 56 dp compact navigation bar.
func (nav *BottomNav) Layout(gtx layout.Context, e *env.Env) layout.Dimensions {
	if e == nil || e.Th == nil || e.Nav == nil {
		return layout.Dimensions{}
	}
	nav.initialize()
	for index := range nav.items {
		if nav.items[index].click.Clicked(gtx) {
			sidebarNavigate(e, env.Route{Kind: nav.items[index].kind, Title: nav.items[index].label})
		}
	}
	width, height := gtx.Constraints.Max.X, gtx.Dp(look.BottomNav)
	size := image.Pt(width, height)
	gtx.Constraints = layout.Exact(size)
	area := clip.Rect(image.Rectangle{Max: size}).Push(gtx.Ops)
	paint.Fill(gtx.Ops, e.Th.R.Stone1)
	selected := bottomNavIndex(e.Nav.Current())
	nav.selection.Duration = e.Th.Motion.Select
	nav.selection.Ease = anim.EaseChisel
	nav.selection.Set(gtx, float32(selected))
	at := nav.selection.Value(gtx)
	column := width / len(nav.items)
	x := int(at*float32(column)) + gtx.Dp(4)
	kit.FillCut(gtx.Ops, image.Rect(x, gtx.Dp(4), x+max(0, column-gtx.Dp(8)), gtx.Dp(52)), gtx.Dp(6), e.Th.R.Sel)
	for index := range nav.items {
		left := index * column
		right := left + column
		if index == len(nav.items)-1 {
			right = width
		}
		offset := op.Offset(image.Pt(left+gtx.Dp(4), gtx.Dp(4))).Push(gtx.Ops)
		nav.layoutItem(gtx, e, &nav.items[index], index == selected, image.Pt(max(0, right-left-gtx.Dp(8)), gtx.Dp(48)))
		offset.Pop()
	}
	area.Pop()
	return layout.Dimensions{Size: size}
}
