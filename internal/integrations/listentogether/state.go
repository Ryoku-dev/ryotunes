package listentogether

import (
	"context"
	"strconv"
	"strings"

	"github.com/ryoku-dev/ryotunes/internal/ltproto"
	"github.com/ryoku-dev/ryotunes/internal/model"
)

// PlaybackSnapshot is the local host playback state used to seed and update a room.
type PlaybackSnapshot struct {
	Track      *model.Track
	Queue      []model.Track
	Playing    bool
	PositionMS int64
	Volume     float64
}

// Host is the narrow daemon boundary needed by Listen Together.
type Host interface {
	PlayTrack(ctx context.Context, track model.Track, positionMS int64, playing bool) error
	Pause(ctx context.Context) error
	Resume(ctx context.Context) error
	Seek(ctx context.Context, positionMS int64) error
	Enqueue(ctx context.Context, track model.Track, guestBoundary bool) error
	PlaybackSnapshot() PlaybackSnapshot
	Username() string
}

func roomTrack(track model.Track) ltproto.Track {
	return ltproto.Track{
		ID:         track.VideoID,
		Title:      track.Title,
		Artist:     track.Artists,
		Thumbnail:  track.Thumbnail,
		DurationMS: durationMS(track.Duration),
		QueuedBy:   track.QueuedBy,
	}
}

func modelTrack(track ltproto.Track) model.Track {
	return model.Track{
		VideoID:   track.ID,
		Title:     track.Title,
		Artists:   track.Artist,
		Thumbnail: track.Thumbnail,
		QueuedBy:  track.QueuedBy,
	}
}

func roomTracks(tracks []model.Track) []ltproto.Track {
	result := make([]ltproto.Track, len(tracks))
	for index, track := range tracks {
		result[index] = roomTrack(track)
	}
	return result
}

func durationMS(duration *string) int64 {
	if duration == nil {
		return 0
	}
	parts := strings.Split(*duration, ":")
	if len(parts) < 1 || len(parts) > 3 {
		return 0
	}
	var seconds int64
	for index, part := range parts {
		value, err := strconv.ParseInt(part, 10, 64)
		if err != nil || value < 0 || index > 0 && value >= 60 {
			return 0
		}
		seconds = seconds*60 + value
	}
	return seconds * 1000
}

func cloneState(state model.LTState) model.LTState {
	state.Users = append([]model.LTUser(nil), state.Users...)
	state.Queue = append([]model.LTTrack(nil), state.Queue...)
	state.PendingJoins = append([]model.LTPendingJoin(nil), state.PendingJoins...)
	state.Suggestions = append([]model.LTSuggestion(nil), state.Suggestions...)
	if state.RoomCode != nil {
		value := *state.RoomCode
		state.RoomCode = &value
	}
	if state.MyID != nil {
		value := *state.MyID
		state.MyID = &value
	}
	if state.CurrentTrack != nil {
		value := *state.CurrentTrack
		state.CurrentTrack = &value
	}
	return state
}
