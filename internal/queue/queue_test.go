package queue

import (
	"context"
	"math/rand/v2"
	"reflect"
	"testing"
	"time"

	"github.com/ryoku-dev/ryotunes/internal/bus"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/player"
	"github.com/ryoku-dev/ryotunes/internal/provider"
)

type fakeTransport struct {
	events  chan player.Event
	loaded  []model.Stream
	seek    float64
	pos     float64
	paused  bool
	idle    bool
	stopped bool
}

func newFakeTransport() *fakeTransport {
	return &fakeTransport{events: make(chan player.Event, 8), idle: true, paused: true}
}
func (f *fakeTransport) Load(_ context.Context, stream model.Stream, _ float64, paused bool) error {
	f.loaded = append(f.loaded, stream)
	f.paused, f.idle = paused, false
	return nil
}
func (f *fakeTransport) Prefetch(model.Stream)       {}
func (f *fakeTransport) Play() error                 { f.paused = false; return nil }
func (f *fakeTransport) Pause() error                { f.paused = true; return nil }
func (f *fakeTransport) TogglePause() error          { f.paused = !f.paused; return nil }
func (f *fakeTransport) Stop() error                 { f.stopped, f.idle = true, true; return nil }
func (f *fakeTransport) Seek(position float64) error { f.seek, f.pos = position, position; return nil }
func (f *fakeTransport) SetVolume(int) error         { return nil }
func (f *fakeTransport) SetLoudness(*float64)        {}
func (f *fakeTransport) Position() float64           { return f.pos }
func (f *fakeTransport) Duration() float64           { return 0 }
func (f *fakeTransport) Paused() bool                { return f.paused }
func (f *fakeTransport) Idle() bool                  { return f.idle }
func (f *fakeTransport) Events() <-chan player.Event { return f.events }

type fakeHooks struct {
	autoplay bool
	prevent  bool
}

func (f *fakeHooks) OnPlayed(context.Context, model.Track)      {}
func (f *fakeHooks) StreamCacheGet(string) (model.Stream, bool) { return model.Stream{}, false }
func (f *fakeHooks) StreamCachePut(string, model.Stream)        {}
func (f *fakeHooks) Quality() model.Quality                     { return model.QualityAuto }
func (f *fakeHooks) Autoplay() bool                             { return f.autoplay }
func (f *fakeHooks) PreventDuplicates() bool                    { return f.prevent }
func (f *fakeHooks) HealIdentity() bool                         { return false }

type fakeProvider struct {
	next        []model.Track
	nextCalls   int
	nextStarted chan struct{}
	blockNext   bool
}

func (f *fakeProvider) ID() model.Provider                               { return model.ProviderYouTube }
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
	return model.Stream{URL: "https://stream/" + track.VideoID}, nil
}
func (f *fakeProvider) Next(ctx context.Context, _ model.Track, _ []string) ([]model.Track, error) {
	f.nextCalls++
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
		player: transport, registry: provider.NewRegistry(p), hooks: hooks, bus: bus.Discard,
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
	if got := q.extendAutoplay(context.Background()); got != 0 || p.nextCalls != 0 {
		t.Fatalf("far-tail extension = %d calls=%d", got, p.nextCalls)
	}
	q.state.current = 2
	if got := q.extendAutoplay(context.Background()); got != 1 || p.nextCalls != 1 {
		t.Fatalf("near-tail extension = %d calls=%d", got, p.nextCalls)
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
