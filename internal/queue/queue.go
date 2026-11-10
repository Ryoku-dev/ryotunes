package queue

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"math/rand/v2"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/ryoku-dev/ryotunes/internal/bus"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
	"github.com/ryoku-dev/ryotunes/internal/provider"
)

// Queue owns queue ordering and the active player transport.
type Queue struct {
	mu       sync.Mutex
	player   Transport
	registry *provider.Registry
	persist  Persist
	hooks    Hooks
	bus      bus.Sink
	log      *slog.Logger
	rng      *rand.Rand
	now      func() time.Time
	state    state

	volume           int
	stopAfterCurrent bool
	autoplayFetching bool
	position         float64
	duration         float64
	playing          bool
	streamClient     string
	pendingSeek      float64
	pendingSeekID    string
	playedRecorded   bool
	retriedTrackID   string
	lastPositionSave time.Time
	lastBlobFP       string
	generation       uint64
	pendingStart     *playbackStart

	ctx         context.Context
	cancel      context.CancelFunc
	lifecycleMu sync.Mutex
	closing     bool
	wg          sync.WaitGroup
	closed      chan struct{}
}

// New restores queue state, applies the saved volume, and starts the player event pump.
func New(o Options) (*Queue, error) {
	if o.Player == nil {
		return nil, errors.New("queue: nil player")
	}
	if o.Registry == nil {
		return nil, errors.New("queue: nil provider registry")
	}
	if o.Bus == nil {
		o.Bus = bus.Discard
	}
	if o.Log == nil {
		o.Log = slog.Default()
	}
	if o.Rand == nil {
		o.Rand = rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), uint64(time.Now().UnixNano()>>1)))
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	queueCtx, cancel := context.WithCancel(context.Background())
	q := &Queue{
		player: o.Player, persist: o.Persist, hooks: o.Hooks, bus: o.Bus,
		log: o.Log, rng: o.Rand, now: o.Now, volume: 100,
		state: state{repeat: model.RepeatOff}, ctx: queueCtx, cancel: cancel,
		closed: make(chan struct{}),
	}
	q.registry = o.Registry
	q.restore()
	if err := q.player.SetVolume(q.volume); err != nil {
		cancel()
		return nil, fmt.Errorf("restore volume: %w", err)
	}
	q.start(func(context.Context) { q.pump() })
	q.emitRestored()
	return q, nil
}

// Close cancels and joins every queue-owned background operation.
func (q *Queue) Close() {
	q.persistPosition(true)
	q.lifecycleMu.Lock()
	if q.closing {
		q.lifecycleMu.Unlock()
		q.wg.Wait()
		return
	}
	q.closing = true
	q.cancel()
	close(q.closed)
	q.lifecycleMu.Unlock()
	q.wg.Wait()
}

func (q *Queue) start(fn func(context.Context)) {
	q.lifecycleMu.Lock()
	if q.closing {
		q.lifecycleMu.Unlock()
		return
	}
	q.wg.Add(1)
	q.lifecycleMu.Unlock()
	go func() {
		defer q.wg.Done()
		fn(q.ctx)
	}()
}

// Snapshot returns a detached complete queue snapshot.
func (q *Queue) Snapshot() model.QueueSnapshot {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.snapshotLocked()
}

// Playback returns the current transport snapshot.
func (q *Queue) Playback() model.PlaybackSnapshot {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.playbackLocked()
}

// Play replaces the context with one seed while retaining upcoming manual rows.
func (q *Queue) Play(ctx context.Context, item model.Track) error {
	return q.play(ctx, item, false)
}

func (q *Queue) play(ctx context.Context, item model.Track, paused bool) error {
	if item.VideoID == "" {
		return errNothingToPlay
	}
	q.mu.Lock()
	manual := upcomingManual(q.state.items, q.state.current)
	forceHydrate := len(manual) > 1
	q.generation++
	q.state = state{items: append([]model.Track{cleanQueueFlags(item)}, manual...), repeat: q.state.repeat, radioSeed: item.VideoID, radio: model.ProviderOf(item.VideoID) == model.ProviderRadio}
	q.stopAfterCurrent = false
	q.mu.Unlock()
	q.changed()
	if err := q.playIndex(ctx, 0, paused); err != nil {
		return err
	}
	if forceHydrate {
		q.start(func(ctx context.Context) { q.extendAutoplaySeed(ctx) })
	}
	return nil
}

// PlayPlaylist replaces the context with a collection and optionally walks its continuation.
func (q *Queue) PlayPlaylist(ctx context.Context, items []model.Track, start *int, sourceID, sourceName *string, shuffle bool, continuation *string) error {
	if len(items) == 0 {
		return errors.New("playlist has no tracks")
	}
	index := 0
	if start != nil {
		index = *start
	}
	if index < 0 || index >= len(items) {
		return errors.New("queue index out of range")
	}
	items, index, _ = dropUnplayable(cloneTracks(items), index, index)
	if index < 0 {
		return errNothingToPlay
	}
	for i := range items {
		items[i] = cleanQueueFlags(items[i])
	}
	q.mu.Lock()
	manual := upcomingManual(q.state.items, q.state.current)
	items = slices.Insert(items, index+1, manual...)
	q.generation++
	gen := q.generation
	q.state = state{items: items, current: index, playedFrom: index, repeat: q.state.repeat, sourceID: cloneString(sourceID), sourceName: cloneString(sourceName)}
	if sourceID != nil {
		q.state.radioSeed = *sourceID
	}
	if shuffle {
		q.state.shuffleOrig = cloneTracks(q.state.items)
		q.shuffleUpcomingLocked()
	}
	q.stopAfterCurrent = false
	q.mu.Unlock()
	q.changed()
	if continuation != nil && *continuation != "" {
		q.start(func(ctx context.Context) { q.fillContinuation(ctx, gen, *continuation, false, nil) })
	}
	return q.playIndex(ctx, index, false)
}

// PlayIndex starts one existing queue row.
func (q *Queue) PlayIndex(ctx context.Context, index int) error {
	return q.playIndex(ctx, index, false)
}

// PlayNext inserts tracks at the FIFO play-next boundary.
func (q *Queue) PlayNext(items []model.Track, from *model.QueueContext) error {
	q.mu.Lock()
	wasEmpty := len(q.state.items) == 0
	q.mu.Unlock()
	if err := q.enqueue(items, from, true); err != nil {
		return err
	}
	if wasEmpty && len(items) > 0 {
		return q.playIndex(q.ctx, 0, false)
	}
	return nil
}

// AddToQueue inserts tracks at the back of the manual block and walks an optional continuation.
func (q *Queue) AddToQueue(ctx context.Context, items []model.Track, from *model.QueueContext, continuation *string) error {
	q.mu.Lock()
	wasEmpty := len(q.state.items) == 0
	q.mu.Unlock()
	if err := q.enqueue(items, from, false); err != nil {
		return err
	}
	q.mu.Lock()
	gen := q.generation
	q.mu.Unlock()
	if continuation != nil && *continuation != "" {
		q.start(func(ctx context.Context) { q.fillContinuation(ctx, gen, *continuation, true, from) })
	}
	if wasEmpty && len(items) > 0 {
		return q.playIndex(ctx, 0, false)
	}
	return nil
}

func (q *Queue) enqueue(items []model.Track, from *model.QueueContext, next bool) error {
	if len(items) == 0 {
		return nil
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	items = q.prepareManualLocked(items, from, next)
	if len(items) == 0 {
		return nil
	}
	at := manualBoundary(q.state.items, q.state.current, next)
	q.state.items = slices.Insert(q.state.items, at, items...)
	if q.state.shuffleOrig != nil {
		origAt := manualBoundary(q.state.shuffleOrig, min(q.state.current, len(q.state.shuffleOrig)-1), next)
		q.state.shuffleOrig = slices.Insert(q.state.shuffleOrig, origAt, cloneTracks(items)...)
	}
	q.generation++
	gen := q.generation
	q.start(func(context.Context) { q.changed() })
	q.start(func(ctx context.Context) { q.prime(ctx, gen) })
	return nil
}

// MoveInQueue moves one upcoming row without moving history or the current row.
func (q *Queue) MoveInQueue(from, to int) error {
	q.mu.Lock()
	if from <= q.state.current || to <= q.state.current || from >= len(q.state.items) || to >= len(q.state.items) {
		q.mu.Unlock()
		return errors.New("only upcoming queue items can be moved")
	}
	item := q.state.items[from]
	q.state.items = slices.Delete(q.state.items, from, from+1)
	q.state.items = slices.Insert(q.state.items, to, item)
	q.generation++
	gen := q.generation
	q.mu.Unlock()
	q.changed()
	q.start(func(ctx context.Context) { q.prime(ctx, gen) })
	return nil
}

// RemoveFromQueue removes a non-current row and adjusts queue pointers.
func (q *Queue) RemoveFromQueue(index int) error {
	q.mu.Lock()
	if index < 0 || index >= len(q.state.items) || index == q.state.current {
		q.mu.Unlock()
		return errors.New("current queue item cannot be removed")
	}
	q.state.items = slices.Delete(q.state.items, index, index+1)
	if index < q.state.current {
		q.state.current--
	}
	if index < q.state.playedFrom {
		q.state.playedFrom--
	}
	q.generation++
	gen := q.generation
	q.mu.Unlock()
	q.changed()
	q.start(func(ctx context.Context) { q.prime(ctx, gen) })
	return nil
}

// ClearQueued removes all upcoming manually queued rows.
func (q *Queue) ClearQueued() {
	q.mu.Lock()
	start := min(max(q.state.current+1, 0), len(q.state.items))
	prefix := cloneTracks(q.state.items[:start])
	for _, item := range q.state.items[start:] {
		if !item.Queued && !item.QueuedEnd {
			prefix = append(prefix, item)
		}
	}
	q.state.items = prefix
	q.generation++
	gen := q.generation
	q.mu.Unlock()
	q.changed()
	q.start(func(ctx context.Context) { q.prime(ctx, gen) })
}

// Next advances manually, treating repeat-one as repeat-all so the user can leave the loop.
func (q *Queue) Next(ctx context.Context) error {
	q.mu.Lock()
	index, ok := q.nextIndexLocked(true)
	q.mu.Unlock()
	if ok {
		return q.playIndex(ctx, index, false)
	}
	if q.extendAutoplay(ctx) > 0 {
		q.mu.Lock()
		index = q.state.current + 1
		q.mu.Unlock()
		return q.playIndex(ctx, index, false)
	}
	return q.Stop()
}

// Prev restarts after five seconds; otherwise it moves to the previous row.
func (q *Queue) Prev(ctx context.Context) error {
	if q.player.Position() >= previousRestart.Seconds() {
		return q.Seek(0)
	}
	q.mu.Lock()
	index := max(0, q.state.current-1)
	q.mu.Unlock()
	return q.playIndex(ctx, index, false)
}

// ToggleShuffle toggles upcoming-row shuffling and returns the resulting state.
func (q *Queue) ToggleShuffle() bool {
	q.mu.Lock()
	if len(q.state.items) == 0 {
		q.mu.Unlock()
		return false
	}
	if q.state.shuffleOrig == nil {
		q.state.shuffleOrig = cloneTracks(q.state.items)
		q.shuffleUpcomingLocked()
	} else {
		q.unshuffleLocked()
	}
	q.generation++
	gen := q.generation
	enabled := q.state.shuffleOrig != nil
	q.mu.Unlock()
	q.changed()
	q.start(func(ctx context.Context) { q.prime(ctx, gen) })
	return enabled
}

// SetRepeat changes repeat behavior.
func (q *Queue) SetRepeat(mode model.RepeatMode) error {
	if mode != model.RepeatOff && mode != model.RepeatAll && mode != model.RepeatOne {
		return errors.New("invalid repeat mode")
	}
	q.mu.Lock()
	q.state.repeat = mode
	disarmed := mode == model.RepeatOne && q.stopAfterCurrent
	if disarmed {
		q.stopAfterCurrent = false
	}
	q.mu.Unlock()
	if disarmed {
		q.bus.Emit(protocol.EventStopAfterCurrent, false)
	}
	q.changed()
	return nil
}

// SetStopAfterCurrent arms or disarms the one-shot EOF stop.
func (q *Queue) SetStopAfterCurrent(enabled bool) {
	q.mu.Lock()
	q.stopAfterCurrent = enabled
	if enabled && q.state.repeat == model.RepeatOne {
		q.state.repeat = model.RepeatOff
	}
	q.mu.Unlock()
	q.bus.Emit(protocol.EventStopAfterCurrent, enabled)
	q.changed()
}

// the host sends its position several times a second; seeking on each one would stutter
const followDriftSeconds = 2.0

// Follow brings playback to track at position, the way a Listen Together guest follows its host. A
// track already open is never reloaded, since that restarts the song and counts another play, and it
// seeks only when it drifted. A paused host opens the track paused.
func (q *Queue) Follow(ctx context.Context, track model.Track, position float64, playing bool) error {
	if !q.isLoaded(track.VideoID) {
		if err := q.play(ctx, track, !playing); err != nil {
			return err
		}
		if position > 0 {
			if err := q.Seek(position); err != nil {
				return err
			}
		}
	} else if math.Abs(q.Playback().Position-position) > followDriftSeconds {
		if err := q.Seek(position); err != nil {
			return err
		}
	}
	if !playing {
		return q.Pause()
	}
	return q.PlayTransport(ctx)
}

func (q *Queue) isLoaded(videoID string) bool {
	if q.player.Idle() {
		return false
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.state.current >= 0 && q.state.current < len(q.state.items) && q.state.items[q.state.current].VideoID == videoID
}

// PlayTransport resumes playback, loading a restored queue first when needed.
func (q *Queue) PlayTransport(ctx context.Context) error {
	q.mu.Lock()
	has := len(q.state.items) > 0
	index := q.state.current
	q.mu.Unlock()
	if q.player.Idle() && has {
		return q.playIndex(ctx, index, false)
	}
	return q.player.Play()
}

// Pause pauses playback and flushes its restorable position.
func (q *Queue) Pause() error {
	if err := q.player.Pause(); err != nil {
		return err
	}
	q.persistPosition(true)
	return nil
}

// TogglePause toggles playback, loading a restored queue first when needed.
func (q *Queue) TogglePause(ctx context.Context) error {
	if q.player.Idle() {
		return q.PlayTransport(ctx)
	}
	return q.player.TogglePause()
}

// Stop stops playback.
func (q *Queue) Stop() error { return q.player.Stop() }

// Seek seeks to a finite non-negative position.
func (q *Queue) Seek(position float64) error {
	if math.IsNaN(position) || math.IsInf(position, 0) || position < 0 {
		return errors.New("Seek position must be a non-negative finite number.")
	}
	if err := q.player.Seek(position); err != nil {
		return err
	}
	q.mu.Lock()
	q.position = position
	q.mu.Unlock()
	q.bus.Emit(protocol.EventPosition, protocol.PositionEvent{Position: position})
	return nil
}

// SetVolume validates and applies a volume percentage.
func (q *Queue) SetVolume(percent int) error {
	if percent < 0 || percent > 100 {
		return errors.New("Volume must be between 0 and 100.")
	}
	if err := q.player.SetVolume(percent); err != nil {
		return err
	}
	q.mu.Lock()
	q.volume = percent
	q.mu.Unlock()
	if q.persist != nil {
		if err := q.persist.Set(volumeKey, strconv.Itoa(percent)); err != nil {
			q.log.Warn("persisting volume", "error", err)
		}
	}
	q.bus.Emit(protocol.EventVolume, percent)
	return nil
}
