package lithic

import (
	"image"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"
)

// Search is a leading-icon field with Escape and clear behaviour.
type Search struct {
	widget.Editor
	clear IconButton
	focus Tween
}

// Focus requests keyboard focus for the search editor.
func (s *Search) Focus(gtx layout.Context) {
	gtx.Execute(key.FocusCmd{Tag: &s.Editor})
}

// Layout renders the search field, hint, and clear plaque.
func (s *Search) Layout(gtx layout.Context, th *Theme, placeholder, hint string) layout.Dimensions {
	s.SingleLine = true
	for {
		e, ok := gtx.Event(key.Filter{Focus: &s.Editor, Name: key.NameEscape})
		if !ok {
			break
		}
		if event, ok := e.(key.Event); ok && event.State == key.Press {
			s.SetText("")
		}
	}
	if s.clear.Clicked(gtx) {
		s.SetText("")
		s.Focus(gtx)
	}

	w := gtx.Constraints.Max.X
	if w <= 0 {
		w = gtx.Dp(280)
	}
	h := gtx.Dp(44)
	size := gtx.Constraints.Constrain(image.Pt(w, h))
	r := image.Rectangle{Max: size}
	cut := gtx.Dp(th.Cut.Mid)
	FillCut(gtx.Ops, r, cut, disabledInk(gtx, th.R.Stone3))
	focus := focusValue(gtx, th, &s.focus, gtx.Focused(&s.Editor))
	if focus > 0 {
		StrokeCut(gtx.Ops, r, cut, max(1, gtx.Dp(1)), disabledInk(gtx, WithAlpha(th.R.Fg2, focus)))
	}

	iconSize := gtx.Dp(20)
	offset := op.Offset(image.Pt(gtx.Dp(14), (size.Y-iconSize)/2)).Push(gtx.Ops)
	Icon("search").Layout(exact(gtx, image.Pt(iconSize, iconSize)), disabledInk(gtx, th.R.Fg3))
	offset.Pop()

	right := gtx.Dp(14)
	if s.Len() > 0 {
		buttonSize := gtx.Dp(32)
		right = buttonSize + gtx.Dp(10)
		s.clear.Small = true
		offset = op.Offset(image.Pt(size.X-buttonSize-gtx.Dp(6), (size.Y-buttonSize)/2)).Push(gtx.Ops)
		s.clear.Layout(exact(gtx, image.Pt(buttonSize, buttonSize)), th, Quiet, Icon("close"), "Clear search")
		offset.Pop()
	} else if hint != "" && !gtx.Focused(&s.Editor) {
		measureGTX := gtx
		measureGTX.Constraints.Min = image.Point{}
		m := op.Record(gtx.Ops)
		hintDims := rawText(th, th.T.MonoSm, disabledInk(measureGTX, th.R.Fg3), hint)(measureGTX)
		hintCall := m.Stop()
		right = hintDims.Size.X + gtx.Dp(18)
		offset = op.Offset(image.Pt(size.X-right+gtx.Dp(4), (size.Y-hintDims.Size.Y)/2)).Push(gtx.Ops)
		hintCall.Add(gtx.Ops)
		offset.Pop()
	}

	innerWidth := max(0, size.X-gtx.Dp(46)-right)
	editorHeight := max(0, size.Y-gtx.Dp(16))
	editorGTX := exact(gtx, image.Pt(innerWidth, editorHeight))
	offset = op.Offset(image.Pt(gtx.Dp(42), (size.Y-editorHeight)/2)).Push(gtx.Ops)
	layoutEditor(editorGTX, th, &s.Editor, th.T.Body, placeholder)
	offset.Pop()
	return layout.Dimensions{Size: size}
}
