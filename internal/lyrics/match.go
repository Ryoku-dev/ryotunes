package lyrics

import (
	"math"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

const matchToleranceSeconds = 5.0

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

func namesMatch(wantTitle, wantArtists, title, artists string) bool {
	if normalizeMatch(title) != normalizeMatch(wantTitle) {
		return false
	}
	want := normalizeMatch(wantArtists)
	got := normalizeMatch(artists)
	if want == "" || got == "" || want == got {
		return true
	}
	return normalizeMatch(primaryArtist(wantArtists)) == normalizeMatch(primaryArtist(artists))
}

func primaryArtist(value string) string {
	lower := strings.ToLower(value)
	end := len(value)
	for _, separator := range []string{" feat.", " feat ", " featuring ", " ft.", " ft ", " & ", ",", ";", " x "} {
		if index := strings.Index(lower, separator); index >= 0 && index < end {
			end = index
		}
	}
	return strings.TrimSpace(value[:end])
}

func bestByDuration[T any](ours *float64, candidates []T, duration func(T) (float64, bool), matches func(T) bool) (T, bool) {
	var zero T
	if len(candidates) == 0 {
		return zero, false
	}
	pool := candidates
	if matches != nil {
		exact := make([]T, 0, len(candidates))
		for _, candidate := range candidates {
			if matches(candidate) {
				exact = append(exact, candidate)
			}
		}
		if len(exact) != 0 {
			pool = exact
		}
	}
	if ours == nil || *ours <= 0 {
		return pool[0], true
	}
	bestDistance := math.Inf(1)
	best := -1
	for i, candidate := range pool {
		candidateDuration, ok := duration(candidate)
		distance := math.Inf(1)
		if ok {
			distance = math.Abs(candidateDuration - *ours)
			if distance > matchToleranceSeconds {
				continue
			}
		}
		if best < 0 || distance < bestDistance {
			best = i
			bestDistance = distance
		}
	}
	if best < 0 {
		return zero, false
	}
	return pool[best], true
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
