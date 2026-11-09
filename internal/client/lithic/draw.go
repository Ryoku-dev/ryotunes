package lithic

import (
	"image"
	"image/color"
	"math"

	"gioui.org/f32"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
)

func clampCut(c, w, h int) int {
	return min(max(c, 0), min(w, h))
}

func cutPoints(r image.Rectangle, nw, ne, se, sw int) [8]f32.Point {
	w, h := max(0, r.Dx()), max(0, r.Dy())
	nw = clampCut(nw, w, h)
	ne = clampCut(ne, w, h)
	se = clampCut(se, w, h)
	sw = clampCut(sw, w, h)
	x0, y0 := float32(r.Min.X), float32(r.Min.Y)
	x1, y1 := float32(r.Max.X), float32(r.Max.Y)
	return [8]f32.Point{
		f32.Pt(x0+float32(nw), y0),
		f32.Pt(x1-float32(ne), y0),
		f32.Pt(x1, y0+float32(ne)),
		f32.Pt(x1, y1-float32(se)),
		f32.Pt(x1-float32(se), y1),
		f32.Pt(x0+float32(sw), y1),
		f32.Pt(x0, y1-float32(sw)),
		f32.Pt(x0, y0+float32(nw)),
	}
}

func pathFromPoints(ops *op.Ops, points []f32.Point) clip.PathSpec {
	var p clip.Path
	p.Begin(ops)
	if len(points) > 0 {
		p.MoveTo(points[0])
		for _, point := range points[1:] {
			p.LineTo(point)
		}
		p.Close()
	}
	return p.End()
}

type polylineQuad [4]f32.Point

func polylineQuadAlong(from, to, direction f32.Point, width float32) polylineQuad {
	normal := f32.Pt(-direction.Y*width*.5, direction.X*width*.5)
	return polylineQuad{
		from.Add(normal),
		from.Sub(normal),
		to.Sub(normal),
		to.Add(normal),
	}
}

func makePolylineQuad(from, to f32.Point, width float32) (polylineQuad, bool) {
	direction, ok := unitDirection(from, to)
	if !ok || width <= 0 {
		return polylineQuad{}, false
	}
	return polylineQuadAlong(from, to, direction, width), true
}

func polygonArea2(points []f32.Point) float32 {
	var area float32
	for i, point := range points {
		next := points[(i+1)%len(points)]
		area += point.X*next.Y - next.X*point.Y
	}
	return area
}

func appendPolygon(path *clip.Path, points []f32.Point, reverse bool) bool {
	if len(points) < 3 {
		return false
	}
	area := polygonArea2(points)
	if area == 0 {
		return false
	}
	forward := area > 0
	if reverse {
		forward = !forward
	}
	if forward {
		path.MoveTo(points[0])
		for _, point := range points[1:] {
			path.LineTo(point)
		}
	} else {
		path.MoveTo(points[len(points)-1])
		for i := len(points) - 2; i >= 0; i-- {
			path.LineTo(points[i])
		}
	}
	path.Close()
	return true
}

func cross(a, b f32.Point) float32 {
	return a.X*b.Y - a.Y*b.X
}

func unitDirection(from, to f32.Point) (f32.Point, bool) {
	dx, dy := to.X-from.X, to.Y-from.Y
	length := float32(math.Hypot(float64(dx), float64(dy)))
	if length == 0 {
		return f32.Point{}, false
	}
	return f32.Pt(dx/length, dy/length), true
}

func polylineJoin(previous, at, next f32.Point, width float32) ([4]f32.Point, int) {
	before, beforeOK := unitDirection(previous, at)
	after, afterOK := unitDirection(at, next)
	if !beforeOK || !afterOK {
		return [4]f32.Point{}, 0
	}
	turn := cross(before, after)
	if math.Abs(float64(turn)) < 1e-5 {
		return [4]f32.Point{}, 0
	}
	half := width * .5
	beforeNormal := f32.Pt(-before.Y*half, before.X*half)
	afterNormal := f32.Pt(-after.Y*half, after.X*half)
	side := float32(1)
	if turn > 0 {
		side = -1
	}
	a := at.Add(beforeNormal.Mul(side))
	b := at.Add(afterNormal.Mul(side))
	distance := b.Sub(a)
	t := cross(distance, after) / turn
	miter := a.Add(before.Mul(t))
	dx, dy := miter.X-at.X, miter.Y-at.Y
	if float32(math.Hypot(float64(dx), float64(dy))) > width*4 {
		return [4]f32.Point{a, b, at}, 3
	}
	return [4]f32.Point{a, miter, b, at}, 4
}

// CutPath returns the outline of r with independently cut corners.
func CutPath(ops *op.Ops, r image.Rectangle, nw, ne, se, sw int) clip.PathSpec {
	points := cutPoints(r, nw, ne, se, sw)
	return pathFromPoints(ops, points[:])
}

func fillCutCorners(ops *op.Ops, r image.Rectangle, nw, ne, se, sw int, c color.NRGBA) {
	if r.Empty() || c.A == 0 {
		return
	}
	paint.FillShape(ops, c, clip.Outline{Path: CutPath(ops, r, nw, ne, se, sw)}.Op())
}

// FillCut fills a rectangle with equal 45-degree corner cuts.
func FillCut(ops *op.Ops, r image.Rectangle, cut int, c color.NRGBA) {
	fillCutCorners(ops, r, cut, cut, cut, cut, c)
}

// FillCutTop fills a rectangle with only its top corners cut.
func FillCutTop(ops *op.Ops, r image.Rectangle, cut int, c color.NRGBA) {
	fillCutCorners(ops, r, cut, cut, 0, 0, c)
}

func insetCut(cut, width int) int {
	const diagonalInset = 2 - math.Sqrt2
	return max(0, int(math.Round(float64(cut)-float64(width)*diagonalInset)))
}

func strokeCutCorners(ops *op.Ops, r image.Rectangle, nw, ne, se, sw, width int, c color.NRGBA) {
	if r.Empty() || width <= 0 || c.A == 0 {
		return
	}
	outer := cutPoints(r, nw, ne, se, sw)
	var path clip.Path
	path.Begin(ops)
	appendPolygon(&path, outer[:], false)
	innerRect := r.Inset(width)
	if !innerRect.Empty() {
		inner := cutPoints(
			innerRect,
			insetCut(nw, width),
			insetCut(ne, width),
			insetCut(se, width),
			insetCut(sw, width),
		)
		appendPolygon(&path, inner[:], true)
	}
	paint.FillShape(ops, c, clip.Outline{Path: path.End()}.Op())
}

// StrokeCut draws the stroke wholly inside r.
func StrokeCut(ops *op.Ops, r image.Rectangle, cut, width int, c color.NRGBA) {
	strokeCutCorners(ops, r, cut, cut, cut, cut, width, c)
}

// Notch fills the state triangle at a corner.
func Notch(ops *op.Ops, at image.Point, size int, c color.NRGBA) {
	if size <= 0 || c.A == 0 {
		return
	}
	points := [...]f32.Point{
		f32.Pt(float32(at.X), float32(at.Y)),
		f32.Pt(float32(at.X+size), float32(at.Y)),
		f32.Pt(float32(at.X), float32(at.Y+size)),
	}
	paint.FillShape(ops, c, clip.Outline{Path: pathFromPoints(ops, points[:])}.Op())
}

func bladePoints(r image.Rectangle) [4]f32.Point {
	cut := min(max(0, r.Dy()), max(0, r.Dx()))
	x0, y0 := float32(r.Min.X), float32(r.Min.Y)
	x1, y1 := float32(r.Max.X), float32(r.Max.Y)
	return [4]f32.Point{
		f32.Pt(x0, y0),
		f32.Pt(x1-float32(cut), y0),
		f32.Pt(x1, y1),
		f32.Pt(x0, y1),
	}
}

// Blade fills a progress quadrilateral with a cut leading edge.
func Blade(ops *op.Ops, r image.Rectangle, c color.NRGBA) {
	if r.Empty() || c.A == 0 {
		return
	}
	points := bladePoints(r)
	paint.FillShape(ops, c, clip.Outline{Path: pathFromPoints(ops, points[:])}.Op())
}

// Polyline draws a filled polyline with butt caps and mitered joins.
func Polyline(ops *op.Ops, points []f32.Point, closed bool, width float32, c color.NRGBA) {
	if len(points) < 2 || width <= 0 || c.A == 0 {
		return
	}
	segments := len(points) - 1
	if closed {
		segments = len(points)
	}
	var path clip.Path
	path.Begin(ops)
	drawn := false
	for i := range segments {
		quad, ok := makePolylineQuad(points[i], points[(i+1)%len(points)], width)
		if ok {
			drawn = appendPolygon(&path, quad[:], false) || drawn
		}
	}
	if len(points) > 2 {
		first, last := 1, len(points)-1
		if closed {
			first, last = 0, len(points)
		}
		for i := first; i < last; i++ {
			join, count := polylineJoin(
				points[(i-1+len(points))%len(points)],
				points[i%len(points)],
				points[(i+1)%len(points)],
				width,
			)
			if count > 0 {
				drawn = appendPolygon(&path, join[:count], false) || drawn
			}
		}
	}
	spec := path.End()
	if drawn {
		paint.FillShape(ops, c, clip.Outline{Path: spec}.Op())
	}
}

// Dashes draws a butt-ended dashed polyline with a continuous dash phase.
func Dashes(ops *op.Ops, points []f32.Point, closed bool, width, dash, gap float32, c color.NRGBA) {
	if len(points) < 2 || width <= 0 || dash <= 0 || c.A == 0 {
		return
	}
	if gap <= 0 {
		Polyline(ops, points, closed, width, c)
		return
	}
	segments := len(points) - 1
	if closed {
		segments = len(points)
	}
	var path clip.Path
	path.Begin(ops)
	drawing := true
	remaining := dash
	drawn := false
	for i := range segments {
		from, to := points[i], points[(i+1)%len(points)]
		dx, dy := to.X-from.X, to.Y-from.Y
		length := float32(math.Hypot(float64(dx), float64(dy)))
		if length == 0 {
			continue
		}
		direction := f32.Pt(dx/length, dy/length)
		for position := float32(0); position < length; {
			step := min(remaining, length-position)
			if drawing && step > 0 {
				start := from.Add(direction.Mul(position))
				end := from.Add(direction.Mul(position + step))
				quad := polylineQuadAlong(start, end, direction, width)
				drawn = appendPolygon(&path, quad[:], false) || drawn
			}
			position += step
			remaining -= step
			if remaining <= 1e-5 {
				drawing = !drawing
				if drawing {
					remaining = dash
				} else {
					remaining = gap
				}
			}
		}
	}
	spec := path.End()
	if drawn {
		paint.FillShape(ops, c, clip.Outline{Path: spec}.Op())
	}
}

// Hatch draws clipped diagonal engraving lines.
func Hatch(ops *op.Ops, area clip.PathSpec, bounds image.Rectangle, spacing, width int, c color.NRGBA) {
	if bounds.Empty() || spacing <= 0 || width <= 0 || c.A == 0 {
		return
	}
	areaStack := clip.Outline{Path: area}.Op().Push(ops)
	var path clip.Path
	path.Begin(ops)
	h := bounds.Dy()
	drawn := false
	direction := f32.Pt(float32(math.Sqrt2/2), -float32(math.Sqrt2/2))
	for x := bounds.Min.X - h; x <= bounds.Max.X; x += spacing {
		from := f32.Pt(float32(x), float32(bounds.Max.Y))
		to := f32.Pt(float32(x+h), float32(bounds.Min.Y))
		quad := polylineQuadAlong(from, to, direction, float32(width))
		drawn = appendPolygon(&path, quad[:], false) || drawn
	}
	spec := path.End()
	if drawn {
		paint.FillShape(ops, c, clip.Outline{Path: spec}.Op())
	}
	areaStack.Pop()
}

// OctagonPoints returns the vertices of an axis-aligned regular octagon.
func OctagonPoints(center f32.Point, apothem float32) [8]f32.Point {
	a := max(float32(0), apothem)
	k := a * float32(math.Tan(math.Pi/8))
	return [8]f32.Point{
		f32.Pt(center.X-k, center.Y-a),
		f32.Pt(center.X+k, center.Y-a),
		f32.Pt(center.X+a, center.Y-k),
		f32.Pt(center.X+a, center.Y+k),
		f32.Pt(center.X+k, center.Y+a),
		f32.Pt(center.X-k, center.Y+a),
		f32.Pt(center.X-a, center.Y+k),
		f32.Pt(center.X-a, center.Y-k),
	}
}

// Octagon returns the Lithic octagonal outline.
func Octagon(ops *op.Ops, center f32.Point, apothem float32) clip.PathSpec {
	points := OctagonPoints(center, apothem)
	return pathFromPoints(ops, points[:])
}
