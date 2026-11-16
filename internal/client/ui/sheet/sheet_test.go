package sheet

import (
	"image"
	"testing"
	"time"

	"gioui.org/io/input"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/anim"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
	libtheme "github.com/ryoku-dev/ryoku/docs/lithic/library/go/theme"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
)

type harness struct {
	ops    op.Ops
	router input.Router
	now    time.Time
	theme  *look.Theme
}

func newHarness() *harness {
	anim.SetReduce(false)
	return &harness{now: time.Date(2026, 11, 16, 12, 40, 7, 0, time.Local), theme: look.NewTheme(palette.Signature(true), libtheme.Comfortable)}
}

func (h *harness) frame(draw func(gtx layout.Context)) {
	h.ops.Reset()
	draw(layout.Context{
		Ops: &h.ops, Source: h.router.Source(), Now: h.now,
		Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(900, 200)),
	})
	h.router.Frame(&h.ops)
}

// idle asserts a frame at rest asks for no other frame.
func (h *harness) idle(t *testing.T, name string, draw func(gtx layout.Context)) {
	t.Helper()
	h.frame(draw)
	h.router.WakeupTime()
	h.now = h.now.Add(time.Millisecond)
	h.frame(draw)
	if wake, ok := h.router.WakeupTime(); ok {
		t.Fatalf("%s at rest asks for a frame at %v", name, wake)
	}
}

func TestNumberingFollowsTheDecisionsTable(t *testing.T) {
	want := []struct{ number, name, gloss string }{
		{"00", "Home", "表紙"}, {"01", "Library", "書庫"}, {"02", "Radio", "放送"}, {"03", "Downloads", "保存"},
		{"04", "Playlists", "曲集"}, {"05", "Settings", "設定"}, {"06", "Search", "検索"}, {"07", "Now playing", "再生"},
	}
	seen := map[string]bool{}
	for index, place := range Places {
		if seen[place.Number] {
			t.Fatalf("number %s is used twice", place.Number)
		}
		seen[place.Number] = true
		if index < len(want) && (place.Number != want[index].number || place.Name != want[index].name || place.Gloss != want[index].gloss) {
			t.Fatalf("place %d = %s %s %s, want %v", index, place.Number, place.Name, place.Gloss, want[index])
		}
		if got, wantPlace := place.PagePlace(), "P "+kit.Pad2(index+1)+"/"+kit.Pad2(len(Places)); got != wantPlace {
			t.Fatalf("%s page place = %q, want %q", place.ID, got, wantPlace)
		}
	}
	if SettingsNumber(4) != "05.4" || SettingsNumber(12) != "05.12" {
		t.Fatalf("settings numbers = %s %s", SettingsNumber(4), SettingsNumber(12))
	}
	if Library.GlossIf(false) != "" || Library.GlossIf(true) != "書庫" {
		t.Fatal("place gloss does not follow the gloss switch")
	}
	if SectionGloss("sound", true) != "音" || SectionGloss("sound", false) != "" || SectionGloss("playback", true) != "" {
		t.Fatal("section glosses: the Hub's words only, and only while glosses are on")
	}
	if JoinGloss("設定", "音") != "設定・音" || JoinGloss("書庫", "") != "書庫" || JoinGloss("", "曲") != "曲" {
		t.Fatal("JoinGloss")
	}
	for kind, wantPlace := range map[env.RouteKind]Place{
		env.RouteHome: Home, env.RouteAlbum: Library, env.RoutePlaylist: Playlists,
		env.RouteSearch: Search, env.RouteEqualizer: Equalizer, env.RouteSettings: Settings,
	} {
		if got, _ := ForRoute(kind); got != wantPlace {
			t.Fatalf("route %s belongs to %s, want %s", kind, got.ID, wantPlace.ID)
		}
	}
}

func TestCode39EncodesAsTheHubBarcode(t *testing.T) {
	// "*" is b W b w B w B w b: bars at 0 (1), 4 (1), 6 (3), 10 (3), 14 (1)
	bars, width := Code39("")
	start := [][2]int{{0, 1}, {4, 1}, {6, 3}, {10, 3}, {14, 1}}
	if len(bars) < 2*len(start) || width != 2*16-1 {
		t.Fatalf("empty code: %d bars, %d units, want start and stop in 31", len(bars), width)
	}
	for index, bar := range start {
		if bars[index] != bar {
			t.Fatalf("start bar %d = %v, want %v", index, bars[index], bar)
		}
	}
	if _, width := Code39("library"); width != 9*16-1 {
		t.Fatalf("LIBRARY spans %d units, want %d", width, 9*16-1)
	}
	if _, width := Code39("now_playing"); width != 12*16-1 {
		t.Fatalf("unsupported characters are not left out: %d units", width)
	}
	upper, _ := Code39("radio")
	lower, _ := Code39("RADIO")
	if len(upper) != len(lower) {
		t.Fatal("the code is not read in capitals")
	}
}

func TestColophonGatesTheMottoAndRestsStill(t *testing.T) {
	h := newHarness()
	var colophon Colophon
	draw := func(gtx layout.Context) { colophon.Layout(gtx, h.theme, Library.ID) }
	h.frame(draw)
	if colophon.units != 9*16-1 || !colophon.mottoShown {
		t.Fatalf("colophon units %d motto %v", colophon.units, colophon.mottoShown)
	}
	h.idle(t, "colophon", draw)
	if allocs := testing.AllocsPerRun(20, func() { h.frame(draw) }); allocs != 0 {
		t.Fatalf("a colophon at rest allocates %v times a frame", allocs)
	}
	h.theme.Desktop.Gloss = false
	h.frame(draw)
	if colophon.mottoShown {
		t.Fatal("the motto shows while glosses are off")
	}
}

func TestMastheadReadsThePageAndTicksOnlyWhenAmbient(t *testing.T) {
	h := newHarness()
	defer kit.SetFocused(false)
	var masthead Masthead
	page := Page{Place: Library, Part: "Songs", PartGloss: "曲", Provider: "YouTube Music"}
	draw := func(gtx layout.Context) { masthead.Layout(gtx, h.theme, page, .3) }
	kit.SetFocused(false)
	h.frame(draw)
	head := masthead.head
	if head.Number != "01" || head.Where != "Library  /  Songs" || head.Gloss != "書庫・曲" || !head.Clock {
		t.Fatalf("masthead = %q %q %q clock %v", head.Number, head.Where, head.Gloss, head.Clock)
	}
	if len(head.Facts) != 2 || head.Facts[0] != "P 02/10" || head.Facts[1] != "YOUTUBE MUSIC" {
		t.Fatalf("facts = %q", head.Facts)
	}
	h.now = h.now.Add(time.Second)
	h.idle(t, "unfocused masthead", draw)
	if allocs := testing.AllocsPerRun(20, func() { h.frame(draw) }); allocs != 0 {
		t.Fatalf("a masthead at rest allocates %v times a frame", allocs)
	}

	kit.SetFocused(true)
	h.frame(draw)
	if wake, ok := h.router.WakeupTime(); !ok || wake.Sub(h.now) > time.Second {
		t.Fatalf("a focused masthead's clock asks for %v (%v), want the next second", wake, ok)
	}
	h.router.WakeupTime()
	anim.SetReduce(true)
	defer anim.SetReduce(false)
	h.idle(t, "masthead under reduced motion", draw)

	h.theme.Desktop.Gloss = false
	h.frame(draw)
	if masthead.head.Gloss != "" {
		t.Fatalf("gloss %q shows while glosses are off", masthead.head.Gloss)
	}
}

func TestTategakiAndSpecStrip(t *testing.T) {
	h := newHarness()
	var tategaki Tategaki
	var column, strip, one layout.Dimensions
	draw := func(gtx layout.Context) {
		gtx.Constraints.Min = image.Point{}
		column = tategaki.Layout(gtx, h.theme, "書庫", "書庫")
		strip = SpecStrip(gtx, h.theme, []Spec{{Label: "Playlists", Value: "24"}, {Label: "Songs", Value: "1204", Lit: true}})
		one = SpecStrip(gtx, h.theme, []Spec{{Label: "Playlists", Value: "24"}})
	}
	h.frame(draw)
	if column.Size.X != 22 || column.Size.Y < 18+2*20 || tategaki.parent != nil {
		t.Fatalf("tategaki %v (category kept %v), want a 22 wide column of the rule and two kanji", column.Size, tategaki.parent)
	}
	if strip.Size.X <= one.Size.X+18 || strip.Size.Y == 0 {
		t.Fatalf("spec strip %v against one figure %v", strip.Size, one.Size)
	}
	h.idle(t, "tategaki and spec strip", draw)
	h.theme.Desktop.Gloss = false
	h.frame(draw)
	if column.Size != (image.Point{}) {
		t.Fatalf("tategaki shows %v while glosses are off", column.Size)
	}
}

func TestRevealPlaysOnceAndNeverOnRefresh(t *testing.T) {
	h := newHarness()
	var reveal Reveal
	var drawn [7]bool
	draw := func(gtx layout.Context) {
		for order := range drawn {
			reveal.Group(gtx, h.theme, order, func(gtx layout.Context) layout.Dimensions {
				drawn[order] = true
				return layout.Dimensions{Size: image.Pt(100, 20)}
			})
		}
	}
	h.frame(draw)
	if _, ok := h.router.WakeupTime(); !ok {
		t.Fatal("an opening page does not rise")
	}
	if !drawn[6] {
		t.Fatal("a group past the fifth was not drawn")
	}
	h.now = h.now.Add(h.theme.Motion.Navigate + 6*h.theme.Motion.RevealStagger)
	h.frame(draw)
	h.idle(t, "risen page", draw)
	for order := range anim.RevealGroups {
		if !reveal.groups[order].Done() {
			t.Fatalf("group %d still rising", order)
		}
	}
	// a refresh draws the groups again and moves nothing
	h.idle(t, "refreshed page", draw)
	reveal.Reset()
	h.frame(draw)
	if _, ok := h.router.WakeupTime(); !ok {
		t.Fatal("a reopened page does not rise")
	}
	anim.SetReduce(true)
	defer anim.SetReduce(false)
	reveal.Reset()
	h.idle(t, "page under reduced motion", draw)
}
