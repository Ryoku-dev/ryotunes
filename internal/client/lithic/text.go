package lithic

import (
	"image"
	"image/color"

	"gioui.org/f32"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/widget"
	"golang.org/x/image/math/fixed"
)

// Label renders one Lithic text style with line limits and alignment.
type Label struct {
	Style    TextStyle
	Color    color.NRGBA
	MaxLines int
	Align    text.Alignment
}

// Layout shapes and paints a Label.
func (l Label) Layout(gtx layout.Context, th *Theme, s string) layout.Dimensions {
	truncator := ""
	if l.MaxLines > 0 {
		truncator = "…"
	}
	label := widget.Label{
		Alignment:       l.Align,
		MaxLines:        l.MaxLines,
		Truncator:       truncator,
		LineHeight:      l.Style.Line,
		LineHeightScale: 1,
	}
	return label.Layout(gtx, th.Shaper, l.Style.Face, l.Style.Size, s, textMaterial(gtx.Ops, disabledInk(gtx, l.Color)))
}

// Text returns a single-line Lithic label widget.
func Text(th *Theme, style TextStyle, c color.NRGBA, s string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return Label{Style: style, Color: c, MaxLines: 1}.Layout(gtx, th, s)
	}
}

func rawText(th *Theme, style TextStyle, c color.NRGBA, s string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		label := widget.Label{
			MaxLines:  1,
			Truncator: "…",
		}
		return label.Layout(gtx, th.Shaper, style.Face, style.Size, s, textMaterial(gtx.Ops, c))
	}
}

func fixedFloat(v fixed.Int26_6) float32 {
	return float32(v) / 64
}

func clampFraction(v float32) float32 {
	return min(max(v, 0), 1)
}

// OutlineText engraves text and fills it from the left by a fraction.
func OutlineText(gtx layout.Context, th *Theme, style TextStyle, s string, fill float32, solid, line color.NRGBA) layout.Dimensions {
	if s == "" {
		return layout.Dimensions{}
	}

	measure := op.Record(gtx.Ops)
	dims := widget.Label{MaxLines: 1, Truncator: "…", LineHeight: style.Line, LineHeightScale: 1}.Layout(
		gtx,
		th.Shaper,
		style.Face,
		style.Size,
		s,
		textMaterial(gtx.Ops, solid),
	)
	measure.Stop()

	th.Shaper.LayoutString(text.Parameters{
		Font:            style.Face,
		PxPerEm:         fixed.I(gtx.Sp(style.Size)),
		MaxLines:        1,
		Truncator:       "…",
		MaxWidth:        gtx.Constraints.Max.X,
		MinWidth:        gtx.Constraints.Min.X,
		Locale:          gtx.Locale,
		LineHeight:      fixed.I(gtx.Sp(style.Line)),
		LineHeightScale: 1,
	}, s)
	var storage [128]text.Glyph
	glyphs := storage[:0]
	for glyph, ok := th.Shaper.NextGlyph(); ok; glyph, ok = th.Shaper.NextGlyph() {
		glyphs = append(glyphs, glyph)
		if glyph.Flags&text.FlagLineBreak != 0 {
			break
		}
	}
	if len(glyphs) == 0 {
		return dims
	}

	semantic.LabelOp(s).Add(gtx.Ops)
	path := th.Shaper.Shape(glyphs)
	baseline := f32.Pt(fixedFloat(glyphs[0].X), float32(glyphs[0].Y))
	fillWidth := int(float32(dims.Size.X)*clampFraction(fill) + 0.5)
	if fillWidth > 0 && solid.A > 0 {
		fillClip := clip.Rect(image.Rect(0, 0, fillWidth, dims.Size.Y)).Push(gtx.Ops)
		move := op.Affine(f32.AffineId().Offset(baseline)).Push(gtx.Ops)
		outline := clip.Outline{Path: path}.Op().Push(gtx.Ops)
		paint.ColorOp{Color: disabledInk(gtx, solid)}.Add(gtx.Ops)
		paint.PaintOp{}.Add(gtx.Ops)
		outline.Pop()
		move.Pop()
		fillClip.Pop()
	}
	if line.A > 0 {
		move := op.Affine(f32.AffineId().Offset(baseline)).Push(gtx.Ops)
		paint.FillShape(gtx.Ops, disabledInk(gtx, line), clip.Stroke{
			Path:  path,
			Width: float32(max(1, gtx.Dp(1))),
		}.Op())
		move.Pop()
	}
	return dims
}
