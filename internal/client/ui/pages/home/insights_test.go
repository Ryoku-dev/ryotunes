package home

import (
	"image"
	"testing"
	"time"

	"gioui.org/io/input"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
	libtheme "github.com/ryoku-dev/ryoku/docs/lithic/library/go/theme"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/model"
)

func listeningDays(plays ...int) model.ListeningDays {
	start := time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC)
	days := model.ListeningDays{Days: make([]model.DailyListening, len(plays))}
	for index, n := range plays {
		days.Days[index] = model.DailyListening{
			Date:  start.AddDate(0, 0, index).Format(time.DateOnly),
			Plays: n, KnownDurationSeconds: int64(n) * 200,
		}
	}
	return days
}

// The readings come from the daemon's series as it is: the chart's points are the days'
// plays and minutes, the ticks name the first day, the middle one and today, and the proof
// says which day was busiest.
func TestReadingsDrawTheDaemonSeries(t *testing.T) {
	var v insightsView
	v.daysReady, v.days = true, listeningDays(0, 2, 5, 1, 0, 0, 3, 9, 4, 0, 1, 2, 0, 6)
	v.statsReady = true
	v.stats = model.ListeningStats{Plays: 22, KnownDurationSeconds: 4400, TopArtists: []model.ArtistPlayCount{{Name: "Kite Assembly", Plays: 9}}}
	v.revision++
	v.prepare()

	plays, minutes := v.readings[0], v.readings[1]
	if len(plays.values) != 14 || plays.values[7] != 9 || plays.values[0] != 0 || minutes.values[7] != 30 {
		t.Fatalf("series = %v / %v", plays.values, minutes.values)
	}
	if plays.total != 33 || minutes.total != 6600 {
		t.Fatalf("totals = %v plays, %v s", plays.total, minutes.total)
	}
	if plays.ticks[0] != "Nov 3" || plays.ticks[7] != "Nov 10" || plays.ticks[13] != "Today" || plays.ticks[1] != "" {
		t.Fatalf("ticks = %q", plays.ticks)
	}
	if plays.proof != "Last 14 days, most on Tuesday, Nov 10" {
		t.Fatalf("proof = %q", plays.proof)
	}
	if len(v.facts) != 3 || v.facts[0].value != "22" || v.facts[1].value != "1 h 13 min" || v.facts[2].value != "Kite Assembly" {
		t.Fatalf("datasheet = %+v", v.facts)
	}
	if v.quiet {
		t.Fatal("a series with plays reads as quiet")
	}
}

// Without the series (a daemon before listening_days) no chart is drawn, and a window
// with no plays says so instead of drawing a flat line.
func TestReadingsNeverInventASeries(t *testing.T) {
	var v insightsView
	v.statsReady = true
	v.stats = model.ListeningStats{Plays: 4}
	v.revision++
	v.prepare()
	if len(v.readings[0].values) != 0 || v.quiet {
		t.Fatalf("no series drew %d points (quiet %v)", len(v.readings[0].values), v.quiet)
	}

	v.daysReady, v.days = true, listeningDays(0, 0, 0)
	v.revision++
	v.prepare()
	if !v.quiet {
		t.Fatal("a window without plays is not quiet")
	}
}

// Readings count up once, then stand: a settled Home asks for no frame and replays its
// recorded charts.
func TestReadingsSettleWithoutFrames(t *testing.T) {
	th := look.NewTheme(palette.Signature(true), libtheme.Comfortable)
	e := &env.Env{Th: th, Overlays: new(look.Overlays)}
	var v insightsView
	v.daysReady, v.days = true, listeningDays(1, 4, 2, 0, 6, 3, 2)
	v.statsReady = true
	v.stats = model.ListeningStats{Plays: 18, KnownDurationSeconds: 3600}
	v.revision++
	widget := v.layout(e)
	if widget == nil {
		t.Fatal("readings with data drew nothing")
	}

	var ops op.Ops
	var router input.Router
	start := time.Unix(100, 0)
	frame := func(now time.Time) (layout.Dimensions, bool) {
		ops.Reset()
		gtx := layout.Context{Ops: &ops, Source: router.Source(), Now: now,
			Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Constraints{Max: image.Pt(900, 2000)}}
		dimensions := widget(gtx)
		router.Frame(&ops)
		_, wake := router.WakeupTime()
		return dimensions, wake
	}
	if _, wake := frame(start); !wake {
		t.Fatal("the count-up asked for no frame")
	}
	if v.readings[0].shown != "0" {
		t.Fatalf("the count starts at %q", v.readings[0].shown)
	}
	frame(start.Add(th.Motion.CountUp / 2))
	frame(start.Add(th.Motion.CountUp + time.Millisecond))
	call := v.readings[0].chart.call
	dimensions, wake := frame(start.Add(2 * time.Second))
	if wake {
		t.Fatal("settled readings asked for a frame")
	}
	// the plays reading is the series' total, counted up
	if dimensions.Size.Y <= 0 || v.readings[0].shown != "18" {
		t.Fatalf("settled plays reading = %q, height %d", v.readings[0].shown, dimensions.Size.Y)
	}
	if v.readings[0].chart.call != call {
		t.Fatal("a settled chart was recorded again")
	}
}
