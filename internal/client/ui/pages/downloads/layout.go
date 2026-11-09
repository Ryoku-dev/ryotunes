package downloads

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"gioui.org/layout"

	"github.com/ryoku-dev/ryotunes/internal/client/lithic"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/widgets"
	"github.com/ryoku-dev/ryotunes/internal/model"
)

type listEntryKind uint8

const (
	entryActiveHeader listEntryKind = iota
	entryActiveEmpty
	entryDownload
	entryHistoryHeader
	entryHistoryNote
	entryHistoryEmpty
)

type listEntry struct {
	kind listEntryKind
	item downloadItem
}

// Layout renders the event-driven queue and latest terminal history.
func (p *Page) Layout(gtx layout.Context, e *env.Env) layout.Dimensions {
	snapshot := e.Store.Snapshot().Downloads
	active, history, activeCount, queuedCount := projectDownloads(snapshot.Jobs)
	historyCount := terminalCount(snapshot.Jobs)
	connected := e.Conn != nil && e.Conn.Connected()

	if p.openFolder.Clicked(gtx) {
		openDownloadFolder(e)
	}
	if p.clear.Clicked(gtx) && historyCount > 0 && e.Confirm != nil {
		e.Confirm(
			"Clear download history?",
			"Latest 200 downloads. Clearing history keeps your music files.",
			"Clear history",
			"",
			func() { clearHistory(e) },
		)
	}
	if p.settings.Clicked(gtx) {
		e.Nav.Push(env.Route{Kind: env.RouteSettings, Params: "downloads"})
	}

	inset := layout.Inset{Top: e.Th.Sp.S4, Right: e.Th.Sp.S5, Bottom: e.Th.Sp.S5, Left: e.Th.Sp.S5}
	return inset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(widgets.PageHeader(e, "Downloads", downloadsSentence(snapshot.ActiveCount, snapshot.QueuedCount), p.headerActions(e, historyCount))),
			layout.Rigid(layout.Spacer{Height: e.Th.Sp.S2}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return lithic.Label{Style: e.Th.T.Body, Color: e.Th.R.Fg2, MaxLines: 2}.Layout(gtx, e.Th, downloadsSubtitle(snapshot.Jobs))
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if connected {
					return layout.Dimensions{}
				}
				return quietDownloadLine(gtx, e, "Disconnected from Ryotunes. This list will update when it reconnects. Interrupted downloads can be retried.")
			}),
			layout.Rigid(layout.Spacer{Height: e.Th.Sp.S4}.Layout),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				if len(snapshot.Jobs) == 0 {
					title := "No downloads yet"
					body := "Save a track with the download arrow beside its heart in the player. Downloads keep running when you close the window."
					if !connected {
						title = "Not connected"
						body = "Your download history will update when Ryotunes reconnects."
					}
					return widgets.EmptyState(e, "download", title, body)(gtx)
				}
				entries := makeDownloadEntries(active, history)
				return p.list.Layout(gtx, len(entries), func(gtx layout.Context, index int) layout.Dimensions {
					return p.layoutEntry(gtx, e, entries[index], activeCount+queuedCount, historyCount)
				})
			}),
		)
	})
}

func (p *Page) headerActions(e *env.Env, historyCount int) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		children := []layout.FlexChild{
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return p.openFolder.Layout(gtx, e.Th, lithic.Line, lithic.Icon("folder"), "Open folder")
			}),
		}
		if historyCount > 0 {
			children = append(children,
				layout.Rigid(layout.Spacer{Width: e.Th.Sp.S2}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return p.clear.Layout(gtx, e.Th, lithic.Quiet, lithic.Icon("delete_sweep"), "Clear history")
				}),
			)
		}
		children = append(children,
			layout.Rigid(layout.Spacer{Width: e.Th.Sp.S2}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return p.settings.Layout(gtx, e.Th, lithic.Quiet, lithic.Icon("settings"), "Settings")
			}),
		)
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
	}
}

func makeDownloadEntries(active, history []downloadItem) []listEntry {
	entries := make([]listEntry, 0, len(active)+len(history)+5)
	entries = append(entries, listEntry{kind: entryActiveHeader})
	if len(active) == 0 {
		entries = append(entries, listEntry{kind: entryActiveEmpty})
	} else {
		for _, item := range active {
			entries = append(entries, listEntry{kind: entryDownload, item: item})
		}
	}
	entries = append(entries, listEntry{kind: entryHistoryHeader}, listEntry{kind: entryHistoryNote})
	if len(history) == 0 {
		entries = append(entries, listEntry{kind: entryHistoryEmpty})
	} else {
		for _, item := range history {
			entries = append(entries, listEntry{kind: entryDownload, item: item})
		}
	}
	return entries
}

func (p *Page) layoutEntry(gtx layout.Context, e *env.Env, entry listEntry, activeCount, historyCount int) layout.Dimensions {
	switch entry.kind {
	case entryActiveHeader:
		return lithic.GroupHeader(e.Th, "Active and queued", activeCount)(gtx)
	case entryActiveEmpty:
		return quietDownloadLine(gtx, e, "Nothing downloading right now.")
	case entryHistoryHeader:
		return lithic.GroupHeader(e.Th, "History", historyCount)(gtx)
	case entryHistoryNote:
		return quietDownloadLine(gtx, e, "Latest 200 downloads. Clearing history keeps your music files.")
	case entryHistoryEmpty:
		return quietDownloadLine(gtx, e, "Finished downloads will appear here.")
	default:
		if entry.item.collection != nil {
			group := *entry.item.collection
			card := p.collections[group.key]
			if card == nil {
				card = &collectionCard{}
				p.collections[group.key] = card
			}
			return card.layout(gtx, e, group, p.rows)
		}
		if entry.item.job == nil {
			return layout.Dimensions{}
		}
		job := *entry.item.job
		row := p.rows[job.ID]
		if row == nil {
			row = &downloadRow{}
			p.rows[job.ID] = row
		}
		return row.layout(gtx, e, job)
	}
}

func quietDownloadLine(gtx layout.Context, e *env.Env, text string) layout.Dimensions {
	return layout.Inset{Left: e.Th.Sp.S3, Right: e.Th.Sp.S3, Top: e.Th.Sp.S2, Bottom: e.Th.Sp.S3}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return lithic.Label{Style: e.Th.T.Body, Color: e.Th.R.Fg3, MaxLines: 3}.Layout(gtx, e.Th, text)
	})
}

func downloadsSubtitle(jobs []model.DownloadEntry) string {
	for _, job := range jobs {
		if path := strings.TrimSpace(job.Settings.Path); path != "" {
			return "Saving to " + path
		}
	}
	return "Save a track with the download arrow beside its heart in the player."
}

func terminalCount(jobs []model.DownloadEntry) int {
	count := 0
	for _, job := range jobs {
		if job.Status == model.DownloadStatusCompleted || job.Status == model.DownloadStatusFailed || job.Status == model.DownloadStatusCancelled {
			count++
		}
	}
	return min(count, 200)
}

func downloadsSentence(active, queued int) string {
	if active == 0 && queued == 0 {
		return "Nothing downloading right now."
	}
	parts := make([]string, 0, 2)
	if active > 0 {
		parts = append(parts, countSentence(active, "downloading"))
	}
	if queued > 0 {
		parts = append(parts, countSentence(queued, "waiting"))
	}
	return strings.Join(parts, " ")
}

func countSentence(count int, state string) string {
	value := fmt.Sprintf("%d", count)
	words := []string{"Zero", "One", "Two", "Three", "Four", "Five", "Six", "Seven", "Eight", "Nine"}
	if count >= 0 && count < len(words) {
		value = words[count]
	} else if first, size := utf8.DecodeRuneInString(value); first != utf8.RuneError {
		value = string(unicode.ToUpper(first)) + value[size:]
	}
	return value + " " + state + "."
}
