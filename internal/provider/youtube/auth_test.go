package youtube

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ryoku-dev/ryotunes/internal/provider/browsercookies"
)

type fakeCookieSource struct {
	mu        sync.Mutex
	stores    []browsercookies.Store
	cookies   map[string][]browsercookies.Cookie
	changed   bool
	missing   map[string]bool
	discovers int
}

func (f *fakeCookieSource) Discover() []browsercookies.Store {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.discovers++
	return append([]browsercookies.Store(nil), f.stores...)
}

func (f *fakeCookieSource) Resolve(source browsercookies.SessionSource) (browsercookies.Store, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.missing[source.Path] {
		return browsercookies.Store{}, &fs.PathError{Op: "lstat", Path: source.Path, Err: fs.ErrNotExist}
	}
	return browsercookies.Store{Kind: source.Kind, Path: source.Path}, nil
}

func (f *fakeCookieSource) Read(store browsercookies.Store, _ ...string) ([]browsercookies.Cookie, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]browsercookies.Cookie(nil), f.cookies[store.Path]...), nil
}

func (f *fakeCookieSource) ChangedSince(browsercookies.Store, time.Time) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.changed
}

type authRoundTripper func(*http.Request) (*http.Response, error)

func (f authRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestBuildYouTubeCookieCandidatesKeepsSessionsSeparate(t *testing.T) {
	now := time.Unix(1_900_000_000, 0)
	first := browsercookies.Store{Kind: browsercookies.Gecko, Path: "first-profile"}
	second := browsercookies.Store{Kind: browsercookies.Gecko, Path: "second-profile"}
	source := &fakeCookieSource{
		stores: []browsercookies.Store{first, second},
		cookies: map[string][]browsercookies.Cookie{
			first.Path: {
				{Name: "SAPISID", Value: "container", Host: ".youtube.com", Path: "/", Container: "2"},
				{Name: "SAPISID", Value: "default", Host: ".youtube.com", Path: "/"},
				{Name: "PREF", Value: "domain-long", Host: ".youtube.com", Path: "/youtubei/"},
				{Name: "PREF", Value: "music-short", Host: ".music.youtube.com", Path: "/"},
				{Name: "PREF", Value: "music-long", Host: ".music.youtube.com", Path: "/youtubei"},
				{Name: "HOSTONLY", Value: "yes", Host: "music.youtube.com", Path: "/"},
				{Name: "PARTITIONED", Value: "no", Host: ".youtube.com", Partitioned: true},
				{Name: "EXPIRED", Value: "no", Host: ".youtube.com", Expires: now},
				{Name: "WWW", Value: "no", Host: "www.youtube.com"},
				{Name: "ROOT", Value: "no", Host: "youtube.com"},
				{Name: "BADVALUE", Value: "line\nbreak", Host: ".youtube.com"},
				{Name: "SEMICOLON", Value: "a;b", Host: ".youtube.com"},
				{Name: "bad name", Value: "no", Host: ".youtube.com"},
				{Name: "WATCH", Value: "no", Host: ".youtube.com", Path: "/watch"},
			},
			second.Path: {
				{Name: "__Secure-3PAPISID", Value: "second", Host: ".youtube.com", Path: "/"},
			},
		},
	}

	candidates := buildYouTubeCookieCandidates(source, source.stores, now)
	if len(candidates) != 3 {
		t.Fatalf("candidates = %#v", candidates)
	}
	if candidates[0].store != first || candidates[0].container != "" {
		t.Fatalf("first candidate = %#v", candidates[0])
	}
	if candidates[1].store != first || candidates[1].container != "2" {
		t.Fatalf("second candidate = %#v", candidates[1])
	}
	if candidates[2].store != second || candidates[2].container != "" {
		t.Fatalf("third candidate = %#v", candidates[2])
	}
	cookies := parseCookieHeader(candidates[0].header)
	if cookies["SAPISID"] != "default" || cookies["PREF"] != "music-long" || cookies["HOSTONLY"] != "yes" {
		t.Fatalf("default candidate cookies = %#v", cookies)
	}
	for _, name := range []string{"PARTITIONED", "EXPIRED", "WWW", "ROOT", "BADVALUE", "SEMICOLON", "bad name", "WATCH"} {
		if _, ok := cookies[name]; ok {
			t.Errorf("ineligible cookie %q was included", name)
		}
	}
	if got := parseCookieHeader(candidates[1].header)["SAPISID"]; got != "container" {
		t.Fatalf("container SAPISID = %q", got)
	}
	if got := parseCookieHeader(candidates[2].header)["__Secure-3PAPISID"]; got != "second" {
		t.Fatalf("second profile SAPISID = %q", got)
	}
}

func TestSignInTriesNextCookieCandidateAfterTransportFailure(t *testing.T) {
	first := browsercookies.Store{Path: "signed-out-profile"}
	second := browsercookies.Store{Path: "signed-in-profile"}
	source := &fakeCookieSource{
		stores: []browsercookies.Store{first, second},
		cookies: map[string][]browsercookies.Cookie{
			first.Path:  {{Name: "SAPISID", Value: "first-secret", Host: ".youtube.com"}},
			second.Path: {{Name: "SAPISID", Value: "second-secret", Host: ".youtube.com"}},
		},
	}
	var headers []string
	httpClient := &http.Client{Transport: authRoundTripper(func(request *http.Request) (*http.Response, error) {
		if strings.HasSuffix(request.URL.Path, "/account/account_menu") {
			header := request.Header.Get("Cookie")
			headers = append(headers, header)
			if strings.Contains(header, "first-secret") {
				return nil, errors.New("network offline")
			}
			return accountResponse(), nil
		}
		return jsonResponse(http.StatusOK, `{}`), nil
	})}
	client := newAuthTestClient(t, source, httpClient, slog.New(slog.DiscardHandler))
	defer client.Close()

	flow, err := client.SignIn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if flow.URL != "" || flow.Done != nil {
		t.Fatalf("flow = %#v", flow)
	}
	if len(headers) < 4 || !strings.Contains(headers[0], "first-secret") || !strings.Contains(headers[len(headers)-1], "second-secret") {
		t.Fatalf("candidate request order = %#v", headers)
	}
}

func TestSignInFallsBackAndWatcherContinuesAfterFailure(t *testing.T) {
	store := browsercookies.Store{Path: "watch-profile"}
	source := &fakeCookieSource{
		stores:  []browsercookies.Store{store},
		cookies: map[string][]browsercookies.Cookie{store.Path: {{Name: "SAPISID", Value: "never-log-this", Host: ".youtube.com"}}},
		changed: true,
	}
	var mu sync.Mutex
	accountMenuRequests := 0
	httpClient := &http.Client{Transport: authRoundTripper(func(request *http.Request) (*http.Response, error) {
		if strings.HasSuffix(request.URL.Path, "/account/account_menu") {
			mu.Lock()
			accountMenuRequests++
			requestNumber := accountMenuRequests
			mu.Unlock()
			if requestNumber <= 2 {
				return jsonResponse(http.StatusBadRequest, `{}`), nil
			}
			return accountResponse(), nil
		}
		return jsonResponse(http.StatusOK, `{}`), nil
	})}
	var logs bytes.Buffer
	client := newAuthTestClient(t, source, httpClient, slog.New(slog.NewTextHandler(&logs, nil)))
	defer client.Close()

	flow, err := client.SignIn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if flow.URL != youtubeSignInURL || flow.Done == nil {
		t.Fatalf("flow = %#v", flow)
	}
	select {
	case err := <-flow.Done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(6 * time.Second):
		t.Fatal("watcher did not continue to the successful cookie attempt")
	}
	mu.Lock()
	requests := accountMenuRequests
	mu.Unlock()
	if requests < 3 {
		t.Fatalf("account menu requests = %d, want at least 3", requests)
	}
	logText := logs.String()
	if !strings.Contains(logText, "store=watch-profile") || !strings.Contains(logText, "container=\"\"") {
		t.Fatalf("candidate warning = %q", logText)
	}
	if strings.Contains(logText, "never-log-this") {
		t.Fatalf("cookie value reached log: %q", logText)
	}
}

func TestSignInWatchTriesARefusedSessionOnce(t *testing.T) {
	store := browsercookies.Store{Path: "revoked-profile"}
	source := &fakeCookieSource{
		stores:  []browsercookies.Store{store},
		cookies: map[string][]browsercookies.Cookie{store.Path: {{Name: "SAPISID", Value: "revoked", Host: ".youtube.com"}}},
		changed: true,
	}
	var mu sync.Mutex
	accountMenuRequests := 0
	httpClient := &http.Client{Transport: authRoundTripper(func(request *http.Request) (*http.Response, error) {
		if strings.HasSuffix(request.URL.Path, "/account/account_menu") {
			mu.Lock()
			accountMenuRequests++
			mu.Unlock()
			return jsonResponse(http.StatusUnauthorized, `{}`), nil
		}
		return jsonResponse(http.StatusOK, `{}`), nil
	})}
	client := newAuthTestClient(t, source, httpClient, slog.New(slog.DiscardHandler))
	defer client.Close()

	flow, err := client.SignIn(context.Background())
	if err != nil || flow.Done == nil {
		t.Fatalf("flow = %#v, %v", flow, err)
	}
	time.Sleep(4300 * time.Millisecond)
	mu.Lock()
	requests := accountMenuRequests
	mu.Unlock()
	if requests != 1 {
		t.Fatalf("account menu requests = %d over two watch ticks, want the refused session tried once", requests)
	}
}
