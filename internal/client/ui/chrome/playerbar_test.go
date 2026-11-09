package chrome

import (
	"strconv"
	"testing"
	"time"
)

func TestFitPlayerBar(t *testing.T) {
	metrics := playerBarFitMetrics{
		Plaque: 32, SmallGap: 4, MajorGap: 13, CompactGap: 8,
		MaxScrubber: 120, MinScrubber: 72, TransportMin: 220,
	}
	tests := []struct {
		name             string
		side, centre     int
		wantSide         int
		wantCentre       int
		wantScrubber     int
		wantGap          int
		wantShowNow      bool
		wantTrailingSize int
	}{
		{name: "everything fits", side: 400, centre: 500, wantSide: 400, wantCentre: 500, wantScrubber: 120, wantGap: 13, wantShowNow: true, wantTrailingSize: 363},
		{name: "shrink scrubber first", side: 350, centre: 500, wantSide: 350, wantCentre: 500, wantScrubber: 72, wantGap: 13, wantShowNow: true, wantTrailingSize: 315},
		{name: "drop now playing second", side: 300, centre: 500, wantSide: 300, wantCentre: 500, wantScrubber: 72, wantGap: 13, wantShowNow: false, wantTrailingSize: 279},
		{name: "tighten major gaps third", side: 270, centre: 500, wantSide: 270, wantCentre: 500, wantScrubber: 72, wantGap: 8, wantShowNow: false, wantTrailingSize: 264},
		{name: "grow both side zones last", side: 250, centre: 260, wantSide: 264, wantCentre: 232, wantScrubber: 72, wantGap: 8, wantShowNow: false, wantTrailingSize: 264},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := fitPlayerBar(test.side, test.centre, metrics)
			if got.Side != test.wantSide || got.Centre != test.wantCentre || got.Scrubber != test.wantScrubber || got.Gap != test.wantGap || got.ShowNow != test.wantShowNow || got.TotalWidth != test.wantTrailingSize {
				t.Fatalf("fitPlayerBar(%d, %d) = %+v", test.side, test.centre, got)
			}
		})
	}
}

func TestFitPlayerBarAt110Percent(t *testing.T) {
	metrics := playerBarFitMetrics{
		Plaque: 35, SmallGap: 4, MajorGap: 14, CompactGap: 9,
		MaxScrubber: 132, MinScrubber: 79, TransportMin: 242,
	}
	got := fitPlayerBar(354, 526, metrics)
	if !got.ShowNow || got.Scrubber != 79 || got.TotalWidth > got.Side {
		t.Fatalf("110%% fit = %+v", got)
	}
}

func TestFitMiniScrubber(t *testing.T) {
	tests := []struct {
		name      string
		available int
		want      int
	}{
		{name: "preferred fits", available: 334, want: 100},
		{name: "use remaining width", available: 190, want: 68},
		{name: "fit available width", available: 170, want: 48},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := fitMiniScrubber(test.available, 32, 13, 100, 72, true, true); got != test.want {
				t.Fatalf("fitMiniScrubber(%d) = %d, want %d", test.available, got, test.want)
			}
		})
	}
}

func TestFitMiniStageAcrossZoom(t *testing.T) {
	tests := []struct {
		name             string
		remaining, width int
		minimum, maximum int
		wantSide         int
		wantVisible      bool
	}{
		{name: "100 percent", remaining: 185, width: 334, minimum: 96, maximum: 200, wantSide: 185, wantVisible: true},
		{name: "110 percent", remaining: 161, width: 331, minimum: 106, maximum: 220, wantSide: 161, wantVisible: true},
		{name: "125 percent", remaining: 126, width: 328, minimum: 120, maximum: 250, wantSide: 126, wantVisible: true},
		{name: "small window hides stage", remaining: 6, width: 248, minimum: 120, maximum: 250, wantSide: 0, wantVisible: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			side, visible := fitMiniStage(test.remaining, test.width, test.minimum, test.maximum)
			if side != test.wantSide || visible != test.wantVisible {
				t.Fatalf("fitMiniStage() = (%d, %t), want (%d, %t)", side, visible, test.wantSide, test.wantVisible)
			}
		})
	}
}

func TestMiniLayoutSelection(t *testing.T) {
	tests := []struct {
		name          string
		width, height int
		wantLandscape bool
	}{
		{name: "default landscape", width: 520, height: 176, wantLandscape: true},
		{name: "exact landscape boundary", width: 560, height: 400, wantLandscape: true},
		{name: "below landscape boundary", width: 559, height: 400, wantLandscape: false},
		{name: "short portrait falls back to landscape", width: 300, height: 230, wantLandscape: true},
		{name: "portrait", width: 360, height: 420, wantLandscape: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := miniUsesLandscape(test.width, test.height, 251); got != test.wantLandscape {
				t.Fatalf("miniUsesLandscape(%d, %d) = %t, want %t", test.width, test.height, got, test.wantLandscape)
			}
		})
	}
}

func TestMiniFitAcrossZoom(t *testing.T) {
	for _, zoom := range []int{80, 100, 110, 125, 140} {
		t.Run(strconv.Itoa(zoom)+" percent", func(t *testing.T) {
			dp := func(value int) int { return (value*zoom + 50) / 100 }
			landscape := fitMini(
				dp(520), dp(176), dp(251), dp(13), dp(54),
				dp(120), dp(112), dp(32), dp(4),
				dp(360), dp(72),
			)
			if !landscape.Landscape || landscape.ArtSide != dp(176) || landscape.ColumnWidth != dp(344) {
				t.Fatalf("landscape fit at %d%% = %+v", zoom, landscape)
			}
			if !landscape.ShowLike || !landscape.ShowLyrics || !landscape.ShowVolume || landscape.VolumeScrubber != 0 {
				t.Fatalf("landscape controls at %d%% = %+v", zoom, landscape)
			}

			portrait := fitMini(
				dp(360), dp(420), dp(251), dp(13), dp(54),
				dp(120), dp(112), dp(32), dp(4),
				dp(360), dp(72),
			)
			if portrait.Landscape || portrait.ArtSide != 0 || portrait.ColumnWidth != 0 {
				t.Fatalf("portrait fit at %d%% = %+v", zoom, portrait)
			}
		})
	}
}

func TestMiniVolumeScrubberOnlyWhenItFits(t *testing.T) {
	without := fitMini(559, 200, 251, 13, 54, 120, 112, 32, 4, 360, 72)
	if without.VolumeScrubber != 0 {
		t.Fatalf("359 dp column showed a volume scrubber: %+v", without)
	}
	with := fitMini(560, 200, 251, 13, 54, 120, 112, 32, 4, 360, 72)
	if with.VolumeScrubber != 72 {
		t.Fatalf("360 dp column volume scrubber = %d, want 72: %+v", with.VolumeScrubber, with)
	}
	narrow := fitMini(300, 160, 251, 13, 54, 120, 112, 32, 4, 360, 72)
	if !narrow.CompactTransport || narrow.ShowLike || narrow.ShowLyrics || narrow.ShowVolume {
		t.Fatalf("minimum-width landscape did not shed controls cleanly: %+v", narrow)
	}
}

func TestMiniForcedLandscapeFit(t *testing.T) {
	fit := fitMini(300, 230, 251, 13, 54, 120, 112, 32, 4, 360, 72)
	if !fit.Landscape || fit.ColumnWidth != 70 || fit.ColumnInset != 8 || fit.InnerWidth != 54 {
		t.Fatalf("300x230 forced landscape geometry = %+v", fit)
	}
	if fit.ShowSeek || fit.ShowTransport || fit.ShowLike || fit.ShowLyrics || !fit.ShowVolume {
		t.Fatalf("300x230 forced landscape controls do not fit: %+v", fit)
	}
}

func TestMiniLandscapeHeadingKeyCachesSteadyState(t *testing.T) {
	var mini Mini
	if got := mini.landscapeHeadingKey("demo-track", false, 0); got != "track:demo-track" {
		t.Fatalf("track key = %q", got)
	}
	if allocations := testing.AllocsPerRun(100, func() {
		_ = mini.landscapeHeadingKey("demo-track", false, 0)
	}); allocations != 0 {
		t.Fatalf("steady track key allocated %.1f times", allocations)
	}
	if got := mini.landscapeHeadingKey("demo-track", true, 7); got != "lyrics:7:demo-track" {
		t.Fatalf("lyrics key = %q", got)
	}
	if allocations := testing.AllocsPerRun(100, func() {
		_ = mini.landscapeHeadingKey("demo-track", true, 7)
	}); allocations != 0 {
		t.Fatalf("steady lyrics key allocated %.1f times", allocations)
	}
}

func TestLyricsFollowPlanAndManualPause(t *testing.T) {
	plan := planLyricsFollow(9, 0, 122, false, false)
	if !plan.anchor || plan.anchorIndex != 10 || plan.anchorOffset != 122 || plan.springDelta != 0 {
		t.Fatalf("distant jump plan = %+v", plan)
	}
	plan = planLyricsFollow(10, 244, 122, true, false)
	if plan.anchor || plan.springDelta != 122 {
		t.Fatalf("visible line plan = %+v", plan)
	}

	now := time.Unix(100, 0)
	var deadline time.Time
	deadline = lyricManualDeadline(now, deadline, false)
	if !deadline.IsZero() {
		t.Fatalf("programmatic follow started manual pause at %v", deadline)
	}
	deadline = lyricManualDeadline(now, deadline, true)
	if want := now.Add(lyricsManualHold); !deadline.Equal(want) {
		t.Fatalf("real input deadline = %v, want %v", deadline, want)
	}
}
