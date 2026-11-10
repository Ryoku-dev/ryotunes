// Package conn provides the reconnecting Ryotunes daemon socket client.
package conn

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

const reconnectDelay = 2 * time.Second

// Client is a concurrent NDJSON client that reconnects and restores its subscription.
type Client struct {
	path string

	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
	wake   chan struct{}

	mu      sync.Mutex
	conn    net.Conn
	enc     *protocol.Encoder
	pending map[uint64]chan protocol.Response
	writeMu sync.Mutex
	eventMu sync.Mutex
	nextID  atomic.Uint64

	events    chan protocol.Event
	snapshots chan protocol.Snapshot
	replaying bool
	early     []protocol.Event
}

// New starts a client for protocol.SocketPath and returns immediately.
func New() *Client {
	return NewPath(protocol.SocketPath())
}

// NewPath starts a client for a caller-selected Unix socket and returns immediately.
func NewPath(path string) *Client {
	ctx, cancel := context.WithCancel(context.Background())
	c := &Client{
		path:      path,
		ctx:       ctx,
		cancel:    cancel,
		done:      make(chan struct{}),
		wake:      make(chan struct{}, 1),
		pending:   make(map[uint64]chan protocol.Response),
		events:    make(chan protocol.Event, 128),
		snapshots: make(chan protocol.Snapshot, 1),
	}
	go c.run()
	return c
}

// Events returns daemon events in wire order.
func (c *Client) Events() <-chan protocol.Event { return c.events }

// Snapshots returns the latest full snapshot from successful subscription replay.
func (c *Client) Snapshots() <-chan protocol.Snapshot { return c.snapshots }

// Connected reports whether a live Unix socket is currently installed.
func (c *Client) Connected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn != nil
}

// Call performs one correlated RPC and decodes its result.
func (c *Client) Call(ctx context.Context, method string, params any, result any) error {
	var payload json.RawMessage
	switch params.(type) {
	case nil, protocol.Null, *protocol.Null:
		payload = json.RawMessage("null")
	default:
		encoded, err := json.Marshal(params)
		if err != nil {
			return err
		}
		payload = encoded
	}
	id := c.nextID.Add(1)
	response := make(chan protocol.Response, 1)

	c.mu.Lock()
	conn, enc := c.conn, c.enc
	if conn == nil || enc == nil {
		c.mu.Unlock()
		return disconnected("ryotunesd is not connected")
	}
	c.pending[id] = response
	c.mu.Unlock()

	request := protocol.Request{ID: id, Method: method, Params: payload}
	c.writeMu.Lock()
	err := enc.Encode(request)
	c.writeMu.Unlock()
	if err != nil {
		c.removePending(id)
		c.lose(conn)
		return disconnected("connection lost")
	}

	select {
	case reply := <-response:
		if reply.Error != nil {
			return reply.Error
		}
		if result == nil || len(reply.Result) == 0 || string(reply.Result) == "null" {
			return nil
		}
		return json.Unmarshal(reply.Result, result)
	case <-ctx.Done():
		c.removePending(id)
		return ctx.Err()
	case <-c.ctx.Done():
		c.removePending(id)
		return disconnected("connection lost")
	}
}

// Do performs one typed RPC through c.
func Do[P, R any](ctx context.Context, c *Client, method string, params P) (R, error) {
	var result R
	err := c.Call(ctx, method, params, &result)
	return result, err
}

// Show asks the connected daemon to raise the subscribed client window.
func (c *Client) Show(ctx context.Context) error {
	return c.Call(ctx, protocol.MethodShow, protocol.Null{}, nil)
}

// Close stops reconnecting, rejects pending calls, and closes output channels.
func (c *Client) Close() {
	c.cancel()
	c.mu.Lock()
	if c.conn != nil {
		_ = c.conn.Close()
	}
	c.mu.Unlock()
	<-c.done
}

// Launch activates the user socket unit, falling back to a detached ryotunesd process.
func Launch() error {
	return LaunchContext(context.Background())
}

// LaunchContext is the cancellable form of Launch.
func LaunchContext(ctx context.Context) error {
	dialCtx, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
	conn, err := (&net.Dialer{}).DialContext(dialCtx, "unix", protocol.SocketPath())
	cancel()
	if err == nil {
		_ = conn.Close()
		return nil
	}
	if err := exec.CommandContext(ctx, "systemctl", "--user", "start", "ryotunesd.socket").Run(); err == nil {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	cmd := exec.Command("ryotunesd")
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// Show makes a one-shot show request to an already reachable daemon.
func Show(ctx context.Context) error {
	conn, err := (&net.Dialer{}).DialContext(ctx, "unix", protocol.SocketPath())
	if err != nil {
		return err
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { _ = conn.SetDeadline(time.Now()) })
	defer stop()
	enc := protocol.NewEncoder(conn)
	if err := enc.Encode(protocol.Request{ID: 1, Method: protocol.MethodShow, Params: json.RawMessage("null")}); err != nil {
		return err
	}
	reader := bufio.NewReader(conn)
	for {
		line, err := protocol.ReadLine(reader)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return err
		}
		var response protocol.Response
		if json.Unmarshal(line, &response) != nil || response.ID != 1 {
			continue
		}
		if response.Error != nil {
			return response.Error
		}
		return nil
	}
}

func (c *Client) run() {
	defer close(c.done)
	defer close(c.events)
	defer close(c.snapshots)
	for {
		if c.ctx.Err() != nil {
			c.rejectAll("connection lost")
			return
		}
		conn, err := (&net.Dialer{}).DialContext(c.ctx, "unix", c.path)
		if err != nil {
			if !c.waitRetry() {
				return
			}
			continue
		}
		c.install(conn)
		go c.readLoop(conn)

		handshake, cancel := context.WithTimeout(c.ctx, 10*time.Second)
		var hello protocol.Hello
		err = c.Call(handshake, protocol.MethodHello, protocol.Null{}, &hello)
		if err == nil {
			err = c.subscribe(handshake)
		}
		cancel()
		if err != nil {
			c.lose(conn)
		}

		select {
		case <-c.ctx.Done():
			c.lose(conn)
			return
		case <-c.wake:
		}
		if !c.waitRetry() {
			return
		}
	}
}

func (c *Client) readLoop(conn net.Conn) {
	reader := bufio.NewReaderSize(conn, 64*1024)
	for {
		line, err := protocol.ReadLine(reader)
		if err != nil {
			c.lose(conn)
			return
		}
		var envelope struct {
			ID    *uint64         `json:"id"`
			Event string          `json:"event"`
			Data  json.RawMessage `json:"data"`
		}
		if json.Unmarshal(line, &envelope) != nil {
			continue
		}
		if envelope.ID != nil {
			var response protocol.Response
			if json.Unmarshal(line, &response) != nil {
				continue
			}
			c.deliver(response)
			continue
		}
		if envelope.Event != "" {
			event := protocol.Event{Event: envelope.Event, Data: envelope.Data}
			c.eventMu.Lock()
			if !c.isCurrent(conn) {
				c.eventMu.Unlock()
				continue
			}
			if c.replaying {
				c.early = append(c.early, event)
				c.eventMu.Unlock()
				continue
			}
			select {
			case c.events <- event:
				c.eventMu.Unlock()
			case <-c.ctx.Done():
				c.eventMu.Unlock()
				return
			}
		}
	}
}

// subscribe replays the daemon snapshot, then the events that arrived before it.
func (c *Client) subscribe(ctx context.Context) error {
	// the daemon streams events before it answers, so the read loop must never wait on eventMu here
	c.eventMu.Lock()
	c.replaying, c.early = true, nil
	c.eventMu.Unlock()
	var snapshot protocol.Snapshot
	err := c.Call(ctx, protocol.MethodSubscribe, protocol.SubscribeParams{Events: []string{"*"}}, &snapshot)
	c.eventMu.Lock()
	defer c.eventMu.Unlock()
	early := c.early
	c.replaying, c.early = false, nil
	if err != nil {
		return err
	}
	c.publishSnapshot(snapshot)
	for _, event := range early {
		select {
		case c.events <- event:
		case <-c.ctx.Done():
			return c.ctx.Err()
		}
	}
	return nil
}

func (c *Client) publishSnapshot(snapshot protocol.Snapshot) {
	select {
	case c.snapshots <- snapshot:
		return
	default:
	}
	select {
	case <-c.snapshots:
	default:
	}
	select {
	case c.snapshots <- snapshot:
	case <-c.ctx.Done():
	}
}

func (c *Client) isCurrent(conn net.Conn) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn == conn
}

func (c *Client) install(conn net.Conn) {
	c.mu.Lock()
	c.conn = conn
	c.enc = protocol.NewEncoder(conn)
	c.mu.Unlock()
}

func (c *Client) lose(conn net.Conn) {
	c.mu.Lock()
	if c.conn != conn {
		c.mu.Unlock()
		return
	}
	c.conn = nil
	c.enc = nil
	pending := c.pending
	c.pending = make(map[uint64]chan protocol.Response)
	_ = conn.Close()
	c.mu.Unlock()

	failure := protocol.Response{Error: disconnected("connection lost")}
	for _, waiter := range pending {
		waiter <- failure
	}
	select {
	case c.wake <- struct{}{}:
	default:
	}
}

func (c *Client) rejectAll(message string) {
	c.mu.Lock()
	pending := c.pending
	c.pending = make(map[uint64]chan protocol.Response)
	c.mu.Unlock()
	failure := protocol.Response{Error: disconnected(message)}
	for _, waiter := range pending {
		waiter <- failure
	}
}

func (c *Client) deliver(response protocol.Response) {
	c.mu.Lock()
	waiter := c.pending[response.ID]
	if waiter != nil {
		delete(c.pending, response.ID)
	}
	c.mu.Unlock()
	if waiter != nil {
		waiter <- response
	}
}

func (c *Client) removePending(id uint64) {
	c.mu.Lock()
	delete(c.pending, id)
	c.mu.Unlock()
}

func (c *Client) waitRetry() bool {
	timer := time.NewTimer(reconnectDelay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-c.ctx.Done():
		return false
	}
}

func disconnected(message string) *protocol.Error {
	return &protocol.Error{Code: protocol.ErrorDisconnected, Message: message}
}
