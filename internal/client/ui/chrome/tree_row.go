package chrome

import (
	"image"
	"image/color"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/anim"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
	libtheme "github.com/ryoku-dev/ryoku/docs/lithic/library/go/theme"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
)

// kitStyle is a Lithic text style.
type kitStyle = libtheme.TextStyle

// The Hub's tree, in dp (components/nav/NavRow.qml, NavFolder.qml, NavOutline.qml,
// HubSidebar.qml): a numbered line 34 tall, a leaf 30 (a collection leaf keeps its 32 dp
// cover and stands 44), a step in of 22 a level, the guide 16 in from the rows' edge, the
// branch tick 8 long, the row's own cut 6, and the outline's rows 26 on a thread 36 in.
const (
	treeLine        unit.Dp = 34
	treeLeaf        unit.Dp = 30
	treeCoverLeaf   unit.Dp = 44
	treeRailTile    unit.Dp = 48
	treeIndent      unit.Dp = 22
	treeGuide       unit.Dp = 16
	treeBranch      unit.Dp = 8
	treeRowCut      unit.Dp = 6
	treeTail        unit.Dp = 10
	treeChevron     unit.Dp = 16
	treeCover       unit.Dp = 32
	treeOutlineRow  unit.Dp = 26
	treeThread      unit.Dp = 36
	treeOutlineText unit.Dp = 55
	treeNodeRest    unit.Dp = 5
	treeNodeHere    unit.Dp = 7
	// treeInset is the rows' distance from the sidebar's edges (HubSidebar body x 4)
	treeInset unit.Dp = 4
	// treeSlide is how far a leaf slides in from as its folder opens
	treeSlide unit.Dp = 8
)

// Tracking of the tree's spaced capitals and glosses, in sp (NavRow.qml:165,191).
const (
	trackTreeName  = 1.4
	trackTreeGloss = 1
	trackTreeHead  = 1.6
)

// treeGlossSize is a numbered line's gloss size (NavRow.qml:190).
const treeGlossSize unit.Sp = 12

// treeRow is one line's own motion: the Hub's hover and press wash with its lean, the
// ink turning to the selection's over th.Motion.Quick, and the folder chevron's turn.
type treeRow struct {
	click   kit.Click
	wash    look.RowWash
	tip     look.Tip
	chevron look.Chevron
	lit     anim.Tween
	litSet  bool
	// hover and press are the wash's amounts this frame, sel the selection's
	hover, press, sel float32
}

// update moves the row's wash and selection ink; call once a frame before drawing.
func (row *treeRow) update(gtx layout.Context, th *look.Theme, selected bool) {
	enabled := gtx.Source.Enabled()
	row.hover, row.press = row.wash.Update(gtx, th, enabled && row.click.Hovered(), enabled && row.click.Pressed())
	row.lit.Duration, row.lit.Ease = th.Motion.Quick, anim.EaseOutCubic
	target := float32(0)
	if selected {
		target = 1
	}
	if !row.litSet {
		row.litSet = true
		row.lit.Set(gtx, target)
	}
	row.lit.Set(gtx, target)
	row.sel = row.lit.Value(gtx)
}

// lean is the row's lean in pixels: a selected line stands still.
func (row *treeRow) lean(gtx layout.Context) int {
	if row.sel >= 1 {
		return 0
	}
	return int(float32(row.wash.Lean(gtx))*(1-row.sel) + .5)
}

// mixInk moves from rest toward lit by the hover and on toward the selection's ink.
func mixInk(rest, lit, chosen color.NRGBA, hover, sel float32) color.NRGBA {
	return palette.Mix(palette.Mix(rest, lit, hover), chosen, sel)
}

// recorded is a widget laid out ahead of placing it: its ops and size.
type recorded struct {
	call op.CallOp
	size image.Point
}

// record lays w out with no minimum and keeps its ops for placing later.
func record(gtx layout.Context, w layout.Widget) recorded {
	gtx.Constraints.Min = image.Point{}
	macro := op.Record(gtx.Ops)
	dimensions := w(gtx)
	return recorded{call: macro.Stop(), size: dimensions.Size}
}

// at places a recorded widget with its top left at x and its middle on centre.
func (r recorded) at(gtx layout.Context, x, centre int) {
	if r.size.X <= 0 {
		return
	}
	offset := op.Offset(image.Pt(x, centre-r.size.Y/2)).Push(gtx.Ops)
	r.call.Add(gtx.Ops)
	offset.Pop()
}

// spaced records a line of spaced capitals (also a gloss: kanji have no case).
func spaced(gtx layout.Context, th *look.Theme, style kitStyle, ink color.NRGBA, tracking float32, value string) recorded {
	if value == "" {
		return recorded{}
	}
	return record(gtx, func(gtx layout.Context) layout.Dimensions {
		return kit.LayoutSpacedCaps(gtx, th.Theme, style, ink, tracking, value)
	})
}

// line records one plain line, elided to width.
func line(gtx layout.Context, th *look.Theme, style kitStyle, ink color.NRGBA, value string, width int) recorded {
	if value == "" || width <= 0 {
		return recorded{}
	}
	return record(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Max.X = width
		return kit.Label{Style: style, Color: ink, MaxLines: 1}.Layout(gtx, th.Theme, value)
	})
}

// bold is style in the tree's strong weight: the Hub's DemiBold, which Lithic's faces
// carry as Bold.
func bold(style kitStyle) kitStyle {
	style.Face.Weight = font.Bold
	return style
}

// sized is style at size, its line kept to the size's own.
func sized(style kitStyle, size unit.Sp) kitStyle {
	style.Size = size
	if style.Line < size {
		style.Line = size
	}
	return style
}

// hline paints a 1 px rule from x0 to x1 on row y in ink.
func hline(gtx layout.Context, x0, x1, y int, ink color.NRGBA) {
	if x1 <= x0 {
		return
	}
	paint.FillShape(gtx.Ops, ink, clip.Rect(image.Rect(x0, y, x1, y+max(1, gtx.Dp(1)))).Op())
}

// vline paints a 1 px rule from y0 to y1 at x in ink.
func vline(gtx layout.Context, x, y0, y1 int, ink color.NRGBA) {
	if y1 <= y0 {
		return
	}
	paint.FillShape(gtx.Ops, ink, clip.Rect(image.Rect(x, y0, x+max(1, gtx.Dp(1)), y1)).Op())
}

// glyph draws icon size pixels square with its middle at (cx, cy), scaled by scale.
func glyph(gtx layout.Context, icon *widget.Icon, ink color.NRGBA, size, cx, cy int, angle, scale float32) {
	if icon == nil || size <= 0 {
		return
	}
	offset := op.Offset(image.Pt(cx-size/2, cy-size/2)).Push(gtx.Ops)
	look.DrawGlyph(gtx, icon, ink, size, angle, scale)
	offset.Pop()
}

// node paints the outline's round node of diameter d with its middle at (cx, cy): filled
// when fill is set, else a 1 px ring.
func node(gtx layout.Context, d float32, cx, cy int, ink color.NRGBA, fill bool, rest color.NRGBA) {
	if d <= 0 {
		return
	}
	half := d / 2
	box := image.Rect(int(float32(cx)-half+.5), int(float32(cy)-half+.5), int(float32(cx)+half+.5), int(float32(cy)+half+.5))
	paint.FillShape(gtx.Ops, ink, clip.Ellipse(box).Op(gtx.Ops))
	if fill {
		return
	}
	inner := box.Inset(max(1, gtx.Dp(1)))
	if inner.Empty() {
		return
	}
	paint.FillShape(gtx.Ops, rest, clip.Ellipse(inner).Op(gtx.Ops))
}

// lerp moves from a to b by t, rounded.
func lerp(a, b int, t float32) int {
	return a + int(float32(b-a)*min(max(t, 0), 1)+.5)
}
