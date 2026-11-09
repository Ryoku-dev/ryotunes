package look

import (
	"image"
	"image/color"
	"math"
	"time"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/anim"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
	"github.com/ryoku-dev/ryotunes/internal/client/state"
)

const (
	visualizerRiseOmega   = 4 / .050
	visualizerFallOmega   = 4 / .180
	visualizerScopeFrames = 2048
	peakCapHold           = 140 * time.Millisecond
	peakCapFall           = 550 * time.Millisecond
)

// VisualizerStyle configures the visual rhythm and ink.
type VisualizerStyle struct {
	Ink             color.NRGBA
	Bar, Gap, Floor unit.Dp
	// HeaderLane replaces Vectorscope with time-domain blades in short headers.
	HeaderLane bool
}

// Visualizer retains all drawing state and scratch storage between frames.
type Visualizer struct {
	springs []anim.Spring
	values  []float32
	caps    []peakCap
	used    int

	curvePoints   []f32.Point
	curveSlopes   []float32
	curveTangents []float32

	scopeBuffer []float32
	scopeFade   motionValue
	osc         oscilloscopeState
	vector      vectorscopeState
	pulse       beatDetector
	kick        float32

	style           state.VisualizerStyle
	previousStyle   state.VisualizerStyle
	styleReady      bool
	styleTransition time.Time
}

type motionValue struct {
	value, from, target float32
	start               time.Time
	ready               bool
}

type peakCap struct {
	height, peak, lastBlade float32
	holdUntil, fallStart    time.Time
	ready, resting          bool
}

func (cap *peakCap) update(now time.Time, blade, lane, gap float32) (bool, time.Time) {
	blade = min(max(blade, 0), lane)
	rising := cap.ready && blade > cap.lastBlade+.01
	cap.lastBlade = blade
	if !cap.ready {
		cap.ready = true
		if blade <= 0 {
			cap.height, cap.peak, cap.resting = 0, 0, true
			return false, time.Time{}
		}
		cap.height = min(lane, blade+gap)
		cap.peak = cap.height
		cap.holdUntil = now.Add(peakCapHold)
		return false, cap.holdUntil
	}
	if rising && blade+gap > cap.height+.01 {
		cap.height = min(lane, blade+gap)
		cap.peak = cap.height
		cap.holdUntil = now.Add(peakCapHold)
		cap.fallStart = time.Time{}
		cap.resting = false
		return false, cap.holdUntil
	}
	if !cap.holdUntil.IsZero() && now.Before(cap.holdUntil) {
		return false, cap.holdUntil
	}
	if cap.fallStart.IsZero() {
		if cap.resting && blade >= cap.height-.01 {
			cap.height = blade
			return false, time.Time{}
		}
		cap.peak = cap.height
		if cap.resting || cap.holdUntil.IsZero() {
			cap.fallStart = now
		} else {
			cap.fallStart = cap.holdUntil
		}
		cap.holdUntil = time.Time{}
		cap.resting = false
	}
	elapsed := max(0, now.Sub(cap.fallStart).Seconds())
	fallSeconds := peakCapFall.Seconds()
	acceleration := 2 * float64(lane) / (fallSeconds * fallSeconds)
	height := cap.peak - float32(.5*acceleration*elapsed*elapsed)
	if height <= blade+.01 {
		cap.height = blade
		cap.fallStart = time.Time{}
		cap.resting = true
		return false, time.Time{}
	}
	cap.height = height
	return true, time.Time{}
}

func (motion *motionValue) set(now time.Time, target float32, duration time.Duration) (float32, bool) {
	if !motion.ready {
		motion.value, motion.from, motion.target, motion.ready = target, target, target, true
		return target, false
	}
	if anim.Reduced() || duration <= 0 {
		motion.value, motion.from, motion.target, motion.start = target, target, target, time.Time{}
		return target, false
	}
	if target != motion.target {
		motion.value, _ = motion.at(now, duration)
		motion.from, motion.target, motion.start = motion.value, target, now
	}
	return motion.at(now, duration)
}

func (motion *motionValue) at(now time.Time, duration time.Duration) (float32, bool) {
	if motion.start.IsZero() {
		return motion.target, false
	}
	progress := float32(now.Sub(motion.start)) / float32(duration)
	if progress >= 1 {
		motion.value, motion.start = motion.target, time.Time{}
		return motion.target, false
	}
	progress = max(0, progress)
	motion.value = motion.from + (motion.target-motion.from)*anim.EaseChisel(progress)
	return motion.value, true
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

func scopeBarLevel(samples []float32, index, count int) float32 {
	frames := len(samples) / 2
	if frames == 0 || count <= 0 {
		return 0
	}
	start := index * frames / count
	end := max(start+1, (index+1)*frames/count)
	peak := float32(0)
	for frame := start; frame < min(end, frames); frame++ {
		mono := (samples[frame*2] + samples[frame*2+1]) * .5
		peak = max(peak, float32(math.Abs(float64(mono))))
	}
	return min(1, peak)
}

func (visualizer *Visualizer) prepare(count int) {
	for len(visualizer.springs) < count {
		spring := anim.Spring{Omega: visualizerFallOmega, Rest: .02}
		spring.Snap(0)
		visualizer.springs = append(visualizer.springs, spring)
		visualizer.values = append(visualizer.values, 0)
		visualizer.caps = append(visualizer.caps, peakCap{})
	}
	if count < visualizer.used {
		for index := count; index < visualizer.used; index++ {
			visualizer.springs[index].Snap(0)
			visualizer.values[index] = 0
			visualizer.caps[index] = peakCap{}
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

// ScopeBuffer returns reusable storage for the sample window visualizers read.
func (visualizer *Visualizer) ScopeBuffer() []float32 {
	required := visualizerScopeFrames * 2
	if cap(visualizer.scopeBuffer) < required {
		visualizer.scopeBuffer = make([]float32, required)
	}
	return visualizer.scopeBuffer[:required]
}

func effectiveVisualizerStyle(kind state.VisualizerStyle, style VisualizerStyle) state.VisualizerStyle {
	if style.HeaderLane && kind == state.VisualizerVectorscope {
		return state.VisualizerBars
	}
	return kind
}

func visualizerAlphaBucket(level float32) int {
	switch {
	case level < .25:
		return 0
	case level < .5:
		return 1
	case level < .75:
		return 2
	default:
		return 3
	}
}

// visualizerAlphaSteps brightens louder blades; bucket i paints at step i.
var visualizerAlphaSteps = [...]float32{.55, .7, .85, 1}

func pulsedVisualizerStyle(style VisualizerStyle, kick float32) VisualizerStyle {
	alpha := min(1, float32(style.Ink.A)/255+.25*min(max(kick, 0), 1))
	style.Ink.A = uint8(math.Round(float64(alpha * 255)))
	return style
}

func visualizerDisplayHeight(value, lane, kick float32) float32 {
	return min(lane, max(0, value)*(1+.14*min(max(kick, 0), 1)))
}

func (visualizer *Visualizer) updateBars(gtx layout.Context, levels, samples []float32, live, scopeBars bool, style VisualizerStyle) bool {
	size := gtx.Constraints.Max
	bar, gap, floor := visualizerMetrics(gtx, style)
	count := visualizerBarCount(size.X, bar, gap)
	visualizer.prepare(count)
	moving := false
	for index := range count {
		target := float32(0)
		if live {
			level := resampleLevel(levels, index, count)
			if scopeBars {
				level = scopeBarLevel(samples, index, count)
			}
			level = shapeVisualizerLevel(level)
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
		if value <= .01 && !spring.Moving() {
			value = 0
		}
		visualizer.values[index] = value
		moving = moving || spring.Moving()
	}
	return moving
}

func (visualizer *Visualizer) updatePeakCaps(gtx layout.Context) (bool, time.Time) {
	lane := float32(gtx.Constraints.Max.Y)
	gap := float32(max(1, gtx.Dp(2)))
	moving := false
	var wake time.Time
	for index := range visualizer.used {
		blade := visualizerDisplayHeight(visualizer.values[index], lane, visualizer.kick)
		capMoving, capWake := visualizer.caps[index].update(gtx.Now, blade, lane, gap)
		moving = moving || capMoving
		if !capWake.IsZero() && (wake.IsZero() || capWake.Before(wake)) {
			wake = capWake
		}
	}
	return moving, wake
}

func (visualizer *Visualizer) clearPeakCaps() {
	for index := range visualizer.used {
		visualizer.caps[index] = peakCap{}
	}
}

func (visualizer *Visualizer) drawBars(gtx layout.Context, style VisualizerStyle) {
	size := gtx.Constraints.Max
	bar, gap, _ := visualizerMetrics(gtx, style)
	for bucket, step := range visualizerAlphaSteps {
		var path clip.Path
		path.Begin(gtx.Ops)
		visible := false
		for index := range visualizer.used {
			value := visualizerDisplayHeight(visualizer.values[index], float32(size.Y), visualizer.kick)
			if value <= .01 || visualizerAlphaBucket(value/float32(size.Y)) != bucket {
				continue
			}
			left := float32(index * (bar + gap))
			right := float32(min(index*(bar+gap)+bar, size.X))
			appendVisualizerBlade(&path, left, right, float32(size.Y), value)
			visible = true
		}
		outline := path.End()
		if visible && style.Ink.A > 0 {
			paint.FillShape(gtx.Ops, palette.WithAlpha(style.Ink, step), clip.Outline{Path: outline}.Op())
		}
	}

	var caps clip.Path
	caps.Begin(gtx.Ops)
	capsVisible := false
	capThickness := float32(max(1, gtx.Dp(2)))
	for index := range visualizer.used {
		capHeight := visualizer.caps[index].height
		if capHeight <= .01 {
			continue
		}
		left := float32(index * (bar + gap))
		right := float32(min(index*(bar+gap)+bar, size.X))
		capTop := max(0, float32(size.Y)-capHeight-capThickness)
		caps.MoveTo(f32.Pt(left, capTop))
		caps.LineTo(f32.Pt(right, capTop))
		caps.LineTo(f32.Pt(right, min(float32(size.Y), capTop+capThickness)))
		caps.LineTo(f32.Pt(left, min(float32(size.Y), capTop+capThickness)))
		caps.Close()
		capsVisible = true
	}
	capOutline := caps.End()
	if capsVisible && style.Ink.A > 0 {
		paint.FillShape(gtx.Ops, style.Ink, clip.Outline{Path: capOutline}.Op())
	}
}

func (visualizer *Visualizer) drawStyle(gtx layout.Context, th *Theme, kind state.VisualizerStyle, style VisualizerStyle, alpha, scopeLevel float32) {
	if alpha <= 0 {
		return
	}
	style = pulsedVisualizerStyle(style, visualizer.kick)
	opacity := paint.PushOpacity(gtx.Ops, min(alpha, 1))
	switch effectiveVisualizerStyle(kind, style) {
	case state.VisualizerCurve:
		visualizer.drawCurve(gtx, style)
	case state.VisualizerOscilloscope:
		visualizer.drawOscilloscope(gtx, th, style, scopeLevel)
	case state.VisualizerVectorscope:
		visualizer.drawVectorscope(gtx, th, style, scopeLevel)
	default:
		visualizer.drawBars(gtx, style)
	}
	opacity.Pop()
}

func (visualizer *Visualizer) styleProgress(now time.Time, duration time.Duration) (float32, bool) {
	if visualizer.styleTransition.IsZero() || anim.Reduced() || duration <= 0 {
		return 1, false
	}
	progress := float32(now.Sub(visualizer.styleTransition)) / float32(duration)
	if progress >= 1 {
		visualizer.styleTransition = time.Time{}
		visualizer.previousStyle = ""
		return 1, false
	}
	return anim.EaseChisel(max(0, progress)), true
}

// Layout draws the selected visualizer into the maximum constraints.
func (visualizer *Visualizer) Layout(gtx layout.Context, th *Theme, levels, samples []float32, live bool, kind state.VisualizerStyle, style VisualizerStyle) layout.Dimensions {
	size := gtx.Constraints.Max
	if size.X <= 0 || size.Y <= 0 {
		return layout.Dimensions{Size: gtx.Constraints.Constrain(image.Point{})}
	}
	if anim.Reduced() {
		live = false
	}
	if !visualizer.styleReady {
		visualizer.style, visualizer.styleReady = kind, true
	} else if kind != visualizer.style {
		visualizer.previousStyle, visualizer.style = visualizer.style, kind
		visualizer.styleTransition = gtx.Now
	}

	current := effectiveVisualizerStyle(kind, style)
	if anim.Reduced() {
		visualizer.pulse = beatDetector{}
		visualizer.kick = 0
	} else {
		energy := spectrumBassEnergy(levels)
		if kind == state.VisualizerOscilloscope || kind == state.VisualizerVectorscope {
			energy = scopeRMSEnergy(samples)
		}
		visualizer.kick = visualizer.pulse.update(gtx.Now, energy, live)
	}
	pulseMoving := visualizer.kick > 0
	spectrumLive := live && (kind == state.VisualizerBars || kind == state.VisualizerCurve)
	scopeBars := style.HeaderLane && kind == state.VisualizerVectorscope
	barsMoving := visualizer.updateBars(gtx, levels, samples, spectrumLive || (live && scopeBars), scopeBars, style)
	scopeLive := live && (kind == state.VisualizerOscilloscope || (kind == state.VisualizerVectorscope && !scopeBars))
	scopeLevel, scopeMoving := visualizer.scopeFade.set(gtx.Now, boolFloat(scopeLive), th.Motion.Field)
	if current == state.VisualizerOscilloscope || visualizer.previousStyle == state.VisualizerOscilloscope {
		visualizer.osc.update(samples, gtx.Now, scopeLive)
	}
	if current == state.VisualizerVectorscope && scopeLive {
		visualizer.vector.update(samples)
	}

	barsVisible := current == state.VisualizerBars || effectiveVisualizerStyle(visualizer.previousStyle, style) == state.VisualizerBars
	capsMoving := false
	var capWake time.Time
	if barsVisible {
		capsMoving, capWake = visualizer.updatePeakCaps(gtx)
	} else {
		visualizer.clearPeakCaps()
	}

	progress, transitioning := visualizer.styleProgress(gtx.Now, th.Motion.Field)
	if transitioning {
		visualizer.drawStyle(gtx, th, visualizer.previousStyle, style, 1-progress, scopeLevel)
		visualizer.drawStyle(gtx, th, visualizer.style, style, progress, scopeLevel)
	} else {
		visualizer.drawStyle(gtx, th, visualizer.style, style, 1, scopeLevel)
	}
	if barsMoving || capsMoving || pulseMoving || scopeLive || scopeMoving || transitioning {
		gtx.Execute(op.InvalidateCmd{At: gtx.Now.Add(playbackMotionFrameInterval)})
	} else if !capWake.IsZero() && capWake.After(gtx.Now) {
		gtx.Execute(op.InvalidateCmd{At: capWake})
	}
	return layout.Dimensions{Size: size}
}

func boolFloat(value bool) float32 {
	if value {
		return 1
	}
	return 0
}
