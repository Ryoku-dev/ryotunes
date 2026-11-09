package settings

import (
	"testing"

	"github.com/ryoku-dev/ryotunes/internal/model"
)

func TestEqualizerProof(t *testing.T) {
	tests := []struct {
		name      string
		equalizer model.Equalizer
		want      string
	}{
		{name: "bypassed", equalizer: model.Equalizer{Enabled: false, Preset: "Bass boost"}, want: "Bypassed"},
		{name: "preset", equalizer: model.Equalizer{Enabled: true, Preset: "Bass boost"}, want: "On · Bass boost"},
		{name: "custom empty", equalizer: model.Equalizer{Enabled: true}, want: "On · Custom"},
		{name: "custom whitespace", equalizer: model.Equalizer{Enabled: true, Preset: "  "}, want: "On · Custom"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := equalizerProof(test.equalizer); got != test.want {
				t.Fatalf("equalizerProof() = %q, want %q", got, test.want)
			}
		})
	}
}
