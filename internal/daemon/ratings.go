package daemon

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

// YouTube Music never says whether a song in a queue, radio, search or home
// answer is liked (only Liked music's own rows carry it), so the daemon keeps
// the signed-in account's liked songs and fills in every track it hands out
// that came without a rating. Nothing of it is stored on disk.
const (
	// likesLimit bounds how many liked songs are read; YouTube pages them about
	// a hundred at a time.
	likesLimit = 5000
	likesPages = likesLimit / 50
	// likesRefresh re-reads the likes, catching ones made on another device.
	likesRefresh = 30 * time.Minute
	// likesRetry is the first wait after a failed read; it doubles up to likesRefresh.
	likesRetry  = time.Minute
	likesFetch  = 2 * time.Minute
	likedMusic  = "VLLM"
	likesDevice = "device"
)

// ratingBook is what the daemon knows about liked songs, one set per place a
// like lives: the YouTube Music account, Spotify, SoundCloud, and the device's
// Liked songs playlist (local files, and every song while YouTube Music is
// signed out or a service has no likes of its own).
type ratingBook struct {
	mu sync.Mutex
	// youtubeSignedIn routes YouTube songs to the account's likes or the device's.
	youtubeSignedIn bool
	sets            map[string]*likedSet
}

// likedSet is one account's liked songs. Once loaded, a song missing from it is
// not liked; before that only songs rated since are known.
type likedSet struct {
	account string
	loaded  bool
	liked   map[string]struct{}
	// rated holds ratings confirmed since the load running at epoch began; a
	// finished load drops the ones older than it.
	rated map[string]ratedSong
	epoch uint64
}

type ratedSong struct {
	rating model.Rating
	epoch  uint64
}

func newRatingBook() *ratingBook {
	return &ratingBook{sets: make(map[string]*likedSet)}
}

// setLocked returns the set for key, starting a new one when the account changed.
func (b *ratingBook) setLocked(key, account string) *likedSet {
	set := b.sets[key]
	if set == nil || set.account != account {
		set = &likedSet{account: account, rated: make(map[string]ratedSong)}
		b.sets[key] = set
	}
	return set
}

// homeLocked names the set that holds the rating of the song with id.
func (b *ratingBook) homeLocked(id string) string {
	switch model.ProviderOf(id) {
	case model.ProviderYouTube:
		if b.youtubeSignedIn {
			return string(model.ProviderYouTube)
		}
		return likesDevice
	case model.ProviderSpotify, model.ProviderSoundCloud:
		return string(model.ProviderOf(id))
	case model.ProviderRadio, "":
		return ""
	default:
		return likesDevice
	}
}

// Rating implements queue.Ratings: the rating of the song with id, or nil
// while its account's likes aren't known.
func (b *ratingBook) Rating(id string) *model.Rating {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.ratingLocked(id)
}

func (b *ratingBook) ratingLocked(id string) *model.Rating {
	if id == "" {
		return nil
	}
	set := b.sets[b.homeLocked(id)]
	if set == nil {
		return nil
	}
	rating := model.RatingIndifferent
	if rated, ok := set.rated[id]; ok {
		rating = rated.rating
	} else if !set.loaded {
		return nil
	} else if _, liked := set.liked[id]; liked {
		rating = model.RatingLike
	}
	return &rating
}

// setYouTubeAccount follows the YouTube Music account; account is "" when
// signed out. It reports whether where YouTube songs' likes come from changed.
func (b *ratingBook) setYouTubeAccount(account string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	signedIn := account != ""
	changed := signedIn != b.youtubeSignedIn
	b.youtubeSignedIn = signedIn
	if set := b.sets[string(model.ProviderYouTube)]; signedIn && (set == nil || set.account != account) {
		changed = changed || (set != nil && set.loaded)
		b.setLocked(string(model.ProviderYouTube), account)
	}
	return changed
}

// rate records a rating the account just confirmed.
func (b *ratingBook) rate(key, account, id string, rating model.Rating) {
	b.mu.Lock()
	defer b.mu.Unlock()
	set := b.setLocked(key, account)
	set.rated[id] = ratedSong{rating: rating, epoch: set.epoch}
}

// account returns the account the set for key belongs to.
func (b *ratingBook) account(key string) string {
	b.mu.Lock()
	defer b.mu.Unlock()
	if set := b.sets[key]; set != nil {
		return set.account
	}
	return ""
}

// forget drops what is known of the set for key (its account signed in or out).
func (b *ratingBook) forget(key string) {
	b.mu.Lock()
	delete(b.sets, key)
	b.mu.Unlock()
}

// beginLoad starts reading the likes of account into the set for key.
func (b *ratingBook) beginLoad(key, account string) uint64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	set := b.setLocked(key, account)
	set.epoch++
	return set.epoch
}

// finishLoad installs the likes read by the load begun at epoch, keeping the
// ratings confirmed while it ran. It reports whether what is known changed.
func (b *ratingBook) finishLoad(key, account string, epoch uint64, ids []string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	set := b.sets[key]
	if set == nil || set.account != account || set.epoch != epoch {
		return false
	}
	liked := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		liked[id] = struct{}{}
	}
	changed := !set.loaded || len(liked) != len(set.liked)
	for id := range liked {
		if _, ok := set.liked[id]; !ok {
			changed = true
			break
		}
	}
	for id, rated := range set.rated {
		if rated.epoch < epoch {
			delete(set.rated, id)
		}
	}
	set.liked, set.loaded = liked, true
	return changed
}

// youtubeAccount is the key of the YouTube Music account's likes: its channel,
// email or handle, never a display name. "" when signed out.
func youtubeAccount(account model.Account) string {
	if !account.SignedIn {
		return ""
	}
	for _, value := range []*string{account.ChannelID, account.Email, account.Handle} {
		if value != nil && *value != "" {
			return *value
		}
	}
	return "signed-in"
}

// currentAccounter answers the account a provider holds without network work.
type currentAccounter interface {
	CurrentAccount() model.Account
}

// followLikes keeps the YouTube Music account's likes and the device's Liked
// songs loaded until ctx ends: at start, after a sign in or account change, and
// every likesRefresh.
func (a *App) followLikes(ctx context.Context) {
	a.loadDeviceLikes(ctx)
	wait, failures, loadedAt := time.Duration(0), 0, time.Time{}
	for {
		if wait > 0 {
			timer := time.NewTimer(wait)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-a.likesWake:
				timer.Stop()
			case <-timer.C:
				loadedAt = time.Time{}
			}
		}
		account := a.followYouTubeAccount()
		if account == "" {
			wait = likesRefresh
			continue
		}
		if !loadedAt.IsZero() && a.ratings.account(string(model.ProviderYouTube)) == account && time.Since(loadedAt) < likesRefresh {
			wait = likesRefresh - time.Since(loadedAt)
			continue
		}
		if err := a.loadYouTubeLikes(ctx, account); err != nil {
			if ctx.Err() != nil {
				return
			}
			a.Log.Info("YouTube Music liked songs were not read", "error", err)
			wait = min(likesRetry<<min(failures, 5), likesRefresh)
			failures++
			continue
		}
		failures, loadedAt, wait = 0, time.Now(), likesRefresh
	}
}

// followYouTubeAccount routes YouTube songs by the account the provider holds now
// and re-announces the queue when that changes where their likes come from.
func (a *App) followYouTubeAccount() string {
	youtube, ok := a.Providers.Get(model.ProviderYouTube)
	if !ok {
		return ""
	}
	holder, ok := youtube.(currentAccounter)
	if !ok {
		return ""
	}
	account := youtubeAccount(holder.CurrentAccount())
	if a.ratings.setYouTubeAccount(account) {
		a.ratingsChanged()
	}
	return account
}

func (a *App) loadYouTubeLikes(ctx context.Context, account string) error {
	youtube, ok := a.Providers.Get(model.ProviderYouTube)
	if !ok {
		return errors.New("no YouTube Music provider")
	}
	ctx, cancel := context.WithTimeout(ctx, likesFetch)
	defer cancel()
	epoch := a.ratings.beginLoad(string(model.ProviderYouTube), account)
	page, err := youtube.Playlist(ctx, likedMusic, model.PlaylistSortDefault, false)
	if err != nil {
		return err
	}
	ids := make([]string, 0, len(page.Items))
	ids = appendIDs(ids, page.Items)
	token, seen := page.Continuation, make(map[string]bool)
	for range likesPages {
		if token == nil || *token == "" || seen[*token] || len(ids) >= likesLimit {
			break
		}
		seen[*token] = true
		more, err := youtube.PlaylistMore(ctx, *token)
		if err != nil {
			return err
		}
		ids = appendIDs(ids, more.Items)
		token = more.Continuation
	}
	if a.ratings.finishLoad(string(model.ProviderYouTube), account, epoch, ids[:min(len(ids), likesLimit)]) {
		a.ratingsChanged()
	}
	return nil
}

// loadDeviceLikes reads the device's Liked songs playlist once; ratings keep it
// current afterwards.
func (a *App) loadDeviceLikes(ctx context.Context) {
	if a.Local == nil {
		return
	}
	epoch := a.ratings.beginLoad(likesDevice, likesDevice)
	page, err := a.Local.Playlist(ctx, model.LocalLikedPlaylistID, model.PlaylistSortDefault, false)
	if err != nil {
		if ctx.Err() == nil {
			a.Log.Warn("reading the device's liked songs", "error", err)
		}
		return
	}
	if a.ratings.finishLoad(likesDevice, likesDevice, epoch, appendIDs(nil, page.Items)) {
		a.ratingsChanged()
	}
}

func appendIDs(ids []string, items []model.Track) []string {
	for _, item := range items {
		if item.VideoID != "" {
			ids = append(ids, item.VideoID)
		}
	}
	return ids
}

// ratingsChanged re-announces the queue and the playing song's rating after
// what the daemon knows of likes changed.
func (a *App) ratingsChanged() {
	if a.Queue == nil {
		return
	}
	a.Queue.RatingsChanged()
	if now := a.Queue.Playback().Now; now != nil && now.Rating != nil {
		a.Bus.Emit(protocol.EventRating, protocol.RatingEvent{VideoID: now.VideoID, Rating: *now.Rating})
	}
}

// rated records a rating the account (key) just confirmed for the song with id,
// carries it onto the queue and now playing, and announces it to every client.
func (a *App) rated(key, account, id string, rating model.Rating) {
	a.ratings.rate(key, account, id, rating)
	if a.Queue != nil {
		a.Queue.SetRating(id, rating)
	}
	a.Bus.Emit(protocol.EventRating, protocol.RatingEvent{VideoID: id, Rating: rating})
}

// wakeLikes asks followLikes to look at the YouTube Music account again.
func (a *App) wakeLikes() {
	select {
	case a.likesWake <- struct{}{}:
	default:
	}
}

// stampRatings fills in the rating of every song in a method's result that came
// without one, from what the daemon knows of the account's likes. Slices are
// copied before they change: a provider may keep the page it returned.
func (b *ratingBook) stampRatings(result any) any {
	b.mu.Lock()
	defer b.mu.Unlock()
	switch value := result.(type) {
	case []model.Track:
		return b.tracksLocked(value)
	case model.SearchPage:
		value.Items = b.tracksLocked(value.Items)
		return value
	case model.PlaylistPage:
		value.Items = b.tracksLocked(value.Items)
		return value
	case model.PlaylistTracksPage:
		value.Items = b.tracksLocked(value.Items)
		return value
	case model.ArtistPage:
		value.TopSongs = b.tracksLocked(value.TopSongs)
		return value
	case []model.BrowseItem:
		value, _ = b.cardsLocked(value)
		return value
	case model.CardsPage:
		value.Items, _ = b.cardsLocked(value.Items)
		return value
	case model.SearchAll:
		value.Top, _ = b.cardsLocked(value.Top)
		value.Songs, _ = b.cardsLocked(value.Songs)
		return value
	case model.Feed:
		var sections []model.Shelf
		for i, shelf := range value.Sections {
			items, changed := b.cardsLocked(shelf.Items)
			if !changed {
				continue
			}
			if sections == nil {
				sections = append([]model.Shelf(nil), value.Sections...)
			}
			sections[i].Items = items
		}
		if sections != nil {
			value.Sections = sections
		}
		return value
	}
	return result
}

func (b *ratingBook) tracksLocked(items []model.Track) []model.Track {
	var out []model.Track
	for i := range items {
		if items[i].Rating != nil {
			continue
		}
		rating := b.ratingLocked(items[i].VideoID)
		if rating == nil {
			continue
		}
		if out == nil {
			out = append([]model.Track(nil), items...)
		}
		out[i].Rating = rating
	}
	if out == nil {
		return items
	}
	return out
}

// cardsLocked stamps song cards, reporting whether any changed.
func (b *ratingBook) cardsLocked(items []model.BrowseItem) ([]model.BrowseItem, bool) {
	var out []model.BrowseItem
	for i := range items {
		if items[i].Rating != nil || !songCard(items[i]) {
			continue
		}
		rating := b.ratingLocked(items[i].ID)
		if rating == nil {
			continue
		}
		if out == nil {
			out = append([]model.BrowseItem(nil), items...)
		}
		out[i].Rating = rating
	}
	if out == nil {
		return items, false
	}
	return out, true
}

func songCard(item model.BrowseItem) bool {
	switch strings.ToLower(item.Kind) {
	case "song", "track", "video":
		return true
	}
	return false
}
