package queue

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
	"github.com/ryoku-dev/ryotunes/internal/provider"
)

func (q *Queue) playIndex(ctx context.Context, index int, paused bool) error {
	for {
		q.mu.Lock()
		if index < 0 || index >= len(q.state.items) {
			q.mu.Unlock()
			return errors.New("queue index out of range")
		}
		item := q.state.items[index]
		gen := q.generation
		q.mu.Unlock()

		stream, cached, err := q.resolve(ctx, item, true)
		if err != nil {
			q.log.Info("track resolve failed", "track_id", item.VideoID, "title", item.Title, "error", err)
			if errors.Is(err, provider.ErrSignInRequired) {
				q.bus.Emit(protocol.EventPlaybackError, protocol.MessageEvent{Message: signInMessage(model.ProviderOf(item.VideoID))})
				_ = q.player.Pause()
				return err
			}
			if model.ProviderOf(item.VideoID) == model.ProviderLocal {
				q.bus.Emit(protocol.EventPlaybackNotice, protocol.MessageEvent{Message: item.Title + " is no longer on your disk"})
				q.removeUnavailable(index)
				q.changed()
				q.mu.Lock()
				hasNext := index < len(q.state.items)
				q.mu.Unlock()
				if !hasNext {
					return err
				}
				continue
			}
			q.bus.Emit(protocol.EventPlaybackNotice, protocol.MessageEvent{Message: "Skipped (unavailable): " + item.Title})
			q.mu.Lock()
			if q.generation != gen || index+1 >= len(q.state.items) {
				q.mu.Unlock()
				return err
			}
			index++
			q.mu.Unlock()
			continue
		}

		q.mu.Lock()
		startAt := 0.0
		if q.pendingSeekID == item.VideoID && q.pendingSeek > 0 {
			startAt = q.pendingSeek
			q.pendingSeek = 0
			q.pendingSeekID = ""
		}
		q.mu.Unlock()
		q.player.SetLoudness(stream.LoudnessDB)
		err = q.player.Load(ctx, stream, startAt, paused)
		if err != nil && cached {
			stream, _, resolveErr := q.resolveFresh(ctx, item)
			if resolveErr == nil {
				q.player.SetLoudness(stream.LoudnessDB)
				err = q.player.Load(ctx, stream, startAt, paused)
			}
		}
		if err != nil {
			q.log.Info("track load failed", "track_id", item.VideoID, "title", item.Title, "cached", cached, "error", err)
			q.bus.Emit(protocol.EventPlaybackNotice, protocol.MessageEvent{Message: "Skipped (unavailable): " + item.Title})
			q.mu.Lock()
			if index+1 >= len(q.state.items) {
				q.mu.Unlock()
				return err
			}
			index++
			q.mu.Unlock()
			continue
		}

		q.mu.Lock()
		if q.generation != gen {
			q.mu.Unlock()
			return nil
		}
		q.state.current = index
		q.state.playedFrom = min(q.state.playedFrom, index)
		q.position = startAt
		q.duration = 0
		q.playedRecorded = false
		q.retriedTrackID = ""
		q.streamClient = string(model.ProviderOf(item.VideoID))
		if stream.IsLive {
			q.streamClient = "radio"
		}
		now := q.nowPlayingLocked()
		queueIndex := q.indexLocked()
		q.mu.Unlock()
		q.persistPosition(true)
		q.bus.Emit(protocol.EventNowPlaying, now)
		q.bus.Emit(protocol.EventQueueIndex, queueIndex)
		q.persistState()
		q.start(func(ctx context.Context) { q.prime(ctx, gen) })
		q.start(func(ctx context.Context) { q.extendAutoplay(ctx) })
		return nil
	}
}

func (q *Queue) resolve(ctx context.Context, track model.Track, allowCache bool) (model.Stream, bool, error) {
	kind := model.ProviderOf(track.VideoID)
	if allowCache && q.hooks != nil && kind != model.ProviderSpotify && kind != model.ProviderTidal && kind != model.ProviderLocal {
		if stream, ok := q.hooks.StreamCacheGet(track.VideoID); ok && cacheAlive(stream, q.now()) {
			return stream, true, nil
		}
	}
	return q.resolveFresh(ctx, track)
}

func (q *Queue) resolveFresh(ctx context.Context, track model.Track) (model.Stream, bool, error) {
	p, ok := q.registry.ForTrack(track.VideoID)
	if !ok {
		return model.Stream{}, false, fmt.Errorf("%w: provider for %s", provider.ErrNotFound, track.VideoID)
	}
	quality := model.QualityAuto
	if q.hooks != nil {
		quality = q.hooks.Quality()
	}
	stream, err := p.Resolve(ctx, track, quality)
	if errors.Is(err, provider.ErrBotGated) && q.hooks != nil && q.hooks.HealIdentity() {
		stream, err = p.Resolve(ctx, track, quality)
	}
	if err != nil {
		return model.Stream{}, false, err
	}
	kind := model.ProviderOf(track.VideoID)
	if q.hooks != nil && kind != model.ProviderSpotify && kind != model.ProviderTidal && kind != model.ProviderLocal && cacheAlive(stream, q.now()) {
		q.hooks.StreamCachePut(track.VideoID, stream)
	}
	return stream, false, nil
}

func cacheAlive(stream model.Stream, now time.Time) bool {
	return stream.URL != "" && (stream.ExpiresAt == nil || *stream.ExpiresAt > now.Add(time.Minute).Unix())
}

func signInMessage(kind model.Provider) string {
	switch kind {
	case model.ProviderSpotify:
		return "Sign in to Spotify to play this"
	case model.ProviderTidal:
		return "Sign in to TIDAL to play this"
	case model.ProviderSoundCloud:
		return "Sign in to SoundCloud to play this"
	default:
		return "Sign in to YouTube Music to play this"
	}
}

func (q *Queue) removeUnavailable(index int) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if index < 0 || index >= len(q.state.items) {
		return
	}
	q.state.items = append(q.state.items[:index], q.state.items[index+1:]...)
	if index < q.state.current {
		q.state.current--
	}
	q.generation++
}

func (q *Queue) prime(ctx context.Context, generation uint64) {
	q.mu.Lock()
	if q.stopAfterCurrent || generation != q.generation {
		q.mu.Unlock()
		return
	}
	index, ok := q.nextIndexLocked(false)
	if !ok || index == q.state.current {
		q.mu.Unlock()
		return
	}
	q.mu.Unlock()
	for attempts := 0; attempts < 3; attempts++ {
		q.mu.Lock()
		if generation != q.generation || index < 0 || index >= len(q.state.items) {
			q.mu.Unlock()
			return
		}
		item := q.state.items[index]
		q.mu.Unlock()
		if model.ProviderOf(item.VideoID) == model.ProviderSpotify {
			return
		}
		stream, _, err := q.resolve(ctx, item, true)
		if err == nil {
			q.mu.Lock()
			valid := generation == q.generation
			q.mu.Unlock()
			if valid {
				q.player.Prefetch(stream)
			}
			return
		}
		if errors.Is(err, provider.ErrSignInRequired) {
			return
		}
		q.bus.Emit(protocol.EventPlaybackNotice, protocol.MessageEvent{Message: "Skipped (unavailable): " + item.Title})
		q.mu.Lock()
		if generation != q.generation || index <= q.state.current || index >= len(q.state.items) {
			q.mu.Unlock()
			return
		}
		q.state.items = append(q.state.items[:index], q.state.items[index+1:]...)
		q.mu.Unlock()
		q.changed()
	}
}

func (q *Queue) extendAutoplay(ctx context.Context) int {
	return q.extendAutoplayFrom(ctx, false)
}

func (q *Queue) extendAutoplaySeed(ctx context.Context) int {
	return q.extendAutoplayFrom(ctx, true)
}

func (q *Queue) extendAutoplayFrom(ctx context.Context, seedOnly bool) int {
	q.mu.Lock()
	if q.autoplayFetching || q.hooks == nil || !q.hooks.Autoplay() || q.state.repeat != model.RepeatOff || len(q.state.items) == 0 || (!seedOnly && len(q.state.items)-q.state.current > 2) {
		q.mu.Unlock()
		return 0
	}
	seed := q.state.items[len(q.state.items)-1]
	if seedOnly {
		for _, item := range q.state.items {
			if item.VideoID == q.state.radioSeed {
				seed = item
				break
			}
		}
	}
	if kind := model.ProviderOf(seed.VideoID); kind == model.ProviderLocal || kind == model.ProviderRadio {
		q.mu.Unlock()
		return 0
	}
	played := make([]string, 0, len(q.state.items))
	existing := make(map[string]struct{}, len(q.state.items))
	for _, item := range q.state.items {
		played = append(played, item.VideoID)
		existing[item.VideoID] = struct{}{}
	}
	gen := q.generation
	q.autoplayFetching = true
	q.mu.Unlock()
	defer func() {
		q.mu.Lock()
		q.autoplayFetching = false
		q.mu.Unlock()
	}()
	p, ok := q.registry.ForTrack(seed.VideoID)
	if !ok {
		return 0
	}
	items, err := p.Next(ctx, seed, played)
	if err != nil {
		return 0
	}
	appendItems := make([]model.Track, 0, min(len(items), autoplayBatch))
	for _, item := range items {
		if item.VideoID == "" {
			continue
		}
		if _, exists := existing[item.VideoID]; exists {
			continue
		}
		item = cleanQueueFlags(item)
		item.Autoplay = true
		appendItems = append(appendItems, item)
		existing[item.VideoID] = struct{}{}
		if len(appendItems) == autoplayBatch {
			break
		}
	}
	if len(appendItems) == 0 {
		return 0
	}
	q.mu.Lock()
	if q.generation != gen {
		q.mu.Unlock()
		return 0
	}
	q.state.items = append(q.state.items, appendItems...)
	if q.state.shuffleOrig != nil {
		q.state.shuffleOrig = append(q.state.shuffleOrig, cloneTracks(appendItems)...)
		q.shuffleUpcomingLocked()
	}
	q.mu.Unlock()
	q.changed()
	q.start(func(ctx context.Context) { q.prime(ctx, gen) })
	return len(appendItems)
}

// PrefetchStream resolves a track into the shared stream cache without loading it.
func (q *Queue) PrefetchStream(ctx context.Context, track model.Track) {
	if model.ProviderOf(track.VideoID) == model.ProviderSpotify {
		return
	}
	_, _, _ = q.resolve(ctx, track, false)
}

func (q *Queue) fillContinuation(ctx context.Context, generation uint64, token string, queued bool, from *model.QueueContext) {
	p := q.continuationProvider()
	if p == nil {
		return
	}
	var lastEmit time.Time
	pages := 0
	for range maxFillPages {
		if token == "" {
			break
		}
		page, err := p.PlaylistMore(ctx, token)
		if err != nil || len(page.Items) == 0 {
			return
		}
		items := cloneTracks(page.Items)
		q.mu.Lock()
		if q.generation != generation {
			q.mu.Unlock()
			return
		}
		if queued {
			for i := range items {
				items[i].QueuedEnd = true
				if from != nil {
					value := string(*from)
					items[i].QueuedFrom = &value
				}
			}
			at := manualBoundary(q.state.items, q.state.current, false)
			q.state.items = slices.Insert(q.state.items, at, items...)
		} else {
			if q.state.shuffleOrig != nil {
				q.state.shuffleOrig = append(q.state.shuffleOrig, cloneTracks(items)...)
			}
			q.state.items = append(q.state.items, items...)
			if q.state.shuffleOrig != nil {
				q.shuffleUpcomingLocked()
			}
		}
		q.mu.Unlock()
		pages++
		if lastEmit.IsZero() || q.now().Sub(lastEmit) >= time.Second {
			q.emitChanged()
			lastEmit = q.now()
		}
		if page.Continuation == nil {
			break
		}
		token = *page.Continuation
	}
	if pages > 0 {
		q.emitChanged()
		q.persistState()
	}
}

func (q *Queue) continuationProvider() provider.Provider {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.state.sourceID != nil {
		if p, ok := q.registry.ForTrack(*q.state.sourceID); ok {
			return p
		}
	}
	p, _ := q.registry.Get(model.ProviderYouTube)
	return p
}
