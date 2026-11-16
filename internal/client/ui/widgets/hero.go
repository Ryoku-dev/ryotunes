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
	"github.com/ryoku-dev/ryotunes/internal/client/ui/sheet"
	"github.com/ryoku-dev/ryotunes/internal/model"
)

// HeroContent describes one album, artist, playlist, or top-result header.
// Kind is the kind line ("Album"), set in spaced capitals. Place and
// PartGloss, both optional, set the page's kanji vertically beside the art
// while glosses are on: PartGloss large over Place's gloss small.
type HeroContent struct {
	Art              string
	Octagon, Collage bool
	Kind, Title      string
	Meta             []kit.Span
	MetaAction       func()
	Acts             layout.Widget
	Place            sheet.Place
	PartGloss        string
}

// heroKindTracking is the kind line's tracking (the Hub's group caps).
const heroKindTracking = 1.2

// Hero is a retained cover-tinted page header.
type Hero struct {
	art        look.ArtView
	meta       kit.Click
	visualizer look.Visualizer
	tategaki   sheet.Tategaki
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
	return InsidePaneStroke(gtx, e.Th, func(gtx layout.Context) layout.Dimensions {
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
				return kit.LayoutSpacedCaps(gtx, e.Th.Theme, e.Th.T.Group, e.Th.R.Fg2, heroKindTracking, content.Kind)
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
			gloss, category := content.PartGloss, content.Place.Gloss
			if gloss == "" {
				gloss = category
			}
			glossed := e.Th.Desktop.Gloss && gloss != ""
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.End}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if coverMissing(artURL) {
						return EmptyCover(gtx, e.Th, image.Pt(artPixels, artPixels), shape, emptyCoverIcon(content.Kind, false))
					}
					return h.art.Layout(gtx, e.Th, e.Art, artURL, image.Pt(artPixels, artPixels), shape, e.Th.Motion.Field)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if !glossed {
						return layout.Dimensions{}
					}
					// The tategaki stands beside the art, top-aligned with it,
					// 12 from it as the Hub's stands beside its preview.
					offset := op.Offset(image.Pt(gtx.Dp(12), 0)).Push(gtx.Ops)
					h.tategaki.Layout(gtx, e.Th, gloss, category)
					offset.Pop()
					return layout.Dimensions{Size: image.Pt(gtx.Dp(sheet.TategakiRoom), artPixels)}
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

// Page head geometry (PageHeader.qml), in dp.
const (
	headTitleSize     = 32
	headTitleLine     = 40
	headLeadMax       = 720
	headLeadMargin    = 44
	headLeadAbove     = 4
	headStateAbove    = 6
	headActionsGap    = 12
	headSideGap       = 28
	headPreviewWidth  = 264
	headPreviewHeight = 128
	headPreviewTop    = 44
	headPreviewMin    = 660
	headGlossTop      = 4
)

// Header is a retained page head over the source field, as the Hub's
// PageHeader: the title (sentence 32/40, or the greeting's 34/40 when
// Sentence), the actions on the title's line, the lead (Layout's sentence,
// body fg2 up to 720), a state line (detail fg), the spec strip 16 below the
// words, and at the right, while glosses are on, the page's kanji set
// vertically (sheet.Tategaki) beside an optional preview. The masthead and
// colophon belong to the frame: a page hands Part and PartGloss to it.
//
// Below lays out under the words inside the field; NoLane drops the
// visualizer lane along the field's bottom edge. Every new field is optional:
// a zero Header draws the title, lead and actions as before.
type Header struct {
	Sentence bool
	NoLane   bool
	Below    layout.Widget

	// Place is the page's place on the sheet; its gloss heads the tategaki.
	// The zero Place shows no tategaki.
	Place sheet.Place
	// Part is the part of the place shown ("Songs" in the Library), sentence
	// case, and PartGloss its kanji ("曲"); with PartGloss set the tategaki
	// sets the part's kanji large and the place's small beneath. A page's
	// Masthead method returns them to the frame.
	Part, PartGloss string
	// State is the state line under the lead: what the page says about the
	// library now, in detail fg.
	State string
	// Specs is the page's spec strip ("SONGS 1204"), 16 under the words; a
	// value that changes rolls to its new figure.
	Specs []sheet.Spec
	// Preview is a live illustration at the head's right (264x128, 44 down),
	// shown when the head is at least 660 wide.
	Preview layout.Widget

	visualizer look.Visualizer
	tategaki   sheet.Tategaki
	rolls      []look.CountRoll
}

// Spec strip geometry (PageHeader.qml:115-139, as sheet.SpecStrip), in dp.
const (
	headSpecGap      = 18
	headSpecLabelGap = 6
	headSpecTracking = 1.2
)

// specStrip lays the page's figures out as sheet.SpecStrip does, each label
// in small mono spaced capitals fg3 and its value in small mono fg (sel when
// lit), but each value rolls when it changes (look.CountRoll), as the Hub's
// counts do.
func (h *Header) specStrip(gtx layout.Context, th *look.Theme) layout.Dimensions {
	for len(h.rolls) < len(h.Specs) {
		h.rolls = append(h.rolls, look.CountRoll{})
	}
	gtx.Constraints.Min = image.Point{}
	x, height := 0, 0
	for index, spec := range h.Specs {
		if index > 0 {
			x += gtx.Dp(headSpecGap)
		}
		offset := op.Offset(image.Pt(x, 0)).Push(gtx.Ops)
		label := kit.LayoutSpacedCaps(gtx, th.Theme, th.T.MonoSm, th.R.Fg3, headSpecTracking, spec.Label)
		offset.Pop()
		x += label.Size.X + gtx.Dp(headSpecLabelGap)
		ink := th.R.Fg
		if spec.Lit {
			ink = th.R.Sel
		}
		offset = op.Offset(image.Pt(x, 0)).Push(gtx.Ops)
		value := h.rolls[index].Layout(gtx, th, th.T.MonoSm, ink, spec.Value)
		offset.Pop()
		x += value.Size.X
		height = max(height, label.Size.Y, value.Size.Y)
	}
	return layout.Dimensions{Size: image.Pt(x, height)}
}

// Gloss is the tategaki's page kanji and its category's: the part's gloss
// over the place's, or the place's alone.
func (h *Header) Gloss() (page, category string) {
	if h.PartGloss != "" {
		return h.PartGloss, h.Place.Gloss
	}
	return h.Place.Gloss, h.Place.Gloss
}

// Masthead returns the part the frame's masthead names, with its gloss.
func (h *Header) Masthead() (part, partGloss string) {
	return h.Part, h.PartGloss
}

// Layout draws the head. sentence is the lead and trailing the page's
// actions, which stand at the end of the title's line.
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
			return h.head(gtx, e, title, sentence, trailing)
		})
	})
}

// headTitleStyle is the page title's type: the Hub's sentence at 32/40, or
// the greeting's own 34/40.
func headTitleStyle(th *look.Theme, greeting bool) theme.TextStyle {
	style := th.T.Sentence
	if !greeting {
		style.Size, style.Line = headTitleSize, headTitleLine
	}
	return style
}

func (h *Header) head(gtx layout.Context, e *env.Env, title, sentence string, trailing layout.Widget) layout.Dimensions {
	gtx.Constraints.Min.Y = 0
	width := gtx.Constraints.Max.X
	previewed := h.Preview != nil && gtx.Metric.PxToDp(width) >= headPreviewMin
	gloss, category := h.Gloss()
	glossed := e.Th.Desktop.Gloss && gloss != ""
	side := 0
	if previewed {
		side += gtx.Dp(headPreviewWidth)
	}
	if glossed {
		side += gtx.Dp(sheet.TategakiRoom)
	}
	wordsWidth := width
	if side > 0 {
		wordsWidth = max(0, width-side-gtx.Dp(headSideGap))
	}
	words := gtx
	words.Constraints = layout.Constraints{Max: image.Pt(wordsWidth, gtx.Constraints.Max.Y)}
	height := h.words(words, e, title, sentence, trailing, side > 0)
	if previewed {
		slot := gtx
		size := image.Pt(gtx.Dp(headPreviewWidth), gtx.Dp(headPreviewHeight))
		slot.Constraints = layout.Exact(size)
		top := gtx.Dp(headPreviewTop)
		offset := op.Offset(image.Pt(width-size.X, top)).Push(gtx.Ops)
		h.Preview(slot)
		offset.Pop()
		height = max(height, top+size.Y)
	}
	if glossed {
		top := gtx.Dp(headGlossTop)
		offset := op.Offset(image.Pt(width-side+gtx.Dp(2), top)).Push(gtx.Ops)
		column := h.tategaki.Layout(gtx, e.Th, gloss, category)
		offset.Pop()
		height = max(height, top+column.Size.Y)
	}
	if h.Below != nil {
		below := gtx
		below.Constraints = layout.Constraints{Max: image.Pt(width, max(0, gtx.Constraints.Max.Y-height))}
		offset := op.Offset(image.Pt(0, height)).Push(gtx.Ops)
		dimensions := h.Below(below)
		offset.Pop()
		height += dimensions.Size.Y
	}
	return layout.Dimensions{Size: image.Pt(width, height)}
}

// words lays out the title line, lead, state and spec strip down the words
// column and returns their height.
func (h *Header) words(gtx layout.Context, e *env.Env, title, sentence string, trailing layout.Widget, sided bool) int {
	th := e.Th
	width := gtx.Constraints.Max.X
	actionsWidth, actionsHeight := 0, 0
	var actions op.CallOp
	if trailing != nil {
		record := op.Record(gtx.Ops)
		child := gtx
		child.Constraints.Min = image.Point{}
		dimensions := trailing(child)
		actions = record.Stop()
		actionsWidth, actionsHeight = dimensions.Size.X, dimensions.Size.Y
	}
	titleGTX := gtx
	titleWidth := width
	if actionsWidth > 0 {
		titleWidth = max(0, width-actionsWidth-gtx.Dp(headActionsGap))
	}
	titleGTX.Constraints = layout.Constraints{Max: image.Pt(titleWidth, gtx.Constraints.Max.Y)}
	record := op.Record(gtx.Ops)
	titleDims := kit.Label{Style: headTitleStyle(th, h.Sentence), Color: th.R.Fg, MaxLines: 2}.Layout(titleGTX, th.Theme, title)
	titleCall := record.Stop()
	line := max(titleDims.Size.Y, actionsHeight)
	offset := op.Offset(image.Pt(0, (line-titleDims.Size.Y)/2)).Push(gtx.Ops)
	titleCall.Add(gtx.Ops)
	offset.Pop()
	if actionsWidth > 0 {
		offset := op.Offset(image.Pt(width-actionsWidth, (line-actionsHeight)/2)).Push(gtx.Ops)
		actions.Add(gtx.Ops)
		offset.Pop()
	}
	y := line
	proseWidth := width
	if !sided {
		proseWidth = min(max(0, width-gtx.Dp(headLeadMargin)), gtx.Dp(headLeadMax))
	}
	prose := gtx
	prose.Constraints = layout.Constraints{Max: image.Pt(proseWidth, gtx.Constraints.Max.Y)}
	if strings.TrimSpace(sentence) != "" {
		y += gtx.Dp(headLeadAbove)
		offset := op.Offset(image.Pt(0, y)).Push(gtx.Ops)
		dimensions := kit.Label{Style: th.T.Body, Color: th.R.Fg2, MaxLines: 3}.Layout(prose, th.Theme, sentence)
		offset.Pop()
		y += dimensions.Size.Y
	}
	if strings.TrimSpace(h.State) != "" {
		y += gtx.Dp(headStateAbove)
		offset := op.Offset(image.Pt(0, y)).Push(gtx.Ops)
		dimensions := kit.Label{Style: th.T.Detail, Color: th.R.Fg, MaxLines: 2}.Layout(prose, th.Theme, h.State)
		offset.Pop()
		y += dimensions.Size.Y
	}
	if len(h.Specs) > 0 {
		y += gtx.Dp(sheet.SpecStripAbove)
		strip := gtx
		strip.Constraints = layout.Constraints{Max: image.Pt(width, gtx.Constraints.Max.Y)}
		offset := op.Offset(image.Pt(0, y)).Push(gtx.Ops)
		dimensions := h.specStrip(strip, th)
		offset.Pop()
		y += dimensions.Size.Y
	}
	return y
}
