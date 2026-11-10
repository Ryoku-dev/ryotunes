package daemon

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ryoku-dev/ryotunes/internal/integrations/listentogether"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
	"github.com/ryoku-dev/ryotunes/internal/provider"
	"github.com/ryoku-dev/ryotunes/internal/provider/soundcloud"
	"github.com/ryoku-dev/ryotunes/internal/provider/youtube"
	"github.com/ryoku-dev/ryotunes/internal/store"
)

type queuePersist struct{ kv provider.KV }

func (p queuePersist) Get(key string) (string, bool) { return p.kv.Get(key) }
func (p queuePersist) Set(key, value string) error   { return p.kv.Set(key, value) }

type queueHooks struct{ app *App }

func (h queueHooks) DownloadedStream(trackID string) (model.Stream, bool) {
	if h.app.Downloads == nil {
		return model.Stream{}, false
	}
	return h.app.Downloads.Stream(trackID)
}

func (h queueHooks) OnPlayed(ctx context.Context, track model.Track) {
	if ctx.Err() != nil {
		return
	}
	if err := h.app.Store.RecordPlay(track); err != nil {
		h.app.Log.Warn("recording played track", "error", err)
	}
	h.app.syncIntegrations(false)
}

func (h queueHooks) OnPlaybackStart(ctx context.Context, track model.Track, playlistID string) {
	if model.ProviderOf(track.VideoID) != model.ProviderYouTube || h.app.YouTube == nil {
		return
	}
	// taken first so a resolve's ping never outlives the play it was minted for
	ping, found := h.app.YouTube.TakePlaybackPing(track.VideoID)
	if !h.app.Store.Settings().EnableHistory {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if !found {
		var err error
		if ping, err = h.app.YouTube.PlaybackPing(ctx, track); err != nil {
			if ctx.Err() == nil {
				h.app.Log.Warn("fetching YouTube playback metadata", "track", track.VideoID, "error", err)
			}
			return
		}
	}
	if err := h.app.YouTube.ReportPlayback(ctx, ping, youtubePlaylistContext(playlistID)); err != nil && ctx.Err() == nil {
		h.app.Log.Warn("reporting YouTube playback", "track", track.VideoID, "error", err)
	}
}

func youtubePlaylistContext(id string) string {
	id = strings.TrimSpace(id)
	id = strings.TrimPrefix(id, "VL")
	switch {
	case strings.HasPrefix(id, "PL"), strings.HasPrefix(id, "RD"), strings.HasPrefix(id, "OLAK5uy_"):
		return id
	default:
		return ""
	}
}

func (h queueHooks) StreamCacheGet(trackID string) (model.Stream, bool) {
	cached, err := h.app.Store.GetStream(trackID, time.Now())
	if err != nil {
		h.app.Log.Warn("reading stream cache", "track", trackID, "error", err)
		return model.Stream{}, false
	}
	if cached == nil {
		return model.Stream{}, false
	}
	expires := cached.ExpiresAt.Unix()
	return model.Stream{URL: cached.URL, ExpiresAt: &expires, LoudnessDB: cached.LoudnessDB}, true
}

func (h queueHooks) StreamCachePut(trackID string, stream model.Stream) {
	if stream.URL == "" || stream.ExpiresAt == nil {
		return
	}
	if err := h.app.Store.PutStream(store.CachedStream{
		VideoID: trackID, URL: stream.URL, ExpiresAt: time.Unix(*stream.ExpiresAt, 0),
		LoudnessDB: stream.LoudnessDB,
	}); err != nil {
		h.app.Log.Warn("writing stream cache", "track", trackID, "error", err)
	}
}

func (h queueHooks) Quality() model.Quality  { return h.app.Store.Settings().Quality }
func (h queueHooks) Autoplay() bool          { return h.app.Store.Settings().Autoplay }
func (h queueHooks) PreventDuplicates() bool { return h.app.Store.Settings().PreventDuplicates }
func (h queueHooks) HealIdentity() bool {
	ctx, cancel := context.WithTimeout(h.app.ctx, 30*time.Second)
	defer cancel()
	if err := h.app.YouTube.RefreshVisitorData(ctx); err != nil {
		h.app.Log.Info("YouTube visitor identity repair failed", "error", err)
		return false
	}
	h.app.Log.Info("refreshed YouTube visitor identity after playback gate")
	return true
}

type lyricsCache struct{ store *store.Store }

func (c lyricsCache) GetLyrics(videoID string, now time.Time, missTTL time.Duration) (*model.Lyrics, bool, error) {
	return c.store.GetLyrics(videoID, now, missTTL)
}
func (c lyricsCache) PutLyrics(videoID string, value *model.Lyrics, fetchedAt time.Time) error {
	return c.store.PutLyrics(videoID, value, fetchedAt)
}

type youtubeDownloadSearcher struct{ youtube *youtube.Client }

func (s youtubeDownloadSearcher) Match(ctx context.Context, query string) (string, error) {
	page, err := s.youtube.Search(ctx, query, model.SearchKindSongs)
	if err != nil {
		return "", err
	}
	if len(page.Items) == 0 || page.Items[0].VideoID == "" {
		return "", fmt.Errorf("%w: no YouTube download match", provider.ErrNotFound)
	}
	return page.Items[0].VideoID, nil
}

type soundCloudDownloadResolver struct{ soundcloud *soundcloud.Provider }

func (r soundCloudDownloadResolver) Permalink(ctx context.Context, trackID string) (string, error) {
	return r.soundcloud.Permalink(ctx, trackID)
}

type downloadLyricsSource struct {
	lyrics interface {
		Get(context.Context, protocol.LyricsParams) (*model.Lyrics, error)
	}
}

func (s downloadLyricsSource) Lyrics(ctx context.Context, params protocol.LyricsParams) (*model.Lyrics, error) {
	return s.lyrics.Get(ctx, params)
}

type mprisController struct{ app *App }

func (c mprisController) PlayPause() error { return c.app.Queue.TogglePause(c.app.ctx) }
func (c mprisController) Play() error      { return c.app.Queue.PlayTransport(c.app.ctx) }
func (c mprisController) Pause() error     { return c.app.Queue.Pause() }
func (c mprisController) Stop() error      { return c.app.Queue.Stop() }
func (c mprisController) Next() error      { return c.app.Queue.Next(c.app.ctx) }
func (c mprisController) Previous() error  { return c.app.Queue.Prev(c.app.ctx) }
func (c mprisController) Seek(offset float64) error {
	return c.app.Queue.Seek(max(0, c.app.Queue.Playback().Position+offset))
}
func (c mprisController) SetPosition(position float64) error { return c.app.Queue.Seek(position) }
func (c mprisController) SetVolume(percent int) error        { return c.app.Queue.SetVolume(percent) }
func (c mprisController) SetLoopStatus(mode model.RepeatMode) error {
	return c.app.Queue.SetRepeat(mode)
}
func (c mprisController) SetShuffle(shuffle bool) error {
	if c.app.Queue.Snapshot().Shuffle != shuffle {
		c.app.Queue.ToggleShuffle()
	}
	return nil
}
func (c mprisController) Raise() error {
	if c.app.Server == nil {
		return errors.New("daemon server is not ready")
	}
	return c.app.Server.Show()
}
func (c mprisController) Quit() error { c.app.Quit(); return nil }

type trayActions struct{ app *App }

func (a trayActions) Show() error {
	if a.app.Server == nil {
		return errors.New("daemon server is not ready")
	}
	return a.app.Server.Show()
}
func (a trayActions) PlayPause() error { return a.app.Queue.TogglePause(a.app.ctx) }
func (a trayActions) Next() error      { return a.app.Queue.Next(a.app.ctx) }
func (a trayActions) Previous() error  { return a.app.Queue.Prev(a.app.ctx) }
func (a trayActions) Quit() error      { a.app.Quit(); return nil }

type listenTogetherHost struct{ app *App }

func (h listenTogetherHost) PlayTrack(ctx context.Context, track model.Track, positionMS int64, playing bool) error {
	return h.app.Queue.Follow(ctx, track, float64(positionMS)/1000, playing)
}
func (h listenTogetherHost) Pause(context.Context) error      { return h.app.Queue.Pause() }
func (h listenTogetherHost) Resume(ctx context.Context) error { return h.app.Queue.PlayTransport(ctx) }
func (h listenTogetherHost) Seek(_ context.Context, positionMS int64) error {
	return h.app.Queue.Seek(float64(positionMS) / 1000)
}
func (h listenTogetherHost) Enqueue(ctx context.Context, track model.Track, _ bool) error {
	return h.app.Queue.AddToQueue(ctx, []model.Track{track}, nil, nil)
}
func (h listenTogetherHost) PlaybackSnapshot() listentogether.PlaybackSnapshot {
	playback := h.app.Queue.Playback()
	queueState := h.app.Queue.Snapshot()
	var current *model.Track
	if queueState.CurrentIndex >= 0 && queueState.CurrentIndex < len(queueState.Items) {
		track := queueState.Items[queueState.CurrentIndex]
		current = &track
	}
	items := append([]model.Track(nil), queueState.Items...)
	return listentogether.PlaybackSnapshot{
		Track: current, Queue: items, Playing: playback.Now != nil && !playback.Paused,
		PositionMS: int64(playback.Position * 1000), Volume: float64(playback.Volume) / 100,
	}
}
func (h listenTogetherHost) Username() string {
	account := h.app.YouTube.CurrentAccount()
	if account.Name != nil && *account.Name != "" {
		return *account.Name
	}
	return "Host"
}
