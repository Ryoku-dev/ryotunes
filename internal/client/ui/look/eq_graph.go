package look

import (
	"fmt"
	"image"
	"math"
	"time"

	"gioui.org/f32"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/anim"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
	"github.com/ryoku-dev/ryotunes/internal/eq"
	"github.com/ryoku-dev/ryotunes/internal/model"
)

const graphDoubleClick = 400 * time.Millisecond

// EQGraphResult reports edits that need transport or a context menu.
type EQGraphResult struct {
	Changed, Dragging, Released bool
	Context                     bool
	ContextBand                 int
	DeleteBand                  int
}

// EqualizerGraph owns graph focus, selection, precise pointer input and motion.
type EqualizerGraph struct {
	tag struct{}

	selected  int
	hovered   int
	pointer   image.Point
	active    bool
	pointerID pointer.ID

	lastEmptyTime time.Duration
	lastEmpty     image.Point
	wheelFraction float32
	keyEditing    bool

	bypass     anim.Tween
	ready      bool
	grows      []anim.Tween
	known      int
	points     []f32.Point
	curveInput model.Equalizer
	curve      []float64
	curvePeak  float64
	curveValid bool
}

// Selected returns the selected band index, or -1.
func (graph *EqualizerGraph) Selected() int { return graph.selected }

// Select changes the graph and strip selection.
func (graph *EqualizerGraph) Select(index int) { graph.selected = index }

func (graph *EqualizerGraph) plotBounds(gtx layout.Context, size image.Point) image.Rectangle {
	left, right := gtx.Dp(38), gtx.Dp(13)
	top, bottom := gtx.Dp(16), gtx.Dp(26)
	return image.Rect(left, top, max(left+1, size.X-right), max(top+1, size.Y-bottom))
}

func graphBandGain(equalizer *model.Equalizer, index int) float64 {
	band := equalizer.Bands[index]
	if band.HasGain() {
		return band.Gain
	}
	gain := equalizer.Preamp
	for _, candidate := range equalizer.Bands {
		if candidate.Enabled {
			gain += eq.BandResponse(candidate, band.Frequency)
		}
	}
	return gain
}

func graphNodePoint(equalizer *model.Equalizer, index int, bounds image.Rectangle) f32.Point {
	band := equalizer.Bands[index]
	return f32.Pt(float32(bounds.Min.X)+FrequencyX(band.Frequency, bounds.Dx()), float32(bounds.Min.Y)+gainY(graphBandGain(equalizer, index), bounds.Dy()))
}

func graphHit(equalizer *model.Equalizer, bounds image.Rectangle, position f32.Point, radius float32) int {
	best, distance := -1, radius*radius
	for index := range equalizer.Bands {
		point := graphNodePoint(equalizer, index, bounds)
		dx, dy := point.X-position.X, point.Y-position.Y
		candidate := dx*dx + dy*dy
		if candidate <= distance {
			best, distance = index, candidate
		}
	}
	return best
}

func (graph *EqualizerGraph) applyPointerBand(equalizer *model.Equalizer, bounds image.Rectangle, position f32.Point) {
	if graph.selected < 0 || graph.selected >= len(equalizer.Bands) {
		return
	}
	band := &equalizer.Bands[graph.selected]
	band.Frequency = math.Round(XFrequency(position.X-float32(bounds.Min.X), bounds.Dx()))
	if band.HasGain() {
		band.Gain = math.Round(yGain(position.Y-float32(bounds.Min.Y), bounds.Dy())*10) / 10
	}
	band.Frequency = min(max(band.Frequency, model.EQMinFrequency), model.EQMaxFrequency)
	band.Gain = min(max(band.Gain, model.EQMinGain), model.EQMaxGain)
	equalizer.Preset = ""
}

func (graph *EqualizerGraph) adjustQ(equalizer *model.Equalizer, delta float32) bool {
	if graph.selected < 0 || graph.selected >= len(equalizer.Bands) || delta == 0 {
		return false
	}
	graph.wheelFraction += delta / 150
	whole := int(graph.wheelFraction)
	graph.wheelFraction -= float32(whole)
	if whole == 0 {
		return false
	}
	band := &equalizer.Bands[graph.selected]
	band.Q *= math.Pow(1.1, float64(-whole))
	band.Q = min(max(band.Q, model.EQMinQ), model.EQMaxQ)
	band.Q = math.Round(band.Q*100) / 100
	equalizer.Preset = ""
	return true
}

func (graph *EqualizerGraph) updatePointer(gtx layout.Context, equalizer *model.Equalizer, bounds image.Rectangle, result *EQGraphResult) {
	filter := pointer.Filter{
		Target:  &graph.tag,
		Kinds:   pointer.Enter | pointer.Leave | pointer.Move | pointer.Press | pointer.Drag | pointer.Release | pointer.Cancel | pointer.Scroll,
		ScrollX: pointer.ScrollRange{Min: -100000, Max: 100000},
		ScrollY: pointer.ScrollRange{Min: -100000, Max: 100000},
	}
	for {
		raw, ok := gtx.Event(filter)
		if !ok {
			break
		}
		value := raw.(pointer.Event)
		graph.pointer = value.Position.Round()
		hit := graphHit(equalizer, bounds, value.Position, float32(gtx.Dp(12)))
		switch value.Kind {
		case pointer.Enter, pointer.Move:
			graph.hovered = hit
		case pointer.Leave:
			if !graph.active {
				graph.hovered = -1
			}
		case pointer.Press:
			gtx.Execute(key.FocusCmd{Tag: &graph.tag})
			if value.Buttons.Contain(pointer.ButtonSecondary) && hit >= 0 {
				graph.selected = hit
				result.Context, result.ContextBand = true, hit
				continue
			}
			if !value.Buttons.Contain(pointer.ButtonPrimary) {
				continue
			}
			if hit >= 0 {
				graph.selected, graph.hovered = hit, hit
				graph.active, graph.pointerID = true, value.PointerID
				graph.wheelFraction = 0
				gtx.Execute(pointer.GrabCmd{Tag: &graph.tag, ID: value.PointerID})
				continue
			}
			if len(equalizer.Bands) < model.EQMaxBands && value.Time-graph.lastEmptyTime <= graphDoubleClick && value.Time > graph.lastEmptyTime {
				dx, dy := graph.pointer.X-graph.lastEmpty.X, graph.pointer.Y-graph.lastEmpty.Y
				if dx*dx+dy*dy <= gtx.Dp(8)*gtx.Dp(8) && graph.pointer.In(bounds) {
					frequency := math.Round(XFrequency(value.Position.X-float32(bounds.Min.X), bounds.Dx()))
					gain := math.Round(yGain(value.Position.Y-float32(bounds.Min.Y), bounds.Dy())*10) / 10
					equalizer.Bands = append(equalizer.Bands, model.EQBand{Type: model.EQPeak, Frequency: frequency, Gain: gain, Q: 1, Enabled: true})
					graph.selected = len(equalizer.Bands) - 1
					equalizer.Preset = ""
					result.Changed, result.Released = true, true
					graph.lastEmptyTime = 0
					continue
				}
			}
			graph.lastEmptyTime, graph.lastEmpty = value.Time, graph.pointer
		case pointer.Drag:
			if graph.active && value.PointerID == graph.pointerID {
				graph.applyPointerBand(equalizer, bounds, value.Position)
				result.Changed, result.Dragging = true, true
			}
		case pointer.Release:
			if graph.active && value.PointerID == graph.pointerID {
				graph.applyPointerBand(equalizer, bounds, value.Position)
				graph.active = false
				result.Changed, result.Released = true, true
			}
		case pointer.Cancel:
			if graph.active && value.PointerID == graph.pointerID {
				graph.active = false
				result.Released = true
			}
		case pointer.Scroll:
			if hit < 0 {
				continue
			}
			graph.selected = hit
			delta := value.Scroll.Y
			if delta == 0 {
				delta = value.Scroll.X
			}
			if graph.adjustQ(equalizer, delta) {
				result.Changed, result.Released = true, true
			}
		}
	}
}

func (graph *EqualizerGraph) updateKeys(gtx layout.Context, equalizer *model.Equalizer, result *EQGraphResult) {
	filters := []event.Filter{
		key.FocusFilter{Target: &graph.tag},
		key.Filter{Focus: &graph.tag, Name: key.NameLeftArrow, Optional: key.ModShift},
		key.Filter{Focus: &graph.tag, Name: key.NameRightArrow, Optional: key.ModShift},
		key.Filter{Focus: &graph.tag, Name: key.NameUpArrow, Optional: key.ModShift},
		key.Filter{Focus: &graph.tag, Name: key.NameDownArrow, Optional: key.ModShift},
		key.Filter{Focus: &graph.tag, Name: key.Name("[")},
		key.Filter{Focus: &graph.tag, Name: key.Name("]")},
		key.Filter{Focus: &graph.tag, Name: key.NameDeleteForward},
		key.Filter{Focus: &graph.tag, Name: key.NameDeleteBackward},
		key.Filter{Focus: &graph.tag, Name: key.NameTab, Optional: key.ModShift},
	}
	for {
		raw, ok := gtx.Event(filters...)
		if !ok {
			break
		}
		value, ok := raw.(key.Event)
		if !ok {
			continue
		}
		if value.State == key.Release {
			if graph.keyEditing {
				graph.keyEditing = false
				result.Released = true
			}
			continue
		}
		if value.State != key.Press || len(equalizer.Bands) == 0 {
			continue
		}
		if graph.selected < 0 || graph.selected >= len(equalizer.Bands) {
			graph.selected = 0
		}
		band := &equalizer.Bands[graph.selected]
		changed := true
		switch value.Name {
		case key.NameLeftArrow:
			band.Frequency /= math.Pow(2, 1.0/6)
		case key.NameRightArrow:
			band.Frequency *= math.Pow(2, 1.0/6)
		case key.NameUpArrow:
			step := .5
			if value.Modifiers.Contain(key.ModShift) {
				step = .1
			}
			if band.HasGain() {
				band.Gain += step
			}
		case key.NameDownArrow:
			step := .5
			if value.Modifiers.Contain(key.ModShift) {
				step = .1
			}
			if band.HasGain() {
				band.Gain -= step
			}
		case key.Name("["):
			band.Q /= 1.1
		case key.Name("]"):
			band.Q *= 1.1
		case key.NameDeleteForward, key.NameDeleteBackward:
			result.DeleteBand = graph.selected
			changed = false
		case key.NameTab:
			direction := 1
			if value.Modifiers.Contain(key.ModShift) {
				direction = -1
			}
			graph.selected = (graph.selected + direction + len(equalizer.Bands)) % len(equalizer.Bands)
			changed = false
		default:
			changed = false
		}
		if changed {
			if graph.selected >= 0 && graph.selected < len(equalizer.Bands) {
				band = &equalizer.Bands[graph.selected]
				band.Frequency = min(max(band.Frequency, model.EQMinFrequency), model.EQMaxFrequency)
				band.Gain = math.Round(min(max(band.Gain, model.EQMinGain), model.EQMaxGain)*10) / 10
				band.Q = math.Round(min(max(band.Q, model.EQMinQ), model.EQMaxQ)*100) / 100
			}
			equalizer.Preset = ""
			graph.keyEditing = true
			result.Changed, result.Dragging = true, true
		}
	}
}

func curveInputEqual(first, second model.Equalizer) bool {
	if first.Preamp != second.Preamp || len(first.Bands) != len(second.Bands) {
		return false
	}
	for index := range first.Bands {
		if first.Bands[index] != second.Bands[index] {
			return false
		}
	}
	return true
}

// Curve returns the cached response and peak for the current local equalizer.
func (graph *EqualizerGraph) Curve(equalizer model.Equalizer) ([]float64, float64) {
	if graph.curveValid && curveInputEqual(graph.curveInput, equalizer) {
		return graph.curve, graph.curvePeak
	}
	graph.curve = eq.Curve(equalizer.Preamp, equalizer.Bands, equalizerFrequencies, graph.curve)
	graph.curvePeak = math.Inf(-1)
	for _, value := range graph.curve {
		graph.curvePeak = max(graph.curvePeak, value)
	}
	if math.IsInf(graph.curvePeak, -1) {
		graph.curvePeak = equalizer.Preamp
	}
	graph.curveInput = equalizer
	graph.curveInput.Bands = append([]model.EQBand(nil), equalizer.Bands...)
	graph.curveValid = true
	return graph.curve, graph.curvePeak
}

func (graph *EqualizerGraph) syncGrowth(gtx layout.Context, count int, duration time.Duration) {
	if len(graph.grows) < count {
		graph.grows = append(graph.grows, make([]anim.Tween, count-len(graph.grows))...)
	}
	if !graph.ready {
		for index := range count {
			graph.grows[index].Set(gtx, 1)
		}
		graph.known, graph.ready = count, true
		return
	}
	if count > graph.known {
		for index := graph.known; index < count; index++ {
			graph.grows[index].Duration = duration
			graph.grows[index].Ease = anim.EaseChisel
			graph.grows[index].Set(gtx, 0)
			graph.grows[index].Set(gtx, 1)
		}
	}
	graph.known = count
}

func (graph *EqualizerGraph) draw(gtx layout.Context, th *Theme, equalizer model.Equalizer, bounds image.Rectangle, spectrum []float32, playing bool) {
	if playing {
		drawSpectrum(gtx, th, bounds, spectrum)
	}
	drawGraphGrid(gtx, th, bounds)
	curve, _ := graph.Curve(equalizer)
	graph.points = responsePoints(curve, bounds.Dx(), bounds.Dy(), graph.points)
	for index := range graph.points {
		graph.points[index].X += float32(bounds.Min.X)
		graph.points[index].Y += float32(bounds.Min.Y)
	}
	active := graph.bypass.Value(gtx)
	lineInk := palette.Mix(palette.WithAlpha(th.R.Fg3, .6), th.Accent.Ink, active)
	zero := float32(bounds.Min.Y) + gainY(0, bounds.Dy())
	fillResponse(gtx.Ops, graph.points, zero, palette.WithAlpha(th.Accent.Ink, .18*active))
	strokePolyline(gtx.Ops, graph.points, float32(max(1, gtx.Dp(2))), lineInk)

	graph.syncGrowth(gtx, len(equalizer.Bands), th.Motion.Notch)
	for index, band := range equalizer.Bands {
		point := graphNodePoint(&equalizer, index, bounds)
		scale := graph.grows[index].Value(gtx)
		radius := float32(gtx.Dp(6)) * scale
		fill, rim, hollow := th.Accent.Fill, th.R.Stone1, !band.Enabled
		if index == graph.selected {
			fill, rim, hollow = th.R.Sel, th.R.OnSel, false
		} else if hollow {
			fill, rim = th.R.Stone1, th.R.Fg3
		}
		drawDiamond(gtx.Ops, point, radius, fill, rim, hollow)
	}
	if !equalizer.Enabled {
		labelGTX := gtx
		labelGTX.Constraints.Min = image.Point{}
		labelGTX.Constraints.Max = image.Pt(gtx.Dp(96), gtx.Dp(22))
		offset := op.Offset(image.Pt(bounds.Max.X-gtx.Dp(94), bounds.Min.Y+gtx.Dp(8))).Push(gtx.Ops)
		layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(labelGTX,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return kit.Flake(gtx, th.Theme, palette.Paused, 1, unit.Dp(8))
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
			layout.Rigid(kit.Text(th.Theme, th.T.Caption, th.R.Fg3, "Bypassed")),
		)
		offset.Pop()
	}
}

// Layout draws and operates the response graph. It edits only the caller's local copy.
func (graph *EqualizerGraph) Layout(gtx layout.Context, th *Theme, equalizer *model.Equalizer, spectrum []float32, playing bool, height unit.Dp) (layout.Dimensions, EQGraphResult) {
	size := gtx.Constraints.Constrain(image.Pt(gtx.Constraints.Max.X, gtx.Dp(height)))
	bounds := graph.plotBounds(gtx, size)
	if !graph.ready {
		graph.hovered = -1
		graph.selected = -1
		if len(equalizer.Bands) > 0 {
			graph.selected = 0
		}
		graph.bypass.Duration, graph.bypass.Ease = th.Motion.Field, anim.EaseChisel
		if equalizer.Enabled {
			graph.bypass.Set(gtx, 1)
		} else {
			graph.bypass.Set(gtx, 0)
		}
	}
	if equalizer.Enabled {
		graph.bypass.Set(gtx, 1)
	} else {
		graph.bypass.Set(gtx, 0)
	}
	if graph.selected >= len(equalizer.Bands) {
		graph.selected = len(equalizer.Bands) - 1
	}
	result := EQGraphResult{ContextBand: -1, DeleteBand: -1}
	graph.updatePointer(gtx, equalizer, bounds, &result)
	graph.updateKeys(gtx, equalizer, &result)

	outer := image.Rectangle{Max: size}
	area := clip.Rect(outer).Push(gtx.Ops)
	kit.FillCut(gtx.Ops, outer, gtx.Dp(6), th.R.Stone1)
	graph.draw(gtx, th, *equalizer, bounds, spectrum, playing)
	semantic.Editor.Add(gtx.Ops)
	semantic.DescriptionOp("Equalizer response graph").Add(gtx.Ops)
	event.Op(gtx.Ops, &graph.tag)
	if graph.active {
		pointer.CursorGrabbing.Add(gtx.Ops)
	} else {
		pointer.CursorCrosshair.Add(gtx.Ops)
	}
	area.Pop()
	drawGraphLabels(gtx, th, bounds)
	if gtx.Focused(&graph.tag) {
		kit.FocusRing(gtx, th.Theme, outer, gtx.Dp(6))
	}
	if graph.hovered >= 0 && graph.hovered < len(equalizer.Bands) {
		band := equalizer.Bands[graph.hovered]
		frequency := fmt.Sprintf("%.0f Hz", band.Frequency)
		if band.Frequency >= 1000 {
			frequency = fmt.Sprintf("%g kHz", math.Round(band.Frequency/100)/10)
		}
		value := fmt.Sprintf("%s · %s dB · Q %.2f", frequency, FormatGain(band.Gain), band.Q)
		Bubble(gtx, th, graph.pointer, value, 1)
	}
	return layout.Dimensions{Size: size}, result
}
