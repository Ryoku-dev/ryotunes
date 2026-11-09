package lithic

import (
	"gioui.org/layout"
)

// Confirm is a destructive decision dialog with optional typed confirmation.
type Confirm struct {
	Dialog Dialog

	cancel  Button
	confirm Button
	field   TextField
	closed  bool
}

func confirmReady(input, required string) bool {
	return required == "" || input == required
}

// Layout renders the confirmation dialog and reports a confirmed action.
func (c *Confirm) Layout(gtx layout.Context, th *Theme, title, body, action, typeToConfirm string) bool {
	enabled := confirmReady(c.field.Text(), typeToConfirm)
	confirmed := false
	if c.cancel.Clicked(gtx) {
		c.Dialog.dismiss()
		c.field.SetText("")
	} else if enabled && c.confirm.Clicked(gtx) {
		confirmed = true
		c.Dialog.dismiss()
		c.field.SetText("")
	}

	bodyWidget := func(gtx layout.Context) layout.Dimensions {
		children := []layout.FlexChild{
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return Label{Style: th.T.Body, Color: th.R.Fg2}.Layout(gtx, th, body)
			}),
		}
		if typeToConfirm != "" {
			children = append(children,
				layout.Rigid(layout.Spacer{Height: 21}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return c.field.Layout(gtx, th, "Type "+typeToConfirm+" to confirm", "", "", "")
				}),
			)
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	}
	keep := func(gtx layout.Context) layout.Dimensions {
		return c.cancel.Layout(gtx, th, Line, nil, "Keep it")
	}
	danger := func(gtx layout.Context) layout.Dimensions {
		if !enabled {
			gtx = gtx.Disabled()
		}
		return c.confirm.Layout(gtx, th, Danger, nil, action)
	}
	c.Dialog.Layout(gtx, th, title, bodyWidget, keep, danger)
	if c.Dialog.Closed(gtx) {
		c.field.SetText("")
		c.closed = true
	}
	return confirmed
}

// Closed reports and clears a dismissal without confirmation.
func (c *Confirm) Closed() bool {
	closed := c.closed
	c.closed = false
	return closed
}
