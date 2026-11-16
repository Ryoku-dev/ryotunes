package look

import (
	"fmt"
	"image"
	"image/color"
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
	"gioui.org/op/paint"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/anim"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/tokens"
	"github.com/ryoku-dev/ryotunes/internal/eq"
	"github.com/ryoku-dev/ryotunes/internal/model"
)

const graphDoubleClick = 400 * time.Millisecond

// Node geometry, in dp: a resting node's half-diagonal, how much a hovered or
// held node grows, and how far the selection's register mark stands off it.
const (
	nodeRadius     unit.Dp = 6
	nodeGrow               = .3
	nodeRing       unit.Dp = 4
	nodeRingLandAt         = .6
	nodeRim        unit.Dp = 1.5
	graphLine      unit.Dp = 2
)

// chaseEpsilon is the plot fraction (about a fifth of a pixel across a wide
// plot) at which a node's chase counts as landed.
const nodeChaseEpsilon = .0002

// EQGraphResult reports edits that need transport or a context menu.
type EQGraphResult struct {
	Changed, Dragging, Released bool
	Context                     bool
	ContextBand                 int
	DeleteBand                  int
}

// curveCache keeps one equalizer's response and peak until its bands change.
type curveCache struct {
	input model.Equalizer
	bands []model.EQBand
	curve []float64
	peak  float64
	valid bool
}

func (cache *curveCache) get(equalizer model.Equalizer) ([]float64, float64) {
	if cache.valid && curveInputEqual(cache.input, equalizer) {
		return cache.curve, cache.peak
	}
	cache.curve = eq.Curve(equalizer.Preamp, equalizer.Bands, equalizerFrequencies, cache.curve)
	cache.peak = math.Inf(-1)
	for _, value := range cache.curve {
		cache.peak = max(cache.peak, value)
	}
	if math.IsInf(cache.peak, -1) {
		cache.peak = equalizer.Preamp
	}
	cache.bands = append(cache.bands[:0], equalizer.Bands...)
	cache.input = equalizer
	cache.input.Bands = cache.bands
	cache.valid = true
	return cache.curve, cache.peak
}

// EqualizerGraph owns graph focus, selection, precise pointer input and motion.
//
// It is drawn as the Hub draws its charts: an instrument card (EQCard) with a
// faint engraved grid, the 0 dB line a vein hairline, the response a sel line
// over a 45 degree hatch between it and 0 dB. Its nodes are sharp diamonds:
// hovering one grows it and draws its guides to the scales over 120 ms on
// OutCubic (240 out); the selected one is filled sel and carries a register
// mark that lands (fades in while it closes from wide) over 240 ms. A node the
// pointer holds stands exactly under it (R2); one the keys step glides to its
// value through anim.Chase, the curve with it, never overshooting. At rest it
// asks for no frame; only the spectrum, while music plays, moves it.
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

	bypass anim.Tween
	ready  bool
	grows  []anim.Tween
	hovers []anim.Tween
	known  int
	points []f32.Point

	land         anim.Tween
	landSelected int

	chaseX, chaseY anim.Chase
	chaseBand      int
	chaseCount     int
	chaseReady     bool
	following      bool
	chasing        bool
	display        model.Equalizer
	displayBands   []model.EQBand

	peakCurve  curveCache
	drawnCurve curveCache

	hatchOps   op.Ops
	hatchCall  op.CallOp
	hatchKey   hatchKey
	hatchReady bool
}

type hatchKey struct {
	bounds         image.Rectangle
	spacing, width int
	ink            color.NRGBA
}

// Selected returns the selected band index, or -1.
func (graph *EqualizerGraph) Selected() int { return graph.selected }

// Select changes the graph and strip selection.
func (graph *EqualizerGraph) Select(index int) { graph.selected = index }

// Holding reports whether the pointer holds a node: the one Selected returns.
func (graph *EqualizerGraph) Holding() bool { return graph.active }

func (graph *EqualizerGraph) plotBounds(gtx layout.Context, size image.Point) image.Rectangle {
	left, right := gtx.Dp(38), gtx.Dp(13)
	top, bottom := gtx.Dp(16), gtx.Dp(30)
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
				graph.hovered = hit
				result.Changed, result.Released = true, true
			}
		case pointer.Cancel:
			if graph.active && value.PointerID == graph.pointerID {
				graph.active = false
				graph.hovered = -1
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
	return graph.peakCurve.get(equalizer)
}

// syncNodes keeps one grow and one hover tween per band: a band that arrives
// after the first frame grows in from nothing over the notch time.
func (graph *EqualizerGraph) syncNodes(gtx layout.Context, count int, duration time.Duration) {
	if len(graph.grows) < count {
		graph.grows = append(graph.grows, make([]anim.Tween, count-len(graph.grows))...)
	}
	if len(graph.hovers) < count {
		graph.hovers = append(graph.hovers, make([]anim.Tween, count-len(graph.hovers))...)
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
			graph.grows[index] = anim.Tween{Duration: duration, Ease: anim.EaseOutCubic}
			graph.grows[index].Set(gtx, 0)
			graph.grows[index].Set(gtx, 1)
			graph.hovers[index] = anim.Tween{}
		}
	}
	graph.known = count
}

// nodeHover moves band index's hover toward on: in over th.Motion.Quick, out
// over th.Motion.Move, on OutCubic, as the Hub's rows. The tween takes its
// new target under the duration it was running with, so a hover reversed
// mid-way carries on from where it stands.
func (graph *EqualizerGraph) nodeHover(gtx layout.Context, th *Theme, index int, on bool) float32 {
	tween := &graph.hovers[index]
	tween.Ease = anim.EaseOutCubic
	tween.Set(gtx, boolValue(on))
	tween.Duration = th.Motion.Move
	if on {
		tween.Duration = th.Motion.Quick
	}
	return tween.Value(gtx)
}

// displayed is the equalizer as the graph shows it this frame: the selected
// band where its chase stands, the others where they are. A node the keys
// step glides to its value through anim.Chase and keeps gliding after the key
// is let go until it lands; a node the pointer holds stands exactly under it
// (R2), and any other change (a preset, a fader, an import) stands at once.
// It returns the equalizer itself when nothing chases, so a graph at rest
// copies nothing.
func (graph *EqualizerGraph) displayed(gtx layout.Context, equalizer model.Equalizer) model.Equalizer {
	graph.chasing = false
	selected := graph.selected
	if selected < 0 || selected >= len(equalizer.Bands) {
		graph.chaseReady, graph.following = false, false
		return equalizer
	}
	band := equalizer.Bands[selected]
	x, y := frequencyFraction(band.Frequency), gainY(band.Gain, 1)
	graph.chaseX.Epsilon, graph.chaseY.Epsilon = nodeChaseEpsilon, nodeChaseEpsilon
	if graph.keyEditing {
		graph.following = true
	}
	if !graph.chaseReady || !graph.following || graph.active || graph.chaseBand != selected || graph.chaseCount != len(equalizer.Bands) {
		// another node, bands that came and went, a node the pointer holds,
		// or a change no key made: the node stands where its value is
		graph.chaseX.Snap(x)
		graph.chaseY.Snap(y)
		graph.chaseReady, graph.chaseBand, graph.chaseCount = true, selected, len(equalizer.Bands)
		graph.following = graph.keyEditing && !graph.active
		return equalizer
	}
	graph.chaseX.Set(x)
	graph.chaseY.Set(y)
	shownX, shownY := graph.chaseX.Value(gtx), graph.chaseY.Value(gtx)
	if shownX == x && shownY == y {
		graph.following = graph.keyEditing
		return equalizer
	}
	graph.displayBands = append(graph.displayBands[:0], equalizer.Bands...)
	graph.display = equalizer
	graph.display.Bands = graph.displayBands
	shown := &graph.display.Bands[selected]
	shown.Frequency = fractionFrequency(float64(shownX))
	if shown.HasGain() {
		shown.Gain = yGain(shownY, 1)
	}
	graph.chasing = true
	return graph.display
}

// hatch engraves the response area with the Hub chart's 45 degree hatch. The
// lines depend only on the plot and the ink, so they are recorded once into
// the graph's own ops and replayed under the area's clip every frame.
func (graph *EqualizerGraph) hatch(gtx layout.Context, area clip.PathSpec, bounds image.Rectangle, ink color.NRGBA) {
	if ink.A == 0 || bounds.Empty() {
		return
	}
	k := hatchKey{bounds: bounds, spacing: max(1, gtx.Dp(unit.Dp(tokens.SizeChartHatchGap))), width: max(1, gtx.Dp(unit.Dp(tokens.SizeChartHatchLine))), ink: ink}
	if !graph.hatchReady || k != graph.hatchKey {
		graph.hatchOps.Reset()
		record := op.Record(&graph.hatchOps)
		var path clip.Path
		path.Begin(&graph.hatchOps)
		height := bounds.Dy()
		for x := bounds.Min.X - height; x <= bounds.Max.X; x += k.spacing {
			path.MoveTo(f32.Pt(float32(x), float32(bounds.Max.Y)))
			path.LineTo(f32.Pt(float32(x+height), float32(bounds.Min.Y)))
		}
		paint.FillShape(&graph.hatchOps, ink, clip.Stroke{Path: path.End(), Width: float32(k.width)}.Op())
		graph.hatchCall = record.Stop()
		graph.hatchKey, graph.hatchReady = k, true
	}
	stack := clip.Outline{Path: area}.Op().Push(gtx.Ops)
	graph.hatchCall.Add(gtx.Ops)
	stack.Pop()
}

func (graph *EqualizerGraph) draw(gtx layout.Context, th *Theme, equalizer model.Equalizer, bounds image.Rectangle, spectrum []float32, playing bool) {
	plane := eqCardPlane(th)
	if playing {
		drawSpectrum(gtx, th, bounds, spectrum)
	}
	drawGraphGrid(gtx, th, bounds)
	shown := graph.displayed(gtx, equalizer)
	curve, _ := graph.drawnCurve.get(shown)
	graph.points = responsePoints(curve, bounds.Dx(), bounds.Dy(), graph.points)
	for index := range graph.points {
		graph.points[index].X += float32(bounds.Min.X)
		graph.points[index].Y += float32(bounds.Min.Y)
	}
	active := graph.bypass.Value(gtx)
	zero := float32(bounds.Min.Y) + gainY(0, bounds.Dy())
	if len(graph.points) >= 2 {
		// the hatch stays while bypassed, greyed, so the shape still reads
		hatchInk := palette.SRGBAlpha(th.R.Fg, plane, tokens.AlphaHatch*(.45+.55*active))
		graph.hatch(gtx, responseArea(gtx.Ops, graph.points, zero), bounds, hatchInk)
	}
	lineInk := palette.Mix(palette.WithAlpha(th.R.Fg3, .7), th.R.Sel, active)
	strokePolyline(gtx.Ops, graph.points, float32(max(1, gtx.Dp(graphLine))), lineInk)

	graph.syncNodes(gtx, len(shown.Bands), th.Motion.Notch)
	if graph.selected != graph.landSelected {
		graph.landSelected = graph.selected
		graph.land = anim.Tween{Duration: th.Motion.Move, Ease: anim.EaseOutCubic}
		graph.land.Set(gtx, 0)
		graph.land.Set(gtx, 1)
	}
	land := graph.land.Value(gtx)
	hair := max(1, gtx.Dp(unit.Dp(tokens.SizeHairline)))
	rim := float32(gtx.Dp(nodeRim))
	if rim <= 0 {
		rim = 1
	}
	// the guides of the node under the pointer (or held) run to both scales
	for index := range shown.Bands {
		on := index == graph.hovered || (graph.active && index == graph.selected)
		hover := graph.nodeHover(gtx, th, index, on)
		if hover <= 0 {
			continue
		}
		point := graphNodePoint(&shown, index, bounds)
		guide := palette.WithAlpha(th.R.GuideLit(), hover)
		x, y := int(point.X+.5), int(point.Y+.5)
		paint.FillShape(gtx.Ops, guide, clip.Rect(image.Rect(x, y, x+hair, bounds.Max.Y)).Op())
		paint.FillShape(gtx.Ops, guide, clip.Rect(image.Rect(bounds.Min.X, y, x, y+hair)).Op())
	}
	for index, band := range shown.Bands {
		point := graphNodePoint(&shown, index, bounds)
		grow := graph.grows[index].Value(gtx)
		hover := graph.hovers[index].Value(gtx)
		radius := float32(gtx.Dp(nodeRadius)) * grow * (1 + nodeGrow*hover)
		selected := index == graph.selected
		switch {
		case selected:
			// the handle: filled sel, set off from the curve by the plane, and
			// its register mark landing around it
			ring := radius + float32(gtx.Dp(nodeRing))*(1+(1-nodeRingLandAt)*(1-land)*2.5)
			drawDiamond(gtx.Ops, point, ring, color.NRGBA{}, palette.WithAlpha(th.R.GuideLit(), land*grow), float32(hair), true)
			drawDiamond(gtx.Ops, point, radius+rim, plane, color.NRGBA{}, 0, false)
			drawDiamond(gtx.Ops, point, radius, palette.Mix(th.R.Sel, th.R.Fg, tokens.AlphaHoverFill*hover), color.NRGBA{}, 0, false)
		case band.Enabled:
			fill := palette.Mix(plane, th.R.Sel, .35*hover)
			drawDiamond(gtx.Ops, point, radius, fill, lineInk, rim, false)
		default:
			drawDiamond(gtx.Ops, point, radius, plane, th.R.Fg3, rim, false)
		}
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
		graph.landSelected = graph.selected
		graph.land.Set(gtx, 1)
		graph.bypass.Duration, graph.bypass.Ease = th.Motion.Field, anim.EaseOutCubic
		graph.bypass.Set(gtx, boolValue(equalizer.Enabled))
	}
	graph.bypass.Duration, graph.bypass.Ease = th.Motion.Field, anim.EaseOutCubic
	graph.bypass.Set(gtx, boolValue(equalizer.Enabled))
	if graph.selected >= len(equalizer.Bands) {
		graph.selected = len(equalizer.Bands) - 1
	}
	if graph.hovered >= len(equalizer.Bands) {
		graph.hovered = -1
	}
	result := EQGraphResult{ContextBand: -1, DeleteBand: -1}
	graph.updatePointer(gtx, equalizer, bounds, &result)
	graph.updateKeys(gtx, equalizer, &result)

	outer := image.Rectangle{Max: size}
	cut := gtx.Dp(th.Cut.Mid)
	area := clip.Rect(outer).Push(gtx.Ops)
	EQCard(gtx, th, outer, cut)
	graph.draw(gtx, th, *equalizer, bounds, spectrum, playing)
	semantic.Editor.Add(gtx.Ops)
	semantic.DescriptionOp("Equalizer response graph").Add(gtx.Ops)
	event.Op(gtx.Ops, &graph.tag)
	if graph.active {
		pointer.CursorGrabbing.Add(gtx.Ops)
	} else if graph.hovered >= 0 {
		pointer.CursorGrab.Add(gtx.Ops)
	} else {
		pointer.CursorCrosshair.Add(gtx.Ops)
	}
	area.Pop()
	drawGraphLabels(gtx, th, bounds)
	if gtx.Focused(&graph.tag) {
		kit.FocusRing(gtx, th.Theme, outer, cut)
	}
	if graph.hovered >= 0 && graph.hovered < len(equalizer.Bands) {
		band := equalizer.Bands[graph.hovered]
		frequency := fmt.Sprintf("%.0f Hz", band.Frequency)
		if band.Frequency >= 1000 {
			frequency = fmt.Sprintf("%g kHz", math.Round(band.Frequency/100)/10)
		}
		value := fmt.Sprintf("%s · %s dB · Q %.2f", frequency, FormatGain(band.Gain), band.Q)
		// the bubble rides the node, chasing with it while it is held
		shown := equalizer
		if graph.chasing && graph.hovered == graph.selected {
			shown = &graph.display
		}
		point := graphNodePoint(shown, graph.hovered, bounds)
		at := image.Pt(int(point.X+.5), int(point.Y+.5)-gtx.Dp(nodeRadius))
		Bubble(gtx, th, at, value, graph.hovers[graph.hovered].Value(gtx))
	}
	return layout.Dimensions{Size: size}, result
}
