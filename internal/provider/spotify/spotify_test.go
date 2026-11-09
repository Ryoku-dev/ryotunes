package spotify

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	audiofilespb "github.com/devgianlu/go-librespot/proto/spotify/extendedmetadata/audiofiles"
	metadatapb "github.com/devgianlu/go-librespot/proto/spotify/metadata"
	"github.com/ryoku-dev/ryotunes/internal/bus"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

func TestTrackIDConversions(t *testing.T) {
	const bare = "2FY7b99s15jUprqC0M5NCT"
	for _, input := range []string{bare, "spotify:track:" + bare} {
		id, got, err := trackID(input)
		if err != nil {
			t.Fatalf("trackID(%q): %v", input, err)
		}
		if got != bare || id.Base62() != bare {
			t.Fatalf("trackID(%q) = %q, %q", input, got, id.Base62())
		}
	}
	if _, _, err := trackID("spotify:album:" + bare); err == nil {
		t.Fatal("accepted an album as a track")
	}
}

func TestChooseVorbisByQuality(t *testing.T) {
	files := []*audiofilespb.ExtendedAudioFile{
		{File: &metadatapb.AudioFile{Format: new(metadatapb.AudioFile_OGG_VORBIS_96), FileId: []byte{96}}},
		{File: &metadatapb.AudioFile{Format: new(metadatapb.AudioFile_OGG_VORBIS_160), FileId: []byte{160}}},
		{File: &metadatapb.AudioFile{Format: new(metadatapb.AudioFile_OGG_VORBIS_320), FileId: []byte{32}}},
	}
	for _, test := range []struct {
		quality model.Quality
		want    int
	}{{model.QualityLow, 96}, {model.QualityAuto, 160}, {model.QualityHigh, 320}} {
		_, bitrate := chooseVorbis(files, test.quality)
		if bitrate != test.want {
			t.Errorf("chooseVorbis(%s) bitrate = %d, want %d", test.quality, bitrate, test.want)
		}
	}
}

func TestOggReaderSkipsSpotifyHeader(t *testing.T) {
	header := bytes.Repeat([]byte{0xff}, int(spotifyOggHeaderOffset))
	payload := []byte("OggS payload")
	reader := newOggReader(bytes.NewReader(append(header, payload...)), int64(len(payload)))
	got, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("got %q, want %q", got, payload)
	}
}

func TestServeSeekableRange(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "http://loopback/track.ogg", nil)
	request.Header.Set("Range", "bytes=3-6")
	response := httptest.NewRecorder()
	body := []byte("0123456789")
	serveSeekable(response, request, "track.ogg", bytes.NewReader(body), int64(len(body)))
	if response.Code != http.StatusPartialContent {
		t.Fatalf("status = %d", response.Code)
	}
	if got := response.Body.String(); got != "3456" {
		t.Fatalf("body = %q", got)
	}
	if got := response.Header().Get("Content-Range"); got != "bytes 3-6/10" {
		t.Fatalf("Content-Range = %q", got)
	}
}

func TestRepeatedSignInReemitsPendingURL(t *testing.T) {
	const authURL = "https://accounts.spotify.com/authorize?state=stable"
	events := make(chan model.SpotifyAuthEvent, 2)
	client := &Spotify{
		events: bus.SinkFunc(func(event string, data any) {
			if event == protocol.EventSpotifyAuth {
				events <- data.(model.SpotifyAuthEvent)
			}
		}),
		signing: true,
		authURL: authURL,
	}
	for range 2 {
		started, err := client.SignIn(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if !started {
			t.Fatal("pending flow was not reported as started")
		}
	}
	for range 2 {
		event := <-events
		if event.State != "url" || event.URL == nil || *event.URL != authURL {
			t.Fatalf("event = %#v", event)
		}
	}
}
