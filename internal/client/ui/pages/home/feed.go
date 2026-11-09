package home

import (
	"strings"

	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

func (p *page) fetchAll(e *env.Env) {
	p.started = true
	p.loading = true
	p.errorText = ""
	p.pendingChip = 0
	p.feedFilters.Selected = 0
	p.cancelMore()
	p.request++
	request := p.request
	snapshot := e.Store.Snapshot()
	if snapshot.Provider == model.ProviderTidal && !snapshot.Tidal.SignedIn {
		p.loading = false
		p.loaded = true
		p.pendingChip = -1
		p.feed = model.Feed{}
		p.allFeed = model.Feed{}
		p.allLoaded = true
		p.setChips(nil)
		p.markFeedChanged()
		return
	}
	p.markContentChanged()
	act.Call[model.Feed](e, protocol.MethodGetHome, protocol.HomeParams{}, func(feed model.Feed, err error) {
		if request != p.request {
			return
		}
		p.loading = false
		p.pendingChip = -1
		if err != nil {
			if p.loaded {
				if e.Toast != nil {
					e.Toast("Could not load Home", env.ToastError)
				}
			} else {
				p.errorText = err.Error()
			}
			p.markContentChanged()
			return
		}
		p.errorText = ""
		p.loaded = true
		p.allLoaded = true
		p.activeChip = 0
		p.feed = feed
		p.allFeed = feed
		p.setChips(feed.Chips)
		p.markFeedChanged()
		if p.initialParams != "" {
			params := p.initialParams
			p.initialParams = ""
			for index := 1; index < len(p.chips); index++ {
				if p.chips[index].Params == params {
					p.selectChip(e, index)
					break
				}
			}
		}
	})
}

func (p *page) fetchChip(e *env.Env, index int) {
	if index <= 0 || index >= len(p.chips) {
		return
	}
	previous := p.activeChip
	chip := p.chips[index]
	p.loading = true
	p.errorText = ""
	p.pendingChip = index
	p.feedFilters.Selected = index
	p.cancelMore()
	p.request++
	request := p.request
	p.markContentChanged()
	act.Call[model.Feed](e, protocol.MethodGetHome, protocol.HomeParams{Params: optionalString(chip.Params)}, func(feed model.Feed, err error) {
		p.finishChip(e, request, previous, index, chip.Title, feed, err)
	})
}

func (p *page) finishChip(e *env.Env, request uint64, previous, selected int, title string, feed model.Feed, err error) {
	if request != p.request {
		return
	}
	p.loading = false
	p.pendingChip = -1
	if err != nil {
		p.feedFilters.Selected = previous
		if e != nil && e.Toast != nil {
			e.Toast("Could not load "+title, env.ToastError)
		}
		p.markContentChanged()
		return
	}
	p.activeChip = selected
	p.feedFilters.Selected = selected
	p.feed = feed
	p.loaded = true
	p.markFeedChanged()
}

func (p *page) selectChip(e *env.Env, index int) {
	if index < 0 || index >= len(p.chips) {
		return
	}
	if index == 0 && p.allLoaded {
		p.request++
		p.cancelMore()
		p.loading = false
		p.errorText = ""
		p.pendingChip = -1
		p.activeChip = 0
		p.feedFilters.Selected = 0
		p.feed = p.allFeed
		p.loaded = true
		p.markFeedChanged()
		return
	}
	if index == p.activeChip && p.pendingChip < 0 {
		p.feedFilters.Selected = p.activeChip
		return
	}
	p.fetchChip(e, index)
}

func (p *page) setChips(provider []model.FeedChip) {
	chips := make([]model.FeedChip, 1, len(provider)+1)
	chips[0] = model.FeedChip{Title: "All"}
	for _, chip := range provider {
		title := strings.TrimSpace(chip.Title)
		if title == "" || strings.EqualFold(title, "All") || strings.EqualFold(title, "Podcasts") {
			continue
		}
		chip.Title = title
		chips = append(chips, chip)
	}
	labels := make([]string, len(chips))
	for index := range chips {
		labels[index] = chips[index].Title
	}
	p.chips = chips
	p.chipLabels = labels
	p.feedFilters.Labels = labels
	p.feedFilters.Selected = p.activeChip
}

func (p *page) cancelMore() {
	p.moreRequest++
	p.loadingMore = false
	p.moreFailed = ""
}

func (p *page) markContentChanged() {
	p.contentRevision++
}

func (p *page) markFeedChanged() {
	p.feedRevision++
	p.markContentChanged()
}

func (p *page) loadMore(e *env.Env) {
	if p.loading || !p.loaded || p.loadingMore || p.feed.Continuation == nil || strings.TrimSpace(*p.feed.Continuation) == "" {
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
			if e.Toast != nil {
				e.Toast("Could not load more", env.ToastError)
			}
			return
		}
		p.moreFailed = ""
		p.feed.Sections = append(p.feed.Sections, more.Sections...)
		p.feed.Continuation = more.Continuation
		if p.activeChip == 0 {
			p.allFeed = p.feed
		}
		p.markFeedChanged()
	})
}

func optionalString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}
