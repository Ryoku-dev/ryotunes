package chrome

import (
	"image"
	"strings"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/anim"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryotunes/internal/client/state"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/sheet"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

// Up next geometry, in dp: its ruled head, its rows (a 40 cover and 2 above
// and below), the gap between rows, and the room its spec strip takes.
const (
	upNextRows             = 3
	upNextHead     unit.Dp = 24
	upNextRow      unit.Dp = 44
	upNextCover    unit.Dp = 40
	upNextRowGap   unit.Dp = 2
	upNextSpecRise unit.Dp = 8
)

// upNext is now playing's look ahead, beside its head: a ruled head
// ("UP NEXT ---- 31 / 67"), the next rows of the queue (cover, title,
// artists, length; a press plays the row, with the Hub's row wash and
// lean), and under them the track's spec strip (source, length, plays,
// album, as many as fit). With nothing queued it reads where the music
// plays from and the spec strip. When the queue moves on the rows travel:
// the new rows rise in as the old ones leave. Rows are dropped first when
// the room is short; the spec strip stays.
type upNext struct {
	rows    [upNextRows]upNextRowState
	travel  anim.Tween
	trackID string
	specs   [4]sheet.Spec

	place          string
	placeAt, total int
}

type upNextRowState struct {
	click kit.Click
	wash  look.RowWash
}

// upNextAt is the queue's i-th row to come: the manual block, then autoplay.
func upNextAt(groups state.QueueGroups, at int) (state.QueueRow, bool) {
	if at < len(groups.UpNextManual) {
		return groups.UpNextManual[at], true
	}
	at -= len(groups.UpNextManual)
	if at < len(groups.Continuation) {
		return groups.Continuation[at], true
	}
	return state.QueueRow{}, false
}

// upNextFit is how many rows fit in height under the head and over the
// spec strip, at most upNextRows.
func upNextFit(height, head, row, gap, spec, upcoming int) int {
	room := height - head - spec
	fit := 0
	for fit < min(upcoming, upNextRows) && room >= row {
		fit++
		room -= row + gap
	}
	return fit
}

// Layout fills gtx.Constraints.Max with the block for track.
func (u *upNext) Layout(gtx layout.Context, e *env.Env, track model.Track) layout.Dimensions {
	size := gtx.Constraints.Max
	gtx.Constraints = layout.Exact(size)
	defer clip.Rect(image.Rectangle{Max: size}).Push(gtx.Ops).Pop()
	snapshot := e.Store.Snapshot()
	groups := e.Store.QueueGroups()
	upcoming := len(groups.UpNextManual) + len(groups.Continuation)

	// the head: where the queue stands, or where the music plays from
	head := gtx.Dp(upNextHead)
	headGTX := gtx
	headGTX.Constraints = layout.Exact(image.Pt(size.X, head))
	if upcoming > 0 {
		u.updatePlace(snapshot.Queue.CurrentIndex+1, len(snapshot.Queue.Items))
		ruledHead(headGTX, e.Th, "Up next", "", u.place, upNextHead, nil)
	} else {
		source := ""
		if name := snapshot.Queue.SourceName; name != nil {
			source = strings.TrimSpace(*name)
		}
		name := "Playing from"
		if source == "" {
			name = "This track"
		}
		ruledHead(headGTX, e.Th, name, source, "", upNextHead, nil)
	}

	// the spec strip at the foot
	specs := u.fitSpecs(gtx, e, track, size.X)
	specTall := 0
	if len(specs) > 0 {
		record := op.Record(gtx.Ops)
		dims := sheet.SpecStrip(gtx, e.Th, specs)
		call := record.Stop()
		specTall = dims.Size.Y + gtx.Dp(upNextSpecRise)
		offset := op.Offset(image.Pt(0, size.Y-dims.Size.Y)).Push(gtx.Ops)
		call.Add(gtx.Ops)
		offset.Pop()
	}

	// the rows, travelling with the track
	row, gap := gtx.Dp(upNextRow), gtx.Dp(upNextRowGap)
	fit := upNextFit(size.Y, head, row, gap, specTall, upcoming)
	if fit == 0 {
		return layout.Dimensions{Size: size}
	}
	rowsArea := image.Pt(size.X, fit*row+(fit-1)*gap)
	offset := op.Offset(image.Pt(0, head)).Push(gtx.Ops)
	area := clip.Rect(image.Rectangle{Max: rowsArea}).Push(gtx.Ops)
	travel := u.travelled(gtx, e, track.VideoID)
	for at := range fit {
		queued, ok := upNextAt(groups, at)
		if !ok {
			break
		}
		y := at*(row+gap) + int(travel*float32(row+gap)+.5)
		rowOffset := op.Offset(image.Pt(0, y)).Push(gtx.Ops)
		rowGTX := gtx
		rowGTX.Constraints = layout.Exact(image.Pt(size.X, row))
		u.layoutRow(rowGTX, e, &u.rows[at], queued)
		rowOffset.Pop()
	}
	area.Pop()
	offset.Pop()
	return layout.Dimensions{Size: size}
}

// travelled is how far, in rows, the rows still stand below their places:
// when the track changes the queue has moved on by one, so each row starts
// in the slot under its own (where it stood) and rises into place over
// th.Motion.Move on OutCubic. It is 0 at rest and under reduced motion.
func (u *upNext) travelled(gtx layout.Context, e *env.Env, trackID string) float32 {
	if trackID != u.trackID {
		moved := u.trackID != ""
		u.trackID = trackID
		u.travel = anim.Tween{Duration: e.Th.Motion.Move, Ease: anim.EaseOutCubic}
		if moved {
			u.travel.Set(gtx, 1)
		}
		u.travel.Set(gtx, 0)
	}
	return u.travel.Value(gtx)
}

// updatePlace writes the queue's place, "31 / 67", when it changes.
func (u *upNext) updatePlace(at, total int) {
	if at == u.placeAt && total == u.total && u.place != "" {
		return
	}
	u.placeAt, u.total = at, total
	u.place = kit.Ordinal(at, total)
}

// fitSpecs is the track's spec strip, as many figures as fit width.
func (u *upNext) fitSpecs(gtx layout.Context, e *env.Env, track model.Track, width int) []sheet.Spec {
	count := 0
	add := func(label, value string) {
		if value = strings.TrimSpace(value); value != "" {
			u.specs[count] = sheet.Spec{Label: label, Value: value}
			count++
		}
	}
	if provider := model.ProviderOf(track.VideoID); track.VideoID != "" {
		add("Source", look.ProviderName(provider))
	}
	add("Length", optionalString(track.Duration))
	add("Plays", optionalString(track.Plays))
	add("Album", optionalString(track.Album))
	for ; count > 0; count-- {
		measure := gtx
		measure.Constraints = layout.Constraints{Max: image.Pt(1<<20, gtx.Constraints.Max.Y)}
		record := op.Record(gtx.Ops)
		dims := sheet.SpecStrip(measure, e.Th, u.specs[:count])
		record.Stop()
		if dims.Size.X <= width {
			break
		}
	}
	return u.specs[:count]
}

// layoutRow draws one queued row: its cover, title and artists, and its
// length, with the Hub's row wash and lean; a press plays it.
func (u *upNext) layoutRow(gtx layout.Context, e *env.Env, row *upNextRowState, queued state.QueueRow) {
	size := gtx.Constraints.Max
	bounds := image.Rectangle{Max: size}
	row.wash.Update(gtx, e.Th, row.click.Hovered(), row.click.Pressed())
	row.wash.Paint(gtx, e.Th, bounds, gtx.Dp(queueRowCut))
	lean := op.Offset(image.Pt(row.wash.Lean(gtx), 0)).Push(gtx.Ops)
	row.click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		inset := (size.Y - gtx.Dp(upNextCover)) / 2
		return layout.Inset{Top: unit.Dp(float32(inset) / gtx.Metric.PxPerDp), Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			// the row's height reaches here as a minimum; without clearing it
			// the length would stand at the row's top, not centred on the cover
			gtx.Constraints.Min.Y = 0
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					side := gtx.Dp(upNextCover)
					return look.Art(gtx, e.Th, e.Art, optionalString(queued.Item.Thumbnail), image.Pt(side, side), look.Shape{Cut: 3})
				}),
				layout.Rigid(layout.Spacer{Width: 8}.Layout),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.Y = 0
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(kit.Text(e.Th.Theme, e.Th.T.Row, e.Th.R.Fg, queueTrackTitle(queued.Item))),
						layout.Rigid(kit.Text(e.Th.Theme, e.Th.T.Caption, e.Th.R.Fg2, strings.TrimSpace(queued.Item.Artists))),
					)
				}),
				layout.Rigid(layout.Spacer{Width: 8}.Layout),
				layout.Rigid(kit.Text(e.Th.Theme, e.Th.T.Mono, e.Th.R.Fg3, optionalString(queued.Item.Duration))),
			)
		})
	})
	lean.Pop()
	if row.click.Clicked(gtx) {
		callVoid(e, protocol.MethodPlayIndex, protocol.IndexParams{Index: queued.Index})
	}
}
