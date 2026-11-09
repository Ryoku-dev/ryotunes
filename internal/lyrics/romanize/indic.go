package romanize

import "strings"

type indicEntry struct {
	key   string
	latin string
}

type vowelSign struct {
	rune  rune
	latin string
}

type carrier struct {
	rune  rune
	alone string
}

type indicScript struct {
	consonants []indicEntry
	vowelSigns []vowelSign
	others     []indicEntry
	virama     rune
	gemination rune
	carriers   []carrier
	modifiers  string
	vowels     string
}

type indicVowel uint8

const (
	inherent indicVowel = iota
	explicit
	dead
)

type indicSyllable struct {
	consonant string
	vowel     indicVowel
	sign      string
	tail      string
	doubled   bool
	silent    bool
}

func (s indicSyllable) voiced() bool {
	return s.vowel != dead
}

func romanizeIndic(text string, script indicScript) string {
	chars := []rune(text)
	var out strings.Builder
	out.Grow(len(text))
	word := make([]indicSyllable, 0, 16)
	doubleNext := false
	for i := 0; i < len(chars); {
		r := chars[i]
		if length, latin, ok := longestIndic(chars[i:], script.consonants); ok {
			i += length
			syllable := indicSyllable{consonant: latin, vowel: inherent, doubled: doubleNext}
			doubleNext = false
			if i < len(chars) && chars[i] == script.virama {
				syllable.vowel = dead
				i++
			} else if i < len(chars) {
				if sign, ok := findVowelSign(script.vowelSigns, chars[i]); ok {
					syllable.vowel = explicit
					syllable.sign = sign
					i++
				}
			}
			word = append(word, syllable)
			continue
		}
		if alone, ok := findCarrier(script.carriers, r); ok {
			i++
			if i < len(chars) {
				if sign, found := findVowelSign(script.vowelSigns, chars[i]); found {
					alone = sign
					i++
				}
			}
			word = append(word, indicSyllable{vowel: explicit, sign: alone})
			continue
		}
		if script.gemination != 0 && r == script.gemination {
			doubleNext = true
			i++
			continue
		}
		if sign, ok := findVowelSign(script.vowelSigns, r); ok {
			word = append(word, indicSyllable{vowel: explicit, sign: sign})
			i++
			continue
		}
		if length, latin, ok := longestIndic(chars[i:], script.others); ok {
			i += length
			switch {
			case strings.ContainsRune(script.modifiers, r) && len(word) > 0 && word[len(word)-1].tail == "":
				word[len(word)-1].tail = latin
			case strings.ContainsRune(script.vowels, r):
				word = append(word, indicSyllable{vowel: explicit, sign: latin})
			default:
				flushIndicWord(&out, word)
				word = word[:0]
				out.WriteString(latin)
			}
			continue
		}
		flushIndicWord(&out, word)
		word = word[:0]
		out.WriteRune(r)
		i++
	}
	flushIndicWord(&out, word)
	return out.String()
}

func flushIndicWord(out *strings.Builder, word []indicSyllable) {
	for i := len(word) - 1; i >= 0; i-- {
		syllable := &word[i]
		if syllable.vowel != inherent || syllable.consonant == "" || syllable.tail != "" {
			continue
		}
		if i == len(word)-1 {
			syllable.silent = len(word) > 1
		} else {
			syllable.silent = i > 0 && word[i-1].voiced() && word[i+1].consonant != "" && word[i+1].voiced() && !word[i+1].silent
		}
	}
	for i := range word {
		syllable := &word[i]
		if syllable.doubled && syllable.consonant != "" {
			out.WriteByte(syllable.consonant[0])
		}
		out.WriteString(syllable.consonant)
		switch syllable.vowel {
		case inherent:
			if !syllable.silent {
				out.WriteByte('a')
			}
		case explicit:
			out.WriteString(syllable.sign)
		}
		out.WriteString(syllable.tail)
	}
}

func longestIndic(chars []rune, entries []indicEntry) (int, string, bool) {
	bestLength := 0
	bestLatin := ""
	for _, entry := range entries {
		matched := 0
		for _, r := range entry.key {
			if matched >= len(chars) || chars[matched] != r {
				matched = 0
				break
			}
			matched++
		}
		if matched > bestLength {
			bestLength = matched
			bestLatin = entry.latin
		}
	}
	return bestLength, bestLatin, bestLength > 0
}

func findVowelSign(signs []vowelSign, r rune) (string, bool) {
	for _, sign := range signs {
		if sign.rune == r {
			return sign.latin, true
		}
	}
	return "", false
}

func findCarrier(carriers []carrier, r rune) (string, bool) {
	for _, item := range carriers {
		if item.rune == r {
			return item.alone, true
		}
	}
	return "", false
}

var devanagari = indicScript{
	consonants: []indicEntry{
		{"क", "k"}, {"ख", "kh"}, {"ग", "g"}, {"घ", "gh"}, {"ङ", "ng"},
		{"च", "ch"}, {"छ", "chh"}, {"ज", "j"}, {"झ", "jh"}, {"ञ", "ny"},
		{"ट", "t"}, {"ठ", "th"}, {"ड", "d"}, {"ढ", "dh"}, {"ण", "n"},
		{"त", "t"}, {"थ", "th"}, {"द", "d"}, {"ध", "dh"}, {"न", "n"},
		{"प", "p"}, {"फ", "ph"}, {"ब", "b"}, {"भ", "bh"}, {"म", "m"},
		{"य", "y"}, {"र", "r"}, {"ल", "l"}, {"व", "v"}, {"श", "sh"},
		{"ष", "sh"}, {"स", "s"}, {"ह", "h"}, {"क्ष", "ksh"}, {"त्र", "tr"},
		{"ज्ञ", "gy"}, {"श्र", "shr"}, {"क़", "q"}, {"ख़", "kh"}, {"ग़", "g"},
		{"ज़", "z"}, {"ड़", "r"}, {"ढ़", "rh"}, {"फ़", "f"}, {"य़", "y"},
	},
	vowelSigns: []vowelSign{
		{'ा', "aa"}, {'ि', "i"}, {'ी', "ee"}, {'ु', "u"}, {'ू', "oo"},
		{'ृ', "ri"}, {'े', "e"}, {'ै', "ai"}, {'ो', "o"}, {'ौ', "au"},
	},
	others: []indicEntry{
		{"अ", "a"}, {"आ", "aa"}, {"इ", "i"}, {"ई", "ee"}, {"उ", "u"},
		{"ऊ", "oo"}, {"ऋ", "ri"}, {"ए", "e"}, {"ऐ", "ai"}, {"ओ", "o"},
		{"औ", "au"}, {"ं", "n"}, {"ः", "h"}, {"ँ", "n"}, {"़", ""},
		{"्", ""}, {"०", "0"}, {"१", "1"}, {"२", "2"}, {"३", "3"},
		{"४", "4"}, {"५", "5"}, {"६", "6"}, {"७", "7"}, {"८", "8"},
		{"९", "9"}, {"ॐ", "Om"}, {"ऽ", ""}, {"।", "."}, {"॥", "."},
	},
	virama:    '्',
	modifiers: "ंँः",
	vowels:    "अआइईउऊऋएऐओऔ",
}

var gurmukhi = indicScript{
	consonants: []indicEntry{
		{"ਸ", "s"}, {"ਹ", "h"}, {"ਕ", "k"}, {"ਖ", "kh"}, {"ਗ", "g"},
		{"ਘ", "gh"}, {"ਙ", "ng"}, {"ਚ", "ch"}, {"ਛ", "chh"}, {"ਜ", "j"},
		{"ਝ", "jh"}, {"ਞ", "ny"}, {"ਟ", "t"}, {"ਠ", "th"}, {"ਡ", "d"},
		{"ਢ", "dh"}, {"ਣ", "n"}, {"ਤ", "t"}, {"ਥ", "th"}, {"ਦ", "d"},
		{"ਧ", "dh"}, {"ਨ", "n"}, {"ਪ", "p"}, {"ਫ", "ph"}, {"ਬ", "b"},
		{"ਭ", "bh"}, {"ਮ", "m"}, {"ਯ", "y"}, {"ਰ", "r"}, {"ਲ", "l"},
		{"ਵ", "v"}, {"ੜ", "r"}, {"ਸ਼", "sh"}, {"ਖ਼", "kh"}, {"ਗ਼", "g"},
		{"ਜ਼", "z"}, {"ਫ਼", "f"}, {"ਲ਼", "l"},
	},
	vowelSigns: []vowelSign{
		{'ਾ', "aa"}, {'ਿ', "i"}, {'ੀ', "ee"}, {'ੁ', "u"}, {'ੂ', "oo"},
		{'ੇ', "e"}, {'ੈ', "ai"}, {'ੋ', "o"}, {'ੌ', "au"},
	},
	others: []indicEntry{
		{"ਅ", "a"}, {"ਆ", "aa"}, {"ਇ", "i"}, {"ਈ", "ee"}, {"ਉ", "u"},
		{"ਊ", "oo"}, {"ਏ", "e"}, {"ਐ", "ai"}, {"ਓ", "o"}, {"ਔ", "au"},
		{"ੰ", "n"}, {"ਂ", "n"}, {"੍", ""}, {"਼", ""}, {"ੴ", "Ek Onkar"},
		{"੦", "0"}, {"੧", "1"}, {"੨", "2"}, {"੩", "3"}, {"੪", "4"},
		{"੫", "5"}, {"੬", "6"}, {"੭", "7"}, {"੮", "8"}, {"੯", "9"},
	},
	virama:     '੍',
	gemination: 'ੱ',
	carriers:   []carrier{{'ੳ', "o"}, {'ੲ', "e"}},
	modifiers:  "ੰਂ",
	vowels:     "ਅਆਇਈਉਊਏਐਓਔ",
}
