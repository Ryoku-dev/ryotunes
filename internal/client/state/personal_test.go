package state

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/ryoku-dev/ryotunes/internal/model"
)

func TestPersonalPickDedupeAndLimits(t *testing.T) {
	personal := EmptyPersonal()
	for i := range maxPicks {
		id := fmt.Sprintf("p%02d", i)
		if !AddPick(&personal, Pick{BrowseItem: model.BrowseItem{ID: id, Kind: "playlist", Title: id}}, int64(i+1)) {
			t.Fatalf("new pick %q was rejected", id)
		}
	}
	if AddPick(&personal, Pick{BrowseItem: model.BrowseItem{ID: "p05", Kind: "playlist", Title: "duplicate"}}, 100) {
		t.Fatal("duplicate pick reported as new")
	}
	if len(personal.Picks) != maxPicks || personal.Picks[5].LastUsedAt != 100 {
		t.Fatalf("duplicate did not refresh in place: %#v", personal.Picks[5])
	}
	AddPick(&personal, Pick{BrowseItem: model.BrowseItem{ID: "new", Kind: "playlist", Title: "new"}}, 101)
	if len(personal.Picks) != maxPicks {
		t.Fatalf("pick limit = %d, want %d", len(personal.Picks), maxPicks)
	}
	for _, pick := range personal.Picks {
		if pick.ID == "p00" {
			t.Fatal("stalest pick was not evicted")
		}
	}

	for i := range maxPins {
		if got := TogglePin(&personal, fmt.Sprintf("pin%d", i)); got != "pinned" {
			t.Fatalf("pin result = %q", got)
		}
	}
	if got := TogglePin(&personal, "overflow"); got != "full" {
		t.Fatalf("overflow pin result = %q", got)
	}
}

func TestPersonalRecentDedupeAndLimit(t *testing.T) {
	personal := EmptyPersonal()
	for i := range maxRecent + 5 {
		id := fmt.Sprintf("v%d", i)
		NoteRecent(&personal, RecentItem{BrowseItem: model.BrowseItem{ID: id, Kind: "song", Title: id}}, int64(i+1))
	}
	if len(personal.Recent) != maxRecent {
		t.Fatalf("recent limit = %d, want %d", len(personal.Recent), maxRecent)
	}
	if _, exists := personal.Recent["v4"]; exists {
		t.Fatal("oldest recent survived eviction")
	}
	NoteRecent(&personal, RecentItem{BrowseItem: model.BrowseItem{ID: "v50", Kind: "song", Title: "changed"}}, 999)
	if len(personal.Recent) != maxRecent || personal.Recent["v50"].At != 999 {
		t.Fatal("replayed recent duplicated instead of refreshing")
	}
}

func TestHydrateFiltersInvalidMembers(t *testing.T) {
	personal := HydratePersonal(json.RawMessage(`{
		"picks":[{"kind":"playlist","id":"kept","title":"Kept"},{"id":"old","manual":false},{}],
		"pins":["a",7,"b","c","d"],
		"dismissedSeeds":["x",9],
		"home":{"order":[1,"@recent","A"]}
	}`))
	if len(personal.Picks) != 1 || personal.Picks[0].ID != "kept" {
		t.Fatalf("filtered picks = %#v", personal.Picks)
	}
	if len(personal.Pins) != 3 || len(personal.DismissedSeeds) != 1 {
		t.Fatalf("filtered arrays = pins %#v dismissed %#v", personal.Pins, personal.DismissedSeeds)
	}
	if got := fmt.Sprint(personal.Home.Order); got != "[@recent @familiar A]" {
		t.Fatalf("home order = %s", got)
	}
}
