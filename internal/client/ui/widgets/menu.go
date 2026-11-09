package widgets

import (
	"strings"

	"gioui.org/layout"

	"github.com/ryoku-dev/ryotunes/internal/client/lithic"
	"github.com/ryoku-dev/ryotunes/internal/client/state"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

type menuEntry struct {
	item lithic.MenuItem
	run  func()
	keep bool
}

// ItemMenu builds and executes the actions available for one track or collection.
type ItemMenu struct {
	menu      lithic.Menu
	add       AddToPlaylistDialog
	edit      EditPlaylistDialog
	done      bool
	subdialog bool
}

// Done reports and clears whether the menu completed an action or dialog.
func (m *ItemMenu) Done() bool {
	done := m.done
	m.done = false
	return done
}

func menuItem(icon, label string, run func()) menuEntry {
	return menuEntry{item: lithic.MenuItem{Icon: lithic.Icon(icon), Label: label}, run: run}
}

func dangerItem(icon, label string, run func()) menuEntry {
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
	t := *target.Track
	provider := model.ProviderOf(t.VideoID)
	entries := []menuEntry{
		menuItem("playlist_play", "Play next", func() { act.PlayNext(e, []model.Track{t}, target.Context) }),
		menuItem("queue", "Add to queue", func() { act.AddToQueue(e, []model.Track{t}, target.Context) }),
	}
	if provider != model.ProviderLocal && provider != model.ProviderRadio {
		entries = append(entries, menuItem("radio", "Start radio", func() { act.StartRadio(e, "song", t.VideoID, t.Title) }))
	}
	add := menuItem("playlist_add", "Add to playlist", func() {
		track := t
		m.add.Track = &track
		m.add.Open = true
		m.subdialog = true
	})
	add.keep = true
	entries = append(entries, add)
	artistID, artistName := "", t.Artists
	if t.ArtistID != nil {
		artistID = *t.ArtistID
	}
	for _, artist := range t.ArtistRuns {
		if artistID == "" && artist.ID != nil {
			artistID, artistName = *artist.ID, artist.Text
			break
		}
	}
	if artistID != "" {
		entries = append(entries, menuItem("person", "Go to artist", func() { act.OpenArtist(e, artistID, artistName) }))
	}
	if t.AlbumID != nil && *t.AlbumID != "" {
		title := ""
		if t.Album != nil {
			title = *t.Album
		}
		entries = append(entries, menuItem("album", "Go to album", func() { act.OpenAlbum(e, *t.AlbumID, title) }))
	}
	if provider != model.ProviderLocal && provider != model.ProviderRadio {
		entries = append(entries, menuItem("download", "Download", func() { act.Download(e, t) }))
	}
	if provider == model.ProviderYouTube || provider == model.ProviderSpotify || provider == model.ProviderSoundCloud || provider == model.ProviderTidal {
		entries = append(entries, menuItem("link", "Copy link", func() { act.CopyLink(e, t) }))
	}
	if target.Context == "queue" {
		entries = append(entries, dangerItem("remove", "Remove from queue", func() { removeQueue(e, target.QueueIndex) }))
	} else if target.Context != "" && target.Context != "search" {
		entries = append(entries, dangerItem("remove", "Remove from playlist", func() { act.RemoveFromPlaylist(e, target.Context, t) }))
	}
	if trackRateable(t) {
		liked := t.Rating != nil && *t.Rating == model.RatingLike
		label, rating, icon := "Like", model.RatingLike, "favorite_border"
		if liked {
			label, rating, icon = "Unlike", model.RatingIndifferent, "favorite"
		}
		entries = append(entries, menuItem(icon, label, func() { act.Rate(e, t, rating) }))
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
		for i, item := range personal.Saved {
			if item.Kind != card.Kind || item.ID != card.ID {
				continue
			}
			if save {
				return false
			}
			personal.Saved = append(personal.Saved[:i], personal.Saved[i+1:]...)
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
		menuItem("play", "Play", func() {
			withCollection(e, card, func(items []model.Track) { act.PlayCollection(e, items, 0, card.ID, card.Title, false, nil) })
		}),
		menuItem("shuffle", "Shuffle", func() {
			withCollection(e, card, func(items []model.Track) { act.PlayCollection(e, items, 0, card.ID, card.Title, true, nil) })
		}),
		menuItem("queue", "Add to queue", func() {
			withCollection(e, card, func(items []model.Track) { act.AddToQueue(e, items, card.Title) })
		}),
	}
	if provider != model.ProviderLocal && provider != model.ProviderRadio {
		entries = append(entries, menuItem("radio", "Start radio", func() { act.StartRadio(e, kind, card.ID, card.Title) }))
	}
	if (kind == "album" || kind == "playlist" || kind == "mix") && provider != model.ProviderLocal && provider != model.ProviderRadio {
		label := "Download album"
		if kind != "album" {
			label = "Download playlist"
		}
		entries = append(entries, menuItem("download", label, func() {
			withCollection(e, card, func(items []model.Track) { act.DownloadCollection(e, card.Title, items) })
		}))
	}
	saved := personalSaved(e, card)
	saveLabel, saveIcon := "Save to library", "star"
	if saved {
		saveLabel, saveIcon = "Remove from library", "star"
	}
	entries = append(entries, menuItem(saveIcon, saveLabel, func() {
		if kind == "album" {
			toastCall(e, protocol.MethodSetAlbumSaved, protocol.SetAlbumSavedParams{PlaylistID: card.ID, Saved: !saved}, "Library updated")
			return
		}
		setPersonalSaved(e, card, !saved)
	}))
	if link := collectionLink(card); link != "" {
		entries = append(entries, menuItem("link", "Copy link", func() { act.CopyText(e, link, "Link copied") }))
	}
	return entries
}

func browseTrack(card model.BrowseItem) model.Track {
	artists := ""
	if card.Subtitle != nil {
		artists = *card.Subtitle
	}
	return model.Track{
		VideoID: card.ID, Title: card.Title, Artists: artists,
		ArtistRuns: card.ArtistRuns, Duration: card.Duration,
		Thumbnail: card.Thumbnail, PlayCount: card.PlayCount,
		IsVideo: card.IsVideo, IsUpload: card.IsUpload, Explicit: card.Explicit,
	}
}

func targetEntries(m *ItemMenu, e *env.Env, target env.MenuTarget) []menuEntry {
	if target.Track != nil {
		return trackEntries(m, e, target)
	}
	if target.Card != nil {
		if strings.EqualFold(target.Card.Kind, "song") || strings.EqualFold(target.Card.Kind, "track") {
			track := browseTrack(*target.Card)
			target.Track = &track
			return trackEntries(m, e, target)
		}
		return collectionEntries(e, *target.Card)
	}
	if target.Playlist != nil {
		id, title := target.Context, "Playlist"
		if target.Playlist.Title != nil {
			title = *target.Playlist.Title
		}
		card := model.BrowseItem{Kind: "playlist", ID: id, Title: title, Thumbnail: target.Playlist.Thumbnail}
		entries := collectionEntries(e, card)
		if target.Playlist.Owned {
			edit := menuItem("edit", "Edit playlist", func() {
				m.edit.Page = target.Playlist
				m.edit.Open = true
				m.subdialog = true
			})
			edit.keep = true
			entries = append(entries, edit)
			entries = append(entries,
				menuItem("album", "Change cover", func() {
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
				dangerItem("delete", "Delete playlist", func() {
					if e.Confirm == nil {
						return
					}
					confirmTitle, body, typeToConfirm := deleteConfirmCopy(id, title)
					e.Confirm(confirmTitle, body, "Delete", typeToConfirm, func() {
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
		}
		return entries
	}
	return nil
}

// Layout renders the target menu or its Add to playlist dialog.
func (m *ItemMenu) Layout(gtx layout.Context, e *env.Env, target env.MenuTarget) layout.Dimensions {
	if m.subdialog && !m.add.Open && !m.edit.Open {
		m.subdialog = false
		m.done = true
		return layout.Dimensions{}
	}
	if m.add.Open {
		dims := m.add.Layout(gtx, e)
		if !m.add.Open {
			m.done = true
			m.subdialog = false
		}
		return dims
	}
	if m.edit.Open {
		dims := m.edit.Layout(gtx, e)
		if !m.edit.Open {
			m.done = true
			m.subdialog = false
		}
		return dims
	}
	entries := targetEntries(m, e, target)
	items := make([]lithic.MenuItem, len(entries))
	for i := range entries {
		items[i] = entries[i].item
	}
	dims := m.menu.Layout(gtx, e.Th, items)
	if index, ok := m.menu.Clicked(gtx); ok && index >= 0 && index < len(entries) {
		entry := entries[index]
		if entry.run != nil {
			entry.run()
		}
		if !entry.keep {
			m.done = true
		}
	}
	return dims
}
