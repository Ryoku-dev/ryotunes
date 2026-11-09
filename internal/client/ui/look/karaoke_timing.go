package look

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/ryoku-dev/ryotunes/internal/model"
)

const (
	karaokeTailPauseMS       = uint64(120)
	karaokeMinSpanMS         = uint64(500)
	karaokeLastCapMS         = uint64(8_000)
	karaokeTimingToleranceMS = uint64(50)
)

// KaraokeWord is one measured lyric token and its playback interval.
type KaraokeWord struct {
	Text        string
	StartMS     uint64
	EndMS       uint64
	SpaceBefore bool
	RTL         bool
}

// PrepareKaraoke resolves real or estimated word timings once for a lyric document.
func PrepareKaraoke(lines []model.LyricsLine) [][]KaraokeWord {
	prepared := make([][]KaraokeWord, len(lines))
	for index := range lines {
		line := lines[index]
		if line.TimeMS == nil {
			continue
		}
		end := karaokeLineEnd(lines, index)
		rtl := karaokeLineRTL(line)
		if words, ok := karaokeSourceWords(line, end, rtl); ok {
			prepared[index] = words
			continue
		}
		prepared[index] = estimateKaraokeWords(line, end, rtl)
	}
	return prepared
}

func karaokeLineEnd(lines []model.LyricsLine, index int) uint64 {
	line := lines[index]
	if line.TimeMS == nil {
		return 0
	}
	start := *line.TimeMS
	if line.EndTimeMS != nil && *line.EndTimeMS > start {
		return *line.EndTimeMS
	}
	for next := index + 1; next < len(lines); next++ {
		if lines[next].TimeMS != nil && *lines[next].TimeMS > start {
			return *lines[next].TimeMS
		}
	}
	return start + karaokeLastCapMS
}

func karaokeSourceWords(line model.LyricsLine, lineEnd uint64, rtl bool) ([]KaraokeWord, bool) {
	if line.TimeMS == nil || lineEnd <= *line.TimeMS || len(line.Words) == 0 {
		return nil, false
	}
	lineStart := *line.TimeMS
	words := make([]KaraokeWord, 0, len(line.Words))
	remaining := line.Text
	var previousStart, previousEnd uint64
	for index, word := range line.Words {
		trimmed := strings.TrimSpace(word.Text)
		if trimmed == "" ||
			(word.StartMS < lineStart && lineStart-word.StartMS > karaokeTimingToleranceMS) ||
			(word.EndMS > lineEnd && word.EndMS-lineEnd > karaokeTimingToleranceMS) {
			return nil, false
		}
		start := max(lineStart, word.StartMS)
		end := min(lineEnd, word.EndMS)
		if start >= lineEnd || end <= start {
			return nil, false
		}
		if index > 0 && (start < previousStart || end < previousEnd) {
			return nil, false
		}
		spaceBefore := len(words) > 0 && karaokeNeedsSpace(words[len(words)-1].Text, trimmed)
		if at := strings.Index(remaining, trimmed); at >= 0 {
			spaceBefore = len(words) > 0 && strings.IndexFunc(remaining[:at], unicode.IsSpace) >= 0
			remaining = remaining[at+len(trimmed):]
		}
		pieces := []string{trimmed}
		if karaokeContainsNoSpaceScript(trimmed) {
			tokens := splitKaraokeText(trimmed)
			if len(tokens) > 1 {
				pieces = pieces[:0]
				for _, token := range tokens {
					pieces = append(pieces, token.Text)
				}
			}
		}
		for pieceIndex, piece := range pieces {
			words = append(words, KaraokeWord{
				Text: piece, StartMS: start, EndMS: end,
				SpaceBefore: pieceIndex == 0 && spaceBefore, RTL: rtl,
			})
		}
		previousStart, previousEnd = start, end
	}
	return words, true
}

func estimateKaraokeWords(line model.LyricsLine, lineEnd uint64, rtl bool) []KaraokeWord {
	if line.TimeMS == nil || lineEnd <= *line.TimeMS {
		return nil
	}
	tokens := splitKaraokeText(line.Text)
	if len(tokens) == 0 {
		return nil
	}
	totalWeight := 0
	for index := range tokens {
		totalWeight += utf8.RuneCountInString(tokens[index].Text) + 1
	}
	target := max(karaokeMinSpanMS, uint64((875*totalWeight+5)/10))
	available := lineEnd - *line.TimeMS
	if available > karaokeTailPauseMS {
		available -= karaokeTailPauseMS
	}
	span := min(available, target)
	if span == 0 {
		return nil
	}

	words := make([]KaraokeWord, len(tokens))
	elapsedWeight := 0
	for index, token := range tokens {
		start := *line.TimeMS + span*uint64(elapsedWeight)/uint64(totalWeight)
		elapsedWeight += utf8.RuneCountInString(token.Text) + 1
		end := *line.TimeMS + span*uint64(elapsedWeight)/uint64(totalWeight)
		if index == len(tokens)-1 {
			end = *line.TimeMS + span
		}
		words[index] = KaraokeWord{
			Text: token.Text, StartMS: start, EndMS: end,
			SpaceBefore: token.SpaceBefore, RTL: rtl,
		}
	}
	return words
}

func splitKaraokeText(value string) []KaraokeWord {
	words := make([]KaraokeWord, 0, len(strings.Fields(value)))
	var token strings.Builder
	spaceBefore, pendingSpace, clustered := false, false, false
	flush := func() {
		if token.Len() == 0 {
			return
		}
		words = append(words, KaraokeWord{Text: token.String(), SpaceBefore: spaceBefore})
		token.Reset()
		spaceBefore, clustered = false, false
	}
	startToken := func(r rune, cluster bool) {
		if token.Len() == 0 {
			spaceBefore = pendingSpace
			pendingSpace = false
		}
		token.WriteRune(r)
		clustered = cluster
	}
	for _, r := range value {
		switch {
		case unicode.IsSpace(r):
			flush()
			pendingSpace = len(words) > 0
		case unicode.In(r, unicode.Mn, unicode.Me, unicode.Mc):
			if token.Len() > 0 {
				token.WriteRune(r)
			} else if len(words) > 0 {
				words[len(words)-1].Text += string(r)
			} else {
				startToken(r, false)
			}
		case karaokeClosingPunctuation(r):
			flush()
			if len(words) > 0 {
				words[len(words)-1].Text += string(r)
			} else {
				startToken(r, false)
			}
		case karaokeHangulJamo(r):
			if token.Len() > 0 && (!karaokeHangulContinuation(r) || !clustered) {
				flush()
			}
			startToken(r, true)
		case karaokeNoSpaceScript(r):
			if token.Len() > 0 && !(clustered && karaokeClusterJoinsNext(token.String())) {
				flush()
			}
			startToken(r, true)
		case karaokeCJK(r):
			flush()
			words = append(words, KaraokeWord{Text: string(r), SpaceBefore: pendingSpace})
			pendingSpace = false
		default:
			if clustered {
				flush()
			}
			startToken(r, false)
		}
	}
	flush()
	return words
}

func karaokeCJK(r rune) bool {
	return unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul)
}

func karaokeNoSpaceScript(r rune) bool {
	return unicode.In(r, unicode.Thai, unicode.Lao, unicode.Khmer, unicode.Myanmar)
}

func karaokeContainsNoSpaceScript(value string) bool {
	return strings.IndexFunc(value, karaokeNoSpaceScript) >= 0
}

func karaokeHangulJamo(r rune) bool {
	return r >= 0x1100 && r <= 0x11ff || r >= 0xa960 && r <= 0xa97f || r >= 0xd7b0 && r <= 0xd7ff
}

func karaokeHangulContinuation(r rune) bool {
	return r >= 0x1160 && r <= 0x11ff || r >= 0xd7b0 && r <= 0xd7ff
}

func karaokeClusterJoinsNext(value string) bool {
	r, _ := utf8.DecodeLastRuneInString(value)
	return r == 0x17d2 || r == 0x1039
}

func karaokeClosingPunctuation(r rune) bool {
	if r > unicode.MaxASCII && unicode.Is(unicode.Pe, r) {
		return true
	}
	return strings.ContainsRune("、。，．！？：；」』】〕〉》〗〙〛", r)
}

func karaokeLineRTL(line model.LyricsLine) bool {
	value := line.Text
	if strings.TrimSpace(value) == "" {
		var joined strings.Builder
		for _, word := range line.Words {
			joined.WriteString(word.Text)
		}
		value = joined.String()
	}
	for _, r := range value {
		if unicode.In(r, unicode.Hebrew, unicode.Arabic, unicode.Syriac, unicode.Thaana, unicode.Nko, unicode.Samaritan, unicode.Mandaic, unicode.Manichaean, unicode.Adlam) {
			return true
		}
		if unicode.IsLetter(r) {
			return false
		}
	}
	return false
}

func karaokeNeedsSpace(left, right string) bool {
	leftRune, _ := utf8.DecodeLastRuneInString(strings.TrimSpace(left))
	rightRune, _ := utf8.DecodeRuneInString(strings.TrimSpace(right))
	return !karaokeCJK(leftRune) && !karaokeCJK(rightRune)
}
