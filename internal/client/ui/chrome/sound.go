package chrome

import (
	"image"
	"math"
	"strconv"
	"time"

	"gioui.org/layout"
	"gioui.org/op"

	"github.com/ryoku-dev/ryotunes/internal/client/lithic"
	"github.com/ryoku-dev/ryotunes/internal/client/state"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

// SoundPanel renders volume, tempo, pitch, and playback effects.
type SoundPanel struct {
	volume   lithic.Slider
	speed    lithic.Slider
	pitch    lithic.Slider
	reverb   lithic.Slider
	bass     lithic.Slider
	width    lithic.Slider
	reset    lithic.Button
	volumeOn bool

	volumePending bool
	volumeTarget  int
	fxPending     bool
	fxTarget      model.AudioFx
	initialized   bool
	fxDirty       bool
	fxAt          time.Time
}

// Layout renders the sound controls inside a player-bar popover.
func (p *SoundPanel) Layout(gtx layout.Context, e *env.Env) layout.Dimensions {
	state := e.Store.Snapshot()
	p.reconcile(state)

	width := min(gtx.Constraints.Max.X, gtx.Dp(340))
	height := min(gtx.Constraints.Max.Y, gtx.Dp(456))
	if width <= 0 || height <= 0 {
		return layout.Dimensions{}
	}
	gtx.Constraints = layout.Exact(image.Pt(width, height))
	lithic.FillCut(gtx.Ops, image.Rect(0, 0, width, height), gtx.Dp(8), e.Th.R.Stone4)
	lithic.StrokeCut(gtx.Ops, image.Rect(0, 0, width, height), gtx.Dp(8), max(1, gtx.Dp(1)), e.Th.R.Vein)

	reset := p.reset.Clicked(gtx)
	volumeChanged := false
	paramsChanged, effectsChanged := false, false
	content := layout.Inset{Top: 21, Bottom: 21, Left: 21, Right: 21}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(lithic.Text(e.Th, e.Th.T.Title, e.Th.R.Fg, "Sound")),
			layout.Rigid(layout.Spacer{Height: 13}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				changed, dims := soundSliderRow(gtx, e, "Volume", strconv.Itoa(p.volumeTarget)+"%", &p.volume)
				if changed {
					p.volumeTarget = int(math.Round(float64(p.volume.Value * 100)))
					p.volumePending = true
					volumeChanged = true
				}
				return dims
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				value := 0.5 + float64(p.speed.Value)*1.5
				changed, dims := soundSliderRow(gtx, e, "Speed", strconv.FormatFloat(value, 'f', 2, 64)+"×", &p.speed)
				paramsChanged = paramsChanged || changed
				return dims
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				value := int(math.Round(float64(p.pitch.Value)*24 - 12))
				changed, dims := soundSliderRow(gtx, e, "Pitch", signed(value)+" st", &p.pitch)
				paramsChanged = paramsChanged || changed
				return dims
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				changed, dims := soundSliderRow(gtx, e, "Reverb", percent(float64(p.reverb.Value)), &p.reverb)
				effectsChanged = effectsChanged || changed
				return dims
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				value := float64(p.bass.Value)*18 - 6
				changed, dims := soundSliderRow(gtx, e, "Bass", signedFloat(value)+" dB", &p.bass)
				effectsChanged = effectsChanged || changed
				return dims
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				changed, dims := soundSliderRow(gtx, e, "Width", percent(float64(p.width.Value)), &p.width)
				effectsChanged = effectsChanged || changed
				return dims
			}),
			layout.Rigid(layout.Spacer{Height: 8}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				p.reset.Small = true
				return p.reset.Layout(gtx, e.Th, lithic.Quiet, lithic.Icon("refresh"), "Reset")
			}),
		)
	})

	dragging := p.volume.Dragging()
	if dragging != p.volumeOn {
		e.Store.SetVolumeDragging(dragging)
		p.volumeOn = dragging
	}
	if volumeChanged {
		p.setVolume(e, p.volumeTarget)
	}

	fx := p.values()
	if reset {
		fx = model.DefaultAudioFx()
		p.setValues(fx)
		p.fxTarget, p.fxPending = fx, true
		p.fxDirty = false
		callVoid(e, protocol.MethodSetAudioFx, fx)
	} else {
		if paramsChanged {
			p.fxTarget, p.fxPending = fx, true
			callVoid(e, protocol.MethodSetPlaybackParams, model.PlaybackParams{Speed: fx.Speed, Semitones: fx.Semitones})
		}
		if effectsChanged {
			p.fxTarget, p.fxPending = fx, true
			p.fxDirty = true
			p.fxAt = gtx.Now.Add(80 * time.Millisecond)
		}
	}
	if p.fxDirty {
		if gtx.Now.Before(p.fxAt) {
			gtx.Execute(op.InvalidateCmd{At: p.fxAt})
		} else {
			p.fxDirty = false
			callVoid(e, protocol.MethodSetAudioFx, p.fxTarget)
		}
	}
	return content
}

func (p *SoundPanel) reconcile(snapshot state.State) {
	if !p.initialized {
		p.volumeTarget = max(0, min(100, snapshot.Playback.Volume))
		p.volume.Value = float32(p.volumeTarget) / 100
		p.setValues(snapshot.AudioFx)
		p.initialized = true
	}
	if p.volumePending {
		if snapshot.Playback.Volume == p.volumeTarget {
			p.volumePending = false
		} else {
			p.volume.Value = float32(p.volumeTarget) / 100
		}
	} else if !p.volume.Dragging() {
		p.volumeTarget = max(0, min(100, snapshot.Playback.Volume))
		p.volume.Value = float32(p.volumeTarget) / 100
	}
	if p.fxPending {
		if audioFxEqual(snapshot.AudioFx, p.fxTarget) {
			p.fxPending = false
		} else {
			p.setValues(p.fxTarget)
		}
	} else if !p.speed.Dragging() && !p.pitch.Dragging() && !p.reverb.Dragging() && !p.bass.Dragging() && !p.width.Dragging() {
		p.setValues(snapshot.AudioFx)
	}
}

func (p *SoundPanel) setVolume(e *env.Env, value int) {
	value = max(0, min(100, value))
	p.volumeTarget = value
	p.volumePending = true
	callVoid(e, protocol.MethodSetVolume, protocol.SetVolumeParams{Volume: value})
}

func (p *SoundPanel) values() model.AudioFx {
	return model.AudioFx{
		Speed:     0.5 + float64(p.speed.Value)*1.5,
		Semitones: int(math.Round(float64(p.pitch.Value)*24 - 12)),
		Reverb:    float64(p.reverb.Value),
		Bass:      float64(p.bass.Value)*18 - 6,
		Width:     float64(p.width.Value),
	}
}

func (p *SoundPanel) setValues(fx model.AudioFx) {
	p.speed.Value = float32((fx.Speed - 0.5) / 1.5)
	p.pitch.Value = float32(fx.Semitones+12) / 24
	p.reverb.Value = float32(fx.Reverb)
	p.bass.Value = float32((fx.Bass + 6) / 18)
	p.width.Value = float32(fx.Width)
}

func audioFxEqual(a, b model.AudioFx) bool {
	return math.Abs(a.Speed-b.Speed) < 0.0001 &&
		a.Semitones == b.Semitones &&
		math.Abs(a.Reverb-b.Reverb) < 0.0001 &&
		math.Abs(a.Bass-b.Bass) < 0.0001 &&
		math.Abs(a.Width-b.Width) < 0.0001
}

func soundSliderRow(gtx layout.Context, e *env.Env, label, value string, slider *lithic.Slider) (bool, layout.Dimensions) {
	before := slider.Value
	dims := layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, lithic.Text(e.Th, e.Th.T.Label, e.Th.R.Fg2, label)),
				layout.Rigid(lithic.Text(e.Th, e.Th.T.MonoSm, e.Th.R.Fg3, value)),
			)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Max.Y = gtx.Dp(32)
			return slider.Layout(gtx, e.Th, 0, 1)
		}),
	)
	return math.Abs(float64(slider.Value-before)) > 0.0001, dims
}

func signed(value int) string {
	if value > 0 {
		return "+" + strconv.Itoa(value)
	}
	return strconv.Itoa(value)
}

func signedFloat(value float64) string {
	if math.Abs(value) < 0.05 {
		value = 0
	}
	text := strconv.FormatFloat(value, 'f', 1, 64)
	if value > 0 {
		return "+" + text
	}
	return text
}

func percent(value float64) string {
	return strconv.Itoa(int(math.Round(value*100))) + "%"
}
