package shell

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

const windowPersistDebounce = 250 * time.Millisecond

type windowSizeCandidate struct {
	width, height int
	pid           int
	title         string
}

type windowSizePersistence struct {
	mu       sync.Mutex
	timer    *time.Timer
	serial   uint64
	debounce time.Duration
	allow    func(context.Context, int, string) (bool, error)
}

func (p *windowSizePersistence) schedule(candidate windowSizeCandidate, save func(int, int)) {
	p.mu.Lock()
	p.serial++
	serial := p.serial
	if p.timer != nil {
		p.timer.Stop()
	}
	delay := p.debounce
	if delay <= 0 {
		delay = windowPersistDebounce
	}
	allow := p.allow
	if allow == nil {
		allow = compositorAllowsWindowSizePersistence
	}
	p.timer = time.AfterFunc(delay, func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		ok, err := allow(ctx, candidate.pid, candidate.title)
		p.mu.Lock()
		current := serial == p.serial
		if current {
			p.timer = nil
		}
		if current && err == nil && ok {
			save(candidate.width, candidate.height)
		}
		p.mu.Unlock()
	})
	p.mu.Unlock()
}

func (p *windowSizePersistence) cancel() {
	p.mu.Lock()
	p.serial++
	if p.timer != nil {
		p.timer.Stop()
		p.timer = nil
	}
	p.mu.Unlock()
}

func compositorAllowsWindowSizePersistence(ctx context.Context, pid int, title string) (bool, error) {
	if socket := os.Getenv("NIRI_SOCKET"); socket != "" {
		return niriAllowsWindowSizePersistence(ctx, socket, pid, title)
	}
	if signature := os.Getenv("HYPRLAND_INSTANCE_SIGNATURE"); signature != "" {
		return hyprlandAllowsWindowSizePersistence(ctx, hyprlandSocket(signature), pid, title)
	}
	return true, nil
}

type niriWindow struct {
	Title      string `json:"title"`
	PID        int    `json:"pid"`
	IsFloating bool   `json:"is_floating"`
	Layout     struct {
		TileSize   [2]float64 `json:"tile_size"`
		WindowSize [2]int     `json:"window_size"`
	} `json:"layout"`
}

type niriOutput struct {
	Logical *struct {
		Width  int `json:"width"`
		Height int `json:"height"`
	} `json:"logical"`
}

func niriAllowsWindowSizePersistence(ctx context.Context, socket string, pid int, title string) (bool, error) {
	var windows struct {
		Windows []niriWindow `json:"Windows"`
	}
	if err := niriRequest(ctx, socket, "Windows", &windows); err != nil {
		return false, err
	}
	var outputs struct {
		Outputs map[string]niriOutput `json:"Outputs"`
	}
	if err := niriRequest(ctx, socket, "Outputs", &outputs); err != nil {
		return false, err
	}
	for _, window := range windows.Windows {
		if window.PID != pid || window.Title != title {
			continue
		}
		if !window.IsFloating {
			return false, nil
		}
		for _, output := range outputs.Outputs {
			if output.Logical == nil {
				continue
			}
			if fillsOutput(window.Layout.WindowSize[0], window.Layout.WindowSize[1], output.Logical.Width, output.Logical.Height) ||
				fillsOutput(int(window.Layout.TileSize[0]), int(window.Layout.TileSize[1]), output.Logical.Width, output.Logical.Height) {
				return false, nil
			}
		}
		return true, nil
	}
	return false, nil
}

func fillsOutput(width, height, outputWidth, outputHeight int) bool {
	return outputWidth > 0 && outputHeight > 0 && width*100 >= outputWidth*93 && height*100 >= outputHeight*93
}

func niriRequest(ctx context.Context, socket, request string, result any) error {
	connection, err := (&net.Dialer{}).DialContext(ctx, "unix", socket)
	if err != nil {
		return err
	}
	defer connection.Close()
	deadline, ok := ctx.Deadline()
	if ok {
		_ = connection.SetDeadline(deadline)
	}
	if err := json.NewEncoder(connection).Encode(request); err != nil {
		return err
	}
	var response struct {
		OK    json.RawMessage `json:"Ok"`
		Error json.RawMessage `json:"Err"`
	}
	if err := json.NewDecoder(connection).Decode(&response); err != nil {
		return err
	}
	if len(response.Error) != 0 && string(response.Error) != "null" {
		return errors.New("niri IPC request failed")
	}
	if len(response.OK) == 0 {
		return errors.New("niri IPC response has no result")
	}
	return json.Unmarshal(response.OK, result)
}

type hyprlandClient struct {
	Title            string          `json:"title"`
	PID              int             `json:"pid"`
	Floating         bool            `json:"floating"`
	Fullscreen       json.RawMessage `json:"fullscreen"`
	FullscreenClient json.RawMessage `json:"fullscreenClient"`
}

func hyprlandAllowsWindowSizePersistence(ctx context.Context, socket string, pid int, title string) (bool, error) {
	connection, err := (&net.Dialer{}).DialContext(ctx, "unix", socket)
	if err != nil {
		return false, err
	}
	defer connection.Close()
	deadline, ok := ctx.Deadline()
	if ok {
		_ = connection.SetDeadline(deadline)
	}
	if _, err := connection.Write([]byte("j/clients")); err != nil {
		return false, err
	}
	var clients []hyprlandClient
	if err := json.NewDecoder(connection).Decode(&clients); err != nil {
		return false, err
	}
	for _, client := range clients {
		if client.PID == pid && client.Title == title {
			return client.Floating && !fullscreenValue(client.Fullscreen) && !fullscreenValue(client.FullscreenClient), nil
		}
	}
	return false, nil
}

func fullscreenValue(raw json.RawMessage) bool {
	if string(raw) == "true" {
		return true
	}
	value, err := strconv.Atoi(string(raw))
	return err == nil && value != 0
}

func hyprlandSocket(signature string) string {
	if runtime := os.Getenv("XDG_RUNTIME_DIR"); runtime != "" {
		return filepath.Join(runtime, "hypr", signature, ".socket.sock")
	}
	return filepath.Join("/tmp/hypr", signature, ".socket.sock")
}
