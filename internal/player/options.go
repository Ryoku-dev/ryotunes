package player

import (
	stderrors "errors"
	"fmt"
	"time"

	mpv "github.com/gen2brain/go-mpv"
)

type mpvOption struct {
	name  string
	value string
}

type observedProperty struct {
	name   string
	format mpv.Format
}

func playerOptions(cacheDir string, lowResource bool) []mpvOption {
	maxBytes := "268435456"
	maxBackBytes := "67108864"
	cacheSeconds := "120"
	readaheadSeconds := "20"
	if lowResource {
		maxBytes = "67108864"
		maxBackBytes = "16777216"
		cacheSeconds = "30"
		readaheadSeconds = "5"
	}
	return []mpvOption{
		{name: "vid", value: "no"},
		{name: "video", value: "no"},
		{name: "audio-display", value: "no"},
		{name: "ao", value: "pipewire,pulse,alsa,null"},
		{name: "ytdl", value: "no"},
		{name: "force-window", value: "no"},
		{name: "gapless-audio", value: "yes"},
		{name: "prefetch-playlist", value: "yes"},
		{name: "cache", value: "yes"},
		{name: "cache-on-disk", value: "yes"},
		{name: "cache-secs", value: cacheSeconds},
		{name: "demuxer-max-bytes", value: maxBytes},
		{name: "demuxer-max-back-bytes", value: maxBackBytes},
		{name: "demuxer-readahead-secs", value: readaheadSeconds},
		{name: "demuxer-cache-dir", value: cacheDir},
		{name: "replaygain", value: "no"},
		{name: "audio-pitch-correction", value: "yes"},
		{name: "idle", value: "yes"},
		{name: "terminal", value: "no"},
		{name: "input-default-bindings", value: "no"},
		{name: "input-vo-keyboard", value: "no"},
	}
}

func observedProperties() []observedProperty {
	return []observedProperty{
		{name: "time-pos", format: mpv.FormatDouble},
		{name: "duration", format: mpv.FormatDouble},
		{name: "pause", format: mpv.FormatFlag},
		{name: "idle-active", format: mpv.FormatFlag},
		{name: "media-title", format: mpv.FormatString},
	}
}

func positionCadence(lowResource bool) time.Duration {
	if lowResource {
		return 500 * time.Millisecond
	}
	return 250 * time.Millisecond
}

func shouldEmitPosition(last, now time.Time, cadence time.Duration, force bool) bool {
	return force || last.IsZero() || now.Sub(last) >= cadence
}

func friendlyError(err error) error {
	switch {
	case stderrors.Is(err, mpv.ErrLoadingFailed):
		return fmt.Errorf("Couldn't load this track, YouTube rejected the stream link")
	case stderrors.Is(err, mpv.ErrNothingToPlay):
		return fmt.Errorf("This stream contains no playable audio")
	case stderrors.Is(err, mpv.ErrUnknownFormat):
		return fmt.Errorf("Unrecognized audio format")
	case stderrors.Is(err, mpv.ErrAoInitFailed):
		return fmt.Errorf("Couldn't start audio output")
	case err == nil:
		return fmt.Errorf("Playback failed (unknown mpv error)")
	default:
		return fmt.Errorf("Playback failed (%v)", err)
	}
}
