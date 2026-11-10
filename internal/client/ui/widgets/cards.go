package widgets

import (
	"image"

	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/anim"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/model"
)

const cardPlateInset unit.Dp = 8

func cardPlateGap(gtx layout.Context) layout.Dimensions {
	return layout.Dimensions{Size: image.Pt(max(0, gtx.Dp(21)-2*gtx.Dp(cardPlateInset)), 0)}
}

// Card is one retained browse card.
type Card struct {
	Preview     kit.State
	PlayPreview kit.State

	click      kit.Click
	play       kit.Click
	art        look.ArtView
	reveal     anim.Tween
	playHover  anim.Tween
	contextTag int
	context    contextRelease
	menu       ItemMenu
}

func (c *Card) openMenu(e *env.Env, item model.BrowseItem) {
	target := env.MenuTarget{Card: &item}
	if e.Menu != nil {
		e.Menu(target)
		return
	}
	if e.Overlays != nil {
		c.menu.OpenAt(e, target, e.Overlays.Press())
	}
}

func playBrowseItem(e *env.Env, item model.BrowseItem) {
	if browseIsTrack(item) {
		act.Play(e, act.TrackFromCard(item))
		return
	}
	withCollection(e, item, func(items []model.Track) {
		act.PlayCollection(e, items, 0, item.ID, item.Title, false, nil)
	})
}

type contextRelease struct {
	pending   bool
	pointerID pointer.ID
}

func (release *contextRelease) poll(gtx layout.Context, tag any, run func()) {
	for {
		raw, ok := gtx.Event(pointer.Filter{Target: tag, Kinds: pointer.Press | pointer.Release | pointer.Cancel})
		if !ok {
			return
		}
		event := raw.(pointer.Event)
		switch event.Kind {
		case pointer.Press:
			release.pending = event.Buttons.Contain(pointer.ButtonSecondary)
			release.pointerID = event.PointerID
		case pointer.Release:
			if release.pending && event.PointerID == release.pointerID {
				release.pending = false
				run()
			}
		case pointer.Cancel:
			if event.PointerID == release.pointerID {
				release.pending = false
			}
		}
	}
}

func cardSize(e *env.Env) unit.Dp {
	switch e.Th.Class {
	case look.Compact:
		return 136
	case look.Medium:
		return 152
	default:
		return 176
	}
}

func (c *Card) process(gtx layout.Context, e *env.Env, item model.BrowseItem) {
	if c.play.Clicked(gtx) {
		playBrowseItem(e, item)
	}
	for c.click.Clicked(gtx) {
		act.OpenCard(e, item)
	}
	c.context.poll(gtx, &c.contextTag, func() { c.openMenu(e, item) })
}

func cardArtShape(item model.BrowseItem) look.Shape {
	return look.Shape{Cut: 7, Octagon: browseIsArtist(item)}
}

func (c *Card) artLayout(gtx layout.Context, e *env.Env, item model.BrowseItem, size int) layout.Dimensions {
	return c.art.Layout(gtx, e.Th, e.Art, textValue(item.Thumbnail), image.Pt(size, size), cardArtShape(item), e.Th.Motion.Navigate)
}

func (c *Card) playPlate(gtx layout.Context, e *env.Env, reveal float32) layout.Dimensions {
	if reveal <= 0 {
		return layout.Dimensions{}
	}
	c.playHover.Duration = e.Th.Motion.HoverIn
	c.playHover.Ease = anim.EaseChisel
	hot := c.play.Hovered() || c.PlayPreview == kit.StateHover
	target := float32(0)
	if hot {
		target = 1
	}
	c.playHover.Set(gtx, target)
	hover := c.playHover.Value(gtx)
	pixels := gtx.Dp(40)
	gtx.Constraints = layout.Exact(image.Pt(pixels, pixels))
	opacity := paint.PushOpacity(gtx.Ops, reveal)
	dims := c.play.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		bounds := image.Rect(0, 0, pixels, pixels)
		ink := palette.Mix(e.Th.R.Stone4, e.Th.R.Gold, hover)
		kit.FillCut(gtx.Ops, bounds, gtx.Dp(e.Th.Cut.Small), ink)
		iconInk := palette.Mix(e.Th.R.Fg, e.Th.R.OnGold, hover)
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return drawIcon(gtx, look.Icon.Play, iconInk, 20)
		})
	})
	opacity.Pop()
	return dims
}

// Layout draws artwork, copy, and the rising play action.
func (c *Card) Layout(gtx layout.Context, e *env.Env, item model.BrowseItem, width unit.Dp) layout.Dimensions {
	if e == nil || e.Th == nil {
		return layout.Dimensions{}
	}
	c.process(gtx, e, item)
	if width <= 0 {
		width = cardSize(e)
	}
	artSize := min(gtx.Constraints.Max.X-2*gtx.Dp(cardPlateInset), gtx.Dp(width))
	if artSize <= 0 {
		return layout.Dimensions{}
	}
	padding := gtx.Dp(cardPlateInset)
	outerWidth := artSize + 2*padding
	line := gtx.Sp(e.Th.T.Label.Line)
	caption := gtx.Sp(e.Th.T.Caption.Line)
	height := 2*padding + artSize + gtx.Dp(8) + 2*line + caption
	size := gtx.Constraints.Constrain(image.Pt(outerWidth, height))

	hot := c.click.Hovered() || c.Preview == kit.StateHover || c.Preview == kit.StatePressed
	c.reveal.Duration = e.Th.Motion.HoverOut
	if hot {
		c.reveal.Duration = e.Th.Motion.HoverIn
	}
	c.reveal.Ease = anim.EaseChisel
	target := float32(0)
	if hot {
		target = 1
	}
	c.reveal.Set(gtx, target)
	reveal := c.reveal.Value(gtx)

	cardGTX := gtx
	cardGTX.Constraints = layout.Exact(size)
	dims := c.click.Layout(cardGTX, func(gtx layout.Context) layout.Dimensions {
		bounds := image.Rectangle{Max: size}
		if reveal > 0 {
			kit.FillCut(gtx.Ops, bounds, gtx.Dp(e.Th.Cut.Mid), palette.WithAlpha(e.Th.R.Stone1, reveal))
		}
		insetGTX := gtx
		insetGTX.Constraints = layout.Exact(image.Pt(artSize, max(0, size.Y-2*padding)))
		offset := op.Offset(image.Pt(padding, padding)).Push(gtx.Ops)
		layout.Flex{Axis: layout.Vertical, Alignment: layout.Start}.Layout(insetGTX,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return c.artLayout(gtx, e, item, artSize) }),
			layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return kit.Label{Style: e.Th.T.Label, Color: e.Th.R.Fg, MaxLines: 2}.Layout(gtx, e.Th.Theme, item.Title)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return kit.Label{Style: e.Th.T.Caption, Color: e.Th.R.Fg3, MaxLines: 1}.Layout(gtx, e.Th.Theme, browseSubtitle(item))
			}),
		)
		offset.Pop()
		if reveal > 0 {
			rise := int(float32(gtx.Dp(4)) * (1 - reveal))
			plateOffset := op.Offset(image.Pt(padding+artSize-gtx.Dp(48), padding+artSize-gtx.Dp(48)+rise)).Push(gtx.Ops)
			c.playPlate(gtx, e, reveal)
			plateOffset.Pop()
		}
		return layout.Dimensions{Size: size}
	})
	pass := pointer.PassOp{}.Push(gtx.Ops)
	area := clip.Rect(image.Rectangle{Max: dims.Size}).Push(gtx.Ops)
	event.Op(gtx.Ops, &c.contextTag)
	area.Pop()
	pass.Pop()
	return dims
}

// Shelf is a titled horizontal strip of retained cards.
type Shelf struct {
	scroll         look.HScroller
	cards          map[string]*Card
	more           kit.Plaque
	prev           kit.IconPlaque
	next           kit.IconPlaque
	headerChildren [7]layout.FlexChild
	itemChildren   [2]layout.FlexChild
}

func (s *Shelf) card(item model.BrowseItem) *Card {
	if s.cards == nil {
		s.cards = make(map[string]*Card)
	}
	key := browseKey(item)
	card := s.cards[key]
	if card == nil {
		card = new(Card)
		s.cards[key] = card
	}
	return card
}

func (s *Shelf) header(gtx layout.Context, e *env.Env, title string, more *env.Route, steps bool) layout.Dimensions {
	if s.more.Clicked(gtx) && more != nil && e.Nav != nil {
		e.Nav.Push(*more)
	}
	if s.prev.Clicked(gtx) {
		s.scroll.Page(gtx, -1)
	}
	if s.next.Clicked(gtx) {
		s.scroll.Page(gtx, 1)
	}
	children := s.headerChildren[:1]
	children[0] = layout.Flexed(1, kit.Text(e.Th.Theme, e.Th.T.Section, e.Th.R.Fg, title))
	if more != nil {
		s.more.Kind = kit.PlaqueQuiet
		s.more.Small = true
		s.more.Label = "Show all"
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.more.Layout(gtx, e.Th.Theme)
		}))
	}
	if steps {
		s.prev.Kind, s.prev.Small, s.prev.Icon, s.prev.Description = kit.PlaqueQuiet, true, look.Icon.Back, "Previous cards"
		s.next.Kind, s.next.Small, s.next.Icon, s.next.Description = kit.PlaqueQuiet, true, look.Icon.Forward, "Next cards"
		children = append(children,
			layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if !s.scroll.CanPage(-1) {
					gtx = gtx.Disabled()
				}
				return s.prev.Layout(gtx, e.Th.Theme)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if !s.scroll.CanPage(1) {
					gtx = gtx.Disabled()
				}
				return s.next.Layout(gtx, e.Th.Theme)
			}),
		)
	}
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
}

// Layout draws a titled shelf and optional route to its full list.
func (s *Shelf) Layout(gtx layout.Context, e *env.Env, title string, items []model.BrowseItem, more *env.Route) layout.Dimensions {
	width := cardSize(e)
	padding := gtx.Dp(cardPlateInset)
	viewport := gtx.Constraints.Max.X
	footprint := len(items)*gtx.Dp(width) + max(0, len(items)-1)*gtx.Dp(21)
	steps := footprint > viewport
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return s.header(gtx, e, title, more, steps) }),
		layout.Rigid(layout.Spacer{Height: unit.Dp(13)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			scrollGTX := gtx
			scrollGTX.Constraints.Min.X = viewport + 2*padding
			scrollGTX.Constraints.Max.X = viewport + 2*padding
			offset := op.Offset(image.Pt(-padding, 0)).Push(gtx.Ops)
			dims := s.scroll.Layout(scrollGTX, e.Th, len(items), func(gtx layout.Context, index int) layout.Dimensions {
				children := s.itemChildren[:1]
				children[0] = layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return s.card(items[index]).Layout(gtx, e, items[index], width)
				})
				if index+1 < len(items) {
					children = append(children, layout.Rigid(cardPlateGap))
				}
				return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, children...)
			})
			offset.Pop()
			dims.Size.X = viewport
			return dims
		}),
	)
}

// Grid is a retained wrapping card grid virtualised one row at a time.
type Grid struct {
	cards map[string]*Card
}

func (g *Grid) card(item model.BrowseItem) *Card {
	if g.cards == nil {
		g.cards = make(map[string]*Card)
	}
	key := browseKey(item)
	card := g.cards[key]
	if card == nil {
		card = new(Card)
		g.cards[key] = card
	}
	return card
}

func gridColumns(width, gap, minWidth int) int {
	if width <= 0 {
		return 1
	}
	return max(1, (width+gap)/(minWidth+gap))
}

// Rows returns the number of virtual rows required at the supplied logical dp width.
func (g *Grid) Rows(width int, count int) int {
	if count <= 0 {
		return 0
	}
	columns := gridColumns(width, 21, 160)
	return (count + columns - 1) / columns
}

// Row draws one virtual grid row.
func (g *Grid) Row(gtx layout.Context, e *env.Env, items []model.BrowseItem, row int) layout.Dimensions {
	viewport := gtx.Constraints.Max.X
	gap := gtx.Dp(21)
	padding := gtx.Dp(cardPlateInset)
	columns := gridColumns(viewport, gap, gtx.Dp(160))
	start := row * columns
	if start < 0 || start >= len(items) {
		return layout.Dimensions{}
	}
	end := min(len(items), start+columns)
	available := viewport - gap*(columns-1)
	width := min(gtx.Dp(200), max(gtx.Dp(160), available/max(1, columns)))
	children := make([]layout.FlexChild, 0, (end-start)*2)
	for index := start; index < end; index++ {
		item := items[index]
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return g.card(item).Layout(gtx, e, item, gtx.Metric.PxToDp(width))
		}))
		if index+1 < end {
			children = append(children, layout.Rigid(cardPlateGap))
		}
	}
	rowGTX := gtx
	rowGTX.Constraints.Min.X = viewport + 2*padding
	rowGTX.Constraints.Max.X = viewport + 2*padding
	offset := op.Offset(image.Pt(-padding, 0)).Push(gtx.Ops)
	dims := layout.Flex{Axis: layout.Horizontal, Alignment: layout.Start}.Layout(rowGTX, children...)
	offset.Pop()
	dims.Size.X = viewport
	return dims
}

// QuickTile is a compact home shortcut with a trailing play action.
type QuickTile struct {
	Preview kit.State

	click      kit.Click
	play       kit.Click
	art        look.ArtView
	hover      anim.Tween
	contextTag int
	context    contextRelease
	menu       ItemMenu
	children   [7]layout.FlexChild
}

func (q *QuickTile) process(gtx layout.Context, e *env.Env, item model.BrowseItem) {
	if q.play.Clicked(gtx) {
		playBrowseItem(e, item)
	}
	for q.click.Clicked(gtx) {
		act.OpenCard(e, item)
	}
	q.context.poll(gtx, &q.contextTag, func() {
		target := env.MenuTarget{Card: &item}
		if e.Menu != nil {
			e.Menu(target)
		} else if e.Overlays != nil {
			q.menu.OpenAt(e, target, e.Overlays.Press())
		}
	})
}

// Layout draws a 56 dp shortcut tile.
func (q *QuickTile) Layout(gtx layout.Context, e *env.Env, item model.BrowseItem) layout.Dimensions {
	q.process(gtx, e, item)
	size := gtx.Constraints.Constrain(image.Pt(gtx.Constraints.Max.X, gtx.Dp(56)))
	hot := q.click.Hovered() || q.Preview == kit.StateHover
	q.hover.Duration = e.Th.Motion.HoverOut
	if hot {
		q.hover.Duration = e.Th.Motion.HoverIn
	}
	q.hover.Ease = anim.EaseChisel
	target := float32(0)
	if hot {
		target = 1
	}
	q.hover.Set(gtx, target)
	hover := q.hover.Value(gtx)
	childGTX := gtx
	childGTX.Constraints = layout.Exact(size)
	dims := q.click.Layout(childGTX, func(gtx layout.Context) layout.Dimensions {
		plane := palette.Mix(e.Th.R.Stone1, e.Th.R.Stone3, hover)
		kit.FillCutCorners(gtx.Ops, image.Rectangle{Max: size}, gtx.Dp(e.Th.Cut.Small), 0, 0, 0, plane)
		artSize := min(size.Y, size.X)
		children := q.children[:3]
		children[0] = layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return q.art.Layout(gtx, e.Th, e.Art, textValue(item.Thumbnail), image.Pt(artSize, artSize), look.Shape{Cut: e.Th.Cut.Small, Corners: [4]bool{true, false, false, false}}, e.Th.Motion.Navigate)
		})
		children[1] = layout.Rigid(layout.Spacer{Width: unit.Dp(13)}.Layout)
		children[2] = layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.X = gtx.Constraints.Max.X
				return kit.Label{Style: e.Th.T.Label, Color: e.Th.R.Fg, MaxLines: 2}.Layout(gtx, e.Th.Theme, item.Title)
			})
		})
		if hover > 0 {
			children = append(children,
				layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					opacity := paint.PushOpacity(gtx.Ops, hover)
					dims := q.play.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						pixels := gtx.Dp(32)
						gtx.Constraints = layout.Exact(image.Pt(pixels, pixels))
						kit.FillCut(gtx.Ops, image.Rect(0, 0, pixels, pixels), gtx.Dp(e.Th.Cut.Small), e.Th.R.Stone4)
						return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions { return drawIcon(gtx, look.Icon.Play, e.Th.R.Fg, 18) })
					})
					opacity.Pop()
					return dims
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
			)
		}
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
	})
	pass := pointer.PassOp{}.Push(gtx.Ops)
	area := clip.Rect(image.Rectangle{Max: dims.Size}).Push(gtx.Ops)
	event.Op(gtx.Ops, &q.contextTag)
	area.Pop()
	pass.Pop()
	return dims
}
