package lithic

import (
	"image"
	"image/color"
	"math"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/text"
)

// RowContent supplies a tablet's mark, copy, state, progress, and action.
type RowContent struct {
	Mark, FactsWidget layout.Widget
	Title, Facts      string
	FactsMono         bool
	Claim, Proof      string
	Tone              Tone
	Progress          float64
	Action            layout.Widget
}

// Row is an interactive state-notched resource tablet.
type Row struct {
	Click
	Pane color.NRGBA

	hover     Tween
	press     Tween
	notch     Tween
	progress  Spring
	tone      Tone
	toneReady bool

	eventAt    time.Time
	eventReady bool
	clicked    bool
	double     bool
	clickedOut bool
	doubleOut  bool
}

func (r *Row) poll(gtx layout.Context) {
	if r.eventReady && r.eventAt == gtx.Now {
		return
	}
	r.eventAt = gtx.Now
	r.eventReady = true
	r.clicked = false
	r.double = false
	r.clickedOut = false
	r.doubleOut = false
	for {
		click, ok := r.Click.Update(gtx)
		if !ok {
			break
		}
		r.clicked = true
		if click.NumClicks >= 2 {
			r.double = true
		}
	}
}

// Clicked and DoubleClicked share one event drain, so a double click can both
// select the row and run its primary action in the same frame.
func (r *Row) Clicked(gtx layout.Context) bool {
	r.poll(gtx)
	if r.clickedOut || !r.clicked {
		return false
	}
	r.clickedOut = true
	return true
}

// DoubleClicked reports one double-click primary action.
func (r *Row) DoubleClicked(gtx layout.Context) bool {
	r.poll(gtx)
	if r.doubleOut || !r.double {
		return false
	}
	r.doubleOut = true
	return true
}

// Layout renders the tablet while the list owns its selected slab.
func (r *Row) Layout(gtx layout.Context, th *Theme, c RowContent, selected, focused bool) layout.Dimensions {
	r.poll(gtx)
	r.hover.Ease = EaseChisel
	r.notch.Duration = th.Motion.Notch
	r.notch.Ease = EaseChisel
	if !r.toneReady {
		r.tone = c.Tone
		r.toneReady = true
		r.notch.Set(gtx, 1)
	} else if r.tone != c.Tone {
		r.tone = c.Tone
		r.notch = Tween{Duration: th.Motion.Notch, Ease: EaseChisel}
		r.notch.Set(gtx, 0)
		r.notch.Set(gtx, 1)
	}

	h := gtx.Dp(th.RowHeight())
	w := gtx.Constraints.Max.X
	if w < gtx.Constraints.Min.X {
		w = gtx.Constraints.Min.X
	}
	rowCtx := gtx
	rowCtx.Constraints = layout.Exact(image.Pt(w, h))
	dims := r.Click.Layout(rowCtx, func(gtx layout.Context) layout.Dimensions {
		hot := r.Hovered() || focused
		hover := hoverValue(gtx, th, &r.hover, hot)
		pressed := transitionValue(gtx, &r.press, r.Pressed(), th.Motion.Press, th.Motion.Press)
		if !selected {
			fill := Mix(th.R.Stone2, th.R.Stone3, hover)
			fill = Mix(fill, th.R.Fg, .06*pressed)
			FillCut(gtx.Ops, image.Rect(0, 0, w, h), 0, fill)
		} else if pressed > 0 {
			FillCut(gtx.Ops, image.Rect(0, 0, w, h), 0, WithAlpha(th.R.OnSel, .08*pressed))
		}

		notchColor := th.R.Stone(c.Tone)
		if c.Tone == Bad {
			notchColor = th.R.Jasper
		}
		if c.Tone == Idle {
			notchColor = r.Pane
			if notchColor == (color.NRGBA{}) {
				notchColor = th.R.Stone1
			}
		}
		notchSize := int(math.Round(float64(gtx.Dp(th.Cut.Notch)) * float64(r.notch.Value(gtx))))
		if notchSize > 0 {
			Notch(gtx.Ops, image.Point{}, notchSize, notchColor)
		}

		titleInk, quietInk := th.R.Fg, th.R.Fg3
		claimInk := th.R.Stone(c.Tone)
		if c.Tone == Idle {
			claimInk = th.R.Fg2
		}
		if selected {
			titleInk, quietInk, claimInk = th.R.OnSel, th.R.OnSel2, th.R.OnSel
		}

		body := func(gtx layout.Context) layout.Dimensions {
			mark := func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints = layout.Exact(image.Pt(gtx.Dp(32), gtx.Dp(32)))
				if c.Mark == nil {
					return layout.Dimensions{Size: gtx.Constraints.Min}
				}
				return layout.Center.Layout(gtx, c.Mark)
			}
			copyBlock := func(gtx layout.Context) layout.Dimensions {
				children := []layout.FlexChild{
					layout.Rigid(Text(th, th.T.Row, titleInk, c.Title)),
				}
				style := th.T.Caption
				if c.FactsMono {
					style = th.T.Mono
				}
				if c.FactsWidget != nil {
					children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						slotGTX := gtx
						slotGTX.Constraints.Min = image.Point{}

						var measureOps op.Ops
						measureGTX := slotGTX
						measureGTX.Ops = &measureOps
						reference := rawText(th, style, quietInk, "Ag")(measureGTX)
						slotGTX.Constraints.Max.Y = min(slotGTX.Constraints.Max.Y, reference.Size.Y)

						record := op.Record(gtx.Ops)
						dims := c.FactsWidget(slotGTX)
						call := record.Stop()
						dims.Size = slotGTX.Constraints.Constrain(dims.Size)
						baselineY := reference.Size.Y - reference.Baseline
						childBaselineY := dims.Size.Y - dims.Baseline
						y := baselineY - childBaselineY
						stack := clip.Rect(image.Rect(0, 0, slotGTX.Constraints.Max.X, reference.Size.Y)).Push(gtx.Ops)
						offset := op.Offset(image.Pt(0, y)).Push(gtx.Ops)
						call.Add(gtx.Ops)
						offset.Pop()
						stack.Pop()
						return layout.Dimensions{
							Size:     image.Pt(dims.Size.X, reference.Size.Y),
							Baseline: reference.Baseline,
						}
					}))
				} else if c.Facts != "" {
					children = append(children, layout.Rigid(Text(th, style, quietInk, c.Facts)))
				}
				return layout.Flex{Axis: layout.Vertical, Alignment: layout.Start}.Layout(gtx, children...)
			}
			trailing := func(gtx layout.Context) layout.Dimensions {
				if hot && c.Action != nil {
					return c.Action(gtx)
				}
				gtx.Constraints.Min.X = min(gtx.Constraints.Max.X, gtx.Dp(72))
				return layout.Flex{Axis: layout.Vertical, Alignment: layout.End}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return Label{Style: th.T.Label, Color: claimInk, MaxLines: 1, Align: text.End}.Layout(gtx, th, c.Claim)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return Label{Style: th.T.MonoSm, Color: quietInk, MaxLines: 1, Align: text.End}.Layout(gtx, th, c.Proof)
					}),
				)
			}
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(mark),
				layout.Rigid(layout.Spacer{Width: th.Sp.S3}.Layout),
				layout.Flexed(1, copyBlock),
				layout.Rigid(layout.Spacer{Width: th.Sp.S3}.Layout),
				layout.Rigid(trailing),
			)
		}
		layout.Inset{Left: 16, Right: 12}.Layout(gtx, body)

		if c.Progress >= 0 {
			r.progress.Omega = 4 / float32(th.Motion.Select.Seconds())
			r.progress.Rest = .001
			r.progress.Set(float32(max(0, min(1, c.Progress))))
			frac := r.progress.Value(gtx)
			bladeW := int(math.Round(float64(w) * float64(frac)))
			if bladeW > 0 {
				Blade(gtx.Ops, image.Rect(0, h-max(1, gtx.Dp(2)), bladeW, h), th.R.Lapis)
			}
		}
		return layout.Dimensions{Size: image.Pt(w, h)}
	})
	return dims
}
