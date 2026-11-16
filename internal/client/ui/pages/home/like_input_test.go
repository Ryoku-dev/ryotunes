package home

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
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
	libtheme "github.com/ryoku-dev/ryoku/docs/lithic/library/go/theme"
	"github.com/ryoku-dev/ryotunes/internal/client/conn"
	"github.com/ryoku-dev/ryotunes/internal/client/state"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

// cardNav is the router surface the card's action row reads; nothing else is reached.
type cardNav struct{ env.Nav }

func (cardNav) Panel() env.Panel { return env.PanelNone }
func (cardNav) NowPlaying() bool { return false }

// Unliking from Home's now playing card goes through the shared heart: the daemon is told
// to unlike, then the notice offers Undo, which likes the song again.
func TestCardHeartUnlikeOffersUndo(t *testing.T) {
	socket := filepath.Join(t.TempDir(), "ryotunesd.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	requests := make(chan protocol.Request, 32)
	go serveCardTest(listener, requests)
	client := conn.NewPath(socket)
	t.Cleanup(client.Close)

	type offer struct {
		message string
		undo    func()
	}
	offers := make(chan offer, 4)
	th := look.NewTheme(palette.Signature(true), libtheme.Comfortable)
	e := &env.Env{
		Th: th, Conn: client, Overlays: new(look.Overlays), Nav: cardNav{}, Store: state.NewEmpty(),
		Go:       func(run func(context.Context)) { go run(context.Background()) },
		Undoable: func(message string, undo func()) { offers <- offer{message, undo} },
	}
	liked := model.RatingLike
	var card nowPlayingCard
	card.track = model.Track{VideoID: "dQw4w9WgXcQ", Title: "Stone in the water", Artists: "Kite Assembly"}
	card.snapshot.Rating = liked

	var ops op.Ops
	var router input.Router
	now := time.Unix(500, 0)
	frame := func() {
		ops.Reset()
		gtx := layout.Context{Ops: &ops, Source: router.Source(), Now: now,
			Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(700, 48))}
		act.PendingClipboard(gtx)
		card.layoutActions(gtx, e)
		e.Overlays.Layout(gtx, th)
		router.Frame(&ops)
		now = now.Add(5 * time.Millisecond)
	}
	frame()

	// find the heart by pointing along the row, as a pointer would
	at := f32.Point{}
	for x := float32(2); x < 700 && at == (f32.Point{}); x += 4 {
		point := f32.Pt(x, 24)
		router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, PointerID: 1, Position: point})
		frame()
		if card.like.Click.Clickable.Hovered() {
			at = point
		}
	}
	if at == (f32.Point{}) {
		t.Fatal("no point on the action row reaches the heart")
	}
	router.Queue(
		pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Buttons: pointer.ButtonPrimary, Position: at},
		pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: at},
	)
	frame()

	awaitRating := func(want model.Rating) {
		t.Helper()
		deadline := time.After(5 * time.Second)
		for {
			select {
			case request := <-requests:
				if request.Method != protocol.MethodRate {
					continue
				}
				var params protocol.RateParams
				if err := json.Unmarshal(request.Params, &params); err != nil {
					t.Fatal(err)
				}
				if params.Rating != want || params.VideoID != card.track.VideoID {
					t.Fatalf("rate %+v, want %q for %s", params, want, card.track.VideoID)
				}
				return
			case <-deadline:
				t.Fatalf("no %q rating reached the daemon", want)
			case <-time.After(10 * time.Millisecond):
				frame()
			}
		}
	}
	awaitRating(model.RatingIndifferent)
	var shown offer
	deadline := time.Now().Add(5 * time.Second)
	for shown.undo == nil {
		select {
		case shown = <-offers:
		case <-time.After(10 * time.Millisecond):
			frame()
		}
		if shown.undo == nil && time.Now().After(deadline) {
			t.Fatal("the card's unlike offered no Undo")
		}
	}
	if shown.message != "Removed from liked songs." {
		t.Fatalf("notice = %q", shown.message)
	}
	shown.undo()
	awaitRating(model.RatingLike)
}

func serveCardTest(listener net.Listener, requests chan<- protocol.Request) {
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
