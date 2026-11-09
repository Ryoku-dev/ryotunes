// Package romanize converts supported lyric scripts to readable Latin text.
//
// Ported from PeekMusic/PeekMusic (app/src/main/kotlin/com/metrolist/music/lyrics/LyricsUtils.kt, GPL-3.0), commit 83217188a, itself a fork of MetrolistGroup/Metrolist.
package romanize

import (
	"unicode"
	"unicode/utf8"
)

// Line romanizes each supported script run, preserving Japanese text identified by kana because its kanji require a morphological dictionary.
func Line(s string) (string, bool) {
	info := inspectScripts(s)
	if s == "" || !info.needsRomanization() {
		return s, false
	}

	japanese := info.kana > 0 && info.kana*10 >= info.han+info.kana
	out := s
	if info.hangul {
		out = romanizeKorean(out)
	}
	if info.han > 0 && !japanese {
		out = romanizeChinese(out)
	}
	if info.cyrillic {
		out = romanizeCyrillic(out, detectCyrillicLanguage(s))
	}
	if info.devanagari {
		out = romanizeIndic(out, devanagari)
	}
	if info.gurmukhi {
		out = romanizeIndic(out, gurmukhi)
	}
	if sameText(out, s) {
		return s, false
	}
	return out, true
}

// Needs reports whether a line contains a supported script that may need romanization.
func Needs(s string) bool {
	return inspectScripts(s).needsRomanization()
}

type scriptInfo struct {
	han        int
	kana       int
	hangul     bool
	cyrillic   bool
	devanagari bool
	gurmukhi   bool
}

func inspectScripts(s string) scriptInfo {
	var info scriptInfo
	for _, r := range s {
		switch {
		case isHan(r):
			info.han++
		case r >= 0x3040 && r <= 0x30FF:
			info.kana++
		case r >= 0xAC00 && r <= 0xD7A3:
			info.hangul = true
		case r >= 0x0400 && r <= 0x04FF:
			info.cyrillic = true
		case r >= 0x0900 && r <= 0x097F:
			info.devanagari = true
		case r >= 0x0A00 && r <= 0x0A7F:
			info.gurmukhi = true
		}
	}
	return info
}

func (i scriptInfo) needsRomanization() bool {
	if i.hangul || i.cyrillic || i.devanagari || i.gurmukhi {
		return true
	}
	return i.han > 0 && (i.kana == 0 || i.kana*10 < i.han+i.kana)
}

func isHan(r rune) bool {
	return r >= 0x4E00 && r <= 0x9FFF || r >= 0x3400 && r <= 0x4DBF
}

func sameText(a, b string) bool {
	var ai, bi int
	for {
		ar, aok := nextNormalizedRune(a, &ai)
		br, bok := nextNormalizedRune(b, &bi)
		if aok != bok || ar != br {
			return false
		}
		if !aok {
			return true
		}
	}
}

func nextNormalizedRune(s string, offset *int) (rune, bool) {
	for *offset < len(s) {
		r, size := utf8.DecodeRuneInString(s[*offset:])
		*offset += size
		if !unicode.IsSpace(r) {
			return unicode.ToLower(r), true
		}
	}
	return 0, false
}
