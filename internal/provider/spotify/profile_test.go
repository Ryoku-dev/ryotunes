package spotify

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/devgianlu/go-librespot/session"
	providerapi "github.com/ryoku-dev/ryotunes/internal/provider"
)

// profileServer answers every request, whatever its host, from handler, so
// the profile check never reaches Spotify.
func profileServer(t *testing.T, handler http.HandlerFunc) (*http.Client, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		handler(response, request)
	}))
	t.Cleanup(server.Close)
	target, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	inner := server.Client().Transport
	return &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		redirected := request.Clone(request.Context())
		redirected.URL.Scheme, redirected.URL.Host = target.Scheme, target.Host
		return inner.RoundTrip(redirected)
	})}, &calls
}

func fixtureToken(context.Context) (string, error) { return "fixture-token", nil }

func TestUnansweredProfileCheckKeepsTheRestoredSession(t *testing.T) {
	for _, test := range []struct {
		name      string
		status    int
		header    string
		body      string
		wantRetry time.Duration
	}{
		{name: "rate limited", status: http.StatusTooManyRequests, header: "57", wantRetry: 57 * time.Second},
		{name: "outage", status: http.StatusServiceUnavailable, wantRetry: profileRetryDefault},
		{name: "unreadable body", status: http.StatusOK, body: "<html>", wantRetry: profileRetryDefault},
	} {
		t.Run(test.name, func(t *testing.T) {
			client, _ := profileServer(t, func(response http.ResponseWriter, _ *http.Request) {
				if test.header != "" {
					response.Header().Set("Retry-After", test.header)
				}
				response.WriteHeader(test.status)
				fmt.Fprint(response, test.body)
			})
			provider := newTestProvider(t, Options{DataDir: t.TempDir(), HTTPClient: client})
			before := time.Now()
			profile, err := provider.premiumProfile(context.Background(), fixtureToken)
			if err != nil {
				t.Fatalf("an unanswered profile check failed the restore: %v", err)
			}
			wait := profile.recheckAt.Sub(before)
			if wait < test.wantRetry-2*time.Second || wait > test.wantRetry+2*time.Second {
				t.Fatalf("re-check in %s, want about %s", wait, test.wantRetry)
			}
			if !provider.installSession(new(session.Session), profile) {
				t.Fatal("session was not installed")
			}
			if status := provider.Status(); !status.SignedIn || status.Premium == nil || !*status.Premium {
				t.Fatalf("status after an unanswered profile check = %#v", status)
			}
		})
	}
}

func TestProfileRecheckWaitsThenEnforcesPremium(t *testing.T) {
	var answer atomic.Value
	answer.Store("rate")
	client, calls := profileServer(t, func(response http.ResponseWriter, _ *http.Request) {
		switch answer.Load().(string) {
		case "rate":
			response.Header().Set("Retry-After", "30")
			response.WriteHeader(http.StatusTooManyRequests)
		case "unauthorized":
			response.WriteHeader(http.StatusUnauthorized)
		default:
			fmt.Fprint(response, `{"display_name":"Fixture","product":"free"}`)
		}
	})
	provider := newTestProvider(t, Options{DataDir: t.TempDir(), HTTPClient: client})
	owner := new(session.Session)
	provider.installSession(owner, accountProfile{DisplayName: "fixture", recheckAt: time.Now().Add(-time.Second)})

	if !provider.recheckProfile(context.Background(), owner, fixtureToken) {
		t.Fatal("a rate-limited re-check refused playback")
	}
	if calls.Load() != 1 {
		t.Fatalf("profile requests = %d, want 1", calls.Load())
	}
	if !provider.recheckProfile(context.Background(), owner, fixtureToken) || calls.Load() != 1 {
		t.Fatalf("re-check asked again before Retry-After passed (requests = %d)", calls.Load())
	}

	answer.Store("unauthorized")
	provider.mu.Lock()
	provider.profileRecheck = time.Now().Add(-time.Second)
	provider.mu.Unlock()
	if !provider.recheckProfile(context.Background(), owner, fixtureToken) || !provider.Status().SignedIn {
		t.Fatal("a failing re-check signed the session out")
	}

	answer.Store("free")
	provider.mu.Lock()
	provider.profileRecheck = time.Now().Add(-time.Second)
	provider.mu.Unlock()
	if provider.recheckProfile(context.Background(), owner, fixtureToken) {
		t.Fatal("a free account was allowed to play")
	}
	status := provider.Status()
	if !status.SignedIn || status.Premium == nil || *status.Premium {
		t.Fatalf("status after a free answer = %#v", status)
	}
	if _, err := provider.session(context.Background()); !errors.Is(err, providerapi.ErrNotPremium) {
		t.Fatalf("session error = %v, want ErrNotPremium", err)
	}
}

func TestRetryAfterIsClamped(t *testing.T) {
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		header string
		want   time.Duration
	}{
		{"57", 57 * time.Second},
		{now.Add(90 * time.Second).Format(http.TimeFormat), 90 * time.Second},
		{"0", time.Second},
		{"86400", profileRetryMax},
		{"", profileRetryDefault},
		{"soon", profileRetryDefault},
		{"-5", profileRetryDefault},
	} {
		if got := retryAfter(test.header, now); got != test.want {
			t.Errorf("retryAfter(%q) = %s, want %s", test.header, got, test.want)
		}
	}
}
