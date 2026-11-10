package tidal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ryoku-dev/ryotunes/internal/bus"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
	baseprovider "github.com/ryoku-dev/ryotunes/internal/provider"
)

type lockedKV struct {
	mu     sync.Mutex
	values map[string]string
}

func newLockedKV(values map[string]string) *lockedKV {
	return &lockedKV{values: values}
}

func (k *lockedKV) Get(key string) (string, bool) {
	k.mu.Lock()
	defer k.mu.Unlock()
	value, ok := k.values[key]
	return value, ok
}

func (k *lockedKV) Set(key, value string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.values[key] = value
	return nil
}

func (k *lockedKV) Delete(key string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	delete(k.values, key)
	return nil
}

func storedAuth(t *testing.T, auth Auth) string {
	t.Helper()
	body, err := json.Marshal(auth)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func expiredAuth() Auth {
	refresh := "refresh-old"
	return Auth{
		AccessToken: "access-old", RefreshToken: &refresh, ExpiresAt: time.Now().Add(-time.Hour).Unix(),
		UserID: 42, CountryCode: "US", Entitlement: "standard",
	}
}

func TestTidalStatusRequiresReusableRefreshToken(t *testing.T) {
	accessOnly := Auth{AccessToken: "access", ExpiresAt: time.Now().Add(time.Hour).Unix()}
	kv := baseprovider.MapKV{tidalAuthKey: storedAuth(t, accessOnly)}
	provider, err := New(Options{KV: kv})
	if err != nil {
		t.Fatal(err)
	}
	if provider.Status().SignedIn {
		t.Fatal("access-token-only authorization was reported as signed in")
	}
	if _, ok := kv.Get(tidalAuthKey); ok {
		t.Fatal("access-token-only authorization was left in storage")
	}

	refresh := "refresh"
	reusable := Auth{RefreshToken: &refresh, ExpiresAt: 0}
	provider, err = New(Options{KV: baseprovider.MapKV{tidalAuthKey: storedAuth(t, reusable)}})
	if err != nil {
		t.Fatal(err)
	}
	if !provider.Status().SignedIn {
		t.Fatal("stored refresh token was not reported as signed in")
	}
}

func TestTidalRefreshIsSingleFlightAndPersistedBeforeUse(t *testing.T) {
	var tokenCalls atomic.Int32
	var persistedBeforeUse atomic.Bool
	kv := newLockedKV(map[string]string{tidalAuthKey: storedAuth(t, expiredAuth())})
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/token":
			tokenCalls.Add(1)
			response.Header().Set("Content-Type", "application/json")
			fmt.Fprint(response, `{"access_token":"access-new","refresh_token":"refresh-new","expires_in":3600}`)
		case "/resource":
			raw, ok := kv.Get(tidalAuthKey)
			var auth Auth
			if ok && json.Unmarshal([]byte(raw), &auth) == nil && auth.AccessToken == "access-new" && auth.RefreshToken != nil && *auth.RefreshToken == "refresh-new" {
				persistedBeforeUse.Store(true)
			}
			response.Header().Set("Content-Type", "application/json")
			fmt.Fprint(response, `{}`)
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()
	provider, err := New(Options{KV: kv, HTTPClient: server.Client(), APIBase: server.URL, TokenURL: server.URL + "/token"})
	if err != nil {
		t.Fatal(err)
	}

	var wait sync.WaitGroup
	errorsSeen := make(chan error, 8)
	for range 8 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			var result map[string]any
			errorsSeen <- provider.api(context.Background(), "/resource", nil, &result)
		}()
	}
	wait.Wait()
	close(errorsSeen)
	for err := range errorsSeen {
		if err != nil {
			t.Fatal(err)
		}
	}
	if got := tokenCalls.Load(); got != 1 {
		t.Fatalf("token refresh requests = %d, want 1", got)
	}
	if !persistedBeforeUse.Load() {
		t.Fatal("refreshed credentials were used before persistence")
	}
}

func TestTidalTransientRefreshFailureKeepsSession(t *testing.T) {
	for _, test := range []struct {
		name       string
		closeEarly bool
	}{
		{name: "server error"},
		{name: "network error", closeEarly: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			kv := newLockedKV(map[string]string{tidalAuthKey: storedAuth(t, expiredAuth())})
			events := make(chan model.TidalAuthEvent, 1)
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
				http.Error(response, "temporarily unavailable", http.StatusServiceUnavailable)
			}))
			client := server.Client()
			endpoint := server.URL + "/token"
			if test.closeEarly {
				server.Close()
			} else {
				defer server.Close()
			}
			provider, err := New(Options{
				KV: kv, HTTPClient: client, TokenURL: endpoint,
				Bus: bus.SinkFunc(func(event string, value any) {
					if event == protocol.EventTidalAuth {
						events <- value.(model.TidalAuthEvent)
					}
				}),
			})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := provider.ensureFresh(context.Background()); err == nil {
				t.Fatal("refresh unexpectedly succeeded")
			}
			if !provider.Status().SignedIn {
				t.Fatal("transient refresh failure signed the account out")
			}
			if _, ok := kv.Get(tidalAuthKey); !ok {
				t.Fatal("transient refresh failure deleted authorization")
			}
			select {
			case event := <-events:
				t.Fatalf("transient failure emitted auth event %#v", event)
			default:
			}
		})
	}
}

func TestTidalRejectedRefreshSignsOutWithReason(t *testing.T) {
	for _, test := range []struct {
		name   string
		status int
		body   string
	}{
		{name: "invalid grant", status: http.StatusBadRequest, body: `{"error":"invalid_grant","error_description":"refresh token expired"}`},
		{name: "access denied", status: http.StatusUnauthorized, body: `{"error":"access_denied"}`},
		{name: "invalid grant on other status", status: http.StatusForbidden, body: `{"error":"invalid_grant"}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			kv := newLockedKV(map[string]string{tidalAuthKey: storedAuth(t, expiredAuth())})
			events := make(chan model.TidalAuthEvent, 1)
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
				response.Header().Set("Content-Type", "application/json")
				response.WriteHeader(test.status)
				fmt.Fprint(response, test.body)
			}))
			defer server.Close()
			provider, err := New(Options{
				KV: kv, HTTPClient: server.Client(),
				Bus: bus.SinkFunc(func(event string, value any) {
					if event == protocol.EventTidalAuth {
						events <- value.(model.TidalAuthEvent)
					}
				}),
				TokenURL: server.URL,
			})
			if err != nil {
				t.Fatal(err)
			}
			_, err = provider.ensureFresh(context.Background())
			if !errors.Is(err, baseprovider.ErrSignInRequired) {
				t.Fatalf("refresh error = %v", err)
			}
			if provider.Status().SignedIn {
				t.Fatal("rejected refresh left the account signed in")
			}
			if _, ok := kv.Get(tidalAuthKey); ok {
				t.Fatal("rejected refresh kept authorization")
			}
			event := <-events
			if event.State != "session_expired" || event.Message == nil || *event.Message == "" {
				t.Fatalf("reason event = %#v", event)
			}
		})
	}
}

func TestTidalRefusedClientKeepsRefreshToken(t *testing.T) {
	for _, test := range []struct {
		name   string
		status int
		body   string
	}{
		{name: "invalid client", status: http.StatusUnauthorized, body: `{"error":"invalid_client","error_description":"Client not found"}`},
		{name: "unauthorized client", status: http.StatusBadRequest, body: `{"error":"unauthorized_client"}`},
		{name: "no error code", status: http.StatusBadRequest, body: `bad request`},
	} {
		t.Run(test.name, func(t *testing.T) {
			kv := newLockedKV(map[string]string{tidalAuthKey: storedAuth(t, expiredAuth())})
			var emitted atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
				response.WriteHeader(test.status)
				fmt.Fprint(response, test.body)
			}))
			defer server.Close()
			provider, err := New(Options{
				KV: kv, HTTPClient: server.Client(), TokenURL: server.URL,
				Bus: bus.SinkFunc(func(event string, _ any) {
					if event == protocol.EventTidalAuth {
						emitted.Add(1)
					}
				}),
			})
			if err != nil {
				t.Fatal(err)
			}
			_, err = provider.ensureFresh(context.Background())
			if err == nil || errors.Is(err, baseprovider.ErrSignInRequired) {
				t.Fatalf("refresh error = %v, want a non sign-in error", err)
			}
			if !provider.Status().SignedIn {
				t.Fatal("refused client signed the account out")
			}
			raw, ok := kv.Get(tidalAuthKey)
			var kept Auth
			if !ok || json.Unmarshal([]byte(raw), &kept) != nil || kept.RefreshToken == nil || *kept.RefreshToken != "refresh-old" {
				t.Fatal("refused client deleted the refresh token")
			}
			if emitted.Load() != 0 {
				t.Fatal("refused client emitted an auth event")
			}
		})
	}
}

func TestTidalAPIUnauthorizedRefreshesOnlyForTokenSubStatus(t *testing.T) {
	for _, test := range []struct {
		name        string
		body        string
		wantRefresh bool
	}{
		{name: "expired token", body: `{"status":401,"subStatus":11003,"userMessage":"The token has expired."}`, wantRefresh: true},
		{name: "no body", body: ``, wantRefresh: true},
		{name: "not a token problem", body: `{"status":401,"subStatus":4005,"userMessage":"Asset is not ready for playback"}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			var tokenCalls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
				switch request.URL.Path {
				case "/token":
					tokenCalls.Add(1)
					response.Header().Set("Content-Type", "application/json")
					fmt.Fprint(response, `{"access_token":"access-new","refresh_token":"refresh-new","expires_in":3600}`)
				case "/resource":
					if request.Header.Get("Authorization") == "Bearer access-old" {
						response.WriteHeader(http.StatusUnauthorized)
						fmt.Fprint(response, test.body)
						return
					}
					response.Header().Set("Content-Type", "application/json")
					fmt.Fprint(response, `{}`)
				default:
					http.NotFound(response, request)
				}
			}))
			defer server.Close()
			auth := expiredAuth()
			auth.ExpiresAt = time.Now().Add(time.Hour).Unix()
			kv := newLockedKV(map[string]string{tidalAuthKey: storedAuth(t, auth)})
			provider, err := New(Options{KV: kv, HTTPClient: server.Client(), APIBase: server.URL, TokenURL: server.URL + "/token"})
			if err != nil {
				t.Fatal(err)
			}
			var result map[string]any
			err = provider.api(context.Background(), "/resource", nil, &result)
			if test.wantRefresh {
				if err != nil {
					t.Fatal(err)
				}
				if tokenCalls.Load() != 1 {
					t.Fatalf("token calls = %d, want 1", tokenCalls.Load())
				}
			} else {
				if err == nil {
					t.Fatal("non-token 401 unexpectedly succeeded")
				}
				if tokenCalls.Load() != 0 {
					t.Fatalf("non-token 401 forced %d token refreshes", tokenCalls.Load())
				}
			}
			if !provider.Status().SignedIn {
				t.Fatal("API 401 signed the account out")
			}
		})
	}
}

func TestTidalLoginWithoutRefreshTokenIsNotPersisted(t *testing.T) {
	kv := baseprovider.MapKV{}
	provider, err := New(Options{KV: kv})
	if err != nil {
		t.Fatal(err)
	}
	if err := provider.completeSignIn(context.Background(), Auth{AccessToken: "access"}, provider.epoch); err == nil {
		t.Fatal("access-token-only login unexpectedly succeeded")
	}
	if _, ok := kv.Get(tidalAuthKey); ok {
		t.Fatal("access-token-only login was persisted")
	}
	if provider.Status().SignedIn {
		t.Fatal("access-token-only login was installed")
	}
}

func TestTidalRefreshWithEmptyRefreshTokenKeepsPriorToken(t *testing.T) {
	kv := newLockedKV(map[string]string{tidalAuthKey: storedAuth(t, expiredAuth())})
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		fmt.Fprint(response, `{"access_token":"access-new","refresh_token":" ","expires_in":3600}`)
	}))
	defer server.Close()
	provider, err := New(Options{KV: kv, HTTPClient: server.Client(), TokenURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	auth, err := provider.ensureFresh(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if auth.RefreshToken == nil || *auth.RefreshToken != "refresh-old" {
		t.Fatal("blank refresh_token replaced the prior refresh token")
	}
	raw, ok := kv.Get(tidalAuthKey)
	var stored Auth
	if !ok || json.Unmarshal([]byte(raw), &stored) != nil || stored.RefreshToken == nil || *stored.RefreshToken != "refresh-old" {
		t.Fatal("persisted authorization lost the prior refresh token")
	}
}

func TestTidalRenewedTokenRefusedIsNotRefreshedOnEveryCall(t *testing.T) {
	var tokenCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/token" {
			n := tokenCalls.Add(1)
			response.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(response, `{"access_token":"access-%d","refresh_token":"refresh-new","expires_in":3600}`, n)
			return
		}
		response.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(response, `{"status":401,"subStatus":11003,"userMessage":"The token has expired."}`)
	}))
	defer server.Close()
	auth := expiredAuth()
	auth.ExpiresAt = time.Now().Add(time.Hour).Unix()
	provider, err := New(Options{
		KV: newLockedKV(map[string]string{tidalAuthKey: storedAuth(t, auth)}), HTTPClient: server.Client(),
		APIBase: server.URL, TokenURL: server.URL + "/token",
	})
	if err != nil {
		t.Fatal(err)
	}
	for range 4 {
		var result map[string]any
		if err := provider.api(context.Background(), "/resource", nil, &result); err == nil {
			t.Fatal("refused request unexpectedly succeeded")
		}
	}
	if got := tokenCalls.Load(); got != 1 {
		t.Fatalf("token refreshes over 4 refused calls = %d, want 1", got)
	}
	if !provider.Status().SignedIn {
		t.Fatal("refused API calls signed the account out")
	}
}
