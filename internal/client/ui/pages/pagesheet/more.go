package pagesheet

import (
	"image"
	"time"

	"gioui.org/layout"
	"gioui.org/op"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
)

// moreDelay is how long work runs before the page says so (docs/hub.md "work pattern").
const moreDelay = time.Second

// More is the line under a list that is fetching its next page, in the Hub's work
// pattern: for the first second nothing shows (the line's room is kept, so nothing moves
// when rows arrive), then the three working flakes and a word. It asks for one frame at
// the second and then only the flakes' own.
type More struct {
	since  time.Time
	active bool
}

// Layout draws the line; active says the next page is on its way.
func (m *More) Layout(gtx layout.Context, th *look.Theme, active bool, word string) layout.Dimensions {
	height := gtx.Sp(th.T.Body.Line)
	size := image.Pt(gtx.Constraints.Max.X, height)
	if !active {
		m.active = false
		return layout.Dimensions{Size: size}
	}
	if !m.active {
		m.active, m.since = true, gtx.Now
	}
	if wait := m.since.Add(moreDelay); gtx.Now.Before(wait) {
		gtx.Execute(op.InvalidateCmd{At: wait})
		return layout.Dimensions{Size: size}
	}
	gtx.Constraints.Min = image.Point{}
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return kit.Working{Active: true}.Layout(gtx, th.Theme)
		}),
		layout.Rigid(layout.Spacer{Width: th.Sp.S2}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return kit.Label{Style: th.T.Body, Color: th.R.Fg2, MaxLines: 1}.Layout(gtx, th.Theme, word)
		}),
	)
}
