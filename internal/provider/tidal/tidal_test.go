package tidal

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/ryoku-dev/ryotunes/internal/bus"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
	baseprovider "github.com/ryoku-dev/ryotunes/internal/provider"
)

func TestParseIDs(t *testing.T) {
	t.Parallel()
	id, err := parseNumericID("tidal:track:12345", trackPrefix)
	if err != nil || id != 12345 {
		t.Fatalf("numeric id = %d, %v", id, err)
	}
	if _, err := parseNumericID("tidal:album:12345", trackPrefix); err == nil {
		t.Fatal("accepted wrong prefix")
	}
	uuid, err := parseStringID("tidal:playlist:abc-def", playlistPrefix)
	if err != nil || uuid != "abc-def" {
		t.Fatalf("playlist id = %q, %v", uuid, err)
	}
}

func TestQualityCeiling(t *testing.T) {
	t.Parallel()
	standard := qualityLossless
	if got := qualityFor(model.Quality("HI_RES_LOSSLESS"), standard); got != qualityLossless {
		t.Fatalf("standard hi-res = %s", got.String())
	}
	if got := qualityFor(model.QualityHigh, standard); got != qualityLossless {
		t.Fatalf("high standard = %s", got.String())
	}
	if got := qualityFor(model.QualityLow, qualityHiResLossless); got != qualityLow {
		t.Fatalf("low hi-res = %s", got.String())
	}
}

func TestBTSManifestDecoding(t *testing.T) {
	t.Parallel()
	manifest := base64.StdEncoding.EncodeToString([]byte(`{"urls":["","https://audio.example/track.flac"],"codecs":"flac","encryptionType":"NONE"}`))
	quality := "LOSSLESS"
	full := "FULL"
	stream, err := streamFromPlaybackInfo(playbackInfo{ManifestMimeType: "application/vnd.tidal.bts", Manifest: manifest, AudioQuality: &quality, AssetPresentation: &full})
	if err != nil {
		t.Fatal(err)
	}
	if stream.URL != "https://audio.example/track.flac" || stream.Codec != "flac" {
		t.Fatalf("stream = %#v", stream)
	}
}

func TestProtectedBTSManifest(t *testing.T) {
	t.Parallel()
	manifest := base64.StdEncoding.EncodeToString([]byte(`{"urls":["https://audio.example/track"],"encryptionType":"OLD_AES"}`))
	quality := "HIGH"
	_, err := streamFromPlaybackInfo(playbackInfo{ManifestMimeType: "application/vnd.tidal.bts", Manifest: manifest, AudioQuality: &quality})
	if err == nil || !errors.Is(err, baseprovider.ErrUnavailable) {
		t.Fatalf("protected error = %v", err)
	}
	if got := err.Error(); got != "tidal returned a protected asset (OLD_AES); this build does not play protected streams" {
		t.Fatalf("protected sentence = %q", got)
	}
}

func TestTokenExpiryMargin(t *testing.T) {
	t.Parallel()
	const now = int64(5_000)
	if tokenExpired(now+61, now) {
		t.Fatal("token with 61 seconds remaining is expired")
	}
	if !tokenExpired(now+60, now) {
		t.Fatal("token at margin is fresh")
	}
}

type tidalAuthRoundTrip func(*http.Request) (*http.Response, error)

func (f tidalAuthRoundTrip) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestDeviceFlowWaitingEventCarriesVerificationLink(t *testing.T) {
	for _, test := range []struct {
		name     string
		response string
		wantURL  string
	}{
		{
			name:     "complete URI",
			response: `{"deviceCode":"device","userCode":"RYO-KU","verificationUri":"link.tidal.com","verificationUriComplete":"https://link.tidal.com/RYO-KU","expiresIn":300,"interval":60}`,
			wantURL:  "https://link.tidal.com/RYO-KU",
		},
		{
			name:     "verification URI fallback",
			response: `{"deviceCode":"device","userCode":"RYO-KU","verificationUri":"link.tidal.com","expiresIn":300,"interval":60}`,
			wantURL:  "https://link.tidal.com",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			events := make(chan model.TidalAuthEvent, 1)
			sink := bus.SinkFunc(func(event string, data any) {
				if event == protocol.EventTidalAuth {
					events <- data.(model.TidalAuthEvent)
				}
			})
			client, err := New(Options{
				Bus: sink,
				HTTPClient: &http.Client{Transport: tidalAuthRoundTrip(func(*http.Request) (*http.Response, error) {
					return &http.Response{
						StatusCode: http.StatusOK,
						Header:     make(http.Header),
						Body:       io.NopCloser(strings.NewReader(test.response)),
					}, nil
				})},
			})
			if err != nil {
				t.Fatal(err)
			}
			client.credentials = Credentials{DeviceID: "device-client"}
			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan error, 1)
			go func() { done <- client.deviceFlow(ctx, client.epoch) }()
			var event model.TidalAuthEvent
			select {
			case event = <-events:
			case <-time.After(time.Second):
				t.Fatal("waiting event was not emitted")
			}
			cancel()
			if err := <-done; err != nil && !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
			if event.State != "waiting" || event.URL == nil || *event.URL != test.wantURL {
				t.Fatalf("event = %#v, want waiting url %q", event, test.wantURL)
			}
			if event.Code == nil || *event.Code != "RYO-KU" {
				t.Fatalf("code = %#v", event.Code)
			}
		})
	}
}
