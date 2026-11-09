package lithic

import (
	"image"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
)

// RowList combines scrolling rows with a moving selection slab.
type RowList struct {
	List

	heights       []int
	offsets       []int
	defaultHeight int
	slabY         Tween
	slabTarget    int
	selected      int
	selectedReady bool
	slabVisible   bool
}

func fillRowOffsets(dst, heights []int, gap, fallback int) []int {
	if cap(dst) < len(heights) {
		dst = make([]int, len(heights))
	} else {
		dst = dst[:len(heights)]
	}
	y := 0
	for i, height := range heights {
		dst[i] = y
		if height <= 0 {
			height = fallback
		}
		y += height
		if i+1 < len(heights) {
			y += gap
		}
	}
	return dst
}

func (l *RowList) resize(n, fallback int) {
	if l.defaultHeight != fallback {
		l.heights = nil
		l.offsets = nil
		l.defaultHeight = fallback
	}
	if cap(l.heights) < n {
		next := make([]int, n)
		copy(next, l.heights)
		l.heights = next
	} else {
		old := len(l.heights)
		l.heights = l.heights[:n]
		if n > old {
			clear(l.heights[old:])
		}
	}
}

// Layout renders rows and tracks the selected row through scrolling.
func (l *RowList) Layout(gtx layout.Context, th *Theme, n int, selected int, row func(gtx layout.Context, i int) layout.Dimensions) layout.Dimensions {
	n = max(0, n)
	gap := gtx.Dp(4)
	fallback := gtx.Dp(th.RowHeight())
	l.resize(n, fallback)
	l.List.Gap = gap
	l.slabY.Duration = th.Motion.Select
	l.slabY.Ease = EaseChisel

	macro := op.Record(gtx.Ops)
	dims := l.List.Layout(gtx, th, n, func(gtx layout.Context, i int) layout.Dimensions {
		dims := row(gtx, i)
		if i >= 0 && i < len(l.heights) {
			l.heights[i] = dims.Size.Y
		}
		return dims
	})
	content := macro.Stop()

	l.offsets = fillRowOffsets(l.offsets, l.heights, gap, fallback)
	visible := false
	targetY, slabH := 0, fallback
	if selected >= 0 && selected < len(l.offsets) && l.Position.First >= 0 && l.Position.First < len(l.offsets) {
		targetY = l.offsets[selected] - l.offsets[l.Position.First] - l.Position.Offset
		slabH = l.heights[selected]
		if slabH <= 0 {
			slabH = fallback
		}
		visible = targetY < dims.Size.Y && targetY+slabH > 0
	}
	changed := !l.selectedReady || l.selected != selected
	if visible {
		switch {
		case !l.selectedReady || !l.slabVisible:
			l.slabY = Tween{Duration: th.Motion.Select, Ease: EaseChisel}
			l.slabY.Set(gtx, float32(targetY))
		case changed:
			l.slabY.Set(gtx, float32(targetY))
		case l.slabTarget != targetY:
			// Scrolling moves the selected row and slab together; only a
			// selection change glides independently.
			l.slabY = Tween{Duration: th.Motion.Select, Ease: EaseChisel}
			l.slabY.Set(gtx, float32(targetY))
		}
		l.slabTarget = targetY
		y := int(l.slabY.Value(gtx) + .5)
		stack := clip.Rect(image.Rectangle{Max: dims.Size}).Push(gtx.Ops)
		FillCut(gtx.Ops, image.Rect(0, y, dims.Size.X, y+slabH), 0, th.R.Sel)
		stack.Pop()
	}
	l.selected = selected
	l.selectedReady = true
	l.slabVisible = visible
	content.Add(gtx.Ops)
	return dims
}
