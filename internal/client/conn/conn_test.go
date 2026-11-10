package conn

import (
	"bufio"
	"encoding/json"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

// The real daemon attaches a subscriber to the event bus before it writes the
// subscribe answer, so while music plays a position event usually comes first.
func TestSubscribeDeliversSnapshotWhenEventsArriveFirst(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ryotunesd.sock")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go serveEventsBeforeSnapshot(t, listener)

	client := NewPath(path)
	defer client.Close()

	select {
	case snapshot := <-client.Snapshots():
		if snapshot.Settings["provider"] != "soundcloud" {
			t.Fatalf("snapshot settings = %v, want the daemon's snapshot", snapshot.Settings)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no snapshot within 2 s: an event sent before the subscribe answer stalled the handshake")
	}
	for _, want := range []string{`{"position":1}`, `{"position":1.25}`} {
		select {
		case event := <-client.Events():
			if event.Event != protocol.EventPosition || string(event.Data) != want {
				t.Fatalf("event = %s %s, want %s %s in wire order", event.Event, event.Data, protocol.EventPosition, want)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("event %s was not delivered after the snapshot", want)
		}
	}
}

func serveEventsBeforeSnapshot(t *testing.T, listener net.Listener) {
	conn, err := listener.Accept()
	if err != nil {
		return
	}
	defer conn.Close()
	reader := bufio.NewReader(conn)
	encoder := protocol.NewEncoder(conn)
	snapshot, err := json.Marshal(protocol.Snapshot{Settings: model.SettingsSnapshot{"provider": "soundcloud"}})
	if err != nil {
		t.Error(err)
		return
	}
	hello, err := json.Marshal(protocol.Hello{Protocol: 1})
	if err != nil {
		t.Error(err)
		return
	}
	for {
		line, err := protocol.ReadLine(reader)
		if err != nil {
			return
		}
		var request protocol.Request
		if err := json.Unmarshal(line, &request); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		switch request.Method {
		case protocol.MethodHello:
			err = encoder.Encode(protocol.Response{ID: request.ID, Result: hello})
		case protocol.MethodSubscribe:
			err = encoder.Encode(protocol.Event{Event: protocol.EventPosition, Data: json.RawMessage(`{"position":1}`)})
			if err == nil {
				err = encoder.Encode(protocol.Response{ID: request.ID, Result: snapshot})
			}
			if err == nil {
				err = encoder.Encode(protocol.Event{Event: protocol.EventPosition, Data: json.RawMessage(`{"position":1.25}`)})
			}
		default:
			err = encoder.Encode(protocol.Response{ID: request.ID, Result: json.RawMessage("null")})
		}
		if err != nil {
			return
		}
	}
}
