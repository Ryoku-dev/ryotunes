package lithic

import (
	"image"

	"gioui.org/io/key"
	"gioui.org/layout"
)

// Picker combines a field anchor, popover, and menu.
type Picker struct {
	button       Click
	popover      Popover
	menu         Menu
	hover, focus Tween
}

// Focus requests keyboard focus for the picker anchor.
func (p *Picker) Focus(gtx layout.Context) {
	gtx.Execute(key.FocusCmd{Tag: &p.button.Clickable})
}

// Layout renders the anchor and its option menu.
func (p *Picker) Layout(gtx layout.Context, th *Theme, label string, options []string, selected int) layout.Dimensions {
	if p.button.Clicked(gtx) {
		p.popover.Open = !p.popover.Open
	}
	value := ""
	if selected >= 0 && selected < len(options) {
		value = options[selected]
	}

	anchor := func(gtx layout.Context) layout.Dimensions {
		children := make([]layout.FlexChild, 0, 3)
		if label != "" {
			children = append(children,
				layout.Rigid(Text(th, th.T.Caption, th.R.Fg3, label)),
				layout.Rigid(layout.Spacer{Height: 4}.Layout),
			)
		}
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			dims := p.button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				height := gtx.Dp(44)
				width := max(gtx.Constraints.Min.X, gtx.Dp(160))
				width = min(width, gtx.Constraints.Max.X)
				r := image.Rect(0, 0, width, height)
				hover := hoverValue(gtx, th, &p.hover, p.button.Hovered())
				fill := Mix(th.R.Stone3, th.R.Fg, .06*hover)
				FillCut(gtx.Ops, r, gtx.Dp(th.Cut.Mid), fill)
				gtx.Constraints = layout.Exact(r.Max)
				return layout.Inset{Left: 14, Right: 14}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return centerY(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							layout.Flexed(1, rawText(th, th.T.Body, th.R.Fg, value)),
							layout.Rigid(layout.Spacer{Width: 10}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								size := gtx.Dp(20)
								gtx.Constraints = layout.Exact(image.Pt(size, size))
								return Icon("expand_more").Layout(gtx, th.R.Fg2)
							}),
						)
					})
				})
			})
			focus := focusValue(gtx, th, &p.focus, gtx.Focused(&p.button.Clickable))
			FocusRingAlpha(gtx, th, image.Rectangle{Max: dims.Size}, gtx.Dp(th.Cut.Mid), focus)
			return dims
		}))
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	}
	content := func(gtx layout.Context) layout.Dimensions {
		items := make([]MenuItem, len(options))
		for i, option := range options {
			items[i] = MenuItem{Label: option}
		}
		return p.menu.Layout(gtx, th, items)
	}
	return p.popover.Layout(gtx, anchor, content)
}

// Changed reports and closes after an option selection.
func (p *Picker) Changed(gtx layout.Context) (int, bool) {
	if selected, ok := p.menu.Clicked(gtx); ok {
		p.popover.Open = false
		return selected, true
	}
	return 0, false
}
