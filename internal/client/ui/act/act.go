// Package act provides non-blocking daemon actions shared by client surfaces.
package act

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"

	"gioui.org/io/clipboard"
	"gioui.org/layout"
	"gioui.org/op"

	"github.com/ryoku-dev/ryotunes/internal/client/conn"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

var pending struct {
	sync.Mutex
	frame     []func()
	clipboard []string
}

func dispatch(fn func()) {
	if fn == nil {
		return
	}
	pending.Lock()
	pending.frame = append(pending.frame, fn)
	pending.Unlock()
}

func toast(e *env.Env, message string, kind env.ToastKind) {
	if e != nil && e.Toast != nil && message != "" {
		e.Toast(message, kind)
	}
}

func resultToast(e *env.Env, success string) func(protocol.Null, error) {
	return func(_ protocol.Null, err error) {
		if err != nil {
			toast(e, err.Error(), env.ToastError)
			return
		}
		toast(e, success, env.ToastSuccess)
	}
}

// Call runs one typed daemon call off the frame goroutine and delivers its result on a later frame.
func Call[R any](e *env.Env, method string, params any, done func(R, error)) {
	if e == nil || e.Conn == nil || e.Go == nil {
		var zero R
		if done != nil {
			dispatch(func() { done(zero, fmt.Errorf("daemon connection is not available")) })
		}
		return
	}
	e.Go(func(ctx context.Context) {
		result, err := conn.Do[any, R](ctx, e.Conn, method, params)
		if done != nil {
			dispatch(func() { done(result, err) })
		}
	})
}

// Play starts one track.
func Play(e *env.Env, t model.Track) {
	Call[protocol.Null](e, protocol.MethodPlay, protocol.PlayParams{Item: t}, resultToast(e, ""))
}

// PlayCollection replaces the queue with a collection and starts at the requested row.
func PlayCollection(e *env.Env, items []model.Track, start int, sourceID, sourceName string, shuffle bool, continuation *string) {
	items = append([]model.Track(nil), items...)
	params := protocol.PlayPlaylistParams{Items: items, Start: &start, Shuffle: &shuffle, Continuation: continuation}
	if sourceID != "" {
		params.SourceID = &sourceID
	}
	if sourceName != "" {
		params.SourceName = &sourceName
	}
	Call[protocol.Null](e, protocol.MethodPlayPlaylist, params, resultToast(e, ""))
}

func queueContext(from string) *model.QueueContext {
	if from == "" {
		return nil
	}
	value := model.QueueContext(from)
	return &value
}

// AddToQueue appends tracks to the manual queue.
func AddToQueue(e *env.Env, items []model.Track, from string) {
	items = append([]model.Track(nil), items...)
	Call[protocol.Null](e, protocol.MethodAddToQueue, protocol.AddToQueueParams{Items: items, From: queueContext(from)}, resultToast(e, "Added to queue"))
}

// PlayNext inserts tracks after the current row.
func PlayNext(e *env.Env, items []model.Track, from string) {
	items = append([]model.Track(nil), items...)
	Call[protocol.Null](e, protocol.MethodPlayNext, protocol.PlayNextParams{Items: items, From: queueContext(from)}, resultToast(e, "Playing next"))
}

// StartRadio starts provider radio from an entity.
func StartRadio(e *env.Env, kind, id, name string) {
	params := protocol.StartRadioParams{Kind: kind, ID: id}
	if name != "" {
		params.Name = &name
	}
	Call[protocol.Null](e, protocol.MethodStartRadio, params, resultToast(e, "Radio started"))
}

// Rate changes a track rating.
func Rate(e *env.Env, t model.Track, rating model.Rating) {
	Call[protocol.Null](e, protocol.MethodRate, protocol.RateParams{VideoID: t.VideoID, Rating: rating}, resultToast(e, ""))
}

// Download queues one track for download.
func Download(e *env.Env, t model.Track) {
	artists := t.Artists
	Call[protocol.Null](e, protocol.MethodEnqueueDownload, protocol.EnqueueDownloadParams{
		VideoID: t.VideoID, Title: t.Title, Artists: &artists, Thumbnail: t.Thumbnail,
	}, resultToast(e, "Download queued"))
}

// DownloadCollection queues every track in a collection for download.
func DownloadCollection(e *env.Env, title string, items []model.Track) {
	entries := make([]model.DownloadCollectionEntry, 0, len(items))
	collection := title
	for _, item := range items {
		thumbnail := ""
		if item.Thumbnail != nil {
			thumbnail = *item.Thumbnail
		}
		entries = append(entries, model.DownloadCollectionEntry{
			VideoID: item.VideoID, Title: item.Title, Artists: item.Artists,
			Thumbnail: thumbnail, Collection: &collection,
		})
	}
	Call[model.DownloadCollectionResult](e, protocol.MethodEnqueueCollection, protocol.EnqueueCollectionParams{Entries: entries}, func(_ model.DownloadCollectionResult, err error) {
		if err != nil {
			toast(e, err.Error(), env.ToastError)
			return
		}
		toast(e, "Downloads queued", env.ToastSuccess)
	})
}

func trackFromCard(c model.BrowseItem) model.Track {
	return model.Track{
		VideoID: c.ID, Title: c.Title, Artists: value(c.Subtitle), ArtistRuns: c.ArtistRuns,
		Duration: c.Duration, Thumbnail: c.Thumbnail, PlayCount: c.PlayCount,
		IsVideo: c.IsVideo, IsUpload: c.IsUpload, Explicit: c.Explicit,
	}
}

func value(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

// OpenCard routes a collection card or plays a song card.
func OpenCard(e *env.Env, c model.BrowseItem) {
	if e == nil {
		return
	}
	switch strings.ToLower(c.Kind) {
	case "song", "track", "video":
		Play(e, trackFromCard(c))
	case "artist", "channel", "user":
		OpenArtist(e, c.ID, c.Title)
	case "album", "single", "ep":
		OpenAlbum(e, c.ID, c.Title)
	case "playlist", "mix":
		if e.Nav != nil {
			e.Nav.Push(env.Route{Kind: env.RoutePlaylist, ID: c.ID, Title: c.Title})
		}
	default:
		if e.Nav != nil {
			e.Nav.Push(env.Route{Kind: env.RouteList, ID: c.ID, Title: c.Title})
		}
	}
}

// OpenArtist routes to an artist detail page.
func OpenArtist(e *env.Env, id, name string) {
	if e != nil && e.Nav != nil {
		e.Nav.Push(env.Route{Kind: env.RouteArtist, ID: id, Title: name})
	}
}

// OpenAlbum routes to an album detail page.
func OpenAlbum(e *env.Env, id, title string) {
	if e != nil && e.Nav != nil {
		e.Nav.Push(env.Route{Kind: env.RouteAlbum, ID: id, Title: title})
	}
}

// AddToPlaylist adds a track to a provider or device playlist.
func AddToPlaylist(e *env.Env, playlistID string, t model.Track) {
	if strings.HasPrefix(playlistID, model.LocalPlaylistPrefix) {
		Call[protocol.Null](e, protocol.MethodAddToLocalPlaylist, protocol.AddToLocalPlaylistParams{PlaylistID: playlistID, Item: t}, resultToast(e, "Added to playlist"))
		return
	}
	Call[protocol.Null](e, protocol.MethodAddToPlaylist, protocol.PlaylistTrackParams{PlaylistID: playlistID, VideoID: t.VideoID}, resultToast(e, "Added to playlist"))
}

// RemoveFromPlaylist removes a track from a provider or device playlist.
func RemoveFromPlaylist(e *env.Env, playlistID string, t model.Track) {
	Call[protocol.Null](e, protocol.MethodRemoveFromPlaylist, protocol.RemoveFromPlaylistParams{
		PlaylistID: playlistID, VideoID: t.VideoID, SetVideoID: t.SetVideoID,
	}, resultToast(e, "Removed from playlist"))
}

// CopyText queues plain text for Gio's next clipboard command.
func CopyText(e *env.Env, text, success string) {
	if text == "" {
		toast(e, "Nothing to copy", env.ToastError)
		return
	}
	pending.Lock()
	pending.clipboard = append(pending.clipboard, text)
	pending.Unlock()
	toast(e, success, env.ToastSuccess)
}

// CopyLink queues a provider share URL for the next frame's clipboard command.
func CopyLink(e *env.Env, t model.Track) {
	link := shareURL(t)
	if link == "" {
		toast(e, "This item has no share link", env.ToastError)
		return
	}
	CopyText(e, link, "Link copied")
}

func shareURL(t model.Track) string {
	id := t.VideoID
	switch model.ProviderOf(id) {
	case model.ProviderSpotify:
		id = strings.TrimPrefix(id, "spotify:track:")
		id = strings.TrimPrefix(id, "spotify:")
		return "https://open.spotify.com/track/" + id
	case model.ProviderSoundCloud:
		if strings.HasPrefix(id, "https://soundcloud.com/") || strings.HasPrefix(id, "https://www.soundcloud.com/") {
			return id
		}
		if strings.HasPrefix(id, "sc:system:") {
			return "https://soundcloud.com/" + strings.TrimPrefix(id, "sc:system:")
		}
		return ""
	case model.ProviderTidal:
		id = strings.TrimPrefix(id, "tidal:track:")
		id = strings.TrimPrefix(id, "tidal:")
		return "https://tidal.com/browse/track/" + id
	case model.ProviderYouTube:
		return "https://music.youtube.com/watch?v=" + id
	default:
		return ""
	}
}

// PendingClipboard delivers queued callbacks and clipboard writes during a frame.
func PendingClipboard(gtx layout.Context) {
	pending.Lock()
	frame := pending.frame
	texts := pending.clipboard
	pending.frame = nil
	pending.clipboard = nil
	pending.Unlock()
	for _, fn := range frame {
		fn()
	}
	for _, text := range texts {
		gtx.Execute(clipboard.WriteCmd{Type: "text/plain", Data: io.NopCloser(strings.NewReader(text))})
	}
	if len(frame) > 0 || len(texts) > 0 {
		gtx.Execute(op.InvalidateCmd{})
	}
}
