package lastfm

import "testing"

func TestSignatureSortsAndConcatenatesParameters(t *testing.T) {
	params := map[string]string{
		"token":   "xyz",
		"api_key": "abc",
		"method":  "auth.getSession",
		"format":  "json",
	}
	if got, want := sign(params, "secret"), "f81f920d21fa903b7fa44ae2857c6bbd"; got != want {
		t.Fatalf("signature = %q, want %q", got, want)
	}
}

func TestScrobbleThreshold(t *testing.T) {
	tests := []struct {
		name     string
		position float64
		duration float64
		want     bool
	}{
		{name: "half short", position: 90, duration: 180, want: true},
		{name: "before half", position: 89, duration: 180, want: false},
		{name: "four minutes long", position: 240, duration: 1200, want: true},
		{name: "under thirty seconds", position: 29, duration: 20, want: false},
		{name: "unknown duration", position: 240, duration: 0, want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := crossesThreshold(test.position, test.duration); got != test.want {
				t.Fatalf("crossesThreshold(%v, %v) = %v", test.position, test.duration, got)
			}
		})
	}
}
