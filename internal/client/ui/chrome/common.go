package chrome

import (
	"strings"

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

func currentDownload(snapshot model.DownloadSnapshot, videoID string) *model.DownloadEntry {
	for index := range snapshot.Jobs {
		if snapshot.Jobs[index].VideoID == videoID {
			return &snapshot.Jobs[index]
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
