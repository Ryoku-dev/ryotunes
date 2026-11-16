package chrome

import (
	"bufio"
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

// TestUnlikeFromTheBarOffersAnUndoThatLikesAgain clicks the bar's heart on a
// liked track with a real pointer against a fake daemon: the daemon is asked
// to unlike it, a placard offers Undo with the Hub's copy once the daemon has
// done it, and the undo asks the daemon to like the same track again, which
// raises no placard of its own.
func TestUnlikeFromTheBarOffersAnUndoThatLikesAgain(t *testing.T) {
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
	liked := model.RatingLike
	const videoID = "dQw4w9WgXcQ"
	store := state.New(protocol.Snapshot{
		Provider: model.ProviderYouTube,
		Playback: model.PlaybackSnapshot{Now: &model.PlaybackTrack{VideoID: videoID, Title: "Stone", Artists: "Kite", Rating: &liked}},
	})
	nav := &topBarTestNav{current: env.Route{Kind: env.RouteHome}}
	e := &env.Env{Th: h.theme, Overlays: new(look.Overlays), Store: store, Nav: nav, Conn: client}
	e.Go = func(run func(context.Context)) { go run(context.Background()) }
	var notices []string
	var undo func()
	e.Undoable = func(message string, run func()) { notices, undo = append(notices, message), run }

	var bar PlayerBar
	draw := func(gtx layout.Context) {
		gtx.Constraints = layout.Exact(image.Pt(1280, 88))
		// the shell delivers daemon replies at the start of each frame
		act.PendingClipboard(gtx)
		bar.Layout(gtx, e)
	}
	// settle runs frames until want notices have come or wait has passed,
	// delivering the daemon's replies
	settle := func(want int, wait time.Duration) {
		deadline := time.Now().Add(wait)
		for len(notices) < want && time.Now().Before(deadline) {
			h.frame(draw)
			time.Sleep(time.Millisecond)
		}
	}
	h.frame(draw)

	// the heart: 21 in, a 358 track zone ending in like (32) then 4 and the
	// download (32), both small plaques centred in the 88 tall bar
	heart := f32.Pt(21+358-32-4-16, 44)
	h.router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, PointerID: 1, Position: heart})
	h.frame(draw)
	h.router.Queue(
		pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Buttons: pointer.ButtonPrimary, Position: heart},
		pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: heart},
	)
	h.frame(draw)
	h.frame(draw)

	if rating := rateParams(t, waitForUndoRequest(t, requests, protocol.MethodRate)); rating.VideoID != videoID || rating.Rating != model.RatingIndifferent {
		t.Fatalf("the heart asked for %+v, want an unlike of %s", rating, videoID)
	}
	settle(1, 3*time.Second)
	if len(notices) != 1 || notices[0] != "Removed from liked songs." || undo == nil {
		t.Fatalf("an unlike should offer one Undo once the daemon has done it, got %q", notices)
	}
	undo()
	if rating := rateParams(t, waitForUndoRequest(t, requests, protocol.MethodRate)); rating.VideoID != videoID || rating.Rating != model.RatingLike {
		t.Fatalf("Undo asked for %+v, want a like of %s", rating, videoID)
	}
	// a like is its own undo (the heart): it raises no placard
	settle(2, 300*time.Millisecond)
	if len(notices) != 1 {
		t.Fatalf("the like offered another notice: %q", notices)
	}
}

func rateParams(t *testing.T, request protocol.Request) protocol.RateParams {
	t.Helper()
	var params protocol.RateParams
	if err := json.Unmarshal(request.Params, &params); err != nil {
		t.Fatal(err)
	}
	return params
}

func serveUndoTest(listener net.Listener, requests chan<- protocol.Request) {
	connection, err := listener.Accept()
	if err != nil {
		return
	}
	defer connection.Close()
	reader := bufio.NewReader(connection)
	encoder := json.NewEncoder(connection)
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			return
		}
		var request protocol.Request
		if json.Unmarshal(line, &request) != nil {
			continue
		}
		result := json.RawMessage("null")
		if request.Method == protocol.MethodHello || request.Method == protocol.MethodSubscribe {
			result = json.RawMessage("{}")
		}
		if encoder.Encode(protocol.Response{ID: request.ID, Result: result}) != nil {
			return
		}
		requests <- request
	}
}

func waitForUndoRequest(t *testing.T, requests <-chan protocol.Request, method string) protocol.Request {
	t.Helper()
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	for {
		select {
		case request := <-requests:
			if request.Method == method {
				return request
			}
		case <-timer.C:
			t.Fatalf("timed out waiting for %s", method)
		}
	}
}
