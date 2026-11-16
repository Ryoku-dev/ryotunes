package home

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"strconv"
	"strings"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/tokens"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/widgets"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

// readingDays is how many days Home's readings draw: two weeks, inside the 30 the daemon
// keeps.
const readingDays = 14

// readingCellMin is the narrowest a reading may sit beside another (Home.qml:965).
const readingCellMin unit.Dp = 112

// insightsView is Home's "Your listening": the week as a datasheet (keys in mono, one vein
// rule, values) and two readings drawn as engraved charts from the daemon's per-day series
// (listening_days), as the Hub's Home draws this computer. Nothing is drawn from made-up
// data: without the series (an older daemon) the charts stay away, and with no plays in the
// window a sentence says how they will fill.
type insightsView struct {
	stats      model.ListeningStats
	days       model.ListeningDays
	statsReady bool
	daysReady  bool
	loading    int
	request    uint64
	revision   uint64

	built    uint64
	facts    []readingFact
	readings [2]reading
	quiet    bool

	up look.CountUp
}

type readingFact struct {
	key, value string
}

// reading is one cell: its name, the figure (counted up, then rolling), the chart and the
// proof under it.
type reading struct {
	name   string
	total  float64
	format func(float64) string
	proof  string
	values []float64
	ticks  []string
	chart  chartCache
	roll   look.CountRoll
	shown  string
	shownN float64
}

func (v *insightsView) refresh(e *env.Env) {
	v.request++
	request := v.request
	v.loading = 2
	v.revision++
	act.Call[model.ListeningStats](e, protocol.MethodListeningStats, protocol.ListeningStatsParams{Period: model.ListeningWeek}, func(stats model.ListeningStats, err error) {
		if request != v.request {
			return
		}
		v.loading--
		v.revision++
		if err == nil {
			v.stats, v.statsReady = stats, true
		}
	})
	act.Call[model.ListeningDays](e, protocol.MethodListeningDays, protocol.ListeningDaysParams{Days: readingDays}, func(days model.ListeningDays, err error) {
		if request != v.request {
			return
		}
		v.loading--
		v.revision++
		if err == nil {
			v.days, v.daysReady = days, true
		}
	})
}

// prepare derives the datasheet and the readings once per arrival.
func (v *insightsView) prepare() {
	if v.built == v.revision {
		return
	}
	v.built = v.revision
	v.facts = v.facts[:0]
	if v.statsReady {
		if v.stats.Plays > 0 {
			v.facts = append(v.facts, readingFact{key: "Plays this week", value: strconv.Itoa(v.stats.Plays)})
		}
		if v.stats.KnownDurationSeconds > 0 {
			v.facts = append(v.facts, readingFact{key: "Time listening", value: listeningTime(v.stats.KnownDurationSeconds)})
		}
		if len(v.stats.TopArtists) != 0 && strings.TrimSpace(v.stats.TopArtists[0].Name) != "" {
			v.facts = append(v.facts, readingFact{key: "Top artist", value: strings.TrimSpace(v.stats.TopArtists[0].Name)})
		}
		if len(v.stats.TopTracks) != 0 && strings.TrimSpace(v.stats.TopTracks[0].Title) != "" {
			song := strings.TrimSpace(v.stats.TopTracks[0].Title)
			if artists := strings.TrimSpace(v.stats.TopTracks[0].Artists); artists != "" {
				song += ", " + artists
			}
			v.facts = append(v.facts, readingFact{key: "Top song", value: song})
		}
	}
	v.quiet = false
	plays, seconds := &v.readings[0], &v.readings[1]
	plays.values, seconds.values = plays.values[:0], seconds.values[:0]
	if !v.daysReady || len(v.days.Days) < 2 {
		return
	}
	days := v.days.Days
	var totalPlays, totalSeconds int64
	busiest := -1
	for index, day := range days {
		plays.values = append(plays.values, float64(day.Plays))
		seconds.values = append(seconds.values, float64(day.KnownDurationSeconds)/60)
		totalPlays += int64(day.Plays)
		totalSeconds += day.KnownDurationSeconds
		if day.Plays > 0 && (busiest < 0 || day.Plays >= days[busiest].Plays) {
			busiest = index
		}
	}
	if totalPlays == 0 {
		v.quiet = true
		return
	}
	ticks := readingTicks(days, plays.ticks)
	plays.ticks, seconds.ticks = ticks, ticks
	plays.name = "Plays"
	plays.total = float64(totalPlays)
	plays.format = formatCount
	plays.proof = fmt.Sprintf("Last %d days, most on %s", len(days), dayName(days[busiest].Date))
	seconds.name = "Listening time"
	seconds.total = float64(totalSeconds)
	seconds.format = formatListening
	seconds.proof = "About " + listeningTime(totalSeconds/int64(len(days))) + " a day"
	plays.chart.revision++
	seconds.chart.revision++
}

// layout returns the readings for the page's list, or nil while nothing has arrived.
func (v *insightsView) layout(e *env.Env) layout.Widget {
	if v.loading > 0 && !v.statsReady && !v.daysReady {
		return nil
	}
	v.prepare()
	charted := len(v.readings[0].values) >= 2
	if len(v.facts) == 0 && !charted && !v.quiet {
		return nil
	}
	return func(gtx layout.Context) layout.Dimensions {
		return v.draw(gtx, e, charted)
	}
}

func (v *insightsView) draw(gtx layout.Context, e *env.Env, charted bool) layout.Dimensions {
	th := e.Th
	fraction := v.up.Fraction(gtx, th, true)
	mark := ""
	if charted || v.quiet {
		mark = kit.Pad2(readingDays) + " days"
	}
	children := [4]layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return widgets.RuledHead(gtx, e, "Your listening", mark, nil)
		}),
		layout.Rigid(layout.Spacer{Height: th.Sp.S3}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return v.datasheet(gtx, e) }),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			switch {
			case charted:
				return layout.Inset{Top: th.Sp.S4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return v.grid(gtx, e, fraction)
				})
			case v.quiet:
				return layout.Inset{Top: th.Sp.S3}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return kit.Label{Style: th.T.Body, Color: th.R.Fg2, MaxLines: 2}.Layout(gtx, th.Theme, "Play something and your days draw here.")
				})
			}
			return layout.Dimensions{}
		}),
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children[:]...)
}

// datasheet is the week's standing facts: keys in the small mono, spaced capitals, fg3; one
// vein rule; values in detail, fg (Home.qml:930-963).
func (v *insightsView) datasheet(gtx layout.Context, e *env.Env) layout.Dimensions {
	if len(v.facts) == 0 {
		return layout.Dimensions{}
	}
	th := e.Th
	gtx.Constraints.Min = image.Point{}
	row := gtx.Sp(th.T.Detail.Line) + gtx.Dp(th.Sp.S1)
	keyWidth := 0
	for index, fact := range v.facts {
		record := op.Record(gtx.Ops)
		size := kit.LayoutSpacedCaps(gtx, th.Theme, th.T.MonoSm, th.R.Fg3, 1.2, fact.key).Size
		call := record.Stop()
		offset := op.Offset(image.Pt(0, index*row+(row-size.Y)/2)).Push(gtx.Ops)
		call.Add(gtx.Ops)
		offset.Pop()
		keyWidth = max(keyWidth, size.X)
	}
	gap := gtx.Dp(th.Sp.S3)
	hair := max(1, gtx.Dp(tokens.SizeHairline))
	height := row * len(v.facts)
	ruleX := keyWidth + gap
	paint.FillShape(gtx.Ops, th.R.Vein, clip.Rect(image.Rect(ruleX, 0, ruleX+hair, height)).Op())
	valueX := ruleX + hair + gap
	valueGTX := gtx
	valueGTX.Constraints.Max.X = max(0, gtx.Constraints.Max.X-valueX)
	for index, fact := range v.facts {
		record := op.Record(gtx.Ops)
		size := kit.Label{Style: th.T.Detail, Color: th.R.Fg, MaxLines: 1}.Layout(valueGTX, th.Theme, fact.value).Size
		call := record.Stop()
		offset := op.Offset(image.Pt(valueX, index*row+(row-size.Y)/2)).Push(gtx.Ops)
		call.Add(gtx.Ops)
		offset.Pop()
	}
	return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, height)}
}

// grid lays the readings out two to a row when there is room, one under the other when not.
func (v *insightsView) grid(gtx layout.Context, e *env.Env, fraction float32) layout.Dimensions {
	th := e.Th
	gap := gtx.Dp(th.Sp.S4)
	width := gtx.Constraints.Max.X
	perRow := 1
	if width >= 2*gtx.Dp(readingCellMin)+gap {
		perRow = 2
	}
	cell := (width - (perRow-1)*gap) / perRow
	x, y, rowHeight := 0, 0, 0
	for index := range v.readings {
		if index > 0 && index%perRow == 0 {
			x, y, rowHeight = 0, y+rowHeight+gap, 0
		}
		child := gtx
		child.Constraints = layout.Constraints{Max: image.Pt(cell, gtx.Constraints.Max.Y)}
		offset := op.Offset(image.Pt(x, y)).Push(gtx.Ops)
		size := v.readings[index].layout(child, e, fraction)
		offset.Pop()
		rowHeight = max(rowHeight, size.Y)
		x += cell + gap
	}
	return layout.Dimensions{Size: image.Pt(width, y+rowHeight)}
}

func (r *reading) layout(gtx layout.Context, e *env.Env, fraction float32) image.Point {
	th := e.Th
	gtx.Constraints.Min = image.Point{}
	y := 0
	place := func(w layout.Widget) {
		offset := op.Offset(image.Pt(0, y)).Push(gtx.Ops)
		size := w(gtx).Size
		offset.Pop()
		y += size.Y
	}
	place(func(gtx layout.Context) layout.Dimensions {
		return kit.Label{Style: th.T.Caption, Color: th.R.Fg2, MaxLines: 1}.Layout(gtx, th.Theme, r.name)
	})
	y += gtx.Dp(th.Sp.S1)
	place(func(gtx layout.Context) layout.Dimensions {
		if fraction < 1 {
			// counting up from nothing: a plain figure, so the roll does not play every frame
			return kit.Label{Style: th.T.Section, Color: th.R.Fg, MaxLines: 1}.Layout(gtx, th.Theme, r.figure(r.total*float64(fraction)))
		}
		return r.roll.Layout(gtx, th, th.T.Section, th.R.Fg, r.figure(r.total))
	})
	y += gtx.Dp(th.Sp.S1)
	place(func(gtx layout.Context) layout.Dimensions {
		return r.chart.layout(gtx, th, r.values, r.ticks)
	})
	y += gtx.Dp(th.Sp.S1)
	place(func(gtx layout.Context) layout.Dimensions {
		return kit.Label{Style: th.T.Caption, Color: th.R.Fg3, MaxLines: 1}.Layout(gtx, th.Theme, r.proof)
	})
	return image.Pt(gtx.Constraints.Max.X, y)
}

// figure formats n, keeping the last string while n has not changed.
func (r *reading) figure(n float64) string {
	if r.shown == "" || n != r.shownN {
		r.shown, r.shownN = r.format(n), n
	}
	return r.shown
}

// chartCache keeps one engraved chart recorded for its width, scale, inks and data, and
// replays it: a chart at rest costs a CallOp.
type chartCache struct {
	revision uint64
	key      chartKey
	ops      op.Ops
	call     op.CallOp
	size     image.Point
}

type chartKey struct {
	revision         uint64
	width            int
	scale            float32
	fg, fg3, vein, g color.NRGBA
}

// chartHeight is a reading's chart, as tall as the Hub's (size.searchHeight).
const chartHeight unit.Dp = tokens.SizeSearchHeight

func (c *chartCache) layout(gtx layout.Context, th *look.Theme, values []float64, ticks []string) layout.Dimensions {
	key := chartKey{revision: c.revision, width: gtx.Constraints.Max.X, scale: gtx.Metric.PxPerDp,
		fg: th.R.Fg, fg3: th.R.Fg3, vein: th.R.Vein, g: th.R.Ground}
	if key != c.key || c.call == (op.CallOp{}) {
		c.key = key
		c.ops.Reset()
		record := op.Record(&c.ops)
		recording := gtx
		recording.Ops = &c.ops
		recording.Constraints = layout.Constraints{Max: image.Pt(gtx.Constraints.Max.X, gtx.Constraints.Max.Y)}
		c.size = kit.EngravedChart{Values: values, Min: 0, Max: math.NaN(), Height: chartHeight, Ticks: ticks}.Layout(recording, th.Theme).Size
		c.call = record.Stop()
	}
	c.call.Add(gtx.Ops)
	return layout.Dimensions{Size: c.size}
}

// readingTicks labels the first day, the middle one and today; the rest stay blank so the
// labels sit over their days. It reuses into.
func readingTicks(days []model.DailyListening, into []string) []string {
	into = into[:0]
	middle := len(days) / 2
	for index, day := range days {
		switch index {
		case 0, middle:
			into = append(into, shortDate(day.Date))
		case len(days) - 1:
			into = append(into, "Today")
		default:
			into = append(into, "")
		}
	}
	return into
}

func shortDate(date string) string {
	parsed, err := time.Parse(time.DateOnly, date)
	if err != nil {
		return ""
	}
	return parsed.Format("Jan 2")
}

func dayName(date string) string {
	parsed, err := time.Parse(time.DateOnly, date)
	if err != nil {
		return "one day"
	}
	return parsed.Format("Monday, Jan 2")
}

func formatCount(n float64) string { return strconv.Itoa(int(n + .5)) }

func formatListening(seconds float64) string { return listeningTime(int64(seconds)) }

func listeningTime(seconds int64) string {
	minutes := seconds / 60
	if minutes < 60 {
		return fmt.Sprintf("%d min", minutes)
	}
	hours := minutes / 60
	remaining := minutes % 60
	if remaining == 0 {
		return fmt.Sprintf("%d h", hours)
	}
	return fmt.Sprintf("%d h %d min", hours, remaining)
}
