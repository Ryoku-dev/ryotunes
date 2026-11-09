package equalizer

import (
	"image"
	"strconv"
	"strings"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

type savePresetDialog struct {
	dialog kit.Dialog
	name   kit.TextField
	cancel kit.Plaque
	save   kit.Plaque
	busy   bool
	err    string
}

func (dialog *savePresetDialog) Open() {
	dialog.dialog.Open = true
	dialog.name.SetText("")
	dialog.err = ""
}

func (dialog *savePresetDialog) Layout(gtx layout.Context, e *env.Env, page *Page) layout.Dimensions {
	if dialog.cancel.Clicked(gtx) {
		dialog.dialog.Dismiss(gtx)
	}
	if dialog.save.Clicked(gtx) && !dialog.busy {
		name := strings.TrimSpace(dialog.name.Text())
		if name == "" {
			dialog.err = "Name the preset first."
		} else {
			dialog.busy, dialog.err = true, ""
			act.Call[[]model.EQPreset](e, protocol.MethodSaveEqualizerPreset, protocol.EqualizerPresetParams{Name: name}, func(presets []model.EQPreset, err error) {
				dialog.busy = false
				if err != nil {
					dialog.err = err.Error()
					return
				}
				page.presets = append(page.presets[:0], presets...)
				page.presetsLoaded = true
				page.working.Preset = name
				dialog.dialog.Open = false
				if e.Toast != nil {
					e.Toast("Preset saved", env.ToastSuccess)
				}
			})
		}
	}
	dialog.name.Label, dialog.name.Placeholder, dialog.name.Error = "Preset name", "My headphones", dialog.err
	dialog.cancel.Kind, dialog.cancel.Label = kit.PlaqueQuiet, "Cancel"
	dialog.save.Kind, dialog.save.Label, dialog.save.Busy = kit.PlaqueLine, "Save", dialog.busy
	dialog.dialog.Width = 520
	return dialog.dialog.Layout(gtx, e.Th.Theme, "Save as preset",
		func(gtx layout.Context) layout.Dimensions { return dialog.name.Layout(gtx, e.Th.Theme) },
		func(gtx layout.Context) layout.Dimensions { return dialog.cancel.Layout(gtx, e.Th.Theme) },
		func(gtx layout.Context) layout.Dimensions { return dialog.save.Layout(gtx, e.Th.Theme) },
	)
}

type deletePresetDialog struct {
	dialog kit.Dialog
	cancel kit.Plaque
	remove kit.Plaque
	name   string
	busy   bool
	err    string
}

func (dialog *deletePresetDialog) Open(name string) {
	dialog.name = name
	dialog.err = ""
	dialog.dialog.Open = true
}

func (dialog *deletePresetDialog) Layout(gtx layout.Context, e *env.Env, page *Page) layout.Dimensions {
	if dialog.cancel.Clicked(gtx) {
		dialog.dialog.Dismiss(gtx)
	}
	if dialog.remove.Clicked(gtx) && !dialog.busy {
		dialog.busy, dialog.err = true, ""
		params := protocol.EqualizerPresetParams{Name: dialog.name}
		act.Call[[]model.EQPreset](e, protocol.MethodDeleteEqualizerPreset, params, func(presets []model.EQPreset, err error) {
			dialog.busy = false
			if err != nil {
				dialog.err = err.Error()
				return
			}
			page.presets = append(page.presets[:0], presets...)
			page.presetsLoaded = true
			dialog.dialog.Open = false
			if e.Toast != nil {
				e.Toast("Preset removed", env.ToastSuccess)
			}
		})
	}
	dialog.cancel.Kind, dialog.cancel.Label = kit.PlaqueQuiet, "Cancel"
	dialog.remove.Kind, dialog.remove.Label, dialog.remove.Busy = kit.PlaqueDangerLine, "Remove", dialog.busy
	dialog.dialog.Width = 480
	return dialog.dialog.Layout(gtx, e.Th.Theme, "Remove saved preset",
		func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(kit.Text(e.Th.Theme, e.Th.T.Body, e.Th.R.Fg, "Remove “"+dialog.name+"”?")),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if dialog.err == "" {
						return layout.Dimensions{}
					}
					return layout.Inset{Top: unit.Dp(8)}.Layout(gtx, kit.Text(e.Th.Theme, e.Th.T.Proof, e.Th.R.Fg2, dialog.err))
				}),
			)
		},
		func(gtx layout.Context) layout.Dimensions { return dialog.cancel.Layout(gtx, e.Th.Theme) },
		func(gtx layout.Context) layout.Dimensions { return dialog.remove.Layout(gtx, e.Th.Theme) },
	)
}

type profileEditor struct {
	widget.Editor
}

func (editor *profileEditor) Layout(gtx layout.Context, e *env.Env) layout.Dimensions {
	editor.SingleLine = false
	editor.Submit = false
	size := gtx.Constraints.Constrain(image.Pt(gtx.Constraints.Max.X, gtx.Dp(180)))
	kit.FillCut(gtx.Ops, image.Rectangle{Max: size}, gtx.Dp(7), e.Th.R.Stone3)
	inset := gtx.Dp(13)
	textGTX := gtx
	textGTX.Constraints.Min = image.Pt(size.X-2*inset, size.Y-2*inset)
	textGTX.Constraints.Max = textGTX.Constraints.Min
	record := op.Record(gtx.Ops)
	paint.ColorOp{Color: e.Th.R.Fg}.Add(gtx.Ops)
	textCall := record.Stop()
	record = op.Record(gtx.Ops)
	paint.ColorOp{Color: e.Th.R.Sel}.Add(gtx.Ops)
	selectionCall := record.Stop()
	offset := op.Offset(image.Pt(inset, inset)).Push(gtx.Ops)
	editor.Editor.Layout(textGTX, e.Th.Shaper, e.Th.T.Mono.Face, e.Th.T.Mono.Size, textCall, selectionCall)
	offset.Pop()
	if editor.Len() == 0 {
		placeholderGTX := gtx
		placeholderGTX.Constraints.Min = image.Point{}
		placeholderGTX.Constraints.Max = image.Pt(size.X-2*inset, size.Y-2*inset)
		offset = op.Offset(image.Pt(inset, inset)).Push(gtx.Ops)
		kit.Label{Style: e.Th.T.Body, Color: e.Th.R.Fg3, MaxLines: 4}.Layout(placeholderGTX, e.Th.Theme, "Paste an AutoEQ or Equalizer APO parametric profile")
		offset.Pop()
	}
	return layout.Dimensions{Size: size}
}

type importProfileDialog struct {
	dialog       kit.Dialog
	profile      profileEditor
	name         kit.TextField
	cancel       kit.Plaque
	importButton kit.Plaque
	busy         bool
	err          string
}

func (dialog *importProfileDialog) Open() {
	dialog.dialog.Open = true
	dialog.profile.SetText("")
	dialog.name.SetText("")
	dialog.err = ""
}

func (dialog *importProfileDialog) Layout(gtx layout.Context, e *env.Env, page *Page) layout.Dimensions {
	if dialog.cancel.Clicked(gtx) {
		dialog.dialog.Dismiss(gtx)
	}
	if dialog.importButton.Clicked(gtx) && !dialog.busy {
		profile := strings.TrimSpace(dialog.profile.Text())
		if profile == "" {
			dialog.err = "Paste a profile first."
		} else {
			dialog.busy, dialog.err = true, ""
			params := protocol.ImportEqualizerParams{Text: profile, Name: strings.TrimSpace(dialog.name.Text())}
			act.Call[protocol.ImportEqualizerResult](e, protocol.MethodImportEqualizer, params, func(result protocol.ImportEqualizerResult, err error) {
				dialog.busy = false
				if err != nil {
					dialog.err = err.Error()
					return
				}
				page.working = cloneEqualizer(result.Equalizer)
				page.pending = true
				page.submitted = cloneEqualizer(result.Equalizer)
				dialog.dialog.Open = false
				message := "Imported " + countWord(len(result.Equalizer.Bands), "band") + "."
				if len(result.Warnings) > 0 {
					message += " Skipped " + countWord(len(result.Warnings), "line") + "."
				}
				if e.Toast != nil {
					e.Toast(message, env.ToastSuccess)
				}
			})
		}
	}
	dialog.name.Label, dialog.name.Placeholder = "Save as (optional)", "Imported profile"
	dialog.name.Error = dialog.err
	dialog.cancel.Kind, dialog.cancel.Label = kit.PlaqueQuiet, "Cancel"
	dialog.importButton.Kind, dialog.importButton.Label, dialog.importButton.Busy = kit.PlaqueLine, "Import", dialog.busy
	dialog.dialog.Width = 640
	return dialog.dialog.Layout(gtx, e.Th.Theme, "Import equalizer profile",
		func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return dialog.profile.Layout(gtx, e) }),
				layout.Rigid(layout.Spacer{Height: unit.Dp(13)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return dialog.name.Layout(gtx, e.Th.Theme) }),
			)
		},
		func(gtx layout.Context) layout.Dimensions { return dialog.cancel.Layout(gtx, e.Th.Theme) },
		func(gtx layout.Context) layout.Dimensions { return dialog.importButton.Layout(gtx, e.Th.Theme) },
	)
}

func countWord(count int, noun string) string {
	if count == 1 {
		return "1 " + noun
	}
	return strconv.Itoa(count) + " " + noun + "s"
}
