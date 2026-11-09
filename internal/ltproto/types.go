package ltproto

import "github.com/ryoku-dev/ryotunes/internal/model"

const (
	// ReconnectGraceSeconds is how long a disconnected session remains resumable.
	ReconnectGraceSeconds = 120
	// MaxUsersPerRoom includes both admitted and pending users.
	MaxUsersPerRoom = 50
	// MaxRooms is the maximum number of rooms retained by one relay.
	MaxRooms = 256
	// MaxQueueTracks bounds a shared queue.
	MaxQueueTracks = 1000
	// MaxSuggestionsPerRoom bounds pending guest suggestions.
	MaxSuggestionsPerRoom = 100
	// MaxMessageBytes bounds one complete WebSocket message.
	MaxMessageBytes = 512 * 1024
	// MaxFrameBytes bounds one WebSocket frame.
	MaxFrameBytes = 256 * 1024
	// OutboundQueueCapacity bounds messages waiting for one client.
	OutboundQueueCapacity = 128
	// MaxTrackIDBytes bounds a relayed track identifier.
	MaxTrackIDBytes = 256
	// MaxTrackTextBytes bounds relayed titles and artist names.
	MaxTrackTextBytes = 512
	// MaxThumbnailBytes bounds a relayed thumbnail URL.
	MaxThumbnailBytes = 2048
	// MaxUsernameRunes bounds a sanitized display name.
	MaxUsernameRunes = 50
)

// Track is the exact track shape exchanged by Listen Together peers.
type Track = model.LTTrack

// User is one participant in a shared room.
type User = model.LTUser

// RoomState is the relay's authoritative shared playback state.
type RoomState = model.LTRoom

// Suggestion is a guest track suggestion awaiting host handling.
type Suggestion = model.LTSuggestion

// PlaybackKind identifies a host playback action.
type PlaybackKind string

const (
	// PlaybackPlay starts playback at PositionMS.
	PlaybackPlay PlaybackKind = "play"
	// PlaybackPause pauses playback at PositionMS.
	PlaybackPause PlaybackKind = "pause"
	// PlaybackSeek moves playback to PositionMS.
	PlaybackSeek PlaybackKind = "seek"
	// PlaybackChangeTrack replaces the current track and optional queue.
	PlaybackChangeTrack PlaybackKind = "change_track"
	// PlaybackSyncQueue replaces the shared queue.
	PlaybackSyncQueue PlaybackKind = "sync_queue"
	// PlaybackSetVolume changes the shared volume scalar.
	PlaybackSetVolume PlaybackKind = "set_volume"
)

// Playback is the flat host-to-room playback action payload.
type Playback struct {
	Kind         PlaybackKind `json:"kind"`
	PositionMS   int64        `json:"position_ms"`
	ServerTimeMS int64        `json:"server_time_ms"`
	Track        *Track       `json:"track,omitempty"`
	Queue        *[]Track     `json:"queue,omitempty"`
	Playing      bool         `json:"playing"`
	Volume       float64      `json:"volume"`
}

// LivePositionMS extrapolates a room's current position at nowMS.
func LivePositionMS(state RoomState, nowMS int64) int64 {
	position := state.PositionMS
	if state.IsPlaying && state.LastUpdateMS > 0 && nowMS > state.LastUpdateMS {
		position += nowMS - state.LastUpdateMS
	}
	if position < 0 {
		return 0
	}
	return position
}
