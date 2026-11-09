package widgets

import (
	"image"
	"strings"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/unit"
	"gioui.org/widget"

	"github.com/ryoku-dev/ryotunes/internal/client/lithic"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/model"
)

// Card is one retained browse card.
type Card struct {
	click   lithic.Click
	hover   lithic.Tween
	press   lithic.Tween
	clicked bool
}

// Clicked reports and clears a card activation.
func (c *Card) Clicked() bool {
	clicked := c.clicked
	c.clicked = false
	return clicked
}

func cardIsArtist(item model.BrowseItem) bool {
	kind := strings.ToLower(item.Kind)
	return kind == "artist" || kind == "channel" || kind == "user"
}

func cardArt(gtx layout.Context, e *env.Env, item model.BrowseItem, size unit.Dp) layout.Dimensions {
	url := ""
	if item.Thumbnail != nil {
		url = *item.Thumbnail
	}
	if !cardIsArtist(item) {
		return Art(gtx, e, url, size, 7)
	}
	px := gtx.Dp(size)
	gtx.Constraints = layout.Exact(image.Pt(px, px))
	center := f32.Pt(float32(px)/2, float32(px)/2)
	mask := clip.Outline{Path: lithic.Octagon(gtx.Ops, center, float32(px)/2)}.Op().Push(gtx.Ops)
	dims := Art(gtx, e, url, size, 0)
	mask.Pop()
	return dims
}

// Layout renders a cut browse card with stepped hover and chipped press states.
func (c *Card) Layout(gtx layout.Context, e *env.Env, item model.BrowseItem, size unit.Dp) layout.Dimensions {
	if c.click.Clicked(gtx) {
		c.clicked = true
	}
	width := gtx.Dp(size)
	gtx.Constraints.Min.X = min(gtx.Constraints.Max.X, width)
	gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, width)
	return c.click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		c.hover.Duration = e.Th.Motion.HoverIn
		c.hover.Ease = lithic.EaseChisel
		if !c.click.Hovered() {
			c.hover.Duration = e.Th.Motion.HoverOut
		}
		c.press.Duration = e.Th.Motion.Press
		c.press.Ease = lithic.EaseChisel
		hoverTarget, pressTarget := float32(0), float32(0)
		if c.click.Hovered() {
			hoverTarget = 1
		}
		if c.click.Pressed() {
			pressTarget = 1
		}
		c.hover.Set(gtx, hoverTarget)
		c.press.Set(gtx, pressTarget)
		hover := c.hover.Value(gtx)
		pressed := c.press.Value(gtx)
		cut := gtx.Dp(7) + int(float32(gtx.Dp(4))*pressed+.5)
		sink := int(float32(gtx.Dp(1))*pressed + .5)

		contentMacro := op.Record(gtx.Ops)
		contentOffset := op.Offset(image.Pt(0, sink)).Push(gtx.Ops)
		children := []layout.FlexChild{
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				artSize := size - 2*e.Th.Sp.S2
				return cardArt(gtx, e, item, max(unit.Dp(0), artSize))
			}),
			layout.Rigid(layout.Spacer{Height: e.Th.Sp.S3}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return lithic.Label{Style: e.Th.T.Row, Color: e.Th.R.Fg, MaxLines: 1}.Layout(gtx, e.Th, item.Title)
			}),
		}
		if item.Subtitle != nil && *item.Subtitle != "" {
			children = append(children,
				layout.Rigid(layout.Spacer{Height: e.Th.Sp.S1}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return lithic.Label{Style: e.Th.T.Caption, Color: e.Th.R.Fg3, MaxLines: 2}.Layout(gtx, e.Th, *item.Subtitle)
				}),
			)
		}
		dims := layout.Inset{Top: e.Th.Sp.S2, Right: e.Th.Sp.S2, Bottom: e.Th.Sp.S3, Left: e.Th.Sp.S2}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
		})
		contentOffset.Pop()
		contentCall := contentMacro.Stop()
		fill := lithic.Mix(e.Th.R.Stone2, e.Th.R.Stone3, hover)
		fill = lithic.Mix(fill, e.Th.R.Fg, .08*pressed)
		lithic.FillCut(gtx.Ops, image.Rectangle{Max: dims.Size}, cut, fill)
		contentCall.Add(gtx.Ops)
		return dims
	})
}

// Shelf is a titled horizontal row of retained cards.
type Shelf struct {
	list  widget.List
	cards map[string]*Card
	more  lithic.Button
}

func (s *Shelf) card(item model.BrowseItem) *Card {
	if s.cards == nil {
		s.cards = make(map[string]*Card)
	}
	key := item.Kind + "\x00" + item.ID
	card := s.cards[key]
	if card == nil {
		card = new(Card)
		s.cards[key] = card
	}
	return card
}

// Layout renders a shelf and routes card or More activations.
func (s *Shelf) Layout(gtx layout.Context, e *env.Env, shelf model.Shelf) layout.Dimensions {
	if s.more.Clicked(gtx) && shelf.MoreBrowseID != nil && e.Nav != nil {
		params := ""
		if shelf.MoreParams != nil {
			params = *shelf.MoreParams
		}
		e.Nav.Push(env.Route{Kind: env.RouteList, ID: *shelf.MoreBrowseID, Params: params, Title: shelf.Title})
	}
	header := func(gtx layout.Context) layout.Dimensions {
		children := []layout.FlexChild{
			layout.Flexed(1, lithic.Text(e.Th, e.Th.T.Title, e.Th.R.Fg, shelf.Title)),
		}
		if shelf.MoreBrowseID != nil {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				s.more.Small = true
				return s.more.Layout(gtx, e.Th, lithic.Quiet, lithic.Icon("chevron_right"), "More")
			}))
		}
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
	}
	s.list.Axis = layout.Horizontal
	cards := func(gtx layout.Context) layout.Dimensions {
		return s.list.Layout(gtx, len(shelf.Items), func(gtx layout.Context, i int) layout.Dimensions {
			item := shelf.Items[i]
			card := s.card(item)
			gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(172))
			dims := layout.Inset{Right: e.Th.Sp.S3}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return card.Layout(gtx, e, item, 172)
			})
			if card.Clicked() {
				act.OpenCard(e, item)
			}
			return dims
		})
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(header),
		layout.Rigid(layout.Spacer{Height: e.Th.Sp.S3}.Layout),
		layout.Rigid(cards),
	)
}

// CardGrid lays retained cards in responsive columns.
type CardGrid struct {
	cards map[string]*Card
}

func (g *CardGrid) card(item model.BrowseItem) *Card {
	if g.cards == nil {
		g.cards = make(map[string]*Card)
	}
	key := item.Kind + "\x00" + item.ID
	card := g.cards[key]
	if card == nil {
		card = new(Card)
		g.cards[key] = card
	}
	return card
}

// Layout renders a responsive browse-card grid and routes activations.
func (g *CardGrid) Layout(gtx layout.Context, e *env.Env, items []model.BrowseItem) layout.Dimensions {
	return lithic.Columns(gtx, 180, e.Th.Sp.S3, len(items), func(gtx layout.Context, i int) layout.Dimensions {
		item := items[i]
		card := g.card(item)
		size := unit.Dp(float32(gtx.Constraints.Max.X) / gtx.Metric.PxPerDp)
		dims := card.Layout(gtx, e, item, size)
		if card.Clicked() {
			act.OpenCard(e, item)
		}
		return dims
	})
}
