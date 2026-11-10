package look

import (
	"image"
	"math"
	"time"

	"gioui.org/f32"
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

func drawArtPlaceholder(gtx layout.Context, th *Theme, size image.Point, shape Shape) layout.Dimensions {
	bounds := image.Rectangle{Max: size}
	path := artPath(gtx, size, shape)
	mask := clip.Outline{Path: path}.Op().Push(gtx.Ops)
	paint.FillShape(gtx.Ops, th.R.Stone3, clip.Rect(bounds).Op())
	kit.Hatch(gtx.Ops, path, bounds, max(1, gtx.Dp(8)), max(1, gtx.Dp(1)), palette.WithAlpha(th.R.Fg3, .45))
	mask.Pop()
	return layout.Dimensions{Size: size}
}

// Art draws a centre-cropped cached cover or a quiet Lithic hatch.
func Art(gtx layout.Context, th *Theme, cache *art.Cache, url string, size image.Point, shape Shape) layout.Dimensions {
	size.X = min(max(size.X, 0), max(gtx.Constraints.Max.X, 0))
	size.Y = min(max(size.Y, 0), max(gtx.Constraints.Max.Y, 0))
	path := artPath(gtx, size, shape)
	mask := clip.Outline{Path: path}.Op().Push(gtx.Ops)
	var source paint.ImageOp
	loaded := false
	if cache != nil {
		source, loaded = cache.Get(url, max(size.X, size.Y))
	}
	if loaded {
		widget.Image{Src: source, Fit: widget.Cover, Position: layout.Center}.Layout(exactArtContext(gtx, size))
		mask.Pop()
		return layout.Dimensions{Size: size}
	}
	mask.Pop()
	return drawArtPlaceholder(gtx, th, size, shape)
}

// ArtView reveals artwork changes with a diagonal Lithic cut.
type ArtView struct {
	current       string
	previous      string
	ready         bool
	currentLoaded bool
	transition    anim.Tween
	transitioning bool
	reveal        bool
}

// Layout draws the current cover and retains the outgoing cover only for its reveal.
func (a *ArtView) Layout(gtx layout.Context, th *Theme, cache *art.Cache, url string, size image.Point, shape Shape, duration time.Duration) layout.Dimensions {
	requestSize := max(size.X, size.Y)
	loaded := false
	if cache != nil {
		_, loaded = cache.Get(url, requestSize)
	}
	if !a.ready {
		a.current, a.currentLoaded, a.ready = url, loaded, true
	} else if url != a.current {
		if a.currentLoaded {
			a.previous = a.current
		}
		a.current, a.currentLoaded = url, false
		a.transitioning = false
	}
	if loaded && !a.currentLoaded {
		a.currentLoaded = true
		a.reveal = a.previous != ""
		a.transition = anim.Tween{Duration: duration, Ease: anim.EaseChisel}
		a.transition.Set(gtx, 0)
		a.transition.Set(gtx, 1)
		a.transitioning = duration > 0 && !anim.Reduced()
		if !a.transitioning {
			a.previous = ""
		}
	}
	if !a.currentLoaded && (cache == nil || cache.Failed(url, requestSize)) {
		a.previous = ""
	}
	if !a.currentLoaded {
		if a.previous != "" {
			return Art(gtx, th, cache, a.previous, size, shape)
		}
		return drawArtPlaceholder(gtx, th, size, shape)
	}
	if !a.transitioning {
		return Art(gtx, th, cache, a.current, size, shape)
	}

	progress := a.transition.Value(gtx)
	var dimensions layout.Dimensions
	if a.reveal {
		Art(gtx, th, cache, a.previous, size, shape)
		points, count := revealMask(size, progress)
		if count >= 3 {
			var path clip.Path
			path.Begin(gtx.Ops)
			path.MoveTo(points[0])
			for index := 1; index < count; index++ {
				path.LineTo(points[index])
			}
			path.Close()
			mask := clip.Outline{Path: path.End()}.Op().Push(gtx.Ops)
			dimensions = Art(gtx, th, cache, a.current, size, shape)
			mask.Pop()
		} else {
			dimensions = layout.Dimensions{Size: size}
		}
	} else {
		drawArtPlaceholder(gtx, th, size, shape)
		incoming := paint.PushOpacity(gtx.Ops, progress)
		dimensions = Art(gtx, th, cache, a.current, size, shape)
		incoming.Pop()
	}
	if progress >= 1 {
		a.previous = ""
		a.transitioning = false
	}
	return dimensions
}

func revealMask(size image.Point, progress float32) ([5]f32.Point, int) {
	var points [5]f32.Point
	if size.X <= 0 || size.Y <= 0 || progress <= 0 {
		return points, 0
	}
	progress = min(progress, 1)
	width, height := float32(size.X), float32(size.Y)
	edge := progress * (width + height)
	count := 0
	add := func(point f32.Point) {
		if count > 0 && points[count-1] == point {
			return
		}
		points[count] = point
		count++
	}
	add(f32.Pt(0, 0))
	add(f32.Pt(min(width, edge), 0))
	if edge >= width {
		add(f32.Pt(width, min(height, edge-width)))
	}
	if edge >= height {
		add(f32.Pt(min(width, edge-height), height))
	}
	add(f32.Pt(0, min(height, edge)))
	if count > 1 && points[count-1] == points[0] {
		count--
	}
	return points, count
}
