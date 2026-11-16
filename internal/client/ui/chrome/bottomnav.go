package chrome

import (
	"image"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/text"
	"gioui.org/widget"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/sheet"
)

type bottomNavItem struct {
	place sheet.Place
	kind  env.RouteKind
	icon  *widget.Icon
	row   treeRow
}

// BottomNav is the compact class's five places on the window's paper under a hairline:
// the open one wears the travelling selection wash and its filled glyph, each answers the
// pointer with the Hub's row wash.
type BottomNav struct {
	items [5]bottomNavItem
	wash  look.SelectWash
}

func (nav *BottomNav) initialize() {
	if nav.items[0].icon != nil {
		return
	}
	nav.items = [5]bottomNavItem{
		{place: sheet.Home, kind: env.RouteHome, icon: look.Icon.Home},
		{place: sheet.Library, kind: env.RouteLibrary, icon: look.Icon.Library},
		{place: sheet.Radio, kind: env.RouteRadio, icon: look.Icon.Radio},
		{place: sheet.Downloads, kind: env.RouteDownloads, icon: look.Icon.Downloads},
		{place: sheet.Settings, kind: env.RouteSettings, icon: look.Icon.Settings},
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
	th := e.Th
	gtx.Constraints = layout.Exact(size)
	return item.row.click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		item.row.update(gtx, th, selected)
		item.row.wash.Paint(gtx, th, image.Rectangle{Max: size}, gtx.Dp(treeRowCut))
		icon := item.icon
		if selected {
			icon = look.Filled(icon)
		}
		ink := mixInk(th.R.Fg3, th.R.Fg, th.R.Fg, item.row.hover, item.row.sel)
		glyphInk := mixInk(th.R.Fg3, th.R.Fg, th.R.Sel, item.row.hover, item.row.sel)
		iconSize := gtx.Dp(20)
		glyph(gtx, icon, glyphInk, iconSize, size.X/2, size.Y/2-gtx.Dp(8), 0, item.row.wash.GlyphScale())
		label := record(gtx, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Max.X = size.X
			return kit.Label{Style: th.T.Caption, Color: ink, MaxLines: 1, Align: text.Middle}.Layout(gtx, th.Theme, item.place.Name)
		})
		label.at(gtx, (size.X-label.size.X)/2, size.Y/2+gtx.Dp(12))
		return layout.Dimensions{Size: size}
	})
}

// Layout draws the full-width 56 dp compact navigation bar.
func (nav *BottomNav) Layout(gtx layout.Context, e *env.Env) layout.Dimensions {
	if e == nil || e.Th == nil || e.Nav == nil {
		return layout.Dimensions{}
	}
	nav.initialize()
	for index := range nav.items {
		if nav.items[index].row.click.Clicked(gtx) {
			sidebarNavigate(e, env.Route{Kind: nav.items[index].kind, Title: nav.items[index].place.Name})
		}
	}
	width, height := gtx.Constraints.Max.X, gtx.Dp(look.BottomNav)
	size := image.Pt(width, height)
	gtx.Constraints = layout.Exact(size)
	area := clip.Rect(image.Rectangle{Max: size}).Push(gtx.Ops)
	// one paper: the bar is the window's own, drawn off the page by a hairline
	hline(gtx, 0, width, 0, e.Th.R.Vein)
	selected := bottomNavIndex(e.Nav.Current())
	column := width / len(nav.items)
	inset := gtx.Dp(4)
	target := image.Rect(selected*column+inset, inset, (selected+1)*column-inset, height-inset)
	nav.wash.Layout(gtx, e.Th, target, 0, gtx.Dp(treeRowCut))
	for index := range nav.items {
		left := index * column
		right := left + column
		if index == len(nav.items)-1 {
			right = width
		}
		offset := op.Offset(image.Pt(left+inset, inset)).Push(gtx.Ops)
		nav.layoutItem(gtx, e, &nav.items[index], index == selected, image.Pt(max(0, right-left-2*inset), height-2*inset))
		offset.Pop()
	}
	area.Pop()
	return layout.Dimensions{Size: size}
}
