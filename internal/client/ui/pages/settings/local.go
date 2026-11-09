package settings

import (
	"path/filepath"
	"strings"

	"gioui.org/layout"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
	localprovider "github.com/ryoku-dev/ryotunes/internal/provider/local"
)

func (p *page) layoutLocal(gtx layout.Context, e *env.Env) layout.Dimensions {
	rows := make([]layout.Widget, 0, len(p.local.library.Folders)+2)
	if p.local.errorMessage != "" {
		rows = append(rows, settingRow(e.Th, "Local music", p.local.errorMessage, nil))
	}
	if p.local.loaded && len(p.local.library.Folders) == 0 {
		rows = append(rows, settingRow(e.Th, "Watched folders", "No folders yet. Add the one your music sits in.", nil))
	}
	for _, folder := range p.local.library.Folders {
		folder := folder
		button := p.local.remove[folder]
		if button == nil {
			button = new(kit.Plaque)
			p.local.remove[folder] = button
		}
		rows = append(rows, settingRow(e.Th, filepath.Base(folder), folder, func(gtx layout.Context) layout.Dimensions {
			button.Kind, button.Label, button.Small = kit.PlaqueDangerLine, "Remove", true
			if button.Clicked(gtx) {
				p.local.busy = true
				act.Call[model.LocalLibrary](e, protocol.MethodRemoveLocalFolder, protocol.PathParams{Path: folder}, func(local model.LocalLibrary, err error) {
					p.local.busy = false
					if err != nil {
						toastError(e, err, "Could not remove local folder")
						return
					}
					p.local.library = local
				})
			}
			count := localTrackCount(folder, p.local.library.Songs)
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(kit.Text(e.Th.Theme, e.Th.T.Mono, e.Th.R.Fg2, pluralCount(count, "track"))),
				layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return button.Layout(gtx, e.Th.Theme) }),
			)
		}))
	}
	rows = append(rows,
		settingRow(e.Th, "Rescan watched folders", "Refresh every watched folder and update the local library.", func(gtx layout.Context) layout.Dimensions {
			p.local.rescan.Kind, p.local.rescan.Label, p.local.rescan.Busy = kit.PlaqueLine, "Rescan", p.local.busy
			if p.local.rescan.Clicked(gtx) && !p.local.busy {
				p.local.loaded = false
				p.loadLocal(e, p.generation)
			}
			return p.local.rescan.Layout(gtx, e.Th.Theme)
		}),
		settingRow(e.Th, "Add folder", "Folders Ryotunes watches for files on disk. The daemon rescans them on demand.", func(gtx layout.Context) layout.Dimensions {
			p.local.path.field.Mono = true
			p.local.path.field.Placeholder = "/home/you/Music"
			p.local.add.Kind, p.local.add.Label, p.local.add.Busy = kit.PlaqueLine, "Add folder", p.local.busy
			if p.local.add.Clicked(gtx) && !p.local.busy {
				path := strings.TrimSpace(p.local.path.field.Text())
				if path == "" {
					e.Toast("Choose a music folder", env.ToastError)
				} else {
					p.local.busy = true
					act.Call[model.LocalLibrary](e, protocol.MethodAddLocalFolder, protocol.PathParams{Path: path}, func(local model.LocalLibrary, err error) {
						p.local.busy = false
						if err != nil {
							toastError(e, err, "Could not add local folder")
							return
						}
						p.local.library, p.local.loaded = local, true
						p.local.path.field.SetText("")
					})
				}
			}
			return layout.Flex{Axis: layout.Vertical, Alignment: layout.End}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return p.local.path.field.Layout(gtx, e.Th.Theme) }),
				layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return p.local.add.Layout(gtx, e.Th.Theme) }),
			)
		}),
	)
	return settingRows(gtx, rows...)
}

func localTrackCount(folder string, tracks []model.Track) int {
	folder = filepath.Clean(folder)
	count := 0
	for _, track := range tracks {
		path, ok := localprovider.PathFromID(track.VideoID)
		if !ok {
			continue
		}
		relative, err := filepath.Rel(folder, filepath.Clean(path))
		if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			count++
		}
	}
	return count
}

func pluralCount(count int, noun string) string {
	if count == 1 {
		return "1 " + noun
	}
	return intString(count) + " " + noun + "s"
}
