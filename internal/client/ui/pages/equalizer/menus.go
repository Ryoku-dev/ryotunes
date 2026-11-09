package equalizer

import (
	"image"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
)

const dividerCommand = -2
const deletePresetCommandBase = -100

func (p *Page) openPresetMenu(gtx layout.Context, e *env.Env) {
	if e.Overlays == nil {
		return
	}
	anchor := e.Overlays.AnchorOf(&p.presetButton.Clickable, image.Pt(gtx.Dp(168), gtx.Dp(32)))
	items := make([]kit.MenuItem, 0, len(p.presets)+3)
	p.presetCommands = p.presetCommands[:0]
	builtins := 0
	for index, preset := range p.presets {
		if preset.Builtin {
			items = append(items, kit.MenuItem{Label: preset.Name})
			p.presetCommands = append(p.presetCommands, index)
			builtins++
		}
	}
	saved := 0
	for _, preset := range p.presets {
		if !preset.Builtin {
			saved++
		}
	}
	if builtins > 0 && saved > 0 {
		items = append(items, kit.MenuItem{Divider: true})
		p.presetCommands = append(p.presetCommands, dividerCommand)
	}
	for index, preset := range p.presets {
		if !preset.Builtin {
			items = append(items, kit.MenuItem{Label: preset.Name})
			p.presetCommands = append(p.presetCommands, index)
			items = append(items, kit.MenuItem{Label: "Remove " + preset.Name, Danger: true})
			p.presetCommands = append(p.presetCommands, deletePresetCommandBase-index)
		}
	}
	if len(items) > 0 {
		items = append(items, kit.MenuItem{Divider: true})
		p.presetCommands = append(p.presetCommands, dividerCommand)
	}
	items = append(items, kit.MenuItem{Label: "Save as preset"})
	p.presetCommands = append(p.presetCommands, savePresetCommand)
	content := func(menuGTX layout.Context) layout.Dimensions {
		dimensions := p.presetMenu.Layout(menuGTX, e.Th.Theme, items)
		if selected, ok := p.presetMenu.Clicked(menuGTX); ok && selected >= 0 && selected < len(p.presetCommands) {
			command := p.presetCommands[selected]
			e.Overlays.Close(&p.presetTag)
			switch {
			case command == savePresetCommand:
				p.saveDialog.Open()
			case command <= deletePresetCommandBase:
				index := deletePresetCommandBase - command
				if index >= 0 && index < len(p.presets) && !p.presets[index].Builtin {
					p.deleteDialog.Open(p.presets[index].Name)
				}
			case command >= 0 && command < len(p.presets):
				p.animateTo(menuGTX, e, presetEqualizer(p.presets[command], p.working.Enabled))
			}
			menuGTX.Execute(op.InvalidateCmd{})
		}
		return dimensions
	}
	e.Overlays.Open(&p.presetTag, anchor, look.Placement{Prefer: look.Below, Align: look.End, Width: unit.Dp(260)}, content)
}

func (p *Page) openShapeMenu(e *env.Env, band int, at image.Point) {
	if e.Overlays == nil || band < 0 || band >= len(p.working.Bands) {
		return
	}
	p.shapeBand = band
	content := func(gtx layout.Context) layout.Dimensions {
		current := p.shapeBand
		if current < 0 || current >= len(p.working.Bands) {
			e.Overlays.Close(&p.shapeTag)
			return layout.Dimensions{}
		}
		bandValue := p.working.Bands[current]
		items := make([]kit.MenuItem, 0, len(shapeTypes)+3)
		for index, label := range shapeLabels {
			items = append(items, kit.MenuItem{Label: label, Preview: selectedPreview(shapeTypes[index] == bandValue.Type)})
		}
		items = append(items, kit.MenuItem{Divider: true})
		if bandValue.Enabled {
			items = append(items, kit.MenuItem{Label: "Disable"})
		} else {
			items = append(items, kit.MenuItem{Label: "Enable"})
		}
		items = append(items, kit.MenuItem{Label: "Remove", Danger: true})
		dimensions := p.shapeMenu.Layout(gtx, e.Th.Theme, items)
		if selected, ok := p.shapeMenu.Clicked(gtx); ok {
			e.Overlays.Close(&p.shapeTag)
			switch {
			case selected >= 0 && selected < len(shapeTypes):
				p.working.Bands[current].Type = shapeTypes[selected]
				p.markEdit(gtx, e, false, true)
			case selected == len(shapeTypes)+1:
				p.working.Bands[current].Enabled = !p.working.Bands[current].Enabled
				p.markEdit(gtx, e, false, true)
			case selected == len(shapeTypes)+2:
				p.removeBand(gtx, e, current)
			}
			gtx.Execute(op.InvalidateCmd{})
		}
		return dimensions
	}
	e.Overlays.OpenAt(&p.shapeTag, at, content)
}

func selectedPreview(selected bool) kit.State {
	if selected {
		return kit.StateFocus
	}
	return kit.StateRest
}
