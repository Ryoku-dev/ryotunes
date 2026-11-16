package chrome

import (
	"context"
	"encoding/json"
	"image"
	"net"
	"path/filepath"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/pointer"
	"gioui.org/layout"

	"github.com/ryoku-dev/ryotunes/internal/client/conn"
	"github.com/ryoku-dev/ryotunes/internal/client/state"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

// TestLikingThePlayingSongFromTheBarStaysLiked presses the bar's heart with a
// real pointer on a song YouTube sent without a rating. The heart fills once
// the daemon confirms, before any rating event, stays filled at rest and
// through a now playing announcement that carries no rating, and follows the
// daemon's rating events from elsewhere both ways.
func TestLikingThePlayingSongFromTheBarStaysLiked(t *testing.T) {
	socket := filepath.Join(t.TempDir(), "ryotunesd.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	requests := make(chan protocol.Request, 16)
	go serveUndoTest(listener, requests)
	t.Cleanup(func() { _ = listener.Close() })
	client := conn.NewPath(socket)
	t.Cleanup(client.Close)
	waitForUndoRequest(t, requests, protocol.MethodSubscribe)

	h := newPlayerHarness(t)
	const videoID = "YgKDE5eYNqc"
	now := &model.PlaybackTrack{VideoID: videoID, Title: "Goodbyes", Artists: "Post Malone, Young Thug"}
	store := state.New(protocol.Snapshot{
		Provider: model.ProviderYouTube,
		Playback: model.PlaybackSnapshot{Now: now, Paused: true},
		Queue:    model.QueueSnapshot{Items: []model.Track{{VideoID: videoID, Title: now.Title, Artists: now.Artists}}},
	})
	e := &env.Env{Th: h.theme, Overlays: new(look.Overlays), Store: store, Nav: &topBarTestNav{current: env.Route{Kind: env.RouteHome}}, Conn: client}
	e.Go = func(run func(context.Context)) { go run(context.Background()) }
	e.Undoable = func(string, func()) {}

	var bar PlayerBar
	draw := func(gtx layout.Context) {
		gtx.Constraints = layout.Exact(image.Pt(1280, 88))
		act.PendingClipboard(gtx)
		bar.Layout(gtx, e)
	}
	liked := func() bool { return bar.like.plaque.Description == "Liked" && bar.like.heart.liked }
	// rest moves the pointer off the bar and runs frames until nothing moves
	rest := func() {
		h.router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, PointerID: 1, Position: f32.Pt(640, 400)})
		for range 40 {
			h.advance(100 * time.Millisecond)
			h.frame(draw)
			if !h.wants() {
				return
			}
		}
		t.Fatal("the bar kept asking for frames")
	}
	apply := func(name string, payload any) {
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.Apply(name, raw); err != nil {
			t.Fatal(err)
		}
		rest()
	}
	h.frame(draw)
	rest()
	if liked() {
		t.Fatal("a song YouTube sent without a rating reads liked")
	}

	heart := f32.Pt(21+358-32-4-16, 44)
	h.router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, PointerID: 1, Position: heart})
	h.frame(draw)
	h.router.Queue(
		pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Buttons: pointer.ButtonPrimary, Position: heart},
		pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: heart},
	)
	h.frame(draw)
	if rating := rateParams(t, waitForUndoRequest(t, requests, protocol.MethodRate)); rating.VideoID != videoID || rating.Rating != model.RatingLike {
		t.Fatalf("the heart asked for %+v, want a like of %s", rating, videoID)
	}
	// the fake daemon confirms and sends no rating event
	for deadline := time.Now().Add(3 * time.Second); !liked(); time.Sleep(time.Millisecond) {
		h.frame(draw)
		if time.Now().After(deadline) {
			t.Fatal("the daemon confirmed the like but the bar's heart stayed empty")
		}
	}
	rest()
	if !liked() {
		t.Fatal("the bar's heart emptied at rest after the like")
	}

	// the daemon re-announces the song as it had it before the like
	apply(protocol.EventNowPlaying, now)
	if !liked() {
		t.Fatal("a now playing announcement without a rating emptied the liked heart")
	}
	apply(protocol.EventRating, protocol.RatingEvent{VideoID: videoID, Rating: model.RatingLike})
	if !liked() {
		t.Fatal("the daemon's rating event for the like emptied the heart")
	}

	// unliked and liked again elsewhere (another client, the item menu)
	apply(protocol.EventRating, protocol.RatingEvent{VideoID: videoID, Rating: model.RatingIndifferent})
	if liked() {
		t.Fatal("an unlike from elsewhere left the bar's heart filled")
	}
	if rating := store.Snapshot().Queue.Items[0].Rating; rating == nil || *rating != model.RatingIndifferent {
		t.Fatalf("the queue row after the unlike = %v", rating)
	}
	apply(protocol.EventRating, protocol.RatingEvent{VideoID: videoID, Rating: model.RatingLike})
	if !liked() {
		t.Fatal("a like from elsewhere left the bar's heart empty")
	}
}
