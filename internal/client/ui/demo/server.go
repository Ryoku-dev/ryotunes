// Package demo serves plausible client fixtures over the Ryotunes protocol.
package demo

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

// Serve runs an in-process fake daemon until ctx is cancelled.
func Serve(ctx context.Context, socketPath string, playing bool) error {
	if err := os.MkdirAll(filepath.Dir(socketPath), 0o700); err != nil {
		return err
	}
	data, err := makeFixtures(filepath.Dir(socketPath), playing)
	if err != nil {
		return err
	}
	_ = os.Remove(socketPath)
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return err
	}
	defer listener.Close()
	defer os.Remove(socketPath)
	server := demoServer{data: data}
	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()
	for {
		connection, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		go server.serveConnection(ctx, connection)
	}
}

type demoServer struct {
	data       fixtures
	subscribed atomic.Int32
}

func (s *demoServer) serveConnection(ctx context.Context, connection net.Conn) {
	defer connection.Close()
	reader := bufio.NewReaderSize(connection, 64*1024)
	encoder := protocol.NewEncoder(connection)
	subscribed := false
	defer func() {
		if subscribed {
			s.subscribed.Add(-1)
		}
	}()
	for {
		_ = connection.SetDeadline(noDeadline(ctx))
		line, err := protocol.ReadLine(reader)
		if err != nil {
			if !errors.Is(err, io.EOF) && ctx.Err() == nil {
				_ = encoder.Encode(protocol.Response{Error: &protocol.Error{Code: protocol.ErrorBadRequest, Message: err.Error()}})
			}
			return
		}
		var request protocol.Request
		if err := json.Unmarshal(line, &request); err != nil {
			_ = encoder.Encode(protocol.Response{ID: request.ID, Error: &protocol.Error{Code: protocol.ErrorBadRequest, Message: "Malformed request"}})
			continue
		}
		result := s.result(request.Method, request.Params)
		if request.Method == protocol.MethodSubscribe && !subscribed {
			subscribed = true
			s.subscribed.Add(1)
		}
		payload, err := json.Marshal(result)
		if err != nil {
			_ = encoder.Encode(protocol.Response{ID: request.ID, Error: &protocol.Error{Code: protocol.ErrorFailed, Message: err.Error()}})
			continue
		}
		if err := encoder.Encode(protocol.Response{ID: request.ID, Result: payload}); err != nil {
			return
		}
	}
}

func noDeadline(ctx context.Context) time.Time {
	if deadline, ok := ctx.Deadline(); ok {
		return deadline
	}
	return time.Time{}
}

func (s *demoServer) result(method string, params json.RawMessage) any {
	switch method {
	case protocol.MethodHello:
		return protocol.Hello{Protocol: protocol.Version, Daemon: "2.1.0-demo", Clients: int(s.subscribed.Load())}
	case protocol.MethodSubscribe:
		return s.data.snapshot
	case protocol.MethodGetPlayback:
		return s.data.snapshot.Playback
	case protocol.MethodGetQueue:
		return s.data.snapshot.Queue
	case protocol.MethodGetSettings:
		return s.data.snapshot.Settings
	case protocol.MethodGetAccount:
		return s.data.snapshot.Auth
	case protocol.MethodGetAccountIdentities:
		return model.AccountIdentities{{SelectionKey: "demo", Name: dereference(s.data.snapshot.Auth.Name), Handle: s.data.snapshot.Auth.Handle, Selected: true}}
	case protocol.MethodGetProvider:
		return protocol.ProviderResult{Provider: s.data.snapshot.Provider}
	case protocol.MethodGetAudioFx:
		return s.data.snapshot.AudioFx
	case protocol.MethodGetDownloads:
		return s.data.snapshot.Downloads
	case protocol.MethodGetDownloadSettings:
		return s.data.snapshot.Downloads.Jobs[0].Settings
	case protocol.MethodGetPersonal:
		return protocol.PersonalResult{Personal: s.data.snapshot.Personal}
	case protocol.MethodLTGetState:
		return s.data.snapshot.LT
	case protocol.MethodSpotifyStatus:
		return s.data.snapshot.Spotify
	case protocol.MethodSoundCloudStatus:
		return s.data.snapshot.SoundCloud
	case protocol.MethodTidalStatus:
		return s.data.snapshot.Tidal
	case protocol.MethodGetHome:
		var request protocol.HomeParams
		if json.Unmarshal(params, &request) == nil && request.Params != nil && *request.Params == "focus" {
			return s.data.focusFeed
		}
		return s.data.feed
	case protocol.MethodGetHomeMore:
		return s.data.feed
	case protocol.MethodSearch:
		return append([]model.Track(nil), s.data.tracks...)
	case protocol.MethodSearchPage, protocol.MethodSearchPageMore:
		return model.SearchPage{Items: append([]model.Track(nil), s.data.tracks...)}
	case protocol.MethodSearchAll, protocol.MethodSearchAllMore:
		return searchAll(s.data.cards)
	case protocol.MethodSearchCards:
		return append([]model.BrowseItem(nil), s.data.cards...)
	case protocol.MethodSearchCardsPage, protocol.MethodSearchCardsMore:
		return model.CardsPage{Items: append([]model.BrowseItem(nil), s.data.cards...)}
	case protocol.MethodGetLibrary:
		return append([]model.BrowseItem(nil), s.data.library...)
	case protocol.MethodGetLibraryAlbums:
		return cardsOfKind(s.data.cards, "album")
	case protocol.MethodGetLibraryArtists:
		return cardsOfKind(s.data.cards, "artist")
	case protocol.MethodGetPlaylist:
		return s.data.playlist
	case protocol.MethodGetPlaylistMore:
		return model.PlaylistTracksPage{Items: append([]model.Track(nil), s.data.tracks...)}
	case protocol.MethodGetAlbum:
		return s.data.album
	case protocol.MethodGetArtist, protocol.MethodGetSCUser:
		return s.data.artist
	case protocol.MethodGetBrowseGrid:
		return append([]model.BrowseItem(nil), s.data.cards...)
	case protocol.MethodGetLyrics:
		lyrics := s.data.lyrics
		return &lyrics
	case protocol.MethodRadioStations:
		return append([]model.RadioStation(nil), s.data.radio...)
	case protocol.MethodGetLocalLibrary:
		return model.LocalLibrary{
			Folders: []string{filepath.Join(filepath.Dir(s.data.logs.Path), "Music"), filepath.Join(filepath.Dir(s.data.logs.Path), "Archive")},
			Albums:  cardsOfKind(s.data.cards, "album"), Artists: cardsOfKind(s.data.cards, "artist"),
			Songs: append([]model.Track(nil), s.data.tracks...),
		}
	case protocol.MethodGetLogs:
		return s.data.logs
	case protocol.MethodListeningStats:
		return s.data.stats
	case protocol.MethodPlayCounts:
		counts := make(model.PlayCounts, len(s.data.tracks))
		for index, track := range s.data.tracks {
			counts[track.VideoID] = len(s.data.tracks) - index
		}
		return counts
	case protocol.MethodGetStreamClients:
		return []model.StreamClient{model.StreamClientWebRemix, model.StreamClientVisionOS, model.StreamClientAndroidVR165}
	case protocol.MethodCheckForUpdates:
		return model.UpdateInfo{
			CurrentVersion: "2.0.0-demo", LatestVersion: new("2.1.0-demo"), Available: true,
			ReleaseURL: "https://github.com/ryoku-dev/ryotunes/releases", Notes: "Refined player window and provider controls.",
			CanInstall: true,
		}
	case protocol.MethodDiscordStatus:
		return model.DiscordStatus{Enabled: true, Status: "Connected"}
	case protocol.MethodLastfmStatus:
		return model.LastfmStatus{Configured: true, Connected: true, Username: new("nero")}
	default:
		return nil
	}
}

func cardsOfKind(items []model.BrowseItem, kind string) []model.BrowseItem {
	filtered := make([]model.BrowseItem, 0, len(items)/4)
	for _, item := range items {
		if item.Kind == kind {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func searchAll(cards []model.BrowseItem) model.SearchAll {
	result := model.SearchAll{Top: append([]model.BrowseItem(nil), cards[:min(4, len(cards))]...)}
	for _, card := range cards {
		switch card.Kind {
		case "song":
			result.Songs = append(result.Songs, card)
		case "album":
			result.Albums = append(result.Albums, card)
		case "artist":
			result.Artists = append(result.Artists, card)
		case "playlist":
			result.Playlists = append(result.Playlists, card)
		}
	}
	return result
}

func dereference(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
