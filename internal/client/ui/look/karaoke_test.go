package look

import (
	"image"
	"math"
	"testing"

	"gioui.org/text"
)

func TestKaraokeFlowWrapsAndCentresEachRow(t *testing.T) {
	flow := layoutKaraokeFlowInto(nil, nil, []int{40, 40, 40}, []int{0, 10, 10}, 100, 20, 2, 2, .8, text.Middle, false)
	if got := len(flow.rows); got != 2 {
		t.Fatalf("rows = %d, want 2", got)
	}
	wantX := []int{5, 55, 30}
	wantY := []int{0, 0, 20}
	for index := range wantX {
		if flow.positions[index].X != wantX[index] || flow.positions[index].Y != wantY[index] {
			t.Fatalf("word %d position = %v, want (%d,%d)", index, flow.positions[index], wantX[index], wantY[index])
		}
	}
}

func TestKaraokeFlowScalesBeforeWrapping(t *testing.T) {
	flow := layoutKaraokeFlowInto(nil, nil, []int{50, 50}, []int{0, 10}, 100, 20, 1, 2, .8, text.Start, false)
	if got := len(flow.rows); got != 1 {
		t.Fatalf("rows = %d, want one fitted row", got)
	}
	if flow.scale >= 1 || math.Abs(float64(flow.scale-100.0/110.0)) > .01 {
		t.Fatalf("scale = %.4f, want about %.4f", flow.scale, 100.0/110.0)
	}

	flow = layoutKaraokeFlowInto(flow.positions, flow.rows, []int{65, 65}, []int{0, 10}, 100, 20, 1, 2, .8, text.Start, false)
	if got := len(flow.rows); got != 2 {
		t.Fatalf("rows after minimum scale = %d, want 2", got)
	}
	if math.Abs(float64(flow.scale-.8)) > .001 {
		t.Fatalf("two-row scale = %.4f, want .8", flow.scale)
	}
}

func TestKaraokeFlowFitsAnOverwideWord(t *testing.T) {
	flow := layoutKaraokeFlowInto(nil, nil, []int{300}, []int{0}, 100, 20, 2, 2, .8, text.Start, false)
	if len(flow.rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(flow.rows))
	}
	if drawn := float32(flow.rows[0].width) * flow.scale; drawn > 100.01 {
		t.Fatalf("drawn width = %.2f, exceeds row width 100", drawn)
	}
	if math.Abs(float64(flow.scale-1.0/3.0)) > .01 {
		t.Fatalf("scale = %.4f, want about %.4f", flow.scale, 1.0/3.0)
	}
}

func TestKaraokeFlowMirrorsRTLWords(t *testing.T) {
	flow := layoutKaraokeFlowInto(nil, nil, []int{30, 30, 30}, []int{0, 10, 10}, 120, 20, 1, 1, .8, text.Start, true)
	want := []int{90, 50, 10}
	for index, x := range want {
		if flow.positions[index].X != x {
			t.Fatalf("word %d x = %d, want %d", index, flow.positions[index].X, x)
		}
	}
}

func TestKaraokeFillPolygonMirrorsForRTL(t *testing.T) {
	point := image.Pt(100, 50)
	ltr := karaokeFillPolygon(point, 80, 20, .25, false)
	rtl := karaokeFillPolygon(point, 80, 20, .25, true)
	sum := float32(280)
	for index := range ltr {
		if got, want := rtl[index].X, sum-ltr[index].X; got != want {
			t.Fatalf("point %d RTL x = %.1f, want mirrored %.1f", index, got, want)
		}
		if rtl[index].Y != ltr[index].Y {
			t.Fatalf("point %d RTL y = %.1f, want %.1f", index, rtl[index].Y, ltr[index].Y)
		}
	}
}

func TestKaraokeFillProgressAtWordEdges(t *testing.T) {
	word := KaraokeWord{StartMS: 1_000, EndMS: 1_400}
	cases := []struct {
		position int64
		want     float32
	}{
		{999, 0}, {1_000, 0}, {1_100, .25}, {1_200, .5}, {1_399, .9975}, {1_400, 1}, {1_500, 1},
	}
	for _, test := range cases {
		if got := KaraokeFillProgress(word, test.position); math.Abs(float64(got-test.want)) > .0001 {
			t.Errorf("progress at %d = %.4f, want %.4f", test.position, got, test.want)
		}
	}
}

func TestKaraokeFillProgressSettlesZeroLengthWord(t *testing.T) {
	word := KaraokeWord{StartMS: 1_000, EndMS: 1_000}
	if got := KaraokeFillProgress(word, 999); got != 0 {
		t.Fatalf("progress before zero-length word = %v, want 0", got)
	}
	if got := KaraokeFillProgress(word, 1_000); got != 1 {
		t.Fatalf("progress at zero-length word = %v, want 1", got)
	}
}
