package youtube

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ryoku-dev/ryotunes/internal/model"
)

// youtubeTLSClient answers every https host from server, so tests can use real youtube.com URLs.
func youtubeTLSClient(server *httptest.Server) *http.Client {
	return &http.Client{Transport: &http.Transport{
		DialTLSContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			conn, err := (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
			if err != nil {
				return nil, err
			}
			return tls.Client(conn, &tls.Config{InsecureSkipVerify: true}), nil
		},
	}}
}

func TestReportPlaybackSendsOneAuthenticatedPlaylistPing(t *testing.T) {
	var calls atomic.Int32
	var got *http.Request
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		got = r.Clone(context.Background())
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := &Client{http: youtubeTLSClient(server), cookie: "SID=session; SAPISID=auth-value", visitor: "visitor-id"}
	ping := model.PlaybackPing{URL: "https://s.youtube.com/api/stats/playback?docid=video", Client: "TVHTML5"}
	if err := client.ReportPlayback(context.Background(), ping, "PLmix"); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 || got == nil {
		t.Fatalf("playback requests = %d", calls.Load())
	}
	query := got.URL.Query()
	if query.Get("docid") != "video" || query.Get("c") != "TVHTML5" || query.Get("ver") != "2" {
		t.Fatalf("playback query = %q", got.URL.RawQuery)
	}
	if cpn := query.Get("cpn"); len(cpn) != 16 {
		t.Fatalf("cpn = %q", cpn)
	}
	if query.Get("list") != "PLmix" || query.Get("referrer") != "PLmix" {
		t.Fatalf("playlist query = %q", got.URL.RawQuery)
	}
	if got.Header.Get("Cookie") != "SID=session; SAPISID=auth-value" {
		t.Fatalf("cookie header = %q", got.Header.Get("Cookie"))
	}
	if auth := got.Header.Get("Authorization"); !strings.HasPrefix(auth, "SAPISIDHASH ") {
		t.Fatalf("authorization header = %q", auth)
	}
	if got.Header.Get("X-YouTube-Client-Name") != clients["TVHTML5"].id || got.Header.Get("X-YouTube-Client-Version") != clients["TVHTML5"].version {
		t.Fatalf("client headers = %q, %q", got.Header.Get("X-YouTube-Client-Name"), got.Header.Get("X-YouTube-Client-Version"))
	}
	if got.Header.Get("X-Goog-Visitor-Id") != "visitor-id" || got.Header.Get("User-Agent") != clients["TVHTML5"].userAgent {
		t.Fatalf("identity headers = %#v", got.Header)
	}
}

func TestReportPlaybackNeverSendsTheSessionOffYouTube(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		calls.Add(1)
	}))
	defer server.Close()
	client := &Client{http: youtubeTLSClient(server), cookie: "SAPISID=auth-value"}
	for _, raw := range []string{
		"http://s.youtube.com/api/stats/playback",
		"https://s.youtube.com.evil.test/api/stats/playback",
		"https://evil-youtube.com/api/stats/playback",
		"https://127.0.0.1/api/stats/playback",
	} {
		if err := client.ReportPlayback(context.Background(), model.PlaybackPing{URL: raw, Client: "TVHTML5"}, ""); err == nil {
			t.Errorf("ReportPlayback(%q) succeeded", raw)
		}
	}
	if calls.Load() != 0 {
		t.Fatalf("the session reached a non-YouTube host %d times", calls.Load())
	}
}

func TestReportPlaybackSkipsSignedOutSession(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		calls.Add(1)
	}))
	defer server.Close()
	client := &Client{http: youtubeTLSClient(server)}
	ping := model.PlaybackPing{URL: "https://s.youtube.com/ping", Client: "TVHTML5"}
	if err := client.ReportPlayback(context.Background(), ping, ""); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 0 {
		t.Fatalf("signed-out playback requests = %d", calls.Load())
	}
}

func TestTakePlaybackPingOnlyReturnsAFreshPingOnce(t *testing.T) {
	client := &Client{}
	ping := model.PlaybackPing{URL: "https://s.youtube.com/api/stats/playback?docid=fresh", Client: "TVHTML5"}
	client.rememberPlaybackPing("fresh", ping)
	if got, ok := client.TakePlaybackPing("fresh"); !ok || got != ping {
		t.Fatalf("fresh ping = %#v, %v", got, ok)
	}
	if _, ok := client.TakePlaybackPing("fresh"); ok {
		t.Fatal("a ping was reported twice")
	}

	client.rememberPlaybackPing("stale", ping)
	client.playbackPings["stale"] = rememberedPing{ping: ping, at: time.Now().Add(-playbackPingLifetime)}
	if _, ok := client.TakePlaybackPing("stale"); ok {
		t.Fatal("an old player response was replayed")
	}
}

func TestRememberedPingsStayBoundedWhenPrefetchesAreNeverPlayed(t *testing.T) {
	client := &Client{}
	for i := range 10 * maxRememberedPings {
		client.rememberPlaybackPing(fmt.Sprint("prefetched-", i), model.PlaybackPing{URL: "https://s.youtube.com/p", Client: "TVHTML5"})
	}
	if len(client.playbackPings) > maxRememberedPings {
		t.Fatalf("remembered pings = %d, want at most %d", len(client.playbackPings), maxRememberedPings)
	}
	last := fmt.Sprint("prefetched-", 10*maxRememberedPings-1)
	if _, ok := client.TakePlaybackPing(last); !ok {
		t.Fatal("the newest resolve's ping was dropped")
	}
}
