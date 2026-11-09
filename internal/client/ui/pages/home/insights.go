package home

import (
	"fmt"

	"gioui.org/layout"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryotunes/internal/client/lithic"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

type insightsView struct {
	filters lithic.Filters
	retry   lithic.Button
	period  model.ListeningPeriod
	stats   model.ListeningStats
	loaded  bool
	loading bool
	error   string
	request uint64
}

func (v *insightsView) handle(gtx layout.Context, e *env.Env) {
	if selected, changed := v.filters.Changed(gtx); changed {
		periods := []model.ListeningPeriod{model.ListeningDay, model.ListeningWeek, model.ListeningMonth}
		if selected >= 0 && selected < len(periods) && periods[selected] != v.period {
			v.period = periods[selected]
			v.refresh(e)
		}
	}
	if v.retry.Clicked(gtx) {
		v.refresh(e)
	}
}

func (v *insightsView) refresh(e *env.Env) {
	if v.period == "" {
		v.period = model.ListeningWeek
	}
	v.request++
	request := v.request
	v.loading = true
	v.error = ""
	act.Call[model.ListeningStats](e, protocol.MethodListeningStats, protocol.ListeningStatsParams{Period: v.period}, func(stats model.ListeningStats, err error) {
		if request != v.request {
			return
		}
		v.loading = false
		if err != nil {
			v.error = err.Error()
			return
		}
		v.stats = stats
		v.loaded = true
	})
}

func (v *insightsView) layout(e *env.Env) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		selected := 1
		if v.period == model.ListeningDay {
			selected = 0
		} else if v.period == model.ListeningMonth {
			selected = 2
		}
		children := []layout.FlexChild{
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return v.filters.Layout(gtx, e.Th, []string{"Day", "Week", "Month"}, nil, selected)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(13)}.Layout),
		}
		switch {
		case v.loading && !v.loaded:
			children = append(children, layout.Rigid(lithic.Text(e.Th, e.Th.T.Body, e.Th.R.Fg2, "Loading listening insights…")))
		case v.error != "" && !v.loaded:
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Gap: gtx.Dp(13)}.Layout(gtx,
					layout.Flexed(1, lithic.Text(e.Th, e.Th.T.Body, e.Th.R.Jasper, v.error)),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return v.retry.Layout(gtx, e.Th, lithic.Quiet, lithic.Icon("refresh"), "Try again")
					}),
				)
			}))
		default:
			children = append(children, layout.Rigid(v.statsLayout(e)))
		}
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.Start}.Layout(gtx, children...)
	}
}

func (v *insightsView) statsLayout(e *env.Env) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		minutes := v.stats.KnownDurationSeconds / 60
		summary := fmt.Sprintf("%d plays · %d min known duration", v.stats.Plays, minutes)
		return lithic.Slab(gtx, e.Th.R.Stone1, e.Th.Cut.Mid, func(gtx layout.Context) layout.Dimensions {
			children := []layout.FlexChild{layout.Rigid(lithic.Text(e.Th, e.Th.T.Mono, e.Th.R.Fg, summary))}
			if len(v.stats.TopTracks) == 0 && len(v.stats.TopArtists) == 0 {
				children = append(children,
					layout.Rigid(layout.Spacer{Height: unit.Dp(13)}.Layout),
					layout.Rigid(lithic.Text(e.Th, e.Th.T.Body, e.Th.R.Fg2, "No listening history in this period.")),
				)
			} else {
				if len(v.stats.TopTracks) != 0 {
					children = append(children,
						layout.Rigid(layout.Spacer{Height: unit.Dp(21)}.Layout),
						layout.Rigid(lithic.Text(e.Th, e.Th.T.Title, e.Th.R.Fg, "Top tracks")),
					)
					if len(v.stats.TopTracks) > 1 {
						values := make([]float64, len(v.stats.TopTracks))
						for i, track := range v.stats.TopTracks {
							values[i] = float64(track.Plays)
						}
						children = append(children,
							layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return lithic.EngravedChart(gtx, e.Th, values, 0, float64(values[0]), unit.Dp(92), nil)
							}),
						)
					}
					for _, track := range v.stats.TopTracks {
						track := track
						children = append(children,
							layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								label := track.Title
								if track.Artists != "" {
									label += " · " + track.Artists
								}
								return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
									layout.Flexed(1, lithic.Text(e.Th, e.Th.T.Body, e.Th.R.Fg2, label)),
									layout.Rigid(lithic.Text(e.Th, e.Th.T.MonoSm, e.Th.R.Fg3, fmt.Sprintf("%d plays", track.Plays))),
								)
							}),
						)
					}
				}
				if len(v.stats.TopArtists) != 0 {
					children = append(children,
						layout.Rigid(layout.Spacer{Height: unit.Dp(21)}.Layout),
						layout.Rigid(lithic.Text(e.Th, e.Th.T.Title, e.Th.R.Fg, "Top artists")),
					)
				}
				for _, artist := range v.stats.TopArtists {
					artist := artist
					children = append(children,
						layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
								layout.Flexed(1, lithic.Text(e.Th, e.Th.T.Body, e.Th.R.Fg2, artist.Name)),
								layout.Rigid(lithic.Text(e.Th, e.Th.T.MonoSm, e.Th.R.Fg3, fmt.Sprintf("%d plays", artist.Plays))),
							)
						}),
					)
				}
			}
			return layout.Flex{Axis: layout.Vertical, Alignment: layout.Start}.Layout(gtx, children...)
		})
	}
}
