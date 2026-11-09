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
	p.scroll.UpdateInput(gtx)
	userScrolled := p.scroll.UserScrolled()
	if p.indexHold >= 0 && !userScrolled {
		first, offset := p.scroll.Offset()
		userScrolled = first != p.indexHold || offset != 0
	}
	if userScrolled {
		p.indexHold = -1
	}
	p.updateCurrentSection()
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
		if gtx.Constraints.Max.X >= gtx.Dp(900) {
			return p.layoutExpanded(gtx, e, snapshot)
		}
		return p.layoutMedium(gtx, e, snapshot)
	})
}

func (p *page) layoutExpanded(gtx layout.Context, e *env.Env, snapshot state.State) layout.Dimensions {
	width := min(gtx.Constraints.Max.X, gtx.Dp(994))
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X, gtx.Constraints.Max.X = width, width
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Start}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				dims := p.index.Layout(gtx, e.Th.Theme, p.sectionLabels, p.currentSection, kit.FrameExpanded)
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
	width := gtx.Constraints.Max.X
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X, gtx.Constraints.Max.X = width, width
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				dims := p.index.Layout(gtx, e.Th.Theme, p.sectionLabels, p.currentSection, kit.FrameMedium)
				p.consumeIndexSelection()
				return dims
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(21)}.Layout),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return p.layoutSections(gtx, e, snapshot) }),
		)
	})
}

func (p *page) consumeIndexSelection() {
	if selected, ok := p.index.Selected(); ok {
		p.scroll.ScrollTo(selected)
		p.currentSection = selected
		p.indexHold = selected
	}
}

func (p *page) layoutSections(gtx layout.Context, e *env.Env, snapshot state.State) layout.Dimensions {
	p.viewportHeight = gtx.Constraints.Max.Y
	if !p.initialScroll {
		p.scroll.ScrollTo(p.initialSection)
		p.currentSection = p.initialSection
		p.indexHold = p.initialSection
		p.initialScroll = true
	}
	return p.scroll.Layout(gtx, e.Th, len(settingsSections), func(gtx layout.Context, index int) layout.Dimensions {
		section := settingsSections[index]
		sectionWidget := func(gtx layout.Context) layout.Dimensions {
			return kit.Section(e.Th.Theme, section.title, func(gtx layout.Context) layout.Dimensions {
				return kit.SlabContent(gtx, e.Th.Theme, p.sectionBody(index, e, snapshot))
			})(gtx)
		}
		var dims layout.Dimensions
		if index+1 < len(settingsSections) {
			dims = layout.Inset{Bottom: unit.Dp(34)}.Layout(gtx, sectionWidget)
		} else {
			dims = sectionWidget(gtx)
		}
		p.sectionHeights[index] = dims.Size.Y
		return dims
	})
}

func (p *page) sectionBody(index int, e *env.Env, snapshot state.State) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		p.rowSection, p.rowCursor = index, 0
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
	if p.indexHold >= 0 || len(settingsSections) == 0 {
		return
	}
	first, offset := p.scroll.Offset()
	atEnd := p.scroll.Position.First+p.scroll.Position.Count >= len(settingsSections) && p.scroll.Position.OffsetLast >= 0
	p.currentSection = scrollspySection(first, offset, p.viewportHeight, p.sectionHeights, atEnd)
}

func scrollspySection(first, offset, viewportHeight int, sectionHeights []int, atEnd bool) int {
	if len(sectionHeights) == 0 {
		return 0
	}
	if atEnd {
		return len(sectionHeights) - 1
	}
	first = min(max(first, 0), len(sectionHeights)-1)
	current := first
	threshold := viewportHeight / 3
	top := -offset
	for index := first + 1; index < len(sectionHeights); index++ {
		height := sectionHeights[index-1]
		if height <= 0 {
			break
		}
		top += height
		if top >= threshold {
			break
		}
		current = index
	}
	return current
}

func (p *page) settingRows(gtx layout.Context, rows ...layout.Widget) layout.Dimensions {
	children := p.rowChildren[p.rowSection][:0]
	for i, row := range rows {
		children = append(children, layout.Rigid(row))
		if i+1 < len(rows) {
			children = append(children, layout.Rigid(layout.Spacer{Height: unit.Dp(13)}.Layout))
		}
	}
	p.rowChildren[p.rowSection] = children
	return layout.Flex{Axis: layout.Vertical, Alignment: layout.Start}.Layout(gtx, children...)
}

type settingRowDescriptor struct {
	th           *look.Theme
	title        string
	detail       string
	description  string
	control      layout.Widget
	controlWidth int

	widget         layout.Widget
	copyBlock      layout.Widget
	detailBlock    layout.Widget
	controlBlock   layout.Widget
	controlColumn  layout.Widget
	alignedControl layout.Widget
}

func newSettingRowDescriptor() *settingRowDescriptor {
	row := new(settingRowDescriptor)
	row.widget = row.layout
	row.copyBlock = row.layoutCopy
	row.detailBlock = row.layoutDetail
	row.controlBlock = row.layoutControl
	row.controlColumn = row.layoutControlColumn
	row.alignedControl = row.layoutAlignedControl
	return row
}

func (p *page) settingRow(th *look.Theme, title, detail string, control layout.Widget) layout.Widget {
	rows := p.rowDescriptors[p.rowSection]
	if p.rowCursor == len(rows) {
		rows = append(rows, newSettingRowDescriptor())
		p.rowDescriptors[p.rowSection] = rows
	}
	row := rows[p.rowCursor]
	p.rowCursor++
	row.th, row.control = th, control
	if row.title != title || row.detail != detail {
		row.title, row.detail = title, detail
		row.description = title
		if detail != "" {
			row.description += ". " + detail
		}
	}
	return row.widget
}

func (row *settingRowDescriptor) layout(gtx layout.Context) layout.Dimensions {
	if row.control == nil {
		return row.copyBlock(gtx)
	}
	if gtx.Constraints.Max.X < gtx.Dp(440) {
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.Start}.Layout(gtx,
			layout.Rigid(row.copyBlock),
			layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
			layout.Rigid(row.alignedControl),
		)
	}
	row.controlWidth = min(gtx.Dp(320), gtx.Constraints.Max.X*45/100)
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Start}.Layout(gtx,
		layout.Flexed(1, row.copyBlock),
		layout.Rigid(layout.Spacer{Width: unit.Dp(21)}.Layout),
		layout.Rigid(row.controlColumn),
	)
}

func (row *settingRowDescriptor) layoutCopy(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical, Alignment: layout.Start}.Layout(gtx,
		layout.Rigid(kit.Text(row.th.Theme, row.th.T.Row, row.th.R.Fg, row.title)),
		layout.Rigid(layout.Spacer{Height: unit.Dp(2)}.Layout),
		layout.Rigid(row.detailBlock),
	)
}

func (row *settingRowDescriptor) layoutDetail(gtx layout.Context) layout.Dimensions {
	return kit.Label{Style: row.th.T.Caption, Color: row.th.R.Fg2, MaxLines: 3}.Layout(gtx, row.th.Theme, row.detail)
}

func (row *settingRowDescriptor) layoutControl(gtx layout.Context) layout.Dimensions {
	semantic.DescriptionOp(row.description).Add(gtx.Ops)
	gtx.Constraints.Min = image.Point{}
	gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(320))
	return row.control(gtx)
}

func (row *settingRowDescriptor) layoutControlColumn(gtx layout.Context) layout.Dimensions {
	gtx.Constraints.Min.X, gtx.Constraints.Max.X = row.controlWidth, row.controlWidth
	return layout.NE.Layout(gtx, row.controlBlock)
}

func (row *settingRowDescriptor) layoutAlignedControl(gtx layout.Context) layout.Dimensions {
	return layout.E.Layout(gtx, row.controlBlock)
}
