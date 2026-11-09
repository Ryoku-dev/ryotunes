package romanize

import (
	"strings"
	"unicode"
)

type cyrillicLanguage uint8

const (
	russian cyrillicLanguage = iota
	ukrainian
	belarusian
	serbian
	macedonian
	bulgarian
	kyrgyz
)

func detectCyrillicLanguage(text string) cyrillicLanguage {
	var hasKyrgyz, hasMacedonian, hasSerbian, hasBelarusian, hasUkrainian bool
	var hasRussianMarker, hasHardSign bool
	for _, r := range text {
		switch {
		case strings.ContainsRune("ҢңӨөҮү", r):
			hasKyrgyz = true
		case strings.ContainsRune("ЃѓЅѕЌќЀѐЍѝ", r):
			hasMacedonian = true
		case strings.ContainsRune("ЂђЋћЈјЉљЊњЏџ", r):
			hasSerbian = true
		case strings.ContainsRune("Ўў", r):
			hasBelarusian = true
		case strings.ContainsRune("ҐґЄєІіЇї", r):
			hasUkrainian = true
		case strings.ContainsRune("ЫыЭэЁё", r):
			hasRussianMarker = true
		case strings.ContainsRune("Ъъ", r):
			hasHardSign = true
		}
	}
	switch {
	case hasKyrgyz:
		return kyrgyz
	case hasMacedonian:
		return macedonian
	case hasSerbian:
		return serbian
	case hasBelarusian:
		return belarusian
	case hasUkrainian:
		return ukrainian
	case !hasRussianMarker && hasHardSign:
		return bulgarian
	default:
		return russian
	}
}

func romanizeCyrillic(text string, language cyrillicLanguage) string {
	var out strings.Builder
	out.Grow(len(text) * 2)
	word := make([]rune, 0, 16)
	flush := func() {
		romanizeCyrillicWord(&out, word, language)
		word = word[:0]
	}
	for _, r := range text {
		if r >= 0x0400 && r <= 0x04FF && unicode.IsLetter(r) {
			word = append(word, r)
			continue
		}
		flush()
		out.WriteRune(r)
	}
	flush()
	return out.String()
}

func romanizeCyrillicWord(out *strings.Builder, word []rune, language cyrillicLanguage) {
	if len(word) == 0 {
		return
	}
	end := len(word)
	genitive := false
	if language == russian && len(word) >= 3 {
		lower := strings.ToLower(string(word))
		if (strings.HasSuffix(lower, "ого") || strings.HasSuffix(lower, "его")) && !keepsRussianG(lower) {
			end -= 2
			genitive = true
		}
	}
	for i, r := range word[:end] {
		out.WriteString(cyrillicRune(language, word, i, r))
	}
	if genitive {
		out.WriteString("vo")
	}
}

func keepsRussianG(word string) bool {
	switch word {
	case "ого", "много", "немного", "дорого", "строго", "убого", "полого":
		return true
	default:
		return false
	}
}

func cyrillicRune(language cyrillicLanguage, word []rune, index int, r rune) string {
	if (language == russian || language == belarusian) && index == 0 {
		switch r {
		case 'е':
			return "ye"
		case 'Е':
			return "Ye"
		}
	}
	if language == ukrainian && index > 0 && isCyrillicConsonant(word[index-1]) {
		switch r {
		case 'Ю':
			return "Iu"
		case 'ю':
			return "iu"
		case 'Я':
			return "Ia"
		case 'я':
			return "ia"
		}
	}
	if reading, ok := languageCyrillicRune(language, r); ok {
		return reading
	}
	if reading, ok := generalCyrillicRune(r); ok {
		return reading
	}
	return string(r)
}

func isCyrillicConsonant(r rune) bool {
	return unicode.IsLetter(r) && !strings.ContainsRune("АаЕеЄєИиІіЇїОоУуЮюЯяЫыЭэ", r)
}

func languageCyrillicRune(language cyrillicLanguage, r rune) (string, bool) {
	switch language {
	case ukrainian:
		switch r {
		case 'Г':
			return "H", true
		case 'г':
			return "h", true
		case 'Ґ':
			return "G", true
		case 'ґ':
			return "g", true
		case 'Є':
			return "Ye", true
		case 'є':
			return "ye", true
		case 'І':
			return "I", true
		case 'і':
			return "i", true
		case 'Ї':
			return "Yi", true
		case 'ї':
			return "yi", true
		}
	case belarusian:
		switch r {
		case 'Г':
			return "H", true
		case 'г':
			return "h", true
		case 'Ў':
			return "W", true
		case 'ў':
			return "w", true
		}
	case serbian:
		switch r {
		case 'Ж':
			return "Ž", true
		case 'Љ':
			return "Lj", true
		case 'Њ':
			return "Nj", true
		case 'Ц':
			return "C", true
		case 'Ч':
			return "Č", true
		case 'Џ':
			return "Dž", true
		case 'Ш':
			return "Š", true
		case 'Х':
			return "H", true
		case 'ж':
			return "ž", true
		case 'љ':
			return "lj", true
		case 'њ':
			return "nj", true
		case 'ц':
			return "c", true
		case 'ч':
			return "č", true
		case 'џ':
			return "dž", true
		case 'ш':
			return "š", true
		case 'х':
			return "h", true
		}
	case macedonian:
		switch r {
		case 'Ѓ':
			return "Gj", true
		case 'Ѕ':
			return "Dz", true
		case 'И':
			return "I", true
		case 'Ј':
			return "J", true
		case 'Љ':
			return "Lj", true
		case 'Њ':
			return "Nj", true
		case 'Ќ':
			return "Kj", true
		case 'Џ':
			return "Dž", true
		case 'Ч':
			return "Č", true
		case 'Ш':
			return "Sh", true
		case 'Ж':
			return "Zh", true
		case 'Ц':
			return "C", true
		case 'Х':
			return "H", true
		case 'ѓ':
			return "gj", true
		case 'ѕ':
			return "dz", true
		case 'и':
			return "i", true
		case 'ј':
			return "j", true
		case 'љ':
			return "lj", true
		case 'њ':
			return "nj", true
		case 'ќ':
			return "kj", true
		case 'џ':
			return "dž", true
		case 'ч':
			return "č", true
		case 'ш':
			return "sh", true
		case 'ж':
			return "zh", true
		case 'ц':
			return "c", true
		case 'х':
			return "h", true
		case 'Ѐ':
			return "E", true
		case 'ѐ':
			return "e", true
		case 'Ѝ':
			return "I", true
		case 'ѝ':
			return "i", true
		}
	case bulgarian:
		switch r {
		case 'Ж':
			return "Zh", true
		case 'Ц':
			return "Ts", true
		case 'Ч':
			return "Ch", true
		case 'Ш':
			return "Sh", true
		case 'Щ':
			return "Sht", true
		case 'Ъ':
			return "A", true
		case 'Ь':
			return "Y", true
		case 'Ю':
			return "Yu", true
		case 'Я':
			return "Ya", true
		case 'ж':
			return "zh", true
		case 'ц':
			return "ts", true
		case 'ч':
			return "ch", true
		case 'ш':
			return "sh", true
		case 'щ':
			return "sht", true
		case 'ъ':
			return "a", true
		case 'ь':
			return "y", true
		case 'ю':
			return "yu", true
		case 'я':
			return "ya", true
		}
	case kyrgyz:
		switch r {
		case 'Ү':
			return "Ü", true
		case 'ү':
			return "ü", true
		case 'Ы':
			return "Y", true
		case 'ы':
			return "y", true
		}
	}
	return "", false
}

func generalCyrillicRune(r rune) (string, bool) {
	switch r {
	case 'А':
		return "A", true
	case 'Б':
		return "B", true
	case 'В':
		return "V", true
	case 'Г':
		return "G", true
	case 'Ґ':
		return "G", true
	case 'Д':
		return "D", true
	case 'Ѓ':
		return "Ǵ", true
	case 'Ђ':
		return "Đ", true
	case 'Е':
		return "E", true
	case 'Ё':
		return "Yo", true
	case 'Є':
		return "Ye", true
	case 'Ж':
		return "Zh", true
	case 'З':
		return "Z", true
	case 'Ѕ':
		return "Dz", true
	case 'И', 'І':
		return "I", true
	case 'Ї':
		return "Yi", true
	case 'Й', 'Ј':
		return "Y", true
	case 'К':
		return "K", true
	case 'Л':
		return "L", true
	case 'Љ':
		return "Ly", true
	case 'М':
		return "M", true
	case 'Н':
		return "N", true
	case 'Њ':
		return "Ny", true
	case 'О':
		return "O", true
	case 'П':
		return "P", true
	case 'Р':
		return "R", true
	case 'С':
		return "S", true
	case 'Т':
		return "T", true
	case 'Ћ':
		return "Ć", true
	case 'У':
		return "U", true
	case 'Ў':
		return "Ŭ", true
	case 'Ф':
		return "F", true
	case 'Х':
		return "Kh", true
	case 'Ц':
		return "Ts", true
	case 'Ч':
		return "Ch", true
	case 'Џ':
		return "Dž", true
	case 'Ш':
		return "Sh", true
	case 'Щ':
		return "Shch", true
	case 'Ъ':
		return "ʺ", true
	case 'Ы':
		return "Y", true
	case 'Ь':
		return "ʹ", true
	case 'Э':
		return "E", true
	case 'Ю':
		return "Yu", true
	case 'Я':
		return "Ya", true
	case 'Ѡ':
		return "O", true
	case 'Ѣ', 'Ѧ', 'Ѩ':
		return "Ya", true
	case 'Ѥ':
		return "Ye", true
	case 'Ѫ':
		return "U", true
	case 'Ѭ':
		return "Yu", true
	case 'Ѯ':
		return "Ks", true
	case 'Ѱ':
		return "Ps", true
	case 'Ѳ':
		return "F", true
	case 'Ѵ', 'Ѷ':
		return "I", true
	case 'Ғ':
		return "Gh", true
	case 'Ҕ':
		return "G", true
	case 'Җ':
		return "Zh", true
	case 'Ҙ':
		return "Dz", true
	case 'Қ':
		return "Q", true
	case 'Ҝ', 'Ҟ', 'Ҡ':
		return "K", true
	case 'Ң', 'Ҥ':
		return "Ng", true
	case 'Ҧ':
		return "P", true
	case 'Ҩ':
		return "O", true
	case 'Ҫ':
		return "S", true
	case 'Ҭ':
		return "T", true
	case 'Ү', 'Ұ':
		return "U", true
	case 'Ҳ':
		return "Kh", true
	case 'Ҵ':
		return "Ts", true
	case 'Ҷ', 'Ҹ':
		return "Ch", true
	case 'Һ':
		return "H", true
	case 'Ҽ', 'Ҿ':
		return "Ch", true
	case 'Ќ':
		return "Ḱ", true
	case 'Ө':
		return "Ö", true
	case 'а':
		return "a", true
	case 'б':
		return "b", true
	case 'в':
		return "v", true
	case 'г', 'ґ':
		return "g", true
	case 'д':
		return "d", true
	case 'ѓ':
		return "ǵ", true
	case 'ђ':
		return "đ", true
	case 'е':
		return "e", true
	case 'ё':
		return "yo", true
	case 'є':
		return "ye", true
	case 'ж':
		return "zh", true
	case 'з':
		return "z", true
	case 'ѕ':
		return "dz", true
	case 'и', 'і':
		return "i", true
	case 'ї':
		return "yi", true
	case 'й', 'ј':
		return "y", true
	case 'к':
		return "k", true
	case 'л':
		return "l", true
	case 'љ':
		return "ly", true
	case 'м':
		return "m", true
	case 'н':
		return "n", true
	case 'њ':
		return "ny", true
	case 'о':
		return "o", true
	case 'п':
		return "p", true
	case 'р':
		return "r", true
	case 'с':
		return "s", true
	case 'т':
		return "t", true
	case 'ћ':
		return "ć", true
	case 'у':
		return "u", true
	case 'ў':
		return "ŭ", true
	case 'ф':
		return "f", true
	case 'х':
		return "kh", true
	case 'ц':
		return "ts", true
	case 'ч':
		return "ch", true
	case 'џ':
		return "dž", true
	case 'ш':
		return "sh", true
	case 'щ':
		return "shch", true
	case 'ъ':
		return "ʺ", true
	case 'ы':
		return "y", true
	case 'ь':
		return "ʹ", true
	case 'э':
		return "e", true
	case 'ю':
		return "yu", true
	case 'я':
		return "ya", true
	case 'ѡ':
		return "o", true
	case 'ѣ', 'ѧ', 'ѩ':
		return "ya", true
	case 'ѥ':
		return "ye", true
	case 'ѫ':
		return "u", true
	case 'ѭ':
		return "yu", true
	case 'ѯ':
		return "ks", true
	case 'ѱ':
		return "ps", true
	case 'ѳ':
		return "f", true
	case 'ѵ', 'ѷ':
		return "i", true
	case 'ғ':
		return "gh", true
	case 'ҕ':
		return "g", true
	case 'җ':
		return "zh", true
	case 'ҙ':
		return "dz", true
	case 'қ':
		return "q", true
	case 'ҝ', 'ҟ', 'ҡ':
		return "k", true
	case 'ң', 'ҥ':
		return "ng", true
	case 'ҧ':
		return "p", true
	case 'ҩ':
		return "o", true
	case 'ҫ':
		return "s", true
	case 'ҭ':
		return "t", true
	case 'ү', 'ұ':
		return "u", true
	case 'ҳ':
		return "kh", true
	case 'ҵ':
		return "ts", true
	case 'ҷ', 'ҹ':
		return "ch", true
	case 'һ':
		return "h", true
	case 'ҽ', 'ҿ':
		return "ch", true
	case 'ќ':
		return "ḱ", true
	case 'ө':
		return "ö", true
	default:
		return "", false
	}
}
