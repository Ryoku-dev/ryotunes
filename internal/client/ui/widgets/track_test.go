package widgets

import (
	"testing"

	"github.com/ryoku-dev/ryotunes/internal/model"
)

func TestSourceTrackIndexDisambiguatesPlaylistEntries(t *testing.T) {
	t.Parallel()
	first := "set-first"
	second := "set-second"
	source := []model.Track{
		{VideoID: "same-video", SetVideoID: &first},
		{VideoID: "same-video", SetVideoID: &second},
		{VideoID: "standalone"},
	}
	cases := []struct {
		name   string
		target model.Track
		want   int
	}{
		{name: "second playlist occurrence", target: model.Track{VideoID: "same-video", SetVideoID: &second}, want: 1},
		{name: "standalone", target: model.Track{VideoID: "standalone"}, want: 2},
		{name: "missing", target: model.Track{VideoID: "absent"}, want: -1},
	}
	for _, test := range cases {
		if got := sourceTrackIndex(source, test.target); got != test.want {
			t.Errorf("%s: sourceTrackIndex = %d, want %d", test.name, got, test.want)
		}
	}
}
