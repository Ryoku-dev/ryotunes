package chrome

import (
	"math"
	"strconv"
	"time"

	"gioui.org/layout"
	"gioui.org/op"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

const volumeSendInterval = 50 * time.Millisecond

type volumeControl struct {
	icon     kit.IconPlaque
	iconTip  look.Tip
	scrubber look.Scrubber

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
	v.icon.Small = true
	v.icon.Kind = kit.PlaqueQuiet
	v.icon.Icon = look.Icon.VolumeUp
	v.icon.Description = volumeTip
	switch {
	case v.target == 0:
		volumeTip = "Unmute"
		v.icon.Icon = look.Icon.VolumeOff
		v.icon.Description = volumeTip
	case v.target < 34:
		v.icon.Icon = look.Icon.VolumeMute
	case v.target < 67:
		v.icon.Icon = look.Icon.VolumeDown
	}

	dimensions := layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return v.iconTip.Layout(gtx, e.Th, e.Overlays, volumeTip, func(gtx layout.Context) layout.Dimensions {
				return v.icon.Layout(gtx, e.Th.Theme)
			})
		}),
		layout.Rigid(layout.Spacer{Width: 4}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			width = max(0, min(width, gtx.Constraints.Max.X))
			gtx.Constraints.Min.X, gtx.Constraints.Max.X = width, width
			return v.scrubber.Layout(gtx, e.Th, float32(v.target)/100, look.ScrubberStyle{
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
