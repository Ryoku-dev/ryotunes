package lithic

import (
	"image"
	"time"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
)

// Popover shows content anchored under (or above) a control. A press
// anywhere outside the content closes it and goes no further, so closing a
// menu never also presses whatever was under the pointer.
type Popover struct {
	Open        bool
	contentRect image.Rectangle
	focusTag    int
	outsideTag  int
	blockerTag  int
	wasOpen     bool
	visible     bool
	enter       Tween
}

// popoverGap is the space between a popover and its anchor, in dp.
const popoverGap = 8

// popoverEnter is how long a popover takes to arrive: the theme's hover
// time, so a menu answers as quickly as the control that opened it.
const popoverEnter = 160 * time.Millisecond

const popoverExit = 100 * time.Millisecond

// farAway bounds the outside-press catcher. The catcher has an area of its
// own so press positions arrive in this popover's coordinates wherever the
// caller drew it; borrowing the caller's area put them in another space and
// every press inside a menu counted as outside.
const farAway = 1 << 20

var activePopover *Popover

func (p *Popover) close() {
	p.Open = false
	if activePopover == p {
		activePopover = nil
	}
}

const overlayHostValue = "ryotunes.lithic.overlay-host"

func overlayContentContext(gtx layout.Context) layout.Context {
	values := make(map[string]any, len(gtx.Values)+1)
	for key, value := range gtx.Values {
		values[key] = value
	}
	values[overlayHostValue] = true
	gtx.Values = values
	return gtx
}

func deferOverlay(gtx layout.Context, call op.CallOp) {
	if hosted, _ := gtx.Values[overlayHostValue].(bool); hosted {
		call.Add(gtx.Ops)
		return
	}
	op.Defer(gtx.Ops, call)
}

func placePopover(viewport, anchor image.Rectangle, content image.Point, gap int) image.Point {
	maxX := viewport.Max.X - content.X
	maxY := viewport.Max.Y - content.Y
	x := clampCoordinate(anchor.Min.X, viewport.Min.X, maxX)

	below := anchor.Max.Y + gap
	above := anchor.Min.Y - gap - content.Y
	y := below
	switch {
	case below+content.Y <= viewport.Max.Y:
	case above >= viewport.Min.Y:
		y = above
	default:
		y = clampCoordinate(below, viewport.Min.Y, maxY)
	}
	return image.Pt(x, clampCoordinate(y, viewport.Min.Y, maxY))
}

func clampCoordinate(v, lo, hi int) int {
	if hi < lo {
		return lo
	}
	return max(lo, min(v, hi))
}

// Layout renders an anchor and defers its flipped, clamped overlay content.
func (p *Popover) Layout(gtx layout.Context, anchor, content layout.Widget) layout.Dimensions {
	anchorDims := anchor(gtx)
	opening := p.Open && (!p.wasOpen || !p.visible)
	closing := !p.Open && p.wasOpen

	if p.Open && activePopover != nil && activePopover != p {
		activePopover.Open = false
	}
	if p.Open {
		activePopover = p
	}
	if opening {
		p.visible = true
		p.enter = Tween{Duration: popoverEnter, Ease: EaseChisel}
		p.enter.Set(gtx, 0)
		p.enter.Set(gtx, 1)
	}
	if closing {
		p.enter.Ease = EaseChisel
		p.enter.Set(gtx, 0)
		p.enter.Duration = popoverExit
		if activePopover == p {
			activePopover = nil
		}
	}
	p.wasOpen = p.Open
	if !p.visible {
		p.contentRect = image.Rectangle{}
		return anchorDims
	}

	if p.Open {
		event.Op(gtx.Ops, &p.focusTag)
		if opening || !gtx.Focused(&p.focusTag) {
			gtx.Execute(key.FocusCmd{Tag: &p.focusTag})
		}

		for {
			e, ok := gtx.Event(pointer.Filter{Target: &p.outsideTag, Kinds: pointer.Press})
			if !ok {
				break
			}
			press, ok := e.(pointer.Event)
			if ok && !press.Position.Round().In(p.contentRect) {
				p.close()
				p.wasOpen = false
				p.enter.Ease = EaseChisel
				p.enter.Set(gtx, 0)
				p.enter.Duration = popoverExit
			}
		}
		for {
			event, ok := gtx.Event(key.Filter{Focus: &p.focusTag, Name: key.NameEscape})
			if !ok {
				break
			}
			if event, ok := event.(key.Event); ok && event.State == key.Press {
				p.close()
				p.wasOpen = false
				p.enter.Ease = EaseChisel
				p.enter.Set(gtx, 0)
				p.enter.Duration = popoverExit
			}
		}
	}

	if !p.Open {
		for {
			_, ok := gtx.Event(pointer.Filter{Target: &p.blockerTag, Kinds: pointer.Press | pointer.Release})
			if !ok {
				break
			}
		}
	}

	progress := p.enter.Value(gtx)
	if !p.Open && progress <= 0 {
		p.visible = false
		p.contentRect = image.Rectangle{}
		return anchorDims
	}

	window := gtx.Constraints.Max
	contentCtx := overlayContentContext(gtx)
	if !p.Open {
		contentCtx = contentCtx.Disabled()
	}
	contentCtx.Constraints.Min = image.Point{}
	contentCtx.Constraints.Max.X = max(0, window.X)
	contentCtx.Constraints.Max.Y = max(0, window.Y)
	contentRecord := op.Record(gtx.Ops)
	contentDims := content(contentCtx)
	contentCall := contentRecord.Stop()

	viewport := image.Rect(0, -window.Y, window.X, window.Y)
	anchorRect := image.Rectangle{Max: anchorDims.Size}
	position := placePopover(viewport, anchorRect, contentDims.Size, gtx.Dp(popoverGap))
	p.contentRect = image.Rectangle{Min: position, Max: position.Add(contentDims.Size)}

	distance := gtx.Dp(6)
	if !p.Open {
		distance = gtx.Dp(4)
	}
	slide := int(float32(distance) * (1 - progress))
	if position.Y >= anchorRect.Max.Y {
		slide = -slide
	}

	overlay := op.Record(gtx.Ops)
	if p.Open {
		catcher := clip.Rect(image.Rect(-farAway, -farAway, farAway, farAway)).Push(gtx.Ops)
		event.Op(gtx.Ops, &p.outsideTag)
		catcher.Pop()
	}
	opacity := paint.PushOpacity(gtx.Ops, progress)
	offset := op.Offset(position.Add(image.Pt(0, slide))).Push(gtx.Ops)
	contentCall.Add(gtx.Ops)
	if !p.Open {
		blocker := clip.Rect(image.Rect(-farAway, -farAway, farAway, farAway)).Push(gtx.Ops)
		event.Op(gtx.Ops, &p.blockerTag)
		blocker.Pop()
	}
	offset.Pop()
	opacity.Pop()
	op.Defer(gtx.Ops, overlay.Stop())
	return anchorDims
}
