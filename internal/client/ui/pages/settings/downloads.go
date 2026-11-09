package settings

import (
	"strings"

	"gioui.org/layout"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

var (
	downloadFormatLabels = []string{"Original", "MP3", "Opus", "FLAC"}
	downloadFormatValues = []model.DownloadFormat{model.DownloadOriginal, model.DownloadMP3, model.DownloadOpus, model.DownloadFLAC}
)

func (p *page) layoutDownloads(gtx layout.Context, e *env.Env) layout.Dimensions {
	p.download.path.sync(p.downloads.Path)
	return p.settingRows(gtx,
		p.settingRow(e.Th, "Destination", "An absolute path, or ~/ for your home folder. Missing folders are created when needed.", func(gtx layout.Context) layout.Dimensions {
			p.download.path.field.Mono = true
			p.download.path.field.Placeholder = "/home/you/Music"
			return p.download.path.field.Layout(gtx, e.Th.Theme)
		}),
		p.settingRow(e.Th, "Format", "Keep the original stream, or transcode it to MP3, Opus or FLAC.", func(gtx layout.Context) layout.Dimensions {
			selected := 0
			for i, format := range downloadFormatValues {
				if p.downloads.Format == format {
					selected = i
					break
				}
			}
			dims := p.download.format.Layout(gtx, e.Th, e.Overlays, downloadFormatLabels, selected)
			if changed, ok := p.download.format.Changed(); ok {
				p.downloads.Format = downloadFormatValues[changed]
			}
			return dims
		}),
		p.settingRow(e.Th, "Artist subfolders", "Put each artist's downloads in a separate folder.", func(gtx layout.Context) layout.Dimensions {
			p.download.artistFolders.Value = p.downloads.OrganizeByArtist
			dims := p.download.artistFolders.Layout(gtx, e.Th.Theme)
			if p.download.artistFolders.Changed() {
				p.downloads.OrganizeByArtist = p.download.artistFolders.Value
			}
			return dims
		}),
		p.settingRow(e.Th, "Embed metadata", "Write track metadata, cover art and lyrics into the completed download.", func(gtx layout.Context) layout.Dimensions {
			p.download.embedMetadata.Value = p.downloads.EmbedMetadata
			dims := p.download.embedMetadata.Layout(gtx, e.Th.Theme)
			if p.download.embedMetadata.Changed() {
				p.downloads.EmbedMetadata = p.download.embedMetadata.Value
			}
			return dims
		}),
		p.settingRow(e.Th, "Simultaneous downloads", "Run one to four downloads at the same time.", func(gtx layout.Context) layout.Dimensions {
			p.download.workers.Value = max(1, p.downloads.Workers)
			p.download.workers.Min, p.download.workers.Max, p.download.workers.Step = 1, 4, 1
			dims := p.download.workers.Layout(gtx, e.Th.Theme)
			if p.download.workers.Changed() {
				p.downloads.Workers = p.download.workers.Value
			}
			return dims
		}),
		p.settingRow(e.Th, "Download settings", "Changes apply to downloads queued after they are saved.", func(gtx layout.Context) layout.Dimensions {
			p.download.save.Kind, p.download.save.Label = kit.PlaqueLine, "Save"
			if p.download.save.Clicked(gtx) {
				settings := p.downloads
				settings.Path = strings.TrimSpace(p.download.path.field.Text())
				act.Call[model.DownloadSettings](e, protocol.MethodSetDownloadSettings, protocol.SetDownloadSettingsParams{Settings: settings}, func(saved model.DownloadSettings, err error) {
					if err != nil {
						toastError(e, err, "Could not save download settings")
						return
					}
					p.downloads = saved
					p.download.path.initialized = false
					e.Toast("Download settings saved", env.ToastSuccess)
				})
			}
			return p.download.save.Layout(gtx, e.Th.Theme)
		}))
}
