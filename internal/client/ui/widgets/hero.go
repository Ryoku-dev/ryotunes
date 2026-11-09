package widgets

import (
	"image"
	"strings"
	"unicode"

	"gioui.org/layout"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
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

// Hero is a retained source-tinted page header.
type Hero struct {
	art  look.ArtView
	meta kit.Click
}

func heroArtSize(class look.Class) unit.Dp {
	switch class {
	case look.Compact:
		return 120
	case look.Medium:
		return 160
	default:
		return 200
	}
}

type heroMetaRun struct {
	span kit.Span
	gap  bool
}

func heroMeta(meta []kit.Span) []heroMetaRun {
	out := make([]heroMetaRun, 0, len(meta))
	for _, span := range meta {
		text := span.Text
		words := strings.Fields(text)
		if len(words) == 0 {
			continue
		}
		leading := unicode.IsSpace(rune(text[0]))
		trailing := unicode.IsSpace(rune(text[len(text)-1]))
		span.Text = strings.Join(words, " ")
		if leading {
			span.Text = " " + span.Text
		}
		if trailing {
			span.Text += " "
		}
		run := heroMetaRun{span: span}
		if len(out) > 0 {
			previous := &out[len(out)-1]
			if previous.span.Mono && !span.Mono && (strings.HasSuffix(previous.span.Text, " ") || leading) {
				previous.span.Text = strings.TrimRightFunc(previous.span.Text, unicode.IsSpace)
				run.span.Text = strings.TrimLeftFunc(run.span.Text, unicode.IsSpace)
				run.gap = true
			} else if strings.HasSuffix(previous.span.Text, " ") {
				run.span.Text = strings.TrimLeftFunc(run.span.Text, unicode.IsSpace)
			}
		}
		out = append(out, run)
	}
	return out
}

func heroSentence(gtx layout.Context, e *env.Env, runs []heroMetaRun) layout.Dimensions {
	children := make([]layout.FlexChild, 0, len(runs)*2)
	for _, run := range runs {
		if run.gap {
			children = append(children, layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout))
		}
		span := run.span
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return kit.Sentence(gtx, e.Th.Theme, e.Th.T.Proof, e.Th.R.Fg2, []kit.Span{span})
		}))
	}
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Baseline}.Layout(gtx, children...)
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

func (h *Hero) copy(gtx layout.Context, e *env.Env, content HeroContent, height int) layout.Dimensions {
	for h.meta.Clicked(gtx) {
		if content.MetaAction != nil {
			content.MetaAction()
		}
	}
	meta := heroMeta(content.Meta)
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
	if len(meta) > 0 {
		children = append(children,
			layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if content.MetaAction == nil {
					return heroSentence(gtx, e, meta)
				}
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Baseline}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return h.meta.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return heroSentence(gtx, e, meta[:1])
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return heroSentence(gtx, e, meta[1:])
					}),
				)
			}),
		)
	}
	if content.Acts != nil {
		children = append(children,
			layout.Rigid(layout.Spacer{Height: unit.Dp(13)}.Layout),
			layout.Rigid(content.Acts),
		)
	}
	gtx.Constraints.Min.Y = height
	gtx.Constraints.Max.Y = height
	return layout.S.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.Start}.Layout(gtx, children...)
	})
}

// Layout draws a responsive bottom-aligned hero over the provider field.
func (h *Hero) Layout(gtx layout.Context, e *env.Env, content HeroContent) layout.Dimensions {
	if e == nil || e.Th == nil {
		return layout.Dimensions{}
	}
	artDp := heroArtSize(e.Th.Class)
	artPixels := gtx.Dp(artDp)
	inset := look.ContentInset(e.Th.Class)
	cut := gtx.Dp(e.Th.Cut.Large)
	return look.Field(gtx, e.Th, [4]int{cut, cut, 0, 0}, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: inset, Right: inset, Bottom: inset, Left: inset}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			shape := look.Shape{Cut: 7, Octagon: content.Octagon}
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.End}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return h.art.Layout(gtx, e.Th, e.Art, heroArtURL(content), image.Pt(artPixels, artPixels), shape, e.Th.Motion.Field)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(21)}.Layout),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return h.copy(gtx, e, content, artPixels)
				}),
			)
		})
	})
}

// PageHeader returns a source-field page title with optional proof and actions.
func PageHeader(e *env.Env, title, sentence string, trailing layout.Widget) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		if e == nil || e.Th == nil {
			return layout.Dimensions{}
		}
		cut := gtx.Dp(e.Th.Cut.Large)
		inset := look.ContentInset(e.Th.Class)
		return look.Field(gtx, e.Th, [4]int{cut, cut, 0, 0}, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: inset, Right: inset, Bottom: inset, Left: inset}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				copy := []layout.FlexChild{layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return kit.Label{Style: e.Th.T.Section, Color: e.Th.R.Fg, MaxLines: 2}.Layout(gtx, e.Th.Theme, title)
				})}
				if strings.TrimSpace(sentence) != "" {
					copy = append(copy,
						layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return kit.Label{Style: e.Th.T.Proof, Color: e.Th.R.Fg2, MaxLines: 2}.Layout(gtx, e.Th.Theme, sentence)
						}),
					)
				}
				children := []layout.FlexChild{layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical, Alignment: layout.Start}.Layout(gtx, copy...)
				})}
				if trailing != nil {
					children = append(children,
						layout.Rigid(layout.Spacer{Width: unit.Dp(21)}.Layout),
						layout.Rigid(trailing),
					)
				}
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
			})
		})
	}
}
