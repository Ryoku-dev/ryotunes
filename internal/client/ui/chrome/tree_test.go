package chrome

import (
	"image"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/anim"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
	libtheme "github.com/ryoku-dev/ryoku/docs/lithic/library/go/theme"
	"github.com/ryoku-dev/ryotunes/internal/client/state"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

// treeHarness lays a sidebar out frame by frame on a router, at 1 px a dp.
type treeHarness struct {
	t          *testing.T
	sidebar    Sidebar
	nav        *topBarTestNav
	e          *env.Env
	operations op.Ops
	router     input.Router
	now        time.Time
	size       image.Point
	rail       float32
	// scale is the pixels a dp; zero is 1
	scale float32
}

func newTreeHarness(t *testing.T) *treeHarness {
	t.Helper()
	anim.SetReduce(false)
	t.Cleanup(func() { anim.SetReduce(false) })
	store := state.New(protocol.Snapshot{Provider: model.ProviderYouTube})
	theme := look.NewTheme(palette.Signature(true), libtheme.Comfortable)
	nav := &topBarTestNav{current: env.Route{Kind: env.RouteHome}}
	return &treeHarness{
		t: t, nav: nav, now: time.Unix(1000, 0), size: image.Pt(272, 700),
		e: &env.Env{Th: theme, Overlays: new(look.Overlays), Store: store, Nav: nav},
	}
}

// frame lays the sidebar out once, d after the last frame, and returns whether the
// router was asked for another frame.
func (h *treeHarness) frame(d time.Duration) bool {
	h.now = h.now.Add(d)
	h.operations.Reset()
	gtx := layout.Context{
		Ops: &h.operations, Now: h.now, Source: h.router.Source(),
		Metric: h.metric(), Constraints: layout.Exact(h.size),
	}
	h.sidebar.Layout(gtx, h.e, look.Expanded, h.rail)
	h.router.Frame(&h.operations)
	_, wake := h.router.WakeupTime()
	return wake
}

func (h *treeHarness) metric() unit.Metric {
	scale := h.scale
	if scale <= 0 {
		scale = 1
	}
	return unit.Metric{PxPerDp: scale, PxPerSp: scale}
}

// settle runs frames until none is asked for, failing past limit.
func (h *treeHarness) settle(limit time.Duration) {
	h.t.Helper()
	for elapsed := time.Duration(0); elapsed <= limit; elapsed += 16 * time.Millisecond {
		if !h.frame(16 * time.Millisecond) {
			// one more frame at rest must ask for nothing either
			if h.frame(time.Millisecond) {
				h.t.Fatal("a settled tree asked for another frame")
			}
			return
		}
	}
	h.t.Fatalf("the tree still moved after %v", limit)
}

func (h *treeHarness) click(at f32.Point) {
	h.router.Queue(
		pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Buttons: pointer.ButtonPrimary, Position: at},
		pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: at},
	)
}

// lineY is the middle of the tree's nth line (0 Home .. 4 Playlists) at 1 px a dp.
func lineY(n int) float32 {
	return float32(treeBodyTop) + float32(n)*float32(treeLine) + float32(treeLine)/2
}

func TestTreeAtRestDrawsNoFrames(t *testing.T) {
	h := newTreeHarness(t)
	h.frame(0)
	h.settle(time.Second)
}

func TestTreeSelectionWashTravelsBetweenPlacesAndSettles(t *testing.T) {
	h := newTreeHarness(t)
	h.frame(0)
	h.settle(time.Second)
	home := h.sidebar.tree.bodyWash.Rect()
	if home.Empty() || home.Min.Y != 0 {
		t.Fatalf("Home's wash = %v, want the first line", home)
	}
	h.click(f32.Pt(100, lineY(2)))
	h.frame(time.Millisecond)
	if h.nav.current.Kind != env.RouteRadio {
		t.Fatalf("pressing Radio's line opened %q", h.nav.current.Kind)
	}
	h.frame(time.Millisecond)
	if !h.frame(80 * time.Millisecond) {
		t.Fatal("the wash did not ask for frames while travelling")
	}
	middle := h.sidebar.tree.bodyWash.Rect()
	radioTop := 2 * int(treeLine)
	if middle.Min.Y <= 0 || middle.Min.Y >= radioTop {
		t.Fatalf("mid-way the wash stands at %d, want between 0 and %d", middle.Min.Y, radioTop)
	}
	h.settle(time.Second)
	if got := h.sidebar.tree.bodyWash.Rect().Min.Y; got != radioTop {
		t.Fatalf("the wash landed at %d, want %d", got, radioTop)
	}
}

func TestPlaylistsFolderUnfoldsWithStaggeredLeaves(t *testing.T) {
	h := newTreeHarness(t)
	h.frame(0)
	h.settle(time.Second)
	if !h.sidebar.tree.open {
		t.Fatal("the Playlists folder should start open")
	}
	// fold it, then unfold it again
	h.click(f32.Pt(100, lineY(4)))
	h.frame(time.Millisecond)
	h.settle(2 * time.Second)
	if h.sidebar.tree.open || h.sidebar.tree.unfold.Value(layout.Context{Now: h.now}) != 0 {
		t.Fatal("pressing the folder did not fold it")
	}
	h.click(f32.Pt(100, lineY(4)))
	h.frame(time.Millisecond)
	if !h.sidebar.tree.open || h.sidebar.tree.entered.IsZero() {
		t.Fatal("pressing the folder again did not unfold it with its leaves entering")
	}
	opened := h.sidebar.tree.entered
	stagger := h.e.Th.Motion.Stagger
	gtx := layout.Context{Now: opened.Add(stagger / 2)}
	first, firstAlpha := h.sidebar.tree.leafEntry(gtx, h.e, 0)
	second, secondAlpha := h.sidebar.tree.leafEntry(gtx, h.e, 1)
	if firstAlpha <= 0 || secondAlpha != 0 || first <= -int(treeSlide) || second != -int(treeSlide) {
		t.Fatalf("half a stagger in: leaf 0 = (%d, %.2f), leaf 1 = (%d, %.2f); want leaf 0 under way and leaf 1 still waiting 8 to the left",
			first, firstAlpha, second, secondAlpha)
	}
	if !h.frame(16 * time.Millisecond) {
		t.Fatal("the unfolding folder did not ask for frames")
	}
	h.settle(2 * time.Second)
	if x, alpha := h.sidebar.tree.leafEntry(layout.Context{Now: h.now}, h.e, 1); x != 0 || alpha != 1 {
		t.Fatalf("after unfolding leaf 1 stands at (%d, %.2f), want (0, 1)", x, alpha)
	}
}

func TestReducedMotionFoldsAtOnce(t *testing.T) {
	h := newTreeHarness(t)
	h.frame(0)
	h.settle(time.Second)
	anim.SetReduce(true)
	h.click(f32.Pt(100, lineY(4)))
	h.frame(time.Millisecond)
	if h.frame(time.Millisecond) {
		t.Fatal("under reduced motion the folder still asked for frames")
	}
	if v := h.sidebar.tree.unfold.Value(layout.Context{Now: h.now}); v != 0 {
		t.Fatalf("under reduced motion the folder stands at %.2f, want 0", v)
	}
	h.click(f32.Pt(100, lineY(4)))
	h.frame(time.Millisecond)
	if x, alpha := h.sidebar.tree.leafEntry(layout.Context{Now: h.now}, h.e, 3); x != 0 || alpha != 1 {
		t.Fatalf("under reduced motion leaf 3 enters at (%d, %.2f), want (0, 1)", x, alpha)
	}
}

// fakeOutline is a Settings page's outline.
type fakeOutline struct {
	names   []string
	current int
	went    int
}

func (o *fakeOutline) Outline() (names []string, current int) {
	return o.names, o.current
}

func (o *fakeOutline) GoTo(index int) { o.went = index }

// twelveSections are the Settings page's sections, as many as it has.
var twelveSections = []string{"Account", "Playback", "Sound", "Lyrics", "Downloads", "Local music",
	"Playlists", "Appearance", "Integrations", "Data and storage", "Diagnostics", "About"}

func TestSettingsOutlineUnfoldsUnderSettingsAndGoesToASection(t *testing.T) {
	h := newTreeHarness(t)
	h.nav.current = env.Route{Kind: env.RouteSettings}
	outline := &fakeOutline{names: []string{"Account", "Playback", "Sound"}, current: 0, went: -1}
	h.sidebar.SetOutline(outline)
	h.frame(0)
	h.settle(2 * time.Second)
	footY := h.sidebar.tree.footY
	if want := h.size.Y - int(treeFootGap) - int(treeLine) - (3*int(treeOutlineRow) + 4); footY != want {
		t.Fatalf("the foot stands at %d, want %d: Settings with its three sections", footY, want)
	}
	soundY := float32(footY + int(treeLine) + 2*int(treeOutlineRow) + int(treeOutlineRow)/2)
	h.click(f32.Pt(120, soundY))
	h.frame(time.Millisecond)
	h.frame(time.Millisecond)
	if outline.went != 2 {
		t.Fatalf("pressing Sound went to section %d, want 2", outline.went)
	}
	// leaving Settings folds the outline away over the Hub's unfold
	h.nav.current = env.Route{Kind: env.RouteHome}
	h.sidebar.SetOutline(nil)
	if !h.frame(time.Millisecond) || !h.frame(40*time.Millisecond) {
		t.Fatal("the outline did not fold away with motion")
	}
	h.settle(2 * time.Second)
}

// settingsLineY is the middle of the 05 SETTINGS line as the last layout placed it, in px.
func (h *treeHarness) settingsLineY() float32 {
	return float32(h.sidebar.tree.footY) + float32(layout.Context{Metric: h.metric()}.Dp(treeLine))/2
}

func TestSettingsChevronFoldsAndUnfoldsTheOutline(t *testing.T) {
	h := newTreeHarness(t)
	h.nav.current = env.Route{Kind: env.RouteHome}
	h.frame(0)
	h.settle(time.Second)
	// opening Settings unfolds its outline, sections sliding in
	h.nav.current = env.Route{Kind: env.RouteSettings}
	outline := &fakeOutline{names: twelveSections, went: -1}
	h.sidebar.SetOutline(outline)
	h.frame(time.Millisecond)
	if !h.frame(16*time.Millisecond) || h.sidebar.tree.outlineEntered.IsZero() {
		t.Fatal("opening Settings did not unfold its outline with motion")
	}
	h.settle(2 * time.Second)
	folded := h.size.Y - int(treeFootGap) - int(treeLine)
	if h.sidebar.tree.footY >= folded || !h.sidebar.OutlineOpen() {
		t.Fatal("the outline did not stand open under Settings")
	}
	// the chevron folds it; the route stays
	chevronX := float32(treeInset + 8 + treeChevron/2)
	h.click(f32.Pt(chevronX, h.settingsLineY()))
	h.frame(time.Millisecond)
	if h.sidebar.OutlineOpen() {
		t.Fatal("pressing the chevron did not fold the outline")
	}
	if !h.frame(16 * time.Millisecond) {
		t.Fatal("the outline folded without motion")
	}
	h.settle(2 * time.Second)
	if h.sidebar.tree.footY != folded || h.nav.current.Kind != env.RouteSettings {
		t.Fatalf("folded, the foot stands at %d on %q; want %d on Settings", h.sidebar.tree.footY, h.nav.current.Kind, folded)
	}
	// and unfolds it again, the choice kept
	h.click(f32.Pt(chevronX, h.settingsLineY()))
	h.frame(time.Millisecond)
	if !h.sidebar.OutlineOpen() || !h.frame(16*time.Millisecond) {
		t.Fatal("pressing the chevron again did not unfold the outline with motion")
	}
	h.settle(2 * time.Second)
	if h.sidebar.tree.footY >= folded {
		t.Fatal("the outline did not unfold again")
	}
	// pressing the line itself opens Settings and never folds
	h.nav.current = env.Route{Kind: env.RouteHome}
	h.sidebar.SetOutline(nil)
	h.settle(2 * time.Second)
	h.click(f32.Pt(140, h.settingsLineY()))
	h.frame(time.Millisecond)
	if h.nav.current.Kind != env.RouteSettings || !h.sidebar.OutlineOpen() {
		t.Fatalf("pressing 05 SETTINGS opened %q with the outline open %v", h.nav.current.Kind, h.sidebar.OutlineOpen())
	}
}

// The owner's window, 1280x800 at zoom 110 with the player bar: Settings open with its
// twelve sections still leaves Liked songs and the first three playlists in view; the
// outline scrolls within the room left.
func TestSettingsOutlineLeavesThePlaylistsInView(t *testing.T) {
	h := newTreeHarness(t)
	h.scale = 1.1
	h.size = image.Pt(299, 685)
	snapshot := h.e.Store.Snapshot()
	sidebar := &h.sidebar
	sidebar.loaded, sidebar.loadedProvider = true, snapshot.Provider
	sidebar.loadedSignedIn = providerSignedIn(snapshot, snapshot.Provider)
	for n := 1; n <= 12; n++ {
		sidebar.playlists = append(sidebar.playlists, model.BrowseItem{Kind: "playlist", ID: "PL" + kitPad(n), Title: "Mix " + kitPad(n)})
	}
	h.nav.current = env.Route{Kind: env.RouteSettings}
	sidebar.SetOutline(&fakeOutline{names: twelveSections, went: -1})
	h.frame(0)
	h.settle(2 * time.Second)
	tree := &sidebar.tree
	if !tree.open {
		t.Fatal("the Playlists folder should stand open")
	}
	gtx := layout.Context{Metric: h.metric()}
	outline := h.size.Y - gtx.Dp(treeFootGap) - gtx.Dp(treeLine) - tree.footY
	if outline < 3*gtx.Dp(treeOutlineRow) {
		t.Fatalf("the outline stands %d px, want at least three sections", outline)
	}
	if full := len(twelveSections)*gtx.Dp(treeOutlineRow) + gtx.Dp(4); outline >= full {
		t.Fatalf("the outline stands its full %d px; it should scroll within the room left", full)
	}
	// leaf 0 is New playlist, 1 Liked songs, 2.. the playlists
	if len(tree.leafTops) < 5 {
		t.Fatalf("the folder holds %d leaves", len(tree.leafTops))
	}
	if bottom := tree.leafTops[4] + gtx.Dp(treeCoverLeaf); bottom > tree.bodyRect.Dy() {
		t.Fatalf("the third playlist ends at %d px, under the tree's %d", bottom, tree.bodyRect.Dy())
	}
}

func kitPad(n int) string { return string(rune('0'+n/10)) + string(rune('0'+n%10)) }

func TestSearchHitsGroupByKindWithCrumbs(t *testing.T) {
	subtitle := "Kite Assembly"
	hits := topSearchHits(model.SearchAll{
		Top:     []model.BrowseItem{{Kind: "album", Title: "Mineral lines", Subtitle: &subtitle}},
		Songs:   []model.BrowseItem{{Kind: "song", Title: "Stone in the water", ArtistRuns: []model.Artist{{Text: "Kite Assembly"}}}, {Kind: "song", Title: "mineral LINES"}},
		Artists: []model.BrowseItem{{Kind: "artist", Title: "Kite Assembly"}},
	})
	want := []topSearchHit{
		{text: "Mineral lines", group: "Albums", crumb: "Album › Kite Assembly"},
		{text: "Stone in the water", group: "Songs", crumb: "Song › Kite Assembly"},
		{text: "Kite Assembly", group: "Artists", crumb: "Artist"},
	}
	if len(hits) != len(want) {
		t.Fatalf("hits = %+v, want %+v", hits, want)
	}
	for index := range want {
		if hits[index] != want[index] {
			t.Fatalf("hit %d = %+v, want %+v", index, hits[index], want[index])
		}
	}
}

func TestSearchUnderlineFoldsAccents(t *testing.T) {
	title := []rune("Unschärfe Café")
	folded := make([]rune, len(title))
	for index, r := range title {
		folded[index] = foldRune(r)
	}
	for query, want := range map[string]int{"unscharfe": 0, "CAFE": 10, "fé": 7, "tea": -1} {
		target := []rune(query)
		for index := range target {
			target[index] = foldRune(target[index])
		}
		if got := runeIndex(folded, target); got != want {
			t.Fatalf("%q matches at %d, want %d", query, got, want)
		}
	}
}

func TestSettingsOutlineFallsBackToChipsWhenTheSidebarIsShort(t *testing.T) {
	gtx := layout.Context{Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}}
	var sidebar Sidebar
	sidebar.tree.initialize()
	// the owner's window (800 tall at 110 %) leaves the tree about 623 dp
	if !sidebar.OutlineFits(gtx, 623) {
		t.Fatal("the outline did not fit the owner's window")
	}
	// the 560 dp minimum window leaves the tree about 456 dp: the playlists keep it
	if sidebar.OutlineFits(gtx, 456) {
		t.Fatal("the outline starved the playlists in the smallest window")
	}
	// with the Playlists folder shut the same window has room for it
	sidebar.tree.open = false
	if !sidebar.OutlineFits(gtx, 456) {
		t.Fatal("the outline found no room beside a shut Playlists folder")
	}
}
