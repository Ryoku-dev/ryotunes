package romanize

import "strings"

var koreanInitial = [...]string{
	"g", "kk", "n", "d", "tt", "r", "m", "b", "pp", "s", "ss", "", "j", "jj", "ch", "k", "t", "p", "h",
}

var koreanVowel = [...]string{
	"a", "ae", "ya", "yae", "eo", "e", "yeo", "ye", "o", "wa", "wae", "oe", "yo", "u", "wo", "we", "wi", "yu", "eu", "eui", "i",
}

var koreanFinal = [...]string{
	"", "k", "kk", "k", "n", "n", "n", "t", "l", "k", "m", "p", "t", "t", "p", "l", "m", "p", "p", "t", "t", "ng", "t", "t", "k", "t", "p", "t",
}

func romanizeKorean(text string) string {
	var out strings.Builder
	out.Grow(len(text))
	pendingFinal := 0
	for _, r := range text {
		if r < 0xAC00 || r > 0xD7A3 {
			if pendingFinal != 0 {
				out.WriteString(koreanFinal[pendingFinal])
				pendingFinal = 0
			}
			out.WriteRune(r)
			continue
		}

		index := int(r - 0xAC00)
		initial := index / (21 * 28)
		vowel := index % (21 * 28) / 28
		final := index % 28
		skipInitial := false
		if pendingFinal != 0 {
			if merged, ok := koreanFinalBefore(pendingFinal, initial); ok {
				out.WriteString(merged)
				skipInitial = true
			} else {
				out.WriteString(koreanFinal[pendingFinal])
			}
		}
		if !skipInitial {
			out.WriteString(koreanInitial[initial])
		}
		out.WriteString(koreanVowel[vowel])
		pendingFinal = final
	}
	if pendingFinal != 0 {
		out.WriteString(koreanFinal[pendingFinal])
	}
	return out.String()
}

func koreanFinalBefore(final, nextInitial int) (string, bool) {
	switch [2]int{final, nextInitial} {
	case [2]int{1, 2}, [2]int{1, 5}:
		return "ngn", true
	case [2]int{1, 6}:
		return "ngm", true
	case [2]int{1, 11}:
		return "g", true
	case [2]int{1, 18}:
		return "kh", true
	case [2]int{2, 2}, [2]int{2, 5}:
		return "ngn", true
	case [2]int{2, 6}:
		return "ngm", true
	case [2]int{2, 11}:
		return "kk", true
	case [2]int{2, 18}:
		return "kh", true
	case [2]int{3, 2}, [2]int{3, 5}:
		return "ngn", true
	case [2]int{3, 6}:
		return "ngm", true
	case [2]int{3, 11}:
		return "ks", true
	case [2]int{3, 18}:
		return "kch", true
	case [2]int{4, 5}:
		return "ll", true
	case [2]int{5, 2}, [2]int{5, 5}:
		return "nn", true
	case [2]int{5, 6}:
		return "nm", true
	case [2]int{5, 11}:
		return "nj", true
	case [2]int{5, 18}:
		return "nch", true
	case [2]int{6, 5}:
		return "nn", true
	case [2]int{6, 11}:
		return "nh", true
	case [2]int{7, 2}, [2]int{7, 5}:
		return "nn", true
	case [2]int{7, 6}:
		return "nm", true
	case [2]int{7, 11}:
		return "d", true
	case [2]int{7, 18}:
		return "th", true
	case [2]int{8, 2}, [2]int{8, 5}:
		return "ll", true
	case [2]int{8, 11}:
		return "r", true
	case [2]int{9, 2}, [2]int{9, 5}:
		return "ngn", true
	case [2]int{9, 6}:
		return "ngm", true
	case [2]int{9, 11}:
		return "lg", true
	case [2]int{9, 18}:
		return "lkh", true
	case [2]int{10, 2}, [2]int{10, 5}:
		return "mn", true
	case [2]int{10, 6}:
		return "mm", true
	case [2]int{10, 11}:
		return "lm", true
	case [2]int{10, 18}:
		return "lmh", true
	case [2]int{11, 2}, [2]int{11, 5}:
		return "mn", true
	case [2]int{11, 6}:
		return "mm", true
	case [2]int{11, 11}:
		return "lb", true
	case [2]int{11, 18}:
		return "lph", true
	case [2]int{12, 2}, [2]int{12, 5}:
		return "nn", true
	case [2]int{12, 6}:
		return "nm", true
	case [2]int{12, 11}:
		return "ls", true
	case [2]int{12, 18}:
		return "lsh", true
	case [2]int{13, 2}, [2]int{13, 5}:
		return "nn", true
	case [2]int{13, 6}:
		return "nm", true
	case [2]int{13, 11}:
		return "lt", true
	case [2]int{13, 18}:
		return "lth", true
	case [2]int{14, 2}, [2]int{14, 5}:
		return "mn", true
	case [2]int{14, 6}:
		return "mm", true
	case [2]int{14, 11}:
		return "lp", true
	case [2]int{14, 18}:
		return "lph", true
	case [2]int{15, 2}, [2]int{15, 5}:
		return "ll", true
	case [2]int{15, 6}:
		return "lm", true
	case [2]int{15, 11}, [2]int{15, 18}:
		return "lh", true
	case [2]int{16, 5}:
		return "mn", true
	case [2]int{17, 2}, [2]int{17, 5}:
		return "mn", true
	case [2]int{17, 6}:
		return "mm", true
	case [2]int{17, 11}:
		return "b", true
	case [2]int{17, 18}:
		return "ph", true
	case [2]int{18, 2}, [2]int{18, 5}:
		return "mn", true
	case [2]int{18, 6}:
		return "mm", true
	case [2]int{18, 11}:
		return "ps", true
	case [2]int{18, 18}:
		return "psh", true
	case [2]int{19, 2}, [2]int{19, 5}:
		return "nn", true
	case [2]int{19, 6}:
		return "nm", true
	case [2]int{19, 11}:
		return "s", true
	case [2]int{19, 18}:
		return "sh", true
	case [2]int{20, 2}, [2]int{20, 5}:
		return "nn", true
	case [2]int{20, 6}:
		return "nm", true
	case [2]int{20, 11}:
		return "ss", true
	case [2]int{20, 18}:
		return "th", true
	case [2]int{22, 2}, [2]int{22, 5}:
		return "nn", true
	case [2]int{22, 6}:
		return "nm", true
	case [2]int{22, 11}:
		return "j", true
	case [2]int{22, 18}:
		return "ch", true
	case [2]int{23, 2}, [2]int{23, 5}:
		return "nn", true
	case [2]int{23, 6}:
		return "nm", true
	case [2]int{23, 11}, [2]int{23, 18}:
		return "ch", true
	case [2]int{24, 2}, [2]int{24, 5}:
		return "ngn", true
	case [2]int{24, 6}:
		return "ngm", true
	case [2]int{24, 11}:
		return "k", true
	case [2]int{24, 18}:
		return "kh", true
	case [2]int{25, 2}, [2]int{25, 5}:
		return "nn", true
	case [2]int{25, 6}:
		return "nm", true
	case [2]int{25, 11}:
		return "t", true
	case [2]int{25, 18}:
		return "th", true
	case [2]int{26, 2}, [2]int{26, 5}:
		return "mn", true
	case [2]int{26, 6}:
		return "mm", true
	case [2]int{26, 11}:
		return "p", true
	case [2]int{26, 18}:
		return "ph", true
	case [2]int{27, 0}:
		return "k", true
	case [2]int{27, 2}, [2]int{27, 5}:
		return "nn", true
	case [2]int{27, 6}:
		return "mm", true
	case [2]int{27, 11}:
		return "", true
	case [2]int{27, 18}:
		return "t", true
	default:
		return "", false
	}
}
