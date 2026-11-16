package lyrics

import (
	"math"
	"strings"
	"unicode"

	"github.com/ryoku-dev/ryotunes/internal/model"
	"golang.org/x/text/unicode/norm"
)

const (
	// matchToleranceSeconds is how far a candidate's length may sit from
	// ours and still be the same cut of the same recording.
	matchToleranceSeconds = 5.0
	// A tempo variant is retimed from the original only when its length is
	// a plausible slowdown or speedup of the original's.
	minVariantRatio = 0.6
	maxVariantRatio = 1.5
)

func normalizeMatch(value string) string {
	value = norm.NFKD.String(strings.ToLower(value))
	var out strings.Builder
	space := false
	for _, r := range value {
		switch {
		case unicode.Is(unicode.Mn, r):
			continue
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if space && out.Len() != 0 {
				out.WriteByte(' ')
			}
			out.WriteRune(r)
			space = false
		default:
			space = true
		}
	}
	return out.String()
}

// matches reports whether a provider's song is the one q names: the same
// title once both are cleaned, and a shared artist when both credit one.
func (q query) matches(title, artists string) bool {
	if normalizeMatch(cleanTitle(title)) != normalizeMatch(q.Title) {
		return false
	}
	return artistsOverlap(q.Artists, artists)
}

// candidate is what a provider says about one search result.
type candidate struct {
	title    string
	artists  string
	duration float64
	known    bool
}

// timing says how a matched candidate's times relate to our recording.
type timing struct {
	// scale multiplies every time; 0 or 1 keeps them.
	scale float64
	// unsynced drops the times: they cannot follow this recording.
	unsynced bool
}

// pick chooses the candidate whose lyrics belong to q. A candidate whose
// names do not match is never lyrics for this track, however close its
// length. Among name matches the one within matchToleranceSeconds of our
// length wins as it is. A tempo variant takes the first name match whose
// length makes a plausible tempo change and scales its times, or failing
// that any name match without its times.
func pick[T any](q query, candidates []T, describe func(T) candidate) (T, timing, bool) {
	var zero T
	named := make([]T, 0, len(candidates))
	info := make([]candidate, 0, len(candidates))
	for _, value := range candidates {
		about := describe(value)
		if about.known && about.duration <= 0 {
			about.known = false
		}
		if q.matches(about.title, about.artists) {
			named = append(named, value)
			info = append(info, about)
		}
	}
	if len(named) == 0 {
		return zero, timing{}, false
	}
	if q.Duration == nil {
		return named[0], timing{unsynced: q.Variant}, true
	}
	ours := *q.Duration
	best, unknown := -1, -1
	bestDistance := math.Inf(1)
	for i, about := range info {
		if !about.known {
			if unknown < 0 {
				unknown = i
			}
			continue
		}
		if distance := math.Abs(about.duration - ours); distance <= matchToleranceSeconds && distance < bestDistance {
			best, bestDistance = i, distance
		}
	}
	if best >= 0 {
		return named[best], timing{}, true
	}
	if !q.Variant {
		if unknown >= 0 {
			return named[unknown], timing{}, true
		}
		return zero, timing{}, false
	}
	for i, about := range info {
		if !about.known {
			continue
		}
		if ratio := ours / about.duration; ratio >= minVariantRatio && ratio <= maxVariantRatio {
			return named[i], timing{scale: ratio}, true
		}
	}
	return named[0], timing{unsynced: true}, true
}

// apply retimes lyrics in place and returns them.
func (t timing) apply(lyrics *model.Lyrics) *model.Lyrics {
	if lyrics == nil || lyrics.Instrumental {
		return lyrics
	}
	switch {
	case t.unsynced:
		lyrics.Synced = false
		for i := range lyrics.Lines {
			lyrics.Lines[i].TimeMS = nil
			lyrics.Lines[i].EndTimeMS = nil
			lyrics.Lines[i].Words = nil
		}
	case t.scale > 0 && t.scale != 1:
		for i := range lyrics.Lines {
			line := &lyrics.Lines[i]
			line.TimeMS = scaledMS(line.TimeMS, t.scale)
			line.EndTimeMS = scaledMS(line.EndTimeMS, t.scale)
			if len(line.Words) != 0 {
				words := make([]model.LyricsWord, len(line.Words))
				for j, word := range line.Words {
					word.StartMS = uint64(math.Round(float64(word.StartMS) * t.scale))
					word.EndMS = uint64(math.Round(float64(word.EndMS) * t.scale))
					words[j] = word
				}
				line.Words = words
			}
		}
	}
	return lyrics
}

func scaledMS(value *uint64, scale float64) *uint64 {
	if value == nil {
		return nil
	}
	scaled := uint64(math.Round(float64(*value) * scale))
	return &scaled
}

func sameFoldedText(a, b string) bool {
	fold := func(value string) string {
		var out strings.Builder
		for _, r := range value {
			if !unicode.IsSpace(r) {
				out.WriteRune(unicode.ToLower(r))
			}
		}
		return out.String()
	}
	return fold(a) == fold(b)
}
