package widgets

import (
	"bufio"
	"context"
	"encoding/json"
	"image"
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

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
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

func TestLikingARowFillsItsHeartAtRestOnceTheDaemonConfirms(t *testing.T) {
	socket, requests := recordingDaemon(t)
	client := conn.NewPath(socket)
	t.Cleanup(client.Close)
	theme := hubTheme()
	e := &env.Env{
		Th: theme, Conn: client, Store: state.NewEmpty(),
		Go: func(fn func(context.Context)) { go fn(context.Background()) },
	}
	indifferent := model.RatingIndifferent
	items := []model.Track{
		{VideoID: "rowHeart001", Title: "Night drive", Artists: "Someone", Rating: &indifferent},
		{VideoID: "rowHeart002", Title: "Other song", Artists: "Someone", Rating: &indifferent},
	}
	var table TrackTable
	frame := newHubFrame(900, 400)
	rows := func() {
		frame.run(func(gtx layout.Context) layout.Dimensions {
			act.PendingClipboard(gtx)
			height := gtx.Dp(theme.RowHeight())
			for index := range items {
				offset := op.Offset(image.Pt(0, index*height)).Push(gtx.Ops)
				table.Row(gtx, e, items, index, TrackTableOpts{})
				offset.Pop()
			}
			return layout.Dimensions{}
		})
	}
	rows()
	var scratch op.Ops
	gtx := frame.context()
	gtx.Ops = &scratch
	gtx.Constraints = layout.Constraints{Max: image.Pt(800, 100)}
	titleWidth := kit.Label{Style: theme.T.Row, MaxLines: 1}.Layout(gtx, theme.Theme, items[0].Title).Size.X
	heartX := float32(gtx.Dp(48) + titleWidth + gtx.Dp(8) + gtx.Dp(8))
	heartY := float32(gtx.Dp(theme.RowHeight()) / 2)

	// Hover the row: its outline heart appears, leaning 2 dp with the row.
	hover := f32.Pt(heartX+float32(gtx.Dp(2)), heartY)
	frame.router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, PointerID: 1, Position: hover})
	frame.advance(400 * time.Millisecond)
	rows()
	rows()
	if trackLiked(e, items[0]) {
		t.Fatal("the row reads liked before it was liked")
	}
	frame.router.Queue(
		pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Buttons: pointer.ButtonPrimary, Position: hover},
		pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: hover},
	)
	rows()
	if rating := awaitRate(t, requests, rows); rating != model.RatingLike {
		t.Fatalf("the heart sent %q, want %q", rating, model.RatingLike)
	}
	for deadline := time.Now().Add(5 * time.Second); !trackLiked(e, items[0]); time.Sleep(10 * time.Millisecond) {
		rows()
		if time.Now().After(deadline) {
			t.Fatal("the daemon confirmed the like but the row still reads unliked")
		}
	}

	// Leave the row and let it settle: a liked row keeps its filled heart at
	// rest, so a press where the heart stands reaches it (an unliked row at
	// rest has no heart there, and the press would only select the row).
	frame.router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, PointerID: 1, Position: f32.Pt(heartX, 350)})
	frame.advance(time.Second)
	rows()
	rows()
	rest := f32.Pt(heartX, heartY)
	frame.router.Queue(
		pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Buttons: pointer.ButtonPrimary, Position: rest},
		pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: rest},
	)
	rows()
	if rating := awaitRate(t, requests, rows); rating != model.RatingIndifferent {
		t.Fatalf("the heart at rest sent %q, want %q (unlike)", rating, model.RatingIndifferent)
	}
}

// TestRatingEventsFromElsewhereFillAndEmptyRows: a like or unlike made
// elsewhere (another client, Ctrl L, the player bar) reaches rows fetched
// before it, both ways, and a row filled that way unlikes from its heart at rest.
func TestRatingEventsFromElsewhereFillAndEmptyRows(t *testing.T) {
	socket, requests := recordingDaemon(t)
	client := conn.NewPath(socket)
	t.Cleanup(client.Close)
	theme := hubTheme()
	store := state.NewEmpty()
	e := &env.Env{
		Th: theme, Conn: client, Store: store,
		Go: func(fn func(context.Context)) { go fn(context.Background()) },
	}
	liked := model.RatingLike
	items := []model.Track{
		{VideoID: "elsewhere01", Title: "Night drive", Artists: "Someone"},
		{VideoID: "elsewhere02", Title: "Other song", Artists: "Someone", Rating: &liked},
	}
	var table TrackTable
	frame := newHubFrame(900, 400)
	rows := func() {
		frame.run(func(gtx layout.Context) layout.Dimensions {
			act.PendingClipboard(gtx)
			height := gtx.Dp(theme.RowHeight())
			for index := range items {
				offset := op.Offset(image.Pt(0, index*height)).Push(gtx.Ops)
				table.Row(gtx, e, items, index, TrackTableOpts{})
				offset.Pop()
			}
			return layout.Dimensions{}
		})
	}
	event := func(id string, rating model.Rating) {
		raw, err := json.Marshal(protocol.RatingEvent{VideoID: id, Rating: rating})
		if err != nil {
			t.Fatal(err)
		}
		if err := store.Apply(protocol.EventRating, raw); err != nil {
			t.Fatal(err)
		}
		frame.advance(time.Second)
		rows()
		rows()
	}
	rows()
	if trackLiked(e, items[0]) || !trackLiked(e, items[1]) {
		t.Fatal("rows should read as they were fetched before any event")
	}

	event(items[0].VideoID, model.RatingLike)
	event(items[1].VideoID, model.RatingIndifferent)
	if !trackLiked(e, items[0]) {
		t.Fatal("a like from elsewhere left the row unliked")
	}
	if trackLiked(e, items[1]) {
		t.Fatal("an unlike from elsewhere left the row liked")
	}

	var scratch op.Ops
	gtx := frame.context()
	gtx.Ops = &scratch
	gtx.Constraints = layout.Constraints{Max: image.Pt(800, 100)}
	titleWidth := kit.Label{Style: theme.T.Row, MaxLines: 1}.Layout(gtx, theme.Theme, items[0].Title).Size.X
	rest := f32.Pt(float32(gtx.Dp(48)+titleWidth+gtx.Dp(8)+gtx.Dp(8)), float32(gtx.Dp(theme.RowHeight())/2))
	frame.router.Queue(
		pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Buttons: pointer.ButtonPrimary, Position: rest},
		pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: rest},
	)
	rows()
	if rating := awaitRate(t, requests, rows); rating != model.RatingIndifferent {
		t.Fatalf("the heart filled by the event sent %q, want %q (unlike)", rating, model.RatingIndifferent)
	}
}
