package lyrics

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/provider"
)

type cacheRecord struct {
	lyrics    *model.Lyrics
	fetchedAt time.Time
}

type fakeCache struct {
	mu       sync.Mutex
	records  map[string]cacheRecord
	lastNow  time.Time
	lastTTL  time.Duration
	getCalls int
	putCalls int
}

func (cache *fakeCache) GetLyrics(videoID string, now time.Time, ttl time.Duration) (*model.Lyrics, bool, error) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	cache.getCalls++
	cache.lastNow, cache.lastTTL = now, ttl
	record, ok := cache.records[videoID]
	if !ok {
		return nil, false, nil
	}
	if record.lyrics != nil || now.Sub(record.fetchedAt) < ttl {
		return cloneLyrics(record.lyrics), true, nil
	}
	return nil, false, nil
}

func (cache *fakeCache) PutLyrics(videoID string, lyrics *model.Lyrics, fetchedAt time.Time) error {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	cache.putCalls++
	cache.records[videoID] = cacheRecord{lyrics: cloneLyrics(lyrics), fetchedAt: fetchedAt}
	return nil
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func TestCacheTTLUsesFakeClock(t *testing.T) {
	t.Setenv("RYOTUNES_LYRICS_ONLY", "")
	start := time.Unix(1_800_000_000, 0)
	now := start
	cache := &fakeCache{records: map[string]cacheRecord{
		"fresh-miss": {fetchedAt: start.Add(-23 * time.Hour)},
		"old-hit": {
			fetchedAt: start.Add(-30 * 24 * time.Hour),
			lyrics:    &model.Lyrics{Source: "LRCLIB", Lines: []model.LyricsLine{{Text: "still cached"}}},
		},
	}}
	requests := 0
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		requests++
		return &http.Response{StatusCode: http.StatusNotFound, Status: "404 Not Found", Body: http.NoBody, Header: make(http.Header)}, nil
	})}
	service := New(Options{
		Cache: cache, HTTPClient: client, Now: func() time.Time { return now },
		Settings: provider.MapKV{string(model.SettingLyricsBoidu): "false"},
	})
	duration := 180.0

	lyrics, err := service.Get(context.Background(), Request{VideoID: "fresh-miss", Title: "Song", Artists: "Artist", Duration: &duration})
	if err != nil || lyrics != nil {
		t.Fatalf("fresh negative cache = (%v, %v), want (nil, nil)", lyrics, err)
	}
	if requests != 0 {
		t.Fatalf("fresh negative cache made %d requests", requests)
	}
	if cache.lastNow != start || cache.lastTTL != 24*time.Hour {
		t.Fatalf("cache clock arguments = (%v, %v)", cache.lastNow, cache.lastTTL)
	}

	lyrics, err = service.Get(context.Background(), Request{VideoID: "old-hit", Title: "Song", Artists: "Artist", Duration: &duration})
	if err != nil || lyrics == nil || lyrics.Lines[0].Text != "still cached" {
		t.Fatalf("old positive cache = (%v, %v), want permanent hit", lyrics, err)
	}
	if requests != 0 {
		t.Fatalf("positive cache made %d requests", requests)
	}

	now = start.Add(2 * time.Hour)
	lyrics, err = service.Get(context.Background(), Request{VideoID: "fresh-miss", Title: "Song", Artists: "Artist", Duration: &duration})
	if err != nil || lyrics != nil {
		t.Fatalf("expired negative cache = (%v, %v), want remote miss", lyrics, err)
	}
	if requests == 0 {
		t.Fatal("expired negative cache did not refetch")
	}
	if cache.putCalls != 1 {
		t.Fatalf("expired definitive miss writes = %d, want 1", cache.putCalls)
	}
}

func TestRomanizationDoesNotMutateCachedSource(t *testing.T) {
	t.Setenv("RYOTUNES_LYRICS_ONLY", "")
	start := time.Unix(1_800_000_000, 0)
	cache := &fakeCache{records: map[string]cacheRecord{
		"song": {
			fetchedAt: start,
			lyrics:    &model.Lyrics{Source: "LRCLIB", Lines: []model.LyricsLine{{Text: "사랑해"}}},
		},
	}}
	service := New(Options{Cache: cache, Romanize: true, Now: func() time.Time { return start }})
	lyrics, err := service.Get(context.Background(), Request{VideoID: "song"})
	if err != nil || lyrics == nil || lyrics.Lines[0].Romanized == nil {
		t.Fatalf("romanized output = (%v, %v)", lyrics, err)
	}
	if !strings.Contains(*lyrics.Lines[0].Romanized, "sarang") {
		t.Fatalf("romanized output = %q", *lyrics.Lines[0].Romanized)
	}
	if cache.records["song"].lyrics.Lines[0].Romanized != nil {
		t.Fatal("romanized reading leaked into cached source")
	}
}
