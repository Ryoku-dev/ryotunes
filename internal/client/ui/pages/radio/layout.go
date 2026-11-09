package radio

import (
	"fmt"
	"image"
	"strings"

	"gioui.org/layout"
	"gioui.org/widget"

	"github.com/ryoku-dev/ryotunes/internal/client/lithic"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/widgets"
)

// Layout renders the directory and starts its first request only once visible.
func (p *Page) Layout(gtx layout.Context, e *env.Env) layout.Dimensions {
	p.ensureLoaded(e)
	for {
		event, ok := p.search.Update(gtx)
		if !ok {
			break
		}
		if submit, ok := event.(widget.SubmitEvent); ok {
			query := normalizedQuery(submit.Text)
			p.search.SetText(query)
			p.load(e, query)
		}
	}
	if p.retry.Clicked(gtx) {
		p.load(e, p.query)
	}
	if p.moreRetry.Clicked(gtx) {
		p.loadMore(e)
	}

	inset := layout.Inset{Top: e.Th.Sp.S4, Right: e.Th.Sp.S5, Bottom: e.Th.Sp.S5, Left: e.Th.Sp.S5}
	return inset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(widgets.PageHeader(e, "Airwaves", p.meta(), nil)),
			layout.Rigid(layout.Spacer{Height: e.Th.Sp.S3}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(520))
				return p.search.Layout(gtx, e.Th, "Search stations by name…", "Enter")
			}),
			layout.Rigid(layout.Spacer{Height: e.Th.Sp.S4}.Layout),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return p.content(gtx, e)
			}),
		)
	})
}

func (p *Page) meta() string {
	count := len(p.stations)
	if p.query == "" {
		if !p.loaded {
			return "Top stations"
		}
		return fmt.Sprintf("Top stations · %d %s", count, stationWord(count))
	}
	if !p.loaded {
		return fmt.Sprintf("Results for “%s”", p.query)
	}
	return fmt.Sprintf("Results for “%s” · %d %s", p.query, count, stationWord(count))
}

func stationWord(count int) string {
	if count == 1 {
		return "station"
	}
	return "stations"
}

func (p *Page) content(gtx layout.Context, e *env.Env) layout.Dimensions {
	if strings.TrimSpace(p.search.Text()) == "" && p.query != "" && !p.loading {
		p.load(e, "")
	}
	if p.loading && len(p.stations) == 0 {
		return radioState(gtx, e, "Tuning", "Finding live stations.", "Trying available Radio Browser mirrors without blocking the player.", nil)
	}
	if p.error != "" {
		return radioState(gtx, e, "Signal lost", "Radio directory unavailable.", p.error, func(gtx layout.Context) layout.Dimensions {
			return p.retry.Layout(gtx, e.Th, lithic.Line, lithic.Icon("refresh"), "Try again")
		})
	}
	if p.loaded && len(p.stations) == 0 {
		return widgets.EmptyState(e, "radio", "No stations found.", "Try a shorter station name or return to the popular directory.")(gtx)
	}

	nowID := ""
	if snapshot := e.Store.Snapshot(); snapshot.Playback.Now != nil {
		nowID = snapshot.Playback.Now.VideoID
	}
	count := len(p.stations) + 1
	dims := p.list.Layout(gtx, count, func(gtx layout.Context, index int) layout.Dimensions {
		if index == len(p.stations) {
			return p.footer(gtx, e)
		}
		station := p.stations[index]
		row := p.rows[station.StationUUID]
		if row == nil {
			row = &stationRow{}
			p.rows[station.StationUUID] = row
		}
		playing := nowID == "RYOTUNES_RADIO:"+station.StationUUID
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return row.layout(gtx, e, station, playing)
			}),
			layout.Rigid(layout.Spacer{Height: e.Th.Sp.S1}.Layout),
		)
	})
	if p.hasMore && !p.more && p.moreErr == "" && p.list.Position.First+p.list.Position.Count >= len(p.stations)-4 {
		p.loadMore(e)
	}
	return dims
}

func (p *Page) footer(gtx layout.Context, e *env.Env) layout.Dimensions {
	gtx.Constraints.Min.Y = min(gtx.Constraints.Max.Y, gtx.Dp(56))
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		switch {
		case p.more:
			return lithic.Text(e.Th, e.Th.T.Body, e.Th.R.Fg2, "Finding more…")(gtx)
		case p.moreErr != "":
			return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(lithic.Text(e.Th, e.Th.T.Caption, e.Th.R.Jasper, p.moreErr)),
				layout.Rigid(layout.Spacer{Height: e.Th.Sp.S2}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return p.moreRetry.Layout(gtx, e.Th, lithic.Quiet, lithic.Icon("refresh"), "Try again")
				}),
			)
		case p.hasMore:
			return lithic.Text(e.Th, e.Th.T.Body, e.Th.R.Fg2, "More stations load as you reach them.")(gtx)
		default:
			return lithic.Text(e.Th, e.Th.T.MonoSm, e.Th.R.Fg3, "End of this signal set")(gtx)
		}
	})
}

func radioState(gtx layout.Context, e *env.Env, title, body, detail string, action layout.Widget) layout.Dimensions {
	gtx.Constraints.Min = image.Point{}
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		children := []layout.FlexChild{
			layout.Rigid(lithic.Text(e.Th, e.Th.T.Title, e.Th.R.Fg, title)),
			layout.Rigid(layout.Spacer{Height: e.Th.Sp.S2}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return lithic.Label{Style: e.Th.T.Body, Color: e.Th.R.Fg2, MaxLines: 3}.Layout(gtx, e.Th, body)
			}),
			layout.Rigid(layout.Spacer{Height: e.Th.Sp.S1}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return lithic.Label{Style: e.Th.T.Caption, Color: e.Th.R.Fg3, MaxLines: 4}.Layout(gtx, e.Th, detail)
			}),
		}
		if action != nil {
			children = append(children,
				layout.Rigid(layout.Spacer{Height: e.Th.Sp.S3}.Layout),
				layout.Rigid(action),
			)
		}
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx, children...)
	})
}
