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
)

const (
	wavePeriod                  = 1200 * time.Millisecond
	playbackMotionFrameInterval = time.Second / 60
)

// WaveState retains the wave amplitude and phase between frames.
type WaveState struct {
	amplitude anim.Spring
	ready     bool
	phase     float32
	phaseAt   time.Time
	polygon   []f32.Point
}

func (state *WaveState) reset() {
	if !state.ready {
		return
	}
	state.amplitude.Snap(0)
	state.phaseAt = time.Time{}
}

func waveDefaults(thickness, amplitude, wavelength unit.Dp) (unit.Dp, unit.Dp, unit.Dp) {
	if thickness <= 0 {
		thickness = 4
	}
	if amplitude <= 0 {
		amplitude = 3
	}
	if wavelength <= 0 {
		wavelength = 20
	}
	return thickness, amplitude, wavelength
}

func triangleWave(x, origin, phase, halfWave float32) float32 {
	u := (x - origin - phase) / halfWave
	n := int(math.Floor(float64(u)))
	fraction := u - float32(n)
	start := float32(-1)
	if n%2 != 0 {
		start = 1
	}
	return start + (-start-start)*fraction
}

func appendWavePoint(points []f32.Point, point f32.Point) []f32.Point {
	if len(points) > 0 {
		last := points[len(points)-1]
		if last.X == point.X && last.Y == point.Y {
			return points
		}
	}
	return append(points, point)
}

func wavePolygon(points []f32.Point, start, end, centre, amplitude, wavelength, phase, thickness float32) []f32.Point {
	points = points[:0]
	if end <= start || thickness <= 0 {
		return points
	}
	if amplitude <= 0 || wavelength <= 0 {
		return append(points,
			f32.Pt(start, centre-thickness/2),
			f32.Pt(end, centre-thickness/2),
			f32.Pt(end, centre+thickness/2),
			f32.Pt(start, centre+thickness/2),
		)
	}

	halfWave := wavelength / 2
	slope := 2 * amplitude / halfWave
	offset := thickness / 2 * float32(math.Sqrt(float64(1+slope*slope)))
	valueAt := func(x float32) float32 {
		return centre + amplitude*triangleWave(x, start, phase, halfWave)
	}
	points = appendWavePoint(points, f32.Pt(start, valueAt(start)-offset))
	firstVertex := int(math.Ceil(float64(-phase / halfWave)))
	for vertex := firstVertex; ; vertex++ {
		x := start + phase + float32(vertex)*halfWave
		if x <= start {
			continue
		}
		if x >= end {
			break
		}
		points = appendWavePoint(points, f32.Pt(x, valueAt(x)-offset))
	}
	points = appendWavePoint(points, f32.Pt(end, valueAt(end)-offset))
	upper := len(points)
	for index := upper - 1; index >= 0; index-- {
		point := points[index]
		points = append(points, f32.Pt(point.X, point.Y+2*offset))
	}
	return points
}

func fillPolygon(ops *op.Ops, points []f32.Point, ink color.NRGBA) {
	if len(points) < 3 || ink.A == 0 {
		return
	}
	var path clip.Path
	path.Begin(ops)
	path.MoveTo(points[0])
	for _, point := range points[1:] {
		path.LineTo(point)
	}
	path.LineTo(points[0])
	paint.FillShape(ops, ink, clip.Outline{Path: path.End()}.Op())
}

// DrawWave draws a chiselled played wave and its square unplayed track.
func DrawWave(gtx layout.Context, th *Theme, state *WaveState, rect image.Rectangle, fraction float32, fill, track color.NRGBA, thickness, amplitude, wavelength unit.Dp, live bool) {
	if state == nil || rect.Empty() {
		return
	}
	thickness, amplitude, wavelength = waveDefaults(thickness, amplitude, wavelength)
	thicknessPx := float32(gtx.Dp(thickness))
	amplitudePx := float32(gtx.Dp(amplitude))
	wavelengthPx := float32(gtx.Dp(wavelength))
	if thicknessPx <= 0 || wavelengthPx <= 0 {
		return
	}

	duration := th.Motion.Field
	if duration <= 0 {
		duration = time.Millisecond
	}
	omega := 4 / float32(duration.Seconds())
	target := float32(0)
	if live && !anim.Reduced() {
		target = amplitudePx
	}
	if !state.ready {
		state.amplitude = anim.Spring{Omega: omega, Rest: .02}
		state.amplitude.Snap(0)
		state.ready = true
	}
	state.amplitude.Omega = omega
	state.amplitude.Set(target)
	currentAmplitude := state.amplitude.Value(gtx.Disabled())
	if currentAmplitude < .01 && !state.amplitude.Moving() {
		currentAmplitude = 0
	}

	phaseMoving := live && currentAmplitude > 0 && !anim.Reduced()
	if phaseMoving {
		if !state.phaseAt.IsZero() {
			delta := gtx.Now.Sub(state.phaseAt)
			if delta > 0 {
				state.phase += float32(delta) / float32(wavePeriod) * wavelengthPx
				state.phase = float32(math.Mod(float64(state.phase), float64(wavelengthPx)))
			}
		}
		state.phaseAt = gtx.Now
	} else {
		state.phaseAt = time.Time{}
	}
	if phaseMoving || state.amplitude.Moving() {
		gtx.Execute(op.InvalidateCmd{At: gtx.Now.Add(playbackMotionFrameInterval)})
	}

	fraction = clampFraction(fraction)
	start := float32(rect.Min.X)
	end := float32(rect.Max.X)
	played := start + fraction*(end-start)
	centre := float32(rect.Min.Y+rect.Max.Y) / 2
	gap := float32(0)
	if currentAmplitude > float32(gtx.Dp(unit.Dp(.5))) {
		gap = float32(gtx.Dp(4))
	}
	trackStart := min(played+gap, end)
	if trackStart < end && track.A > 0 {
		top := int(math.Round(float64(centre - thicknessPx/2)))
		bottom := int(math.Round(float64(centre + thicknessPx/2)))
		if bottom <= top {
			bottom = top + 1
		}
		paint.FillShape(gtx.Ops, track, clip.Rect(image.Rect(int(math.Round(float64(trackStart))), top, rect.Max.X, bottom)).Op())
	}
	if played <= start {
		return
	}
	state.polygon = wavePolygon(state.polygon, start, played, centre, currentAmplitude, wavelengthPx, state.phase, thicknessPx)
	fillPolygon(gtx.Ops, state.polygon, fill)
}
