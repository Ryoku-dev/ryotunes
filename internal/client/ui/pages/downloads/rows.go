package downloads

import (
	"fmt"
	"image"
	"path/filepath"
	"strings"

	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/unit"
	"gioui.org/widget"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/model"
)

type contextRelease struct {
	pending   bool
	pointerID pointer.ID
}

func (release *contextRelease) poll(gtx layout.Context, tag any, run func()) {
	for {
		raw, ok := gtx.Event(pointer.Filter{Target: tag, Kinds: pointer.Press | pointer.Release | pointer.Cancel})
		if !ok {
			return
		}
		event := raw.(pointer.Event)
		switch event.Kind {
		case pointer.Press:
			release.pending = event.Buttons.Contain(pointer.ButtonSecondary)
			release.pointerID = event.PointerID
		case pointer.Release:
			if release.pending && event.PointerID == release.pointerID {
				release.pending = false
				run()
			}
		case pointer.Cancel:
			if event.PointerID == release.pointerID {
				release.pending = false
			}
		}
	}
}

type downloadRow struct {
	tablet     kit.Tablet
	action     kit.Plaque
	progress   kit.BladeProgress
	menu       kit.Menu
	menuTag    int
	contextTag struct{}
	context    contextRelease
}

type collectionCard struct {
	tablet     kit.Tablet
	action     kit.Plaque
	expanded   bool
	progress   kit.BladeProgress
	menu       kit.Menu
	menuTag    int
	contextTag struct{}
	context    contextRelease
}

func (r *downloadRow) layout(gtx layout.Context, e *env.Env, job model.DownloadEntry, visible []downloadItem) layout.Dimensions {
	r.handleContext(gtx, e, job, visible)
	if r.action.Clicked(gtx) {
		primaryDownloadAction(e, job, visible)
	}
	if r.tablet.DoubleClicked(gtx) && job.Status == model.DownloadStatusCompleted {
		playDownload(e, job, visible)
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
	dims := layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	pass := pointer.PassOp{}.Push(gtx.Ops)
	area := clip.Rect(image.Rectangle{Max: dims.Size}).Push(gtx.Ops)
	event.Op(gtx.Ops, &r.contextTag)
	area.Pop()
	pass.Pop()
	return dims
}

func (r *downloadRow) handleContext(gtx layout.Context, e *env.Env, job model.DownloadEntry, visible []downloadItem) {
	r.context.poll(gtx, &r.contextTag, func() {
		r.openMenu(e, job, visible)
	})
}

func (r *downloadRow) openMenu(e *env.Env, job model.DownloadEntry, visible []downloadItem) {
	if e == nil || e.Overlays == nil || job.Status != model.DownloadStatusCompleted {
		return
	}
	context, start := savedDownloadContext(visible, job.ID)
	items := []kit.MenuItem{
		{Icon: look.Icon.Play, Label: "Play"},
		{Icon: look.Icon.Folder, Label: "Show in folder"},
	}
	e.Overlays.OpenAt(&r.menuTag, e.Overlays.Press(), func(gtx layout.Context) layout.Dimensions {
		dims := r.menu.Layout(gtx, e.Th.Theme, items)
		if selected, ok := r.menu.Clicked(gtx); ok {
			e.Overlays.Close(&r.menuTag)
			switch selected {
			case 0:
				playDownloadTracks(e, job, context, start)
			case 1:
				openDownload(e, job.ID)
			}
		}
		return dims
	})
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
		return palette.Alive, "Saved", proof, -1, false, "Play", look.Icon.Play
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

func (c *collectionCard) layout(gtx layout.Context, e *env.Env, group downloadCollection, rows map[string]*downloadRow, visible []downloadItem, all []model.DownloadEntry) layout.Dimensions {
	c.handleContext(gtx, e, group.jobs, all)
	stats := statsFor(group.jobs)
	if c.action.Clicked(gtx) {
		actOnCollection(e, group.jobs, stats, all)
	}
	if c.tablet.DoubleClicked(gtx) && stats.done > 0 && stats.moving+stats.waiting+stats.failed+stats.cancelled == 0 {
		actOnCollection(e, group.jobs, stats, all)
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
	var tabletSize image.Point
	children := []layout.FlexChild{layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		dims := c.tablet.Layout(gtx, e.Th.Theme, content, false, false)
		tabletSize = dims.Size
		return dims
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
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx, collectionChildren(e, group.jobs, rows, visible)...)
			})
		}))
	}
	dims := layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	pass := pointer.PassOp{}.Push(gtx.Ops)
	area := clip.Rect(image.Rectangle{Max: tabletSize}).Push(gtx.Ops)
	event.Op(gtx.Ops, &c.contextTag)
	area.Pop()
	pass.Pop()
	return dims
}

func collectionChildren(e *env.Env, jobs []model.DownloadEntry, rows map[string]*downloadRow, visible []downloadItem) []layout.FlexChild {
	children := make([]layout.FlexChild, 0, len(jobs)*2)
	for i := range jobs {
		job := jobs[i]
		row := rows[job.ID]
		if row == nil {
			row = new(downloadRow)
			rows[job.ID] = row
		}
		children = append(children,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return row.layout(gtx, e, job, visible) }),
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
	case stats.cancelled > 0:
		return palette.Idle, "Cancelled", fmt.Sprintf("%d cancelled", stats.cancelled), -1, false, "Retry", look.Icon.Refresh
	case stats.done > 0:
		return palette.Alive, "Saved", fmt.Sprintf("%d saved", stats.done), -1, false, "Play", look.Icon.Play
	default:
		return palette.Idle, "Cancelled", "Ready to retry", -1, false, "Retry", look.Icon.Refresh
	}
}

func actOnCollection(e *env.Env, jobs []model.DownloadEntry, stats downloadStats, all []model.DownloadEntry) {
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
	playCollectionDownloads(e, jobs, all)
}

func playCollectionDownloads(e *env.Env, jobs, all []model.DownloadEntry) {
	tracks := savedJobTracks(jobs, all)
	if len(tracks) > 0 {
		act.PlayCollection(e, tracks, 0, "", "Downloads", false, nil)
	}
}

func (c *collectionCard) handleContext(gtx layout.Context, e *env.Env, jobs, all []model.DownloadEntry) {
	c.context.poll(gtx, &c.contextTag, func() {
		c.openMenu(e, jobs, all)
	})
}

func (c *collectionCard) openMenu(e *env.Env, jobs, all []model.DownloadEntry) {
	if e == nil || e.Overlays == nil {
		return
	}
	tracks := savedJobTracks(jobs, all)
	if len(tracks) == 0 {
		return
	}
	items := []kit.MenuItem{{Icon: look.Icon.Play, Label: "Play saved tracks"}}
	e.Overlays.OpenAt(&c.menuTag, e.Overlays.Press(), func(gtx layout.Context) layout.Dimensions {
		dims := c.menu.Layout(gtx, e.Th.Theme, items)
		if _, ok := c.menu.Clicked(gtx); ok {
			e.Overlays.Close(&c.menuTag)
			act.PlayCollection(e, tracks, 0, "", "Downloads", false, nil)
		}
		return dims
	})
}
