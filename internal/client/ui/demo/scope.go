package demo

import "math"

const (
	demoScopeRate   = 48000
	demoScopeFrames = 8192
)

// ScopeSamples returns a short chord with a centred body and stereo side voice.
func ScopeSamples() []float32 {
	samples := make([]float32, demoScopeFrames*2)
	for frame := range demoScopeFrames {
		t := float64(frame) / demoScopeRate
		centre := .30*math.Sin(2*math.Pi*220*t) +
			.20*math.Sin(2*math.Pi*330*t) +
			.13*math.Sin(2*math.Pi*440*t)
		side := .13*math.Sin(2*math.Pi*277*t+.35) +
			.08*math.Sin(2*math.Pi*554*t+1.1)
		samples[frame*2] = float32(centre + side)
		samples[frame*2+1] = float32(centre - side*.82)
	}
	return samples
}
