package player

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	mpv "github.com/gen2brain/go-mpv"
	"github.com/ryoku-dev/ryotunes/internal/model"
)

// Options configures a Player.
type Options struct {
	// CacheDir is the directory used by mpv's on-disk demuxer cache.
	CacheDir string
	// Log receives best-effort playback diagnostics.
	Log *slog.Logger
	// LowResource selects reduced cache budgets and a slower position cadence.
	LowResource bool
}

// Player is a serialized, event-driven libmpv audio session.
type Player struct {
	log       *slog.Logger
	requests  chan request
	stop      chan struct{}
	closed    chan struct{}
	events    chan Event
	closeOnce sync.Once

	position atomic.Uint64
	duration atomic.Uint64
	paused   atomic.Bool
	idle     atomic.Bool
	live     atomic.Bool
	cadence  atomic.Int64
	asyncID  atomic.Uint64
	fxState  atomic.Pointer[model.AudioFx]

	fx       model.AudioFx
	loudness *float64
}

type request struct {
	ctx   context.Context
	do    func(*mpv.Mpv) error
	reply chan error
}

// New creates an audio-only player with its disk cache rooted at o.CacheDir.
func New(o Options) (*Player, error) {
	if o.CacheDir == "" {
		return nil, fmt.Errorf("player cache directory is empty")
	}
	if err := os.MkdirAll(o.CacheDir, 0o700); err != nil {
		return nil, fmt.Errorf("create player cache: %w", err)
	}
	log := o.Log
	if log == nil {
		log = slog.Default()
	}
	p := &Player{
		log:      log,
		requests: make(chan request),
		stop:     make(chan struct{}),
		closed:   make(chan struct{}),
		events:   make(chan Event, 256),
		fx:       model.DefaultAudioFx(),
	}
	initialFx := p.fx
	p.fxState.Store(&initialFx)
	p.cadence.Store(int64(positionCadence(o.LowResource)))
	p.idle.Store(true)

	ready := make(chan error, 1)
	go p.run(o, ready)
	if err := <-ready; err != nil {
		return nil, err
	}
	return p, nil
}

// Load replaces the current playlist with s, starting at startAt seconds in the requested pause state.
func (p *Player) Load(ctx context.Context, s model.Stream, startAt float64, paused bool) error {
	if ctx == nil {
		ctx = context.Background()
	}
	return p.call(ctx, func(m *mpv.Mpv) error {
		previousLoudness := p.loudness
		p.loudness = cloneFloat(s.LoudnessDB)
		if err := p.applyHeadersAsync(m, s); err != nil {
			p.loudness = previousLoudness
			return err
		}
		if err := p.setPropertyAsync(m, "af", mpv.FormatString, afChain(loudnessGain(p.loudness), p.fx)); err != nil {
			p.loudness = previousLoudness
			return err
		}
		if err := p.setPropertyAsync(m, "pause", mpv.FormatFlag, paused); err != nil {
			p.loudness = previousLoudness
			return err
		}

		// Async commands are copied by libmpv before returning. This keeps the
		// request/event loop responsive while the core opens a remote stream.
		effectiveStart := 0.0
		args := []string{"loadfile", s.URL, "replace", "-1"}
		if startAt > 0 && !math.IsNaN(startAt) && !math.IsInf(startAt, 0) {
			effectiveStart = startAt
			args = append(args, "start="+formatFloat(startAt))
		}
		if err := m.CommandAsync(p.nextAsyncID(), args); err != nil {
			p.loudness = previousLoudness
			return friendlyError(err)
		}

		p.position.Store(math.Float64bits(effectiveStart))
		p.duration.Store(0)
		p.idle.Store(false)
		p.live.Store(s.IsLive)
		return nil
	})
}

// Prefetch appends one compatible lookahead URL and asks mpv to open playlist entries ahead of time.
// MPV headers are global, so the lookahead inherits the currently loaded stream's headers.
func (p *Player) Prefetch(s model.Stream) {
	err := p.call(context.Background(), func(m *mpv.Mpv) error {
		if err := p.setPropertyAsync(m, "prefetch-playlist", mpv.FormatString, "yes"); err != nil {
			return err
		}
		return m.CommandAsync(p.nextAsyncID(), []string{"loadfile", s.URL, "append", "-1"})
	})
	if err != nil {
		p.log.Debug("mpv lookahead prefetch failed", "error", err)
	}
}

// Play resumes playback.
func (p *Player) Play() error {
	return p.call(context.Background(), func(m *mpv.Mpv) error {
		return p.setPropertyAsync(m, "pause", mpv.FormatFlag, false)
	})
}

// Pause pauses playback.
func (p *Player) Pause() error {
	return p.call(context.Background(), func(m *mpv.Mpv) error {
		return p.setPropertyAsync(m, "pause", mpv.FormatFlag, true)
	})
}

// TogglePause toggles mpv's pause property.
func (p *Player) TogglePause() error {
	return p.call(context.Background(), func(m *mpv.Mpv) error {
		return m.CommandAsync(p.nextAsyncID(), []string{"cycle", "pause"})
	})
}

// Stop stops playback and clears the resumable media session.
func (p *Player) Stop() error {
	return p.call(context.Background(), func(m *mpv.Mpv) error {
		if err := m.CommandAsync(p.nextAsyncID(), []string{"stop"}); err != nil {
			return err
		}
		p.idle.Store(true)
		return nil
	})
}

// Seek moves to an absolute position in seconds.
func (p *Player) Seek(pos float64) error {
	if pos < 0 || math.IsNaN(pos) || math.IsInf(pos, 0) {
		return fmt.Errorf("Seek position must be a non-negative finite number.")
	}
	return p.call(context.Background(), func(m *mpv.Mpv) error {
		return m.CommandAsync(p.nextAsyncID(), []string{"seek", formatFloat(pos), "absolute"})
	})
}

// SetVolume sets output volume from a perceptual percentage in the range 0–100.
func (p *Player) SetVolume(percent int) error {
	percent = min(max(percent, 0), 100)
	return p.call(context.Background(), func(m *mpv.Mpv) error {
		return p.setPropertyAsync(m, "volume", mpv.FormatDouble, perceptualVolume(percent))
	})
}

// SetFx applies tempo, pitch, reverb, bass and stereo-width effects atomically.
func (p *Player) SetFx(fx model.AudioFx) error {
	if err := validateFx(fx); err != nil {
		return err
	}
	return p.call(context.Background(), func(m *mpv.Mpv) error {
		previous := p.fx
		if err := p.setPropertyAsync(m, "speed", mpv.FormatDouble, fx.Speed); err != nil {
			return err
		}
		if err := p.setPropertyAsync(m, "af", mpv.FormatString, afChain(loudnessGain(p.loudness), fx)); err != nil {
			_ = p.setPropertyAsync(m, "speed", mpv.FormatDouble, previous.Speed)
			_ = p.setPropertyAsync(m, "af", mpv.FormatString, afChain(loudnessGain(p.loudness), previous))
			return err
		}
		p.fx = fx
		copy := fx
		p.fxState.Store(&copy)
		return nil
	})
}

// Fx returns a lock-free copy of the effects most recently accepted by mpv.
func (p *Player) Fx() model.AudioFx {
	if fx := p.fxState.Load(); fx != nil {
		return *fx
	}
	return model.DefaultAudioFx()
}

// SetLoudness applies the stream's loudness measurement while preserving all user effects.
func (p *Player) SetLoudness(db *float64) {
	value := cloneFloat(db)
	err := p.call(context.Background(), func(m *mpv.Mpv) error {
		previous := p.loudness
		p.loudness = value
		if err := p.setPropertyAsync(m, "af", mpv.FormatString, afChain(loudnessGain(value), p.fx)); err != nil {
			p.loudness = previous
			return err
		}
		return nil
	})
	if err != nil {
		p.log.Warn("apply mpv loudness failed", "error", err)
	}
}

// Position returns the most recently observed position in seconds.
func (p *Player) Position() float64 {
	return math.Float64frombits(p.position.Load())
}

// Duration returns the most recently observed duration in seconds.
func (p *Player) Duration() float64 {
	return math.Float64frombits(p.duration.Load())
}

// Paused reports the most recently observed pause state.
func (p *Player) Paused() bool {
	return p.paused.Load()
}

// Idle reports whether mpv has no active media.
func (p *Player) Idle() bool {
	return p.idle.Load()
}

// SetPositionCadence changes the minimum interval between position events.
func (p *Player) SetPositionCadence(cadence time.Duration) {
	if cadence <= 0 {
		cadence = 250 * time.Millisecond
	}
	p.cadence.Store(int64(cadence))
}

// Events returns the player's event stream, which closes with the player.
func (p *Player) Events() <-chan Event {
	return p.events
}

// Close terminates libmpv and closes the event stream.
func (p *Player) Close() {
	p.closeOnce.Do(func() {
		close(p.stop)
		<-p.closed
	})
}

func (p *Player) call(ctx context.Context, fn func(*mpv.Mpv) error) error {
	reply := make(chan error, 1)
	req := request{ctx: ctx, do: fn, reply: reply}
	select {
	case p.requests <- req:
	case <-ctx.Done():
		return ctx.Err()
	case <-p.closed:
		return fmt.Errorf("player is closed")
	}
	select {
	case err := <-reply:
		return err
	case <-ctx.Done():
		return ctx.Err()
	case <-p.closed:
		return fmt.Errorf("player is closed")
	}
}

func (p *Player) run(o Options, ready chan<- error) {
	mpv.SetLocale(mpv.LCNumeric, "C")
	m := mpv.New()
	for _, option := range playerOptions(o.CacheDir, o.LowResource) {
		if err := m.SetOptionString(option.name, option.value); err != nil {
			m.Destroy()
			ready <- fmt.Errorf("set mpv option %s: %w", option.name, err)
			close(p.events)
			close(p.closed)
			return
		}
	}
	if err := m.Initialize(); err != nil {
		m.Destroy()
		ready <- fmt.Errorf("initialize mpv: %w", err)
		close(p.events)
		close(p.closed)
		return
	}
	defer func() {
		m.TerminateDestroy()
		close(p.events)
		close(p.closed)
	}()

	for id, observed := range observedProperties() {
		if err := m.ObserveProperty(uint64(id), observed.name, observed.format); err != nil {
			ready <- fmt.Errorf("observe mpv property %s: %w", observed.name, err)
			return
		}
	}
	for _, id := range []mpv.EventID{11, 14, 19, 23} {
		_ = m.RequestEvent(id, false)
	}
	ready <- nil

	var lastPositionEmit time.Time
	var hasPosition bool
	var lastTitle string
	for {
		select {
		case <-p.stop:
			return
		case req := <-p.requests:
			if err := req.ctx.Err(); err != nil {
				req.reply <- err
			} else {
				req.reply <- req.do(m)
			}
		default:
		}

		event := m.WaitEvent(0.05)
		if event.EventID == mpv.EventNone {
			continue
		}
		p.handleMPVEvent(event, &lastPositionEmit, &hasPosition, &lastTitle)
	}
}

func (p *Player) handleMPVEvent(event *mpv.Event, lastPositionEmit *time.Time, hasPosition *bool, lastTitle *string) {
	switch event.EventID {
	case mpv.EventStart:
		*lastTitle = ""
	case mpv.EventFileLoaded:
		p.idle.Store(false)
		*lastTitle = ""
		p.emit(Event{Kind: FileLoaded})
	case mpv.EventPropertyChange:
		property := event.Property()
		switch property.Name {
		case "time-pos":
			position, ok := property.Data.(float64)
			if !ok || math.IsNaN(position) || math.IsInf(position, 0) {
				return
			}
			p.position.Store(math.Float64bits(position))
			*hasPosition = true
			now := time.Now()
			if shouldEmitPosition(*lastPositionEmit, now, time.Duration(p.cadence.Load()), false) {
				*lastPositionEmit = now
				p.emit(Event{Kind: Position, Position: position})
			}
		case "duration":
			duration, ok := property.Data.(float64)
			if !ok || math.IsNaN(duration) || math.IsInf(duration, 0) {
				return
			}
			p.duration.Store(math.Float64bits(duration))
			p.emit(Event{Kind: Duration, Duration: duration})
		case "pause":
			paused, ok := eventFlag(property.Data)
			if !ok {
				return
			}
			changed := p.paused.Swap(paused) != paused
			if paused && *hasPosition {
				now := time.Now()
				if shouldEmitPosition(*lastPositionEmit, now, time.Duration(p.cadence.Load()), true) {
					*lastPositionEmit = now
					p.emit(Event{Kind: Position, Position: p.Position()})
				}
			}
			if changed {
				p.emit(Event{Kind: PauseChanged, Paused: paused})
			}
		case "idle-active":
			idle, ok := eventFlag(property.Data)
			if !ok {
				return
			}
			changed := p.idle.Swap(idle) != idle
			if changed && idle {
				p.emit(Event{Kind: Idle})
			}
		case "media-title":
			title, ok := property.Data.(string)
			if ok && p.live.Load() && title != "" && title != *lastTitle {
				*lastTitle = title
				p.emit(Event{Kind: IcyTitle, Title: title})
			}
		}
	case mpv.EventEnd:
		end := event.EndFile()
		switch end.Reason {
		case mpv.EndFileEOF:
			p.emit(Event{Kind: EndOfFile})
		case mpv.EndFileError:
			p.emit(Event{Kind: Error, Message: friendlyError(end.Error).Error()})
		}
	case mpv.EventShutdown:
		p.idle.Store(true)
		p.emit(Event{Kind: Idle})
	case mpv.EventQueueOverflow:
		p.log.Warn("mpv event queue overflow")
	}
}

func (p *Player) emit(event Event) {
	select {
	case p.events <- event:
	default:
		p.log.Warn("dropping player event", "kind", event.Kind)
	}
}

func (p *Player) nextAsyncID() uint64 {
	return p.asyncID.Add(1)
}

func (p *Player) setPropertyAsync(m *mpv.Mpv, name string, format mpv.Format, value any) error {
	return m.SetPropertyAsync(name, p.nextAsyncID(), format, value)
}

func (p *Player) applyHeadersAsync(m *mpv.Mpv, stream model.Stream) error {
	userAgent := stream.UserAgent
	keys := make([]string, 0, len(stream.Headers))
	for key := range stream.Headers {
		if strings.EqualFold(key, "user-agent") {
			if userAgent == "" {
				userAgent = stream.Headers[key]
			}
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	fields := make([]string, 0, len(keys))
	for _, key := range keys {
		fields = append(fields, key+": "+stream.Headers[key])
	}
	if err := p.setPropertyAsync(m, "user-agent", mpv.FormatString, userAgent); err != nil {
		return err
	}
	return p.setPropertyAsync(m, "http-header-fields", mpv.FormatString, strings.Join(fields, ","))
}

func eventFlag(value any) (bool, bool) {
	switch value := value.(type) {
	case int:
		return value != 0, true
	case bool:
		return value, true
	default:
		return false, false
	}
}

func cloneFloat(value *float64) *float64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func formatFloat(value float64) string {
	return strconv.FormatFloat(value, 'g', -1, 64)
}
