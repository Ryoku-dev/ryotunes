package search

import (
	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

func (p *page) loadSelected(e *env.Env) {
	if p.query == "" {
		return
	}
	switch p.selected {
	case 0:
		p.loadMixed(e)
	case 1:
		p.loadTracks(e)
	default:
		p.loadCards(e, p.selected-2)
	}
}

func (p *page) loadMixed(e *env.Env) {
	if p.mixed.loading || p.mixed.loaded {
		return
	}
	p.mixed.loading = true
	p.mixed.error = ""
	p.mixed.request++
	request := p.mixed.request
	act.Call[model.SearchAll](e, protocol.MethodSearchAll, protocol.QueryParams{Query: p.query}, func(result model.SearchAll, err error) {
		if request != p.mixed.request {
			return
		}
		p.mixed.loading = false
		if err != nil {
			p.mixed.error = err.Error()
			return
		}
		p.mixed.result = result
		p.mixed.loaded = true
	})
}

func (p *page) loadTracks(e *env.Env) {
	if p.tracks.loading || p.tracks.loaded {
		return
	}
	p.tracks.loading = true
	p.tracks.error = ""
	p.tracks.request++
	request := p.tracks.request
	act.Call[model.SearchPage](e, protocol.MethodSearchPage, protocol.QueryParams{Query: p.query}, func(result model.SearchPage, err error) {
		if request != p.tracks.request {
			return
		}
		p.tracks.loading = false
		if err != nil {
			p.tracks.error = err.Error()
			return
		}
		p.tracks.items = dedupeTracks(nil, result.Items)
		p.tracks.continuation = result.Continuation
		p.tracks.loaded = true
	})
}

func (p *page) loadTracksMore(e *env.Env) {
	if !p.tracks.loaded || p.tracks.paging || p.tracks.continuation == nil || *p.tracks.continuation == "" {
		return
	}
	p.tracks.paging = true
	token := *p.tracks.continuation
	p.tracks.request++
	request := p.tracks.request
	act.Call[model.SearchPage](e, protocol.MethodSearchPageMore, protocol.TokenParams{Token: token}, func(result model.SearchPage, err error) {
		if request != p.tracks.request {
			return
		}
		p.tracks.paging = false
		if err != nil {
			e.Toast("Could not load more", env.ToastError)
			return
		}
		p.tracks.items = dedupeTracks(p.tracks.items, result.Items)
		p.tracks.continuation = result.Continuation
	})
}

func (p *page) loadCards(e *env.Env, index int) {
	if index < 0 || index >= len(p.cards) {
		return
	}
	state := &p.cards[index]
	if state.loading || state.loaded {
		return
	}
	state.loading = true
	state.error = ""
	state.request++
	request := state.request
	kind := searchTabs[index+2].kind
	act.Call[model.CardsPage](e, protocol.MethodSearchCardsPage, protocol.SearchCardsParams{Query: p.query, Category: kind}, func(result model.CardsPage, err error) {
		if request != state.request {
			return
		}
		state.loading = false
		if err != nil {
			state.error = err.Error()
			return
		}
		state.items = dedupeCards(nil, result.Items)
		state.continuation = result.Continuation
		state.loaded = true
	})
}

func (p *page) loadCardsMore(e *env.Env, index int) {
	if index < 0 || index >= len(p.cards) {
		return
	}
	state := &p.cards[index]
	if !state.loaded || state.paging || state.continuation == nil || *state.continuation == "" {
		return
	}
	state.paging = true
	token := *state.continuation
	state.request++
	request := state.request
	act.Call[model.CardsPage](e, protocol.MethodSearchCardsMore, protocol.TokenParams{Token: token}, func(result model.CardsPage, err error) {
		if request != state.request {
			return
		}
		state.paging = false
		if err != nil {
			e.Toast("Could not load more", env.ToastError)
			return
		}
		state.items = dedupeCards(state.items, result.Items)
		state.continuation = result.Continuation
	})
}

func dedupeTracks(existing, incoming []model.Track) []model.Track {
	seen := make(map[string]struct{}, len(existing)+len(incoming))
	out := make([]model.Track, 0, len(existing)+len(incoming))
	for _, items := range [][]model.Track{existing, incoming} {
		for _, item := range items {
			if item.VideoID == "" {
				continue
			}
			if _, duplicate := seen[item.VideoID]; duplicate {
				continue
			}
			seen[item.VideoID] = struct{}{}
			out = append(out, item)
		}
	}
	return out
}

func dedupeCards(existing, incoming []model.BrowseItem) []model.BrowseItem {
	seen := make(map[string]struct{}, len(existing)+len(incoming))
	out := make([]model.BrowseItem, 0, len(existing)+len(incoming))
	for _, items := range [][]model.BrowseItem{existing, incoming} {
		for _, item := range items {
			if item.ID == "" {
				continue
			}
			if _, duplicate := seen[item.ID]; duplicate {
				continue
			}
			seen[item.ID] = struct{}{}
			out = append(out, item)
		}
	}
	return out
}
