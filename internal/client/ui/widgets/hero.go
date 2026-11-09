package widgets

import (
	"image"
	"strings"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op/clip"

	"github.com/ryoku-dev/ryotunes/internal/client/lithic"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/model"
)

// HeroContent describes the artwork, copy, facts, tone, and actions in a detail hero.
type HeroContent struct {
	Art                 string
	Collage, Octagon    bool
	Kicker, Name, Proof string
	Facts               []string
	Tone                lithic.Tone
	Acts                layout.Widget
}

// Hero is the retained mineral field at the head of a detail page.
type Hero struct {
	field lithic.Field
}

func heroArt(gtx layout.Context, e *env.Env, c HeroContent) layout.Dimensions {
	url := c.Art
	if c.Collage && !strings.HasPrefix(url, model.CollageThumbnailPrefix) && strings.HasPrefix(strings.TrimSpace(url), "[") {
		url = model.CollageThumbnailPrefix + url
	}
	if !c.Octagon {
		return Art(gtx, e, url, 160, 7)
	}
	px := gtx.Dp(160)
	gtx.Constraints = layout.Exact(image.Pt(px, px))
	mask := clip.Outline{Path: lithic.Octagon(gtx.Ops, f32.Pt(float32(px)/2, float32(px)/2), float32(px)/2)}.Op().Push(gtx.Ops)
	dims := Art(gtx, e, url, 160, 0)
	mask.Pop()
	return dims
}

// Layout renders a detail hero on an animated mineral field.
func (h *Hero) Layout(gtx layout.Context, e *env.Env, c HeroContent) layout.Dimensions {
	return h.field.Layout(gtx, e.Th, c.Tone, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: 30, Right: 36, Bottom: 26, Left: 36}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			copy := func(gtx layout.Context) layout.Dimensions {
				nameStyle := e.Th.T.Name
				if gtx.Constraints.Max.X < gtx.Dp(640) {
					nameStyle = e.Th.T.Sentence
				}
				children := make([]layout.FlexChild, 0, 9)
				if c.Kicker != "" {
					children = append(children,
						layout.Rigid(lithic.Text(e.Th, e.Th.T.Group, e.Th.R.Fg3, c.Kicker)),
						layout.Rigid(layout.Spacer{Height: e.Th.Sp.S2}.Layout),
					)
				}
				children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return lithic.Label{Style: nameStyle, Color: e.Th.R.Fg, MaxLines: 2}.Layout(gtx, e.Th, c.Name)
				}))
				if c.Proof != "" {
					children = append(children,
						layout.Rigid(layout.Spacer{Height: e.Th.Sp.S2}.Layout),
						layout.Rigid(lithic.Text(e.Th, e.Th.T.Proof, e.Th.R.Fg2, c.Proof)),
					)
				}
				if len(c.Facts) > 0 {
					children = append(children,
						layout.Rigid(layout.Spacer{Height: e.Th.Sp.S1}.Layout),
						layout.Rigid(lithic.Text(e.Th, e.Th.T.Mono, e.Th.R.Fg3, strings.Join(c.Facts, " · "))),
					)
				}
				if c.Acts != nil {
					children = append(children,
						layout.Rigid(layout.Spacer{Height: e.Th.Sp.S4}.Layout),
						layout.Rigid(c.Acts),
					)
				}
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
			}
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return heroArt(gtx, e, c) }),
				layout.Rigid(layout.Spacer{Width: e.Th.Sp.S4}.Layout),
				layout.Flexed(1, copy),
			)
		})
	})
}
