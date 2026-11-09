package chrome

import "testing"

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
		{name: "use remaining width", available: 190, want: 86},
		{name: "preserve minimum", available: 170, want: 72},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := fitMiniScrubber(test.available, 32, 4, 100, 72, true); got != test.want {
				t.Fatalf("fitMiniScrubber(%d) = %d, want %d", test.available, got, test.want)
			}
		})
	}
}
