package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/ryoku-dev/ryotunes/internal/model"
)

func TestOpenMigratesEmptyDatabase(t *testing.T) {
	dirs := testDirs(t)
	if err := os.MkdirAll(dirs.Data, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dirs.Data, databaseName)
	if err := os.WriteFile(path, nil, 0o666); err != nil {
		t.Fatal(err)
	}
	store, err := Open(dirs, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var version int
	if err := store.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != schemaVersion {
		t.Fatalf("schema version = %d, want %d", version, schemaVersion)
	}
	for _, table := range []string{"settings", "stream_url_cache", "lyrics_cache", "plays", "local_tracks", "playlist_track", "local_playlists", "local_playlist_tracks", "radio_station_cache"} {
		var name string
		if err := store.db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&name); err != nil {
			t.Errorf("table %s: %v", table, err)
		}
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("database mode = %o, want 600", info.Mode().Perm())
	}
}

func TestOpenMigratesLegacySchemaWithoutDataLoss(t *testing.T) {
	dirs := testDirs(t)
	if err := os.MkdirAll(dirs.Data, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dirs.Data, databaseName)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		CREATE TABLE settings(key TEXT PRIMARY KEY,value TEXT);
		CREATE TABLE stream_url_cache(video_id TEXT PRIMARY KEY,url TEXT,itag INTEGER,expires_at INTEGER);
		INSERT INTO settings(key,value) VALUES('visitor_data','legacy-token');
		INSERT INTO stream_url_cache(video_id,url,itag,expires_at) VALUES('video','https://example.test/audio',251,4102444800);
	`); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := Open(dirs, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if value, ok := store.Setting("visitor_data"); !ok || value != "legacy-token" {
		t.Fatalf("legacy setting = %q, %v", value, ok)
	}
	stream, err := store.GetStream("video", time.Unix(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	if stream == nil || stream.URL != "https://example.test/audio" || stream.Itag != 251 {
		t.Fatalf("legacy stream = %#v", stream)
	}
}

func TestOpenAdoptsOnlySiblingDatabaseWithoutDataLoss(t *testing.T) {
	dirs := testDirs(t)
	if err := os.MkdirAll(dirs.Data, 0o700); err != nil {
		t.Fatal(err)
	}
	legacy := filepath.Join(dirs.Data, "legacy.sqlite")
	if err := os.WriteFile(legacy, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := Open(dirs, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.KV().Set("visitor_data", "kept"); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatalf("legacy database still exists: %v", err)
	}
	store, err = Open(dirs, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if value, ok := store.Setting("visitor_data"); !ok || value != "kept" {
		t.Fatalf("adopted setting = %q, %v", value, ok)
	}
}

func TestSettingsDefaultsValidationAndRoundTrip(t *testing.T) {
	store := testStore(t)
	defaults := store.Settings()
	if defaults.Volume != 100 || defaults.Quality != model.QualityHigh || !defaults.EnableHistory || !defaults.Autoplay || defaults.UIScale != 100 || defaults.DiscordPresenceName != "Ryotunes" {
		t.Fatalf("unexpected defaults: %#v", defaults)
	}
	if err := store.SetSetting("volume", "101"); err == nil {
		t.Fatal("invalid volume accepted")
	}
	if err := store.SetSetting("disabled_stream_clients", "TVHTML5, WEB_REMIX,TVHTML5"); err != nil {
		t.Fatal(err)
	}
	if err := store.SetSetting("volume", "42"); err != nil {
		t.Fatal(err)
	}
	if err := store.SetSetting("discord_rpc", "true"); err != nil {
		t.Fatal(err)
	}
	settings := store.Settings()
	if settings.Volume != 42 || !settings.DiscordRPC || !reflect.DeepEqual(settings.DisabledStreamClients, []model.StreamClient{model.StreamClientTVHTML5, model.StreamClientWebRemix}) {
		t.Fatalf("round-trip settings: %#v", settings)
	}
	snapshot := store.SettingsSnapshot()
	if snapshot["disabled_stream_clients"] != "TVHTML5,WEB_REMIX" || snapshot["volume"] != "42" {
		t.Fatalf("snapshot: %#v", snapshot)
	}
	if _, ok := snapshot["visitor_data"]; ok {
		t.Fatal("daemon-only setting leaked into renderer snapshot")
	}
}

func TestDerivePlaylistArtwork(t *testing.T) {
	if got := DerivePlaylistArtwork(nil); got != "" {
		t.Fatalf("empty artwork = %q", got)
	}
	if got := DerivePlaylistArtwork([]string{"a", "a", "b"}); got != "a" {
		t.Fatalf("partial artwork = %q", got)
	}
	got := DerivePlaylistArtwork([]string{"a", "a", "b", "c", "d", "e"})
	want := `ryotunes-collage:["a","b","c","d"]`
	if got != want {
		t.Fatalf("collage = %q, want %q", got, want)
	}
}

func TestListeningStatsPeriodsAndPlayCounts(t *testing.T) {
	store := testStore(t)
	now := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }
	durationA := "3:00"
	durationB := "1:30"
	insertPlay(t, store, model.Track{VideoID: "a", Title: "Alpha", Artists: "Artist A", Duration: &durationA}, now.Add(-2*time.Hour))
	insertPlay(t, store, model.Track{VideoID: "a", Title: "Alpha", Artists: "Artist A", Duration: &durationA}, now.Add(-2*24*time.Hour))
	insertPlay(t, store, model.Track{VideoID: "b", Title: "Beta", Artists: "Artist B", Duration: &durationB}, now.Add(-10*24*time.Hour))

	day, err := store.ListeningStats(model.ListeningDay)
	if err != nil {
		t.Fatal(err)
	}
	week, err := store.ListeningStats(model.ListeningWeek)
	if err != nil {
		t.Fatal(err)
	}
	month, err := store.ListeningStats(model.ListeningMonth)
	if err != nil {
		t.Fatal(err)
	}
	fallback, err := store.ListeningStats("bad")
	if err != nil {
		t.Fatal(err)
	}
	if day.Plays != 1 || week.Plays != 2 || month.Plays != 3 || fallback.Period != model.ListeningWeek || fallback.Plays != 2 {
		t.Fatalf("period stats day=%#v week=%#v month=%#v fallback=%#v", day, week, month, fallback)
	}
	if month.KnownDurationSeconds != 450 {
		t.Fatalf("known duration = %d, want 450", month.KnownDurationSeconds)
	}
	counts, err := store.PlayCounts(now.Add(-30 * 24 * time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(counts, model.PlayCounts{"a": 2, "b": 1}) {
		t.Fatalf("counts = %#v", counts)
	}
}

func testDirs(t *testing.T) Dirs {
	t.Helper()
	root := t.TempDir()
	return Dirs{
		Data: filepath.Join(root, "data"), Cache: filepath.Join(root, "cache"),
		Config: filepath.Join(root, "config"), State: filepath.Join(root, "state"),
		Runtime: filepath.Join(root, "runtime"),
	}
}

func testStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(testDirs(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
			t.Errorf("close store: %v", err)
		}
	})
	return store
}

func insertPlay(t *testing.T, store *Store, track model.Track, at time.Time) {
	t.Helper()
	raw, err := json.Marshal(track)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec("INSERT INTO plays(video_id,played_at,song_json) VALUES(?,?,?)", track.VideoID, at.Unix(), string(raw)); err != nil {
		t.Fatal(err)
	}
}
