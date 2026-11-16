package widgets

import (
	"image"
	"image/color"
	"math"
	"strconv"
	"strings"
	"time"

	"gioui.org/f32"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/anim"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/theme"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/model"
)

// TrackColumns selects the optional columns in a TrackTable.
type TrackColumns uint8

const (
	ColArt TrackColumns = 1 << iota
	ColArtist
	ColAlbum
	ColPlays
	ColAdded
)

// TrackTableOpts configures table columns and collection playback context.
type TrackTableOpts struct {
	Columns              TrackColumns
	Context              string
	SourceID, SourceName string
	Number               bool

	// SourceItems and SourceIndices preserve full-collection playback when rows are filtered.
	SourceItems   []model.Track
	SourceIndices []int
	Continuation  *string

	// DisplayItems lets the table omit columns that are empty in the visible rows.
	DisplayItems []model.Track
}

// TrackTable draws a retained table one virtualised row at a time, as the
// Hub's ruled list: a ruled head in small mono capitals, ordinals padded to
// the list's digits, the row wash (in 120 / out 240 OutCubic, a 2 dp lean,
// a press wash with the glyph at .86) and one selection wash that travels
// between rows over 240 OutCubic and lands, fading in as it widens from .6,
// where nothing was selected.
type TrackTable struct {
	rows             map[string]*trackRow
	selected         int
	selectionReady   bool
	selection        anim.Tween
	selectionFade    anim.Tween
	selectionWidth   anim.Tween
	selectionTarget  int
	selectionStarted bool
	cachedFirst      *model.Track
	cachedLength     int
	cachedMixed      bool
	PreviewRow       int
	PreviewRowState  kit.State
	PreviewSelection *int
}

type trackRow struct {
	click             kit.Click
	play              kit.Click
	like              kit.Click
	more              kit.IconPlaque
	fill              look.Fill
	wash              look.RowWash
	contextTag        int
	context           contextRelease
	menu              ItemMenu
	ordinal           string
	ordinalOf         [2]int
	titleChildren     [3]layout.FlexChild
	titleLineChildren [3]layout.FlexChild
	inlineChildren    [5]layout.FlexChild
	titleCopyChildren [2]layout.FlexChild
	rowChildren       [8]layout.FlexChild
}

// ordinalDigits is how many digits a list of n rows numbers with: two at
// least, as the Hub pads its ordinals ("01" .. "128").
func ordinalDigits(n int) int {
	digits := 2
	for limit := 100; n >= limit && digits < 18; limit *= 10 {
		digits++
	}
	return digits
}

// padOrdinal is n zero-padded to digits.
func padOrdinal(n, digits int) string {
	value := strconv.Itoa(n)
	if len(value) >= digits {
		return value
	}
	return strings.Repeat("0", digits-len(value)) + value
}

// ordinalText is the row's ordinal, built only when its place or the list's
// digits change.
func (r *trackRow) ordinalText(index, digits int) string {
	if r.ordinal == "" || r.ordinalOf != [2]int{index, digits} {
		r.ordinalOf = [2]int{index, digits}
		r.ordinal = padOrdinal(index+1, digits)
	}
	return r.ordinal
}

type trackSpec struct {
	width                                      int
	index, artist, album, plays, added, length int
	art, compact, showArtist, showAlbum        bool
	showPlays, showAdded, showLength           bool
}

func trackKey(track model.Track, index int) string {
	if track.VideoID != "" {
		return track.VideoID + "\x00" + textValue(track.SetVideoID) + "\x00" + strconv.Itoa(index)
	}
	return track.Title + "\x00" + track.Artists + "\x00" + strconv.Itoa(index)
}

func (t *TrackTable) row(track model.Track, index int) *trackRow {
	if t.rows == nil {
		t.rows = make(map[string]*trackRow)
	}
	key := trackKey(track, index)
	row := t.rows[key]
	if row == nil {
		row = new(trackRow)
		t.rows[key] = row
	}
	return row
}

func trackRateable(track model.Track) bool {
	provider := model.ProviderOf(track.VideoID)
	return provider != "" && provider != model.ProviderLocal && provider != model.ProviderRadio
}

func trackLiked(track model.Track) bool {
	return track.Rating != nil && *track.Rating == model.RatingLike
}

func trackTime(track model.Track) string {
	if track.Duration == nil || strings.TrimSpace(*track.Duration) == "" {
		return ""
	}
	return strings.TrimSpace(*track.Duration)
}

func trackPlayCount(track model.Track) string {
	if track.Plays != nil && strings.TrimSpace(*track.Plays) != "" {
		return strings.TrimSpace(*track.Plays)
	}
	if track.PlayCount != nil && strings.TrimSpace(*track.PlayCount) != "" {
		return strings.TrimSpace(*track.PlayCount)
	}
	return ""
}

func anyPlayCount(items []model.Track) bool {
	for _, track := range items {
		if trackPlayCount(track) != "" {
			return true
		}
	}
	return false
}

func sameArtist(items []model.Track) bool {
	if len(items) == 0 {
		return false
	}
	artist := strings.TrimSpace(items[0].Artists)
	if artist == "" {
		return false
	}
	for _, track := range items[1:] {
		if !strings.EqualFold(artist, strings.TrimSpace(track.Artists)) {
			return false
		}
	}
	return true
}

func anyTrackValue(items []model.Track, value func(model.Track) string) bool {
	for _, track := range items {
		if strings.TrimSpace(value(track)) != "" {
			return true
		}
	}
	return false
}

func mixedProviders(items []model.Track) bool {
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

func (t *TrackTable) listHasMixedProviders(items []model.Track) bool {
	if len(items) == 0 {
		return false
	}
	if t.cachedFirst != &items[0] || t.cachedLength != len(items) {
		t.cachedFirst = &items[0]
		t.cachedLength = len(items)
		t.cachedMixed = mixedProviders(items)
	}
	return t.cachedMixed
}

func tableSpec(gtx layout.Context, e *env.Env, opts TrackTableOpts) trackSpec {
	width := gtx.Constraints.Max.X
	contentWidth := gtx.Metric.PxToDp(width)
	spec := trackSpec{width: width, index: gtx.Dp(48)}
	spec.compact = contentWidth < 560
	spec.art = opts.Columns&ColArt != 0
	values := opts.DisplayItems
	hasValues := len(values) > 0
	spec.showArtist = opts.Columns&ColArtist != 0 && !spec.compact
	if hasValues && !anyTrackValue(values, func(track model.Track) string { return track.Artists }) {
		spec.showArtist = false
	}
	if opts.Context == "album" && sameArtist(opts.SourceItems) {
		spec.showArtist = false
	}
	spec.showAlbum = opts.Columns&ColAlbum != 0 && !spec.compact && contentWidth >= 900
	if hasValues && !anyTrackValue(values, func(track model.Track) string { return textValue(track.Album) }) {
		spec.showAlbum = false
	}
	spec.showPlays = opts.Columns&ColPlays != 0 && !spec.compact && contentWidth >= 720
	if hasValues && !anyPlayCount(values) {
		spec.showPlays = false
	}
	spec.showAdded = opts.Columns&ColAdded != 0 && !spec.compact
	if hasValues && !anyTrackValue(values, func(track model.Track) string { return textValue(track.AddedBy) }) {
		spec.showAdded = false
	}
	spec.showLength = !hasValues || anyTrackValue(values, trackTime)
	if spec.showAlbum {
		spec.album = int(float32(width) * .27)
	}
	if spec.showPlays {
		spec.plays = gtx.Dp(108)
	}
	if spec.showAdded {
		spec.added = gtx.Dp(132)
	}
	if spec.showLength {
		spec.length = gtx.Dp(72)
	}
	if spec.showArtist {
		spec.artist = int(float32(width) * .21)
		if opts.Context == "album" {
			fixed := spec.index + spec.album + spec.plays + spec.added + spec.length + gtx.Dp(40)
			spec.artist = int(float32(max(0, width-fixed)) * .335)
		}
	}
	return spec
}

// trackHeadTracking is the table head's tracking (the Hub's spec labels).
const trackHeadTracking = 1.2

// headerCell sets one column's name in the smallest mono, spaced capitals,
// fg3, inset 8 within width and aligned as its column's values.
func headerCell(gtx layout.Context, e *env.Env, x, width, height int, value string, align text.Alignment) {
	inset := gtx.Dp(8)
	room := max(0, width-2*inset)
	cell := gtx
	cell.Constraints = layout.Constraints{Max: image.Pt(room, height)}
	record := op.Record(gtx.Ops)
	dimensions := kit.LayoutSpacedCaps(cell, e.Th.Theme, e.Th.T.MonoSm, e.Th.R.Fg3, trackHeadTracking, value)
	call := record.Stop()
	left := x + inset
	switch align {
	case text.Middle:
		left += (room - dimensions.Size.X) / 2
	case text.End:
		left += room - dimensions.Size.X
	}
	offset := op.Offset(image.Pt(left, (height-dimensions.Size.Y)/2)).Push(gtx.Ops)
	call.Add(gtx.Ops)
	offset.Pop()
}

// Header draws the table's ruled head: 32 dp of column names in small mono
// capitals over a vein hairline.
func (t *TrackTable) Header(gtx layout.Context, e *env.Env, opts TrackTableOpts) layout.Dimensions {
	spec := tableSpec(gtx, e, opts)
	height := gtx.Dp(32)
	size := gtx.Constraints.Constrain(image.Pt(spec.width, height))
	fixed := spec.index + spec.artist + spec.album + spec.plays + spec.added + spec.length + gtx.Dp(40)
	headerCell(gtx, e, 0, spec.index, height, "#", text.Middle)
	x := spec.index
	title := max(0, size.X-fixed)
	headerCell(gtx, e, x, title, height, "Title", text.Start)
	x += title
	columns := [...]struct {
		show  bool
		width int
		name  string
		align text.Alignment
	}{
		{spec.showArtist, spec.artist, "Artist", text.Start},
		{spec.showAlbum, spec.album, "Album", text.Start},
		{spec.showPlays, spec.plays, "Plays", text.End},
		{spec.showAdded, spec.added, "Added", text.Start},
		{spec.showLength, spec.length, "Length", text.End},
	}
	for _, column := range columns {
		if !column.show {
			continue
		}
		headerCell(gtx, e, x, column.width, height, column.name, column.align)
		x += column.width
	}
	hair := max(1, gtx.Dp(1))
	paint.FillShape(gtx.Ops, e.Th.R.Vein, clip.Rect(image.Rect(0, max(0, size.Y-hair), size.X, size.Y)).Op())
	return layout.Dimensions{Size: size}
}

func iconWidget(icon *widget.Icon, ink paletteColor, size unit.Dp) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		pixels := gtx.Dp(size)
		gtx.Constraints = layout.Exact(image.Pt(pixels, pixels))
		if icon == nil {
			return layout.Dimensions{Size: image.Pt(pixels, pixels)}
		}
		return icon.Layout(gtx, ink)
	}
}

type paletteColor = color.NRGBA

func rowPlaying(e *env.Env, track model.Track) bool {
	if e.Store == nil {
		return false
	}
	now := e.Store.Snapshot().Playback.Now
	return now != nil && now.VideoID != "" && now.VideoID == track.VideoID
}

func (t *TrackTable) play(e *env.Env, items []model.Track, index int, opts TrackTableOpts) {
	if index < 0 || index >= len(items) {
		return
	}
	source := items
	sourceIndex := index
	if len(opts.SourceItems) > 0 {
		source = opts.SourceItems
		if index < len(opts.SourceIndices) {
			sourceIndex = opts.SourceIndices[index]
		} else {
			sourceIndex = sourceTrackIndex(source, items[index])
		}
	}
	if sourceIndex < 0 || sourceIndex >= len(source) {
		return
	}
	act.PlayCollection(e, source, sourceIndex, opts.SourceID, opts.SourceName, false, opts.Continuation)
}

func sourceTrackIndex(source []model.Track, target model.Track) int {
	for index, track := range source {
		if track.VideoID != target.VideoID {
			continue
		}
		if track.SetVideoID == nil && target.SetVideoID == nil {
			return index
		}
		if track.SetVideoID != nil && target.SetVideoID != nil && *track.SetVideoID == *target.SetVideoID {
			return index
		}
	}
	return -1
}

func (t *TrackTable) processRow(gtx layout.Context, e *env.Env, row *trackRow, items []model.Track, index int, opts TrackTableOpts) {
	for {
		click, ok := row.click.Update(gtx)
		if !ok {
			break
		}
		t.selected = index
		t.selectionReady = true
		if click.NumClicks >= 2 {
			t.play(e, items, index, opts)
		}
	}
	if row.play.Clicked(gtx) {
		t.selected = index
		t.selectionReady = true
		t.play(e, items, index, opts)
	}
	track := items[index]
	if row.like.Clicked(gtx) && trackRateable(track) {
		ToggleLike(e, track, trackLiked(track))
	}
	if row.more.Clicked(gtx) {
		row.openMenu(e, track, opts.Context, index)
	}
	row.context.poll(gtx, &row.contextTag, func() {
		row.openMenu(e, track, opts.Context, index)
	})
	for {
		raw, ok := gtx.Event(
			key.Filter{Focus: &row.click.Clickable, Name: key.NameEnter},
			key.Filter{Focus: &row.click.Clickable, Name: key.NameReturn},
			key.Filter{Focus: &row.click.Clickable, Name: key.NameUpArrow},
			key.Filter{Focus: &row.click.Clickable, Name: key.NameDownArrow},
		)
		if !ok {
			break
		}
		press, ok := raw.(key.Event)
		if !ok || press.State != key.Press {
			continue
		}
		switch press.Name {
		case key.NameEnter, key.NameReturn:
			t.play(e, items, index, opts)
		case key.NameUpArrow, key.NameDownArrow:
			next := index - 1
			if press.Name == key.NameDownArrow {
				next = index + 1
			}
			next = min(max(0, next), len(items)-1)
			t.selected = next
			t.selectionReady = true
			nextRow := t.row(items[next], next)
			gtx.Execute(key.FocusCmd{Tag: &nextRow.click.Clickable})
			gtx.Execute(op.InvalidateCmd{})
		}
	}
}

func (r *trackRow) openMenu(e *env.Env, track model.Track, context string, index int) {
	target := env.MenuTarget{Track: &track, Context: context, QueueIndex: index}
	if e.Menu != nil {
		e.Menu(target)
		return
	}
	if e.Overlays != nil {
		r.menu.OpenAt(e, target, e.Overlays.Press())
	}
}

func drawIcon(gtx layout.Context, icon *widget.Icon, ink paletteColor, size unit.Dp) layout.Dimensions {
	pixels := gtx.Dp(size)
	gtx.Constraints = layout.Exact(image.Pt(pixels, pixels))
	if icon == nil {
		return layout.Dimensions{Size: image.Pt(pixels, pixels)}
	}
	return icon.Layout(gtx, ink)
}

// drawScaledIcon draws icon at size, scaled about its centre (a pressed
// row's glyph stands at .86).
func drawScaledIcon(gtx layout.Context, icon *widget.Icon, ink paletteColor, size unit.Dp, scale float32) layout.Dimensions {
	if scale >= 1 {
		return drawIcon(gtx, icon, ink, size)
	}
	pixels := gtx.Dp(size)
	centre := f32.Pt(float32(pixels)/2, float32(pixels)/2)
	stack := op.Affine(f32.Affine2D{}.Scale(centre, f32.Pt(scale, scale))).Push(gtx.Ops)
	dimensions := drawIcon(gtx, icon, ink, size)
	stack.Pop()
	return dimensions
}

// indexCell is the row's ordinal, padded to the list's digits in mono: fg3
// at rest, the accent (sel) on the selected row. The playing row shows the
// sounding glyph in the music's accent; a hot row offers play (pause while
// playing), its glyph pressing to .86.
func (t *TrackTable) indexCell(gtx layout.Context, e *env.Env, row *trackRow, index, digits int, selected, playing, hot bool) layout.Dimensions {
	gtx.Constraints = layout.Exact(image.Pt(gtx.Dp(48), gtx.Constraints.Max.Y))
	quiet, active := e.Th.R.Fg3, e.Th.Accent.Ink
	if selected {
		quiet = e.Th.R.Sel
	}
	scale := row.wash.GlyphScale()
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		if hot {
			icon, ink := look.Icon.Play, e.Th.R.Fg
			if playing {
				icon, ink = look.Icon.Pause, active
			}
			return row.play.Layout(gtx, func(gtx layout.Context) layout.Dimensions { return drawScaledIcon(gtx, icon, ink, 18, scale) })
		}
		if playing {
			return drawIcon(gtx, look.Icon.VolumeUp, active, 18)
		}
		return kit.Label{Style: e.Th.T.Mono, Color: quiet, MaxLines: 1, Align: text.Middle}.Layout(gtx, e.Th.Theme, row.ordinalText(index, digits))
	})
}

func (t *TrackTable) titleCell(gtx layout.Context, e *env.Env, row *trackRow, track model.Track, selected, playing, hot, mixed, compact, art bool) layout.Dimensions {
	ink := e.Th.R.Fg
	quiet := e.Th.R.Fg3
	if selected {
		quiet = e.Th.R.Fg2
	}
	if playing {
		ink = e.Th.Accent.Ink
	}
	liked := trackLiked(track)
	likeAlpha := float32(0)
	if liked || hot {
		likeAlpha = 1
	}
	if row.like.Hovered() {
		likeAlpha = 1
	}
	children := row.titleChildren[:0]
	if art {
		artSize := min(gtx.Dp(40), max(1, gtx.Constraints.Max.Y-gtx.Dp(16)))
		children = append(children,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return look.Art(gtx, e.Th, e.Art, textValue(track.Thumbnail), image.Pt(artSize, artSize), look.Shape{Cut: 3})
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(13)}.Layout),
		)
	}
	children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
		title := row.titleLineChildren[:0]
		if mixed {
			provider := model.ProviderOf(track.VideoID)
			if provider != "" {
				providerInk := look.BrandInk(provider, e.Th.R)
				if selected {
					providerInk = quiet
				}
				title = append(title,
					layout.Rigid(look.ProviderGlyph(provider, 14, providerInk)),
					layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
				)
			}
		}
		showLike := trackRateable(track) && likeAlpha > 0
		extraWidth := 0
		if track.Explicit {
			extraWidth += gtx.Dp(8 + 14)
		}
		if showLike {
			if track.Explicit {
				extraWidth += gtx.Dp(4 + 16)
			} else {
				extraWidth += gtx.Dp(8 + 16)
			}
		}
		title = append(title, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			inline := row.inlineChildren[:0]
			inline = append(inline, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.X = 0
				gtx.Constraints.Max.X = max(0, gtx.Constraints.Max.X-extraWidth)
				return kit.Label{Style: e.Th.T.Row, Color: ink, MaxLines: 1}.Layout(gtx, e.Th.Theme, track.Title)
			}))
			if track.Explicit {
				inline = append(inline,
					layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions { return drawIcon(gtx, look.Icon.Explicit, quiet, 14) }),
				)
			}
			if showLike {
				gap := unit.Dp(8)
				if track.Explicit {
					gap = 4
				}
				inline = append(inline,
					layout.Rigid(layout.Spacer{Width: gap}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						opacity := paint.PushOpacity(gtx.Ops, likeAlpha)
						dims := row.like.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							likedInk := e.Th.Accent.Ink
							return row.fill.Layout(gtx, e.Th, liked,
								iconWidget(look.Icon.HeartOutline, quiet, 16),
								iconWidget(look.Icon.Heart, likedInk, 16),
							)
						})
						opacity.Pop()
						return dims
					}),
				)
			}
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, inline...)
		}))
		copy := row.titleCopyChildren[:1]
		copy[0] = layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, title...)
		})
		if compact && strings.TrimSpace(track.Artists) != "" {
			copy = append(copy, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return kit.Label{Style: e.Th.T.Caption, Color: quiet, MaxLines: 1}.Layout(gtx, e.Th.Theme, track.Artists)
			}))
		}
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.Start}.Layout(gtx, copy...)
	}))
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
	})
}

func fixedText(gtx layout.Context, e *env.Env, width int, style theme.TextStyle, ink paletteColor, value string, align text.Alignment) layout.Dimensions {
	gtx.Constraints = layout.Exact(image.Pt(max(0, width), gtx.Constraints.Max.Y))
	return layout.Inset{Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
			return kit.Label{Style: style, Color: ink, MaxLines: 1, Align: align}.Layout(gtx, e.Th.Theme, value)
		})
	})
}

func (t *TrackTable) rowContent(gtx layout.Context, e *env.Env, row *trackRow, items []model.Track, index, digits int, spec trackSpec, selected, playing, hot, mixed bool) layout.Dimensions {
	track := items[index]
	quiet := e.Th.R.Fg3
	if selected {
		quiet = e.Th.R.Fg2
	}
	children := row.rowChildren[:2]
	children[0] = layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return t.indexCell(gtx, e, row, index, digits, selected, playing, hot)
	})
	children[1] = layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
		return t.titleCell(gtx, e, row, track, selected, playing, hot, mixed, spec.compact, spec.art)
	})
	if spec.showArtist {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return fixedText(gtx, e, spec.artist, e.Th.T.Caption, quiet, track.Artists, text.Start)
		}))
	}
	if spec.showAlbum {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return fixedText(gtx, e, spec.album, e.Th.T.Caption, quiet, textValue(track.Album), text.Start)
		}))
	}
	if spec.showPlays {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return fixedText(gtx, e, spec.plays, e.Th.T.Mono, quiet, trackPlayCount(track), text.End)
		}))
	}
	if spec.showAdded {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return fixedText(gtx, e, spec.added, e.Th.T.Caption, quiet, textValue(track.AddedBy), text.Start)
		}))
	}
	if spec.showLength {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return fixedText(gtx, e, spec.length, e.Th.T.Mono, quiet, trackTime(track), text.End)
		}))
	}
	children = append(children,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints = layout.Exact(image.Pt(gtx.Dp(40), gtx.Constraints.Max.Y))
			if !hot {
				return layout.Dimensions{Size: gtx.Constraints.Min}
			}
			row.more.Small = true
			row.more.Kind = kit.PlaqueQuiet
			row.more.Icon = look.Icon.More
			row.more.Description = "More actions"
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return row.more.Layout(gtx, e.Th.Theme)
			})
		}),
	)
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
}

// Selection landing (TreeView.qml): a selection arriving where none was
// widens from .6 of the row to all of it over 320 ms while it fades in.
const (
	selectionLandFrom  = .6
	selectionLandWidth = 320 * time.Millisecond
)

// snapTween stands an OutCubic tween of duration d at v; its next Set moves.
func snapTween(gtx layout.Context, tween *anim.Tween, d time.Duration, v float32) {
	*tween = anim.Tween{Duration: d, Ease: anim.EaseOutCubic}
	tween.Set(gtx, v)
}

// stepSelection moves the selection wash toward the selected row and
// returns where it stands (in rows), its opacity and its width share. From
// row to row it travels over th.Motion.Move on OutCubic; arriving where
// nothing was selected (the table's first selection too) it lands: it
// stands at the row and fades in over Move while it widens from .6 over
// 320 ms. Calling it again within a frame changes nothing.
func (t *TrackTable) stepSelection(gtx layout.Context, th *look.Theme) (at, opacity, width float32) {
	target := t.selected
	move := th.Motion.Move
	if !t.selectionStarted || target != t.selectionTarget {
		switch {
		case target < 0:
			snapTween(gtx, &t.selectionFade, move, 0)
		case !t.selectionStarted || t.selectionTarget < 0:
			snapTween(gtx, &t.selection, move, float32(target))
			snapTween(gtx, &t.selectionFade, move, 0)
			snapTween(gtx, &t.selectionWidth, selectionLandWidth, selectionLandFrom)
			t.selectionFade.Set(gtx, 1)
			t.selectionWidth.Set(gtx, 1)
		default:
			t.selection.Set(gtx, float32(target))
		}
		t.selectionStarted, t.selectionTarget = true, target
	}
	if target < 0 {
		return 0, 0, 1
	}
	return t.selection.Value(gtx), t.selectionFade.Value(gtx), t.selectionWidth.Value(gtx)
}

// Row draws one row. Pages call it from look.Scroller so only visible rows are laid out.
func (t *TrackTable) Row(gtx layout.Context, e *env.Env, items []model.Track, index int, opts TrackTableOpts) layout.Dimensions {
	if index < 0 || index >= len(items) || e == nil || e.Th == nil {
		return layout.Dimensions{}
	}
	row := t.row(items[index], index)
	t.processRow(gtx, e, row, items, index, opts)
	if t.PreviewSelection != nil {
		t.selected = *t.PreviewSelection
		t.selectionReady = true
	}
	if !t.selectionReady {
		t.selected = -1
		t.selectionReady = true
	}
	height := gtx.Dp(e.Th.RowHeight())
	size := gtx.Constraints.Constrain(image.Pt(gtx.Constraints.Max.X, height))
	specGTX := gtx
	specGTX.Constraints.Max.X = size.X
	spec := tableSpec(specGTX, e, opts)
	selected := index == t.selected
	playing := rowPlaying(e, items[index])
	mixed := t.listHasMixedProviders(items)
	digits := ordinalDigits(len(items))
	selectionAt, selectionOpacity, selectionWidth := t.stepSelection(gtx, e.Th)
	preview := kit.State(0)
	if t.PreviewRow == index {
		preview = t.PreviewRowState
	}
	hot := row.click.Hovered() || gtx.Focused(&row.click.Clickable) || preview == kit.StateHover || preview == kit.StatePressed
	pressed := row.click.Pressed() || preview == kit.StatePressed
	row.wash.Update(gtx, e.Th, hot, pressed)
	cut := gtx.Dp(e.Th.Cut.Small)

	rowGTX := gtx
	rowGTX.Constraints = layout.Exact(size)
	dims := row.click.Layout(rowGTX, func(gtx layout.Context) layout.Dimensions {
		bounds := image.Rectangle{Max: size}
		clipStack := clip.Rect(bounds).Push(gtx.Ops)
		if selectionOpacity > 0 {
			// The one selection wash, in table space: each row paints its own
			// slice of it, so it travels across the rows as one.
			y := int(math.Round(float64((selectionAt - float32(index)) * float32(height))))
			w := int(float32(size.X)*selectionWidth + .5)
			x := (size.X - w) / 2
			kit.Fill(gtx, e.Th.Theme, image.Rect(x, y, x+w, y+height), cut, palette.WithAlpha(e.Th.R.Select(), selectionOpacity))
		}
		row.wash.Paint(gtx, e.Th, bounds, cut)
		lean := op.Offset(image.Pt(row.wash.Lean(gtx), 0)).Push(gtx.Ops)
		t.rowContent(gtx, e, row, items, index, digits, spec, selected, playing, hot, mixed)
		lean.Pop()
		clipStack.Pop()
		return layout.Dimensions{Size: size}
	})
	if gtx.Focused(&row.click.Clickable) {
		kit.FocusRing(gtx, e.Th.Theme, image.Rectangle{Max: dims.Size}, cut)
	}
	pass := pointer.PassOp{}.Push(gtx.Ops)
	area := clip.Rect(image.Rectangle{Max: dims.Size}).Push(gtx.Ops)
	event.Op(gtx.Ops, &row.contextTag)
	area.Pop()
	pass.Pop()
	return dims
}
