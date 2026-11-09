package eq

import (
	"strings"
	"testing"

	"github.com/ryoku-dev/ryotunes/internal/model"
)

func TestParseUnnumberedAPOWithBOM(t *testing.T) {
	text := "\ufeffPreamp: -6.4 dB\r\nFilter: ON PEQ Fc 1000 Hz Gain 3 dB Q 1.2\r\nFilter: ON Modal Fc 2500 Hz Gain -2 dB Q 2\r\n"
	preamp, bands, warnings, err := Parse(text)
	if err != nil {
		t.Fatal(err)
	}
	if preamp != -6.4 || len(bands) != 2 || len(warnings) != 1 || !strings.Contains(warnings[0], "Modal") {
		t.Fatalf("preamp=%g bands=%#v warnings=%v", preamp, bands, warnings)
	}
	for _, band := range bands {
		if band.Type != model.EQPeak {
			t.Fatalf("type = %s", band.Type)
		}
	}
}

func TestParseFirstAPOChannelOnly(t *testing.T) {
	text := `Channel: L
Filter: ON PK Fc 1000 Hz Gain 4 dB Q 1
Channel: R
Filter: ON PK Fc 1000 Hz Gain 4 dB Q 1
`
	_, bands, warnings, err := Parse(text)
	if err != nil {
		t.Fatal(err)
	}
	if len(bands) != 1 || bands[0].Gain != 4 {
		t.Fatalf("bands = %#v", bands)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "Only channel L") {
		t.Fatalf("warnings = %v", warnings)
	}
}

func TestParseShelfSlopeVariants(t *testing.T) {
	text := `Filter: ON LSC 6 dB Fc 80 Hz Gain 4 dB
Filter: ON HSC 12dB Fc 12000 Hz Gain -2 dB
Filter: ON LS 6dB Fc 100 Hz Gain 3 dB
Filter: ON HS 12 dB Fc 10000 Hz Gain 2 dB
`
	_, bands, warnings, err := Parse(text)
	if err != nil {
		t.Fatal(err)
	}
	if len(bands) != 4 || len(warnings) != 4 {
		t.Fatalf("bands=%#v warnings=%v", bands, warnings)
	}
	want := []model.EQBandType{model.EQLowShelf, model.EQHighShelf, model.EQLowShelf, model.EQHighShelf}
	for index := range bands {
		if bands[index].Type != want[index] || bands[index].Q != .707 {
			t.Fatalf("band %d = %#v", index, bands[index])
		}
	}
}

func TestParseRefusesEmptyOrUnsupportedProfile(t *testing.T) {
	for name, text := range map[string]string{
		"empty":       "Preamp: -6 dB\n",
		"unsupported": "Preamp: -6 dB\nFilter: ON IIR Order 2 Coefficients 1 0 0 1 0 0\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, bands, _, err := Parse(text); err == nil || len(bands) != 0 {
				t.Fatalf("bands=%#v err=%v", bands, err)
			}
		})
	}
}

func TestSelectBandsPrefersEnabledFilters(t *testing.T) {
	bands := make([]model.EQBand, 0, model.EQMaxBands+1)
	bands = append(bands, model.EQBand{Type: model.EQHighPass, Frequency: 30, Q: .707})
	for index := range model.EQMaxBands {
		bands = append(bands, model.EQBand{
			Type: model.EQPeak, Frequency: float64(100 + index*100), Gain: .1, Q: 1, Enabled: true,
		})
	}
	selected := selectBands(bands)
	if len(selected) != model.EQMaxBands {
		t.Fatalf("selected = %d", len(selected))
	}
	for _, band := range selected {
		if !band.Enabled {
			t.Fatalf("disabled band was retained: %#v", band)
		}
	}
}
