package shell

import (
	"bufio"
	"context"
	"encoding/json"
	"image"
	"net"
	"path/filepath"
	"testing"
	"time"

	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryotunes/internal/client/conn"
	"github.com/ryoku-dev/ryotunes/internal/client/state"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

// TestCtrlLUnlikeOffersTheHeartsUndo presses Ctrl L through the real router on a liked
// track against a fake daemon: the key unlikes through the hearts' shared action, so it
// offers the same Undo, which likes the track again.
func TestCtrlLUnlikeOffersTheHeartsUndo(t *testing.T) {
	socket := filepath.Join(t.TempDir(), "ryotunesd.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	requests := make(chan protocol.Request, 16)
	go serveKeysTest(listener, requests)
	client := conn.NewPath(socket)
	t.Cleanup(client.Close)
	waitForKeysRequest(t, requests, protocol.MethodSubscribe)

	const videoID = "dQw4w9WgXcQ"
	liked := model.RatingLike
	store := state.New(protocol.Snapshot{
		Provider: model.ProviderYouTube,
		Playback: model.PlaybackSnapshot{Now: &model.PlaybackTrack{VideoID: videoID, Title: "Stone", Artists: "Kite", Rating: &liked}},
	})
	nav := newNavigator(env.Route{Kind: env.RouteHome}, env.PanelNone, false, nil)
	e := &env.Env{Store: store, Nav: nav, Conn: client}
	e.Go = func(run func(context.Context)) { go run(context.Background()) }
	var notices []string
	var undo func()
	e.Undoable = func(message string, run func()) { notices, undo = append(notices, message), run }

	var shell uiShell
	var operations op.Ops
	var router input.Router
	frame := func() {
		operations.Reset()
		gtx := layout.Context{
			Ops: &operations, Now: time.Now(), Source: router.Source(),
			Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(800, 600)),
		}
		// the window delivers daemon replies at the start of each frame
		act.PendingClipboard(gtx)
		shell.handleKeys(gtx, e)
		router.Frame(&operations)
	}
	frame()
	router.Queue(key.Event{Name: "L", Modifiers: key.ModShortcut, State: key.Press})
	frame()

	request := waitForKeysRequest(t, requests, protocol.MethodRate)
	var params protocol.RateParams
	if err := json.Unmarshal(request.Params, &params); err != nil {
		t.Fatal(err)
	}
	if params.VideoID != videoID || params.Rating != model.RatingIndifferent {
		t.Fatalf("Ctrl L asked for %+v, want an unlike of %s", params, videoID)
	}
	// the Undo is offered once the daemon has done the unlike
	for deadline := time.Now().Add(3 * time.Second); len(notices) == 0 && time.Now().Before(deadline); {
		frame()
		time.Sleep(time.Millisecond)
	}
	if len(notices) != 1 || notices[0] != "Removed from liked songs." || undo == nil {
		t.Fatalf("Ctrl L should offer the hearts' one Undo, got %q", notices)
	}
	undo()
	request = waitForKeysRequest(t, requests, protocol.MethodRate)
	if err := json.Unmarshal(request.Params, &params); err != nil {
		t.Fatal(err)
	}
	if params.VideoID != videoID || params.Rating != model.RatingLike {
		t.Fatalf("Undo asked for %+v, want a like of %s", params, videoID)
	}
}

// serveKeysTest answers every request with an empty result and reports it.
func serveKeysTest(listener net.Listener, requests chan<- protocol.Request) {
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

func waitForKeysRequest(t *testing.T, requests <-chan protocol.Request, method string) protocol.Request {
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
