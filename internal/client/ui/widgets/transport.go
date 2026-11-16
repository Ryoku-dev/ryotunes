package widgets

import (
	"image"
	"image/color"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/anim"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/tokens"
	"github.com/ryoku-dev/ryotunes/internal/client/state"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

// glyphPressed is a plaque glyph's scale while pressed, the Hub's .86.
const glyphPressed = .86

// Transport is the shuffle, previous, play, next and repeat row. Basic keeps
// only previous, play and next for surfaces that are not the player.
type Transport struct {
	Basic bool
	Gap   unit.Dp

	shuffle  kit.IconPlaque
	previous kit.IconPlaque
	play     PlayButton
	next     kit.IconPlaque
	repeat   kit.IconPlaque

	shuffleGlyph, previousGlyph, nextGlyph, repeatGlyph PressGlyph

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
	t.shuffle.Kind = kit.PlaqueQuiet
	if snapshot.Queue.Shuffle {
		shuffleDescription = "Shuffle on"
		t.shuffle.Kind = kit.PlaqueOn
	}
	t.shuffle.Description = shuffleDescription
	t.previous.Small = true
	t.previous.Description = "Previous"
	t.previous.Kind = kit.PlaqueQuiet
	t.next.Small = true
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
			return t.previousGlyph.Layout(gtx, e.Th, &t.previous, look.Icon.Previous)
		})
	})
	play := layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return t.playTip.Layout(gtx, e.Th, e.Overlays, playDescription, func(gtx layout.Context) layout.Dimensions {
			return t.play.Layout(gtx, e.Th, playIcon, playDescription)
		})
	})
	next := layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return t.nextTip.Layout(gtx, e.Th, e.Overlays, "Next", func(gtx layout.Context) layout.Dimensions {
			return t.nextGlyph.Layout(gtx, e.Th, &t.next, look.Icon.Next)
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
					return t.shuffleGlyph.Layout(gtx, e.Th, &t.shuffle, look.Icon.Shuffle)
				})
			}),
			spacer, previous, spacer, play, spacer, next, spacer,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return t.repeatTip.Layout(gtx, e.Th, e.Overlays, repeatDescription, func(gtx layout.Context) layout.Dimensions {
					return t.repeatGlyph.Layout(gtx, e.Th, &t.repeat, repeatIcon)
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

// PressGlyph draws an icon plaque's glyph as the Hub's icon buttons press
// it: the plaque keeps the library's hover and press (its cut grows and it
// sinks 1 dp over 90 ms), and the glyph sinks with it and shrinks to .86 of
// its size on OutCubic over th.Motion.Press. A glyph at rest asks for no
// frame; under reduced motion the press is immediate.
type PressGlyph struct {
	press anim.Tween
	ready bool
}

// Layout lays plaque out with icon as its glyph, in the plaque's own ink for
// its kind.
func (g *PressGlyph) Layout(gtx layout.Context, th *look.Theme, plaque *kit.IconPlaque, icon *widget.Icon) layout.Dimensions {
	plaque.Icon = nil
	dimensions := plaque.Layout(gtx, th.Theme)
	pressed := g.Value(gtx, th, plaque.Pressed() && !plaque.Busy)
	size := tokens.SizeIconPlaqueIcon
	if plaque.Small {
		size = tokens.SizeIconPlaqueIconSmall
	}
	ink := PlaqueInk(gtx, th, plaque.Kind)
	DrawPressedGlyph(gtx, dimensions.Size, pressed, func(gtx layout.Context) layout.Dimensions {
		side := gtx.Dp(unit.Dp(size))
		return look.DrawGlyph(gtx, icon, ink, side, 0, 1)
	}, gtx.Dp(unit.Dp(size)))
	return dimensions
}

// Value moves the press toward pressed and returns it, 0..1.
func (g *PressGlyph) Value(gtx layout.Context, th *look.Theme, pressed bool) float32 {
	g.press.Duration, g.press.Ease = th.Motion.Press, anim.EaseOutCubic
	target := float32(0)
	if pressed {
		target = 1
	}
	if !g.ready {
		g.ready = true
		g.press.Set(gtx, 0)
	}
	g.press.Set(gtx, target)
	return g.press.Value(gtx)
}

// DrawPressedGlyph centres a side-pixel glyph in a plaque of size, sunk and
// shrunk by pressed (0..1).
func DrawPressedGlyph(gtx layout.Context, size image.Point, pressed float32, glyph layout.Widget, side int) {
	sink := int(float32(gtx.Dp(unit.Dp(tokens.SizeControlPressSink)))*pressed + .5)
	at := image.Pt((size.X-side)/2, (size.Y-side)/2+sink)
	offset := op.Offset(at).Push(gtx.Ops)
	if pressed > 0 {
		scale := 1 - (1-glyphPressed)*pressed
		centre := f32.Pt(float32(side)/2, float32(side)/2)
		transform := op.Affine(f32.AffineId().Scale(centre, f32.Pt(scale, scale))).Push(gtx.Ops)
		child := gtx
		child.Constraints = layout.Exact(image.Pt(side, side))
		glyph(child)
		transform.Pop()
	} else {
		child := gtx
		child.Constraints = layout.Exact(image.Pt(side, side))
		glyph(child)
	}
	offset.Pop()
}

// PlaqueInk is the glyph ink kit gives a plaque of kind: fg2 on a quiet
// plaque, on-sel on an on plaque, on-gold on the gold one, faded when the
// plaque is disabled.
func PlaqueInk(gtx layout.Context, th *look.Theme, kind kit.PlaqueKind) color.NRGBA {
	ink := th.R.Fg2
	switch kind {
	case kit.PlaqueOn:
		ink = th.R.OnSel
	case kit.PlaqueGold:
		ink = th.R.OnGold
	case kit.PlaqueTonal:
		ink = th.R.Fg
	case kit.PlaqueLine:
		ink = th.R.GoldInk
	case kit.PlaqueDanger:
		ink = palette.InkOn(th.R.Jasper)
	case kit.PlaqueDangerLine:
		ink = th.R.Jasper
	}
	if !gtx.Source.Enabled() {
		ink = palette.SRGBAlpha(ink, th.R.Ground, tokens.AlphaDisabled)
	}
	return ink
}

// PlayButton is the gold play and pause plaque: its glyph morphs between the
// two and presses as the Hub's icon buttons do.
type PlayButton struct {
	Plaque kit.IconPlaque
	morph  look.Morph
	glyph  PressGlyph
}

// Clicked reports one activation.
func (p *PlayButton) Clicked(gtx layout.Context) bool { return p.Plaque.Clicked(gtx) }

// Layout draws the plaque with icon, the next action.
func (p *PlayButton) Layout(gtx layout.Context, th *look.Theme, icon *widget.Icon, description string) layout.Dimensions {
	p.Plaque.Small = false
	p.Plaque.Kind = kit.PlaqueGold
	p.Plaque.Icon = nil
	p.Plaque.Description = description
	dimensions := p.Plaque.Layout(gtx, th.Theme)
	pressed := p.glyph.Value(gtx, th, p.Plaque.Pressed())
	side := gtx.Dp(20)
	ink := PlaqueInk(gtx, th, kit.PlaqueGold)
	DrawPressedGlyph(gtx, dimensions.Size, pressed, func(gtx layout.Context) layout.Dimensions {
		return p.morph.Layout(gtx, th, icon, ink, 20)
	}, side)
	return dimensions
}

func transportCall(e *env.Env, method string, params any) {
	act.Call[protocol.Null](e, method, params, func(_ protocol.Null, err error) {
		if err != nil && e.Toast != nil {
			e.Toast(err.Error(), env.ToastError)
		}
	})
}
