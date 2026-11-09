package playlist

import (
	"fmt"
	"strings"

	"gioui.org/layout"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

type exportDialog struct {
	dialog kit.Dialog
	path   kit.TextField
	cancel kit.Plaque
	export kit.Plaque
	err    string
	busy   bool
	title  string
}

func (d *exportDialog) Open(title string) {
	d.dialog.Open = true
	d.title = title
	d.err = ""
	d.path.SetText(defaultExportName(title))
}

func (d *exportDialog) Layout(gtx layout.Context, e *env.Env, p *Page) layout.Dimensions {
	if d.cancel.Clicked(gtx) {
		d.dialog.Open = false
	}
	if d.export.Clicked(gtx) && !d.busy {
		path := strings.TrimSpace(d.path.Text())
		if path == "" {
			d.err = "Enter where to write the playlist file."
		} else {
			d.busy = true
			d.err = ""
			p.collectAll(e, append([]model.Track(nil), p.page.Items...), p.page.Continuation, make(map[string]struct{}), func(items []model.Track, err error) {
				if err != nil {
					d.busy = false
					d.err = err.Error()
					return
				}
				act.Call[bool](e, protocol.MethodExportPlaylistFile, protocol.ExportPlaylistFileParams{Title: p.Title(), Items: items, Path: path}, func(_ bool, err error) {
					d.busy = false
					if err != nil {
						d.err = err.Error()
						return
					}
					d.dialog.Open = false
					e.Toast("Playlist exported", env.ToastSuccess)
				})
			})
		}
	}
	d.path.Label = "File path"
	d.path.Placeholder = "/home/you/Music/playlist.json"
	d.path.Help = "Ryotunes playlist files end in .json."
	d.path.Error = d.err
	d.path.Mono = true
	d.cancel.Kind, d.cancel.Label = kit.PlaqueQuiet, "Cancel"
	d.export.Kind, d.export.Icon, d.export.Label = kit.PlaqueLine, look.Icon.Download, "Export"
	d.export.Busy = d.busy
	d.dialog.Width = 560
	dims := d.dialog.Layout(gtx, e.Th.Theme, "Export playlist",
		func(gtx layout.Context) layout.Dimensions { return d.path.Layout(gtx, e.Th.Theme) },
		func(gtx layout.Context) layout.Dimensions { return d.cancel.Layout(gtx, e.Th.Theme) },
		func(gtx layout.Context) layout.Dimensions { return d.export.Layout(gtx, e.Th.Theme) },
	)
	return dims
}

func defaultExportName(title string) string {
	name := strings.ToLower(strings.TrimSpace(title))
	name = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			return r
		case r == ' ', r == '-', r == '_':
			return '-'
		default:
			return -1
		}
	}, name)
	name = strings.Trim(name, "-")
	if name == "" {
		name = "ryotunes-playlist"
	}
	return name + ".json"
}

func (p *Page) confirmDownload(e *env.Env) {
	if len(p.page.Items) == 0 {
		return
	}
	e.Confirm(
		"Download playlist?",
		"This queues every available track in "+p.Title()+". Tracks already saved are left alone.",
		"Download",
		"",
		func() {
			e.Toast("Preparing "+p.Title()+" downloads…", env.ToastInfo)
			p.collectAll(e, append([]model.Track(nil), p.page.Items...), p.page.Continuation, make(map[string]struct{}), func(items []model.Track, err error) {
				if err != nil {
					if err.Error() == "Pagination repeated" {
						e.Toast("Pagination repeated; no downloads were added.", env.ToastError)
					} else {
						e.Toast("Could not download "+p.Title(), env.ToastError)
					}
					return
				}
				p.enqueueDownloads(e, items)
			})
		},
	)
}

func (p *Page) collectAll(e *env.Env, items []model.Track, token *string, seen map[string]struct{}, done func([]model.Track, error)) {
	if token == nil || *token == "" {
		done(items, nil)
		return
	}
	if _, repeated := seen[*token]; repeated {
		done(nil, fmt.Errorf("Pagination repeated"))
		return
	}
	seen[*token] = struct{}{}
	act.Call[model.PlaylistTracksPage](e, protocol.MethodGetPlaylistMore, protocol.TokenParams{Token: *token}, func(more model.PlaylistTracksPage, err error) {
		if err != nil {
			done(nil, err)
			return
		}
		p.collectAll(e, append(items, more.Items...), more.Continuation, seen, done)
	})
}

func (p *Page) enqueueDownloads(e *env.Env, items []model.Track) {
	collection := p.Title()
	kind := "playlist"
	entries := make([]model.DownloadCollectionEntry, 0, len(items))
	for _, track := range items {
		entries = append(entries, model.DownloadCollectionEntry{
			VideoID: track.VideoID, Title: track.Title, Artists: track.Artists, Thumbnail: value(track.Thumbnail), Collection: &collection, CollectionKind: &kind,
		})
	}
	act.Call[model.DownloadCollectionResult](e, protocol.MethodEnqueueCollection, protocol.EnqueueCollectionParams{Entries: entries}, func(result model.DownloadCollectionResult, err error) {
		if err != nil {
			e.Toast("Could not download "+p.Title(), env.ToastError)
			return
		}
		if result.Added == 0 {
			e.Toast("Nothing new to download", env.ToastInfo)
			return
		}
		message := fmt.Sprintf("%s added · %d already saved · %d unavailable", trackCount(result.Added), result.AlreadyDownloaded, result.Skipped)
		e.Toast(message, env.ToastSuccess)
	})
}

func trackCount(n int) string {
	if n == 1 {
		return "1 track"
	}
	return fmt.Sprintf("%d tracks", n)
}
