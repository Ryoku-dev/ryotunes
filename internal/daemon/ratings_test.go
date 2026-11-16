package daemon

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/ryoku-dev/ryotunes/internal/bus"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/player"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
	"github.com/ryoku-dev/ryotunes/internal/provider"
	"github.com/ryoku-dev/ryotunes/internal/provider/local"
	"github.com/ryoku-dev/ryotunes/internal/queue"
	"github.com/ryoku-dev/ryotunes/internal/store"
)

// fakeYouTube is a YouTube Music account whose answers carry no likeStatus,
// like YouTube's queue, radio and search answers, except Liked music's rows.
type fakeYouTube struct {
	mu       sync.Mutex
	signedIn bool
	liked    [][]string // Liked music, page by page
	pages    map[string][]model.Track
	rates    []protocol.RateParams
}

func (f *fakeYouTube) ID() model.Provider                                   { return model.ProviderYouTube }
func (f *fakeYouTube) Capabilities() provider.Capabilities                  { return provider.Capabilities{} }
func (f *fakeYouTube) Home(context.Context, string) (model.Feed, error)     { return model.Feed{}, nil }
func (f *fakeYouTube) HomeMore(context.Context, string) (model.Feed, error) { return model.Feed{}, nil }
func (f *fakeYouTube) Search(context.Context, string, model.SearchKind) (model.SearchPage, error) {
	return model.SearchPage{}, nil
}
func (f *fakeYouTube) SearchMore(context.Context, string) (model.SearchPage, error) {
	return model.SearchPage{}, nil
}
func (f *fakeYouTube) SearchAll(context.Context, string) (model.SearchAll, error) {
	return model.SearchAll{Songs: []model.BrowseItem{
		{Kind: "song", ID: "liked1", Title: "liked1"}, {Kind: "song", ID: "other", Title: "other"},
		{Kind: "album", ID: "MPREalbum", Title: "an album"},
	}}, nil
}
func (f *fakeYouTube) SearchAllMore(context.Context, string) (model.SearchAll, error) {
	return model.SearchAll{}, nil
}
func (f *fakeYouTube) SearchCards(context.Context, string, string) (model.CardsPage, error) {
	return model.CardsPage{}, nil
}
func (f *fakeYouTube) SearchCardsMore(context.Context, string) (model.CardsPage, error) {
	return model.CardsPage{}, nil
}
func (f *fakeYouTube) Suggest(context.Context, string) ([]string, error) { return nil, nil }
func (f *fakeYouTube) Album(_ context.Context, id string) (model.AlbumPage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return model.AlbumPage{Items: f.pages[id]}, nil
}
func (f *fakeYouTube) Artist(context.Context, string) (model.ArtistPage, error) {
	return model.ArtistPage{}, nil
}

func (f *fakeYouTube) Playlist(_ context.Context, id string, _ model.PlaylistSort, _ bool) (model.PlaylistPage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if id == likedMusic {
		return f.likedPageLocked(0), nil
	}
	return model.PlaylistPage{Items: f.pages[id]}, nil
}

func (f *fakeYouTube) PlaylistMore(_ context.Context, token string) (model.PlaylistTracksPage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var index int
	if err := json.Unmarshal([]byte(token), &index); err != nil {
		return model.PlaylistTracksPage{}, err
	}
	page := f.likedPageLocked(index)
	return model.PlaylistTracksPage{Items: page.Items, Continuation: page.Continuation}, nil
}

func (f *fakeYouTube) likedPageLocked(index int) model.PlaylistPage {
	var page model.PlaylistPage
	if index >= len(f.liked) {
		return page
	}
	for _, id := range f.liked[index] {
		rating := model.RatingLike
		page.Items = append(page.Items, model.Track{VideoID: id, Title: id, Rating: &rating})
	}
	if index+1 < len(f.liked) {
		token, _ := json.Marshal(index + 1)
		next := string(token)
		page.Continuation = &next
	}
	return page
}

func (f *fakeYouTube) Library(context.Context) (model.Library, error) { return model.Library{}, nil }
func (f *fakeYouTube) Resolve(_ context.Context, track model.Track, _ model.Quality) (model.Stream, error) {
	return model.Stream{URL: "https://media.test/" + track.VideoID}, nil
}
func (f *fakeYouTube) Next(context.Context, model.Track, []string) ([]model.Track, error) {
	return nil, nil
}

func (f *fakeYouTube) Account(context.Context) (model.Account, error) {
	return f.CurrentAccount(), nil
}

func (f *fakeYouTube) CurrentAccount() model.Account {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.signedIn {
		return model.Account{}
	}
	channel := "UCtester"
	return model.Account{SignedIn: true, ChannelID: &channel}
}

func (f *fakeYouTube) Rate(_ context.Context, id string, rating model.Rating) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rates = append(f.rates, protocol.RateParams{VideoID: id, Rating: rating})
	return nil
}

// ratingsTransport is a player that opens whatever it is given.
type ratingsTransport struct {
	mu     sync.Mutex
	events chan player.Event
	paused bool
}

func (t *ratingsTransport) Load(_ context.Context, _ model.Stream, _ float64, paused bool) error {
	t.mu.Lock()
	t.paused = paused
	t.mu.Unlock()
	return nil
}
func (t *ratingsTransport) Prefetch(model.Stream) {}
func (t *ratingsTransport) Play() error           { return nil }
func (t *ratingsTransport) Pause() error          { return nil }
func (t *ratingsTransport) TogglePause() error    { return nil }
func (t *ratingsTransport) Stop() error           { return nil }
func (t *ratingsTransport) Seek(float64) error    { return nil }
func (t *ratingsTransport) SetVolume(int) error   { return nil }
func (t *ratingsTransport) SetLoudness(*float64)  {}
func (t *ratingsTransport) Position() float64     { return 0 }
func (t *ratingsTransport) Duration() float64     { return 0 }
func (t *ratingsTransport) Paused() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.paused
}
func (t *ratingsTransport) Idle() bool                  { return false }
func (t *ratingsTransport) Events() <-chan player.Event { return t.events }

type recordedEvent struct {
	name string
	data any
}

type eventLog struct {
	mu     sync.Mutex
	events []recordedEvent
}

func (l *eventLog) Emit(name string, data any) {
	l.mu.Lock()
	l.events = append(l.events, recordedEvent{name, data})
	l.mu.Unlock()
}

func (l *eventLog) take() []recordedEvent {
	l.mu.Lock()
	defer l.mu.Unlock()
	events := l.events
	l.events = nil
	return events
}

func (l *eventLog) ratings() []protocol.RatingEvent {
	var out []protocol.RatingEvent
	for _, event := range l.take() {
		if event.name == protocol.EventRating {
			out = append(out, event.data.(protocol.RatingEvent))
		}
	}
	return out
}

func ratingsApp(t *testing.T, youtube *fakeYouTube, extra ...provider.Provider) (*App, *eventLog) {
	t.Helper()
	log := &eventLog{}
	app := &App{
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Bus: &bus.Fanout{}, ctx: context.Background(),
		Providers: provider.NewRegistry(append([]provider.Provider{youtube}, extra...)...),
		ratings:   newRatingBook(), likesWake: make(chan struct{}, 1),
		provider: string(model.ProviderYouTube),
	}
	app.Bus.Attach(log)
	q, err := queue.New(queue.Options{
		Player: &ratingsTransport{events: make(chan player.Event)}, Registry: app.Providers,
		Ratings: app.ratings, Bus: app.Bus, Log: app.Log,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(q.Close)
	app.Queue = q
	return app, log
}

func songs(ids ...string) []model.Track {
	out := make([]model.Track, len(ids))
	for i, id := range ids {
		out[i] = model.Track{VideoID: id, Title: id, Artists: "artist"}
	}
	return out
}

func call[R any](t *testing.T, app *App, method string, params any) R {
	t.Helper()
	raw, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}
	result, rpcErr := app.Dispatch(context.Background(), method, raw)
	if rpcErr != nil {
		t.Fatalf("%s: %v", method, rpcErr.Message)
	}
	// through JSON, as a client reads it
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var out R
	if err := json.Unmarshal(encoded, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func ratingOf(rating *model.Rating) string {
	if rating == nil {
		return "unknown"
	}
	return string(*rating)
}

func queueRatings(app *App) map[string]string {
	out := make(map[string]string)
	for _, item := range app.Queue.Snapshot().Items {
		out[item.VideoID] = ratingOf(item.Rating)
	}
	return out
}

func nowRating(app *App) string {
	now := app.Queue.Playback().Now
	if now == nil {
		return "nothing playing"
	}
	return ratingOf(now.Rating)
}

func waitFor(t *testing.T, what string, done func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !done() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// YouTube's queue never says what is liked: until the account's likes are read
// nothing is claimed, and once they are, the queue, now playing and every page
// show them, with a rating event for the playing song.
func TestLikedSongsStampQueueNowPlayingAndPages(t *testing.T) {
	youtube := &fakeYouTube{
		signedIn: true,
		liked:    [][]string{{"liked1"}, {"liked2"}},
		pages:    map[string][]model.Track{"MPREmix": songs("liked2", "other")},
	}
	app, events := ratingsApp(t, youtube)
	if err := app.Queue.PlayPlaylist(context.Background(), songs("liked1", "other", "liked2"), nil, nil, nil, false, nil); err != nil {
		t.Fatal(err)
	}
	if got := queueRatings(app); got["liked1"] != "unknown" || got["other"] != "unknown" {
		t.Fatalf("before the likes are read the queue claims %v", got)
	}
	if got := nowRating(app); got != "unknown" {
		t.Fatalf("before the likes are read now playing claims %s", got)
	}
	events.take()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		app.followLikes(ctx)
	}()
	waitFor(t, "the likes to be read", func() bool { return queueRatings(app)["liked1"] != "unknown" })
	cancel()
	<-done

	want := map[string]string{"liked1": "like", "other": "indifferent", "liked2": "like"}
	if got := queueRatings(app); !equalRatings(got, want) {
		t.Fatalf("queue ratings = %v, want %v", got, want)
	}
	if got := nowRating(app); got != "like" {
		t.Fatalf("now playing rating = %s, want like", got)
	}
	var announced *model.QueueSnapshot
	var rated []protocol.RatingEvent
	for _, event := range events.take() {
		switch event.name {
		case protocol.EventQueueChanged:
			snapshot := event.data.(model.QueueSnapshot)
			announced = &snapshot
		case protocol.EventRating:
			rated = append(rated, event.data.(protocol.RatingEvent))
		}
	}
	if announced == nil {
		t.Fatal("the queue was not announced again once the likes were read")
	}
	for _, item := range announced.Items {
		if ratingOf(item.Rating) != want[item.VideoID] {
			t.Errorf("announced queue: %s is %s, want %s", item.VideoID, ratingOf(item.Rating), want[item.VideoID])
		}
	}
	if len(rated) != 1 || rated[0] != (protocol.RatingEvent{VideoID: "liked1", Rating: model.RatingLike}) {
		t.Fatalf("rating events = %+v, want liked1 like", rated)
	}

	got := call[model.QueueSnapshot](t, app, protocol.MethodGetQueue, nil)
	if got.Items[0].Rating == nil || *got.Items[0].Rating != model.RatingLike {
		t.Fatalf("get_queue: liked1 = %s", ratingOf(got.Items[0].Rating))
	}
	playback := call[model.PlaybackSnapshot](t, app, protocol.MethodGetPlayback, nil)
	if playback.Now == nil || ratingOf(playback.Now.Rating) != "like" {
		t.Fatalf("get_playback now = %+v", playback.Now)
	}
	page := call[model.AlbumPage](t, app, protocol.MethodGetAlbum, protocol.IDParams{ID: "MPREmix"})
	if ratingOf(page.Items[0].Rating) != "like" || ratingOf(page.Items[1].Rating) != "indifferent" {
		t.Fatalf("playlist rows = %s, %s", ratingOf(page.Items[0].Rating), ratingOf(page.Items[1].Rating))
	}
	if youtube.pages["MPREmix"][0].Rating != nil {
		t.Fatal("stamping wrote into the page the provider returned")
	}
	all := call[model.SearchAll](t, app, protocol.MethodSearchAll, protocol.QueryParams{Query: "x"})
	if ratingOf(all.Songs[0].Rating) != "like" || ratingOf(all.Songs[1].Rating) != "indifferent" || all.Songs[2].Rating != nil {
		t.Fatalf("search cards = %s, %s, album %s", ratingOf(all.Songs[0].Rating), ratingOf(all.Songs[1].Rating), ratingOf(all.Songs[2].Rating))
	}
}

func equalRatings(got, want map[string]string) bool {
	if len(got) != len(want) {
		return false
	}
	for id, rating := range want {
		if got[id] != rating {
			return false
		}
	}
	return true
}

// A like confirmed by the account reaches the queue rows, now playing and every
// client at once; clearing it does the same.
func TestRateCarriesOntoQueueNowPlayingAndAnnounces(t *testing.T) {
	youtube := &fakeYouTube{signedIn: true, liked: [][]string{{"liked1"}}}
	app, events := ratingsApp(t, youtube)
	account := app.followYouTubeAccount()
	if err := app.loadYouTubeLikes(context.Background(), account); err != nil {
		t.Fatal(err)
	}
	queued := songs("song", "liked1", "song2")
	// played from Liked music, whose rows say they are liked
	like := model.RatingLike
	queued[1].Rating = &like
	if err := app.Queue.PlayPlaylist(context.Background(), queued, nil, nil, nil, false, nil); err != nil {
		t.Fatal(err)
	}
	if got := nowRating(app); got != "indifferent" {
		t.Fatalf("now playing before the like = %s", got)
	}
	events.take()

	call[protocol.Null](t, app, protocol.MethodRate, protocol.RateParams{VideoID: "song", Rating: model.RatingLike})
	if len(youtube.rates) != 1 || youtube.rates[0] != (protocol.RateParams{VideoID: "song", Rating: model.RatingLike}) {
		t.Fatalf("YouTube got %+v", youtube.rates)
	}
	if got := queueRatings(app)["song"]; got != "like" {
		t.Fatalf("queue row after the like = %s", got)
	}
	if got := nowRating(app); got != "like" {
		t.Fatalf("now playing after the like = %s", got)
	}
	if got := events.ratings(); len(got) != 1 || got[0] != (protocol.RatingEvent{VideoID: "song", Rating: model.RatingLike}) {
		t.Fatalf("rating events after the like = %+v", got)
	}
	youtube.mu.Lock()
	youtube.pages = map[string][]model.Track{"MPREother": songs("song")}
	youtube.mu.Unlock()
	if page := call[model.AlbumPage](t, app, protocol.MethodGetAlbum, protocol.IDParams{ID: "MPREother"}); ratingOf(page.Items[0].Rating) != "like" {
		t.Fatalf("a page fetched after the like shows %s", ratingOf(page.Items[0].Rating))
	}

	call[protocol.Null](t, app, protocol.MethodRate, protocol.RateParams{VideoID: "song", Rating: model.RatingIndifferent})
	if got := queueRatings(app)["song"]; got != "indifferent" {
		t.Fatalf("queue row after clearing = %s", got)
	}
	if got := nowRating(app); got != "indifferent" {
		t.Fatalf("now playing after clearing = %s", got)
	}
	if got := events.ratings(); len(got) != 1 || got[0] != (protocol.RatingEvent{VideoID: "song", Rating: model.RatingIndifferent}) {
		t.Fatalf("rating events after clearing = %+v", got)
	}

	// a queued song that isn't playing, whose row came with YouTube's own rating
	call[protocol.Null](t, app, protocol.MethodRate, protocol.RateParams{VideoID: "liked1", Rating: model.RatingIndifferent})
	if got := queueRatings(app)["liked1"]; got != "indifferent" {
		t.Fatalf("queued row after unliking = %s", got)
	}
	if got := events.ratings(); len(got) != 1 || got[0].VideoID != "liked1" {
		t.Fatalf("rating events after unliking a queued song = %+v", got)
	}
}

// A like made while the likes are being read survives the read.
func TestLikeDuringALoadSurvivesIt(t *testing.T) {
	book := newRatingBook()
	book.setYouTubeAccount("UCtester")
	epoch := book.beginLoad("youtube", "UCtester")
	book.rate("youtube", "UCtester", "fresh", model.RatingLike)
	book.rate("youtube", "UCtester", "gone", model.RatingIndifferent)
	book.finishLoad("youtube", "UCtester", epoch, []string{"gone", "old"})
	for id, want := range map[string]string{"fresh": "like", "gone": "indifferent", "old": "like", "never": "indifferent"} {
		if got := ratingOf(book.Rating(id)); got != want {
			t.Errorf("%s = %s, want %s", id, got, want)
		}
	}
	// the next read is the truth again
	book.finishLoad("youtube", "UCtester", book.beginLoad("youtube", "UCtester"), []string{"gone"})
	if got := ratingOf(book.Rating("fresh")); got != "indifferent" {
		t.Fatalf("fresh after a later read = %s", got)
	}
	// another account's likes are not this one's
	book.setYouTubeAccount("UCsomeoneelse")
	if got := ratingOf(book.Rating("gone")); got != "unknown" {
		t.Fatalf("another account's like = %s", got)
	}
}

// Signed out of YouTube Music, a like goes into the device's Liked songs and
// is carried and announced the same way.
func TestRateOnTheDeviceCarriesOntoQueueAndAnnounces(t *testing.T) {
	root := t.TempDir()
	st, err := store.Open(store.Dirs{
		Data: filepath.Join(root, "data"), Cache: filepath.Join(root, "cache"),
		Config: filepath.Join(root, "config"), State: filepath.Join(root, "state"), Runtime: filepath.Join(root, "run"),
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	device, err := local.New(local.Options{Index: st, CacheDir: filepath.Join(root, "local")})
	if err != nil {
		t.Fatal(err)
	}
	youtube := &fakeYouTube{}
	app, events := ratingsApp(t, youtube, device)
	app.Local = device
	app.followYouTubeAccount()
	app.loadDeviceLikes(context.Background())
	if err := app.Queue.PlayPlaylist(context.Background(), songs("song", "next"), nil, nil, nil, false, nil); err != nil {
		t.Fatal(err)
	}
	if got := nowRating(app); got != "indifferent" {
		t.Fatalf("now playing before the like = %s", got)
	}
	events.take()

	call[protocol.Null](t, app, protocol.MethodRate, protocol.RateParams{VideoID: "song", Rating: model.RatingLike})
	if len(youtube.rates) != 0 {
		t.Fatalf("a signed-out like reached YouTube: %+v", youtube.rates)
	}
	page, err := device.Playlist(context.Background(), model.LocalLikedPlaylistID, model.PlaylistSortDefault, false)
	if err != nil || len(page.Items) != 1 || page.Items[0].VideoID != "song" {
		t.Fatalf("device Liked songs = %+v, %v", page.Items, err)
	}
	if got := nowRating(app); got != "like" {
		t.Fatalf("now playing after the like = %s", got)
	}
	if got := events.ratings(); len(got) != 1 || got[0] != (protocol.RatingEvent{VideoID: "song", Rating: model.RatingLike}) {
		t.Fatalf("rating events after the like = %+v", got)
	}

	call[protocol.Null](t, app, protocol.MethodRate, protocol.RateParams{VideoID: "song", Rating: model.RatingIndifferent})
	if got := nowRating(app); got != "indifferent" {
		t.Fatalf("now playing after clearing = %s", got)
	}
	if got := events.ratings(); len(got) != 1 || got[0].Rating != model.RatingIndifferent {
		t.Fatalf("rating events after clearing = %+v", got)
	}
}
