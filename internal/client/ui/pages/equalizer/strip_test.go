package equalizer

import (
	"testing"

	"gioui.org/unit"
)

func TestBandsSpreadToFillTheStripUntilTheyHaveToScroll(t *testing.T) {
	metric := unit.Metric{PxPerDp: 1.4}
	viewport := 1100
	preamp := metric.Dp(preampColumn) + metric.Dp(preampGap)
	natural := metric.Dp(bandPitch)

	pitch := stripPitch(metric, viewport, 5)
	if free := viewport - preamp - 5*pitch; pitch <= natural || free >= 5 {
		t.Fatalf("five bands in %d px spread %d apart and left %d px empty", viewport, pitch, free)
	}
	if pitch := stripPitch(metric, viewport, 20); pitch != natural {
		t.Fatalf("twenty bands spread %d apart, want their natural %d so the strip scrolls", pitch, natural)
	}
	if pitch := stripPitch(metric, viewport, 1); pitch > 3*natural {
		t.Fatalf("a lone band spread to %d, past three natural columns", pitch)
	}
	before, during, after := stripPitch(metric, viewport, 5), stripPitch(metric, viewport, 5.5), stripPitch(metric, viewport, 6)
	if before <= during || during <= after {
		t.Fatalf("columns while a sixth unfolds = %d, %d, %d, want them narrowing smoothly", before, during, after)
	}
}
