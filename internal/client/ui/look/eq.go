package look

import (
	"fmt"
	"image"
	"image/color"
	"math"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
	"github.com/ryoku-dev/ryotunes/internal/eq"
	"github.com/ryoku-dev/ryotunes/internal/model"
)

var equalizerFrequencies = eq.LogFrequencies(384)

var graphFrequencyTicks = []float64{20, 50, 100, 200, 500, 1000, 2000, 5000, 10000, 20000}
var graphGainTicks = []float64{12, 6, 0, -6, -12}

// FrequencyX maps the audible range logarithmically into width.
func FrequencyX(frequency float64, width int) float32 {
	frequency = min(max(frequency, model.EQMinFrequency), model.EQMaxFrequency)
	return float32(math.Log(frequency/model.EQMinFrequency)/math.Log(model.EQMaxFrequency/model.EQMinFrequency)) * float32(width)
}

// XFrequency maps a graph position back into the audible range.
func XFrequency(x float32, width int) float64 {
	if width <= 0 {
		return model.EQMinFrequency
	}
	fraction := min(max(float64(x)/float64(width), 0), 1)
	return model.EQMinFrequency * math.Exp(math.Log(model.EQMaxFrequency/model.EQMinFrequency)*fraction)
}

func gainY(gain float64, height int) float32 {
	gain = min(max(gain, model.EQMinGain), model.EQMaxGain)
	return float32((model.EQMaxGain-gain)/(model.EQMaxGain-model.EQMinGain)) * float32(height)
}

func yGain(y float32, height int) float64 {
	if height <= 0 {
		return 0
	}
	gain := model.EQMaxGain - float64(y)/float64(height)*(model.EQMaxGain-model.EQMinGain)
	return min(max(gain, model.EQMinGain), model.EQMaxGain)
}

func appendLine(path *clip.Path, points []f32.Point) {
	if len(points) == 0 {
		return
	}
	path.MoveTo(points[0])
	for _, point := range points[1:] {
		path.LineTo(point)
	}
}

func strokePolyline(ops *op.Ops, points []f32.Point, width float32, ink color.NRGBA) {
	if len(points) < 2 || width <= 0 || ink.A == 0 {
		return
	}
	var path clip.Path
	path.Begin(ops)
	appendLine(&path, points)
	paint.FillShape(ops, ink, clip.Stroke{Path: path.End(), Width: width}.Op())
}

func fillResponse(ops *op.Ops, points []f32.Point, zero float32, ink color.NRGBA) {
	if len(points) < 2 || ink.A == 0 {
		return
	}
	var path clip.Path
	path.Begin(ops)
	path.MoveTo(f32.Pt(points[0].X, zero))
	for _, point := range points {
		path.LineTo(point)
	}
	path.LineTo(f32.Pt(points[len(points)-1].X, zero))
	path.Close()
	paint.FillShape(ops, ink, clip.Outline{Path: path.End()}.Op())
}

func formatGraphFrequency(value float64) string {
	if value >= 1000 {
		return fmt.Sprintf("%gk", value/1000)
	}
	return fmt.Sprintf("%g", value)
}

// FormatFrequency writes a compact frequency value for instrument labels.
func FormatFrequency(value float64) string {
	if value >= 1000 {
		if value >= 9950 {
			return fmt.Sprintf("%.0fk", value/1000)
		}
		return fmt.Sprintf("%.1fk", value/1000)
	}
	return fmt.Sprintf("%.0f", value)
}

// FormatGain writes a signed one-decimal dB value.
func FormatGain(value float64) string {
	if math.Abs(value) < .05 {
		value = 0
	}
	return fmt.Sprintf("%+.1f", value)
}

func responsePoints(curve []float64, width, height int, dst []f32.Point) []f32.Point {
	if cap(dst) < len(curve) {
		dst = make([]f32.Point, len(curve))
	} else {
		dst = dst[:len(curve)]
	}
	for index, gain := range curve {
		dst[index] = f32.Pt(float32(index)*float32(width)/float32(max(1, len(curve)-1)), gainY(gain, height))
	}
	return dst
}

func graphTickInk(th *Theme, zero bool) color.NRGBA {
	if zero {
		return th.R.Fg3
	}
	return th.R.Vein
}

func drawGraphGrid(gtx layout.Context, th *Theme, bounds image.Rectangle) {
	for _, frequency := range graphFrequencyTicks {
		x := bounds.Min.X + int(math.Round(float64(FrequencyX(frequency, bounds.Dx()))))
		paint.FillShape(gtx.Ops, th.R.Vein, clip.Rect(image.Rect(x, bounds.Min.Y, x+max(1, gtx.Dp(1)), bounds.Max.Y)).Op())
	}
	for _, gain := range graphGainTicks {
		y := bounds.Min.Y + int(math.Round(float64(gainY(gain, bounds.Dy()))))
		paint.FillShape(gtx.Ops, graphTickInk(th, gain == 0), clip.Rect(image.Rect(bounds.Min.X, y, bounds.Max.X, y+max(1, gtx.Dp(1)))).Op())
	}
}

func drawGraphLabels(gtx layout.Context, th *Theme, bounds image.Rectangle) {
	for index, frequency := range graphFrequencyTicks {
		x := bounds.Min.X + int(math.Round(float64(FrequencyX(frequency, bounds.Dx()))))
		align := text.Middle
		labelX := x - gtx.Dp(20)
		if index == 0 {
			align, labelX = text.Start, bounds.Min.X
		} else if index == len(graphFrequencyTicks)-1 {
			align, labelX = text.End, bounds.Max.X-gtx.Dp(40)
		}
		labelGTX := gtx
		labelGTX.Constraints.Min = image.Pt(gtx.Dp(40), 0)
		labelGTX.Constraints.Max = image.Pt(gtx.Dp(40), gtx.Dp(18))
		offset := op.Offset(image.Pt(labelX, bounds.Max.Y+gtx.Dp(4))).Push(gtx.Ops)
		kit.Label{Style: th.T.MonoSm, Color: th.R.Fg3, MaxLines: 1, Align: align}.Layout(labelGTX, th.Theme, formatGraphFrequency(frequency))
		offset.Pop()
	}
	for _, gain := range graphGainTicks {
		y := bounds.Min.Y + int(math.Round(float64(gainY(gain, bounds.Dy())))) - gtx.Dp(7)
		labelGTX := gtx
		labelGTX.Constraints.Min = image.Pt(gtx.Dp(32), 0)
		labelGTX.Constraints.Max = image.Pt(gtx.Dp(32), gtx.Dp(16))
		offset := op.Offset(image.Pt(gtx.Dp(6), y)).Push(gtx.Ops)
		label := fmt.Sprintf("%+g", gain)
		if gain == 0 {
			label = "0"
		}
		kit.Label{Style: th.T.MonoSm, Color: th.R.Fg3, MaxLines: 1}.Layout(labelGTX, th.Theme, label)
		offset.Pop()
	}
}

func drawSpectrum(gtx layout.Context, th *Theme, bounds image.Rectangle, levels []float32) {
	if len(levels) == 0 {
		return
	}
	const lowFrequency, highFrequency = 50.0, 10000.0
	var path clip.Path
	path.Begin(gtx.Ops)
	drawn := false
	span := math.Log(highFrequency / lowFrequency)
	for index, level := range levels {
		level = min(max(level, 0), 1)
		fraction := float64(index) / float64(max(1, len(levels)-1))
		frequency := lowFrequency * math.Exp(span*fraction)
		nextFraction := float64(index+1) / float64(max(1, len(levels)-1))
		nextFrequency := min(highFrequency, lowFrequency*math.Exp(span*nextFraction))
		left := bounds.Min.X + int(math.Round(float64(FrequencyX(frequency, bounds.Dx()))))
		right := bounds.Min.X + int(math.Round(float64(FrequencyX(nextFrequency, bounds.Dx())))) - gtx.Dp(2)
		right = min(bounds.Max.X, max(left+1, right))
		height := int(float32(bounds.Dy()) * level * .78)
		if height <= 0 {
			continue
		}
		top, bottom := bounds.Max.Y-height, bounds.Max.Y
		cut := min(gtx.Dp(3), max(0, right-left))
		path.MoveTo(f32.Pt(float32(left), float32(bottom)))
		path.LineTo(f32.Pt(float32(left), float32(top)))
		path.LineTo(f32.Pt(float32(right-cut), float32(top)))
		path.LineTo(f32.Pt(float32(right), float32(top+cut)))
		path.LineTo(f32.Pt(float32(right), float32(bottom)))
		path.Close()
		drawn = true
	}
	spec := path.End()
	if drawn {
		paint.FillShape(gtx.Ops, palette.WithAlpha(th.R.Fg3, .1), clip.Outline{Path: spec}.Op())
	}
}

func drawDiamond(ops *op.Ops, center f32.Point, radius float32, fill, rim color.NRGBA, hollow bool) {
	var path clip.Path
	path.Begin(ops)
	path.MoveTo(f32.Pt(center.X, center.Y-radius))
	path.LineTo(f32.Pt(center.X+radius, center.Y))
	path.LineTo(f32.Pt(center.X, center.Y+radius))
	path.LineTo(f32.Pt(center.X-radius, center.Y))
	path.Close()
	spec := path.End()
	if !hollow {
		paint.FillShape(ops, fill, clip.Outline{Path: spec}.Op())
	}
	paint.FillShape(ops, rim, clip.Stroke{Path: spec, Width: 1.5}.Op())
}

// EqualizerPreview draws the 48-high response used by Sound and Settings.
func EqualizerPreview(gtx layout.Context, th *Theme, equalizer model.Equalizer, height unit.Dp) layout.Dimensions {
	width := gtx.Constraints.Max.X
	if width <= 0 {
		width = gtx.Dp(180)
	}
	h := gtx.Dp(height)
	if h <= 0 {
		h = gtx.Dp(48)
	}
	size := gtx.Constraints.Constrain(image.Pt(width, h))
	bounds := image.Rect(0, 0, size.X, size.Y)
	area := clip.Rect(bounds).Push(gtx.Ops)
	zero := int(math.Round(float64(gainY(0, size.Y))))
	paint.FillShape(gtx.Ops, th.R.Vein, clip.Rect(image.Rect(0, zero, size.X, zero+max(1, gtx.Dp(1)))).Op())
	curve := eq.Curve(equalizer.Preamp, equalizer.Bands, equalizerFrequencies, nil)
	points := responsePoints(curve, size.X, size.Y, nil)
	ink := th.Accent.Ink
	if !equalizer.Enabled {
		ink = palette.WithAlpha(th.R.Fg3, .6)
	}
	strokePolyline(gtx.Ops, points, float32(max(1, gtx.Dp(2))), ink)
	area.Pop()
	return layout.Dimensions{Size: size}
}

// EQShapeGlyph draws one filter shape as a compact response path.
func EQShapeGlyph(gtx layout.Context, th *Theme, kind model.EQBandType, ink color.NRGBA) layout.Dimensions {
	size := gtx.Constraints.Constrain(image.Pt(gtx.Dp(18), gtx.Dp(18)))
	w, h := float32(size.X), float32(size.Y)
	points := make([]f32.Point, 0, 17)
	for index := range 17 {
		x := float32(index) / 16
		y := float32(.5)
		switch kind {
		case model.EQPeak:
			y = .68 - .42*float32(math.Exp(-math.Pow(float64((x-.5)/.2), 2)))
		case model.EQLowShelf:
			y = .28 + .44*x
		case model.EQHighShelf:
			y = .72 - .44*x
		case model.EQLowPass:
			y = .32 + .5*float32(1/(1+math.Exp(-18*float64(x-.62))))
		case model.EQHighPass:
			y = .82 - .5*float32(1/(1+math.Exp(-18*float64(x-.38))))
		case model.EQNotch:
			y = .35 + .5*float32(math.Exp(-math.Pow(float64((x-.5)/.1), 2)))
		case model.EQBandPass:
			y = .8 - .5*float32(math.Exp(-math.Pow(float64((x-.5)/.18), 2)))
		}
		points = append(points, f32.Pt(x*w, y*h))
	}
	strokePolyline(gtx.Ops, points, float32(max(1, gtx.Dp(1))), ink)
	return layout.Dimensions{Size: size}
}
