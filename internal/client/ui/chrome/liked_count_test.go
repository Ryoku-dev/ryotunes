package chrome

import (
	"image"
	"testing"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
	libtheme "github.com/ryoku-dev/ryoku/docs/lithic/library/go/theme"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/model"
)

// On YouTube the Liked songs leaf counts YouTube's liked list, never the device's liked
// list that the library also carries.
func TestLikedLeafCountsTheProvidersList(t *testing.T) {
	for _, test := range []struct {
		id, liked string
		want      bool
	}{
		{"VLLM", "VLLM", true},
		{"LM", "VLLM", true},
		{model.LocalLikedPlaylistID, "VLLM", false},
		{"RYOTUNES_LIKED", "VLLM", false},
		{model.LocalLikedPlaylistID, model.LocalLikedPlaylistID, true},
		{"spotify:playlist:liked", "spotify:playlist:liked", true},
	} {
		if got := sameLikedCollection(test.id, test.liked); got != test.want {
			t.Errorf("sameLikedCollection(%q, %q) = %v, want %v", test.id, test.liked, got, test.want)
		}
	}
}

// A liked count the provider has not given shows no count at all, not "No songs yet".
func TestLikedLeafWithAnUnknownCountShowsNone(t *testing.T) {
	th := look.NewTheme(palette.Signature(true), libtheme.Comfortable)
	e := &env.Env{Th: th}
	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Constraints{Max: image.Pt(200, 40)}}
	item := sidebarCollection{liked: true}
	if size := sidebarCaption(e, item, likedCountUnknown, false)(gtx).Size; size != (image.Point{}) {
		t.Fatalf("unknown count caption takes %v", size)
	}
	if size := sidebarCaption(e, item, 0, false)(gtx).Size; size.Y == 0 {
		t.Fatal("a known empty list says nothing")
	}
}
