package lithic

import (
	"image"
	"sync"
	"time"

	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
)

type tooltipState struct {
	tag     struct{}
	hovered bool
	since   time.Time
	shown   bool
	enter   Tween
}

type tooltipKey struct {
	ops *op.Ops
	tip string
}

var tooltipStates = struct {
	sync.Mutex
	m map[tooltipKey]*tooltipState
}{m: make(map[tooltipKey]*tooltipState)}

// Tooltip reveals a deferred Lithic label after the configured hover delay.
func Tooltip(gtx layout.Context, th *Theme, tip string, w layout.Widget) layout.Dimensions {
	key := tooltipKey{ops: gtx.Ops, tip: tip}
	tooltipStates.Lock()
	state := tooltipStates.m[key]
	if state == nil {
		state = &tooltipState{}
		tooltipStates.m[key] = state
	}
	tooltipStates.Unlock()

	for {
		e, ok := gtx.Event(pointer.Filter{Target: &state.tag, Kinds: pointer.Enter | pointer.Leave})
		if !ok {
			break
		}
		if e, ok := e.(pointer.Event); ok {
			switch e.Kind {
			case pointer.Enter:
				state.hovered = true
				state.since = gtx.Now
				state.shown = false
				gtx.Execute(op.InvalidateCmd{At: gtx.Now.Add(th.Motion.TooltipDelay)})
			case pointer.Leave:
				state.hovered = false
				state.shown = false
			}
		}
	}

	dims := w(gtx)
	pass := pointer.PassOp{}.Push(gtx.Ops)
	area := clip.Rect{Max: dims.Size}.Push(gtx.Ops)
	event.Op(gtx.Ops, &state.tag)
	area.Pop()
	pass.Pop()
	if tip == "" || !state.hovered || gtx.Now.Sub(state.since) < th.Motion.TooltipDelay {
		return dims
	}

	if !state.shown {
		state.enter = Tween{Duration: th.Motion.TooltipIn, Ease: EaseChisel}
		state.enter.Set(gtx, 0)
		state.enter.Set(gtx, 1)
		state.shown = true
	}
	progress := state.enter.Value(gtx)

	tipCtx := gtx
	tipCtx.Constraints.Min = image.Point{}
	tipCtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(320))
	tipRecord := op.Record(gtx.Ops)
	tipDims := layout.Inset{Top: 6, Bottom: 6, Left: 8, Right: 8}.Layout(tipCtx, Text(th, th.T.Caption, th.R.Fg, tip))
	tipCall := tipRecord.Stop()

	viewport := image.Rect(0, -gtx.Constraints.Max.Y, gtx.Constraints.Max.X, gtx.Constraints.Max.Y)
	anchor := image.Rectangle{Max: dims.Size}
	position := placePopover(viewport, anchor, tipDims.Size, gtx.Dp(8))
	position.Y += int(float32(gtx.Dp(8)) * (1 - progress))

	overlay := op.Record(gtx.Ops)
	opacity := paint.PushOpacity(gtx.Ops, progress)
	offset := op.Offset(position).Push(gtx.Ops)
	r := image.Rectangle{Max: tipDims.Size}
	cut := gtx.Dp(4)
	FillCut(gtx.Ops, r, cut, th.R.Stone4)
	StrokeCut(gtx.Ops, r, cut, max(1, gtx.Dp(1)), th.R.Vein)
	tipCall.Add(gtx.Ops)
	offset.Pop()
	opacity.Pop()
	deferOverlay(gtx, overlay.Stop())
	return dims
}
