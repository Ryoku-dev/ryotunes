package romanize

import "testing"

func TestLine(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
		ok   bool
	}{
		{name: "latin", in: "Hello, world! 123", want: "Hello, world! 123", ok: false},
		{name: "korean greeting", in: "안녕하세요", want: "annyeonghaseyo", ok: true},
		{name: "korean assimilation", in: "국내 신라 좋아", want: "gungnae silla joa", ok: true},
		{name: "korean mixed latin", in: "너를 사랑해 baby", want: "neoreul saranghae baby", ok: true},
		{name: "chinese", in: "我爱你", want: "wo ai ni", ok: true},
		{name: "chinese punctuation", in: "「月亮代表我的心」", want: "「yue liang dai biao wo de xin」", ok: true},
		{name: "chinese mixed latin", in: "爱你baby", want: "ai ni baby", ok: true},
		{name: "russian", in: "Я тебя люблю", want: "Ya tebya lyublyu", ok: true},
		{name: "russian genitive", in: "Его нет, много дней", want: "Yevo net, mnogo dney", ok: true},
		{name: "ukrainian", in: "Слава Україні, моя земля", want: "Slava Ukrayini, moya zemlia", ok: true},
		{name: "serbian", in: "Љубав и живот", want: "Ljubav i život", ok: true},
		{name: "macedonian", in: "Сѐ што имам и куќа", want: "Se shto imam i kukja", ok: true},
		{name: "bulgarian", in: "Обичам те, България", want: "Obicham te, Balgariya", ok: true},
		{name: "belarusian", in: "Я ўсё ведаю", want: "Ya wsyo vedayu", ok: true},
		{name: "kyrgyz", in: "Өмүр жашоо", want: "Ömür zhashoo", ok: true},
		{name: "devanagari schwa", in: "कमल", want: "kamal", ok: true},
		{name: "devanagari sentence", in: "मैं तुमसे प्यार करता हूँ", want: "main tumse pyaar kartaa hoon", ok: true},
		{name: "devanagari nukta and virama", in: "फ़िल्म", want: "film", ok: true},
		{name: "gurmukhi", in: "ਸਤ ਸ੍ਰੀ ਅਕਾਲ", want: "sat sree akaal", ok: true},
		{name: "gurmukhi addak", in: "ਪੱਕਾ", want: "pakkaa", ok: true},
		{name: "gurmukhi carriers", in: "ਉਹ ਇੱਥੇ ਹੈ", want: "uh itthe hai", ok: true},
		{name: "mixed runs", in: "사랑해 / 我爱你 / Привет", want: "saranghae / wo ai ni / Privet", ok: true},
		{name: "japanese", in: "君の名は", want: "君の名は", ok: false},
		{name: "kana", in: "ありがとう", want: "ありがとう", ok: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := Line(test.in)
			if got != test.want || ok != test.ok {
				t.Fatalf("Line(%q) = (%q, %v), want (%q, %v)", test.in, got, ok, test.want, test.ok)
			}
		})
	}
}

func TestNeeds(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{in: "Plain text 123", want: false},
		{in: "사랑해", want: true},
		{in: "我爱你", want: true},
		{in: "Привет", want: true},
		{in: "नमस्ते", want: true},
		{in: "ਸਤ", want: true},
		{in: "君の名は", want: false},
		{in: "カタカナ", want: false},
	}
	for _, test := range tests {
		if got := Needs(test.in); got != test.want {
			t.Errorf("Needs(%q) = %v, want %v", test.in, got, test.want)
		}
	}
}

func TestLinePreservesSpacingAndPunctuation(t *testing.T) {
	got, ok := Line("你好，世界! 42")
	if !ok || got != "ni hao，shi jie! 42" {
		t.Fatalf("Line punctuation = (%q, %v)", got, ok)
	}
}
