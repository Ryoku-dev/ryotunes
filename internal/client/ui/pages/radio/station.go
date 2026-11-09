package radio

import (
	"fmt"
	"strings"

	"gioui.org/layout"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryotunes/internal/client/lithic"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/widgets"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

type stationRow struct {
	row      lithic.Row
	play     lithic.Button
	homepage lithic.Button
}

func (r *stationRow) layout(gtx layout.Context, e *env.Env, station model.RadioStation, playing bool) layout.Dimensions {
	if r.play.Clicked(gtx) || r.row.DoubleClicked(gtx) {
		playStation(e, station)
	}
	if r.homepage.Clicked(gtx) && station.Homepage != "" {
		act.Call[protocol.Null](e, protocol.MethodOpenExternal, protocol.URLParams{URL: station.Homepage}, func(_ protocol.Null, err error) {
			if err != nil && e.Toast != nil {
				e.Toast(err.Error(), env.ToastError)
			}
		})
	}

	r.play.Small = true
	r.homepage.Small = true
	actions := func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if station.Homepage == "" {
					return layout.Dimensions{}
				}
				return r.homepage.Layout(gtx, e.Th, lithic.Quiet, lithic.Icon("open_in_new"), "Homepage")
			}),
			layout.Rigid(layout.Spacer{Width: e.Th.Sp.S2}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return r.play.Layout(gtx, e.Th, lithic.Line, lithic.Icon("play"), "Play")
			}),
		)
	}

	tone, claim := lithic.Idle, ""
	if playing {
		tone, claim = lithic.Alive, "Live"
	}
	mark := func(gtx layout.Context) layout.Dimensions {
		if station.Favicon != "" {
			return widgets.Art(gtx, e, station.Favicon, unit.Dp(32), unit.Dp(6))
		}
		icon := lithic.Icon("radio")
		if icon == nil {
			return layout.Dimensions{Size: gtx.Constraints.Min}
		}
		return icon.Layout(gtx, e.Th.R.Fg2)
	}
	return r.row.Layout(gtx, e.Th, lithic.RowContent{
		Mark:      mark,
		Title:     station.Name,
		Facts:     stationFacts(station),
		FactsMono: true,
		Claim:     claim,
		Tone:      tone,
		Progress:  -1,
		Action:    actions,
	}, false, false)
}

func stationFacts(station model.RadioStation) string {
	facts := make([]string, 0, 4)
	if country := strings.TrimSpace(station.Country); country != "" {
		facts = append(facts, country)
	}
	if tags := strings.TrimSpace(station.Tags); tags != "" {
		facts = append(facts, tags)
	}
	codec := strings.TrimSpace(station.Codec)
	if codec != "" && station.Bitrate > 0 {
		facts = append(facts, fmt.Sprintf("%s · %d kbps", codec, station.Bitrate))
	} else if codec != "" {
		facts = append(facts, codec)
	} else if station.Bitrate > 0 {
		facts = append(facts, fmt.Sprintf("%d kbps", station.Bitrate))
	}
	return strings.Join(facts, " · ")
}

func playStation(e *env.Env, station model.RadioStation) {
	act.Call[protocol.Null](e, protocol.MethodPlayRadioStation, protocol.PlayRadioStationParams{StationUUID: station.StationUUID}, func(_ protocol.Null, err error) {
		if e.Toast == nil {
			return
		}
		if err != nil {
			e.Toast(err.Error(), env.ToastError)
			return
		}
		e.Toast("Playing "+station.Name, env.ToastSuccess)
	})
}
