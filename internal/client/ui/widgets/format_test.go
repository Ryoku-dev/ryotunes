package widgets

import (
	"math"
	"testing"
)

func TestDuration(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		seconds float64
		want    string
	}{
		"zero":       {0, "0:00"},
		"floors":     {65.9, "1:05"},
		"hours":      {3661, "1:01:01"},
		"negative":   {-1, "0:00"},
		"not number": {math.NaN(), "0:00"},
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := Duration(test.seconds); got != test.want {
				t.Fatalf("Duration(%v) = %q, want %q", test.seconds, got, test.want)
			}
		})
	}
}

func TestCount(t *testing.T) {
	t.Parallel()
	cases := map[int]string{
		999:       "999",
		1_200:     "1.2K",
		3_400_000: "3.4M",
	}
	for value, want := range cases {
		if got := Count(value); got != want {
			t.Errorf("Count(%d) = %q, want %q", value, got, want)
		}
	}
}
