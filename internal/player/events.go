package player

// EventKind identifies a player state transition.
type EventKind int

const (
	// FileLoaded reports that mpv finished opening the current stream.
	FileLoaded EventKind = iota
	// Position carries a throttled playback-position sample.
	Position
	// Duration carries a duration update.
	Duration
	// PauseChanged carries a real pause-state transition.
	PauseChanged
	// EndOfFile reports a normal end of the current stream.
	EndOfFile
	// Error reports a friendly playback failure.
	Error
	// IcyTitle carries an updated live-radio metadata title.
	IcyTitle
	// Idle reports that mpv has no active media.
	Idle
)

// Event is one event-driven player update.
type Event struct {
	// Kind identifies the update carried by this event.
	Kind EventKind
	// Position is the current playback position in seconds.
	Position float64
	// Duration is the current media duration in seconds.
	Duration float64
	// Paused is the new pause state for PauseChanged.
	Paused bool
	// Message is the friendly failure text for Error.
	Message string
	// Title is the live metadata text for IcyTitle.
	Title string
}
