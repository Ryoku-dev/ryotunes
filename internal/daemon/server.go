package daemon

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

var umaskMu sync.Mutex

// Server owns the daemon's Unix listener and subscribed client sinks.
type Server struct {
	app       *App
	listener  net.Listener
	path      string
	activated bool

	mu       sync.Mutex
	conns    map[*serverConn]struct{}
	clients  int
	closed   bool
	closeErr error
	wg       sync.WaitGroup
}

type serverConn struct {
	server *Server
	conn   net.Conn
	enc    *protocol.Encoder

	writeMu    sync.Mutex
	closeOnce  sync.Once
	subscribed bool
	sinkHandle int
}

// NewServer adopts systemd fd 3 when supplied, otherwise binds socketPath.
func NewServer(app *App, socketPath string) (*Server, error) {
	if app == nil {
		return nil, errors.New("daemon server requires an app")
	}
	if socketPath == "" {
		socketPath = protocol.SocketPath()
	}
	listener, activated, err := daemonListener(socketPath)
	if err != nil {
		return nil, err
	}
	if unix, ok := listener.(*net.UnixListener); ok {
		unix.SetUnlinkOnClose(false)
	}
	return &Server{app: app, listener: listener, path: socketPath, activated: activated, conns: make(map[*serverConn]struct{})}, nil
}

func daemonListener(socketPath string) (net.Listener, bool, error) {
	if listener, ok, err := activatedListener(); ok || err != nil {
		return listener, ok, err
	}
	if err := secureSocketParent(socketPath); err != nil {
		return nil, false, err
	}
	if info, err := os.Lstat(socketPath); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return nil, false, fmt.Errorf("socket path exists and is not a socket: %s", socketPath)
		}
		if err := os.Remove(socketPath); err != nil {
			return nil, false, fmt.Errorf("remove stale socket: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return nil, false, fmt.Errorf("inspect socket path: %w", err)
	}

	umaskMu.Lock()
	oldMask := syscall.Umask(0o077)
	listener, err := net.Listen("unix", socketPath)
	syscall.Umask(oldMask)
	umaskMu.Unlock()
	if err != nil {
		return nil, false, fmt.Errorf("bind daemon socket: %w", err)
	}
	if err := os.Chmod(socketPath, 0o600); err != nil {
		listener.Close()
		_ = os.Remove(socketPath)
		return nil, false, fmt.Errorf("secure daemon socket: %w", err)
	}
	return listener, false, nil
}

func secureSocketParent(socketPath string) error {
	parent := filepath.Dir(socketPath)
	info, err := os.Lstat(parent)
	switch {
	case os.IsNotExist(err):
		if err := os.MkdirAll(parent, 0o700); err != nil {
			return fmt.Errorf("create socket directory: %w", err)
		}
		if err := os.Chmod(parent, 0o700); err != nil {
			return fmt.Errorf("secure socket directory: %w", err)
		}
		return nil
	case err != nil:
		return fmt.Errorf("inspect socket directory: %w", err)
	case !info.IsDir() || info.Mode()&os.ModeSymlink != 0:
		return fmt.Errorf("socket parent is not a directory: %s", parent)
	case filepath.Clean(parent) == filepath.Clean(protocol.RuntimeDir()):
		if err := os.Chmod(parent, 0o700); err != nil {
			return fmt.Errorf("secure socket directory: %w", err)
		}
	}
	return nil
}

func activatedListener() (net.Listener, bool, error) {
	pid, err := strconv.Atoi(os.Getenv("LISTEN_PID"))
	if err != nil || pid != os.Getpid() {
		return nil, false, nil
	}
	fds, err := strconv.Atoi(os.Getenv("LISTEN_FDS"))
	if err != nil || fds < 1 {
		return nil, false, nil
	}
	file := os.NewFile(uintptr(3), "ryotunesd.socket")
	if file == nil {
		return nil, true, errors.New("adopt systemd socket: fd 3 is unavailable")
	}
	listener, listenErr := net.FileListener(file)
	closeErr := file.Close()
	if listenErr != nil {
		return nil, true, fmt.Errorf("adopt systemd socket: %w", listenErr)
	}
	if closeErr != nil {
		listener.Close()
		return nil, true, fmt.Errorf("close inherited socket descriptor: %w", closeErr)
	}
	if _, ok := listener.(*net.UnixListener); !ok {
		listener.Close()
		return nil, true, errors.New("adopt systemd socket: fd 3 is not a Unix listener")
	}
	return listener, true, nil
}

// Activated reports whether the listener came from systemd socket activation.
func (s *Server) Activated() bool { return s != nil && s.activated }

// Serve accepts clients until ctx is cancelled or Close is called.
func (s *Server) Serve(ctx context.Context) error {
	serveDone := make(chan struct{})
	defer close(serveDone)
	go func() {
		select {
		case <-ctx.Done():
			_ = s.Close()
		case <-serveDone:
		}
	}()
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			if ctx.Err() != nil || s.isClosed() || errors.Is(err, net.ErrClosed) {
				return nil
			}
			if temporary, ok := err.(interface{ Temporary() bool }); ok && temporary.Temporary() {
				continue
			}
			return err
		}
		client := &serverConn{server: s, conn: conn, enc: protocol.NewEncoder(conn)}
		s.mu.Lock()
		if s.closed {
			s.mu.Unlock()
			_ = conn.Close()
			return nil
		}
		s.conns[client] = struct{}{}
		s.wg.Add(1)
		s.mu.Unlock()
		go func() {
			defer s.wg.Done()
			client.serve()
		}()
	}
}

func (c *serverConn) serve() {
	defer c.close()
	reader := bufio.NewReaderSize(c.conn, 4096)
	for {
		line, err := protocol.ReadLine(reader)
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
				return
			}
			c.server.app.Log.Warn("invalid client request", "target", "rpc:framing", "error", err)
			c.writeResponse(protocol.Response{ID: 0, Error: &protocol.Error{Code: protocol.ErrorBadRequest, Message: err.Error()}})
			continue
		}
		request, err := decodeWireRequest(line)
		if err != nil {
			c.server.app.Log.Warn("invalid client request", "target", "rpc:framing", "error", err)
			c.writeResponse(protocol.Response{ID: 0, Error: &protocol.Error{Code: protocol.ErrorBadRequest, Message: err.Error()}})
			continue
		}
		c.server.wg.Add(1)
		go func() {
			defer c.server.wg.Done()
			c.dispatch(request)
		}()
	}
}

func decodeWireRequest(line []byte) (protocol.Request, error) {
	var envelope struct {
		ID     *uint64         `json:"id"`
		Method *string         `json:"method"`
		Params json.RawMessage `json:"params"`
	}
	if err := json.Unmarshal(line, &envelope); err != nil {
		return protocol.Request{}, err
	}
	if envelope.ID == nil {
		return protocol.Request{}, errors.New("missing field \"id\"")
	}
	if envelope.Method == nil {
		return protocol.Request{}, errors.New("missing field \"method\"")
	}
	params := envelope.Params
	if params == nil {
		params = json.RawMessage("null")
	}
	return protocol.Request{ID: *envelope.ID, Method: *envelope.Method, Params: params}, nil
}

func (c *serverConn) dispatch(request protocol.Request) {
	if request.Method == protocol.MethodSubscribe {
		if _, err := decode[protocol.SubscribeParams](request.Params); err != nil {
			c.writeResponse(protocol.Response{ID: request.ID, Error: asProtocolError(err)})
			return
		}
		c.subscribe()
		c.writeResult(request.ID, c.server.app.Snapshot())
		return
	}
	result, rpcErr := c.server.app.Dispatch(c.server.app.ctx, request.Method, request.Params)
	if rpcErr != nil {
		c.writeResponse(protocol.Response{ID: request.ID, Error: rpcErr})
		return
	}
	c.writeResult(request.ID, result)
	if request.Method == protocol.MethodQuit {
		c.server.app.Quit()
	}
}

func (c *serverConn) subscribe() {
	c.server.mu.Lock()
	if c.subscribed || c.server.closed {
		c.server.mu.Unlock()
		return
	}
	c.subscribed = true
	c.sinkHandle = c.server.app.Bus.Attach(c)
	c.server.clients++
	c.server.mu.Unlock()
	c.server.app.Player.SetPositionCadence(250 * time.Millisecond)
}

// Emit implements bus.Sink for one subscribed connection.
func (c *serverConn) Emit(event string, data any) {
	raw, err := json.Marshal(data)
	if err != nil {
		c.server.app.Log.Warn("encoding daemon event", "event", event, "error", err)
		return
	}
	err = c.encode(protocol.Event{Event: event, Data: raw})
	if err != nil {
		c.close()
	}
}

func (c *serverConn) writeResult(id uint64, result any) {
	raw := json.RawMessage("null")
	var err error
	switch result.(type) {
	case nil, protocol.Null, *protocol.Null:
	default:
		raw, err = json.Marshal(result)
	}
	if err != nil {
		c.writeResponse(protocol.Response{ID: id, Error: &protocol.Error{Code: protocol.ErrorFailed, Message: err.Error()}})
		return
	}
	c.writeResponse(protocol.Response{ID: id, Result: raw})
}

func (c *serverConn) writeResponse(response protocol.Response) {
	if err := c.encode(response); err != nil {
		c.close()
	}
}

func (c *serverConn) encode(value any) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if err := c.conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return err
	}
	err := c.enc.Encode(value)
	_ = c.conn.SetWriteDeadline(time.Time{})
	return err
}

func (c *serverConn) close() {
	c.closeOnce.Do(func() {
		_ = c.conn.Close()
		c.server.mu.Lock()
		delete(c.server.conns, c)
		noSubscribers := false
		if c.subscribed {
			c.server.app.Bus.Detach(c.sinkHandle)
			c.server.clients--
			c.subscribed = false
			noSubscribers = c.server.clients == 0
		}
		c.server.mu.Unlock()
		if noSubscribers {
			c.server.app.Player.SetPositionCadence(time.Second)
		}
	})
}

// Clients returns the number of live subscribed connections.
func (s *Server) Clients() int {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.clients
}

// Show raises subscribed clients or starts a detached Ryotunes client.
func (s *Server) Show() error {
	if s.Clients() > 0 {
		s.app.Bus.Emit(protocol.EventShow, nil)
		if s.Clients() > 0 {
			return nil
		}
	}
	devnull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer devnull.Close()
	cmd := exec.Command("ryotunes")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = devnull, devnull, devnull
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start Ryotunes: %w", err)
	}
	return cmd.Process.Release()
}

// Wait blocks until every accepted connection and in-flight dispatch has returned.
func (s *Server) Wait() {
	if s != nil {
		s.wg.Wait()
	}
}

func (s *Server) isClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

// Close stops admission, closes clients, and removes only a self-created socket.
func (s *Server) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	if s.closed {
		err := s.closeErr
		s.mu.Unlock()
		return err
	}
	s.closed = true
	connections := make([]*serverConn, 0, len(s.conns))
	for conn := range s.conns {
		connections = append(connections, conn)
	}
	s.mu.Unlock()

	err := s.listener.Close()
	for _, conn := range connections {
		conn.close()
	}
	if !s.activated {
		if removeErr := os.Remove(s.path); removeErr != nil && !os.IsNotExist(removeErr) && err == nil {
			err = removeErr
		}
	}
	s.mu.Lock()
	s.closeErr = err
	s.mu.Unlock()
	return err
}
