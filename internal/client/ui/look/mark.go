package look

import (
	"image"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
)

const ryokuMarkPath = "M381 0V210.7H70.8V366.9H374.7C355.9 543 284.6 749.5 36.2 875.3C73.9 902.5 131.6 962.3 156.7 1000C446 842.8 521.5 584.9 538.3 366.9H799.3C784.6 648.8 764.7 780.9 734.3 811.3C719.6 824.9 707 829.1 686.1 829.1C656.7 829.1 598 829.1 534.1 823.9C563.4 867.9 585.4 937.1 587.5 982.2C650.4 984.3 715.4 984.3 756.3 976.9C805.6 969.6 839.1 956 873.7 910.9C920.9 853.2 940.8 692.9 960.7 280.9C962.8 261 963.8 210.7 963.8 210.7H543.5V0Z"

// RyokuMark draws the Ryoku force mark.
func RyokuMark(size unit.Dp) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		pixels := gtx.Dp(size)
		dimensions := image.Pt(pixels, pixels)
		path, _, err := buildSVGPath(gtx.Ops, ryokuMarkPath, f32.Pt(1000, 1000), dimensions)
		if err == nil {
			paint.FillShape(gtx.Ops, RyokuOrange, clip.Outline{Path: path}.Op())
		}
		return layout.Dimensions{Size: dimensions}
	}
}
