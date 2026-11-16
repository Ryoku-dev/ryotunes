package settings

import (
	"image"

	"gioui.org/gesture"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/anim"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/tokens"
	"github.com/ryoku-dev/ryotunes/internal/client/state"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/sheet"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/widgets"
)

// The page's sheet geometry, from the Hub's SettingsPage and PageHeader, in
// dp.
const (
	// pageTop is the Hub's page inset under the masthead.
	pageTop unit.Dp = 8
	// pageSide is the stage's left inset; pageSideNarrow on a narrow stage.
	pageSide       unit.Dp = 28
	pageSideNarrow unit.Dp = 13
	// pageNarrow is the stage width under which the narrow inset applies.
	pageNarrow unit.Dp = 640
	// pageRail keeps the column clear of the scroll rail at the right edge.
	pageRail unit.Dp = 4
	// columnMax is the widest the column grows (SettingsPage.qml:92).
	columnMax unit.Dp = 960
	// headBelow stands between the page head and the first section.
	headBelow unit.Dp = 26
	// leadMax is the widest the lead runs.
	leadMax unit.Dp = 720
	// chipsBelow stands between the section chips and the sheet.
	chipsBelow unit.Dp = 13
	// specTracking is the spec strip's tracking (PageHeader.qml).
	specTracking = 1.2
)

const pageLead = "How Ryotunes signs in, plays, keeps your music and wears the desktop's look."

// Layout draws Settings as the Hub's page sheet: the page head (title,
// lead, the spec strip with the count of changed settings rolling as it
// moves), then each section as a ruled, numbered part with its rows in a
// slab. The first sections rise in as the page first opens. The sections are
// the outline in the sidebar while it shows them; otherwise a strip of
// chips above the sheet jumps between them.
func (p *page) Layout(gtx layout.Context, e *env.Env) layout.Dimensions {
	p.th, p.env = e.Th, e
	p.scroll.UpdateInput(gtx)
	if p.scroll.UserScrolled() {
		p.indexHold = -1
	}
	if p.goTo >= 0 {
		p.scroll.GlideTo(p.goTo+1, e.Th.Motion.Move)
		p.goTo = -1
	}
	p.updateCurrentSection()
	if !p.refreshed {
		p.Refresh(e)
	}
	snapshot := e.Store.Snapshot()
	if p.retry.Clicked(gtx) {
		p.Refresh(e)
	}
	if p.loading && len(snapshot.Settings) == 0 {
		return widgets.Loading(e, widgets.LoadingTable)(gtx)
	}
	if p.loadError != "" && len(snapshot.Settings) == 0 {
		p.retry.Kind, p.retry.Label = kit.PlaqueQuiet, "Retry"
		return widgets.Failed(e, p.loadError, &p.retry)(gtx)
	}
	p.forgetSettledWrites(snapshot)
	// the Hub's page inset under the masthead stands outside the sheet, so
	// the sheet scrolls under a clean edge
	return layout.Inset{Top: pageTop}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		if p.outlined {
			return p.layoutSheet(gtx, e, snapshot)
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return p.column(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Bottom: chipsBelow}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return p.layoutChips(gtx, e)
					})
				})
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return p.layoutSheet(gtx, e, snapshot) }),
		)
	})
}

// layoutChips draws the sections as wrapping chips; a chip glides the sheet
// to its section.
func (p *page) layoutChips(gtx layout.Context, e *env.Env) layout.Dimensions {
	p.sectionChips.Labels, p.sectionChips.Selected = outlineNames, p.currentSection
	dims := p.sectionChips.Layout(gtx, e.Th)
	if selected, ok := p.sectionChips.Changed(); ok {
		p.GoTo(selected)
		gtx.Execute(op.InvalidateCmd{})
	}
	return dims
}

// column lays w out in the page's column: inset from the stage's left, clear
// of the scroll rail, min(usable, 960) wide and centred, as the Hub sets its
// settings column.
func (p *page) column(gtx layout.Context, w layout.Widget) layout.Dimensions {
	stage := gtx.Constraints.Max.X
	side := gtx.Dp(pageSide)
	if stage < gtx.Dp(pageNarrow) {
		side = gtx.Dp(pageSideNarrow)
	}
	right := gtx.Dp(pageRail) + gtx.Dp(unit.Dp(tokens.SizeScrollThumb)) + gtx.Dp(e3)
	usable := max(0, stage-side-right)
	width := min(usable, gtx.Dp(columnMax))
	x := side + (usable-width)/2
	child := gtx
	child.Constraints.Min.X, child.Constraints.Max.X = width, width
	child.Constraints.Min.Y = 0
	offset := op.Offset(image.Pt(x, 0)).Push(gtx.Ops)
	dims := w(child)
	offset.Pop()
	return layout.Dimensions{Size: image.Pt(stage, dims.Size.Y), Baseline: dims.Baseline}
}

// e3 is the Hub's s3 gap between the column and the scroll rail.
const e3 unit.Dp = 13

func (p *page) layoutSheet(gtx layout.Context, e *env.Env, snapshot state.State) layout.Dimensions {
	p.viewportHeight = gtx.Constraints.Max.Y
	if !p.initialScroll {
		p.initialScroll = true
		if p.initialSection >= 0 {
			p.scroll.ScrollTo(p.initialSection + 1)
		}
	}
	p.revealFrames++
	if p.revealFrames == 1 {
		p.revealBase = p.scroll.Position.First
	}
	return p.scroll.Layout(gtx, e.Th, len(sections)+1, func(gtx layout.Context, index int) layout.Dimensions {
		dims := p.revealItem(gtx, e, index, func(gtx layout.Context) layout.Dimensions {
			return p.column(gtx, func(gtx layout.Context) layout.Dimensions {
				if index == 0 {
					return p.layoutHead(gtx, e, snapshot)
				}
				return p.layoutSection(gtx, e, snapshot, index-1)
			})
		})
		p.itemHeights[index] = dims.Size.Y
		return dims
	})
}

// revealItem rises the items the page first opens on, the Hub's Reveal: the
// first five laid out in the page's first frame, 40 ms apart. Items first
// laid out later (scrolled to) stand, so nothing rises on a scroll or a
// refresh.
func (p *page) revealItem(gtx layout.Context, e *env.Env, index int, w layout.Widget) layout.Dimensions {
	order := index - p.revealBase
	if order < 0 || order >= anim.RevealGroups || order >= 64 {
		return w(gtx)
	}
	bit := uint64(1) << order
	if p.revealFrames == 1 {
		p.revealSeen |= bit
	}
	if p.revealSeen&bit == 0 {
		return w(gtx)
	}
	return p.reveal.Group(gtx, e.Th, order, w)
}

// layoutHead draws the page head: "Settings" at 32/40, the lead, and the
// spec strip, 26 above the first section.
func (p *page) layoutHead(gtx layout.Context, e *env.Env, snapshot state.State) layout.Dimensions {
	th := e.Th
	title := th.T.Sentence
	title.Size, title.Line = 32, 40
	changed := p.changedCount(snapshot)
	return layout.Inset{Bottom: headBelow}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				semantic.LabelOp("Settings").Add(gtx.Ops)
				return kit.Label{Style: title, Color: th.R.Fg, MaxLines: 1}.Layout(gtx, th.Theme, "Settings")
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(leadMax))
				return layout.Inset{Top: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return kit.Label{Style: th.T.Body, Color: th.R.Fg2}.Layout(gtx, th.Theme, pageLead)
				})
			}),
			layout.Rigid(layout.Spacer{Height: sheet.SpecStripAbove}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return p.layoutSpecStrip(gtx, th, changed)
			}),
		)
	})
}

// layoutSpecStrip draws "SECTIONS 12   CHANGED 03" as the Hub's page head
// does, the changed count rolling to its new figure and lit while any
// setting stands away from its default.
func (p *page) layoutSpecStrip(gtx layout.Context, th *look.Theme, changed int) layout.Dimensions {
	gtx.Constraints.Min = image.Point{}
	sections := sheet.SpecStrip(gtx, th, p.sectionsFig[:])
	x := sections.Size.X + gtx.Dp(18)
	offset := op.Offset(image.Pt(x, 0)).Push(gtx.Ops)
	label := kit.LayoutSpacedCaps(gtx, th.Theme, th.T.MonoSm, th.R.Fg3, specTracking, "Changed")
	offset.Pop()
	x += label.Size.X + gtx.Dp(6)
	ink := th.R.Fg
	if changed > 0 {
		ink = th.R.Sel
	}
	offset = op.Offset(image.Pt(x, 0)).Push(gtx.Ops)
	value := p.changedRoll.Layout(gtx, th, th.T.MonoSm, ink, kit.Pad2(changed))
	offset.Pop()
	return layout.Dimensions{Size: image.Pt(x+value.Size.X, max(sections.Size.Y, label.Size.Y, value.Size.Y)), Baseline: label.Baseline}
}

// layoutSection draws one section: its ruled head numbered "04 / 12" and its
// rows in a slab; the last section keeps the page's foot.
func (p *page) layoutSection(gtx layout.Context, e *env.Env, snapshot state.State, index int) layout.Dimensions {
	section := kit.SectionStyle{Title: sections[index].title, Ruled: true, Mark: sectionMarks[index]}
	body := p.sectionBody(index, e, snapshot)
	dims := section.Layout(gtx, e.Th.Theme, body)
	if index == len(sections)-1 {
		dims.Size.Y += gtx.Dp(unit.Dp(tokens.SizeSettingsInsetVertical))
	}
	return dims
}

func (p *page) sectionBody(index int, e *env.Env, snapshot state.State) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		p.rowSection, p.rowCursor, p.slabStart, p.th, p.env = index, 0, 0, e.Th, e
		switch sections[index].id {
		case "account":
			return p.layoutAccount(gtx, e, snapshot)
		case "playback":
			return p.layoutPlayback(gtx, e, snapshot)
		case "sound":
			return p.layoutSound(gtx, e, snapshot)
		case "lyrics":
			return p.layoutLyrics(gtx, e, snapshot)
		case "downloads":
			return p.layoutDownloads(gtx, e)
		case "local":
			return p.layoutLocal(gtx, e)
		case "playlists":
			return p.layoutPlaylists(gtx, e, snapshot)
		case "appearance":
			return p.layoutAppearance(gtx, e, snapshot)
		case "integrations":
			return p.layoutIntegrations(gtx, e, snapshot)
		case "data":
			return p.layoutData(gtx, e)
		case "diagnostics":
			return p.layoutDiagnostics(gtx, e)
		default:
			return p.layoutAbout(gtx, e)
		}
	}
}

// updateCurrentSection follows the reading position, unless a jump holds
// its section until the reader scrolls.
func (p *page) updateCurrentSection() {
	if p.indexHold >= 0 {
		return
	}
	first, offset := p.scroll.Offset()
	atEnd := p.scroll.Position.First+p.scroll.Position.Count >= len(p.itemHeights) && p.scroll.Position.OffsetLast >= 0
	item := scrollspySection(first, offset, p.viewportHeight, p.itemHeights, atEnd)
	p.currentSection = min(max(item-1, 0), len(sections)-1)
}

// scrollspySection is the item being read: the last one whose top has
// passed a third of the viewport, or the last item once the list ends.
func scrollspySection(first, offset, viewportHeight int, sectionHeights []int, atEnd bool) int {
	if len(sectionHeights) == 0 {
		return 0
	}
	if atEnd {
		return len(sectionHeights) - 1
	}
	first = min(max(first, 0), len(sectionHeights)-1)
	current := first
	threshold := viewportHeight / 3
	top := -offset
	for index := first + 1; index < len(sectionHeights); index++ {
		height := sectionHeights[index-1]
		if height <= 0 {
			break
		}
		top += height
		if top >= threshold {
			break
		}
		current = index
	}
	return current
}

// readProgress is how far the sheet is read, for the masthead's marker.
func (p *page) readProgress() float32 { return p.scroll.Progress() }

// settingRows stacks a section's rows in its slab: the design's card plane
// (th.Planes, the Hub's Tone.card) inside the mid cut, as Lithic draws it by
// its hairline alone, Material and Adwaita lift it a tone off the pane, and
// Fluent outlines a lifted card.
func (p *page) settingRows(gtx layout.Context, rows ...layout.Widget) layout.Dimensions {
	th := p.th
	record := op.Record(gtx.Ops)
	dims := layout.UniformInset(unit.Dp(tokens.SizeSlabInset)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		return stackRows(gtx, rows)
	})
	call := record.Stop()
	paintCard(gtx, th, image.Rectangle{Max: dims.Size})
	call.Add(gtx.Ops)
	return dims
}

// paintCard paints a slab's card plane in bounds in the theme's design.
func paintCard(gtx layout.Context, th *look.Theme, bounds image.Rectangle) {
	planes := th.Planes()
	cut := gtx.Dp(th.Cut.Mid)
	if planes.Card != th.R.Slab {
		kit.Fill(gtx, th.Theme, bounds, cut, planes.Card)
	}
	if planes.CardStroke.A > 0 {
		kit.Stroke(gtx, th.Theme, bounds, cut, max(1, gtx.Dp(unit.Dp(tokens.SizeHairline))), planes.CardStroke)
	}
}

// newSlab starts another slab in the section being laid out: the next row
// is its first and draws no hairline above it.
func (p *page) newSlab() { p.slabStart = p.rowCursor }

// stackRows lays rows out one under the other at full width.
func stackRows(gtx layout.Context, rows []layout.Widget) layout.Dimensions {
	y, width := 0, gtx.Constraints.Max.X
	for _, row := range rows {
		child := gtx
		child.Constraints.Min = image.Pt(width, 0)
		child.Constraints.Max.Y = max(0, gtx.Constraints.Max.Y-y)
		offset := op.Offset(image.Pt(0, y)).Push(gtx.Ops)
		dims := row(child)
		offset.Pop()
		y += dims.Size.Y
	}
	return layout.Dimensions{Size: image.Pt(width, y)}
}

// disclosure is the Hub's "More options" fold at a slab's foot: a quiet row
// whose chevron turns (240 ms, OutBack 1.6) as the rows under it unfold
// (Lithic's unfold, growing and fading in). The rows are laid out only while
// the fold shows them, so they are made the first time it opens. Hovered,
// the head washes and leans like a row.
type disclosure struct {
	fold    kit.Disclosure
	head    kit.Click
	hover   gesture.Hover
	wash    look.RowWash
	chevron look.Chevron
}

// layout draws the fold's head and the rows under it as far as they show.
func (d *disclosure) layout(gtx layout.Context, th *look.Theme, label string, rows func(gtx layout.Context) layout.Dimensions) layout.Dimensions {
	if d.head.Clicked(gtx) {
		d.fold.Open = !d.fold.Open
	}
	hovered := d.hover.Update(gtx.Source)
	d.wash.Update(gtx, th, hovered, d.head.Pressed())
	width := gtx.Constraints.Max.X
	gap := gtx.Dp(tokens.SizeSettingsRowGap)
	height := gtx.Dp(unit.Dp(tokens.SizeIconPlaqueSmall))
	box := image.Rect(0, gap, width, gap+height)
	bleed := image.Rect(box.Min.X-gtx.Dp(th.Sp.S2), box.Min.Y-gtx.Dp(th.Sp.S1), box.Max.X+gtx.Dp(th.Sp.S2), box.Max.Y+gtx.Dp(th.Sp.S1))
	line := max(1, gtx.Dp(unit.Dp(tokens.SizeHairline)))
	kit.FillCut(gtx.Ops, image.Rect(0, gap/2, width, gap/2+line), 0, th.R.Vein)

	area := clip.Rect(bleed).Push(gtx.Ops)
	d.hover.Add(gtx.Ops)
	d.wash.Paint(gtx, th, bleed, gtx.Dp(th.Cut.Small))
	headGTX := gtx
	headGTX.Constraints = layout.Exact(bleed.Size())
	offset := op.Offset(bleed.Min).Push(gtx.Ops)
	d.head.Layout(headGTX, func(gtx layout.Context) layout.Dimensions {
		semantic.Button.Add(gtx.Ops)
		semantic.LabelOp(label).Add(gtx.Ops)
		semantic.SelectedOp(d.fold.Open).Add(gtx.Ops)
		return layout.Dimensions{Size: gtx.Constraints.Min}
	})
	offset.Pop()
	area.Pop()

	size := gtx.Dp(16)
	lean := d.wash.Lean(gtx)
	angle := d.chevron.Angle(gtx, th, d.fold.Open)
	offset = op.Offset(image.Pt(lean, gap+(height-size)/2)).Push(gtx.Ops)
	// the glyph points down closed and up open: the chevron's quarter turn
	// (with its OutBack settle) doubled into a half turn
	look.DrawGlyph(gtx, look.Icon.ChevronDown, th.R.Fg2, size, angle*2, d.wash.GlyphScale())
	offset.Pop()
	labelGTX := gtx
	labelGTX.Constraints.Min = image.Point{}
	record := op.Record(gtx.Ops)
	text := kit.Label{Style: th.T.Label, Color: th.R.Fg2, MaxLines: 1}.Layout(labelGTX, th.Theme, label)
	call := record.Stop()
	offset = op.Offset(image.Pt(lean+size+gtx.Dp(th.Sp.S2), gap+(height-text.Size.Y)/2)).Push(gtx.Ops)
	call.Add(gtx.Ops)
	offset.Pop()

	y := gap + height
	offset = op.Offset(image.Pt(0, y)).Push(gtx.Ops)
	child := gtx
	child.Constraints.Min = image.Pt(width, 0)
	folded := d.fold.Layout(child, th.Theme, rows)
	offset.Pop()
	return layout.Dimensions{Size: image.Pt(width, y+folded.Size.Y)}
}
