package home

import (
	"strings"

	"gioui.org/layout"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryotunes/internal/client/lithic"
	"github.com/ryoku-dev/ryotunes/internal/client/state"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/widgets"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

var providerOptions = []struct {
	label string
	value model.Provider
}{
	{"YouTube Music", model.ProviderYouTube},
	{"Spotify", model.ProviderSpotify},
	{"SoundCloud", model.ProviderSoundCloud},
	{"Tidal", model.ProviderTidal},
	{"Local", model.ProviderLocal},
}

type page struct {
	route env.Route
	list  layout.List

	started        bool
	loading        bool
	loaded         bool
	errorText      string
	request        uint64
	loadedProvider model.Provider
	sessionKey     string
	feed           model.Feed
	feedParams     string
	loadingMore    bool
	moreRequest    uint64
	moreFailed     string

	providerPicker lithic.Picker
	feedFilters    lithic.Filters
	retry          lithic.Button
	resume         lithic.Button

	recentShelf widgets.Shelf
	pickedGrid  widgets.CardGrid
	shelves     map[string]*widgets.Shelf
	shortcuts   map[string]*shortcutTile
	shortcut    shortcutDialog
	insights    insightsView
	saver       personalSaver
}

// New returns a retained Home page for the route.
func New(r env.Route) env.Page {
	p := &page{
		route:     r,
		list:      layout.List{Axis: layout.Vertical},
		shelves:   make(map[string]*widgets.Shelf),
		shortcuts: make(map[string]*shortcutTile),
	}
	p.feedParams = strings.TrimSpace(r.Params)
	return p
}

func (p *page) Title() string { return "Home" }

func (p *page) Refresh(e *env.Env) {
	p.sessionKey = homeSession(e.Store.Snapshot())
	p.fetchHome(e)
	p.insights.refresh(e)
}

func (p *page) Layout(gtx layout.Context, e *env.Env) layout.Dimensions {
	snapshot := e.Store.Snapshot()
	p.handleControls(gtx, e, snapshot)
	sessionKey := homeSession(snapshot)
	if !p.started {
		p.sessionKey = sessionKey
		p.fetchHome(e)
		p.insights.refresh(e)
	} else if p.sessionKey != sessionKey && !p.loading {
		p.sessionKey = sessionKey
		p.feedParams = ""
		p.fetchHome(e)
	}

	children := []layout.Widget{p.header(e, snapshot)}
	if p.loading && !p.loaded {
		children = append(children, widgets.Loading(e))
	} else if p.errorText != "" && !p.loaded {
		children = append(children, widgets.Failed(e, p.errorText, &p.retry))
	} else {
		children = append(children, p.content(e, snapshot)...)
	}

	inset := layout.Inset{Top: unit.Dp(34), Right: unit.Dp(34), Bottom: unit.Dp(55), Left: unit.Dp(34)}
	dims := inset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return p.list.Layout(gtx, len(children), func(gtx layout.Context, index int) layout.Dimensions {
			if index >= len(children)-2 {
				p.loadMore(e)
			}
			return layout.Inset{Bottom: unit.Dp(34)}.Layout(gtx, children[index])
		})
	})
	p.shortcut.layoutDialog(gtx, e, snapshot.Personal)
	return dims
}

func (p *page) handleControls(gtx layout.Context, e *env.Env, snapshot state.State) {
	if p.retry.Clicked(gtx) {
		p.Refresh(e)
	}
	if selected, changed := p.providerPicker.Changed(gtx); changed && selected >= 0 && selected < len(providerOptions) {
		provider := providerOptions[selected].value
		if provider != snapshot.Provider {
			p.feedParams = ""
			act.Call[protocol.ProviderResult](e, protocol.MethodSetProvider, protocol.ProviderParams{Provider: provider}, func(_ protocol.ProviderResult, err error) {
				if err != nil {
					e.Toast(err.Error(), env.ToastError)
				}
			})
		}
	}
	if selected, changed := p.feedFilters.Changed(gtx); changed && selected >= 0 && selected < len(p.feed.Chips) {
		p.feedParams = p.feed.Chips[selected].Params
		p.fetchHome(e)
	}
	if p.resume.Clicked(gtx) {
		index := snapshot.Queue.CurrentIndex
		if index < 0 && len(snapshot.Queue.Items) != 0 {
			index = 0
		}
		if index >= 0 && index < len(snapshot.Queue.Items) {
			method := protocol.MethodPlayIndex
			var params any = protocol.IndexParams{Index: index}
			if snapshot.Playback.Paused && snapshot.Playback.Now != nil && snapshot.Playback.Now.VideoID == snapshot.Queue.Items[index].VideoID {
				method = protocol.MethodTogglePause
				params = protocol.Null{}
			}
			act.Call[protocol.Null](e, method, params, func(_ protocol.Null, err error) {
				if err != nil {
					e.Toast(err.Error(), env.ToastError)
				}
			})
		}
	}
	p.shortcut.handle(gtx, e, p, snapshot.Personal)
	p.insights.handle(gtx, e)
}

func (p *page) header(e *env.Env, snapshot state.State) layout.Widget {
	sentence := "Pick up where you left off, or find the next thing worth hearing."
	name := snapshot.Auth.Name
	switch snapshot.Provider {
	case model.ProviderSpotify:
		name = snapshot.Spotify.Name
	case model.ProviderSoundCloud:
		name = snapshot.SoundCloud.Name
		if !snapshot.SoundCloud.SignedIn {
			sentence = "SoundCloud · listening as a guest"
		}
	case model.ProviderTidal:
		name = snapshot.Tidal.Name
	}
	return widgets.PageHeader(e, greeting(e.Now.Hour(), name), sentence, func(gtx layout.Context) layout.Dimensions {
		selected := providerIndex(snapshot.Provider)
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Gap: gtx.Dp(13)}.Layout(gtx,
			layout.Rigid(widgets.ProviderBadge(e, snapshot.Provider)),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.X = min(gtx.Constraints.Max.X, gtx.Dp(180))
				labels := make([]string, len(providerOptions))
				for i := range providerOptions {
					labels[i] = providerOptions[i].label
				}
				return p.providerPicker.Layout(gtx, e.Th, "Provider", labels, selected)
			}),
		)
	})
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

func homeSession(snapshot state.State) string {
	signedIn := true
	switch snapshot.Provider {
	case model.ProviderYouTube:
		signedIn = snapshot.Auth.SignedIn
	case model.ProviderSpotify:
		signedIn = snapshot.Spotify.SignedIn
	case model.ProviderSoundCloud:
		signedIn = snapshot.SoundCloud.SignedIn
	case model.ProviderTidal:
		signedIn = snapshot.Tidal.SignedIn
	}
	if signedIn {
		return string(snapshot.Provider) + ":signed-in"
	}
	return string(snapshot.Provider) + ":signed-out"
}

func providerIndex(provider model.Provider) int {
	for i := range providerOptions {
		if providerOptions[i].value == provider {
			return i
		}
	}
	return 0
}

func section(e *env.Env, title string, body layout.Widget) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.Start}.Layout(gtx,
			layout.Rigid(lithic.Text(e.Th, e.Th.T.Section, e.Th.R.Fg, title)),
			layout.Rigid(layout.Spacer{Height: unit.Dp(13)}.Layout),
			layout.Rigid(body),
		)
	}
}
