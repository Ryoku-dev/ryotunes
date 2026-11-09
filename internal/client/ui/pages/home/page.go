package home

import (
	"strings"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryotunes/internal/client/state"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/widgets"
	"github.com/ryoku-dev/ryotunes/internal/model"
)

type page struct {
	scroll look.Scroller

	started       bool
	loading       bool
	loaded        bool
	errorText     string
	request       uint64
	sessionKey    homeSessionKey
	feed          model.Feed
	allFeed       model.Feed
	allLoaded     bool
	initialParams string
	activeChip    int
	pendingChip   int
	chips         []model.FeedChip
	chipLabels    []string
	loadingMore   bool
	moreRequest   uint64
	moreFailed    string

	contentRevision uint64
	contentKey      homeContentKey
	children        []layout.Widget
	feedRevision    uint64
	shelfRevision   uint64
	shelfChildren   []layout.Widget
	projectionKey   homeProjectionKey
	quickWidget     layout.Widget
	recentWidget    layout.Widget
	pickedWidget    layout.Widget
	projectionReady bool

	feedFilters kit.Filters
	retry       kit.Plaque
	signIn      kit.Plaque
	header      widgets.Header
	tiles       map[string]*widgets.QuickTile
	recent      widgets.Shelf
	picked      widgets.Shelf
	shelves     map[string]*widgets.Shelf
	insights    insightsView
}

// New returns a retained Home page for the route.
func New(r env.Route) env.Page {
	return &page{
		initialParams: strings.TrimSpace(r.Params),
		pendingChip:   -1,
		header:        widgets.Header{Sentence: true},
		tiles:         make(map[string]*widgets.QuickTile),
		shelves:       make(map[string]*widgets.Shelf),
	}
}

func (p *page) Title() string { return "Home" }

func (p *page) Refresh(e *env.Env) {
	p.sessionKey = homeSession(e.Store.Snapshot())
	if p.activeChip > 0 {
		p.fetchChip(e, p.activeChip)
	} else {
		p.fetchAll(e)
	}
	p.insights.refresh(e)
}

func (p *page) Layout(gtx layout.Context, e *env.Env) layout.Dimensions {
	snapshot := e.Store.Snapshot()
	session := homeSession(snapshot)
	if !p.started {
		p.sessionKey = session
		p.fetchAll(e)
		p.insights.refresh(e)
	} else if p.sessionKey != session && !p.loading {
		p.reset(session)
		p.fetchAll(e)
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

func (p *page) reset(session homeSessionKey) {
	p.request++
	p.moreRequest++
	p.sessionKey = session
	p.started = false
	p.loading = false
	p.loaded = false
	p.errorText = ""
	p.feed = model.Feed{}
	p.allFeed = model.Feed{}
	p.allLoaded = false
	p.activeChip = 0
	p.pendingChip = -1
	p.chips = nil
	p.chipLabels = nil
	p.feedFilters.Labels = nil
	p.feedFilters.Selected = 0
	p.loadingMore = false
	p.moreFailed = ""
	p.markFeedChanged()
}

func (p *page) headerLayout(e *env.Env, snapshot state.State) layout.Widget {
	provider := look.ProviderName(snapshot.Provider)
	signedIn := providerSignedIn(snapshot)
	proof := "Picked from " + provider + " for you"
	if !signedIn {
		proof = "Sign in to shape your " + provider + " home"
	}
	var trailing layout.Widget
	if !signedIn {
		trailing = func(gtx layout.Context) layout.Dimensions {
			p.signIn.Kind = kit.PlaqueLine
			p.signIn.Label = "Sign in"
			p.signIn.Small = true
			dimensions := p.signIn.Layout(gtx, e.Th.Theme)
			if p.signIn.Clicked(gtx) {
				act.SignIn(e, snapshot.Provider)
				gtx.Execute(op.InvalidateCmd{})
			}
			return dimensions
		}
	}
	title := greeting(e.Now.Hour(), accountName(snapshot))
	return func(gtx layout.Context) layout.Dimensions {
		return p.header.Layout(gtx, e, title, proof, trailing)
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

type homeSessionKey struct {
	provider model.Provider
	signedIn bool
}

func homeSession(snapshot state.State) homeSessionKey {
	return homeSessionKey{provider: snapshot.Provider, signedIn: providerSignedIn(snapshot)}
}

func (p *page) feedFiltersLayout(e *env.Env) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		dimensions := p.feedFilters.Layout(gtx, e.Th.Theme)
		if selected, changed := p.feedFilters.Changed(); changed {
			p.selectChip(e, selected)
			gtx.Execute(op.InvalidateCmd{})
		}
		return dimensions
	}
}
