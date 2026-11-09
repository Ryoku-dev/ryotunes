package lithic

import (
	"image"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/widget"
)

// CutImage draws an image fitted inside a cut rectangle.
func CutImage(gtx layout.Context, img paint.ImageOp, cut int, fit widget.Fit) layout.Dimensions {
	size := gtx.Constraints.Max
	if size.X < 0 || size.Y < 0 {
		size = image.Point{}
	}
	gtx.Constraints = layout.Exact(size)
	path := CutPath(gtx.Ops, image.Rectangle{Max: size}, cut, cut, cut, cut)
	stack := clip.Outline{Path: path}.Op().Push(gtx.Ops)
	dims := widget.Image{Src: img, Fit: fit, Position: layout.Center}.Layout(gtx)
	stack.Pop()
	return dims
}

// Collage draws four covers in a cut two-by-two device-playlist grid.
func Collage(gtx layout.Context, four [4]paint.ImageOp) layout.Dimensions {
	size := gtx.Constraints.Max
	if size.X < 0 || size.Y < 0 {
		size = image.Point{}
	}
	gap := max(0, gtx.Dp(4))
	left := max(0, (size.X-gap)/2)
	top := max(0, (size.Y-gap)/2)
	widths := [2]int{left, max(0, size.X-gap-left)}
	heights := [2]int{top, max(0, size.Y-gap-top)}
	outer := CutPath(gtx.Ops, image.Rectangle{Max: size}, gtx.Dp(7), gtx.Dp(7), gtx.Dp(7), gtx.Dp(7))
	mask := clip.Outline{Path: outer}.Op().Push(gtx.Ops)
	for i, img := range four {
		column, row := i%2, i/2
		x := column * (widths[0] + gap)
		y := row * (heights[0] + gap)
		offset := op.Offset(image.Pt(x, y)).Push(gtx.Ops)
		cell := gtx
		cell.Constraints = layout.Exact(image.Pt(widths[column], heights[row]))
		CutImage(cell, img, 0, widget.Cover)
		offset.Pop()
	}
	mask.Pop()
	return layout.Dimensions{Size: gtx.Constraints.Constrain(size)}
}
