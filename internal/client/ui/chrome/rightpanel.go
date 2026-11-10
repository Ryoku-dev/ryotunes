package chrome

import (
	"gioui.org/layout"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
)

// RightPanel owns the queue, lyrics, and Listen Together panel surfaces.
type RightPanel struct {
	tabs     kit.Tabs
	close    kit.IconPlaque
	queue    Queue
	lyrics   Lyrics
	together Together
}

// Layout draws the panel header and the selected panel body.
func (panel *RightPanel) Layout(gtx layout.Context, e *env.Env) layout.Dimensions {
	current := e.Nav.Panel()
	selected := rightPanelIndex(current)
	panel.tabs.Labels = []string{"Queue", "Lyrics", "Together"}
	panel.tabs.Selected = selected
	panel.close.Kind = kit.PlaqueQuiet
	panel.close.Icon = look.Icon.Close
	panel.close.Description = "Close panel"
	panel.close.Small = true

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.Y = gtx.Dp(48)
			gtx.Constraints.Max.Y = gtx.Dp(48)
			return layout.Inset{Left: 13, Right: 13}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Gap: gtx.Dp(8)}.Layout(gtx,
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						dims := panel.tabs.Layout(gtx, e.Th.Theme)
						if index, changed := panel.tabs.Changed(); changed {
							e.Nav.OpenPanel(rightPanelAt(index))
						}
						return dims
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						dims := panel.close.Layout(gtx, e.Th.Theme)
						if panel.close.Clicked(gtx) {
							e.Nav.TogglePanel(current)
						}
						return dims
					}),
				)
			})
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			switch current {
			case env.PanelLyrics:
				return panel.lyrics.Layout(gtx, e, LyricsPanel, layout.Inset{})
			case env.PanelListen:
				return panel.together.Layout(gtx, e)
			default:
				return panel.queue.Layout(gtx, e)
			}
		}),
	)
}

func rightPanelIndex(panel env.Panel) int {
	switch panel {
	case env.PanelLyrics:
		return 1
	case env.PanelListen:
		return 2
	default:
		return 0
	}
}

func rightPanelAt(index int) env.Panel {
	switch index {
	case 1:
		return env.PanelLyrics
	case 2:
		return env.PanelListen
	default:
		return env.PanelQueue
	}
}
