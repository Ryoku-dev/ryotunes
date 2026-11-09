package downloads

import (
	"fmt"
	"image"
	"path/filepath"
	"strings"

	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/model"
)

type downloadRow struct {
	tablet   kit.Tablet
	action   kit.Plaque
	progress kit.BladeProgress
}

type collectionCard struct {
	tablet   kit.Tablet
	action   kit.Plaque
	expanded bool
	progress kit.BladeProgress
}

func (r *downloadRow) layout(gtx layout.Context, e *env.Env, job model.DownloadEntry) layout.Dimensions {
	if r.action.Clicked(gtx) || r.tablet.DoubleClicked(gtx) {
		primaryDownloadAction(e, job)
	}
	tone, claim, proof, progress, showProgress, action, icon := downloadPresentation(job)
	r.action.Kind, r.action.Icon, r.action.Label, r.action.Small = kit.PlaqueQuiet, icon, action, true
	mark := func(gtx layout.Context) layout.Dimensions {
		size := gtx.Constraints.Max
		if size.X <= 0 || size.Y <= 0 {
			size = image.Pt(gtx.Dp(48), gtx.Dp(48))
		}
		if job.Thumbnail != "" {
			return look.Art(gtx, e.Th, e.Art, job.Thumbnail, size, look.Shape{Cut: unit.Dp(6)})
		}
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return look.Icon.Download.Layout(gtx, e.Th.R.Fg2)
		})
	}
	content := kit.TabletContent{
		Mark: mark, Title: job.Title, Facts: strings.TrimSpace(job.Artists),
		Claim: claim, Proof: proof, Tone: tone, Progress: -1, Action: &r.action,
	}
	children := []layout.FlexChild{layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return r.tablet.Layout(gtx, e.Th.Theme, content, false, false)
	})}
	if showProgress {
		r.progress.Fraction, r.progress.Active = progress, job.Status == model.DownloadStatusDownloading
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: unit.Dp(16), Right: unit.Dp(12), Top: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return r.progress.Layout(gtx, e.Th.Theme)
			})
		}))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

func downloadPresentation(job model.DownloadEntry) (palette.Tone, string, string, float64, bool, string, *widget.Icon) {
	switch job.Status {
	case model.DownloadStatusDownloading:
		progress := max(0, min(100, job.Progress))
		return palette.Becoming, "Downloading", fmt.Sprintf("%d%%", progress), float64(progress) / 100, true, "Cancel", look.Icon.Close
	case model.DownloadStatusQueued:
		return palette.Attention, "Waiting", "Queued", 0, true, "Cancel", look.Icon.Close
	case model.DownloadStatusCompleted:
		proof := "Saved"
		if job.FilePath != nil && strings.TrimSpace(*job.FilePath) != "" {
			proof = filepath.Base(*job.FilePath)
		}
		return palette.Alive, "Saved", proof, -1, false, "Show in folder", look.Icon.Folder
	case model.DownloadStatusFailed:
		proof := "Download failed"
		if job.Error != nil && strings.TrimSpace(*job.Error) != "" {
			proof = strings.TrimSpace(*job.Error)
		}
		return palette.Failed, "Failed", proof, -1, false, "Retry", look.Icon.Refresh
	default:
		return palette.Idle, "Cancelled", "Ready to retry", -1, false, "Retry", look.Icon.Refresh
	}
}

func (c *collectionCard) layout(gtx layout.Context, e *env.Env, group downloadCollection, rows map[string]*downloadRow) layout.Dimensions {
	stats := statsFor(group.jobs)
	if c.action.Clicked(gtx) || c.tablet.DoubleClicked(gtx) {
		actOnCollection(e, group.jobs, stats)
	}
	if c.tablet.Clicked(gtx) {
		c.expanded = !c.expanded
	}
	tone, claim, proof, progress, showProgress, action, icon := collectionPresentation(stats, len(group.jobs))
	c.action.Kind, c.action.Icon, c.action.Label, c.action.Small = kit.PlaqueQuiet, icon, action, true
	content := kit.TabletContent{
		Mark:  func(gtx layout.Context) layout.Dimensions { return look.Icon.Album.Layout(gtx, e.Th.R.Fg2) },
		Title: group.label, Facts: stats.summary(), Claim: claim, Proof: proof,
		Tone: tone, Progress: -1, Action: &c.action,
	}
	children := []layout.FlexChild{layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return c.tablet.Layout(gtx, e.Th.Theme, content, false, false)
	})}
	if showProgress {
		c.progress.Fraction, c.progress.Active = progress, stats.moving > 0
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: unit.Dp(16), Right: unit.Dp(12), Top: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return c.progress.Layout(gtx, e.Th.Theme)
			})
		}))
	}
	if c.expanded {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: unit.Dp(21), Top: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx, collectionChildren(e, group.jobs, rows)...)
			})
		}))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

func collectionChildren(e *env.Env, jobs []model.DownloadEntry, rows map[string]*downloadRow) []layout.FlexChild {
	children := make([]layout.FlexChild, 0, len(jobs)*2)
	for i := range jobs {
		job := jobs[i]
		row := rows[job.ID]
		if row == nil {
			row = new(downloadRow)
			rows[job.ID] = row
		}
		children = append(children,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return row.layout(gtx, e, job) }),
			layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
		)
	}
	return children
}

func collectionPresentation(stats downloadStats, total int) (palette.Tone, string, string, float64, bool, string, *widget.Icon) {
	switch {
	case stats.moving > 0:
		return palette.Becoming, "Downloading", fmt.Sprintf("%d of %d", stats.done+stats.moving, total), float64(stats.average) / 100, true, "Cancel", look.Icon.Close
	case stats.waiting > 0:
		return palette.Attention, "Waiting", fmt.Sprintf("%d queued", stats.waiting), 0, true, "Cancel", look.Icon.Close
	case stats.failed > 0:
		return palette.Failed, "Failed", fmt.Sprintf("%d failed", stats.failed), -1, false, "Retry", look.Icon.Refresh
	case stats.done > 0:
		return palette.Alive, "Saved", fmt.Sprintf("%d saved", stats.done), -1, false, "Show in folder", look.Icon.Folder
	default:
		return palette.Idle, "Cancelled", fmt.Sprintf("%d cancelled", stats.cancelled), -1, false, "Retry", look.Icon.Refresh
	}
}

func actOnCollection(e *env.Env, jobs []model.DownloadEntry, stats downloadStats) {
	if stats.moving+stats.waiting > 0 {
		for _, job := range jobs {
			if job.Status == model.DownloadStatusDownloading || job.Status == model.DownloadStatusQueued {
				cancelDownload(e, job.ID)
			}
		}
		return
	}
	if stats.failed+stats.cancelled > 0 {
		for _, job := range jobs {
			if job.Status == model.DownloadStatusFailed || job.Status == model.DownloadStatusCancelled {
				retryDownload(e, job.ID)
			}
		}
		return
	}
	for _, job := range jobs {
		if job.Status == model.DownloadStatusCompleted {
			openDownload(e, job.ID)
			return
		}
	}
}
