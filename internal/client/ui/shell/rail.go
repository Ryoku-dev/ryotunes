package shell

import (
	"context"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/unit"
	"image"
	"sync/atomic"

	"github.com/ryoku-dev/ryotunes/internal/client/conn"
	"github.com/ryoku-dev/ryotunes/internal/client/lithic"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

type railDestination struct {
	label string
	icon  string
	route env.RouteKind
}

var railDestinations = [...]railDestination{
	{label: "Home", icon: "home", route: env.RouteHome},
	{label: "Search", icon: "search", route: env.RouteSearch},
	{label: "Library", icon: "library", route: env.RouteLibrary},
	{label: "Radio", icon: "radio", route: env.RouteRadio},
	{label: "Downloads", icon: "download", route: env.RouteDownloads},
	{label: "Settings", icon: "settings", route: env.RouteSettings},
}

type rail struct {
	clicks          [len(railDestinations)]lithic.Click
	hovers          [len(railDestinations)]lithic.Tween
	focuses         [len(railDestinations)]lithic.Tween
	selection       lithic.Tween
	updateChecked   atomic.Bool
	updateAvailable atomic.Bool
}

func (r *rail) Layout(gtx layout.Context, e *env.Env, expanded bool) layout.Dimensions {
	r.checkForUpdate(e)
	size := gtx.Constraints.Max
	tile := gtx.Dp(56)
	if expanded {
		tile = gtx.Dp(64)
	}
	gap := gtx.Dp(8)
	top := gtx.Dp(16)
	settingsY := max(top, size.Y-gtx.Dp(16)-tile)
	positions := [len(railDestinations)]int{}
	for index := range positions {
		positions[index] = top + index*(tile+gap)
	}
	positions[len(positions)-1] = settingsY

	selected := railSelection(e.Nav.Current().Kind)
	r.selection.Duration = e.Th.Motion.Select
	r.selection.Ease = lithic.EaseChisel
	r.selection.Set(gtx, float32(positions[selected]))
	selectedY := int(r.selection.Value(gtx) + .5)
	lithic.FillCut(gtx.Ops, image.Rect((size.X-tile)/2, selectedY, (size.X+tile)/2, selectedY+tile), gtx.Dp(e.Th.Cut.Mid), e.Th.R.Sel)

	for index, destination := range railDestinations {
		index, destination := index, destination
		layoutAt(gtx, image.Pt((size.X-tile)/2, positions[index]), image.Pt(tile, tile), func(gtx layout.Context) layout.Dimensions {
			return r.layoutDestination(gtx, e, index, destination, index == selected, expanded)
		})
	}
	return layout.Dimensions{Size: size}
}

func (r *rail) LayoutBottom(gtx layout.Context, e *env.Env) layout.Dimensions {
	r.checkForUpdate(e)
	size := gtx.Constraints.Max
	count := len(railDestinations)
	column := max(1, size.X/count)
	selected := railSelection(e.Nav.Current().Kind)
	r.selection.Duration = e.Th.Motion.Select
	r.selection.Ease = lithic.EaseChisel
	r.selection.Set(gtx, float32(selected*column))
	x := int(r.selection.Value(gtx) + .5)
	lithic.FillCut(gtx.Ops, image.Rect(x+gtx.Dp(4), gtx.Dp(4), min(size.X, x+column-gtx.Dp(4)), size.Y-gtx.Dp(4)), gtx.Dp(e.Th.Cut.Small), e.Th.R.Sel)
	for index, destination := range railDestinations {
		index, destination := index, destination
		width := column
		if index == count-1 {
			width = size.X - index*column
		}
		layoutAt(gtx, image.Pt(index*column, 0), image.Pt(width, size.Y), func(gtx layout.Context) layout.Dimensions {
			return r.layoutDestination(gtx, e, index, destination, index == selected, false)
		})
	}
	return layout.Dimensions{Size: size}
}

func (r *rail) layoutDestination(gtx layout.Context, e *env.Env, index int, destination railDestination, selected, label bool) layout.Dimensions {
	if r.clicks[index].Clicked(gtx) {
		e.Nav.(*navigator).root(env.Route{Kind: destination.route})
	}
	return r.clicks[index].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		semantic.Button.Add(gtx.Ops)
		semantic.LabelOp(destination.label).Add(gtx.Ops)
		r.hovers[index].Duration = e.Th.Motion.HoverOut
		if r.clicks[index].Hovered() {
			r.hovers[index].Duration = e.Th.Motion.HoverIn
			r.hovers[index].Set(gtx, 1)
		} else {
			r.hovers[index].Set(gtx, 0)
		}
		hover := r.hovers[index].Value(gtx)
		if hover > 0 {
			fill := lithic.Mix(e.Th.R.Stone1, e.Th.R.Stone3, hover)
			if selected {
				fill = lithic.Mix(e.Th.R.Sel, e.Th.R.OnSel, .08*hover)
			}
			lithic.FillCut(gtx.Ops, image.Rectangle{Max: gtx.Constraints.Max}, gtx.Dp(e.Th.Cut.Mid), fill)
		}
		r.focuses[index].Duration = e.Th.Motion.FocusOut
		if gtx.Focused(&r.clicks[index].Clickable) {
			r.focuses[index].Duration = e.Th.Motion.FocusIn
			r.focuses[index].Set(gtx, 1)
		} else {
			r.focuses[index].Set(gtx, 0)
		}
		lithic.FocusRingAlpha(gtx, e.Th, image.Rectangle{Max: gtx.Constraints.Max}, gtx.Dp(e.Th.Cut.Mid), r.focuses[index].Value(gtx))
		ink := e.Th.R.Fg2
		if selected {
			ink = e.Th.R.OnSel
		}
		var glyph layout.Widget
		if index == 0 {
			glyph = lithic.BrandMark(e.Th, unit.Dp(40))
		} else {
			icon := lithic.Icon(destination.icon)
			glyph = func(gtx layout.Context) layout.Dimensions {
				sz := gtx.Dp(21)
				gtx.Constraints = layout.Exact(image.Pt(sz, sz))
				return icon.Layout(gtx, ink)
			}
		}
		var dims layout.Dimensions
		if label {
			dims = layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(glyph),
					layout.Rigid(layout.Spacer{Height: 4}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return lithic.Label{Style: e.Th.T.Caption, Color: ink, MaxLines: 1}.Layout(gtx, e.Th, destination.label)
					}),
				)
			})
		} else {
			dims = layout.Center.Layout(gtx, glyph)
		}
		if destination.route == env.RouteSettings && r.updateAvailable.Load() {
			lithic.Notch(gtx.Ops, image.Point{}, gtx.Dp(e.Th.Cut.Notch), e.Th.R.Amber)
		}
		if destination.route == env.RouteDownloads {
			downloads := e.Store.Snapshot().Downloads
			if downloads.ActiveCount+downloads.QueuedCount > 0 {
				lithic.Notch(gtx.Ops, image.Point{}, gtx.Dp(e.Th.Cut.Notch), e.Th.R.Lapis)
			}
		}
		return dims
	})
}

func (r *rail) checkForUpdate(e *env.Env) {
	if !e.Conn.Connected() || !r.updateChecked.CompareAndSwap(false, true) {
		return
	}
	e.Go(func(ctx context.Context) {
		result, err := conn.Do[protocol.Null, model.UpdateInfo](ctx, e.Conn, protocol.MethodCheckForUpdates, protocol.Null{})
		if err != nil {
			r.updateChecked.Store(false)
			return
		}
		r.updateAvailable.Store(result.Available)
	})
}

func railSelection(kind env.RouteKind) int {
	switch kind {
	case env.RouteSearch:
		return 1
	case env.RouteLibrary, env.RouteArtist, env.RouteAlbum, env.RoutePlaylist:
		return 2
	case env.RouteRadio:
		return 3
	case env.RouteDownloads:
		return 4
	case env.RouteSettings, env.RouteDiagnostics:
		return 5
	default:
		return 0
	}
}
