package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"testing"

	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

func TestUserMessageMapsURLErrorsByService(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{name: "YouTube Music", url: "https://music.youtube.com/youtubei/v1/account/account_menu", want: "Couldn't reach YouTube Music. Check your connection and try again."},
		{name: "Spotify", url: "https://api.spotify.com/v1/me", want: "Couldn't reach Spotify. Check your connection and try again."},
		{name: "SoundCloud", url: "https://api-v2.soundcloud.com/me", want: "Couldn't reach SoundCloud. Check your connection and try again."},
		{name: "TIDAL", url: "https://api.tidal.com/v1/users/me", want: "Couldn't reach TIDAL. Check your connection and try again."},
		{name: "other", url: "https://example.com/api", want: "Couldn't reach the server. Check your connection and try again."},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := fmt.Errorf("provider request: %w", &url.Error{Op: "Post", URL: test.url, Err: errors.New("network down")})
			if got := userMessage(err); got != test.want {
				t.Fatalf("message = %q, want %q", got, test.want)
			}
		})
	}
}

func TestUserMessageMapsURLTimeout(t *testing.T) {
	err := &url.Error{
		Op:  "Post",
		URL: "https://music.youtube.com/youtubei/v1/account/account_menu",
		Err: context.DeadlineExceeded,
	}
	want := "YouTube Music didn't answer in time. Try again."
	if got := userMessage(err); got != want {
		t.Fatalf("message = %q, want %q", got, want)
	}
}

func TestDispatchLogsRawGenericError(t *testing.T) {
	const method = protocol.MethodGetProvider
	old := handlers[method]
	handlers[method] = func(context.Context, *App, json.RawMessage) (any, error) {
		return nil, &url.Error{Op: "Post", URL: "https://music.youtube.com/youtubei/v1/account/account_menu", Err: errors.New("connection reset")}
	}
	t.Cleanup(func() { handlers[method] = old })

	var log bytes.Buffer
	app := &App{Log: slog.New(slog.NewTextHandler(&log, nil))}
	_, rpcError := app.Dispatch(context.Background(), method, nil)
	if rpcError == nil {
		t.Fatal("Dispatch returned no protocol error")
	}
	if rpcError.Message != "Couldn't reach YouTube Music. Check your connection and try again." {
		t.Fatalf("protocol message = %q", rpcError.Message)
	}
	logged := log.String()
	if !strings.Contains(logged, "method=get_provider") || !strings.Contains(logged, "connection reset") || !strings.Contains(logged, "music.youtube.com") {
		t.Fatalf("warning = %q", logged)
	}
}
