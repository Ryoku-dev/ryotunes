package home

import (
	"fmt"

	"gioui.org/layout"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryotunes/internal/client/lithic"
	"github.com/ryoku-dev/ryotunes/internal/client/state"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/widgets"
	"github.com/ryoku-dev/ryotunes/internal/model"
)

func (p *page) content(e *env.Env, snapshot state.State) []layout.Widget {
	children := make([]layout.Widget, 0, 10+len(p.feed.Sections))
	if len(snapshot.Queue.Items) != 0 {
		children = append(children, section(e, "Continue listening", p.continueListening(e, snapshot)))
	}
	recent := recentItems(snapshot.Personal)
	if len(recent) == 0 {
		children = append(children, section(e, "Recently played", lithic.Text(e.Th, e.Th.T.Body, e.Th.R.Fg2, "Play something and it will appear here.")))
	} else {
		children = append(children, func(gtx layout.Context) layout.Dimensions {
			return p.recentShelf.Layout(gtx, e, model.Shelf{Title: "Recently played", Items: recent})
		})
	}
	children = append(children, section(e, "Shortcuts", p.shortcutsLayout(e, snapshot.Personal)))
	children = append(children, section(e, "Picked for you", p.pickedLayout(e, snapshot.Personal)))
	if snapshot.Provider == model.ProviderTidal && !snapshot.Tidal.SignedIn {
		children = append(children, widgets.EmptyState(e, "login", "Sign in to Tidal", "Sign in from Settings to see your Tidal home feed."))
	} else {
		if len(p.feed.Chips) != 0 {
			children = append(children, p.feedChips(e))
		}
		for index, shelf := range p.feed.Sections {
			key := fmt.Sprintf("%d:%s", index, shelf.Title)
			view := p.shelves[key]
			if view == nil {
				view = &widgets.Shelf{}
				p.shelves[key] = view
			}
			modelShelf := shelf
			children = append(children, func(gtx layout.Context) layout.Dimensions {
				return view.Layout(gtx, e, modelShelf)
			})
		}
	}
	children = append(children, section(e, "Listening insights", p.insights.layout(e)))
	return children
}

func (p *page) continueListening(e *env.Env, snapshot state.State) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		index := snapshot.Queue.CurrentIndex
		if index < 0 || index >= len(snapshot.Queue.Items) {
			index = 0
		}
		track := snapshot.Queue.Items[index]
		return lithic.Slab(gtx, e.Th.R.Stone1, e.Th.Cut.Mid, func(gtx layout.Context) layout.Dimensions {
			copy := layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				children := []layout.FlexChild{
					layout.Rigid(lithic.Text(e.Th, e.Th.T.Title, e.Th.R.Fg, track.Title)),
					layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
					layout.Rigid(lithic.Text(e.Th, e.Th.T.Body, e.Th.R.Fg2, track.Artists)),
				}
				if snapshot.Playback.Duration > 0 {
					proof := widgets.Duration(e.Store.PositionAt(e.Now)) + " of " + widgets.Duration(snapshot.Playback.Duration)
					children = append(children,
						layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
						layout.Rigid(lithic.Text(e.Th, e.Th.T.MonoSm, e.Th.R.Fg3, proof)),
					)
				}
				return layout.Flex{Axis: layout.Vertical, Alignment: layout.Start}.Layout(gtx, children...)
			})
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Gap: gtx.Dp(13)}.Layout(gtx,
				layout.Rigid(widgets.ProviderBadge(e, model.ProviderOf(track.VideoID))),
				copy,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return p.resume.Layout(gtx, e.Th, lithic.Gold, lithic.Icon("play"), "Resume queue")
				}),
			)
		})
	}
}

func recentItems(personal state.Personal) []model.BrowseItem {
	picked := make(map[string]struct{}, len(personal.Picks))
	for _, pick := range personal.Picks {
		picked[pick.ID] = struct{}{}
	}
	items := make([]model.BrowseItem, 0, 12)
	for _, recent := range state.RecentItems(personal, 0) {
		item := model.BrowseItem{
			Kind:       recent.Kind,
			ID:         recent.ID,
			Title:      recent.Title,
			Subtitle:   recent.Subtitle,
			Thumbnail:  recent.Thumbnail,
			Duration:   recent.Duration,
			ArtistRuns: recent.ArtistRuns,
			PlayCount:  recent.PlayCount,
			IsVideo:    recent.IsVideo,
			IsUpload:   recent.IsUpload,
			Explicit:   recent.Explicit,
		}
		if _, exists := picked[item.ID]; exists {
			continue
		}
		switch item.Kind {
		case "song", "album", "playlist":
			items = append(items, item)
		}
		if len(items) == 12 {
			break
		}
	}
	return items
}

func (p *page) pickedLayout(e *env.Env, personal state.Personal) layout.Widget {
	items := state.PickedForYou(p.feed, personal)
	provider := providerLabel(p.loadedProvider)
	var sentence string
	switch {
	case len(items) == 0:
		sentence = "Nothing new to surface here yet. This fills in as your " + provider + " feed grows."
	case hasListeningHistory(personal):
		sentence = "Based on your recent listening and shortcuts, with room for different artists. Ranked on your device."
	default:
		sentence = "A varied selection from " + provider + ". Your listening will help shape future picks."
	}
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.Start}.Layout(gtx,
			layout.Rigid(lithic.Text(e.Th, e.Th.T.Body, e.Th.R.Fg2, sentence)),
			layout.Rigid(layout.Spacer{Height: unit.Dp(13)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if len(items) == 0 {
					return layout.Dimensions{}
				}
				return p.pickedGrid.Layout(gtx, e, items)
			}),
		)
	}
}

func (p *page) feedChips(e *env.Env) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		labels := make([]string, len(p.feed.Chips))
		selected := 0
		for i, chip := range p.feed.Chips {
			labels[i] = chip.Title
			if chip.Params == p.feedParams {
				selected = i
			}
		}
		return p.feedFilters.Layout(gtx, e.Th, labels, nil, selected)
	}
}

func providerLabel(provider model.Provider) string {
	switch provider {
	case model.ProviderYouTube:
		return "YouTube Music"
	case model.ProviderSpotify:
		return "Spotify"
	case model.ProviderSoundCloud:
		return "SoundCloud"
	case model.ProviderTidal:
		return "Tidal"
	case model.ProviderLocal:
		return "your local library"
	default:
		return "your feed"
	}
}

func hasListeningHistory(personal state.Personal) bool {
	return len(personal.Recent) != 0 || len(personal.Artists) != 0
}
