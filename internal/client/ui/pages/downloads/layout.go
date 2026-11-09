package downloads

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gioui.org/layout"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/widgets"
	"github.com/ryoku-dev/ryotunes/internal/model"
)

const (
	downloadFilterAll = iota
	downloadFilterActive
	downloadFilterSaved
	downloadFilterFailed
)

// Layout renders the live queue and terminal download history.
func (p *Page) Layout(gtx layout.Context, e *env.Env) layout.Dimensions {
	snapshot := e.Store.Snapshot().Downloads
	p.updateUsedSpace(snapshot.Jobs)
	counts := downloadFilterCounts(snapshot.Jobs)
	p.filter.Labels = []string{"All", "Active", "Saved", "Failed"}
	p.filter.Counts = counts
	p.filter.Selected = p.filterIndex
	if selected, ok := p.filter.Changed(); ok {
		p.filterIndex = selected
		p.scroll.ScrollTo(0)
	}
	p.handleActions(gtx, e, snapshot.Jobs)
	items := filteredDownloads(snapshot.Jobs, p.filterIndex)
	inset := look.ContentInset(e.Th.Class)
	count := 2 + max(1, len(items))
	return p.scroll.Layout(gtx, e.Th, count, func(gtx layout.Context, index int) layout.Dimensions {
		if index == 0 {
			return p.layoutHeader(gtx, e, counts[downloadFilterActive], counts[downloadFilterSaved])
		}
		return layout.Inset{Right: inset, Left: inset}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			if index == 1 {
				return p.layoutControls(gtx, e)
			}
			rowIndex := index - 2
			slot := func(gtx layout.Context) layout.Dimensions {
				return p.layoutDownloadSlot(gtx, e, snapshot.Jobs, items, rowIndex)
			}
			if index+1 == count {
				return layout.Inset{Bottom: inset}.Layout(gtx, slot)
			}
			return slot(gtx)
		})
	})
}

func (p *Page) handleActions(gtx layout.Context, e *env.Env, jobs []model.DownloadEntry) {
	if p.openFolder.Clicked(gtx) {
		openDownloadFolder(e)
	}
	if p.clear.Clicked(gtx) && terminalCount(jobs) > 0 && e.Confirm != nil {
		e.Confirm("Clear download history?", "Clearing history keeps your music files.", "Clear history", "", func() { clearHistory(e) })
	}
	if p.settings.Clicked(gtx) {
		e.Nav.Push(env.Route{Kind: env.RouteSettings, Params: "downloads"})
	}
}

func (p *Page) layoutHeader(gtx layout.Context, e *env.Env, active, saved int) layout.Dimensions {
	cut := gtx.Dp(e.Th.Cut.Large)
	inset := look.ContentInset(e.Th.Class)
	return look.Field(gtx, e.Th, [4]int{cut, cut, 0, 0}, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: inset, Right: inset, Bottom: inset, Left: inset}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical, Alignment: layout.Start}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return kit.Label{Style: e.Th.T.Section, Color: e.Th.R.Fg, MaxLines: 1}.Layout(gtx, e.Th.Theme, "Downloads")
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return kit.Sentence(gtx, e.Th.Theme, e.Th.T.Proof, e.Th.R.Fg2, []kit.Span{
						{Text: strconv.Itoa(active), Mono: true}, {Text: " downloading, "},
						{Text: strconv.Itoa(saved), Mono: true}, {Text: " saved"},
					})
				}),
			)
		})
	})
}

func (p *Page) layoutControls(gtx layout.Context, e *env.Env) layout.Dimensions {
	p.openFolder.Kind, p.openFolder.Icon, p.openFolder.Label = kit.PlaqueQuiet, look.Icon.Folder, "Open folder"
	p.clear.Kind, p.clear.Label = kit.PlaqueQuiet, "Clear history"
	p.settings.Kind, p.settings.Icon, p.settings.Label = kit.PlaqueQuiet, look.Icon.Settings, "Settings"
	children := []layout.FlexChild{
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return p.filter.Layout(gtx, e.Th.Theme) }),
		layout.Rigid(layout.Spacer{Width: unit.Dp(13)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return p.openFolder.Layout(gtx, e.Th.Theme) }),
	}
	if p.filter.Counts[downloadFilterSaved]+p.filter.Counts[downloadFilterFailed] > 0 && e.Th.Class != look.Compact {
		children = append(children,
			layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return p.clear.Layout(gtx, e.Th.Theme) }),
		)
	}
	if e.Th.Class != look.Compact {
		children = append(children,
			layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return p.settings.Layout(gtx, e.Th.Theme) }),
		)
	}
	return layout.Inset{Top: unit.Dp(21), Bottom: unit.Dp(21)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
	})
}

func (p *Page) layoutDownloadSlot(gtx layout.Context, e *env.Env, jobs []model.DownloadEntry, items []downloadItem, index int) layout.Dimensions {
	wide := gtx.Constraints.Max.X >= gtx.Dp(1000)
	factsWidth := gtx.Dp(300)
	gap := gtx.Dp(34)
	rowWidth := gtx.Constraints.Max.X
	if wide {
		rowWidth -= factsWidth + gap
	}
	row := func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X, gtx.Constraints.Max.X = rowWidth, rowWidth
		if len(items) == 0 {
			if index > 0 {
				return layout.Dimensions{}
			}
			return widgets.EmptyState(e, look.Icon.Download, "No downloads here", "Tracks matching this filter will appear here.")(gtx)
		}
		return p.layoutItem(gtx, e, items[index])
	}
	if !wide || index > 0 {
		return layout.Inset{Bottom: unit.Dp(4)}.Layout(gtx, row)
	}
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Start}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return layout.Inset{Bottom: unit.Dp(4)}.Layout(gtx, row) }),
		layout.Rigid(layout.Spacer{Width: unit.Dp(34)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X, gtx.Constraints.Max.X = factsWidth, factsWidth
			return kit.Section(e.Th.Theme, "Storage", func(gtx layout.Context) layout.Dimensions {
				return kit.SlabContent(gtx, e.Th.Theme, kit.Facts(e.Th.Theme, downloadFacts(jobs, p.usedBytes)))
			})(gtx)
		}),
	)
}

func (p *Page) layoutItem(gtx layout.Context, e *env.Env, item downloadItem) layout.Dimensions {
	if item.collection != nil {
		group := *item.collection
		card := p.collections[group.key]
		if card == nil {
			card = new(collectionCard)
			p.collections[group.key] = card
		}
		return card.layout(gtx, e, group, p.rows)
	}
	if item.job == nil {
		return layout.Dimensions{}
	}
	job := *item.job
	row := p.rows[job.ID]
	if row == nil {
		row = new(downloadRow)
		p.rows[job.ID] = row
	}
	return row.layout(gtx, e, job)
}

func filteredDownloads(jobs []model.DownloadEntry, selected int) []downloadItem {
	active, history, _, _ := projectDownloads(jobs)
	switch selected {
	case downloadFilterActive:
		return active
	case downloadFilterSaved:
		return collapseCollections(filterJobs(jobs, func(status model.DownloadStatus) bool { return status == model.DownloadStatusCompleted }), "saved")
	case downloadFilterFailed:
		return collapseCollections(filterJobs(jobs, func(status model.DownloadStatus) bool {
			return status == model.DownloadStatusFailed || status == model.DownloadStatusCancelled
		}), "failed")
	default:
		return append(active, history...)
	}
}

func filterJobs(jobs []model.DownloadEntry, keep func(model.DownloadStatus) bool) []model.DownloadEntry {
	filtered := make([]model.DownloadEntry, 0, len(jobs))
	for _, job := range jobs {
		if keep(job.Status) {
			filtered = append(filtered, job)
		}
	}
	return filtered
}

func downloadFilterCounts(jobs []model.DownloadEntry) []int {
	counts := make([]int, 4)
	counts[downloadFilterAll] = len(jobs)
	for _, job := range jobs {
		switch job.Status {
		case model.DownloadStatusDownloading, model.DownloadStatusQueued:
			counts[downloadFilterActive]++
		case model.DownloadStatusCompleted:
			counts[downloadFilterSaved]++
		case model.DownloadStatusFailed, model.DownloadStatusCancelled:
			counts[downloadFilterFailed]++
		}
	}
	return counts
}

func downloadFacts(jobs []model.DownloadEntry, used int64) []kit.Fact {
	location, format := "Not set", "Original"
	for _, job := range jobs {
		if path := strings.TrimSpace(job.Settings.Path); path != "" {
			location = path
		}
		if job.Settings.Format != "" {
			format = strings.ToUpper(string(job.Settings.Format))
		}
		if location != "Not set" && format != "Original" {
			break
		}
	}
	return []kit.Fact{
		{Label: "Location", Value: location, Mono: true},
		{Label: "Used space", Value: formatBytes(used), Mono: true},
		{Label: "Format", Value: format},
	}
}

func (p *Page) updateUsedSpace(jobs []model.DownloadEntry) {
	var revision int64
	for _, job := range jobs {
		if job.Status != model.DownloadStatusCompleted || job.FilePath == nil {
			continue
		}
		if job.FinishedAt != nil {
			revision += *job.FinishedAt
		}
		revision += int64(len(*job.FilePath))
	}
	if revision == p.usedRevision {
		return
	}
	p.usedRevision = revision
	p.usedBytes = 0
	for _, job := range jobs {
		if job.Status != model.DownloadStatusCompleted || job.FilePath == nil {
			continue
		}
		if info, err := os.Stat(filepath.Clean(*job.FilePath)); err == nil && info.Mode().IsRegular() {
			p.usedBytes += info.Size()
		}
	}
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

func formatBytes(bytes int64) string {
	const (
		kib = int64(1024)
		mib = 1024 * kib
		gib = 1024 * mib
	)
	switch {
	case bytes >= gib:
		return fmt.Sprintf("%.1f GiB", float64(bytes)/float64(gib))
	case bytes >= mib:
		return fmt.Sprintf("%.1f MiB", float64(bytes)/float64(mib))
	case bytes >= kib:
		return fmt.Sprintf("%.1f KiB", float64(bytes)/float64(kib))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
