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

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/anim"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/tokens"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

// The Hub's dialog widths (§6.6): a decision is 520 wide; the profile
// editor, which takes a pasted file, may be 640.
const (
	dialogWidth unit.Dp = 520
	editorWidth unit.Dp = 640
)

// dialogError is a dialog's failure line: what went wrong, in detail jasper.
func dialogError(e *env.Env, message string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		if message == "" {
			return layout.Dimensions{}
		}
		return layout.Inset{Top: unit.Dp(8)}.Layout(gtx, kit.Text(e.Th.Theme, e.Th.T.Detail, e.Th.R.Jasper, message))
	}
}

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
	dialog.save.Kind, dialog.save.Label, dialog.save.Busy = kit.PlaqueGold, "Save", dialog.busy
	dialog.dialog.Width = dialogWidth
	return dialog.dialog.Layout(gtx, e.Th.Theme, "Save as preset",
		func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(kit.Text(e.Th.Theme, e.Th.T.Body, e.Th.R.Fg2, "Keep this curve in the preset list, under a name you choose.")),
				layout.Rigid(layout.Spacer{Height: unit.Dp(13)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return dialog.name.Layout(gtx, e.Th.Theme) }),
			)
		},
		func(gtx layout.Context) layout.Dimensions { return dialog.cancel.Layout(gtx, e.Th.Theme) },
		func(gtx layout.Context) layout.Dimensions { return dialog.save.Layout(gtx, e.Th.Theme) },
	)
}

type deletePresetDialog struct {
	dialog kit.Dialog
	cancel kit.Plaque
	remove kit.Plaque
	name   string
	body   string
	busy   bool
	err    string
}

func (dialog *deletePresetDialog) Open(name string) {
	dialog.name = name
	dialog.body = "“" + name + "” leaves the preset list. The curve you hear now stays as it is."
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
	dialog.remove.Kind, dialog.remove.Label, dialog.remove.Busy = kit.PlaqueDanger, "Remove", dialog.busy
	dialog.dialog.Width = dialogWidth
	return dialog.dialog.Layout(gtx, e.Th.Theme, "Remove saved preset?",
		func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(kit.Text(e.Th.Theme, e.Th.T.Body, e.Th.R.Fg2, dialog.body)),
				layout.Rigid(dialogError(e, dialog.err)),
			)
		},
		func(gtx layout.Context) layout.Dimensions { return dialog.cancel.Layout(gtx, e.Th.Theme) },
		func(gtx layout.Context) layout.Dimensions { return dialog.remove.Layout(gtx, e.Th.Theme) },
	)
}

// profileEditor is the pasted profile's field, a many-line text field as the
// kit draws one: the field plane in the mid cut, its edge brightening to fg2
// while it has focus (the focus time), mono text, the selection a sel wash.
type profileEditor struct {
	widget.Editor
	focus anim.Tween
}

func (editor *profileEditor) Layout(gtx layout.Context, e *env.Env) layout.Dimensions {
	editor.SingleLine = false
	editor.Submit = false
	focused := gtx.Focused(&editor.Editor)
	editor.focus.Ease = anim.EaseOutCubic
	// the new target is taken under the running duration, then the next
	// run is timed: focus reversed mid-way carries on from where it stands
	if focused {
		editor.focus.Set(gtx, 1)
		editor.focus.Duration = e.Th.Motion.FocusIn
	} else {
		editor.focus.Set(gtx, 0)
		editor.focus.Duration = e.Th.Motion.FocusOut
	}
	focus := editor.focus.Value(gtx)
	size := gtx.Constraints.Constrain(image.Pt(gtx.Constraints.Max.X, gtx.Dp(180)))
	bounds := image.Rectangle{Max: size}
	cut := gtx.Dp(e.Th.Cut.Mid)
	kit.Fill(gtx, e.Th.Theme, bounds, cut, e.Th.R.Field)
	kit.Stroke(gtx, e.Th.Theme, bounds, cut, max(1, gtx.Dp(unit.Dp(tokens.SizeHairline))), palette.Mix(e.Th.R.Edge, e.Th.R.Fg2, focus))
	inset := gtx.Dp(13)
	textGTX := gtx
	textGTX.Constraints.Min = image.Pt(size.X-2*inset, size.Y-2*inset)
	textGTX.Constraints.Max = textGTX.Constraints.Min
	record := op.Record(gtx.Ops)
	paint.ColorOp{Color: e.Th.R.Fg}.Add(gtx.Ops)
	textCall := record.Stop()
	record = op.Record(gtx.Ops)
	paint.ColorOp{Color: e.Th.R.Select()}.Add(gtx.Ops)
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
	dialog.cancel.Kind, dialog.cancel.Label = kit.PlaqueQuiet, "Cancel"
	dialog.importButton.Kind, dialog.importButton.Label, dialog.importButton.Busy = kit.PlaqueGold, "Import", dialog.busy
	dialog.dialog.Width = editorWidth
	return dialog.dialog.Layout(gtx, e.Th.Theme, "Import equalizer profile",
		func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return dialog.profile.Layout(gtx, e) }),
				layout.Rigid(dialogError(e, dialog.err)),
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
