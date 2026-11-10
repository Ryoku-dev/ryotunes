package daemon

import (
	"context"
	"crypto/tls"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/provider/youtube"
	"github.com/ryoku-dev/ryotunes/internal/store"
)

func TestPlaybackReportFollowsTheWatchHistorySetting(t *testing.T) {
	var players, reports atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/youtubei/v1/player"):
			players.Add(1)
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"playabilityStatus":{"status":"OK"},"playbackTracking":{"videostatsPlaybackUrl":{"baseUrl":"https://s.youtube.com/api/stats/playback?docid=video"}}}`)
		case r.URL.Path == "/api/stats/playback":
			reports.Add(1)
			if r.URL.Query().Get("list") != "PLmix" || r.Header.Get("Cookie") == "" || r.Header.Get("Authorization") == "" {
				t.Errorf("report = %q, authenticated %v", r.URL.RawQuery, r.Header.Get("Cookie") != "")
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	// every https host lands on the test server, so the real youtube.com URLs are exercised
	client := &http.Client{Transport: &http.Transport{
		DialTLSContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			conn, err := (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
			if err != nil {
				return nil, err
			}
			return tls.Client(conn, &tls.Config{InsecureSkipVerify: true}), nil
		},
	}}

	root := t.TempDir()
	dirs := store.Dirs{
		Data: filepath.Join(root, "data"), Cache: filepath.Join(root, "cache"),
		Config: filepath.Join(root, "config"), State: filepath.Join(root, "state"), Runtime: filepath.Join(root, "run"),
	}
	st, err := store.Open(dirs, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.KV().Set("session_cookie", "SID=session; SAPISID=auth-value"); err != nil {
		t.Fatal(err)
	}
	if err := st.KV().Set("visitor_data", "visitor-id"); err != nil {
		t.Fatal(err)
	}
	yt, err := youtube.New(youtube.Options{KV: st.KV(), HTTPClient: client, DataDir: dirs.Data})
	if err != nil {
		t.Fatal(err)
	}
	defer yt.Close()
	app := &App{Store: st, YouTube: yt, Log: slog.New(slog.NewTextHandler(io.Discard, nil)), ctx: context.Background()}
	hooks := queueHooks{app: app}
	track := model.Track{VideoID: "video"}

	if err := st.SetSetting(string(model.SettingEnableHistory), "false"); err != nil {
		t.Fatal(err)
	}
	hooks.OnPlaybackStart(context.Background(), track, "VLPLmix")
	if players.Load() != 0 || reports.Load() != 0 {
		t.Fatalf("with Watch history off: player requests = %d, reports = %d", players.Load(), reports.Load())
	}

	if err := st.SetSetting(string(model.SettingEnableHistory), "true"); err != nil {
		t.Fatal(err)
	}
	hooks.OnPlaybackStart(context.Background(), track, "VLPLmix")
	if players.Load() != 1 || reports.Load() != 1 {
		t.Fatalf("with Watch history on: player requests = %d, reports = %d", players.Load(), reports.Load())
	}
}
