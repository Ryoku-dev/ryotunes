package playlist

import (
	"testing"

	"github.com/ryoku-dev/ryotunes/internal/client/state"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/model"
)

// Removing a playlist from the library offers Undo, which puts it back where it stood,
// silently, as the item menu's Undo does.
func TestRemoveFromLibraryUndoRestoresThePlace(t *testing.T) {
	store := state.NewEmpty()
	store.UpdatePersonal(func(personal *state.Personal) bool {
		personal.Saved = append(personal.Saved,
			model.BrowseItem{Kind: "album", ID: "a"}, model.BrowseItem{Kind: "playlist", ID: "PLmine", Title: "Night signals"},
			model.BrowseItem{Kind: "album", ID: "b"})
		return true
	})
	var notices []string
	var undo func()
	e := &env.Env{
		Store:    store,
		Toast:    func(message string, _ env.ToastKind) { notices = append(notices, message) },
		Undoable: func(message string, run func()) { notices, undo = append(notices, message), run },
	}
	p := New(env.Route{Kind: env.RoutePlaylist, ID: "PLmine", Title: "Night signals"}).(*Page)

	p.setSaved(e, false)
	saved := store.Snapshot().Personal.Saved
	if len(saved) != 2 || undo == nil || len(notices) != 1 || notices[0] != "Removed Night signals from your library." {
		t.Fatalf("saved %v, notices %q, undo %v", saved, notices, undo != nil)
	}
	undo()
	saved = store.Snapshot().Personal.Saved
	if len(saved) != 3 || saved[1].ID != "PLmine" || saved[1].Title != "Night signals" {
		t.Fatalf("after Undo saved = %v, want the playlist back in the middle", saved)
	}
	if len(notices) != 1 {
		t.Fatalf("the Undo raised notices %q", notices[1:])
	}
}
