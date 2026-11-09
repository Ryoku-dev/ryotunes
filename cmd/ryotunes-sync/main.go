// Command ryotunes-sync runs the optional Listen Together WebSocket relay.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode"

	"github.com/gorilla/websocket"
	"github.com/ryoku-dev/ryotunes/internal/ltproto"
)

const (
	writeTimeout    = 10 * time.Second
	cleanupInterval = 30 * time.Second
)

type sender chan ltproto.ServerMessage

func (s sender) send(message ltproto.ServerMessage) {
	select {
	case s <- message:
	default:
	}
}

type peer struct {
	username       string
	tx             sender
	connected      bool
	sessionToken   string
	disconnectedAt time.Time
}

type pending struct {
	username string
	tx       sender
}

type room struct {
	hostID       string
	peers        map[string]*peer
	pending      map[string]pending
	suggestions  map[string]ltproto.Suggestion
	currentTrack *ltproto.Track
	isPlaying    bool
	positionMS   int64
	lastUpdateMS int64
	volume       float64
	queue        []ltproto.Track
}

func (r *room) state(code string) ltproto.RoomState {
	ids := make([]string, 0, len(r.peers))
	for id := range r.peers {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	users := make([]ltproto.User, 0, len(ids))
	for _, id := range ids {
		member := r.peers[id]
		users = append(users, ltproto.User{
			UserID: id, Username: member.username, IsHost: id == r.hostID, IsConnected: member.connected,
		})
	}
	return ltproto.RoomState{
		RoomCode: code, HostID: r.hostID, Users: users, CurrentTrack: r.currentTrack,
		IsPlaying: r.isPlaying, PositionMS: r.positionMS, LastUpdateMS: r.lastUpdateMS,
		Volume: r.volume, Queue: append([]ltproto.Track(nil), r.queue...),
	}
}

func (r *room) broadcast(message ltproto.ServerMessage, except string) {
	for id, member := range r.peers {
		if member.connected && id != except {
			member.tx.send(message)
		}
	}
}

func (r *room) sendTo(userID string, message ltproto.ServerMessage) {
	if member := r.peers[userID]; member != nil && member.connected {
		member.tx.send(message)
	}
}

func (r *room) connectedOther(except string) string {
	ids := make([]string, 0, len(r.peers))
	for id, member := range r.peers {
		if id != except && member.connected {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return ""
	}
	sort.Strings(ids)
	return ids[0]
}

type relay struct {
	mu          sync.Mutex
	rooms       map[string]*room
	connections map[*websocket.Conn]struct{}
	log         *slog.Logger
	upgrader    websocket.Upgrader
}

func newRelay(log *slog.Logger) *relay {
	return &relay{
		rooms: make(map[string]*room), connections: make(map[*websocket.Conn]struct{}), log: log,
		upgrader: websocket.Upgrader{
			ReadBufferSize: 16 * 1024, WriteBufferSize: 16 * 1024,
			CheckOrigin: func(*http.Request) bool { return true },
		},
	}
}

type connectionState struct {
	userID   string
	roomCode string
}

func (s *relay) serveHTTP(writer http.ResponseWriter, request *http.Request) {
	connection, err := s.upgrader.Upgrade(writer, request, nil)
	if err != nil {
		s.log.Debug("WebSocket handshake failed", "error", err)
		return
	}
	connection.SetReadLimit(ltproto.MaxMessageBytes)
	s.mu.Lock()
	s.connections[connection] = struct{}{}
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.connections, connection)
		s.mu.Unlock()
		connection.Close()
	}()

	outbound := make(sender, ltproto.OutboundQueueCapacity)
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		for message := range outbound {
			data, err := ltproto.EncodeServer(message)
			if err != nil {
				continue
			}
			connection.SetWriteDeadline(time.Now().Add(writeTimeout))
			if err := connection.WriteMessage(websocket.TextMessage, data); err != nil {
				return
			}
		}
	}()

	identity := connectionState{}
	for {
		kind, data, err := connection.ReadMessage()
		if err != nil {
			break
		}
		if kind != websocket.TextMessage {
			continue
		}
		if len(data) > ltproto.MaxMessageBytes {
			break
		}
		message, err := ltproto.DecodeClient(data)
		if err != nil {
			s.log.Debug("Ignoring invalid client message", "error", err)
			continue
		}
		s.dispatch(message, outbound, &identity)
	}
	s.disconnect(identity)
	close(outbound)
	<-writerDone
}

func (s *relay) dispatch(message ltproto.ClientMessage, tx sender, identity *connectionState) {
	switch value := message.(type) {
	case *ltproto.Ping:
		tx.send(&ltproto.Pong{})
	case *ltproto.CreateRoom:
		if identity.userID == "" {
			s.createRoom(value.Username, tx, identity)
		}
	case *ltproto.JoinRoom:
		if identity.userID == "" {
			s.joinRoom(value.RoomCode, value.Username, tx, identity)
		}
	case *ltproto.ApproveJoin:
		s.approveJoin(value.UserID, tx, *identity)
	case *ltproto.RejectJoin:
		s.rejectJoin(value.UserID, *identity)
	case *ltproto.PlaybackMessage:
		s.playback(value.Playback, tx, *identity)
	case *ltproto.RequestSync:
		s.requestSync(tx, *identity)
	case *ltproto.Suggest:
		s.suggest(value.Track, tx, *identity)
	case *ltproto.ApproveSuggestion:
		s.approveSuggestion(value.ID, *identity)
	case *ltproto.RejectSuggestion:
		s.rejectSuggestion(value.ID, *identity)
	case *ltproto.KickUser:
		s.kick(value.UserID, *identity)
	case *ltproto.TransferHost:
		s.transferHost(value.UserID, *identity)
	case *ltproto.Reconnect:
		if identity.userID == "" {
			s.reconnect(value.SessionToken, tx, identity)
		}
	case *ltproto.LeaveRoom:
		s.mu.Lock()
		s.removeMemberLocked(identity.roomCode, identity.userID, true)
		s.mu.Unlock()
		identity.userID = ""
		identity.roomCode = ""
	}
}

func (s *relay) createRoom(username string, tx sender, identity *connectionState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.rooms) >= ltproto.MaxRooms {
		tx.send(relayError("server_full", "This relay has reached its room limit."))
		return
	}
	code := ""
	for code == "" || s.rooms[code] != nil {
		var err error
		code, err = ltproto.GenerateRoomCode()
		if err != nil {
			s.log.Error("Room code generation failed", "error", err)
			return
		}
	}
	userID := generateUserID()
	token := generateToken()
	created := &room{
		hostID: userID, peers: map[string]*peer{}, pending: map[string]pending{},
		suggestions: map[string]ltproto.Suggestion{}, volume: 1, queue: []ltproto.Track{},
	}
	created.peers[userID] = &peer{username: sanitizeUsername(username), tx: tx, connected: true, sessionToken: token}
	s.rooms[code] = created
	identity.userID = userID
	identity.roomCode = code
	tx.send(&ltproto.RoomCreated{RoomCode: code, UserID: userID, SessionToken: token, State: created.state(code)})
}

func (s *relay) joinRoom(rawCode, username string, tx sender, identity *connectionState) {
	code := ltproto.NormalizeRoomCode(rawCode)
	s.mu.Lock()
	defer s.mu.Unlock()
	joined := s.rooms[code]
	if joined == nil {
		tx.send(relayError("room_not_found", "No room with that code."))
		return
	}
	host := joined.peers[joined.hostID]
	if host == nil || !host.connected {
		tx.send(relayError("host_unavailable", "The host isn't in that room right now."))
		return
	}
	if len(joined.peers)+len(joined.pending) >= ltproto.MaxUsersPerRoom {
		tx.send(relayError("room_full", "That room is full."))
		return
	}
	userID := generateUserID()
	name := sanitizeUsername(username)
	joined.pending[userID] = pending{username: name, tx: tx}
	joined.sendTo(joined.hostID, &ltproto.JoinRequest{UserID: userID, Username: name})
	identity.userID = userID
	identity.roomCode = code
}

func (s *relay) approveJoin(joiner string, tx sender, identity connectionState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	joined := s.rooms[identity.roomCode]
	if joined == nil {
		return
	}
	if joined.hostID != identity.userID {
		tx.send(relayError("not_host", "Only the host can approve joins."))
		return
	}
	waiting, ok := joined.pending[joiner]
	if !ok {
		return
	}
	delete(joined.pending, joiner)
	token := generateToken()
	joined.peers[joiner] = &peer{username: waiting.username, tx: waiting.tx, connected: true, sessionToken: token}
	now := time.Now().UnixMilli()
	state := joined.state(identity.roomCode)
	state.PositionMS = ltproto.LivePositionMS(state, now)
	state.LastUpdateMS = now
	waiting.tx.send(&ltproto.JoinApproved{UserID: joiner, SessionToken: token, State: state})
	joined.broadcast(&ltproto.UserJoined{User: ltproto.User{UserID: joiner, Username: waiting.username, IsConnected: true}}, joiner)
}

func (s *relay) rejectJoin(joiner string, identity connectionState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	joined := s.rooms[identity.roomCode]
	if joined == nil || joined.hostID != identity.userID {
		return
	}
	if waiting, ok := joined.pending[joiner]; ok {
		delete(joined.pending, joiner)
		waiting.tx.send(&ltproto.JoinRejected{Reason: "The host declined your request."})
	}
}

func (s *relay) playback(playback ltproto.Playback, tx sender, identity connectionState) {
	if !validPlayback(playback) {
		tx.send(relayError("invalid_playback", "Playback payload exceeds relay limits."))
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	joined := s.rooms[identity.roomCode]
	if joined == nil {
		return
	}
	if joined.hostID != identity.userID {
		tx.send(relayError("not_host", "Only the host controls playback."))
		return
	}
	now := time.Now().UnixMilli()
	switch playback.Kind {
	case ltproto.PlaybackPlay:
		joined.isPlaying = true
		joined.positionMS = playback.PositionMS
		joined.lastUpdateMS = now
		playback.ServerTimeMS = now
	case ltproto.PlaybackPause:
		joined.isPlaying = false
		joined.positionMS = playback.PositionMS
		joined.lastUpdateMS = now
	case ltproto.PlaybackSeek:
		joined.positionMS = playback.PositionMS
		joined.lastUpdateMS = now
	case ltproto.PlaybackChangeTrack:
		joined.currentTrack = playback.Track
		joined.positionMS = playback.PositionMS
		joined.isPlaying = playback.Playing
		joined.lastUpdateMS = now
		if playback.Queue != nil {
			joined.queue = append([]ltproto.Track(nil), (*playback.Queue)...)
		}
	case ltproto.PlaybackSyncQueue:
		if playback.Queue != nil {
			joined.queue = append([]ltproto.Track(nil), (*playback.Queue)...)
		}
	case ltproto.PlaybackSetVolume:
		joined.volume = playback.Volume
	}
	joined.broadcast(&ltproto.SyncPlayback{Playback: playback}, identity.userID)
}

func (s *relay) requestSync(tx sender, identity connectionState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	joined := s.rooms[identity.roomCode]
	if joined == nil || joined.peers[identity.userID] == nil {
		return
	}
	now := time.Now().UnixMilli()
	state := joined.state(identity.roomCode)
	state.PositionMS = ltproto.LivePositionMS(state, now)
	state.LastUpdateMS = now
	tx.send(&ltproto.SyncState{State: state})
}

func (s *relay) suggest(track ltproto.Track, tx sender, identity connectionState) {
	if !validTrack(track) {
		tx.send(relayError("invalid_track", "Suggested track exceeds relay limits."))
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	joined := s.rooms[identity.roomCode]
	if joined == nil || joined.hostID == identity.userID {
		return
	}
	if len(joined.suggestions) >= ltproto.MaxSuggestionsPerRoom {
		tx.send(relayError("suggestions_full", "This room has too many pending suggestions."))
		return
	}
	member := joined.peers[identity.userID]
	if member == nil {
		return
	}
	suggestion := ltproto.Suggestion{ID: generateToken(), FromUserID: identity.userID, FromUsername: member.username, Track: track}
	joined.suggestions[suggestion.ID] = suggestion
	joined.sendTo(joined.hostID, &ltproto.SuggestionReceived{Suggestion: suggestion})
}

func (s *relay) approveSuggestion(id string, identity connectionState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	joined := s.rooms[identity.roomCode]
	if joined == nil || joined.hostID != identity.userID {
		return
	}
	if suggestion, ok := joined.suggestions[id]; ok {
		delete(joined.suggestions, id)
		joined.sendTo(suggestion.FromUserID, &ltproto.SuggestionApproved{ID: id})
	}
}

func (s *relay) rejectSuggestion(id string, identity connectionState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	joined := s.rooms[identity.roomCode]
	if joined == nil || joined.hostID != identity.userID {
		return
	}
	if suggestion, ok := joined.suggestions[id]; ok {
		delete(joined.suggestions, id)
		joined.sendTo(suggestion.FromUserID, &ltproto.SuggestionRejected{ID: id, Reason: "The host declined your suggestion."})
	}
}

func (s *relay) kick(target string, identity connectionState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	joined := s.rooms[identity.roomCode]
	if joined == nil || joined.hostID != identity.userID || target == identity.userID {
		return
	}
	if member := joined.peers[target]; member != nil {
		delete(joined.peers, target)
		member.tx.send(&ltproto.Kicked{Reason: "The host removed you from the room."})
		joined.broadcast(&ltproto.UserLeft{UserID: target}, "")
	}
}

func (s *relay) transferHost(target string, identity connectionState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	joined := s.rooms[identity.roomCode]
	if joined == nil || joined.hostID != identity.userID || joined.peers[target] == nil {
		return
	}
	joined.hostID = target
	joined.broadcast(&ltproto.HostChanged{HostID: target}, "")
}

func (s *relay) reconnect(token string, tx sender, identity *connectionState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for code, joined := range s.rooms {
		for userID, member := range joined.peers {
			if member.sessionToken != token {
				continue
			}
			member.tx = tx
			member.connected = true
			member.disconnectedAt = time.Time{}
			now := time.Now().UnixMilli()
			state := joined.state(code)
			state.PositionMS = ltproto.LivePositionMS(state, now)
			state.LastUpdateMS = now
			tx.send(&ltproto.Reconnected{UserID: userID, IsHost: joined.hostID == userID, State: state})
			joined.broadcast(&ltproto.UserReconnected{UserID: userID}, userID)
			identity.userID = userID
			identity.roomCode = code
			return
		}
	}
	tx.send(relayError("session_expired", "Your session has expired."))
}

func (s *relay) disconnect(identity connectionState) {
	if identity.userID == "" || identity.roomCode == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	joined := s.rooms[identity.roomCode]
	if joined == nil {
		return
	}
	if _, ok := joined.pending[identity.userID]; ok {
		delete(joined.pending, identity.userID)
		return
	}
	member := joined.peers[identity.userID]
	if member == nil {
		return
	}
	member.connected = false
	member.disconnectedAt = time.Now()
	joined.broadcast(&ltproto.UserDisconnected{UserID: identity.userID}, identity.userID)
	if joined.hostID == identity.userID {
		if next := joined.connectedOther(identity.userID); next != "" {
			joined.hostID = next
			joined.broadcast(&ltproto.HostChanged{HostID: next}, "")
		}
	}
}

func (s *relay) removeMemberLocked(code, userID string, graceful bool) {
	joined := s.rooms[code]
	if joined == nil {
		return
	}
	delete(joined.pending, userID)
	if _, ok := joined.peers[userID]; ok {
		delete(joined.peers, userID)
		if graceful {
			joined.broadcast(&ltproto.UserLeft{UserID: userID}, "")
		}
	}
	if joined.hostID == userID {
		if next := joined.connectedOther(userID); next != "" {
			joined.hostID = next
			joined.broadcast(&ltproto.HostChanged{HostID: next}, "")
		}
	}
	if len(joined.peers) == 0 {
		for _, waiting := range joined.pending {
			waiting.tx.send(&ltproto.JoinRejected{Reason: "The host closed the session."})
		}
		delete(s.rooms, code)
	}
}

func (s *relay) cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()
	cutoff := time.Now().Add(-time.Duration(ltproto.ReconnectGraceSeconds) * time.Second)
	for code, joined := range s.rooms {
		expired := make([]string, 0)
		for userID, member := range joined.peers {
			if !member.connected && !member.disconnectedAt.IsZero() && member.disconnectedAt.Before(cutoff) {
				expired = append(expired, userID)
			}
		}
		for _, userID := range expired {
			s.removeMemberLocked(code, userID, false)
		}
	}
}

func (s *relay) closeConnections() {
	s.mu.Lock()
	connections := make([]*websocket.Conn, 0, len(s.connections))
	for connection := range s.connections {
		connections = append(connections, connection)
	}
	s.mu.Unlock()
	for _, connection := range connections {
		connection.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseGoingAway, "server shutting down"), time.Now().Add(time.Second))
		connection.Close()
	}
}

func relayError(code, message string) *ltproto.Error {
	return &ltproto.Error{Code: code, Message: message}
}

func validTrack(track ltproto.Track) bool {
	return track.ID != "" && boundedText(track.ID, ltproto.MaxTrackIDBytes) &&
		boundedText(track.Title, ltproto.MaxTrackTextBytes) && boundedText(track.Artist, ltproto.MaxTrackTextBytes) &&
		track.DurationMS >= 0 && (track.Thumbnail == nil || len(*track.Thumbnail) <= ltproto.MaxThumbnailBytes &&
		(strings.HasPrefix(*track.Thumbnail, "https://") || strings.HasPrefix(*track.Thumbnail, "http://"))) &&
		(track.QueuedBy == nil || boundedText(*track.QueuedBy, ltproto.MaxUsernameRunes))
}

func validPlayback(playback ltproto.Playback) bool {
	if playback.PositionMS < 0 {
		return false
	}
	switch playback.Kind {
	case ltproto.PlaybackPlay, ltproto.PlaybackPause, ltproto.PlaybackSeek,
		ltproto.PlaybackChangeTrack, ltproto.PlaybackSyncQueue, ltproto.PlaybackSetVolume:
	default:
		return false
	}
	if playback.Track != nil && !validTrack(*playback.Track) {
		return false
	}
	if playback.Queue != nil {
		if len(*playback.Queue) > ltproto.MaxQueueTracks {
			return false
		}
		for _, track := range *playback.Queue {
			if !validTrack(track) {
				return false
			}
		}
	}
	return playback.Kind != ltproto.PlaybackSetVolume || !math.IsNaN(playback.Volume) && !math.IsInf(playback.Volume, 0) && playback.Volume >= 0 && playback.Volume <= 1
}

func boundedText(value string, maximum int) bool {
	if len(value) > maximum {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func sanitizeUsername(value string) string {
	value = strings.TrimSpace(value)
	var result strings.Builder
	count := 0
	for _, character := range value {
		if unicode.IsControl(character) {
			continue
		}
		if count == ltproto.MaxUsernameRunes {
			break
		}
		result.WriteRune(character)
		count++
	}
	if result.Len() == 0 {
		return "Guest"
	}
	return result.String()
}

func generateUserID() string {
	return "user_" + generateToken()
}

func generateToken() string {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		panic(fmt.Sprintf("secure random source failed: %v", err))
	}
	return hex.EncodeToString(buffer)
}

func defaultBindAddress() string {
	if address := os.Getenv("RYOTUNES_SYNC_BIND"); address != "" {
		return address
	}
	port := 8080
	if raw := os.Getenv("PORT"); raw != "" {
		if parsed, err := strconv.ParseUint(raw, 10, 16); err == nil && parsed != 0 {
			port = int(parsed)
		}
	}
	return "127.0.0.1:" + strconv.Itoa(port)
}

func main() {
	bind := flag.String("bind", defaultBindAddress(), "plain WebSocket bind address")
	flag.Parse()
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	hub := newRelay(log)
	server := &http.Server{Addr: *bind, Handler: http.HandlerFunc(hub.serveHTTP), ReadHeaderTimeout: 10 * time.Second}
	stopping := make(chan struct{})
	go func() {
		ticker := time.NewTicker(cleanupInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				hub.cleanup()
			case <-stopping:
				return
			}
		}
	}()
	go func() {
		log.Info("ryotunes-sync listening", "address", *bind)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("relay stopped", "error", err)
			os.Exit(1)
		}
	}()
	shutdown, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-shutdown.Done()
	close(stopping)
	hub.closeConnections()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Error("graceful shutdown failed", "error", err)
	}
}
