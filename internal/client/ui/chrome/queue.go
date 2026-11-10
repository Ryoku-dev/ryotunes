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
	"gioui.org/op/clip"
	"gioui.org/op/paint"

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
}

type queueRowState struct {
	click      kit.Click
	remove     kit.IconPlaque
	hover      look.Hover
	hoverValue float32
	drag       queueDrag
	context    queueContextArea
	arrival    anim.Tween
	arrivalSet bool
	removal    anim.Tween
	removing   bool
	removeSent bool
	lastSeen   uint64
	children   [5]layout.FlexChild
}

type queueDrag struct {
	tag       struct{}
	active    bool
	pointerID pointer.ID
	startY    float32
	target    int
}

type queueContextArea struct{ tag struct{} }

// Queue renders the editable grouped playback queue.
type Queue struct {
	scroll      look.Scroller
	clear       kit.Plaque
	more        kit.IconPlaque
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
		for _, row := range rows {
			videoID := row.Item.VideoID
			occurrence := queue.seen[videoID]
			queue.seen[videoID] = occurrence + 1
			key := queueEntryKey{videoID: videoID, occurrence: occurrence}
			item := queue.items[key]
			if item == nil {
				item = new(queueRowState)
				item.arrival.Duration = notch
				item.arrival.Ease = anim.EaseChisel
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
			queue.entries = append(queue.entries, queueEntry{kind: queueTrack, row: row, key: key})
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
	trailing := layout.Widget(nil)
	switch entry.title {
	case "Now playing":
		trailing = func(gtx layout.Context) layout.Dimensions {
			queue.more.Kind = kit.PlaqueQuiet
			queue.more.Icon = look.Icon.More
			queue.more.Description = "Queue actions"
			queue.more.Small = true
			dims := queue.more.Layout(gtx, e.Th.Theme)
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
	header := kit.GroupHeader(e.Th.Theme, entry.title, entry.count)
	if entry.title == "Now playing" {
		header = queueNowPlayingHeader(e)
	}
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Flexed(1, header),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if trailing == nil {
				return layout.Dimensions{}
			}
			return trailing(gtx)
		}),
	)
}

func queueNowPlayingHeader(e *env.Env) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		source := ""
		if name := e.Store.Snapshot().Queue.SourceName; name != nil {
			source = strings.TrimSpace(*name)
		}
		return layout.Inset{
			Top: tokens.SizeGroupHeaderTop, Right: tokens.SizeGroupHeaderRight,
			Bottom: tokens.SizeGroupHeaderBottom, Left: tokens.SizeGroupHeaderLeft,
		}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			children := []layout.FlexChild{
				layout.Rigid(kit.Text(e.Th.Theme, e.Th.T.Group, e.Th.R.Fg3, "Now playing")),
			}
			if source != "" {
				children = append(children,
					layout.Rigid(layout.Spacer{Width: 4}.Layout),
					layout.Rigid(kit.Text(e.Th.Theme, e.Th.T.Group, e.Th.R.Fg2, "· From "+source)),
				)
			}
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
		})
	}
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
	queueSnapshot := e.Store.Snapshot().Queue
	playing := row.Index == queueSnapshot.CurrentIndex
	if item.context.Pressed(gtx) && e.Menu != nil {
		track := row.Item
		e.Menu(env.MenuTarget{Track: &track, Context: "queue", QueueIndex: row.Index})
	}
	rowHeight := gtx.Dp(56)
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
		gtx.Constraints.Min.Y = rowHeight
		gtx.Constraints.Max.Y = rowHeight
	}

	rowClip := clip.Rect(image.Rect(0, 0, gtx.Constraints.Max.X, rowHeight)).Push(gtx.Ops)
	hoverValue := item.hoverValue
	dims, nextHover := item.hover.Layout(gtx, e.Th, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.Y = rowHeight
		gtx.Constraints.Max.Y = rowHeight
		bounds := image.Rect(0, 0, gtx.Constraints.Max.X, rowHeight)
		if hoverInk := palette.WithAlpha(e.Th.R.Stone3, hoverValue); hoverInk.A > 0 {
			paint.FillShape(gtx.Ops, hoverInk, clip.Rect(bounds).Op())
		}
		return item.click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: 8, Right: 8, Bottom: 8, Left: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				item.children[0] = layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					size := image.Pt(gtx.Dp(40), gtx.Dp(40))
					return look.Art(gtx, e.Th, e.Art, optionalString(row.Item.Thumbnail), size, look.Shape{Cut: 3})
				})
				item.children[1] = layout.Rigid(layout.Spacer{Width: 13}.Layout)
				item.children[2] = layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					titleInk := e.Th.R.Fg
					if playing {
						titleInk = e.Th.Accent.Ink
					} else if row.Item.Autoplay {
						titleInk = e.Th.R.Fg2
					}
					return layout.Flex{Axis: layout.Vertical, Alignment: layout.Start}.Layout(gtx,
						layout.Rigid(kit.Text(e.Th.Theme, e.Th.T.Row, titleInk, queueTrackTitle(row.Item))),
						layout.Rigid(kit.Text(e.Th.Theme, e.Th.T.Caption, e.Th.R.Fg2, strings.TrimSpace(row.Item.Artists))),
					)
				})
				item.children[3] = layout.Rigid(layout.Spacer{Width: 8}.Layout)
				item.children[4] = layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if !playing && hoverValue > .05 {
						item.remove.Kind = kit.PlaqueQuiet
						item.remove.Icon = look.Icon.Remove
						item.remove.Description = "Remove from queue"
						item.remove.Small = true
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								target, moved, dims := item.drag.Layout(gtx, e.Th, hoverValue, row.Index, queueSnapshot.CurrentIndex+1, len(queueSnapshot.Items)-1)
								if moved && target != row.Index {
									if target > row.Index {
										target--
									}
									callVoid(e, protocol.MethodMoveInQueue, protocol.MoveInQueueParams{From: row.Index, To: target})
								}
								return dims
							}),
							layout.Rigid(layout.Spacer{Width: 4}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								dims := item.remove.Layout(gtx, e.Th.Theme)
								if item.remove.Clicked(gtx) && !item.removing {
									item.removing = true
									item.removal.Duration = e.Th.Motion.Unfold
									item.removal.Ease = anim.EaseChisel
									item.removal.Set(gtx, 1)
									item.removal.Set(gtx, 0)
								}
								return dims
							}),
						)
					}
					return kit.Text(e.Th.Theme, e.Th.T.Mono, e.Th.R.Fg3, optionalString(row.Item.Duration))(gtx)
				})
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, item.children[:]...)
			})
		})
	})
	rowClip.Pop()
	item.hoverValue = nextHover
	if item.click.Clicked(gtx) {
		callVoid(e, protocol.MethodPlayIndex, protocol.IndexParams{Index: row.Index})
	}
	item.context.Layout(gtx, dims.Size)
	if item.arrivalSet {
		arrival := item.arrival.Value(gtx)
		if arrival < 1 {
			kit.Notch(gtx.Ops, image.Point{}, gtx.Dp(e.Th.Cut.Notch), palette.WithAlpha(e.Th.Accent.Ink, 1-arrival))
		}
	}
	return dims
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
				delta := int(math.Round(float64((pointerEvent.Position.Y - drag.startY) / float32(max(1, gtx.Dp(56))))))
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
	size := image.Pt(gtx.Dp(24), gtx.Dp(40))
	gtx.Constraints = layout.Exact(size)
	region := clip.Rect(image.Rectangle{Max: size}).Push(gtx.Ops)
	semantic.Button.Add(gtx.Ops)
	semantic.DescriptionOp("Reorder track").Add(gtx.Ops)
	event.Op(gtx.Ops, &drag.tag)
	ink := palette.WithAlpha(th.R.Fg2, alpha)
	if drag.active {
		ink = th.R.OnSel
		paint.FillShape(gtx.Ops, th.R.Sel, clip.Rect(image.Rectangle{Max: size}).Op())
	}
	iconGTX := gtx
	iconSize := gtx.Dp(18)
	iconGTX.Constraints = layout.Exact(image.Pt(iconSize, iconSize))
	layout.Center.Layout(gtx, func(layout.Context) layout.Dimensions { return look.Icon.Drag.Layout(iconGTX, ink) })
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
