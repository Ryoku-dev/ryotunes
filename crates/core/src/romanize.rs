//! Lyrics romanization: a Latin reading under each line for scripts the listener may not read.
//!
//! Ported from PeekMusic's `LyricsUtils.kt` (itself built on Metrolist's), both GPL-3.0. Korean,
//! Cyrillic, Devanagari and Gurmukhi are table- and rule-based; Chinese uses the `pinyin` crate.
//! Japanese is deliberately not handled yet (it needs a morphological dictionary), so a Japanese
//! song is left untouched rather than read as Chinese.
//!
//! The script is decided once for the whole song, not per line: a line of only kanji inside a
//! Japanese song must not become pinyin, and a short Cyrillic line carries too few letters to
//! tell Russian from Ukrainian. Romanization runs when lyrics are handed to a client, never on
//! the cached or downloaded copy.

use pinyin::ToPinyin;

use crate::lyrics::Lyrics;
use crate::state::AppState;

/// The `get_lyrics` reply as a client should see it: romanized when `lyrics_romanize` is on.
pub fn for_client(state: &AppState, lyrics: Option<Lyrics>) -> Option<Lyrics> {
    let mut lyrics = lyrics?;
    if state.db.get_setting("lyrics_romanize").as_deref() == Some("true") {
        romanize_lyrics(&mut lyrics);
    }
    Some(lyrics)
}

/// Fill `romanized` on every line that has a reading different from its text.
pub fn romanize_lyrics(lyrics: &mut Lyrics) {
    let all: String =
        lyrics.lines.iter().map(|line| line.text.as_str()).collect::<Vec<_>>().join("\n");
    let Some(script) = detect(&all) else {
        return;
    };
    for line in &mut lyrics.lines {
        let romanized = romanize_line(script, &line.text);
        line.romanized = (!romanized.trim().is_empty() && !same_text(&romanized, &line.text))
            .then(|| romanized.trim().to_owned());
    }
}

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
enum Script {
    Korean,
    Chinese,
    Cyrillic(Cyrillic),
    Devanagari,
    Gurmukhi,
}

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
enum Cyrillic {
    Russian,
    Ukrainian,
    Belarusian,
    Serbian,
    Macedonian,
    Bulgarian,
    Kyrgyz,
}

fn is_hangul(c: char) -> bool {
    ('\u{AC00}'..='\u{D7A3}').contains(&c)
}

fn is_kana(c: char) -> bool {
    ('\u{3040}'..='\u{30FF}').contains(&c)
}

fn is_han(c: char) -> bool {
    ('\u{4E00}'..='\u{9FFF}').contains(&c) || ('\u{3400}'..='\u{4DBF}').contains(&c)
}

fn is_cyrillic(c: char) -> bool {
    ('\u{0400}'..='\u{04FF}').contains(&c)
}

fn is_devanagari(c: char) -> bool {
    ('\u{0900}'..='\u{097F}').contains(&c)
}

fn is_gurmukhi(c: char) -> bool {
    ('\u{0A00}'..='\u{0A7F}').contains(&c)
}

/// The song's dominant non-Latin script, or `None` when there is nothing to romanize (Latin-only
/// lyrics, or Japanese).
fn detect(text: &str) -> Option<Script> {
    let (mut hangul, mut kana, mut han, mut cyrillic, mut devanagari, mut gurmukhi) =
        (0usize, 0usize, 0usize, 0usize, 0usize, 0usize);
    for c in text.chars() {
        match c {
            c if is_hangul(c) => hangul += 1,
            c if is_kana(c) => kana += 1,
            c if is_han(c) => han += 1,
            c if is_cyrillic(c) => cyrillic += 1,
            c if is_devanagari(c) => devanagari += 1,
            c if is_gurmukhi(c) => gurmukhi += 1,
            _ => {}
        }
    }
    let cjk = han + kana;
    let best = [hangul, cjk, cyrillic, devanagari, gurmukhi].into_iter().max().unwrap_or(0);
    if best == 0 {
        return None;
    }
    if best == cjk {
        // Chinese below 10% kana among the CJK characters, as PeekMusic decides it; anything with
        // more kana is Japanese, which this module does not romanize yet.
        return (kana * 10 < cjk).then_some(Script::Chinese);
    }
    Some(if best == hangul {
        Script::Korean
    } else if best == cyrillic {
        Script::Cyrillic(cyrillic_language(text))
    } else if best == devanagari {
        Script::Devanagari
    } else {
        Script::Gurmukhi
    })
}

/// Pick the Cyrillic language from letters only it uses, most distinctive first. Text that shows
/// none of them reads as Russian; Bulgarian has no letter of its own, but it lacks ы/э/ё and
/// uses ъ as a vowel, so a ъ without those three is taken as Bulgarian.
fn cyrillic_language(text: &str) -> Cyrillic {
    let has = |set: &str| text.chars().any(|c| set.contains(c));
    if has("ҢңӨөҮү") {
        Cyrillic::Kyrgyz
    } else if has("ЃѓЅѕЌќЀѐЍѝ") {
        Cyrillic::Macedonian
    } else if has("ЂђЋћЈјЉљЊњЏџ") {
        Cyrillic::Serbian
    } else if has("Ўў") {
        Cyrillic::Belarusian
    } else if has("ҐґЄєІіЇї") {
        Cyrillic::Ukrainian
    } else if !has("ЫыЭэЁё") && has("Ъъ") {
        Cyrillic::Bulgarian
    } else {
        Cyrillic::Russian
    }
}

fn romanize_line(script: Script, text: &str) -> String {
    match script {
        Script::Korean => romanize_korean(text),
        Script::Chinese => romanize_chinese(text),
        Script::Cyrillic(language) => romanize_cyrillic(language, text),
        Script::Devanagari => romanize_indic(text, &DEVANAGARI),
        Script::Gurmukhi => romanize_indic(text, &GURMUKHI),
    }
}

/// Equal once spacing and case are ignored: such a reading adds nothing under the line.
fn same_text(a: &str, b: &str) -> bool {
    fn norm(s: &str) -> impl Iterator<Item = char> + '_ {
        s.chars().filter(|c| !c.is_whitespace()).flat_map(char::to_lowercase)
    }
    norm(a).eq(norm(b))
}

// --- Korean (Revised Romanization, with the final consonant read against the next initial) -----

fn romanize_korean(text: &str) -> String {
    let mut out = String::with_capacity(text.len());
    let chars: Vec<char> = text.chars().collect();
    // A final consonant can merge with the next syllable's initial, so it is written only once
    // the next character is known. `skip_cho` drops an initial the merged form already spelled.
    let mut pending_jong = 0usize;
    for &c in &chars {
        if is_hangul(c) {
            let index = c as usize - 0xAC00;
            let (cho, jung, jong) = (index / (21 * 28), (index % (21 * 28)) / 28, index % 28);
            let mut skip_cho = false;
            if pending_jong != 0 {
                match jong_before(pending_jong, cho) {
                    Some(merged) => {
                        out.push_str(merged);
                        skip_cho = true;
                    }
                    None => out.push_str(JONG[pending_jong]),
                }
            }
            if !skip_cho {
                out.push_str(CHO[cho]);
            }
            out.push_str(JUNG[jung]);
            pending_jong = jong;
        } else {
            if pending_jong != 0 {
                out.push_str(JONG[pending_jong]);
                pending_jong = 0;
            }
            out.push(c);
        }
    }
    if pending_jong != 0 {
        out.push_str(JONG[pending_jong]);
    }
    out
}

// --- Chinese (toneless pinyin, one space between syllables) -------------------------------------

fn romanize_chinese(text: &str) -> String {
    let mut out = String::with_capacity(text.len() * 3);
    let mut after_syllable = false;
    for (c, reading) in text.chars().zip(text.to_pinyin()) {
        match reading {
            Some(pinyin) => {
                // Full-width punctuation carries its own spacing, as PeekMusic writes it.
                let spaced = |c: char| {
                    c.is_whitespace()
                        || is_opening_punct(c)
                        || (!c.is_ascii() && is_closing_punct(c))
                };
                if !out.is_empty() && !out.ends_with(spaced) {
                    out.push(' ');
                }
                out.push_str(pinyin.plain());
                after_syllable = true;
            }
            None => {
                if after_syllable && !c.is_whitespace() && !is_closing_punct(c) {
                    out.push(' ');
                }
                out.push(c);
                after_syllable = false;
            }
        }
    }
    out
}

fn is_opening_punct(c: char) -> bool {
    "([{（《〈【『「“‘".contains(c)
}

fn is_closing_punct(c: char) -> bool {
    ",.!?;:)]}，。！？；：、）》〉】』」…".contains(c)
}

// --- Cyrillic ------------------------------------------------------------------------------------

fn romanize_cyrillic(language: Cyrillic, text: &str) -> String {
    let mut out = String::with_capacity(text.len() * 2);
    let mut word = String::new();
    for c in text.chars() {
        if c.is_alphabetic() {
            word.push(c);
        } else {
            cyrillic_word(language, &word, &mut out);
            word.clear();
            out.push(c);
        }
    }
    cyrillic_word(language, &word, &mut out);
    out
}

fn cyrillic_word(language: Cyrillic, word: &str, out: &mut String) {
    if word.is_empty() {
        return;
    }
    let chars: Vec<char> = word.chars().collect();
    let mut end = chars.len();
    // Russian genitive -ого/-его is sung -ovo/-evo ("его" → "yevo"); words like "много" or the
    // interjection "ого" keep their g.
    const KEEPS_G: [&str; 7] = ["ого", "много", "немного", "дорого", "строго", "убого", "полого"];
    let mut genitive = false;
    if language == Cyrillic::Russian && chars.len() >= 3 {
        let lower = word.to_lowercase();
        if (lower.ends_with("ого") || lower.ends_with("его")) && !KEEPS_G.contains(&lower.as_str())
        {
            end -= 2;
            genitive = true;
        }
    }
    for (i, &c) in chars[..end].iter().enumerate() {
        out.push_str(&cyrillic_char(language, &chars, i, c));
    }
    if genitive {
        out.push_str("vo");
    }
}

fn cyrillic_char(language: Cyrillic, chars: &[char], i: usize, c: char) -> String {
    let word_start = i == 0;
    match language {
        Cyrillic::Russian | Cyrillic::Belarusian if word_start && (c == 'е' || c == 'Е') => {
            return if c == 'е' { "ye" } else { "Ye" }.to_owned();
        }
        Cyrillic::Ukrainian if i > 0 && is_consonant(chars[i - 1]) => match c {
            'Ю' => return "Iu".to_owned(),
            'ю' => return "iu".to_owned(),
            'Я' => return "Ia".to_owned(),
            'я' => return "ia".to_owned(),
            _ => {}
        },
        _ => {}
    }
    let specific = match language {
        Cyrillic::Russian => None,
        Cyrillic::Ukrainian => ukrainian(c),
        Cyrillic::Belarusian => belarusian(c),
        Cyrillic::Serbian => serbian(c),
        Cyrillic::Macedonian => macedonian(c),
        Cyrillic::Bulgarian => bulgarian(c),
        Cyrillic::Kyrgyz => kyrgyz(c),
    };
    match specific.or_else(|| general_cyrillic(c)) {
        Some(latin) => latin.to_owned(),
        None => c.to_string(),
    }
}

fn is_consonant(c: char) -> bool {
    c.is_alphabetic() && !"АаЕеЄєИиІіЇїОоУуЮюЯяЫыЭэ".contains(c)
}

// --- Devanagari and Gurmukhi (consonants carry an inherent "a") ----------------------------------

/// One Brahmic script: consonants (single code points or conjunct/nukta sequences), vowel signs
/// that replace the inherent vowel, and everything else that maps on its own.
struct Indic {
    consonants: &'static [(&'static str, &'static str)],
    vowel_signs: &'static [(char, &'static str)],
    others: &'static [(&'static str, &'static str)],
    virama: char,
    /// Gurmukhi's addak (ੱ) doubles the following consonant.
    gemination: Option<char>,
    /// Vowel carriers: read on their own, silent when a vowel sign follows (ੳ + ੁ is "u").
    carriers: &'static [(char, &'static str)],
    /// Signs in `others` that attach to the vowel before them.
    modifiers: &'static [char],
    /// Independent vowel letters in `others`: they voice a syllable like a vowel sign does.
    vowels: &'static [char],
}

fn romanize_indic(text: &str, script: &Indic) -> String {
    let chars: Vec<char> = text.chars().collect();
    let mut out = String::with_capacity(text.len());
    let mut word: Vec<Syllable> = Vec::new();
    let mut double_next = false;
    let mut i = 0;
    while i < chars.len() {
        let c = chars[i];
        if let Some((len, latin)) = longest(&chars[i..], script.consonants) {
            i += len;
            let vowel = match chars.get(i) {
                Some(&next) if next == script.virama => {
                    i += 1;
                    Vowel::Dead
                }
                Some(next) => match vowel_sign(script, *next) {
                    Some(sign) => {
                        i += 1;
                        Vowel::Sign(sign)
                    }
                    None => Vowel::Inherent,
                },
                None => Vowel::Inherent,
            };
            word.push(Syllable { consonant: latin, doubled: double_next, vowel, tail: "" });
            double_next = false;
        } else if let Some(&(_, alone)) = script.carriers.iter().find(|(carrier, _)| *carrier == c)
        {
            i += 1;
            let vowel = match chars.get(i).and_then(|next| vowel_sign(script, *next)) {
                Some(sign) => {
                    i += 1;
                    sign
                }
                None => alone,
            };
            word.push(Syllable {
                consonant: "",
                doubled: false,
                vowel: Vowel::Sign(vowel),
                tail: "",
            });
        } else if Some(c) == script.gemination {
            double_next = true;
            i += 1;
        } else if let Some(sign) = vowel_sign(script, c) {
            word.push(Syllable {
                consonant: "",
                doubled: false,
                vowel: Vowel::Sign(sign),
                tail: "",
            });
            i += 1;
        } else if let Some((len, latin)) = longest(&chars[i..], script.others) {
            i += len;
            match (script.modifiers.contains(&c), word.last_mut()) {
                // ं ँ ः and ੰ ਂ nasalise or aspirate the vowel before them.
                (true, Some(last)) if last.tail.is_empty() => last.tail = latin,
                _ if script.vowels.contains(&c) => word.push(Syllable {
                    consonant: "",
                    doubled: false,
                    vowel: Vowel::Sign(latin),
                    tail: "",
                }),
                _ => {
                    flush_word(&mut word, &mut out);
                    out.push_str(latin);
                }
            }
        } else {
            flush_word(&mut word, &mut out);
            out.push(c);
            i += 1;
        }
    }
    flush_word(&mut word, &mut out);
    out
}

fn vowel_sign(script: &Indic, c: char) -> Option<&'static str> {
    script.vowel_signs.iter().find(|(sign, _)| *sign == c).map(|&(_, latin)| latin)
}

#[derive(Clone, Copy, PartialEq, Eq)]
enum Vowel {
    /// The consonant's own "a", which may be dropped (schwa deletion).
    Inherent,
    /// A written vowel, or an independent vowel letter.
    Sign(&'static str),
    /// Killed by a virama: part of a consonant cluster.
    Dead,
}

struct Syllable {
    consonant: &'static str,
    doubled: bool,
    vowel: Vowel,
    tail: &'static str,
}

impl Syllable {
    fn voiced(&self) -> bool {
        !matches!(self.vowel, Vowel::Dead)
    }
}

/// Write one word, dropping the inherent "a" where Hindi and Punjabi do not say it: at the end
/// of a word (कमल → kamal) and between a vowel and a consonant that has its own vowel
/// (करता → karta, तुमसे → tumse). Working right to left keeps two dropped vowels from meeting.
fn flush_word(word: &mut Vec<Syllable>, out: &mut String) {
    let n = word.len();
    let mut silent = vec![false; n];
    for i in (0..n).rev() {
        let syllable = &word[i];
        if syllable.vowel != Vowel::Inherent
            || syllable.consonant.is_empty()
            || !syllable.tail.is_empty()
        {
            continue;
        }
        silent[i] = if i == n - 1 {
            n > 1
        } else {
            i > 0
                && word[i - 1].voiced()
                && !word[i + 1].consonant.is_empty()
                && word[i + 1].voiced()
                && !silent[i + 1]
        };
    }
    for (syllable, silent) in word.iter().zip(silent) {
        if syllable.doubled {
            out.extend(syllable.consonant.chars().next());
        }
        out.push_str(syllable.consonant);
        match syllable.vowel {
            Vowel::Inherent if !silent => out.push('a'),
            Vowel::Sign(latin) => out.push_str(latin),
            _ => {}
        }
        out.push_str(syllable.tail);
    }
    word.clear();
}

fn longest(
    chars: &[char],
    table: &[(&'static str, &'static str)],
) -> Option<(usize, &'static str)> {
    let mut best: Option<(usize, &'static str)> = None;
    for &(key, latin) in table {
        let len = key.chars().count();
        if len <= chars.len()
            && key.chars().eq(chars[..len].iter().copied())
            && best.is_none_or(|(l, _)| len > l)
        {
            best = Some((len, latin));
        }
    }
    best
}

static DEVANAGARI: Indic = Indic {
    consonants: &[
        ("क", "k"),
        ("ख", "kh"),
        ("ग", "g"),
        ("घ", "gh"),
        ("ङ", "ng"),
        ("च", "ch"),
        ("छ", "chh"),
        ("ज", "j"),
        ("झ", "jh"),
        ("ञ", "ny"),
        ("ट", "t"),
        ("ठ", "th"),
        ("ड", "d"),
        ("ढ", "dh"),
        ("ण", "n"),
        ("त", "t"),
        ("थ", "th"),
        ("द", "d"),
        ("ध", "dh"),
        ("न", "n"),
        ("प", "p"),
        ("फ", "ph"),
        ("ब", "b"),
        ("भ", "bh"),
        ("म", "m"),
        ("य", "y"),
        ("र", "r"),
        ("ल", "l"),
        ("व", "v"),
        ("श", "sh"),
        ("ष", "sh"),
        ("स", "s"),
        ("ह", "h"),
        ("क्ष", "ksh"),
        ("त्र", "tr"),
        ("ज्ञ", "gy"),
        ("श्र", "shr"),
        ("क़", "q"),
        ("ख़", "kh"),
        ("ग़", "g"),
        ("ज़", "z"),
        ("ड़", "r"),
        ("ढ़", "rh"),
        ("फ़", "f"),
        ("य़", "y"),
        ("क\u{093C}", "q"),
        ("ख\u{093C}", "kh"),
        ("ग\u{093C}", "g"),
        ("ज\u{093C}", "z"),
        ("ड\u{093C}", "r"),
        ("ढ\u{093C}", "rh"),
        ("फ\u{093C}", "f"),
        ("य\u{093C}", "y"),
    ],
    vowel_signs: &[
        ('ा', "aa"),
        ('ि', "i"),
        ('ी', "ee"),
        ('ु', "u"),
        ('ू', "oo"),
        ('ृ', "ri"),
        ('े', "e"),
        ('ै', "ai"),
        ('ो', "o"),
        ('ौ', "au"),
    ],
    others: &[
        ("अ", "a"),
        ("आ", "aa"),
        ("इ", "i"),
        ("ई", "ee"),
        ("उ", "u"),
        ("ऊ", "oo"),
        ("ऋ", "ri"),
        ("ए", "e"),
        ("ऐ", "ai"),
        ("ओ", "o"),
        ("औ", "au"),
        ("ं", "n"),
        ("ः", "h"),
        ("ँ", "n"),
        ("़", ""),
        ("्", ""),
        ("०", "0"),
        ("१", "1"),
        ("२", "2"),
        ("३", "3"),
        ("४", "4"),
        ("५", "5"),
        ("६", "6"),
        ("७", "7"),
        ("८", "8"),
        ("९", "9"),
        ("ॐ", "Om"),
        ("ऽ", ""),
        ("।", "."),
        ("॥", "."),
    ],
    virama: '\u{094D}',
    gemination: None,
    carriers: &[],
    modifiers: &['ं', 'ँ', 'ः'],
    vowels: &['अ', 'आ', 'इ', 'ई', 'उ', 'ऊ', 'ऋ', 'ए', 'ऐ', 'ओ', 'औ'],
};

static GURMUKHI: Indic = Indic {
    consonants: &[
        ("ਸ", "s"),
        ("ਹ", "h"),
        ("ਕ", "k"),
        ("ਖ", "kh"),
        ("ਗ", "g"),
        ("ਘ", "gh"),
        ("ਙ", "ng"),
        ("ਚ", "ch"),
        ("ਛ", "chh"),
        ("ਜ", "j"),
        ("ਝ", "jh"),
        ("ਞ", "ny"),
        ("ਟ", "t"),
        ("ਠ", "th"),
        ("ਡ", "d"),
        ("ਢ", "dh"),
        ("ਣ", "n"),
        ("ਤ", "t"),
        ("ਥ", "th"),
        ("ਦ", "d"),
        ("ਧ", "dh"),
        ("ਨ", "n"),
        ("ਪ", "p"),
        ("ਫ", "ph"),
        ("ਬ", "b"),
        ("ਭ", "bh"),
        ("ਮ", "m"),
        ("ਯ", "y"),
        ("ਰ", "r"),
        ("ਲ", "l"),
        ("ਵ", "v"),
        ("ੜ", "r"),
        ("ਸ਼", "sh"),
        ("ਖ਼", "kh"),
        ("ਗ਼", "g"),
        ("ਜ਼", "z"),
        ("ਫ਼", "f"),
        ("ਲ਼", "l"),
        ("ਸ\u{0A3C}", "sh"),
        ("ਖ\u{0A3C}", "kh"),
        ("ਗ\u{0A3C}", "g"),
        ("ਜ\u{0A3C}", "z"),
        ("ਫ\u{0A3C}", "f"),
        ("ਲ\u{0A3C}", "l"),
    ],
    vowel_signs: &[
        ('ਾ', "aa"),
        ('ਿ', "i"),
        ('ੀ', "ee"),
        ('ੁ', "u"),
        ('ੂ', "oo"),
        ('ੇ', "e"),
        ('ੈ', "ai"),
        ('ੋ', "o"),
        ('ੌ', "au"),
    ],
    others: &[
        ("ਅ", "a"),
        ("ਆ", "aa"),
        ("ਇ", "i"),
        ("ਈ", "ee"),
        ("ਉ", "u"),
        ("ਊ", "oo"),
        ("ਏ", "e"),
        ("ਐ", "ai"),
        ("ਓ", "o"),
        ("ਔ", "au"),
        ("ੰ", "n"),
        ("ਂ", "n"),
        ("੍", ""),
        ("਼", ""),
        ("ੴ", "Ek Onkar"),
        ("੦", "0"),
        ("੧", "1"),
        ("੨", "2"),
        ("੩", "3"),
        ("੪", "4"),
        ("੫", "5"),
        ("੬", "6"),
        ("੭", "7"),
        ("੮", "8"),
        ("੯", "9"),
    ],
    virama: '\u{0A4D}',
    gemination: Some('\u{0A71}'),
    carriers: &[('ੳ', "o"), ('ੲ', "e")],
    modifiers: &['ੰ', 'ਂ'],
    vowels: &['ਅ', 'ਆ', 'ਇ', 'ਈ', 'ਉ', 'ਊ', 'ਏ', 'ਐ', 'ਓ', 'ਔ'],
};

// --- Tables ported from PeekMusic (generated from its Kotlin maps) ------------------------------
//
// Changed from PeekMusic: a merged final/initial pair now replaces the next initial instead of
// being followed by it (국내 → gungnae, not gungnnae), and three pairs were corrected: ᆩ+ᄋ "kk",
// ᇂ+ᄋ silent, ᆻ+ᄂ/ᄅ "nn".

fn general_cyrillic(c: char) -> Option<&'static str> {
    Some(match c {
        'А' => "A",
        'Б' => "B",
        'В' => "V",
        'Г' => "G",
        'Ґ' => "G",
        'Д' => "D",
        'Ѓ' => "Ǵ",
        'Ђ' => "Đ",
        'Е' => "E",
        'Ё' => "Yo",
        'Є' => "Ye",
        'Ж' => "Zh",
        'З' => "Z",
        'Ѕ' => "Dz",
        'И' => "I",
        'І' => "I",
        'Ї' => "Yi",
        'Й' => "Y",
        'Ј' => "Y",
        'К' => "K",
        'Л' => "L",
        'Љ' => "Ly",
        'М' => "M",
        'Н' => "N",
        'Њ' => "Ny",
        'О' => "O",
        'П' => "P",
        'Р' => "R",
        'С' => "S",
        'Т' => "T",
        'Ћ' => "Ć",
        'У' => "U",
        'Ў' => "Ŭ",
        'Ф' => "F",
        'Х' => "Kh",
        'Ц' => "Ts",
        'Ч' => "Ch",
        'Џ' => "Dž",
        'Ш' => "Sh",
        'Щ' => "Shch",
        'Ъ' => "ʺ",
        'Ы' => "Y",
        'Ь' => "ʹ",
        'Э' => "E",
        'Ю' => "Yu",
        'Я' => "Ya",
        'Ѡ' => "O",
        'Ѣ' => "Ya",
        'Ѥ' => "Ye",
        'Ѧ' => "Ya",
        'Ѩ' => "Ya",
        'Ѫ' => "U",
        'Ѭ' => "Yu",
        'Ѯ' => "Ks",
        'Ѱ' => "Ps",
        'Ѳ' => "F",
        'Ѵ' => "I",
        'Ѷ' => "I",
        'Ғ' => "Gh",
        'Ҕ' => "G",
        'Җ' => "Zh",
        'Ҙ' => "Dz",
        'Қ' => "Q",
        'Ҝ' => "K",
        'Ҟ' => "K",
        'Ҡ' => "K",
        'Ң' => "Ng",
        'Ҥ' => "Ng",
        'Ҧ' => "P",
        'Ҩ' => "O",
        'Ҫ' => "S",
        'Ҭ' => "T",
        'Ү' => "U",
        'Ұ' => "U",
        'Ҳ' => "Kh",
        'Ҵ' => "Ts",
        'Ҷ' => "Ch",
        'Ҹ' => "Ch",
        'Һ' => "H",
        'Ҽ' => "Ch",
        'Ҿ' => "Ch",
        'Ќ' => "Ḱ",
        'Ө' => "Ö",
        'а' => "a",
        'б' => "b",
        'в' => "v",
        'г' => "g",
        'ґ' => "g",
        'д' => "d",
        'ѓ' => "ǵ",
        'ђ' => "đ",
        'е' => "e",
        'ё' => "yo",
        'є' => "ye",
        'ж' => "zh",
        'з' => "z",
        'ѕ' => "dz",
        'и' => "i",
        'і' => "i",
        'ї' => "yi",
        'й' => "y",
        'ј' => "y",
        'к' => "k",
        'л' => "l",
        'љ' => "ly",
        'м' => "m",
        'н' => "n",
        'њ' => "ny",
        'о' => "o",
        'п' => "p",
        'р' => "r",
        'с' => "s",
        'т' => "t",
        'ћ' => "ć",
        'у' => "u",
        'ў' => "ŭ",
        'ф' => "f",
        'х' => "kh",
        'ц' => "ts",
        'ч' => "ch",
        'џ' => "dž",
        'ш' => "sh",
        'щ' => "shch",
        'ъ' => "ʺ",
        'ы' => "y",
        'ь' => "ʹ",
        'э' => "e",
        'ю' => "yu",
        'я' => "ya",
        'ѡ' => "o",
        'ѣ' => "ya",
        'ѥ' => "ye",
        'ѧ' => "ya",
        'ѩ' => "ya",
        'ѫ' => "u",
        'ѭ' => "yu",
        'ѯ' => "ks",
        'ѱ' => "ps",
        'ѳ' => "f",
        'ѵ' => "i",
        'ѷ' => "i",
        'ғ' => "gh",
        'ҕ' => "g",
        'җ' => "zh",
        'ҙ' => "dz",
        'қ' => "q",
        'ҝ' => "k",
        'ҟ' => "k",
        'ҡ' => "k",
        'ң' => "ng",
        'ҥ' => "ng",
        'ҧ' => "p",
        'ҩ' => "o",
        'ҫ' => "s",
        'ҭ' => "t",
        'ү' => "u",
        'ұ' => "u",
        'ҳ' => "kh",
        'ҵ' => "ts",
        'ҷ' => "ch",
        'ҹ' => "ch",
        'һ' => "h",
        'ҽ' => "ch",
        'ҿ' => "ch",
        'ќ' => "ḱ",
        'ө' => "ö",
        _ => return None,
    })
}

fn ukrainian(c: char) -> Option<&'static str> {
    Some(match c {
        'Г' => "H",
        'г' => "h",
        'Ґ' => "G",
        'ґ' => "g",
        'Є' => "Ye",
        'є' => "ye",
        'І' => "I",
        'і' => "i",
        'Ї' => "Yi",
        'ї' => "yi",
        _ => return None,
    })
}

fn serbian(c: char) -> Option<&'static str> {
    Some(match c {
        'Ж' => "Ž",
        'Љ' => "Lj",
        'Њ' => "Nj",
        'Ц' => "C",
        'Ч' => "Č",
        'Џ' => "Dž",
        'Ш' => "Š",
        'Х' => "H",
        'ж' => "ž",
        'љ' => "lj",
        'њ' => "nj",
        'ц' => "c",
        'ч' => "č",
        'џ' => "dž",
        'ш' => "š",
        'х' => "h",
        _ => return None,
    })
}

fn bulgarian(c: char) -> Option<&'static str> {
    Some(match c {
        'Ж' => "Zh",
        'Ц' => "Ts",
        'Ч' => "Ch",
        'Ш' => "Sh",
        'Щ' => "Sht",
        'Ъ' => "A",
        'Ь' => "Y",
        'Ю' => "Yu",
        'Я' => "Ya",
        'ж' => "zh",
        'ц' => "ts",
        'ч' => "ch",
        'ш' => "sh",
        'щ' => "sht",
        'ъ' => "a",
        'ь' => "y",
        'ю' => "yu",
        'я' => "ya",
        _ => return None,
    })
}

fn belarusian(c: char) -> Option<&'static str> {
    Some(match c {
        'Г' => "H",
        'г' => "h",
        'Ў' => "W",
        'ў' => "w",
        _ => return None,
    })
}

fn kyrgyz(c: char) -> Option<&'static str> {
    Some(match c {
        'Ү' => "Ü",
        'ү' => "ü",
        'Ы' => "Y",
        'ы' => "y",
        _ => return None,
    })
}

fn macedonian(c: char) -> Option<&'static str> {
    Some(match c {
        'Ѓ' => "Gj",
        'Ѕ' => "Dz",
        'И' => "I",
        'Ј' => "J",
        'Љ' => "Lj",
        'Њ' => "Nj",
        'Ќ' => "Kj",
        'Џ' => "Dž",
        'Ч' => "Č",
        'Ш' => "Sh",
        'Ж' => "Zh",
        'Ц' => "C",
        'Х' => "H",
        'ѓ' => "gj",
        'ѕ' => "dz",
        'и' => "i",
        'ј' => "j",
        'љ' => "lj",
        'њ' => "nj",
        'ќ' => "kj",
        'џ' => "dž",
        'ч' => "č",
        'ш' => "sh",
        'ж' => "zh",
        'ц' => "c",
        'х' => "h",
        'Ѐ' => "E",
        'ѐ' => "e",
        'Ѝ' => "I",
        'ѝ' => "i",
        _ => return None,
    })
}

const CHO: [&str; 19] = [
    "g", "kk", "n", "d", "tt", "r", "m", "b", "pp", "s", "ss", "", "j", "jj", "ch", "k", "t", "p",
    "h",
];
const JUNG: [&str; 21] = [
    "a", "ae", "ya", "yae", "eo", "e", "yeo", "ye", "o", "wa", "wae", "oe", "yo", "u", "wo", "we",
    "wi", "yu", "eu", "eui", "i",
];
const JONG: [&str; 28] = [
    "", "k", "kk", "k", "n", "n", "n", "t", "l", "k", "m", "p", "t", "t", "p", "l", "m", "p", "p",
    "t", "t", "ng", "t", "t", "k", "t", "p", "t",
];
fn jong_before(jong: usize, next_cho: usize) -> Option<&'static str> {
    Some(match (jong, next_cho) {
        (1, 2) => "ngn",
        (1, 5) => "ngn",
        (1, 6) => "ngm",
        (1, 11) => "g",
        (1, 18) => "kh",
        (2, 2) => "ngn",
        (2, 5) => "ngn",
        (2, 6) => "ngm",
        (2, 11) => "kk",
        (2, 18) => "kh",
        (3, 2) => "ngn",
        (3, 5) => "ngn",
        (3, 6) => "ngm",
        (3, 11) => "ks",
        (3, 18) => "kch",
        (4, 5) => "ll",
        (5, 2) => "nn",
        (5, 5) => "nn",
        (5, 6) => "nm",
        (5, 11) => "nj",
        (5, 18) => "nch",
        (6, 5) => "nn",
        (6, 11) => "nh",
        (7, 2) => "nn",
        (7, 5) => "nn",
        (7, 6) => "nm",
        (7, 11) => "d",
        (7, 18) => "th",
        (8, 2) => "ll",
        (8, 5) => "ll",
        (8, 11) => "r",
        (9, 2) => "ngn",
        (9, 5) => "ngn",
        (9, 6) => "ngm",
        (9, 11) => "lg",
        (9, 18) => "lkh",
        (10, 2) => "mn",
        (10, 5) => "mn",
        (10, 6) => "mm",
        (10, 11) => "lm",
        (10, 18) => "lmh",
        (11, 2) => "mn",
        (11, 5) => "mn",
        (11, 6) => "mm",
        (11, 11) => "lb",
        (11, 18) => "lph",
        (12, 2) => "nn",
        (12, 5) => "nn",
        (12, 6) => "nm",
        (12, 11) => "ls",
        (12, 18) => "lsh",
        (13, 2) => "nn",
        (13, 5) => "nn",
        (13, 6) => "nm",
        (13, 11) => "lt",
        (13, 18) => "lth",
        (14, 2) => "mn",
        (14, 5) => "mn",
        (14, 6) => "mm",
        (14, 11) => "lp",
        (14, 18) => "lph",
        (15, 2) => "ll",
        (15, 5) => "ll",
        (15, 6) => "lm",
        (15, 11) => "lh",
        (15, 18) => "lh",
        (16, 5) => "mn",
        (17, 2) => "mn",
        (17, 5) => "mn",
        (17, 6) => "mm",
        (17, 11) => "b",
        (17, 18) => "ph",
        (18, 2) => "mn",
        (18, 5) => "mn",
        (18, 6) => "mm",
        (18, 11) => "ps",
        (18, 18) => "psh",
        (19, 2) => "nn",
        (19, 5) => "nn",
        (19, 6) => "nm",
        (19, 11) => "s",
        (19, 18) => "sh",
        (20, 2) => "nn",
        (20, 5) => "nn",
        (20, 6) => "nm",
        (20, 11) => "ss",
        (20, 18) => "th",
        (22, 2) => "nn",
        (22, 5) => "nn",
        (22, 6) => "nm",
        (22, 11) => "j",
        (22, 18) => "ch",
        (23, 2) => "nn",
        (23, 5) => "nn",
        (23, 6) => "nm",
        (23, 11) => "ch",
        (23, 18) => "ch",
        (24, 2) => "ngn",
        (24, 5) => "ngn",
        (24, 6) => "ngm",
        (24, 11) => "k",
        (24, 18) => "kh",
        (25, 2) => "nn",
        (25, 5) => "nn",
        (25, 6) => "nm",
        (25, 11) => "t",
        (25, 18) => "th",
        (26, 2) => "mn",
        (26, 5) => "mn",
        (26, 6) => "mm",
        (26, 11) => "p",
        (26, 18) => "ph",
        (27, 0) => "k",
        (27, 2) => "nn",
        (27, 5) => "nn",
        (27, 6) => "mm",
        (27, 11) => "",
        (27, 18) => "t",
        _ => return None,
    })
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::lyrics::LyricLine;

    fn song(lines: &[&str]) -> Lyrics {
        Lyrics {
            source: "test".into(),
            synced: false,
            instrumental: false,
            lines: lines.iter().map(|text| LyricLine::simple(None, (*text).into())).collect(),
        }
    }

    fn romanized(lines: &[&str]) -> Vec<Option<String>> {
        let mut lyrics = song(lines);
        romanize_lyrics(&mut lyrics);
        lyrics.lines.into_iter().map(|line| line.romanized).collect()
    }

    fn one(line: &str) -> String {
        romanized(&[line]).remove(0).unwrap_or_else(|| panic!("{line} was not romanized"))
    }

    #[test]
    fn korean_follows_revised_romanization_across_syllables() {
        assert_eq!(one("안녕하세요"), "annyeonghaseyo");
        assert_eq!(one("사랑해"), "saranghae");
        assert_eq!(one("감사합니다"), "gamsahamnida");
        assert_eq!(one("한국어"), "hangugeo");
        // A merged final/initial pair is written once: not "gungnnae" / "sillra".
        assert_eq!(one("국내"), "gungnae");
        assert_eq!(one("신라"), "silla");
        assert_eq!(one("좋아"), "joa");
        assert_eq!(one("너를 사랑해 baby"), "neoreul saranghae baby");
    }

    #[test]
    fn chinese_becomes_toneless_pinyin() {
        assert_eq!(one("我爱你"), "wo ai ni");
        assert_eq!(one("你好，世界"), "ni hao，shi jie");
        assert_eq!(one("爱你baby"), "ai ni baby");
        assert_eq!(one("「月亮代表我的心」"), "「yue liang dai biao wo de xin」");
    }

    #[test]
    fn japanese_songs_are_left_alone_even_on_kanji_only_lines() {
        // "東京" alone would read as Chinese; the kana elsewhere in the song marks it Japanese.
        let lines = ["君の名は", "東京", "ありがとう"];
        assert_eq!(romanized(&lines), vec![None, None, None]);
    }

    #[test]
    fn cyrillic_picks_the_language_from_the_whole_song() {
        assert_eq!(one("Я тебя люблю"), "Ya tebya lyublyu");
        assert_eq!(one("Ничего не говори"), "Nichevo ne govori");
        assert_eq!(one("Его нет, много дней"), "Yevo net, mnogo dney");
        // Ukrainian: г is h, Я after a consonant is "ia".
        assert_eq!(one("Слава Україні, моя земля"), "Slava Ukrayini, moya zemlia");
        assert_eq!(one("Љубав и живот"), "Ljubav i život");
        assert_eq!(one("Обичам те, България"), "Obicham te, Balgariya");
        assert_eq!(one("Я ўсё ведаю"), "Ya wsyo vedayu");
        assert_eq!(one("Өмүр жашоо"), "Ömür zhashoo");
    }

    #[test]
    fn macedonian_song_reads_as_macedonian() {
        // PeekMusic's MacedonianRomanizationTest: ц is "c" (Russian would give "ts").
        let lines = [
            "Една цреша што мене ме чека",
            "Сѐ што имам и куќа и нива",
            "Но црешата да ја најдам жива",
            "Што е тоа во луѓето тажно",
        ];
        let out = romanized(&lines);
        assert_eq!(out[0].as_deref(), Some("Edna cresha shto mene me čeka"));
        assert_eq!(out[1].as_deref(), Some("Se shto imam i kukja i niva"));
        assert_eq!(out[3].as_deref(), Some("Shto e toa vo lugjeto tazhno"));
    }

    #[test]
    fn hindi_drops_the_inherent_vowel_where_it_is_not_said() {
        assert_eq!(one("कमल"), "kamal");
        assert_eq!(one("मैं तुमसे प्यार करता हूँ"), "main tumse pyaar kartaa hoon");
        assert_eq!(one("नमस्ते दुनिया"), "namaste duniyaa");
        assert_eq!(one("धड़कन"), "dharkan");
    }

    #[test]
    fn punjabi_reads_vowel_carriers_and_doubling() {
        assert_eq!(one("ਸਤ ਸ੍ਰੀ ਅਕਾਲ"), "sat sree akaal");
        assert_eq!(one("ਪੱਕਾ"), "pakkaa");
        assert_eq!(one("ਉਹ ਇੱਥੇ ਹੈ"), "uh itthe hai");
    }

    #[test]
    fn latin_lines_and_identical_readings_stay_empty() {
        assert_eq!(romanized(&["Hello world", "Oh oh oh"]), vec![None, None]);
        // Mixed song: the English line has nothing to add under it.
        assert_eq!(romanized(&["사랑해", "I love you"]), vec![Some("saranghae".to_owned()), None]);
    }

    #[test]
    fn cached_lyrics_without_the_field_still_parse() {
        let json =
            r#"{"source":"LRCLIB","synced":true,"lines":[{"time_ms":1000,"text":"사랑해"}]}"#;
        let lyrics: Lyrics = serde_json::from_str(json).unwrap();
        assert_eq!(lyrics.lines[0].romanized, None);
        let back = serde_json::to_string(&lyrics).unwrap();
        assert!(!back.contains("romanized"), "an unset reading must not be serialized: {back}");
    }
}
