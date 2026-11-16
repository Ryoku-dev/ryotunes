package lyrics

import (
	"testing"

	"github.com/ryoku-dev/ryotunes/internal/model"
)

func describeCandidate(value candidate) candidate { return value }

func TestPickRejectsUnrelatedCandidateWithinTolerance(t *testing.T) {
	unrelated := []candidate{
		{title: "Pimp", artists: "Some Rapper", duration: 163, known: true},
		{title: "Legacy", artists: "Other Artist", duration: 163.4, known: true},
	}
	for _, variant := range []bool{false, true} {
		q := query{Title: "LEGACY", Artists: "PIXY", Duration: new(163.0), Variant: variant}
		if got, _, ok := pick(q, unrelated, describeCandidate); ok {
			t.Fatalf("variant %v picked unrelated %#v", variant, got)
		}
		q.Duration = nil
		if got, _, ok := pick(q, unrelated, describeCandidate); ok {
			t.Fatalf("variant %v without a length picked unrelated %#v", variant, got)
		}
	}
}

func TestPickPrefersNameMatchWithNearestLength(t *testing.T) {
	q := query{Title: "Shape of You", Artists: "Ed Sheeran", Duration: new(233.0)}
	candidates := []candidate{
		{title: "Castle on the Hill", artists: "Ed Sheeran", duration: 233, known: true},
		{title: "Shape of You (DJ Edit)", artists: "Ed Sheeran", duration: 233.2, known: true},
		{title: "Shape of You", artists: "Ed Sheeran", duration: 231.2, known: true},
		{title: "shape-of you", artists: "Ed Sheeran", duration: 233.7, known: true},
	}
	got, how, ok := pick(q, candidates, describeCandidate)
	if !ok || got.duration != 233.7 || how != (timing{}) {
		t.Fatalf("pick = %#v, %#v, %v; want the 233.7 s name match as it is", got, how, ok)
	}
	if got, _, ok := pick(q, []candidate{{title: "Shape of You", artists: "Ed Sheeran", duration: 263, known: true}}, describeCandidate); ok {
		t.Fatalf("a name match 30 s off was picked for an original: %#v", got)
	}
}

func timedLyrics() *model.Lyrics {
	return &model.Lyrics{Source: "LRCLIB", Synced: true, Lines: []model.LyricsLine{
		{TimeMS: new(uint64(1000)), Text: "one"},
		{TimeMS: new(uint64(14000)), EndTimeMS: new(uint64(17500)), Text: "two words", Words: []model.LyricsWord{
			{Text: "two", StartMS: 14000, EndMS: 15000}, {Text: "words", StartMS: 15000, EndMS: 17500},
		}},
		{TimeMS: new(uint64(28000)), Text: "three"},
	}}
}

func TestPickScalesVariantFromOriginal(t *testing.T) {
	q := query{Title: "LEGACY", Artists: "PIXY", Duration: new(163.0), Variant: true}
	candidates := []candidate{
		{title: "Pimp", artists: "Some Rapper", duration: 163, known: true},
		{title: "Legacy", artists: "PIXY", duration: 140, known: true},
	}
	got, how, ok := pick(q, candidates, describeCandidate)
	if !ok || got.title != "Legacy" {
		t.Fatalf("pick = %#v, %v; want the original by name", got, ok)
	}
	ratio := 163.0 / 140.0
	if how.unsynced || how.scale != ratio {
		t.Fatalf("timing = %#v, want scale %v", how, ratio)
	}
	lyrics := how.apply(timedLyrics())
	if !lyrics.Synced {
		t.Fatal("scaled lyrics lost their timing")
	}
	wantLines := []uint64{1164, 16300, 32600}
	var previous uint64
	for i, line := range lyrics.Lines {
		if line.TimeMS == nil || *line.TimeMS != wantLines[i] {
			t.Fatalf("line %d time = %v, want %d", i, line.TimeMS, wantLines[i])
		}
		if *line.TimeMS < previous {
			t.Fatalf("line %d moved before line %d", i, i-1)
		}
		previous = *line.TimeMS
	}
	if lyrics.Lines[1].EndTimeMS == nil || *lyrics.Lines[1].EndTimeMS != 20375 {
		t.Fatalf("line end = %v, want 20375", lyrics.Lines[1].EndTimeMS)
	}
	words := lyrics.Lines[1].Words
	if words[0].StartMS != 16300 || words[0].EndMS != 17464 || words[1].StartMS != 17464 || words[1].EndMS != 20375 {
		t.Fatalf("word times = %#v", words)
	}
}

func TestPickVariantWithinToleranceKeepsTiming(t *testing.T) {
	q := query{Title: "LEGACY", Artists: "PIXY", Duration: new(163.0), Variant: true}
	_, how, ok := pick(q, []candidate{
		{title: "Legacy", artists: "PIXY", duration: 140, known: true},
		{title: "Legacy (Slowed)", artists: "PIXY", duration: 162, known: true},
	}, describeCandidate)
	if !ok || how != (timing{}) {
		t.Fatalf("timing = %#v, %v; want the slowed release's own timing", how, ok)
	}
}

func TestPickVariantOutsideRatioBandGivesUnsyncedLines(t *testing.T) {
	q := query{Title: "LEGACY", Artists: "PIXY", Duration: new(330.0), Variant: true}
	_, how, ok := pick(q, []candidate{{title: "Legacy", artists: "PIXY", duration: 140, known: true}}, describeCandidate)
	if !ok || !how.unsynced {
		t.Fatalf("timing = %#v, %v; want unsynced", how, ok)
	}
	lyrics := how.apply(timedLyrics())
	if lyrics.Synced {
		t.Fatal("out of band variant stayed synced")
	}
	for i, line := range lyrics.Lines {
		if line.TimeMS != nil || line.EndTimeMS != nil || line.Words != nil {
			t.Fatalf("line %d kept timing: %#v", i, line)
		}
	}
	if lyrics.Lines[1].Text != "two words" {
		t.Fatalf("text changed: %q", lyrics.Lines[1].Text)
	}

	q.Duration = nil
	if _, how, ok := pick(q, []candidate{{title: "Legacy", artists: "PIXY", duration: 140, known: true}}, describeCandidate); !ok || !how.unsynced {
		t.Fatalf("variant without our length = %#v, %v; want unsynced", how, ok)
	}
	q.Duration = new(163.0)
	if _, how, ok := pick(q, []candidate{{title: "Legacy", artists: "PIXY"}}, describeCandidate); !ok || !how.unsynced {
		t.Fatalf("variant against an unknown length = %#v, %v; want unsynced", how, ok)
	}
}

func TestInstrumentalIgnoresTiming(t *testing.T) {
	lyrics := &model.Lyrics{Source: "LRCLIB", Instrumental: true, Lines: []model.LyricsLine{}}
	if got := (timing{unsynced: true}).apply(lyrics); !got.Instrumental || got.Lines == nil {
		t.Fatalf("instrumental = %#v", got)
	}
}
