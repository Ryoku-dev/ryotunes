// Package eq is the parametric equalizer's shared logic: the response curve
// the client draws, the presets, the Equalizer APO text format and the filter
// chain the daemon hands mpv.
package eq

import (
	"math"
	"math/cmplx"

	"github.com/ryoku-dev/ryotunes/internal/model"
)

// SampleRate is the rate the drawn response assumes. The curve barely moves
// between 44.1 and 48 kHz below 20 kHz, so the client never needs the real one.
const SampleRate = 48000.0

// floorDb keeps a notch's centre finite on the drawn curve.
const floorDb = -120.0

type biquad struct{ b0, b1, b2, a1, a2 float64 }

// coefficients returns the band's RBJ cookbook biquad normalised by a0. These
// are the same forms FFmpeg's equalizer, shelf, pass, bandreject and bandpass
// (csg=0) filters run, so the drawn curve is the curve the listener hears.
func coefficients(band model.EQBand, fs float64) (biquad, bool) {
	if band.Frequency <= 0 || band.Q <= 0 || fs <= 0 || band.Frequency >= fs/2 {
		return biquad{}, false
	}
	w0 := 2 * math.Pi * band.Frequency / fs
	c, s := math.Cos(w0), math.Sin(w0)
	alpha := s / (2 * band.Q)
	a := math.Pow(10, band.Gain/40)
	var b0, b1, b2, a0, a1, a2 float64
	switch band.Type {
	case model.EQPeak:
		b0, b1, b2 = 1+alpha*a, -2*c, 1-alpha*a
		a0, a1, a2 = 1+alpha/a, -2*c, 1-alpha/a
	case model.EQLowShelf:
		beta := 2 * math.Sqrt(a) * alpha
		b0 = a * ((a + 1) - (a-1)*c + beta)
		b1 = 2 * a * ((a - 1) - (a+1)*c)
		b2 = a * ((a + 1) - (a-1)*c - beta)
		a0 = (a + 1) + (a-1)*c + beta
		a1 = -2 * ((a - 1) + (a+1)*c)
		a2 = (a + 1) + (a-1)*c - beta
	case model.EQHighShelf:
		beta := 2 * math.Sqrt(a) * alpha
		b0 = a * ((a + 1) + (a-1)*c + beta)
		b1 = -2 * a * ((a - 1) + (a+1)*c)
		b2 = a * ((a + 1) + (a-1)*c - beta)
		a0 = (a + 1) - (a-1)*c + beta
		a1 = 2 * ((a - 1) - (a+1)*c)
		a2 = (a + 1) - (a-1)*c - beta
	case model.EQLowPass:
		b0, b1, b2 = (1-c)/2, 1-c, (1-c)/2
		a0, a1, a2 = 1+alpha, -2*c, 1-alpha
	case model.EQHighPass:
		b0, b1, b2 = (1+c)/2, -(1 + c), (1+c)/2
		a0, a1, a2 = 1+alpha, -2*c, 1-alpha
	case model.EQNotch:
		b0, b1, b2 = 1, -2*c, 1
		a0, a1, a2 = 1+alpha, -2*c, 1-alpha
	case model.EQBandPass:
		b0, b1, b2 = alpha, 0, -alpha
		a0, a1, a2 = 1+alpha, -2*c, 1-alpha
	default:
		return biquad{}, false
	}
	return biquad{b0 / a0, b1 / a0, b2 / a0, a1 / a0, a2 / a0}, true
}

func (q biquad) gainDb(f, fs float64) float64 {
	w := 2 * math.Pi * f / fs
	z1 := cmplx.Exp(complex(0, -w))
	z2 := z1 * z1
	numerator := complex(q.b0, 0) + complex(q.b1, 0)*z1 + complex(q.b2, 0)*z2
	denominator := 1 + complex(q.a1, 0)*z1 + complex(q.a2, 0)*z2
	magnitude := cmplx.Abs(numerator / denominator)
	if magnitude <= 0 || math.IsNaN(magnitude) {
		return floorDb
	}
	return max(floorDb, 20*math.Log10(magnitude))
}

// BandResponse returns one band's gain in dB at frequency f, enabled or not.
func BandResponse(band model.EQBand, f float64) float64 {
	q, ok := coefficients(band, SampleRate)
	if !ok {
		return 0
	}
	return q.gainDb(f, SampleRate)
}

// Curve writes the summed response in dB of the enabled bands plus the preamp
// at each frequency into dst, growing it when it is short, and returns it.
func Curve(preamp float64, bands []model.EQBand, freqs []float64, dst []float64) []float64 {
	if cap(dst) < len(freqs) {
		dst = make([]float64, len(freqs))
	}
	dst = dst[:len(freqs)]
	for i := range dst {
		dst[i] = preamp
	}
	for _, band := range bands {
		if !band.Enabled {
			continue
		}
		q, ok := coefficients(band, SampleRate)
		if !ok {
			continue
		}
		for i, f := range freqs {
			dst[i] += q.gainDb(f, SampleRate)
		}
	}
	return dst
}

// LogFrequencies returns n frequencies spaced evenly on a log axis from
// model.EQMinFrequency to model.EQMaxFrequency.
func LogFrequencies(n int) []float64 {
	if n <= 0 {
		return nil
	}
	if n == 1 {
		return []float64{model.EQMinFrequency}
	}
	span := math.Log(model.EQMaxFrequency / model.EQMinFrequency)
	freqs := make([]float64, n)
	for i := range freqs {
		freqs[i] = model.EQMinFrequency * math.Exp(span*float64(i)/float64(n-1))
	}
	return freqs
}
