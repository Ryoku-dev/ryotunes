package player

import (
	"fmt"
	"math"
	"strings"

	"github.com/ryoku-dev/ryotunes/internal/model"
)

func validateFx(fx model.AudioFx) error {
	switch {
	case !finiteInRange(fx.Speed, 0.25, 2):
		return fmt.Errorf("Tempo must be between 0.25× and 2.00×.")
	case fx.Semitones < -12 || fx.Semitones > 12:
		return fmt.Errorf("Pitch must be between -12 and +12 semitones.")
	case !finiteInRange(fx.Reverb, 0, 1):
		return fmt.Errorf("Reverb must be between 0 and 1.")
	case !finiteInRange(fx.Bass, -6, 12):
		return fmt.Errorf("Bass must be between -6 and +12 dB.")
	case !finiteInRange(fx.Width, 0, 1):
		return fmt.Errorf("Width must be between 0 and 1.")
	default:
		return nil
	}
}

func finiteInRange(value, low, high float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= low && value <= high
}

func loudnessGain(loudness *float64) *float64 {
	if loudness == nil || math.IsNaN(*loudness) || math.IsInf(*loudness, 0) {
		return nil
	}
	gain := -7 - (*loudness - 14)
	if gain >= -0.05 {
		return nil
	}
	gain = max(gain, -24)
	return &gain
}

func afChain(gain *float64, fx model.AudioFx) string {
	chain := make([]string, 0, 5)
	if gain != nil {
		chain = append(chain, "lavfi=[volume="+formatFloat(*gain)+"dB]")
	}
	if fx.Semitones != 0 {
		pitch := math.Pow(2, float64(fx.Semitones)/12)
		chain = append(chain, "rubberband=pitch-scale="+formatFloat(pitch))
	}
	if fx.Reverb > 0 {
		chain = append(chain, "lavfi=[aecho=0.8:0.9:40|80|120:"+
			formatFloat(0.4*fx.Reverb)+"|"+
			formatFloat(0.3*fx.Reverb)+"|"+
			formatFloat(0.2*fx.Reverb)+"]")
	}
	if fx.Bass != 0 {
		chain = append(chain, "lavfi=[bass=g="+formatFloat(fx.Bass)+":f=110:w=0.6]")
	}
	if fx.Width > 0 {
		chain = append(chain, "lavfi=[stereowiden=delay=20:feedback=0.3:crossfeed=0.3:drymix="+
			formatFloat(1-0.6*fx.Width)+"]")
	}
	return strings.Join(chain, ",")
}

func perceptualVolume(percent int) float64 {
	if percent <= 0 {
		return 0
	}
	fraction := float64(min(percent, 100)) / 100
	return 100 * math.Pow(10, -math.Pow(1-fraction, 1.5))
}
