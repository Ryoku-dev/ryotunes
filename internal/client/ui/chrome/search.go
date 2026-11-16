package chrome

import (
	"image"
	"image/color"
	"strings"
	"time"
	"unicode"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/anim"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

const topSearchDebounce = 200 * time.Millisecond

// topSearchLimit is how many suggestions or recent searches the overlay offers.
const (
	topSearchSuggestions = 6
	topSearchRecents     = 8
)

// topSearchHit is one line of the search overlay: what choosing it searches for, the
// group it stands under ("Recent searches", "Songs") and its crumb ("Song › Artist").
type topSearchHit struct {
	text, group, crumb string
}

// topSearchRow is one hit's row in the overlay.
type topSearchRow struct {
	click kit.Click
	wash  look.RowWash
}

type topSearch struct {
	field kit.Search
	tag   struct{}

	recent      []string
	recentHits  []topSearchHit
	suggestions []topSearchHit
	lastText    string
	routeQuery  string
	lastRoute   env.Route
	routeReady  bool
	provider    model.Provider
	due         time.Time
	request     uint64
	highlight   int
	focusNext   bool
	highlighted bool
	active      bool
	suppressed  bool
	wasFocused  bool

	// the overlay: one row per hit, the travelling wash on the highlighted one, and the
	// fade that swaps recent searches and suggestions over th.Motion.Quick
	rows      [max(topSearchSuggestions, topSearchRecents)]topSearchRow
	wash      look.SelectWash
	swap      anim.Tween
	shownKind int
	swapSet   bool
}

func (search *topSearch) focus(gtx layout.Context) {
	search.focusNext = true
	search.suppressed = false
	search.field.Focus(gtx)
}

func (search *topSearch) focused(gtx layout.Context) bool {
	return search.focusNext || gtx.Focused(&search.field.Editor)
}

func topSearchQuery(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func (search *topSearch) syncRoute(gtx layout.Context, route env.Route) bool {
	changed := search.routeReady && route != search.lastRoute
	search.lastRoute, search.routeReady = route, true
	if route.Kind == env.RouteSearch {
		if route.Query != search.routeQuery {
			search.routeQuery = route.Query
			search.field.SetText(route.Query)
			search.lastText = route.Query
		}
		return changed
	}
	if changed || search.routeQuery != "" {
		search.routeQuery = ""
		search.field.SetText("")
		search.lastText = ""
		search.suggestions = nil
		search.highlighted = false
		search.request++
		if search.focused(gtx) {
			gtx.Execute(key.FocusCmd{Tag: nil})
		}
	}
	return changed
}

func (search *topSearch) remember(query string) {
	query = topSearchQuery(query)
	if query == "" {
		return
	}
	recent := []string{query}
	for _, existing := range search.recent {
		if !strings.EqualFold(existing, query) {
			recent = append(recent, existing)
		}
		if len(recent) == topSearchRecents {
			break
		}
	}
	search.recent = recent
	search.recentHits = search.recentHits[:0]
	for _, text := range recent {
		search.recentHits = append(search.recentHits, topSearchHit{text: text, group: "Recent searches"})
	}
}

func (search *topSearch) submit(e *env.Env, query string) {
	query = topSearchQuery(query)
	if query == "" || e == nil || e.Nav == nil {
		return
	}
	search.remember(query)
	search.routeQuery = query
	search.field.SetText(query)
	search.lastText = query
	search.suggestions = nil
	search.highlighted = false
	search.suppressed = true
	search.request++
	if e.Overlays != nil {
		e.Overlays.Close(&search.tag)
	}
	e.Nav.Push(env.Route{Kind: env.RouteSearch, Query: query, Title: "Search"})
}

// topSearchKind names a result's kind for its group and its crumb.
func topSearchKind(item model.BrowseItem) (group, kind string) {
	switch strings.ToLower(strings.TrimSpace(item.Kind)) {
	case "album", "single", "ep":
		return "Albums", "Album"
	case "artist", "channel", "user":
		return "Artists", "Artist"
	case "playlist":
		return "Playlists", "Playlist"
	default:
		return "Songs", "Song"
	}
}

// topSearchCrumb is a result's crumb as the Hub writes one, "Song › Daft Punk": its kind,
// then who made it when the result says.
func topSearchCrumb(item model.BrowseItem, kind string) string {
	names := make([]string, 0, len(item.ArtistRuns))
	for _, artist := range item.ArtistRuns {
		if name := strings.TrimSpace(artist.Text); name != "" && !strings.EqualFold(name, strings.TrimSpace(item.Title)) {
			names = append(names, name)
		}
	}
	if len(names) == 0 && item.Subtitle != nil && kind != "Artist" {
		if name := strings.TrimSpace(*item.Subtitle); name != "" && !strings.EqualFold(name, item.Title) && !strings.EqualFold(name, kind) {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return kind
	}
	return kind + " › " + strings.Join(names, ", ")
}

// topSearchHits picks the first six distinct titles of a mixed search, grouped by kind in
// the order the groups first appear (the top results lead).
func topSearchHits(result model.SearchAll) []topSearchHit {
	sources := [][]model.BrowseItem{result.Top, result.Songs, result.Albums, result.Artists, result.Playlists}
	seen := make(map[string]struct{}, topSearchSuggestions)
	picked := make([]topSearchHit, 0, topSearchSuggestions)
	order := make([]string, 0, 4)
	for _, source := range sources {
		for _, item := range source {
			title := topSearchQuery(item.Title)
			key := strings.ToLower(title)
			if title == "" {
				continue
			}
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			group, kind := topSearchKind(item)
			picked = append(picked, topSearchHit{text: title, group: group, crumb: topSearchCrumb(item, kind)})
			known := false
			for _, name := range order {
				known = known || name == group
			}
			if !known {
				order = append(order, group)
			}
			if len(picked) == topSearchSuggestions {
				break
			}
		}
		if len(picked) == topSearchSuggestions {
			break
		}
	}
	out := make([]topSearchHit, 0, len(picked))
	for _, group := range order {
		for _, hit := range picked {
			if hit.group == group {
				out = append(out, hit)
			}
		}
	}
	return out
}

func (search *topSearch) beginSuggestions(e *env.Env, query string, provider model.Provider) {
	search.request++
	request := search.request
	act.Call[model.SearchAll](e, protocol.MethodSearchAll, protocol.QueryParams{Query: query}, func(result model.SearchAll, err error) {
		if request != search.request || err != nil || provider != search.provider || query != topSearchQuery(search.field.Text()) {
			return
		}
		search.suggestions = topSearchHits(result)
		search.highlighted = false
	})
}

func (search *topSearch) update(gtx layout.Context, e *env.Env) (escaped, blurred bool) {
	entries := search.entries()
	for {
		raw, ok := gtx.Event(
			key.Filter{Focus: &search.field.Editor, Name: key.NameEscape},
			key.Filter{Focus: &search.field.Editor, Name: key.NameUpArrow},
			key.Filter{Focus: &search.field.Editor, Name: key.NameDownArrow},
		)
		if !ok {
			break
		}
		pressed, ok := raw.(key.Event)
		if !ok || pressed.State != key.Press {
			continue
		}
		switch pressed.Name {
		case key.NameEscape:
			escaped = true
			search.suggestions = nil
			search.highlighted = false
			search.request++
			if e != nil && e.Overlays != nil {
				e.Overlays.Close(&search.tag)
			}
			if search.field.Text() != "" || !search.suppressed {
				search.field.SetText("")
				search.lastText = ""
				search.due = time.Time{}
				search.suppressed = true
			} else {
				blurred = true
				gtx.Execute(key.FocusCmd{Tag: nil})
			}
			gtx.Execute(op.InvalidateCmd{})
		case key.NameUpArrow, key.NameDownArrow:
			if len(entries) == 0 {
				continue
			}
			if !search.highlighted {
				search.highlight = 0
				if pressed.Name == key.NameUpArrow {
					search.highlight = len(entries) - 1
				}
			} else if pressed.Name == key.NameUpArrow {
				search.highlight = (search.highlight - 1 + len(entries)) % len(entries)
			} else {
				search.highlight = (search.highlight + 1) % len(entries)
			}
			search.highlighted = true
			gtx.Execute(op.InvalidateCmd{})
		}
	}
	for {
		raw, ok := search.field.Update(gtx)
		if !ok {
			break
		}
		if submitted, ok := raw.(widget.SubmitEvent); ok {
			entries = search.entries()
			if search.highlighted && search.highlight >= 0 && search.highlight < len(entries) {
				search.submit(e, entries[search.highlight].text)
			} else {
				search.submit(e, submitted.Text)
			}
		}
	}
	return escaped, blurred
}

func (search *topSearch) schedule(gtx layout.Context, e *env.Env, provider model.Provider) {
	text := topSearchQuery(search.field.Text())
	if provider != search.provider {
		search.provider = provider
		search.suggestions = nil
		search.highlighted = false
		search.request++
		if text != "" {
			search.due = gtx.Now.Add(topSearchDebounce)
		}
	}
	if text != search.lastText {
		search.lastText = text
		search.suggestions = nil
		search.highlighted = false
		search.suppressed = false
		search.request++
		if text == "" {
			search.due = time.Time{}
		} else {
			search.due = gtx.Now.Add(topSearchDebounce)
		}
	}
	if search.due.IsZero() || text == "" {
		return
	}
	if gtx.Now.Before(search.due) {
		gtx.Execute(op.InvalidateCmd{At: search.due})
		return
	}
	search.due = time.Time{}
	search.beginSuggestions(e, text, provider)
}

func (search *topSearch) entries() []topSearchHit {
	if topSearchQuery(search.field.Text()) == "" {
		return search.recentHits
	}
	return search.suggestions
}

func (search *topSearch) openSuggestions(gtx layout.Context, e *env.Env, anchor image.Rectangle) {
	if e == nil || e.Overlays == nil {
		return
	}
	entries := search.entries()
	if search.suppressed || !search.focused(gtx) || len(entries) == 0 {
		e.Overlays.Close(&search.tag)
		return
	}
	query := topSearchQuery(search.field.Text())
	content := func(gtx layout.Context) layout.Dimensions {
		return search.layoutOverlay(gtx, e, entries, query)
	}
	width := unit.Dp(float32(anchor.Dx()) / gtx.Metric.PxPerDp)
	e.Overlays.Open(&search.tag, anchor, look.Placement{Prefer: look.Below, Align: look.Start, Gap: 8, Width: width, Passive: true}, content)
}

// The overlay's measures, in dp (the Hub's SearchResults.qml): a group head in label fg2,
// 13 above every head but the first; a suggestion 48 tall with its crumb under its title,
// a recent search 36; rows 13 in.
const (
	searchPad       unit.Dp = 8
	searchHead      unit.Dp = 28
	searchHeadGap   unit.Dp = 13
	searchHitTall   unit.Dp = 48
	searchHitRecent unit.Dp = 36
	searchInset     unit.Dp = 13
)

// layoutOverlay draws the hits in the Hub's search language: grouped under quiet heads,
// each row with the row wash under the pointer, the highlighted one under the travelling
// selection wash, the query's words underlined in each title. Swapping recent searches
// for suggestions fades the new set in over th.Motion.Quick.
func (search *topSearch) layoutOverlay(gtx layout.Context, e *env.Env, entries []topSearchHit, query string) layout.Dimensions {
	th := e.Th
	width := gtx.Constraints.Max.X
	kind := 1
	if query == "" {
		kind = 0
	}
	switch {
	case !search.swapSet:
		// the first set stands; the overlay's own entrance carries it
		search.swapSet, search.shownKind = true, kind
		search.swap = anim.Tween{Duration: th.Motion.Quick, Ease: anim.EaseOutCubic}
		search.swap.Set(gtx, 1)
	case kind != search.shownKind:
		search.shownKind = kind
		search.swap = anim.Tween{Duration: th.Motion.Quick, Ease: anim.EaseOutCubic}
		search.swap.Set(gtx, 0)
		search.swap.Set(gtx, 1)
	}
	fade := search.swap.Value(gtx)

	hitHeight := gtx.Dp(searchHitTall)
	if kind == 0 {
		hitHeight = gtx.Dp(searchHitRecent)
	}
	inset := gtx.Dp(searchInset)
	cut := gtx.Dp(treeRowCut)
	// the rows' places, and the wash under the highlighted one
	y := gtx.Dp(searchPad)
	tops := [len(search.rows)]int{}
	heads := [len(search.rows)]int{}
	group := ""
	for index, hit := range entries {
		if index >= len(search.rows) {
			break
		}
		heads[index] = -1
		if hit.group != group {
			if index > 0 {
				y += gtx.Dp(searchHeadGap)
			}
			heads[index] = y
			y += gtx.Dp(searchHead)
			group = hit.group
		}
		tops[index] = y
		y += hitHeight
	}
	height := y + gtx.Dp(searchPad)
	target := image.Rectangle{}
	if search.highlighted && search.highlight >= 0 && search.highlight < min(len(entries), len(search.rows)) {
		top := tops[search.highlight]
		target = image.Rect(gtx.Dp(4), top, width-gtx.Dp(4), top+hitHeight)
	}

	opacity := paint.PushOpacity(gtx.Ops, fade)
	search.wash.Layout(gtx, th, target, kind, cut)
	for index, hit := range entries {
		if index >= len(search.rows) {
			break
		}
		row := &search.rows[index]
		if row.click.Clicked(gtx) {
			search.submit(e, hit.text)
			gtx.Execute(op.InvalidateCmd{})
		}
		if heads[index] >= 0 {
			head := line(gtx, th, th.T.Label, th.R.Fg2, hit.group, width-2*inset)
			head.at(gtx, inset, heads[index]+gtx.Dp(searchHead)/2)
		}
		at := op.Offset(image.Pt(0, tops[index])).Push(gtx.Ops)
		rowGtx := gtx
		rowGtx.Constraints = layout.Exact(image.Pt(width, hitHeight))
		row.click.Layout(rowGtx, func(gtx layout.Context) layout.Dimensions {
			enabled := gtx.Source.Enabled()
			hover, _ := row.wash.Update(gtx, th, enabled && row.click.Hovered(), enabled && row.click.Pressed())
			row.wash.Paint(gtx, th, image.Rect(gtx.Dp(4), 0, width-gtx.Dp(4), hitHeight), cut)
			x := inset + row.wash.Lean(gtx)
			textWidth := width - x - inset
			titleInk := palette.Mix(th.R.Fg2, th.R.Fg, max(hover, boolFloat(search.highlighted && search.highlight == index)))
			if kind == 0 {
				size := gtx.Dp(16)
				glyph(gtx, look.Icon.Search, th.R.Fg3, size, x+size/2, hitHeight/2, 0, row.wash.GlyphScale())
				words := line(gtx, th, th.T.Row, titleInk, hit.text, textWidth-size-gtx.Dp(10))
				words.at(gtx, x+size+gtx.Dp(10), hitHeight/2)
				return layout.Dimensions{Size: gtx.Constraints.Max}
			}
			title := line(gtx, th, th.T.Row, th.R.Fg, hit.text, textWidth)
			crumb := line(gtx, th, th.T.Detail, th.R.Fg2, hit.crumb, textWidth)
			stack := title.size.Y + gtx.Dp(2) + crumb.size.Y
			titleTop := (hitHeight - stack) / 2
			title.at(gtx, x, titleTop+title.size.Y/2)
			searchUnderline(gtx, th, hit.text, query, x, titleTop, title.size, th.R.Fg)
			crumb.at(gtx, x, titleTop+title.size.Y+gtx.Dp(2)+crumb.size.Y/2)
			return layout.Dimensions{Size: gtx.Constraints.Max}
		})
		at.Pop()
	}
	opacity.Pop()
	return layout.Dimensions{Size: image.Pt(width, height)}
}

// searchUnderline underlines, in a title drawn at (x, top) and size, each of the query's
// words where it first matches, accents folded on both sides as the Hub's search marks them
// ("unscharfe" marks "Unschärfe").
func searchUnderline(gtx layout.Context, th *look.Theme, title, query string, x, top int, size image.Point, ink color.NRGBA) {
	if query == "" || size.X <= 0 {
		return
	}
	runes := []rune(title)
	folded := make([]rune, len(runes))
	for index, r := range runes {
		folded[index] = foldRune(r)
	}
	baseline := top + size.Y - gtx.Dp(3)
	for _, word := range strings.FieldsFunc(query, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) }) {
		target := []rune(word)
		for index := range target {
			target[index] = foldRune(target[index])
		}
		at := runeIndex(folded, target)
		if at < 0 {
			continue
		}
		from := searchPrefixWidth(gtx, th, string(runes[:at]))
		to := searchPrefixWidth(gtx, th, string(runes[:at+len(target)]))
		from, to = min(from, size.X), min(to, size.X)
		if to > from {
			paint.FillShape(gtx.Ops, ink, clip.Rect(image.Rect(x+from, baseline, x+to, baseline+max(1, gtx.Dp(1)))).Op())
		}
	}
}

// searchPrefixWidth measures a title's prefix in the row style without drawing it.
func searchPrefixWidth(gtx layout.Context, th *look.Theme, prefix string) int {
	if prefix == "" {
		return 0
	}
	return line(gtx, th, th.T.Row, th.R.Fg, prefix, 1<<20).size.X
}

// runeIndex is the first index of target in runes, or -1.
func runeIndex(runes, target []rune) int {
	if len(target) == 0 {
		return -1
	}
	for start := 0; start+len(target) <= len(runes); start++ {
		match := true
		for offset, r := range target {
			if runes[start+offset] != r {
				match = false
				break
			}
		}
		if match {
			return start
		}
	}
	return -1
}

// accentFolds maps the accented Latin letters to their base letter, one rune for one.
var accentFolds = func() map[rune]rune {
	folds := map[rune]rune{}
	for base, accented := range map[rune]string{
		'a': "àáâãäåāăą", 'c': "çćĉċč", 'e': "èéêëēĕėęě", 'i': "ìíîïĩīĭįı", 'n': "ñńņňŉ",
		'o': "òóôõöøōŏő", 'u': "ùúûüũūŭůűų", 'y': "ýÿŷ", 's': "śŝşš", 'z': "źżž", 'g': "ĝğġģ",
		'l': "ĺļľŀł", 'r': "ŕŗř", 't': "ţťŧ", 'd': "ďđ",
	} {
		for _, r := range accented {
			folds[r] = base
		}
	}
	return folds
}()

// foldRune lowers r and takes its accent off.
func foldRune(r rune) rune {
	r = unicode.ToLower(r)
	if base, ok := accentFolds[r]; ok {
		return base
	}
	return r
}

func (search *topSearch) layout(gtx layout.Context, e *env.Env, anchor image.Rectangle) layout.Dimensions {
	if e == nil || e.Th == nil {
		return layout.Dimensions{}
	}
	routeChanged := search.syncRoute(gtx, e.Nav.Current())
	if routeChanged {
		search.suppressed = true
		search.highlighted = false
		e.Overlays.Close(&search.tag)
	}
	search.field.Submit = true
	escaped, blurred := search.update(gtx, e)
	snapshot := e.Store.Snapshot()
	search.schedule(gtx, e, snapshot.Provider)
	search.field.Placeholder = "Search tracks, albums, artists…"
	search.field.Hint = "/"
	requested := search.focusNext
	dimensions := search.field.Layout(gtx, e.Th.Theme)
	if requested {
		search.field.Focus(gtx)
		search.focusNext = false
	}
	focused := requested || gtx.Focused(&search.field.Editor)
	if blurred {
		focused = false
	}
	if focused && !search.wasFocused && !escaped && !routeChanged {
		search.suppressed = false
	}
	search.wasFocused = focused
	search.active = focused
	if routeChanged || escaped || !focused {
		e.Overlays.Close(&search.tag)
	} else {
		search.openSuggestions(gtx, e, anchor)
	}
	return dimensions
}
