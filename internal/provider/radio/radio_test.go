package radio

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

const stationFixture = `[
  {
    "stationuuid": "station_123-abc",
    "name": "  Example FM  ",
    "url_resolved": "https://stream.example.org/live",
    "url": "https://stream.example.org/original",
    "homepage": "https://example.org",
    "favicon": "https://example.org/icon.png",
    "country": "Canada",
    "countrycode": "ca",
    "tags": "indie,public",
    "codec": "aac",
    "bitrate": 192,
    "votes": 42,
    "clickcount": 7
  }
]`

func TestIDRoundTrip(t *testing.T) {
	t.Parallel()
	uuid := "station_123-abc"
	if got, ok := UUIDFromID(ID(uuid)); !ok || got != uuid {
		t.Fatalf("UUIDFromID(ID()) = %q, %v", got, ok)
	}
	for _, invalid := range []string{"", IDPrefix, IDPrefix + "bad/id", IDPrefix + "é"} {
		if _, ok := UUIDFromID(invalid); ok {
			t.Fatalf("accepted invalid radio ID %q", invalid)
		}
	}
}

func TestStationFixtureMapping(t *testing.T) {
	t.Parallel()
	var rows []stationJSON
	if err := json.Unmarshal([]byte(stationFixture), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("decoded %d rows", len(rows))
	}
	station, ok := normalizeStation(rows[0].model())
	if !ok {
		t.Fatal("fixture station was rejected")
	}
	if station.StationUUID != "station_123-abc" || station.Name != "Example FM" {
		t.Fatalf("identity mapping = %#v", station)
	}
	if station.StreamURL != "https://stream.example.org/live" || station.URL != "https://stream.example.org/original" {
		t.Fatalf("URL mapping = %#v", station)
	}
	if station.CountryCode != "CA" || station.Codec != "AAC" {
		t.Fatalf("normalization = %#v", station)
	}
	if station.Bitrate != 192 || station.Votes != 42 || station.ClickCount != 7 {
		t.Fatalf("numeric mapping = %#v", station)
	}
	track := Track(station)
	if track.VideoID != ID(station.StationUUID) || track.Artists != "Live Radio" {
		t.Fatalf("track mapping = %#v", track)
	}
}

func TestStationsIsDemandDrivenAndUsesDefaults(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		if request.URL.Path != "/json/stations/topvote" {
			t.Errorf("path = %q", request.URL.Path)
		}
		query := request.URL.Query()
		if query.Get("offset") != "0" || query.Get("limit") != "36" || query.Get("hidebroken") != "true" {
			t.Errorf("query = %q", request.URL.RawQuery)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(stationFixture))
	}))
	defer server.Close()

	provider := New(Options{BaseURL: server.URL, HTTPClient: server.Client()})
	if requests.Load() != 0 {
		t.Fatal("constructor fetched the radio directory")
	}
	stations, err := provider.Stations(context.Background(), "", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 1 || len(stations) != 1 {
		t.Fatalf("requests = %d, stations = %d", requests.Load(), len(stations))
	}
}
