package look

import (
	"image"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"golang.org/x/exp/shiny/materialdesign/icons"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/tokens"
)

var choiceChevron = kit.Icon(icons.NavigationExpandMore)

// Choice is a window-coordinate menu selector.
type Choice struct {
	click kit.Click
	menu  kit.Menu
	size  image.Point
	value int
	ready bool
}

// Layout draws the choice plaque and manages its overlay menu.
func (choice *Choice) Layout(gtx layout.Context, theme *Theme, overlays *Overlays, options []string, selected int) layout.Dimensions {
	if selected >= 0 && selected < len(options) && !choice.ready {
		choice.value = selected
	}
	if choice.click.Clicked(gtx) && overlays != nil {
		if overlays.IsOpen(&choice.click.Clickable) {
			overlays.Close(&choice.click.Clickable)
		} else {
			anchor := overlays.AnchorOf(&choice.click.Clickable, choice.size)
			items := make([]kit.MenuItem, len(options))
			for index, option := range options {
				items[index] = kit.MenuItem{Label: option}
			}
			content := func(gtx layout.Context) layout.Dimensions {
				dimensions := choice.menu.Layout(gtx, theme.Theme, items)
				if index, ok := choice.menu.Clicked(gtx); ok {
					choice.value, choice.ready = index, true
					overlays.Close(&choice.click.Clickable)
					gtx.Execute(op.InvalidateCmd{})
				}
				return dimensions
			}
			width := unit.Dp(float32(choice.size.X) / gtx.Metric.PxPerDp)
			overlays.Open(&choice.click.Clickable, anchor, Placement{Prefer: Below, Align: Start, Width: width}, content)
		}
	}
	label := ""
	if selected >= 0 && selected < len(options) {
		label = options[selected]
	}
	if choice.ready && choice.value >= 0 && choice.value < len(options) {
		label = options[choice.value]
	}
	height := gtx.Dp(40)
	body := func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
		return layout.Inset{Left: 13, Right: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return choiceCenterY(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Flexed(1, kit.Text(theme.Theme, theme.T.Label, theme.R.Fg, label)),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						size := gtx.Dp(tokens.SizePickerArrow)
						gtx.Constraints = layout.Exact(image.Pt(size, size))
						return choiceChevron.Layout(gtx, theme.R.Fg2)
					}),
				)
			})
		})
	}
	record := op.Record(gtx.Ops)
	dimensions := choice.click.Layout(gtx, body)
	call := record.Stop()
	kit.FillCut(gtx.Ops, image.Rectangle{Max: dimensions.Size}, gtx.Dp(theme.Cut.Small), theme.R.Stone3)
	call.Add(gtx.Ops)
	choice.size = dimensions.Size
	return dimensions
}

// Changed reports and clears the most recent menu selection.
func (choice *Choice) Changed() (int, bool) {
	value, ready := choice.value, choice.ready
	choice.ready = false
	return value, ready
}

func choiceCenterY(gtx layout.Context, widget layout.Widget) layout.Dimensions {
	original := gtx.Constraints
	gtx.Constraints.Min.Y = 0
	record := op.Record(gtx.Ops)
	dimensions := widget(gtx)
	call := record.Stop()
	size := original.Constrain(image.Pt(dimensions.Size.X, max(original.Min.Y, dimensions.Size.Y)))
	y := (size.Y - dimensions.Size.Y) / 2
	offset := op.Offset(image.Pt(0, y)).Push(gtx.Ops)
	call.Add(gtx.Ops)
	offset.Pop()
	return layout.Dimensions{Size: size, Baseline: dimensions.Baseline + size.Y - dimensions.Size.Y - y}
}
