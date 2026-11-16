package sheet

import (
	"image"
	"image/color"
	"strings"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
)

// Colophon geometry, from the Hub's sheet (components/sheet/Colophon.qml,
// components/about/CardBarcode.qml), in dp.
const (
	colophonHeight   unit.Dp = 26
	colophonMottoGap unit.Dp = 16
	barHeight        unit.Dp = 9
	barUnit          unit.Dp = 1
	// the code's words stand 3 under the bars, in the smallest mono
	barWordsGap   unit.Dp = 3
	barWordsTall  unit.Dp = 14
	mottoSize             = 11
	mottoTracking         = 2
	codeTracking          = 1.6
)

// code39 holds each character's nine elements: b and w narrow bar and gap,
// B and W wide (three units), as the Hub's CardBarcode draws them.
var code39 = map[rune]string{
	'0': "bwbWBwBwb", '1': "BwbWbwbwB", '2': "bwBWbwbwB", '3': "BwBWbwbwb",
	'4': "bwbWBwbwB", '5': "BwbWBwbwb", '6': "bwBWBwbwb", '7': "bwbWbwBwB",
	'8': "BwbWbwBwb", '9': "bwBWbwBwb", 'A': "BwbwbWbwB", 'B': "bwBwbWbwB",
	'C': "BwBwbWbwb", 'D': "bwbwBWbwB", 'E': "BwbwBWbwb", 'F': "bwBwBWbwb",
	'G': "bwbwbWBwB", 'H': "BwbwbWBwb", 'I': "bwBwbWBwb", 'J': "bwbwBWBwb",
	'K': "BwbwbwbWB", 'L': "bwBwbwbWB", 'M': "BwBwbwbWb", 'N': "bwbwBwbWB",
	'O': "BwbwBwbWb", 'P': "bwBwBwbWb", 'Q': "bwbwbwBWB", 'R': "BwbwbwBWb",
	'S': "bwBwbwBWb", 'T': "bwbwBwBWb", 'U': "BWbwbwbwB", 'V': "bWBwbwbwB",
	'W': "BWBwbwbwb", 'X': "bWbwBwbwB", 'Y': "BWbwBwbwb", 'Z': "bWBwBwbwb",
	'-': "bWbwbwBwB", '.': "BWbwbwBwb", ' ': "bWBwbwBwb", '*': "bWbwBwBwb",
}

// Code39 returns the bars of text as Code 39 encodes it, framed by the start
// and stop character: each entry is a bar's start and width in units, and
// the code's whole width in units. Characters Code 39 has no bars for are
// left out, as the Hub's barcode leaves them.
func Code39(text string) (bars [][2]int, width int) {
	coded := "*" + strings.ToUpper(text) + "*"
	x := 0
	for _, character := range coded {
		elements, ok := code39[character]
		if !ok {
			continue
		}
		for _, element := range elements {
			wide := 1
			if element == 'B' || element == 'W' {
				wide = 3
			}
			if element == 'b' || element == 'B' {
				bars = append(bars, [2]int{x, wide})
			}
			x += wide
		}
		x++
	}
	return bars, max(0, x-1)
}

// Colophon is the sheet's foot, 26 tall, as the Hub sets it: at the right
// the page's code in Code 39 bars with the code's words under them, and,
// while glosses are on, Ryoku's motto 16 to its left, both in fg3. The bars
// are recorded once per code, scale and ink and replayed, so a colophon at
// rest allocates nothing and asks for no frame.
type Colophon struct {
	ops   op.Ops
	call  op.CallOp
	key   colophonKey
	upper string
	units int
	// mottoShown says the last Layout set the motto
	mottoShown bool
}

type colophonKey struct {
	code  string
	scale float32
	ink   color.NRGBA
}

// Layout draws the colophon across the width given for the page whose code
// is code (its place's ID, "library"; empty means "home").
func (c *Colophon) Layout(gtx layout.Context, th *look.Theme, code string) layout.Dimensions {
	if code == "" {
		code = Home.ID
	}
	size := gtx.Constraints.Constrain(image.Pt(gtx.Constraints.Max.X, gtx.Dp(colophonHeight)))
	ink := th.R.Fg3
	key := colophonKey{code: code, scale: gtx.Metric.PxPerDp, ink: ink}
	if key != c.key || c.call == (op.CallOp{}) {
		c.key = key
		c.upper = strings.ToUpper(code)
		c.record(gtx)
	}
	codeWidth := c.units * max(1, gtx.Dp(barUnit))
	codeHeight := gtx.Dp(barHeight) + gtx.Dp(barWordsTall)
	x := size.X - codeWidth
	y := (size.Y - codeHeight) / 2

	offset := op.Offset(image.Pt(x, y)).Push(gtx.Ops)
	c.call.Add(gtx.Ops)
	words := op.Offset(image.Pt(0, gtx.Dp(barHeight)+gtx.Dp(barWordsGap))).Push(gtx.Ops)
	wordsGtx := gtx
	wordsGtx.Constraints.Min = image.Point{}
	kit.LayoutSpacedCaps(wordsGtx, th.Theme, th.T.MonoSm, ink, codeTracking, c.upper)
	words.Pop()
	offset.Pop()

	c.mottoShown = th.Desktop.Gloss
	if th.Desktop.Gloss {
		style := th.T.Caption
		style.Size = mottoSize
		mottoGtx := gtx
		mottoGtx.Constraints.Min = image.Point{}
		record := op.Record(gtx.Ops)
		motto := kit.LayoutSpacedCaps(mottoGtx, th.Theme, style, ink, mottoTracking, Motto)
		call := record.Stop()
		at := image.Pt(x-gtx.Dp(colophonMottoGap)-motto.Size.X, (size.Y-motto.Size.Y)/2)
		place := op.Offset(at).Push(gtx.Ops)
		call.Add(gtx.Ops)
		place.Pop()
	}
	return layout.Dimensions{Size: size}
}

// record draws the bars into the colophon's own ops.
func (c *Colophon) record(gtx layout.Context) {
	c.ops.Reset()
	macro := op.Record(&c.ops)
	bars, units := Code39(c.key.code)
	c.units = units
	unitPx, tall := max(1, gtx.Dp(barUnit)), gtx.Dp(barHeight)
	var path clip.Path
	path.Begin(&c.ops)
	for _, bar := range bars {
		x0, x1 := bar[0]*unitPx, (bar[0]+bar[1])*unitPx
		path.MoveTo(layout.FPt(image.Pt(x0, 0)))
		path.LineTo(layout.FPt(image.Pt(x1, 0)))
		path.LineTo(layout.FPt(image.Pt(x1, tall)))
		path.LineTo(layout.FPt(image.Pt(x0, tall)))
		path.Close()
	}
	paint.FillShape(&c.ops, c.key.ink, clip.Outline{Path: path.End()}.Op())
	c.call = macro.Stop()
}
