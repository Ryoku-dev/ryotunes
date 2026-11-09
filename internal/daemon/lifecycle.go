package daemon

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/gofrs/flock"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

const defaultIdleExit = 5 * time.Minute

// Run owns singleton admission, socket serving, idle exit, signals, and joined teardown.
// An optional socketPath overrides the normal per-user socket for development and tests.
func Run(ctx context.Context, socketPath ...string) error {
	path := protocol.SocketPath()
	if len(socketPath) != 0 && socketPath[0] != "" {
		path = socketPath[0]
	}
	if err := secureRuntimeParent(path); err != nil {
		return err
	}
	lock := flock.New(path+".lock", flock.SetPermissions(0o600))
	locked, err := lock.TryLock()
	if err != nil {
		return fmt.Errorf("lock daemon instance: %w", err)
	}
	if !locked {
		return forwardShow(path)
	}
	defer lock.Close()

	runCtx, stopSignals := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	app, err := New(runCtx, slog.Default())
	if err != nil {
		return err
	}
	server, err := NewServer(app, path)
	if err != nil {
		closeCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return errors.Join(err, app.Close(closeCtx))
	}
	app.Server = server
	serveErr := make(chan error, 1)
	go func() { serveErr <- server.Serve(runCtx) }()

	idleDuration := idleExitDuration()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	idle := appIdle(app)
	var timer *time.Timer
	var timerC <-chan time.Time
	if idle {
		timer = time.NewTimer(idleDuration)
		timerC = timer.C
	}
	var runErr error
runLoop:
	for {
		select {
		case <-app.ctx.Done():
			break runLoop
		case err := <-serveErr:
			if err != nil && !errors.Is(err, net.ErrClosed) {
				runErr = err
			}
			break runLoop
		case <-timerC:
			if appIdle(app) {
				break runLoop
			}
			idle = false
			timerC = nil
		case <-ticker.C:
			nowIdle := appIdle(app)
			switch {
			case nowIdle && !idle:
				if timer == nil {
					timer = time.NewTimer(idleDuration)
				} else {
					timer.Reset(idleDuration)
				}
				timerC = timer.C
			case !nowIdle && idle:
				if timer != nil && !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timerC = nil
			}
			idle = nowIdle
		}
	}
	if timer != nil {
		timer.Stop()
	}
	_ = server.Close()
	closeCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return errors.Join(runErr, app.Close(closeCtx))
}

func secureRuntimeParent(socketPath string) error {
	return secureSocketParent(socketPath)
}

func forwardShow(socketPath string) error {
	var conn net.Conn
	var err error
	for range 20 {
		conn, err = net.DialTimeout("unix", socketPath, 250*time.Millisecond)
		if err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err != nil {
		return fmt.Errorf("another ryotunesd owns the lock but is not reachable at %s: %w", socketPath, err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
		return err
	}
	request := protocol.Request{ID: 1, Method: protocol.MethodShow, Params: json.RawMessage("null")}
	if err := protocol.NewEncoder(conn).Encode(request); err != nil {
		return fmt.Errorf("forward show request: %w", err)
	}
	line, err := protocol.ReadLine(bufio.NewReader(conn))
	if err != nil {
		return fmt.Errorf("read incumbent response: %w", err)
	}
	fmt.Fprintln(os.Stdout, string(line))
	return nil
}

func idleExitDuration() time.Duration {
	value := os.Getenv("RYOTUNESD_IDLE_EXIT_SECS")
	seconds, err := strconv.ParseInt(value, 10, 64)
	if err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	return defaultIdleExit
}

func appIdle(app *App) bool {
	if app.Server != nil && app.Server.Clients() != 0 {
		return false
	}
	if app.Queue != nil {
		playback := app.Queue.Playback()
		if playback.Now != nil && !playback.Paused {
			return false
		}
	}
	if app.Downloads != nil && app.Downloads.Active() {
		return false
	}
	if app.LT != nil {
		state := app.LT.State()
		if state.Requesting || state.Status != "disconnected" || state.Role != "none" || state.RoomCode != nil {
			return false
		}
	}
	return true
}
