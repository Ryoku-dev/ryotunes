package daemon

import (
	"context"
	"fmt"
	"math"

	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
	"github.com/ryoku-dev/ryotunes/internal/provider"
	"github.com/ryoku-dev/ryotunes/internal/provider/radio"
)

func init() {
	register(protocol.MethodPlay, typed(play))
	register(protocol.MethodPlayPlaylist, typed(playPlaylist))
	register(protocol.MethodPlayIndex, typed(playIndex))
	register(protocol.MethodNextTrack, typed(nextTrack))
	register(protocol.MethodPrevTrack, typed(prevTrack))
	register(protocol.MethodTogglePause, typed(togglePause))
	register(protocol.MethodSeek, typed(seek))
	register(protocol.MethodSetVolume, typed(setVolume))
	register(protocol.MethodToggleShuffle, typed(toggleShuffle))
	register(protocol.MethodSetRepeat, typed(setRepeat))
	register(protocol.MethodSetStopAfterCurrent, typed(setStopAfterCurrent))
	register(protocol.MethodAddToQueue, typed(addToQueue))
	register(protocol.MethodPlayNext, typed(playNext))
	register(protocol.MethodMoveInQueue, typed(moveInQueue))
	register(protocol.MethodRemoveFromQueue, typed(removeFromQueue))
	register(protocol.MethodClearQueued, typed(clearQueued))
	register(protocol.MethodPrefetchStream, typed(prefetchStream))
	register(protocol.MethodGetPlayback, typed(getPlayback))
	register(protocol.MethodGetQueue, typed(getQueue))
	register(protocol.MethodSetPlaybackParams, typed(setPlaybackParams))
	register(protocol.MethodSetAudioFx, typed(setAudioFx))
	register(protocol.MethodGetAudioFx, typed(getAudioFx))
	register(protocol.MethodStartRadio, typed(startRadio))
	register(protocol.MethodRadioStations, typed(radioStations))
	register(protocol.MethodPlayRadioStation, typed(playRadioStation))
	register(protocol.MethodGetWaveform, typed(getWaveform))
	register(protocol.MethodRate, typed(rate))
}

func play(ctx context.Context, a *App, p protocol.PlayParams) (protocol.Null, error) {
	return protocol.Null{}, a.Queue.Play(ctx, p.Item)
}
func playPlaylist(ctx context.Context, a *App, p protocol.PlayPlaylistParams) (protocol.Null, error) {
	shuffle := p.Shuffle != nil && *p.Shuffle
	return protocol.Null{}, a.Queue.PlayPlaylist(ctx, p.Items, p.Start, p.SourceID, p.SourceName, shuffle, p.Continuation)
}
func playIndex(ctx context.Context, a *App, p protocol.IndexParams) (protocol.Null, error) {
	return protocol.Null{}, a.Queue.PlayIndex(ctx, p.Index)
}
func nextTrack(ctx context.Context, a *App, _ protocol.Null) (protocol.Null, error) {
	return protocol.Null{}, a.Queue.Next(ctx)
}
func prevTrack(ctx context.Context, a *App, _ protocol.Null) (protocol.Null, error) {
	return protocol.Null{}, a.Queue.Prev(ctx)
}
func togglePause(ctx context.Context, a *App, _ protocol.Null) (protocol.Null, error) {
	return protocol.Null{}, a.Queue.TogglePause(ctx)
}
func seek(_ context.Context, a *App, p protocol.SeekParams) (protocol.Null, error) {
	if math.IsNaN(p.Position) || math.IsInf(p.Position, 0) || p.Position < 0 {
		return protocol.Null{}, fmt.Errorf("Seek position must be a non-negative finite number.")
	}
	return protocol.Null{}, a.Queue.Seek(p.Position)
}
func setVolume(_ context.Context, a *App, p protocol.SetVolumeParams) (protocol.Null, error) {
	return protocol.Null{}, a.Queue.SetVolume(p.Volume)
}
func toggleShuffle(_ context.Context, a *App, _ protocol.Null) (protocol.Null, error) {
	a.Queue.ToggleShuffle()
	return protocol.Null{}, nil
}
func setRepeat(_ context.Context, a *App, p protocol.SetRepeatParams) (protocol.Null, error) {
	if p.Mode != model.RepeatOff && p.Mode != model.RepeatAll && p.Mode != model.RepeatOne {
		return protocol.Null{}, fmt.Errorf("unknown repeat mode: %s", p.Mode)
	}
	return protocol.Null{}, a.Queue.SetRepeat(p.Mode)
}
func setStopAfterCurrent(_ context.Context, a *App, p protocol.SetStopAfterCurrentParams) (protocol.Null, error) {
	a.Queue.SetStopAfterCurrent(p.Enabled)
	return protocol.Null{}, nil
}
func addToQueue(ctx context.Context, a *App, p protocol.AddToQueueParams) (protocol.Null, error) {
	return protocol.Null{}, a.Queue.AddToQueue(ctx, p.Items, p.From, p.Continuation)
}
func playNext(_ context.Context, a *App, p protocol.PlayNextParams) (protocol.Null, error) {
	return protocol.Null{}, a.Queue.PlayNext(p.Items, p.From)
}
func moveInQueue(_ context.Context, a *App, p protocol.MoveInQueueParams) (protocol.Null, error) {
	return protocol.Null{}, a.Queue.MoveInQueue(p.From, p.To)
}
func removeFromQueue(_ context.Context, a *App, p protocol.IndexParams) (protocol.Null, error) {
	return protocol.Null{}, a.Queue.RemoveFromQueue(p.Index)
}
func clearQueued(_ context.Context, a *App, _ protocol.Null) (protocol.Null, error) {
	a.Queue.ClearQueued()
	return protocol.Null{}, nil
}
func prefetchStream(_ context.Context, a *App, p protocol.PrefetchStreamParams) (protocol.Null, error) {
	track := model.Track{VideoID: p.VideoID}
	if p.IsUpload != nil {
		track.IsUpload = *p.IsUpload
	}
	a.background.Add(1)
	go func() {
		defer a.background.Done()
		a.Queue.PrefetchStream(a.ctx, track)
	}()
	return protocol.Null{}, nil
}
func getPlayback(_ context.Context, a *App, _ protocol.Null) (model.PlaybackSnapshot, error) {
	return a.Queue.Playback(), nil
}
func getQueue(_ context.Context, a *App, _ protocol.Null) (model.QueueSnapshot, error) {
	return a.Queue.Snapshot(), nil
}
func setPlaybackParams(_ context.Context, a *App, p model.PlaybackParams) (protocol.Null, error) {
	fx := a.Player.Fx()
	fx.Speed, fx.Semitones = p.Speed, p.Semitones
	if err := a.Player.SetFx(fx); err != nil {
		return protocol.Null{}, err
	}
	a.Bus.Emit(protocol.EventAudioFx, fx)
	return protocol.Null{}, nil
}
func setAudioFx(_ context.Context, a *App, fx model.AudioFx) (protocol.Null, error) {
	if err := a.Player.SetFx(fx); err != nil {
		return protocol.Null{}, err
	}
	a.Bus.Emit(protocol.EventAudioFx, fx)
	return protocol.Null{}, nil
}
func getAudioFx(_ context.Context, a *App, _ protocol.Null) (model.AudioFx, error) {
	return a.Player.Fx(), nil
}
func startRadio(ctx context.Context, a *App, p protocol.StartRadioParams) (protocol.Null, error) {
	if _, ok := radio.UUIDFromID(p.ID); ok || model.ProviderOf(p.ID) == model.ProviderLocal || len(p.ID) >= len(model.LocalPlaylistPrefix) && p.ID[:len(model.LocalPlaylistPrefix)] == model.LocalPlaylistPrefix {
		return protocol.Null{}, fmt.Errorf("This item does not have a YouTube Music radio seed.")
	}
	pr := providerForID(a, p.ID)
	r, ok := pr.(provider.Radio)
	if !ok {
		return protocol.Null{}, provider.ErrUnsupported
	}
	items, continuation, err := r.StartRadio(ctx, p.Kind, p.ID)
	if err != nil {
		return protocol.Null{}, providerError(pr.ID(), err)
	}
	name := p.Name
	cont := &continuation
	if continuation == "" {
		cont = nil
	}
	return protocol.Null{}, a.Queue.PlayPlaylist(ctx, items, nil, &p.ID, name, false, cont)
}
func radioStations(ctx context.Context, a *App, p protocol.RadioStationsParams) ([]model.RadioStation, error) {
	query, offset, limit := "", 0, 36
	if p.Query != nil {
		query = *p.Query
	}
	if p.Offset != nil {
		offset = *p.Offset
	}
	if p.Limit != nil {
		limit = *p.Limit
	}
	return a.Radio.Stations(ctx, query, offset, limit)
}
func playRadioStation(ctx context.Context, a *App, p protocol.PlayRadioStationParams) (protocol.Null, error) {
	station, err := a.Radio.Station(ctx, p.StationUUID)
	if err != nil {
		return protocol.Null{}, err
	}
	return protocol.Null{}, a.Queue.Play(ctx, radio.Track(station))
}
func getWaveform(ctx context.Context, a *App, p protocol.IDParams) (model.Waveform, error) {
	if model.ProviderOf(p.ID) != model.ProviderSoundCloud {
		return model.Waveform{}, fmt.Errorf("get_waveform expects a SoundCloud track id")
	}
	waveform, err := a.SoundCloud.Waveform(ctx, p.ID)
	return waveform, providerError(model.ProviderSoundCloud, err)
}
func rate(ctx context.Context, a *App, p protocol.RateParams) (protocol.Null, error) {
	pr := providerForID(a, p.VideoID)
	useProvider := pr.ID() == model.ProviderSpotify || pr.ID() == model.ProviderSoundCloud || pr.ID() == model.ProviderLocal
	if pr.ID() == model.ProviderYouTube {
		account, err := a.YouTube.Account(ctx)
		useProvider = err == nil && account.SignedIn
	}
	if r, ok := pr.(provider.Rater); ok && useProvider {
		return protocol.Null{}, providerError(pr.ID(), r.Rate(ctx, p.VideoID, p.Rating))
	}
	if p.Rating == model.RatingLike {
		track := model.Track{VideoID: p.VideoID}
		for _, candidate := range a.Queue.Snapshot().Items {
			if candidate.VideoID == p.VideoID {
				track = candidate
				break
			}
		}
		_, err := a.Local.AddTrackToPlaylist(ctx, model.LocalLikedPlaylistID, track)
		return protocol.Null{}, err
	}
	return protocol.Null{}, a.Local.RemoveFromPlaylist(ctx, model.LocalLikedPlaylistID, p.VideoID, "")
}

func providerForID(a *App, id string) provider.Provider {
	if p, ok := a.Providers.ForTrack(id); ok {
		return p
	}
	a.mu.Lock()
	selected := a.provider
	a.mu.Unlock()
	if p, ok := a.Providers.Get(model.Provider(selected)); ok {
		return p
	}
	p, _ := a.Providers.Get(model.ProviderYouTube)
	return p
}
