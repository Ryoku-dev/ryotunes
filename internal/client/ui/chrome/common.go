package chrome

import (
	"image"
	"math"
	"strconv"
	"strings"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/widget"

	"github.com/ryoku-dev/ryotunes/internal/client/lithic"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

func callVoid(e *env.Env, method string, params any) {
	act.Call[protocol.Null](e, method, params, func(_ protocol.Null, err error) {
		if err != nil && e.Toast != nil {
			e.Toast(err.Error(), env.ToastError)
		}
	})
}

func trackFromPlayback(now *model.PlaybackTrack, current *model.Track) model.Track {
	if now == nil {
		return model.Track{}
	}
	if current != nil && current.VideoID == now.VideoID {
		track := *current
		track.Title = now.Title
		track.Artists = now.Artists
		track.ArtistID = now.ArtistID
		track.ArtistRuns = now.ArtistRuns
		track.Thumbnail = now.Thumbnail
		track.Duration = now.Duration
		track.Rating = now.Rating
		track.Plays = now.Plays
		track.Likes = now.Likes
		track.Genre = now.Genre
		return track
	}
	return model.Track{
		VideoID: now.VideoID, Title: now.Title, Artists: now.Artists,
		ArtistID: now.ArtistID, ArtistRuns: now.ArtistRuns,
		Thumbnail: now.Thumbnail, Duration: now.Duration, Rating: now.Rating,
		Plays: now.Plays, Likes: now.Likes, Genre: now.Genre,
	}
}

func optionalString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func formatClock(seconds float64) string {
	if math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds < 0 {
		seconds = 0
	}
	total := int(math.Floor(seconds))
	if total >= 3600 {
		return strconv.Itoa(total/3600) + ":" + twoDigits((total/60)%60) + ":" + twoDigits(total%60)
	}
	return strconv.Itoa(total/60) + ":" + twoDigits(total%60)
}

func twoDigits(value int) string {
	if value < 10 {
		return "0" + strconv.Itoa(value)
	}
	return strconv.Itoa(value)
}

func providerName(provider model.Provider) string {
	switch provider {
	case model.ProviderYouTube:
		return "YouTube Music"
	case model.ProviderSpotify:
		return "Spotify"
	case model.ProviderSoundCloud:
		return "SoundCloud"
	case model.ProviderTidal:
		return "TIDAL"
	case model.ProviderLocal:
		return "Local"
	case model.ProviderRadio:
		return "Radio"
	default:
		return "Unknown"
	}
}

func currentDownload(snapshot model.DownloadSnapshot, videoID string) *model.DownloadEntry {
	for i := range snapshot.Jobs {
		if snapshot.Jobs[i].VideoID == videoID {
			return &snapshot.Jobs[i]
		}
	}
	return nil
}

func settingEnabled(settings model.SettingsSnapshot, key model.SettingKey, fallback bool) bool {
	value, ok := settings[string(key)]
	if !ok {
		return fallback
	}
	return strings.EqualFold(value, "true")
}

func activeKind(active bool) lithic.ButtonKind {
	if active {
		return lithic.On
	}
	return lithic.Quiet
}

func topCutArtwork(gtx layout.Context, e *env.Env, rawURL string, side int) layout.Dimensions {
	side = max(0, min(side, min(gtx.Constraints.Max.X, gtx.Constraints.Max.Y)))
	size := image.Pt(side, side)
	gtx.Constraints = layout.Exact(size)
	lithic.FillCutTop(gtx.Ops, image.Rectangle{Max: size}, gtx.Dp(16), e.Th.R.Stone3)
	if e.Art == nil || rawURL == "" {
		return layout.Dimensions{Size: size}
	}
	art, _ := e.Art.Get(rawURL, side)
	mask := clip.Outline{Path: lithic.CutPath(gtx.Ops, image.Rectangle{Max: size}, gtx.Dp(16), gtx.Dp(16), 0, 0)}.Op().Push(gtx.Ops)
	widget.Image{Src: art, Fit: widget.Cover, Position: layout.Center}.Layout(gtx)
	mask.Pop()
	return layout.Dimensions{Size: size}
}
