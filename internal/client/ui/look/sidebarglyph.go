package look

import (
	"image"
	"image/color"
	"math"

	"gioui.org/f32"
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

// SidebarPlaque is the top bar's sidebar toggle. It is app-owned because the library's
// icon plaque cannot draw a morphing glyph: the pane frame's left column fills as the
// sidebar opens (th.Motion.Notch, chisel). It answers the pointer with the Hub's row wash
// (120 in, 240 out on OutCubic), the plaque's press cut and sink, and its corners follow
// the desktop's design.
type SidebarPlaque struct {
	kit.Click
	Description string
	Open        bool

	wash  RowWash
	press anim.Tween
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

		enabled := gtx.Source.Enabled()
		plaque.wash.Update(original, th, enabled && plaque.Hovered(), enabled && plaque.Pressed())
		pressed := controlTransition(original, &plaque.press, plaque.Pressed(), th.Motion.Press, th.Motion.Press)
		cut = gtx.Dp(th.Cut.Small) + int(float32(gtx.Dp(unit.Dp(tokens.SizeControlPressCutGrow)))*pressed+.5)
		sink := int(float32(gtx.Dp(unit.Dp(tokens.SizeControlPressSink)))*pressed + .5)
		bounds := image.Rectangle{Max: gtx.Constraints.Constrain(image.Pt(size, size))}
		plaque.wash.Paint(gtx, th, bounds, cut)

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
		ink := palette.Mix(th.R.Fg2, th.R.Fg, plaque.wash.hovered)
		glyphGtx := gtx
		glyphGtx.Constraints = layout.Exact(image.Pt(glyphSize, glyphSize))
		SidebarGlyph(glyphGtx, th, ink, open, plaque.wash.GlyphScale())
		move.Pop()
		return layout.Dimensions{Size: bounds.Size()}
	})
	focused := controlTransition(original, &plaque.focus, original.Focused(&plaque.Click.Clickable), th.Motion.FocusIn, th.Motion.FocusOut)
	kit.FocusRingAlpha(original, th.Theme, image.Rectangle{Max: dimensions.Size}, cut, focused)
	return dimensions
}

// SidebarGlyph draws a pane frame whose left column fills while the sidebar is open
// (open, 0..1), in ink, scaled about its centre, with the design's corners.
func SidebarGlyph(gtx layout.Context, th *Theme, ink color.NRGBA, open, scale float32) layout.Dimensions {
	size := gtx.Constraints.Max
	if size.X <= 0 || size.Y <= 0 {
		return layout.Dimensions{}
	}
	if scale != 1 {
		centre := f32.Pt(float32(size.X)/2, float32(size.Y)/2)
		defer op.Affine(f32.AffineId().Scale(centre, f32.Pt(scale, scale))).Push(gtx.Ops).Pop()
	}
	inset := max(1, int(math.Round(float64(min(size.X, size.Y))/9)))
	bounds := image.Rect(inset, inset, size.X-inset, size.Y-inset)
	stroke := max(1, int(math.Round(float64(min(size.X, size.Y))/18)))
	cut := max(stroke, int(math.Round(float64(min(size.X, size.Y))/6)))
	column := max(stroke*2, int(math.Round(float64(bounds.Dx())*.34)))
	columnBounds := image.Rect(bounds.Min.X, bounds.Min.Y, min(bounds.Max.X, bounds.Min.X+column), bounds.Max.Y)
	kit.FillCorners(gtx, th.Theme, columnBounds, cut, 0, 0, cut, palette.WithAlpha(ink, min(max(open, 0), 1)))
	dividerX := columnBounds.Max.X
	paint.FillShape(gtx.Ops, ink, clip.Rect(image.Rect(dividerX, bounds.Min.Y+stroke, dividerX+stroke, bounds.Max.Y-stroke)).Op())
	kit.Stroke(gtx, th.Theme, bounds, cut, stroke, ink)
	return layout.Dimensions{Size: size}
}

// FillArt fills a tile size pixels across with ink in a cover's shape, by Art's rule: cut
// in Lithic, a rounded square in the round designs, never a pill. The sidebar's Liked
// songs tile stands among covers and wears their shape.
func FillArt(gtx layout.Context, th *Theme, size image.Point, shape Shape, ink color.NRGBA) {
	paint.FillShape(gtx.Ops, ink, clip.Outline{Path: artPath(gtx, th, size, shape)}.Op())
}
