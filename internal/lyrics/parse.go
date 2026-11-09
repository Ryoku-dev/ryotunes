package lyrics

import (
	"encoding/json"
	"encoding/xml"
	"sort"
	"strconv"
	"strings"

	"github.com/ryoku-dev/ryotunes/internal/model"
)

func parseLRC(text string) []model.LyricsLine {
	var lines []model.LyricsLine
	var offset int64
	for _, raw := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		rest := strings.TrimSpace(raw)
		if value, ok := metadataValue(rest, "offset"); ok {
			if parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64); err == nil {
				offset = parsed
			}
			continue
		}

		var stamps []lineStamp
		for strings.HasPrefix(rest, "[") {
			end := strings.IndexByte(rest, ']')
			if end < 0 {
				break
			}
			stamp, ok := parseLineStamp(rest[1:end])
			if !ok {
				break
			}
			stamps = append(stamps, stamp)
			rest = strings.TrimLeft(rest[end+1:], " \t")
		}
		for _, stamp := range stamps {
			lines = append(lines, parseEnhancedLine(rest, stamp))
		}
	}
	if offset != 0 {
		for i := range lines {
			shiftLine(&lines[i], offset)
		}
	}
	sort.SliceStable(lines, func(i, j int) bool {
		if lines[i].TimeMS == nil {
			return false
		}
		if lines[j].TimeMS == nil {
			return true
		}
		return *lines[i].TimeMS < *lines[j].TimeMS
	})
	return lines
}

type lineStamp struct {
	start uint64
	end   *uint64
}

func parseLineStamp(tag string) (lineStamp, bool) {
	if ms, ok := parseLRCTime(tag); ok {
		return lineStamp{start: ms}, true
	}
	parts := strings.Split(tag, ",")
	if len(parts) < 2 {
		return lineStamp{}, false
	}
	start, errStart := strconv.ParseUint(strings.TrimSpace(parts[0]), 10, 64)
	duration, errDuration := strconv.ParseUint(strings.TrimSpace(parts[1]), 10, 64)
	if errStart != nil || errDuration != nil {
		return lineStamp{}, false
	}
	end := start + duration
	return lineStamp{start: start, end: &end}, true
}

func parseLRCTime(tag string) (uint64, bool) {
	minuteText, rest, ok := strings.Cut(strings.TrimSpace(tag), ":")
	if !ok {
		return 0, false
	}
	minutes, err := strconv.ParseUint(strings.TrimSpace(minuteText), 10, 64)
	if err != nil {
		return 0, false
	}
	secondText := rest
	fraction := ""
	hadFraction := false
	if seconds, frac, found := strings.Cut(rest, "."); found {
		secondText, fraction, hadFraction = seconds, frac, true
	}
	seconds, err := strconv.ParseUint(strings.TrimSpace(secondText), 10, 64)
	if err != nil {
		return 0, false
	}
	var digits strings.Builder
	for _, r := range fraction {
		if r >= '0' && r <= '9' && digits.Len() < 3 {
			digits.WriteRune(r)
		}
	}
	if hadFraction && digits.Len() == 0 {
		return 0, false
	}
	millis := uint64(0)
	if digits.Len() > 0 {
		millis, err = strconv.ParseUint(digits.String(), 10, 64)
		if err != nil {
			return 0, false
		}
		switch digits.Len() {
		case 1:
			millis *= 100
		case 2:
			millis *= 10
		}
	}
	return (minutes*60+seconds)*1000 + millis, true
}

func parseEnhancedLine(raw string, stamp lineStamp) model.LyricsLine {
	start := stamp.start
	line := model.LyricsLine{TimeMS: &start, EndTimeMS: stamp.end, Text: raw}
	if words, text := parseParenthesizedWords(raw, stamp); len(words) != 0 {
		line.Words = words
		line.Text = text
		return line
	}
	if words, text := parseAngleWords(raw, stamp); len(words) != 0 {
		line.Words = words
		line.Text = text
	}
	return line
}

func parseParenthesizedWords(raw string, stamp lineStamp) ([]model.LyricsWord, string) {
	var words []model.LyricsWord
	var text strings.Builder
	for pos := 0; pos < len(raw); {
		if raw[pos] != '(' {
			r, size := nextRune(raw[pos:])
			text.WriteRune(r)
			pos += size
			continue
		}
		close := strings.IndexByte(raw[pos:], ')')
		if close < 0 {
			text.WriteByte(raw[pos])
			pos++
			continue
		}
		parts := strings.Split(raw[pos+1:pos+close], ",")
		if len(parts) < 2 {
			text.WriteString(raw[pos : pos+close+1])
			pos += close + 1
			continue
		}
		offset, errOffset := strconv.ParseUint(strings.TrimSpace(parts[0]), 10, 64)
		duration, errDuration := strconv.ParseUint(strings.TrimSpace(parts[1]), 10, 64)
		if errOffset != nil || errDuration != nil {
			text.WriteString(raw[pos : pos+close+1])
			pos += close + 1
			continue
		}
		wordStart := pos + close + 1
		next := strings.IndexByte(raw[wordStart:], '(')
		wordEnd := len(raw)
		if next >= 0 {
			wordEnd = wordStart + next
		}
		word := raw[wordStart:wordEnd]
		text.WriteString(word)
		words = append(words, model.LyricsWord{Text: word, StartMS: stamp.start + offset, EndMS: stamp.start + offset + duration})
		pos = wordEnd
	}
	return words, strings.TrimSpace(text.String())
}

func parseAngleWords(raw string, stamp lineStamp) ([]model.LyricsWord, string) {
	var words []model.LyricsWord
	var plain strings.Builder
	start := stamp.start
	for pos := 0; pos < len(raw); {
		open := strings.IndexByte(raw[pos:], '<')
		if open < 0 {
			tail := raw[pos:]
			plain.WriteString(tail)
			if strings.TrimSpace(tail) != "" {
				end := start
				if stamp.end != nil && *stamp.end > start {
					end = *stamp.end
				}
				words = append(words, model.LyricsWord{Text: tail, StartMS: start, EndMS: end})
			}
			break
		}
		open += pos
		close := strings.IndexByte(raw[open:], '>')
		if close < 0 {
			tail := raw[pos:]
			plain.WriteString(tail)
			if strings.TrimSpace(tail) != "" {
				end := start
				if stamp.end != nil && *stamp.end > start {
					end = *stamp.end
				}
				words = append(words, model.LyricsWord{Text: tail, StartMS: start, EndMS: end})
			}
			break
		}
		close += open
		boundary, ok := parseLRCTime(raw[open+1 : close])
		if !ok {
			plain.WriteString(raw[pos : close+1])
			pos = close + 1
			continue
		}
		text := raw[pos:open]
		plain.WriteString(text)
		if strings.TrimSpace(text) != "" {
			words = append(words, model.LyricsWord{Text: text, StartMS: start, EndMS: boundary})
		}
		start = boundary
		pos = close + 1
	}
	if len(words) == 0 {
		return nil, raw
	}
	return words, strings.TrimSpace(plain.String())
}

func metadataValue(line, key string) (string, bool) {
	if len(line) < 3 || line[0] != '[' || line[len(line)-1] != ']' {
		return "", false
	}
	name, value, ok := strings.Cut(line[1:len(line)-1], ":")
	return value, ok && strings.EqualFold(strings.TrimSpace(name), key)
}

func shiftLine(line *model.LyricsLine, offset int64) {
	line.TimeMS = shifted(line.TimeMS, offset)
	line.EndTimeMS = shifted(line.EndTimeMS, offset)
	for i := range line.Words {
		line.Words[i].StartMS = shiftMillis(line.Words[i].StartMS, offset)
		line.Words[i].EndMS = shiftMillis(line.Words[i].EndMS, offset)
	}
}

func shifted(value *uint64, offset int64) *uint64 {
	if value == nil {
		return nil
	}
	result := shiftMillis(*value, offset)
	return &result
}

func shiftMillis(value uint64, offset int64) uint64 {
	if offset >= 0 {
		return value + uint64(offset)
	}
	magnitude := uint64(-offset)
	if magnitude >= value {
		return 0
	}
	return value - magnitude
}

func parseProviderText(text string) []model.LyricsLine {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil
	}
	if (strings.HasPrefix(trimmed, "[") || strings.HasPrefix(trimmed, "{")) && jsonLooksTimed(trimmed) {
		if lines := parseJSONLyrics([]byte(trimmed)); len(lines) != 0 {
			return lines
		}
	}
	if strings.HasPrefix(trimmed, "<") {
		if lines := parseTTML(trimmed); len(lines) != 0 {
			return lines
		}
	}
	return parseLRC(text)
}

func jsonLooksTimed(text string) bool {
	return strings.Contains(text, `"text"`) || strings.Contains(text, `"time"`) ||
		strings.Contains(text, `"words"`) || strings.Contains(text, `"start"`)
}

func parseJSONLyrics(data []byte) []model.LyricsLine {
	var root any
	if json.Unmarshal(data, &root) != nil {
		return nil
	}
	items := jsonItems(root)
	lines := make([]model.LyricsLine, 0, len(items))
	for _, item := range items {
		text := stringField(item, "text", "line")
		if text == "" {
			if words, ok := item["words"].(string); ok {
				text = words
			}
		}
		start, hasStart := timeField(item, "time", "startTime", "start", "t")
		end, hasEnd := timeField(item, "endTime", "end")
		words := parseJSONWords(item["words"], start, hasStart)
		if text == "" && len(words) != 0 {
			var b strings.Builder
			for _, word := range words {
				b.WriteString(word.Text)
			}
			text = b.String()
		}
		if text == "" && !hasStart {
			continue
		}
		line := model.LyricsLine{Text: strings.TrimSpace(text), Words: words}
		if hasStart {
			line.TimeMS = &start
		}
		if hasEnd {
			line.EndTimeMS = &end
		}
		lines = append(lines, line)
	}
	sortLines(lines)
	return lines
}

func jsonItems(root any) []map[string]any {
	if list, ok := root.([]any); ok {
		return objectList(list)
	}
	object, ok := root.(map[string]any)
	if !ok {
		return nil
	}
	for _, key := range []string{"lyrics", "lines", "element"} {
		if list, ok := object[key].([]any); ok {
			return objectList(list)
		}
	}
	return nil
}

func objectList(list []any) []map[string]any {
	out := make([]map[string]any, 0, len(list))
	for _, value := range list {
		if object, ok := value.(map[string]any); ok {
			out = append(out, object)
		}
	}
	return out
}

func parseJSONWords(value any, fallback uint64, hasFallback bool) []model.LyricsWord {
	list, ok := value.([]any)
	if !ok {
		return nil
	}
	words := make([]model.LyricsWord, 0, len(list))
	for _, value := range list {
		word, ok := value.(map[string]any)
		if !ok {
			continue
		}
		text := stringField(word, "text", "word")
		start, hasStart := timeField(word, "startTime", "start", "time")
		if !hasStart && hasFallback {
			start, hasStart = fallback, true
		}
		end, hasEnd := timeField(word, "endTime", "end")
		if !hasEnd && hasStart {
			end, hasEnd = start+500, true
		}
		if hasStart && hasEnd {
			words = append(words, model.LyricsWord{Text: text, StartMS: start, EndMS: end})
		}
	}
	return words
}

func stringField(object map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := object[key].(string); ok {
			return value
		}
	}
	return ""
}

func timeField(object map[string]any, keys ...string) (uint64, bool) {
	for _, key := range keys {
		if value, ok := object[key]; ok {
			if millis, ok := parseJSONTime(value); ok {
				return millis, true
			}
		}
	}
	return 0, false
}

func parseJSONTime(value any) (uint64, bool) {
	var number float64
	switch value := value.(type) {
	case float64:
		number = value
	case string:
		parsed, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return 0, false
		}
		number = parsed
	default:
		return 0, false
	}
	if number < 0 {
		return 0, false
	}
	if number < 500 {
		number *= 1000
	}
	return uint64(number), true
}

func parseTTML(source string) []model.LyricsLine {
	decoder := xml.NewDecoder(strings.NewReader(source))
	var lines []model.LyricsLine
	var paragraph *ttmlParagraph
	var span *ttmlSpan
	for {
		token, err := decoder.Token()
		if err != nil {
			break
		}
		switch token := token.(type) {
		case xml.StartElement:
			switch token.Name.Local {
			case "p":
				paragraph = &ttmlParagraph{start: xmlTimeAttr(token.Attr, "begin"), end: xmlTimeAttr(token.Attr, "end")}
			case "span":
				if paragraph != nil {
					span = &ttmlSpan{start: xmlTimeAttr(token.Attr, "begin"), end: xmlTimeAttr(token.Attr, "end")}
				}
			case "br":
				if paragraph != nil {
					paragraph.text.WriteByte('\n')
				}
			}
		case xml.CharData:
			if paragraph != nil {
				paragraph.text.Write([]byte(token))
				if span != nil {
					span.text.Write([]byte(token))
				}
			}
		case xml.EndElement:
			switch token.Name.Local {
			case "span":
				if paragraph != nil && span != nil {
					start := firstTime(span.start, paragraph.start)
					end := firstTime(span.end, paragraph.end)
					if start != nil && end != nil && strings.TrimSpace(span.text.String()) != "" {
						paragraph.words = append(paragraph.words, model.LyricsWord{Text: span.text.String(), StartMS: *start, EndMS: *end})
					}
				}
				span = nil
			case "p":
				if paragraph != nil {
					text := strings.TrimSpace(paragraph.text.String())
					if text != "" || paragraph.start != nil {
						lines = append(lines, model.LyricsLine{TimeMS: paragraph.start, EndTimeMS: paragraph.end, Text: text, Words: paragraph.words})
					}
				}
				paragraph = nil
				span = nil
			}
		}
	}
	sortLines(lines)
	return lines
}

type ttmlParagraph struct {
	start *uint64
	end   *uint64
	text  strings.Builder
	words []model.LyricsWord
}

type ttmlSpan struct {
	start *uint64
	end   *uint64
	text  strings.Builder
}

func xmlTimeAttr(attrs []xml.Attr, name string) *uint64 {
	for _, attr := range attrs {
		if attr.Name.Local == name {
			if value, ok := parseTTMLTime(attr.Value); ok {
				return &value
			}
		}
	}
	return nil
}

func parseTTMLTime(value string) (uint64, bool) {
	value = strings.TrimSpace(value)
	if number, ok := strings.CutSuffix(value, "ms"); ok {
		parsed, err := strconv.ParseFloat(number, 64)
		if err != nil || parsed < 0 {
			return 0, false
		}
		return uint64(parsed), true
	}
	if number, ok := strings.CutSuffix(value, "s"); ok {
		parsed, err := strconv.ParseFloat(number, 64)
		if err != nil || parsed < 0 {
			return 0, false
		}
		return uint64(parsed * 1000), true
	}
	parts := strings.Split(value, ":")
	if len(parts) == 3 {
		hours, errHours := strconv.ParseFloat(parts[0], 64)
		minutes, errMinutes := strconv.ParseFloat(parts[1], 64)
		seconds, errSeconds := strconv.ParseFloat(parts[2], 64)
		if errHours == nil && errMinutes == nil && errSeconds == nil && hours >= 0 && minutes >= 0 && seconds >= 0 {
			return uint64((hours*3600 + minutes*60 + seconds) * 1000), true
		}
	}
	return parseLRCTime(value)
}

func firstTime(values ...*uint64) *uint64 {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func lyricsFromLines(source string, lines []model.LyricsLine) *model.Lyrics {
	if len(lines) == 0 {
		return nil
	}
	synced := false
	for i := range lines {
		if lines[i].TimeMS != nil {
			synced = true
			break
		}
	}
	return &model.Lyrics{Source: source, Synced: synced, Lines: lines}
}

func plainLyrics(text, source string) *model.Lyrics {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	rawLines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	lines := make([]model.LyricsLine, len(rawLines))
	for i := range rawLines {
		lines[i].Text = strings.TrimRight(rawLines[i], " \t\r")
	}
	return &model.Lyrics{Source: source, Lines: lines}
}

func sortLines(lines []model.LyricsLine) {
	sort.SliceStable(lines, func(i, j int) bool {
		if lines[i].TimeMS == nil {
			return false
		}
		if lines[j].TimeMS == nil {
			return true
		}
		return *lines[i].TimeMS < *lines[j].TimeMS
	})
}

func nextRune(text string) (rune, int) {
	for _, r := range text {
		return r, len(string(r))
	}
	return 0, 0
}
