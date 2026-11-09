package chrome

import (
	"image"
	"math"
	"strconv"

	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op/clip"

	"github.com/ryoku-dev/ryotunes/internal/client/lithic"
	"github.com/ryoku-dev/ryotunes/internal/client/state"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/widgets"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

type queueEntryKind uint8

const (
	queueHeader queueEntryKind = iota
	queueTrack
)

type queueEntry struct {
	kind  queueEntryKind
	title string
	count int
	row   state.QueueRow
	key   string
}

type queueItemState struct {
	row      widgets.TrackRow
	drag     queueDrag
	lastSeen uint64
}

type queueDrag struct {
	tag       int
	active    bool
	pointerID pointer.ID
	startY    float32
	target    int
}

// QueuePanel renders the grouped, editable daemon queue.
type QueuePanel struct {
	list     lithic.List
	clear    lithic.Button
	save     lithic.Button
	autoplay lithic.Toggle
	items    map[string]*queueItemState
	entries  []queueEntry
	seen     map[string]int
	build    uint64

	autoplayPending bool
	autoplayTarget  bool
}

// Layout renders queue groups, row actions, reordering, and queue-wide acts.
func (p *QueuePanel) Layout(gtx layout.Context, e *env.Env) layout.Dimensions {
	snapshot := e.Store.Snapshot()
	groups := e.Store.QueueGroups()
	p.buildEntries(groups)

	if p.clear.Clicked(gtx) {
		callVoid(e, protocol.MethodClearQueued, protocol.Null{})
	}
	if p.save.Clicked(gtx) {
		p.saveAsPlaylist(e, snapshot)
	}
	if p.autoplayPending {
		if settingEnabled(snapshot.Settings, model.SettingAutoplay, true) == p.autoplayTarget {
			p.autoplayPending = false
		} else {
			p.autoplay.Value = p.autoplayTarget
		}
	} else {
		p.autoplay.Value = settingEnabled(snapshot.Settings, model.SettingAutoplay, true)
	}

	return layout.Inset{Top: 21, Bottom: 13, Left: 13, Right: 13}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Flexed(1, lithic.Text(e.Th, e.Th.T.Section, e.Th.R.Fg, "Queue")),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						before := p.autoplay.Value
						dims := p.autoplay.Layout(gtx, e.Th, "Autoplay")
						if p.autoplay.Value != before {
							p.autoplayTarget = p.autoplay.Value
							p.autoplayPending = true
							callVoid(e, protocol.MethodSetSetting, protocol.SetSettingParams{Key: string(model.SettingAutoplay), Value: strconv.FormatBool(p.autoplay.Value)})
						}
						return dims
					}),
				)
			}),
			layout.Rigid(layout.Spacer{Height: 13}.Layout),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				if groups.NowPlaying == nil && len(groups.UpNextManual) == 0 && len(groups.Continuation) == 0 {
					return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(lithic.Text(e.Th, e.Th.T.Title, e.Th.R.Fg, "Nothing queued")),
							layout.Rigid(layout.Spacer{Height: 8}.Layout),
							layout.Rigid(lithic.Text(e.Th, e.Th.T.Body, e.Th.R.Fg2, "Play a track to start the queue.")),
						)
					})
				}
				return p.list.Layout(gtx, e.Th, len(p.entries), func(gtx layout.Context, index int) layout.Dimensions {
					entry := p.entries[index]
					if entry.kind == queueHeader {
						return lithic.GroupHeader(e.Th, entry.title, entry.count)(gtx)
					}
					return p.layoutRow(gtx, e, snapshot.Queue, entry)
				})
			}),
			layout.Rigid(layout.Spacer{Height: 13}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceBetween}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						p.clear.Small = true
						return p.clear.Layout(gtx, e.Th, lithic.DangerLine, lithic.Icon("delete"), "Clear queue")
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						p.save.Small = true
						return p.save.Layout(gtx, e.Th, lithic.Line, lithic.Icon("playlist_add"), "Save as playlist")
					}),
				)
			}),
		)
	})
}

func (p *QueuePanel) buildEntries(groups state.QueueGroups) {
	if p.items == nil {
		p.items = make(map[string]*queueItemState)
		p.seen = make(map[string]int)
	}
	clear(p.seen)
	p.entries = p.entries[:0]
	p.build++
	appendGroup := func(title string, rows []state.QueueRow) {
		if len(rows) == 0 {
			return
		}
		p.entries = append(p.entries, queueEntry{kind: queueHeader, title: title, count: len(rows)})
		for _, row := range rows {
			key := row.Item.VideoID
			occurrence := p.seen[key]
			p.seen[key] = occurrence + 1
			key += "#" + strconv.Itoa(occurrence)
			item := p.items[key]
			if item == nil {
				item = new(queueItemState)
				p.items[key] = item
			}
			item.lastSeen = p.build
			p.entries = append(p.entries, queueEntry{kind: queueTrack, row: row, key: key})
		}
	}
	if groups.NowPlaying != nil {
		appendGroup("Now playing", []state.QueueRow{*groups.NowPlaying})
	}
	appendGroup("Up next", groups.UpNextManual)
	appendGroup("Continuation", groups.Continuation)
	for key, item := range p.items {
		if item.lastSeen != p.build {
			delete(p.items, key)
		}
	}
}

func (p *QueuePanel) layoutRow(gtx layout.Context, e *env.Env, queue model.QueueSnapshot, entry queueEntry) layout.Dimensions {
	item := p.items[entry.key]
	if item == nil {
		return layout.Dimensions{}
	}
	row := entry.row
	playing := row.Index == queue.CurrentIndex
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			opts := widgets.TrackRowOpts{
				Index: row.Index, ShowArt: true, ShowAlbum: true, Playing: playing, Context: "queue",
			}
			if !playing {
				opts.HoverActionActive = item.drag.active
				opts.HoverAction = func(gtx layout.Context) layout.Dimensions {
					target, moved, dims := item.drag.Layout(gtx, e.Th, row.Index, queue.CurrentIndex+1, len(queue.Items)-1)
					if moved && target != row.Index {
						callVoid(e, protocol.MethodMoveInQueue, protocol.MoveInQueueParams{From: row.Index, To: target})
					}
					return dims
				}
			}
			dims := item.row.Layout(gtx, e, row.Item, opts)
			if item.row.Activated() {
				callVoid(e, protocol.MethodPlayIndex, protocol.IndexParams{Index: row.Index})
			}
			return dims
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			caption := ""
			if row.Item.QueuedFrom != nil && *row.Item.QueuedFrom != "" {
				caption = "Queued from " + *row.Item.QueuedFrom
			} else if row.Item.Autoplay {
				caption = "Autoplay"
			}
			if caption == "" {
				return layout.Dimensions{}
			}
			return layout.Inset{Left: 16, Bottom: 4}.Layout(gtx,
				lithic.Text(e.Th, e.Th.T.Caption, e.Th.R.Fg3, caption),
			)
		}),
	)
}

func (d *queueDrag) Layout(gtx layout.Context, th *lithic.Theme, index, first, last int) (int, bool, layout.Dimensions) {
	moved := false
	for {
		raw, ok := gtx.Event(pointer.Filter{Target: &d.tag, Kinds: pointer.Press | pointer.Drag | pointer.Release | pointer.Cancel})
		if !ok {
			break
		}
		e, ok := raw.(pointer.Event)
		if !ok {
			continue
		}
		switch e.Kind {
		case pointer.Press:
			if e.Buttons&pointer.ButtonPrimary == 0 {
				continue
			}
			d.active, d.pointerID, d.startY, d.target = true, e.PointerID, e.Position.Y, index
			gtx.Execute(pointer.GrabCmd{Tag: &d.tag, ID: e.PointerID})
		case pointer.Drag:
			if d.active && e.PointerID == d.pointerID {
				delta := int(math.Round(float64((e.Position.Y - d.startY) / float32(max(1, gtx.Dp(60))))))
				d.target = max(first, min(last, index+delta))
			}
		case pointer.Release:
			if d.active && e.PointerID == d.pointerID {
				moved = true
				d.active = false
			}
		case pointer.Cancel:
			d.active = false
		}
	}

	size := image.Pt(gtx.Dp(32), gtx.Dp(32))
	gtx.Constraints = layout.Exact(size)
	region := clip.Rect(image.Rectangle{Max: size}).Push(gtx.Ops)
	kind := lithic.Quiet
	if d.active {
		kind = lithic.On
	}
	fill, ink, line := th.R.Stone2, th.R.Fg2, th.R.Vein
	if kind == lithic.On {
		fill, ink, line = th.R.Sel, th.R.OnSel, th.R.Sel
	}
	lithic.FillCut(gtx.Ops, image.Rectangle{Max: size}, gtx.Dp(6), fill)
	lithic.StrokeCut(gtx.Ops, image.Rectangle{Max: size}, gtx.Dp(6), max(1, gtx.Dp(1)), line)
	iconSize := gtx.Dp(18)
	iconGTX := gtx
	iconGTX.Constraints = layout.Exact(image.Pt(iconSize, iconSize))
	layout.Center.Layout(gtx, func(layout.Context) layout.Dimensions {
		return lithic.Icon("drag_handle").Layout(iconGTX, ink)
	})
	semantic.Button.Add(gtx.Ops)
	semantic.DescriptionOp("Reorder track").Add(gtx.Ops)
	event.Op(gtx.Ops, &d.tag)
	region.Pop()
	return d.target, moved, layout.Dimensions{Size: size}
}

func (p *QueuePanel) saveAsPlaylist(e *env.Env, snapshot state.State) {
	items := snapshot.Queue.Items
	if len(items) == 0 {
		return
	}
	title := "Queue"
	if snapshot.Queue.SourceName != nil && *snapshot.Queue.SourceName != "" {
		title = *snapshot.Queue.SourceName
	}
	act.Call[protocol.PlaylistIDResult](e, protocol.MethodCreatePlaylist, protocol.CreatePlaylistParams{Title: title}, func(id protocol.PlaylistIDResult, err error) {
		if err != nil {
			if e.Toast != nil {
				e.Toast(err.Error(), env.ToastError)
			}
			return
		}
		if id == "" {
			return
		}
		for _, track := range items {
			act.AddToPlaylist(e, string(id), track)
		}
	})
}
