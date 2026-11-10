package queue

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"math/rand/v2"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ryoku-dev/ryotunes/internal/bus"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/player"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
	"github.com/ryoku-dev/ryotunes/internal/provider"
)

type fakeTransport struct {
	events   chan player.Event
	loaded   []model.Stream
	seek     float64
	pos      float64
	paused   bool
	idle     bool
	stopped  bool
	loudness *float64
	// onLoad runs inside Load, like mpv opening the file before the next command lands
	onLoad func()
}

func newFakeTransport() *fakeTransport {
	return &fakeTransport{events: make(chan player.Event, 8), idle: true, paused: true}
}
func (f *fakeTransport) Load(_ context.Context, stream model.Stream, _ float64, paused bool) error {
	f.loaded = append(f.loaded, stream)
	f.paused, f.idle = paused, false
	if f.onLoad != nil {
		f.onLoad()
	}
	return nil
}
func (f *fakeTransport) Prefetch(model.Stream)       {}
func (f *fakeTransport) Play() error                 { f.paused = false; return nil }
func (f *fakeTransport) Pause() error                { f.paused = true; return nil }
func (f *fakeTransport) TogglePause() error          { f.paused = !f.paused; return nil }
func (f *fakeTransport) Stop() error                 { f.stopped, f.idle = true, true; return nil }
func (f *fakeTransport) Seek(position float64) error { f.seek, f.pos = position, position; return nil }
func (f *fakeTransport) SetVolume(int) error         { return nil }
func (f *fakeTransport) SetLoudness(db *float64)     { f.loudness = db }
func (f *fakeTransport) Position() float64           { return f.pos }
func (f *fakeTransport) Duration() float64           { return 0 }
func (f *fakeTransport) Paused() bool                { return f.paused }
func (f *fakeTransport) Idle() bool                  { return f.idle }
func (f *fakeTransport) Events() <-chan player.Event { return f.events }

type playbackStartCall struct {
	track      model.Track
	playlistID string
}

type fakeHooks struct {
	autoplay       bool
	prevent        bool
	downloads      map[string]model.Stream
	cache          map[string]model.Stream
	downloadCalls  atomic.Int32
	playbackStarts chan playbackStartCall
	holdPlayback   chan struct{}
}

func (f *fakeHooks) DownloadedStream(trackID string) (model.Stream, bool) {
	f.downloadCalls.Add(1)
	stream, ok := f.downloads[trackID]
	return stream, ok
}
func (f *fakeHooks) OnPlayed(context.Context, model.Track) {}
func (f *fakeHooks) OnPlaybackStart(ctx context.Context, track model.Track, playlistID string) {
	if f.playbackStarts != nil {
		f.playbackStarts <- playbackStartCall{track: track, playlistID: playlistID}
	}
	if f.holdPlayback != nil {
		select {
		case <-f.holdPlayback:
		case <-ctx.Done():
		}
	}
}
func (f *fakeHooks) StreamCacheGet(trackID string) (model.Stream, bool) {
	stream, ok := f.cache[trackID]
	return stream, ok
}
func (f *fakeHooks) StreamCachePut(string, model.Stream) {}
func (f *fakeHooks) Quality() model.Quality              { return model.QualityAuto }
func (f *fakeHooks) Autoplay() bool                      { return f.autoplay }
func (f *fakeHooks) PreventDuplicates() bool             { return f.prevent }
func (f *fakeHooks) HealIdentity() bool                  { return false }

type fakeProvider struct {
	id           model.Provider
	next         []model.Track
	nextCalls    atomic.Int32
	resolveCalls atomic.Int32
	resolveErr   error
	nextStarted  chan struct{}
	blockNext    bool
}

func (f *fakeProvider) ID() model.Provider {
	if f.id == "" {
		return model.ProviderYouTube
	}
	return f.id
}
func (f *fakeProvider) Capabilities() provider.Capabilities              { return provider.Capabilities{} }
func (f *fakeProvider) Home(context.Context, string) (model.Feed, error) { return model.Feed{}, nil }
func (f *fakeProvider) HomeMore(context.Context, string) (model.Feed, error) {
	return model.Feed{}, nil
}
func (f *fakeProvider) Search(context.Context, string, model.SearchKind) (model.SearchPage, error) {
	return model.SearchPage{}, nil
}
func (f *fakeProvider) SearchMore(context.Context, string) (model.SearchPage, error) {
	return model.SearchPage{}, nil
}
func (f *fakeProvider) SearchAll(context.Context, string) (model.SearchAll, error) {
	return model.SearchAll{}, nil
}
func (f *fakeProvider) SearchAllMore(context.Context, string) (model.SearchAll, error) {
	return model.SearchAll{}, nil
}
func (f *fakeProvider) SearchCards(context.Context, string, string) (model.CardsPage, error) {
	return model.CardsPage{}, nil
}
func (f *fakeProvider) SearchCardsMore(context.Context, string) (model.CardsPage, error) {
	return model.CardsPage{}, nil
}
func (f *fakeProvider) Suggest(context.Context, string) ([]string, error) { return nil, nil }
func (f *fakeProvider) Album(context.Context, string) (model.AlbumPage, error) {
	return model.AlbumPage{}, nil
}
func (f *fakeProvider) Artist(context.Context, string) (model.ArtistPage, error) {
	return model.ArtistPage{}, nil
}
func (f *fakeProvider) Playlist(context.Context, string, model.PlaylistSort, bool) (model.PlaylistPage, error) {
	return model.PlaylistPage{}, nil
}
func (f *fakeProvider) PlaylistMore(context.Context, string) (model.PlaylistTracksPage, error) {
	return model.PlaylistTracksPage{}, nil
}
func (f *fakeProvider) Library(context.Context) (model.Library, error) { return model.Library{}, nil }
func (f *fakeProvider) Resolve(_ context.Context, track model.Track, _ model.Quality) (model.Stream, error) {
	f.resolveCalls.Add(1)
	if f.resolveErr != nil {
		return model.Stream{}, f.resolveErr
	}
	return model.Stream{URL: "https://stream/" + track.VideoID}, nil
}
func (f *fakeProvider) Next(ctx context.Context, _ model.Track, _ []string) ([]model.Track, error) {
	f.nextCalls.Add(1)
	if f.blockNext {
		close(f.nextStarted)
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return cloneTracks(f.next), nil
}
func (f *fakeProvider) Account(context.Context) (model.Account, error) { return model.Account{}, nil }

func testQueue(t *testing.T, p *fakeProvider, hooks *fakeHooks) (*Queue, *fakeTransport) {
	t.Helper()
	transport := newFakeTransport()
	ctx, cancel := context.WithCancel(context.Background())
	q := &Queue{
		player: transport, registry: provider.NewRegistry(p), hooks: hooks, bus: bus.Discard, log: slog.Default(),
		rng: rand.New(rand.NewPCG(1, 2)), now: time.Now, volume: 100,
		state: state{repeat: model.RepeatOff}, ctx: ctx, cancel: cancel, closed: make(chan struct{}),
	}
	t.Cleanup(q.Close)
	return q, transport
}

func tracks(ids ...string) []model.Track {
	out := make([]model.Track, len(ids))
	for i, id := range ids {
		out[i] = model.Track{VideoID: id, Title: id, Artists: "artist"}
	}
	return out
}

func ids(items []model.Track) []string {
	out := make([]string, len(items))
	for i, item := range items {
		out[i] = item.VideoID
	}
	return out
}

func TestShuffleUnshuffleRestoresOrder(t *testing.T) {
	q, _ := testQueue(t, &fakeProvider{}, &fakeHooks{})
	q.state.items = tracks("a", "b", "c", "d", "e")
	q.state.current = 0
	original := ids(q.state.items)
	if !q.ToggleShuffle() {
		t.Fatal("shuffle did not turn on")
	}
	if q.ToggleShuffle() {
		t.Fatal("shuffle did not turn off")
	}
	if got := ids(q.state.items); !reflect.DeepEqual(got, original) {
		t.Fatalf("unshuffle = %v, want %v", got, original)
	}
}

func TestRepeatIndexes(t *testing.T) {
	q, _ := testQueue(t, &fakeProvider{}, &fakeHooks{})
	q.state.items = tracks("a", "b")
	q.state.current = 1
	q.state.repeat = model.RepeatOff
	if _, ok := q.nextIndexLocked(false); ok {
		t.Fatal("repeat off wrapped")
	}
	q.state.repeat = model.RepeatAll
	if got, ok := q.nextIndexLocked(false); !ok || got != 0 {
		t.Fatalf("repeat all = %d, %v", got, ok)
	}
	q.state.repeat = model.RepeatOne
	if got, ok := q.nextIndexLocked(false); !ok || got != 1 {
		t.Fatalf("repeat one EOF = %d, %v", got, ok)
	}
	if got, ok := q.nextIndexLocked(true); !ok || got != 0 {
		t.Fatalf("manual next in repeat one = %d, %v", got, ok)
	}
}

func TestPrevRestartThreshold(t *testing.T) {
	q, transport := testQueue(t, &fakeProvider{}, &fakeHooks{})
	q.state.items = tracks("a", "b")
	q.state.current = 1
	transport.pos = previousRestart.Seconds()
	if err := q.Prev(context.Background()); err != nil {
		t.Fatal(err)
	}
	if transport.seek != 0 || q.state.current != 1 {
		t.Fatalf("seek=%v current=%d", transport.seek, q.state.current)
	}
}

func TestPreventDuplicatesMovesManualItem(t *testing.T) {
	q, _ := testQueue(t, &fakeProvider{}, &fakeHooks{prevent: true})
	q.state.items = tracks("current", "dup", "context")
	q.state.items[1].QueuedEnd = true
	if err := q.PlayNext(tracks("dup"), nil); err != nil {
		t.Fatal(err)
	}
	if got := ids(q.state.items); !reflect.DeepEqual(got, []string{"current", "dup", "context"}) {
		t.Fatalf("queue = %v", got)
	}
	if !q.state.items[1].Queued || q.state.items[1].QueuedEnd {
		t.Fatal("duplicate was not moved into play-next block")
	}
}

func TestStopAfterCurrentIsOneShot(t *testing.T) {
	q, transport := testQueue(t, &fakeProvider{}, &fakeHooks{})
	q.state.items = tracks("a", "b")
	q.SetStopAfterCurrent(true)
	q.handlePlayerEvent(player.Event{Kind: player.EndOfFile})
	if q.stopAfterCurrent || !transport.stopped || q.state.current != 0 {
		t.Fatalf("armed=%v stopped=%v current=%d", q.stopAfterCurrent, transport.stopped, q.state.current)
	}
}

func TestAutoplayOnlyNearTail(t *testing.T) {
	p := &fakeProvider{next: tracks("e")}
	q, _ := testQueue(t, p, &fakeHooks{autoplay: true})
	q.state.items = tracks("a", "b", "c", "d")
	q.state.current = 0
	if got := q.extendAutoplay(context.Background()); got != 0 || p.nextCalls.Load() != 0 {
		t.Fatalf("far-tail extension = %d calls=%d", got, p.nextCalls.Load())
	}
	q.state.current = 2
	if got := q.extendAutoplay(context.Background()); got != 1 || p.nextCalls.Load() != 1 {
		t.Fatalf("near-tail extension = %d calls=%d", got, p.nextCalls.Load())
	}
}

func TestCloseCancelsAndJoinsBackgroundWork(t *testing.T) {
	p := &fakeProvider{blockNext: true, nextStarted: make(chan struct{})}
	q, _ := testQueue(t, p, &fakeHooks{autoplay: true})
	q.state.items = tracks("a")
	q.start(func(ctx context.Context) { q.extendAutoplay(ctx) })
	select {
	case <-p.nextStarted:
	case <-time.After(time.Second):
		t.Fatal("autoplay did not start")
	}
	done := make(chan struct{})
	go func() {
		q.Close()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Close did not join cancelled autoplay")
	}
}

type mapPersist map[string]string

func (m mapPersist) Get(key string) (string, bool) { value, ok := m[key]; return value, ok }
func (m mapPersist) Set(key, value string) error   { m[key] = value; return nil }

func TestPlayWithoutAnIDKeepsTheQueue(t *testing.T) {
	q, _ := testQueue(t, &fakeProvider{}, &fakeHooks{})
	q.state.items = tracks("a", "b")
	q.state.current = 1
	if err := q.Play(context.Background(), model.Track{}); !errors.Is(err, errNothingToPlay) {
		t.Fatalf("Play(empty) error = %v, want %v", err, errNothingToPlay)
	}
	start := 1
	if err := q.PlayPlaylist(context.Background(), []model.Track{{VideoID: "x"}, {}}, &start, nil, nil, false, nil); !errors.Is(err, errNothingToPlay) {
		t.Fatalf("PlayPlaylist(start on empty) error = %v, want %v", err, errNothingToPlay)
	}
	if got := ids(q.state.items); !reflect.DeepEqual(got, []string{"a", "b"}) || q.state.current != 1 {
		t.Fatalf("queue = %v at %d, want [a b] at 1 untouched", got, q.state.current)
	}
}
func TestPlayUsesDownloadedFileWithoutProviderResolution(t *testing.T) {
	provider := &fakeProvider{}
	loudness := -8.5
	hooks := &fakeHooks{
		downloads: map[string]model.Stream{"downloaded": {URL: "file:///music/downloaded.opus"}},
		cache:     map[string]model.Stream{"downloaded": {LoudnessDB: &loudness}},
	}
	q, transport := testQueue(t, provider, hooks)
	if err := q.Play(context.Background(), model.Track{VideoID: "downloaded", Title: "Saved"}); err != nil {
		t.Fatal(err)
	}
	if len(transport.loaded) != 1 || transport.loaded[0].URL != "file:///music/downloaded.opus" {
		t.Fatalf("loaded streams = %#v", transport.loaded)
	}
	if transport.loudness == nil || *transport.loudness != loudness {
		t.Fatalf("downloaded loudness = %v, want %v", transport.loudness, loudness)
	}
	if provider.resolveCalls.Load() != 0 {
		t.Fatalf("provider resolved %d times", provider.resolveCalls.Load())
	}
}

func TestProviderOwnedFileAndRemoteIDsSkipDownloadLookup(t *testing.T) {
	tests := []struct {
		name string
		id   string
		kind model.Provider
	}{
		{name: "local", id: "LOCAL:/music/track.flac", kind: model.ProviderLocal},
		{name: "radio", id: "RYOTUNES_RADIO:station", kind: model.ProviderRadio},
		{name: "Spotify", id: "spotify:track:123", kind: model.ProviderSpotify},
		{name: "TIDAL", id: "tidal:track:123", kind: model.ProviderTidal},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			hooks := &fakeHooks{downloads: map[string]model.Stream{
				test.id: {URL: "file:///wrong.opus"},
			}}
			q, transport := testQueue(t, &fakeProvider{id: test.kind}, hooks)
			if err := q.Play(context.Background(), model.Track{VideoID: test.id}); err != nil {
				t.Fatal(err)
			}
			if hooks.downloadCalls.Load() != 0 {
				t.Fatalf("download lookup calls = %d", hooks.downloadCalls.Load())
			}
			if got := transport.loaded[0].URL; got != "https://stream/"+test.id {
				t.Fatalf("loaded URL = %q", got)
			}
		})
	}
}

func TestMissingOnlyLocalTrackClearsNowPlaying(t *testing.T) {
	q, transport := testQueue(t, &fakeProvider{id: model.ProviderLocal, resolveErr: provider.ErrNotFound}, &fakeHooks{})
	var nowPlayingJSON []byte
	var paused bool
	q.bus = bus.SinkFunc(func(event string, data any) {
		switch event {
		case protocol.EventNowPlaying:
			nowPlayingJSON, _ = json.Marshal(data)
		case protocol.EventPlaybackState:
			paused = data == protocol.PlaybackStatePaused
		}
	})
	err := q.Play(context.Background(), model.Track{VideoID: "LOCAL:/missing.flac", Title: "Missing"})
	if !errors.Is(err, provider.ErrNotFound) {
		t.Fatalf("Play(missing local) error = %v", err)
	}
	if string(nowPlayingJSON) != "null" {
		t.Fatalf("now-playing event = %s, want null", nowPlayingJSON)
	}
	if !transport.stopped || !paused {
		t.Fatalf("transport stopped = %v, paused event = %v", transport.stopped, paused)
	}
	if snapshot := q.Snapshot(); len(snapshot.Items) != 0 {
		t.Fatalf("queue snapshot = %#v", snapshot)
	}
	if playback := q.Playback(); playback.Now != nil {
		t.Fatalf("playback snapshot = %#v", playback)
	}
}

func TestMissingLocalRowKeepsCurrentPlayback(t *testing.T) {
	q, transport := testQueue(t, &fakeProvider{id: model.ProviderLocal, resolveErr: provider.ErrNotFound}, &fakeHooks{})
	q.state.items = []model.Track{
		{VideoID: "LOCAL:/playing.flac", Title: "Playing"},
		{VideoID: "LOCAL:/missing.flac", Title: "Missing"},
	}
	q.state.current = 0
	q.playing = true
	transport.idle = false
	transport.paused = false
	var cleared bool
	q.bus = bus.SinkFunc(func(event string, data any) {
		if event == protocol.EventNowPlaying {
			raw, _ := json.Marshal(data)
			cleared = string(raw) == "null"
		}
	})
	err := q.PlayIndex(context.Background(), 1)
	if !errors.Is(err, provider.ErrNotFound) {
		t.Fatalf("PlayIndex(missing local) error = %v", err)
	}
	if transport.stopped || cleared {
		t.Fatalf("transport stopped = %v, now-playing cleared = %v", transport.stopped, cleared)
	}
	snapshot := q.Snapshot()
	if got := ids(snapshot.Items); !reflect.DeepEqual(got, []string{"LOCAL:/playing.flac"}) || snapshot.CurrentIndex != 0 {
		t.Fatalf("queue = %v at %d", got, snapshot.CurrentIndex)
	}
	if playback := q.Playback(); playback.Now == nil || playback.Now.VideoID != "LOCAL:/playing.flac" || playback.Paused {
		t.Fatalf("playback snapshot = %#v", playback)
	}
}

// fileLoaded is mpv opening the stream the queue just loaded.
func fileLoaded(q *Queue) { q.handlePlayerEvent(player.Event{Kind: player.FileLoaded}) }

func TestPlaybackStartReportsFreshCachedDownloadedAndReplayButNotPrefetch(t *testing.T) {
	starts := make(chan playbackStartCall, 8)
	hooks := &fakeHooks{
		cache: map[string]model.Stream{
			"cached": {URL: "https://cache/cached"},
		},
		downloads: map[string]model.Stream{
			"downloaded": {URL: "file:///music/downloaded.opus"},
		},
		playbackStarts: starts,
	}
	q, _ := testQueue(t, &fakeProvider{}, hooks)
	sourceID := "PLmix"
	if err := q.PlayPlaylist(context.Background(), tracks("fresh"), nil, &sourceID, nil, false, nil); err != nil {
		t.Fatal(err)
	}
	fileLoaded(q)
	if got := waitPlaybackStart(t, starts); got.track.VideoID != "fresh" || got.playlistID != sourceID {
		t.Fatalf("fresh playback start = %#v", got)
	}

	q.PrefetchStream(context.Background(), model.Track{VideoID: "prefetch"})
	noPlaybackStart(t, starts)

	if err := q.Play(context.Background(), model.Track{VideoID: "cached"}); err != nil {
		t.Fatal(err)
	}
	fileLoaded(q)
	if got := waitPlaybackStart(t, starts); got.track.VideoID != "cached" {
		t.Fatalf("cached playback start = %#v", got)
	}
	if err := q.Play(context.Background(), model.Track{VideoID: "downloaded"}); err != nil {
		t.Fatal(err)
	}
	fileLoaded(q)
	if got := waitPlaybackStart(t, starts); got.track.VideoID != "downloaded" {
		t.Fatalf("downloaded playback start = %#v", got)
	}
	if err := q.PlayIndex(context.Background(), 0); err != nil {
		t.Fatal(err)
	}
	fileLoaded(q)
	if got := waitPlaybackStart(t, starts); got.track.VideoID != "downloaded" {
		t.Fatalf("replayed playback start = %#v", got)
	}
	fileLoaded(q)
	noPlaybackStart(t, starts)
}

func TestPlaybackStartSkipsTracksThatNeverLoaded(t *testing.T) {
	starts := make(chan playbackStartCall, 8)
	q, _ := testQueue(t, &fakeProvider{}, &fakeHooks{playbackStarts: starts})
	if err := q.PlayPlaylist(context.Background(), tracks("skipped", "kept"), nil, nil, nil, false, nil); err != nil {
		t.Fatal(err)
	}
	if err := q.Next(context.Background()); err != nil {
		t.Fatal(err)
	}
	fileLoaded(q)
	if got := waitPlaybackStart(t, starts); got.track.VideoID != "kept" {
		t.Fatalf("playback start = %#v, want only the track that loaded", got)
	}
	noPlaybackStart(t, starts)
}

func TestPlaybackStartWaitsUntilAPausedTrackPlays(t *testing.T) {
	starts := make(chan playbackStartCall, 8)
	q, _ := testQueue(t, &fakeProvider{}, &fakeHooks{playbackStarts: starts})
	if err := q.Play(context.Background(), model.Track{VideoID: "paused"}); err != nil {
		t.Fatal(err)
	}
	if err := q.Pause(); err != nil {
		t.Fatal(err)
	}
	fileLoaded(q)
	noPlaybackStart(t, starts)

	if err := q.PlayTransport(context.Background()); err != nil {
		t.Fatal(err)
	}
	q.handlePlayerEvent(player.Event{Kind: player.PauseChanged})
	if got := waitPlaybackStart(t, starts); got.track.VideoID != "paused" {
		t.Fatalf("playback start = %#v", got)
	}
	q.handlePlayerEvent(player.Event{Kind: player.PauseChanged})
	noPlaybackStart(t, starts)
}

func TestQueuedRowsAreReportedOutsideThePlaylist(t *testing.T) {
	starts := make(chan playbackStartCall, 8)
	q, _ := testQueue(t, &fakeProvider{}, &fakeHooks{playbackStarts: starts})
	sourceID := "PLmix"
	if err := q.PlayPlaylist(context.Background(), tracks("first", "second"), nil, &sourceID, nil, false, nil); err != nil {
		t.Fatal(err)
	}
	fileLoaded(q)
	waitPlaybackStart(t, starts)
	if err := q.PlayNext(tracks("queued"), nil); err != nil {
		t.Fatal(err)
	}
	if err := q.Next(context.Background()); err != nil {
		t.Fatal(err)
	}
	fileLoaded(q)
	if got := waitPlaybackStart(t, starts); got.track.VideoID != "queued" || got.playlistID != "" {
		t.Fatalf("queued playback start = %#v, want no playlist context", got)
	}
}

func TestFollowSeeksOnlyWhenTheGuestDrifted(t *testing.T) {
	starts := make(chan playbackStartCall, 8)
	q, transport := testQueue(t, &fakeProvider{}, &fakeHooks{playbackStarts: starts})
	ctx := context.Background()
	track := model.Track{VideoID: "shared"}
	if err := q.Follow(ctx, track, 100, true); err != nil {
		t.Fatal(err)
	}
	fileLoaded(q)
	waitPlaybackStart(t, starts)
	for _, host := range []float64{100.25, 100.5, 101.5} {
		transport.seek = -1
		if err := q.Follow(ctx, track, host, true); err != nil {
			t.Fatal(err)
		}
		if transport.seek != -1 {
			t.Fatalf("a host snapshot %.2f s ahead made the guest seek to %v", host-100, transport.seek)
		}
	}
	if err := q.Follow(ctx, track, 0, true); err != nil {
		t.Fatal(err)
	}
	if transport.seek != 0 {
		t.Fatalf("the host restarted the track and the guest seeked to %v", transport.seek)
	}
	if len(transport.loaded) != 1 {
		t.Fatalf("the shared track loaded %d times", len(transport.loaded))
	}
	noPlaybackStart(t, starts)
}

func TestFollowingAPausedRoomReportsOnlyOnceItPlays(t *testing.T) {
	starts := make(chan playbackStartCall, 8)
	q, transport := testQueue(t, &fakeProvider{}, &fakeHooks{playbackStarts: starts})
	transport.onLoad = func() { fileLoaded(q) }
	ctx := context.Background()
	track := model.Track{VideoID: "shared"}
	if err := q.Follow(ctx, track, 30, false); err != nil {
		t.Fatal(err)
	}
	noPlaybackStart(t, starts)

	transport.onLoad = nil
	if err := q.Follow(ctx, track, 30.5, true); err != nil {
		t.Fatal(err)
	}
	q.handlePlayerEvent(player.Event{Kind: player.PauseChanged})
	if got := waitPlaybackStart(t, starts); got.track.VideoID != "shared" {
		t.Fatalf("playback start = %#v", got)
	}
	if len(transport.loaded) != 1 {
		t.Fatalf("the shared track loaded %d times", len(transport.loaded))
	}
}

func TestSlowPlaybackReportNeverHoldsPlayerEvents(t *testing.T) {
	starts := make(chan playbackStartCall, 1)
	release := make(chan struct{})
	defer close(release)
	hooks := &fakeHooks{
		downloads:      map[string]model.Stream{"offline": {URL: "file:///music/offline.opus"}},
		playbackStarts: starts,
		holdPlayback:   release,
	}
	q, _ := testQueue(t, &fakeProvider{}, hooks)
	if err := q.Play(context.Background(), model.Track{VideoID: "offline"}); err != nil {
		t.Fatal(err)
	}
	handled := make(chan struct{})
	go func() {
		fileLoaded(q)
		q.handlePlayerEvent(player.Event{Kind: player.Position, Position: 1})
		close(handled)
	}()
	waitPlaybackStart(t, starts)
	select {
	case <-handled:
	case <-time.After(time.Second):
		t.Fatal("a held playback report stalled player events")
	}
}

func waitPlaybackStart(t *testing.T, starts <-chan playbackStartCall) playbackStartCall {
	t.Helper()
	select {
	case call := <-starts:
		return call
	case <-time.After(time.Second):
		t.Fatal("playback start was not reported")
		return playbackStartCall{}
	}
}

func noPlaybackStart(t *testing.T, starts <-chan playbackStartCall) {
	t.Helper()
	select {
	case got := <-starts:
		t.Fatalf("unexpected playback start: %#v", got)
	case <-time.After(20 * time.Millisecond):
	}
}

func TestRestoreDropsTracksWithoutAnID(t *testing.T) {
	tests := []struct {
		name         string
		items        []model.Track
		current      int
		wantIDs      []string
		wantCurrent  int
		wantSeekFrom string
	}{
		{name: "only an empty track", items: []model.Track{{}}, wantIDs: []string{}},
		{name: "current kept and moved", items: []model.Track{{}, {VideoID: "a"}, {VideoID: "b"}}, current: 2, wantIDs: []string{"a", "b"}, wantCurrent: 1, wantSeekFrom: "b"},
		{name: "current dropped", items: []model.Track{{VideoID: "a"}, {}, {VideoID: "b"}}, current: 1, wantIDs: []string{"a", "b"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			raw, err := json.Marshal(savedQueue{Items: test.items, Current: test.current, PlayedFrom: test.current, Repeat: model.RepeatOff})
			if err != nil {
				t.Fatal(err)
			}
			q, _ := testQueue(t, &fakeProvider{}, &fakeHooks{})
			q.persist = mapPersist{queueJSONKey: string(raw), queuePositionKey: "42"}
			q.restore()
			if got := ids(q.state.items); !reflect.DeepEqual(got, test.wantIDs) || q.state.current != test.wantCurrent {
				t.Fatalf("restored %v at %d, want %v at %d", got, q.state.current, test.wantIDs, test.wantCurrent)
			}
			if q.pendingSeekID != test.wantSeekFrom {
				t.Fatalf("pending seek on %q, want %q", q.pendingSeekID, test.wantSeekFrom)
			}
		})
	}
}
