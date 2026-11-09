package look

import (
	"math"
	"time"
)

const (
	beatAverageTime = 1200 * time.Millisecond
	beatRefractory  = 160 * time.Millisecond
	kickDecayTime   = 180 * time.Millisecond
)

func shapeVisualizerLevel(level float32) float32 {
	level = min(max(level, 0), 1)
	if level <= .03 {
		return 0
	}
	return float32(math.Pow(float64((level-.03)/.97), 1.35))
}

func spectrumBassEnergy(levels []float32) float32 {
	if len(levels) == 0 {
		return 0
	}
	count := max(1, len(levels)/8)
	energy := float32(0)
	for _, level := range levels[:count] {
		energy += min(max(level, 0), 1)
	}
	return energy / float32(count)
}

func scopeRMSEnergy(samples []float32) float32 {
	if len(samples) == 0 {
		return 0
	}
	squares := float64(0)
	for _, sample := range samples {
		squares += float64(sample) * float64(sample)
	}
	return min(1, float32(math.Sqrt(squares/float64(len(samples)))))
}

type beatDetector struct {
	average  float32
	kick     float32
	last     time.Time
	lastBeat time.Time
	armed    bool
	ready    bool
}

func (detector *beatDetector) update(now time.Time, energy float32, live bool) float32 {
	energy = min(max(energy, 0), 1)
	if !live {
		energy = 0
	}
	if !detector.ready {
		detector.average = energy
		detector.last = now
		detector.armed = true
		detector.ready = true
		return 0
	}

	delta := now.Sub(detector.last)
	if delta < 0 {
		delta = 0
	}
	detector.last = now
	if detector.kick > 0 && delta > 0 {
		detector.kick *= float32(math.Exp(-float64(delta) / float64(kickDecayTime)))
		if detector.kick < .004 {
			detector.kick = 0
		}
	}

	threshold := energy > .25 && energy > detector.average*1.35
	if !threshold {
		detector.armed = true
	} else if detector.armed {
		detector.armed = false
		if detector.lastBeat.IsZero() || now.Sub(detector.lastBeat) >= beatRefractory {
			detector.kick = 1
			detector.lastBeat = now
		}
	}

	if delta > 0 {
		weight := 1 - float32(math.Exp(-float64(delta)/float64(beatAverageTime)))
		detector.average += (energy - detector.average) * weight
	}
	return detector.kick
}
