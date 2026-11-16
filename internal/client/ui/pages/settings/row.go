package settings

import (
	"image"

	"gioui.org/gesture"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/anim"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/tokens"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
)

// Row geometry, from the Hub's SettingRow (Lithic/SettingRow.qml) and the
// Lithic tokens, in dp.
const (
	// rowStackWidth is the width under which a row stacks its control
	// below its words; a wide control stacks under rowStackWideWidth.
	rowStackWidth     unit.Dp = 440
	rowStackWideWidth unit.Dp = 620
	// rowStackGap stands between the words and a stacked control.
	rowStackGap unit.Dp = 8
	// rowLeanRoom keeps the words clear of the control while the row leans
	// (look.RowWash leans 2 dp).
	rowLeanRoom unit.Dp = 2
)

// rowSpec is what one setting row shows and does.
type rowSpec struct {
	title, detail string
	// control is the row's control at the right; nil for a row of words.
	control layout.Widget
	// maxControl is the widest the control may grow; wide controls (chips)
	// take a larger share of the row and stack later.
	maxControl unit.Dp
	wide       bool
	// changed marks a row that stands away from its default: the title
	// carries "Changed" and a reset plaque shows on hover and focus.
	changed bool
	reset   func()
	// activate makes the whole row a button (a toggle row, a hand-off),
	// as the Hub's activatable rows are.
	activate func()
}

// settingRow is one row of a section's slab, drawn as the Hub draws its
// rows: a vein hairline above it unless it is the section's first, the
// title and its detail wrapping at the left (never elided), the control at
// the right, and, for a changed row, "Changed" beside the title and a reset
// plaque that shows while the row is hovered or focused. Hovered, the row's
// plane takes the Hub's wash (in 120 ms, out 240, OutCubic) bleeding 8 by 4
// past it at the small cut, and its words lean 2 dp to the right; pressed (an
// activatable row), the wash deepens. A row at rest draws no wash and asks
// for no frame.
type settingRow struct {
	spec  rowSpec
	theme *look.Theme
	first bool

	hover     gesture.Hover
	hovered   bool
	wash      look.RowWash
	row       kit.Click
	reset     kit.IconPlaque
	resetShow anim.Tween
	focus     anim.Tween
	resetIcon bool

	description string
	descFor     [2]string
	widget      layout.Widget
}

func newSettingRow() *settingRow {
	row := new(settingRow)
	row.widget = row.layoutWidget
	return row
}

// settingRow returns the widget for the next row of the section being laid
// out. Rows are kept per section in the order they are asked for, so a row
// keeps its hover and press across frames.
func (p *page) settingRow(th *look.Theme, title, detail string, control layout.Widget) layout.Widget {
	return p.row(th, rowSpec{title: title, detail: detail, control: control})
}

// row returns the widget for the next row of the section being laid out.
func (p *page) row(th *look.Theme, spec rowSpec) layout.Widget {
	rows := p.rows[p.rowSection]
	if p.rowCursor == len(rows) {
		rows = append(rows, newSettingRow())
		p.rows[p.rowSection] = rows
	}
	row := rows[p.rowCursor]
	row.first = p.rowCursor == p.slabStart
	p.rowCursor++
	if spec.maxControl == 0 {
		spec.maxControl = unit.Dp(tokens.SizeSettingsControlMax)
		if spec.wide {
			spec.maxControl = 520
		}
	}
	if spec.reset != nil {
		reset, title := spec.reset, spec.title
		spec.reset = func() { p.resetWithUndo(title, reset) }
	}
	row.spec = spec
	row.theme = th
	if row.descFor != [2]string{spec.title, spec.detail} {
		row.descFor = [2]string{spec.title, spec.detail}
		row.description = spec.title
		if spec.detail != "" {
			row.description += ". " + spec.detail
		}
	}
	return row.widget
}

func (r *settingRow) layoutWidget(gtx layout.Context) layout.Dimensions {
	return r.Layout(gtx, r.theme)
}

// Layout draws the row across the width given.
func (r *settingRow) Layout(gtx layout.Context, th *look.Theme) layout.Dimensions {
	spec := &r.spec
	r.hovered = r.hover.Update(gtx.Source)
	activatable := spec.activate != nil
	if activatable && r.row.Clicked(gtx) {
		spec.activate()
	}
	if spec.changed && spec.reset != nil && r.reset.Clicked(gtx) {
		spec.reset()
	}
	r.wash.Update(gtx, th, r.hovered, activatable && r.row.Pressed())

	width := gtx.Constraints.Max.X
	gap := 0
	if !r.first {
		gap = gtx.Dp(tokens.SizeSettingsRowGap)
	}
	textGap := gtx.Dp(th.Sp.S4)
	resetSize := gtx.Dp(tokens.SizeIconPlaqueSmall)
	resetGap := gtx.Dp(th.Sp.S2)
	reserve := 0
	if spec.changed && spec.reset != nil {
		reserve = resetSize + resetGap
	}
	lean := r.wash.Lean(gtx)

	stacked := spec.control != nil && (width < gtx.Dp(rowStackWidth) || spec.wide && width < gtx.Dp(rowStackWideWidth))
	controlMax := 0
	if spec.control != nil {
		share := 45
		if spec.wide {
			share = 58
		}
		controlMax = min(gtx.Dp(spec.maxControl), width*share/100)
		if stacked {
			controlMax = min(gtx.Dp(spec.maxControl), width)
		}
	}

	// the control first, so the words take the width it leaves
	controlDims := layout.Dimensions{}
	var controlCall op.CallOp
	if spec.control != nil {
		controlGTX := gtx
		controlGTX.Constraints = layout.Constraints{Max: image.Pt(max(0, controlMax), gtx.Constraints.Max.Y)}
		record := op.Record(gtx.Ops)
		semantic.DescriptionOp(r.description).Add(gtx.Ops)
		controlDims = spec.control(controlGTX)
		controlCall = record.Stop()
	}
	textWidth := width - reserve
	if spec.control != nil && !stacked {
		textWidth -= controlDims.Size.X + textGap
	}
	textWidth = max(0, textWidth-gtx.Dp(rowLeanRoom))
	record := op.Record(gtx.Ops)
	textDims := r.layoutText(gtx, th, textWidth)
	textCall := record.Stop()

	height := max(gtx.Dp(tokens.SizeFactsRowHeight), textDims.Size.Y, controlDims.Size.Y)
	textY := (height - textDims.Size.Y) / 2
	controlY := (height - controlDims.Size.Y) / 2
	if stacked {
		height = max(gtx.Dp(tokens.SizeFactsRowHeight), textDims.Size.Y+gtx.Dp(rowStackGap)+controlDims.Size.Y)
		textY, controlY = 0, textDims.Size.Y+gtx.Dp(rowStackGap)
	}
	box := image.Rect(0, gap, width, gap+height)
	bleed := image.Rect(box.Min.X-gtx.Dp(th.Sp.S2), box.Min.Y-gtx.Dp(th.Sp.S1), box.Max.X+gtx.Dp(th.Sp.S2), box.Max.Y+gtx.Dp(th.Sp.S1))
	cut := gtx.Dp(th.Cut.Small)

	if gap > 0 {
		line := max(1, gtx.Dp(unit.Dp(tokens.SizeHairline)))
		y := gap / 2
		kit.FillCut(gtx.Ops, image.Rect(0, y, width, y+line), 0, th.R.Vein)
	}

	area := clip.Rect(bleed).Push(gtx.Ops)
	r.hover.Add(gtx.Ops)
	r.wash.Paint(gtx, th, bleed, cut)

	rowFocused := false
	if activatable {
		rowGTX := gtx
		rowGTX.Constraints = layout.Exact(bleed.Size())
		offset := op.Offset(bleed.Min).Push(gtx.Ops)
		r.row.Layout(rowGTX, func(gtx layout.Context) layout.Dimensions {
			semantic.Button.Add(gtx.Ops)
			semantic.LabelOp(spec.title).Add(gtx.Ops)
			if spec.detail != "" {
				semantic.DescriptionOp(spec.detail).Add(gtx.Ops)
			}
			semantic.EnabledOp(gtx.Source.Enabled()).Add(gtx.Ops)
			return layout.Dimensions{Size: gtx.Constraints.Min}
		})
		offset.Pop()
		rowFocused = gtx.Focused(&r.row.Clickable)
	}

	offset := op.Offset(image.Pt(lean, gap+textY)).Push(gtx.Ops)
	textCall.Add(gtx.Ops)
	offset.Pop()

	controlX := width - controlDims.Size.X
	if spec.changed && spec.reset != nil {
		focusWithin := rowFocused || gtx.Focused(&r.reset.Clickable)
		if !r.resetIcon {
			r.resetIcon = true
			r.reset.Kind, r.reset.Small, r.reset.Icon = kit.PlaqueQuiet, true, look.Icon.Refresh
		}
		r.reset.Description = "Reset " + spec.title
		// retarget on the running leg's duration, then give the new leg its
		// own, so a reversal mid-way carries on from where it stands
		shown := r.hovered || focusWithin
		r.resetShow.Ease = anim.EaseOutCubic
		r.resetShow.Set(gtx, boolFloat(shown))
		r.resetShow.Duration = th.Motion.Move
		if shown {
			r.resetShow.Duration = th.Motion.Quick
		}
		show := r.resetShow.Value(gtx)
		if focusWithin {
			show = 1
		}
		resetY := gap + (height-resetSize)/2
		if stacked {
			resetY = gap + controlY + (controlDims.Size.Y-resetSize)/2
		}
		offset := op.Offset(image.Pt(controlX-resetGap-resetSize, resetY)).Push(gtx.Ops)
		fade := paint.PushOpacity(gtx.Ops, show)
		resetGTX := gtx
		resetGTX.Constraints = layout.Exact(image.Pt(resetSize, resetSize))
		if show <= 0 {
			resetGTX = resetGTX.Disabled()
		}
		r.reset.Layout(resetGTX, th.Theme)
		fade.Pop()
		offset.Pop()
	}

	if spec.control != nil {
		offset := op.Offset(image.Pt(controlX, gap+controlY)).Push(gtx.Ops)
		controlCall.Add(gtx.Ops)
		offset.Pop()
	}
	area.Pop()

	if activatable {
		r.focus.Ease = anim.EaseOutCubic
		r.focus.Set(gtx, boolFloat(rowFocused))
		r.focus.Duration = th.Motion.FocusOut
		if rowFocused {
			r.focus.Duration = th.Motion.FocusIn
		}
		kit.FocusRingAlpha(gtx, th.Theme, bleed, cut, r.focus.Value(gtx))
	}
	return layout.Dimensions{Size: image.Pt(width, gap+height)}
}

// layoutText draws the title (with "Changed" beside it on a changed row)
// and the detail under it, wrapping at width.
func (r *settingRow) layoutText(gtx layout.Context, th *look.Theme, width int) layout.Dimensions {
	gtx.Constraints = layout.Constraints{Max: image.Pt(width, gtx.Constraints.Max.Y)}
	title := r.layoutTitle(gtx, th, width)
	if r.spec.detail == "" {
		return title
	}
	y := title.Size.Y + gtx.Dp(tokens.SizeSettingsDetailGap)
	offset := op.Offset(image.Pt(0, y)).Push(gtx.Ops)
	detailGTX := gtx
	detailGTX.Constraints.Max.Y = max(0, gtx.Constraints.Max.Y-y)
	detail := kit.Label{Style: th.T.Detail, Color: th.R.Fg2}.Layout(detailGTX, th.Theme, r.spec.detail)
	offset.Pop()
	return layout.Dimensions{Size: image.Pt(max(title.Size.X, detail.Size.X), y+detail.Size.Y)}
}

func (r *settingRow) layoutTitle(gtx layout.Context, th *look.Theme, width int) layout.Dimensions {
	if !r.spec.changed {
		return kit.Label{Style: th.T.Row, Color: th.R.Fg}.Layout(gtx, th.Theme, r.spec.title)
	}
	markGTX := gtx
	markGTX.Constraints.Min = image.Point{}
	record := op.Record(gtx.Ops)
	mark := kit.Label{Style: th.T.Detail, Color: th.R.Fg3, MaxLines: 1}.Layout(markGTX, th.Theme, "Changed")
	markCall := record.Stop()
	space := gtx.Dp(th.Sp.S3)
	titleGTX := gtx
	titleGTX.Constraints.Max.X = max(0, width-mark.Size.X-space)
	title := kit.Label{Style: th.T.Row, Color: th.R.Fg}.Layout(titleGTX, th.Theme, r.spec.title)
	baseline := title.Size.Y - title.Baseline
	markY := baseline - (mark.Size.Y - mark.Baseline)
	offset := op.Offset(image.Pt(title.Size.X+space, markY)).Push(gtx.Ops)
	markCall.Add(gtx.Ops)
	offset.Pop()
	return layout.Dimensions{
		Size:     image.Pt(title.Size.X+space+mark.Size.X, max(title.Size.Y, markY+mark.Size.Y)),
		Baseline: title.Baseline,
	}
}

func boolFloat(on bool) float32 {
	if on {
		return 1
	}
	return 0
}
