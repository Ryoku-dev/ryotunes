package equalizer

import (
	"image"
	"image/color"
	"slices"
	"strings"

	"gioui.org/f32"
	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/anim"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/sheet"
	"github.com/ryoku-dev/ryotunes/internal/eq"
	"github.com/ryoku-dev/ryotunes/internal/model"
)

const savePresetCommand = -1

var shapeTypes = []model.EQBandType{
	model.EQPeak, model.EQLowShelf, model.EQHighShelf, model.EQLowPass,
	model.EQHighPass, model.EQNotch, model.EQBandPass,
}

var shapeLabels = []string{"Bell", "Low shelf", "High shelf", "Low pass", "High pass", "Notch", "Band pass"}

// The sheet's geometry, from the Hub's settings page (SettingsPage.qml,
// PageHeader.qml), in dp: the page's top padding under the masthead, the
// column's measure, the title (sentence at 32/40), the lead's measure, the
// space under the head, the spacing of the head's plaques, and the room at
// the page's foot.
const (
	pageTop      unit.Dp = 8
	pageMeasure  unit.Dp = 960
	pageTitle            = 32
	pageTitleLn          = 40
	leadMeasure  unit.Dp = 720
	headBelow    unit.Dp = 26
	plaqueGap    unit.Dp = 6
	pageFoot     unit.Dp = 34
	cardInset    unit.Dp = 13
	columnCut    unit.Dp = 6
	shapeCut     unit.Dp = 5
	headroomGap  unit.Dp = 13
	equalizerKan         = "調音"
)

// The sections of the sheet, numbered as the Hub numbers a page's parts.
var (
	responseMark = kit.Ordinal(1, 2)
	bandsMark    = kit.Ordinal(2, 2)
)

const pageLead = "Shape everything Ryotunes plays. Drag a node or a fader, and double-click the curve to add a band."

// Layout draws the complete equalizer page and its deferred dialogs.
func (p *Page) Layout(gtx layout.Context, e *env.Env) layout.Dimensions {
	p.finishRemoval(gtx, e)
	p.reconcile(e)
	p.loadPresets(e)
	p.advancePreset(gtx, e)
	p.flushDrag(gtx, e)
	p.ensureBandControls()
	p.handleTitleActions(gtx, e)
	if p.contextBand >= 0 {
		p.openShapeMenu(e, p.contextBand, e.Overlays.Press())
		p.contextBand = -1
	}

	p.graphHeight = unit.Dp(float32(gtx.Constraints.Max.Y) * .32 / gtx.Metric.PxPerDp)
	p.graphHeight = min(max(p.graphHeight, unit.Dp(184)), unit.Dp(320))
	inset := look.ContentInset(e.Th.Class)
	dimensions := p.scroll.Layout(gtx, e.Th, 1, func(gtx layout.Context, _ int) layout.Dimensions {
		return layout.Inset{Top: pageTop, Right: inset, Bottom: pageFoot, Left: inset}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			// the column is centred at the Hub's measure
			width := min(gtx.Constraints.Max.X, gtx.Dp(pageMeasure))
			left := (gtx.Constraints.Max.X - width) / 2
			gtx.Constraints.Min.X, gtx.Constraints.Max.X = width, width
			offset := op.Offset(image.Pt(left, 0)).Push(gtx.Ops)
			dimensions := layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return p.reveal.Group(gtx, e.Th, 0, func(gtx layout.Context) layout.Dimensions { return p.layoutHead(gtx, e) })
				}),
				layout.Rigid(layout.Spacer{Height: headBelow}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return p.reveal.Group(gtx, e.Th, 1, func(gtx layout.Context) layout.Dimensions {
						return kit.SectionStyle{Title: "Response", Ruled: true, Mark: responseMark}.Layout(gtx, e.Th.Theme, func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions { return p.layoutGraph(gtx, e) }),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions { return p.layoutHeadroom(gtx, e) }),
							)
						})
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return p.reveal.Group(gtx, e.Th, 2, func(gtx layout.Context) layout.Dimensions {
						return kit.SectionStyle{Title: "Bands", Ruled: true, Mark: bandsMark}.Layout(gtx, e.Th.Theme, func(gtx layout.Context) layout.Dimensions {
							return p.layoutStripCard(gtx, e)
						})
					})
				}),
			)
			offset.Pop()
			dimensions.Size.X += left
			return dimensions
		})
	})
	p.saveDialog.Layout(gtx, e, p)
	p.deleteDialog.Layout(gtx, e, p)
	p.importDialog.Layout(gtx, e, p)
	return dimensions
}

func (p *Page) handleTitleActions(gtx layout.Context, e *env.Env) {
	if p.back.Clicked(gtx) {
		p.Leave(e)
	}
	if p.presetButton.Clicked(gtx) {
		p.openPresetMenu(gtx, e)
	}
	if p.importButton.Clicked(gtx) {
		p.importDialog.Open()
	}
	if p.exportButton.Clicked(gtx) {
		act.CopyText(e, eq.Format(p.working), "Copied the profile")
	}
	if p.reset.Clicked(gtx) {
		before, target := p.settledEqualizer(), p.resetTarget()
		p.animateTo(gtx, e, target)
		p.offerUndo(e, "Reset to a flat curve.", equalizerChange{before: before, after: target, removed: -1})
	}
	if p.lower.Clicked(gtx) {
		_, peak := p.graph.Curve(p.working)
		if peak > 0 {
			p.lowerPreamp(gtx, e, peak)
		}
	}
	if p.add.Clicked(gtx) {
		p.addBand(gtx, e)
	}
}

// layoutHead draws the page head as the Hub's PageHeader: the title in
// sentence at 32/40 with the page's plaques at the line's end, the lead in
// body fg2, the spec strip 16 below, and the tategaki at the right while
// glosses are on.
func (p *Page) layoutHead(gtx layout.Context, e *env.Env) layout.Dimensions {
	words := func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return p.layoutTitle(gtx, e) }),
			layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(leadMeasure))
				gtx.Constraints.Min.X = 0
				return kit.Label{Style: e.Th.T.Body, Color: e.Th.R.Fg2, MaxLines: 3}.Layout(gtx, e.Th.Theme, pageLead)
			}),
			layout.Rigid(layout.Spacer{Height: sheet.SpecStripAbove}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return sheet.SpecStrip(gtx, e.Th, p.specStrip())
			}),
		)
	}
	if !e.Gloss() {
		return words(gtx)
	}
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		layout.Flexed(1, words),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			width := gtx.Dp(sheet.TategakiRoom)
			gtx.Constraints.Min.X, gtx.Constraints.Max.X = width, width
			return layout.E.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return p.tategaki.Layout(gtx, e.Th, equalizerKan, equalizerKan)
			})
		}),
	)
}

// updateFigures rebuilds the spec strip and the headroom words when what
// they read has changed, so a page at rest formats nothing.
func (p *Page) updateFigures() {
	_, peak := p.graph.Curve(p.working)
	now := figures{ready: true, enabled: p.working.Enabled, preset: presetName(p.working), bands: len(p.working.Bands), preamp: p.working.Preamp, peak: peak}
	if now == p.figures {
		return
	}
	p.figures = now
	state := "Bypassed"
	if now.enabled {
		state = "On"
	}
	p.specs = append(p.specs[:0],
		sheet.Spec{Label: "State", Value: state, Lit: now.enabled},
		sheet.Spec{Label: "Preset", Value: now.preset},
		sheet.Spec{Label: "Bands", Value: kit.Pad2(now.bands)},
		sheet.Spec{Label: "Preamp", Value: look.FormatGain(now.preamp) + " dB"},
		sheet.Spec{Label: "Peak", Value: look.FormatGain(now.peak) + " dB", Lit: now.enabled && now.peak > .05},
	)
	p.headroomText = "Peaks reach " + look.FormatGain(now.peak) + " dB, so loud passages may clip."
}

func (p *Page) specStrip() []sheet.Spec {
	p.updateFigures()
	return p.specs
}

func (p *Page) layoutTitle(gtx layout.Context, e *env.Env) layout.Dimensions {
	p.toggle.Value = p.working.Enabled
	if p.working.Enabled {
		p.toggle.Label = "On"
	} else {
		p.toggle.Label = "Bypassed"
	}
	p.presetButton.Kind, p.presetButton.Small = kit.PlaqueLine, true
	p.presetButton.Icon, p.presetButton.Label = look.Icon.ChevronDown, presetName(p.working)
	p.importButton.Kind, p.importButton.Small, p.importButton.Label = kit.PlaqueQuiet, true, "Import"
	p.exportButton.Kind, p.exportButton.Small, p.exportButton.Label = kit.PlaqueQuiet, true, "Export"
	p.reset.Kind, p.reset.Small, p.reset.Icon, p.reset.Label = kit.PlaqueQuiet, true, look.Icon.Refresh, "Reset"

	gap := layout.Rigid(layout.Spacer{Width: plaqueGap}.Layout)
	controls := func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceEnd}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return p.toggle.Layout(gtx, e.Th.Theme) }),
			layout.Rigid(layout.Spacer{Width: unit.Dp(13)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return p.presetButton.Layout(gtx, e.Th.Theme) }),
			gap,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return p.importButton.Layout(gtx, e.Th.Theme) }),
			gap,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return p.exportButton.Layout(gtx, e.Th.Theme) }),
			gap,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return p.reset.Layout(gtx, e.Th.Theme) }),
		)
	}
	p.back.Kind, p.back.Small, p.back.Icon, p.back.Description = kit.PlaqueQuiet, true, look.Icon.Back, "Back"
	titleStyle := e.Th.T.Sentence
	titleStyle.Size, titleStyle.Line = pageTitle, pageTitleLn
	title := func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return p.back.Layout(gtx, e.Th.Theme) }),
			layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
			layout.Rigid(kit.Text(e.Th.Theme, titleStyle, e.Th.R.Fg, "Equalizer")),
		)
	}
	var dimensions layout.Dimensions
	if gtx.Constraints.Max.X >= gtx.Dp(760) {
		dimensions = layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Flexed(1, title),
			layout.Rigid(controls),
		)
	} else {
		dimensions = layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(title),
			layout.Rigid(layout.Spacer{Height: unit.Dp(13)}.Layout),
			layout.Rigid(controls),
		)
	}
	if p.toggle.Changed() {
		p.working.Enabled = p.toggle.Value
		p.markEdit(gtx, e, false, true)
	}
	return dimensions
}

func (p *Page) layoutGraph(gtx layout.Context, e *env.Env) layout.Dimensions {
	playing := false
	var levels []float32
	if snapshot := e.Store.Snapshot(); snapshot.Playback.Now != nil && !snapshot.Playback.Paused && e.Spectrum != nil {
		playing = true
		e.Spectrum.Want(gtx.Now)
		levels, _ = e.Spectrum.Levels(gtx.Now)
	}
	dimensions, result := p.graph.Layout(gtx, e.Th, &p.working, levels, playing, p.graphHeight)
	if result.Context && result.ContextBand >= 0 {
		p.contextBand = result.ContextBand
		gtx.Execute(op.InvalidateCmd{})
	}
	if result.DeleteBand >= 0 {
		p.removeBand(gtx, e, result.DeleteBand)
	}
	if result.Changed {
		p.markEdit(gtx, e, result.Dragging, result.Released)
	} else if result.Released && p.dragging {
		p.markEdit(gtx, e, false, true)
	}
	return dimensions
}

// layoutStripCard lays the band strip in an instrument card.
func (p *Page) layoutStripCard(gtx layout.Context, e *env.Env) layout.Dimensions {
	gtx.Constraints.Min.Y = 0
	record := op.Record(gtx.Ops)
	dimensions := layout.UniformInset(cardInset).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return p.layoutStrip(gtx, e)
	})
	call := record.Stop()
	dimensions.Size.X = gtx.Constraints.Max.X
	look.EQCard(gtx, e.Th, image.Rectangle{Max: dimensions.Size}, gtx.Dp(e.Th.Cut.Mid))
	call.Add(gtx.Ops)
	return dimensions
}

func (p *Page) layoutStrip(gtx layout.Context, e *env.Env) layout.Dimensions {
	if p.stripPrevious.Clicked(gtx) && p.strip.CanPage(-1) {
		p.strip.Page(gtx, -1)
	}
	if p.stripNext.Clicked(gtx) && p.strip.CanPage(1) {
		p.strip.Page(gtx, 1)
	}
	// the selection wash travels a pitch per band, as the Hub's mark moves
	// between lines: 240 ms on OutCubic
	selected := p.graph.Selected()
	p.stripSelection.Duration = e.Th.Motion.Move
	p.stripSelection.Ease = anim.EaseOutCubic
	if selected >= 0 {
		if !p.stripSelectionReady {
			p.stripSelection = anim.Tween{Duration: e.Th.Motion.Move, Ease: anim.EaseOutCubic}
			p.stripSelectionReady = true
		}
		p.stripSelection.Set(gtx, float32(selected))
	}
	selectionAt := p.stripSelection.Value(gtx)
	columns := p.unfoldBands(gtx, e)

	// the add tile is measured first so the bands spread over exactly what it leaves,
	// and it stays in reach while they scroll
	addGTX := gtx
	addGTX.Constraints.Min = image.Point{}
	addMacro := op.Record(gtx.Ops)
	add := p.layoutAdd(addGTX, e)
	addCall := addMacro.Stop()

	stripGTX := gtx
	stripGTX.Constraints.Min.X = 0
	stripGTX.Constraints.Max.X = max(0, gtx.Constraints.Max.X-add.Size.X)
	strip := p.layoutStripScroller(stripGTX, e, len(p.working.Bands)+1, columns, func(gtx layout.Context, index int) layout.Dimensions {
		if index == 0 {
			return p.layoutPreamp(gtx, e)
		}
		return p.layoutBand(gtx, e, index-1, selectionAt)
	})
	height := max(strip.Size.Y, add.Size.Y)
	offset := op.Offset(image.Pt(strip.Size.X, (height-add.Size.Y)/2)).Push(gtx.Ops)
	addCall.Add(gtx.Ops)
	offset.Pop()
	return layout.Dimensions{Size: image.Pt(strip.Size.X+add.Size.X, height)}
}

func (p *Page) layoutStripScroller(gtx layout.Context, e *env.Env, count int, columns float32, element layout.ListElement) layout.Dimensions {
	button := gtx.Dp(unit.Dp(32))
	gap := gtx.Dp(unit.Dp(8))
	width := gtx.Constraints.Max.X
	gutter := 0
	showSteps := p.stripSteps
	if showSteps {
		gutter = button + gap
	}
	viewport := max(0, width-2*gutter)
	p.stripPitch = stripPitch(gtx.Metric, viewport, columns)

	stripGTX := gtx
	stripGTX.Constraints.Min.X, stripGTX.Constraints.Max.X = 0, viewport
	stripGTX.Constraints.Min.Y = 0
	stripOffset := op.Offset(image.Pt(gutter, 0)).Push(gtx.Ops)
	stripDimensions := p.strip.Layout(stripGTX, e.Th, count, element)
	stripOffset.Pop()
	overflow := p.strip.Position.Length > width
	if overflow != p.stripSteps {
		p.stripSteps = overflow
		gtx.Execute(op.InvalidateCmd{})
	}
	if !showSteps {
		return stripDimensions
	}

	// R8: the step plaques at the strip's ends say there is more
	height := max(button, stripDimensions.Size.Y)
	p.stripPrevious.Kind, p.stripPrevious.Small = kit.PlaqueQuiet, true
	p.stripPrevious.Icon, p.stripPrevious.Description = look.Icon.Back, "Previous equalizer bands"
	p.stripNext.Kind, p.stripNext.Small = kit.PlaqueQuiet, true
	p.stripNext.Icon, p.stripNext.Description = look.Icon.Forward, "Next equalizer bands"
	layoutStep := func(x int, plaque *kit.IconPlaque, enabled bool) {
		buttonGTX := gtx
		buttonGTX.Constraints = layout.Exact(image.Pt(button, button))
		if !enabled {
			buttonGTX = buttonGTX.Disabled()
		}
		offset := op.Offset(image.Pt(x, (height-button)/2)).Push(gtx.Ops)
		plaque.Layout(buttonGTX, e.Th.Theme)
		offset.Pop()
	}
	layoutStep(0, &p.stripPrevious, p.strip.CanPage(-1))
	layoutStep(width-button, &p.stripNext, p.strip.CanPage(1))
	return layout.Dimensions{Size: gtx.Constraints.Constrain(image.Pt(width, height))}
}

// A band's controls keep this width however far apart the columns spread,
// so a wide strip spaces the faders out instead of stretching them.
const (
	bandColumn   = unit.Dp(56)
	bandPitch    = unit.Dp(60)
	preampColumn = unit.Dp(64)
	preampGap    = unit.Dp(13)
)

// stripPitch spreads columns band columns over the strip after the preamp.
// Past three natural pitches the faders stop reading as one instrument, so the
// rest stays free; below one the strip scrolls instead.
func stripPitch(metric unit.Metric, viewport int, columns float32) int {
	natural := metric.Dp(bandPitch)
	if columns <= 0 {
		return natural
	}
	spread := int(float32(viewport-metric.Dp(preampColumn)-metric.Dp(preampGap)) / columns)
	return min(max(spread, natural), 3*natural)
}

// unfoldBands advances every band column's unfold and returns their sum, the
// number of columns the strip spreads over this frame, so the others narrow
// smoothly while one unfolds: the Hub's unfold, 260 ms on OutCubic.
func (p *Page) unfoldBands(gtx layout.Context, e *env.Env) float32 {
	p.ensureBandControls()
	p.stripUnfold = slices.Grow(p.stripUnfold[:0], len(p.working.Bands))[:len(p.working.Bands)]
	var columns float32
	for index := range p.working.Bands {
		control := &p.bands[index]
		control.unfold.Duration = e.Th.Motion.HubUnfold
		control.unfold.Ease = anim.EaseOutCubic
		if !control.ready {
			if control.existing {
				control.unfold.Set(gtx, 1)
			} else {
				control.unfold.Set(gtx, 0)
				control.unfold.Set(gtx, 1)
			}
			control.ready = true
		}
		progress := control.unfold.Value(gtx)
		if index == p.removing && progress <= .001 {
			p.removeComplete = true
			gtx.Execute(op.InvalidateCmd{})
			progress = 0
		}
		p.stripUnfold[index] = progress
		columns += progress
	}
	return columns
}

// stripColumn lays a band's controls in its pitch over its washes: the
// column's own hover and press wash (the Hub's row wash, cut 6), and its part
// of the one selection wash that glides a pitch per band, so the wash moves
// across the strip as a single mark even while the strip scrolls.
func stripColumn(gtx layout.Context, pitch, index int, selectionAt float32, th *look.Theme, wash *look.RowWash, body layout.Widget) layout.Dimensions {
	content := gtx.Dp(bandColumn)
	pitch = max(pitch, content)
	left := (pitch - content) / 2
	bodyGTX := gtx
	bodyGTX.Constraints.Min.X, bodyGTX.Constraints.Max.X = content, content
	macro := op.Record(gtx.Ops)
	dimensions := body(bodyGTX)
	call := macro.Stop()
	size := image.Pt(pitch, dimensions.Size.Y)
	area := clip.Rect(image.Rectangle{Max: size}).Push(gtx.Ops)
	cut := gtx.Dp(columnCut)
	column := image.Rect(left, 0, left+content, size.Y)
	wash.Paint(gtx, th, column, cut)
	x := left + int((selectionAt-float32(index))*float32(pitch))
	if selected := image.Rect(x, 0, x+content, size.Y); selected.Overlaps(image.Rectangle{Max: size}) {
		kit.Fill(gtx, th.Theme, selected, cut, th.R.Select())
	}
	offset := op.Offset(image.Pt(left, 0)).Push(gtx.Ops)
	call.Add(gtx.Ops)
	offset.Pop()
	area.Pop()
	return layout.Dimensions{Size: size}
}

func centered(w layout.Widget) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions { return layout.Center.Layout(gtx, w) }
}

func stripFaderHeight(class look.Class) unit.Dp {
	switch class {
	case look.Compact:
		return 76
	case look.Medium:
		return 88
	default:
		return 104
	}
}

func (p *Page) layoutPreamp(gtx layout.Context, e *env.Env) layout.Dimensions {
	preamp := p.working.Preamp
	if p.presetAnimating {
		preamp = p.presetTarget.Preamp
	}
	dimensions := layout.Inset{Right: preampGap}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		width := gtx.Dp(preampColumn)
		gtx.Constraints.Min.X, gtx.Constraints.Max.X = width, width
		return layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(centered(func(gtx layout.Context) layout.Dimensions {
					return p.preampRoll.Layout(gtx, e.Th, e.Th.T.MonoSm, e.Th.R.Fg2, preamp, look.FormatGain, p.preamp.Held())
				})),
				layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return p.preamp.Layout(gtx, e.Th, p.working.Preamp, look.FaderStyle{Min: model.EQMinPreamp, Max: model.EQMaxPreamp, Reset: 0, Step: .5, Height: stripFaderHeight(e.Th.Class), Label: "Preamp"})
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
				layout.Rigid(centered(kit.SpacedCaps(e.Th.Theme, e.Th.T.MonoSm, e.Th.R.Fg3, 1.2, "Pre"))),
			)
		})
	})
	x := dimensions.Size.X - gtx.Dp(7)
	paint.FillShape(gtx.Ops, e.Th.R.Vein, clip.Rect(image.Rect(x, 0, x+max(1, gtx.Dp(1)), dimensions.Size.Y)).Op())
	p.consumePreamp(gtx, e)
	return dimensions
}

func (p *Page) consumePreamp(gtx layout.Context, e *env.Env) {
	if value, dragging := p.preamp.Dragging(); dragging && value != p.working.Preamp {
		p.working.Preamp = value
		p.markEdit(gtx, e, true, false)
	}
	if value, ok := p.preamp.Committed(); ok {
		changed := value != p.working.Preamp
		p.working.Preamp = value
		if changed || p.dragging {
			p.markEdit(gtx, e, false, true)
		}
	}
}

func revealHorizontal(gtx layout.Context, progress float32, body layout.Widget) layout.Dimensions {
	progress = min(max(progress, 0), 1)
	macro := op.Record(gtx.Ops)
	dimensions := body(gtx)
	call := macro.Stop()
	dimensions.Size.X = int(float32(dimensions.Size.X) * progress)
	area := clip.Rect(image.Rectangle{Max: dimensions.Size}).Push(gtx.Ops)
	call.Add(gtx.Ops)
	area.Pop()
	return dimensions
}

// shownBand is the band a column's readouts show: while a preset moves in,
// the value it lands on, so each figure rolls once; otherwise the band.
func (p *Page) shownBand(index int) model.EQBand {
	if p.presetAnimating && index < len(p.presetTarget.Bands) {
		return p.presetTarget.Bands[index]
	}
	return p.working.Bands[index]
}

// columnInput reads the column's hover and press. Its area passes every
// event on to the controls under it, and a press anywhere in the column
// selects its band.
func (p *Page) columnInput(gtx layout.Context, index int, control *bandControl) {
	for {
		raw, ok := gtx.Event(pointer.Filter{Target: &control.hoverTag, Kinds: pointer.Enter | pointer.Leave | pointer.Press | pointer.Release | pointer.Cancel})
		if !ok {
			break
		}
		switch raw.(pointer.Event).Kind {
		case pointer.Enter:
			control.hovered = true
		case pointer.Leave:
			control.hovered = false
		case pointer.Press:
			control.pressed = true
			p.graph.Select(index)
		case pointer.Release, pointer.Cancel:
			control.pressed = false
		}
	}
}

func (p *Page) layoutBand(gtx layout.Context, e *env.Env, index int, selectionAt float32) layout.Dimensions {
	if index < 0 || index >= len(p.working.Bands) || index >= len(p.stripUnfold) {
		return layout.Dimensions{}
	}
	progress := p.stripUnfold[index]
	if index == p.removing && progress <= .001 {
		return layout.Dimensions{}
	}
	control := &p.bands[index]
	band := &p.working.Bands[index]
	shown := p.shownBand(index)
	selected := index == p.graph.Selected()
	held := selected && p.graph.Holding()
	p.columnInput(gtx, index, control)
	control.wash.Update(gtx, e.Th, control.hovered, control.pressed)
	return revealHorizontal(gtx, progress, func(gtx layout.Context) layout.Dimensions {
		dimensions := stripColumn(gtx, p.stripPitch, index, selectionAt, e.Th, &control.wash, func(gtx layout.Context) layout.Dimensions {
			ink := e.Th.R.Fg2
			if selected {
				ink = e.Th.R.Fg
			}
			return layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				dimensions := layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(centered(func(gtx layout.Context) layout.Dimensions {
						if !band.HasGain() {
							return kit.Text(e.Th.Theme, e.Th.T.MonoSm, ink, " ")(gtx)
						}
						return control.gainRoll.Layout(gtx, e.Th, e.Th.T.MonoSm, ink, shown.Gain, look.FormatGain, held || control.fader.Held())
					})),
					layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return control.fader.Layout(gtx, e.Th, band.Gain, look.FaderStyle{Min: model.EQMinGain, Max: model.EQMaxGain, Reset: 0, Step: .5, Height: stripFaderHeight(e.Th.Class), Label: "Band gain", Disabled: !band.HasGain()})
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
					layout.Rigid(centered(func(gtx layout.Context) layout.Dimensions {
						return control.freqRoll.Layout(gtx, e.Th, e.Th.T.MonoSm, ink, shown.Frequency, look.FormatFrequency, held)
					})),
					layout.Rigid(layout.Spacer{Height: unit.Dp(3)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return p.layoutShapeButton(gtx, e, index, control, band.Type, ink)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(3)}.Layout),
					layout.Rigid(centered(func(gtx layout.Context) layout.Dimensions {
						return control.q.Layout(gtx, e.Th, band.Q, model.EQMinQ, model.EQMaxQ, func(gtx layout.Context) layout.Dimensions {
							return layout.Inset{Top: unit.Dp(2), Bottom: unit.Dp(2), Left: unit.Dp(4), Right: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return control.qRoll.Layout(gtx, e.Th, e.Th.T.MonoSm, ink, shown.Q, look.FormatQ, control.q.Held())
							})
						})
					})),
				)
				p.consumeBand(gtx, e, index, control)
				return dimensions
			})
		})
		pass := pointer.PassOp{}.Push(gtx.Ops)
		area := clip.Rect(image.Rectangle{Max: dimensions.Size}).Push(gtx.Ops)
		event.Op(gtx.Ops, &control.hoverTag)
		area.Pop()
		pass.Pop()
		return dimensions
	})
}

func (p *Page) consumeBand(gtx layout.Context, e *env.Env, index int, control *bandControl) {
	band := &p.working.Bands[index]
	if value, dragging := control.fader.Dragging(); dragging && value != band.Gain {
		p.graph.Select(index)
		band.Gain = value
		p.markEdit(gtx, e, true, false)
	}
	if value, ok := control.fader.Committed(); ok {
		p.graph.Select(index)
		changed := value != band.Gain
		band.Gain = value
		if changed || p.dragging {
			p.markEdit(gtx, e, false, true)
		}
	}
	if value, dragging := control.q.Dragging(); dragging && value != band.Q {
		p.graph.Select(index)
		band.Q = value
		p.markEdit(gtx, e, true, false)
	}
	if value, ok := control.q.Committed(); ok {
		p.graph.Select(index)
		changed := value != band.Q
		band.Q = value
		if changed || p.dragging {
			p.markEdit(gtx, e, false, true)
		}
	}
}

// layoutShapeButton is the band's filter shape: a hairline outline in the
// design's corners that washes under the pointer, its glyph pressing to .86.
func (p *Page) layoutShapeButton(gtx layout.Context, e *env.Env, index int, control *bandControl, shape model.EQBandType, ink color.NRGBA) layout.Dimensions {
	click := &control.shape
	if click.Clicked(gtx) {
		p.graph.Select(index)
		anchor := e.Overlays.AnchorOf(&click.Clickable, image.Pt(gtx.Dp(32), gtx.Dp(28)))
		p.openShapeMenu(e, index, anchor.Min)
	}
	hover, _ := control.shapeWash.Update(gtx, e.Th, click.Hovered(), click.Pressed())
	ink = palette.Mix(ink, e.Th.R.Fg, hover)
	return click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		size := gtx.Constraints.Constrain(image.Pt(gtx.Dp(32), gtx.Dp(28)))
		bounds := image.Rectangle{Max: size}
		cut := gtx.Dp(shapeCut)
		control.shapeWash.Paint(gtx, e.Th, bounds, cut)
		kit.Stroke(gtx, e.Th.Theme, bounds, cut, max(1, gtx.Dp(1)), e.Th.R.Vein)
		glyph := gtx.Dp(18)
		scale := control.shapeWash.GlyphScale()
		centre := f32.Pt(float32(size.X)/2, float32(size.Y)/2)
		transform := op.Affine(f32.AffineId().Offset(f32.Pt(-float32(glyph)/2, -float32(glyph)/2)).Scale(f32.Point{}, f32.Pt(scale, scale)).Offset(centre)).Push(gtx.Ops)
		glyphGTX := gtx
		glyphGTX.Constraints = layout.Exact(image.Pt(glyph, glyph))
		look.EQShapeGlyph(glyphGTX, e.Th, shape, ink)
		transform.Pop()
		return layout.Dimensions{Size: size}
	})
}

func (p *Page) layoutAdd(gtx layout.Context, e *env.Env) layout.Dimensions {
	disabled := len(p.working.Bands) >= model.EQMaxBands
	p.add.Kind, p.add.Small, p.add.Icon, p.add.Label = kit.PlaqueQuiet, true, look.Icon.Add, "Band"
	body := func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Left: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			if disabled {
				gtx = gtx.Disabled()
			}
			return p.add.Layout(gtx, e.Th.Theme)
		})
	}
	if disabled {
		return p.addTip.Layout(gtx, e.Th, e.Overlays, "Twenty bands is the most", body)
	}
	return body(gtx)
}

// layoutHeadroom warns under the graph when the curve lifts the signal past
// 0 dB: a paused claim, the reading in detail fg2, and a quiet plaque that
// lowers the preamp by the peak.
func (p *Page) layoutHeadroom(gtx layout.Context, e *env.Env) layout.Dimensions {
	p.updateFigures()
	if !p.working.Enabled || p.figures.peak <= .05 {
		return layout.Dimensions{}
	}
	p.lower.Kind, p.lower.Small, p.lower.Label = kit.PlaqueQuiet, true, "Lower preamp"
	return layout.Inset{Top: headroomGap}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(kit.Claim(e.Th.Theme, palette.Paused, "May clip")),
			layout.Rigid(layout.Spacer{Width: unit.Dp(13)}.Layout),
			layout.Flexed(1, kit.Text(e.Th.Theme, e.Th.T.Detail, e.Th.R.Fg2, p.headroomText)),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return p.lower.Layout(gtx, e.Th.Theme) }),
		)
	})
}

func presetName(equalizer model.Equalizer) string {
	if strings.TrimSpace(equalizer.Preset) != "" {
		return equalizer.Preset
	}
	return "Custom"
}
