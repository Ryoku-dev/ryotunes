package youtube

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
	"github.com/ryoku-dev/ryotunes/internal/provider"
	"github.com/ryoku-dev/ryotunes/internal/provider/browsercookies"
)

func TestRestoreReAdoptsFreshBrowserSessionAndPersistsIt(t *testing.T) {
	store := browsercookies.Store{Kind: browsercookies.Gecko, Path: "zen-profile"}
	source := youtubeStore(store.Path, "SAPISID", "fresh")
	kv := signedInKV("SAPISID=stale", `{"signedIn":true,"name":"Tester","handle":"@tester"}`)
	saveTestCookieSource(t, kv, browsercookies.SessionSource{Kind: store.Kind, Path: store.Path})
	httpClient := routeYouTube(func(endpoint, _ string) *http.Response {
		if endpoint == "account/account_menu" {
			return accountResponse()
		}
		return jsonResponse(http.StatusOK, `{}`)
	})
	events := &accountEvents{}

	client := newSessionTestClient(t, kv, source, httpClient, events)
	if err := client.Restore(context.Background()); err != nil {
		t.Fatal(err)
	}
	client.Close()
	if got := kv[cookieKey]; !strings.Contains(got, "fresh") {
		t.Fatal("persisted cookie was not refreshed")
	}
	if got := events.all(); len(got) != 1 || !got[0].SignedIn {
		t.Fatalf("re-adoption events = %#v, want one signed-in account", got)
	}

	restarted := newSessionTestClient(t, kv, source, httpClient, events)
	defer restarted.Close()
	if err := restarted.Restore(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !restarted.CurrentAccount().SignedIn {
		t.Fatal("restarted session is signed out")
	}
}

func TestRestoreRefusesSameNameWithDifferentHandle(t *testing.T) {
	store := browsercookies.Store{Kind: browsercookies.Gecko, Path: "switched-zen"}
	source := youtubeStore(store.Path, "SAPISID", "work")
	kv := signedInKV("SAPISID=personal", `{"signedIn":true,"name":"Sam Doe","handle":"@sam_personal"}`)
	saveTestCookieSource(t, kv, browsercookies.SessionSource{Kind: store.Kind, Path: store.Path})
	httpClient := routeYouTube(func(endpoint, cookie string) *http.Response {
		if endpoint != "account/account_menu" {
			return jsonResponse(http.StatusOK, `{}`)
		}
		if strings.Contains(cookie, "work") {
			return accountMenu("Sam Doe", "@sam_work")
		}
		return accountMenu("Sam Doe", "@sam_personal")
	})
	client := newSessionTestClient(t, kv, source, httpClient, nil)
	defer client.Close()

	if err := client.Restore(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := kv[cookieKey]; got != "SAPISID=personal" {
		t.Fatal("a different account with the same display name was adopted")
	}
	if handle := client.CurrentAccount().Handle; handle == nil || *handle != "@sam_personal" {
		t.Fatalf("account handle = %v", handle)
	}
}

func TestRestoreAdoptsNameOnlyAccountOnlyByStoredIdentity(t *testing.T) {
	for _, test := range []struct {
		name     string
		selected string
		listed   []string
		adopt    bool
	}{
		{name: "same Google account", selected: "DS-PERSONAL", listed: []string{"DS-PERSONAL"}, adopt: true},
		{name: "different Google account", selected: "DS-WORK", listed: []string{"DS-WORK"}, adopt: false},
		{name: "browser signed in to both, using the other", selected: "DS-WORK", listed: []string{"DS-WORK", "DS-PERSONAL"}, adopt: false},
		{name: "browser signed in to both, using this one", selected: "DS-PERSONAL", listed: []string{"DS-WORK", "DS-PERSONAL"}, adopt: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := browsercookies.Store{Kind: browsercookies.Gecko, Path: "name-only"}
			source := youtubeStore(store.Path, "SAPISID", "browser")
			kv := signedInKV("SAPISID=stored", `{"signedIn":true,"name":"Sam"}`)
			kv[identityKey] = selectionKeyFor("DS-PERSONAL")
			saveTestCookieSource(t, kv, browsercookies.SessionSource{Kind: store.Kind, Path: store.Path})
			httpClient := routeYouTube(func(endpoint, cookie string) *http.Response {
				switch endpoint {
				case "account/account_menu":
					return accountMenu("Sam", "")
				case "account/accounts_list":
					if strings.Contains(cookie, "browser") {
						return accountsList(test.selected, test.listed...)
					}
					return accountsList("DS-PERSONAL", "DS-PERSONAL")
				}
				return jsonResponse(http.StatusOK, `{}`)
			})
			client := newSessionTestClient(t, kv, source, httpClient, nil)
			defer client.Close()

			changed, err := client.refreshFromSource(context.Background(), "")
			if test.adopt && (err != nil || !changed || !strings.Contains(kv[cookieKey], "browser")) {
				t.Fatalf("same identity: changed=%v err=%v", changed, err)
			}
			if !test.adopt && (!errors.Is(err, errDifferentBrowserAccount) || kv[cookieKey] != "SAPISID=stored") {
				t.Fatalf("different identity: changed=%v err=%v", changed, err)
			}
		})
	}
}

func TestSignInStoresOnlyTheServerSelectedIdentity(t *testing.T) {
	for _, test := range []struct {
		name     string
		selected string
		want     string
	}{
		{name: "selected row", selected: "DS-ONE", want: selectionKeyFor("DS-ONE")},
		{name: "no row selected", selected: "", want: ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := browsercookies.Store{Kind: browsercookies.Gecko, Path: "importing"}
			source := youtubeStore(store.Path, "SAPISID", "new")
			kv := provider.MapKV{visitorKey: "visitor"}
			httpClient := routeYouTube(func(endpoint, _ string) *http.Response {
				switch endpoint {
				case "account/account_menu":
					return accountMenu("Sam", "")
				case "account/accounts_list":
					return accountsList(test.selected, "DS-ONE")
				}
				return jsonResponse(http.StatusOK, `{}`)
			})
			client := newSessionTestClient(t, kv, source, httpClient, nil)
			defer client.Close()

			flow, err := client.SignIn(context.Background())
			if err != nil || flow.URL != "" {
				t.Fatalf("sign in = %#v, %v", flow, err)
			}
			if got := kv[identityKey]; got != test.want {
				t.Fatalf("stored identity = %q, want %q", got, test.want)
			}
		})
	}
}

func TestSignOutDuringReAdoptionProofStaysSignedOut(t *testing.T) {
	store := browsercookies.Store{Kind: browsercookies.Gecko, Path: "proving"}
	source := youtubeStore(store.Path, "SAPISID", "fresh")
	kv := newLockedKV(signedInKV("SAPISID=stale", `{"signedIn":true,"name":"Tester","handle":"@tester"}`))
	raw, err := json.Marshal(browsercookies.SessionSource{Kind: store.Kind, Path: store.Path})
	if err != nil {
		t.Fatal(err)
	}
	if err := kv.Set(cookieSourceKey, string(raw)); err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	httpClient := routeYouTube(func(endpoint, cookie string) *http.Response {
		if endpoint == "account/account_menu" && strings.Contains(cookie, "fresh") {
			once.Do(func() { close(entered) })
			<-release
		}
		if endpoint == "account/account_menu" {
			return accountResponse()
		}
		return jsonResponse(http.StatusOK, `{}`)
	})
	events := &accountEvents{}
	client := newSessionTestClient(t, kv, source, httpClient, events)
	defer client.Close()

	done := make(chan error, 1)
	go func() { done <- client.Restore(context.Background()) }()
	<-entered
	if err := client.SignOut(); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatalf("restore = %v", err)
	}
	if _, ok := kv.Get(cookieKey); ok || client.CurrentAccount().SignedIn {
		t.Fatal("a re-adoption finishing after sign out stored the session again")
	}
	for _, account := range events.all() {
		if account.SignedIn {
			t.Fatalf("a signed-in account was announced after sign out: %#v", events.all())
		}
	}
}

func TestTwoCallersRejectedEverywhereBothReportTheRejection(t *testing.T) {
	store := browsercookies.Store{Kind: browsercookies.Gecko, Path: "dead-racing"}
	source := youtubeStore(store.Path, "SAPISID", "dead")
	kv := newLockedKV(signedInKV("SAPISID=dead", `{"signedIn":true,"name":"Tester","handle":"@tester"}`))
	var staleSent sync.WaitGroup
	staleSent.Add(2)
	var mu sync.Mutex
	withoutCookie := 0
	httpClient := routeYouTube(func(endpoint, cookie string) *http.Response {
		if endpoint == "browse" {
			if cookie == "" {
				mu.Lock()
				withoutCookie++
				mu.Unlock()
			} else {
				staleSent.Done()
				staleSent.Wait()
			}
		}
		return jsonResponse(http.StatusUnauthorized, `{}`)
	})
	client := newSessionTestClient(t, kv, source, httpClient, nil)
	defer client.Close()

	errs := make(chan error, 2)
	for range 2 {
		go func() {
			_, err := client.do(context.Background(), "WEB_REMIX", "browse", map[string]any{}, true)
			errs <- err
		}()
	}
	for range 2 {
		err := <-errs
		if !definitiveSessionFailure(err) {
			t.Errorf("caller error = %v, want the rejection", err)
		} else if strings.Count(err.Error(), errSessionRejected.Error()) != 1 {
			t.Errorf("rejection reads %q", err.Error())
		}
	}
	if withoutCookie != 0 {
		t.Fatalf("%d requests were re-sent without a cookie", withoutCookie)
	}
}

func TestForbiddenForAStaleChannelDropsTheChannelNotTheSession(t *testing.T) {
	store := browsercookies.Store{Kind: browsercookies.Gecko, Path: "brand"}
	source := youtubeStore(store.Path, "SAPISID", "same")
	kv := signedInKV("SAPISID=same", `{"signedIn":true,"name":"Brand","handle":"@brand"}`)
	kv[dataSyncKey] = "DS-BRAND-GONE"
	saveTestCookieSource(t, kv, browsercookies.SessionSource{Kind: store.Kind, Path: store.Path})
	httpClient := routeYouTubeRequest(func(endpoint, _, body string) *http.Response {
		if strings.Contains(body, "DS-BRAND-GONE") {
			return jsonResponse(http.StatusForbidden, `{}`)
		}
		if endpoint == "account/account_menu" {
			return accountMenu("Tester", "@tester")
		}
		return jsonResponse(http.StatusOK, `{}`)
	})
	events := &accountEvents{}
	client := newSessionTestClient(t, kv, source, httpClient, events)
	defer client.Close()

	if _, err := client.do(context.Background(), "WEB_REMIX", "browse", map[string]any{}, true); err != nil {
		t.Fatalf("browse after dropping the stale channel = %v", err)
	}
	if kv[cookieKey] != "SAPISID=same" {
		t.Fatal("a 403 for a stale channel erased the session")
	}
	if _, ok := kv[dataSyncKey]; ok {
		t.Fatal("the refused channel is still remembered")
	}
	if got := events.all(); len(got) != 1 || !got[0].SignedIn || got[0].Handle == nil || *got[0].Handle != "@tester" {
		t.Fatalf("events = %#v, want the account itself announced", got)
	}
}

func TestRestoreKeepsWorkingSessionWhenBrowserHoldsAnotherAccount(t *testing.T) {
	store := browsercookies.Store{Kind: browsercookies.Gecko, Path: "other-account"}
	source := youtubeStore(store.Path, "SAPISID", "other")
	kv := signedInKV("SAPISID=mine", `{"signedIn":true,"name":"Tester","handle":"@tester"}`)
	saveTestCookieSource(t, kv, browsercookies.SessionSource{Kind: store.Kind, Path: store.Path})
	httpClient := routeYouTube(func(endpoint, cookie string) *http.Response {
		if endpoint != "account/account_menu" {
			return jsonResponse(http.StatusOK, `{}`)
		}
		if strings.Contains(cookie, "other") {
			return accountMenu("Other", "@other")
		}
		return accountResponse()
	})
	client := newSessionTestClient(t, kv, source, httpClient, nil)
	defer client.Close()

	if err := client.Restore(context.Background()); err != nil {
		t.Fatalf("restore = %v, want the stored session confirmed", err)
	}
	if got := client.CurrentAccount(); !got.SignedIn || got.Handle == nil || *got.Handle != "@tester" {
		t.Fatalf("account = %#v", got)
	}
	if kv[cookieKey] != "SAPISID=mine" {
		t.Fatal("the stored cookie was replaced")
	}
}

func TestRejectedBrowserSessionFallsBackToSignInFlow(t *testing.T) {
	store := browsercookies.Store{Kind: browsercookies.Gecko, Path: "signed-out-zen"}
	source := &fakeCookieSource{stores: []browsercookies.Store{store}, cookies: map[string][]browsercookies.Cookie{}}
	kv := signedInKV("SAPISID=dead", `{"signedIn":true,"name":"Tester","handle":"@tester"}`)
	saveTestCookieSource(t, kv, browsercookies.SessionSource{Kind: store.Kind, Path: store.Path})
	httpClient := routeYouTube(func(string, string) *http.Response { return jsonResponse(http.StatusOK, `{}`) })
	client := newSessionTestClient(t, kv, source, httpClient, nil)
	defer client.Close()

	account, err := client.Account(context.Background())
	if err == nil || account.SignedIn {
		t.Fatalf("rejected account = %#v, %v", account, err)
	}
	flow, err := client.SignIn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if flow.URL != youtubeSignInURL || flow.Done == nil {
		t.Fatalf("sign-in flow = %#v", flow)
	}
}

func TestMissingRememberedStoreFallsThroughToSignInFlow(t *testing.T) {
	gone := browsercookies.Store{Kind: browsercookies.Gecko, Path: "/deleted-profile/cookies.sqlite"}
	source := &fakeCookieSource{cookies: map[string][]browsercookies.Cookie{}, missing: map[string]bool{gone.Path: true}}
	kv := signedInKV("SAPISID=dead", `{"signedIn":true,"name":"Tester","handle":"@tester"}`)
	saveTestCookieSource(t, kv, browsercookies.SessionSource{Kind: gone.Kind, Path: gone.Path})
	httpClient := routeYouTube(func(string, string) *http.Response { return jsonResponse(http.StatusUnauthorized, `{}`) })
	client := newSessionTestClient(t, kv, source, httpClient, nil)
	defer client.Close()

	if _, err := client.do(context.Background(), "WEB_REMIX", "browse", map[string]any{}, true); !definitiveSessionFailure(err) {
		t.Fatalf("browse error = %v, want a definitive rejection", err)
	}
	if _, ok := kv[cookieKey]; ok {
		t.Fatal("a session both YouTube and the missing browser store reject was kept")
	}
	flow, err := client.SignIn(context.Background())
	if err != nil || flow.URL != youtubeSignInURL {
		t.Fatalf("sign in = %#v, %v, want the import flow", flow, err)
	}
}

func TestUnreadableRememberedStoreSignInStillRunsImportFlow(t *testing.T) {
	store := browsercookies.Store{Kind: browsercookies.Gecko, Path: "locked"}
	source := &unreadableCookies{}
	kv := signedInKV("SAPISID=dead", `{"signedIn":true,"name":"Tester","handle":"@tester"}`)
	saveTestCookieSource(t, kv, browsercookies.SessionSource{Kind: store.Kind, Path: store.Path})
	httpClient := routeYouTube(func(string, string) *http.Response { return jsonResponse(http.StatusOK, `{}`) })
	client := newSessionTestClient(t, kv, source, httpClient, nil)
	defer client.Close()

	flow, err := client.SignIn(context.Background())
	if err != nil || flow.URL != youtubeSignInURL {
		t.Fatalf("sign in = %#v, %v, want the import flow", flow, err)
	}
}

func TestAuthenticatedRequestRefreshesAndRetriesOnce(t *testing.T) {
	store := browsercookies.Store{Kind: browsercookies.Gecko, Path: "retry-zen"}
	source := youtubeStore(store.Path, "SAPISID", "fresh")
	kv := signedInKV("SAPISID=stale", `{"signedIn":true,"name":"Tester","handle":"@tester"}`)
	saveTestCookieSource(t, kv, browsercookies.SessionSource{Kind: store.Kind, Path: store.Path})
	var mu sync.Mutex
	var browseHeaders []string
	httpClient := routeYouTube(func(endpoint, cookie string) *http.Response {
		switch endpoint {
		case "account/account_menu":
			return accountResponse()
		case "browse":
			mu.Lock()
			browseHeaders = append(browseHeaders, cookie)
			mu.Unlock()
			if strings.Contains(cookie, "stale") {
				return jsonResponse(http.StatusUnauthorized, `{}`)
			}
		}
		return jsonResponse(http.StatusOK, `{}`)
	})
	client := newSessionTestClient(t, kv, source, httpClient, nil)
	defer client.Close()

	if _, err := client.do(context.Background(), "WEB_REMIX", "browse", map[string]any{}, true); err != nil {
		t.Fatal(err)
	}
	if len(browseHeaders) != 2 || !strings.Contains(browseHeaders[0], "stale") || !strings.Contains(browseHeaders[1], "fresh") {
		t.Fatal("browse cookie sequence does not show one refresh and retry")
	}
}

func TestConcurrentRejectionsKeepTheReAdoptedSession(t *testing.T) {
	store := browsercookies.Store{Kind: browsercookies.Gecko, Path: "racing-zen"}
	source := youtubeStore(store.Path, "SAPISID", "fresh")
	kv := newLockedKV(signedInKV("SAPISID=stale", `{"signedIn":true,"name":"Tester","handle":"@tester"}`))
	raw, err := json.Marshal(browsercookies.SessionSource{Kind: store.Kind, Path: store.Path})
	if err != nil {
		t.Fatal(err)
	}
	if err := kv.Set(cookieSourceKey, string(raw)); err != nil {
		t.Fatal(err)
	}
	var staleSent sync.WaitGroup
	staleSent.Add(2)
	httpClient := routeYouTube(func(endpoint, cookie string) *http.Response {
		switch endpoint {
		case "account/account_menu":
			return accountResponse()
		case "browse":
			if strings.Contains(cookie, "stale") {
				staleSent.Done()
				staleSent.Wait()
				return jsonResponse(http.StatusUnauthorized, `{}`)
			}
		}
		return jsonResponse(http.StatusOK, `{}`)
	})
	client := newSessionTestClient(t, kv, source, httpClient, nil)
	defer client.Close()

	errs := make(chan error, 2)
	for range 2 {
		go func() {
			_, err := client.do(context.Background(), "WEB_REMIX", "browse", map[string]any{}, true)
			errs <- err
		}()
	}
	for range 2 {
		if err := <-errs; err != nil {
			t.Errorf("request rejected together with another = %v", err)
		}
	}
	if cookie, ok := kv.Get(cookieKey); !ok || !strings.Contains(cookie, "fresh") {
		t.Fatal("the second rejection erased the session the first re-adopted")
	}
}

func TestForbiddenWithUnchangedStoreKeepsAcceptedSession(t *testing.T) {
	store := browsercookies.Store{Kind: browsercookies.Gecko, Path: "same-zen"}
	source := youtubeStore(store.Path, "SAPISID", "same")
	kv := signedInKV("SAPISID=same", `{"signedIn":true,"name":"Tester","handle":"@tester"}`)
	saveTestCookieSource(t, kv, browsercookies.SessionSource{Kind: store.Kind, Path: store.Path})
	httpClient := routeYouTube(func(endpoint, _ string) *http.Response {
		switch endpoint {
		case "account/account_menu":
			return accountResponse()
		case "playlist/delete":
			return jsonResponse(http.StatusForbidden, `{"error":{"status":"PERMISSION_DENIED"}}`)
		}
		return jsonResponse(http.StatusOK, `{}`)
	})
	events := &accountEvents{}
	client := newSessionTestClient(t, kv, source, httpClient, events)
	defer client.Close()

	_, err := client.do(context.Background(), "WEB_REMIX", "playlist/delete", map[string]any{"playlistId": "PL1"}, true)
	if err == nil || definitiveSessionFailure(err) {
		t.Fatalf("delete error = %v, want a plain 403", err)
	}
	if kv[cookieKey] != "SAPISID=same" || !client.CurrentAccount().SignedIn {
		t.Fatal("a 403 on one endpoint erased a session the account menu accepts")
	}
	if got := events.all(); len(got) != 0 {
		t.Fatalf("events = %#v", got)
	}
}

func TestRejectionEverywhereSignsOutAndAnnouncesIt(t *testing.T) {
	store := browsercookies.Store{Kind: browsercookies.Gecko, Path: "dead-zen"}
	source := youtubeStore(store.Path, "SAPISID", "dead")
	kv := signedInKV("SAPISID=dead", `{"signedIn":true,"name":"Tester","handle":"@tester"}`)
	saveTestCookieSource(t, kv, browsercookies.SessionSource{Kind: store.Kind, Path: store.Path})
	httpClient := routeYouTube(func(string, string) *http.Response { return jsonResponse(http.StatusUnauthorized, `{}`) })
	events := &accountEvents{}
	client := newSessionTestClient(t, kv, source, httpClient, events)
	defer client.Close()

	if _, err := client.do(context.Background(), "WEB_REMIX", "browse", map[string]any{}, true); !definitiveSessionFailure(err) {
		t.Fatalf("browse error = %v", err)
	}
	if _, ok := kv[cookieKey]; ok || client.CurrentAccount().SignedIn {
		t.Fatal("a session YouTube rejects everywhere was kept")
	}
	if got := events.all(); len(got) != 1 || got[0].SignedIn {
		t.Fatalf("events = %#v, want one signed-out account", got)
	}
	if _, ok := kv[cookieSourceKey]; !ok {
		t.Fatal("the remembered source was dropped; the next sign in must follow the same browser")
	}
}

func TestLegacySessionFindsAndPersistsBrowserSource(t *testing.T) {
	store := browsercookies.Store{Kind: browsercookies.Gecko, Path: "legacy-zen"}
	source := youtubeStore(store.Path, "SAPISID", "shared", "SID", "rotated")
	kv := signedInKV("SAPISID=shared; SID=old", `{"signedIn":true,"name":"Tester","handle":"@tester"}`)
	httpClient := routeYouTube(func(endpoint, _ string) *http.Response {
		if endpoint == "account/account_menu" {
			return accountResponse()
		}
		return jsonResponse(http.StatusOK, `{}`)
	})
	client := newSessionTestClient(t, kv, source, httpClient, nil)
	defer client.Close()

	if err := client.Restore(context.Background()); err != nil {
		t.Fatal(err)
	}
	var saved browsercookies.SessionSource
	if err := json.Unmarshal([]byte(kv[cookieSourceKey]), &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Path != store.Path || saved.Kind != store.Kind || !strings.Contains(kv[cookieKey], "rotated") {
		t.Fatalf("saved source = %#v", saved)
	}
}

func TestLegacyLookupMissWaitsForAStoreChange(t *testing.T) {
	store := browsercookies.Store{Kind: browsercookies.Gecko, Path: "unrelated"}
	source := youtubeStore(store.Path, "SAPISID", "someone-else")
	kv := signedInKV("SAPISID=mine", `{"signedIn":true,"name":"Tester","handle":"@tester"}`)
	client := newSessionTestClient(t, kv, source, routeYouTube(func(string, string) *http.Response {
		return jsonResponse(http.StatusOK, `{}`)
	}), nil)
	defer client.Close()

	for range 3 {
		if _, err := client.refreshFromSource(context.Background(), ""); !errors.Is(err, errNoBrowserSession) {
			t.Fatalf("legacy lookup error = %v", err)
		}
	}
	if source.discovers != 1 {
		t.Fatalf("Discover ran %d times with no store change, want 1", source.discovers)
	}
	source.mu.Lock()
	source.changed = true
	source.mu.Unlock()
	if _, err := client.refreshFromSource(context.Background(), ""); !errors.Is(err, errNoBrowserSession) {
		t.Fatalf("legacy lookup error = %v", err)
	}
	if source.discovers != 2 {
		t.Fatalf("Discover ran %d times after a store change, want 2", source.discovers)
	}
}

func TestOwnerLegacyShapeReAdoptsRotatedZenStore(t *testing.T) {
	home := t.TempDir()
	config := filepath.Join(home, ".config")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", config)
	storePath := filepath.Join(config, "zen", "li238x8k.Default (release)-1", "cookies.sqlite")
	writeZenStore(t, storePath, map[string]string{
		"SAPISID": "shared-sapisid", "__Secure-3PAPISID": "shared-sapisid",
		"SID": "rotated-sid", "__Secure-1PSID": "rotated-1psid", "__Secure-3PSID": "rotated-3psid",
	})
	kv := signedInKV(
		"SAPISID=shared-sapisid; SID=old-sid; __Secure-1PSID=old-1psid; __Secure-3PAPISID=shared-sapisid; __Secure-3PSID=old-3psid",
		`{"signedIn":true,"name":"Owner","handle":"@owner","email":null,"channelId":null}`,
	)
	kv[cookieSourceKey] = `{"kind":0,"path":` + jsonString(t, storePath) + `}`
	httpClient := routeYouTube(func(endpoint, _ string) *http.Response {
		if endpoint == "account/account_menu" {
			return accountMenu("Owner", "@owner")
		}
		return jsonResponse(http.StatusOK, `{}`)
	})

	client := newSessionTestClient(t, kv, systemCookies{}, httpClient, nil)
	if err := client.Restore(context.Background()); err != nil {
		t.Fatal(err)
	}
	client.Close()
	for _, rotated := range []string{"rotated-sid", "rotated-1psid", "rotated-3psid"} {
		if !strings.Contains(kv[cookieKey], rotated) {
			t.Fatalf("rotated cookie %s was not adopted", rotated)
		}
	}
	if !strings.Contains(kv[cookieSourceKey], `"kind":"gecko"`) {
		t.Fatalf("source was not rewritten with a named kind: %s", kv[cookieSourceKey])
	}

	restarted := newSessionTestClient(t, kv, systemCookies{}, httpClient, nil)
	defer restarted.Close()
	if err := restarted.Restore(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := restarted.CurrentAccount(); !got.SignedIn || got.Handle == nil || *got.Handle != "@owner" {
		t.Fatalf("restarted account = %#v", got)
	}
}

func TestZenCookieStoreWithoutRememberedSourceReAdopts(t *testing.T) {
	home := t.TempDir()
	config := filepath.Join(home, ".config")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", config)
	storePath := filepath.Join(config, "zen", "li238x8k.Default (release)-1", "cookies.sqlite")
	writeZenStore(t, storePath, map[string]string{"SAPISID": "shared", "SID": "rotated"})
	kv := signedInKV("SAPISID=shared; SID=stale", `{"signedIn":true,"name":"Tester","handle":"@tester"}`)
	httpClient := routeYouTube(func(endpoint, _ string) *http.Response {
		if endpoint == "account/account_menu" {
			return accountResponse()
		}
		return jsonResponse(http.StatusOK, `{}`)
	})

	client := newSessionTestClient(t, kv, systemCookies{}, httpClient, nil)
	defer client.Close()
	if err := client.Restore(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(kv[cookieKey], "rotated") {
		t.Fatal("Zen cookie rotation was not persisted")
	}
}

func TestSymlinkedRememberedSourceIsNotRead(t *testing.T) {
	home := t.TempDir()
	config := filepath.Join(home, ".config")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", config)
	outside := filepath.Join(t.TempDir(), "cookies.sqlite")
	writeZenStore(t, outside, map[string]string{"SAPISID": "planted"})
	linked := filepath.Join(config, "zen", "profile", "cookies.sqlite")
	if err := os.MkdirAll(filepath.Dir(linked), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, linked); err != nil {
		t.Fatal(err)
	}
	kv := signedInKV("SAPISID=mine", `{"signedIn":true,"name":"Tester","handle":"@tester"}`)
	kv[cookieSourceKey] = `{"kind":"gecko","path":` + jsonString(t, linked) + `}`
	httpClient := routeYouTube(func(endpoint, _ string) *http.Response {
		if endpoint == "account/account_menu" {
			return accountResponse()
		}
		return jsonResponse(http.StatusOK, `{}`)
	})
	client := newSessionTestClient(t, kv, systemCookies{}, httpClient, nil)
	defer client.Close()

	if _, err := client.refreshFromSource(context.Background(), ""); !errors.Is(err, errNoBrowserSession) || !errors.Is(err, browsercookies.ErrInvalidSource) {
		t.Fatalf("symlinked source error = %v", err)
	}
	if kv[cookieKey] != "SAPISID=mine" {
		t.Fatal("cookies behind a symlinked source were adopted")
	}
}

type accountEvents struct {
	mu       sync.Mutex
	accounts []model.Account
}

func (e *accountEvents) Emit(event string, data any) {
	if event != protocol.EventAuthChanged {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.accounts = append(e.accounts, data.(model.Account))
}

func (e *accountEvents) all() []model.Account {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]model.Account(nil), e.accounts...)
}

type unreadableCookies struct{}

func (unreadableCookies) Discover() []browsercookies.Store { return nil }
func (unreadableCookies) Resolve(source browsercookies.SessionSource) (browsercookies.Store, error) {
	return browsercookies.Store{Kind: source.Kind, Path: source.Path}, nil
}
func (unreadableCookies) Read(browsercookies.Store, ...string) ([]browsercookies.Cookie, error) {
	return nil, errors.New("file is not a database")
}
func (unreadableCookies) ChangedSince(browsercookies.Store, time.Time) bool { return false }

// lockedKV is a provider.KV safe for the concurrent requests a race test makes.
type lockedKV struct {
	mu sync.Mutex
	kv provider.MapKV
}

func newLockedKV(kv provider.MapKV) *lockedKV { return &lockedKV{kv: kv} }

func (l *lockedKV) Get(key string) (string, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.kv.Get(key)
}

func (l *lockedKV) Set(key, value string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.kv.Set(key, value)
}

func (l *lockedKV) Delete(key string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.kv.Delete(key)
}

func youtubeStore(path string, pairs ...string) *fakeCookieSource {
	var cookies []browsercookies.Cookie
	for i := 0; i+1 < len(pairs); i += 2 {
		cookies = append(cookies, browsercookies.Cookie{Name: pairs[i], Value: pairs[i+1], Host: ".youtube.com", Path: "/"})
	}
	return &fakeCookieSource{
		stores:  []browsercookies.Store{{Kind: browsercookies.Gecko, Path: path}},
		cookies: map[string][]browsercookies.Cookie{path: cookies},
	}
}

func signedInKV(cookie, account string) provider.MapKV {
	return provider.MapKV{visitorKey: "visitor", cookieKey: cookie, accountKey: account}
}

func routeYouTube(answer func(endpoint, cookie string) *http.Response) *http.Client {
	return routeYouTubeRequest(func(endpoint, cookie, _ string) *http.Response { return answer(endpoint, cookie) })
}

func routeYouTubeRequest(answer func(endpoint, cookie, body string) *http.Response) *http.Client {
	return &http.Client{Transport: authRoundTripper(func(request *http.Request) (*http.Response, error) {
		endpoint := strings.TrimPrefix(request.URL.Path, "/youtubei/v1/")
		var body []byte
		if request.Body != nil {
			var err error
			if body, err = io.ReadAll(request.Body); err != nil {
				return nil, err
			}
		}
		return answer(endpoint, request.Header.Get("Cookie"), string(body)), nil
	})}
}

func newSessionTestClient(t *testing.T, kv provider.KV, source CookieSource, httpClient *http.Client, events *accountEvents) *Client {
	t.Helper()
	options := Options{
		KV:         kv,
		HTTPClient: httpClient,
		Log:        slog.New(slog.DiscardHandler),
		DataDir:    t.TempDir(),
		Cookies:    source,
	}
	if events != nil {
		options.Bus = events
	}
	client, err := New(options)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func newAuthTestClient(t *testing.T, source CookieSource, httpClient *http.Client, logger *slog.Logger) *Client {
	t.Helper()
	client, err := New(Options{
		KV:         provider.MapKV{"visitor_data": "visitor"},
		HTTPClient: httpClient,
		Log:        logger,
		DataDir:    t.TempDir(),
		Cookies:    source,
	})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func saveTestCookieSource(t *testing.T, kv provider.MapKV, source browsercookies.SessionSource) {
	t.Helper()
	raw, err := json.Marshal(source)
	if err != nil {
		t.Fatal(err)
	}
	kv[cookieSourceKey] = string(raw)
}

func writeZenStore(t *testing.T, path string, cookies map[string]string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE moz_cookies (
		name TEXT, value TEXT, host TEXT, path TEXT, expiry INTEGER, originAttributes TEXT
	)`); err != nil {
		t.Fatal(err)
	}
	expires := time.Now().Add(24 * time.Hour).Unix()
	for name, value := range cookies {
		if _, err := db.Exec(`INSERT INTO moz_cookies VALUES (?, ?, '.youtube.com', '/', ?, '')`, name, value, expires); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
}

func jsonString(t *testing.T, value string) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func selectionKeyFor(dataSync string) string {
	sum := sha256.Sum256([]byte(dataSync))
	return hex.EncodeToString(sum[:16])
}

func accountMenu(name, handle string) *http.Response {
	header := map[string]any{"accountName": map[string]any{"runs": []map[string]string{{"text": name}}}}
	if handle != "" {
		header["channelHandle"] = map[string]any{"runs": []map[string]string{{"text": handle}}}
	}
	body, _ := json.Marshal(map[string]any{"activeAccountHeaderRenderer": header})
	return jsonResponse(http.StatusOK, string(body))
}

func accountResponse() *http.Response { return accountMenu("Tester", "@tester") }

// accountsList answers accounts_list with one row per data sync id; the row
// matching selected carries the server's isSelected.
func accountsList(selected string, dataSyncs ...string) *http.Response {
	items := make([]map[string]any, 0, len(dataSyncs))
	for _, dataSync := range dataSyncs {
		items = append(items, map[string]any{"accountItem": map[string]any{
			"accountName": map[string]any{"simpleText": "Sam"}, "datasyncId": dataSync, "hasChannel": true,
			"isSelected": dataSync == selected,
		}})
	}
	body, _ := json.Marshal(map[string]any{"contents": items})
	return jsonResponse(http.StatusOK, string(body))
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     http.StatusText(status),
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
