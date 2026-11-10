package daemon

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/ryoku-dev/ryotunes/internal/bus"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
	"github.com/ryoku-dev/ryotunes/internal/provider"
	"github.com/ryoku-dev/ryotunes/internal/provider/browsercookies"
	"github.com/ryoku-dev/ryotunes/internal/provider/soundcloud"
	"github.com/ryoku-dev/ryotunes/internal/provider/youtube"
)

func TestEveryProtocolMethodHasHandler(t *testing.T) {
	for _, method := range protocol.Methods {
		if method.Name == protocol.MethodSubscribe {
			continue
		}
		if handlers[method.Name] == nil {
			t.Errorf("protocol method %q has no daemon handler", method.Name)
		}
	}
}

func TestGetLogsIncludesRuntimeDiagnostics(t *testing.T) {
	app := &App{EngineVersion: "mpv 0.40.0"}
	result, err := getLogs(context.Background(), app, protocol.GetLogsParams{})
	if err != nil {
		t.Fatal(err)
	}
	if result.EngineVersion != app.EngineVersion {
		t.Fatalf("engine version = %q, want %q", result.EngineVersion, app.EngineVersion)
	}
	if result.MPRIS {
		t.Fatal("MPRIS reported active without a service")
	}
}

type authCookieSource struct {
	cookies []browsercookies.Cookie
}

func (f authCookieSource) Discover() []browsercookies.Store {
	return []browsercookies.Store{{Path: "fake"}}
}

func (authCookieSource) Resolve(source browsercookies.SessionSource) (browsercookies.Store, error) {
	return browsercookies.Store{Kind: source.Kind, Path: source.Path}, nil
}

func (f authCookieSource) Read(browsercookies.Store, ...string) ([]browsercookies.Cookie, error) {
	return f.cookies, nil
}

func (authCookieSource) ChangedSince(browsercookies.Store, time.Time) bool { return false }

type authRoundTrip func(*http.Request) (*http.Response, error)

func (f authRoundTrip) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func youtubeAuthHTTP() *http.Client {
	return &http.Client{Transport: authRoundTrip(func(*http.Request) (*http.Response, error) {
		body := `{"activeAccountHeaderRenderer":{"accountName":{"runs":[{"text":"Tester"}]}}}`
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	})}
}

func TestYouTubeSignInMethodReturnsBrowserHandoff(t *testing.T) {
	for _, test := range []struct {
		name    string
		cookies []browsercookies.Cookie
		want    string
	}{
		{name: "browser needed", want: "https://music.youtube.com"},
		{name: "cookie found", cookies: []browsercookies.Cookie{
			{Name: "SAPISID", Value: "stored", Host: ".youtube.com"},
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			client, err := youtube.New(youtube.Options{
				KV:         provider.MapKV{"visitor_data": "visitor"},
				DataDir:    t.TempDir(),
				HTTPClient: youtubeAuthHTTP(),
				Cookies:    authCookieSource{cookies: test.cookies},
			})
			if err != nil {
				t.Fatal(err)
			}
			app := &App{YouTube: client, Bus: &bus.Fanout{}, ctx: context.Background()}
			result, err := signIn(context.Background(), app, protocol.Null{})
			if err != nil {
				t.Fatal(err)
			}
			if result.URL != test.want {
				t.Fatalf("url = %q, want %q", result.URL, test.want)
			}
			client.Close()
			app.background.Wait()
		})
	}
}

func TestSoundCloudSignInMethodReturnsBrowserHandoff(t *testing.T) {
	for _, test := range []struct {
		name    string
		cookies []browsercookies.Cookie
		want    string
	}{
		{name: "browser needed", want: "https://soundcloud.com/signin"},
		{name: "cookie found", cookies: []browsercookies.Cookie{
			{Name: "oauth_token", Value: "stored", Host: ".soundcloud.com"},
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			client, err := soundcloud.New(soundcloud.Options{
				KV:      provider.MapKV{},
				Bus:     bus.Discard,
				Cookies: authCookieSource{cookies: test.cookies},
				HTTPClient: &http.Client{Transport: authRoundTrip(func(*http.Request) (*http.Response, error) {
					return &http.Response{
						StatusCode: http.StatusOK,
						Header:     make(http.Header),
						Body:       io.NopCloser(strings.NewReader(`{"id":7,"username":"Tester"}`)),
					}, nil
				})},
			})
			if err != nil {
				t.Fatal(err)
			}
			result, err := soundCloudSignIn(context.Background(), &App{SoundCloud: client}, protocol.Null{})
			if err != nil {
				t.Fatal(err)
			}
			if result.URL != test.want {
				t.Fatalf("url = %q, want %q", result.URL, test.want)
			}
			if test.want == "" {
				deadline := time.Now().Add(time.Second)
				for !client.Status().SignedIn && time.Now().Before(deadline) {
					time.Sleep(time.Millisecond)
				}
				if !client.Status().SignedIn {
					t.Fatal("cookie import did not complete")
				}
			} else {
				_ = client.SignOut()
			}
		})
	}
}
