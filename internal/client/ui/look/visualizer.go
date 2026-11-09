package look

import (
	"image"
	"image/color"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/anim"
)

const (
	visualizerRiseOmega = 4 / .070
	visualizerFallOmega = 4 / .220
)

// VisualizerStyle configures the blade rhythm and ink.
type VisualizerStyle struct {
	Ink             color.NRGBA
	Bar, Gap, Floor unit.Dp
}

// Visualizer retains one spring per visible blade.
type Visualizer struct {
	springs []anim.Spring
	values  []float32
	used    int
}

func visualizerMetrics(gtx layout.Context, style VisualizerStyle) (bar, gap, floor int) {
	if style.Bar <= 0 {
		style.Bar = 6
	}
	if style.Gap < 0 {
		style.Gap = 0
	} else if style.Gap == 0 {
		style.Gap = 3
	}
	if style.Floor <= 0 {
		style.Floor = 2
	}
	return max(1, gtx.Dp(style.Bar)), max(0, gtx.Dp(style.Gap)), max(0, gtx.Dp(style.Floor))
}

func visualizerBarCount(width, bar, gap int) int {
	if width <= 0 || bar <= 0 {
		return 0
	}
	return max(1, (width+gap)/(bar+gap))
}

func resampleLevel(levels []float32, index, count int) float32 {
	if len(levels) == 0 || count <= 0 {
		return 0
	}
	if len(levels) == 1 || count == 1 {
		return min(max(levels[0], 0), 1)
	}
	position := float32(index) * float32(len(levels)-1) / float32(count-1)
	left := min(max(int(position), 0), len(levels)-1)
	right := min(left+1, len(levels)-1)
	fraction := position - float32(left)
	value := levels[left] + (levels[right]-levels[left])*fraction
	return min(max(value, 0), 1)
}

func (visualizer *Visualizer) prepare(count int) {
	for len(visualizer.springs) < count {
		spring := anim.Spring{Omega: visualizerFallOmega, Rest: .02}
		spring.Snap(0)
		visualizer.springs = append(visualizer.springs, spring)
		visualizer.values = append(visualizer.values, 0)
	}
	if count < visualizer.used {
		for index := count; index < visualizer.used; index++ {
			visualizer.springs[index].Snap(0)
			visualizer.values[index] = 0
		}
	}
	visualizer.used = count
}

func appendVisualizerBlade(path *clip.Path, left, right, bottom, height float32) {
	if height <= 0 || right <= left {
		return
	}
	cut := min(right-left, height)
	top := bottom - height
	path.MoveTo(f32.Pt(left, bottom))
	path.LineTo(f32.Pt(left, top))
	path.LineTo(f32.Pt(right-cut, top))
	path.LineTo(f32.Pt(right, top+cut))
	path.LineTo(f32.Pt(right, bottom))
	path.LineTo(f32.Pt(left, bottom))
}

// Layout draws the visualizer into the maximum constraints.
func (visualizer *Visualizer) Layout(gtx layout.Context, th *Theme, levels []float32, live bool, style VisualizerStyle) layout.Dimensions {
	_ = th
	size := gtx.Constraints.Max
	if size.X <= 0 || size.Y <= 0 {
		return layout.Dimensions{Size: gtx.Constraints.Constrain(image.Point{})}
	}
	bar, gap, floor := visualizerMetrics(gtx, style)
	count := visualizerBarCount(size.X, bar, gap)
	visualizer.prepare(count)
	if anim.Reduced() {
		for index := 0; index < count; index++ {
			visualizer.springs[index].Snap(0)
			visualizer.values[index] = 0
		}
		return layout.Dimensions{Size: size}
	}

	var path clip.Path
	path.Begin(gtx.Ops)
	moving := false
	visible := false
	for index := 0; index < count; index++ {
		target := float32(0)
		if live {
			level := resampleLevel(levels, index, count)
			target = float32(floor) + level*float32(max(size.Y-floor, 0))
		}
		spring := &visualizer.springs[index]
		if target > visualizer.values[index] {
			spring.Omega = visualizerRiseOmega
		} else {
			spring.Omega = visualizerFallOmega
		}
		spring.Set(target)
		value := spring.Value(gtx.Disabled())
		visualizer.values[index] = value
		moving = moving || spring.Moving()
		if value <= .01 {
			continue
		}
		visible = true
		left := float32(index * (bar + gap))
		right := float32(min(index*(bar+gap)+bar, size.X))
		appendVisualizerBlade(&path, left, right, float32(size.Y), min(value, float32(size.Y)))
	}
	outline := path.End()
	if visible && style.Ink.A > 0 {
		paint.FillShape(gtx.Ops, style.Ink, clip.Outline{Path: outline}.Op())
	}
	if live || moving {
		gtx.Execute(op.InvalidateCmd{At: gtx.Now.Add(playbackMotionFrameInterval)})
	}
	return layout.Dimensions{Size: size}
}
