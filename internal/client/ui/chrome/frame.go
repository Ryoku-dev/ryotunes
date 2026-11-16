package chrome

import (
	"image"

	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/widgets"
	"github.com/ryoku-dev/ryotunes/internal/model"
)

// Frame describes the shell geometry the top bar needs in window pixels.
type Frame struct {
	Class           look.Class
	Sidebar         int
	SidebarProgress float32
	SidebarSheet    bool
	Content         image.Rectangle
	Decorated       bool
	Maximized       bool
}

// ToggleLikePlaying rates the playing track as its heart does (widgets.ToggleLike): a like,
// or an unlike whose notice offers Undo. The shell's Ctrl L goes through it, so the key and
// every heart behave alike.
func ToggleLikePlaying(e *env.Env) {
	now := e.Store.Snapshot().Playback.Now
	if now == nil {
		return
	}
	widgets.ToggleLike(e, trackFromPlayback(now, e.Store.CurrentTrack()), now.Rating != nil && *now.Rating == model.RatingLike)
}
