package lyrics

import (
	"strings"
)

// query is the song a lyrics request names, as every remote provider
// searches for and matches it. Uploads dress a song up: "Artist - Title
// (Official Video)" on a channel that is not the artist, " - Topic" and
// "VEVO" channel names, "ft. X" inside the title, and tempo variants such
// as "(slowed + reverb)". newQuery takes all of that off.
type query struct {
	Title    string
	Artists  string
	Album    *string
	Duration *float64
	// Variant marks a slowed, sped up, nightcore, reverb, 8D or daycore
	// upload: its timing never follows the original recording.
	Variant bool
}

func newQuery(req Request) query {
	q := query{Album: req.Album, Duration: req.Duration}
	if q.Duration != nil && *q.Duration <= 0 {
		q.Duration = nil
	}
	artists := cleanChannel(req.Artists)
	title, variant := stripDecoration(req.Title)
	if left, right, ok := splitArtistTitle(title); ok {
		switch {
		case artistsOverlap(left, artists) && artists != "":
			title = right
		case req.Album == nil || strings.TrimSpace(*req.Album) == "":
			// A catalogue song carries an album; an upload titled
			// "Artist - Title" on someone else's channel does not.
			title, artists = right, left
		}
	}
	title, tailVariant := stripTail(title)
	if title == "" {
		title = strings.TrimSpace(req.Title)
	}
	if artists == "" {
		artists = strings.TrimSpace(req.Artists)
	}
	q.Title, q.Artists, q.Variant = title, artists, variant || tailVariant
	return q
}

// cleanTitle takes a candidate's title through the same steps as the
// request's, so "Song (Remastered 2011)" and "Song" compare equal.
func cleanTitle(title string) string {
	cleaned, _ := stripDecoration(title)
	cleaned, _ = stripTail(cleaned)
	if cleaned == "" {
		return strings.TrimSpace(title)
	}
	return cleaned
}

// cleanChannel drops what YouTube adds to an artist's channel name.
func cleanChannel(artists string) string {
	artists = strings.TrimSpace(artists)
	if len(artists) > len(" - topic") && strings.EqualFold(artists[len(artists)-len(" - topic"):], " - topic") {
		artists = strings.TrimSpace(artists[:len(artists)-len(" - topic")])
	}
	if len(artists) > len("vevo") && strings.EqualFold(artists[len(artists)-len("vevo"):], "vevo") {
		artists = strings.TrimSpace(artists[:len(artists)-len("vevo")])
	}
	return artists
}

var bracketPairs = map[rune]rune{'(': ')', '[': ']', '【': '】'}

// stripDecoration removes bracketed decoration ("(Official Video)",
// "[Lyrics]", "(feat. X)", "(slowed + reverb)") and trailing " - " or " | "
// segments made only of decoration, and reports a tempo variant among them.
func stripDecoration(title string) (string, bool) {
	variant := false
	var out strings.Builder
	runes := []rune(title)
	for i := 0; i < len(runes); i++ {
		closer, opens := bracketPairs[runes[i]]
		if !opens {
			out.WriteRune(runes[i])
			continue
		}
		end, depth := -1, 0
		for j := i + 1; j < len(runes) && end < 0; j++ {
			switch runes[j] {
			case runes[i]:
				depth++
			case closer:
				if depth == 0 {
					end = j
				}
				depth--
			}
		}
		if end < 0 {
			out.WriteRune(runes[i])
			continue
		}
		decoration, isVariant := classifyDecoration(string(runes[i+1 : end]))
		if !decoration {
			out.WriteString(string(runes[i : end+1]))
		} else {
			variant = variant || isVariant
			out.WriteByte(' ')
		}
		i = end
	}
	cleaned := strings.Join(strings.Fields(out.String()), " ")
	for {
		cut := -1
		width := 0
		for _, separator := range []string{" - ", " \u2013 ", " \u2014 ", " | ", " // "} {
			if index := strings.LastIndex(cleaned, separator); index > cut {
				cut, width = index, len(separator)
			}
		}
		if cut <= 0 {
			break
		}
		decoration, isVariant := classifyDecoration(cleaned[cut+width:])
		if !decoration {
			break
		}
		variant = variant || isVariant
		cleaned = strings.TrimSpace(cleaned[:cut])
	}
	return cleaned, variant
}

// classifyDecoration reports whether text from brackets or a trailing
// segment only dresses the title, and whether it names a tempo variant.
func classifyDecoration(text string) (decoration, variant bool) {
	words := strings.Fields(normalizeMatch(text))
	if len(words) == 0 {
		return strings.TrimSpace(text) == "", false
	}
	for i, word := range words {
		next := ""
		if i+1 < len(words) {
			next = words[i+1]
		}
		switch {
		case variantWord(word, next):
			variant = true
		case word == "m" && next == "v":
			decoration = true
		}
		switch word {
		case "official", "video", "audio", "lyric", "lyrics", "visualizer", "visualiser",
			"mv", "hd", "hq", "4k", "remaster", "remastered", "explicit", "letra", "coded":
			decoration = true
		}
	}
	switch words[0] {
	case "feat", "ft", "featuring", "prod":
		decoration = true
	}
	return decoration || variant, variant
}

func variantWord(word, next string) bool {
	switch word {
	case "slowed", "reverb", "reverbed", "nightcore", "daycore", "8d", "spedup":
		return true
	case "sped", "speed":
		return next == "up"
	}
	return false
}

// splitArtistTitle splits "Artist - Title" at the first dash.
func splitArtistTitle(title string) (string, string, bool) {
	cut, width := -1, 0
	for _, separator := range []string{" - ", " \u2013 ", " \u2014 "} {
		if index := strings.Index(title, separator); index > 0 && (cut < 0 || index < cut) {
			cut, width = index, len(separator)
		}
	}
	if cut < 0 {
		return "", "", false
	}
	left := strings.TrimSpace(title[:cut])
	right := strings.TrimSpace(title[cut+width:])
	return left, right, left != "" && right != ""
}

// stripTail cuts an unbracketed "ft. X" off the title and trailing tempo
// words ("Song slowed + reverb"), reporting the latter as a variant.
func stripTail(title string) (string, bool) {
	lower := strings.ToLower(title)
	for _, marker := range []string{" feat. ", " feat ", " featuring ", " ft. ", " ft "} {
		if index := strings.Index(lower, marker); index > 0 {
			title, lower = title[:index], lower[:index]
		}
	}
	fields := strings.Fields(title)
	keep := len(fields)
	strong := false
	for keep > 1 {
		word := normalizeMatch(fields[keep-1])
		next := ""
		if keep < len(fields) {
			next = normalizeMatch(fields[keep])
		}
		if variantWord(word, next) {
			strong = true
		} else if word != "" && word != "up" {
			break
		}
		keep--
	}
	if !strong {
		return strings.Join(fields, " "), false
	}
	return strings.Join(fields[:keep], " "), true
}

// artistsOverlap reports whether two artist credits share an artist. An
// empty credit matches anything: the title then decides alone.
func artistsOverlap(want, got string) bool {
	wantNames, gotNames := splitArtists(want), splitArtists(got)
	if len(wantNames) == 0 || len(gotNames) == 0 {
		return true
	}
	for _, name := range wantNames {
		for _, other := range gotNames {
			if name == other {
				return true
			}
		}
	}
	return false
}

// splitArtists lists every credited artist, folded to letters and digits
// without spaces so "TaylorSwift" and "Taylor Swift" agree.
func splitArtists(value string) []string {
	lower := " " + strings.ToLower(value) + " "
	for _, separator := range []string{" feat. ", " feat ", " featuring ", " ft. ", " ft ", " & ", ",", ";", " x ", "、", "/", " with ", " vs. ", " vs "} {
		lower = strings.ReplaceAll(lower, separator, "\x00")
	}
	var names []string
	whole := strings.ReplaceAll(normalizeMatch(value), " ", "")
	if whole != "" {
		names = append(names, whole)
	}
	for _, part := range strings.Split(lower, "\x00") {
		if name := strings.ReplaceAll(normalizeMatch(part), " ", ""); name != "" && name != whole {
			names = append(names, name)
		}
	}
	return names
}
