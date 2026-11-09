package queue

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

func (q *Queue) restore() {
	if q.persist == nil {
		return
	}
	if raw, ok := q.persist.Get(volumeKey); ok {
		if value, err := strconv.Atoi(raw); err == nil && value >= 0 && value <= 100 {
			q.volume = value
		}
	}
	raw, ok := q.persist.Get(queueJSONKey)
	if !ok {
		return
	}
	var saved savedQueue
	if json.Unmarshal([]byte(raw), &saved) != nil || len(saved.Items) == 0 {
		return
	}
	if saved.Repeat != model.RepeatOff && saved.Repeat != model.RepeatAll && saved.Repeat != model.RepeatOne {
		saved.Repeat = model.RepeatOff
	}
	q.state = state{
		items: cloneTracks(saved.Items), current: clampIndex(saved.Current, len(saved.Items)),
		playedFrom: clampIndex(saved.PlayedFrom, len(saved.Items)), shuffleOrig: cloneTracks(saved.ShuffleOrig),
		repeat: saved.Repeat, radioSeed: saved.RadioSeed, sourceID: cloneString(saved.SourceID),
		sourceName: cloneString(saved.SourceName), radio: saved.Radio,
	}
	fp := fingerprint(saved)
	q.lastBlobFP = fp
	if rawIndex, ok := q.persist.Get(queueIndexKey); ok {
		var index savedIndex
		if json.Unmarshal([]byte(rawIndex), &index) == nil && index.Fingerprint == fp {
			q.state.current = clampIndex(index.Current, len(q.state.items))
			q.state.playedFrom = clampIndex(index.PlayedFrom, len(q.state.items))
			if index.Repeat == model.RepeatOff || index.Repeat == model.RepeatAll || index.Repeat == model.RepeatOne {
				q.state.repeat = index.Repeat
			}
		}
	}
	items, current, playedFrom := dropUnplayable(q.state.items, q.state.current, q.state.playedFrom)
	if len(items) == 0 {
		q.state = state{repeat: q.state.repeat}
		return
	}
	if current < 0 {
		q.state.items, q.state.current, q.state.playedFrom = items, 0, 0
		return
	}
	q.state.items, q.state.current, q.state.playedFrom = items, current, playedFrom
	if rawPosition, ok := q.persist.Get(queuePositionKey); ok {
		if position, err := strconv.ParseFloat(rawPosition, 64); err == nil && position > 0 {
			q.pendingSeek = position
			q.pendingSeekID = q.state.items[q.state.current].VideoID
			q.position = position
		}
	}
}

var errNothingToPlay = errors.New("nothing to play")

// dropUnplayable removes tracks without an id, which can never play, in place,
// and moves current and playedFrom with the tracks they pointed at; current is
// -1 when its own track was dropped.
func dropUnplayable(items []model.Track, current, playedFrom int) ([]model.Track, int, int) {
	kept := items[:0]
	mappedCurrent, mappedFrom := -1, 0
	for i, item := range items {
		if item.VideoID == "" {
			continue
		}
		if i == current {
			mappedCurrent = len(kept)
		}
		if i <= playedFrom {
			mappedFrom = len(kept)
		}
		kept = append(kept, item)
	}
	return kept, mappedCurrent, mappedFrom
}

func clampIndex(value, length int) int {
	if length <= 0 || value < 0 {
		return 0
	}
	return min(value, length-1)
}

func (q *Queue) persistState() {
	if q.persist == nil {
		return
	}
	q.mu.Lock()
	saved := savedQueue{
		Items: cloneTracks(q.state.items), Current: q.state.current, PlayedFrom: q.state.playedFrom,
		Repeat: q.state.repeat, ShuffleOrig: cloneTracks(q.state.shuffleOrig), RadioSeed: q.state.radioSeed,
		SourceID: cloneString(q.state.sourceID), SourceName: cloneString(q.state.sourceName), Radio: q.state.radio,
	}
	fp := fingerprint(saved)
	writeBlob := fp != q.lastBlobFP
	q.lastBlobFP = fp
	index := savedIndex{Fingerprint: fp, Current: q.state.current, PlayedFrom: q.state.playedFrom, Repeat: q.state.repeat}
	q.mu.Unlock()
	if writeBlob {
		if raw, err := json.Marshal(saved); err == nil {
			if err := q.persist.Set(queueJSONKey, string(raw)); err != nil {
				q.log.Warn("persisting queue", "error", err)
			}
		}
	}
	if raw, err := json.Marshal(index); err == nil {
		if err := q.persist.Set(queueIndexKey, string(raw)); err != nil {
			q.log.Warn("persisting queue index", "error", err)
		}
	}
}

func fingerprint(saved savedQueue) string {
	projection := struct {
		Items       []model.Track `json:"items"`
		ShuffleOrig []model.Track `json:"shuffleOrig,omitempty"`
		RadioSeed   string        `json:"radioSeed,omitempty"`
		SourceID    *string       `json:"sourceId,omitempty"`
		SourceName  *string       `json:"sourceName,omitempty"`
		Radio       bool          `json:"radio"`
	}{saved.Items, saved.ShuffleOrig, saved.RadioSeed, saved.SourceID, saved.SourceName, saved.Radio}
	raw, _ := json.Marshal(projection)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func (q *Queue) persistPosition(force bool) {
	if q.persist == nil {
		return
	}
	q.mu.Lock()
	if !force && q.now().Sub(q.lastPositionSave) < 5*time.Second {
		q.mu.Unlock()
		return
	}
	q.lastPositionSave = q.now()
	position := q.position
	q.mu.Unlock()
	if err := q.persist.Set(queuePositionKey, strconv.FormatFloat(position, 'f', -1, 64)); err != nil {
		q.log.Warn("persisting queue position", "error", err)
	}
}

func (q *Queue) snapshotLocked() model.QueueSnapshot {
	return model.QueueSnapshot{
		Items: cloneTracks(q.state.items), CurrentIndex: q.state.current, PlayedFrom: q.state.playedFrom,
		Shuffle: q.state.shuffleOrig != nil, Repeat: q.state.repeat, SourceName: cloneString(q.state.sourceName),
	}
}

func (q *Queue) indexLocked() model.QueueIndex {
	index := model.QueueIndex{
		CurrentIndex: q.state.current, PlayedFrom: q.state.playedFrom, Shuffle: q.state.shuffleOrig != nil,
		Repeat: q.state.repeat, SourceName: cloneString(q.state.sourceName),
	}
	if q.state.current >= 0 && q.state.current < len(q.state.items) {
		current := q.state.items[q.state.current]
		index.Current = &current
	}
	return index
}

func (q *Queue) nowPlayingLocked() *model.PlaybackTrack {
	if q.state.current < 0 || q.state.current >= len(q.state.items) {
		return nil
	}
	track := q.state.items[q.state.current]
	return &model.PlaybackTrack{
		VideoID: track.VideoID, Title: track.Title, Artists: track.Artists, ArtistID: track.ArtistID,
		ArtistRuns: append([]model.Artist(nil), track.ArtistRuns...), Thumbnail: track.Thumbnail,
		Duration: track.Duration, StreamClient: q.streamClient, Rating: track.Rating,
		Plays: track.Plays, Likes: track.Likes, Genre: track.Genre,
	}
}

func (q *Queue) playbackLocked() model.PlaybackSnapshot {
	return model.PlaybackSnapshot{
		Now: q.nowPlayingLocked(), Paused: !q.playing, Position: q.position,
		Duration: q.duration, Volume: q.volume, StopAfterCurrent: q.stopAfterCurrent,
	}
}

func (q *Queue) emitRestored() {
	q.mu.Lock()
	now := q.nowPlayingLocked()
	snapshot := q.snapshotLocked()
	q.mu.Unlock()
	if now != nil {
		q.bus.Emit(protocol.EventNowPlaying, now)
	}
	q.bus.Emit(protocol.EventQueueChanged, snapshot)
	q.bus.Emit(protocol.EventPlaybackState, protocol.PlaybackStatePaused)
}

func (q *Queue) changed() {
	q.emitChanged()
	q.persistState()
}

func (q *Queue) emitChanged() {
	q.mu.Lock()
	snapshot := q.snapshotLocked()
	q.mu.Unlock()
	q.bus.Emit(protocol.EventQueueChanged, snapshot)
}

func playbackState(paused bool) protocol.PlaybackState {
	if paused {
		return protocol.PlaybackStatePaused
	}
	return protocol.PlaybackStatePlaying
}
