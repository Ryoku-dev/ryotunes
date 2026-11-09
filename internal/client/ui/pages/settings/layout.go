package settings

import (
	"image"

	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryotunes/internal/client/state"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/widgets"
)

// Layout draws the responsive Lithic settings archetype.
func (p *page) Layout(gtx layout.Context, e *env.Env) layout.Dimensions {
	if !p.refreshed {
		p.Refresh(e)
	}
	snapshot := e.Store.Snapshot()
	if p.retry.Clicked(gtx) {
		p.Refresh(e)
	}
	if p.loading && len(snapshot.Settings) == 0 {
		return widgets.Loading(e, widgets.LoadingTable)(gtx)
	}
	if p.loadError != "" && len(snapshot.Settings) == 0 {
		p.retry.Kind, p.retry.Label = kit.PlaqueLine, "Retry"
		return widgets.Failed(e, p.loadError, &p.retry)(gtx)
	}
	return layout.Inset{Top: unit.Dp(34), Right: unit.Dp(36), Bottom: unit.Dp(34), Left: unit.Dp(36)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		switch e.Th.Class {
		case look.Expanded:
			return p.layoutExpanded(gtx, e, snapshot)
		case look.Medium:
			return p.layoutMedium(gtx, e, snapshot)
		default:
			return p.layoutCompact(gtx, e, snapshot)
		}
	})
}

func sectionLabels() []string {
	labels := make([]string, len(settingsSections))
	for i, section := range settingsSections {
		labels[i] = section.title
	}
	return labels
}

func (p *page) layoutExpanded(gtx layout.Context, e *env.Env, snapshot state.State) layout.Dimensions {
	width := min(gtx.Constraints.Max.X, gtx.Dp(994))
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X, gtx.Constraints.Max.X = width, width
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Start}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				dims := p.index.Layout(gtx, e.Th.Theme, sectionLabels(), p.currentSection, kit.FrameExpanded)
				p.consumeIndexSelection()
				return dims
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(34)}.Layout),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(760))
				return p.layoutSections(gtx, e, snapshot)
			}),
		)
	})
}

func (p *page) layoutMedium(gtx layout.Context, e *env.Env, snapshot state.State) layout.Dimensions {
	width := min(gtx.Constraints.Max.X, gtx.Dp(760))
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X, gtx.Constraints.Max.X = width, width
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				dims := p.index.Layout(gtx, e.Th.Theme, sectionLabels(), p.currentSection, kit.FrameMedium)
				p.consumeIndexSelection()
				return dims
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(21)}.Layout),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return p.layoutSections(gtx, e, snapshot) }),
		)
	})
}

func (p *page) layoutCompact(gtx layout.Context, e *env.Env, snapshot state.State) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(kit.Text(e.Th.Theme, e.Th.T.Section, e.Th.R.Fg, "Settings")),
		layout.Rigid(layout.Spacer{Height: unit.Dp(21)}.Layout),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return p.layoutSections(gtx, e, snapshot) }),
	)
}

func (p *page) consumeIndexSelection() {
	if selected, ok := p.index.Selected(); ok {
		p.scroll.ScrollTo(selected)
		p.currentSection = selected
		p.indexHold = selected
	}
}

func (p *page) layoutSections(gtx layout.Context, e *env.Env, snapshot state.State) layout.Dimensions {
	if !p.initialScroll {
		p.scroll.ScrollTo(p.initialSection)
		p.currentSection = p.initialSection
		p.indexHold = p.initialSection
		p.initialScroll = true
	}
	dims := p.scroll.Layout(gtx, e.Th, len(settingsSections), func(gtx layout.Context, index int) layout.Dimensions {
		section := settingsSections[index]
		sectionWidget := func(gtx layout.Context) layout.Dimensions {
			dims := kit.Section(e.Th.Theme, section.title, func(gtx layout.Context) layout.Dimensions {
				return kit.SlabContent(gtx, e.Th.Theme, p.sectionBody(index, e, snapshot))
			})(gtx)
			p.sectionHeights[index] = dims.Size.Y
			return dims
		}
		if index+1 < len(settingsSections) {
			return layout.Inset{Bottom: unit.Dp(34)}.Layout(gtx, sectionWidget)
		}
		return sectionWidget(gtx)
	})
	p.updateCurrentSection()
	return dims
}

func (p *page) sectionBody(index int, e *env.Env, snapshot state.State) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		switch settingsSections[index].slug {
		case "account":
			return p.layoutAccount(gtx, e, snapshot)
		case "playback":
			return p.layoutPlayback(gtx, e, snapshot)
		case "sound":
			return p.layoutSound(gtx, e, snapshot)
		case "lyrics":
			return p.layoutLyrics(gtx, e, snapshot)
		case "downloads":
			return p.layoutDownloads(gtx, e)
		case "local":
			return p.layoutLocal(gtx, e)
		case "playlists":
			return p.layoutPlaylists(gtx, e, snapshot)
		case "appearance":
			return p.layoutAppearance(gtx, e, snapshot)
		case "integrations":
			return p.layoutIntegrations(gtx, e, snapshot)
		case "data":
			return p.layoutData(gtx, e)
		case "diagnostics":
			return p.layoutDiagnostics(gtx, e)
		default:
			return p.layoutAbout(gtx, e)
		}
	}
}

func (p *page) updateCurrentSection() {
	first, offset := p.scroll.Offset()
	first = min(max(first, 0), len(settingsSections)-1)
	if p.indexHold >= 0 {
		if first == p.indexHold && offset == 0 {
			p.currentSection = p.indexHold
			return
		}
		p.indexHold = -1
	}
	current := first
	height := p.sectionHeights[current]
	if current+1 < len(settingsSections) && height > 0 && -offset > height*2/3 {
		current++
	}
	if p.scroll.Position.First+p.scroll.Position.Count >= len(settingsSections) && p.scroll.Position.OffsetLast >= 0 {
		current = len(settingsSections) - 1
	}
	p.currentSection = current
}

func settingRows(gtx layout.Context, rows ...layout.Widget) layout.Dimensions {
	children := make([]layout.FlexChild, 0, len(rows)*2)
	for i, row := range rows {
		children = append(children, layout.Rigid(row))
		if i+1 < len(rows) {
			children = append(children, layout.Rigid(layout.Spacer{Height: unit.Dp(13)}.Layout))
		}
	}
	return layout.Flex{Axis: layout.Vertical, Alignment: layout.Start}.Layout(gtx, children...)
}

func settingRow(th *look.Theme, title, detail string, control layout.Widget) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		copyBlock := func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical, Alignment: layout.Start}.Layout(gtx,
				layout.Rigid(kit.Text(th.Theme, th.T.Row, th.R.Fg, title)),
				layout.Rigid(layout.Spacer{Height: unit.Dp(2)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return kit.Label{Style: th.T.Caption, Color: th.R.Fg2, MaxLines: 3}.Layout(gtx, th.Theme, detail)
				}),
			)
		}
		if control == nil {
			return copyBlock(gtx)
		}
		controlWithDescription := func(gtx layout.Context) layout.Dimensions {
			description := title
			if detail != "" {
				description += ". " + detail
			}
			semantic.DescriptionOp(description).Add(gtx.Ops)
			gtx.Constraints.Min = image.Point{}
			gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(320))
			return control(gtx)
		}
		if gtx.Constraints.Max.X < gtx.Dp(560) {
			return layout.Flex{Axis: layout.Vertical, Alignment: layout.Start}.Layout(gtx,
				layout.Rigid(copyBlock),
				layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return layout.E.Layout(gtx, controlWithDescription) }),
			)
		}
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Start}.Layout(gtx,
			layout.Flexed(1, copyBlock),
			layout.Rigid(layout.Spacer{Width: unit.Dp(21)}.Layout),
			layout.Rigid(controlWithDescription),
		)
	}
}
