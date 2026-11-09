package look

import (
	"image"
	"math"
	"time"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/anim"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
	"github.com/ryoku-dev/ryotunes/internal/client/art"
)

// Shape selects cut artwork corners or a regular octagon.
type Shape struct {
	Cut     unit.Dp
	Octagon bool
	Corners [4]bool
}

func artPath(gtx layout.Context, size image.Point, shape Shape) clip.PathSpec {
	bounds := image.Rectangle{Max: size}
	cut := gtx.Dp(shape.Cut)
	if shape.Octagon {
		cut = int(math.Round(float64(min(size.X, size.Y)) * .293))
		return kit.CutPath(gtx.Ops, bounds, cut, cut, cut, cut)
	}
	anyCorner := false
	for _, enabled := range shape.Corners {
		anyCorner = anyCorner || enabled
	}
	if !anyCorner {
		return kit.CutPath(gtx.Ops, bounds, cut, cut, cut, cut)
	}
	cuts := [4]int{}
	for index, enabled := range shape.Corners {
		if enabled {
			cuts[index] = cut
		}
	}
	return kit.CutPath(gtx.Ops, bounds, cuts[0], cuts[1], cuts[2], cuts[3])
}

func exactArtContext(gtx layout.Context, size image.Point) layout.Context {
	gtx.Constraints = layout.Exact(size)
	return gtx
}

var artRequestSizes = [...]int{32, 48, 64, 96, 128, 192, 256, 384, 512, 768, 1024, 1536, 2048}

func artRequestSize(size int) int {
	for _, candidate := range artRequestSizes {
		if size <= candidate {
			return candidate
		}
	}
	return artRequestSizes[len(artRequestSizes)-1]
}

// Art draws a centre-cropped cached cover or a quiet Lithic hatch.
func Art(gtx layout.Context, th *Theme, cache *art.Cache, url string, size image.Point, shape Shape) layout.Dimensions {
	size.X = min(max(size.X, 0), max(gtx.Constraints.Max.X, 0))
	size.Y = min(max(size.Y, 0), max(gtx.Constraints.Max.Y, 0))
	bounds := image.Rectangle{Max: size}
	path := artPath(gtx, size, shape)
	mask := clip.Outline{Path: path}.Op().Push(gtx.Ops)
	var source paint.ImageOp
	loaded := false
	if cache != nil {
		source, loaded = cache.Get(url, artRequestSize(max(size.X, size.Y)))
	}
	if loaded {
		widget.Image{Src: source, Fit: widget.Cover, Position: layout.Center}.Layout(exactArtContext(gtx, size))
	} else {
		paint.FillShape(gtx.Ops, th.R.Stone3, clip.Rect(bounds).Op())
		kit.Hatch(gtx.Ops, path, bounds, max(1, gtx.Dp(8)), max(1, gtx.Dp(1)), palette.WithAlpha(th.R.Fg3, .45))
	}
	mask.Pop()
	return layout.Dimensions{Size: size}
}

// ArtView crossfades artwork when its URL changes.
type ArtView struct {
	current  string
	previous string
	ready    bool
	fade     anim.Tween
	fading   bool
}

// Layout draws the current cover and retains the outgoing cover only while fading.
func (a *ArtView) Layout(gtx layout.Context, th *Theme, cache *art.Cache, url string, size image.Point, shape Shape, duration time.Duration) layout.Dimensions {
	if !a.ready {
		a.current = url
		a.ready = true
	} else if url != a.current {
		a.previous = a.current
		a.current = url
		a.fade = anim.Tween{Duration: duration, Ease: anim.EaseChisel}
		a.fade.Set(gtx, 0)
		a.fade.Set(gtx, 1)
		a.fading = duration > 0 && !anim.Reduced()
	}
	if !a.fading {
		return Art(gtx, th, cache, a.current, size, shape)
	}
	progress := a.fade.Value(gtx)
	if progress < 1 {
		outgoing := paint.PushOpacity(gtx.Ops, 1-progress)
		Art(gtx, th, cache, a.previous, size, shape)
		outgoing.Pop()
	}
	incoming := paint.PushOpacity(gtx.Ops, progress)
	dimensions := Art(gtx, th, cache, a.current, size, shape)
	incoming.Pop()
	if progress >= 1 {
		a.previous = ""
		a.fading = false
	}
	return dimensions
}
