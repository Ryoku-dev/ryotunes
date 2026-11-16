package radio

import (
	"testing"

	"github.com/ryoku-dev/ryotunes/internal/client/state"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/model"
)

func savedIDs(store *state.Store) []string {
	var ids []string
	for _, item := range store.Snapshot().Personal.Saved {
		ids = append(ids, item.ID)
	}
	return ids
}

// Unfavouriting a station offers Undo, and the Undo puts it back where it stood, with no
// notice of its own.
func TestUnfavouriteUndoRestoresThePlace(t *testing.T) {
	store := state.NewEmpty()
	station := model.RadioStation{StationUUID: "s-2", Name: "Signal 02"}
	store.UpdatePersonal(func(personal *state.Personal) bool {
		personal.Saved = append(personal.Saved,
			model.BrowseItem{Kind: "playlist", ID: "a"}, stationBrowse(station), model.BrowseItem{Kind: "album", ID: "b"})
		return true
	})
	var notices []string
	var undo func()
	e := &env.Env{
		Store:    store,
		Toast:    func(message string, _ env.ToastKind) { notices = append(notices, message) },
		Undoable: func(message string, run func()) { notices, undo = append(notices, message), run },
	}
	p := New(env.Route{}).(*Page)

	p.toggleFavourite(e, station)
	if got := savedIDs(store); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("after unfavourite saved = %v", got)
	}
	if undo == nil || len(notices) != 1 || notices[0] != "Removed Signal 02 from your favourites." {
		t.Fatalf("notices = %q, undo offered %v", notices, undo != nil)
	}
	undo()
	if got := savedIDs(store); len(got) != 3 || got[1] != stationPersonalID("s-2") {
		t.Fatalf("after Undo saved = %v, want the station back in the middle", got)
	}
	undo()
	if got := savedIDs(store); len(got) != 3 {
		t.Fatalf("a second Undo duplicated the station: %v", got)
	}
	if len(notices) != 1 {
		t.Fatalf("the Undo raised notices %q", notices[1:])
	}
}
