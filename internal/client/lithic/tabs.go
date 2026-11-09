package lithic

import (
	"image"

	"gioui.org/f32"
	"gioui.org/io/key"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
)

// Tabs owns keyboard selection and the moving blade.
type Tabs struct {
	tabs         []Click
	hover, focus []Tween
	widths       []int
	x, width     Tween
	changed      int
	pending      bool
}

func (t *Tabs) ensure(n int) {
	for len(t.tabs) < n {
		t.tabs = append(t.tabs, Click{})
		t.hover = append(t.hover, Tween{})
		t.focus = append(t.focus, Tween{})
	}
	t.tabs = t.tabs[:n]
	t.hover = t.hover[:n]
	t.focus = t.focus[:n]
}

func (t *Tabs) poll(gtx layout.Context) {
	for i := range t.tabs {
		if t.tabs[i].Clicked(gtx) {
			t.changed, t.pending = i, true
		}
		for {
			e, ok := gtx.Event(
				key.Filter{Focus: &t.tabs[i].Clickable, Name: key.NameLeftArrow},
				key.Filter{Focus: &t.tabs[i].Clickable, Name: key.NameRightArrow},
			)
			if !ok {
				break
			}
			ke, ok := e.(key.Event)
			if !ok || ke.State != key.Press || len(t.tabs) == 0 {
				continue
			}
			next := i
			if ke.Name == key.NameLeftArrow {
				next = max(0, i-1)
			} else {
				next = min(len(t.tabs)-1, i+1)
			}
			if next != i {
				t.changed, t.pending = next, true
				gtx.Execute(key.FocusCmd{Tag: &t.tabs[next].Clickable})
				gtx.Execute(op.InvalidateCmd{})
			}
		}
	}
}

// Changed reports and clears a selected tab index.
func (t *Tabs) Changed(gtx layout.Context) (int, bool) {
	t.poll(gtx)
	if !t.pending {
		return 0, false
	}
	changed := t.changed
	t.pending = false
	return changed, true
}

func tabBladePoints(r image.Rectangle) [4]f32.Point {
	cut := min(max(0, r.Dy()), max(0, r.Dx()/2))
	return [4]f32.Point{
		f32.Pt(float32(r.Min.X+cut), float32(r.Min.Y)),
		f32.Pt(float32(r.Max.X), float32(r.Min.Y)),
		f32.Pt(float32(r.Max.X-cut), float32(r.Max.Y)),
		f32.Pt(float32(r.Min.X), float32(r.Max.Y)),
	}
}

// Layout renders the tab strip, baseline, and moving blade.
func (t *Tabs) Layout(gtx layout.Context, th *Theme, labels []string, selected int) layout.Dimensions {
	t.ensure(len(labels))
	t.poll(gtx)
	if len(labels) == 0 {
		return layout.Dimensions{}
	}
	selected = min(max(selected, 0), len(labels)-1)
	h := gtx.Dp(44)
	if cap(t.widths) < len(labels) {
		t.widths = make([]int, len(labels))
	} else {
		t.widths = t.widths[:len(labels)]
	}
	total := 0
	for i, label := range labels {
		measureGTX := gtx
		measureGTX.Constraints.Min = image.Point{}
		m := op.Record(gtx.Ops)
		dims := Label{Style: th.T.Label, Color: th.R.Fg, MaxLines: 1}.Layout(measureGTX, th, label)
		m.Stop()
		t.widths[i] = dims.Size.X + gtx.Dp(24)
		total += t.widths[i]
	}
	targetX := 0
	for i := range selected {
		targetX += t.widths[i]
	}
	t.x.Duration, t.width.Duration = th.Motion.Select, th.Motion.Select
	t.x.Ease, t.width.Ease = EaseChisel, EaseChisel
	t.x.Set(gtx, float32(targetX))
	t.width.Set(gtx, float32(t.widths[selected]))
	indicatorX := int(t.x.Value(gtx) + 0.5)
	indicatorW := int(t.width.Value(gtx) + 0.5)

	lineHeight := max(1, gtx.Dp(1))
	baseY := h - lineHeight
	FillCut(gtx.Ops, image.Rect(0, baseY, total, h), 0, disabledInk(gtx, th.R.Vein))
	bladeHeight := max(1, gtx.Dp(3))
	bladeRect := image.Rect(indicatorX, h-bladeHeight, indicatorX+indicatorW, h)
	points := tabBladePoints(bladeRect)
	paint.FillShape(gtx.Ops, disabledInk(gtx, th.R.Fg), clip.Outline{Path: pathFromPoints(gtx.Ops, points[:])}.Op())

	x := 0
	for i, label := range labels {
		i, label := i, label
		width := t.widths[i]
		offset := op.Offset(image.Pt(x, 0)).Push(gtx.Ops)
		tabGTX := exact(gtx, image.Pt(width, h))
		dims := t.tabs[i].Layout(tabGTX, func(gtx layout.Context) layout.Dimensions {
			semantic.Button.Add(gtx.Ops)
			semantic.LabelOp(label).Add(gtx.Ops)
			semantic.SelectedOp(i == selected).Add(gtx.Ops)
			hover := hoverValue(gtx, th, &t.hover[i], t.tabs[i].Hovered() && i != selected)
			ink := th.R.Fg3
			if i == selected {
				ink = th.R.Fg
			} else {
				ink = Mix(th.R.Fg3, th.R.Fg2, hover)
			}
			return layout.Center.Layout(gtx, rawText(th, th.T.Label, disabledInk(gtx, ink), label))
		})
		focus := focusValue(gtx, th, &t.focus[i], gtx.Focused(&t.tabs[i].Clickable))
		FocusRingAlpha(gtx, th, image.Rectangle{Max: dims.Size}, 0, focus)
		offset.Pop()
		x += width
	}
	return layout.Dimensions{Size: constrainMax(gtx, image.Pt(total, h))}
}
