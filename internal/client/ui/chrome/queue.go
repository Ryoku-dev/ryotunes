package chrome

import (
	"image"
	"math"
	"strings"
	"time"

	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/anim"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/tokens"
	"github.com/ryoku-dev/ryotunes/internal/client/state"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/widgets"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

// Queue row geometry, in dp: the row, its leading column (the numeral, the
// grip on hover, the playing glyph) and the Hub's row cut.
const (
	queueRowTall unit.Dp = 56
	queueLead    unit.Dp = 24
	queueRowCut  unit.Dp = 6
)

type queueEntryKind uint8

const (
	queueSection queueEntryKind = iota
	queueTrack
)

type queueEntryKey struct {
	videoID    string
	occurrence int
}

type queueEntry struct {
	kind  queueEntryKind
	title string
	count int
	row   state.QueueRow
	key   queueEntryKey
	// ordinal is the row's place in its group, from 1; last marks the
	// group's last row, which carries no rule under it
	ordinal int
	last    bool
}

type queueRowState struct {
	click       kit.Click
	remove      kit.IconPlaque
	removeGlyph widgets.PressGlyph
	wash        look.RowWash
	selected    look.SelectWash
	drag        queueDrag
	context     queueContextArea
	arrival     anim.Tween
	arrivalSet  bool
	removal     anim.Tween
	removing    bool
	removeSent  bool
	lastSeen    uint64
	children    [7]layout.FlexChild
}

type queueDrag struct {
	tag       struct{}
	active    bool
	pointerID pointer.ID
	startY    float32
	target    int
}

type queueContextArea struct{ tag struct{} }

// Queue renders the editable grouped playback queue as the Hub's ruled
// list: ruled group heads ("UP NEXT ---- 06"), rows numbered in the
// smallest mono with a vein rule between them, the Hub's row wash and lean
// on hover and press, and the selection wash on the row that plays.
type Queue struct {
	scroll      look.Scroller
	clear       kit.Plaque
	more        kit.IconPlaque
	moreGlyph   widgets.PressGlyph
	menu        kit.Menu
	items       map[queueEntryKey]*queueRowState
	seen        map[string]int
	entries     []queueEntry
	build       uint64
	initialized bool
	menuTag     struct{}
}

// Layout draws the queue body used by the right panel.
func (queue *Queue) Layout(gtx layout.Context, e *env.Env) layout.Dimensions {
	return queue.LayoutList(gtx, e, layout.Inset{Bottom: 13, Left: 13, Right: 13})
}

// LayoutList draws the queue rows inside rows, or the empty state, which owns
// its spacing, in the whole area.
func (queue *Queue) LayoutList(gtx layout.Context, e *env.Env, rows layout.Inset) layout.Dimensions {
	groups := e.Store.QueueGroups()
	queue.buildEntries(gtx, e.Th.Motion.Notch, groups)
	if len(queue.entries) == 0 {
		return widgets.EmptyState(e, look.Icon.Queue, "Nothing queued", "Play something and the next tracks show here.")(gtx)
	}
	return rows.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return queue.scroll.Layout(gtx, e.Th, len(queue.entries), func(gtx layout.Context, index int) layout.Dimensions {
			entry := queue.entries[index]
			if entry.kind == queueTrack {
				return queue.layoutRow(gtx, e, entry)
			}
			return queue.layoutSection(gtx, e, entry)
		})
	})
}

func (queue *Queue) buildEntries(gtx layout.Context, notch time.Duration, groups state.QueueGroups) {
	if queue.items == nil {
		queue.items = make(map[queueEntryKey]*queueRowState)
		queue.seen = make(map[string]int)
	}
	clear(queue.seen)
	queue.entries = queue.entries[:0]
	queue.build++
	appendGroup := func(title string, rows []state.QueueRow) {
		if len(rows) == 0 {
			return
		}
		queue.entries = append(queue.entries, queueEntry{kind: queueSection, title: title, count: len(rows)})
		for at, row := range rows {
			videoID := row.Item.VideoID
			occurrence := queue.seen[videoID]
			queue.seen[videoID] = occurrence + 1
			key := queueEntryKey{videoID: videoID, occurrence: occurrence}
			item := queue.items[key]
			if item == nil {
				item = new(queueRowState)
				item.arrival.Duration = notch
				item.arrival.Ease = anim.EaseOutCubic
				if queue.initialized {
					item.arrival.Set(gtx, 0)
					item.arrival.Set(gtx, 1)
					item.arrivalSet = true
				} else {
					item.arrival.Set(gtx, 1)
				}
				queue.items[key] = item
			}
			item.lastSeen = queue.build
			queue.entries = append(queue.entries, queueEntry{
				kind: queueTrack, row: row, key: key, ordinal: at + 1, last: at == len(rows)-1,
			})
		}
	}
	if groups.NowPlaying != nil {
		appendGroup("Now playing", []state.QueueRow{*groups.NowPlaying})
	}
	appendGroup("Up next", groups.UpNextManual)
	appendGroup("Autoplay", groups.Continuation)
	for key, item := range queue.items {
		if item.lastSeen != queue.build && !item.removing {
			delete(queue.items, key)
		}
	}
	queue.initialized = true
}

func (queue *Queue) layoutSection(gtx layout.Context, e *env.Env, entry queueEntry) layout.Dimensions {
	var trailing layout.Widget
	lead, count := "", entry.count
	switch entry.title {
	case "Now playing":
		count = -1
		if name := e.Store.Snapshot().Queue.SourceName; name != nil && strings.TrimSpace(*name) != "" {
			lead = "From " + strings.TrimSpace(*name)
		}
		trailing = func(gtx layout.Context) layout.Dimensions {
			queue.more.Kind = kit.PlaqueQuiet
			queue.more.Description = "Queue actions"
			queue.more.Small = true
			dims := queue.moreGlyph.Layout(gtx, e.Th, &queue.more, look.Icon.More)
			if queue.more.Clicked(gtx) && e.Overlays != nil {
				anchor := e.Overlays.AnchorOf(&queue.more.Clickable, dims.Size)
				e.Overlays.Open(&queue.menuTag, anchor, look.Placement{Prefer: look.Below, Align: look.End, Width: 240}, func(gtx layout.Context) layout.Dimensions {
					return queue.layoutMenu(gtx, e)
				})
			}
			return dims
		}
	case "Up next":
		trailing = func(gtx layout.Context) layout.Dimensions {
			queue.clear.Kind = kit.PlaqueQuiet
			queue.clear.Label = "Clear"
			queue.clear.Small = true
			dims := queue.clear.Layout(gtx, e.Th.Theme)
			if queue.clear.Clicked(gtx) {
				callVoid(e, protocol.MethodClearQueued, protocol.Null{})
			}
			return dims
		}
	}
	return ruledHead(gtx, e.Th, entry.title, lead, count, trailing)
}

func (queue *Queue) layoutMenu(gtx layout.Context, e *env.Env) layout.Dimensions {
	snapshot := e.Store.Snapshot()
	stopLabel := "Stop after this track"
	if snapshot.Playback.StopAfterCurrent {
		stopLabel = "Keep playing after this track"
	}
	items := []kit.MenuItem{
		{Icon: look.Icon.Pause, Label: stopLabel},
		{Icon: look.Icon.PlaylistAdd, Label: "Save queue as playlist"},
		{Icon: look.Icon.Remove, Label: "Clear", Danger: true},
	}
	dims := queue.menu.Layout(gtx, e.Th.Theme, items)
	if index, clicked := queue.menu.Clicked(gtx); clicked {
		if e.Overlays != nil {
			e.Overlays.Close(&queue.menuTag)
		}
		switch index {
		case 0:
			callVoid(e, protocol.MethodSetStopAfterCurrent, protocol.SetStopAfterCurrentParams{Enabled: !snapshot.Playback.StopAfterCurrent})
		case 1:
			queue.saveAsPlaylist(e, snapshot)
		case 2:
			callVoid(e, protocol.MethodClearQueued, protocol.Null{})
		}
	}
	return dims
}

func (queue *Queue) layoutRow(gtx layout.Context, e *env.Env, entry queueEntry) layout.Dimensions {
	item := queue.items[entry.key]
	if item == nil {
		return layout.Dimensions{}
	}
	row := entry.row
	snapshot := e.Store.Snapshot()
	queueSnapshot := snapshot.Queue
	playing := row.Index == queueSnapshot.CurrentIndex
	if item.context.Pressed(gtx) && e.Menu != nil {
		track := row.Item
		e.Menu(env.MenuTarget{Track: &track, Context: "queue", QueueIndex: row.Index})
	}
	rowHeight := gtx.Dp(queueRowTall)
	if item.removing {
		progress := item.removal.Value(gtx)
		if progress <= 0.001 && !item.removeSent {
			item.removeSent = true
			callVoid(e, protocol.MethodRemoveFromQueue, protocol.IndexParams{Index: row.Index})
		}
		rowHeight = int(float32(rowHeight) * progress)
		if rowHeight <= 0 {
			return layout.Dimensions{}
		}
	}
	width := gtx.Constraints.Max.X
	gtx.Constraints = layout.Exact(image.Pt(width, rowHeight))
	bounds := image.Rect(0, 0, width, rowHeight)
	rowClip := clip.Rect(bounds).Push(gtx.Ops)

	// where you are, then the hover and press washes over it
	cut := gtx.Dp(queueRowCut)
	if playing {
		item.selected.Layout(gtx, e.Th, bounds, 0, cut)
	} else {
		item.selected.Layout(gtx, e.Th, image.Rectangle{}, 0, cut)
	}
	hover, _ := item.wash.Update(gtx, e.Th, item.click.Hovered() || item.drag.active, item.click.Pressed() || item.drag.active)
	item.wash.Paint(gtx, e.Th, bounds, cut)

	// the vein rule under the row, from the art to the end, still while the
	// row leans
	if !entry.last && !item.removing {
		hair := max(1, gtx.Dp(unit.Dp(tokens.SizeHairline)))
		start := gtx.Dp(8 + queueLead + 8)
		paint.FillShape(gtx.Ops, e.Th.R.Line(), clip.Rect(image.Rect(start, rowHeight-hair, width-gtx.Dp(8), rowHeight)).Op())
	}

	lean := item.wash.Lean(gtx)
	leanOffset := op.Offset(image.Pt(lean, 0)).Push(gtx.Ops)
	dims := item.click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: 8, Right: 8, Bottom: 8, Left: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			item.children[0] = layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return queue.layoutLead(gtx, e, item, entry, playing, snapshot.Playback.Paused, hover, queueSnapshot)
			})
			item.children[1] = layout.Rigid(layout.Spacer{Width: 8}.Layout)
			item.children[2] = layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				size := image.Pt(gtx.Dp(40), gtx.Dp(40))
				return look.Art(gtx, e.Th, e.Art, optionalString(row.Item.Thumbnail), size, look.Shape{Cut: 3})
			})
			item.children[3] = layout.Rigid(layout.Spacer{Width: 13}.Layout)
			item.children[4] = layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				titleInk := e.Th.R.Fg
				if row.Item.Autoplay && !playing {
					titleInk = e.Th.R.Fg2
				}
				return layout.Flex{Axis: layout.Vertical, Alignment: layout.Start}.Layout(gtx,
					layout.Rigid(kit.Text(e.Th.Theme, e.Th.T.Row, titleInk, queueTrackTitle(row.Item))),
					layout.Rigid(kit.Text(e.Th.Theme, e.Th.T.Caption, e.Th.R.Fg2, strings.TrimSpace(row.Item.Artists))),
				)
			})
			item.children[5] = layout.Rigid(layout.Spacer{Width: 8}.Layout)
			item.children[6] = layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if !playing && hover > .05 {
					item.remove.Kind = kit.PlaqueQuiet
					item.remove.Description = "Remove from queue"
					item.remove.Small = true
					dims := item.removeGlyph.Layout(gtx, e.Th, &item.remove, look.Icon.Remove)
					if item.remove.Clicked(gtx) && !item.removing {
						item.removing = true
						item.removal.Duration = e.Th.Motion.HubUnfold
						item.removal.Ease = anim.EaseOutCubic
						item.removal.Set(gtx, 1)
						item.removal.Set(gtx, 0)
					}
					return dims
				}
				return kit.Text(e.Th.Theme, e.Th.T.Mono, e.Th.R.Fg3, optionalString(row.Item.Duration))(gtx)
			})
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, item.children[:]...)
		})
	})
	leanOffset.Pop()
	rowClip.Pop()
	if item.click.Clicked(gtx) {
		callVoid(e, protocol.MethodPlayIndex, protocol.IndexParams{Index: row.Index})
	}
	item.context.Layout(gtx, bounds.Size())
	if item.arrivalSet {
		arrival := item.arrival.Value(gtx)
		if arrival < 1 {
			kit.Notch(gtx.Ops, image.Point{}, gtx.Dp(e.Th.Cut.Notch), palette.WithAlpha(e.Th.Accent.Ink, 1-arrival))
		}
	}
	dims.Size = bounds.Size()
	return dims
}

// layoutLead draws the row's leading column: the playing glyph in the
// accent on the row that plays, else the row's numeral in the smallest
// mono, fading into the reorder grip as the row is hovered.
func (queue *Queue) layoutLead(gtx layout.Context, e *env.Env, item *queueRowState, entry queueEntry, playing, paused bool, hover float32, snapshot model.QueueSnapshot) layout.Dimensions {
	size := image.Pt(gtx.Dp(queueLead), gtx.Dp(40))
	gtx.Constraints = layout.Exact(size)
	if playing {
		ink := e.Th.Accent.Ink
		if paused {
			ink = e.Th.R.Fg3
		}
		side := gtx.Dp(16)
		offset := op.Offset(image.Pt((size.X-side)/2, (size.Y-side)/2)).Push(gtx.Ops)
		look.DrawGlyph(gtx, look.Icon.Play, ink, side, 0, 1)
		offset.Pop()
		return layout.Dimensions{Size: size}
	}
	grip := hover
	if item.drag.active {
		grip = 1
	}
	if grip < 1 {
		alpha := paint.PushOpacity(gtx.Ops, 1-grip)
		numeral := gtx
		numeral.Constraints.Min = image.Point{}
		record := op.Record(gtx.Ops)
		dims := kit.LayoutSpacedCaps(numeral, e.Th.Theme, e.Th.T.MonoSm, e.Th.R.Fg3, 0, kit.Pad2(entry.ordinal))
		call := record.Stop()
		offset := op.Offset(image.Pt((size.X-dims.Size.X)/2, (size.Y-dims.Size.Y)/2)).Push(gtx.Ops)
		call.Add(gtx.Ops)
		offset.Pop()
		alpha.Pop()
	}
	if grip > .05 {
		target, moved, _ := item.drag.Layout(gtx, e.Th, grip, entry.row.Index, snapshot.CurrentIndex+1, len(snapshot.Items)-1)
		if moved && target != entry.row.Index {
			if target > entry.row.Index {
				target--
			}
			callVoid(e, protocol.MethodMoveInQueue, protocol.MoveInQueueParams{From: entry.row.Index, To: target})
		}
	}
	return layout.Dimensions{Size: size}
}

func queueTrackTitle(track model.Track) string {
	title := strings.TrimSpace(track.Title)
	if title == "" {
		return "Untitled track"
	}
	return title
}

func (area *queueContextArea) Pressed(gtx layout.Context) bool {
	pressed := false
	for {
		raw, ok := gtx.Event(pointer.Filter{Target: &area.tag, Kinds: pointer.Press})
		if !ok {
			break
		}
		pointerEvent, ok := raw.(pointer.Event)
		if ok && pointerEvent.Buttons&pointer.ButtonSecondary != 0 {
			pressed = true
		}
	}
	return pressed
}

func (area *queueContextArea) Layout(gtx layout.Context, size image.Point) {
	pass := pointer.PassOp{}.Push(gtx.Ops)
	region := clip.Rect(image.Rectangle{Max: size}).Push(gtx.Ops)
	event.Op(gtx.Ops, &area.tag)
	region.Pop()
	pass.Pop()
}

// Layout draws the reorder grip at alpha and runs its drag. While dragged
// the row takes the press wash (the row draws it) and the grip stands in
// fg; nothing turns solid.
func (drag *queueDrag) Layout(gtx layout.Context, th *look.Theme, alpha float32, index, first, last int) (int, bool, layout.Dimensions) {
	moved := false
	for {
		raw, ok := gtx.Event(pointer.Filter{Target: &drag.tag, Kinds: pointer.Press | pointer.Drag | pointer.Release | pointer.Cancel})
		if !ok {
			break
		}
		pointerEvent, ok := raw.(pointer.Event)
		if !ok {
			continue
		}
		switch pointerEvent.Kind {
		case pointer.Press:
			if pointerEvent.Buttons&pointer.ButtonPrimary == 0 {
				continue
			}
			drag.active = true
			drag.pointerID = pointerEvent.PointerID
			drag.startY = pointerEvent.Position.Y
			drag.target = index
			gtx.Execute(pointer.GrabCmd{Tag: &drag.tag, ID: pointerEvent.PointerID})
		case pointer.Drag:
			if drag.active && pointerEvent.PointerID == drag.pointerID {
				delta := int(math.Round(float64((pointerEvent.Position.Y - drag.startY) / float32(max(1, gtx.Dp(queueRowTall))))))
				drag.target = max(first, min(last, index+delta))
			}
		case pointer.Release:
			if drag.active && pointerEvent.PointerID == drag.pointerID {
				drag.active = false
				moved = true
			}
		case pointer.Cancel:
			drag.active = false
		}
	}
	size := gtx.Constraints.Min
	region := clip.Rect(image.Rectangle{Max: size}).Push(gtx.Ops)
	semantic.Button.Add(gtx.Ops)
	semantic.DescriptionOp("Reorder track").Add(gtx.Ops)
	event.Op(gtx.Ops, &drag.tag)
	pointer.CursorGrab.Add(gtx.Ops)
	ink := palette.WithAlpha(th.R.Fg2, alpha)
	if drag.active {
		ink = th.R.Fg
	}
	side := gtx.Dp(18)
	offset := op.Offset(image.Pt((size.X-side)/2, (size.Y-side)/2)).Push(gtx.Ops)
	look.DrawGlyph(gtx, look.Icon.Drag, ink, side, 0, 1)
	offset.Pop()
	region.Pop()
	return drag.target, moved, layout.Dimensions{Size: size}
}

func (queue *Queue) saveAsPlaylist(e *env.Env, snapshot state.State) {
	if len(snapshot.Queue.Items) == 0 {
		return
	}
	title := "Queue"
	if snapshot.Queue.SourceName != nil && strings.TrimSpace(*snapshot.Queue.SourceName) != "" {
		title = strings.TrimSpace(*snapshot.Queue.SourceName)
	}
	act.Call[protocol.PlaylistIDResult](e, protocol.MethodCreatePlaylist, protocol.CreatePlaylistParams{Title: title}, func(id protocol.PlaylistIDResult, err error) {
		if err != nil {
			if e.Toast != nil {
				e.Toast(err.Error(), env.ToastError)
			}
			return
		}
		for _, track := range snapshot.Queue.Items {
			act.AddToPlaylist(e, string(id), track)
		}
	})
}
