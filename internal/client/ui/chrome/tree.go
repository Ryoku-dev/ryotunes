package chrome

import (
	"image"
	"time"

	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/widget"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/anim"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
	"github.com/ryoku-dev/ryotunes/internal/client/state"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/sheet"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/widgets"
)

// Ryotunes' own name in the sidebar's head and its gloss, "music" [the Hub's head reads
// SETTINGS 設定].
const (
	treeAppName  = "Ryotunes"
	treeAppGloss = "音楽"
)

// The head's measures, in dp (HubSidebar.qml:26-67): 40 tall, 8 from the top, the mark 20
// wide 14 in, the name 10 after it.
const (
	treeHeadTop    = 8
	treeHeadHeight = 40
	treeHeadMark   = 20
	treeHeadMarkX  = 14
	treeBodyTop    = 56
	treeFootGap    = 8
)

// SettingsOutline is the open Settings page's sections as the tree reads them: their
// names in sentence case, the section being read, and a way to bring one to the top of
// the page.
type SettingsOutline interface {
	Outline() (names []string, current int)
	GoTo(index int)
}

// The tree's reserve under the Settings outline: the places and the Playlists folder,
// and, while the folder is open, its New playlist line and the first four collections
// (Liked songs and three playlists), so opening
// Settings never takes the playlists away.
const (
	treeReserveLines  = 5
	treeReserveCovers = 4
	// treeOutlineLeast is the fewest sections the outline shows; a sidebar too short for
	// them leaves the outline off and the Settings page keeps its own section chips
	treeOutlineLeast = 3
)

// outlineRoom is how tall the outline may stand in a sidebar height pixels tall: what the
// foot can take while the tree keeps its reserve.
func (t *tree) outlineRoom(gtx layout.Context, height int) int {
	reserve := treeReserveLines * gtx.Dp(treeLine)
	if t.open || !t.openSet {
		reserve += gtx.Dp(treeLeaf) + treeReserveCovers*gtx.Dp(treeCoverLeaf)
	}
	room := height - gtx.Dp(treeBodyTop) - gtx.Dp(4) - reserve - gtx.Dp(treeLine) - gtx.Dp(treeFootGap)
	return max(0, room)
}

// OutlineFits reports whether a sidebar height pixels tall holds the Settings outline
// (at least three sections, scrolling within the room for more) under Settings while the
// tree keeps its places and the first playlists. Below it the outline stays off and the
// Settings page keeps its own section chips.
func (sidebar *Sidebar) OutlineFits(gtx layout.Context, height int) bool {
	return sidebar.tree.outlineRoom(gtx, height) >= treeOutlineLeast*gtx.Dp(treeOutlineRow)+gtx.Dp(4)
}

// OutlineOpen reports whether the reader keeps the Settings outline unfolded under
// Settings (the chevron beside 05 SETTINGS): on until folded, then as left for the
// session. While it is folded the Settings page shows its own section chips.
func (sidebar *Sidebar) OutlineOpen() bool {
	t := &sidebar.tree
	return !t.outlineFoldSet || t.outlineOpen
}

// treePlace is one numbered place of the tree: Home, Library, Radio, Downloads and, at the
// foot, Settings.
type treePlace struct {
	place sheet.Place
	kind  env.RouteKind
	icon  *widget.Icon
	row   treeRow
}

// outlineLine is one section of the Settings outline: its row and its node's growth.
type outlineLine struct {
	row  treeRow
	node anim.Tween
	set  bool
}

// tree is the sidebar's navigation tree and its motion.
type tree struct {
	places   [4]treePlace
	settings treePlace
	folder   treeRow
	add      treeRow

	open, openSet bool
	unfold        anim.Tween
	unfoldSet     bool
	leafTops      []int
	entered       time.Time

	bodyWash, footWash look.SelectWash

	outline        SettingsOutline
	outlineNames   []string
	outlineLines   []outlineLine
	outlineUnfold  anim.Tween
	outlineSet     bool
	outlineScroll  look.Scroller
	outlineCurrent int
	outlineEntered time.Time
	// outlineOpen is the reader's fold choice, outlineFoldSet once they made one
	outlineOpen, outlineFoldSet bool
	chevron                     kit.Click
	// where the last layout put the body and the foot, in the sidebar's pixels
	bodyRect image.Rectangle
	footY    int
}

func (t *tree) initialize() {
	if t.places[0].icon != nil {
		return
	}
	t.places = [4]treePlace{
		{place: sheet.Home, kind: env.RouteHome, icon: look.Icon.Home},
		{place: sheet.Library, kind: env.RouteLibrary, icon: look.Icon.Library},
		{place: sheet.Radio, kind: env.RouteRadio, icon: look.Icon.Radio},
		{place: sheet.Downloads, kind: env.RouteDownloads, icon: look.Icon.Downloads},
	}
	t.settings = treePlace{place: sheet.Settings, kind: env.RouteSettings, icon: look.Icon.Settings}
	if !t.openSet {
		t.open, t.openSet = true, true
	}
}

// SetOutline hands the sidebar the open Settings page's sections while the tree shows
// them, nil otherwise. The shell calls it every frame before the sidebar's Layout.
func (sidebar *Sidebar) SetOutline(outline SettingsOutline) {
	sidebar.tree.outline = outline
}

// treeMetrics are the tree's line heights at a fold amount (0: the tree, 1: the rail).
type treeMetrics struct {
	width, line, leaf, cover, outlineRow int
	rail, labels, tiles                  float32
}

func treeMeasure(gtx layout.Context, width int, rail float32) treeMetrics {
	rail = min(max(rail, 0), 1)
	tile := gtx.Dp(treeRailTile)
	return treeMetrics{
		width:      width,
		line:       lerp(gtx.Dp(treeLine), tile, rail),
		leaf:       lerp(gtx.Dp(treeLeaf), tile, rail),
		cover:      lerp(gtx.Dp(treeCoverLeaf), tile, rail),
		outlineRow: gtx.Dp(treeOutlineRow),
		rail:       rail,
		labels:     max(0, 1-2*rail),
		tiles:      max(0, 2*rail-1),
	}
}

// washSpan is where a line's wash stands across a row of width w: from the line's step in
// plus 4 to the edge in the tree, inset 8 a side in the rail.
func (m treeMetrics) washSpan(gtx layout.Context, depth int) (int, int) {
	left := lerp(depth*gtx.Dp(treeIndent)+gtx.Dp(treeInset), gtx.Dp(8), m.rail)
	right := lerp(m.width, m.width-gtx.Dp(8), m.rail)
	return left, right
}

// treeSelection is which line is the open page's.
type treeSelection struct {
	place  int // 0..3 a place, -1 none
	leaf   int // a collection leaf, -1 none
	footer bool
}

func treeSelected(current env.Route, collections []sidebarCollection) treeSelection {
	selection := treeSelection{place: -1, leaf: -1}
	for index, item := range collections {
		if sidebarRouteMatches(current, item.route) {
			selection.leaf = index
			return selection
		}
	}
	switch current.Kind {
	case env.RouteHome:
		selection.place = 0
	case env.RouteLibrary:
		selection.place = 1
	case env.RouteRadio:
		selection.place = 2
	case env.RouteDownloads:
		selection.place = 3
	case env.RouteSettings:
		selection.footer = true
	}
	return selection
}

// layoutTree draws the head, the tree and the foot across gtx.Constraints.Max.
func (sidebar *Sidebar) layoutTree(gtx layout.Context, e *env.Env, rail float32, snapshot state.State) layout.Dimensions {
	t := &sidebar.tree
	t.initialize()
	size := gtx.Constraints.Max
	m := treeMeasure(gtx, size.X-2*gtx.Dp(treeInset), rail)
	collections := sidebar.collections(snapshot)
	selection := treeSelected(e.Nav.Current(), collections)

	sidebar.layoutHead(gtx, e, size.X, m)

	// the chevron beside 05 SETTINGS folds the outline; pressed away from Settings it
	// opens Settings with its outline unfolded
	if t.chevron.Clicked(gtx) {
		open := !selection.footer || !sidebar.OutlineOpen()
		t.outlineOpen, t.outlineFoldSet = open, true
		if !selection.footer {
			sidebar.navigated = true
			sidebarNavigate(e, env.Route{Kind: env.RouteSettings, Title: sheet.Settings.Name})
		}
		gtx.Execute(op.InvalidateCmd{})
	}

	// the foot: Settings and, while it is unfolded, the open page's outline
	current := -1
	if t.outline != nil {
		var names []string
		names, current = t.outline.Outline()
		if len(names) > 0 {
			// kept while the outline folds away after Settings closes
			t.outlineNames = names
		}
	}
	available := t.outline != nil && selection.footer && len(t.outlineNames) >= 2 && m.rail == 0
	shown := available && sidebar.OutlineOpen()
	room := t.outlineRoom(gtx, size.Y)
	outlineMax := min(len(t.outlineNames)*m.outlineRow+gtx.Dp(4), room)
	outlineHeight := t.outlineHeight(gtx, e, shown, outlineMax)
	footHeight := m.line + outlineHeight
	footY := max(gtx.Dp(treeBodyTop), size.Y-gtx.Dp(treeFootGap)-footHeight)
	inset := gtx.Dp(treeInset)

	body := image.Rect(inset, gtx.Dp(treeBodyTop), size.X-inset, footY-gtx.Dp(4))
	t.bodyRect, t.footY = body, footY
	overflowing := sidebar.layoutBody(gtx, e, body, m, collections, selection, snapshot)
	if overflowing {
		hline(gtx, gtx.Dp(12), size.X-gtx.Dp(12), footY-1, e.Th.R.Line())
	}

	foot := op.Offset(image.Pt(inset, footY)).Push(gtx.Ops)
	footGtx := gtx
	footGtx.Constraints = layout.Exact(image.Pt(m.width, footHeight))
	sidebar.layoutFoot(footGtx, e, m, selection, available, shown, outlineHeight, outlineMax, t.outlineNames, current)
	foot.Pop()
	return layout.Dimensions{Size: size}
}

// layoutHead draws the mark and Ryotunes' name, quiet, as the Hub's title line.
func (sidebar *Sidebar) layoutHead(gtx layout.Context, e *env.Env, width int, m treeMetrics) {
	mark := gtx.Dp(treeHeadMark)
	x := lerp(gtx.Dp(treeHeadMarkX), (width-mark)/2, m.rail)
	centre := gtx.Dp(treeHeadTop) + gtx.Dp(treeHeadHeight)/2
	offset := op.Offset(image.Pt(x, centre-mark/2)).Push(gtx.Ops)
	look.RyokuMark(treeHeadMark, e.Th.R.Fg)(gtx)
	offset.Pop()
	if m.labels <= 0 {
		return
	}
	name := treeAppName
	if e.Gloss() {
		name += "  " + treeAppGloss
	}
	words := spaced(gtx, e.Th, e.Th.T.Group, e.Th.R.Fg2, trackTreeHead, name)
	right := width - gtx.Dp(12)
	left := x + mark + gtx.Dp(10)
	if right <= left {
		return
	}
	area := clip.Rect(image.Rect(left, 0, right, gtx.Dp(treeBodyTop))).Push(gtx.Ops)
	opacity := paint.PushOpacity(gtx.Ops, m.labels)
	words.at(gtx, left, centre)
	opacity.Pop()
	area.Pop()
}

// layoutBody draws the scrolling tree: the places, the Playlists folder and its leaves. It
// reports whether the tree runs under the foot.
func (sidebar *Sidebar) layoutBody(gtx layout.Context, e *env.Env, body image.Rectangle, m treeMetrics, collections []sidebarCollection, selection treeSelection, snapshot state.State) bool {
	t := &sidebar.tree
	if body.Empty() {
		return false
	}
	// the folder unfolds over the Hub's 260 on OutCubic; its leaves slide in 18 apart
	if t.folder.click.Clicked(gtx) {
		t.open = !t.open
		if t.open && !anim.Reduced() {
			t.entered = gtx.Now
		}
		gtx.Execute(op.InvalidateCmd{})
	}
	t.unfold.Duration, t.unfold.Ease = e.Th.Motion.HubUnfold, anim.EaseOutCubic
	target := float32(0)
	if t.open {
		target = 1
	}
	if !t.unfoldSet {
		// the folder stands as it was left: it unfolds only when opened
		t.unfoldSet = true
		t.unfold.Set(gtx, target)
	}
	t.unfold.Set(gtx, target)
	fraction := t.unfold.Value(gtx)

	places := len(t.places)
	leafTops := t.leafTops[:0]
	defer func() { t.leafTops = leafTops }()
	boxTop := (places + 1) * m.line
	at := boxTop
	leafTops = append(leafTops, at)
	at += m.leaf
	for range collections {
		leafTops = append(leafTops, at)
		at += m.cover
	}
	box := int(float32(at-boxTop)*fraction + .5)
	visible := func(leaf int) int {
		top := leafTops[leaf] - boxTop
		height := m.cover
		if leaf == 0 {
			height = m.leaf
		}
		return min(max(box-top, 0), height)
	}

	// the selection: one wash travelling between the lines, landing in the folder
	target2 := image.Rectangle{}
	group := 0
	holds := selection.leaf >= 0 && fraction <= 0
	switch {
	case selection.place >= 0:
		left, right := m.washSpan(gtx, 0)
		target2 = image.Rect(left, selection.place*m.line, right, (selection.place+1)*m.line)
	case selection.leaf >= 0 && !holds:
		left, right := m.washSpan(gtx, 1)
		top := leafTops[selection.leaf+1]
		target2 = image.Rect(left, top, right, top+m.cover)
		group = 1
	case holds:
		left, right := m.washSpan(gtx, 0)
		target2 = image.Rect(left, places*m.line, right, (places+1)*m.line)
	}
	wash, washAlpha := t.bodyWash.Step(gtx, e.Th, target2, group)
	washInk := palette.WithAlpha(e.Th.R.Select(), washAlpha)
	cut := gtx.Dp(treeRowCut)

	count := places + 1
	if box > 0 {
		count += 1 + len(collections)
	}
	entering := false
	offset := op.Offset(body.Min).Push(gtx.Ops)
	listGtx := gtx
	listGtx.Constraints = layout.Exact(body.Size())
	sidebar.scroll.Layout(listGtx, e.Th, count, func(gtx layout.Context, index int) layout.Dimensions {
		var top, height int
		switch {
		case index <= places:
			top, height = index*m.line, m.line
		default:
			leaf := index - places - 1
			top, height = leafTops[leaf], visible(leaf)
		}
		size := image.Pt(m.width, height)
		gtx.Constraints = layout.Exact(size)
		if height <= 0 {
			return layout.Dimensions{Size: size}
		}
		area := clip.Rect(image.Rectangle{Max: size}).Push(gtx.Ops)
		if !wash.Empty() && washAlpha > 0 {
			kit.Fill(gtx, e.Th.Theme, wash.Sub(image.Pt(0, top)), cut, washInk)
		}
		switch {
		case index < places:
			sidebar.layoutPlace(gtx, e, &t.places[index], m, index == selection.place, snapshot)
		case index == places:
			// the chevron, name and glyph follow the choice at once; the leaves follow the fold
			sidebar.layoutFolder(gtx, e, m, len(collections), t.open, holds)
		default:
			leaf := index - places - 1
			slide, alpha := t.leafEntry(gtx, e, leaf)
			if alpha < 1 {
				entering = true
			}
			last := leaf == len(collections)
			lit := selection.leaf >= 0
			if leaf == 0 {
				sidebar.layoutAddLeaf(gtx, e, m, slide, alpha, last, lit)
			} else {
				sidebar.layoutCollectionLeaf(gtx, e, m, collections[leaf-1], leaf-1, leaf-1 == selection.leaf, slide, alpha, last, lit, snapshot)
			}
		}
		area.Pop()
		return layout.Dimensions{Size: size}
	})
	offset.Pop()
	if entering {
		gtx.Execute(op.InvalidateCmd{})
	}
	progress := sidebar.scroll.Progress()
	return progress >= 0 && progress < .999
}

// leafEntry is how far leaf (0 is the New playlist line) has slid in since its folder
// opened: its x offset and opacity.
func (t *tree) leafEntry(gtx layout.Context, e *env.Env, leaf int) (int, float32) {
	return treeEntry(gtx, e, t.entered, leaf)
}

// treeEntry is how far the line at index of a folder opened at entered has slid in: its
// x offset and opacity, each line 18 ms after the one above (th.Motion.Stagger) over
// th.Motion.Move on OutCubic. Lines past the 24th share its delay, so a long folder is
// never kept waiting.
func treeEntry(gtx layout.Context, e *env.Env, entered time.Time, index int) (int, float32) {
	if entered.IsZero() || anim.Reduced() {
		return 0, 1
	}
	delay := time.Duration(min(index, 24)) * e.Th.Motion.Stagger
	elapsed := gtx.Now.Sub(entered) - delay
	move := e.Th.Motion.Move
	if move <= 0 || elapsed >= move {
		return 0, 1
	}
	progress := float32(0)
	if elapsed > 0 {
		progress = anim.EaseOutCubic(float32(elapsed) / float32(move))
	}
	return -int(float32(gtx.Dp(treeSlide))*(1-progress) + .5), progress
}

// layoutPlace draws one of the four places: its numeral, name, leader and gloss in the
// tree; its glyph over its numeral in the rail.
func (sidebar *Sidebar) layoutPlace(gtx layout.Context, e *env.Env, place *treePlace, m treeMetrics, selected bool, snapshot state.State) {
	if place.row.click.Clicked(gtx) {
		sidebar.navigated = true
		sidebarNavigate(e, env.Route{Kind: place.kind, Title: place.place.Name})
	}
	trailing, flake := "", false
	if place.kind == env.RouteDownloads {
		if count := snapshot.Downloads.ActiveCount + snapshot.Downloads.QueuedCount; count > 0 {
			trailing, flake = treeCount(count), true
		}
	}
	sidebar.layoutNumbered(gtx, e, &place.row, m, treeNumbered{
		number: place.place.Number, name: place.place.Name, gloss: place.place.GlossIf(e.Gloss()),
		icon: place.icon, trailing: trailing, flake: flake, selected: selected,
	})
}

// treeNumbered is a numbered line of the tree.
type treeNumbered struct {
	number, name, gloss, trailing string
	icon                          *widget.Icon
	folder, open, holds, flake    bool
	selected                      bool
}

// layoutNumbered draws a numbered line into the row's clip, the wash already under it.
func (sidebar *Sidebar) layoutNumbered(gtx layout.Context, e *env.Env, row *treeRow, m treeMetrics, line treeNumbered) {
	th := e.Th
	size := gtx.Constraints.Max
	body := func(gtx layout.Context) layout.Dimensions {
		return row.click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			row.update(gtx, th, line.selected)
			left, right := m.washSpan(gtx, 0)
			row.wash.Paint(gtx, th, image.Rect(left, 0, right, size.Y), gtx.Dp(treeRowCut))
			accent := th.R.Sel
			lit := max(row.sel, boolFloat(line.holds || (line.folder && line.open)))
			centre := size.Y / 2
			if m.labels > 0 {
				opacity := paint.PushOpacity(gtx.Ops, m.labels)
				lean := row.lean(gtx)
				x := gtx.Dp(treeInset) + gtx.Dp(6)
				if line.folder {
					chevronX := gtx.Dp(treeInset) + gtx.Dp(8)
					angle := row.chevron.Angle(gtx, th, line.open)
					chevron := gtx.Dp(treeChevron)
					glyph(gtx, look.Icon.ChevronDown, palette.Mix(th.R.Fg3, th.R.Fg, row.hover), chevron, chevronX+chevron/2, centre, angle-90, 1)
					x = chevronX + chevron + gtx.Dp(4)
				}
				x += lean
				numeral := spaced(gtx, th, th.T.MonoSm, mixInk(th.R.Fg3, th.R.Fg, accent, row.hover, lit), 0, line.number)
				numeral.at(gtx, x, centre)
				x += numeral.size.X + gtx.Dp(10)
				style := th.T.Group
				if line.selected || line.folder {
					style = bold(style)
				}
				nameRest := th.R.Fg2
				if line.folder || line.holds {
					nameRest = th.R.Fg
				}
				name := spaced(gtx, th, style, mixInk(nameRest, th.R.Fg, th.R.Fg, row.hover, row.sel), trackTreeName, line.name)
				name.at(gtx, x, centre)
				nameEnd := x + name.size.X

				tailRight := size.X - gtx.Dp(treeTail)
				tailX := tailRight
				if line.trailing != "" {
					count := spaced(gtx, th, th.T.MonoSm, th.R.Fg3, 0, line.trailing)
					tailX = tailRight - count.size.X
					shown := float32(1)
					if line.folder && line.open {
						shown = .6 * row.hover
					}
					if shown > 0 {
						fade := paint.PushOpacity(gtx.Ops, shown)
						count.at(gtx, tailX, centre)
						fade.Pop()
					}
				}
				if line.flake {
					flakeX := tailX - gtx.Dp(14)
					treeFlake(gtx, e, flakeX, centre)
					tailX = flakeX
				}
				glossEnd := tailX
				if tailX < tailRight {
					glossEnd -= gtx.Dp(8)
				}
				leaderEnd := glossEnd
				if line.gloss != "" {
					gloss := spaced(gtx, th, sized(th.T.Caption, treeGlossSize), mixInk(th.R.Fg3, th.R.Fg2, accent, row.hover, lit), trackTreeGloss, line.gloss)
					glossX := glossEnd - gloss.size.X
					gloss.at(gtx, glossX, centre)
					leaderEnd = glossX
				}
				leaderStart, leaderStop := nameEnd+gtx.Dp(10), leaderEnd-gtx.Dp(8)
				if leaderStop-leaderStart > gtx.Dp(6) {
					leader := palette.Mix(th.R.Guide(), th.R.GuideLit(), max(row.hover, boolFloat(line.open)))
					hline(gtx, leaderStart, leaderStop, centre, leader)
				}
				opacity.Pop()
			}
			if m.tiles > 0 {
				opacity := paint.PushOpacity(gtx.Ops, m.tiles)
				icon := line.icon
				if line.selected || (line.folder && line.open) {
					icon = look.Filled(icon)
				}
				ink := mixInk(th.R.Fg2, th.R.Fg, accent, row.hover, max(row.sel, boolFloat(line.holds)))
				iconSize := gtx.Dp(20)
				glyph(gtx, icon, ink, iconSize, size.X/2, centre-gtx.Dp(7), 0, row.wash.GlyphScale())
				numeral := spaced(gtx, th, th.T.MonoSm, mixInk(th.R.Fg3, th.R.Fg2, accent, row.hover, lit), 0, line.number)
				numeral.at(gtx, (size.X-numeral.size.X)/2, centre+gtx.Dp(12))
				if line.flake {
					treeFlake(gtx, e, size.X-gtx.Dp(18), gtx.Dp(10))
				}
				opacity.Pop()
			}
			return layout.Dimensions{Size: size}
		})
	}
	if m.rail >= .999 {
		row.tip.Layout(gtx, th, e.Overlays, line.name, body)
		return
	}
	body(gtx)
}

// layoutFolder draws the Playlists folder line.
func (sidebar *Sidebar) layoutFolder(gtx layout.Context, e *env.Env, m treeMetrics, count int, open, holds bool) {
	t := &sidebar.tree
	sidebar.layoutNumbered(gtx, e, &t.folder, m, treeNumbered{
		number: sheet.Playlists.Number, name: sheet.Playlists.Name, gloss: sheet.Playlists.GlossIf(e.Gloss()),
		icon: look.Icon.Folder, trailing: treeCount(count), folder: true, open: open, holds: holds,
	})
}

// leafFrame draws what every leaf shares: its branch from the guide (and the guide itself,
// down to the last leaf's branch), its hover wash, and the slide it enters with. body draws
// the leaf's own marks at x, leaning with the pointer.
func (sidebar *Sidebar) leafFrame(gtx layout.Context, e *env.Env, row *treeRow, m treeMetrics, full int, selected bool, slide int, alpha float32, last, holdsPage bool, body func(gtx layout.Context, x int)) {
	th := e.Th
	size := gtx.Constraints.Max
	row.update(gtx, th, selected)
	left, right := m.washSpan(gtx, 1)
	row.wash.Paint(gtx, th, image.Rect(left, 0, right, size.Y), gtx.Dp(treeRowCut))
	if m.labels > 0 {
		guide := th.R.Guide()
		if holdsPage {
			guide = th.R.GuideLit()
		}
		fade := paint.PushOpacity(gtx.Ops, m.labels)
		guideX := gtx.Dp(treeGuide)
		bottom := size.Y
		if last {
			bottom = min(size.Y, full/2)
		}
		vline(gtx, guideX, 0, bottom, guide)
		fade.Pop()
	}
	opacity := paint.PushOpacity(gtx.Ops, alpha)
	shift := op.Offset(image.Pt(slide, 0)).Push(gtx.Ops)
	if m.labels > 0 {
		fade := paint.PushOpacity(gtx.Ops, m.labels)
		tick := th.R.Guide()
		if selected {
			tick = th.R.Sel
		}
		indent := gtx.Dp(treeIndent)
		hline(gtx, indent-gtx.Dp(6), indent-gtx.Dp(6)+gtx.Dp(treeBranch), full/2, tick)
		fade.Pop()
	}
	body(gtx, gtx.Dp(treeIndent)+gtx.Dp(6)+row.lean(gtx))
	shift.Pop()
	opacity.Pop()
}

// layoutAddLeaf draws the folder's first leaf, New playlist.
func (sidebar *Sidebar) layoutAddLeaf(gtx layout.Context, e *env.Env, m treeMetrics, slide int, alpha float32, last, holdsPage bool) {
	t := &sidebar.tree
	if t.add.click.Clicked(gtx) {
		sidebar.newPlaylistRequested = true
	}
	th := e.Th
	full := image.Pt(gtx.Constraints.Max.X, m.leaf)
	body := func(gtx layout.Context) layout.Dimensions {
		return t.add.click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			sidebar.leafFrame(gtx, e, &t.add, m, m.leaf, false, slide, alpha, last, holdsPage, func(gtx layout.Context, x int) {
				centre := m.leaf / 2
				ink := palette.Mix(th.R.Fg2, th.R.Fg, t.add.hover)
				if m.labels > 0 {
					fade := paint.PushOpacity(gtx.Ops, m.labels)
					size := gtx.Dp(17)
					glyph(gtx, look.Icon.Add, ink, size, x+size/2, centre, 0, t.add.wash.GlyphScale())
					words := line(gtx, th, th.T.Detail, ink, "New playlist", full.X-x-size-gtx.Dp(10)-gtx.Dp(treeTail))
					words.at(gtx, x+size+gtx.Dp(10), centre)
					fade.Pop()
				}
				if m.tiles > 0 {
					fade := paint.PushOpacity(gtx.Ops, m.tiles)
					glyph(gtx, look.Icon.Add, ink, gtx.Dp(20), full.X/2, centre, 0, t.add.wash.GlyphScale())
					fade.Pop()
				}
			})
			return layout.Dimensions{Size: gtx.Constraints.Max}
		})
	}
	if m.rail >= .999 {
		t.add.tip.Layout(gtx, th, e.Overlays, "New playlist", body)
		return
	}
	body(gtx)
}

// layoutCollectionLeaf draws a collection: its cover, title and caption, the playing one in
// the music's colour with its speaker.
func (sidebar *Sidebar) layoutCollectionLeaf(gtx layout.Context, e *env.Env, m treeMetrics, item sidebarCollection, index int, selected bool, slide int, alpha float32, last, holdsPage bool, snapshot state.State) {
	th := e.Th
	key := sidebarItemKey(item, index)
	row := sidebar.collectionRow(key)
	if row.row.click.Clicked(gtx) {
		sidebar.navigated = true
		sidebarNavigate(e, item.route)
	}
	for {
		raw, ok := gtx.Event(pointer.Filter{Target: &row.contextTag, Kinds: pointer.Press})
		if !ok {
			break
		}
		if press, ok := raw.(pointer.Event); ok && press.Buttons.Contain(pointer.ButtonSecondary) {
			sidebar.openCollectionMenu(e, row, item)
		}
	}
	playing := sidebarPlaying(snapshot, item)
	width := gtx.Constraints.Max.X
	body := func(gtx layout.Context) layout.Dimensions {
		dimensions := row.row.click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			sidebar.leafFrame(gtx, e, &row.row, m, m.cover, selected, slide, alpha, last, holdsPage, func(gtx layout.Context, x int) {
				centre := m.cover / 2
				cover := min(gtx.Dp(treeCover), m.cover-gtx.Dp(8))
				coverX := lerp(x, (width-cover)/2, m.rail)
				coverAt := op.Offset(image.Pt(coverX, centre-cover/2)).Push(gtx.Ops)
				sidebar.layoutCollectionArt(gtx, e, row, item, cover)
				coverAt.Pop()
				if m.labels <= 0 {
					return
				}
				fade := paint.PushOpacity(gtx.Ops, m.labels)
				textX := x + cover + gtx.Dp(10)
				textRight := width - gtx.Dp(treeTail)
				if playing {
					textRight -= gtx.Dp(22)
				}
				ink := mixInk(th.R.Fg2, th.R.Fg, th.R.Fg, row.row.hover, row.row.sel)
				if playing {
					ink = th.Accent.Ink
				}
				style := th.T.Detail
				if selected {
					style = bold(style)
				}
				title := line(gtx, th, style, ink, item.card.Title, textRight-textX)
				caption := record(gtx, func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Max.X = max(0, textRight-textX)
					return sidebarCaption(e, item, sidebar.likedCount, selected)(gtx)
				})
				stack := title.size.Y + caption.size.Y
				title.at(gtx, textX, centre-stack/2+title.size.Y/2)
				caption.at(gtx, textX, centre-stack/2+title.size.Y+caption.size.Y/2)
				if playing {
					size := gtx.Dp(16)
					glyph(gtx, look.Icon.VolumeUp, th.Accent.Ink, size, width-gtx.Dp(treeTail)-size/2, centre, 0, 1)
				}
				fade.Pop()
			})
			pass := pointer.PassOp{}.Push(gtx.Ops)
			contextArea := clip.Rect(image.Rectangle{Max: gtx.Constraints.Max}).Push(gtx.Ops)
			event.Op(gtx.Ops, &row.contextTag)
			contextArea.Pop()
			pass.Pop()
			return layout.Dimensions{Size: gtx.Constraints.Max}
		})
		return dimensions
	}
	if m.rail >= .999 {
		row.row.tip.Layout(gtx, th, e.Overlays, item.card.Title, body)
		return
	}
	body(gtx)
}

// outlineHeight moves the outline's unfold toward shown over the Hub's unfold (260
// OutCubic) and returns its height now, most pixels when unfolded.
func (t *tree) outlineHeight(gtx layout.Context, e *env.Env, shown bool, most int) int {
	t.outlineUnfold.Duration, t.outlineUnfold.Ease = e.Th.Motion.HubUnfold, anim.EaseOutCubic
	target := float32(0)
	if shown {
		target = 1
	}
	if !t.outlineSet {
		t.outlineSet = true
		t.outlineUnfold.Set(gtx, target)
	}
	if shown && t.outlineUnfold.Value(gtx) == 0 && !anim.Reduced() {
		// the sections slide in as the outline unfolds, 18 ms apart
		t.outlineEntered = gtx.Now
	}
	t.outlineUnfold.Set(gtx, target)
	return int(float32(most)*t.outlineUnfold.Value(gtx) + .5)
}

// layoutFoot draws Settings as a folder and, under it, the open page's outline: its
// chevron folds the outline, its line opens Settings. The outline stands at most
// outlineMax tall and scrolls within it, so the tree above keeps its playlists.
func (sidebar *Sidebar) layoutFoot(gtx layout.Context, e *env.Env, m treeMetrics, selection treeSelection, available, shown bool, outlineHeight, outlineMax int, names []string, current int) {
	t := &sidebar.tree
	th := e.Th
	if t.settings.row.click.Clicked(gtx) {
		sidebar.navigated = true
		sidebarNavigate(e, env.Route{Kind: env.RouteSettings, Title: sheet.Settings.Name})
	}
	target := image.Rectangle{}
	if selection.footer {
		left, right := m.washSpan(gtx, 0)
		target = image.Rect(left, 0, right, m.line)
	}
	t.footWash.Layout(gtx, th, target, 0, gtx.Dp(treeRowCut))
	lineGtx := gtx
	lineGtx.Constraints = layout.Exact(image.Pt(m.width, m.line))
	area := clip.Rect(image.Rectangle{Max: lineGtx.Constraints.Max}).Push(gtx.Ops)
	trailing := ""
	if available {
		trailing = treeCount(len(names))
	}
	sidebar.layoutNumbered(lineGtx, e, &t.settings.row, m, treeNumbered{
		number: sheet.Settings.Number, name: sheet.Settings.Name, gloss: sheet.Settings.GlossIf(e.Gloss()),
		icon: t.settings.icon, selected: selection.footer, folder: true, open: shown, trailing: trailing,
	})
	if m.rail == 0 {
		// the chevron's own target, over the line's: it folds instead of opening
		chevronGtx := lineGtx
		chevronGtx.Constraints = layout.Exact(image.Pt(gtx.Dp(treeInset)+gtx.Dp(8)+gtx.Dp(treeChevron)+gtx.Dp(4), m.line))
		t.chevron.Layout(chevronGtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Dimensions{Size: gtx.Constraints.Max}
		})
	}
	area.Pop()
	if outlineHeight <= 0 || len(names) < 2 {
		return
	}
	for len(t.outlineLines) < len(names) {
		t.outlineLines = append(t.outlineLines, outlineLine{})
	}
	row := m.outlineRow
	// the section being read stays in view as the page scrolls
	if current != t.outlineCurrent && current >= 0 {
		t.outlineCurrent = current
		first, _ := t.outlineScroll.Offset()
		visible := max(1, (outlineMax-gtx.Dp(4))/row)
		if current < first || current >= first+visible {
			t.outlineScroll.GlideTo(max(0, current-visible/2), th.Motion.Move)
		}
	}
	offset := op.Offset(image.Pt(0, m.line)).Push(gtx.Ops)
	box := clip.Rect(image.Rect(0, 0, m.width, outlineHeight)).Push(gtx.Ops)
	threadX := gtx.Dp(treeThread)
	entering := false
	listGtx := gtx
	listGtx.Constraints = layout.Exact(image.Pt(m.width, max(row, outlineMax-gtx.Dp(4))))
	t.outlineScroll.Layout(listGtx, th, len(names), func(gtx layout.Context, index int) layout.Dimensions {
		size := image.Pt(m.width, row)
		gtx.Constraints = layout.Exact(size)
		section := &t.outlineLines[index]
		if section.row.click.Clicked(gtx) && t.outline != nil {
			t.outline.GoTo(index)
			gtx.Execute(op.InvalidateCmd{})
		}
		// the thread the sections hang on, down to the last one's node
		bottom := row
		if index == len(names)-1 {
			bottom = row / 2
		}
		vline(gtx, threadX, 0, bottom, th.R.Guide())
		slide, alpha := treeEntry(gtx, e, t.outlineEntered, index)
		if alpha < 1 {
			entering = true
		}
		here := index == current
		name := names[index]
		section.row.click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			section.row.update(gtx, th, here)
			fade := paint.PushOpacity(gtx.Ops, alpha)
			shift := op.Offset(image.Pt(slide, 0)).Push(gtx.Ops)
			section.row.wash.Paint(gtx, th, image.Rect(gtx.Dp(46), 0, m.width-gtx.Dp(4), row), gtx.Dp(treeRowCut))
			// the node grows as its section is read, OutBack 2
			section.node.Duration, section.node.Ease = th.Motion.Move, look.EaseOutBackNode
			nodeSize := float32(gtx.Dp(treeNodeRest))
			if here {
				nodeSize = float32(gtx.Dp(treeNodeHere))
			}
			if !section.set {
				section.set = true
				section.node.Set(gtx, nodeSize)
			}
			section.node.Set(gtx, nodeSize)
			ring := palette.Mix(th.R.Guide(), th.R.Fg2, section.row.hover)
			node(gtx, section.node.Value(gtx), threadX-slide, row/2, palette.Mix(ring, th.R.Sel, section.row.sel), here, th.Planes().Side)
			style := th.T.Caption
			if here {
				style = bold(style)
			}
			ink := mixInk(th.R.Fg3, th.R.Fg2, th.R.Fg, section.row.hover, section.row.sel)
			x := gtx.Dp(treeOutlineText) + section.row.lean(gtx)
			words := line(gtx, th, style, ink, name, m.width-x-gtx.Dp(treeTail))
			words.at(gtx, x, row/2)
			shift.Pop()
			fade.Pop()
			return layout.Dimensions{Size: size}
		})
		return layout.Dimensions{Size: size}
	})
	if entering {
		gtx.Execute(op.InvalidateCmd{})
	}
	box.Pop()
	offset.Pop()
}

// treeFlake draws the downloads flake with its middle at (cx, cy).
func treeFlake(gtx layout.Context, e *env.Env, cx, cy int) {
	size := gtx.Dp(9)
	width, height := max(1, int(float32(size)*1.4+.5)), max(1, int(float32(size)*.7+.5))
	offset := op.Offset(image.Pt(cx-width/2, cy-height/2)).Push(gtx.Ops)
	flakeGtx := gtx
	flakeGtx.Constraints = layout.Exact(image.Pt(width, height))
	kit.Flake(flakeGtx, e.Th.Theme, palette.Becoming, 1, 9)
	offset.Pop()
}

// treeCount writes a count as the tree's mono figures: two digits below 100.
func treeCount(count int) string {
	if count < 100 {
		return kit.Pad2(count)
	}
	return widgets.Count(count)
}

func boolFloat(on bool) float32 {
	if on {
		return 1
	}
	return 0
}
