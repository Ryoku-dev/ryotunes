package playlist

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"net"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"gioui.org/io/input"
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
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

// testNav is the router surface the page reads; nothing else is reached.
type testNav struct{ env.Nav }

func (testNav) Panel() env.Panel { return env.PanelNone }
func (testNav) NowPlaying() bool { return false }
func (testNav) Current() env.Route {
	return env.Route{Kind: env.RoutePlaylist, ID: "VLLM"}
}

// likedPageJSON is YouTube Music's Liked music as the daemon answers it: no title, no
// subtitle, a first page of tracks and a continuation (synthetic tracks).
func likedPageJSON(count int) json.RawMessage {
	items := make([]map[string]any, count)
	for index := range items {
		items[index] = map[string]any{
			"video_id": fmt.Sprintf("vid%08d", index), "title": fmt.Sprintf("Song %02d", index+1), "artists": "Someone",
			"rating": "LIKE", "queued": false, "queued_end": false, "autoplay": false, "is_video": false, "is_upload": false, "explicit": false,
		}
	}
	raw, _ := json.Marshal(map[string]any{
		"title": nil, "subtitle": nil, "thumbnail": "https://img.test/liked.jpg", "description": nil, "privacy": nil,
		"cover": "https://img.test/liked.jpg", "items": items, "continuation": "token-1", "owned": false, "collaborative": false,
		"sortMenu": map[string]any{"selected": "default", "editable": false},
	})
	return raw
}

func serveLiked(t *testing.T, page func(call int32) json.RawMessage) (*conn.Client, *atomic.Int32) {
	t.Helper()
	socket := filepath.Join(t.TempDir(), "ryotunesd.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	var playlistCalls atomic.Int32
	go func() {
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
			switch request.Method {
			case protocol.MethodHello, protocol.MethodSubscribe:
				result = json.RawMessage("{}")
			case protocol.MethodGetPlaylist:
				result = page(playlistCalls.Add(1))
			case protocol.MethodGetPlaylistMore:
				result = json.RawMessage(`{"items":[],"continuation":null}`)
			}
			if encoder.Encode(protocol.Response{ID: request.ID, Result: result}) != nil {
				return
			}
		}
	}()
	client := conn.NewPath(socket)
	t.Cleanup(client.Close)
	for wait := time.Now().Add(5 * time.Second); !client.Connected(); time.Sleep(5 * time.Millisecond) {
		if time.Now().After(wait) {
			t.Fatal("the test daemon never connected")
		}
	}
	return client, &playlistCalls
}

// The sidebar's Liked songs, restored from the last session (route with no title), lists
// the songs the daemon sends and names itself, though the daemon sends no title.
func TestRestoredLikedSongsListsItsTracks(t *testing.T) {
	client, _ := serveLiked(t, func(int32) json.RawMessage { return likedPageJSON(20) })
	th := look.NewTheme(palette.Signature(true), libtheme.Comfortable)
	e := &env.Env{
		Th: th, Conn: client, Store: state.NewEmpty(), Overlays: new(look.Overlays), Nav: testNav{},
		Go: func(run func(context.Context)) { go run(context.Background()) },
	}
	p := New(env.Route{Kind: env.RoutePlaylist, ID: "VLLM"}).(*Page)

	var ops op.Ops
	var router input.Router
	now := time.Unix(900, 0)
	frame := func() {
		ops.Reset()
		gtx := layout.Context{Ops: &ops, Source: router.Source(), Now: now,
			Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(1000, 700))}
		act.PendingClipboard(gtx)
		p.Layout(gtx, e)
		router.Frame(&ops)
		now = now.Add(16 * time.Millisecond)
	}
	for deadline := time.Now().Add(5 * time.Second); len(p.page.Items) == 0 || p.loading; time.Sleep(5 * time.Millisecond) {
		frame()
		if time.Now().After(deadline) {
			t.Fatalf("the page never loaded (loading %v, items %d, error %q)", p.loading, len(p.page.Items), p.errText)
		}
	}
	for range 10 {
		frame()
		time.Sleep(2 * time.Millisecond)
	}
	if len(p.page.Items) != 20 {
		t.Fatalf("the page holds %d songs, want the daemon's 20", len(p.page.Items))
	}
	if got := p.Title(); got != "Liked songs" {
		t.Fatalf("title = %q, want Liked songs", got)
	}
	if len(p.meta) == 0 || p.meta[0].Text != "20 tracks" {
		t.Fatalf("meta = %+v, want 20 tracks first", p.meta)
	}
}

// A Liked songs page asked for before the daemon adopted the browser's session comes back
// empty (no title, no songs); once the session arrives the page asks again and lists them.
func TestLikedSongsReloadsWhenTheSessionArrives(t *testing.T) {
	client, calls := serveLiked(t, func(call int32) json.RawMessage {
		if call == 1 {
			return json.RawMessage(`{"title":null,"subtitle":null,"items":[],"continuation":null,"owned":false,"collaborative":false}`)
		}
		return likedPageJSON(20)
	})
	th := look.NewTheme(palette.Signature(true), libtheme.Comfortable)
	store := state.NewEmpty()
	e := &env.Env{
		Th: th, Conn: client, Store: store, Overlays: new(look.Overlays), Nav: testNav{},
		Go: func(run func(context.Context)) { go run(context.Background()) },
	}
	p := New(env.Route{Kind: env.RoutePlaylist, ID: "VLLM"}).(*Page)
	var ops op.Ops
	var router input.Router
	now := time.Unix(900, 0)
	frame := func() {
		ops.Reset()
		gtx := layout.Context{Ops: &ops, Source: router.Source(), Now: now,
			Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(1000, 700))}
		act.PendingClipboard(gtx)
		p.Layout(gtx, e)
		router.Frame(&ops)
		now = now.Add(16 * time.Millisecond)
	}
	settle := func(done func() bool, what string) {
		t.Helper()
		for deadline := time.Now().Add(5 * time.Second); !done(); time.Sleep(5 * time.Millisecond) {
			frame()
			if time.Now().After(deadline) {
				t.Fatalf("%s (calls %d, loading %v, items %d)", what, calls.Load(), p.loading, len(p.page.Items))
			}
		}
	}
	settle(func() bool { return calls.Load() == 1 && !p.loading }, "the first answer never arrived")
	for range 5 {
		frame()
	}
	if calls.Load() != 1 || len(p.page.Items) != 0 {
		t.Fatalf("before the session: %d calls, %d songs", calls.Load(), len(p.page.Items))
	}

	if err := store.Apply(protocol.EventAuthChanged, json.RawMessage(`{"signedIn":true,"name":"Nero","channelId":"UC123"}`)); err != nil {
		t.Fatal(err)
	}
	settle(func() bool { return calls.Load() == 2 && !p.loading && len(p.page.Items) == 20 }, "the page never asked again with the session")
	for range 5 {
		frame()
	}
	if calls.Load() != 2 || p.Title() != "Liked songs" {
		t.Fatalf("after the session: %d calls, title %q", calls.Load(), p.Title())
	}
}
