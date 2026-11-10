package look

import (
	"image"
	"image/color"
	"math"
	"testing"
	"time"

	"gioui.org/io/input"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
	libtheme "github.com/ryoku-dev/ryoku/docs/lithic/library/go/theme"
)

func TestBackdropToneKeepsTheFieldsLightnessWhateverTheCover(t *testing.T) {
	cover := image.NewRGBA(image.Rect(0, 0, 4, 1))
	copy(cover.Pix, []uint8{
		255, 255, 255, 255,
		235, 20, 40, 255,
		0, 0, 0, 255,
		// half see-through olive, premultiplied
		64, 64, 0, 128,
	})
	for _, dark := range []bool{true, false} {
		field := NewTheme(palette.Signature(dark), libtheme.Comfortable).Accent.Field
		want := colorToOKLCH(field).L
		toned := toneBackdrop(cover, want)
		for index := range 4 {
			at := index * 4
			alpha := toned.Pix[at+3]
			if alpha != cover.Pix[at+3] {
				t.Fatalf("dark=%v pixel %d alpha = %d, want %d", dark, index, alpha, cover.Pix[at+3])
			}
			straight := func(channel uint8) uint8 {
				return uint8(min(float64(channel)*255/float64(alpha)+.5, 255))
			}
			pixel := colorToOKLCH(color.NRGBA{R: straight(toned.Pix[at]), G: straight(toned.Pix[at+1]), B: straight(toned.Pix[at+2]), A: 255})
			if math.Abs(pixel.L-want) > backdropLightness+.01 {
				t.Errorf("dark=%v pixel %d lightness %.3f strays from the field's %.3f", dark, index, pixel.L, want)
			}
			if pixel.C > backdropChroma+.01 {
				t.Errorf("dark=%v pixel %d chroma %.3f, want at most %.3f", dark, index, pixel.C, backdropChroma)
			}
		}
	}
}

func TestBackdropCoverLandingMidFadeNeverJumpsAndSettles(t *testing.T) {
	theme := NewTheme(palette.Signature(true), libtheme.Comfortable)
	var operations op.Ops
	var router input.Router
	now := time.Unix(100, 0)
	bounds := image.Rect(0, 0, 200, 100)
	context := func() layout.Context {
		operations.Reset()
		return layout.Context{
			Ops: &operations, Now: now, Source: router.Source(),
			Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(bounds.Size()),
		}
	}
	solid := func(value uint8) *image.RGBA {
		cover := image.NewRGBA(image.Rect(0, 0, 4, 4))
		for index := range cover.Pix {
			cover.Pix[index] = value
		}
		return cover
	}
	old, next := solid(200), solid(40)
	field := theme.Accent.Field
	visible := func(backdrop *Backdrop) (float32, float32) {
		return backdrop.visible(backdrop.fade.Value(context()))
	}

	var backdrop Backdrop
	backdrop.paintSource(context(), theme, old, field, bounds, [4]int{})
	if leaving, shown := visible(&backdrop); leaving != 0 || shown != 1 {
		t.Fatalf("first cover shows %v with %v leaving, want it whole at once", shown, leaving)
	}

	// The track changes while its cover is still being sampled.
	now = now.Add(time.Millisecond)
	backdrop.paintSource(context(), theme, nil, field, bounds, [4]int{})
	now = now.Add(theme.Motion.Field / 2)
	before, _ := visible(&backdrop)
	if before <= 0 || before >= 1 {
		t.Fatalf("old cover halfway through its fade shows %v, want it partly gone", before)
	}
	backdrop.paintSource(context(), theme, next, field, bounds, [4]int{})
	after, shown := visible(&backdrop)
	if math.Abs(float64(after-before)) > .01 {
		t.Fatalf("old cover jumped from %v to %v when the next one landed", before, after)
	}
	if shown != 0 {
		t.Fatalf("next cover starts at %v, want it to fade in from 0", shown)
	}

	now = now.Add(theme.Motion.Field)
	backdrop.paintSource(context(), theme, next, field, bounds, [4]int{})
	if leaving, shown := visible(&backdrop); leaving != 0 || shown != 1 {
		t.Fatalf("settled backdrop shows %v with %v leaving, want only the next cover", shown, leaving)
	}
	router.Frame(&operations)
	router.WakeupTime()
	now = now.Add(time.Millisecond)
	backdrop.paintSource(context(), theme, next, field, bounds, [4]int{})
	router.Frame(&operations)
	if wake, ok := router.WakeupTime(); ok {
		t.Fatalf("settled backdrop requested a frame at %v", wake)
	}
}
