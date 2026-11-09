package eq

import (
	"bufio"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/ryoku-dev/ryotunes/internal/model"
)

var (
	preampLine  = regexp.MustCompile(`(?i)^preamp\s*:\s*([^\s]+)\s*dB(?:\s.*)?$`)
	filterLine  = regexp.MustCompile(`(?i)^filter(?:\s*\d+)?\s*:\s*(on|off)\s+([^\s]+)\s+(.*)$`)
	channelLine = regexp.MustCompile(`(?i)^channel\s*:\s*(.+)$`)
)

// Parse reads Equalizer APO and AutoEQ ParametricEQ text.
func Parse(text string) (preamp float64, bands []model.EQBand, warnings []string, err error) {
	text = strings.TrimPrefix(text, "\ufeff")
	scanner := bufio.NewScanner(strings.NewReader(text))
	lineNumber := 0
	sawFilter := false
	firstChannel := ""
	skipChannel := false
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if mark := strings.IndexByte(line, '#'); mark >= 0 {
			line = strings.TrimSpace(line[:mark])
		}
		if line == "" || strings.HasPrefix(line, ";") {
			continue
		}
		if match := preampLine.FindStringSubmatch(line); match != nil {
			value, parseErr := strconv.ParseFloat(match[1], 64)
			if parseErr != nil || math.IsNaN(value) || math.IsInf(value, 0) {
				warnings = append(warnings, fmt.Sprintf("Line %d has an invalid preamp; using 0 dB.", lineNumber))
				preamp = 0
				continue
			}
			preamp, warnings = clamped(value, model.EQMinPreamp, model.EQMaxPreamp, "preamp", lineNumber, warnings)
			continue
		}
		if match := channelLine.FindStringSubmatch(line); match != nil {
			channel := strings.TrimSpace(match[1])
			if strings.EqualFold(channel, "all") {
				skipChannel = false
				continue
			}
			if firstChannel == "" {
				firstChannel = channel
				skipChannel = false
			} else {
				skipChannel = !strings.EqualFold(channel, firstChannel)
				if skipChannel {
					warnings = append(warnings, fmt.Sprintf("Only channel %s was imported; channel %s was skipped.", firstChannel, channel))
				}
			}
			continue
		}
		if strings.HasPrefix(strings.ToLower(line), "filter") {
			sawFilter = true
		}
		match := filterLine.FindStringSubmatch(line)
		if match == nil {
			warnings = append(warnings, fmt.Sprintf("Line %d was not recognised and was skipped.", lineNumber))
			continue
		}
		if skipChannel {
			continue
		}
		typ, ok := parseBandType(match[2])
		if !ok {
			warnings = append(warnings, fmt.Sprintf("Line %d uses unsupported filter type %s and was skipped.", lineNumber, match[2]))
			continue
		}
		if strings.EqualFold(match[2], "Modal") {
			warnings = append(warnings, fmt.Sprintf("Line %d Modal filter was approximated with a peak filter.", lineNumber))
		}
		fields, slopeWarning := shelfFields(strings.Fields(match[3]), typ, lineNumber)
		if slopeWarning != "" {
			warnings = append(warnings, slopeWarning)
		}
		parsed, lineWarnings, ok := parseFilterFields(fields, typ, strings.EqualFold(match[1], "on"), lineNumber)
		warnings = append(warnings, lineWarnings...)
		if !ok {
			continue
		}
		bands = append(bands, parsed)
	}
	if scanErr := scanner.Err(); scanErr != nil {
		return 0, nil, warnings, fmt.Errorf("read equalizer profile: %w", scanErr)
	}
	if len(bands) == 0 {
		if sawFilter {
			return 0, nil, warnings, fmt.Errorf("No supported Equalizer APO filters were found.")
		}
		return 0, nil, warnings, fmt.Errorf("The profile contains no filters.")
	}
	if len(bands) > model.EQMaxBands {
		original := len(bands)
		bands = selectBands(bands)
		warnings = append(warnings, fmt.Sprintf("The profile has %d filters; kept the 20 most significant filters.", original))
	}
	return preamp, bands, warnings, nil
}

func parseFilterFields(fields []string, typ model.EQBandType, enabled bool, line int) (model.EQBand, []string, bool) {
	warnings := []string{}
	band := model.EQBand{Type: typ, Enabled: enabled, Q: defaultQ(typ)}
	if len(fields) < 3 || !strings.EqualFold(fields[0], "Fc") || !strings.EqualFold(fields[2], "Hz") {
		return band, append(warnings, fmt.Sprintf("Line %d has no valid Fc value and was skipped.", line)), false
	}
	frequency, err := strconv.ParseFloat(fields[1], 64)
	if err != nil || math.IsNaN(frequency) || math.IsInf(frequency, 0) {
		return band, append(warnings, fmt.Sprintf("Line %d has an invalid frequency and was skipped.", line)), false
	}
	band.Frequency, warnings = clamped(frequency, model.EQMinFrequency, model.EQMaxFrequency, "frequency", line, warnings)

	for index := 3; index < len(fields); {
		switch {
		case strings.EqualFold(fields[index], "Gain"):
			if index+2 >= len(fields) || !strings.EqualFold(fields[index+2], "dB") {
				warnings = append(warnings, fmt.Sprintf("Line %d has an invalid Gain value; using 0 dB.", line))
				index = len(fields)
				continue
			}
			gain, parseErr := strconv.ParseFloat(fields[index+1], 64)
			if parseErr != nil || math.IsNaN(gain) || math.IsInf(gain, 0) {
				warnings = append(warnings, fmt.Sprintf("Line %d has an invalid Gain value; using 0 dB.", line))
				index += 3
				continue
			}
			band.Gain, warnings = clamped(gain, model.EQMinGain, model.EQMaxGain, "gain", line, warnings)
			index += 3
		case strings.EqualFold(fields[index], "Q"):
			if index+1 >= len(fields) {
				warnings = append(warnings, fmt.Sprintf("Line %d has no Q value; using %s.", line, number(band.Q)))
				index++
				continue
			}
			q, parseErr := strconv.ParseFloat(fields[index+1], 64)
			if parseErr != nil || math.IsNaN(q) || math.IsInf(q, 0) {
				warnings = append(warnings, fmt.Sprintf("Line %d has an invalid Q value; using %s.", line, number(band.Q)))
				index += 2
				continue
			}
			band.Q, warnings = clamped(q, model.EQMinQ, model.EQMaxQ, "Q", line, warnings)
			index += 2
		case strings.EqualFold(fields[index], "BW"):
			valueIndex, next := index+1, index+2
			if valueIndex < len(fields) && strings.EqualFold(fields[valueIndex], "Oct") {
				valueIndex, next = index+2, index+3
			}
			if valueIndex >= len(fields) {
				warnings = append(warnings, fmt.Sprintf("Line %d has no bandwidth value; using Q %s.", line, number(band.Q)))
				index = len(fields)
				continue
			}
			bw, parseErr := strconv.ParseFloat(fields[valueIndex], 64)
			if parseErr != nil || math.IsNaN(bw) || math.IsInf(bw, 0) || bw <= 0 {
				warnings = append(warnings, fmt.Sprintf("Line %d has an invalid bandwidth; using Q %s.", line, number(band.Q)))
				index = next
				continue
			}
			if next < len(fields) && strings.EqualFold(fields[next], "Oct") {
				next++
			}
			q := bandwidthQ(band.Frequency, bw)
			band.Q, warnings = clamped(q, model.EQMinQ, model.EQMaxQ, "converted Q", line, warnings)
			index = next
		default:
			warnings = append(warnings, fmt.Sprintf("Line %d has unsupported trailing fields: %s.", line, strings.Join(fields[index:], " ")))
			index = len(fields)
		}
	}
	return band, warnings, true
}

func shelfFields(fields []string, typ model.EQBandType, line int) ([]string, string) {
	if typ != model.EQLowShelf && typ != model.EQHighShelf || len(fields) == 0 {
		return fields, ""
	}
	slopeText := strings.TrimSuffix(strings.ToLower(fields[0]), "db")
	consumed := 1
	if slopeText == fields[0] {
		if len(fields) < 2 || !strings.EqualFold(fields[1], "dB") {
			return fields, ""
		}
		consumed = 2
	}
	slope, err := strconv.ParseFloat(slopeText, 64)
	if err != nil || math.IsNaN(slope) || math.IsInf(slope, 0) || slope <= 0 {
		return fields, ""
	}
	return fields[consumed:], fmt.Sprintf("Line %d shelf slope %s dB was approximated with Q 0.707.", line, number(slope))
}

func bandwidthQ(frequency, octaves float64) float64 {
	w0 := 2 * math.Pi * frequency / SampleRate
	sine := math.Sin(w0)
	if math.Abs(sine) < 1e-12 {
		return 1
	}
	return 1 / (2 * math.Sinh(math.Ln2/2*octaves*w0/sine))
}

func clamped(value, low, high float64, field string, line int, warnings []string) (float64, []string) {
	clampedValue := clamp(value, low, high)
	if clampedValue != value {
		warnings = append(warnings, fmt.Sprintf("Line %d %s was clamped to %s.", line, field, number(clampedValue)))
	}
	return clampedValue, warnings
}

func defaultQ(typ model.EQBandType) float64 {
	if typ == model.EQPeak {
		return 1
	}
	return .707
}

func parseBandType(value string) (model.EQBandType, bool) {
	switch strings.ToUpper(value) {
	case "PK", "PEAK", "PEQ", "MODAL":
		return model.EQPeak, true
	case "LS", "LSC":
		return model.EQLowShelf, true
	case "HS", "HSC":
		return model.EQHighShelf, true
	case "LP", "LPQ":
		return model.EQLowPass, true
	case "HP", "HPQ":
		return model.EQHighPass, true
	case "NO", "NOTCH":
		return model.EQNotch, true
	case "BP":
		return model.EQBandPass, true
	default:
		return "", false
	}
}

// Format writes a stable Equalizer APO profile that Parse can read without loss.
func Format(equalizer model.Equalizer) string {
	equalizer = Normalize(equalizer)
	var out strings.Builder
	out.WriteString("# Ryotunes Parametric EQ export\n")
	fmt.Fprintf(&out, "Preamp: %s dB\n", number(equalizer.Preamp))
	for index, band := range equalizer.Bands {
		state := "OFF"
		if band.Enabled {
			state = "ON"
		}
		fmt.Fprintf(&out, "Filter %d: %s %s Fc %s Hz", index+1, state, formatBandType(band.Type), number(band.Frequency))
		if band.HasGain() {
			fmt.Fprintf(&out, " Gain %s dB", number(band.Gain))
		}
		fmt.Fprintf(&out, " Q %s\n", number(band.Q))
	}
	return out.String()
}

func formatBandType(typ model.EQBandType) string {
	switch typ {
	case model.EQLowShelf:
		return "LSC"
	case model.EQHighShelf:
		return "HSC"
	case model.EQLowPass:
		return "LP"
	case model.EQHighPass:
		return "HP"
	case model.EQNotch:
		return "NO"
	case model.EQBandPass:
		return "BP"
	default:
		return "PK"
	}
}

func number(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func selectBands(bands []model.EQBand) []model.EQBand {
	if len(bands) <= model.EQMaxBands {
		return bands
	}
	type ranked struct {
		index    int
		priority int
		gain     float64
	}
	ranks := make([]ranked, len(bands))
	for index, band := range bands {
		priority := 0
		if band.Enabled {
			priority = 2
		}
		if !band.HasGain() {
			priority++
		}
		ranks[index] = ranked{index: index, priority: priority, gain: math.Abs(band.Gain)}
	}
	sort.SliceStable(ranks, func(i, j int) bool {
		if ranks[i].priority != ranks[j].priority {
			return ranks[i].priority > ranks[j].priority
		}
		return ranks[i].gain > ranks[j].gain
	})
	kept := ranks[:model.EQMaxBands]
	sort.Slice(kept, func(i, j int) bool { return kept[i].index < kept[j].index })
	out := make([]model.EQBand, len(kept))
	for index, rank := range kept {
		out[index] = bands[rank.index]
	}
	return out
}
