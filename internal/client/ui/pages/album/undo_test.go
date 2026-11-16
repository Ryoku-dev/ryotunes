package album

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"path/filepath"
	"testing"
	"time"

	"gioui.org/layout"

	"github.com/ryoku-dev/ryotunes/internal/client/conn"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

// Removing an album offers Undo; the Undo saves it again without a "Saved" notice of its
// own (the item menu's Undo is silent), while a plain save still says so.
func TestRemoveAlbumUndoIsSilent(t *testing.T) {
	socket := filepath.Join(t.TempDir(), "ryotunesd.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	saves := make(chan bool, 8)
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
			if request.Method == protocol.MethodHello || request.Method == protocol.MethodSubscribe {
				result = json.RawMessage("{}")
			}
			if encoder.Encode(protocol.Response{ID: request.ID, Result: result}) != nil {
				return
			}
			if request.Method == protocol.MethodSetAlbumSaved {
				var params protocol.SetAlbumSavedParams
				_ = json.Unmarshal(request.Params, &params)
				saves <- params.Saved
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
	var notices []string
	var undo func()
	e := &env.Env{
		Conn:     client,
		Go:       func(run func(context.Context)) { go run(context.Background()) },
		Toast:    func(message string, _ env.ToastKind) { notices = append(notices, message) },
		Undoable: func(message string, run func()) { notices, undo = append(notices, message), run },
	}
	p := New(env.Route{Kind: env.RouteAlbum, ID: "MPREb_album"}).(*Page)
	p.saved = true
	settle := func(want bool) {
		t.Helper()
		select {
		case saved := <-saves:
			if saved != want {
				t.Fatalf("saved = %v, want %v", saved, want)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("no set_album_saved reached the daemon")
		}
		for deadline := time.Now().Add(5 * time.Second); p.savePending; time.Sleep(5 * time.Millisecond) {
			act.PendingClipboard(layout.Context{})
			if time.Now().After(deadline) {
				t.Fatal("the answer never arrived")
			}
		}
	}

	p.setSaved(e, false, false)
	settle(false)
	if undo == nil || len(notices) != 1 || notices[0] != "Removed from your library." {
		t.Fatalf("notices = %q, undo %v", notices, undo != nil)
	}
	undo()
	settle(true)
	if len(notices) != 1 || !p.saved {
		t.Fatalf("the Undo raised %q (saved %v)", notices[1:], p.saved)
	}
	p.setSaved(e, false, false)
	settle(false)
	p.setSaved(e, true, false)
	settle(true)
	if notices[len(notices)-1] != "Saved to library" {
		t.Fatalf("a plain save said %q", notices[len(notices)-1])
	}
}
