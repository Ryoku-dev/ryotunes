package queue

import (
	"context"
	"log/slog"
	"math/rand/v2"
	"time"

	"github.com/ryoku-dev/ryotunes/internal/bus"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/player"
	"github.com/ryoku-dev/ryotunes/internal/provider"
)

const (
	queueJSONKey     = "queue_json"
	queueIndexKey    = "queue_index"
	queuePositionKey = "queue_position"
	volumeKey        = "volume"
	previousRestart  = 5 * time.Second
	autoplayBatch    = 20
	maxFillPages     = 50
)

// Persist is the queue's narrow settings-store dependency.
type Persist interface {
	Get(key string) (string, bool)
	Set(key, value string) error
}

// Hooks connects queue policy to daemon-owned downloads, history, stream cache, and settings.
type Hooks interface {
	DownloadedStream(trackID string) (model.Stream, bool)
	OnPlayed(ctx context.Context, track model.Track)
	OnPlaybackStart(ctx context.Context, track model.Track, playlistID string)
	StreamCacheGet(trackID string) (model.Stream, bool)
	StreamCachePut(trackID string, stream model.Stream)
	Quality() model.Quality
	Autoplay() bool
	PreventDuplicates() bool
	HealIdentity() bool
}

// Transport is the player surface used by the queue.
type Transport interface {
	Load(ctx context.Context, stream model.Stream, startAt float64, paused bool) error
	Prefetch(stream model.Stream)
	Play() error
	Pause() error
	TogglePause() error
	Stop() error
	Seek(position float64) error
	SetVolume(percent int) error
	SetLoudness(db *float64)
	Position() float64
	Duration() float64
	Paused() bool
	Idle() bool
	Events() <-chan player.Event
}

// Options supplies queue dependencies.
type Options struct {
	Player   Transport
	Registry *provider.Registry
	Persist  Persist
	Hooks    Hooks
	Bus      bus.Sink
	Log      *slog.Logger
	Rand     *rand.Rand
	Now      func() time.Time
}

type state struct {
	items       []model.Track
	current     int
	playedFrom  int
	shuffleOrig []model.Track
	repeat      model.RepeatMode
	sourceID    *string
	sourceName  *string
	radioSeed   string
	radio       bool
}

type savedQueue struct {
	Items       []model.Track    `json:"items"`
	Current     int              `json:"current"`
	PlayedFrom  int              `json:"playedFrom"`
	Repeat      model.RepeatMode `json:"repeat"`
	ShuffleOrig []model.Track    `json:"shuffleOrig,omitempty"`
	RadioSeed   string           `json:"radioSeed,omitempty"`
	SourceID    *string          `json:"sourceId,omitempty"`
	SourceName  *string          `json:"sourceName,omitempty"`
	Radio       bool             `json:"radio"`
}

type savedIndex struct {
	Fingerprint string           `json:"fp"`
	Current     int              `json:"current"`
	PlayedFrom  int              `json:"playedFrom"`
	Repeat      model.RepeatMode `json:"repeat"`
}
