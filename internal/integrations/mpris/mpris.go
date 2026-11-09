// Package mpris exposes Ryotunes playback controls on the session D-Bus.
package mpris

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"reflect"
	"strings"
	"sync"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"
	"github.com/godbus/dbus/v5/prop"
	"github.com/ryoku-dev/ryotunes/internal/model"
)

const (
	busName         = "org.mpris.MediaPlayer2.ryotunes"
	objectPath      = dbus.ObjectPath("/org/mpris/MediaPlayer2")
	rootInterface   = "org.mpris.MediaPlayer2"
	playerInterface = "org.mpris.MediaPlayer2.Player"
)

// Controller is the playback and lifecycle surface controlled through MPRIS.
type Controller interface {
	PlayPause() error
	Play() error
	Pause() error
	Stop() error
	Next() error
	Previous() error
	Seek(offset float64) error
	SetPosition(position float64) error
	SetVolume(percent int) error
	SetLoopStatus(mode model.RepeatMode) error
	SetShuffle(shuffle bool) error
	Raise() error
	Quit() error
}

// URIOpener is the optional controller capability used by MPRIS OpenUri.
type URIOpener interface {
	OpenUri(uri string) error
}

// Service owns the MPRIS bus name and exported object.
type Service struct {
	conn       *dbus.Conn
	controller Controller
	log        *slog.Logger
	props      *prop.Properties

	mu                 sync.RWMutex
	position           float64
	trackID            dbus.ObjectPath
	metadataTrack      string
	metadataTitle      string
	metadataArtists    string
	metadataArtistRuns []model.Artist
	metadataArtwork    string
	metadataDuration   float64
	metadataAlbum      string
	metadata           map[string]dbus.Variant
	closed             bool
	done               chan struct{}
}

// New registers the Ryotunes MPRIS object on the session bus.
func New(ctx context.Context, controller Controller, log *slog.Logger) (*Service, error) {
	if controller == nil {
		return nil, errors.New("mpris: nil controller")
	}
	if log == nil {
		log = slog.Default()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, fmt.Errorf("mpris: connect session bus: %w", err)
	}
	reply, err := conn.RequestName(busName, dbus.NameFlagDoNotQueue)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("mpris: request name: %w", err)
	}
	if reply != dbus.RequestNameReplyPrimaryOwner {
		_ = conn.Close()
		return nil, fmt.Errorf("mpris: bus name %s is already owned", busName)
	}

	s := &Service{conn: conn, controller: controller, log: log, trackID: noTrackPath(), done: make(chan struct{})}
	if err := conn.Export(s, objectPath, rootInterface); err != nil {
		s.Close()
		return nil, fmt.Errorf("mpris: export root interface: %w", err)
	}
	// Seek is the MPRIS name, but a Go method called Seek must match io.Seeker
	if err := conn.ExportWithMap(s, map[string]string{"SeekBy": "Seek"}, objectPath, playerInterface); err != nil {
		s.Close()
		return nil, fmt.Errorf("mpris: export player interface: %w", err)
	}

	props, err := prop.Export(conn, objectPath, s.propertySpec())
	if err != nil {
		s.Close()
		return nil, fmt.Errorf("mpris: export properties: %w", err)
	}
	s.props = props
	node := &introspect.Node{
		Name: string(objectPath),
		Interfaces: []introspect.Interface{
			introspect.IntrospectData,
			prop.IntrospectData,
			rootIntrospection(props),
			playerIntrospection(props),
		},
	}
	if err := conn.Export(introspect.NewIntrospectable(node), objectPath, "org.freedesktop.DBus.Introspectable"); err != nil {
		s.Close()
		return nil, fmt.Errorf("mpris: export introspection: %w", err)
	}
	go func() {
		select {
		case <-ctx.Done():
			s.Close()
		case <-s.done:
		}
	}()
	return s, nil
}

func (s *Service) propertySpec() prop.Map {
	failed := func(err error) *dbus.Error {
		if err == nil {
			return nil
		}
		return dbus.MakeFailedError(err)
	}
	return prop.Map{
		rootInterface: {
			"CanQuit":             {Value: true, Emit: prop.EmitConst},
			"CanRaise":            {Value: true, Emit: prop.EmitConst},
			"HasTrackList":        {Value: false, Emit: prop.EmitConst},
			"Identity":            {Value: "Ryotunes", Emit: prop.EmitConst},
			"DesktopEntry":        {Value: "ryotunes", Emit: prop.EmitConst},
			"SupportedUriSchemes": {Value: []string{"file", "http", "https"}, Emit: prop.EmitConst},
			"SupportedMimeTypes":  {Value: []string{}, Emit: prop.EmitConst},
			"Fullscreen":          {Value: false, Emit: prop.EmitConst},
			"CanSetFullscreen":    {Value: false, Emit: prop.EmitConst},
		},
		playerInterface: {
			"PlaybackStatus": {Value: "Stopped", Emit: prop.EmitTrue},
			"LoopStatus": {Value: "None", Writable: true, Emit: prop.EmitTrue, Callback: func(c *prop.Change) *dbus.Error {
				mode, ok := loopMode(c.Value.(string))
				if !ok {
					return dbus.NewError("org.freedesktop.DBus.Error.InvalidArgs", []any{"LoopStatus must be None, Track, or Playlist"})
				}
				return failed(s.controller.SetLoopStatus(mode))
			}},
			"Rate": {Value: float64(1), Writable: true, Emit: prop.EmitTrue, Callback: func(c *prop.Change) *dbus.Error {
				if c.Value.(float64) != 1 {
					return dbus.NewError("org.freedesktop.DBus.Error.InvalidArgs", []any{"Ryotunes supports only rate 1.0"})
				}
				return nil
			}},
			"Shuffle": {Value: false, Writable: true, Emit: prop.EmitTrue, Callback: func(c *prop.Change) *dbus.Error {
				return failed(s.controller.SetShuffle(c.Value.(bool)))
			}},
			"Metadata": {Value: map[string]dbus.Variant{}, Emit: prop.EmitTrue},
			"Volume": {Value: float64(1), Writable: true, Emit: prop.EmitTrue, Callback: func(c *prop.Change) *dbus.Error {
				volume := c.Value.(float64)
				if volume < 0 {
					return dbus.NewError("org.freedesktop.DBus.Error.InvalidArgs", []any{"Volume must not be negative"})
				}
				return failed(s.controller.SetVolume(int(volume*100 + 0.5)))
			}},
			"Position":      {Value: int64(0), Emit: prop.EmitFalse},
			"MinimumRate":   {Value: float64(1), Emit: prop.EmitConst},
			"MaximumRate":   {Value: float64(1), Emit: prop.EmitConst},
			"CanGoNext":     {Value: false, Emit: prop.EmitTrue},
			"CanGoPrevious": {Value: false, Emit: prop.EmitTrue},
			"CanPlay":       {Value: false, Emit: prop.EmitTrue},
			"CanPause":      {Value: false, Emit: prop.EmitTrue},
			"CanSeek":       {Value: false, Emit: prop.EmitTrue},
			"CanControl":    {Value: true, Emit: prop.EmitConst},
		},
	}
}

// Update publishes a complete playback and queue snapshot to MPRIS clients.
func (s *Service) Update(playback model.PlaybackSnapshot, queue model.QueueSnapshot) {
	if s.props == nil {
		return
	}
	status := "Stopped"
	if playback.Now != nil {
		if playback.Paused {
			status = "Paused"
		} else {
			status = "Playing"
		}
	}
	trackKey := ""
	titleKey := ""
	artistsKey := ""
	artworkKey := ""
	var artistRunsKey []model.Artist
	if playback.Now != nil {
		trackKey = playback.Now.VideoID
		titleKey = playback.Now.Title
		artistsKey = playback.Now.Artists
		artistRunsKey = playback.Now.ArtistRuns
		if playback.Now.Thumbnail != nil {
			artworkKey = *playback.Now.Thumbnail
		}
	}
	albumKey := currentAlbum(trackKey, queue)
	s.mu.Lock()
	metadataDuration := playback.Duration
	if s.metadataTrack != trackKey {
		metadataDuration = 0
	}
	if s.metadata == nil || s.metadataTrack != trackKey || s.metadataTitle != titleKey || s.metadataArtists != artistsKey || !reflect.DeepEqual(s.metadataArtistRuns, artistRunsKey) || s.metadataArtwork != artworkKey || s.metadataDuration != metadataDuration || s.metadataAlbum != albumKey {
		metadataPlayback := playback
		metadataPlayback.Duration = metadataDuration
		s.metadata, s.trackID = metadataFor(metadataPlayback, queue)
		s.metadataTrack = trackKey
		s.metadataTitle = titleKey
		s.metadataArtists = artistsKey
		s.metadataArtistRuns = append(s.metadataArtistRuns[:0], artistRunsKey...)
		s.metadataArtwork = artworkKey
		s.metadataDuration = metadataDuration
		s.metadataAlbum = albumKey
	}
	metadata := s.metadata
	s.position = max(playback.Position, 0)
	s.mu.Unlock()

	s.setProperty(playerInterface, "PlaybackStatus", status)
	s.setProperty(playerInterface, "Metadata", metadata)
	s.setProperty(playerInterface, "Position", secondsToMicroseconds(playback.Position))
	s.setProperty(playerInterface, "Volume", float64(playback.Volume)/100)
	s.setProperty(playerInterface, "LoopStatus", mprisLoop(queue.Repeat))
	s.setProperty(playerInterface, "Shuffle", queue.Shuffle)
	hasTrack := playback.Now != nil
	hasQueuePosition := queue.CurrentIndex >= 0 && queue.CurrentIndex < len(queue.Items)
	s.setProperty(playerInterface, "CanPlay", hasTrack || hasQueuePosition)
	s.setProperty(playerInterface, "CanPause", hasTrack)
	s.setProperty(playerInterface, "CanSeek", hasTrack && playback.Duration > 0)
	s.setProperty(playerInterface, "CanGoPrevious", hasQueuePosition && queue.CurrentIndex > 0)
	s.setProperty(playerInterface, "CanGoNext", hasQueuePosition && queue.CurrentIndex+1 < len(queue.Items))
}

func (s *Service) setProperty(iface, name string, value any) {
	if reflect.DeepEqual(s.props.GetMust(iface, name), value) {
		return
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			s.log.Debug("mpris property update failed", "property", name, "error", recovered)
		}
	}()
	s.props.SetMust(iface, name, value)
}

// Raise asks the client window to show itself.
func (s *Service) Raise() *dbus.Error { return dbusError(s.controller.Raise()) }

// Quit asks the daemon to shut down.
func (s *Service) Quit() *dbus.Error { return dbusError(s.controller.Quit()) }

// Next advances to the next queue item.
func (s *Service) Next() *dbus.Error { return dbusError(s.controller.Next()) }

// Previous returns to the previous queue item.
func (s *Service) Previous() *dbus.Error { return dbusError(s.controller.Previous()) }

// Pause pauses playback.
func (s *Service) Pause() *dbus.Error { return dbusError(s.controller.Pause()) }

// PlayPause toggles pause state.
func (s *Service) PlayPause() *dbus.Error { return dbusError(s.controller.PlayPause()) }

// Stop stops playback.
func (s *Service) Stop() *dbus.Error { return dbusError(s.controller.Stop()) }

// Play resumes playback.
func (s *Service) Play() *dbus.Error { return dbusError(s.controller.Play()) }

// SeekBy changes position by a signed microsecond offset; D-Bus calls it Seek.
func (s *Service) SeekBy(offset int64) *dbus.Error {
	if err := s.controller.Seek(float64(offset) / 1_000_000); err != nil {
		return dbusError(err)
	}
	s.mu.RLock()
	position := s.position + float64(offset)/1_000_000
	s.mu.RUnlock()
	if position < 0 {
		position = 0
	}
	s.emitSeeked(position)
	return nil
}

// SetPosition seeks to an absolute microsecond position for the current track.
func (s *Service) SetPosition(trackID dbus.ObjectPath, position int64) *dbus.Error {
	s.mu.RLock()
	current := s.trackID
	s.mu.RUnlock()
	if trackID != current {
		return nil
	}
	seconds := float64(position) / 1_000_000
	if seconds < 0 {
		seconds = 0
	}
	if err := s.controller.SetPosition(seconds); err != nil {
		return dbusError(err)
	}
	s.emitSeeked(seconds)
	return nil
}

// OpenUri opens a URI when the controller supplies the optional capability.
func (s *Service) OpenUri(uri string) *dbus.Error {
	opener, ok := s.controller.(URIOpener)
	if !ok {
		return dbus.NewError("org.mpris.MediaPlayer2.Error.NotSupported", []any{"OpenUri is not supported"})
	}
	return dbusError(opener.OpenUri(uri))
}

func (s *Service) emitSeeked(position float64) {
	if err := s.conn.Emit(objectPath, playerInterface+".Seeked", secondsToMicroseconds(position)); err != nil {
		s.log.Debug("mpris Seeked signal failed", "error", err)
	}
}

// Close unregisters the MPRIS object and releases its bus name.
func (s *Service) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	close(s.done)
	s.mu.Unlock()
	if s.props != nil {
		s.setProperty(playerInterface, "PlaybackStatus", "Stopped")
		s.setProperty(playerInterface, "Metadata", map[string]dbus.Variant{})
		s.setProperty(playerInterface, "Position", int64(0))
	}
	if s.conn == nil {
		return
	}
	_ = s.conn.Export(nil, objectPath, rootInterface)
	_ = s.conn.Export(nil, objectPath, playerInterface)
	_ = s.conn.Export(nil, objectPath, "org.freedesktop.DBus.Properties")
	_ = s.conn.Export(nil, objectPath, "org.freedesktop.DBus.Introspectable")
	_, _ = s.conn.ReleaseName(busName)
	_ = s.conn.Close()
}

func metadataFor(playback model.PlaybackSnapshot, queue model.QueueSnapshot) (map[string]dbus.Variant, dbus.ObjectPath) {
	if playback.Now == nil {
		return map[string]dbus.Variant{}, noTrackPath()
	}
	now := playback.Now
	trackID := trackPath(now.VideoID)
	metadata := map[string]dbus.Variant{
		"mpris:trackid": dbus.MakeVariant(trackID),
		"xesam:title":   dbus.MakeVariant(now.Title),
		"xesam:artist":  dbus.MakeVariant(artistNames(now)),
	}
	if playback.Duration > 0 {
		metadata["mpris:length"] = dbus.MakeVariant(secondsToMicroseconds(playback.Duration))
	}
	if now.Thumbnail != nil && *now.Thumbnail != "" {
		metadata["mpris:artUrl"] = dbus.MakeVariant(metadataArtURL(now.VideoID, *now.Thumbnail))
	}
	if queue.CurrentIndex >= 0 && queue.CurrentIndex < len(queue.Items) {
		item := queue.Items[queue.CurrentIndex]
		if item.VideoID == now.VideoID && item.Album != nil && *item.Album != "" {
			metadata["xesam:album"] = dbus.MakeVariant(*item.Album)
		}
	}
	return metadata, trackID
}

func currentAlbum(trackID string, queue model.QueueSnapshot) string {
	if trackID == "" || queue.CurrentIndex < 0 || queue.CurrentIndex >= len(queue.Items) {
		return ""
	}
	item := queue.Items[queue.CurrentIndex]
	if item.VideoID != trackID || item.Album == nil {
		return ""
	}
	return *item.Album
}

func metadataArtURL(id, artwork string) string {
	if model.ProviderOf(id) == model.ProviderLocal && strings.HasPrefix(artwork, "/") {
		return (&url.URL{Scheme: "file", Path: artwork}).String()
	}
	return artwork
}

func artistNames(track *model.PlaybackTrack) []string {
	if len(track.ArtistRuns) != 0 {
		artists := make([]string, 0, len(track.ArtistRuns))
		for _, artist := range track.ArtistRuns {
			if name := strings.TrimSpace(artist.Text); name != "" {
				artists = append(artists, name)
			}
		}
		if len(artists) != 0 {
			return artists
		}
	}
	if strings.TrimSpace(track.Artists) == "" {
		return []string{}
	}
	return []string{track.Artists}
}

func trackPath(id string) dbus.ObjectPath {
	hash := sha1.Sum([]byte(id))
	return dbus.ObjectPath("/org/mpris/MediaPlayer2/Track/" + hex.EncodeToString(hash[:]))
}

func noTrackPath() dbus.ObjectPath { return dbus.ObjectPath("/org/mpris/MediaPlayer2/Track/NoTrack") }

func secondsToMicroseconds(seconds float64) int64 {
	if seconds <= 0 {
		return 0
	}
	return int64(seconds * 1_000_000)
}

func mprisLoop(mode model.RepeatMode) string {
	switch mode {
	case model.RepeatOne:
		return "Track"
	case model.RepeatAll:
		return "Playlist"
	default:
		return "None"
	}
}

func loopMode(value string) (model.RepeatMode, bool) {
	switch value {
	case "None":
		return model.RepeatOff, true
	case "Track":
		return model.RepeatOne, true
	case "Playlist":
		return model.RepeatAll, true
	default:
		return "", false
	}
}

func dbusError(err error) *dbus.Error {
	if err == nil {
		return nil
	}
	return dbus.MakeFailedError(err)
}

func rootIntrospection(props *prop.Properties) introspect.Interface {
	return introspect.Interface{
		Name: rootInterface,
		Methods: []introspect.Method{
			{Name: "Raise"},
			{Name: "Quit"},
		},
		Properties: props.Introspection(rootInterface),
	}
}

func playerIntrospection(props *prop.Properties) introspect.Interface {
	return introspect.Interface{
		Name: playerInterface,
		Methods: []introspect.Method{
			{Name: "Next"}, {Name: "Previous"}, {Name: "Pause"}, {Name: "PlayPause"},
			{Name: "Stop"}, {Name: "Play"},
			{Name: "Seek", Args: []introspect.Arg{{Name: "Offset", Type: "x", Direction: "in"}}},
			{Name: "SetPosition", Args: []introspect.Arg{{Name: "TrackId", Type: "o", Direction: "in"}, {Name: "Position", Type: "x", Direction: "in"}}},
			{Name: "OpenUri", Args: []introspect.Arg{{Name: "Uri", Type: "s", Direction: "in"}}},
		},
		Signals:    []introspect.Signal{{Name: "Seeked", Args: []introspect.Arg{{Name: "Position", Type: "x"}}}},
		Properties: props.Introspection(playerInterface),
	}
}
