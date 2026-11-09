package look

import (
	"image/color"
	"math"
)

type oklch struct {
	L, C, H float64
	A       uint8
}

func colorToOKLCH(value color.NRGBA) oklch {
	r := srgbToLinear(float64(value.R) / 255)
	g := srgbToLinear(float64(value.G) / 255)
	b := srgbToLinear(float64(value.B) / 255)

	l := math.Cbrt(.4122214708*r + .5363325363*g + .0514459929*b)
	m := math.Cbrt(.2119034982*r + .6806995451*g + .1073969566*b)
	s := math.Cbrt(.0883024619*r + .2817188376*g + .6299787005*b)
	a := 1.9779984951*l - 2.4285922050*m + .4505937099*s
	bb := .0259040371*l + .7827717662*m - .8086757660*s
	h := math.Atan2(bb, a)
	if h < 0 {
		h += 2 * math.Pi
	}
	return oklch{
		L: .2104542553*l + .7936177850*m - .0040720468*s,
		C: math.Hypot(a, bb),
		H: h,
		A: value.A,
	}
}

func oklchToColor(value oklch) color.NRGBA {
	value.L = min(max(value.L, 0), 1)
	value.C = max(value.C, 0)
	if rgb, ok := oklchLinearRGB(value); ok {
		return linearRGBToColor(rgb, value.A)
	}
	low, high := 0.0, value.C
	rgb, _ := oklchLinearRGB(oklch{L: value.L, H: value.H, A: value.A})
	for range 24 {
		mid := (low + high) / 2
		candidate := value
		candidate.C = mid
		if next, ok := oklchLinearRGB(candidate); ok {
			low, rgb = mid, next
		} else {
			high = mid
		}
	}
	return linearRGBToColor(rgb, value.A)
}

func oklchLinearRGB(value oklch) ([3]float64, bool) {
	a := value.C * math.Cos(value.H)
	b := value.C * math.Sin(value.H)
	l := value.L + .3963377774*a + .2158037573*b
	m := value.L - .1055613458*a - .0638541728*b
	s := value.L - .0894841775*a - 1.2914855480*b
	l, m, s = l*l*l, m*m*m, s*s*s
	rgb := [3]float64{
		4.0767416621*l - 3.3077115913*m + .2309699292*s,
		-1.2684380046*l + 2.6097574011*m - .3413193965*s,
		-.0041960863*l - .7034186147*m + 1.7076147010*s,
	}
	const tolerance = 1e-7
	return rgb, rgb[0] >= -tolerance && rgb[0] <= 1+tolerance &&
		rgb[1] >= -tolerance && rgb[1] <= 1+tolerance &&
		rgb[2] >= -tolerance && rgb[2] <= 1+tolerance
}

func linearRGBToColor(rgb [3]float64, alpha uint8) color.NRGBA {
	channel := func(value float64) uint8 {
		value = linearToSRGB(min(max(value, 0), 1))
		return uint8(min(max(math.Round(value*255), 0), 255))
	}
	return color.NRGBA{R: channel(rgb[0]), G: channel(rgb[1]), B: channel(rgb[2]), A: alpha}
}

func srgbToLinear(value float64) float64 {
	if value <= .04045 {
		return value / 12.92
	}
	return math.Pow((value+.055)/1.055, 2.4)
}

func linearToSRGB(value float64) float64 {
	if value <= .0031308 {
		return 12.92 * value
	}
	return 1.055*math.Pow(value, 1.0/2.4) - .055
}
