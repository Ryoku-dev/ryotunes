package daemon

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
	"github.com/ryoku-dev/ryotunes/internal/store"
)

func TestListeningDaysAnswersRecordedPlays(t *testing.T) {
	root := t.TempDir()
	st, err := store.Open(store.Dirs{
		Data: filepath.Join(root, "data"), Cache: filepath.Join(root, "cache"),
		Config: filepath.Join(root, "config"), State: filepath.Join(root, "state"), Runtime: filepath.Join(root, "run"),
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	length := "2:30"
	for _, id := range []string{"one", "two", "LOCAL:/music/song.flac"} {
		if err := st.RecordPlay(model.Track{VideoID: id, Title: id, Duration: &length}); err != nil {
			t.Fatal(err)
		}
	}
	app := &App{Store: st}

	result, rpcErr := app.Dispatch(context.Background(), protocol.MethodListeningDays, json.RawMessage(`{"days":7}`))
	if rpcErr != nil {
		t.Fatal(rpcErr)
	}
	series, ok := result.(model.ListeningDays)
	if !ok {
		t.Fatalf("result = %T", result)
	}
	if len(series.Days) != 7 {
		t.Fatalf("days = %d, want 7", len(series.Days))
	}
	today := series.Days[6]
	if today.Date != time.Now().Format(time.DateOnly) || today.Plays != 2 || today.KnownDurationSeconds != 300 {
		t.Fatalf("today = %#v (local plays are not history)", today)
	}
	for _, day := range series.Days[:6] {
		if day.Plays != 0 {
			t.Fatalf("earlier day %#v has plays", day)
		}
	}

	result, rpcErr = app.Dispatch(context.Background(), protocol.MethodListeningDays, nil)
	if rpcErr != nil {
		t.Fatal(rpcErr)
	}
	if got := len(result.(model.ListeningDays).Days); got != 1 {
		t.Fatalf("absent days = %d rows, want today only", got)
	}
	if _, rpcErr = app.Dispatch(context.Background(), protocol.MethodListeningDays, json.RawMessage(`{"days":"week"}`)); rpcErr == nil || rpcErr.Code != protocol.ErrorBadParams {
		t.Fatalf("bad params error = %#v", rpcErr)
	}
}
