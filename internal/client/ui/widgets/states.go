package widgets

import (
	"image"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
)

// LoadingShape selects a page-shaped static loading state.
type LoadingShape uint8

const (
	LoadingTable LoadingShape = iota
	LoadingGrid
	LoadingHero
	LoadingShelf
)

func loadingTable(gtx layout.Context, e *env.Env) layout.Dimensions {
	return kit.Skeleton{Active: true, Rows: 8}.Layout(gtx, e.Th.Theme)
}

func loadingGrid(gtx layout.Context, e *env.Env) layout.Dimensions {
	gap := gtx.Dp(21)
	columns := gridColumns(gtx.Constraints.Max.X, gap, gtx.Dp(160))
	available := gtx.Constraints.Max.X - gap*(columns-1)
	width := min(gtx.Dp(200), max(gtx.Dp(160), available/max(1, columns)))
	children := make([]layout.FlexChild, 0, columns*2)
	for column := range columns {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			size := image.Pt(width, width+gtx.Dp(64))
			gtx.Constraints = layout.Exact(size)
			kit.FillCut(gtx.Ops, image.Rect(0, 0, width, width), gtx.Dp(e.Th.Cut.Mid), e.Th.R.Stone3)
			skeletonGTX := gtx
			skeletonGTX.Constraints = layout.Exact(image.Pt(width, gtx.Dp(56)))
			offset := op.Offset(image.Pt(0, width+gtx.Dp(8))).Push(gtx.Ops)
			kit.Skeleton{Active: true, Rows: 1}.Layout(skeletonGTX, e.Th.Theme)
			offset.Pop()
			return layout.Dimensions{Size: size}
		}))
		if column+1 < columns {
			children = append(children, layout.Rigid(layout.Spacer{Width: unit.Dp(21)}.Layout))
		}
	}
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Start}.Layout(gtx, children...)
}

func loadingShelf(gtx layout.Context, e *env.Env) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical, Alignment: layout.Start}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(220))
			return kit.Skeleton{Active: true, Rows: 1}.Layout(gtx, e.Th.Theme)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(13)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return loadingGrid(gtx, e)
		}),
	)
}

func loadingHero(gtx layout.Context, e *env.Env) layout.Dimensions {
	art := heroArtSize(gtx.Metric.PxToDp(gtx.Constraints.Max.X))
	pixels := gtx.Dp(art)
	cut := gtx.Dp(e.Th.Cut.Large)
	return look.Field(gtx, e.Th, [4]int{cut, cut, 0, 0}, func(gtx layout.Context) layout.Dimensions {
		inset := look.ContentInset(e.Th.Class)
		return layout.Inset{Top: inset, Right: inset, Bottom: inset, Left: inset}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					size := image.Pt(pixels, pixels)
					gtx.Constraints = layout.Exact(size)
					kit.FillCut(gtx.Ops, image.Rectangle{Max: size}, gtx.Dp(e.Th.Cut.Mid), e.Th.R.Stone3)
					return layout.Dimensions{Size: size}
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(21)}.Layout),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return kit.Skeleton{Active: true, Rows: 3}.Layout(gtx, e.Th.Theme)
				}),
			)
		})
	})
}

// Loading returns a static loading state shaped like its destination.
func Loading(e *env.Env, shape LoadingShape) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		if e == nil || e.Th == nil {
			return layout.Dimensions{}
		}
		switch shape {
		case LoadingGrid:
			return loadingGrid(gtx, e)
		case LoadingHero:
			return loadingHero(gtx, e)
		case LoadingShelf:
			return loadingShelf(gtx, e)
		default:
			return loadingTable(gtx, e)
		}
	}
}

// Failed returns an error state with an optional Retry action.
func Failed(e *env.Env, message string, retry *kit.Plaque) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		if retry != nil {
			retry.Kind = kit.PlaqueLine
			if retry.Label == "" {
				retry.Label = "Retry"
			}
		}
		return (kit.Empty{
			Icon: look.Icon.Refresh, Title: "Could not load this page", Body: message,
			Actions: []*kit.Plaque{retry},
		}).Layout(gtx, e.Th.Theme)
	}
}

// EmptyState returns a centred state that names the next useful step.
func EmptyState(e *env.Env, icon *widget.Icon, title, body string, actions ...*kit.Plaque) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return (kit.Empty{Icon: icon, Title: title, Body: body, Actions: actions}).Layout(gtx, e.Th.Theme)
	}
}
