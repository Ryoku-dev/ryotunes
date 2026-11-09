package home

import (
	"fmt"
	"strings"

	"gioui.org/layout"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/widgets"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

type insightsView struct {
	stats   model.ListeningStats
	loaded  bool
	loading bool
	request uint64
}

func (v *insightsView) refresh(e *env.Env) {
	v.request++
	request := v.request
	v.loading = true
	act.Call[model.ListeningStats](e, protocol.MethodListeningStats, protocol.ListeningStatsParams{Period: model.ListeningWeek}, func(stats model.ListeningStats, err error) {
		if request != v.request {
			return
		}
		v.loading = false
		if err != nil {
			return
		}
		v.stats = stats
		v.loaded = true
	})
}

func (v *insightsView) layout(e *env.Env) layout.Widget {
	if !v.loaded || v.loading {
		return nil
	}
	facts := make([]insightFact, 0, 3)
	if v.stats.Plays > 0 {
		facts = append(facts, insightFact{label: "Plays this week", value: widgets.Count(v.stats.Plays)})
	}
	if v.stats.KnownDurationSeconds > 0 {
		facts = append(facts, insightFact{label: "Time listening", value: listeningTime(v.stats.KnownDurationSeconds)})
	}
	if len(v.stats.TopArtists) != 0 && strings.TrimSpace(v.stats.TopArtists[0].Name) != "" {
		facts = append(facts, insightFact{label: "Top artist", value: strings.TrimSpace(v.stats.TopArtists[0].Name), text: true})
	}
	if len(facts) == 0 {
		return nil
	}
	return func(gtx layout.Context) layout.Dimensions {
		return kit.SlabContent(gtx, e.Th.Theme, func(gtx layout.Context) layout.Dimensions {
			children := []layout.FlexChild{
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return kit.Label{Style: e.Th.T.Label, Color: e.Th.R.Fg, MaxLines: 1}.Layout(gtx, e.Th.Theme, "Listening insights")
				}),
			}
			for _, fact := range facts {
				fact := fact
				children = append(children,
					layout.Rigid(layout.Spacer{Width: unit.Dp(21)}.Layout),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return fact.layout(gtx, e) }),
				)
			}
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
		})
	}
}

type insightFact struct {
	label string
	value string
	text  bool
}

func (f insightFact) layout(gtx layout.Context, e *env.Env) layout.Dimensions {
	style := e.Th.T.Mono
	if f.text {
		style = e.Th.T.Body
	}
	return layout.Flex{Axis: layout.Vertical, Alignment: layout.Start}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return kit.Label{Style: e.Th.T.Group, Color: e.Th.R.Fg3, MaxLines: 1}.Layout(gtx, e.Th.Theme, f.label)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(2)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return kit.Label{Style: style, Color: e.Th.R.Fg, MaxLines: 1}.Layout(gtx, e.Th.Theme, f.value)
		}),
	)
}

func listeningTime(seconds int64) string {
	minutes := seconds / 60
	if minutes < 60 {
		return fmt.Sprintf("%d min", minutes)
	}
	hours := minutes / 60
	remaining := minutes % 60
	if remaining == 0 {
		return fmt.Sprintf("%d hr", hours)
	}
	return fmt.Sprintf("%d hr %d min", hours, remaining)
}
