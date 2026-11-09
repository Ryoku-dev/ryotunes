package widgets

import (
	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryotunes/internal/client/state"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

// Transport is the shuffle, previous, play, next and repeat row. Basic keeps
// only previous, play and next for surfaces that are not the player.
type Transport struct {
	Basic bool
	Gap   unit.Dp

	shuffle  kit.IconPlaque
	previous kit.IconPlaque
	play     kit.IconPlaque
	next     kit.IconPlaque
	repeat   kit.IconPlaque
	morph    look.Morph

	shuffleTip  look.Tip
	previousTip look.Tip
	playTip     look.Tip
	nextTip     look.Tip
	repeatTip   look.Tip
}

// Layout draws the row centred in its constraints and runs the clicked action.
func (t *Transport) Layout(gtx layout.Context, e *env.Env, snapshot state.State) layout.Dimensions {
	gap := t.Gap
	if gap <= 0 {
		gap = 13
	}
	shuffleDescription := "Shuffle"
	t.shuffle.Small = true
	t.shuffle.Icon = look.Icon.Shuffle
	t.shuffle.Kind = kit.PlaqueQuiet
	if snapshot.Queue.Shuffle {
		shuffleDescription = "Shuffle on"
		t.shuffle.Kind = kit.PlaqueOn
	}
	t.shuffle.Description = shuffleDescription
	t.previous.Small = true
	t.previous.Icon = look.Icon.Previous
	t.previous.Description = "Previous"
	t.previous.Kind = kit.PlaqueQuiet
	t.next.Small = true
	t.next.Icon = look.Icon.Next
	t.next.Description = "Next"
	t.next.Kind = kit.PlaqueQuiet

	repeatIcon := look.Icon.Repeat
	repeatDescription := "Repeat off"
	t.repeat.Kind = kit.PlaqueQuiet
	if snapshot.Queue.Repeat == model.RepeatAll {
		t.repeat.Kind = kit.PlaqueOn
		repeatDescription = "Repeat all"
	} else if snapshot.Queue.Repeat == model.RepeatOne {
		t.repeat.Kind = kit.PlaqueOn
		repeatIcon = look.Icon.RepeatOne
		repeatDescription = "Repeat one"
	}
	t.repeat.Small = true
	t.repeat.Icon = repeatIcon
	t.repeat.Description = repeatDescription

	playIcon := look.Icon.Play
	playDescription := "Play"
	if !snapshot.Playback.Paused {
		playIcon = look.Icon.Pause
		playDescription = "Pause"
	}

	spacer := layout.Rigid(layout.Spacer{Width: gap}.Layout)
	previous := layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return t.previousTip.Layout(gtx, e.Th, e.Overlays, "Previous", func(gtx layout.Context) layout.Dimensions {
			return t.previous.Layout(gtx, e.Th.Theme)
		})
	})
	play := layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return t.playTip.Layout(gtx, e.Th, e.Overlays, playDescription, func(gtx layout.Context) layout.Dimensions {
			return PlayPlaque(gtx, e.Th, &t.play, &t.morph, playIcon, playDescription)
		})
	})
	next := layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return t.nextTip.Layout(gtx, e.Th, e.Overlays, "Next", func(gtx layout.Context) layout.Dimensions {
			return t.next.Layout(gtx, e.Th.Theme)
		})
	})
	dimensions := layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		row := layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}
		if t.Basic {
			return row.Layout(gtx, previous, spacer, play, spacer, next)
		}
		return row.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return t.shuffleTip.Layout(gtx, e.Th, e.Overlays, shuffleDescription, func(gtx layout.Context) layout.Dimensions {
					return t.shuffle.Layout(gtx, e.Th.Theme)
				})
			}),
			spacer, previous, spacer, play, spacer, next, spacer,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return t.repeatTip.Layout(gtx, e.Th, e.Overlays, repeatDescription, func(gtx layout.Context) layout.Dimensions {
					return t.repeat.Layout(gtx, e.Th.Theme)
				})
			}),
		)
	})

	if !t.Basic && t.shuffle.Clicked(gtx) {
		transportCall(e, protocol.MethodToggleShuffle, protocol.Null{})
	}
	if t.previous.Clicked(gtx) {
		transportCall(e, protocol.MethodPrevTrack, protocol.Null{})
	}
	if t.play.Clicked(gtx) {
		transportCall(e, protocol.MethodTogglePause, protocol.Null{})
	}
	if t.next.Clicked(gtx) {
		transportCall(e, protocol.MethodNextTrack, protocol.Null{})
	}
	if !t.Basic && t.repeat.Clicked(gtx) {
		next := model.RepeatAll
		if snapshot.Queue.Repeat == model.RepeatAll {
			next = model.RepeatOne
		} else if snapshot.Queue.Repeat == model.RepeatOne {
			next = model.RepeatOff
		}
		transportCall(e, protocol.MethodSetRepeat, protocol.SetRepeatParams{Mode: next})
	}
	return dimensions
}

// PlayPlaque is the gold play and pause plaque whose icon morphs between the two.
func PlayPlaque(gtx layout.Context, th *look.Theme, plaque *kit.IconPlaque, morph *look.Morph, icon *widget.Icon, description string) layout.Dimensions {
	plaque.Small = false
	plaque.Kind = kit.PlaqueGold
	plaque.Icon = nil
	plaque.Description = description
	return layout.Stack{Alignment: layout.Center}.Layout(gtx,
		layout.Stacked(func(gtx layout.Context) layout.Dimensions { return plaque.Layout(gtx, th.Theme) }),
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return morph.Layout(gtx, th, icon, th.R.OnGold, unit.Dp(20))
			})
		}),
	)
}

func transportCall(e *env.Env, method string, params any) {
	act.Call[protocol.Null](e, method, params, func(_ protocol.Null, err error) {
		if err != nil && e.Toast != nil {
			e.Toast(err.Error(), env.ToastError)
		}
	})
}
