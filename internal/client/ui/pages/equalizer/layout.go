package equalizer

import (
	"fmt"
	"image"
	"image/color"
	"strings"

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
	"github.com/ryoku-dev/ryotunes/internal/eq"
	"github.com/ryoku-dev/ryotunes/internal/model"
)

const savePresetCommand = -1

var shapeTypes = []model.EQBandType{
	model.EQPeak, model.EQLowShelf, model.EQHighShelf, model.EQLowPass,
	model.EQHighPass, model.EQNotch, model.EQBandPass,
}

var shapeLabels = []string{"Bell", "Low shelf", "High shelf", "Low pass", "High pass", "Notch", "Band pass"}

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

	p.graphHeight = unit.Dp(float32(gtx.Constraints.Max.Y) * .4 / gtx.Metric.PxPerDp)
	p.graphHeight = min(max(p.graphHeight, unit.Dp(220)), unit.Dp(360))
	inset := look.ContentInset(e.Th.Class)
	dimensions := p.scroll.Layout(gtx, e.Th, 1, func(gtx layout.Context, _ int) layout.Dimensions {
		return layout.Inset{Top: inset, Right: inset, Bottom: inset, Left: inset}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return p.layoutTitle(gtx, e) }),
				layout.Rigid(layout.Spacer{Height: unit.Dp(21)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return p.layoutGraph(gtx, e) }),
				layout.Rigid(layout.Spacer{Height: unit.Dp(21)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return p.layoutStrip(gtx, e) }),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return p.layoutHeadroom(gtx, e) }),
			)
		})
	})
	p.saveDialog.Layout(gtx, e, p)
	p.deleteDialog.Layout(gtx, e, p)
	p.importDialog.Layout(gtx, e, p)
	return dimensions
}

func (p *Page) handleTitleActions(gtx layout.Context, e *env.Env) {
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
		p.animateTo(gtx, e, p.resetTarget())
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

	controls := func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceEnd}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return p.toggle.Layout(gtx, e.Th.Theme) }),
			layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return p.presetButton.Layout(gtx, e.Th.Theme) }),
			layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return p.importButton.Layout(gtx, e.Th.Theme) }),
			layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return p.exportButton.Layout(gtx, e.Th.Theme) }),
			layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return p.reset.Layout(gtx, e.Th.Theme) }),
		)
	}
	var dimensions layout.Dimensions
	if gtx.Constraints.Max.X >= gtx.Dp(760) {
		dimensions = layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Flexed(1, kit.Text(e.Th.Theme, e.Th.T.Section, e.Th.R.Fg, "Equalizer")),
			layout.Rigid(controls),
		)
	} else {
		dimensions = layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(kit.Text(e.Th.Theme, e.Th.T.Section, e.Th.R.Fg, "Equalizer")),
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

func (p *Page) layoutStrip(gtx layout.Context, e *env.Env) layout.Dimensions {
	if p.stripPrevious.Clicked(gtx) && p.strip.CanPage(-1) {
		p.strip.Page(gtx, -1)
	}
	if p.stripNext.Clicked(gtx) && p.strip.CanPage(1) {
		p.strip.Page(gtx, 1)
	}
	selected := p.graph.Selected()
	p.stripSelection.Duration = e.Th.Motion.Select
	p.stripSelection.Ease = anim.EaseChisel
	if selected >= 0 {
		if !p.stripSelectionReady {
			p.stripSelection = anim.Tween{}
			p.stripSelectionReady = true
		}
		p.stripSelection.Duration = e.Th.Motion.Select
		p.stripSelection.Ease = anim.EaseChisel
		p.stripSelection.Set(gtx, float32(selected))
	}
	selectionAt := p.stripSelection.Value(gtx)
	count := len(p.working.Bands) + 2
	return p.layoutStripScroller(gtx, e, count, func(gtx layout.Context, index int) layout.Dimensions {
		switch {
		case index == 0:
			return p.layoutPreamp(gtx, e)
		case index == count-1:
			return p.layoutAdd(gtx, e)
		default:
			return p.layoutBand(gtx, e, index-1, selectionAt)
		}
	})
}

func (p *Page) layoutStripScroller(gtx layout.Context, e *env.Env, count int, element layout.ListElement) layout.Dimensions {
	button := gtx.Dp(unit.Dp(32))
	gap := gtx.Dp(unit.Dp(8))
	width := gtx.Constraints.Max.X
	gutter := 0
	showSteps := p.stripSteps
	if showSteps {
		gutter = button + gap
	}
	viewport := max(0, width-2*gutter)

	stripGTX := gtx
	stripGTX.Constraints.Min.X, stripGTX.Constraints.Max.X = viewport, viewport
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
		return layout.Dimensions{Size: gtx.Constraints.Constrain(image.Pt(width, stripDimensions.Size.Y))}
	}

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

func stripCell(gtx layout.Context, width unit.Dp, selected bool, th *look.Theme, body layout.Widget) layout.Dimensions {
	widthPx := gtx.Dp(width)
	gtx.Constraints.Min.X, gtx.Constraints.Max.X = widthPx, widthPx
	if selected {
		return kit.Slab(gtx, th.R.Sel, unit.Dp(6), body)
	}
	return body(gtx)
}

func movingStripCell(gtx layout.Context, width unit.Dp, index int, selectionAt float32, th *look.Theme, body layout.Widget) layout.Dimensions {
	widthPx := gtx.Dp(width)
	gtx.Constraints.Min.X, gtx.Constraints.Max.X = widthPx, widthPx
	macro := op.Record(gtx.Ops)
	dimensions := body(gtx)
	call := macro.Stop()
	area := clip.Rect(image.Rectangle{Max: dimensions.Size}).Push(gtx.Ops)
	x := int((selectionAt - float32(index)) * float32(gtx.Dp(60)))
	kit.FillCut(gtx.Ops, image.Rect(x, 0, x+widthPx, dimensions.Size.Y), gtx.Dp(6), th.R.Sel)
	call.Add(gtx.Ops)
	area.Pop()
	return dimensions
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
	dimensions := layout.Inset{Right: unit.Dp(13)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return stripCell(gtx, 64, false, e.Th, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(kit.Text(e.Th.Theme, e.Th.T.MonoSm, e.Th.R.Fg2, look.FormatGain(p.working.Preamp))),
					layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return p.preamp.Layout(gtx, e.Th, p.working.Preamp, look.FaderStyle{Min: model.EQMinPreamp, Max: model.EQMaxPreamp, Reset: 0, Step: .5, Height: stripFaderHeight(e.Th.Class), Label: "Preamp"})
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
					layout.Rigid(kit.Text(e.Th.Theme, e.Th.T.Caption, e.Th.R.Fg3, "Pre")),
				)
			})
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

func (p *Page) layoutBand(gtx layout.Context, e *env.Env, index int, selectionAt float32) layout.Dimensions {
	if index < 0 || index >= len(p.working.Bands) {
		return layout.Dimensions{}
	}
	if index >= len(p.bands) {
		p.bands = append(p.bands, make([]bandControl, index-len(p.bands)+1)...)
	}
	control := &p.bands[index]
	control.unfold.Duration = e.Th.Motion.Unfold
	control.unfold.Ease = anim.EaseChisel
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
		return layout.Dimensions{}
	}
	band := &p.working.Bands[index]
	selected := index == p.graph.Selected()
	gainText := " "
	if band.HasGain() {
		gainText = look.FormatGain(band.Gain)
	}
	return revealHorizontal(gtx, progress, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Right: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return movingStripCell(gtx, 56, index, selectionAt, e.Th, func(gtx layout.Context) layout.Dimensions {
				ink := e.Th.R.Fg2
				if selected {
					ink = e.Th.R.OnSel
				}
				return layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					dimensions := layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(kit.Text(e.Th.Theme, e.Th.T.MonoSm, ink, gainText)),
						layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return control.fader.Layout(gtx, e.Th, band.Gain, look.FaderStyle{Min: model.EQMinGain, Max: model.EQMaxGain, Reset: 0, Step: .5, Height: stripFaderHeight(e.Th.Class), Label: "Band gain", Disabled: !band.HasGain()})
						}),
						layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
						layout.Rigid(kit.Text(e.Th.Theme, e.Th.T.MonoSm, ink, look.FormatFrequency(band.Frequency))),
						layout.Rigid(layout.Spacer{Height: unit.Dp(3)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return p.layoutShapeButton(gtx, e, index, &control.shape, band.Type, ink)
						}),
						layout.Rigid(layout.Spacer{Height: unit.Dp(3)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return control.q.Layout(gtx, band.Q, model.EQMinQ, model.EQMaxQ, kit.Text(e.Th.Theme, e.Th.T.MonoSm, ink, fmt.Sprintf("Q %.2f", band.Q)))
						}),
					)
					p.consumeBand(gtx, e, index, control)
					return dimensions
				})
			})
		})
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

func (p *Page) layoutShapeButton(gtx layout.Context, e *env.Env, index int, click *kit.Click, shape model.EQBandType, ink color.NRGBA) layout.Dimensions {
	if click.Clicked(gtx) {
		p.graph.Select(index)
		anchor := e.Overlays.AnchorOf(&click.Clickable, image.Pt(gtx.Dp(32), gtx.Dp(28)))
		p.openShapeMenu(e, index, anchor.Min)
	}
	return click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		size := gtx.Constraints.Constrain(image.Pt(gtx.Dp(32), gtx.Dp(28)))
		kit.StrokeCut(gtx.Ops, image.Rectangle{Max: size}, gtx.Dp(5), max(1, gtx.Dp(1)), e.Th.R.Vein)
		offset := op.Offset(image.Pt((size.X-gtx.Dp(18))/2, (size.Y-gtx.Dp(18))/2)).Push(gtx.Ops)
		glyphGTX := gtx
		glyphGTX.Constraints = layout.Exact(image.Pt(gtx.Dp(18), gtx.Dp(18)))
		look.EQShapeGlyph(glyphGTX, e.Th, shape, ink)
		offset.Pop()
		return layout.Dimensions{Size: size}
	})
}

func (p *Page) layoutAdd(gtx layout.Context, e *env.Env) layout.Dimensions {
	disabled := len(p.working.Bands) >= model.EQMaxBands
	p.add.Kind, p.add.Small, p.add.Icon, p.add.Label = kit.PlaqueQuiet, true, look.Icon.Add, "Band"
	body := func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: unit.Dp(72), Left: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
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

func (p *Page) layoutHeadroom(gtx layout.Context, e *env.Env) layout.Dimensions {
	if !p.working.Enabled {
		return layout.Spacer{Height: unit.Dp(8)}.Layout(gtx)
	}
	_, peak := p.graph.Curve(p.working)
	if peak <= .05 {
		return layout.Spacer{Height: unit.Dp(8)}.Layout(gtx)
	}
	p.lower.Kind, p.lower.Small, p.lower.Label = kit.PlaqueLine, true, "Lower preamp"
	return layout.Inset{Top: unit.Dp(13)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(kit.Claim(e.Th.Theme, palette.Paused, "May clip")),
			layout.Rigid(layout.Spacer{Width: unit.Dp(13)}.Layout),
			layout.Flexed(1, kit.Text(e.Th.Theme, e.Th.T.Proof, e.Th.R.Fg2, fmt.Sprintf("Peaks reach +%.1f dB", peak))),
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
