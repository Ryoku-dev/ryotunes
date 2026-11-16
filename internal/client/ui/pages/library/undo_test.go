package library

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

// An Undo of "Stopped watching" that the daemon refuses (the folder was unmounted) says
// why, instead of failing silently.
func TestWatchFolderUndoShowsItsFailure(t *testing.T) {
	socket := filepath.Join(t.TempDir(), "ryotunesd.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
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
			response := protocol.Response{ID: request.ID, Result: json.RawMessage("{}")}
			if request.Method == protocol.MethodAddLocalFolder {
				response = protocol.Response{ID: request.ID, Error: &protocol.Error{Code: protocol.ErrorFailed, Message: "folder does not exist"}}
			}
			if encoder.Encode(response) != nil {
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
	errors := make(chan string, 4)
	e := &env.Env{
		Conn:  client,
		Go:    func(run func(context.Context)) { go run(context.Background()) },
		Toast: func(message string, kind env.ToastKind) { errors <- string(kind) + ": " + message },
	}
	p := New(env.Route{}).(*Page)
	p.watchFolder(e, "/run/media/usb/Music")

	deadline := time.After(5 * time.Second)
	for {
		act.PendingClipboard(layout.Context{})
		select {
		case notice := <-errors:
			if notice != "error: folder does not exist" {
				t.Fatalf("notice = %q", notice)
			}
			return
		case <-deadline:
			t.Fatal("a refused Undo said nothing")
		case <-time.After(10 * time.Millisecond):
		}
	}
}
