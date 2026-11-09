package settings

import (
	"strings"

	"gioui.org/layout"

	"github.com/ryoku-dev/ryotunes/internal/client/lithic"
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
	p.download.path.field.Mono = true
	return settingRows(gtx,
		settingRow(e.Th, "Destination", "An absolute path, or ~/ for your home folder. Missing folders are created when needed.", func(gtx layout.Context) layout.Dimensions {
			return p.download.path.field.Layout(gtx, e.Th, "", "/home/you/Music", "", "")
		}),
		settingRow(e.Th, "Format", "Keep the original stream, or transcode it to MP3, Opus or FLAC.", func(gtx layout.Context) layout.Dimensions {
			selected := 0
			for i, format := range downloadFormatValues {
				if p.downloads.Format == format {
					selected = i
					break
				}
			}
			dims := p.download.format.Layout(gtx, e.Th, "", downloadFormatLabels, selected)
			if changed, ok := p.download.format.Changed(gtx); ok {
				p.downloads.Format = downloadFormatValues[changed]
			}
			return dims
		}),
		settingRow(e.Th, "Artist subfolders", "Put each artist's downloads in a separate folder.", func(gtx layout.Context) layout.Dimensions {
			p.download.artistFolders.Value = p.downloads.OrganizeByArtist
			before := p.download.artistFolders.Value
			dims := p.download.artistFolders.Layout(gtx, e.Th, "")
			if p.download.artistFolders.Value != before {
				p.downloads.OrganizeByArtist = p.download.artistFolders.Value
			}
			return dims
		}),
		settingRow(e.Th, "Embed metadata", "Write track metadata, cover art and lyrics into the completed download.", func(gtx layout.Context) layout.Dimensions {
			p.download.embedMetadata.Value = p.downloads.EmbedMetadata
			before := p.download.embedMetadata.Value
			dims := p.download.embedMetadata.Layout(gtx, e.Th, "")
			if p.download.embedMetadata.Value != before {
				p.downloads.EmbedMetadata = p.download.embedMetadata.Value
			}
			return dims
		}),
		settingRow(e.Th, "Simultaneous downloads", "Run one to four downloads at the same time.", func(gtx layout.Context) layout.Dimensions {
			p.download.workers.Value = max(1, p.downloads.Workers)
			p.download.workers.Min = 1
			p.download.workers.Max = 4
			p.download.workers.Step = 1
			dims := p.download.workers.Layout(gtx, e.Th, "")
			if p.download.workers.Changed() {
				p.downloads.Workers = p.download.workers.Value
			}
			return dims
		}),
		settingRow(e.Th, "Save download settings", "Changes apply to downloads queued after they are saved.", func(gtx layout.Context) layout.Dimensions {
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
			return p.download.save.Layout(gtx, e.Th, lithic.Gold, nil, "Save")
		}),
	)
}
