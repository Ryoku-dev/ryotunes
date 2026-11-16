package chrome

import (
	"image"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/anim"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/tokens"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/widgets"
)

// panelHead is the panel's head: its tabs and close, 48 tall, over one vein
// rule.
const panelHead unit.Dp = 48

// rightPanelKeys name the bodies for the swap between them.
var rightPanelKeys = [3]string{"queue", "lyrics", "together"}

// RightPanel owns the queue, lyrics, and Listen Together panel surfaces: the
// interior the frame docks beside the page or slides in as a drawer. Its
// plane and hairline are the frame's.
type RightPanel struct {
	tabs       kit.Tabs
	close      kit.IconPlaque
	closeGlyph widgets.PressGlyph
	body       look.Swap
	queue      Queue
	lyrics     Lyrics
	together   Together
}

// Layout draws the panel head and the selected body; changing tabs
// crossfades the bodies with the Hub's short rise on OutCubic.
func (panel *RightPanel) Layout(gtx layout.Context, e *env.Env) layout.Dimensions {
	current := e.Nav.Panel()
	selected := rightPanelIndex(current)
	panel.tabs.Labels = []string{"Queue", "Lyrics", "Together"}
	panel.tabs.Selected = selected
	panel.close.Kind = kit.PlaqueQuiet
	panel.close.Description = "Close panel"
	panel.close.Small = true
	panel.body.Ease = anim.EaseOutCubic

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			height := gtx.Dp(panelHead)
			gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
			dims := layout.Inset{Left: 13, Right: 13}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Gap: gtx.Dp(8)}.Layout(gtx,
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						dims := panel.tabs.Layout(gtx, e.Th.Theme)
						if index, changed := panel.tabs.Changed(); changed {
							e.Nav.OpenPanel(rightPanelAt(index))
						}
						return dims
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						dims := panel.closeGlyph.Layout(gtx, e.Th, &panel.close, look.Icon.Close)
						if panel.close.Clicked(gtx) {
							e.Nav.TogglePanel(current)
						}
						return dims
					}),
				)
			})
			hair := max(1, gtx.Dp(unit.Dp(tokens.SizeHairline)))
			inset := gtx.Dp(13)
			rule := image.Rect(inset, height-hair, max(inset, dims.Size.X-inset), height)
			paint.FillShape(gtx.Ops, e.Th.R.Line(), clip.Rect(rule).Op())
			return dims
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return panel.body.Layout(gtx, e.Th, rightPanelKeys[selected], 8, e.Th.Motion.Move, func(gtx layout.Context, key string) layout.Dimensions {
				switch key {
				case rightPanelKeys[1]:
					return panel.lyrics.Layout(gtx, e, LyricsPanel, layout.Inset{})
				case rightPanelKeys[2]:
					return panel.together.Layout(gtx, e)
				default:
					return panel.queue.Layout(gtx, e)
				}
			})
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
