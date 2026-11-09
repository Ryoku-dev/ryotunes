package lithic

import (
	"image"
	"strconv"

	"gioui.org/layout"
)

// Stepper changes a bounded integer with two small icon plaques.
type Stepper struct {
	Value, Min, Max, Step int
	minus, plus           IconButton
	changed               bool
}

func (s *Stepper) clamp() {
	if s.Max < s.Min {
		s.Max = s.Min
	}
	s.Value = min(max(s.Value, s.Min), s.Max)
}

// Layout renders the bounded value and controls.
func (s *Stepper) Layout(gtx layout.Context, th *Theme, unit string) layout.Dimensions {
	gtx.Constraints.Min = image.Point{}
	s.clamp()
	step := s.Step
	if step <= 0 {
		step = 1
	}
	if s.minus.Clicked(gtx) && s.Value > s.Min {
		s.Value = max(s.Min, s.Value-step)
		s.changed = true
	}
	if s.plus.Clicked(gtx) && s.Value < s.Max {
		s.Value = min(s.Max, s.Value+step)
		s.changed = true
	}
	minusGTX, plusGTX := gtx, gtx
	if s.Value <= s.Min {
		minusGTX = minusGTX.Disabled()
	}
	if s.Value >= s.Max {
		plusGTX = plusGTX.Disabled()
	}
	s.minus.Small = true
	s.plus.Small = true
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Gap: gtx.Dp(8)}.Layout(gtx,
		layout.Rigid(func(layout.Context) layout.Dimensions {
			return s.minus.Layout(minusGTX, th, Quiet, Icon("remove"), "Decrease")
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Baseline, Gap: gtx.Dp(4)}.Layout(gtx,
				layout.Rigid(rawText(th, th.T.Mono, disabledInk(gtx, th.R.Fg), strconv.Itoa(s.Value))),
				layout.Rigid(rawText(th, th.T.Caption, disabledInk(gtx, th.R.Fg3), unit)),
			)
		}),
		layout.Rigid(func(layout.Context) layout.Dimensions {
			return s.plus.Layout(plusGTX, th, Quiet, Icon("add"), "Increase")
		}),
	)
}

// Changed reports and clears a value change.
func (s *Stepper) Changed() bool {
	changed := s.changed
	s.changed = false
	return changed
}
