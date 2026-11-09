package settings

import (
	"strings"
	"testing"

	"github.com/ryoku-dev/ryotunes/internal/client/state"
)

func TestVisualizerStyleOptions(t *testing.T) {
	tests := []struct {
		name                              string
		spectrumInstalled, scopeInstalled bool
		wantLabels                        []string
		wantValues                        []state.VisualizerStyle
	}{
		{name: "both", spectrumInstalled: true, scopeInstalled: true, wantLabels: visualizerStyleLabels, wantValues: state.VisualizerStyles},
		{name: "spectrum only", spectrumInstalled: true, wantLabels: visualizerSpectrumLabels, wantValues: state.VisualizerStyles[:2]},
		{name: "scope only", scopeInstalled: true, wantLabels: visualizerScopeLabels, wantValues: state.VisualizerStyles[2:]},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			labels, values := visualizerStyleOptions(test.spectrumInstalled, test.scopeInstalled)
			if strings.Join(labels, "|") != strings.Join(test.wantLabels, "|") {
				t.Fatalf("labels = %v, want %v", labels, test.wantLabels)
			}
			if len(values) != len(test.wantValues) {
				t.Fatalf("values = %v, want %v", values, test.wantValues)
			}
			for index := range values {
				if values[index] != test.wantValues[index] {
					t.Fatalf("values = %v, want %v", values, test.wantValues)
				}
			}
		})
	}
}

func TestVisualizerPreferenceDetailDistinguishesMissingAndStopped(t *testing.T) {
	missing, _ := visualizerPreferenceDetail(state.VisualizerVectorscope, true, false, true, false)
	if !strings.Contains(missing, "Install pw-record") {
		t.Fatalf("missing detail = %q", missing)
	}
	stopped, _ := visualizerPreferenceDetail(state.VisualizerVectorscope, true, true, true, false)
	if strings.Contains(stopped, "Install") || !strings.Contains(stopped, "stopped") {
		t.Fatalf("stopped detail = %q", stopped)
	}
}

func TestCornerDpForCustom(t *testing.T) {
	if got := cornerDpForCustom(state.CornersChisel, 0, 23); got != 16 {
		t.Fatalf("chisel seed = %d, want 16", got)
	}
	if got := cornerDpForCustom(state.CornersDesktop, 0, 23); got != 23 {
		t.Fatalf("desktop seed = %d, want 23", got)
	}
	if got := cornerDpForCustom(state.CornersFine, 11, 23); got != 11 {
		t.Fatalf("stored custom value = %d, want 11", got)
	}
}
