package lyrics

import (
	"reflect"
	"testing"
)

func TestParseLRCTimestamps(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []uint64
		text  []string
	}{
		{
			name:  "accepted precision",
			input: "[01:02]whole\n[00:03.4]tenths\n[00:04.56]hundredths\n[00:05.7899]thousandths",
			want:  []uint64{3400, 4560, 5789, 62000},
			text:  []string{"tenths", "hundredths", "thousandths", "whole"},
		},
		{
			name:  "multiple stamps metadata and empty cue",
			input: "[ar:Artist]\n[00:02.00][00:01.00][Chorus]\n[00:03.00]",
			want:  []uint64{1000, 2000, 3000},
			text:  []string{"[Chorus]", "[Chorus]", ""},
		},
		{
			name:  "global offset",
			input: "[00:01.00]first\n[offset:-250]\n[00:02.00]second",
			want:  []uint64{750, 1750},
			text:  []string{"first", "second"},
		},
		{
			name:  "fraction filters non digits",
			input: "[00:01.a2b3c4]line",
			want:  []uint64{1234},
			text:  []string{"line"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := parseLRC(test.input)
			times := make([]uint64, len(got))
			texts := make([]string, len(got))
			for i := range got {
				if got[i].TimeMS == nil {
					t.Fatalf("line %d has no timestamp", i)
				}
				times[i] = *got[i].TimeMS
				texts[i] = got[i].Text
			}
			if !reflect.DeepEqual(times, test.want) {
				t.Fatalf("times = %v, want %v", times, test.want)
			}
			if !reflect.DeepEqual(texts, test.text) {
				t.Fatalf("text = %q, want %q", texts, test.text)
			}
		})
	}
}

func TestParseLRCWordTiming(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantText  string
		wantStart []uint64
		wantEnd   []uint64
	}{
		{
			name:      "enhanced LRC",
			input:     "[00:10.00]<00:10.00>Hello <00:10.50>world",
			wantText:  "Hello world",
			wantStart: []uint64{10000, 10000},
			wantEnd:   []uint64{10000, 10500},
		},
		{
			name:      "K LRC",
			input:     "[10000,1200](0,500,0)Hello (500,700,0)world",
			wantText:  "Hello world",
			wantStart: []uint64{10000, 10500},
			wantEnd:   []uint64{10500, 11200},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			lines := parseLRC(test.input)
			if len(lines) != 1 {
				t.Fatalf("got %d lines, want 1", len(lines))
			}
			if lines[0].Text != test.wantText {
				t.Fatalf("text = %q, want %q", lines[0].Text, test.wantText)
			}
			starts := make([]uint64, len(lines[0].Words))
			ends := make([]uint64, len(lines[0].Words))
			for i, word := range lines[0].Words {
				starts[i], ends[i] = word.StartMS, word.EndMS
			}
			if !reflect.DeepEqual(starts, test.wantStart) || !reflect.DeepEqual(ends, test.wantEnd) {
				t.Fatalf("word timing = %v..%v, want %v..%v", starts, ends, test.wantStart, test.wantEnd)
			}
		})
	}
}

func TestNormalizeMatch(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"  Beyoncé \u2014 DÉJÀ Vu!  ", "beyonce deja vu"},
		{"Sigur Rós / Svefn-g-englar", "sigur ros svefn g englar"},
		{"ＡＢＣ １２３", "abc 123"},
	}
	for _, test := range tests {
		if got := normalizeMatch(test.input); got != test.want {
			t.Errorf("normalizeMatch(%q) = %q, want %q", test.input, got, test.want)
		}
	}
	if !namesMatch("Save Your Tears", "The Weeknd feat. Ariana Grande", "save-your tears", "The Weeknd") {
		t.Error("normalized title and primary artist should match")
	}
}

func TestBestByDurationPrefersNormalizedMatchAndClosestCut(t *testing.T) {
	type candidate struct {
		title    string
		artist   string
		duration float64
	}
	ours := 233.0
	candidates := []candidate{
		{title: "Shape of You (DJ Edit)", artist: "Ed Sheeran", duration: 263},
		{title: "shape-of you", artist: "Ed Sheeran", duration: 233.7},
		{title: "Shape of You", artist: "Ed Sheeran", duration: 231.2},
	}
	got, ok := bestByDuration(&ours, candidates, func(value candidate) (float64, bool) {
		return value.duration, true
	}, func(value candidate) bool {
		return namesMatch("Shape of You", "Ed Sheeran", value.title, value.artist)
	})
	if !ok || got.duration != 233.7 {
		t.Fatalf("best candidate = %#v, %v", got, ok)
	}
	if _, ok := bestByDuration(&ours, candidates[:1], func(value candidate) (float64, bool) {
		return value.duration, true
	}, nil); ok {
		t.Fatal("wrong cut outside five-second tolerance matched")
	}
}
