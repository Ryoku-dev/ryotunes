package downloads

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gioui.org/layout"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/sheet"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/widgets"
	"github.com/ryoku-dev/ryotunes/internal/model"
)

var downloadFilterLabels = []string{"All", "Active", "Saved", "Failed"}

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
	p.filter.Labels = downloadFilterLabels
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
		return p.reveal.Group(gtx, e.Th, index, func(gtx layout.Context) layout.Dimensions {
			if index == 0 {
				return layout.Inset{Top: e.Th.Sp.S2}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return p.layoutHeader(gtx, e, counts)
				})
			}
			if index == 2 && len(items) == 0 {
				return p.layoutDownloadSlot(gtx, e, snapshot.Jobs, items, 0)
			}
			return layout.Inset{Right: inset, Left: inset}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				if index == 1 {
					return p.layoutControls(gtx, e, len(items) == 0)
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
	})
}

// Masthead names the filter shown on the frame's masthead and says how far the list is
// read.
func (p *Page) Masthead() (part, partGloss string, progress float32) {
	if p.filterIndex > 0 {
		part = downloadFilterLabels[p.filterIndex]
	}
	return part, "", p.scroll.Progress()
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

// layoutHeader is the page head: the lead, and the queue as the spec strip, ACTIVE, SAVED
// and FAILED, the failures lit while there are any.
func (p *Page) layoutHeader(gtx layout.Context, e *env.Env, counts []int) layout.Dimensions {
	p.header.Place = sheet.Downloads
	p.header.Part = ""
	if p.filterIndex > 0 {
		p.header.Part = downloadFilterLabels[p.filterIndex]
	}
	p.strip.Set(0, "Active", counts[downloadFilterActive], counts[downloadFilterActive] > 0)
	p.strip.Set(1, "Saved", counts[downloadFilterSaved], false)
	p.strip.Set(2, "Failed", counts[downloadFilterFailed], counts[downloadFilterFailed] > 0)
	p.header.Specs = p.strip.Specs(true)
	return p.header.Layout(gtx, e, "Downloads", "Saved songs play from their files, even offline.", nil)
}

func (p *Page) layoutControls(gtx layout.Context, e *env.Env, empty bool) layout.Dimensions {
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
	bottom := unit.Dp(21)
	if empty {
		bottom = 0
	}
	return layout.Inset{Top: unit.Dp(21), Bottom: bottom}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
	})
}

// layoutDownloadSlot lays out one row, and the Storage facts beside the first
// one when wide. With no items the slot is the page state, which arrives
// without the page inset because it owns its own.
func (p *Page) layoutDownloadSlot(gtx layout.Context, e *env.Env, jobs []model.DownloadEntry, items []downloadItem, index int) layout.Dimensions {
	empty := len(items) == 0
	inset := gtx.Dp(look.ContentInset(e.Th.Class))
	content := gtx.Constraints.Max.X
	if empty {
		content -= 2 * inset
	}
	wide := content >= gtx.Dp(1000)
	factsWidth := gtx.Dp(300)
	gap := gtx.Dp(34)
	rowWidth := gtx.Constraints.Max.X
	if wide {
		rowWidth -= factsWidth + gap
		if empty {
			rowWidth -= inset
		}
	}
	row := func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X, gtx.Constraints.Max.X = rowWidth, rowWidth
		if empty {
			if index > 0 {
				return layout.Dimensions{}
			}
			return widgets.EmptyState(e, look.Icon.Download, "No downloads here", "Tracks matching this filter will appear here.")(gtx)
		}
		return p.layoutItem(gtx, e, items[index], items, jobs)
	}
	rowWidget := row
	if !empty {
		rowWidget = func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: unit.Dp(4)}.Layout(gtx, row)
		}
	}
	if !wide || index > 0 {
		return rowWidget(gtx)
	}
	rowHeight := 0
	factsInset := layout.Inset{}
	if empty {
		factsInset = layout.Inset{Top: look.ContentInset(e.Th.Class), Right: look.ContentInset(e.Th.Class)}
	}
	dimensions := layout.Flex{Axis: layout.Horizontal, Alignment: layout.Start}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			rowDimensions := rowWidget(gtx)
			rowHeight = rowDimensions.Size.Y
			return rowDimensions
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(34)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return factsInset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.X, gtx.Constraints.Max.X = factsWidth, factsWidth
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return widgets.RuledHead(gtx, e, "Storage", "", nil)
					}),
					layout.Rigid(layout.Spacer{Height: e.Th.Sp.S3}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return kit.SlabContent(gtx, e.Th.Theme, kit.Facts(e.Th.Theme, downloadFacts(jobs, p.usedBytes)))
					}),
				)
			})
		}),
	)
	if empty {
		return dimensions
	}
	// the Storage facts hang beside the rows that follow (their column is kept free), so
	// the first row keeps its own height instead of the facts'
	dimensions.Size.Y = rowHeight
	return dimensions
}

func (p *Page) layoutItem(gtx layout.Context, e *env.Env, item downloadItem, items []downloadItem, jobs []model.DownloadEntry) layout.Dimensions {
	if item.collection != nil {
		group := *item.collection
		card := p.collections[group.key]
		if card == nil {
			card = new(collectionCard)
			p.collections[group.key] = card
		}
		return card.layout(gtx, e, group, p.rows, items, jobs)
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
	return row.layout(gtx, e, job, items)
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
