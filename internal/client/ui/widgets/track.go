package widgets

import (
	"fmt"
	"image"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"

	"github.com/ryoku-dev/ryotunes/internal/client/lithic"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/model"
)

// TrackRowOpts controls the optional columns and state of a track row.
type TrackRowOpts struct {
	Index                                 int
	ShowArt, ShowAlbum, Playing, Selected bool
	ShowProvider, HoverActionActive       bool
	HoverAction                           layout.Widget
	Context                               string
}

// TrackRow is one interactive track tablet.
type TrackRow struct {
	row       lithic.Row
	like      lithic.IconButton
	play      lithic.IconButton
	menu      lithic.IconButton
	menuTag   uint8
	activated bool
	selected  bool
}

// Activated reports and clears a double-click or keyboard activation.
func (r *TrackRow) Activated() bool {
	activated := r.activated
	r.activated = false
	return activated
}

func (r *TrackRow) tookSelection() bool {
	selected := r.selected
	r.selected = false
	return selected
}

func (r *TrackRow) openMenu(e *env.Env, t model.Track, context string, index int) {
	if e.Menu != nil {
		track := t
		e.Menu(env.MenuTarget{Track: &track, Context: context, QueueIndex: index})
	}
}

func (r *TrackRow) pollSecondary(gtx layout.Context, e *env.Env, t model.Track, context string, index int) {
	for {
		raw, ok := gtx.Event(pointer.Filter{Target: &r.menuTag, Kinds: pointer.Press})
		if !ok {
			return
		}
		press, ok := raw.(pointer.Event)
		if ok && press.Buttons.Contain(pointer.ButtonSecondary) {
			r.openMenu(e, t, context, index)
		}
	}
}
func (r *TrackRow) pollActivationKey(gtx layout.Context) {
	for {
		raw, ok := gtx.Event(
			key.Filter{Focus: &r.row.Clickable, Name: key.NameReturn},
			key.Filter{Focus: &r.row.Clickable, Name: key.NameEnter},
		)
		if !ok {
			return
		}
		if press, ok := raw.(key.Event); ok && press.State == key.Press {
			r.activated = true
		}
	}
}

func trackTone(t model.Track, playing bool) lithic.Tone {
	if t.VideoID == "" {
		return lithic.Attention
	}
	if playing {
		return lithic.Alive
	}
	return lithic.Idle
}

func trackFacts(e *env.Env, t model.Track, showAlbum, selected bool) layout.Widget {
	facts := t.Artists
	if showAlbum && t.Album != nil && *t.Album != "" {
		if facts != "" {
			facts += " · "
		}
		facts += *t.Album
	}
	ink := e.Th.R.Fg3
	if selected {
		ink = e.Th.R.OnSel2
	}
	return func(gtx layout.Context) layout.Dimensions {
		children := []layout.FlexChild{
			layout.Flexed(1, lithic.Text(e.Th, e.Th.T.Caption, ink, facts)),
		}
		if t.Explicit {
			children = append(children,
				layout.Rigid(layout.Spacer{Width: e.Th.Sp.S2}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					size := gtx.Dp(16)
					gtx.Constraints = layout.Exact(image.Pt(size, size))
					return lithic.Icon("explicit").Layout(gtx, ink)
				}),
			)
		}
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
	}
}

func trackMark(e *env.Env, t model.Track, o TrackRowOpts) layout.Widget {
	if o.ShowArt && t.Thumbnail != nil && *t.Thumbnail != "" {
		return func(gtx layout.Context) layout.Dimensions {
			return Art(gtx, e, *t.Thumbnail, 40, 3)
		}
	}
	label := fmt.Sprintf("%d", o.Index+1)
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Center.Layout(gtx, lithic.Text(e.Th, e.Th.T.Mono, e.Th.R.Fg3, label))
	}
}

func ratingLiked(t model.Track) bool {
	return t.Rating != nil && *t.Rating == model.RatingLike
}

func trackDuration(t model.Track) string {
	if t.Duration == nil || *t.Duration == "" {
		return "0:00"
	}
	return *t.Duration
}

func trackRateable(t model.Track) bool {
	provider := model.ProviderOf(t.VideoID)
	return provider != "" && provider != model.ProviderLocal && provider != model.ProviderRadio
}

func (r *TrackRow) trailing(gtx layout.Context, e *env.Env, t model.Track, o TrackRowOpts, hot bool) layout.Dimensions {
	if trackRateable(t) && r.like.Clicked(gtx) {
		rating := model.RatingLike
		if ratingLiked(t) {
			rating = model.RatingIndifferent
		}
		act.Rate(e, t, rating)
	}
	if r.play.Clicked(gtx) {
		r.activated = true
	}
	if r.menu.Clicked(gtx) {
		r.openMenu(e, t, o.Context, o.Index)
	}
	likeIcon, likeDesc := "favorite_border", "Like"
	if ratingLiked(t) {
		likeIcon, likeDesc = "favorite", "Unlike"
	}
	r.like.Small = true
	r.play.Small = true
	r.menu.Small = true

	children := make([]layout.FlexChild, 0, 9)
	appendGap := func() {
		if len(children) > 0 {
			children = append(children, layout.Rigid(layout.Spacer{Width: e.Th.Sp.S2}.Layout))
		}
	}
	if o.ShowProvider && gtx.Constraints.Max.X >= gtx.Dp(420) {
		children = append(children, layout.Rigid(ProviderBadge(e, model.ProviderOf(t.VideoID))))
	}
	appendGap()
	children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = min(gtx.Constraints.Max.X, gtx.Dp(40))
		return layout.E.Layout(gtx, lithic.Text(e.Th, e.Th.T.Mono, e.Th.R.Fg3, trackDuration(t)))
	}))
	appendGap()
	children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		if (hot || o.HoverActionActive) && o.HoverAction != nil {
			return o.HoverAction(gtx)
		}
		if !trackRateable(t) {
			return layout.Spacer{Width: 32, Height: 32}.Layout(gtx)
		}
		kind := lithic.Quiet
		if ratingLiked(t) {
			kind = lithic.On
		}
		return r.like.Layout(gtx, e.Th, kind, lithic.Icon(likeIcon), likeDesc)
	}))
	appendGap()
	children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return r.menu.Layout(gtx, e.Th, lithic.Quiet, lithic.Icon("more_vert"), "More actions")
	}))
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
}

// Layout renders one track tablet and handles its direct actions.
func (r *TrackRow) Layout(gtx layout.Context, e *env.Env, t model.Track, o TrackRowOpts) layout.Dimensions {
	r.pollSecondary(gtx, e, t, o.Context, o.Index)
	r.pollActivationKey(gtx)
	if r.row.Clicked(gtx) {
		r.selected = true
	}
	if r.row.DoubleClicked(gtx) {
		r.activated = true
	}
	playing := o.Playing
	if playing && o.Context == "queue" && e.Store != nil {
		playing = !e.Store.Snapshot().Playback.Paused
	}

	rowMacro := op.Record(gtx.Ops)
	dims := r.row.Layout(gtx, e.Th, lithic.RowContent{
		Tone: trackTone(t, playing), Progress: -1,
	}, o.Selected, false)
	rowCall := rowMacro.Stop()
	rowCall.Add(gtx.Ops)

	hot := r.row.Hovered() || gtx.Focused(&r.row.Clickable)
	titleInk := e.Th.R.Fg
	if o.Selected {
		titleInk = e.Th.R.OnSel
	}
	contentGTX := gtx
	contentGTX.Constraints = layout.Exact(dims.Size)
	contentClip := clip.Rect(image.Rectangle{Max: dims.Size}).Push(gtx.Ops)
	layout.Inset{Left: 16, Right: 12}.Layout(contentGTX, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints = layout.Exact(image.Pt(gtx.Dp(32), gtx.Dp(32)))
				if hot {
					return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return r.play.Layout(gtx, e.Th, lithic.Quiet, lithic.Icon("play"), "Play")
					})
				}
				return layout.Center.Layout(gtx, trackMark(e, t, o))
			}),
			layout.Rigid(layout.Spacer{Width: e.Th.Sp.S3}.Layout),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical, Alignment: layout.Start}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return (lithic.Label{Style: e.Th.T.Row, Color: titleInk, MaxLines: 1}).Layout(gtx, e.Th, t.Title)
					}),
					layout.Rigid(trackFacts(e, t, o.ShowAlbum, o.Selected)),
				)
			}),
			layout.Rigid(layout.Spacer{Width: e.Th.Sp.S2}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return r.trailing(gtx, e, t, o, hot)
			}),
		)
	})
	contentClip.Pop()

	pass := pointer.PassOp{}.Push(gtx.Ops)
	area := clip.Rect(image.Rectangle{Max: dims.Size}).Push(gtx.Ops)
	event.Op(gtx.Ops, &r.menuTag)
	area.Pop()
	pass.Pop()
	return dims
}

// TrackListOpts configures a track list and its continuation callback.
type TrackListOpts struct {
	ShowArt, ShowAlbum bool
	Context            string
	Header, Footer     layout.Widget
	OnPlay             func(i int)
	OnEnd              func()
}

// TrackList is a keyboard-navigable scrolling list of retained track rows.
type TrackList struct {
	rows       lithic.RowList
	byID       map[string]*TrackRow
	selected   int
	ready      bool
	endSent    bool
	lastLength int
}

func (l *TrackList) rowFor(t model.Track) *TrackRow {
	if l.byID == nil {
		l.byID = make(map[string]*TrackRow)
	}
	key := t.VideoID
	if key == "" {
		key = t.Title + "\x00" + t.Artists
	}
	row := l.byID[key]
	if row == nil {
		row = new(TrackRow)
		l.byID[key] = row
	}
	return row
}

func (l *TrackList) pollKeys(gtx layout.Context, items []model.Track) {
	if len(items) == 0 {
		l.selected = -1
		return
	}
	if !l.ready || l.selected < 0 || l.selected >= len(items) {
		l.selected = 0
		l.ready = true
	}
	row := l.rowFor(items[l.selected])
	for {
		raw, ok := gtx.Event(
			key.Filter{Focus: &row.row.Clickable, Name: key.NameUpArrow},
			key.Filter{Focus: &row.row.Clickable, Name: key.NameDownArrow},
			key.Filter{Focus: &row.row.Clickable, Name: key.NameReturn},
			key.Filter{Focus: &row.row.Clickable, Name: key.NameEnter},
		)
		if !ok {
			break
		}
		press, ok := raw.(key.Event)
		if !ok || press.State != key.Press {
			continue
		}
		next := l.selected
		switch press.Name {
		case key.NameUpArrow:
			next = max(0, next-1)
		case key.NameDownArrow:
			next = min(len(items)-1, next+1)
		default:
			row.activated = true
		}
		if next != l.selected {
			position := l.rows.Position
			if next < position.First || next >= position.First+position.Count {
				l.rows.ScrollTo(next)
			}
			l.selected = next
			gtx.Execute(key.FocusCmd{Tag: &l.rowFor(items[next]).row.Clickable})
			gtx.Execute(op.InvalidateCmd{})
		}
	}
}

func (l *TrackList) nearEnd(n int) bool {
	position := l.rows.Position
	return n > 0 && position.First+position.Count >= n-6
}

// Layout renders track rows with selection, keyboard movement, and continuation paging.
func (l *TrackList) Layout(gtx layout.Context, e *env.Env, items []model.Track, o TrackListOpts) layout.Dimensions {
	l.pollKeys(gtx, items)
	if l.lastLength != len(items) {
		l.endSent = false
		l.lastLength = len(items)
	}
	playingID := ""
	if e.Store != nil {
		state := e.Store.Snapshot()
		if state.Playback.Now != nil && !state.Playback.Paused {
			playingID = state.Playback.Now.VideoID
		}
	}
	showProvider := trackListHasMixedProviders(items)
	list := func(gtx layout.Context) layout.Dimensions {
		return l.rows.Layout(gtx, e.Th, len(items), l.selected, func(gtx layout.Context, i int) layout.Dimensions {
			track := items[i]
			row := l.rowFor(track)
			playing := playingID != "" && playingID == track.VideoID
			dims := row.Layout(gtx, e, track, TrackRowOpts{
				Index: i, ShowArt: o.ShowArt, ShowAlbum: o.ShowAlbum,
				Playing: playing, Selected: i == l.selected, ShowProvider: showProvider, Context: o.Context,
			})
			if row.tookSelection() {
				l.selected = i
				gtx.Execute(key.FocusCmd{Tag: &row.row.Clickable})
			}
			if row.Activated() && o.OnPlay != nil {
				o.OnPlay(i)
			}
			return dims
		})
	}
	children := make([]layout.FlexChild, 0, 3)
	if o.Header != nil {
		children = append(children, layout.Rigid(o.Header))
	}
	children = append(children, layout.Flexed(1, list))
	if o.Footer != nil {
		children = append(children, layout.Rigid(o.Footer))
	}
	dims := layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	near := l.nearEnd(len(items))
	if near && !l.endSent && o.OnEnd != nil {
		l.endSent = true
		o.OnEnd()
	} else if !near {
		l.endSent = false
	}
	return dims
}
func trackListHasMixedProviders(items []model.Track) bool {
	var first model.Provider
	for _, track := range items {
		provider := model.ProviderOf(track.VideoID)
		if provider == "" {
			continue
		}
		if first == "" {
			first = provider
			continue
		}
		if provider != first {
			return true
		}
	}
	return false
}
