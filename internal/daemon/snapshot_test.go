package daemon

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/ryoku-dev/ryotunes/internal/bus"
	"github.com/ryoku-dev/ryotunes/internal/provider"
	"github.com/ryoku-dev/ryotunes/internal/provider/soundcloud"
	"github.com/ryoku-dev/ryotunes/internal/provider/youtube"
)

type blackHole struct{}

func (blackHole) RoundTrip(request *http.Request) (*http.Response, error) {
	<-request.Context().Done()
	return nil, request.Context().Err()
}

func TestSnapshotAnswersFromMemoryOnABlackHoledNetwork(t *testing.T) {
	client := &http.Client{Transport: blackHole{}}
	kv := provider.MapKV{
		"visitor_data":   "visitor",
		"session_cookie": "SAPISID=stored",
		"account_json":   `{"signedIn":true,"name":"Tester","handle":"@tester"}`,
	}
	yt, err := youtube.New(youtube.Options{KV: kv, HTTPClient: client, Log: slog.New(slog.DiscardHandler), DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer yt.Close()
	payload, err := json.Marshal(map[string]any{"sub": "42", "exp": time.Now().Add(-time.Hour).Unix()})
	if err != nil {
		t.Fatal(err)
	}
	expired := "header." + base64.RawURLEncoding.EncodeToString(payload) + ".signature"
	auth, err := json.Marshal(soundcloud.Auth{AccessToken: expired, ExpiresAt: time.Now().Add(-time.Hour).Unix()})
	if err != nil {
		t.Fatal(err)
	}
	sc, err := soundcloud.New(soundcloud.Options{KV: provider.MapKV{"soundcloud_auth": string(auth)}, HTTPClient: client, APIBase: "https://api.invalid"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	app := &App{YouTube: yt, SoundCloud: sc, Bus: &bus.Fanout{}, ctx: ctx}

	started := time.Now()
	snapshot := app.Snapshot()
	if elapsed := time.Since(started); elapsed > 100*time.Millisecond {
		t.Fatalf("Snapshot took %v on a black-holed network", elapsed)
	}
	if !snapshot.Auth.SignedIn || snapshot.Auth.Handle == nil || *snapshot.Auth.Handle != "@tester" {
		t.Fatalf("offline snapshot dropped the cached YouTube account: %#v", snapshot.Auth)
	}
}
