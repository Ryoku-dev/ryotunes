package downloads

import (
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

func primaryDownloadAction(e *env.Env, job model.DownloadEntry) {
	switch job.Status {
	case model.DownloadStatusDownloading, model.DownloadStatusQueued:
		cancelDownload(e, job.ID)
	case model.DownloadStatusCompleted:
		openDownload(e, job.ID)
	case model.DownloadStatusFailed, model.DownloadStatusCancelled:
		retryDownload(e, job.ID)
	}
}
