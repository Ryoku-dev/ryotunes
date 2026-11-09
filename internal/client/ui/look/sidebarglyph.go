package look

import (
	"image"
	"math"

	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/anim"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/tokens"
)

// SidebarPlaque is app-owned because the library icon plaque cannot draw a morphing glyph.
type SidebarPlaque struct {
	kit.Click
	Description string
	Open        bool

	press anim.Tween
	hover anim.Tween
	focus anim.Tween
	morph anim.Tween
}

// Clicked reports one activation.
func (plaque *SidebarPlaque) Clicked(gtx layout.Context) bool {
	return plaque.Click.Clicked(gtx)
}

// Layout draws the small quiet plaque and its folding-sidebar state.
func (plaque *SidebarPlaque) Layout(gtx layout.Context, th *Theme) layout.Dimensions {
	original := gtx
	size := gtx.Dp(unit.Dp(tokens.SizeIconPlaqueSmall))
	cut := gtx.Dp(th.Cut.Small)
	dimensions := plaque.Click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		semantic.Button.Add(gtx.Ops)
		semantic.DescriptionOp(plaque.Description).Add(gtx.Ops)
		semantic.EnabledOp(gtx.Source.Enabled()).Add(gtx.Ops)

		pressed := controlTransition(original, &plaque.press, plaque.Pressed(), th.Motion.Press, th.Motion.Press)
		hovered := controlTransition(original, &plaque.hover, plaque.Hovered(), th.Motion.HoverIn, th.Motion.HoverOut)
		cut = gtx.Dp(th.Cut.Small) + int(float32(gtx.Dp(unit.Dp(tokens.SizeControlPressCutGrow)))*pressed+.5)
		sink := int(float32(gtx.Dp(unit.Dp(tokens.SizeControlPressSink)))*pressed + .5)
		bounds := image.Rectangle{Max: gtx.Constraints.Constrain(image.Pt(size, size))}
		if hovered > 0 {
			kit.FillCut(gtx.Ops, bounds, cut, palette.WithAlpha(th.R.Fg2, tokens.AlphaHoverFill*hovered))
		}
		kit.StrokeCut(gtx.Ops, bounds, cut, max(1, gtx.Dp(unit.Dp(tokens.SizeHairline))), th.R.Vein)

		plaque.morph.Duration = th.Motion.Notch
		plaque.morph.Ease = anim.EaseChisel
		target := float32(0)
		if plaque.Open {
			target = 1
		}
		plaque.morph.Set(original, target)
		open := plaque.morph.Value(original)
		glyphSize := min(gtx.Dp(unit.Dp(tokens.SizeIconPlaqueIconSmall)), min(bounds.Dx(), bounds.Dy()))
		move := op.Offset(image.Pt((bounds.Dx()-glyphSize)/2, (bounds.Dy()-glyphSize)/2+sink)).Push(gtx.Ops)
		SidebarGlyph(layout.Exact(image.Pt(glyphSize, glyphSize)), gtx.Ops, th, open)
		move.Pop()
		return layout.Dimensions{Size: bounds.Size()}
	})
	focused := controlTransition(original, &plaque.focus, original.Focused(&plaque.Click.Clickable), th.Motion.FocusIn, th.Motion.FocusOut)
	kit.FocusRingAlpha(original, th.Theme, image.Rectangle{Max: dimensions.Size}, cut, focused)
	return dimensions
}

// SidebarGlyph draws a cut pane frame whose left column fills while the sidebar is open.
func SidebarGlyph(constraints layout.Constraints, ops *op.Ops, th *Theme, open float32) layout.Dimensions {
	size := constraints.Constrain(constraints.Max)
	if size.X <= 0 || size.Y <= 0 {
		return layout.Dimensions{}
	}
	inset := max(1, int(math.Round(float64(min(size.X, size.Y))/9)))
	bounds := image.Rect(inset, inset, size.X-inset, size.Y-inset)
	stroke := max(1, int(math.Round(float64(min(size.X, size.Y))/18)))
	cut := max(stroke, int(math.Round(float64(min(size.X, size.Y))/6)))
	column := max(stroke*2, int(math.Round(float64(bounds.Dx())*.34)))
	columnBounds := image.Rect(bounds.Min.X, bounds.Min.Y, min(bounds.Max.X, bounds.Min.X+column), bounds.Max.Y)
	kit.FillCutCorners(ops, columnBounds, cut, 0, 0, cut, palette.WithAlpha(th.R.Fg2, min(max(open, 0), 1)))
	dividerX := columnBounds.Max.X
	paint.FillShape(ops, th.R.Fg2, clip.Rect(image.Rect(dividerX, bounds.Min.Y+stroke, dividerX+stroke, bounds.Max.Y-stroke)).Op())
	kit.StrokeCut(ops, bounds, cut, stroke, th.R.Fg2)
	return layout.Dimensions{Size: size}
}
