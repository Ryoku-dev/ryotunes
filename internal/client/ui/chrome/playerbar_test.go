package chrome

import (
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

func TestPlaybackDisplayDuration(t *testing.T) {
	short := "2:39"
	long := "1:02:03"
	invalid := "soon"
	tests := []struct {
		name     string
		reported float64
		value    *string
		want     float64
	}{
		{name: "reported wins", reported: 42, value: &short, want: 42},
		{name: "minutes and seconds", value: &short, want: 159},
		{name: "hours minutes seconds", value: &long, want: 3723},
		{name: "unknown", value: nil, want: 0},
		{name: "invalid", value: &invalid, want: 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := playbackDisplayDuration(test.reported, test.value); got != test.want {
				t.Fatalf("playbackDisplayDuration() = %v, want %v", got, test.want)
			}
		})
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
