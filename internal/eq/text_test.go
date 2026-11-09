package eq

import (
	"reflect"
	"strings"
	"testing"

	"github.com/ryoku-dev/ryotunes/internal/model"
)

func TestParseAutoEQProfile(t *testing.T) {
	profile := `# AutoEQ profile
Preamp: -6.2 dB
Filter 1: ON LSC Fc 105 Hz Gain 4.0 dB Q 0.707
Filter 2: ON PK Fc 31 Hz Gain 4.5 dB Q 1.41
Filter 3: ON PK Fc 86 Hz Gain -2.1 dB Q 0.71
Filter 4: ON PK Fc 220 Hz Gain 3.2 dB Q 1.20
Filter 5: ON PK Fc 540 Hz Gain -1.8 dB Q 2.10
Filter 6: ON PK Fc 1300 Hz Gain 2.4 dB Q 1.10
Filter 7: ON PK Fc 3100 Hz Gain -3.0 dB Q 2.40
Filter 8: ON PK Fc 6100 Hz Gain 2.2 dB Q 1.80
Filter 9: OFF PK Fc 9000 Hz Gain -1.5 dB Q 1.00
Filter 10: ON HSC Fc 12000 Hz Gain -3.0 dB Q 0.707
`
	preamp, bands, warnings, err := Parse(profile)
	if err != nil {
		t.Fatal(err)
	}
	if preamp != -6.2 || len(bands) != 10 || len(warnings) != 0 {
		t.Fatalf("preamp=%g bands=%d warnings=%v", preamp, len(bands), warnings)
	}
	if bands[0].Type != model.EQLowShelf || bands[9].Type != model.EQHighShelf || bands[8].Enabled {
		t.Fatalf("parsed bands = %#v", bands)
	}
}

func TestParseEveryFilterAlias(t *testing.T) {
	aliases := []struct {
		alias string
		typ   model.EQBandType
	}{
		{"PK", model.EQPeak}, {"PEAK", model.EQPeak},
		{"LSC", model.EQLowShelf}, {"LS", model.EQLowShelf},
		{"HSC", model.EQHighShelf}, {"HS", model.EQHighShelf},
		{"LP", model.EQLowPass}, {"LPQ", model.EQLowPass},
		{"HP", model.EQHighPass}, {"HPQ", model.EQHighPass},
		{"NO", model.EQNotch}, {"NOTCH", model.EQNotch},
		{"BP", model.EQBandPass},
	}
	for _, test := range aliases {
		t.Run(test.alias, func(t *testing.T) {
			gain := ""
			if test.typ == model.EQPeak || test.typ == model.EQLowShelf || test.typ == model.EQHighShelf {
				gain = " Gain 2.5 dB"
			}
			text := "Filter 1: ON " + test.alias + " Fc 1000 Hz" + gain + " Q 1.25\n"
			_, bands, warnings, err := Parse(text)
			if err != nil || len(warnings) != 0 || len(bands) != 1 || bands[0].Type != test.typ {
				t.Fatalf("bands=%#v warnings=%v err=%v", bands, warnings, err)
			}
		})
	}
}

func TestParseBandwidthClampAndOverflow(t *testing.T) {
	var text strings.Builder
	text.WriteString("Preamp: -99 dB\n")
	for index := 1; index <= 22; index++ {
		if index == 1 {
			text.WriteString("Filter 1: ON HP Fc 5 Hz Q 0.01\n")
			continue
		}
		if index == 2 {
			text.WriteString("Filter 2: ON LP Fc 25000 Hz BW Oct 1.5\n")
			continue
		}
		text.WriteString("Filter ")
		text.WriteString(number(float64(index)))
		text.WriteString(": ON PK Fc 1000 Hz Gain ")
		text.WriteString(number(float64(index - 10)))
		text.WriteString(" dB Q 1\n")
	}
	preamp, bands, warnings, err := Parse(text.String())
	if err != nil {
		t.Fatal(err)
	}
	if preamp != model.EQMinPreamp || len(bands) != model.EQMaxBands {
		t.Fatalf("preamp=%g bands=%d", preamp, len(bands))
	}
	if bands[0].Type != model.EQHighPass || bands[1].Type != model.EQLowPass {
		t.Fatalf("pass filters were not retained: %#v", bands[:2])
	}
	if len(warnings) < 4 {
		t.Fatalf("warnings = %v", warnings)
	}
}

func TestFormatParseRoundTrip(t *testing.T) {
	original := Normalize(model.Equalizer{Enabled: true, Preamp: -4.75, Bands: []model.EQBand{
		{Type: model.EQPeak, Frequency: 83.25, Gain: 3.125, Q: 1.414, Enabled: true},
		{Type: model.EQLowShelf, Frequency: 120, Gain: -2.25, Q: .707, Enabled: false},
		{Type: model.EQHighShelf, Frequency: 10000, Gain: 1.75, Q: .8, Enabled: true},
		{Type: model.EQLowPass, Frequency: 19000, Q: .65, Enabled: true},
		{Type: model.EQHighPass, Frequency: 25, Q: .9, Enabled: true},
		{Type: model.EQNotch, Frequency: 60, Q: 10, Enabled: true},
		{Type: model.EQBandPass, Frequency: 2400, Q: 2.5, Enabled: false},
	}})
	preamp, bands, warnings, err := Parse(Format(original))
	if err != nil || len(warnings) != 0 {
		t.Fatalf("warnings=%v err=%v", warnings, err)
	}
	if preamp != original.Preamp || !reflect.DeepEqual(bands, original.Bands) {
		t.Fatalf("round trip\n got: %#v %#v\nwant: %#v %#v", preamp, bands, original.Preamp, original.Bands)
	}
}
