package look

import (
	"fmt"
	"image"
	"math"
	"strconv"
	"strings"
	"sync"
	"unicode"

	"gioui.org/f32"
	"gioui.org/op"
	"gioui.org/op/clip"
)

type svgOperation struct {
	kind   byte
	points [3]f32.Point
}

type svgGeometry struct {
	operations []svgOperation
	minimum    f32.Point
	maximum    f32.Point
	end        f32.Point
	hasBounds  bool
}

type svgScanner struct {
	data  string
	index int
}

type svgCacheEntry struct {
	geometry svgGeometry
	err      error
}

var svgGeometryCache sync.Map

func cachedSVGPath(data string) (svgGeometry, error) {
	if cached, ok := svgGeometryCache.Load(data); ok {
		entry := cached.(svgCacheEntry)
		return entry.geometry, entry.err
	}
	geometry, err := parseSVGPath(data)
	entry := svgCacheEntry{geometry: geometry, err: err}
	actual, _ := svgGeometryCache.LoadOrStore(data, entry)
	stored := actual.(svgCacheEntry)
	return stored.geometry, stored.err
}

func parseSVGPath(data string) (svgGeometry, error) {
	scanner := svgScanner{data: data}
	var geometry svgGeometry
	var command byte
	var current, start, cubicControl, quadControl f32.Point
	var previous byte

	for {
		scanner.space()
		if scanner.index >= len(scanner.data) {
			break
		}
		if isSVGCommand(scanner.data[scanner.index]) {
			command = scanner.data[scanner.index]
			scanner.index++
		} else if command == 0 {
			return geometry, fmt.Errorf("svg path: command expected at %d", scanner.index)
		}
		upper := command
		if upper >= 'a' && upper <= 'z' {
			upper -= 'a' - 'A'
		}
		relative := command >= 'a' && command <= 'z'
		if upper == 'Z' {
			geometry.add(svgOperation{kind: 'Z'})
			current = start
			geometry.end = current
			previous = 'Z'
			command = 0
			continue
		}
		count := svgArgumentCount(upper)
		if count == 0 {
			return geometry, fmt.Errorf("svg path: unsupported command %q", command)
		}
		first := true
		consumed := false
		for scanner.hasNumber() {
			values := make([]float64, count)
			for index := range values {
				value, ok := scanner.number()
				if !ok {
					return geometry, fmt.Errorf("svg path: %c needs %d arguments", command, count)
				}
				values[index] = value
			}
			consumed = true
			actual := upper
			if upper == 'M' && !first {
				actual = 'L'
			}
			first = false
			point := func(x, y float64) f32.Point {
				result := f32.Pt(float32(x), float32(y))
				if relative {
					result = result.Add(current)
				}
				return result
			}
			switch actual {
			case 'M':
				to := point(values[0], values[1])
				geometry.add(svgOperation{kind: 'M', points: [3]f32.Point{to}})
				current, start = to, to
			case 'L':
				to := point(values[0], values[1])
				geometry.add(svgOperation{kind: 'L', points: [3]f32.Point{to}})
				current = to
			case 'H':
				x := float32(values[0])
				if relative {
					x += current.X
				}
				to := f32.Pt(x, current.Y)
				geometry.add(svgOperation{kind: 'L', points: [3]f32.Point{to}})
				current = to
			case 'V':
				y := float32(values[0])
				if relative {
					y += current.Y
				}
				to := f32.Pt(current.X, y)
				geometry.add(svgOperation{kind: 'L', points: [3]f32.Point{to}})
				current = to
			case 'C':
				control0 := point(values[0], values[1])
				control1 := point(values[2], values[3])
				to := point(values[4], values[5])
				geometry.add(svgOperation{kind: 'C', points: [3]f32.Point{control0, control1, to}})
				current, cubicControl = to, control1
			case 'S':
				control0 := current
				if previous == 'C' || previous == 'S' {
					control0 = current.Mul(2).Sub(cubicControl)
				}
				control1 := point(values[0], values[1])
				to := point(values[2], values[3])
				geometry.add(svgOperation{kind: 'C', points: [3]f32.Point{control0, control1, to}})
				current, cubicControl = to, control1
			case 'Q':
				control := point(values[0], values[1])
				to := point(values[2], values[3])
				geometry.add(svgOperation{kind: 'Q', points: [3]f32.Point{control, to}})
				current, quadControl = to, control
			case 'T':
				control := current
				if previous == 'Q' || previous == 'T' {
					control = current.Mul(2).Sub(quadControl)
				}
				to := point(values[0], values[1])
				geometry.add(svgOperation{kind: 'Q', points: [3]f32.Point{control, to}})
				current, quadControl = to, control
			case 'A':
				to := point(values[5], values[6])
				segments := svgArc(current, to, values[0], values[1], values[2], values[3] != 0, values[4] != 0)
				if len(segments) == 0 && current != to {
					geometry.add(svgOperation{kind: 'L', points: [3]f32.Point{to}})
				} else {
					for _, segment := range segments {
						geometry.add(segment)
					}
				}
				current = to
			}
			previous = actual
			if actual != 'C' && actual != 'S' {
				cubicControl = current
			}
			if actual != 'Q' && actual != 'T' {
				quadControl = current
			}
			geometry.end = current
			scanner.space()
			if scanner.index < len(scanner.data) && isSVGCommand(scanner.data[scanner.index]) {
				break
			}
		}
		if !consumed {
			return geometry, fmt.Errorf("svg path: %c has no coordinates", command)
		}
		if upper == 'M' {
			if relative {
				command = 'l'
			} else {
				command = 'L'
			}
		}
	}
	return geometry, nil
}

func (geometry *svgGeometry) add(operation svgOperation) {
	geometry.operations = append(geometry.operations, operation)
	count := 0
	switch operation.kind {
	case 'M', 'L':
		count = 1
	case 'Q':
		count = 2
	case 'C':
		count = 3
	}
	for index := 0; index < count; index++ {
		point := operation.points[index]
		if !geometry.hasBounds {
			geometry.minimum, geometry.maximum, geometry.hasBounds = point, point, true
			continue
		}
		geometry.minimum.X = min(geometry.minimum.X, point.X)
		geometry.minimum.Y = min(geometry.minimum.Y, point.Y)
		geometry.maximum.X = max(geometry.maximum.X, point.X)
		geometry.maximum.Y = max(geometry.maximum.Y, point.Y)
	}
}

func buildSVGPath(ops *op.Ops, data string, source f32.Point, size image.Point) (clip.PathSpec, svgGeometry, error) {
	geometry, err := cachedSVGPath(data)
	if err != nil {
		return clip.PathSpec{}, geometry, err
	}
	scaleX, scaleY := float32(size.X)/source.X, float32(size.Y)/source.Y
	scale := func(point f32.Point) f32.Point { return f32.Pt(point.X*scaleX, point.Y*scaleY) }
	var path clip.Path
	path.Begin(ops)
	for _, operation := range geometry.operations {
		switch operation.kind {
		case 'M':
			path.MoveTo(scale(operation.points[0]))
		case 'L':
			path.LineTo(scale(operation.points[0]))
		case 'Q':
			path.QuadTo(scale(operation.points[0]), scale(operation.points[1]))
		case 'C':
			path.CubeTo(scale(operation.points[0]), scale(operation.points[1]), scale(operation.points[2]))
		case 'Z':
			path.Close()
		}
	}
	return path.End(), geometry, nil
}

func svgArc(from, to f32.Point, rxValue, ryValue, rotation float64, large, sweep bool) []svgOperation {
	rx, ry := math.Abs(rxValue), math.Abs(ryValue)
	if rx == 0 || ry == 0 || from == to {
		return nil
	}
	phi := rotation * math.Pi / 180
	cosPhi, sinPhi := math.Cos(phi), math.Sin(phi)
	dx, dy := (float64(from.X)-float64(to.X))/2, (float64(from.Y)-float64(to.Y))/2
	xp := cosPhi*dx + sinPhi*dy
	yp := -sinPhi*dx + cosPhi*dy
	radiusScale := xp*xp/(rx*rx) + yp*yp/(ry*ry)
	if radiusScale > 1 {
		factor := math.Sqrt(radiusScale)
		rx, ry = rx*factor, ry*factor
	}
	numerator := max(0, rx*rx*ry*ry-rx*rx*yp*yp-ry*ry*xp*xp)
	denominator := rx*rx*yp*yp + ry*ry*xp*xp
	coefficient := 0.0
	if denominator != 0 {
		coefficient = math.Sqrt(numerator / denominator)
	}
	if large == sweep {
		coefficient = -coefficient
	}
	cxp := coefficient * rx * yp / ry
	cyp := -coefficient * ry * xp / rx
	cx := cosPhi*cxp - sinPhi*cyp + (float64(from.X)+float64(to.X))/2
	cy := sinPhi*cxp + cosPhi*cyp + (float64(from.Y)+float64(to.Y))/2
	angle := func(ux, uy, vx, vy float64) float64 {
		dot := ux*vx + uy*vy
		length := math.Hypot(ux, uy) * math.Hypot(vx, vy)
		if length == 0 {
			return 0
		}
		value := math.Acos(max(-1, min(1, dot/length)))
		if ux*vy-uy*vx < 0 {
			value = -value
		}
		return value
	}
	ux, uy := (xp-cxp)/rx, (yp-cyp)/ry
	vx, vy := (-xp-cxp)/rx, (-yp-cyp)/ry
	start := angle(1, 0, ux, uy)
	delta := angle(ux, uy, vx, vy)
	if !sweep && delta > 0 {
		delta -= 2 * math.Pi
	} else if sweep && delta < 0 {
		delta += 2 * math.Pi
	}
	count := max(1, int(math.Ceil(math.Abs(delta)/(math.Pi/2))))
	step := delta / float64(count)
	mapPoint := func(x, y float64) f32.Point {
		return f32.Pt(float32(cx+rx*cosPhi*x-ry*sinPhi*y), float32(cy+rx*sinPhi*x+ry*cosPhi*y))
	}
	segments := make([]svgOperation, 0, count)
	for index := 0; index < count; index++ {
		a0 := start + float64(index)*step
		a1 := a0 + step
		factor := 4.0 / 3.0 * math.Tan((a1-a0)/4)
		cos0, sin0 := math.Cos(a0), math.Sin(a0)
		cos1, sin1 := math.Cos(a1), math.Sin(a1)
		control0 := mapPoint(cos0-factor*sin0, sin0+factor*cos0)
		control1 := mapPoint(cos1+factor*sin1, sin1-factor*cos1)
		end := mapPoint(cos1, sin1)
		if index == count-1 {
			end = to
		}
		segments = append(segments, svgOperation{kind: 'C', points: [3]f32.Point{control0, control1, end}})
	}
	return segments
}

func (scanner *svgScanner) space() {
	for scanner.index < len(scanner.data) {
		r := rune(scanner.data[scanner.index])
		if scanner.data[scanner.index] == ',' || unicode.IsSpace(r) {
			scanner.index++
			continue
		}
		break
	}
}

func (scanner *svgScanner) hasNumber() bool {
	scanner.space()
	if scanner.index >= len(scanner.data) {
		return false
	}
	value := scanner.data[scanner.index]
	return value == '+' || value == '-' || value == '.' || value >= '0' && value <= '9'
}

func (scanner *svgScanner) number() (float64, bool) {
	scanner.space()
	start := scanner.index
	if scanner.index < len(scanner.data) && (scanner.data[scanner.index] == '+' || scanner.data[scanner.index] == '-') {
		scanner.index++
	}
	digits := false
	for scanner.index < len(scanner.data) && scanner.data[scanner.index] >= '0' && scanner.data[scanner.index] <= '9' {
		digits = true
		scanner.index++
	}
	if scanner.index < len(scanner.data) && scanner.data[scanner.index] == '.' {
		scanner.index++
		for scanner.index < len(scanner.data) && scanner.data[scanner.index] >= '0' && scanner.data[scanner.index] <= '9' {
			digits = true
			scanner.index++
		}
	}
	if !digits {
		scanner.index = start
		return 0, false
	}
	if scanner.index < len(scanner.data) && (scanner.data[scanner.index] == 'e' || scanner.data[scanner.index] == 'E') {
		exponent := scanner.index
		scanner.index++
		if scanner.index < len(scanner.data) && (scanner.data[scanner.index] == '+' || scanner.data[scanner.index] == '-') {
			scanner.index++
		}
		exponentDigits := scanner.index
		for scanner.index < len(scanner.data) && scanner.data[scanner.index] >= '0' && scanner.data[scanner.index] <= '9' {
			scanner.index++
		}
		if exponentDigits == scanner.index {
			scanner.index = exponent
		}
	}
	value, err := strconv.ParseFloat(strings.TrimSpace(scanner.data[start:scanner.index]), 64)
	return value, err == nil
}

func isSVGCommand(value byte) bool {
	return strings.ContainsRune("MmLlHhVvCcSsQqTtAaZz", rune(value))
}

func svgArgumentCount(command byte) int {
	switch command {
	case 'M', 'L', 'T':
		return 2
	case 'H', 'V':
		return 1
	case 'C':
		return 6
	case 'S', 'Q':
		return 4
	case 'A':
		return 7
	default:
		return 0
	}
}
