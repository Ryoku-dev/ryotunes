package chrome

import (
	"fmt"
	"image"
	"math"
	"strconv"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

var soundPresetLabels = []string{"Flat", "Slowed and reverb", "Nightcore", "Bass boost", "Wide"}

var soundPresets = []model.AudioFx{
	{Speed: 1},
	{Speed: .85, Semitones: -2, Reverb: .35},
	{Speed: 1.25, Semitones: 3},
	{Speed: 1, Bass: 6},
	{Speed: 1, Width: 1},
}

// Sound owns the anchored sound-shaping overlay content.
type Sound struct {
	presets   look.FlowChoice
	tempo     kit.Slider
	pitch     kit.Slider
	reverb    kit.Slider
	bass      kit.Slider
	width     kit.Slider
	reset     kit.Plaque
	equalizer kit.Click

	initialized bool
	target      model.AudioFx
	pending     bool
	dirty       bool
	applyAt     time.Time
}

// Layout renders the sound controls inside the shell's 340 dp window overlay.
func (sound *Sound) Layout(gtx layout.Context, e *env.Env) layout.Dimensions {
	sound.reconcile(e)
	sound.configure()
	if sound.equalizer.Clicked(gtx) {
		e.Nav.Push(env.Route{Kind: env.RouteEqualizer, Title: "Equalizer"})
		if tag, ok := e.Overlays.Top(); ok {
			e.Overlays.Close(tag)
		}
	}
	sound.presets.Labels = soundPresetLabels
	if index := soundPresetIndex(sound.target); index >= 0 {
		sound.presets.Selected = index
	}

	changed := false
	dimensions := layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(kit.Text(e.Th.Theme, e.Th.T.Section, e.Th.R.Fg, "Sound")),
		layout.Rigid(layout.Spacer{Height: 13}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return sound.presets.Layout(gtx, e.Th)
		}),
		layout.Rigid(layout.Spacer{Height: 21}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			dims := soundSliderRow(gtx, e, "Tempo", fmt.Sprintf("%.2f×", sound.tempo.Value), &sound.tempo)
			changed = changed || sound.tempo.Changed()
			return dims
		}),
		layout.Rigid(layout.Spacer{Height: 8}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			dims := soundSliderRow(gtx, e, "Pitch", signedInt(int(math.Round(float64(sound.pitch.Value))))+" st", &sound.pitch)
			changed = changed || sound.pitch.Changed()
			return dims
		}),
		layout.Rigid(layout.Spacer{Height: 8}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			dims := soundSliderRow(gtx, e, "Reverb", percent(sound.reverb.Value), &sound.reverb)
			changed = changed || sound.reverb.Changed()
			return dims
		}),
		layout.Rigid(layout.Spacer{Height: 8}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			dims := soundSliderRow(gtx, e, "Bass", signedFloat(sound.bass.Value)+" dB", &sound.bass)
			changed = changed || sound.bass.Changed()
			return dims
		}),
		layout.Rigid(layout.Spacer{Height: 8}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			dims := soundSliderRow(gtx, e, "Width", percent(sound.width.Value), &sound.width)
			changed = changed || sound.width.Changed()
			return dims
		}),
		layout.Rigid(layout.Spacer{Height: 13}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			sound.reset.Kind = kit.PlaqueQuiet
			sound.reset.Icon = look.Icon.Refresh
			sound.reset.Label = "Reset"
			sound.reset.Small = true
			return sound.reset.Layout(gtx, e.Th.Theme)
		}),
		layout.Rigid(layout.Spacer{Height: 13}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return sound.layoutEqualizer(gtx, e, e.Store.Snapshot().Equalizer)
		}),
	)

	if selected, ok := sound.presets.Changed(); ok && selected >= 0 && selected < len(soundPresets) {
		sound.setValues(soundPresets[selected])
		changed = true
	}
	if sound.reset.Clicked(gtx) {
		sound.setValues(model.DefaultAudioFx())
		sound.presets.Selected = 0
		changed = true
	}
	if changed {
		sound.target = sound.values()
		sound.pending = true
		sound.dirty = true
		sound.applyAt = gtx.Now.Add(80 * time.Millisecond)
	}
	if sound.dirty {
		if gtx.Now.Before(sound.applyAt) {
			gtx.Execute(op.InvalidateCmd{At: sound.applyAt})
		} else {
			sound.dirty = false
			callVoid(e, protocol.MethodSetAudioFx, sound.target)
		}
	}
	return dimensions
}

func (sound *Sound) layoutEqualizer(gtx layout.Context, e *env.Env, equalizer model.Equalizer) layout.Dimensions {
	return sound.equalizer.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		size := image.Pt(gtx.Constraints.Max.X, gtx.Dp(64))
		size = gtx.Constraints.Constrain(size)
		if sound.equalizer.Hovered() {
			kit.FillCut(gtx.Ops, image.Rectangle{Max: size}, gtx.Dp(6), e.Th.R.Stone3)
		}
		return layout.Inset{Top: unit.Dp(8), Right: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(kit.Text(e.Th.Theme, e.Th.T.Label, e.Th.R.Fg, "Equalizer")),
						layout.Rigid(kit.Text(e.Th.Theme, e.Th.T.Caption, e.Th.R.Fg3, equalizerState(equalizer))),
					)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints = layout.Exact(image.Pt(gtx.Dp(120), gtx.Dp(48)))
					return look.EqualizerPreview(gtx, e.Th, equalizer, 48)
				}),
			)
		})
	})
}

func equalizerState(equalizer model.Equalizer) string {
	if !equalizer.Enabled {
		return "Bypassed"
	}
	preset := equalizer.Preset
	if preset == "" {
		preset = "Custom"
	}
	return "On · " + preset
}

func (sound *Sound) configure() {
	sound.tempo.Min, sound.tempo.Max, sound.tempo.Step = .25, 2, .05
	sound.tempo.Label = "Tempo"
	sound.pitch.Min, sound.pitch.Max, sound.pitch.Step = -12, 12, 1
	sound.pitch.Label = "Pitch"
	sound.reverb.Min, sound.reverb.Max, sound.reverb.Step = 0, 1, .01
	sound.reverb.Label = "Reverb"
	sound.bass.Min, sound.bass.Max, sound.bass.Step = -6, 12, .5
	sound.bass.Label = "Bass"
	sound.width.Min, sound.width.Max, sound.width.Step = 0, 1, .01
	sound.width.Label = "Width"
}

func (sound *Sound) reconcile(e *env.Env) {
	current := e.Store.Snapshot().AudioFx
	if !sound.initialized {
		sound.setValues(current)
		sound.target = current
		sound.initialized = true
		return
	}
	if sound.pending {
		if current == sound.target {
			sound.pending = false
		}
		return
	}
	if !sound.dirty {
		sound.setValues(current)
		sound.target = current
	}
}

func (sound *Sound) values() model.AudioFx {
	return model.AudioFx{
		Speed: roundSoundValue(float64(sound.tempo.Value), 100), Semitones: int(math.Round(float64(sound.pitch.Value))),
		Reverb: roundSoundValue(float64(sound.reverb.Value), 100),
		Bass:   roundSoundValue(float64(sound.bass.Value), 2),
		Width:  roundSoundValue(float64(sound.width.Value), 100),
	}
}

func roundSoundValue(value, steps float64) float64 {
	return math.Round(value*steps) / steps
}

func (sound *Sound) setValues(fx model.AudioFx) {
	sound.tempo.Value = float32(fx.Speed)
	sound.pitch.Value = float32(fx.Semitones)
	sound.reverb.Value = float32(fx.Reverb)
	sound.bass.Value = float32(fx.Bass)
	sound.width.Value = float32(fx.Width)
}

func soundPresetIndex(fx model.AudioFx) int {
	name := fx.PresetName()
	for index, label := range soundPresetLabels {
		if label == name {
			return index
		}
	}
	return -1
}

func soundSliderRow(gtx layout.Context, e *env.Env, label, value string, slider *kit.Slider) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, kit.Text(e.Th.Theme, e.Th.T.Label, e.Th.R.Fg2, label)),
				layout.Rigid(kit.Text(e.Th.Theme, e.Th.T.Mono, e.Th.R.Fg3, value)),
			)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return slider.Layout(gtx, e.Th.Theme) }),
	)
}

func signedInt(value int) string {
	if value > 0 {
		return "+" + strconv.Itoa(value)
	}
	return strconv.Itoa(value)
}

func signedFloat(value float32) string {
	if math.Abs(float64(value)) < .05 {
		value = 0
	}
	formatted := strconv.FormatFloat(float64(value), 'f', 1, 32)
	if value > 0 {
		return "+" + formatted
	}
	return formatted
}

func percent(value float32) string {
	return strconv.Itoa(int(math.Round(float64(value*100)))) + "%"
}
