package model

// Quality selects stream quality.
type Quality string

const (
	// QualityLow favors lower bitrate streams.
	QualityLow Quality = "LOW"
	// QualityAuto lets the resolver balance quality and availability.
	QualityAuto Quality = "AUTO"
	// QualityHigh favors the best available audio stream.
	QualityHigh Quality = "HIGH"
)

// Stream is a provider-resolved playable resource.
type Stream struct {
	URL        string            `json:"url"`
	Headers    map[string]string `json:"headers,omitempty"`
	UserAgent  string            `json:"userAgent,omitempty"`
	ExpiresAt  *int64            `json:"expiresAt,omitempty"`
	Codec      string            `json:"codec,omitempty"`
	Bitrate    int               `json:"bitrate,omitempty"`
	LoudnessDB *float64          `json:"loudnessDb,omitempty"`
	IsLive     bool              `json:"isLive"`
}

// QueueItem is a track carrying queue-only metadata.
type QueueItem = Track

// QueueContext is the optional display label for tracks manually added from a collection.
type QueueContext string

// RepeatMode controls queue tail behavior.
type RepeatMode string

const (
	// RepeatOff stops at the queue tail unless autoplay extends it.
	RepeatOff RepeatMode = "off"
	// RepeatAll wraps from the queue tail to its beginning.
	RepeatAll RepeatMode = "all"
	// RepeatOne loops the current track.
	RepeatOne RepeatMode = "one"
)

// QueueSnapshot is the complete queue wire shape.
type QueueSnapshot struct {
	Items        []QueueItem `json:"items"`
	CurrentIndex int         `json:"currentIndex"`
	PlayedFrom   int         `json:"playedFrom"`
	Shuffle      bool        `json:"shuffle"`
	Repeat       RepeatMode  `json:"repeat"`
	SourceName   *string     `json:"sourceName"`
}

// QueueIndex is the compact queue pointer update emitted when rows are unchanged.
type QueueIndex struct {
	CurrentIndex int        `json:"currentIndex"`
	PlayedFrom   int        `json:"playedFrom"`
	Shuffle      bool       `json:"shuffle"`
	Repeat       RepeatMode `json:"repeat"`
	SourceName   *string    `json:"sourceName"`
	Current      *Track     `json:"current,omitempty"`
}

// PlaybackTrack is the camelCase now-playing projection.
type PlaybackTrack struct {
	VideoID      string   `json:"videoId"`
	Title        string   `json:"title"`
	Artists      string   `json:"artists"`
	ArtistID     *string  `json:"artistId"`
	ArtistRuns   []Artist `json:"artistRuns"`
	Thumbnail    *string  `json:"thumbnail"`
	Duration     *string  `json:"duration"`
	StreamClient string   `json:"streamClient"`
	Rating       *Rating  `json:"rating"`
	Plays        *string  `json:"plays,omitempty"`
	Likes        *string  `json:"likes,omitempty"`
	Genre        *string  `json:"genre,omitempty"`
}

// PlaybackSnapshot is the complete player wire snapshot.
type PlaybackSnapshot struct {
	Now              *PlaybackTrack `json:"now"`
	Paused           bool           `json:"paused"`
	Position         float64        `json:"position"`
	Duration         float64        `json:"duration"`
	Volume           int            `json:"volume"`
	StopAfterCurrent bool           `json:"stopAfterCurrent"`
}

// AudioFx is the five-field playback-effects state used by the daemon and client.
type AudioFx struct {
	Speed     float64 `json:"speed"`
	Semitones int     `json:"semitones"`
	Reverb    float64 `json:"reverb"`
	Bass      float64 `json:"bass"`
	Width     float64 `json:"width"`
}

// DefaultAudioFx returns the neutral effects preset.
func DefaultAudioFx() AudioFx {
	return AudioFx{Speed: 1}
}

// PresetName returns the named sound preset matching these values.
func (fx AudioFx) PresetName() string {
	switch fx {
	case AudioFx{Speed: 1}:
		return "Flat"
	case AudioFx{Speed: .85, Semitones: -2, Reverb: .35}:
		return "Slowed and reverb"
	case AudioFx{Speed: 1.25, Semitones: 3}:
		return "Nightcore"
	case AudioFx{Speed: 1, Bass: 6}:
		return "Bass boost"
	case AudioFx{Speed: 1, Width: 1}:
		return "Wide"
	default:
		return "Custom"
	}
}

// PlaybackParams changes tempo and pitch without altering the other effects bands.
type PlaybackParams struct {
	Speed     float64 `json:"speed"`
	Semitones int     `json:"semitones"`
}

// Waveform is a normalized 0–100 SoundCloud waveform.
type Waveform struct {
	Samples []int `json:"samples"`
}
