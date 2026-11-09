package home

import (
	"testing"

	"github.com/ryoku-dev/ryotunes/internal/client/state"
)

func TestNextVisualizerStyleSkipsMissingFeeds(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name              string
		current           state.VisualizerStyle
		spectrumInstalled bool
		scopeInstalled    bool
		want              state.VisualizerStyle
	}{
		{name: "both feeds", current: state.VisualizerBars, spectrumInstalled: true, scopeInstalled: true, want: state.VisualizerCurve},
		{name: "spectrum only wraps", current: state.VisualizerCurve, spectrumInstalled: true, want: state.VisualizerBars},
		{name: "scope only advances", current: state.VisualizerOscilloscope, scopeInstalled: true, want: state.VisualizerVectorscope},
		{name: "scope only wraps", current: state.VisualizerVectorscope, scopeInstalled: true, want: state.VisualizerOscilloscope},
		{name: "missing current feed selects scope", current: state.VisualizerCurve, scopeInstalled: true, want: state.VisualizerOscilloscope},
		{name: "missing current feed selects spectrum", current: state.VisualizerOscilloscope, spectrumInstalled: true, want: state.VisualizerBars},
		{name: "no feeds keeps selection", current: state.VisualizerVectorscope, want: state.VisualizerVectorscope},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got := nextVisualizerStyle(test.current, test.spectrumInstalled, test.scopeInstalled)
			if got != test.want {
				t.Fatalf("nextVisualizerStyle(%q, %t, %t) = %q, want %q", test.current, test.spectrumInstalled, test.scopeInstalled, got, test.want)
			}
		})
	}
}
