package lithic

import (
	"image"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
)

// columnCount is how many columns of at least minWidth fit. It does not shrink
// to n, so a single search result keeps a tile's width instead of spanning
// the whole pane.
func columnCount(available, minWidth, gap, n int) int {
	if n <= 0 {
		return 0
	}
	minWidth = max(1, minWidth)
	gap = max(0, gap)
	return max(1, (available+gap)/(minWidth+gap))
}

// Columns lays cells into equal responsive columns without stretching sparse rows.
func Columns(gtx layout.Context, minCol, gap unit.Dp, n int, cell func(gtx layout.Context, i int) layout.Dimensions) layout.Dimensions {
	if n <= 0 {
		return layout.Dimensions{Size: gtx.Constraints.Constrain(image.Point{})}
	}
	available := gtx.Constraints.Max.X
	gapPx := max(0, gtx.Dp(gap))
	cols := columnCount(available, gtx.Dp(minCol), gapPx, n)
	colWidth := max(0, (available-gapPx*(cols-1))/cols)
	y := 0
	for first := 0; first < n; first += cols {
		count := min(cols, n-first)
		calls := make([]op.CallOp, count)
		dims := make([]layout.Dimensions, count)
		rowHeight := 0
		for col := range count {
			m := op.Record(gtx.Ops)
			cellGTX := gtx
			cellGTX.Constraints.Min.X = colWidth
			cellGTX.Constraints.Max.X = colWidth
			cellGTX.Constraints.Min.Y = 0
			cellGTX.Constraints.Max.Y = max(0, gtx.Constraints.Max.Y-y)
			dims[col] = cell(cellGTX, first+col)
			calls[col] = m.Stop()
			rowHeight = max(rowHeight, dims[col].Size.Y)
		}
		for col := range count {
			offset := op.Offset(image.Pt(col*(colWidth+gapPx), y)).Push(gtx.Ops)
			calls[col].Add(gtx.Ops)
			offset.Pop()
		}
		y += rowHeight
		if first+count < n {
			y += gapPx
		}
	}
	return layout.Dimensions{Size: gtx.Constraints.Constrain(image.Pt(available, y))}
}
