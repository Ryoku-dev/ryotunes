package queue

import (
	"context"
	"errors"
	"math"

	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/player"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
	"github.com/ryoku-dev/ryotunes/internal/provider"
)

func (q *Queue) pump() {
	for {
		select {
		case <-q.closed:
			return
		case event, ok := <-q.player.Events():
			if !ok {
				return
			}
			q.handlePlayerEvent(event)
		}
	}
}

func (q *Queue) handlePlayerEvent(event player.Event) {
	switch event.Kind {
	case player.FileLoaded:
		q.mu.Lock()
		if q.pendingStart != nil {
			q.pendingStart.loaded = true
		}
		q.mu.Unlock()
		q.updatePlaying()
		q.reportStart()
	case player.Position:
		position := event.Position
		if math.IsNaN(position) || math.IsInf(position, 0) || position < 0 {
			position = 0
		}
		q.mu.Lock()
		q.position = position
		duration := q.duration
		callPlayed := !q.playedRecorded && position >= playedThreshold(duration) && q.state.current >= 0 && q.state.current < len(q.state.items)
		var track model.Track
		if callPlayed {
			q.playedRecorded = true
			track = q.state.items[q.state.current]
		}
		q.mu.Unlock()
		q.bus.Emit(protocol.EventPosition, protocol.PositionEvent{Position: position})
		q.persistPosition(false)
		if callPlayed && q.hooks != nil {
			q.start(func(ctx context.Context) { q.hooks.OnPlayed(ctx, track) })
		}
	case player.Duration:
		duration := event.Duration
		if math.IsNaN(duration) || math.IsInf(duration, 0) || duration < 0 {
			duration = 0
		}
		q.mu.Lock()
		q.duration = duration
		q.mu.Unlock()
		q.bus.Emit(protocol.EventDuration, protocol.DurationEvent{Duration: duration})
	case player.PauseChanged, player.Idle:
		q.updatePlaying()
		q.reportStart()
		if q.player.Paused() {
			q.persistPosition(true)
			q.mu.Lock()
			position := q.position
			q.mu.Unlock()
			q.bus.Emit(protocol.EventPosition, protocol.PositionEvent{Position: position})
		}
	case player.EndOfFile:
		q.handleEOF()
	case player.Error:
		q.start(func(ctx context.Context) { q.recoverPlayerFailure(ctx, event.Message) })
	case player.IcyTitle:
		if event.Title == "" {
			return
		}
		q.mu.Lock()
		if q.state.current >= 0 && q.state.current < len(q.state.items) {
			q.state.items[q.state.current].Title = event.Title
		}
		now := q.nowPlayingLocked()
		q.mu.Unlock()
		q.bus.Emit(protocol.EventNowPlaying, now)
	}
}

// playbackStart is a play the queue asked for that has not been reported yet.
type playbackStart struct {
	track      model.Track
	playlistID string
	loaded     bool
}

// reportStart reports the pending play once its file loaded and it is actually playing,
// so a track skipped before it loads, or opened paused and never played, is never reported.
func (q *Queue) reportStart() {
	if q.hooks == nil {
		return
	}
	q.mu.Lock()
	start := q.pendingStart
	if start == nil || !start.loaded || !q.playing {
		q.mu.Unlock()
		return
	}
	q.pendingStart = nil
	q.mu.Unlock()
	q.start(func(ctx context.Context) { q.hooks.OnPlaybackStart(ctx, start.track, start.playlistID) })
}

func (q *Queue) updatePlaying() {
	playing := !q.player.Paused() && !q.player.Idle()
	q.mu.Lock()
	if q.playing == playing {
		q.mu.Unlock()
		return
	}
	q.playing = playing
	q.mu.Unlock()
	q.bus.Emit(protocol.EventPlaybackState, playbackState(!playing))
}

func (q *Queue) handleEOF() {
	q.mu.Lock()
	if q.stopAfterCurrent {
		q.stopAfterCurrent = false
		q.playing = false
		q.mu.Unlock()
		_ = q.player.Stop()
		q.bus.Emit(protocol.EventStopAfterCurrent, false)
		q.bus.Emit(protocol.EventPlaybackState, protocol.PlaybackStatePaused)
		q.persistState()
		return
	}
	index, ok := q.nextIndexLocked(false)
	q.mu.Unlock()
	if ok {
		_ = q.playIndex(q.ctx, index, false)
		return
	}
	if q.extendAutoplay(q.ctx) > 0 {
		q.mu.Lock()
		index = q.state.current + 1
		q.mu.Unlock()
		_ = q.playIndex(q.ctx, index, false)
		return
	}
	_ = q.player.Stop()
	q.mu.Lock()
	q.playing = false
	q.mu.Unlock()
	q.bus.Emit(protocol.EventPlaybackState, protocol.PlaybackStatePaused)
}

func playedThreshold(duration float64) float64 {
	if duration <= 0 {
		return 30
	}
	return min(duration/2, 30)
}

func (q *Queue) recoverPlayerFailure(ctx context.Context, message string) {
	q.mu.Lock()
	if q.state.current < 0 || q.state.current >= len(q.state.items) {
		q.mu.Unlock()
		q.bus.Emit(protocol.EventPlaybackError, protocol.MessageEvent{Message: message})
		return
	}
	index := q.state.current
	track := q.state.items[index]
	generation := q.generation
	position := q.position
	retry := q.retriedTrackID != track.VideoID
	if retry {
		q.retriedTrackID = track.VideoID
	}
	q.mu.Unlock()
	q.log.Info("player reported playback failure", "track_id", track.VideoID, "title", track.Title, "error", message)
	if retry {
		stream, _, err := q.resolveFresh(ctx, track)
		if errors.Is(err, provider.ErrSignInRequired) {
			_ = q.player.Pause()
			q.bus.Emit(protocol.EventPlaybackError, protocol.MessageEvent{Message: signInMessage(model.ProviderOf(track.VideoID))})
			return
		}
		if err == nil {
			q.mu.Lock()
			valid := generation == q.generation && index == q.state.current
			q.mu.Unlock()
			if valid {
				q.player.SetLoudness(stream.LoudnessDB)
				if q.player.Load(ctx, stream, position, false) == nil {
					return
				}
			}
		}
	}
	q.bus.Emit(protocol.EventPlaybackError, protocol.MessageEvent{Message: message})
	q.mu.Lock()
	hasNext := generation == q.generation && index+1 < len(q.state.items)
	q.mu.Unlock()
	if hasNext {
		_ = q.playIndex(ctx, index+1, false)
		return
	}
	_ = q.player.Stop()
}
