package shell

import (
	"image"

	gioapp "gioui.org/app"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryotunes/internal/client/lithic"
)

type decorations struct {
	window                    *gioapp.Window
	minimize, maximize, close lithic.IconButton
}

func (d *decorations) MoveArea(ops *op.Ops) {
	system.ActionInputOp(system.ActionMove).Add(ops)
}

func (d *decorations) Layout(gtx layout.Context, theme *lithic.Theme, mode gioapp.WindowMode) layout.Dimensions {
	d.minimize.Small, d.maximize.Small, d.close.Small = true, true, true
	children := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return d.minimize.Layout(gtx, theme, lithic.Quiet, lithic.Icon("remove"), "Minimize")
		}),
		layout.Rigid(layout.Spacer{Width: 4}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			label := "Maximize"
			actionIcon := lithic.Icon("open_in_new")
			if mode == gioapp.Maximized {
				label = "Restore"
				actionIcon = lithic.Icon("filter_list")
			}
			return d.maximize.Layout(gtx, theme, lithic.Quiet, actionIcon, label)
		}),
		layout.Rigid(layout.Spacer{Width: 4}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return d.close.Layout(gtx, theme, lithic.Quiet, lithic.Icon("close"), "Close")
		}),
	}
	dims := layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
	if d.minimize.Clicked(gtx) {
		d.window.Perform(system.ActionMinimize)
	}
	if d.maximize.Clicked(gtx) {
		if mode == gioapp.Maximized {
			d.window.Perform(system.ActionUnmaximize)
		} else {
			d.window.Perform(system.ActionMaximize)
		}
	}
	if d.close.Clicked(gtx) {
		d.window.Perform(system.ActionClose)
	}
	return layout.Dimensions{Size: image.Pt(dims.Size.X, max(dims.Size.Y, gtx.Dp(unit.Dp(32))))}
}
