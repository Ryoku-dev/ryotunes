package settings

import (
	"image"

	"gioui.org/io/semantic"
	"gioui.org/layout"

	"github.com/ryoku-dev/ryotunes/internal/client/lithic"
	"github.com/ryoku-dev/ryotunes/internal/client/state"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
)

// Layout draws the responsive Lithic full page.
func (p *page) Layout(gtx layout.Context, e *env.Env) layout.Dimensions {
	if !p.refreshed {
		p.Refresh(e)
	}
	snapshot := e.Store.Snapshot()
	return layout.Inset{Top: 34, Bottom: 34, Left: 36, Right: 36}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		if p.loading && len(snapshot.Settings) == 0 {
			return lithic.Text(e.Th, e.Th.T.Body, e.Th.R.Fg2, "Loading…")(gtx)
		}
		if p.loadError != "" && len(snapshot.Settings) == 0 {
			if p.retry.Clicked(gtx) {
				p.Refresh(e)
			}
			return layout.Flex{Axis: layout.Vertical, Alignment: layout.Start, Gap: gtx.Dp(13)}.Layout(gtx,
				layout.Rigid(lithic.Text(e.Th, e.Th.T.Body, e.Th.R.Jasper, p.loadError)),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return p.retry.Layout(gtx, e.Th, lithic.Line, nil, "Try again")
				}),
			)
		}
		available := gtx.Constraints.Max.X
		expanded := available >= gtx.Dp(994)
		medium := available >= gtx.Dp(620)
		if expanded {
			return p.layoutExpanded(gtx, e, snapshot)
		}
		if medium {
			return p.layoutMedium(gtx, e, snapshot)
		}
		return p.layoutCompact(gtx, e, snapshot)
	})
}

func (p *page) layoutExpanded(gtx layout.Context, e *env.Env, snapshot state.State) layout.Dimensions {
	width := min(gtx.Constraints.Max.X, gtx.Dp(994))
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = width
		gtx.Constraints.Max.X = width
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Start}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.X = gtx.Dp(200)
				gtx.Constraints.Max.X = gtx.Dp(200)
				return p.layoutSectionIndex(gtx, e)
			}),
			layout.Rigid(layout.Spacer{Width: 34}.Layout),
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
		gtx.Constraints.Min.X = width
		gtx.Constraints.Max.X = width
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				p.chipList.Gap = gtx.Dp(4)
				return p.chipList.Layout(gtx, len(settingsSections), func(gtx layout.Context, index int) layout.Dimensions {
					button := &p.chipButtons[index]
					button.Small = true
					if button.Clicked(gtx) {
						p.scrollToSection(index)
					}
					kind := lithic.Quiet
					if index == p.currentSection {
						kind = lithic.On
					}
					return button.Layout(gtx, e.Th, kind, nil, settingsSections[index].title)
				})
			}),
			layout.Rigid(layout.Spacer{Height: 21}.Layout),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return p.layoutSections(gtx, e, snapshot) }),
		)
	})
}

func (p *page) layoutCompact(gtx layout.Context, e *env.Env, snapshot state.State) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(lithic.Text(e.Th, e.Th.T.Section, e.Th.R.Fg, "Settings")),
		layout.Rigid(layout.Spacer{Height: 21}.Layout),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return p.layoutSections(gtx, e, snapshot) }),
	)
}

func (p *page) layoutSectionIndex(gtx layout.Context, e *env.Env) layout.Dimensions {
	children := make([]layout.FlexChild, 0, len(settingsSections)*2)
	for i, section := range settingsSections {
		i, section := i, section
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if p.sectionButtons[i].Clicked(gtx) {
				p.scrollToSection(i)
			}
			kind := lithic.Quiet
			if i == p.currentSection {
				kind = lithic.On
			}
			gtx.Constraints.Min.X = gtx.Dp(200)
			return p.sectionButtons[i].Layout(gtx, e.Th, kind, nil, section.title)
		}))
		if i+1 < len(settingsSections) {
			children = append(children, layout.Rigid(layout.Spacer{Height: 8}.Layout))
		}
	}
	return layout.Flex{Axis: layout.Vertical, Alignment: layout.Start}.Layout(gtx, children...)
}

func (p *page) layoutSections(gtx layout.Context, e *env.Env, snapshot state.State) layout.Dimensions {
	if !p.initialScroll {
		p.list.ScrollTo(p.initialSection)
		p.currentSection = p.initialSection
		p.initialScroll = true
	}
	dims := p.list.Layout(gtx, e.Th, len(settingsSections), func(gtx layout.Context, index int) layout.Dimensions {
		section := settingsSections[index]
		dims := layout.Flex{Axis: layout.Vertical, Alignment: layout.Start}.Layout(gtx,
			layout.Rigid(lithic.Text(e.Th, e.Th.T.Section, e.Th.R.Fg, section.title)),
			layout.Rigid(layout.Spacer{Height: 13}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return lithic.Slab(gtx, e.Th.R.Stone1, e.Th.Cut.Mid, p.sectionBody(index, e, snapshot))
			}),
			layout.Rigid(layout.Spacer{Height: 55}.Layout),
		)
		p.sectionHeights[index] = dims.Size.Y
		return dims
	})
	p.updateCurrentSection()
	return dims
}

func (p *page) sectionBody(index int, e *env.Env, snapshot state.State) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		switch index {
		case 0:
			return p.layoutAppearance(gtx, e, snapshot)
		case 1:
			return p.layoutPlayback(gtx, e, snapshot)
		case 2:
			return p.layoutProviders(gtx, e, snapshot)
		case 3:
			return p.layoutDownloads(gtx, e)
		case 4:
			return p.layoutIntegrations(gtx, e, snapshot)
		case 5:
			return p.layoutData(gtx, e)
		case 6:
			return p.layoutLocal(gtx, e)
		default:
			return p.layoutAbout(gtx, e)
		}
	}
}

func (p *page) scrollToSection(index int) {
	if index < 0 || index >= len(settingsSections) {
		return
	}
	p.list.ScrollTo(index)
	p.chipList.ScrollTo(index)
	p.currentSection = index
}

func (p *page) updateCurrentSection() {
	current := min(max(p.list.Position.First, 0), len(settingsSections)-1)
	height := p.sectionHeights[current]
	if current+1 < len(settingsSections) && height > 0 && -p.list.Position.Offset > height*2/3 {
		current++
	}
	if p.list.Position.First+p.list.Position.Count >= len(settingsSections) && p.list.Position.OffsetLast >= 0 {
		current = len(settingsSections) - 1
	}
	if current != p.currentSection {
		p.chipList.ScrollTo(current)
	}
	p.currentSection = current
}

func settingRows(gtx layout.Context, rows ...layout.Widget) layout.Dimensions {
	children := make([]layout.FlexChild, 0, len(rows)*2)
	for i, row := range rows {
		children = append(children, layout.Rigid(row))
		if i+1 < len(rows) {
			children = append(children, layout.Rigid(layout.Spacer{Height: 13}.Layout))
		}
	}
	return layout.Flex{Axis: layout.Vertical, Alignment: layout.Start}.Layout(gtx, children...)
}

func settingRow(th *lithic.Theme, title, detail string, control layout.Widget) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		copy := func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical, Alignment: layout.Start}.Layout(gtx,
				layout.Rigid(lithic.Text(th, th.T.Row, th.R.Fg, title)),
				layout.Rigid(layout.Spacer{Height: 2}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return lithic.Label{Style: th.T.Caption, Color: th.R.Fg2}.Layout(gtx, th, detail)
				}),
			)
		}
		if control == nil {
			return copy(gtx)
		}
		controlWithDescription := func(gtx layout.Context) layout.Dimensions {
			description := title
			if detail != "" {
				description += ". " + detail
			}
			semantic.DescriptionOp(description).Add(gtx.Ops)
			return control(gtx)
		}
		if gtx.Constraints.Max.X < gtx.Dp(560) {
			return layout.Flex{Axis: layout.Vertical, Alignment: layout.Start}.Layout(gtx,
				layout.Rigid(copy),
				layout.Rigid(layout.Spacer{Height: 8}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.E.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min = image.Point{}
						gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(320))
						return controlWithDescription(gtx)
					})
				}),
			)
		}
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Flexed(1, copy),
			layout.Rigid(layout.Spacer{Width: 21}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min = image.Point{}
				gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(320))
				return controlWithDescription(gtx)
			}),
		)
	}
}
