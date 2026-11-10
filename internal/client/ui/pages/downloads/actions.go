package downloads

import (
	"errors"
	"sort"
	"strings"

	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

func cancelDownload(e *env.Env, id string) {
	act.Call[protocol.Null](e, protocol.MethodCancelDownload, protocol.DownloadIDParams{ID: id}, nullResult(e, ""))
}

func retryDownload(e *env.Env, id string) {
	act.Call[model.DownloadEntry](e, protocol.MethodRetryDownload, protocol.DownloadIDParams{ID: id}, func(_ model.DownloadEntry, err error) {
		reportAction(e, err, "")
	})
}

func openDownload(e *env.Env, id string) {
	act.Call[protocol.Null](e, protocol.MethodOpenDownload, protocol.DownloadIDParams{ID: id}, nullResult(e, ""))
}

func openDownloadFolder(e *env.Env) {
	act.Call[protocol.Null](e, protocol.MethodOpenDownloadFolder, protocol.Null{}, nullResult(e, ""))
}

func clearHistory(e *env.Env) {
	act.Call[protocol.Null](e, protocol.MethodClearDownloadHistory, protocol.Null{}, nullResult(e, "Download history cleared"))
}

func nullResult(e *env.Env, success string) func(protocol.Null, error) {
	return func(_ protocol.Null, err error) {
		reportAction(e, err, success)
	}
}

func reportAction(e *env.Env, err error, success string) {
	if e == nil || e.Toast == nil {
		return
	}
	if err != nil {
		e.Toast(err.Error(), env.ToastError)
	} else if success != "" {
		e.Toast(success, env.ToastSuccess)
	}
}

func downloadTrack(job model.DownloadEntry) (model.Track, bool) {
	if job.Status != model.DownloadStatusCompleted || strings.TrimSpace(job.VideoID) == "" {
		return model.Track{}, false
	}
	track := model.Track{VideoID: job.VideoID, Title: job.Title, Artists: job.Artists}
	if thumbnail := strings.TrimSpace(job.Thumbnail); thumbnail != "" {
		track.Thumbnail = &thumbnail
	}
	return track, true
}

func savedDownloadContext(items []downloadItem, selectedID string) ([]model.Track, int) {
	tracks := make([]model.Track, 0, len(items))
	start := -1
	appendJob := func(job model.DownloadEntry) {
		track, ok := downloadTrack(job)
		if !ok {
			return
		}
		if job.ID == selectedID {
			start = len(tracks)
		}
		tracks = append(tracks, track)
	}
	for _, item := range items {
		if item.job != nil {
			appendJob(*item.job)
		}
		if item.collection != nil {
			for _, job := range item.collection.jobs {
				appendJob(job)
			}
		}
	}
	return tracks, start
}

func savedJobTracks(jobs, all []model.DownloadEntry) []model.Track {
	order := make(map[string]int, len(all))
	for index := range all {
		order[all[index].ID] = index
	}
	ordered := append([]model.DownloadEntry(nil), jobs...)
	sort.SliceStable(ordered, func(i, j int) bool {
		left, leftOK := order[ordered[i].ID]
		right, rightOK := order[ordered[j].ID]
		switch {
		case leftOK && rightOK:
			return left < right
		case leftOK:
			return true
		default:
			return false
		}
	})
	tracks := make([]model.Track, 0, len(ordered))
	for _, job := range ordered {
		if track, ok := downloadTrack(job); ok {
			tracks = append(tracks, track)
		}
	}
	return tracks
}

func playDownload(e *env.Env, job model.DownloadEntry, items []downloadItem) {
	context, start := savedDownloadContext(items, job.ID)
	playDownloadTracks(e, job, context, start)
}

func playDownloadTracks(e *env.Env, job model.DownloadEntry, context []model.Track, start int) {
	track, ok := downloadTrack(job)
	if !ok {
		reportAction(e, errors.New("This download cannot be played"), "")
		return
	}
	if start < 0 || start >= len(context) || context[start].VideoID != track.VideoID {
		context = []model.Track{track}
		start = 0
	}
	act.PlayCollection(e, context, start, "", "Downloads", false, nil)
}

func primaryDownloadAction(e *env.Env, job model.DownloadEntry, items []downloadItem) {
	switch job.Status {
	case model.DownloadStatusDownloading, model.DownloadStatusQueued:
		cancelDownload(e, job.ID)
	case model.DownloadStatusCompleted:
		playDownload(e, job, items)
	case model.DownloadStatusFailed, model.DownloadStatusCancelled:
		retryDownload(e, job.ID)
	}
}
