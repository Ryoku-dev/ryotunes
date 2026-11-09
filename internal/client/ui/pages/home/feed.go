package home

import (
	"strings"

	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

func (p *page) fetchHome(e *env.Env) {
	p.started = true
	p.loading = true
	p.errorText = ""
	p.moreRequest++
	p.loadingMore = false
	p.moreFailed = ""
	p.request++
	request := p.request
	params := optionalString(p.feedParams)
	snapshot := e.Store.Snapshot()
	provider := snapshot.Provider
	if provider == model.ProviderTidal && !snapshot.Tidal.SignedIn {
		p.loading = false
		p.loaded = true
		p.feed = model.Feed{}
		return
	}
	act.Call[model.Feed](e, protocol.MethodGetHome, protocol.HomeParams{Params: params}, func(feed model.Feed, err error) {
		if request != p.request {
			return
		}
		p.loading = false
		if err != nil {
			p.errorText = err.Error()
			return
		}
		p.feed = feed
		p.loaded = true
		p.errorText = ""
	})
}

func (p *page) loadMore(e *env.Env) {
	if !p.loaded || p.loadingMore || p.feed.Continuation == nil || strings.TrimSpace(*p.feed.Continuation) == "" {
		return
	}
	token := *p.feed.Continuation
	if token == p.moreFailed {
		return
	}
	p.loadingMore = true
	p.moreRequest++
	request := p.moreRequest
	act.Call[model.Feed](e, protocol.MethodGetHomeMore, protocol.TokenParams{Token: token}, func(more model.Feed, err error) {
		if request != p.moreRequest {
			return
		}
		p.loadingMore = false
		if err != nil {
			p.moreFailed = token
			e.Toast("Could not load more", env.ToastError)
			return
		}
		p.moreFailed = ""
		p.feed.Sections = append(p.feed.Sections, more.Sections...)
		if len(more.Chips) != 0 {
			p.feed.Chips = more.Chips
		}
		p.feed.Continuation = more.Continuation
	})
}

func optionalString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}
