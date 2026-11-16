package chrome

import (
	"math"
	"strconv"
	"time"

	"gioui.org/layout"
	"gioui.org/op"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/anim"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/widgets"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

const volumeSendInterval = 50 * time.Millisecond

// volumeControl is the mute plaque and the volume scrubber. What the hand
// does on the scrubber is exact (owner ruling: easy to drag precisely): a
// drag, a click, arrow keys and the wheel show their value at once. A
// change that comes from anywhere else (mute, another client, the daemon)
// is chased: the fill and handle follow the new volume as the Hub's Chase
// follows, never overshooting, still once it lands.
type volumeControl struct {
	icon      kit.IconPlaque
	iconGlyph widgets.PressGlyph
	iconTip   look.Tip
	scrubber  look.Scrubber
	shown     anim.Chase
	placed    bool

	target   int
	preMute  int
	pending  bool
	dragging bool
	queued   bool
	lastSent time.Time
	sendAt   time.Time
}

func (v *volumeControl) Layout(gtx layout.Context, e *env.Env, width int) layout.Dimensions {
	v.reconcile(e)
	volumeTip := "Mute"
	icon := look.Icon.VolumeUp
	v.icon.Small = true
	v.icon.Kind = kit.PlaqueQuiet
	switch {
	case v.target == 0:
		volumeTip = "Unmute"
		icon = look.Icon.VolumeOff
	case v.target < 34:
		icon = look.Icon.VolumeMute
	case v.target < 67:
		icon = look.Icon.VolumeDown
	}
	v.icon.Description = volumeTip

	shown := v.shownValue(gtx)
	dimensions := layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return v.iconTip.Layout(gtx, e.Th, e.Overlays, volumeTip, func(gtx layout.Context) layout.Dimensions {
				return v.iconGlyph.Layout(gtx, e.Th, &v.icon, icon)
			})
		}),
		layout.Rigid(layout.Spacer{Width: 4}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			width = max(0, min(width, gtx.Constraints.Max.X))
			gtx.Constraints.Min.X, gtx.Constraints.Max.X = width, width
			return v.scrubber.Layout(gtx, e.Th, shown, look.ScrubberStyle{
				Fill:         e.Th.R.Fg2,
				AlwaysHandle: true,
				Bubble: func(fraction float32) string {
					return strconv.Itoa(int(math.Round(float64(fraction*100)))) + "%"
				},
				WheelStep: .05,
			})
		}),
	)

	if v.icon.Clicked(gtx) {
		if v.target == 0 {
			if v.preMute <= 0 {
				v.preMute = 70
			}
			v.commit(e, v.preMute)
		} else {
			v.preMute = v.target
			v.commit(e, 0)
		}
	}

	if fraction, dragging := v.scrubber.Dragging(); dragging {
		value := int(math.Round(float64(fraction * 100)))
		v.target = max(0, min(100, value))
		v.pending = true
		if !v.dragging {
			v.dragging = true
			e.Store.SetVolumeDragging(true)
		}
		v.queueSend(gtx, e)
	} else if v.dragging {
		v.dragging = false
		v.queued = false
		e.Store.SetVolumeDragging(false)
	}
	if fraction, committed := v.scrubber.Committed(); committed {
		v.commit(e, int(math.Round(float64(fraction*100))))
		// the hand's own change (a click, a key, a settled wheel) stands at
		// once: the next key steps from it and the wheel never glides back
		v.shown.Snap(float32(v.target) / 100)
	}
	if v.dragging {
		// the pointer's own value stands; a chase would trail the hand
		v.shown.Snap(float32(v.target) / 100)
	}
	if v.queued {
		if !gtx.Now.Before(v.sendAt) {
			v.sendVolume(e)
		} else {
			gtx.Execute(op.InvalidateCmd{At: v.sendAt})
		}
	}
	return dimensions
}

// shownValue is the scrubber's value this frame, 0..1: the target itself
// while the pointer drags or on the first frame, else the chase toward it.
func (v *volumeControl) shownValue(gtx layout.Context) float32 {
	target := float32(v.target) / 100
	if !v.placed || v.dragging {
		v.placed = true
		v.shown.Snap(target)
		return target
	}
	v.shown.Set(target)
	return v.shown.Value(gtx)
}

func (v *volumeControl) reconcile(e *env.Env) {
	volume := max(0, min(100, e.Store.Snapshot().Playback.Volume))
	if v.target == 0 && v.preMute == 0 && !v.pending {
		v.target = volume
		if volume > 0 {
			v.preMute = volume
		}
	}
	if v.pending {
		if volume == v.target {
			v.pending = false
		}
		return
	}
	if !v.dragging {
		v.target = volume
		if volume > 0 {
			v.preMute = volume
		}
	}
}

func (v *volumeControl) queueSend(gtx layout.Context, e *env.Env) {
	if v.lastSent.IsZero() || !gtx.Now.Before(v.lastSent.Add(volumeSendInterval)) {
		v.sendVolume(e)
		return
	}
	v.queued = true
	v.sendAt = v.lastSent.Add(volumeSendInterval)
	gtx.Execute(op.InvalidateCmd{At: v.sendAt})
}

func (v *volumeControl) sendVolume(e *env.Env) {
	v.queued = false
	v.lastSent = time.Now()
	callVoid(e, protocol.MethodSetVolume, protocol.SetVolumeParams{Volume: v.target})
}

func (v *volumeControl) commit(e *env.Env, value int) {
	v.target = max(0, min(100, value))
	v.pending = true
	v.sendVolume(e)
	callVoid(e, protocol.MethodSetSetting, protocol.SetSettingParams{
		Key: string(model.SettingVolume), Value: strconv.Itoa(v.target),
	})
}
