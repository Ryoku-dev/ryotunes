package downloads

import (
	"fmt"
	"path/filepath"
	"strings"

	"gioui.org/layout"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryotunes/internal/client/lithic"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/widgets"
	"github.com/ryoku-dev/ryotunes/internal/model"
)

type downloadRow struct {
	row    lithic.Row
	action lithic.Button
}

type collectionCard struct {
	row      lithic.Row
	primary  lithic.Button
	expand   lithic.Button
	expanded bool
}

func (r *downloadRow) layout(gtx layout.Context, e *env.Env, job model.DownloadEntry) layout.Dimensions {
	if r.action.Clicked(gtx) || r.row.DoubleClicked(gtx) {
		primaryDownloadAction(e, job)
	}
	tone, claim, proof, progress, showProgress, action, icon := downloadPresentation(job)
	r.action.Small = true
	mark := func(gtx layout.Context) layout.Dimensions {
		if job.Thumbnail != "" {
			return widgets.Art(gtx, e, job.Thumbnail, unit.Dp(32), unit.Dp(6))
		}
		return lithic.Icon("download").Layout(gtx, e.Th.R.Fg2)
	}
	facts := strings.TrimSpace(job.Artists)
	if source := strings.TrimSpace(job.Source); source != "" {
		if facts != "" {
			facts += " · "
		}
		facts += source
	}
	row := func(gtx layout.Context) layout.Dimensions {
		return r.row.Layout(gtx, e.Th, lithic.RowContent{
			Mark:     mark,
			Title:    job.Title,
			Facts:    facts,
			Claim:    claim,
			Proof:    proof,
			Tone:     tone,
			Progress: -1,
			Action: func(gtx layout.Context) layout.Dimensions {
				return r.action.Layout(gtx, e.Th, lithic.Quiet, lithic.Icon(icon), action)
			},
		}, false, false)
	}
	children := []layout.FlexChild{layout.Rigid(row)}
	if showProgress {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: 16, Right: 12, Top: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return lithic.BladeProgress(gtx, e.Th, progress, false)
			})
		}))
	}
	if len(job.Warnings) > 0 {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: 16, Right: 12, Top: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return lithic.Label{Style: e.Th.T.Caption, Color: e.Th.R.Fg3, MaxLines: 2}.Layout(gtx, e.Th, strings.Join(job.Warnings, " · "))
			})
		}))
	}
	children = append(children, layout.Rigid(layout.Spacer{Height: e.Th.Sp.S1}.Layout))
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

func downloadPresentation(job model.DownloadEntry) (lithic.Tone, string, string, float64, bool, string, string) {
	switch job.Status {
	case model.DownloadStatusDownloading:
		progress := max(0, min(100, job.Progress))
		return lithic.Busy, "Downloading", fmt.Sprintf("%d%%", progress), float64(progress) / 100, true, "Cancel", "close"
	case model.DownloadStatusQueued:
		return lithic.Attention, "Waiting", "Waiting in queue", 0, true, "Cancel", "close"
	case model.DownloadStatusCompleted:
		proof := "Saved"
		if job.FilePath != nil && *job.FilePath != "" {
			proof = filepath.Base(*job.FilePath)
		}
		return lithic.Alive, "Saved", proof, -1, false, "Open", "folder"
	case model.DownloadStatusFailed:
		proof := "Download failed"
		if job.Error != nil && strings.TrimSpace(*job.Error) != "" {
			proof = strings.TrimSpace(*job.Error)
		}
		return lithic.Bad, "Failed", proof, -1, false, "Retry", "refresh"
	default:
		return lithic.Idle, "Cancelled", "Cancelled", -1, false, "Retry", "refresh"
	}
}

func (c *collectionCard) layout(gtx layout.Context, e *env.Env, group downloadCollection, rows map[string]*downloadRow) layout.Dimensions {
	if c.expand.Clicked(gtx) {
		c.expanded = !c.expanded
	}
	stats := statsFor(group.jobs)
	if c.primary.Clicked(gtx) || c.row.DoubleClicked(gtx) {
		actOnCollection(e, group.jobs, stats)
	}
	c.primary.Small = true
	c.expand.Small = true
	tone, claim, proof, progress, showProgress, action, icon := collectionPresentation(stats, len(group.jobs))
	row := func(gtx layout.Context) layout.Dimensions {
		return c.row.Layout(gtx, e.Th, lithic.RowContent{
			Mark: func(gtx layout.Context) layout.Dimensions {
				return lithic.Icon("album").Layout(gtx, e.Th.R.Fg2)
			},
			Title:    group.label,
			Facts:    stats.summary(),
			Claim:    claim,
			Proof:    proof,
			Tone:     tone,
			Progress: -1,
			Action: func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return c.primary.Layout(gtx, e.Th, lithic.Quiet, lithic.Icon(icon), action)
					}),
					layout.Rigid(layout.Spacer{Width: e.Th.Sp.S2}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						expandIcon, label := "expand_more", "Expand"
						if c.expanded {
							expandIcon, label = "expand_less", "Collapse"
						}
						return c.expand.Layout(gtx, e.Th, lithic.Quiet, lithic.Icon(expandIcon), label)
					}),
				)
			},
		}, false, false)
	}
	children := []layout.FlexChild{layout.Rigid(row)}
	if showProgress {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: 16, Right: 12, Top: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return lithic.BladeProgress(gtx, e.Th, progress, false)
			})
		}))
	}
	if c.expanded {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: e.Th.Sp.S4, Top: e.Th.Sp.S2}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx, collectionChildren(e, gtx, group.jobs, rows)...)
			})
		}))
	}
	children = append(children, layout.Rigid(layout.Spacer{Height: e.Th.Sp.S1}.Layout))
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

func collectionChildren(e *env.Env, gtx layout.Context, jobs []model.DownloadEntry, rows map[string]*downloadRow) []layout.FlexChild {
	children := make([]layout.FlexChild, 0, len(jobs))
	for i := range jobs {
		job := jobs[i]
		row := rows[job.ID]
		if row == nil {
			row = &downloadRow{}
			rows[job.ID] = row
		}
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return row.layout(gtx, e, job)
		}))
	}
	return children
}

func collectionPresentation(stats downloadStats, total int) (lithic.Tone, string, string, float64, bool, string, string) {
	switch {
	case stats.moving > 0:
		return lithic.Busy, "Downloading", fmt.Sprintf("Downloading %d of %d", stats.done+stats.moving, total), float64(stats.average) / 100, true, "Cancel", "close"
	case stats.waiting > 0:
		return lithic.Attention, "Waiting", fmt.Sprintf("%d waiting", stats.waiting), 0, true, "Cancel", "close"
	case stats.failed > 0:
		return lithic.Bad, "Failed", fmt.Sprintf("%d failed", stats.failed), -1, false, "Retry", "refresh"
	case stats.done > 0:
		return lithic.Alive, "Saved", fmt.Sprintf("%d saved", stats.done), -1, false, "Open folder", "folder"
	default:
		return lithic.Idle, "Cancelled", fmt.Sprintf("%d cancelled", stats.cancelled), -1, false, "Retry", "refresh"
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
