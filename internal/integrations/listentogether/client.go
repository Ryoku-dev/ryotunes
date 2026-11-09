package listentogether

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/ryoku-dev/ryotunes/internal/bus"
	"github.com/ryoku-dev/ryotunes/internal/ltproto"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

const (
	pingInterval         = 25 * time.Second
	maxReconnectAttempts = 15
)

// Options configures a Listen Together client.
type Options struct {
	ServerURL string
	Host      Host
	Sink      bus.Sink
	Log       *slog.Logger
}

// Client owns one daemon-side Listen Together session.
type Client struct {
	mu         sync.RWMutex
	state      model.LTState
	session    string
	outbound   chan ltproto.ClientMessage
	cancel     context.CancelFunc
	generation uint64
	host       Host
	sink       bus.Sink
	log        *slog.Logger
	dialer     *websocket.Dialer
}

// New creates an idle Listen Together client.
func New(options Options) *Client {
	sink := options.Sink
	if sink == nil {
		sink = bus.Discard
	}
	log := options.Log
	if log == nil {
		log = slog.Default()
	}
	serverURL := DefaultServerURL
	if options.ServerURL != "" {
		normalized, err := ValidateServerURL(options.ServerURL)
		if err == nil {
			serverURL = normalized
		} else {
			log.Warn("Ignoring invalid persisted Listen Together server URL", "error", err)
		}
	}
	return &Client{
		state: model.LTState{
			Status:       "disconnected",
			Role:         "none",
			ServerURL:    serverURL,
			Users:        []model.LTUser{},
			Queue:        []model.LTTrack{},
			PendingJoins: []model.LTPendingJoin{},
			Suggestions:  []model.LTSuggestion{},
		},
		host:   options.Host,
		sink:   sink,
		log:    log,
		dialer: websocket.DefaultDialer,
	}
}

// State returns an isolated snapshot of client-visible Listen Together state.
func (c *Client) State() model.LTState {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return cloneState(c.state)
}

// SetServerURL validates and stores a relay URL.
func (c *Client) SetServerURL(raw string) error {
	normalized, err := ValidateServerURL(raw)
	if err != nil {
		return err
	}
	c.mu.Lock()
	c.state.ServerURL = normalized
	c.mu.Unlock()
	c.emitState()
	return nil
}

// CreateRoom starts a new room request.
func (c *Client) CreateRoom(username string) {
	if username == "" && c.host != nil {
		username = c.host.Username()
	}
	c.start(&ltproto.CreateRoom{Username: username})
}

// JoinRoom requests admission to an existing room.
func (c *Client) JoinRoom(code, username string) {
	if username == "" && c.host != nil {
		username = c.host.Username()
	}
	c.start(&ltproto.JoinRoom{RoomCode: ltproto.NormalizeRoomCode(code), Username: username})
}

// ApproveJoin admits a pending participant.
func (c *Client) ApproveJoin(userID string) {
	c.send(&ltproto.ApproveJoin{UserID: userID})
}

// RejectJoin rejects a pending participant.
func (c *Client) RejectJoin(userID string) {
	c.mu.Lock()
	pending := c.state.PendingJoins[:0]
	for _, join := range c.state.PendingJoins {
		if join.UserID != userID {
			pending = append(pending, join)
		}
	}
	c.state.PendingJoins = pending
	c.mu.Unlock()
	c.send(&ltproto.RejectJoin{UserID: userID})
	c.emitState()
}

// Kick removes a participant from the room.
func (c *Client) Kick(userID string) {
	c.send(&ltproto.KickUser{UserID: userID})
}

// TransferHost hands room authority to another participant.
func (c *Client) TransferHost(userID string) {
	c.send(&ltproto.TransferHost{UserID: userID})
}

// Suggest submits a guest track to the host.
func (c *Client) Suggest(track model.Track) {
	c.send(&ltproto.Suggest{Track: roomTrack(track)})
}

// ApproveSuggestion accepts a pending suggestion and enqueues its track locally.
func (c *Client) ApproveSuggestion(id string) {
	var suggestion *model.LTSuggestion
	c.mu.RLock()
	for index := range c.state.Suggestions {
		item := c.state.Suggestions[index]
		if item.ID == id {
			copy := item
			suggestion = &copy
			break
		}
	}
	c.mu.RUnlock()
	if suggestion != nil && c.enqueueSuggestion(context.Background(), *suggestion) {
		c.removeSuggestion(id)
		c.send(&ltproto.ApproveSuggestion{ID: id})
		c.broadcastQueue()
	}
	c.emitState()
}

// RejectSuggestion rejects a pending suggestion.
func (c *Client) RejectSuggestion(id string) {
	c.mu.Lock()
	remaining := c.state.Suggestions[:0]
	for _, suggestion := range c.state.Suggestions {
		if suggestion.ID != id {
			remaining = append(remaining, suggestion)
		}
	}
	c.state.Suggestions = remaining
	c.mu.Unlock()
	c.send(&ltproto.RejectSuggestion{ID: id})
	c.emitState()
}

// RequestSync asks the relay for its authoritative room state.
func (c *Client) RequestSync() {
	c.send(&ltproto.RequestSync{})
}

// BroadcastPlayback sends a local host playback action to all guests.
func (c *Client) BroadcastPlayback(playback ltproto.Playback) {
	c.mu.RLock()
	isHost := c.state.Role == "host"
	c.mu.RUnlock()
	if isHost {
		c.send(&ltproto.PlaybackMessage{Playback: playback})
	}
}

// BroadcastSnapshot publishes the host's complete current playback and queue.
func (c *Client) BroadcastSnapshot() {
	if c.host == nil {
		return
	}
	snapshot := c.host.PlaybackSnapshot()
	queue := roomTracks(snapshot.Queue)
	playback := ltproto.Playback{
		Kind:       ltproto.PlaybackChangeTrack,
		PositionMS: snapshot.PositionMS,
		Playing:    snapshot.Playing,
		Queue:      &queue,
		Volume:     snapshot.Volume,
	}
	if snapshot.Track != nil {
		track := roomTrack(*snapshot.Track)
		playback.Track = &track
	}
	c.BroadcastPlayback(playback)
}

// Leave leaves the current room and stops reconnection.
func (c *Client) Leave() {
	c.send(&ltproto.LeaveRoom{})
	c.stopSession()
}

func (c *Client) broadcastQueue() {
	if c.host == nil {
		return
	}
	queue := roomTracks(c.host.PlaybackSnapshot().Queue)
	c.BroadcastPlayback(ltproto.Playback{Kind: ltproto.PlaybackSyncQueue, Queue: &queue})
}

// Close releases the client and its active connection.
func (c *Client) Close() {
	c.Leave()
}

func (c *Client) start(initial ltproto.ClientMessage) {
	c.mu.Lock()
	if c.cancel != nil {
		c.cancel()
	}
	c.generation++
	generation := c.generation
	ctx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel
	c.resetRoomLocked()
	c.state.Status = "connecting"
	c.state.Requesting = true
	c.mu.Unlock()
	c.emitState()
	go c.run(ctx, generation, initial)
}

func (c *Client) run(ctx context.Context, generation uint64, initial ltproto.ClientMessage) {
	next := initial
	attempt := 0
	for {
		if ctx.Err() != nil || !c.isGeneration(generation) {
			return
		}
		c.mu.RLock()
		serverURL := c.state.ServerURL
		c.mu.RUnlock()
		if serverURL == "" {
			c.closeLocally(generation, "Set a server URL first (Listen Together settings).")
			return
		}
		c.setConnecting(generation)
		connection, response, err := c.dialer.DialContext(ctx, serverURL, http.Header{})
		if err != nil {
			if response != nil && response.Body != nil {
				response.Body.Close()
			}
			c.log.Warn("Listen Together connection failed", "error", err)
		} else {
			attempt = 0
			connection.SetReadLimit(ltproto.MaxMessageBytes)
			outbound := make(chan ltproto.ClientMessage, ltproto.OutboundQueueCapacity)
			c.mu.Lock()
			if c.generation != generation {
				c.mu.Unlock()
				connection.Close()
				return
			}
			c.outbound = outbound
			c.state.Status = "connected"
			c.mu.Unlock()
			c.emitState()
			fatal := c.serveConnection(ctx, generation, connection, outbound, next)
			connection.Close()
			c.mu.Lock()
			if c.generation == generation && c.outbound == outbound {
				c.outbound = nil
			}
			c.mu.Unlock()
			if fatal {
				return
			}
		}
		if ctx.Err() != nil || !c.isGeneration(generation) {
			return
		}
		c.mu.RLock()
		token := c.session
		c.mu.RUnlock()
		if token == "" {
			c.closeLocally(generation, "Couldn't reach the Listen Together server.")
			return
		}
		attempt++
		if attempt > maxReconnectAttempts {
			c.closeLocally(generation, "Lost connection to the room.")
			return
		}
		c.setConnecting(generation)
		next = &ltproto.Reconnect{SessionToken: token}
		timer := time.NewTimer(reconnectDelay(attempt))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (c *Client) serveConnection(ctx context.Context, generation uint64, connection *websocket.Conn, outbound <-chan ltproto.ClientMessage, initial ltproto.ClientMessage) bool {
	incoming := make(chan ltproto.ServerMessage)
	failed := make(chan error, 1)
	go func() {
		defer close(incoming)
		for {
			kind, data, err := connection.ReadMessage()
			if err != nil {
				failed <- err
				return
			}
			if kind != websocket.TextMessage {
				continue
			}
			message, err := ltproto.DecodeServer(data)
			if err != nil {
				c.log.Debug("Ignoring invalid Listen Together server message", "error", err)
				continue
			}
			select {
			case incoming <- message:
			case <-ctx.Done():
				return
			}
		}
	}()
	if err := c.writeMessage(connection, initial); err != nil {
		return false
	}
	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return false
		case <-failed:
			return false
		case message, ok := <-incoming:
			if !ok {
				return false
			}
			if c.handle(ctx, generation, message) {
				return true
			}
		case message := <-outbound:
			if err := c.writeMessage(connection, message); err != nil {
				return false
			}
		case <-ticker.C:
			if err := c.writeMessage(connection, &ltproto.Ping{}); err != nil {
				return false
			}
		}
	}
}

func (c *Client) writeMessage(connection *websocket.Conn, message ltproto.ClientMessage) error {
	data, err := ltproto.EncodeClient(message)
	if err != nil {
		return err
	}
	connection.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return connection.WriteMessage(websocket.TextMessage, data)
}

func (c *Client) send(message ltproto.ClientMessage) {
	c.mu.RLock()
	outbound := c.outbound
	c.mu.RUnlock()
	if outbound == nil {
		return
	}
	select {
	case outbound <- message:
	default:
		c.log.Warn("Listen Together outbound queue is full")
	}
}

func (c *Client) isGeneration(generation uint64) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.generation == generation
}

func (c *Client) setConnecting(generation uint64) {
	c.mu.Lock()
	if c.generation == generation {
		c.state.Status = "connecting"
	}
	c.mu.Unlock()
	c.emitState()
}

func (c *Client) stopSession() {
	c.mu.Lock()
	if c.cancel != nil {
		c.cancel()
		c.cancel = nil
	}
	c.generation++
	c.resetRoomLocked()
	c.mu.Unlock()
	c.emitState()
}

func (c *Client) closeLocally(generation uint64, notice string) {
	c.mu.Lock()
	if c.generation != generation {
		c.mu.Unlock()
		return
	}
	if c.cancel != nil {
		c.cancel()
		c.cancel = nil
	}
	c.generation++
	c.resetRoomLocked()
	c.mu.Unlock()
	if notice != "" {
		c.emitNotice(notice)
	}
	c.emitState()
}

func (c *Client) resetRoomLocked() {
	c.state.Status = "disconnected"
	c.state.Role = "none"
	c.state.Requesting = false
	c.state.RoomCode = nil
	c.state.MyID = nil
	c.state.Users = []model.LTUser{}
	c.state.CurrentTrack = nil
	c.state.Queue = []model.LTTrack{}
	c.state.PendingJoins = []model.LTPendingJoin{}
	c.state.Suggestions = []model.LTSuggestion{}
	c.session = ""
	c.outbound = nil
}

func (c *Client) absorbLocked(state ltproto.RoomState) {
	roomCode := state.RoomCode
	c.state.RoomCode = &roomCode
	c.state.Users = append([]model.LTUser(nil), state.Users...)
	c.state.CurrentTrack = state.CurrentTrack
	c.state.Queue = append([]model.LTTrack(nil), state.Queue...)
}

func (c *Client) emitState() {
	c.sink.Emit(protocol.EventLTState, c.State())
}

func (c *Client) emitNotice(message string) {
	c.sink.Emit(protocol.EventLTNotice, model.LTNotice(message))
}

func reconnectDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if attempt > 6 {
		attempt = 6
	}
	return time.Second << (attempt - 1)
}

func (c *Client) logHostError(operation string, err error) {
	if err != nil && !errors.Is(err, context.Canceled) {
		c.log.Warn("Listen Together host action failed", "operation", operation, "error", err)
	}
}
