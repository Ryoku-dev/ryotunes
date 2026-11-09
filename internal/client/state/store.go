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

// AuthFlow is one provider's browser handoff state.
type AuthFlow struct {
	Waiting bool
	URL     string
	Code    string
	Error   string
}

// State is the complete daemon mirror and client-only UI state.
type State struct {
	Playback   model.PlaybackSnapshot
	Queue      model.QueueSnapshot
	Settings   model.SettingsSnapshot
	Auth       model.Account
	AudioFx    model.AudioFx
	Equalizer  model.Equalizer
	Provider   model.Provider
	Spotify    SpotifyState
	SoundCloud SoundCloudState
	Tidal      TidalState
	AuthFlows  map[model.Provider]AuthFlow
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
	mu       sync.RWMutex
	state    State
	snapshot State

	subscribers          map[uint64]func()
	nextSubscriber       uint64
	toastSerial          uint64
	positionSampledAt    time.Time
	positionSamplePaused bool
	positionSampleSpeed  float64
	realNowPlaying       bool
	authEventSeen        bool
	spotifyEventSeen     bool
	soundCloudEventSeen  bool
	tidalEventSeen       bool
	lastNowID            string
	pendingPersonal      []json.RawMessage
	lastOpenedAuthURL    map[model.Provider]string
}

// New returns a store seeded from a daemon subscription snapshot.
func New(snapshot protocol.Snapshot) *Store {
	store := &Store{
		subscribers:       make(map[uint64]func()),
		lastOpenedAuthURL: make(map[model.Provider]string),
	}
	store.state.Personal = EmptyPersonal()
	store.state.Client = DefaultClientPreferences()
	store.state.AuthFlows = make(map[model.Provider]AuthFlow)
	store.applySnapshotLocked(snapshot, time.Now())
	store.publishLocked()
	return store
}

// NewEmpty returns a store with canonical empty client state.
func NewEmpty() *Store { return New(protocol.Snapshot{}) }

// Snapshot returns the immutable state published by the latest reducer revision.
func (s *Store) Snapshot() State {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.snapshot
}

// ApplySnapshot replaces daemon-owned state while preserving newer event updates.
func (s *Store) ApplySnapshot(snapshot protocol.Snapshot) {
	s.mu.Lock()
	s.applySnapshotLocked(snapshot, time.Now())
	s.publishLocked()
	callbacks := s.callbacksLocked()
	s.mu.Unlock()
	notify(callbacks)
}

// SetSetting updates one renderer-visible setting after a successful daemon write.
func (s *Store) SetSetting(key, value string) {
	s.mu.Lock()
	settings := cloneSettings(s.state.Settings)
	settings[key] = value
	s.state.Settings = settings
	s.publishLocked()
	callbacks := s.callbacksLocked()
	s.mu.Unlock()
	notify(callbacks)
}

// ReplaceSettings replaces the authoritative renderer-visible settings snapshot.
func (s *Store) ReplaceSettings(settings model.SettingsSnapshot) {
	s.mu.Lock()
	s.state.Settings = cloneSettings(settings)
	s.publishLocked()
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
	if name == protocol.EventPosition {
		invalidate := s.applyPositionLocked(data, now)
		s.snapshot.Playback.Position = s.state.Playback.Position
		var callbacks []func()
		if invalidate {
			callbacks = s.callbacksLocked()
		}
		s.mu.Unlock()
		notify(callbacks)
		return nil
	}
	err := s.applyLocked(name, data, now)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	s.publishLocked()
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
	index := s.snapshot.Queue.CurrentIndex
	if index < 0 || index >= len(s.snapshot.Queue.Items) {
		return nil
	}
	track := s.snapshot.Queue.Items[index]
	return &track
}

// QueueGroups returns the QueuePanel's three stable row groups.
func (s *Store) QueueGroups() QueueGroups {
	s.mu.RLock()
	defer s.mu.RUnlock()
	queue := s.snapshot.Queue
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
	return s.positionAtLocked(now)
}

// SetSeekDrag holds or releases the client seek thumb.
func (s *Store) SetSeekDrag(position *float64) {
	s.mu.Lock()
	if position == nil {
		s.state.SeekDrag = nil
	} else {
		s.state.SeekDrag = new(*position)
	}
	s.publishLocked()
	callbacks := s.callbacksLocked()
	s.mu.Unlock()
	notify(callbacks)
}

// SetVolumeDragging suppresses stale volume echoes while the slider is held.
func (s *Store) SetVolumeDragging(dragging bool) {
	s.mu.Lock()
	s.state.VolumeDragging = dragging
	s.publishLocked()
	s.mu.Unlock()
}

// ConsumeShow reports and clears a daemon request to raise the main window.
func (s *Store) ConsumeShow() bool {
	s.mu.Lock()
	requested := s.state.ShowRequested
	s.state.ShowRequested = false
	s.publishLocked()
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
	s.publishLocked()
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
	s.publishLocked()
	s.mu.Unlock()
}

// BeginAuthFlow marks a provider flow as waiting for its URL event.
func (s *Store) BeginAuthFlow(provider model.Provider) {
	s.mu.Lock()
	flows := cloneAuthFlows(s.state.AuthFlows)
	flow := flows[provider]
	flow.Waiting, flow.Error = true, ""
	flows[provider] = flow
	s.state.AuthFlows = flows
	s.publishLocked()
	callbacks := s.callbacksLocked()
	s.mu.Unlock()
	notify(callbacks)
}

// SetAuthFlow publishes a provider browser handoff returned by a daemon call.
func (s *Store) SetAuthFlow(provider model.Provider, url, code string) {
	s.mu.Lock()
	flows := cloneAuthFlows(s.state.AuthFlows)
	flow := flows[provider]
	flow.Waiting, flow.URL, flow.Code, flow.Error = url != "", url, code, ""
	flows[provider] = flow
	s.state.AuthFlows = flows
	s.publishLocked()
	callbacks := s.callbacksLocked()
	s.mu.Unlock()
	notify(callbacks)
}

// SetAuthFlowError records a failed provider authorization start.
func (s *Store) SetAuthFlowError(provider model.Provider, message string) {
	s.mu.Lock()
	flows := cloneAuthFlows(s.state.AuthFlows)
	flow := flows[provider]
	flow.Waiting, flow.Error = false, message
	flows[provider] = flow
	s.state.AuthFlows = flows
	s.publishLocked()
	callbacks := s.callbacksLocked()
	s.mu.Unlock()
	notify(callbacks)
}

// CancelAuthFlow dismisses one client-side browser handoff.
func (s *Store) CancelAuthFlow(provider model.Provider) {
	s.mu.Lock()
	flows := cloneAuthFlows(s.state.AuthFlows)
	delete(flows, provider)
	s.state.AuthFlows = flows
	s.publishLocked()
	callbacks := s.callbacksLocked()
	s.mu.Unlock()
	notify(callbacks)
}

// ClaimAuthURL returns true once for each provider URL.
func (s *Store) ClaimAuthURL(provider model.Provider, url string) bool {
	if url == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lastOpenedAuthURL[provider] == url {
		return false
	}
	s.lastOpenedAuthURL[provider] = url
	return true
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
	s.state.Equalizer = snapshot.Equalizer
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
	s.state = cloneState(s.state)
	s.positionSampledAt = now
	s.positionSamplePaused = s.state.Playback.Paused
	s.positionSampleSpeed = effectivePlaybackSpeed(s.state.AudioFx.Speed)
	s.realNowPlaying = false
	s.authEventSeen = false
	s.spotifyEventSeen = false
	s.soundCloudEventSeen = false
	s.tidalEventSeen = false
}

func (s *Store) applyLocked(name string, data json.RawMessage, now time.Time) error {
	switch name {
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
		s.state.Playback.Now = clonePlaybackTrack(event)
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
		s.state.Personal = clonePersonal(s.state.Personal)
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
			items := cloneTracks(s.state.Queue.Items)
			items[event.CurrentIndex] = cloneTrack(*event.Current)
			s.state.Queue.Items = items
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
	case protocol.EventEqualizer:
		// a fresh value: decoding into the old one keeps an omitted preset and
		// rewrites the bands the published snapshot still shares
		var equalizer model.Equalizer
		if err := decode(data, &equalizer); err != nil {
			return err
		}
		s.state.Equalizer = equalizer
	case protocol.EventShow:
		s.state.ShowRequested = true
	case protocol.EventAuthChanged:
		oldSignedIn := s.state.Auth.SignedIn
		if err := decode(data, &s.state.Auth); err != nil {
			return err
		}
		s.state.Personal = clonePersonal(s.state.Personal)
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
	s.state.Personal = clonePersonal(s.state.Personal)
	state := &s.state.Spotify
	switch event.State {
	case "url":
		state.Flow, state.URL, state.Error = "browser", stringValue(event.URL), ""
		s.setAuthFlowLocked(model.ProviderSpotify, AuthFlow{Waiting: true, URL: state.URL})
		s.setToastLocked("Opening Spotify sign-in in your browser", ToastInfo)
	case "signed_in":
		wasSignedIn := state.SignedIn
		state.SignedIn, state.Stored, state.Name, state.Flow, state.URL, state.Error = true, true, event.Name, "", "", ""
		s.setAuthFlowLocked(model.ProviderSpotify, AuthFlow{})
		if !wasSignedIn {
			message := "Signed in to Spotify"
			if event.Name != nil {
				message += " as " + *event.Name
			}
			s.setToastLocked(message, ToastSuccess)
		}
	case "restored":
		state.SignedIn, state.Stored, state.Premium, state.Flow, state.URL, state.Error = true, true, new(true), "", "", ""
		s.setAuthFlowLocked(model.ProviderSpotify, AuthFlow{})
	case "restore_failed":
		wasSignedIn := state.SignedIn
		state.SignedIn, state.Stored, state.Flow, state.URL = false, true, "", ""
		state.Error = eventMessage(event.Message, "Spotify sign-in no longer works")
		s.setAuthFlowLocked(model.ProviderSpotify, AuthFlow{Error: state.Error})
		if wasSignedIn && ForgetSpotifyRecents(&s.state.Personal) > 0 {
			s.markPersonalDirtyLocked()
		}
	case "signed_out":
		wasSignedIn := state.SignedIn
		*state = SpotifyState{}
		s.setAuthFlowLocked(model.ProviderSpotify, AuthFlow{})
		if wasSignedIn && ForgetSpotifyRecents(&s.state.Personal) > 0 {
			s.markPersonalDirtyLocked()
		}
	case "error":
		state.Flow, state.URL = "", ""
		state.Error = eventMessage(event.Message, "Spotify sign-in failed")
		s.setAuthFlowLocked(model.ProviderSpotify, AuthFlow{Error: state.Error})
		s.setToastLocked(state.Error, ToastError)
	}
}

func (s *Store) reduceSoundCloudLocked(event model.SoundCloudAuthEvent) {
	state := &s.state.SoundCloud
	switch event.State {
	case "signed_in":
		state.SignedIn, state.Name, state.Flow, state.Error = true, event.Name, "", ""
		s.setAuthFlowLocked(model.ProviderSoundCloud, AuthFlow{})
		message := "Signed in to SoundCloud"
		if event.Name != nil {
			message += " as " + *event.Name
		}
		s.setToastLocked(message, ToastSuccess)
	case "signed_out":
		*state = SoundCloudState{}
		s.setAuthFlowLocked(model.ProviderSoundCloud, AuthFlow{})
	case "waiting":
		state.SignedIn, state.Name, state.Flow, state.Error = false, nil, "waiting", ""
		flow := s.state.AuthFlows[model.ProviderSoundCloud]
		flow.Waiting, flow.Error = true, ""
		s.setAuthFlowLocked(model.ProviderSoundCloud, flow)
		s.setToastLocked("Opened soundcloud.com in your browser, sign in there and Ryotunes will connect", ToastInfo)
	case "expired":
		state.SignedIn, state.Name, state.Flow, state.Error = false, nil, "expired", ""
		s.setAuthFlowLocked(model.ProviderSoundCloud, AuthFlow{})
		s.setToastLocked("No SoundCloud sign-in was completed in the browser", ToastInfo)
	case "restore_failed", "error":
		state.SignedIn, state.Name, state.Flow = false, nil, ""
		state.Error = eventMessage(event.Message, "SoundCloud sign-in failed")
		s.setAuthFlowLocked(model.ProviderSoundCloud, AuthFlow{Error: state.Error})
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
		s.setAuthFlowLocked(model.ProviderTidal, AuthFlow{})
		message := "Signed in to TIDAL"
		if event.Name != nil {
			message += " as " + *event.Name
		}
		s.setToastLocked(message, ToastSuccess)
	case "signed_out":
		hasCredentials := state.HasCredentials
		*state = TidalState{TidalStatus: model.TidalStatus{HasCredentials: hasCredentials}}
		s.setAuthFlowLocked(model.ProviderTidal, AuthFlow{})
	case "waiting":
		state.SignedIn, state.Name, state.Quality, state.HasCredentials = false, nil, nil, true
		state.Code, state.URL, state.Error = stringValue(event.Code), stringValue(event.URL), ""
		s.setAuthFlowLocked(model.ProviderTidal, AuthFlow{Waiting: true, URL: state.URL, Code: state.Code})
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
		s.setAuthFlowLocked(model.ProviderTidal, AuthFlow{})
		state.HasCredentials = true
		s.setToastLocked("The TIDAL code expired before it was approved", ToastInfo)
	default:
		state.SignedIn, state.Name, state.Quality, state.Code, state.URL = false, nil, nil, "", ""
		state.Error = stringValue(event.Message)
		s.setAuthFlowLocked(model.ProviderTidal, AuthFlow{Error: state.Error})
		if state.Error != "" {
			s.setToastLocked(state.Error, ToastError)
		}
	}
}

func (s *Store) applyPositionLocked(data json.RawMessage, now time.Time) bool {
	var event protocol.PositionEvent
	if decode(data, &event) != nil {
		event.Position = 0
	}
	position := clampPosition(s.state.Playback.Duration, event.Position)
	speed := effectivePlaybackSpeed(s.state.AudioFx.Speed)
	confirming := !s.positionSampledAt.IsZero() &&
		s.state.Playback.Paused == s.positionSamplePaused &&
		speed == s.positionSampleSpeed &&
		math.Abs(position-s.positionAtLocked(now)) < .15
	s.state.Playback.Position = position
	s.positionSampledAt = now
	s.positionSamplePaused = s.state.Playback.Paused
	s.positionSampleSpeed = speed
	return !confirming
}

func (s *Store) positionAtLocked(now time.Time) float64 {
	if s.state.SeekDrag != nil {
		return clampPosition(s.state.Playback.Duration, *s.state.SeekDrag)
	}
	return s.projectedPositionLocked(now)
}

func (s *Store) projectedPositionLocked(now time.Time) float64 {
	position := s.state.Playback.Position
	if !s.state.Playback.Paused && !s.positionSampledAt.IsZero() {
		position += max(0, now.Sub(s.positionSampledAt).Seconds()) * effectivePlaybackSpeed(s.state.AudioFx.Speed)
	}
	return clampPosition(s.state.Playback.Duration, position)
}

func effectivePlaybackSpeed(speed float64) float64 {
	if speed <= 0 || math.IsNaN(speed) || math.IsInf(speed, 0) {
		return 1
	}
	return speed
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

func (s *Store) publishLocked() {
	s.snapshot = cloneState(s.state)
}

func (s *Store) setAuthFlowLocked(provider model.Provider, flow AuthFlow) {
	flows := cloneAuthFlows(s.state.AuthFlows)
	if flow == (AuthFlow{}) {
		delete(flows, provider)
	} else {
		flows[provider] = flow
	}
	s.state.AuthFlows = flows
}

func cloneState(state State) State {
	state.Playback.Now = clonePlaybackTrack(state.Playback.Now)
	state.Queue.Items = cloneTracks(state.Queue.Items)
	state.Queue.SourceName = clonePtr(state.Queue.SourceName)
	state.Settings = cloneSettings(state.Settings)
	state.Auth = cloneAccount(state.Auth)
	state.Spotify = cloneSpotifyState(state.Spotify)
	state.SoundCloud = cloneSoundCloudState(state.SoundCloud)
	state.Tidal = cloneTidalState(state.Tidal)
	state.AuthFlows = cloneAuthFlows(state.AuthFlows)
	state.Downloads = cloneDownloads(state.Downloads)
	state.Personal = clonePersonal(state.Personal)
	state.LT = cloneLTState(state.LT)
	state.Lastfm.Username = clonePtr(state.Lastfm.Username)
	state.Lastfm.Error = clonePtr(state.Lastfm.Error)
	state.SeekDrag = clonePtr(state.SeekDrag)
	state.Toast = clonePtr(state.Toast)
	state.LocalRemoved = append([]string(nil), state.LocalRemoved...)
	return state
}

func cloneSettings(settings model.SettingsSnapshot) model.SettingsSnapshot {
	clone := make(model.SettingsSnapshot, len(settings))
	for key, value := range settings {
		clone[key] = value
	}
	return clone
}

func cloneAuthFlows(flows map[model.Provider]AuthFlow) map[model.Provider]AuthFlow {
	clone := make(map[model.Provider]AuthFlow, len(flows))
	for provider, flow := range flows {
		clone[provider] = flow
	}
	return clone
}

func clonePersonal(personal Personal) Personal {
	clone := personal
	clone.Picks = make([]Pick, len(personal.Picks))
	for i, pick := range personal.Picks {
		pick.BrowseItem = cloneBrowseItem(pick.BrowseItem)
		pick.ArtistID = clonePtr(pick.ArtistID)
		pick.Manual = clonePtr(pick.Manual)
		clone.Picks[i] = pick
	}
	clone.Saved = make([]model.BrowseItem, len(personal.Saved))
	for i, item := range personal.Saved {
		clone.Saved[i] = cloneBrowseItem(item)
	}
	clone.Pins = append([]string(nil), personal.Pins...)
	clone.DismissedSeeds = append([]string(nil), personal.DismissedSeeds...)
	clone.Home.Order = append([]string(nil), personal.Home.Order...)
	clone.Home.Hidden = append([]string(nil), personal.Home.Hidden...)
	clone.Home.Seen = append([]string(nil), personal.Home.Seen...)
	clone.Recent = make(map[string]RecentItem, len(personal.Recent))
	for id, item := range personal.Recent {
		item.BrowseItem = cloneBrowseItem(item.BrowseItem)
		item.ArtistID = clonePtr(item.ArtistID)
		clone.Recent[id] = item
	}
	clone.Artists = make(map[string]ArtistStat, len(personal.Artists))
	for id, artist := range personal.Artists {
		clone.Artists[id] = artist
	}
	return clone
}

func cloneAccount(account model.Account) model.Account {
	account.Name = clonePtr(account.Name)
	account.Handle = clonePtr(account.Handle)
	account.Email = clonePtr(account.Email)
	account.Thumbnail = clonePtr(account.Thumbnail)
	account.ChannelID = clonePtr(account.ChannelID)
	return account
}

func cloneSpotifyState(state SpotifyState) SpotifyState {
	state.Name = clonePtr(state.Name)
	state.Premium = clonePtr(state.Premium)
	return state
}

func cloneSoundCloudState(state SoundCloudState) SoundCloudState {
	state.Name = clonePtr(state.Name)
	return state
}

func cloneTidalState(state TidalState) TidalState {
	state.Name = clonePtr(state.Name)
	state.Quality = clonePtr(state.Quality)
	return state
}

func clonePlaybackTrack(track *model.PlaybackTrack) *model.PlaybackTrack {
	if track == nil {
		return nil
	}
	clone := *track
	clone.ArtistID = clonePtr(track.ArtistID)
	clone.ArtistRuns = cloneArtists(track.ArtistRuns)
	clone.Thumbnail = clonePtr(track.Thumbnail)
	clone.Duration = clonePtr(track.Duration)
	clone.Rating = clonePtr(track.Rating)
	clone.Plays = clonePtr(track.Plays)
	clone.Likes = clonePtr(track.Likes)
	clone.Genre = clonePtr(track.Genre)
	return &clone
}

func cloneTracks(tracks []model.Track) []model.Track {
	clone := make([]model.Track, len(tracks))
	for i, track := range tracks {
		clone[i] = cloneTrack(track)
	}
	return clone
}

func cloneTrack(track model.Track) model.Track {
	track.ArtistID = clonePtr(track.ArtistID)
	track.ArtistRuns = cloneArtists(track.ArtistRuns)
	track.Album = clonePtr(track.Album)
	track.AlbumID = clonePtr(track.AlbumID)
	track.Duration = clonePtr(track.Duration)
	track.PlayCount = clonePtr(track.PlayCount)
	track.Thumbnail = clonePtr(track.Thumbnail)
	track.SetVideoID = clonePtr(track.SetVideoID)
	track.AddedBy = clonePtr(track.AddedBy)
	track.AddedByAvatar = clonePtr(track.AddedByAvatar)
	track.Rating = clonePtr(track.Rating)
	track.QueuedBy = clonePtr(track.QueuedBy)
	track.QueuedFrom = clonePtr(track.QueuedFrom)
	track.Plays = clonePtr(track.Plays)
	track.Likes = clonePtr(track.Likes)
	track.Genre = clonePtr(track.Genre)
	return track
}

func cloneBrowseItem(item model.BrowseItem) model.BrowseItem {
	item.Subtitle = clonePtr(item.Subtitle)
	item.Thumbnail = clonePtr(item.Thumbnail)
	item.Duration = clonePtr(item.Duration)
	item.ArtistRuns = cloneArtists(item.ArtistRuns)
	item.PlayCount = clonePtr(item.PlayCount)
	return item
}

func cloneArtists(artists []model.Artist) []model.Artist {
	clone := make([]model.Artist, len(artists))
	for i, artist := range artists {
		artist.ID = clonePtr(artist.ID)
		clone[i] = artist
	}
	return clone
}

func cloneDownloads(downloads model.DownloadSnapshot) model.DownloadSnapshot {
	downloads.Jobs = append([]model.DownloadEntry(nil), downloads.Jobs...)
	for i := range downloads.Jobs {
		job := &downloads.Jobs[i]
		job.FilePath = clonePtr(job.FilePath)
		job.Error = clonePtr(job.Error)
		job.Collection = clonePtr(job.Collection)
		job.CollectionKind = clonePtr(job.CollectionKind)
		job.FinishedAt = clonePtr(job.FinishedAt)
		job.Warnings = append([]string(nil), job.Warnings...)
	}
	return downloads
}

func cloneLTState(state model.LTState) model.LTState {
	state.RoomCode = clonePtr(state.RoomCode)
	state.MyID = clonePtr(state.MyID)
	state.Users = append([]model.LTUser(nil), state.Users...)
	state.CurrentTrack = cloneLTTrack(state.CurrentTrack)
	state.Queue = cloneLTTracks(state.Queue)
	state.PendingJoins = append([]model.LTPendingJoin(nil), state.PendingJoins...)
	state.Suggestions = append([]model.LTSuggestion(nil), state.Suggestions...)
	for i := range state.Suggestions {
		state.Suggestions[i].Track = cloneLTTrackValue(state.Suggestions[i].Track)
	}
	return state
}

func cloneLTTracks(tracks []model.LTTrack) []model.LTTrack {
	clone := make([]model.LTTrack, len(tracks))
	for i, track := range tracks {
		clone[i] = cloneLTTrackValue(track)
	}
	return clone
}

func cloneLTTrack(track *model.LTTrack) *model.LTTrack {
	if track == nil {
		return nil
	}
	clone := cloneLTTrackValue(*track)
	return &clone
}

func cloneLTTrackValue(track model.LTTrack) model.LTTrack {
	track.Thumbnail = clonePtr(track.Thumbnail)
	track.QueuedBy = clonePtr(track.QueuedBy)
	return track
}

func clonePtr[T any](value *T) *T {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
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
