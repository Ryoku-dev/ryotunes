package local

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ryoku-dev/ryotunes/internal/model"
	sourceprovider "github.com/ryoku-dev/ryotunes/internal/provider"
)

const (
	// SongPrefix prefixes local track IDs.
	SongPrefix = "LOCAL:"
	// AlbumPrefix prefixes local album IDs.
	AlbumPrefix = "LOCALALBUM:"
	// ArtistPrefix prefixes local artist IDs.
	ArtistPrefix = "LOCALARTIST:"
	// ScanVersion is bumped when persisted scanner output changes shape or meaning.
	ScanVersion = "5"
	// UnknownArtist is used when neither tags nor a filename identify an artist.
	UnknownArtist = "Unknown artist"
)

// LocalTrack is one persisted scanner row.
type LocalTrack struct {
	Path         string
	Title        string
	Artist       string
	Album        string
	AlbumKey     string
	TrackNo      int64
	DurationSecs int64
	Cover        string
	MTime        int64
}

// Index is the local scanner and device-playlist persistence boundary.
type Index interface {
	UpsertLocalTrack(LocalTrack) error
	DeleteLocalTrack(path string) error
	AllLocalTracks() ([]LocalTrack, error)
	LocalTracksByAlbum(albumKey string) ([]LocalTrack, error)
	PruneMissingLocalTracks(foundPaths []string) ([]string, error)
	LocalFolders() ([]string, error)
	SetLocalFolders([]string) error
	LocalScanVersion() (string, error)
	SetLocalScanVersion(string) error
	LocalPlaylists() ([]model.DevicePlaylist, error)
	LocalPlaylist(id string) (model.DevicePlaylist, bool, error)
	CreateLocalPlaylist(model.DevicePlaylist) error
	RenameLocalPlaylist(id, title string) error
	DeleteLocalPlaylist(id string) error
	AddLocalPlaylistTrack(playlistID string, track model.Track) (bool, error)
	RemoveLocalPlaylistTrack(playlistID, trackID string) error
	LocalPlaylistTracks(playlistID string) ([]model.Track, error)
	LocalPlaylistMemberships() (model.PlaylistIndex, error)
}

// Options configures a local provider.
type Options struct {
	Index    Index
	CacheDir string
	FFProbe  string
	Log      *slog.Logger
}

// Provider serves indexed music and device playlists.
type Provider struct {
	index   Index
	scanner *Scanner
	log     *slog.Logger
}

// New creates a local provider without scanning the filesystem.
func New(o Options) (*Provider, error) {
	if o.Index == nil {
		return nil, errors.New("local provider requires an index")
	}
	if o.Log == nil {
		o.Log = slog.Default()
	}
	return &Provider{
		index:   o.Index,
		scanner: NewScanner(o.Index, o.CacheDir, o.FFProbe, o.Log),
		log:     o.Log,
	}, nil
}

// ID identifies the local provider.
func (p *Provider) ID() model.Provider { return model.ProviderLocal }

// Capabilities describes local library and device-playlist support.
func (p *Provider) Capabilities() sourceprovider.Capabilities {
	return sourceprovider.Capabilities{Library: true, Playlists: true, Editing: true, Rate: true}
}

// Home is unsupported for local music.
func (p *Provider) Home(context.Context, string) (model.Feed, error) {
	return model.Feed{}, sourceprovider.ErrUnsupported
}

// HomeMore is unsupported for local music.
func (p *Provider) HomeMore(context.Context, string) (model.Feed, error) {
	return model.Feed{}, sourceprovider.ErrUnsupported
}

// Search is unsupported because local filtering is client-side over LocalLibrary.
func (p *Provider) Search(context.Context, string, model.SearchKind) (model.SearchPage, error) {
	return model.SearchPage{}, sourceprovider.ErrUnsupported
}

// SearchMore is unsupported for local music.
func (p *Provider) SearchMore(context.Context, string) (model.SearchPage, error) {
	return model.SearchPage{}, sourceprovider.ErrUnsupported
}

// SearchAll is unsupported for local music.
func (p *Provider) SearchAll(context.Context, string) (model.SearchAll, error) {
	return model.SearchAll{}, sourceprovider.ErrUnsupported
}

// SearchAllMore is unsupported for local music.
func (p *Provider) SearchAllMore(context.Context, string) (model.SearchAll, error) {
	return model.SearchAll{}, sourceprovider.ErrUnsupported
}

// SearchCards is unsupported for local music.
func (p *Provider) SearchCards(context.Context, string, string) (model.CardsPage, error) {
	return model.CardsPage{}, sourceprovider.ErrUnsupported
}

// SearchCardsMore is unsupported for local music.
func (p *Provider) SearchCardsMore(context.Context, string) (model.CardsPage, error) {
	return model.CardsPage{}, sourceprovider.ErrUnsupported
}

// Suggest is unsupported for local music.
func (p *Provider) Suggest(context.Context, string) ([]string, error) {
	return nil, sourceprovider.ErrUnsupported
}

// Album returns an on-device album page.
func (p *Provider) Album(ctx context.Context, id string) (model.AlbumPage, error) {
	if err := ctx.Err(); err != nil {
		return model.AlbumPage{}, err
	}
	key, ok := AlbumKeyFromID(id)
	if !ok {
		return model.AlbumPage{}, sourceprovider.ErrNotFound
	}
	tracks, err := p.index.LocalTracksByAlbum(key)
	if err != nil {
		return model.AlbumPage{}, err
	}
	if len(tracks) == 0 {
		return model.AlbumPage{}, sourceprovider.ErrNotFound
	}
	return albumPage(tracks), nil
}

// Artist returns an on-device artist page.
func (p *Provider) Artist(ctx context.Context, id string) (model.ArtistPage, error) {
	if err := ctx.Err(); err != nil {
		return model.ArtistPage{}, err
	}
	name, ok := ArtistNameFromID(id)
	if !ok {
		return model.ArtistPage{}, sourceprovider.ErrNotFound
	}
	all, err := p.index.AllLocalTracks()
	if err != nil {
		return model.ArtistPage{}, err
	}
	tracks := make([]LocalTrack, 0)
	for _, track := range all {
		if primaryArtist(track.Artist) == name {
			tracks = append(tracks, track)
		}
	}
	if len(tracks) == 0 {
		return model.ArtistPage{}, sourceprovider.ErrNotFound
	}
	nameCopy := name
	return model.ArtistPage{
		Name:       &nameCopy,
		TopSongs:   songsOf(tracks),
		TopSongsID: nil,
		Sections:   []model.ArtistSection{{Title: "Albums", Items: albumsOf(tracks)}},
	}, nil
}

// Playlist returns a device playlist.
func (p *Provider) Playlist(ctx context.Context, id string, _ model.PlaylistSort, _ bool) (model.PlaylistPage, error) {
	if err := ctx.Err(); err != nil {
		return model.PlaylistPage{}, err
	}
	if !strings.HasPrefix(id, model.LocalPlaylistPrefix) {
		return model.PlaylistPage{}, sourceprovider.ErrNotFound
	}
	if id == model.LocalLikedPlaylistID {
		if err := p.ensureLikedPlaylist(); err != nil {
			return model.PlaylistPage{}, err
		}
	}
	row, ok, err := p.index.LocalPlaylist(id)
	if err != nil {
		return model.PlaylistPage{}, err
	}
	if !ok {
		return model.PlaylistPage{}, sourceprovider.ErrNotFound
	}
	items, err := p.index.LocalPlaylistTracks(id)
	if err != nil {
		return model.PlaylistPage{}, err
	}
	for i := range items {
		items[i] = shedQueueContext(items[i])
		trackID := items[i].VideoID
		items[i].SetVideoID = &trackID
	}
	title := row.Title
	subtitle := fmt.Sprintf("%d track%s · Stored on this device", len(items), plural(len(items)))
	thumbnail := playlistArtwork(items)
	return model.PlaylistPage{
		Title:         &title,
		Subtitle:      &subtitle,
		Thumbnail:     thumbnail,
		Items:         items,
		Owned:         true,
		Collaborative: false,
	}, nil
}

// PlaylistMore is unsupported because device playlists are returned in full.
func (p *Provider) PlaylistMore(context.Context, string) (model.PlaylistTracksPage, error) {
	return model.PlaylistTracksPage{}, sourceprovider.ErrUnsupported
}

// Library returns local albums, artists, liked tracks, and device playlists.
func (p *Provider) Library(ctx context.Context) (model.Library, error) {
	library, err := p.LocalLibrary(ctx)
	if err != nil {
		return model.Library{}, err
	}
	if err := p.ensureLikedPlaylist(); err != nil {
		return model.Library{}, err
	}
	playlists, err := p.index.LocalPlaylists()
	if err != nil {
		return model.Library{}, err
	}
	cards := make([]model.BrowseItem, 0, len(playlists))
	for _, playlist := range playlists {
		items, err := p.index.LocalPlaylistTracks(playlist.ID)
		if err != nil {
			return model.Library{}, err
		}
		cards = append(cards, playlistCard(playlist, items))
	}
	liked, err := p.index.LocalPlaylistTracks(model.LocalLikedPlaylistID)
	if err != nil {
		return model.Library{}, err
	}
	return model.Library{Liked: liked, Playlists: cards, Albums: library.Albums, Artists: library.Artists}, nil
}

// Resolve returns an indexed local file as a file URL.
func (p *Provider) Resolve(ctx context.Context, track model.Track, _ model.Quality) (model.Stream, error) {
	if err := ctx.Err(); err != nil {
		return model.Stream{}, err
	}
	path, ok := PathFromID(track.VideoID)
	if !ok {
		return model.Stream{}, sourceprovider.ErrNotFound
	}
	all, err := p.index.AllLocalTracks()
	if err != nil {
		return model.Stream{}, err
	}
	indexed := false
	for _, candidate := range all {
		if candidate.Path == path {
			indexed = true
			break
		}
	}
	if !indexed {
		return model.Stream{}, sourceprovider.ErrNotFound
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		changed, forgetErr := p.ForgetMissing(ctx, path)
		if forgetErr != nil {
			p.log.Warn("could not remove missing local track", "path", path, "error", forgetErr)
		}
		return model.Stream{}, &MissingError{Path: path, Changed: changed}
	}
	return model.Stream{URL: (&url.URL{Scheme: "file", Path: path}).String()}, nil
}

// Next returns the remaining tracks in the seed's album or untagged folder group.
func (p *Provider) Next(ctx context.Context, seed model.Track, played []string) ([]model.Track, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	path, ok := PathFromID(seed.VideoID)
	if !ok {
		return nil, sourceprovider.ErrNotFound
	}
	all, err := p.index.AllLocalTracks()
	if err != nil {
		return nil, err
	}
	var current *LocalTrack
	for i := range all {
		if all[i].Path == path {
			current = &all[i]
			break
		}
	}
	if current == nil {
		return nil, sourceprovider.ErrNotFound
	}
	group, err := p.index.LocalTracksByAlbum(current.AlbumKey)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(group, func(i, j int) bool {
		left, right := group[i], group[j]
		if left.TrackNo > 0 && right.TrackNo > 0 && left.TrackNo != right.TrackNo {
			return left.TrackNo < right.TrackNo
		}
		if left.TrackNo > 0 && right.TrackNo == 0 {
			return true
		}
		if left.TrackNo == 0 && right.TrackNo > 0 {
			return false
		}
		return strings.ToLower(left.Path) < strings.ToLower(right.Path)
	})
	seen := make(map[string]struct{}, len(played)+1)
	for _, id := range played {
		seen[id] = struct{}{}
	}
	seen[seed.VideoID] = struct{}{}
	out := make([]model.Track, 0, len(group))
	passedSeed := false
	for _, candidate := range group {
		if candidate.Path == path {
			passedSeed = true
			continue
		}
		id := SongID(candidate.Path)
		if !passedSeed {
			continue
		}
		if _, duplicate := seen[id]; !duplicate {
			out = append(out, trackToModel(candidate))
		}
	}
	return out, nil
}

// Account returns the local source's permanently signed-out account shape.
func (p *Provider) Account(context.Context) (model.Account, error) {
	return model.Account{SignedIn: false}, nil
}

// LocalLibrary scans on demand and returns the complete wire catalogue.
func (p *Provider) LocalLibrary(ctx context.Context) (model.LocalLibrary, error) {
	return p.scanner.Scan(ctx)
}

// AddFolder validates, canonicalizes, stores, and scans a music root.
func (p *Provider) AddFolder(ctx context.Context, raw string) (model.LocalLibrary, error) {
	path, err := canonicalMusicFolder(raw)
	if err != nil {
		return model.LocalLibrary{}, err
	}
	folders, err := p.index.LocalFolders()
	if err != nil {
		return model.LocalLibrary{}, err
	}
	for _, folder := range folders {
		if pathWithin(path, folder) {
			return p.LocalLibrary(ctx)
		}
	}
	kept := folders[:0]
	for _, folder := range folders {
		if !pathWithin(folder, path) {
			kept = append(kept, folder)
		}
	}
	kept = append(kept, path)
	if err := p.index.SetLocalFolders(kept); err != nil {
		return model.LocalLibrary{}, err
	}
	return p.LocalLibrary(ctx)
}

// RemoveFolder removes a stored root even when it is no longer mounted, then scans.
func (p *Provider) RemoveFolder(ctx context.Context, raw string) (model.LocalLibrary, error) {
	path := strings.TrimSpace(raw)
	if path == "" {
		return model.LocalLibrary{}, errors.New("Choose a music folder.")
	}
	if canonical, err := filepath.EvalSymlinks(path); err == nil {
		path = canonical
	}
	if absolute, err := filepath.Abs(path); err == nil {
		path = filepath.Clean(absolute)
	}
	folders, err := p.index.LocalFolders()
	if err != nil {
		return model.LocalLibrary{}, err
	}
	kept := folders[:0]
	for _, folder := range folders {
		if folder != path && folder != raw {
			kept = append(kept, folder)
		}
	}
	if err := p.index.SetLocalFolders(kept); err != nil {
		return model.LocalLibrary{}, err
	}
	return p.LocalLibrary(ctx)
}

// ForgetMissing deletes one missing file and returns the local-changed payload.
func (p *Provider) ForgetMissing(ctx context.Context, path string) (model.LocalChanged, error) {
	if err := ctx.Err(); err != nil {
		return model.LocalChanged{}, err
	}
	before, err := p.index.AllLocalTracks()
	if err != nil {
		return model.LocalChanged{}, err
	}
	if err := p.index.DeleteLocalTrack(path); err != nil {
		return model.LocalChanged{}, err
	}
	removed := []string{SongID(path)}
	var gone *LocalTrack
	for i := range before {
		if before[i].Path == path {
			gone = &before[i]
			break
		}
	}
	if gone == nil {
		return model.LocalChanged{Removed: removed}, nil
	}
	albumStillPresent, artistStillPresent := false, false
	for _, track := range before {
		if track.Path == path {
			continue
		}
		albumStillPresent = albumStillPresent || track.AlbumKey == gone.AlbumKey
		artistStillPresent = artistStillPresent || primaryArtist(track.Artist) == primaryArtist(gone.Artist)
	}
	if !albumStillPresent {
		removed = append(removed, AlbumID(gone.AlbumKey))
	}
	if !artistStillPresent {
		removed = append(removed, ArtistID(primaryArtist(gone.Artist)))
	}
	return model.LocalChanged{Removed: removed}, nil
}

// CreatePlaylist creates a device playlist and returns its synthetic ID.
func (p *Provider) CreatePlaylist(_ context.Context, title string) (string, error) {
	title, err := playlistTitle(title)
	if err != nil {
		return "", err
	}
	var random [12]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", fmt.Errorf("create device playlist id: %w", err)
	}
	id := model.LocalPlaylistPrefix + hex.EncodeToString(random[:])
	now := time.Now().Unix()
	row := model.DevicePlaylist{ID: id, Title: title, CreatedAt: now, UpdatedAt: now}
	if err := p.index.CreateLocalPlaylist(row); err != nil {
		return "", err
	}
	return id, nil
}

// EditPlaylist renames a device playlist; descriptions and privacy are unsupported.
func (p *Provider) EditPlaylist(_ context.Context, id string, name, description *string, public *bool) error {
	if !strings.HasPrefix(id, model.LocalPlaylistPrefix) {
		return sourceprovider.ErrNotFound
	}
	if description != nil || public != nil {
		return errors.New("Device playlists only store a name and local artwork.")
	}
	if _, ok, err := p.index.LocalPlaylist(id); err != nil {
		return err
	} else if !ok {
		return sourceprovider.ErrNotFound
	}
	if name == nil {
		return nil
	}
	title, err := playlistTitle(*name)
	if err != nil {
		return err
	}
	return p.index.RenameLocalPlaylist(id, title)
}

// DeletePlaylist deletes one device playlist and its membership.
func (p *Provider) DeletePlaylist(_ context.Context, id string) error {
	if !strings.HasPrefix(id, model.LocalPlaylistPrefix) {
		return sourceprovider.ErrNotFound
	}
	if _, ok, err := p.index.LocalPlaylist(id); err != nil {
		return err
	} else if !ok {
		return sourceprovider.ErrNotFound
	}
	return p.index.DeleteLocalPlaylist(id)
}

// AddToPlaylist adds an indexed local track to a device playlist.
func (p *Provider) AddToPlaylist(ctx context.Context, playlistID, trackID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	path, ok := PathFromID(trackID)
	if !ok {
		return sourceprovider.ErrNotFound
	}
	all, err := p.index.AllLocalTracks()
	if err != nil {
		return err
	}
	for _, track := range all {
		if track.Path == path {
			_, err = p.AddTrackToPlaylist(ctx, playlistID, trackToModel(track))
			return err
		}
	}
	return sourceprovider.ErrNotFound
}

// AddTrackToPlaylist adds a full wire track to a device playlist.
func (p *Provider) AddTrackToPlaylist(ctx context.Context, playlistID string, track model.Track) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if !strings.HasPrefix(playlistID, model.LocalPlaylistPrefix) {
		return false, errors.New("not a device playlist")
	}
	if model.ProviderOf(track.VideoID) == model.ProviderRadio {
		return false, errors.New("Live radio stations cannot be added to song playlists.")
	}
	if _, ok, err := p.index.LocalPlaylist(playlistID); err != nil {
		return false, err
	} else if !ok {
		return false, sourceprovider.ErrNotFound
	}
	return p.index.AddLocalPlaylistTrack(playlistID, shedQueueContext(track))
}

// RemoveFromPlaylist removes one track from a device playlist.
func (p *Provider) RemoveFromPlaylist(_ context.Context, playlistID, trackID, _ string) error {
	if !strings.HasPrefix(playlistID, model.LocalPlaylistPrefix) {
		return sourceprovider.ErrNotFound
	}
	return p.index.RemoveLocalPlaylistTrack(playlistID, trackID)
}

// Rate stores local likes in the fixed on-device Liked Songs playlist.
func (p *Provider) Rate(ctx context.Context, trackID string, rating model.Rating) error {
	if err := p.ensureLikedPlaylist(); err != nil {
		return err
	}
	if rating != model.RatingLike {
		return p.index.RemoveLocalPlaylistTrack(model.LocalLikedPlaylistID, trackID)
	}
	return p.AddToPlaylist(ctx, model.LocalLikedPlaylistID, trackID)
}

// PlaylistMemberships returns all device-playlist memberships.
func (p *Provider) PlaylistMemberships() (model.PlaylistIndex, error) {
	return p.index.LocalPlaylistMemberships()
}

// MissingError reports an indexed local file that disappeared before playback.
type MissingError struct {
	Path    string
	Changed model.LocalChanged
}

// Error returns the missing-file failure.
func (e *MissingError) Error() string { return "local file is no longer available" }

// Is makes MissingError match provider.ErrUnavailable.
func (e *MissingError) Is(target error) bool { return target == sourceprovider.ErrUnavailable }

// SongID builds a local track ID.
func SongID(path string) string { return SongPrefix + path }

// PathFromID extracts a nonempty path from a local track ID.
func PathFromID(id string) (string, bool) {
	path, ok := strings.CutPrefix(id, SongPrefix)
	return path, ok && path != ""
}

// AlbumID builds a local album ID.
func AlbumID(key string) string { return AlbumPrefix + key }

// AlbumKeyFromID extracts a nonempty key from a local album ID.
func AlbumKeyFromID(id string) (string, bool) {
	key, ok := strings.CutPrefix(id, AlbumPrefix)
	return key, ok && key != ""
}

// ArtistID builds a local artist ID.
func ArtistID(name string) string { return ArtistPrefix + name }

// ArtistNameFromID extracts a nonempty name from a local artist ID.
func ArtistNameFromID(id string) (string, bool) {
	name, ok := strings.CutPrefix(id, ArtistPrefix)
	return name, ok && name != ""
}

func (p *Provider) ensureLikedPlaylist() error {
	_, ok, err := p.index.LocalPlaylist(model.LocalLikedPlaylistID)
	if err != nil || ok {
		return err
	}
	now := time.Now().Unix()
	return p.index.CreateLocalPlaylist(model.DevicePlaylist{
		ID: model.LocalLikedPlaylistID, Title: "Liked Songs", CreatedAt: now, UpdatedAt: now,
	})
}

func canonicalMusicFolder(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("Choose a music folder.")
	}
	path, err := filepath.EvalSymlinks(raw)
	if err != nil {
		return "", errors.New("That music folder no longer exists or cannot be accessed.")
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return "", errors.New("That music folder no longer exists or cannot be accessed.")
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", errors.New("That music folder no longer exists or cannot be accessed.")
	}
	if !info.IsDir() {
		return "", errors.New("Choose a directory, not a file.")
	}
	path = filepath.Clean(path)
	if filepath.Dir(path) == path {
		return "", errors.New("For safety, choose a music folder instead of the filesystem root.")
	}
	directory, err := os.Open(path)
	if err != nil {
		return "", errors.New("That music folder no longer exists or cannot be accessed.")
	}
	_, readErr := directory.Readdirnames(1)
	closeErr := directory.Close()
	if (readErr != nil && !errors.Is(readErr, io.EOF)) || closeErr != nil {
		return "", errors.New("That music folder no longer exists or cannot be accessed.")
	}
	return path, nil
}

func pathWithin(path, root string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func playlistTitle(raw string) (string, error) {
	title := strings.TrimSpace(raw)
	if title == "" {
		return "", errors.New("Playlist name cannot be empty.")
	}
	if utf8.RuneCountInString(title) <= 150 {
		return title, nil
	}
	runes := []rune(title)
	return string(runes[:150]), nil
}

func shedQueueContext(track model.Track) model.Track {
	track.QueuedBy = nil
	track.Queued = false
	track.QueuedEnd = false
	track.QueuedFrom = nil
	track.Autoplay = false
	track.SetVideoID = nil
	track.AddedBy = nil
	track.AddedByAvatar = nil
	return track
}

func plural(count int) string {
	if count == 1 {
		return ""
	}
	return "s"
}

var (
	_ sourceprovider.Provider       = (*Provider)(nil)
	_ sourceprovider.Rater          = (*Provider)(nil)
	_ sourceprovider.PlaylistEditor = (*Provider)(nil)
)
