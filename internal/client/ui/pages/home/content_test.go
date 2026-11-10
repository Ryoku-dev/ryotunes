package home

import (
	"testing"
	"time"

	"github.com/ryoku-dev/ryotunes/internal/client/state"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/model"
)

// A chip answer with exactly one shelf has as many rows as an empty chip
// answer; only the empty one is a page state that drops the content inset.
func TestChipWithOneShelfIsContentNotState(t *testing.T) {
	e := &env.Env{Now: time.Unix(0, 0)}
	p := New(env.Route{}).(*page)
	p.loaded = true
	p.setChips([]model.FeedChip{{Title: "Focus", Params: "focus"}})
	p.activeChip = 1
	p.feed = model.Feed{Sections: []model.Shelf{{Title: "Focus rotation", Items: []model.BrowseItem{{ID: "a", Title: "A"}}}}}
	p.feedRevision++

	children := p.content(e, state.State{})
	if len(children) != 3 || p.stateIndex != -1 {
		t.Fatalf("one shelf: %d rows, state index %d, want 3 rows and no state", len(children), p.stateIndex)
	}

	p.feed = model.Feed{}
	p.feedRevision++
	p.contentRevision++
	children = p.content(e, state.State{})
	if len(children) != 3 || p.stateIndex != 2 {
		t.Fatalf("empty chip: %d rows, state index %d, want the state at 2", len(children), p.stateIndex)
	}
}
