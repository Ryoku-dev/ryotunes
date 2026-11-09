package romanize

import (
	"strings"
	"unicode"

	pinyin "github.com/mozillazg/go-pinyin"
)

// Chinese readings come from the MIT-licensed go-pinyin dictionary. Its first reading is
// authoritative for polyphonic characters; this package deliberately carries no override list.
var chineseArgs = func() pinyin.Args {
	args := pinyin.NewArgs()
	args.Style = pinyin.Normal
	args.Heteronym = false
	return args
}()

func romanizeChinese(text string) string {
	var out strings.Builder
	out.Grow(len(text) * 2)
	var last rune
	haveLast := false
	afterSyllable := false
	writeRune := func(r rune) {
		out.WriteRune(r)
		last = r
		haveLast = true
	}
	writeString := func(s string) {
		out.WriteString(s)
		for _, r := range s {
			last = r
			haveLast = true
		}
	}

	for _, r := range text {
		if isHan(r) {
			readings := pinyin.SinglePinyin(r, chineseArgs)
			if len(readings) > 0 {
				if haveLast && !unicode.IsSpace(last) && !isOpeningPunctuation(last) && !(last > unicode.MaxASCII && isClosingPunctuation(last)) {
					writeRune(' ')
				}
				writeString(readings[0])
				afterSyllable = true
				continue
			}
		}
		if afterSyllable && !unicode.IsSpace(r) && !isClosingPunctuation(r) {
			writeRune(' ')
		}
		writeRune(r)
		afterSyllable = false
	}
	return out.String()
}

func isOpeningPunctuation(r rune) bool {
	return strings.ContainsRune("([{（《〈【『「“‘", r)
}

func isClosingPunctuation(r rune) bool {
	return strings.ContainsRune(",.!?;:)]}，。！？；：、）》〉】』」…", r)
}
