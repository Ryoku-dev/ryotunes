package shell

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestNiriAllowsWindowSizePersistence(t *testing.T) {
	tests := []struct {
		name     string
		floating bool
		width    int
		height   int
		want     bool
	}{
		{name: "floating", floating: true, width: 1280, height: 800, want: true},
		{name: "tiled", floating: false, width: 1280, height: 800},
		{name: "fullscreen", floating: true, width: 1920, height: 1200},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			socket := serveWindowIPC(t, 2, func(request string) string {
				switch request {
				case `"Windows"`:
					return fmt.Sprintf(`{"Ok":{"Windows":[{"title":"Ryotunes","pid":42,"is_floating":%t,"layout":{"tile_size":[%d,%d],"window_size":[%d,%d]}}]}}`, tt.floating, tt.width, tt.height, tt.width, tt.height)
				case `"Outputs"`:
					return `{"Ok":{"Outputs":{"winit":{"logical":{"width":1920,"height":1200}}}}}`
				default:
					return `{"Err":"unexpected request"}`
				}
			})
			got, err := niriAllowsWindowSizePersistence(context.Background(), socket, 42, "Ryotunes")
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("allowed = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestHyprlandAllowsWindowSizePersistence(t *testing.T) {
	tests := []struct {
		name       string
		floating   bool
		fullscreen int
		want       bool
	}{
		{name: "floating", floating: true, want: true},
		{name: "tiled"},
		{name: "fullscreen", floating: true, fullscreen: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			socket := serveWindowIPC(t, 1, func(request string) string {
				if request != "j/clients" {
					return `[]`
				}
				return fmt.Sprintf(`[{"title":"Ryotunes","pid":42,"floating":%t,"fullscreen":%d}]`, tt.floating, tt.fullscreen)
			})
			got, err := hyprlandAllowsWindowSizePersistence(context.Background(), socket, 42, "Ryotunes")
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("allowed = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestWindowSizePersistenceDebounces(t *testing.T) {
	var queries atomic.Int32
	persistence := windowSizePersistence{
		debounce: 10 * time.Millisecond,
		allow: func(context.Context, int, string) (bool, error) {
			queries.Add(1)
			return true, nil
		},
	}
	saved := make(chan [2]int, 1)
	save := func(width, height int) { saved <- [2]int{width, height} }
	persistence.schedule(windowSizeCandidate{width: 800, height: 600, pid: 42, title: "Ryotunes"}, save)
	persistence.schedule(windowSizeCandidate{width: 1000, height: 700, pid: 42, title: "Ryotunes"}, save)
	select {
	case size := <-saved:
		if size != [2]int{1000, 700} {
			t.Fatalf("saved size = %v, want latest 1000x700", size)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for debounced save")
	}
	if queries.Load() != 1 {
		t.Fatalf("queries = %d, want 1", queries.Load())
	}
	persistence.cancel()
}

func serveWindowIPC(t *testing.T, connections int, response func(string) string) string {
	t.Helper()
	dir := t.TempDir()
	socket := filepath.Join(dir, "ipc.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		for range connections {
			connection, err := listener.Accept()
			if err != nil {
				return
			}
			buffer := make([]byte, 256)
			count, err := connection.Read(buffer)
			if err == nil {
				_, _ = connection.Write([]byte(response(strings.TrimSpace(string(buffer[:count]))) + "\n"))
			}
			_ = connection.Close()
		}
	}()
	if _, err := os.Stat(socket); err != nil {
		t.Fatal(err)
	}
	return socket
}
