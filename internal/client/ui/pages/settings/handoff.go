package settings

import (
	"context"

	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/spawn"
)

// stylePage is the Hub page that owns the desktop's look: design, colours,
// light or dark, contrast and tint (Ryoku Settings › Appearance › Style).
const stylePage = "style"

// openHub opens Ryoku Settings on a page as every Ryoku app does, through
// the shell daemon's `ryoku-shell hub open <page>[/<row>]`: it raises the one
// Settings window where it is, or starts it on that page (ryoku/shell
// ipc/control.go). The launch runs in its own scope (spawn.Command), so
// Settings outlives Ryotunes. Tests replace it.
var openHub = func(ctx context.Context, page string) error {
	return spawn.Command(ctx, "ryoku-shell", "hub", "open", page).Run()
}

// handOff opens Ryoku Settings on page off the frame goroutine and says so
// when it cannot (a desktop without Ryoku's shell).
func handOff(e *env.Env, page string) {
	if e == nil || e.Go == nil {
		return
	}
	e.Go(func(ctx context.Context) {
		if err := openHub(ctx, page); err != nil && e.Toast != nil {
			e.Toast("Could not open Ryoku Settings", env.ToastError)
		}
	})
}
