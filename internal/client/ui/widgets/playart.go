package widgets

import (
	"image"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/anim"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

// playArtLinger keeps the plate up after a click, so a click without a hover
// (touch, a trackpad tap that lands as the pointer leaves) still answers.
const playArtLinger = 700 * time.Millisecond

// PlayArt is the loaded track's cover as its play and pause control. Hovering
// lays a plate with the action's glyph over the art; a click toggles playback
// and the glyph morphs to the next action.
type PlayArt struct {
	View  look.ArtView
	click kit.Click
	hover look.Hover
	morph look.Morph
	plate anim.Tween
	until time.Time
}

// Layout draws url crossfading over fade inside size with shape.
func (p *PlayArt) Layout(gtx layout.Context, e *env.Env, url string, size image.Point, shape look.Shape, fade time.Duration) layout.Dimensions {
	snapshot := e.Store.Snapshot()
	loaded := snapshot.Playback.Now != nil
	if !loaded {
		return p.View.Layout(gtx, e.Th, e.Art, url, size, shape, fade)
	}
	icon := look.Icon.Pause
	if snapshot.Playback.Paused {
		icon = look.Icon.Play
	}

	var hover float32
	dimensions := p.click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		dimensions, value := p.hover.Layout(gtx, e.Th, func(gtx layout.Context) layout.Dimensions {
			return p.View.Layout(gtx, e.Th, e.Art, url, size, shape, fade)
		})
		hover = value
		return dimensions
	})
	if p.click.Clicked(gtx) {
		transportCall(e, protocol.MethodTogglePause, protocol.Null{})
		p.until = gtx.Now.Add(playArtLinger)
	}

	target := float32(0)
	if gtx.Now.Before(p.until) {
		target = 1
		gtx.Execute(op.InvalidateCmd{At: p.until})
	}
	p.plate.Duration, p.plate.Ease = e.Th.Motion.HoverIn, anim.EaseChisel
	if target == 0 {
		p.plate.Duration = e.Th.Motion.HoverOut
	}
	p.plate.Set(gtx, target)
	alpha := max(hover, p.plate.Value(gtx))
	if alpha <= 0 {
		return dimensions
	}

	side := min(gtx.Dp(56), min(dimensions.Size.X, dimensions.Size.Y)*2/5)
	if side <= 0 {
		return dimensions
	}
	at := image.Pt((dimensions.Size.X-side)/2, (dimensions.Size.Y-side)/2)
	offset := op.Offset(at).Push(gtx.Ops)
	opacity := paint.PushOpacity(gtx.Ops, min(alpha, 1))
	kit.FillCut(gtx.Ops, image.Rectangle{Max: image.Pt(side, side)}, gtx.Dp(e.Th.Cut.Small), palette.WithAlpha(e.Th.R.Stone4, .9))
	glyph := side / 2
	inner := op.Offset(image.Pt((side-glyph)/2, (side-glyph)/2)).Push(gtx.Ops)
	p.morph.Layout(gtx, e.Th, icon, e.Th.R.Fg, unit.Dp(float32(glyph)/gtx.Metric.PxPerDp))
	inner.Pop()
	opacity.Pop()
	offset.Pop()
	return dimensions
}
