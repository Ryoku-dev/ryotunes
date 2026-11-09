package widgets

import "testing"

func TestGridRows(t *testing.T) {
	t.Parallel()
	var grid Grid
	cases := []struct {
		width, count, want int
	}{
		{160, 0, 0},
		{160, 3, 3},
		{341, 4, 2},
		{1_000, 11, 3},
	}
	for _, test := range cases {
		if got := grid.Rows(test.width, test.count); got != test.want {
			t.Errorf("Rows(%d, %d) = %d, want %d", test.width, test.count, got, test.want)
		}
	}
}
