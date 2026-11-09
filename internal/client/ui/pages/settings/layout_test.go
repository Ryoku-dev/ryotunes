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
