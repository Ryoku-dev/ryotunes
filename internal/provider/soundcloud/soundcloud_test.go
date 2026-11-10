package soundcloud

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
	baseprovider "github.com/ryoku-dev/ryotunes/internal/provider"
	"github.com/ryoku-dev/ryotunes/internal/provider/browsercookies"
)

func TestParseNumericID(t *testing.T) {
	t.Parallel()
	id, err := parseNumericID("sc:track:184467", trackPrefix)
	if err != nil || id != 184467 {
		t.Fatalf("parse valid id = %d, %v", id, err)
	}
	for _, value := range []string{"184467", "sc:track:", "sc:track:not-a-number", "sc:user:184467"} {
		if _, err := parseNumericID(value, trackPrefix); err == nil {
			t.Errorf("parseNumericID(%q) succeeded", value)
		}
	}
}

func TestChooseTranscoding(t *testing.T) {
	t.Parallel()
	progressive := transcoding{URL: "progressive"}
	progressive.Format.Protocol = "progressive"
	progressive.Format.MimeType = "audio/mpeg"
	mp3 := transcoding{URL: "mp3"}
	mp3.Format.Protocol = "hls"
	mp3.Format.MimeType = "audio/mpeg"
	aac := transcoding{URL: "aac"}
	aac.Format.Protocol = "hls"
	aac.Format.MimeType = "audio/mp4; codecs=mp4a.40.2"
	other := transcoding{URL: "other"}
	other.Format.Protocol = "hls"
	other.Format.MimeType = "audio/ogg"
	if got := chooseTranscoding([]transcoding{progressive, mp3, aac}); got == nil || got.URL != "aac" {
		t.Fatalf("AAC choice = %#v", got)
	}
	if got := chooseTranscoding([]transcoding{progressive, mp3}); got == nil || got.URL != "mp3" {
		t.Fatalf("MP3 choice = %#v", got)
	}
	if got := chooseTranscoding([]transcoding{progressive, other}); got == nil || got.URL != "other" {
		t.Fatalf("fallback choice = %#v", got)
	}
	if got := chooseTranscoding([]transcoding{progressive}); got != nil {
		t.Fatalf("progressive selected: %#v", got)
	}
	err := unavailableError("track 12 has no playable HLS transcoding")
	if err.Error() != "track 12 has no playable HLS transcoding" {
		t.Fatalf("missing HLS sentence = %q", err.Error())
	}
}

func TestAuthExpiryMargin(t *testing.T) {
	t.Parallel()
	const now = int64(1_000)
	if authExpired(now+61, now) {
		t.Fatal("token with 61 seconds remaining is expired")
	}
	if !authExpired(now+60, now) {
		t.Fatal("token at 60 second margin is fresh")
	}
	if !authExpired(now-1, now) {
		t.Fatal("past token is fresh")
	}
}

func TestDownsampleWaveform(t *testing.T) {
	t.Parallel()
	got := downsample([]int{0, 5, 10, 20}, 20, 2)
	if len(got) != 2 || got[0] != 25 || got[1] != 100 {
		t.Fatalf("downsample = %v", got)
	}
	zeros := downsample(nil, 0, 240)
	if len(zeros) != 240 {
		t.Fatalf("empty waveform length = %d", len(zeros))
	}
}

type soundCloudCookieSource struct {
	mu         sync.Mutex
	stores     []browsercookies.Store
	cookies    map[string][]browsercookies.Cookie
	missing    map[string]bool
	unreadable map[string]bool
}

func (s *soundCloudCookieSource) Discover() []browsercookies.Store {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]browsercookies.Store(nil), s.stores...)
}

func (s *soundCloudCookieSource) Resolve(source browsercookies.SessionSource) (browsercookies.Store, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.missing[source.Path] {
		return browsercookies.Store{}, &fs.PathError{Op: "lstat", Path: source.Path, Err: fs.ErrNotExist}
	}
	return browsercookies.Store{Kind: source.Kind, Path: source.Path}, nil
}

func (s *soundCloudCookieSource) Read(store browsercookies.Store, _ ...string) ([]browsercookies.Cookie, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.unreadable[store.Path] {
		return nil, errors.New("file is not a database")
	}
	return append([]browsercookies.Cookie(nil), s.cookies[store.Path]...), nil
}

func (s *soundCloudCookieSource) set(path string, cookies ...browsercookies.Cookie) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cookies[path] = cookies
}

func (*soundCloudCookieSource) ChangedSince(browsercookies.Store, time.Time) bool { return true }

type soundCloudRoundTrip func(*http.Request) (*http.Response, error)

func (f soundCloudRoundTrip) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

type soundCloudEvents struct {
	mu     sync.Mutex
	events []model.SoundCloudAuthEvent
}

func (e *soundCloudEvents) Emit(event string, data any) {
	if event != protocol.EventSoundCloudAuth {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.events = append(e.events, data.(model.SoundCloudAuthEvent))
}

func (e *soundCloudEvents) states() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	states := make([]string, 0, len(e.events))
	for _, event := range e.events {
		states = append(states, event.State)
	}
	return states
}

func TestExpiredAuthAdoptsFreshBrowserTokenQuietly(t *testing.T) {
	store := browsercookies.Store{Kind: browsercookies.Gecko, Path: "zen-soundcloud"}
	oldToken := soundCloudJWT("42", time.Now().Add(-time.Hour).Unix())
	freshToken := soundCloudJWT("42", time.Now().Add(time.Hour).Unix())
	source := &soundCloudCookieSource{
		stores: []browsercookies.Store{store},
		cookies: map[string][]browsercookies.Cookie{
			store.Path: {{Name: "oauth_token", Value: freshToken, Container: "work"}},
		},
	}
	auth := authFromToken(oldToken)
	auth.Username = pointerIfSet("Listener")
	auth.Source = &browsercookies.SessionSource{Kind: store.Kind, Path: store.Path, Container: "work"}
	kv := baseprovider.MapKV{}
	saveSoundCloudAuth(t, kv, auth)
	requests := 0
	httpClient := &http.Client{Transport: soundCloudRoundTrip(func(*http.Request) (*http.Response, error) {
		requests++
		return soundCloudResponse(http.StatusOK, `{"id":42,"username":"Listener"}`), nil
	})}
	events := &soundCloudEvents{}
	provider, err := New(Options{KV: kv, Cookies: source, Bus: events, HTTPClient: httpClient, APIBase: "https://api.test"})
	if err != nil {
		t.Fatal(err)
	}

	account, err := provider.Account(context.Background())
	if err != nil || !account.SignedIn || requests != 0 {
		t.Fatalf("account = %#v, %v, requests = %d", account, err, requests)
	}
	var saved Auth
	if err := json.Unmarshal([]byte(kv[soundCloudAuthKey]), &saved); err != nil {
		t.Fatal(err)
	}
	if saved.AccessToken != freshToken {
		t.Fatal("fresh browser token was not persisted")
	}
	if got := events.states(); len(got) != 1 || got[0] != "restored" {
		t.Fatalf("auth events = %v, want one quiet restored", got)
	}
}

func TestExpiredTokenStillInBrowserKeepsAuthWithoutRotating(t *testing.T) {
	store := browsercookies.Store{Kind: browsercookies.Gecko, Path: "closed-browser"}
	expired := soundCloudJWT("42", time.Now().Add(-time.Hour).Unix())
	source := &soundCloudCookieSource{
		stores: []browsercookies.Store{store},
		cookies: map[string][]browsercookies.Cookie{store.Path: {
			{Name: "oauth_token", Value: expired},
			{Name: "oauth_refresh_token", Value: "browser-refresh"},
			{Name: "connect_session", Value: "browser-session"},
		}},
	}
	auth := authFromToken(expired)
	auth.Source = &browsercookies.SessionSource{Kind: store.Kind, Path: store.Path}
	kv := baseprovider.MapKV{}
	saveSoundCloudAuth(t, kv, auth)
	// the shape an earlier build stored, with the browser's refresh token beside it
	kv[soundCloudAuthKey] = strings.TrimSuffix(kv[soundCloudAuthKey], "}") +
		`,"refresh_token":"browser-refresh","connect_session":"browser-session"}`
	requests := 0
	httpClient := &http.Client{Transport: soundCloudRoundTrip(func(*http.Request) (*http.Response, error) {
		requests++
		return soundCloudResponse(http.StatusOK, `{"id":42}`), nil
	})}
	events := &soundCloudEvents{}
	provider, err := New(Options{KV: kv, Cookies: source, Bus: events, HTTPClient: httpClient, APIBase: "https://api.test"})
	if err != nil {
		t.Fatal(err)
	}

	if err := provider.Restore(context.Background()); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := provider.Account(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if provider.Status().SignedIn {
		t.Fatal("an expired token reads signed in")
	}
	if requests != 0 {
		t.Fatalf("%d requests sent for an expired token; the browser's tokens must never be refreshed", requests)
	}
	var saved Auth
	if err := json.Unmarshal([]byte(kv[soundCloudAuthKey]), &saved); err != nil || saved.Source == nil {
		t.Fatalf("expiry dropped the auth or its source: %v", err)
	}
	if strings.Contains(kv[soundCloudAuthKey], "browser-refresh") || strings.Contains(kv[soundCloudAuthKey], "browser-session") {
		t.Fatal("the browser's refresh token or session cookie is still stored")
	}
	if got := events.states(); len(got) != 1 || got[0] != "stale" {
		t.Fatalf("expiry emitted %v, want one quiet stale", got)
	}

	fresh := soundCloudJWT("42", time.Now().Add(time.Hour).Unix())
	source.set(store.Path, browsercookies.Cookie{Name: "oauth_token", Value: fresh})
	if _, err := provider.Account(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !provider.Status().SignedIn {
		t.Fatal("the browser's next token was not adopted")
	}
	if got := events.states(); len(got) != 2 || got[1] != "restored" {
		t.Fatalf("auth events = %v, want stale then a quiet restored", got)
	}
}

func TestRejectedTokenWithoutBrowserTokenExpiresWithReason(t *testing.T) {
	store := browsercookies.Store{Kind: browsercookies.Gecko, Path: "signed-out-soundcloud"}
	source := &soundCloudCookieSource{stores: []browsercookies.Store{store}, cookies: map[string][]browsercookies.Cookie{}}
	auth := authFromToken(soundCloudJWT("42", time.Now().Add(time.Hour).Unix()))
	auth.Source = &browsercookies.SessionSource{Kind: store.Kind, Path: store.Path}
	kv := baseprovider.MapKV{}
	saveSoundCloudAuth(t, kv, auth)
	httpClient := &http.Client{Transport: soundCloudRoundTrip(func(*http.Request) (*http.Response, error) {
		return soundCloudResponse(http.StatusUnauthorized, `{}`), nil
	})}
	events := &soundCloudEvents{}
	provider, err := New(Options{KV: kv, Cookies: source, Bus: events, HTTPClient: httpClient, APIBase: "https://api.test"})
	if err != nil {
		t.Fatal(err)
	}

	if err := provider.get(context.Background(), "/me/activities", nil, true, nil); !errors.Is(err, baseprovider.ErrSignInRequired) {
		t.Fatalf("rejected request error = %v", err)
	}
	if _, ok := kv[soundCloudAuthKey]; ok {
		t.Fatal("rejected authorization remained persisted")
	}
	events.mu.Lock()
	defer events.mu.Unlock()
	if len(events.events) != 1 || events.events[0].State != "session_expired" || events.events[0].Message == nil {
		t.Fatalf("auth events = %#v", events.events)
	}
}

func TestBrowserReadNetworkProofFailureKeepsStoredAuth(t *testing.T) {
	store := browsercookies.Store{Kind: browsercookies.Gecko, Path: "offline-soundcloud"}
	source := &soundCloudCookieSource{
		stores: []browsercookies.Store{store},
		cookies: map[string][]browsercookies.Cookie{
			store.Path: {{Name: "oauth_token", Value: "fresh-opaque"}},
		},
	}
	auth := Auth{
		AccessToken: "old-opaque", ClientID: fallbackClientID,
		ExpiresAt: time.Now().Add(-time.Hour).Unix(), UserID: 42,
		Source: &browsercookies.SessionSource{Kind: store.Kind, Path: store.Path},
	}
	kv := baseprovider.MapKV{}
	saveSoundCloudAuth(t, kv, auth)
	offline := errors.New("offline")
	httpClient := &http.Client{Transport: soundCloudRoundTrip(func(*http.Request) (*http.Response, error) {
		return nil, offline
	})}
	provider, err := New(Options{KV: kv, Cookies: source, HTTPClient: httpClient, APIBase: "https://api.test"})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := provider.Account(context.Background()); !errors.Is(err, offline) {
		t.Fatalf("network error = %v", err)
	}
	if _, ok := kv[soundCloudAuthKey]; !ok || provider.auth == nil {
		t.Fatal("network failure deleted the stored authorization")
	}
}

func TestTooManyRequestsDuringSameUserProofKeepsAuth(t *testing.T) {
	store := browsercookies.Store{Kind: browsercookies.Gecko, Path: "busy-soundcloud"}
	source := &soundCloudCookieSource{
		stores:  []browsercookies.Store{store},
		cookies: map[string][]browsercookies.Cookie{store.Path: {{Name: "oauth_token", Value: "fresh-opaque"}}},
	}
	auth := Auth{
		AccessToken: "old-opaque", ClientID: fallbackClientID,
		ExpiresAt: time.Now().Add(time.Hour).Unix(), UserID: 42,
		Source: &browsercookies.SessionSource{Kind: store.Kind, Path: store.Path},
	}
	kv := baseprovider.MapKV{}
	saveSoundCloudAuth(t, kv, auth)
	httpClient := &http.Client{Transport: soundCloudRoundTrip(func(request *http.Request) (*http.Response, error) {
		if strings.Contains(request.Header.Get("Authorization"), "old-opaque") {
			return soundCloudResponse(http.StatusUnauthorized, `{}`), nil
		}
		return soundCloudResponse(http.StatusTooManyRequests, `{}`), nil
	})}
	events := &soundCloudEvents{}
	provider, err := New(Options{KV: kv, Cookies: source, Bus: events, HTTPClient: httpClient, APIBase: "https://api.test"})
	if err != nil {
		t.Fatal(err)
	}

	if err := provider.get(context.Background(), "/me/activities", nil, true, nil); err == nil || errors.Is(err, baseprovider.ErrSignInRequired) {
		t.Fatalf("throttled proof error = %v, want a non-session error", err)
	}
	if _, ok := kv[soundCloudAuthKey]; !ok || provider.auth == nil {
		t.Fatal("a 429 during the same-user proof ended the session")
	}
	if got := events.states(); len(got) != 0 {
		t.Fatalf("auth events = %v", got)
	}
}

func TestMissingRememberedStoreSignInRunsImportFlow(t *testing.T) {
	for _, expired := range []bool{false, true} {
		store := browsercookies.Store{Kind: browsercookies.Gecko, Path: "/gone/cookies.sqlite"}
		source := &soundCloudCookieSource{cookies: map[string][]browsercookies.Cookie{}, missing: map[string]bool{store.Path: true}}
		expiresAt := time.Now().Add(time.Hour)
		if expired {
			expiresAt = time.Now().Add(-time.Hour)
		}
		auth := authFromToken(soundCloudJWT("42", expiresAt.Unix()))
		auth.Source = &browsercookies.SessionSource{Kind: store.Kind, Path: store.Path}
		kv := baseprovider.MapKV{}
		saveSoundCloudAuth(t, kv, auth)
		httpClient := &http.Client{Transport: soundCloudRoundTrip(func(*http.Request) (*http.Response, error) {
			return soundCloudResponse(http.StatusUnauthorized, `{}`), nil
		})}
		events := &soundCloudEvents{}
		provider, err := New(Options{KV: kv, Cookies: source, Bus: events, HTTPClient: httpClient, APIBase: "https://api.test"})
		if err != nil {
			t.Fatal(err)
		}

		url, err := provider.SignIn(context.Background())
		if err != nil || url != soundCloudSignInURL {
			t.Fatalf("expired=%v: sign in = %q, %v, want the import flow", expired, url, err)
		}
		deadline := time.Now().Add(time.Second)
		for !slices.Contains(events.states(), "waiting") && time.Now().Before(deadline) {
			time.Sleep(10 * time.Millisecond)
		}
		if !slices.Contains(events.states(), "waiting") {
			t.Fatalf("expired=%v: auth events = %v, want waiting", expired, events.states())
		}
		if err := provider.SignOut(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestUnreadableRememberedStoreSignInImportsFromAnotherBrowser(t *testing.T) {
	for _, expired := range []bool{true, false} {
		locked := browsercookies.Store{Kind: browsercookies.Gecko, Path: "locked-profile"}
		other := browsercookies.Store{Kind: browsercookies.Gecko, Path: "other-profile"}
		good := soundCloudJWT("7", time.Now().Add(time.Hour).Unix())
		source := &soundCloudCookieSource{
			stores:     []browsercookies.Store{locked, other},
			cookies:    map[string][]browsercookies.Cookie{other.Path: {{Name: "oauth_token", Value: good}}},
			unreadable: map[string]bool{locked.Path: true},
		}
		expiresAt := time.Now().Add(time.Hour)
		if expired {
			expiresAt = time.Now().Add(-time.Hour)
		}
		auth := authFromToken(soundCloudJWT("42", expiresAt.Unix()))
		auth.Source = &browsercookies.SessionSource{Kind: locked.Kind, Path: locked.Path}
		kv := baseprovider.MapKV{}
		saveSoundCloudAuth(t, kv, auth)
		httpClient := &http.Client{Transport: soundCloudRoundTrip(func(request *http.Request) (*http.Response, error) {
			if strings.Contains(request.Header.Get("Authorization"), good) {
				return soundCloudResponse(http.StatusOK, `{"id":7,"username":"Other"}`), nil
			}
			return soundCloudResponse(http.StatusUnauthorized, `{}`), nil
		})}
		events := &soundCloudEvents{}
		provider, err := New(Options{KV: kv, Cookies: source, Bus: events, HTTPClient: httpClient, APIBase: "https://api.test"})
		if err != nil {
			t.Fatal(err)
		}

		url, err := provider.SignIn(context.Background())
		if err != nil || url != "" {
			t.Fatalf("expired=%v: sign in = %q, %v, want the import flow to sign in from the other browser", expired, url, err)
		}
		if status := provider.Status(); !status.SignedIn || !slices.Contains(events.states(), "signed_in") {
			t.Fatalf("expired=%v: status %#v, events %v", expired, status, events.states())
		}
	}
}

func TestExpiredKeptTokenNeverReachesPublicRequests(t *testing.T) {
	expired := soundCloudJWT("42", time.Now().Add(-time.Hour).Unix())
	store := browsercookies.Store{Kind: browsercookies.Gecko, Path: "closed-browser"}
	source := &soundCloudCookieSource{
		stores:  []browsercookies.Store{store},
		cookies: map[string][]browsercookies.Cookie{store.Path: {{Name: "oauth_token", Value: expired}}},
	}
	auth := authFromToken(expired)
	auth.Source = &browsercookies.SessionSource{Kind: store.Kind, Path: store.Path}
	kv := baseprovider.MapKV{}
	saveSoundCloudAuth(t, kv, auth)
	var mu sync.Mutex
	withToken := 0
	httpClient := &http.Client{Transport: soundCloudRoundTrip(func(request *http.Request) (*http.Response, error) {
		if request.Header.Get("Authorization") != "" {
			mu.Lock()
			withToken++
			mu.Unlock()
			return soundCloudResponse(http.StatusUnauthorized, `{}`), nil
		}
		return soundCloudResponse(http.StatusOK, `{"collection":[]}`), nil
	})}
	provider, err := New(Options{KV: kv, Cookies: source, HTTPClient: httpClient, APIBase: "https://api.test"})
	if err != nil {
		t.Fatal(err)
	}
	if err := provider.Restore(context.Background()); err != nil {
		t.Fatal(err)
	}

	if _, err := provider.Search(context.Background(), "artist", model.SearchKindSongs); err != nil {
		t.Fatalf("guest search with a kept expired token = %v", err)
	}
	if _, err := provider.Home(context.Background(), ""); err != nil {
		t.Fatalf("guest home with a kept expired token = %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if withToken != 0 {
		t.Fatalf("%d requests carried the expired token", withToken)
	}
}

func TestTokenExpiringWhileRunningIsAnnouncedOnce(t *testing.T) {
	store := browsercookies.Store{Kind: browsercookies.Gecko, Path: "idle-browser"}
	token := soundCloudJWT("42", time.Now().Add(expiryMarginSecondsDuration()+2*time.Second).Unix())
	source := &soundCloudCookieSource{
		stores:  []browsercookies.Store{store},
		cookies: map[string][]browsercookies.Cookie{store.Path: {{Name: "oauth_token", Value: token}}},
	}
	auth := authFromToken(token)
	auth.Source = &browsercookies.SessionSource{Kind: store.Kind, Path: store.Path}
	kv := baseprovider.MapKV{}
	saveSoundCloudAuth(t, kv, auth)
	events := &soundCloudEvents{}
	provider, err := New(Options{KV: kv, Cookies: source, Bus: events, APIBase: "https://api.test",
		HTTPClient: &http.Client{Transport: soundCloudRoundTrip(func(*http.Request) (*http.Response, error) {
			return soundCloudResponse(http.StatusOK, `{"collection":[]}`), nil
		})}})
	if err != nil {
		t.Fatal(err)
	}
	if !provider.Status().SignedIn {
		t.Fatal("token should still be valid at start")
	}
	time.Sleep(2500 * time.Millisecond)
	for range 2 {
		if _, err := provider.Library(context.Background()); !errors.Is(err, baseprovider.ErrSignInRequired) {
			t.Fatalf("library with an expired token = %v", err)
		}
	}
	if got := events.states(); len(got) != 1 || got[0] != "stale" {
		t.Fatalf("auth events = %v, want one quiet stale", got)
	}
	if _, ok := kv[soundCloudAuthKey]; !ok {
		t.Fatal("expiry dropped the stored auth")
	}
}

func expiryMarginSecondsDuration() time.Duration {
	return time.Duration(expiryMarginSeconds) * time.Second
}

func TestSignInSurfacesOfflineError(t *testing.T) {
	auth := authFromToken(soundCloudJWT("42", time.Now().Add(time.Hour).Unix()))
	kv := baseprovider.MapKV{}
	saveSoundCloudAuth(t, kv, auth)
	offline := errors.New("offline")
	httpClient := &http.Client{Transport: soundCloudRoundTrip(func(*http.Request) (*http.Response, error) {
		return nil, offline
	})}
	provider, err := New(Options{KV: kv, Cookies: &soundCloudCookieSource{cookies: map[string][]browsercookies.Cookie{}}, HTTPClient: httpClient, APIBase: "https://api.test"})
	if err != nil {
		t.Fatal(err)
	}
	if url, err := provider.SignIn(context.Background()); !errors.Is(err, offline) || url != "" {
		t.Fatalf("offline sign in = %q, %v", url, err)
	}
}

func TestSignInWatchTriesARefusedTokenOnceAndStaysQuiet(t *testing.T) {
	store := browsercookies.Store{Kind: browsercookies.Gecko, Path: "revoked-soundcloud"}
	revoked := soundCloudJWT("42", time.Now().Add(time.Hour).Unix())
	source := &soundCloudCookieSource{
		stores:  []browsercookies.Store{store},
		cookies: map[string][]browsercookies.Cookie{store.Path: {{Name: "oauth_token", Value: revoked}}},
	}
	var mu sync.Mutex
	proofs := 0
	httpClient := &http.Client{Transport: soundCloudRoundTrip(func(*http.Request) (*http.Response, error) {
		mu.Lock()
		proofs++
		mu.Unlock()
		return soundCloudResponse(http.StatusUnauthorized, `{}`), nil
	})}
	events := &soundCloudEvents{}
	provider, err := New(Options{Cookies: source, Bus: events, HTTPClient: httpClient, APIBase: "https://api.test"})
	if err != nil {
		t.Fatal(err)
	}
	defer provider.SignOut()

	if url, err := provider.SignIn(context.Background()); err != nil || url != soundCloudSignInURL {
		t.Fatalf("sign in = %q, %v", url, err)
	}
	time.Sleep(4300 * time.Millisecond)
	mu.Lock()
	got := proofs
	mu.Unlock()
	if got != 1 {
		t.Fatalf("/me proofs = %d over two watch ticks, want the refused token tried once", got)
	}
	if states := events.states(); slices.Contains(states, "error") {
		t.Fatalf("auth events = %v, want no error before the flow ends", states)
	}
}

func TestSignInWatchBacksOffAThrottledToken(t *testing.T) {
	store := browsercookies.Store{Kind: browsercookies.Gecko, Path: "throttled-soundcloud"}
	token := soundCloudJWT("42", time.Now().Add(time.Hour).Unix())
	source := &soundCloudCookieSource{
		stores:  []browsercookies.Store{store},
		cookies: map[string][]browsercookies.Cookie{store.Path: {{Name: "oauth_token", Value: token}}},
	}
	var mu sync.Mutex
	proofs := 0
	httpClient := &http.Client{Transport: soundCloudRoundTrip(func(*http.Request) (*http.Response, error) {
		mu.Lock()
		proofs++
		mu.Unlock()
		return soundCloudResponse(http.StatusTooManyRequests, `{}`), nil
	})}
	provider, err := New(Options{Cookies: source, HTTPClient: httpClient, APIBase: "https://api.test"})
	if err != nil {
		t.Fatal(err)
	}
	defer provider.SignOut()

	if url, err := provider.SignIn(context.Background()); err != nil || url != soundCloudSignInURL {
		t.Fatalf("sign in = %q, %v", url, err)
	}
	time.Sleep(4300 * time.Millisecond)
	mu.Lock()
	got := proofs
	mu.Unlock()
	if got > 2 {
		t.Fatalf("/me proofs = %d in 4.3 s, want the throttled token backed off", got)
	}
}

func TestRejectedAuthStartsBrowserSignInFlow(t *testing.T) {
	store := browsercookies.Store{Kind: browsercookies.Gecko, Path: "rejected-soundcloud"}
	source := &soundCloudCookieSource{stores: []browsercookies.Store{store}, cookies: map[string][]browsercookies.Cookie{}}
	auth := authFromToken(soundCloudJWT("42", time.Now().Add(time.Hour).Unix()))
	auth.Source = &browsercookies.SessionSource{Kind: store.Kind, Path: store.Path}
	kv := baseprovider.MapKV{}
	saveSoundCloudAuth(t, kv, auth)
	httpClient := &http.Client{Transport: soundCloudRoundTrip(func(*http.Request) (*http.Response, error) {
		return soundCloudResponse(http.StatusUnauthorized, `{}`), nil
	})}
	provider, err := New(Options{KV: kv, Cookies: source, HTTPClient: httpClient, APIBase: "https://api.test"})
	if err != nil {
		t.Fatal(err)
	}

	if url, err := provider.SignIn(context.Background()); err != nil || url != soundCloudSignInURL {
		t.Fatalf("sign-in = %q, %v", url, err)
	}
	if provider.auth != nil {
		t.Fatal("rejected authorization still reports signed in")
	}
	if err := provider.SignOut(); err != nil {
		t.Fatal(err)
	}
}

func TestPersonalRequestRefreshesFromBrowserAndRetriesOnce(t *testing.T) {
	store := browsercookies.Store{Kind: browsercookies.Gecko, Path: "retry-soundcloud"}
	oldToken := soundCloudJWT("42", time.Now().Add(time.Hour).Unix())
	freshToken := soundCloudJWT("42", time.Now().Add(2*time.Hour).Unix())
	source := &soundCloudCookieSource{
		stores: []browsercookies.Store{store},
		cookies: map[string][]browsercookies.Cookie{
			store.Path: {{Name: "oauth_token", Value: freshToken}},
		},
	}
	auth := authFromToken(oldToken)
	auth.Source = &browsercookies.SessionSource{Kind: store.Kind, Path: store.Path}
	kv := baseprovider.MapKV{}
	saveSoundCloudAuth(t, kv, auth)
	var headers []string
	httpClient := &http.Client{Transport: soundCloudRoundTrip(func(request *http.Request) (*http.Response, error) {
		headers = append(headers, request.Header.Get("Authorization"))
		if strings.Contains(request.Header.Get("Authorization"), oldToken) {
			return soundCloudResponse(http.StatusUnauthorized, `{}`), nil
		}
		return soundCloudResponse(http.StatusOK, `{}`), nil
	})}
	provider, err := New(Options{KV: kv, Cookies: source, HTTPClient: httpClient, APIBase: "https://api.test"})
	if err != nil {
		t.Fatal(err)
	}

	var out map[string]any
	if err := provider.get(context.Background(), "/me/activities", nil, true, &out); err != nil {
		t.Fatal(err)
	}
	if len(headers) != 2 || !strings.Contains(headers[0], oldToken) || !strings.Contains(headers[1], freshToken) {
		t.Fatalf("authorization sequence does not show one refresh and retry")
	}
}

func TestConcurrentRejectionsKeepTheAdoptedToken(t *testing.T) {
	store := browsercookies.Store{Kind: browsercookies.Gecko, Path: "racing-soundcloud"}
	oldToken := soundCloudJWT("42", time.Now().Add(time.Hour).Unix())
	freshToken := soundCloudJWT("42", time.Now().Add(2*time.Hour).Unix())
	source := &soundCloudCookieSource{
		stores:  []browsercookies.Store{store},
		cookies: map[string][]browsercookies.Cookie{store.Path: {{Name: "oauth_token", Value: freshToken}}},
	}
	auth := authFromToken(oldToken)
	auth.Source = &browsercookies.SessionSource{Kind: store.Kind, Path: store.Path}
	kv := baseprovider.MapKV{}
	saveSoundCloudAuth(t, kv, auth)
	var staleSent sync.WaitGroup
	staleSent.Add(2)
	httpClient := &http.Client{Transport: soundCloudRoundTrip(func(request *http.Request) (*http.Response, error) {
		if strings.Contains(request.Header.Get("Authorization"), oldToken) {
			staleSent.Done()
			staleSent.Wait()
			return soundCloudResponse(http.StatusUnauthorized, `{}`), nil
		}
		return soundCloudResponse(http.StatusOK, `{}`), nil
	})}
	events := &soundCloudEvents{}
	provider, err := New(Options{KV: kv, Cookies: source, Bus: events, HTTPClient: httpClient, APIBase: "https://api.test"})
	if err != nil {
		t.Fatal(err)
	}

	errs := make(chan error, 2)
	for range 2 {
		go func() { errs <- provider.get(context.Background(), "/me/activities", nil, true, nil) }()
	}
	for range 2 {
		if err := <-errs; err != nil {
			t.Errorf("request after the shared rejection = %v", err)
		}
	}
	if !provider.Status().SignedIn || slices.Contains(events.states(), "session_expired") {
		t.Fatalf("second rejection erased the adopted token: events %v", events.states())
	}
}

func soundCloudJWT(subject string, expiresAt int64) string {
	payload, _ := json.Marshal(map[string]any{"sub": subject, "exp": expiresAt})
	return "header." + base64.RawURLEncoding.EncodeToString(payload) + ".signature"
}

func saveSoundCloudAuth(t *testing.T, kv baseprovider.MapKV, auth Auth) {
	t.Helper()
	raw, err := json.Marshal(auth)
	if err != nil {
		t.Fatal(err)
	}
	kv[soundCloudAuthKey] = string(raw)
}

func soundCloudResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     http.StatusText(status),
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
