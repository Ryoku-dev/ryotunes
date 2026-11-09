package player

import (
	"errors"
	"math"
	"testing"
	"time"

	mpv "github.com/gen2brain/go-mpv"
	"github.com/ryoku-dev/ryotunes/internal/model"
)

func TestPlayerOptions(t *testing.T) {
	normal := optionMap(playerOptions("/tmp/audio", false))
	low := optionMap(playerOptions("/tmp/audio", true))

	for name, want := range map[string]string{
		"vid": "no", "video": "no", "ytdl": "no", "gapless-audio": "yes",
		"cache": "yes", "cache-on-disk": "yes", "demuxer-cache-dir": "/tmp/audio",
		"replaygain": "no", "audio-pitch-correction": "yes", "prefetch-playlist": "yes",
	} {
		if got := normal[name]; got != want {
			t.Errorf("normal option %s = %q, want %q", name, got, want)
		}
	}
	if normal["demuxer-max-bytes"] != "268435456" || low["demuxer-max-bytes"] != "67108864" {
		t.Errorf("unexpected normal/low cache limits: %q / %q", normal["demuxer-max-bytes"], low["demuxer-max-bytes"])
	}
	if normal["cache-secs"] != "120" || low["cache-secs"] != "30" {
		t.Errorf("unexpected normal/low cache seconds: %q / %q", normal["cache-secs"], low["cache-secs"])
	}
}

func TestAFChain(t *testing.T) {
	if got := afChain(nil, model.DefaultAudioFx()); got != "" {
		t.Fatalf("neutral chain = %q", got)
	}
	gain := -6.0
	fx := model.AudioFx{Speed: 1, Semitones: -12, Reverb: 1, Bass: -6, Width: 0.5}
	want := "lavfi=[volume=-6dB],rubberband=pitch-scale=0.5," +
		"lavfi=[aecho=0.8:0.9:40|80|120:0.4|0.3|0.2]," +
		"lavfi=[bass=g=-6:f=110:w=0.6]," +
		"lavfi=[stereowiden=delay=20:feedback=0.3:crossfeed=0.3:drymix=0.7]"
	if got := afChain(&gain, fx); got != want {
		t.Fatalf("full chain = %q, want %q", got, want)
	}
	oneSemitone := afChain(nil, model.AudioFx{Speed: 1, Semitones: 1})
	if oneSemitone != "rubberband=pitch-scale=1.0594630943592953" {
		t.Fatalf("one-semitone chain = %q", oneSemitone)
	}
}

func TestLoudnessGain(t *testing.T) {
	loud := 10.0
	if got := loudnessGain(&loud); got == nil || *got != -3 {
		t.Fatalf("gain for 10 dB = %v", got)
	}
	loud = 100
	if got := loudnessGain(&loud); got == nil || *got != -24 {
		t.Fatalf("clamped gain = %v", got)
	}
	loud = 7
	if got := loudnessGain(&loud); got != nil {
		t.Fatalf("attenuate-only gain = %v", *got)
	}
	loud = math.NaN()
	if got := loudnessGain(&loud); got != nil {
		t.Fatalf("NaN gain = %v", *got)
	}
}

func TestFriendlyError(t *testing.T) {
	tests := []struct {
		err  error
		want string
	}{
		{mpv.ErrLoadingFailed, "Couldn't load this track, YouTube rejected the stream link"},
		{mpv.ErrNothingToPlay, "This stream contains no playable audio"},
		{mpv.ErrUnknownFormat, "Unrecognized audio format"},
		{mpv.ErrAoInitFailed, "Couldn't start audio output"},
		{errors.New("decoder exploded"), "Playback failed (decoder exploded)"},
	}
	for _, test := range tests {
		if got := friendlyError(test.err).Error(); got != test.want {
			t.Errorf("friendlyError(%v) = %q, want %q", test.err, got, test.want)
		}
	}
}

func TestShouldEmitPosition(t *testing.T) {
	now := time.Unix(100, 0)
	cadence := 250 * time.Millisecond
	if !shouldEmitPosition(time.Time{}, now, cadence, false) {
		t.Fatal("first sample was suppressed")
	}
	if shouldEmitPosition(now, now.Add(249*time.Millisecond), cadence, false) {
		t.Fatal("early sample was emitted")
	}
	if !shouldEmitPosition(now, now.Add(cadence), cadence, false) {
		t.Fatal("sample at cadence was suppressed")
	}
	if !shouldEmitPosition(now, now.Add(time.Millisecond), cadence, true) {
		t.Fatal("forced pause flush was suppressed")
	}
	if got := positionCadence(true); got != 500*time.Millisecond {
		t.Fatalf("low-resource cadence = %v", got)
	}
}

func optionMap(options []mpvOption) map[string]string {
	out := make(map[string]string, len(options))
	for _, option := range options {
		out[option.name] = option.value
	}
	return out
}
