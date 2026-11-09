package lithic

import (
	"image"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"
)

// TextField is a labelled cut editor with help, error, secret, and mono modes.
type TextField struct {
	widget.Editor
	Secret, Mono bool
	reveal       IconButton
	revealed     bool
	focus        Tween
}

// Layout renders the editor and its optional supporting text.
func (f *TextField) Layout(gtx layout.Context, th *Theme, label, placeholder, help, errText string) layout.Dimensions {
	if f.reveal.Clicked(gtx) {
		f.revealed = !f.revealed
	}
	f.SingleLine = true
	f.Mask = 0
	if f.Secret && !f.revealed {
		f.Mask = '•'
	}
	style := th.T.Body
	if f.Mono {
		style = th.T.Mono
	}

	children := make([]layout.FlexChild, 0, 5)
	if label != "" {
		children = append(children,
			layout.Rigid(Text(th, th.T.Caption, disabledInk(gtx, th.R.Fg3), label)),
			layout.Rigid(layout.Spacer{Height: 4}.Layout),
		)
	}
	children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		w := gtx.Constraints.Max.X
		if w <= 0 {
			w = gtx.Dp(240)
		}
		h := gtx.Dp(44)
		size := gtx.Constraints.Constrain(image.Pt(w, h))
		r := image.Rectangle{Max: size}
		cut := gtx.Dp(th.Cut.Mid)
		FillCut(gtx.Ops, r, cut, disabledInk(gtx, th.R.Stone3))
		outline := th.R.Fg2
		alpha := focusValue(gtx, th, &f.focus, gtx.Focused(&f.Editor))
		if errText != "" {
			outline = th.R.JasperInk
			alpha = 1
		}
		if alpha > 0 {
			StrokeCut(gtx.Ops, r, cut, max(1, gtx.Dp(1)), disabledInk(gtx, WithAlpha(outline, alpha)))
		}

		innerWidth := max(0, size.X-gtx.Dp(28))
		if f.Secret {
			innerWidth = max(0, innerWidth-gtx.Dp(36))
		}
		editorHeight := max(0, size.Y-gtx.Dp(16))
		editorGTX := exact(gtx, image.Pt(innerWidth, editorHeight))
		offset := op.Offset(image.Pt(gtx.Dp(14), (size.Y-editorHeight)/2)).Push(gtx.Ops)
		layoutEditor(editorGTX, th, &f.Editor, style, placeholder)
		offset.Pop()

		if f.Secret {
			icon, desc := Icon("visibility"), "Show value"
			if f.revealed {
				icon, desc = Icon("visibility_off"), "Hide value"
			}
			f.reveal.Small = true
			buttonSize := gtx.Dp(32)
			offset := op.Offset(image.Pt(size.X-buttonSize-gtx.Dp(6), (size.Y-buttonSize)/2)).Push(gtx.Ops)
			f.reveal.Layout(exact(gtx, image.Pt(buttonSize, buttonSize)), th, Quiet, icon, desc)
			offset.Pop()
		}
		return layout.Dimensions{Size: size}
	}))

	message, messageColor := help, th.R.Fg3
	if errText != "" {
		message, messageColor = errText, th.R.JasperInk
	}
	if message != "" {
		children = append(children,
			layout.Rigid(layout.Spacer{Height: 4}.Layout),
			layout.Rigid(Text(th, th.T.Caption, disabledInk(gtx, messageColor), message)),
		)
	}
	return layout.Flex{Axis: layout.Vertical, Alignment: layout.Start}.Layout(gtx, children...)
}

func layoutEditor(gtx layout.Context, th *Theme, editor *widget.Editor, style TextStyle, placeholder string) layout.Dimensions {
	size := gtx.Constraints.Constrain(gtx.Constraints.Max)
	lineGTX := gtx
	lineGTX.Constraints.Min = image.Pt(size.X, 0)
	lineGTX.Constraints.Max = size

	editorRecord := op.Record(gtx.Ops)
	textCall := textMaterial(gtx.Ops, disabledInk(gtx, th.R.Fg))
	selectionCall := textMaterial(gtx.Ops, disabledInk(gtx, th.R.Sel))
	editor.LineHeight = 0
	editorDims := editor.Layout(lineGTX, th.Shaper, style.Face, style.Size, textCall, selectionCall)
	editorCall := editorRecord.Stop()

	editorY := max(0, (size.Y-editorDims.Size.Y)/2)
	offset := op.Offset(image.Pt(0, editorY)).Push(gtx.Ops)
	editorCall.Add(gtx.Ops)
	offset.Pop()

	if editor.Len() == 0 {
		hintGTX := gtx
		hintGTX.Constraints.Min = image.Point{}
		hintGTX.Constraints.Max = size
		hintRecord := op.Record(gtx.Ops)
		hintDims := rawText(th, style, disabledInk(gtx, th.R.Fg3), placeholder)(hintGTX)
		hintCall := hintRecord.Stop()
		hintY := max(0, (size.Y-hintDims.Size.Y)/2)
		offset = op.Offset(image.Pt(0, hintY)).Push(gtx.Ops)
		hintCall.Add(gtx.Ops)
		offset.Pop()
	}
	baseline := size.Y - editorY - editorDims.Size.Y + editorDims.Baseline
	return layout.Dimensions{Size: size, Baseline: baseline}
}
