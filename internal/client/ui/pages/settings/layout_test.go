package settings

import "testing"

func TestScrollspySectionCrossesOneThird(t *testing.T) {
	heights := []int{500, 400, 300}
	const viewport = 600

	for _, test := range []struct {
		name   string
		first  int
		offset int
		want   int
	}{
		{name: "next below threshold", first: 0, offset: 299, want: 0},
		{name: "next at threshold", first: 0, offset: 300, want: 0},
		{name: "next above threshold", first: 0, offset: 301, want: 1},
		{name: "third above threshold", first: 0, offset: 702, want: 2},
		{name: "clamps first", first: 99, offset: 0, want: 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := scrollspySection(test.first, test.offset, viewport, heights, false); got != test.want {
				t.Fatalf("scrollspySection() = %d, want %d", got, test.want)
			}
		})
	}
}

func TestScrollspySectionPinsLastAtEnd(t *testing.T) {
	if got := scrollspySection(1, 0, 600, []int{500, 400, 300}, true); got != 2 {
		t.Fatalf("scrollspySection() = %d, want 2", got)
	}
}

func TestIndexVisibleOffsetKeepsCurrentSectionInPane(t *testing.T) {
	const (
		count    = 12
		viewport = 492
		row      = 48
		gap      = 4
	)
	if got := indexVisibleOffset(0, count, viewport, 80, row, gap); got != 0 {
		t.Fatalf("first section offset = %d, want 0", got)
	}
	if got := indexVisibleOffset(11, count, viewport, 0, row, gap); got != 128 {
		t.Fatalf("last section offset = %d, want 128", got)
	}
	if got := indexVisibleOffset(9, count, viewport, 80, row, gap); got != 80 {
		t.Fatalf("visible section offset = %d, want 80", got)
	}
	if got := indexVisibleOffset(11, count, 300, 0, row, gap); got != 320 {
		t.Fatalf("last section in short pane offset = %d, want 320", got)
	}
	if got := indexVisibleOffset(0, count, 300, 320, row, gap); got != 0 {
		t.Fatalf("first section after index scroll offset = %d, want 0", got)
	}
}
