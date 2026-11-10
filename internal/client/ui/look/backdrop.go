package look

import (
	"image"
	"image/color"
	"math"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/widget"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/anim"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryotunes/internal/client/art"
)

// The backdrop shows the cover, blurred, with its tones pressed into a band
// around the plane's OKLCH lightness: deep on the side away from the text and
// shallow toward it, so type keeps the contrast it has on the bare plane
// whatever the cover. Chroma is capped so a loud cover stays a colour, not a
// glare.
const (
	backdropDeep    = .16
	backdropShallow = .05
	backdropChroma  = .12
	backdropOpacity = .9
	// how far the plane may drift, as it does while a tint tweens, before the
	// cover is toned again
	backdropRetone = .02
)

// Backdrop draws a cover's blur over a plane and fades between covers.
type Backdrop struct {
	shown, leaving backdropLayer
	// how much of the leaving cover still showed when it began to leave
	leavingFrom float32
	fade        anim.Tween
	started     bool
}

type backdropLayer struct {
	source    *image.RGBA
	lightness float64
	dark      bool
	op        paint.ImageOp
	toned     bool
}

// Paint draws the cover's blur scaled over bounds inside the cut shape, toned
// to field. With no cover, or one not sampled yet, the last one fades out.
func (b *Backdrop) Paint(gtx layout.Context, theme *Theme, cache *art.Cache, cover string, field color.NRGBA, bounds image.Rectangle, cuts [4]int) {
	var source *image.RGBA
	if cache != nil {
		source, _ = cache.Backdrop(cover)
	}
	b.paintSource(gtx, theme, source, field, bounds, cuts)
}

func (b *Backdrop) paintSource(gtx layout.Context, theme *Theme, source *image.RGBA, field color.NRGBA, bounds image.Rectangle, cuts [4]int) {
	if !b.started {
		b.started = true
		b.shown = backdropLayer{source: source}
		b.fade = anim.Tween{Duration: theme.Motion.Field, Ease: anim.EaseChisel}
		b.fade.Set(gtx, 1)
	} else if source != b.shown.source {
		b.change(gtx, theme, source)
	}
	b.shown.tone(field, theme.R.Dark)
	progress := b.fade.Value(gtx)
	if progress >= 1 {
		b.leaving = backdropLayer{}
	}
	leaving, shown := b.visible(progress)
	if leaving <= 0 && shown <= 0 {
		return
	}
	mask := clip.Outline{Path: kit.CutPath(gtx.Ops, bounds, cuts[0], cuts[1], cuts[2], cuts[3])}.Op().Push(gtx.Ops)
	b.leaving.paint(gtx, bounds, backdropOpacity*leaving)
	b.shown.paint(gtx, bounds, backdropOpacity*shown)
	mask.Pop()
}

// visible reports how much of the leaving and the shown cover show at progress.
func (b *Backdrop) visible(progress float32) (leaving, shown float32) {
	if b.leaving.toned {
		leaving = b.leavingFrom * (1 - progress)
	}
	if b.shown.toned {
		shown = progress
	}
	return leaving, shown
}

// change fades to source. Whichever cover shows more becomes the one leaving,
// from where it is, so a cover that lands mid-fade never makes one jump.
func (b *Backdrop) change(gtx layout.Context, theme *Theme, source *image.RGBA) {
	leaving, shown := b.visible(b.fade.Value(gtx))
	if shown >= leaving {
		b.leaving, b.leavingFrom = b.shown, shown
	} else {
		b.leavingFrom = leaving
	}
	b.shown = backdropLayer{source: source}
	b.fade = anim.Tween{Duration: theme.Motion.Field, Ease: anim.EaseChisel}
	b.fade.Set(gtx, 0)
	b.fade.Set(gtx, 1)
}

func (layer *backdropLayer) tone(field color.NRGBA, dark bool) {
	if layer.source == nil {
		return
	}
	lightness := colorToOKLCH(field).L
	if layer.toned && layer.dark == dark && math.Abs(layer.lightness-lightness) < backdropRetone {
		return
	}
	layer.op = paint.NewImageOp(toneBackdrop(layer.source, lightness, dark))
	layer.lightness, layer.dark, layer.toned = lightness, dark, true
}

func (layer *backdropLayer) paint(gtx layout.Context, bounds image.Rectangle, opacity float32) {
	if !layer.toned || opacity <= 0 {
		return
	}
	alpha := paint.PushOpacity(gtx.Ops, opacity)
	offset := op.Offset(bounds.Min).Push(gtx.Ops)
	widget.Image{Src: layer.op, Fit: widget.Cover, Position: layout.Center}.Layout(exactArtContext(gtx, bounds.Size()))
	offset.Pop()
	alpha.Pop()
}

// toneBackdrop keeps each pixel's hue and order of tones but presses its
// lightness into the band around the plane's and caps its colour.
func toneBackdrop(source *image.RGBA, lightness float64, dark bool) *image.RGBA {
	low, high := lightness-backdropShallow, min(lightness+backdropDeep, 1)
	if dark {
		low, high = max(lightness-backdropDeep, 0), lightness+backdropShallow
	}
	toned := image.NewRGBA(source.Rect)
	for at := 0; at+3 < len(source.Pix); at += 4 {
		alpha := source.Pix[at+3]
		if alpha == 0 {
			continue
		}
		straight := func(channel uint8) uint8 {
			return uint8(min(float64(channel)*255/float64(alpha)+.5, 255))
		}
		pixel := colorToOKLCH(color.NRGBA{R: straight(source.Pix[at]), G: straight(source.Pix[at+1]), B: straight(source.Pix[at+2]), A: 255})
		pixel.L = low + pixel.L*(high-low)
		pixel.C = min(pixel.C, backdropChroma)
		out := oklchToColor(pixel)
		premultiply := func(channel uint8) uint8 { return uint8((uint32(channel)*uint32(alpha) + 127) / 255) }
		toned.Pix[at], toned.Pix[at+1], toned.Pix[at+2], toned.Pix[at+3] = premultiply(out.R), premultiply(out.G), premultiply(out.B), alpha
	}
	return toned
}

// FieldBackdrop paints a field with its cover's blur behind body.
func FieldBackdrop(gtx layout.Context, theme *Theme, accent Accent, cuts [4]int, backdrop *Backdrop, cache *art.Cache, cover string, body layout.Widget) layout.Dimensions {
	record := op.Record(gtx.Ops)
	dimensions := body(gtx)
	call := record.Stop()
	bounds := image.Rectangle{Max: dimensions.Size}
	area := clip.Rect(bounds).Push(gtx.Ops)
	kit.FillCutCorners(gtx.Ops, bounds, cuts[0], cuts[1], cuts[2], cuts[3], accent.Field)
	if backdrop != nil {
		backdrop.Paint(gtx, theme, cache, cover, accent.Field, bounds, cuts)
	}
	call.Add(gtx.Ops)
	area.Pop()
	return dimensions
}
