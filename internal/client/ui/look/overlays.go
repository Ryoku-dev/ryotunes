package look

import (
	"image"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/anim"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/tokens"
)

const overlayExtent = 1 << 20

// Side is the preferred vertical side of an overlay anchor.
type Side uint8

const (
	Below Side = iota
	Above
)

// Align selects which horizontal edges of an overlay and anchor coincide.
type Align uint8

const (
	Start Align = iota
	End
)

// Placement describes an overlay's anchor relationship.
type Placement struct {
	Prefer  Side
	Align   Align
	Gap     unit.Dp
	Width   unit.Dp
	Passive bool
	Return  any
}

type overlayEntry struct {
	tag       any
	anchor    image.Rectangle
	placement Placement
	content   layout.Widget
	rect      image.Rectangle
	desired   bool
	tip       bool
	tipFrame  uint64
	motion    anim.Tween
	motionSet bool
}

// Overlays places application popovers in window coordinates.
type Overlays struct {
	window, pointer, press  image.Point
	tracker, outside, focus struct{}
	marks                   map[any]image.Rectangle
	items                   []*overlayEntry
	tipFrame                uint64
}

// Window returns the most recently observed full-window size in pixels.
func (overlays *Overlays) Window() image.Point { return overlays.window }

// Pointer returns the last pointer position in window pixels.
func (overlays *Overlays) Pointer() image.Point { return overlays.pointer }

// Press returns the last press position in window pixels.
func (overlays *Overlays) Press() image.Point { return overlays.press }

// AnchorOf derives a clickable's window rectangle from its last local press.
func (overlays *Overlays) AnchorOf(clickable *widget.Clickable, size image.Point) image.Rectangle {
	history := clickable.History()
	if len(history) != 0 {
		local := history[len(history)-1].Position
		minimum := overlays.press.Sub(local)
		return image.Rectangle{Min: minimum, Max: minimum.Add(size)}
	}
	if rectangle, ok := overlays.Rect(clickable); ok {
		return rectangle
	}
	return image.Rectangle{}
}

// Mark records a tag's known window rectangle.
func (overlays *Overlays) Mark(tag any, rectangle image.Rectangle) {
	if overlays.marks == nil {
		overlays.marks = make(map[any]image.Rectangle)
	}
	overlays.marks[tag] = rectangle
}

// Rect returns a previously marked window rectangle.
func (overlays *Overlays) Rect(tag any) (image.Rectangle, bool) {
	if overlays.marks == nil {
		return image.Rectangle{}, false
	}
	rectangle, ok := overlays.marks[tag]
	return rectangle, ok
}

// Open shows or updates an anchored overlay.
func (overlays *Overlays) Open(tag any, anchor image.Rectangle, placement Placement, content layout.Widget) {
	overlays.open(tag, anchor, placement, content, false)
}

func (overlays *Overlays) open(tag any, anchor image.Rectangle, placement Placement, content layout.Widget, tip bool) {
	if placement.Gap == 0 {
		placement.Gap = tokens.SizePopoverAnchorGap
	}
	if tip {
		for _, entry := range overlays.items {
			if entry.tip && entry.tag != tag {
				entry.desired = false
			}
		}
	}
	for index, entry := range overlays.items {
		if entry.tag != tag {
			continue
		}
		entry.anchor, entry.placement, entry.content = anchor, placement, content
		entry.desired, entry.tip = true, tip
		if tip {
			entry.tipFrame = overlays.tipFrame
		}
		if index != len(overlays.items)-1 {
			copy(overlays.items[index:], overlays.items[index+1:])
			overlays.items[len(overlays.items)-1] = entry
		}
		return
	}
	overlays.items = append(overlays.items, &overlayEntry{
		tag: tag, anchor: anchor, placement: placement, content: content,
		desired: true, tip: tip, tipFrame: overlays.tipFrame,
	})
}

func (overlays *Overlays) touchTip(tag any) {
	if entry := overlays.entry(tag); entry != nil && entry.tip {
		entry.tipFrame = overlays.tipFrame
	}
}

// OpenAt opens a command overlay at a window-coordinate press point.
func (overlays *Overlays) OpenAt(tag any, at image.Point, content layout.Widget) {
	overlays.Open(tag, image.Rectangle{Min: at, Max: at}, Placement{Prefer: Below, Align: Start}, content)
}

// Close begins an overlay's exit transition.
func (overlays *Overlays) Close(tag any) {
	for _, entry := range overlays.items {
		if entry.tag == tag {
			entry.desired = false
		}
	}
}

// IsOpen reports whether an overlay is visible or leaving.
func (overlays *Overlays) IsOpen(tag any) bool {
	for _, entry := range overlays.items {
		if entry.tag == tag {
			return true
		}
	}
	return false
}

// Top returns the uppermost overlay that owns dismissal and keyboard focus.
func (overlays *Overlays) Top() (tag any, ok bool) {
	for index := len(overlays.items) - 1; index >= 0; index-- {
		entry := overlays.items[index]
		if !entry.tip && !entry.placement.Passive {
			return entry.tag, true
		}
	}
	return nil, false
}

// Track registers the pass-through full-window pointer observer.
func (overlays *Overlays) Track(gtx layout.Context) {
	for {
		value, ok := gtx.Event(pointer.Filter{Target: &overlays.tracker, Kinds: pointer.Press | pointer.Move | pointer.Drag})
		if !ok {
			break
		}
		pointerEvent := value.(pointer.Event)
		position := image.Pt(int(pointerEvent.Position.X), int(pointerEvent.Position.Y))
		overlays.pointer = position
		if pointerEvent.Kind == pointer.Press {
			overlays.press = position
		}
	}
	pass := pointer.PassOp{}.Push(gtx.Ops)
	area := clip.Rect(image.Rectangle{Max: overlays.window}).Push(gtx.Ops)
	event.Op(gtx.Ops, &overlays.tracker)
	area.Pop()
	pass.Pop()
}

// Layout draws all overlays after the full frame has been laid out.
func (overlays *Overlays) Layout(gtx layout.Context, theme *Theme) {
	overlays.window = gtx.Constraints.Max
	for _, entry := range overlays.items {
		if entry.tip && entry.desired && entry.tipFrame != overlays.tipFrame {
			entry.desired = false
		}
	}

	if tag, ok := overlays.Top(); ok {
		entry := overlays.entry(tag)
		for {
			value, found := gtx.Event(pointer.Filter{Target: &overlays.outside, Kinds: pointer.Press})
			if !found {
				break
			}
			press := value.(pointer.Event)
			if entry == nil || !press.Position.Round().In(entry.rect) {
				overlays.Close(tag)
				gtx.Execute(op.InvalidateCmd{})
			}
		}
		for {
			value, found := gtx.Event(key.Filter{Focus: &overlays.focus, Name: key.NameEscape})
			if !found {
				break
			}
			if value.(key.Event).State == key.Press {
				overlays.Close(tag)
				gtx.Execute(op.InvalidateCmd{})
			}
		}
	}
	overlays.Track(gtx)

	top, hasTop := overlays.Top()
	if hasTop {
		event.Op(gtx.Ops, &overlays.focus)
		if !gtx.Focused(&overlays.focus) {
			gtx.Execute(key.FocusCmd{Tag: &overlays.focus})
		}
	}

	kept := overlays.items[:0]
	for _, entry := range overlays.items {
		entry.motion.Duration = theme.Motion.MenuIn
		if entry.tip && entry.desired {
			entry.motion.Duration = theme.Motion.TooltipIn
		}
		target := float32(1)
		if !entry.desired {
			entry.motion.Duration = theme.Motion.MenuOut
			target = 0
		}
		if !entry.motionSet {
			entry.motion.Ease = anim.EaseChisel
			entry.motion.Set(gtx, 0)
			entry.motionSet = true
		}
		entry.motion.Set(gtx, target)
		progress := entry.motion.Value(gtx)
		if !entry.desired && progress <= 0 {
			if entry.tag == top {
				if target := entry.focusReturn(); target != nil {
					gtx.Execute(key.FocusCmd{Tag: target})
				}
			}
			continue
		}
		kept = append(kept, entry)
		layer := op.Record(gtx.Ops)
		if hasTop && entry.tag == top {
			area := clip.Rect(image.Rectangle{Max: overlays.window}).Push(gtx.Ops)
			event.Op(gtx.Ops, &overlays.outside)
			area.Pop()
		}
		overlays.layoutEntry(gtx, theme, entry, progress)
		op.Defer(gtx.Ops, layer.Stop())
	}
	overlays.items = kept
	overlays.tipFrame++
}

func (entry *overlayEntry) focusReturn() any {
	if entry.tip || entry.placement.Passive {
		return nil
	}
	if entry.placement.Return != nil {
		return entry.placement.Return
	}
	return entry.tag
}

func (overlays *Overlays) entry(tag any) *overlayEntry {
	for _, entry := range overlays.items {
		if entry.tag == tag {
			return entry
		}
	}
	return nil
}

func (overlays *Overlays) desired(tag any) bool {
	entry := overlays.entry(tag)
	return entry != nil && entry.desired
}

func (overlays *Overlays) layoutEntry(gtx layout.Context, theme *Theme, entry *overlayEntry, progress float32) {
	measure := gtx
	measure.Constraints.Min = image.Point{}
	measure.Constraints.Max = image.Pt(max(0, overlays.window.X-32), max(0, overlays.window.Y-32))
	inset := layout.Inset{
		Top: unit.Dp(tokens.SizeMenuPaddingVertical), Right: unit.Dp(tokens.SizeMenuInset),
		Bottom: unit.Dp(tokens.SizeMenuPaddingVertical), Left: unit.Dp(tokens.SizeMenuInset),
	}
	if entry.tip {
		measure.Constraints.Max.X = min(measure.Constraints.Max.X, gtx.Dp(tokens.SizeTooltipWidthMax))
		inset = layout.Inset{
			Top: unit.Dp(tokens.SizeTooltipPaddingVertical), Right: unit.Dp(tokens.SizeTooltipPaddingHorizontal),
			Bottom: unit.Dp(tokens.SizeTooltipPaddingVertical), Left: unit.Dp(tokens.SizeTooltipPaddingHorizontal),
		}
	}
	if entry.placement.Width > 0 {
		measure.Constraints.Min.X = min(measure.Constraints.Max.X, gtx.Dp(entry.placement.Width))
	}
	record := op.Record(gtx.Ops)
	dimensions := inset.Layout(measure, entry.content)
	call := record.Stop()
	position, actual := placeOverlay(overlays.window, entry.anchor, dimensions.Size, gtx.Dp(entry.placement.Gap), 8, entry.placement.Prefer, entry.placement.Align)
	travel := float32(gtx.Dp(tokens.SizePopoverEnterTravel))
	if entry.tip {
		travel = float32(gtx.Dp(tokens.SizeTooltipTravel))
	} else if !entry.desired {
		travel = float32(gtx.Dp(tokens.SizePopoverExitTravel))
	}
	direction := float32(-1)
	if actual == Above {
		direction = 1
	}
	position.Y += int(direction * travel * (1 - progress))

	offset := op.Offset(position).Push(gtx.Ops)
	opacity := paint.PushOpacity(gtx.Ops, progress)
	bounds := image.Rectangle{Max: dimensions.Size}
	entry.rect = image.Rectangle{Min: position, Max: position.Add(dimensions.Size)}
	cut := theme.Cut.Menu
	if entry.tip {
		cut = theme.Cut.Tooltip
	}
	kit.FillCut(gtx.Ops, bounds, gtx.Dp(cut), theme.R.Stone4)
	kit.StrokeCut(gtx.Ops, bounds, gtx.Dp(cut), max(1, gtx.Dp(tokens.SizeHairline)), theme.R.Vein)
	call.Add(gtx.Ops)
	opacity.Pop()
	offset.Pop()
}

func placeOverlay(window image.Point, anchor image.Rectangle, content image.Point, gap, margin int, prefer Side, align Align) (image.Point, Side) {
	x := anchor.Min.X
	if align == End {
		x = anchor.Max.X - content.X
	}
	maximumX := max(margin, window.X-margin-content.X)
	x = min(max(x, margin), maximumX)
	below := anchor.Max.Y + gap
	above := anchor.Min.Y - gap - content.Y
	fitsBelow := below+content.Y <= window.Y-margin
	fitsAbove := above >= margin
	actual := prefer
	if prefer == Below && !fitsBelow && fitsAbove {
		actual = Above
	} else if prefer == Above && !fitsAbove && fitsBelow {
		actual = Below
	}
	y := below
	if actual == Above {
		y = above
	}
	maximumY := max(margin, window.Y-margin-content.Y)
	y = min(max(y, margin), maximumY)
	return image.Pt(x, y), actual
}
