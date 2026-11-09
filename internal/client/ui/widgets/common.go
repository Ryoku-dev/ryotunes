package widgets

import (
	"image"

	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"

	"github.com/ryoku-dev/ryotunes/internal/client/art"
	"github.com/ryoku-dev/ryotunes/internal/client/lithic"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/model"
)

// Art draws artwork from the shared cache inside a cut cover.
func Art(gtx layout.Context, e *env.Env, url string, size unit.Dp, cut unit.Dp) layout.Dimensions {
	px := gtx.Dp(size)
	bounds := image.Pt(px, px)
	gtx.Constraints = layout.Exact(bounds)
	roles := e.Th.R
	lithic.FillCut(gtx.Ops, image.Rectangle{Max: bounds}, gtx.Dp(cut), roles.Stone3)
	img := art.Placeholder()
	if e != nil && e.Art != nil && url != "" {
		img, _ = e.Art.Get(url, px)
	}
	return lithic.CutImage(gtx, img, gtx.Dp(cut), widget.Cover)
}

// ProviderBadge returns a compact provider flake and word.
func ProviderBadge(e *env.Env, provider model.Provider) layout.Widget {
	label := ""
	switch provider {
	case model.ProviderYouTube:
		label = "YouTube"
	case model.ProviderSpotify:
		label = "Spotify"
	case model.ProviderSoundCloud:
		label = "SoundCloud"
	case model.ProviderTidal:
		label = "TIDAL"
	case model.ProviderLocal:
		label = "Local"
	case model.ProviderRadio:
		label = "Radio"
	}
	return func(gtx layout.Context) layout.Dimensions {
		if label == "" {
			return layout.Dimensions{}
		}
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return lithic.Flake(gtx, e.Th, lithic.Idle, 0, 8)
			}),
			layout.Rigid(layout.Spacer{Width: e.Th.Sp.S2}.Layout),
			layout.Rigid(lithic.Text(e.Th, e.Th.T.Caption, e.Th.R.Fg3, label)),
		)
	}
}

// PageHeader returns a page title, optional sentence, and trailing actions.
func PageHeader(e *env.Env, title, sentence string, trailing layout.Widget) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		copy := func(gtx layout.Context) layout.Dimensions {
			children := []layout.FlexChild{
				layout.Rigid(lithic.Text(e.Th, e.Th.T.Section, e.Th.R.Fg, title)),
			}
			if sentence != "" {
				children = append(children,
					layout.Rigid(layout.Spacer{Height: e.Th.Sp.S2}.Layout),
					layout.Rigid(lithic.Text(e.Th, e.Th.T.Body, e.Th.R.Fg2, sentence)),
				)
			}
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
		}
		children := []layout.FlexChild{layout.Flexed(1, copy)}
		if trailing != nil {
			children = append(children,
				layout.Rigid(layout.Spacer{Width: e.Th.Sp.S4}.Layout),
				layout.Rigid(trailing),
			)
		}
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
	}
}

// Loading returns a static Lithic loading skeleton.
func Loading(e *env.Env) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: e.Th.Sp.S4, Right: e.Th.Sp.S4, Bottom: e.Th.Sp.S4, Left: e.Th.Sp.S4}.Layout(gtx, lithic.Skeleton(e.Th, 5))
	}
}

// Failed returns an error empty state with an optional retry plaque.
func Failed(e *env.Env, message string, retry *lithic.Button) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		actions := []lithic.EmptyAction{}
		if retry != nil {
			actions = append(actions, lithic.EmptyAction{Button: retry, Kind: lithic.Line, Icon: lithic.Icon("refresh"), Label: "Try again"})
		}
		return lithic.Empty(e.Th, lithic.Icon("error"), "Could not load", message, actions...)(gtx)
	}
}

// EmptyState returns a centred next-step state.
func EmptyState(e *env.Env, icon, title, body string, actions ...lithic.EmptyAction) layout.Widget {
	glyph := lithic.Icon(icon)
	if glyph == nil {
		glyph = lithic.Icon("info")
	}
	return lithic.Empty(e.Th, glyph, title, body, actions...)
}
