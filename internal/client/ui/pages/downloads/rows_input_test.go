package downloads

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
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

func TestSavedRowPlayActionQueuesVisibleSavedContext(t *testing.T) {
	socket := filepath.Join(t.TempDir(), "ryotunesd.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	requests := make(chan protocol.Request, 16)
	go serveDownloadRowTest(listener, requests)
	t.Cleanup(func() { _ = listener.Close() })

	client := conn.NewPath(socket)
	t.Cleanup(client.Close)
	waitForDownloadRowRequest(t, requests, protocol.MethodSubscribe)

	theme := look.NewTheme(palette.Signature(true), libtheme.Comfortable)
	e := &env.Env{Th: theme, Overlays: new(look.Overlays), Conn: client}
	e.Go = func(run func(context.Context)) { go run(context.Background()) }
	contextJobs := []model.DownloadEntry{
		{ID: "saved-a", VideoID: "a", Title: "Saved A", Status: model.DownloadStatusCompleted},
		{ID: "saved-b", VideoID: "b", Title: "Saved B", Status: model.DownloadStatusCompleted},
	}
	job := contextJobs[1]
	contextItems := []downloadItem{{job: &contextJobs[0]}, {job: &contextJobs[1]}}
	var row downloadRow
	var operations op.Ops
	var router input.Router
	now := time.Unix(100, 0)
	frame := func() {
		operations.Reset()
		gtx := layout.Context{
			Ops: &operations, Source: router.Source(), Now: now,
			Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(600, 48)),
		}
		row.layout(gtx, e, job, contextItems)
		router.Frame(&operations)
		now = now.Add(time.Millisecond)
	}

	frame()
	point := f32.Pt(560, 24)
	router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, PointerID: 1, Position: point})
	frame()
	frame()
	router.Queue(
		pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Buttons: pointer.ButtonPrimary, Position: point},
		pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: point},
	)
	frame()
	frame()

	request := waitForDownloadRowRequest(t, requests, protocol.MethodPlayPlaylist)
	var params protocol.PlayPlaylistParams
	if err := json.Unmarshal(request.Params, &params); err != nil {
		t.Fatal(err)
	}
	if params.Start == nil || *params.Start != 1 {
		t.Fatalf("start = %v, want 1", params.Start)
	}
	if len(params.Items) != 2 || params.Items[0].VideoID != "a" || params.Items[1].VideoID != "b" {
		t.Fatalf("items = %#v", params.Items)
	}
	if params.SourceName == nil || *params.SourceName != "Downloads" {
		t.Fatalf("source name = %v", params.SourceName)
	}
}
func TestSavedRowSecondaryClickOpensContextMenu(t *testing.T) {
	theme := look.NewTheme(palette.Signature(true), libtheme.Comfortable)
	overlays := new(look.Overlays)
	e := &env.Env{Th: theme, Overlays: overlays}
	job := model.DownloadEntry{ID: "saved", VideoID: "saved", Title: "Saved", Status: model.DownloadStatusCompleted}
	var row downloadRow
	var operations op.Ops
	var router input.Router
	now := time.Unix(200, 0)
	frame := func() {
		operations.Reset()
		gtx := layout.Context{
			Ops: &operations, Source: router.Source(), Now: now,
			Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(600, 48)),
		}
		row.layout(gtx, e, job, []downloadItem{{job: &job}})
		overlays.Layout(gtx, theme)
		router.Frame(&operations)
		now = now.Add(time.Millisecond)
	}

	frame()
	point := f32.Pt(200, 24)
	router.Queue(pointer.Event{
		Kind: pointer.Press, Source: pointer.Mouse, PointerID: 2,
		Buttons: pointer.ButtonSecondary, Position: point,
	})
	frame()
	if overlays.IsOpen(&row.menuTag) {
		t.Fatal("secondary press opened the menu before its release")
	}
	router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 2, Position: point})
	frame()
	frame()
	if !overlays.IsOpen(&row.menuTag) || overlays.Press() != point.Round() {
		t.Fatalf("row menu open = %v, press = %v, want %v", overlays.IsOpen(&row.menuTag), overlays.Press(), point.Round())
	}
}

func TestPartlyFailedCollectionSecondaryClickOffersSavedPlayback(t *testing.T) {
	theme := look.NewTheme(palette.Signature(true), libtheme.Comfortable)
	overlays := new(look.Overlays)
	e := &env.Env{Th: theme, Overlays: overlays}
	collection := "Album"
	jobs := []model.DownloadEntry{
		{ID: "saved", VideoID: "saved", Title: "Saved", Status: model.DownloadStatusCompleted, Collection: &collection},
		{ID: "failed", VideoID: "failed", Title: "Failed", Status: model.DownloadStatusFailed, Collection: &collection},
	}
	group := downloadCollection{key: "history:album", label: collection, jobs: jobs}
	items := []downloadItem{{collection: &group}}
	var card collectionCard
	var operations op.Ops
	var router input.Router
	now := time.Unix(300, 0)
	frame := func() {
		operations.Reset()
		gtx := layout.Context{
			Ops: &operations, Source: router.Source(), Now: now,
			Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(600, 48)),
		}
		card.layout(gtx, e, group, make(map[string]*downloadRow), items, jobs)
		overlays.Layout(gtx, theme)
		router.Frame(&operations)
		now = now.Add(time.Millisecond)
	}

	frame()
	if card.action.Label != "Retry" {
		t.Fatalf("primary action = %q, want Retry", card.action.Label)
	}
	point := f32.Pt(200, 24)
	router.Queue(pointer.Event{
		Kind: pointer.Press, Source: pointer.Mouse, PointerID: 3,
		Buttons: pointer.ButtonSecondary, Position: point,
	})
	frame()
	router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 3, Position: point})
	frame()
	frame()
	if !overlays.IsOpen(&card.menuTag) || overlays.Press() != point.Round() {
		t.Fatalf("collection menu open = %v, press = %v, want %v", overlays.IsOpen(&card.menuTag), overlays.Press(), point.Round())
	}
}

func serveDownloadRowTest(listener net.Listener, requests chan<- protocol.Request) {
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

func waitForDownloadRowRequest(t *testing.T, requests <-chan protocol.Request, method string) protocol.Request {
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
