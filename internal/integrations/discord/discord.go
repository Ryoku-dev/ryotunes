// Package discord publishes playback through Discord's local Rich Presence IPC.
package discord

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ryoku-dev/ryotunes/internal/model"
)

const (
	applicationID       = "1525891596804161727"
	defaultPresenceName = "Ryotunes"
	brandImage          = "https://raw.githubusercontent.com/ryoku-dev/ryotunes/main/src-tauri/icons/icon.png"
	brandBadgeText      = "Ryotunes"
	maxFieldRunes       = 128
	maxAssetURLBytes    = 256
)

const (
	opHandshake int32 = 0
	opFrame     int32 = 1
)

var (
	widthHeightMarker = regexp.MustCompile(`=w[0-9]+-h[0-9]+`)
	squareMarker      = regexp.MustCompile(`=s[0-9]+`)
)

// Options configures Discord Rich Presence.
type Options struct {
	Enabled      bool
	PresenceName string
	Log          *slog.Logger
}

// Client owns the Discord IPC worker.
type Client struct {
	cancel    context.CancelFunc
	messages  chan message
	updates   chan playbackUpdate
	done      chan struct{}
	status    atomic.Int32
	enabled   atomic.Bool
	closeOnce sync.Once
}

type message struct {
	kind    messageKind
	enabled bool
	name    string
	ack     chan struct{}
}

type messageKind uint8

const (
	messageEnabled messageKind = iota
	messageName
	messageClose
)

type playbackUpdate struct {
	track    model.PlaybackTrack
	hasTrack bool
	paused   bool
	position float64
	duration float64
	at       time.Time
}

type activity struct {
	Name              string             `json:"name"`
	Type              int                `json:"type"`
	StatusDisplayType string             `json:"status_display_type"`
	Details           string             `json:"details"`
	State             string             `json:"state,omitempty"`
	Timestamps        activityTimestamps `json:"timestamps"`
	Assets            activityAssets     `json:"assets"`
	Buttons           []activityButton   `json:"buttons,omitempty"`
}

type activityTimestamps struct {
	Start int64  `json:"start"`
	End   *int64 `json:"end,omitempty"`
}

type activityAssets struct {
	LargeImage string `json:"large_image"`
	LargeText  string `json:"large_text"`
	SmallImage string `json:"small_image,omitempty"`
	SmallText  string `json:"small_text,omitempty"`
}

type activityButton struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

type worker struct {
	log            *slog.Logger
	enabled        bool
	name           string
	current        playbackUpdate
	trackChangedAt time.Time
	pending        bool
	shown          bool
	lastSend       time.Time
	sentStartMS    int64
	conn           net.Conn
	reader         *bufio.Reader
	backoff        time.Duration
	nextConnect    time.Time
	nonce          uint64
	socketPaths    []string
}

// New starts a fail-soft Discord IPC worker.
func New(ctx context.Context, options Options) *Client {
	if options.Log == nil {
		options.Log = slog.Default()
	}
	name, err := NormalizePresenceName(options.PresenceName)
	if err != nil {
		name = defaultPresenceName
	}
	workerContext, cancel := context.WithCancel(ctx)
	client := &Client{
		cancel:   cancel,
		messages: make(chan message, 8),
		updates:  make(chan playbackUpdate, 1),
		done:     make(chan struct{}),
	}
	client.enabled.Store(options.Enabled)
	if options.Enabled {
		client.status.Store(1)
	}
	go client.run(workerContext, &worker{
		log:         options.Log,
		enabled:     options.Enabled,
		name:        name,
		backoff:     time.Second,
		socketPaths: socketPaths(),
	})
	return client
}

// NormalizePresenceName validates and normalizes the configurable activity name.
func NormalizePresenceName(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return defaultPresenceName, nil
	}
	length := len([]rune(value))
	if length < 2 || length > maxFieldRunes {
		return "", errors.New("Discord presence title must be between 2 and 128 characters.")
	}
	return value, nil
}

// Update replaces the current playback state; paused playback clears presence.
func (c *Client) Update(now *model.PlaybackTrack, paused bool, position, duration float64) {
	var copied model.PlaybackTrack
	hasTrack := now != nil
	if now != nil {
		copied = *now
	}
	c.enqueueUpdate(playbackUpdate{
		track:    copied,
		hasTrack: hasTrack,
		paused:   paused,
		position: max(position, 0),
		duration: max(duration, 0),
		at:       time.Now(),
	})
}

// SetEnabled starts or stops IPC connection attempts immediately.
func (c *Client) SetEnabled(enabled bool) {
	c.enabled.Store(enabled)
	if enabled {
		c.status.Store(1)
	} else {
		c.status.Store(0)
	}
	c.enqueue(message{kind: messageEnabled, enabled: enabled})
}

// SetPresenceName validates and applies the live Discord activity name.
func (c *Client) SetPresenceName(raw string) error {
	name, err := NormalizePresenceName(raw)
	if err != nil {
		return err
	}
	c.enqueue(message{kind: messageName, name: name})
	return nil
}

// Status returns the current Discord IPC state.
func (c *Client) Status() model.DiscordStatus {
	statuses := [...]string{"disabled", "connecting", "connected", "unavailable"}
	state := c.status.Load()
	if state < 0 || int(state) >= len(statuses) {
		state = 3
	}
	return model.DiscordStatus{Enabled: c.enabled.Load(), Status: statuses[state]}
}

// Close clears Rich Presence and waits at most 900 ms for the worker to close.
func (c *Client) Close() {
	c.closeOnce.Do(func() {
		c.enabled.Store(false)
		c.status.Store(0)
		ack := make(chan struct{})
		timer := time.NewTimer(900 * time.Millisecond)
		defer timer.Stop()
		select {
		case c.messages <- message{kind: messageClose, ack: ack}:
			select {
			case <-ack:
			case <-timer.C:
			}
		case <-c.done:
		case <-timer.C:
		}
		c.cancel()
	})
}

func (c *Client) enqueue(msg message) {
	select {
	case c.messages <- msg:
	case <-c.done:
		if msg.ack != nil {
			close(msg.ack)
		}
	}
}

func (c *Client) enqueueUpdate(update playbackUpdate) {
	select {
	case <-c.done:
		return
	default:
	}
	select {
	case c.updates <- update:
	default:
		select {
		case <-c.updates:
		default:
		}
		select {
		case c.updates <- update:
		case <-c.done:
		}
	}
}

func (c *Client) run(ctx context.Context, state *worker) {
	defer close(c.done)
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			state.shutdown()
			return
		case msg := <-c.messages:
			if msg.kind == messageClose {
				state.shutdown()
				c.status.Store(0)
				close(msg.ack)
				return
			}
			state.apply(msg)
		case update := <-c.updates:
			state.applyUpdate(update)
		case <-timer.C:
		}
		wait := state.sync(c)
		if wait < 10*time.Millisecond {
			wait = 10 * time.Millisecond
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(wait)
	}
}

func (w *worker) apply(msg message) {
	switch msg.kind {
	case messageEnabled:
		w.enabled = msg.enabled
		w.pending = true
		w.nextConnect = time.Time{}
		w.backoff = time.Second
	case messageName:
		if w.name != msg.name {
			w.name = msg.name
			w.pending = true
		}
	}
}

func (w *worker) applyUpdate(update playbackUpdate) {
	trackChanged := !samePresenceTrack(w.current, update)
	if trackChanged {
		update.position = 0
		w.trackChangedAt = update.at
	}
	desiredStart := update.at.UnixMilli() - int64(update.position*1000)
	drifted := w.shown && abs64(desiredStart-w.sentStartMS) >= 2_000
	w.pending = w.pending || trackChanged || w.current.paused != update.paused || w.current.duration != update.duration || drifted
	w.current = update
}

func (w *worker) sync(client *Client) time.Duration {
	now := time.Now()
	if !w.enabled {
		if w.conn != nil {
			w.clear()
			w.disconnect()
		}
		w.pending = false
		client.status.Store(0)
		return 5 * time.Minute
	}
	if w.conn == nil {
		if now.Before(w.nextConnect) {
			return time.Until(w.nextConnect)
		}
		client.status.Store(1)
		if err := w.connect(); err != nil {
			client.status.Store(3)
			w.log.Debug("discord not available", "error", err, "backoff", w.backoff)
			w.nextConnect = now.Add(w.backoff)
			wait := w.backoff
			w.backoff = min(w.backoff*2, 120*time.Second)
			return wait
		}
		client.status.Store(2)
		w.backoff = time.Second
		w.nextConnect = time.Time{}
		w.pending = true
	}
	if !w.pending {
		return 5 * time.Minute
	}
	show := w.current.hasTrack && !w.current.paused
	if show && w.current.duration <= 0 && !w.trackChangedAt.IsZero() {
		if remaining := 800*time.Millisecond - now.Sub(w.trackChangedAt); remaining > 0 {
			return remaining
		}
	}
	if !w.lastSend.IsZero() {
		if remaining := 1500*time.Millisecond - now.Sub(w.lastSend); remaining > 0 {
			return remaining
		}
	}
	var next *activity
	if show {
		update := w.current
		if !update.paused {
			update.position += now.Sub(update.at).Seconds()
		}
		value := buildActivity(now, w.name, update)
		next = &value
	}
	if err := w.setActivity(next); err != nil {
		w.log.Debug("discord response read failed, dropping socket", "error", err)
		w.disconnect()
		client.status.Store(3)
		w.nextConnect = now.Add(w.backoff)
		wait := w.backoff
		w.backoff = min(w.backoff*2, 120*time.Second)
		return wait
	}
	w.lastSend = now
	w.pending = false
	w.shown = next != nil
	if next != nil {
		w.sentStartMS = next.Timestamps.Start
	} else {
		w.sentStartMS = 0
	}
	return 5 * time.Minute
}

func (w *worker) connect() error {
	var lastErr error
	for _, path := range w.socketPaths {
		conn, err := net.DialTimeout("unix", path, 500*time.Millisecond)
		if err != nil {
			lastErr = err
			continue
		}
		reader := bufio.NewReader(conn)
		handshake, _ := json.Marshal(map[string]any{"v": 1, "client_id": applicationID})
		_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
		if _, err := conn.Write(encodeFrame(opHandshake, handshake)); err != nil {
			_ = conn.Close()
			lastErr = err
			continue
		}
		opcode, response, err := readFrame(reader)
		if err != nil {
			_ = conn.Close()
			lastErr = err
			continue
		}
		if message := responseError(response); message != "" {
			_ = conn.Close()
			lastErr = errors.New(message)
			continue
		}
		if opcode != opFrame || !responseReady(response) {
			_ = conn.Close()
			lastErr = errors.New("Discord IPC handshake returned no READY event")
			continue
		}
		_ = conn.SetDeadline(time.Time{})
		w.conn = conn
		w.reader = reader
		w.shown = false
		w.log.Info("discord rich presence connected", "socket", path)
		return nil
	}
	if lastErr == nil {
		lastErr = errors.New("no Discord IPC socket found")
	}
	return lastErr
}

func (w *worker) setActivity(value *activity) error {
	w.nonce++
	payload := map[string]any{
		"cmd":   "SET_ACTIVITY",
		"args":  map[string]any{"pid": os.Getpid(), "activity": value},
		"nonce": strconv.FormatUint(w.nonce, 10),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_ = w.conn.SetDeadline(time.Now().Add(2 * time.Second))
	if _, err := w.conn.Write(encodeFrame(opFrame, body)); err != nil {
		return err
	}
	opcode, response, err := readFrame(w.reader)
	if err != nil {
		return err
	}
	if opcode != opFrame {
		return fmt.Errorf("unexpected Discord IPC opcode %d", opcode)
	}
	_ = w.conn.SetDeadline(time.Time{})
	if message := responseError(response); message != "" {
		w.log.Warn("discord rejected the payload", "error", message)
	}
	return nil
}

func (w *worker) clear() {
	if !w.shown || w.conn == nil {
		return
	}
	_ = w.setActivity(nil)
	w.shown = false
}

func (w *worker) shutdown() {
	w.clear()
	w.disconnect()
}

func (w *worker) disconnect() {
	if w.conn != nil {
		_ = w.conn.Close()
	}
	w.conn = nil
	w.reader = nil
	w.shown = false
}

func buildActivity(now time.Time, name string, update playbackUpdate) activity {
	track := &update.track
	position := max(update.position, 0)
	start := now.UnixMilli() - int64(position*1000)
	timestamps := activityTimestamps{Start: start}
	if update.duration > 0 {
		end := start + int64(update.duration*1000)
		timestamps.End = &end
	}
	assets := activityAssets{LargeImage: brandImage, LargeText: field(providerLabel(track.VideoID))}
	if image := discordThumbnail(track.VideoID, pointerValue(track.Thumbnail)); image != "" {
		assets.LargeImage = image
		assets.SmallImage = brandImage
		assets.SmallText = brandBadgeText
	}
	result := activity{
		Name:              field(name),
		Type:              2,
		StatusDisplayType: "state",
		Details:           field(track.Title),
		Timestamps:        timestamps,
		Assets:            assets,
	}
	if strings.TrimSpace(track.Artists) != "" {
		result.State = field(track.Artists)
	}
	if label, link, ok := listenLink(track.VideoID); ok {
		result.Buttons = []activityButton{{Label: label, URL: link}}
	}
	return result
}

func field(value string) string {
	runes := []rune(value)
	if len(runes) > maxFieldRunes {
		runes = append(runes[:maxFieldRunes-1], '…')
	}
	for len(runes) < 2 {
		runes = append(runes, '\u2800')
	}
	return string(runes)
}

func discordThumbnail(id, raw string) string {
	if model.ProviderOf(id) == model.ProviderLocal || raw == "" {
		return ""
	}
	sized := widthHeightMarker.ReplaceAllString(raw, "=w512-h512")
	if sized == raw {
		sized = squareMarker.ReplaceAllString(raw, "=s512")
	}
	if len(sized) > maxAssetURLBytes {
		return ""
	}
	parsed, err := url.Parse(sized)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil {
		return ""
	}
	return sized
}

func listenLink(id string) (string, string, bool) {
	if spotifyID, ok := spotifyTrackID(id); ok {
		return "Listen on Spotify", "https://open.spotify.com/track/" + spotifyID, true
	}
	if isYouTubeID(id) {
		return "Listen on YouTube Music", "https://music.youtube.com/watch?v=" + id, true
	}
	return "", "", false
}

func spotifyTrackID(id string) (string, bool) {
	var value string
	switch {
	case strings.HasPrefix(id, "spotify:track:"):
		value = strings.TrimPrefix(id, "spotify:track:")
	case strings.HasPrefix(id, "spotify:"):
		value = strings.TrimPrefix(id, "spotify:")
	default:
		return "", false
	}
	if len(value) != 22 {
		return "", false
	}
	for _, char := range value {
		if !(char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9') {
			return "", false
		}
	}
	return value, true
}

func isYouTubeID(id string) bool {
	if len(id) != 11 {
		return false
	}
	for _, char := range id {
		if !(char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || char == '-' || char == '_') {
			return false
		}
	}
	return true
}

func providerLabel(id string) string {
	switch model.ProviderOf(id) {
	case model.ProviderLocal:
		return "Local library"
	case model.ProviderRadio:
		return "Radio"
	case model.ProviderSoundCloud:
		return "SoundCloud"
	case model.ProviderSpotify:
		return "Spotify"
	case model.ProviderTidal:
		return "TIDAL"
	default:
		return "YouTube Music"
	}
}

func encodeFrame(opcode int32, payload []byte) []byte {
	frame := make([]byte, 8+len(payload))
	binary.LittleEndian.PutUint32(frame[0:4], uint32(opcode))
	binary.LittleEndian.PutUint32(frame[4:8], uint32(len(payload)))
	copy(frame[8:], payload)
	return frame
}

func readFrame(reader io.Reader) (int32, []byte, error) {
	header := make([]byte, 8)
	if _, err := io.ReadFull(reader, header); err != nil {
		return 0, nil, err
	}
	opcode := int32(binary.LittleEndian.Uint32(header[0:4]))
	length := binary.LittleEndian.Uint32(header[4:8])
	if length > 1<<20 {
		return 0, nil, fmt.Errorf("discord IPC frame is too large: %d", length)
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(reader, payload); err != nil {
		return 0, nil, err
	}
	return opcode, payload, nil
}

func responseError(payload []byte) string {
	var response struct {
		Event string `json:"evt"`
		Data  struct {
			Message string `json:"message"`
		} `json:"data"`
	}
	if json.Unmarshal(payload, &response) == nil && response.Event == "ERROR" {
		if response.Data.Message != "" {
			return response.Data.Message
		}
		return "unknown Discord IPC error"
	}
	return ""
}

func responseReady(payload []byte) bool {
	var response struct {
		Event string `json:"evt"`
	}
	return json.Unmarshal(payload, &response) == nil && response.Event == "READY"
}

func socketPaths() []string {
	bases := make([]string, 0, 6)
	for _, key := range []string{"XDG_RUNTIME_DIR", "TMPDIR", "TMP", "TEMP"} {
		if value := os.Getenv(key); value != "" {
			if key == "XDG_RUNTIME_DIR" && os.Getenv("SNAP") != "" {
				bases = append(bases, filepath.Dir(value))
			}
			bases = append(bases, value)
		}
	}
	if os.Getenv("XDG_RUNTIME_DIR") == "" {
		bases = append(bases, filepath.Join("/run/user", strconv.Itoa(os.Getuid())))
	}
	bases = append(bases, os.TempDir())
	subpaths := []string{
		"",
		filepath.Join("app", "com.discordapp.Discord"),
		filepath.Join("app", "dev.vencord.Vesktop"),
		filepath.Join(".flatpak", "com.discordapp.Discord", "xdg-run"),
		filepath.Join(".flatpak", "dev.vencord.Vesktop", "xdg-run"),
		"snap.discord-canary",
		"snap.discord",
	}
	seen := make(map[string]struct{})
	paths := make([]string, 0, len(bases)*len(subpaths)*10)
	for _, base := range bases {
		for index := range 10 {
			for _, subpath := range subpaths {
				path := filepath.Join(base, subpath, fmt.Sprintf("discord-ipc-%d", index))
				if _, ok := seen[path]; ok {
					continue
				}
				seen[path] = struct{}{}
				paths = append(paths, path)
			}
		}
	}
	return paths
}

func samePresenceTrack(left, right playbackUpdate) bool {
	if left.hasTrack != right.hasTrack {
		return false
	}
	if !left.hasTrack {
		return true
	}
	return left.track.VideoID == right.track.VideoID &&
		left.track.Title == right.track.Title &&
		left.track.Artists == right.track.Artists &&
		pointerValue(left.track.Thumbnail) == pointerValue(right.track.Thumbnail)
}

func pointerValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func abs64(value int64) int64 {
	if value < 0 {
		return -value
	}
	return value
}
