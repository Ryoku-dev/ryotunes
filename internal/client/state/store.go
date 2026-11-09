package state

import (
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

// ToastKind identifies a transient client message treatment.
type ToastKind string

const (
	// ToastInfo is a neutral transient message.
	ToastInfo ToastKind = "info"
	// ToastSuccess is a successful-action message.
	ToastSuccess ToastKind = "success"
	// ToastError is an error message.
	ToastError ToastKind = "error"
)

// Toast is the latest transient message emitted by a reducer.
type Toast struct {
	Message string
	Kind    ToastKind
	Serial  uint64
}

// SpotifyState is Spotify's stable status plus transient authorization state.
type SpotifyState struct {
	model.SpotifyStatus
	Flow  string
	URL   string
	Error string
}

// SoundCloudState is SoundCloud's stable status plus transient authorization state.
type SoundCloudState struct {
	model.SoundCloudStatus
	Flow  string
	Error string
}

// TidalState is TIDAL's stable status plus device-flow state.
type TidalState struct {
	model.TidalStatus
	Code  string
	URL   string
	Error string
}

// State is the complete daemon mirror and client-only UI state.
type State struct {
	Playback   model.PlaybackSnapshot
	Queue      model.QueueSnapshot
	Settings   model.SettingsSnapshot
	Auth       model.Account
	AudioFx    model.AudioFx
	Provider   model.Provider
	Spotify    SpotifyState
	SoundCloud SoundCloudState
	Tidal      TidalState
	Downloads  model.DownloadSnapshot
	Personal   Personal
	LT         model.LTState
	Lastfm     model.LastfmState

	Rating         model.Rating
	LastError      string
	PendingVideoID string
	SeekDrag       *float64
	VolumeDragging bool
	Toast          *Toast
	ShowRequested  bool

	LibraryRevision  uint64
	LocalRevision    uint64
	LyricsRevision   uint64
	LocalRemoved     []string
	ChangedLibraryID string
	PersonalRevision uint64
	PersonalDirty    bool

	Client ClientPreferences
}

// QueueRow pairs a queue item with its daemon index.
type QueueRow struct {
	Index int
	Item  model.Track
}

// QueueGroups is the QueuePanel projection of now playing, manual future, and continuation rows.
type QueueGroups struct {
	NowPlaying   *QueueRow
	UpNextManual []QueueRow
	Continuation []QueueRow
}

// Store holds one synchronized client state and invalidation subscriptions.
type Store struct {
	mu    sync.RWMutex
	state State

	subscribers         map[uint64]func()
	nextSubscriber      uint64
	toastSerial         uint64
	positionSampledAt   time.Time
	realNowPlaying      bool
	authEventSeen       bool
	spotifyEventSeen    bool
	soundCloudEventSeen bool
	tidalEventSeen      bool
	lastNowID           string
	pendingPersonal     []json.RawMessage
}

// New returns a store seeded from a daemon subscription snapshot.
func New(snapshot protocol.Snapshot) *Store {
	store := &Store{subscribers: make(map[uint64]func())}
	store.state.Personal = EmptyPersonal()
	store.state.Client = DefaultClientPreferences()
	store.applySnapshotLocked(snapshot, time.Now())
	return store
}

// NewEmpty returns a store with canonical empty client state.
func NewEmpty() *Store { return New(protocol.Snapshot{}) }

// Snapshot returns a detached copy of the current top-level state.
func (s *Store) Snapshot() State {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneState(s.state)
}

// ApplySnapshot replaces daemon-owned state while preserving newer event updates.
func (s *Store) ApplySnapshot(snapshot protocol.Snapshot) {
	s.mu.Lock()
	s.applySnapshotLocked(snapshot, time.Now())
	callbacks := s.callbacksLocked()
	s.mu.Unlock()
	notify(callbacks)
}

// SetSetting updates one renderer-visible setting after a successful daemon write.
func (s *Store) SetSetting(key, value string) {
	s.mu.Lock()
	if s.state.Settings == nil {
		s.state.Settings = make(model.SettingsSnapshot)
	}
	s.state.Settings[key] = value
	callbacks := s.callbacksLocked()
	s.mu.Unlock()
	notify(callbacks)
}

// ReplaceSettings replaces the authoritative renderer-visible settings snapshot.
func (s *Store) ReplaceSettings(settings model.SettingsSnapshot) {
	s.mu.Lock()
	s.state.Settings = cloneSettings(settings)
	callbacks := s.callbacksLocked()
	s.mu.Unlock()
	notify(callbacks)
}

// Apply reduces one protocol event and invalidates subscribers.
func (s *Store) Apply(name string, data json.RawMessage) error {
	return s.ApplyAt(name, data, time.Now())
}

// ApplyAt is the clock-injected deterministic form of Apply.
func (s *Store) ApplyAt(name string, data json.RawMessage, now time.Time) error {
	s.mu.Lock()
	err := s.applyLocked(name, data, now)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	callbacks := s.callbacksLocked()
	s.mu.Unlock()
	notify(callbacks)
	return nil
}

// Subscribe registers a Gio invalidation hook and returns its unsubscribe function.
func (s *Store) Subscribe(invalidate func()) func() {
	if invalidate == nil {
		return func() {}
	}
	s.mu.Lock()
	s.nextSubscriber++
	id := s.nextSubscriber
	s.subscribers[id] = invalidate
	s.mu.Unlock()
	return func() {
		s.mu.Lock()
		delete(s.subscribers, id)
		s.mu.Unlock()
	}
}

// CurrentTrack returns the queue's current item when its index is valid.
func (s *Store) CurrentTrack() *model.Track {
	s.mu.RLock()
	defer s.mu.RUnlock()
	index := s.state.Queue.CurrentIndex
	if index < 0 || index >= len(s.state.Queue.Items) {
		return nil
	}
	track := s.state.Queue.Items[index]
	return &track
}

// QueueGroups returns the QueuePanel's three stable row groups.
func (s *Store) QueueGroups() QueueGroups {
	s.mu.RLock()
	defer s.mu.RUnlock()
	queue := s.state.Queue
	groups := QueueGroups{UpNextManual: []QueueRow{}, Continuation: []QueueRow{}}
	if queue.CurrentIndex >= 0 && queue.CurrentIndex < len(queue.Items) {
		groups.NowPlaying = &QueueRow{Index: queue.CurrentIndex, Item: queue.Items[queue.CurrentIndex]}
	}
	start := max(0, queue.CurrentIndex+1)
	for index := start; index < len(queue.Items); index++ {
		row := QueueRow{Index: index, Item: queue.Items[index]}
		if row.Item.Queued || row.Item.QueuedEnd {
			groups.UpNextManual = append(groups.UpNextManual, row)
		} else {
			groups.Continuation = append(groups.Continuation, row)
		}
	}
	return groups
}

// PositionAt returns the playback clock projected to now and clamped to duration.
func (s *Store) PositionAt(now time.Time) float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.state.SeekDrag != nil {
		return clampPosition(s.state.Playback.Duration, *s.state.SeekDrag)
	}
	position := s.state.Playback.Position
	if !s.state.Playback.Paused && !s.positionSampledAt.IsZero() {
		position += max(0, now.Sub(s.positionSampledAt).Seconds())
	}
	return clampPosition(s.state.Playback.Duration, position)
}

// SetSeekDrag holds or releases the client seek thumb.
func (s *Store) SetSeekDrag(position *float64) {
	s.mu.Lock()
	if position == nil {
		s.state.SeekDrag = nil
	} else {
		s.state.SeekDrag = new(*position)
	}
	callbacks := s.callbacksLocked()
	s.mu.Unlock()
	notify(callbacks)
}

// SetVolumeDragging suppresses stale volume echoes while the slider is held.
func (s *Store) SetVolumeDragging(dragging bool) {
	s.mu.Lock()
	s.state.VolumeDragging = dragging
	s.mu.Unlock()
}

// ConsumeShow reports and clears a daemon request to raise the main window.
func (s *Store) ConsumeShow() bool {
	s.mu.Lock()
	requested := s.state.ShowRequested
	s.state.ShowRequested = false
	s.mu.Unlock()
	return requested
}

// UpdatePersonal applies one pure personal reducer and marks the document for persistence.
func (s *Store) UpdatePersonal(update func(*Personal) bool) bool {
	if update == nil {
		return false
	}
	s.mu.Lock()
	personal := clonePersonal(s.state.Personal)
	if !update(&personal) {
		s.mu.Unlock()
		return false
	}
	s.state.Personal = personal
	s.markPersonalDirtyLocked()
	callbacks := s.callbacksLocked()
	s.mu.Unlock()
	notify(callbacks)
	return true
}

// MarkPersonalSave records a local personal write so its structurally equal echo is ignored.
func (s *Store) MarkPersonalSave(raw json.RawMessage) {
	copy := append(json.RawMessage(nil), raw...)
	s.mu.Lock()
	s.pendingPersonal = append(s.pendingPersonal, copy)
	s.state.PersonalDirty = false
	s.mu.Unlock()
}

func (s *Store) applySnapshotLocked(snapshot protocol.Snapshot, now time.Time) {
	playback := snapshot.Playback
	if s.realNowPlaying {
		playback.Now = s.state.Playback.Now
		if playback.Now != nil && s.state.Playback.Now != nil && playback.Now.VideoID == s.state.Playback.Now.VideoID {
			playback.Position = s.state.Playback.Position
			playback.Duration = s.state.Playback.Duration
		}
	}
	s.state.Playback = playback
	s.state.Queue = snapshot.Queue
	s.state.Settings = cloneSettings(snapshot.Settings)
	if !s.authEventSeen {
		s.state.Auth = snapshot.Auth
	}
	s.state.AudioFx = snapshot.AudioFx
	s.state.Provider = snapshot.Provider
	if !s.spotifyEventSeen {
		s.state.Spotify = SpotifyState{SpotifyStatus: snapshot.Spotify}
	}
	if !s.soundCloudEventSeen {
		s.state.SoundCloud = SoundCloudState{SoundCloudStatus: snapshot.SoundCloud}
	}
	if !s.tidalEventSeen {
		s.state.Tidal = TidalState{TidalStatus: snapshot.Tidal}
	}
	s.state.Downloads = snapshot.Downloads
	if !s.state.PersonalDirty && len(s.pendingPersonal) == 0 {
		s.state.Personal = HydratePersonal(snapshot.Personal)
		s.state.PersonalRevision++
	}
	s.state.LT = snapshot.LT
	if s.state.Playback.Now != nil && s.state.Playback.Now.Rating != nil {
		s.state.Rating = *s.state.Playback.Now.Rating
	} else {
		s.state.Rating = model.RatingIndifferent
	}
	s.positionSampledAt = now
	s.realNowPlaying = false
	s.authEventSeen = false
	s.spotifyEventSeen = false
	s.soundCloudEventSeen = false
	s.tidalEventSeen = false
}

func (s *Store) applyLocked(name string, data json.RawMessage, now time.Time) error {
	switch name {
	case protocol.EventPosition:
		var event protocol.PositionEvent
		if decode(data, &event) != nil {
			event.Position = 0
		}
		if s.state.SeekDrag == nil {
			s.state.Playback.Position = clampPosition(s.state.Playback.Duration, event.Position)
			s.positionSampledAt = now
		}
	case protocol.EventDuration:
		var event protocol.DurationEvent
		if err := decode(data, &event); err != nil {
			return err
		}
		if math.IsNaN(event.Duration) || math.IsInf(event.Duration, 0) || event.Duration < 0 {
			event.Duration = 0
		}
		s.state.Playback.Duration = event.Duration
		s.state.Playback.Position = clampPosition(event.Duration, s.state.Playback.Position)
	case protocol.EventPlaybackState:
		var event protocol.PlaybackState
		if err := decode(data, &event); err != nil {
			return err
		}
		if event == protocol.PlaybackStatePaused && !s.state.Playback.Paused {
			s.state.Playback.Position = s.projectedPositionLocked(now)
		}
		s.state.Playback.Paused = event == protocol.PlaybackStatePaused
		s.positionSampledAt = now
	case protocol.EventPlaybackError:
		var event protocol.MessageEvent
		if err := decode(data, &event); err != nil {
			return err
		}
		s.state.LastError = event.Message
		s.state.PendingVideoID = ""
		s.setToastLocked(event.Message, ToastError)
	case protocol.EventPlaybackNotice:
		var event protocol.MessageEvent
		if err := decode(data, &event); err != nil {
			return err
		}
		s.setToastLocked(event.Message, ToastInfo)
	case protocol.EventNowPlaying:
		var event *model.PlaybackTrack
		if err := decode(data, &event); err != nil {
			return err
		}
		changed := event == nil || s.state.Playback.Now == nil || event.VideoID != s.state.Playback.Now.VideoID
		s.state.Playback.Now = event
		if changed {
			s.state.Playback.Position = 0
			s.state.Playback.Duration = playbackTrackDuration(event)
			s.positionSampledAt = now
		}
		s.state.PendingVideoID = ""
		s.state.LastError = ""
		s.state.Rating = model.RatingIndifferent
		if event != nil && event.Rating != nil {
			s.state.Rating = *event.Rating
		}
		s.realNowPlaying = true
		if RecordNowPlaying(&s.state.Personal, event, &s.lastNowID, now.UnixMilli()) {
			s.markPersonalDirtyLocked()
		}
	case protocol.EventQueueChanged:
		if err := decode(data, &s.state.Queue); err != nil {
			return err
		}
	case protocol.EventQueueIndex:
		var event model.QueueIndex
		if err := decode(data, &event); err != nil {
			return err
		}
		if event.Current != nil && event.CurrentIndex >= 0 && event.CurrentIndex < len(s.state.Queue.Items) {
			s.state.Queue.Items[event.CurrentIndex] = *event.Current
		}
		s.state.Queue.CurrentIndex = event.CurrentIndex
		s.state.Queue.PlayedFrom = event.PlayedFrom
		s.state.Queue.Shuffle = event.Shuffle
		s.state.Queue.Repeat = event.Repeat
		s.state.Queue.SourceName = event.SourceName
	case protocol.EventRating:
		var event protocol.RatingEvent
		if err := decode(data, &event); err != nil {
			return err
		}
		if s.state.Playback.Now != nil && s.state.Playback.Now.VideoID == event.VideoID {
			s.state.Rating = event.Rating
		}
	case protocol.EventStopAfterCurrent:
		if err := decode(data, &s.state.Playback.StopAfterCurrent); err != nil {
			return err
		}
	case protocol.EventVolume:
		if s.state.VolumeDragging {
			return nil
		}
		if err := decode(data, &s.state.Playback.Volume); err != nil {
			return err
		}
	case protocol.EventAudioFx:
		if err := decode(data, &s.state.AudioFx); err != nil {
			return err
		}
	case protocol.EventShow:
		s.state.ShowRequested = true
	case protocol.EventAuthChanged:
		oldSignedIn := s.state.Auth.SignedIn
		if err := decode(data, &s.state.Auth); err != nil {
			return err
		}
		if oldSignedIn && !s.state.Auth.SignedIn && ForgetYouTubeRecents(&s.state.Personal) > 0 {
			s.markPersonalDirtyLocked()
		}
		s.authEventSeen = true
	case protocol.EventSpotifyAuth:
		var event model.SpotifyAuthEvent
		if err := decode(data, &event); err != nil {
			return err
		}
		s.reduceSpotifyLocked(event)
		s.spotifyEventSeen = true
	case protocol.EventSoundCloudAuth:
		var event model.SoundCloudAuthEvent
		if err := decode(data, &event); err != nil {
			return err
		}
		s.reduceSoundCloudLocked(event)
		s.soundCloudEventSeen = true
	case protocol.EventTidalAuth:
		var event model.TidalAuthEvent
		if err := decode(data, &event); err != nil {
			return err
		}
		s.reduceTidalLocked(event)
		s.tidalEventSeen = true
	case protocol.EventProviderChanged:
		var event protocol.ProviderResult
		if err := decode(data, &event); err != nil {
			return err
		}
		s.state.Provider = event.Provider
	case protocol.EventLibraryChanged:
		var event protocol.LibraryChangedEvent
		if err := decode(data, &event); err != nil {
			return err
		}
		s.state.LibraryRevision++
		s.state.ChangedLibraryID = event.ID
	case protocol.EventLocalChanged:
		var event model.LocalChanged
		if err := decode(data, &event); err != nil {
			return err
		}
		s.state.LocalRevision++
		s.state.LocalRemoved = append([]string(nil), event.Removed...)
	case protocol.EventPersonalChanged:
		if s.consumePersonalEchoLocked(data) {
			return nil
		}
		if s.state.PersonalDirty || len(s.pendingPersonal) != 0 {
			return nil
		}
		s.state.Personal = HydratePersonal(data)
		s.state.PersonalRevision++
		s.state.PersonalDirty = false
	case protocol.EventLyricsChanged:
		s.state.LyricsRevision++
	case protocol.EventCoverError:
		var event protocol.MessageEvent
		if err := decode(data, &event); err != nil {
			return err
		}
		s.setToastLocked(event.Message, ToastError)
	case protocol.EventLastfmState:
		if err := decode(data, &s.state.Lastfm); err != nil {
			return err
		}
	case protocol.EventLTState:
		if err := decode(data, &s.state.LT); err != nil {
			return err
		}
	case protocol.EventLTNotice:
		var event model.LTNotice
		if err := decode(data, &event); err != nil {
			return err
		}
		s.setToastLocked(string(event), ToastInfo)
	case protocol.EventDownloadsChanged:
		if err := decode(data, &s.state.Downloads); err != nil {
			return err
		}
	default:
		return errors.New("unknown protocol event: " + name)
	}
	return nil
}

func (s *Store) reduceSpotifyLocked(event model.SpotifyAuthEvent) {
	state := &s.state.Spotify
	switch event.State {
	case "url":
		state.Flow, state.URL, state.Error = "browser", stringValue(event.URL), ""
		s.setToastLocked("Opening Spotify sign-in in your browser", ToastInfo)
	case "signed_in":
		wasSignedIn := state.SignedIn
		state.SignedIn, state.Stored, state.Name, state.Flow, state.URL, state.Error = true, true, event.Name, "", "", ""
		if !wasSignedIn {
			message := "Signed in to Spotify"
			if event.Name != nil {
				message += " as " + *event.Name
			}
			s.setToastLocked(message, ToastSuccess)
		}
	case "restored":
		state.SignedIn, state.Stored, state.Premium, state.Flow, state.URL, state.Error = true, true, new(true), "", "", ""
	case "restore_failed":
		wasSignedIn := state.SignedIn
		state.SignedIn, state.Stored, state.Flow, state.URL = false, true, "", ""
		state.Error = eventMessage(event.Message, "Spotify sign-in no longer works")
		if wasSignedIn && ForgetSpotifyRecents(&s.state.Personal) > 0 {
			s.markPersonalDirtyLocked()
		}
	case "signed_out":
		wasSignedIn := state.SignedIn
		*state = SpotifyState{}
		if wasSignedIn && ForgetSpotifyRecents(&s.state.Personal) > 0 {
			s.markPersonalDirtyLocked()
		}
	case "error":
		state.Flow, state.URL = "", ""
		state.Error = eventMessage(event.Message, "Spotify sign-in failed")
		s.setToastLocked(state.Error, ToastError)
	}
}

func (s *Store) reduceSoundCloudLocked(event model.SoundCloudAuthEvent) {
	state := &s.state.SoundCloud
	switch event.State {
	case "signed_in":
		state.SignedIn, state.Name, state.Flow, state.Error = true, event.Name, "", ""
		message := "Signed in to SoundCloud"
		if event.Name != nil {
			message += " as " + *event.Name
		}
		s.setToastLocked(message, ToastSuccess)
	case "signed_out":
		*state = SoundCloudState{}
	case "waiting":
		state.SignedIn, state.Name, state.Flow, state.Error = false, nil, "waiting", ""
		s.setToastLocked("Opened soundcloud.com in your browser, sign in there and Ryotunes will connect", ToastInfo)
	case "expired":
		state.SignedIn, state.Name, state.Flow, state.Error = false, nil, "expired", ""
		s.setToastLocked("No SoundCloud sign-in was completed in the browser", ToastInfo)
	case "restore_failed", "error":
		state.SignedIn, state.Name, state.Flow = false, nil, ""
		state.Error = eventMessage(event.Message, "SoundCloud sign-in failed")
		s.setToastLocked(state.Error, ToastError)
	}
}

func (s *Store) reduceTidalLocked(event model.TidalAuthEvent) {
	state := &s.state.Tidal
	switch event.State {
	case "signed_in":
		state.SignedIn, state.Name, state.HasCredentials = true, event.Name, true
		if event.Quality != nil {
			state.Quality = event.Quality
		}
		state.Code, state.URL, state.Error = "", "", ""
		message := "Signed in to TIDAL"
		if event.Name != nil {
			message += " as " + *event.Name
		}
		s.setToastLocked(message, ToastSuccess)
	case "signed_out":
		hasCredentials := state.HasCredentials
		*state = TidalState{TidalStatus: model.TidalStatus{HasCredentials: hasCredentials}}
	case "waiting":
		state.SignedIn, state.Name, state.Quality, state.HasCredentials = false, nil, nil, true
		state.Code, state.URL, state.Error = stringValue(event.Code), stringValue(event.URL), ""
		code, target := state.Code, state.URL
		if code == "" {
			code = "the code"
		}
		if target == "" {
			target = "link.tidal.com"
		}
		s.setToastLocked("Enter "+code+" at "+target+" to finish signing in", ToastInfo)
	case "expired":
		state.SignedIn, state.Name, state.Quality, state.Code, state.URL, state.Error = false, nil, nil, "", "", ""
		state.HasCredentials = true
		s.setToastLocked("The TIDAL code expired before it was approved", ToastInfo)
	default:
		state.SignedIn, state.Name, state.Quality, state.Code, state.URL = false, nil, nil, "", ""
		state.Error = stringValue(event.Message)
		if state.Error != "" {
			s.setToastLocked(state.Error, ToastError)
		}
	}
}

func (s *Store) projectedPositionLocked(now time.Time) float64 {
	position := s.state.Playback.Position
	if !s.state.Playback.Paused && !s.positionSampledAt.IsZero() {
		position += max(0, now.Sub(s.positionSampledAt).Seconds())
	}
	return clampPosition(s.state.Playback.Duration, position)
}

func (s *Store) setToastLocked(message string, kind ToastKind) {
	s.toastSerial++
	s.state.Toast = &Toast{Message: message, Kind: kind, Serial: s.toastSerial}
}

func (s *Store) callbacksLocked() []func() {
	callbacks := make([]func(), 0, len(s.subscribers))
	for _, callback := range s.subscribers {
		callbacks = append(callbacks, callback)
	}
	return callbacks
}

func (s *Store) consumePersonalEchoLocked(raw json.RawMessage) bool {
	for i, pending := range s.pendingPersonal {
		if jsonStructurallyEqual(raw, pending) {
			s.pendingPersonal = append(s.pendingPersonal[:i], s.pendingPersonal[i+1:]...)
			return true
		}
	}
	return false
}

func (s *Store) markPersonalDirtyLocked() {
	s.state.PersonalRevision++
	s.state.PersonalDirty = true
}

func notify(callbacks []func()) {
	for _, callback := range callbacks {
		callback()
	}
}

func decode(raw json.RawMessage, target any) error {
	if len(raw) == 0 {
		raw = json.RawMessage("null")
	}
	return json.Unmarshal(raw, target)
}

func cloneState(state State) State {
	state.Settings = cloneSettings(state.Settings)
	state.Queue.Items = append([]model.QueueItem(nil), state.Queue.Items...)
	state.Downloads.Jobs = append([]model.DownloadEntry(nil), state.Downloads.Jobs...)
	state.LocalRemoved = append([]string(nil), state.LocalRemoved...)
	state.Personal = clonePersonal(state.Personal)
	return state
}

func cloneSettings(settings model.SettingsSnapshot) model.SettingsSnapshot {
	clone := make(model.SettingsSnapshot, len(settings))
	for key, value := range settings {
		clone[key] = value
	}
	return clone
}

func clonePersonal(personal Personal) Personal {
	clone := personal
	clone.Picks = append([]Pick(nil), personal.Picks...)
	clone.Saved = append([]model.BrowseItem(nil), personal.Saved...)
	clone.Pins = append([]string(nil), personal.Pins...)
	clone.DismissedSeeds = append([]string(nil), personal.DismissedSeeds...)
	clone.Home.Order = append([]string(nil), personal.Home.Order...)
	clone.Home.Hidden = append([]string(nil), personal.Home.Hidden...)
	clone.Home.Seen = append([]string(nil), personal.Home.Seen...)
	clone.Recent = make(map[string]RecentItem, len(personal.Recent))
	for id, item := range personal.Recent {
		clone.Recent[id] = item
	}
	clone.Artists = make(map[string]ArtistStat, len(personal.Artists))
	for id, artist := range personal.Artists {
		clone.Artists[id] = artist
	}
	return clone
}

func playbackTrackDuration(track *model.PlaybackTrack) float64 {
	if track == nil || track.Duration == nil {
		return 0
	}
	parts := strings.Split(*track.Duration, ":")
	seconds := 0.0
	for _, part := range parts {
		value, err := strconv.ParseFloat(part, 64)
		if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
			return 0
		}
		seconds = seconds*60 + value
	}
	return max(0, seconds)
}

func clampPosition(duration, position float64) float64 {
	if math.IsNaN(position) || math.IsInf(position, 0) || position < 0 {
		position = 0
	}
	if duration > 0 && position > duration {
		return duration
	}
	return position
}

func jsonStructurallyEqual(left, right json.RawMessage) bool {
	var l, r any
	if json.Unmarshal(left, &l) != nil || json.Unmarshal(right, &r) != nil {
		return false
	}
	return deepJSONEqual(l, r)
}

func deepJSONEqual(left, right any) bool {
	switch l := left.(type) {
	case []any:
		r, ok := right.([]any)
		if !ok || len(l) != len(r) {
			return false
		}
		for i := range l {
			if !deepJSONEqual(l[i], r[i]) {
				return false
			}
		}
		return true
	case map[string]any:
		r, ok := right.(map[string]any)
		if !ok || len(l) != len(r) {
			return false
		}
		for key, value := range l {
			other, exists := r[key]
			if !exists || !deepJSONEqual(value, other) {
				return false
			}
		}
		return true
	default:
		return left == right
	}
}

func eventMessage(message *string, fallback string) string {
	if message != nil && *message != "" {
		return *message
	}
	return fallback
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
