package pagesheet

import "github.com/ryoku-dev/ryotunes/internal/client/ui/env"

// OfferUndo shows message as the frame's notice with an Undo that runs undo (the Hub's
// notices), or as a plain notice where the window offers no Undo (headless renders, tests).
// The undo itself raises no notice of its own, as the item menu's Undo does not.
func OfferUndo(e *env.Env, message string, undo func()) {
	switch {
	case e == nil:
	case e.Undoable != nil:
		e.Undoable(message, undo)
	case e.Toast != nil:
		e.Toast(message, env.ToastSuccess)
	}
}
