package look

import (
	"math"
	"testing"
)

func TestSVGProviderAndMarkBounds(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		maximum    float32
		minimumMax float32
	}{
		{"YouTube Music", youtubeGlyphPath, 24, 18},
		{"Spotify", spotifyGlyphPath, 24, 18},
		{"SoundCloud", soundCloudGlyphPath, 24, 18},
		{"TIDAL", tidalGlyphPath, 24, 18},
		{"Ryoku bone", ryokuMarkBonePath, 556, 450},
		{"Ryoku pointer", ryokuMarkTipPath, 556, 130},
	}
	for _, test := range tests {
		geometry, err := parseSVGPath(test.path)
		if err != nil {
			t.Fatalf("%s: %v", test.name, err)
		}
		if !geometry.hasBounds {
			t.Fatalf("%s has no bounds", test.name)
		}
		if geometry.minimum.X < 0 || geometry.minimum.Y < 0 || geometry.maximum.X > test.maximum || geometry.maximum.Y > test.maximum {
			t.Errorf("%s bounds = %v..%v", test.name, geometry.minimum, geometry.maximum)
		}
		if geometry.maximum.X < test.minimumMax || geometry.maximum.Y < test.minimumMax {
			t.Errorf("%s unexpectedly small bounds = %v", test.name, geometry.maximum)
		}
	}
}

func TestSVGArcEndpointExact(t *testing.T) {
	geometry, err := parseSVGPath("M1 2 A8 5 27 1 0 19.25 14.5")
	if err != nil {
		t.Fatal(err)
	}
	if geometry.end.X != 19.25 || geometry.end.Y != 14.5 {
		t.Fatalf("endpoint = %v", geometry.end)
	}
	last := geometry.operations[len(geometry.operations)-1]
	if last.kind != 'C' || last.points[2] != geometry.end {
		t.Fatalf("last arc segment = %#v", last)
	}
	for _, operation := range geometry.operations {
		for _, point := range operation.points {
			if math.IsNaN(float64(point.X)) || math.IsNaN(float64(point.Y)) {
				t.Fatal("arc generated NaN")
			}
		}
	}
}

func TestSVGNumberAndImplicitSyntax(t *testing.T) {
	geometry, err := parseSVGPath("M1e1-0.5.5 20 30 40h-2v3s1 2 3 4t5 6z")
	if err != nil {
		t.Fatal(err)
	}
	if len(geometry.operations) < 7 {
		t.Fatalf("operations = %d", len(geometry.operations))
	}
}

func TestSVGPathCacheReusesParsedGeometry(t *testing.T) {
	first, err := cachedSVGPath(spotifyGlyphPath)
	if err != nil {
		t.Fatal(err)
	}
	second, err := cachedSVGPath(spotifyGlyphPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.operations) == 0 || len(second.operations) == 0 {
		t.Fatal("cached geometry is empty")
	}
	if &first.operations[0] != &second.operations[0] {
		t.Fatal("cached path was parsed more than once")
	}
}
