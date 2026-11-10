package widgets

import (
	"image"
	"strings"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/anim"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/theme"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/model"
)

// HeroContent describes one album, artist, playlist, or top-result header.
type HeroContent struct {
	Art              string
	Octagon, Collage bool
	Kind, Title      string
	Meta             []kit.Span
	MetaAction       func()
	Acts             layout.Widget
}

// Hero is a retained cover-tinted page header.
type Hero struct {
	art        look.ArtView
	meta       kit.Click
	visualizer look.Visualizer
	facts      []heroFact
	factCalls  []op.CallOp

	tintFrom, tintTo look.Accent
	tint             look.Accent
	tintTween        anim.Tween
	tintRoles        palette.Roles
	tintSet          bool
	backdrop         look.Backdrop
}

func heroArtSize(width unit.Dp) unit.Dp {
	switch {
	case width >= 760:
		return 200
	case width >= 560:
		return 160
	default:
		return 120
	}
}

type heroFact struct {
	start, end int
	width      int
	height     int
	row        int
	x          int
	separatorX int
}

func metaSeparator(span kit.Span) bool {
	return strings.TrimSpace(span.Text) == "·"
}

func (h *Hero) prepareFacts(meta []kit.Span) []heroFact {
	h.facts = h.facts[:0]
	for index := 0; index < len(meta); {
		for index < len(meta) && metaSeparator(meta[index]) {
			index++
		}
		start := index
		for index < len(meta) && !metaSeparator(meta[index]) {
			index++
		}
		if start < index {
			h.facts = append(h.facts, heroFact{start: start, end: index})
		}
	}
	return h.facts
}

func heroFactLayout(gtx layout.Context, e *env.Env, meta []kit.Span, fact heroFact) layout.Dimensions {
	return kit.Sentence(gtx, e.Th.Theme, e.Th.T.Proof, e.Th.R.Fg2, meta[fact.start:fact.end])
}

func (h *Hero) layoutMeta(gtx layout.Context, e *env.Env, content HeroContent) layout.Dimensions {
	facts := h.prepareFacts(content.Meta)
	if len(facts) == 0 || gtx.Constraints.Max.X <= 0 {
		return layout.Dimensions{}
	}
	if cap(h.factCalls) < len(facts) {
		h.factCalls = make([]op.CallOp, len(facts))
	} else {
		h.factCalls = h.factCalls[:len(facts)]
	}
	measure := gtx
	measure.Constraints.Min = image.Point{}
	measure.Constraints.Max.X = 1 << 20
	for index := range facts {
		record := op.Record(gtx.Ops)
		fact := facts[index]
		var dimensions layout.Dimensions
		if index == 0 && content.MetaAction != nil {
			dimensions = h.meta.Layout(measure, func(gtx layout.Context) layout.Dimensions {
				return heroFactLayout(gtx, e, content.Meta, fact)
			})
		} else {
			dimensions = heroFactLayout(measure, e, content.Meta, fact)
		}
		h.factCalls[index] = record.Stop()
		facts[index].width, facts[index].height = dimensions.Size.X, dimensions.Size.Y
	}
	separatorRecord := op.Record(gtx.Ops)
	separatorDimensions := kit.Label{Style: e.Th.T.Proof, Color: e.Th.R.Fg2, MaxLines: 1}.Layout(measure, e.Th.Theme, "·")
	separator := separatorRecord.Stop()
	gap := gtx.Dp(4)
	maxWidth := gtx.Constraints.Max.X
	rowWidths := [2]int{}
	rowHeights := [2]int{}
	row := 0
	visible := 0
	for index := range facts {
		fact := &facts[index]
		required := fact.width
		if rowWidths[row] > 0 {
			required += gap + separatorDimensions.Size.X + gap
		}
		if required > maxWidth-rowWidths[row] {
			if row == 1 {
				break
			}
			row = 1
			required = fact.width
		}
		if fact.width > maxWidth {
			break
		}
		fact.row = row
		fact.x = rowWidths[row]
		fact.separatorX = -1
		if rowWidths[row] > 0 {
			fact.separatorX = rowWidths[row] + gap
			fact.x += gap + separatorDimensions.Size.X + gap
		}
		rowWidths[row] = fact.x + fact.width
		rowHeights[row] = max(rowHeights[row], fact.height)
		visible++
	}
	lineGap := 0
	if rowHeights[1] > 0 {
		lineGap = gtx.Dp(4)
	}
	secondY := rowHeights[0] + lineGap
	for index := range visible {
		fact := facts[index]
		y := 0
		if fact.row == 1 {
			y = secondY
		}
		if fact.separatorX >= 0 {
			offset := op.Offset(image.Pt(fact.separatorX, y+(rowHeights[fact.row]-separatorDimensions.Size.Y)/2)).Push(gtx.Ops)
			separator.Add(gtx.Ops)
			offset.Pop()
		}
		offset := op.Offset(image.Pt(fact.x, y+(rowHeights[fact.row]-fact.height)/2)).Push(gtx.Ops)
		h.factCalls[index].Add(gtx.Ops)
		offset.Pop()
	}
	return layout.Dimensions{Size: image.Pt(max(rowWidths[0], rowWidths[1]), secondY+rowHeights[1])}
}

func heroArtURL(content HeroContent) string {
	url := strings.TrimSpace(content.Art)
	if content.Collage && !strings.HasPrefix(url, model.CollageThumbnailPrefix) && strings.HasPrefix(url, "[") {
		return model.CollageThumbnailPrefix + url
	}
	return url
}

func heroNameStyle(width unit.Dp, th *look.Theme) theme.TextStyle {
	if width < 640 || th.Class == look.Compact {
		return th.T.Sentence
	}
	return th.T.Name
}

func (h *Hero) resolveTint(gtx layout.Context, e *env.Env, rawURL string) look.Accent {
	roles := e.Th.LoadedRoles()
	if h.tintRoles != roles {
		h.tintRoles = roles
		h.tintSet = false
	}
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" || e.Art == nil {
		h.tintSet = false
		h.tint = e.Th.Accent
		return h.tint
	}
	coverPalette, ready := e.Art.Palette(rawURL)
	if !ready {
		if !h.tintSet {
			h.tint = e.Th.Accent
			h.tintFrom, h.tintTo = h.tint, h.tint
			h.tintTween = anim.Tween{Duration: e.Th.Motion.Field, Ease: anim.EaseChisel}
			h.tintSet = true
		}
		return h.advanceTint(gtx)
	}
	target := e.Th.Accent
	if coverPalette.Vibrant.A != 0 || coverPalette.Dominant.A != 0 {
		target = e.Th.ArtAccent(coverPalette)
	}
	if !h.tintSet {
		h.tint, h.tintFrom, h.tintTo = target, target, target
		h.tintTween = anim.Tween{Duration: e.Th.Motion.Field, Ease: anim.EaseChisel}
		h.tintSet = true
		return h.tint
	}
	h.advanceTint(gtx)
	if h.tintTo == target {
		return h.tint
	}
	h.tintFrom, h.tintTo = h.tint, target
	h.tintTween = anim.Tween{Duration: e.Th.Motion.Field, Ease: anim.EaseChisel}
	h.tintTween.Set(gtx, 0)
	h.tintTween.Set(gtx, 1)
	return h.advanceTint(gtx)
}

func (h *Hero) advanceTint(gtx layout.Context) look.Accent {
	progress := h.tintTween.Value(gtx)
	h.tint = look.Accent{
		Fill:  palette.Mix(h.tintFrom.Fill, h.tintTo.Fill, progress),
		On:    palette.Mix(h.tintFrom.On, h.tintTo.On, progress),
		Ink:   palette.Mix(h.tintFrom.Ink, h.tintTo.Ink, progress),
		Field: palette.Mix(h.tintFrom.Field, h.tintTo.Field, progress),
	}
	return h.tint
}

func (h *Hero) field(gtx layout.Context, e *env.Env, tint look.Accent, artURL string, cuts [4]int, body layout.Widget) layout.Dimensions {
	return look.FieldBackdrop(gtx, e.Th, tint, cuts, &h.backdrop, e.Art, artURL, func(gtx layout.Context) layout.Dimensions {
		record := op.Record(gtx.Ops)
		dimensions := body(gtx)
		content := record.Stop()
		_, laneHeight := headerLaneMetrics(e.Th.Class)
		height := min(gtx.Dp(laneHeight), dimensions.Size.Y)
		if dimensions.Size.X > 0 && height > 0 {
			stack := op.Offset(image.Pt(0, dimensions.Size.Y-height)).Push(gtx.Ops)
			laneGTX := gtx
			laneGTX.Constraints = layout.Exact(image.Pt(dimensions.Size.X, height))
			alpha := float32(.55)
			if e.Th.R.Dark {
				alpha = .70
			}
			Visualizer(laneGTX, e, &h.visualizer, look.VisualizerStyle{
				Ink: palette.WithAlpha(tint.Ink, alpha), Bar: 6, Gap: 3, Floor: 2, HeaderLane: true,
			})
			stack.Pop()
		}
		content.Add(gtx.Ops)
		return dimensions
	})
}

func (h *Hero) copy(gtx layout.Context, e *env.Env, content HeroContent) layout.Dimensions {
	for h.meta.Clicked(gtx) {
		if content.MetaAction != nil {
			content.MetaAction()
		}
	}
	children := make([]layout.FlexChild, 0, 8)
	if strings.TrimSpace(content.Kind) != "" {
		children = append(children,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return kit.Label{Style: e.Th.T.Group, Color: e.Th.R.Fg2, MaxLines: 1}.Layout(gtx, e.Th.Theme, content.Kind)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
		)
	}
	children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		style := heroNameStyle(gtx.Metric.PxToDp(gtx.Constraints.Max.X), e.Th)
		return kit.Label{Style: style, Color: e.Th.R.Fg, MaxLines: 2}.Layout(gtx, e.Th.Theme, content.Title)
	}))
	if len(content.Meta) > 0 {
		children = append(children,
			layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return h.layoutMeta(gtx, e, content)
			}),
		)
	}
	if content.Acts != nil {
		children = append(children,
			layout.Rigid(layout.Spacer{Height: unit.Dp(13)}.Layout),
			layout.Rigid(content.Acts),
		)
	}
	return layout.Flex{Axis: layout.Vertical, Alignment: layout.Start}.Layout(gtx, children...)
}

// Layout draws a responsive bottom-aligned hero over its cover field.
func (h *Hero) Layout(gtx layout.Context, e *env.Env, content HeroContent) layout.Dimensions {
	if e == nil || e.Th == nil {
		return layout.Dimensions{}
	}
	artURL := heroArtURL(content)
	tint := h.resolveTint(gtx, e, artURL)
	artDp := heroArtSize(gtx.Metric.PxToDp(gtx.Constraints.Max.X))
	artPixels := gtx.Dp(artDp)
	inset := look.ContentInset(e.Th.Class)
	bottom, _ := headerLaneMetrics(e.Th.Class)
	cut := gtx.Dp(e.Th.Cut.Large)
	return h.field(gtx, e, tint, artURL, [4]int{cut, cut, 0, 0}, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: inset, Right: inset, Bottom: bottom, Left: inset}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			shape := look.Shape{Cut: 7, Octagon: content.Octagon}
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.End}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return h.art.Layout(gtx, e.Th, e.Art, artURL, image.Pt(artPixels, artPixels), shape, e.Th.Motion.Field)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(21)}.Layout),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return h.copy(gtx, e, content)
				}),
			)
		})
	})
}

// HeroActions lays out Play and More at full size and folds trailing optional actions first.
type HeroActions struct {
	calls   []op.CallOp
	sizes   []image.Point
	visible []bool
}

// Hidden reports whether the latest layout folded an action into More.
func (actions *HeroActions) Hidden(index int) bool {
	return index >= 0 && index < len(actions.visible) && !actions.visible[index]
}

func (actions *HeroActions) Layout(gtx layout.Context, widgets []layout.Widget) layout.Dimensions {
	count := len(widgets)
	if count == 0 {
		return layout.Dimensions{}
	}
	if cap(actions.calls) < count {
		actions.calls = make([]op.CallOp, count)
		actions.sizes = make([]image.Point, count)
		actions.visible = make([]bool, count)
	} else {
		actions.calls = actions.calls[:count]
		actions.sizes = actions.sizes[:count]
		actions.visible = actions.visible[:count]
	}
	measure := gtx
	measure.Constraints.Min = image.Point{}
	measure.Constraints.Max.X = 1 << 20
	maxHeight := 0
	for index, widget := range widgets {
		actions.visible[index] = widget != nil
		if widget == nil {
			actions.calls[index] = op.CallOp{}
			actions.sizes[index] = image.Point{}
			continue
		}
		record := op.Record(gtx.Ops)
		dimensions := widget(measure)
		actions.calls[index] = record.Stop()
		actions.sizes[index] = dimensions.Size
		maxHeight = max(maxHeight, dimensions.Size.Y)
	}
	gap := gtx.Dp(8)
	totalWidth := func() int {
		width := 0
		seen := false
		for index := range widgets {
			if !actions.visible[index] {
				continue
			}
			if seen {
				width += gap
			}
			width += actions.sizes[index].X
			seen = true
		}
		return width
	}
	width := totalWidth()
	for index := count - 2; index >= 1 && width > gtx.Constraints.Max.X; index-- {
		actions.visible[index] = false
		width = totalWidth()
	}
	x := 0
	seen := false
	for index := range widgets {
		if !actions.visible[index] {
			continue
		}
		if seen {
			x += gap
		}
		offset := op.Offset(image.Pt(x, (maxHeight-actions.sizes[index].Y)/2)).Push(gtx.Ops)
		actions.calls[index].Add(gtx.Ops)
		offset.Pop()
		x += actions.sizes[index].X
		seen = true
	}
	return layout.Dimensions{Size: gtx.Constraints.Constrain(image.Pt(width, maxHeight))}
}

// Header is a retained source-field page title with optional proof and actions.
// Below lays out under the title row inside the field; NoLane drops the
// visualizer lane along the field's bottom edge.
type Header struct {
	Sentence   bool
	NoLane     bool
	Below      layout.Widget
	visualizer look.Visualizer
	copy       [3]layout.FlexChild
	children   [3]layout.FlexChild
}

func (h *Header) Layout(gtx layout.Context, e *env.Env, title, sentence string, trailing layout.Widget) layout.Dimensions {
	if e == nil || e.Th == nil {
		return layout.Dimensions{}
	}
	cut := gtx.Dp(e.Th.Cut.Large)
	inset := look.ContentInset(e.Th.Class)
	bottom, _ := headerLaneMetrics(e.Th.Class)
	lane := &h.visualizer
	if h.NoLane {
		bottom, lane = inset, nil
	}
	return headerField(gtx, e, lane, [4]int{cut, cut, 0, 0}, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: inset, Right: inset, Bottom: bottom, Left: inset}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			if h.Below == nil {
				return h.titleRow(gtx, e, title, sentence, trailing)
			}
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return h.titleRow(gtx, e, title, sentence, trailing)
				}),
				layout.Rigid(h.Below),
			)
		})
	})
}

func (h *Header) titleRow(gtx layout.Context, e *env.Env, title, sentence string, trailing layout.Widget) layout.Dimensions {
	style := e.Th.T.Section
	if h.Sentence {
		style = e.Th.T.Sentence
	}
	copy := h.copy[:1]
	copy[0] = layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return kit.Label{Style: style, Color: e.Th.R.Fg, MaxLines: 2}.Layout(gtx, e.Th.Theme, title)
	})
	if strings.TrimSpace(sentence) != "" {
		copy = append(copy,
			layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return kit.Label{Style: e.Th.T.Proof, Color: e.Th.R.Fg2, MaxLines: 2}.Layout(gtx, e.Th.Theme, sentence)
			}),
		)
	}
	children := h.children[:1]
	children[0] = layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.Start}.Layout(gtx, copy...)
	})
	if trailing != nil {
		children = append(children,
			layout.Rigid(layout.Spacer{Width: unit.Dp(21)}.Layout),
			layout.Rigid(trailing),
		)
	}
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
}
