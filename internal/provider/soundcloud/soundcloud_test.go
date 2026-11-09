package soundcloud

import (
	"testing"
)

func TestParseNumericID(t *testing.T) {
	t.Parallel()
	id, err := parseNumericID("sc:track:184467", trackPrefix)
	if err != nil || id != 184467 {
		t.Fatalf("parse valid id = %d, %v", id, err)
	}
	for _, value := range []string{"184467", "sc:track:", "sc:track:not-a-number", "sc:user:184467"} {
		if _, err := parseNumericID(value, trackPrefix); err == nil {
			t.Errorf("parseNumericID(%q) succeeded", value)
		}
	}
}

func TestChooseTranscoding(t *testing.T) {
	t.Parallel()
	progressive := transcoding{URL: "progressive"}
	progressive.Format.Protocol = "progressive"
	progressive.Format.MimeType = "audio/mpeg"
	mp3 := transcoding{URL: "mp3"}
	mp3.Format.Protocol = "hls"
	mp3.Format.MimeType = "audio/mpeg"
	aac := transcoding{URL: "aac"}
	aac.Format.Protocol = "hls"
	aac.Format.MimeType = "audio/mp4; codecs=mp4a.40.2"
	other := transcoding{URL: "other"}
	other.Format.Protocol = "hls"
	other.Format.MimeType = "audio/ogg"
	if got := chooseTranscoding([]transcoding{progressive, mp3, aac}); got == nil || got.URL != "aac" {
		t.Fatalf("AAC choice = %#v", got)
	}
	if got := chooseTranscoding([]transcoding{progressive, mp3}); got == nil || got.URL != "mp3" {
		t.Fatalf("MP3 choice = %#v", got)
	}
	if got := chooseTranscoding([]transcoding{progressive, other}); got == nil || got.URL != "other" {
		t.Fatalf("fallback choice = %#v", got)
	}
	if got := chooseTranscoding([]transcoding{progressive}); got != nil {
		t.Fatalf("progressive selected: %#v", got)
	}
	err := unavailableError("track 12 has no playable HLS transcoding")
	if err.Error() != "track 12 has no playable HLS transcoding" {
		t.Fatalf("missing HLS sentence = %q", err.Error())
	}
}

func TestAuthExpiryMargin(t *testing.T) {
	t.Parallel()
	const now = int64(1_000)
	if authExpired(now+61, now) {
		t.Fatal("token with 61 seconds remaining is expired")
	}
	if !authExpired(now+60, now) {
		t.Fatal("token at 60 second margin is fresh")
	}
	if !authExpired(now-1, now) {
		t.Fatal("past token is fresh")
	}
}

func TestDownsampleWaveform(t *testing.T) {
	t.Parallel()
	got := downsample([]int{0, 5, 10, 20}, 20, 2)
	if len(got) != 2 || got[0] != 25 || got[1] != 100 {
		t.Fatalf("downsample = %v", got)
	}
	zeros := downsample(nil, 0, 240)
	if len(zeros) != 240 {
		t.Fatalf("empty waveform length = %d", len(zeros))
	}
}
