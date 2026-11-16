package sheet

import (
	"image"
	"image/color"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	libtheme "github.com/ryoku-dev/ryoku/docs/lithic/library/go/theme"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
)

// Tategaki geometry (components/sheet/Tategaki.qml, PageHeader.qml), in dp.
const (
	// TategakiRoom is the room a page head keeps at its right for the
	// tategaki while glosses are on.
	TategakiRoom      unit.Dp = 36
	tategakiWidth     unit.Dp = 22
	tategakiGap       unit.Dp = 4
	tategakiRule      unit.Dp = 18
	tategakiDot       unit.Dp = 3
	tategakiKanji             = 20
	tategakiKanjiLine         = 22
	tategakiCategory          = 11
)

// Tategaki sets a page's gloss vertically, top to bottom, as a Japanese
// poster sets its title beside a plate: an 18 dp rule in the accent (sel),
// the page's kanji large (20/22, fg2), a 3 dp dot, then its category's kanji
// small (11, fg3), 4 apart in a column 22 wide. A page head shows it only
// while glosses are on, in TategakiRoom at its right. The kanji are split
// once per gloss, so a tategaki at rest allocates nothing.
type Tategaki struct {
	gloss, category string
	page, parent    []string
}

// Layout draws the column for gloss, the page's kanji, and category, its
// category's (left out when it repeats the page's). It draws nothing while
// glosses are off.
func (t *Tategaki) Layout(gtx layout.Context, th *look.Theme, gloss, category string) layout.Dimensions {
	if !th.Desktop.Gloss || gloss == "" {
		return layout.Dimensions{}
	}
	if gloss != t.gloss || category != t.category {
		t.gloss, t.category = gloss, category
		t.page = splitRunes(gloss)
		t.parent = nil
		if category != gloss {
			t.parent = splitRunes(category)
		}
	}
	width, gap := gtx.Dp(tategakiWidth), gtx.Dp(tategakiGap)
	y := 0
	rule := gtx.Dp(tategakiRule)
	hair := max(1, gtx.Dp(1))
	paint.FillShape(gtx.Ops, th.R.Sel, clip.Rect(image.Rect((width-hair)/2, 0, (width-hair)/2+hair, rule)).Op())
	y += rule + gap
	kanji := th.T.Title
	kanji.Size, kanji.Line = tategakiKanji, tategakiKanjiLine
	y = t.column(gtx, th, t.page, kanji, th.R.Fg2, width, y, gap)
	if len(t.parent) > 0 {
		dot := gtx.Dp(tategakiDot)
		paint.FillShape(gtx.Ops, th.R.Fg3, clip.Rect(image.Rect((width-dot)/2, y, (width-dot)/2+dot, y+dot)).Op())
		y += dot + gap
		small := th.T.Caption
		small.Size = tategakiCategory
		y = t.column(gtx, th, t.parent, small, th.R.Fg3, width, y, gap)
	}
	return layout.Dimensions{Size: image.Pt(width, max(0, y-gap))}
}

// column stacks one character a line, centred in width, from y; it returns
// the y after the last line and its gap.
func (t *Tategaki) column(gtx layout.Context, th *look.Theme, characters []string, style libtheme.TextStyle, ink color.NRGBA, width, y, gap int) int {
	label := kit.Label{Style: style, Color: ink, MaxLines: 1, Align: text.Middle}
	for _, character := range characters {
		child := gtx
		child.Constraints = layout.Constraints{Min: image.Pt(width, 0), Max: image.Pt(width, gtx.Constraints.Max.Y)}
		offset := op.Offset(image.Pt(0, y)).Push(gtx.Ops)
		dimensions := label.Layout(child, th.Theme, character)
		offset.Pop()
		y += dimensions.Size.Y + gap
	}
	return y
}

// splitRunes is s one character a string.
func splitRunes(s string) []string {
	out := make([]string, 0, len(s)/3+1)
	for _, character := range s {
		out = append(out, string(character))
	}
	return out
}
