package lithic

import (
	"image"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"
)

// MenuItem describes an action, divider, shortcut, and state in a menu.
type MenuItem struct {
	Icon            *widget.Icon
	Label, Shortcut string
	Danger          bool
	Disabled        bool
	Divider         bool
}

// Menu owns item interaction and Lithic menu-row motion.
type Menu struct {
	clicks       []Click
	disabled     []bool
	divider      []bool
	pending      []bool
	hover, press []Tween
}

func (m *Menu) prepare(items []MenuItem) {
	if len(m.clicks) != len(items) {
		clicks := make([]Click, len(items))
		copy(clicks, m.clicks)
		m.clicks = clicks
	}
	if len(m.disabled) != len(items) {
		m.disabled = make([]bool, len(items))
	}
	if len(m.divider) != len(items) {
		m.divider = make([]bool, len(items))
	}
	if len(m.pending) != len(items) {
		m.pending = make([]bool, len(items))
	}
	if len(m.hover) != len(items) {
		hover := make([]Tween, len(items))
		copy(hover, m.hover)
		m.hover = hover
	}
	if len(m.press) != len(items) {
		press := make([]Tween, len(items))
		copy(press, m.press)
		m.press = press
	}
	for i, item := range items {
		m.disabled[i] = item.Disabled
		m.divider[i] = item.Divider
		if item.Disabled || item.Divider {
			m.pending[i] = false
		}
	}
}

// Layout renders a cut menu panel and all items.
func (m *Menu) Layout(gtx layout.Context, th *Theme, items []MenuItem) layout.Dimensions {
	m.prepare(items)
	for i := range m.clicks {
		if !m.disabled[i] && !m.divider[i] && m.clicks[i].Clicked(gtx) {
			m.pending[i] = true
		}
	}
	width := menuWidth(gtx, th, items)
	gtx.Constraints.Min.X = width
	gtx.Constraints.Max.X = width

	content := op.Record(gtx.Ops)
	dims := layout.Inset{Top: 6, Bottom: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		children := make([]layout.FlexChild, 0, len(items))
		for i, item := range items {
			i, item := i, item
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if item.Divider {
					return layout.Inset{Top: 6, Bottom: 6, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						width := gtx.Constraints.Max.X
						FillCut(gtx.Ops, image.Rect(0, 0, width, max(1, gtx.Dp(1))), 0, th.R.Vein)
						return layout.Dimensions{Size: image.Pt(width, max(1, gtx.Dp(1)))}
					})
				}
				row := func(gtx layout.Context) layout.Dimensions {
					height := gtx.Dp(40)
					gtx.Constraints.Min.Y = height
					gtx.Constraints.Max.Y = height
					hover := hoverValue(gtx, th, &m.hover[i], m.clicks[i].Hovered() && !item.Disabled)
					pressed := transitionValue(gtx, &m.press[i], m.clicks[i].Pressed() && !item.Disabled, th.Motion.Press, th.Motion.Press)
					if hover > 0 || pressed > 0 {
						fill := Mix(th.R.Stone4, th.R.Stone3, hover)
						fill = Mix(fill, th.R.Fg, .08*pressed)
						FillCut(gtx.Ops, image.Rect(0, 0, gtx.Constraints.Max.X, height), 0, fill)
					}
					return layout.Inset{Left: 16, Right: 16}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return centerY(gtx, func(gtx layout.Context) layout.Dimensions {
							ink := th.R.Fg
							if item.Danger {
								ink = th.R.JasperInk
							}
							if item.Disabled {
								ink = th.R.Fg3
							}
							children := make([]layout.FlexChild, 0, 4)
							if item.Icon != nil {
								children = append(children,
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										size := gtx.Dp(20)
										gtx.Constraints = layout.Exact(image.Pt(size, size))
										return item.Icon.Layout(gtx, ink)
									}),
									layout.Rigid(layout.Spacer{Width: 12}.Layout),
								)
							}
							children = append(children, layout.Flexed(1, rawText(th, th.T.Label, ink, item.Label)))
							if item.Shortcut != "" {
								children = append(children,
									layout.Rigid(layout.Spacer{Width: unit.Dp(13)}.Layout),
									layout.Rigid(rawText(th, th.T.MonoSm, th.R.Fg3, item.Shortcut)),
								)
							}
							return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
						})
					})
				}
				if item.Disabled {
					return m.clicks[i].Layout(gtx.Disabled(), row)
				}
				return m.clicks[i].Layout(gtx, row)
			}))
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	})
	contentCall := content.Stop()

	overlay := op.Record(gtx.Ops)
	cut := gtx.Dp(8)
	outline := max(1, gtx.Dp(1))
	r := image.Rectangle{Max: dims.Size}
	FillCut(gtx.Ops, r, cut, th.R.Stone4)
	StrokeCut(gtx.Ops, r, cut, outline, th.R.Vein)
	contentCall.Add(gtx.Ops)
	deferOverlay(gtx, overlay.Stop())
	return dims
}

func menuWidth(gtx layout.Context, th *Theme, items []MenuItem) int {
	maximum := min(gtx.Dp(360), gtx.Constraints.Max.X)
	minimum := min(gtx.Dp(220), maximum)
	widest := minimum
	var measureOps op.Ops
	for _, item := range items {
		if item.Divider {
			continue
		}
		fixed := gtx.Dp(32)
		if item.Icon != nil {
			fixed += gtx.Dp(32)
		}
		if item.Shortcut != "" {
			measure := gtx
			measure.Constraints.Min = image.Point{}
			measure.Constraints.Max.X = max(0, maximum-fixed-gtx.Dp(13))
			measureOps.Reset()
			measure.Ops = &measureOps
			dims := Label{Style: th.T.MonoSm, Color: th.R.Fg3, MaxLines: 1}.Layout(measure, th, item.Shortcut)
			fixed += gtx.Dp(13) + dims.Size.X
		}
		measure := gtx
		measure.Constraints.Min = image.Point{}
		measure.Constraints.Max.X = max(0, maximum-fixed)
		measureOps.Reset()
		measure.Ops = &measureOps
		dims := Label{Style: th.T.Label, Color: th.R.Fg, MaxLines: 1}.Layout(measure, th, item.Label)
		widest = max(widest, min(maximum, fixed+dims.Size.X))
	}
	return widest
}

// Clicked reports and clears the chosen item index.
func (m *Menu) Clicked(gtx layout.Context) (int, bool) {
	for i := range m.clicks {
		if m.pending[i] {
			m.pending[i] = false
			return i, true
		}
		if !m.disabled[i] && !m.divider[i] && m.clicks[i].Clicked(gtx) {
			return i, true
		}
	}
	return 0, false
}

func centerY(gtx layout.Context, w layout.Widget) layout.Dimensions {
	original := gtx.Constraints
	gtx.Constraints.Min.Y = 0
	record := op.Record(gtx.Ops)
	dims := w(gtx)
	call := record.Stop()
	size := original.Constrain(image.Pt(dims.Size.X, max(original.Min.Y, dims.Size.Y)))
	y := (size.Y - dims.Size.Y) / 2
	offset := op.Offset(image.Pt(0, y)).Push(gtx.Ops)
	call.Add(gtx.Ops)
	offset.Pop()
	return layout.Dimensions{
		Size:     size,
		Baseline: dims.Baseline + size.Y - dims.Size.Y - y,
	}
}
