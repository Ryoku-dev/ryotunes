package look

import (
	"reflect"
	"testing"
	"unicode"

	"github.com/ryoku-dev/ryotunes/internal/model"
)

func TestPrepareKaraokeKeepsSaneSourceWords(t *testing.T) {
	line := model.LyricsLine{
		TimeMS: new(uint64(1_000)), EndTimeMS: new(uint64(3_000)), Text: "still water",
		Words: []model.LyricsWord{
			{Text: "still", StartMS: 1_100, EndMS: 1_700},
			{Text: "water", StartMS: 1_750, EndMS: 2_500},
		},
	}
	got := PrepareKaraoke([]model.LyricsLine{line})[0]
	want := []KaraokeWord{
		{Text: "still", StartMS: 1_100, EndMS: 1_700},
		{Text: "water", StartMS: 1_750, EndMS: 2_500, SpaceBefore: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("prepared words = %#v, want %#v", got, want)
	}
}

func TestPrepareKaraokeClampsSmallSourceOvershoot(t *testing.T) {
	line := model.LyricsLine{
		TimeMS: new(uint64(1_000)), EndTimeMS: new(uint64(3_000)), Text: "late",
		Words: []model.LyricsWord{{Text: "late", StartMS: 990, EndMS: 3_035}},
	}
	got := PrepareKaraoke([]model.LyricsLine{line})[0]
	if len(got) != 1 || got[0].StartMS != 1_000 || got[0].EndMS != 3_000 {
		t.Fatalf("clamped source word = %#v", got)
	}
}

func TestPrepareKaraokeDetectsRTLDirection(t *testing.T) {
	line := model.LyricsLine{
		TimeMS: new(uint64(1_000)), EndTimeMS: new(uint64(3_000)), Text: "אחת שתיים שלוש",
	}
	words := PrepareKaraoke([]model.LyricsLine{line})[0]
	if len(words) != 3 {
		t.Fatalf("words = %#v, want three", words)
	}
	for _, word := range words {
		if !word.RTL {
			t.Fatalf("word did not retain RTL direction: %#v", word)
		}
	}
}

func TestPrepareKaraokeEstimatesSpacesAndCJK(t *testing.T) {
	lines := []model.LyricsLine{
		{TimeMS: new(uint64(1_000)), EndTimeMS: new(uint64(4_000)), Text: "soft  signal"},
		{TimeMS: new(uint64(5_000)), EndTimeMS: new(uint64(7_000)), Text: "夜汽車"},
	}
	got := PrepareKaraoke(lines)
	if texts := karaokeTexts(got[0]); !reflect.DeepEqual(texts, []string{"soft", "signal"}) {
		t.Fatalf("space words = %#v", texts)
	}
	if !got[0][1].SpaceBefore {
		t.Fatal("space-delimited word did not retain its advance")
	}
	if texts := karaokeTexts(got[1]); !reflect.DeepEqual(texts, []string{"夜", "汽", "車"}) {
		t.Fatalf("CJK words = %#v", texts)
	}
	for _, word := range got[1] {
		if word.SpaceBefore {
			t.Fatalf("CJK token gained a space: %#v", word)
		}
	}
}

func TestPrepareKaraokeUsesEndBeforeNextLine(t *testing.T) {
	withEnd := []model.LyricsLine{
		{TimeMS: new(uint64(1_000)), EndTimeMS: new(uint64(2_000)), Text: "one two"},
		{TimeMS: new(uint64(9_000)), Text: "later"},
	}
	words := PrepareKaraoke(withEnd)[0]
	if got := words[len(words)-1].EndMS; got > 1_880 {
		t.Fatalf("word end = %d, crossed the explicit end pause", got)
	}

	withNext := []model.LyricsLine{
		{TimeMS: new(uint64(1_000)), Text: "one two"},
		{TimeMS: new(uint64(2_100)), Text: "next"},
	}
	words = PrepareKaraoke(withNext)[0]
	if got := words[len(words)-1].EndMS; got > 1_980 {
		t.Fatalf("word end = %d, crossed the next-line pause", got)
	}
}

func TestPrepareKaraokeCapsInstrumentalGapAndLastLine(t *testing.T) {
	lines := []model.LyricsLine{
		{TimeMS: new(uint64(1_000)), Text: "brief line"},
		{TimeMS: new(uint64(61_000)), Text: "last line"},
	}
	got := PrepareKaraoke(lines)
	if span := got[0][len(got[0])-1].EndMS - got[0][0].StartMS; span >= 5_000 {
		t.Fatalf("instrumental gap stretched words for %d ms", span)
	}
	if end := got[1][len(got[1])-1].EndMS; end > 69_000 {
		t.Fatalf("last line end = %d, want capped by 69000", end)
	}
}

func TestSplitKaraokeTextKeepsGraphemesAndClosingPunctuation(t *testing.T) {
	tests := []struct {
		value string
		want  []string
	}{
		{value: "か\u3099、き。", want: []string{"か\u3099、", "き。"}},
		{value: "사랑", want: []string{"사", "랑"}},
	}
	for _, test := range tests {
		if got := karaokeTexts(splitKaraokeText(test.value)); !reflect.DeepEqual(got, test.want) {
			t.Errorf("split %q = %#v, want %#v", test.value, got, test.want)
		}
	}
	thai := splitKaraokeText("ภาษาไทย")
	if len(thai) < 2 {
		t.Fatalf("Thai line remained one over-wide token: %#v", thai)
	}
	for _, word := range thai {
		if len([]rune(word.Text)) == 1 && unicode.Is(unicode.Mn, []rune(word.Text)[0]) {
			t.Fatalf("combining mark split from its base: %#v", thai)
		}
	}
}

func karaokeTexts(words []KaraokeWord) []string {
	values := make([]string, len(words))
	for index := range words {
		values[index] = words[index].Text
	}
	return values
}
