package eq

import (
	"fmt"
	"strings"

	"github.com/ryoku-dev/ryotunes/internal/model"
)

const preampLabel = "rteq00"

// Chain returns the labelled mpv audio filters for an equalizer.
func Chain(equalizer model.Equalizer) string {
	equalizer = Normalize(equalizer)
	if !equalizer.Enabled {
		return ""
	}
	filters := make([]string, 0, len(equalizer.Bands)+1)
	filters = append(filters, "@"+preampLabel+":lavfi=[volume=volume="+number(equalizer.Preamp)+"dB]")
	for index, band := range equalizer.Bands {
		if !band.Enabled {
			continue
		}
		filters = append(filters, "@"+bandLabel(index)+":lavfi=["+bandFilter(band)+"]")
	}
	return strings.Join(filters, ",")
}

func bandFilter(band model.EQBand) string {
	frequency, gain, q := number(band.Frequency), number(band.Gain), number(band.Q)
	switch band.Type {
	case model.EQLowShelf:
		return "lowshelf=f=" + frequency + ":t=q:w=" + q + ":g=" + gain + ":p=2"
	case model.EQHighShelf:
		return "highshelf=f=" + frequency + ":t=q:w=" + q + ":g=" + gain + ":p=2"
	case model.EQLowPass:
		return "lowpass=f=" + frequency + ":t=q:w=" + q + ":p=2"
	case model.EQHighPass:
		return "highpass=f=" + frequency + ":t=q:w=" + q + ":p=2"
	case model.EQNotch:
		return "bandreject=f=" + frequency + ":t=q:w=" + q
	case model.EQBandPass:
		return "bandpass=f=" + frequency + ":t=q:w=" + q + ":csg=0"
	default:
		return "equalizer=f=" + frequency + ":t=q:w=" + q + ":g=" + gain
	}
}

func bandLabel(index int) string {
	return fmt.Sprintf("rteq%02d", index+1)
}
