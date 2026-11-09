package widgets

import "testing"

func TestPlaybackDuration(t *testing.T) {
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
			if got := PlaybackDuration(test.reported, test.value); got != test.want {
				t.Fatalf("PlaybackDuration() = %v, want %v", got, test.want)
			}
		})
	}
}
