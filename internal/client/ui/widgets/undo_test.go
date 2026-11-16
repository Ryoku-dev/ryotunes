package widgets

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/tokens"
	"github.com/ryoku-dev/ryotunes/internal/client/conn"
	"github.com/ryoku-dev/ryotunes/internal/client/state"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/demo"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

// recordingDaemon runs the demo daemon behind a proxy socket that records
// every request the client sends, so a test sees exactly what reached it.
func recordingDaemon(t *testing.T) (socket string, requests <-chan protocol.Request) {
	t.Helper()
	dir, err := os.MkdirTemp("", "rtw")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() {
		cancel()
		_ = os.RemoveAll(dir)
	})
	daemon := filepath.Join(dir, "d.sock")
	go func() { _ = demo.Serve(ctx, daemon, false, false) }()
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		if _, err := os.Stat(daemon); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("demo daemon did not start")
		}
	}
	socket = filepath.Join(dir, "p.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	seen := make(chan protocol.Request, 256)
	go func() {
		for {
			client, err := listener.Accept()
			if err != nil {
				return
			}
			server, err := net.Dial("unix", daemon)
			if err != nil {
				_ = client.Close()
				return
			}
			go func() { _, _ = io.Copy(client, server); _ = client.Close() }()
			go func() {
				defer server.Close()
				lines := bufio.NewScanner(client)
				lines.Buffer(make([]byte, 1<<20), 1<<24)
				for lines.Scan() {
					var request protocol.Request
					if json.Unmarshal(lines.Bytes(), &request) == nil {
						select {
						case seen <- request:
						default:
						}
					}
					if _, err := server.Write(append(lines.Bytes(), '\n')); err != nil {
						return
					}
				}
			}()
		}
	}()
	return socket, seen
}

// awaitRate waits for a rate request and returns its rating.
func awaitRate(t *testing.T, requests <-chan protocol.Request, pump func()) model.Rating {
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
			return params.Rating
		case <-deadline:
			t.Fatal("no rate request reached the daemon")
		case <-time.After(10 * time.Millisecond):
			pump()
		}
	}
}

func TestUnlikeFromTheItemMenuOffersUndoThatLikesAgain(t *testing.T) {
	socket, requests := recordingDaemon(t)
	client := conn.NewPath(socket)
	t.Cleanup(client.Close)
	theme := hubTheme()
	type offer struct {
		message string
		undo    func()
	}
	offers := make(chan offer, 4)
	e := &env.Env{
		Th: theme, Conn: client,
		Go:       func(fn func(context.Context)) { go fn(context.Background()) },
		Undoable: func(message string, undo func()) { offers <- offer{message, undo} },
	}
	liked := model.RatingLike
	track := model.Track{VideoID: "dQw4w9WgXcQ", Title: "Night drive", Artists: "Someone", Rating: &liked}
	target := env.MenuTarget{Track: &track}
	var menu ItemMenu
	frame := newHubFrame(600, 900)
	layoutMenu := func() {
		frame.run(func(gtx layout.Context) layout.Dimensions {
			act.PendingClipboard(gtx)
			return menu.Layout(gtx, e, target)
		})
	}
	layoutMenu()

	entries := targetEntries(&menu, e, target)
	row := -1
	for index, entry := range entries {
		if entry.item.Label == "Unlike" {
			row = index
		}
	}
	if row < 0 {
		t.Fatal("a liked track's menu has no Unlike")
	}
	var scratch op.Ops
	gtx := frame.context()
	gtx.Ops = &scratch
	kind, title, detail := menuHead(target)
	y := menu.head(gtx, e, kind, title, detail) + gtx.Dp(menuRuleGap) + max(1, gtx.Dp(1)) + gtx.Dp(tokens.SizeMenuPaddingVertical) +
		row*gtx.Dp(tokens.SizeMenuItem) + gtx.Dp(tokens.SizeMenuItem)/2
	at := f32.Pt(40, float32(y))
	frame.router.Queue(
		pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Buttons: pointer.ButtonPrimary, Position: at},
		pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: at},
	)
	layoutMenu()
	layoutMenu()

	if rating := awaitRate(t, requests, layoutMenu); rating != model.RatingIndifferent {
		t.Fatalf("Unlike sent rating %q, want %q", rating, model.RatingIndifferent)
	}
	var shown offer
	for deadline := time.Now().Add(5 * time.Second); shown.undo == nil; {
		select {
		case shown = <-offers:
		case <-time.After(10 * time.Millisecond):
			layoutMenu()
		}
		if time.Now().After(deadline) {
			t.Fatal("the daemon unliked the song but no Undo was offered")
		}
	}
	if shown.message != "Removed from liked songs." {
		t.Fatalf("notice = %q", shown.message)
	}
	shown.undo()
	if rating := awaitRate(t, requests, layoutMenu); rating != model.RatingLike {
		t.Fatalf("Undo sent rating %q, want %q", rating, model.RatingLike)
	}
}

func TestRemovingASavedCollectionUndoesToTheSamePlace(t *testing.T) {
	store := state.NewEmpty()
	cards := []model.BrowseItem{
		{Kind: "playlist", ID: "a", Title: "First"},
		{Kind: "playlist", ID: "b", Title: "Second"},
		{Kind: "playlist", ID: "c", Title: "Third"},
	}
	store.UpdatePersonal(func(personal *state.Personal) bool {
		personal.Saved = append(personal.Saved, cards...)
		return true
	})
	var message string
	var undo func()
	e := &env.Env{Th: hubTheme(), Store: store, Undoable: func(m string, u func()) { message, undo = m, u }}
	entries := collectionEntries(e, cards[1])
	for _, entry := range entries {
		if entry.item.Label == "Remove from library" {
			entry.run()
		}
	}
	if undo == nil || message != "Removed Second from your library." {
		t.Fatalf("removal offered %q, undo %v", message, undo != nil)
	}
	if saved := store.Snapshot().Personal.Saved; len(saved) != 2 {
		t.Fatalf("library holds %d after the removal, want 2", len(saved))
	}
	undo()
	saved := store.Snapshot().Personal.Saved
	if len(saved) != 3 || saved[1].ID != "b" {
		t.Fatalf("after Undo the library is %v, want Second back in the middle", saved)
	}
	undo()
	if again := store.Snapshot().Personal.Saved; len(again) != 3 {
		t.Fatalf("a second Undo duplicated the collection: %v", again)
	}
}
