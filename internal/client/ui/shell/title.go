package shell

import (
	"image"
	"strings"

	"gioui.org/layout"
	"gioui.org/widget"

	"github.com/ryoku-dev/ryotunes/internal/client/lithic"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
)

type titleBar struct {
	back   lithic.IconButton
	search lithic.Search
}

func (t *titleBar) Layout(gtx layout.Context, e *env.Env, page env.Page) layout.Dimensions {
	if e.Nav.(*navigator).takeSearchFocus() {
		t.search.Focus(gtx)
	}
	t.search.Submit = true
	for {
		event, ok := t.search.Update(gtx)
		if !ok {
			break
		}
		if submitted, ok := event.(widget.SubmitEvent); ok {
			query := strings.Join(strings.Fields(submitted.Text), " ")
			if query != "" {
				e.Nav.Push(env.Route{Kind: env.RouteSearch, Query: query, Title: "Search"})
			}
		}
	}

	canBack := len(e.Nav.Stack()) > 1
	back := func(gtx layout.Context) layout.Dimensions {
		if !canBack {
			return layout.Dimensions{}
		}
		return t.back.Layout(gtx, e.Th, lithic.Quiet, lithic.Icon("arrow_back"), "Back")
	}
	title := ""
	switch e.Nav.Current().Kind {
	case env.RouteAlbum, env.RouteArtist, env.RoutePlaylist, env.RouteList:
		if page != nil {
			title = page.Title()
		}
	}
	search := func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min = image.Point{}
		gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(360))
		return t.search.Layout(gtx, e.Th, "Search tracks, albums, artists…", "/")
	}
	dims := layout.Inset{Left: 21, Right: 124, Top: 6, Bottom: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(back),
			layout.Rigid(layout.Spacer{Width: 13}.Layout),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return lithic.Label{Style: e.Th.T.Section, Color: e.Th.R.Fg, MaxLines: 1}.Layout(gtx, e.Th, title)
			}),
			layout.Rigid(layout.Spacer{Width: 21}.Layout),
			layout.Flexed(.55, search),
		)
	})
	if canBack && t.back.Clicked(gtx) {
		e.Nav.Back()
	}
	return dims
}
