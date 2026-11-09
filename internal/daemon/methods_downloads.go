package daemon

import (
	"context"

	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

func init() {
	register(protocol.MethodGetDownloadSettings, typed(getDownloadSettings))
	register(protocol.MethodSetDownloadSettings, typed(setDownloadSettings))
	register(protocol.MethodGetDownloads, typed(getDownloads))
	register(protocol.MethodEnqueueDownload, typed(enqueueDownload))
	register(protocol.MethodEnqueueCollection, typed(enqueueCollection))
	register(protocol.MethodCancelDownload, typed(cancelDownload))
	register(protocol.MethodRetryDownload, typed(retryDownload))
	register(protocol.MethodClearDownloadHistory, typed(clearDownloadHistory))
	register(protocol.MethodOpenDownload, typed(openDownload))
	register(protocol.MethodOpenDownloadFolder, typed(openDownloadFolder))
}

func getDownloadSettings(_ context.Context, a *App, _ protocol.Null) (model.DownloadSettings, error) {
	return a.Downloads.Settings(), nil
}
func setDownloadSettings(_ context.Context, a *App, p protocol.SetDownloadSettingsParams) (model.DownloadSettings, error) {
	return a.Downloads.SetSettings(p.Settings)
}
func getDownloads(_ context.Context, a *App, _ protocol.Null) (model.DownloadSnapshot, error) {
	return a.Downloads.Snapshot(), nil
}
func enqueueDownload(_ context.Context, a *App, p protocol.EnqueueDownloadParams) (model.DownloadEntry, error) {
	return a.Downloads.Enqueue(p)
}
func enqueueCollection(_ context.Context, a *App, p protocol.EnqueueCollectionParams) (model.DownloadCollectionResult, error) {
	return a.Downloads.EnqueueCollection(p.Entries)
}
func cancelDownload(_ context.Context, a *App, p protocol.DownloadIDParams) (protocol.Null, error) {
	return protocol.Null{}, a.Downloads.Cancel(p.ID)
}
func retryDownload(_ context.Context, a *App, p protocol.DownloadIDParams) (model.DownloadEntry, error) {
	return a.Downloads.Retry(p.ID)
}
func clearDownloadHistory(_ context.Context, a *App, _ protocol.Null) (protocol.Null, error) {
	return protocol.Null{}, a.Downloads.ClearHistory()
}
func openDownload(_ context.Context, a *App, p protocol.DownloadIDParams) (protocol.Null, error) {
	return protocol.Null{}, a.Downloads.Open(p.ID)
}
func openDownloadFolder(_ context.Context, a *App, _ protocol.Null) (protocol.Null, error) {
	return protocol.Null{}, a.Downloads.OpenFolder()
}
