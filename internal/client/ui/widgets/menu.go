package widgets

import (
	"image"
	"strings"

	"gioui.org/layout"
	"gioui.org/widget"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryotunes/internal/client/state"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

type menuEntry struct {
	item kit.MenuItem
	run  func()
	keep bool
}

// ItemMenu provides context actions for tracks and browse collections.
type ItemMenu struct {
	menu       kit.Menu
	add        AddToPlaylistDialog
	edit       EditPlaylistDialog
	target     env.MenuTarget
	done       bool
	subdialog  bool
	overlayTag int
}

// Done reports and clears completion of the current menu flow.
func (m *ItemMenu) Done() bool {
	done := m.done
	m.done = false
	return done
}

// OpenAdd starts the full-window Add to playlist dialog without opening a menu.
func (m *ItemMenu) OpenAdd(tracks ...model.Track) {
	m.add.Tracks = append(m.add.Tracks[:0], tracks...)
	m.add.Open = true
	m.edit.Open = false
	m.subdialog = true
	m.done = false
}

// OpenAt opens this menu at a window press point through the overlay host.
func (m *ItemMenu) OpenAt(e *env.Env, target env.MenuTarget, at image.Point) {
	if e == nil || e.Overlays == nil {
		return
	}
	if target.Track != nil {
		track := *target.Track
		target.Track = &track
	}
	if target.Card != nil {
		card := *target.Card
		target.Card = &card
	}
	m.target = target
	m.done = false
	m.subdialog = false
	m.add.Open = false
	m.edit.Open = false
	e.Overlays.OpenAt(&m.overlayTag, at, func(gtx layout.Context) layout.Dimensions {
		return m.Layout(gtx, e, m.target)
	})
}

func menuItem(icon *widget.Icon, label string, run func()) menuEntry {
	return menuEntry{item: kit.MenuItem{Icon: icon, Label: label}, run: run}
}

func dangerItem(icon *widget.Icon, label string, run func()) menuEntry {
	entry := menuItem(icon, label, run)
	entry.item.Danger = true
	return entry
}

func toastCall(e *env.Env, method string, params any, success string) {
	act.Call[protocol.Null](e, method, params, func(_ protocol.Null, err error) {
		if err != nil {
			if e.Toast != nil {
				e.Toast(err.Error(), env.ToastError)
			}
			return
		}
		if success != "" && e.Toast != nil {
			e.Toast(success, env.ToastSuccess)
		}
	})
}

func collectionTracks(e *env.Env, card model.BrowseItem, done func([]model.Track, error)) {
	switch strings.ToLower(card.Kind) {
	case "album", "single", "ep":
		act.Call[model.AlbumPage](e, protocol.MethodGetAlbum, protocol.IDParams{ID: card.ID}, func(page model.AlbumPage, err error) {
			done(page.Items, err)
		})
	case "playlist", "mix":
		act.Call[model.PlaylistPage](e, protocol.MethodGetPlaylist, protocol.PlaylistParams{ID: card.ID}, func(page model.PlaylistPage, err error) {
			done(page.Items, err)
		})
	case "artist", "channel", "user":
		method := protocol.MethodGetArtist
		if model.ProviderOf(card.ID) == model.ProviderSoundCloud {
			method = protocol.MethodGetSCUser
		}
		act.Call[model.ArtistPage](e, method, protocol.IDParams{ID: card.ID}, func(page model.ArtistPage, err error) {
			done(page.TopSongs, err)
		})
	default:
		done(nil, nil)
	}
}

func withCollection(e *env.Env, card model.BrowseItem, run func([]model.Track)) {
	collectionTracks(e, card, func(items []model.Track, err error) {
		if err != nil {
			if e.Toast != nil {
				e.Toast(err.Error(), env.ToastError)
			}
			return
		}
		if len(items) == 0 {
			if e.Toast != nil {
				e.Toast("This collection has no songs", env.ToastInfo)
			}
			return
		}
		run(items)
	})
}

func removeQueue(e *env.Env, index int) {
	toastCall(e, protocol.MethodRemoveFromQueue, protocol.IndexParams{Index: index}, "Removed from queue")
}

func trackEntries(m *ItemMenu, e *env.Env, target env.MenuTarget) []menuEntry {
	track := *target.Track
	provider := model.ProviderOf(track.VideoID)
	entries := []menuEntry{
		menuItem(look.Icon.Next, "Play next", func() { act.PlayNext(e, []model.Track{track}, target.Context) }),
		menuItem(look.Icon.Queue, "Add to queue", func() { act.AddToQueue(e, []model.Track{track}, target.Context) }),
	}
	if provider != model.ProviderLocal && provider != model.ProviderRadio {
		entries = append(entries, menuItem(look.Icon.Radio, "Start radio", func() {
			act.StartRadio(e, "song", track.VideoID, track.Title)
		}))
	}
	add := menuItem(look.Icon.PlaylistAdd, "Add to playlist", func() {
		m.add.Tracks = []model.Track{track}
		m.add.Open = true
		m.subdialog = true
	})
	add.keep = true
	entries = append(entries, add)

	artistID, artistName := "", track.Artists
	if track.ArtistID != nil {
		artistID = *track.ArtistID
	}
	for _, artist := range track.ArtistRuns {
		if artistID == "" && artist.ID != nil {
			artistID, artistName = *artist.ID, artist.Text
			break
		}
	}
	if artistID != "" {
		entries = append(entries, menuItem(look.Icon.Person, "Go to artist", func() { act.OpenArtist(e, artistID, artistName) }))
	}
	if track.AlbumID != nil && *track.AlbumID != "" {
		entries = append(entries, menuItem(look.Icon.Album, "Go to album", func() {
			act.OpenAlbum(e, *track.AlbumID, textValue(track.Album))
		}))
	}
	if provider != model.ProviderLocal && provider != model.ProviderRadio {
		entries = append(entries, menuItem(look.Icon.Download, "Download", func() { act.Download(e, track) }))
	}
	if provider == model.ProviderYouTube || provider == model.ProviderSpotify || provider == model.ProviderSoundCloud || provider == model.ProviderTidal {
		entries = append(entries, menuItem(look.Icon.Share, "Copy link", func() { act.CopyLink(e, track) }))
	}
	if target.Context == "queue" {
		entries = append(entries, dangerItem(look.Icon.Remove, "Remove from queue", func() { removeQueue(e, target.QueueIndex) }))
	} else if target.Context != "" && target.Context != "search" && target.Context != "album" && target.Context != "library" {
		entries = append(entries, dangerItem(look.Icon.Remove, "Remove from playlist", func() {
			act.RemoveFromPlaylist(e, target.Context, track)
		}))
	}
	if trackRateable(track) {
		liked := trackLiked(track)
		label, rating, icon := "Like", model.RatingLike, look.Icon.HeartOutline
		if liked {
			label, rating, icon = "Unlike", model.RatingIndifferent, look.Icon.Heart
		}
		entries = append(entries, menuItem(icon, label, func() { act.Rate(e, track, rating) }))
	}
	return entries
}

func personalSaved(e *env.Env, card model.BrowseItem) bool {
	if e.Store == nil {
		return false
	}
	for _, saved := range e.Store.Snapshot().Personal.Saved {
		if saved.Kind == card.Kind && saved.ID == card.ID {
			return true
		}
	}
	return false
}

func setPersonalSaved(e *env.Env, card model.BrowseItem, save bool) {
	if e.Store == nil {
		return
	}
	e.Store.UpdatePersonal(func(personal *state.Personal) bool {
		for index, item := range personal.Saved {
			if item.Kind != card.Kind || item.ID != card.ID {
				continue
			}
			if save {
				return false
			}
			personal.Saved = append(personal.Saved[:index], personal.Saved[index+1:]...)
			return true
		}
		if !save {
			return false
		}
		personal.Saved = append(personal.Saved, card)
		return true
	})
}

func collectionLink(card model.BrowseItem) string {
	provider := model.ProviderOf(card.ID)
	kind := strings.ToLower(card.Kind)
	switch provider {
	case model.ProviderSpotify:
		id := card.ID
		for _, prefix := range []string{"spotify:album:", "spotify:playlist:", "spotify:artist:"} {
			id = strings.TrimPrefix(id, prefix)
		}
		return "https://open.spotify.com/" + kind + "/" + id
	case model.ProviderTidal:
		id := card.ID
		for _, prefix := range []string{"tidal:album:", "tidal:playlist:", "tidal:artist:"} {
			id = strings.TrimPrefix(id, prefix)
		}
		return "https://tidal.com/browse/" + kind + "/" + id
	case model.ProviderSoundCloud:
		if strings.HasPrefix(card.ID, "sc:system:") {
			return "https://soundcloud.com/" + strings.TrimPrefix(card.ID, "sc:system:")
		}
	case model.ProviderYouTube:
		if kind == "playlist" || kind == "mix" {
			return "https://music.youtube.com/playlist?list=" + strings.TrimPrefix(card.ID, "VL")
		}
		return "https://music.youtube.com/browse/" + card.ID
	}
	return ""
}

func collectionEntries(e *env.Env, card model.BrowseItem) []menuEntry {
	kind := strings.ToLower(card.Kind)
	provider := model.ProviderOf(card.ID)
	entries := []menuEntry{
		menuItem(look.Icon.Play, "Play", func() {
			withCollection(e, card, func(items []model.Track) { act.PlayCollection(e, items, 0, card.ID, card.Title, false, nil) })
		}),
		menuItem(look.Icon.OpenInNew, "Open", func() { act.OpenCard(e, card) }),
		menuItem(look.Icon.Shuffle, "Shuffle", func() {
			withCollection(e, card, func(items []model.Track) { act.PlayCollection(e, items, 0, card.ID, card.Title, true, nil) })
		}),
		menuItem(look.Icon.Queue, "Add to queue", func() {
			withCollection(e, card, func(items []model.Track) { act.AddToQueue(e, items, card.Title) })
		}),
	}
	if provider != model.ProviderLocal && provider != model.ProviderRadio {
		entries = append(entries, menuItem(look.Icon.Radio, "Start radio", func() { act.StartRadio(e, kind, card.ID, card.Title) }))
	}
	if (kind == "album" || kind == "playlist" || kind == "mix") && provider != model.ProviderLocal && provider != model.ProviderRadio {
		label := "Download album"
		if kind != "album" {
			label = "Download playlist"
		}
		entries = append(entries, menuItem(look.Icon.Download, label, func() {
			withCollection(e, card, func(items []model.Track) { act.DownloadCollection(e, card.Title, items) })
		}))
	}
	saved := personalSaved(e, card)
	label, icon := "Save to library", look.Icon.HeartOutline
	if saved {
		label, icon = "Remove from library", look.Icon.Heart
	}
	entries = append(entries, menuItem(icon, label, func() {
		if kind == "album" {
			toastCall(e, protocol.MethodSetAlbumSaved, protocol.SetAlbumSavedParams{PlaylistID: card.ID, Saved: !saved}, "Library updated")
			return
		}
		setPersonalSaved(e, card, !saved)
	}))
	if link := collectionLink(card); link != "" {
		entries = append(entries, menuItem(look.Icon.Share, "Copy link", func() { act.CopyText(e, link, "Link copied") }))
	}
	return entries
}

func browseTrack(card model.BrowseItem) model.Track {
	return model.Track{
		VideoID: card.ID, Title: card.Title, Artists: textValue(card.Subtitle), ArtistRuns: card.ArtistRuns,
		Duration: card.Duration, Thumbnail: card.Thumbnail, PlayCount: card.PlayCount,
		IsVideo: card.IsVideo, IsUpload: card.IsUpload, Explicit: card.Explicit,
	}
}

func targetEntries(m *ItemMenu, e *env.Env, target env.MenuTarget) []menuEntry {
	if target.Track != nil {
		return trackEntries(m, e, target)
	}
	if target.Card != nil {
		if browseIsTrack(*target.Card) {
			track := browseTrack(*target.Card)
			target.Track = &track
			return trackEntries(m, e, target)
		}
		return collectionEntries(e, *target.Card)
	}
	if target.Playlist == nil {
		return nil
	}
	id, title := target.Context, playlistTitle(target.Playlist)
	card := model.BrowseItem{Kind: "playlist", ID: id, Title: title, Thumbnail: target.Playlist.Thumbnail}
	entries := collectionEntries(e, card)
	if !target.Playlist.Owned {
		return entries
	}
	edit := menuItem(nil, "Edit playlist", func() {
		m.edit.Page = target.Playlist
		m.edit.Open = true
		m.subdialog = true
	})
	edit.keep = true
	entries = append(entries, edit,
		menuItem(look.Icon.Album, "Change cover", func() {
			act.Call[protocol.SetPlaylistCoverResult](e, protocol.MethodSetPlaylistCover, protocol.SetPlaylistCoverParams{PlaylistID: id, Pick: true}, func(result protocol.SetPlaylistCoverResult, err error) {
				if err != nil {
					if e.Toast != nil {
						e.Toast(err.Error(), env.ToastError)
					}
					return
				}
				target.Playlist.Cover = result.Cover
				target.Playlist.Thumbnail = result.Thumbnail
				if e.Toast != nil {
					e.Toast("Playlist cover changed", env.ToastSuccess)
				}
			})
		}),
		dangerItem(look.Icon.Remove, "Delete playlist", func() {
			if e.Confirm == nil {
				return
			}
			confirmTitle, body, proof := deleteConfirmCopy(id, title)
			e.Confirm(confirmTitle, body, "Delete", proof, func() {
				act.Call[protocol.Null](e, protocol.MethodDeletePlaylist, protocol.PlaylistIDParams{PlaylistID: id}, func(_ protocol.Null, err error) {
					if err != nil {
						if e.Toast != nil {
							e.Toast(err.Error(), env.ToastError)
						}
						return
					}
					if e.Nav != nil {
						e.Nav.Back()
					}
				})
			})
		}),
	)
	return entries
}

// Layout draws the current menu. Full-window subdialogs are drawn by LayoutDialogs.
func (m *ItemMenu) Layout(gtx layout.Context, e *env.Env, target env.MenuTarget) layout.Dimensions {
	if m.subdialog {
		return layout.Dimensions{}
	}
	entries := targetEntries(m, e, target)
	items := make([]kit.MenuItem, len(entries))
	for index := range entries {
		items[index] = entries[index].item
	}
	dims := m.menu.Layout(gtx, e.Th.Theme, items)
	if index, ok := m.menu.Clicked(gtx); ok && index >= 0 && index < len(entries) {
		entry := entries[index]
		if entry.run != nil {
			entry.run()
		}
		if entry.keep {
			if e.Overlays != nil {
				e.Overlays.Close(&m.overlayTag)
			}
		} else {
			m.done = true
			if e.Overlays != nil {
				e.Overlays.Close(&m.overlayTag)
			}
		}
	}
	return dims
}

// LayoutDialogs draws menu-owned dialogs in full-window coordinates.
func (m *ItemMenu) LayoutDialogs(gtx layout.Context, e *env.Env) layout.Dimensions {
	if !m.subdialog {
		return layout.Dimensions{}
	}
	var dimensions layout.Dimensions
	switch {
	case m.add.Open:
		dimensions = m.add.Layout(gtx, e)
	case m.edit.Open:
		dimensions = m.edit.Layout(gtx, e)
	}
	if !m.add.Open && !m.edit.Open {
		m.subdialog = false
		m.done = true
	}
	return dimensions
}
