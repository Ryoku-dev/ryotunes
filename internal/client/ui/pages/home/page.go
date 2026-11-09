package home

import (
	"strings"

	"gioui.org/layout"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryotunes/internal/client/state"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/widgets"
	"github.com/ryoku-dev/ryotunes/internal/model"
)

type page struct {
	scroll look.Scroller

	started     bool
	loading     bool
	loaded      bool
	errorText   string
	request     uint64
	sessionKey  string
	feed        model.Feed
	feedParams  string
	loadingMore bool
	moreRequest uint64
	moreFailed  string

	feedFilters kit.Filters
	retry       kit.Plaque
	signIn      kit.Plaque
	tiles       map[string]*widgets.QuickTile
	recent      widgets.Shelf
	picked      widgets.Shelf
	shelves     map[string]*widgets.Shelf
	insights    insightsView
}

// New returns a retained Home page for the route.
func New(r env.Route) env.Page {
	return &page{
		feedParams: strings.TrimSpace(r.Params),
		tiles:      make(map[string]*widgets.QuickTile),
		shelves:    make(map[string]*widgets.Shelf),
	}
}

func (p *page) Title() string { return "Home" }

func (p *page) Refresh(e *env.Env) {
	p.sessionKey = homeSession(e.Store.Snapshot())
	p.fetchHome(e)
	p.insights.refresh(e)
}

func (p *page) Layout(gtx layout.Context, e *env.Env) layout.Dimensions {
	snapshot := e.Store.Snapshot()
	p.handleControls(gtx, e)
	session := homeSession(snapshot)
	if !p.started {
		p.sessionKey = session
		p.fetchHome(e)
		p.insights.refresh(e)
	} else if p.sessionKey != session && !p.loading {
		p.sessionKey = session
		p.feedParams = ""
		p.fetchHome(e)
	}

	children := p.content(e, snapshot)
	inset := look.ContentInset(e.Th.Class)
	return layout.Inset{Bottom: unit.Dp(21)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return p.scroll.Layout(gtx, e.Th, len(children), func(gtx layout.Context, index int) layout.Dimensions {
			if index >= len(children)-2 {
				p.loadMore(e)
			}
			bottom := unit.Dp(34)
			if index == len(children)-1 {
				bottom = 0
			}
			if index == 0 {
				return layout.Inset{Bottom: bottom}.Layout(gtx, children[index])
			}
			return layout.Inset{Right: inset, Bottom: bottom, Left: inset}.Layout(gtx, children[index])
		})
	})
}

func (p *page) handleControls(gtx layout.Context, e *env.Env) {
	if p.retry.Clicked(gtx) {
		p.Refresh(e)
	}
	if p.signIn.Clicked(gtx) && e.Nav != nil {
		e.Nav.Push(env.Route{Kind: env.RouteSettings, Title: "Settings"})
	}
	if selected, changed := p.feedFilters.Changed(); changed && selected >= 0 && selected < len(p.feed.Chips) {
		p.feedParams = p.feed.Chips[selected].Params
		p.fetchHome(e)
	}
}

func (p *page) header(e *env.Env, snapshot state.State) layout.Widget {
	provider := look.ProviderName(snapshot.Provider)
	signedIn := providerSignedIn(snapshot)
	proof := "Picked from " + provider + " for you"
	if !signedIn {
		proof = "Sign in to shape your " + provider + " home"
	}
	return func(gtx layout.Context) layout.Dimensions {
		cut := gtx.Dp(e.Th.Cut.Large)
		inset := look.ContentInset(e.Th.Class)
		return look.Field(gtx, e.Th, [4]int{cut, cut, 0, 0}, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: inset, Right: inset, Bottom: inset, Left: inset}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				name := accountName(snapshot)
				copy := layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical, Alignment: layout.Start}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return kit.Label{Style: e.Th.T.Sentence, Color: e.Th.R.Fg, MaxLines: 2}.Layout(gtx, e.Th.Theme, greeting(e.Now.Hour(), name))
						}),
						layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return kit.Label{Style: e.Th.T.Proof, Color: e.Th.R.Fg2, MaxLines: 2}.Layout(gtx, e.Th.Theme, proof)
						}),
					)
				})
				children := []layout.FlexChild{copy}
				if !signedIn {
					p.signIn.Kind = kit.PlaqueLine
					p.signIn.Label = "Sign in"
					p.signIn.Small = true
					children = append(children,
						layout.Rigid(layout.Spacer{Width: unit.Dp(21)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions { return p.signIn.Layout(gtx, e.Th.Theme) }),
					)
				}
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
			})
		})
	}
}

func greeting(hour int, name *string) string {
	var text string
	switch {
	case hour >= 0 && hour < 4:
		text = "Still up"
	case hour < 12:
		text = "Good morning"
	case hour < 17:
		text = "Good afternoon"
	case hour < 21:
		text = "Good evening"
	default:
		text = "Good night"
	}
	if name != nil && strings.TrimSpace(*name) != "" {
		text += ", " + strings.TrimSpace(*name)
	}
	return text
}

func accountName(snapshot state.State) *string {
	switch snapshot.Provider {
	case model.ProviderSpotify:
		return snapshot.Spotify.Name
	case model.ProviderSoundCloud:
		return snapshot.SoundCloud.Name
	case model.ProviderTidal:
		return snapshot.Tidal.Name
	default:
		return snapshot.Auth.Name
	}
}

func providerSignedIn(snapshot state.State) bool {
	switch snapshot.Provider {
	case model.ProviderYouTube:
		return snapshot.Auth.SignedIn
	case model.ProviderSpotify:
		return snapshot.Spotify.SignedIn
	case model.ProviderSoundCloud:
		return snapshot.SoundCloud.SignedIn
	case model.ProviderTidal:
		return snapshot.Tidal.SignedIn
	default:
		return true
	}
}

func homeSession(snapshot state.State) string {
	state := ":signed-out"
	if providerSignedIn(snapshot) {
		state = ":signed-in"
	}
	return string(snapshot.Provider) + state
}

func (p *page) feedFiltersLayout(e *env.Env) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		labels := make([]string, len(p.feed.Chips))
		selected := 0
		for i, chip := range p.feed.Chips {
			labels[i] = chip.Title
			if chip.Params == p.feedParams {
				selected = i
			}
		}
		p.feedFilters.Labels = labels
		p.feedFilters.Selected = selected
		return p.feedFilters.Layout(gtx, e.Th.Theme)
	}
}
