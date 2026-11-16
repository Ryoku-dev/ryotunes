package store

import (
	"testing"
	"time"

	"github.com/ryoku-dev/ryotunes/internal/model"
)

func TestLyricsCacheIgnoresRowsFromAnOlderMatcher(t *testing.T) {
	dirs := testDirs(t)
	store, err := Open(dirs, nil)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_800_000_000, 0)
	// What an older build wrote: wrong lyrics, and a miss, keyed by bare id.
	if _, err := store.db.Exec(`INSERT INTO lyrics_cache(video_id,lyrics,fetched_at) VALUES
		('YjCBpTCgmjM','{"source":"Netease Cloud Music","synced":true,"instrumental":false,"lines":[{"text":"wrong song"}]}',?),
		('missed',NULL,?)`, now.Unix(), now.Unix()); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"YjCBpTCgmjM", "missed"} {
		if lyrics, hit, err := store.GetLyrics(id, now, 24*time.Hour); err != nil || hit || lyrics != nil {
			t.Fatalf("GetLyrics(%q) = %#v, %v, %v; want a miss past the old row", id, lyrics, hit, err)
		}
	}

	want := &model.Lyrics{Source: "LRCLIB", Lines: []model.LyricsLine{{Text: "right song"}}}
	if err := store.PutLyrics("YjCBpTCgmjM", want, now); err != nil {
		t.Fatal(err)
	}
	if lyrics, hit, err := store.GetLyrics("YjCBpTCgmjM", now, 24*time.Hour); err != nil || !hit || lyrics == nil || lyrics.Lines[0].Text != "right song" {
		t.Fatalf("GetLyrics after put = %#v, %v, %v", lyrics, hit, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store, err = Open(dirs, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var stale, kept int
	if err := store.db.QueryRow("SELECT count(*) FROM lyrics_cache WHERE video_id IN ('YjCBpTCgmjM','missed')").Scan(&stale); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow("SELECT count(*) FROM lyrics_cache").Scan(&kept); err != nil {
		t.Fatal(err)
	}
	if stale != 0 || kept != 1 {
		t.Fatalf("after reopen: %d old rows, %d rows in all; want 0 and 1", stale, kept)
	}
	if lyrics, hit, err := store.GetLyrics("YjCBpTCgmjM", now, 24*time.Hour); err != nil || !hit || lyrics.Lines[0].Text != "right song" {
		t.Fatalf("GetLyrics after reopen = %#v, %v, %v", lyrics, hit, err)
	}
}
