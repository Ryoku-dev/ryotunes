package look

import (
	"image"
	"image/color"
	"math"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/anim"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	libtheme "github.com/ryoku-dev/ryoku/docs/lithic/library/go/theme"
)

// CountUp runs a reading up from nothing once, as the Hub's Home counts its
// readings (Home.qml:220-245): from 0 to 1 over 640 ms (th.Motion.CountUp) on
// the chisel curve, the first time its value is ready, and never again, so a
// refresh moves nothing. Under reduced motion it stands at 1.
type CountUp struct {
	tween    anim.Tween
	started  bool
	fraction float32
}

// Fraction returns how far the count stands, 0..1; ready says the value to
// count to has arrived.
func (c *CountUp) Fraction(gtx layout.Context, th *Theme, ready bool) float32 {
	if !c.started {
		if !ready {
			c.fraction = 0
			return 0
		}
		c.started = true
		c.tween = anim.Tween{Duration: th.Motion.CountUp, Ease: anim.EaseChisel}
		c.tween.Set(gtx, 0)
		c.tween.Set(gtx, 1)
	}
	c.fraction = c.tween.Value(gtx)
	return c.fraction
}

// Of is n at the count's last Fraction, rounded, as the Hub shows a counting
// reading.
func (c *CountUp) Of(n float64) float64 {
	return math.Round(n * float64(c.fraction))
}

// CountRoll shows a count that rolls when it changes: the new figure comes up
// from half a line below while the old one leaves half a line above, fading
// across, over 240 ms on OutCubic (th.Motion.Move), as the Hub's folio rolls
// its number. The first value stands; at rest it draws one label and asks for
// no frame; under reduced motion the figure swaps at once.
type CountRoll struct {
	shown, leaving string
	roll           anim.Tween
	ready          bool
}

// Layout draws value in style and ink.
func (c *CountRoll) Layout(gtx layout.Context, th *Theme, style libtheme.TextStyle, ink color.NRGBA, value string) layout.Dimensions {
	if !c.ready {
		c.ready, c.shown = true, value
	} else if value != c.shown {
		c.leaving, c.shown = c.shown, value
		c.roll = anim.Tween{Duration: th.Motion.Move, Ease: anim.EaseOutCubic}
		c.roll.Set(gtx, 0)
		c.roll.Set(gtx, 1)
	}
	label := kit.Label{Style: style, Color: ink, MaxLines: 1}
	progress := float32(1)
	if c.leaving != "" {
		progress = c.roll.Value(gtx)
	}
	if progress >= 1 {
		c.leaving = ""
		return label.Layout(gtx, th.Theme, c.shown)
	}
	gtx.Constraints.Min = image.Point{}
	leaving := op.Record(gtx.Ops)
	old := label.Layout(gtx, th.Theme, c.leaving)
	leavingCall := leaving.Stop()
	coming := op.Record(gtx.Ops)
	current := label.Layout(gtx, th.Theme, c.shown)
	comingCall := coming.Stop()
	size := image.Pt(max(old.Size.X, current.Size.X), max(old.Size.Y, current.Size.Y))
	travel := float32(size.Y) / 2
	area := clip.Rect(image.Rectangle{Max: size}).Push(gtx.Ops)
	addFadedOffset(gtx.Ops, leavingCall, 1-progress, image.Pt(size.X-old.Size.X, -round32(travel*progress)))
	addFadedOffset(gtx.Ops, comingCall, progress, image.Pt(size.X-current.Size.X, round32(travel*(1-progress))))
	area.Pop()
	return layout.Dimensions{Size: size, Baseline: current.Baseline + size.Y - current.Size.Y}
}

// Rolling reports whether the count is moving.
func (c *CountRoll) Rolling() bool { return c.leaving != "" }
