package look

import (
	"image"
	"time"

	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
)

// Tip reveals delayed pointer detail through the window overlay host.
type Tip struct {
	tag         struct{}
	hovered     bool
	shown       bool
	pendingRect bool
	since       time.Time
	local       image.Point
	rect        image.Rectangle
}

// Layout draws w and manages its delayed tooltip.
func (tip *Tip) Layout(gtx layout.Context, theme *Theme, overlays *Overlays, text string, w layout.Widget) layout.Dimensions {
	if overlays != nil {
		overlays.touchTip(&tip.tag)
	}
	if tip.shown && overlays != nil && !overlays.desired(&tip.tag) {
		tip.reset()
	}

	sawPosition := false
	for {
		value, ok := gtx.Event(pointer.Filter{Target: &tip.tag, Kinds: pointer.Enter | pointer.Leave | pointer.Move})
		if !ok {
			break
		}
		pointerEvent := value.(pointer.Event)
		switch pointerEvent.Kind {
		case pointer.Enter:
			tip.hovered = true
			tip.since = gtx.Now
			tip.local = pointerEvent.Position.Round()
			tip.rect = image.Rectangle{}
			tip.pendingRect = true
			sawPosition = true
		case pointer.Move:
			if tip.hovered {
				tip.local = pointerEvent.Position.Round()
				sawPosition = true
			}
		case pointer.Leave:
			tip.dismiss(overlays)
		}
	}

	dimensions := w(gtx)
	semantic.DescriptionOp(text).Add(gtx.Ops)
	pass := pointer.PassOp{}.Push(gtx.Ops)
	area := clip.Rect(image.Rectangle{Max: dimensions.Size}).Push(gtx.Ops)
	event.Op(gtx.Ops, &tip.tag)
	area.Pop()
	pass.Pop()

	if tip.pendingRect && !sawPosition && overlays != nil {
		minimum := overlays.Pointer().Sub(tip.local)
		tip.rect = image.Rectangle{Min: minimum, Max: minimum.Add(dimensions.Size)}
		tip.pendingRect = false
	}
	if sawPosition && tip.pendingRect {
		gtx.Execute(op.InvalidateCmd{})
	}
	if tip.hovered && !tip.rect.Empty() && !sawPosition && overlays != nil && !overlays.Pointer().In(tip.rect) {
		tip.dismiss(overlays)
	}
	if overlays == nil || text == "" || !tip.hovered || tip.rect.Empty() {
		if text == "" {
			tip.dismiss(overlays)
		}
		return dimensions
	}
	ready := tip.since.Add(theme.Motion.TooltipDelay)
	if gtx.Now.Before(ready) {
		gtx.Execute(op.InvalidateCmd{At: ready})
		return dimensions
	}
	overlays.open(&tip.tag, tip.rect, Placement{Prefer: Below, Align: Start}, kit.Text(theme.Theme, theme.T.Caption, theme.R.Fg, text), true)
	tip.shown = true
	return dimensions
}

func (tip *Tip) dismiss(overlays *Overlays) {
	if overlays != nil {
		overlays.Close(&tip.tag)
	}
	tip.reset()
}

func (tip *Tip) reset() {
	tip.hovered = false
	tip.shown = false
	tip.pendingRect = false
	tip.since = time.Time{}
	tip.rect = image.Rectangle{}
}
