package look

import "testing"

func TestArtRequestSizeRoundsUpToStableLadder(t *testing.T) {
	cases := map[int]int{
		0: 32, 1: 32, 32: 32, 33: 48, 64: 64, 65: 96,
		191: 192, 193: 256, 512: 512, 513: 768, 2049: 2048,
	}
	for input, want := range cases {
		if got := artRequestSize(input); got != want {
			t.Fatalf("artRequestSize(%d) = %d, want %d", input, got, want)
		}
	}
}
