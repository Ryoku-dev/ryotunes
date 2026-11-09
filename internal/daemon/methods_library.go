package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
	"github.com/ryoku-dev/ryotunes/internal/provider"
	"github.com/ryoku-dev/ryotunes/internal/provider/radio"
	"github.com/ryoku-dev/ryotunes/internal/store"
)

func init() {
	register(protocol.MethodGetLibrary, typed(getLibrary))
	register(protocol.MethodGetLibraryAlbums, typed(getLibraryAlbums))
	register(protocol.MethodGetLibraryArtists, typed(getLibraryArtists))
	register(protocol.MethodGetLocalLibrary, typed(getLocalLibrary))
	register(protocol.MethodAddLocalFolder, typed(addLocalFolder))
	register(protocol.MethodRemoveLocalFolder, typed(removeLocalFolder))
	register(protocol.MethodPlaylistIndex, typed(playlistIndex))
	register(protocol.MethodSyncPlaylistIndex, typed(syncPlaylistIndex))
	register(protocol.MethodPlayCounts, typed(playCounts))
	register(protocol.MethodListeningStats, typed(listeningStats))
	register(protocol.MethodCreatePlaylist, typed(createPlaylist))
	register(protocol.MethodEditPlaylistDetails, typed(editPlaylistDetails))
	register(protocol.MethodDeletePlaylist, typed(deletePlaylist))
	register(protocol.MethodAddToPlaylist, typed(addToPlaylist))
	register(protocol.MethodAddToLocalPlaylist, typed(addToLocalPlaylist))
	register(protocol.MethodRemoveFromPlaylist, typed(removeFromPlaylist))
	register(protocol.MethodSetPlaylistCover, typed(setPlaylistCover))
	register(protocol.MethodSetPlaylistSort, typed(setPlaylistSort))
	register(protocol.MethodSetAlbumSaved, typed(setAlbumSaved))
	register(protocol.MethodExportPlaylistFile, typed(exportPlaylistFile))
	register(protocol.MethodImportPlaylistFile, typed(importPlaylistFile))
}

func now() time.Time { return time.Now() }

func isSmartPlaylist(id string) bool {
	return id == model.OnRepeatPlaylistID || id == model.RecentlyPlayedPlaylistID || id == model.RediscoverPlaylistID
}

func getLibrary(ctx context.Context, a *App, _ protocol.Null) ([]model.BrowseItem, error) {
	pr := selectedProvider(a)
	if pr.ID() == model.ProviderTidal || pr.ID() == model.ProviderSoundCloud {
		return []model.BrowseItem{}, nil
	}
	library, err := pr.Library(ctx)
	if err != nil {
		return nil, providerError(pr.ID(), err)
	}
	if pr.ID() != model.ProviderYouTube {
		return library.Playlists, nil
	}
	device, err := a.Store.LocalPlaylists()
	if err != nil {
		return nil, err
	}
	items := make([]model.BrowseItem, 0, len(device)+len(library.Playlists)+3)
	for _, smartID := range []string{model.OnRepeatPlaylistID, model.RecentlyPlayedPlaylistID, model.RediscoverPlaylistID} {
		page, pageErr := smartPlaylist(ctx, a, smartID)
		if pageErr != nil {
			return nil, pageErr
		}
		if len(page.Items) == 0 || page.Title == nil {
			continue
		}
		items = append(items, model.BrowseItem{Kind: "playlist", ID: smartID, Title: *page.Title, Subtitle: page.Subtitle})
	}
	for _, row := range device {
		subtitle := fmt.Sprintf("%d tracks · Stored on this device", row.TrackCount)
		item := model.BrowseItem{Kind: "playlist", ID: row.ID, Title: row.Title, Subtitle: &subtitle}
		if cover, coverErr := a.Store.PlaylistArtwork(row.ID); coverErr == nil && cover != "" {
			item.Thumbnail = &cover
		}
		items = append(items, item)
	}
	for i := range library.Playlists {
		if cover, coverErr := a.Store.PlaylistArtwork(library.Playlists[i].ID); coverErr == nil && cover != "" {
			library.Playlists[i].Thumbnail = &cover
		}
	}
	items = append(items, library.Playlists...)
	return items, nil
}
func getLibraryAlbums(ctx context.Context, a *App, _ protocol.Null) ([]model.BrowseItem, error) {
	pr := selectedProvider(a)
	if pr.ID() == model.ProviderTidal || pr.ID() == model.ProviderSoundCloud {
		return []model.BrowseItem{}, nil
	}
	library, err := pr.Library(ctx)
	return library.Albums, providerError(pr.ID(), err)
}
func getLibraryArtists(ctx context.Context, a *App, _ protocol.Null) ([]model.BrowseItem, error) {
	pr := selectedProvider(a)
	if pr.ID() == model.ProviderTidal || pr.ID() == model.ProviderSoundCloud {
		return []model.BrowseItem{}, nil
	}
	library, err := pr.Library(ctx)
	return library.Artists, providerError(pr.ID(), err)
}
func getLocalLibrary(ctx context.Context, a *App, _ protocol.Null) (model.LocalLibrary, error) {
	return a.ScanLocal(ctx)
}
func addLocalFolder(ctx context.Context, a *App, p protocol.PathParams) (model.LocalLibrary, error) {
	return a.AddLocalFolder(ctx, p.Path)
}
func removeLocalFolder(ctx context.Context, a *App, p protocol.PathParams) (model.LocalLibrary, error) {
	return a.RemoveLocalFolder(ctx, p.Path)
}
func playlistIndex(_ context.Context, a *App, _ protocol.Null) (model.PlaylistIndex, error) {
	return a.Store.PlaylistIndex()
}
func syncPlaylistIndex(ctx context.Context, a *App, _ protocol.Null) (model.PlaylistIndex, error) {
	account, accountErr := a.YouTube.Account(ctx)
	if accountErr == nil && !account.SignedIn {
		if err := a.Store.ClearPlaylistTracks(); err != nil {
			return nil, err
		}
		return a.Store.PlaylistIndex()
	}
	if at, ok := a.Store.PlaylistIndexSyncedAt(); ok && now().Before(at.Add(6*time.Hour)) {
		return a.Store.PlaylistIndex()
	}
	library, err := a.YouTube.Library(ctx)
	if err != nil {
		return nil, err
	}
	observed := make([]string, 0, len(library.Playlists))
	for _, card := range library.Playlists {
		if model.IsRyotunesOwn(card.ID) || card.ID == "VLLM" {
			continue
		}
		page, err := a.YouTube.Playlist(ctx, card.ID, model.PlaylistSortDefault, false)
		if err != nil {
			observed = append(observed, card.ID)
			continue
		}
		if !page.Owned && !page.Collaborative {
			continue
		}
		ids := make([]string, 0, len(page.Items))
		for _, item := range page.Items {
			ids = append(ids, item.VideoID)
		}
		token := page.Continuation
		for count := 0; token != nil && count < 50; count++ {
			more, moreErr := a.YouTube.PlaylistMore(ctx, *token)
			if moreErr != nil {
				break
			}
			for _, item := range more.Items {
				ids = append(ids, item.VideoID)
			}
			token = more.Continuation
		}
		if err := a.Store.SetPlaylistTracks(card.ID, ids); err != nil {
			return nil, err
		}
		observed = append(observed, card.ID)
	}
	if err := a.Store.PrunePlaylistIndex(observed); err != nil {
		return nil, err
	}
	if err := a.Store.SetPlaylistIndexSyncedAt(now()); err != nil {
		return nil, err
	}
	return a.Store.PlaylistIndex()
}
func playCounts(_ context.Context, a *App, _ protocol.Null) (model.PlayCounts, error) {
	return a.Store.PlayCounts(now().AddDate(0, 0, -30))
}
func listeningStats(_ context.Context, a *App, p protocol.ListeningStatsParams) (model.ListeningStats, error) {
	return a.Store.ListeningStats(p.Period)
}
func createPlaylist(ctx context.Context, a *App, p protocol.CreatePlaylistParams) (protocol.PlaylistIDResult, error) {
	title := strings.TrimSpace(p.Title)
	if title == "" {
		return "", fmt.Errorf("Playlist name cannot be empty.")
	}
	title = truncateRunes(title, 150)
	pr := selectedProvider(a)
	editor, ok := pr.(provider.PlaylistEditor)
	if pr.ID() == model.ProviderYouTube {
		account, accountErr := a.YouTube.Account(ctx)
		if accountErr != nil || !account.SignedIn {
			ok = false
		}
	}
	if pr.ID() == model.ProviderSpotify && !a.Spotify.Status().SignedIn {
		ok = false
	}
	if !ok {
		editor = a.Local
	}
	id, err := editor.CreatePlaylist(ctx, title)
	return protocol.PlaylistIDResult(id), providerError(pr.ID(), err)
}
func editPlaylistDetails(ctx context.Context, a *App, p protocol.EditPlaylistDetailsParams) (protocol.Null, error) {
	pr := providerForID(a, p.PlaylistID)
	editor, ok := pr.(provider.PlaylistEditor)
	if !ok {
		return protocol.Null{}, provider.ErrUnsupported
	}
	return protocol.Null{}, providerError(pr.ID(), editor.EditPlaylist(ctx, p.PlaylistID, p.Name, p.Description, p.Public))
}
func deletePlaylist(ctx context.Context, a *App, p protocol.PlaylistIDParams) (protocol.Null, error) {
	if isSmartPlaylist(p.PlaylistID) {
		return protocol.Null{}, fmt.Errorf("Smart playlists build themselves from your listening history.")
	}
	if p.PlaylistID == "spotify:playlist:liked" {
		return protocol.Null{}, coded(protocol.ErrorSpotify, "The liked songs playlist cannot be removed.")
	}
	pr := providerForID(a, p.PlaylistID)
	editor, ok := pr.(provider.PlaylistEditor)
	if !ok {
		return protocol.Null{}, provider.ErrUnsupported
	}
	return protocol.Null{}, providerError(pr.ID(), editor.DeletePlaylist(ctx, p.PlaylistID))
}
func addToPlaylist(ctx context.Context, a *App, p protocol.PlaylistTrackParams) (*bool, error) {
	if _, ok := radio.UUIDFromID(p.VideoID); ok {
		return nil, fmt.Errorf("Live radio stations cannot be added to song playlists.")
	}
	if model.ProviderOf(p.PlaylistID) == model.ProviderSpotify && model.ProviderOf(p.VideoID) != model.ProviderSpotify {
		return nil, coded(protocol.ErrorSpotify, "Not a Spotify track.")
	}
	pr := providerForID(a, p.PlaylistID)
	editor, ok := pr.(provider.PlaylistEditor)
	if !ok {
		return nil, provider.ErrUnsupported
	}
	return nil, providerError(pr.ID(), editor.AddToPlaylist(ctx, p.PlaylistID, p.VideoID))
}
func addToLocalPlaylist(ctx context.Context, a *App, p protocol.AddToLocalPlaylistParams) (bool, error) {
	if !strings.HasPrefix(p.PlaylistID, model.LocalPlaylistPrefix) {
		return false, fmt.Errorf("not a device playlist")
	}
	if _, ok := radio.UUIDFromID(p.Item.VideoID); ok {
		return false, fmt.Errorf("Live radio stations cannot be added to song playlists.")
	}
	added, err := a.Local.AddTrackToPlaylist(ctx, p.PlaylistID, cleanTransferTrack(p.Item))
	if err == nil && added {
		a.Bus.Emit(protocol.EventLibraryChanged, protocol.LibraryChangedEvent{ID: p.PlaylistID})
	}
	return added, err
}
func removeFromPlaylist(ctx context.Context, a *App, p protocol.RemoveFromPlaylistParams) (protocol.Null, error) {
	if model.ProviderOf(p.PlaylistID) == model.ProviderSpotify && model.ProviderOf(p.VideoID) != model.ProviderSpotify {
		return protocol.Null{}, coded(protocol.ErrorSpotify, "Not a Spotify track.")
	}
	pr := providerForID(a, p.PlaylistID)
	editor, ok := pr.(provider.PlaylistEditor)
	if !ok {
		return protocol.Null{}, provider.ErrUnsupported
	}
	setID := ""
	if p.SetVideoID != nil {
		setID = *p.SetVideoID
	}
	err := editor.RemoveFromPlaylist(ctx, p.PlaylistID, p.VideoID, setID)
	if err == nil && strings.HasPrefix(p.PlaylistID, model.LocalPlaylistPrefix) {
		a.Bus.Emit(protocol.EventLibraryChanged, protocol.LibraryChangedEvent{ID: p.PlaylistID})
	}
	return protocol.Null{}, providerError(pr.ID(), err)
}
func setPlaylistCover(ctx context.Context, a *App, p protocol.SetPlaylistCoverParams) (protocol.SetPlaylistCoverResult, error) {
	if !p.Pick {
		if err := a.Store.ClearPlaylistCover(p.PlaylistID); err != nil {
			return protocol.SetPlaylistCoverResult{}, err
		}
		var thumbnail *string
		if model.ProviderOf(p.PlaylistID) == model.ProviderYouTube {
			rebuilt, err := a.YouTube.ClearPlaylistCover(ctx, p.PlaylistID)
			if err != nil {
				a.Bus.Emit(protocol.EventCoverError, protocol.MessageEvent{Message: "Removed here, but YouTube Music kept its copy."})
			} else {
				thumbnail = rebuilt
			}
		}
		if thumbnail == nil {
			if derived, deriveErr := a.Store.PlaylistArtwork(p.PlaylistID); deriveErr == nil && derived != "" {
				thumbnail = &derived
			}
		}
		a.Bus.Emit(protocol.EventLibraryChanged, protocol.LibraryChangedEvent{ID: p.PlaylistID})
		return protocol.SetPlaylistCoverResult{Thumbnail: thumbnail}, nil
	}
	if p.Path == nil || strings.TrimSpace(*p.Path) == "" {
		return protocol.SetPlaylistCoverResult{}, fmt.Errorf("Pick a JPEG or PNG image.")
	}
	cover, err := a.Store.SetPlaylistCover(p.PlaylistID, *p.Path)
	if err != nil {
		return protocol.SetPlaylistCoverResult{}, err
	}
	if model.ProviderOf(p.PlaylistID) == model.ProviderYouTube {
		if bytes, readErr := os.ReadFile(cover); readErr == nil {
			a.background.Add(1)
			go func() {
				defer a.background.Done()
				if uploadErr := a.YouTube.SetPlaylistCover(a.ctx, p.PlaylistID, bytes); uploadErr != nil {
					a.Bus.Emit(protocol.EventCoverError, protocol.MessageEvent{Message: fmt.Sprintf("Artwork saved here, but the upload to YouTube Music failed: %v", uploadErr)})
				}
			}()
		}
	}
	a.Bus.Emit(protocol.EventLibraryChanged, protocol.LibraryChangedEvent{ID: p.PlaylistID})
	return protocol.SetPlaylistCoverResult{Cover: &cover}, nil
}
func setPlaylistSort(ctx context.Context, a *App, p protocol.SetPlaylistSortParams) (protocol.Null, error) {
	if strings.HasPrefix(p.PlaylistID, model.LocalPlaylistPrefix) {
		return protocol.Null{}, a.Store.SetPlaylistSort(p.PlaylistID, p.Sort)
	}
	if model.ProviderOf(p.PlaylistID) == model.ProviderYouTube {
		return protocol.Null{}, a.YouTube.SetPlaylistSort(ctx, p.PlaylistID, p.Sort)
	}
	return protocol.Null{}, provider.ErrUnsupported
}
func setAlbumSaved(ctx context.Context, a *App, p protocol.SetAlbumSavedParams) (protocol.Null, error) {
	pr := providerForID(a, p.PlaylistID)
	saver, ok := pr.(provider.AlbumSaver)
	if !ok {
		return protocol.Null{}, provider.ErrUnsupported
	}
	return protocol.Null{}, providerError(pr.ID(), saver.SetAlbumSaved(ctx, p.PlaylistID, p.Saved))
}

const maxTransferBytes = 12 << 20

func exportPlaylistFile(_ context.Context, _ *App, p protocol.ExportPlaylistFileParams) (bool, error) {
	if len(p.Items) > 5000 {
		return false, fmt.Errorf("Export is limited to 5,000 tracks.")
	}
	for _, item := range p.Items {
		if !portableTrack(item) {
			return false, fmt.Errorf("Portable playlist export supports YouTube Music tracks only; local files and live radio stay on this device.")
		}
	}
	path, err := transferPath(p.Path)
	if err != nil {
		return false, err
	}
	items := make([]model.Track, len(p.Items))
	for i, item := range p.Items {
		items[i] = cleanTransferTrack(item)
	}
	transfer := model.PlaylistTransfer{Version: 1, Title: truncateRunes(strings.TrimSpace(p.Title), 150), Items: items}
	bytes, err := json.MarshalIndent(transfer, "", "  ")
	if err != nil {
		return false, err
	}
	if err := store.WriteFileAtomic(path, append(bytes, '\n'), 0600); err != nil {
		return false, err
	}
	return true, nil
}
func importPlaylistFile(_ context.Context, _ *App, p protocol.PathParams) (protocol.ImportPlaylistFileResult, error) {
	path, err := transferPath(p.Path)
	if err != nil {
		return protocol.ImportPlaylistFileResult{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return protocol.ImportPlaylistFileResult{}, err
	}
	if info.Size() > maxTransferBytes {
		return protocol.ImportPlaylistFileResult{}, fmt.Errorf("That playlist file is too large.")
	}
	bytes, err := os.ReadFile(path)
	if err != nil {
		return protocol.ImportPlaylistFileResult{}, err
	}
	var transfer model.PlaylistTransfer
	if err := json.Unmarshal(bytes, &transfer); err != nil {
		return protocol.ImportPlaylistFileResult{}, fmt.Errorf("That is not a Ryotunes playlist file.")
	}
	if transfer.Version != 1 || len(transfer.Items) > 5000 {
		return protocol.ImportPlaylistFileResult{}, fmt.Errorf("That playlist file version or size is not supported.")
	}
	for _, item := range transfer.Items {
		if !portableTrack(item) {
			return protocol.ImportPlaylistFileResult{}, fmt.Errorf("That playlist contains unsupported or unsafe track metadata.")
		}
	}
	items := make([]model.Track, len(transfer.Items))
	for i, item := range transfer.Items {
		items[i] = cleanTransferTrack(item)
	}
	title := truncateRunes(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, strings.TrimSpace(transfer.Title)), 150)
	return protocol.ImportPlaylistFileResult{Title: title, Items: items}, nil
}
func transferPath(raw string) (string, error) {
	path := filepath.Clean(raw)
	ext := strings.ToLower(filepath.Ext(path))
	if ext == "" {
		return path + ".json", nil
	}
	if ext != ".json" {
		return "", fmt.Errorf("Ryotunes playlist files must end in .json")
	}
	return path, nil
}
func portableTrack(item model.Track) bool {
	textOK := func(value string, max int) bool {
		return value != "" && len(value) <= max && !strings.ContainsFunc(value, unicode.IsControl)
	}
	if model.ProviderOf(item.VideoID) != model.ProviderYouTube || !textOK(item.VideoID, 256) || !textOK(item.Title, 1024) || !textOK(item.Artists, 1024) {
		return false
	}
	if item.Thumbnail == nil {
		return true
	}
	if len(*item.Thumbnail) > 4096 {
		return false
	}
	u, err := url.Parse(*item.Thumbnail)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" && u.User == nil
}
func cleanTransferTrack(item model.Track) model.Track {
	item.Queued, item.QueuedEnd, item.Autoplay = false, false, false
	item.QueuedFrom, item.QueuedBy, item.SetVideoID, item.AddedBy, item.AddedByAvatar = nil, nil, nil, nil, nil
	return item
}
func truncateRunes(value string, max int) string {
	r := []rune(value)
	if len(r) > max {
		r = r[:max]
	}
	return string(r)
}
