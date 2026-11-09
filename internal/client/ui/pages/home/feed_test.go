package home

import (
	"errors"
	"testing"

	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/model"
)

func TestSetChipsAddsAllAndRemovesPodcasts(t *testing.T) {
	p := new(page)
	p.setChips([]model.FeedChip{
		{Title: "Podcasts", Params: "podcasts"},
		{Title: " Workout ", Params: "workout"},
		{Title: "All"},
		{Title: "Commute", Params: "commute"},
	})

	want := []model.FeedChip{
		{Title: "All"},
		{Title: "Workout", Params: "workout"},
		{Title: "Commute", Params: "commute"},
	}
	if len(p.chips) != len(want) {
		t.Fatalf("chips = %#v, want %#v", p.chips, want)
	}
	for index := range want {
		if p.chips[index] != want[index] {
			t.Fatalf("chips[%d] = %#v, want %#v", index, p.chips[index], want[index])
		}
		if p.chipLabels[index] != want[index].Title {
			t.Fatalf("label[%d] = %q, want %q", index, p.chipLabels[index], want[index].Title)
		}
	}
}

func TestSelectAllRestoresCachedFeedWithoutRequest(t *testing.T) {
	all := model.Feed{Sections: []model.Shelf{{Title: "All shelf"}}}
	filtered := model.Feed{Sections: []model.Shelf{{Title: "Workout shelf"}}}
	p := &page{
		allLoaded:   true,
		allFeed:     all,
		feed:        filtered,
		activeChip:  1,
		pendingChip: -1,
		chips: []model.FeedChip{
			{Title: "All"},
			{Title: "Workout", Params: "workout"},
		},
		request: 4,
	}

	p.selectChip(nil, 0)

	if p.request != 5 {
		t.Fatalf("request generation = %d, want 5", p.request)
	}
	if p.activeChip != 0 || p.feedFilters.Selected != 0 {
		t.Fatalf("selection = active %d filter %d, want All", p.activeChip, p.feedFilters.Selected)
	}
	if len(p.feed.Sections) != 1 || p.feed.Sections[0].Title != "All shelf" {
		t.Fatalf("feed = %#v, want cached All feed", p.feed)
	}
	if p.loading || !p.loaded {
		t.Fatalf("loading = %t, loaded = %t", p.loading, p.loaded)
	}
}

func TestFinishChipDropsStaleReply(t *testing.T) {
	original := model.Feed{Sections: []model.Shelf{{Title: "All shelf"}}}
	p := &page{request: 2, loading: true, pendingChip: 1, feed: original}

	p.finishChip(nil, 1, 0, 1, "Workout", model.Feed{Sections: []model.Shelf{{Title: "Stale"}}}, nil)

	if !p.loading || p.pendingChip != 1 || p.feed.Sections[0].Title != "All shelf" {
		t.Fatalf("stale reply changed state: %#v", p)
	}
}

func TestFinishChipFailureRevertsSelectionAndKeepsContent(t *testing.T) {
	original := model.Feed{Sections: []model.Shelf{{Title: "All shelf"}}}
	var message string
	e := &env.Env{Toast: func(value string, kind env.ToastKind) {
		message = value
		if kind != env.ToastError {
			t.Fatalf("toast kind = %q, want error", kind)
		}
	}}
	p := &page{request: 2, loading: true, pendingChip: 1, feed: original}
	p.feedFilters.Selected = 1

	p.finishChip(e, 2, 0, 1, "Workout", model.Feed{}, errors.New("offline"))

	if p.loading || p.pendingChip != -1 || p.feedFilters.Selected != 0 {
		t.Fatalf("failed reply state: loading %t, pending %d, selected %d", p.loading, p.pendingChip, p.feedFilters.Selected)
	}
	if len(p.feed.Sections) != 1 || p.feed.Sections[0].Title != "All shelf" {
		t.Fatalf("feed = %#v, want retained content", p.feed)
	}
	if message != "Could not load Workout" {
		t.Fatalf("toast = %q, want %q", message, "Could not load Workout")
	}
}
