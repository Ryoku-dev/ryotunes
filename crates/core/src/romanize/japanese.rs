//! Japanese readings from a morphological dictionary: vibrato with mecab-ipadic, the reading per
//! word turned into romaji. Kanji have no reading of their own, so only a dictionary-backed
//! tokenizer can say that 君の名は is "kimi no na wa".
//!
//! The dictionary (`system.dic.zst`, ~7.7 MiB) ships as package data, not in the binary, and costs
//! ~60 MiB of memory while loaded. So it is loaded only for a Japanese song, freed straight after,
//! and the result is kept per song text so refreshes and toggles don't load it again. Without the
//! file (a development build, another distribution) Japanese simply gets no reading.

use std::collections::VecDeque;
use std::hash::{Hash, Hasher};
use std::path::{Path, PathBuf};
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::{Arc, Mutex};

/// Where the package installs the dictionary.
const DICT_PATH: &str = "/usr/share/ryotunes/dict/ipadic/system.dic.zst";
/// Overrides [`DICT_PATH`], for development builds and the dictionary-backed tests.
const DICT_ENV: &str = "RYOTUNES_JA_DICT";
/// Songs whose readings are kept in memory, a few KiB each.
const CACHED_SONGS: usize = 32;

static CACHE: Mutex<VecDeque<(u64, Arc<Vec<String>>)>> = Mutex::new(VecDeque::new());
/// One dictionary load at a time: two views asking for the same song wait for one result.
static LOADING: Mutex<()> = Mutex::new(());
/// Set once a dictionary file failed to parse, so a corrupt file is not decompressed again for
/// every Japanese song. A missing file is cheap to retry and is not remembered.
static BROKEN: AtomicBool = AtomicBool::new(false);

/// Romaji for each line, or `None` when no dictionary is installed.
pub(super) fn romanize_lines(lines: &[&str]) -> Option<Arc<Vec<String>>> {
    let path = std::env::var_os(DICT_ENV).map(PathBuf::from).unwrap_or_else(|| DICT_PATH.into());
    romanize_lines_with(&path, lines)
}

fn romanize_lines_with(dictionary: &Path, lines: &[&str]) -> Option<Arc<Vec<String>>> {
    let key = song_key(lines);
    if let Some(hit) = cached(key) {
        return Some(hit);
    }
    if BROKEN.load(Ordering::Relaxed) {
        return None;
    }
    let _loading = LOADING.lock().unwrap_or_else(|poisoned| poisoned.into_inner());
    if let Some(hit) = cached(key) {
        return Some(hit);
    }
    let readings = Arc::new(with_dictionary(dictionary, |tokenizer| {
        let mut worker = tokenizer.new_worker();
        lines.iter().map(|line| read_line(&mut worker, line)).collect::<Vec<_>>()
    })?);
    let mut cache = CACHE.lock().unwrap_or_else(|poisoned| poisoned.into_inner());
    if cache.len() == CACHED_SONGS {
        cache.pop_front();
    }
    cache.push_back((key, readings.clone()));
    Some(readings)
}

fn song_key(lines: &[&str]) -> u64 {
    let mut hasher = std::collections::hash_map::DefaultHasher::new();
    lines.hash(&mut hasher);
    hasher.finish()
}

/// A hit moves to the back, so the songs played most stay cached (least recently used goes first).
fn cached(key: u64) -> Option<Arc<Vec<String>>> {
    let mut cache = CACHE.lock().unwrap_or_else(|poisoned| poisoned.into_inner());
    let at = cache.iter().position(|(k, _)| *k == key)?;
    let entry = cache.remove(at)?;
    let readings = entry.1.clone();
    cache.push_back(entry);
    Some(readings)
}

/// Load the dictionary, run `f`, and give the memory back before returning.
fn with_dictionary<T>(path: &Path, f: impl FnOnce(&vibrato::Tokenizer) -> T) -> Option<T> {
    let file = std::fs::File::open(path).ok()?;
    let decoder = match ruzstd::decoding::StreamingDecoder::new(std::io::BufReader::new(file)) {
        Ok(decoder) => decoder,
        Err(e) => {
            tracing::warn!(error = %e, path = %path.display(), "romanize: unreadable Japanese dictionary");
            BROKEN.store(true, Ordering::Relaxed);
            return None;
        }
    };
    let dictionary = match vibrato::Dictionary::read(decoder) {
        Ok(dictionary) => dictionary,
        Err(e) => {
            tracing::warn!(error = %e, path = %path.display(), "romanize: invalid Japanese dictionary");
            BROKEN.store(true, Ordering::Relaxed);
            return None;
        }
    };
    let tokenizer = vibrato::Tokenizer::new(dictionary);
    let out = f(&tokenizer);
    drop(tokenizer);
    release_memory();
    Some(out)
}

/// glibc keeps freed heap pages for reuse, so ~45 MiB of the dictionary would stay resident in the
/// daemon after the drop. Hand them back to the system.
fn release_memory() {
    #[cfg(all(target_os = "linux", target_env = "gnu"))]
    // SAFETY: malloc_trim only releases free heap memory; it has no preconditions.
    unsafe {
        libc::malloc_trim(0);
    }
}

// --- tokens to romaji ----------------------------------------------------------------------------

/// The parts of an IPADIC token the romaji pass uses.
#[derive(Clone, Debug)]
struct Token {
    surface: String,
    /// Part of speech (品詞) and its first subdivision.
    pos: String,
    pos1: String,
    /// Dictionary form (基本形), which tells the copula だ/です apart from other auxiliaries.
    base: String,
    /// Katakana reading, or empty for words the dictionary does not know.
    reading: String,
}

fn read_line(worker: &mut vibrato::tokenizer::worker::Worker<'_>, line: &str) -> String {
    worker.reset_sentence(line);
    worker.tokenize();
    let tokens: Vec<Token> = worker
        .token_iter()
        .map(|token| {
            let feature: Vec<&str> = token.feature().split(',').collect();
            let field = |i: usize| feature.get(i).copied().filter(|f| *f != "*").unwrap_or("");
            Token {
                surface: token.surface().to_owned(),
                pos: field(0).to_owned(),
                pos1: field(1).to_owned(),
                base: field(6).to_owned(),
                reading: field(7).to_owned(),
            }
        })
        .collect();
    assemble(&tokens)
}

/// What one token contributes to the line.
enum Piece {
    /// Katakana for a word or a part of one.
    Kana(String),
    /// Text kept as written (Latin words, digits).
    Literal(String),
    /// Punctuation: `true` for an opening bracket, which hugs the word after it.
    Punct(String, bool),
    Space,
}

/// Words are separated by spaces; auxiliaries, conjunctive て/で and dependent verbs join the word
/// before them (奪っ + た → ubatta, 重なっ + て + い + た → kasanatteita), the way romaji lyrics
/// are written. The copula stays its own word (だけ だった → dake datta).
fn assemble(tokens: &[Token]) -> String {
    let tokens = &merge_compounds(tokens)[..];
    let mut out = String::new();
    let mut word = String::new(); // katakana of the word being built
    let mut prev_conjugates = false;
    let mut prev_verb = false;
    let mut glue_next = false; // after an opening bracket
    let mut quote_open = false; // a straight " alternates between opening and closing
    let flush = |word: &mut String, out: &mut String| {
        if !word.is_empty() {
            out.push_str(&katakana_to_romaji(word));
            word.clear();
        }
    };
    for (i, token) in tokens.iter().enumerate() {
        let fixed;
        let token = match corrected_reading(tokens, i) {
            Some(reading) => {
                fixed = Token { reading: reading.to_owned(), ..token.clone() };
                &fixed
            }
            None => token,
        };
        match piece(token) {
            Piece::Space => {
                flush(&mut word, &mut out);
                if !out.is_empty() && !out.ends_with(' ') {
                    out.push(' ');
                }
                prev_conjugates = false;
                prev_verb = false;
            }
            Piece::Punct(text, opening) => {
                flush(&mut word, &mut out);
                let opening = if text == "\"" {
                    quote_open = !quote_open;
                    quote_open
                } else {
                    opening
                };
                if opening && !out.is_empty() && !out.ends_with(' ') {
                    out.push(' ');
                }
                out.push_str(&text);
                glue_next = opening;
                prev_conjugates = false;
                prev_verb = false;
            }
            Piece::Literal(text) => {
                flush(&mut word, &mut out);
                space_before(&mut out, glue_next);
                out.push_str(&text);
                glue_next = false;
                prev_conjugates = false;
                prev_verb = false;
            }
            Piece::Kana(kana) => {
                // Grammar decides what joins; a trailing っ alone (えっ, あっ) does not glue the next word.
                let joins = !word.is_empty() && attaches(token, prev_conjugates, prev_verb);
                if !joins {
                    flush(&mut word, &mut out);
                    space_before(&mut out, glue_next);
                }
                word.push_str(&kana);
                glue_next = false;
                prev_conjugates = matches!(token.pos.as_str(), "動詞" | "形容詞" | "助動詞");
                prev_verb = token.pos == "動詞";
            }
        }
    }
    flush(&mut word, &mut out);
    out.trim().to_owned()
}

fn space_before(out: &mut String, glued: bool) {
    if !glued && !out.is_empty() && !out.ends_with(' ') {
        out.push(' ');
    }
}

/// Words IPADIC splits in two and then reads wrongly: 一人/二人 as a number plus the suffix 人
/// (ni nin instead of futari), and 苛つく, whose 苛 it does not know at all.
fn merge_compounds(tokens: &[Token]) -> Vec<Token> {
    let mut out: Vec<Token> = Vec::with_capacity(tokens.len());
    let mut i = 0;
    while i < tokens.len() {
        let (token, next) = (&tokens[i], tokens.get(i + 1));
        let merged = next.and_then(|next| {
            let reading = match (token.surface.as_str(), next.surface.as_str()) {
                ("一", "人") if next.pos1 == "接尾" => "ヒトリ".to_owned(),
                ("二", "人") if next.pos1 == "接尾" => "フタリ".to_owned(),
                ("苛", verb) if verb.starts_with('つ') && next.pos == "動詞" => {
                    format!("イラ{}", next.reading)
                }
                _ => return None,
            };
            Some(Token {
                surface: format!("{}{}", token.surface, next.surface),
                reading,
                ..next.clone()
            })
        });
        match merged {
            Some(token) => {
                out.push(token);
                i += 2;
            }
            None => {
                out.push(token.clone());
                i += 1;
            }
        }
    }
    out
}

/// Readings IPADIC gets wrong in lyrics, where the right one depends on the next word. A short,
/// curated list: each entry is a misreading seen in real songs, not a general fix.
fn corrected_reading(tokens: &[Token], i: usize) -> Option<&'static str> {
    let token = &tokens[i];
    let prev = i.checked_sub(1).map(|p| &tokens[p]);
    let next = tokens.get(i + 1).map(|n| n.surface.as_str()).unwrap_or("");
    match token.surface.as_str() {
        // 如何か どうか, 如何に いかに; IPADIC reads いかが in every position.
        "如何" if next.starts_with('か') => Some("ドウ"),
        "如何" if next.starts_with('に') => Some("イカ"),
        // 前世 is ぜんせ; IPADIC has ぜんせい (前前前世 → zen zen zense).
        "前世" => Some("ゼンセ"),
        // 愛して / 愛してる after other text can come out as いとし.
        "愛し" if token.reading == "イトシ" && next.starts_with('て') => Some("アイシ"),
        // 手 after a pronoun (僕ら手を) is the noun て, not the suffix しゅ of 歌手.
        "手" if token.reading == "シュ" && prev.is_some_and(|p| p.pos1 == "代名詞") => {
            Some("テ")
        }
        _ => None,
    }
}

fn piece(token: &Token) -> Piece {
    let surface = token.surface.as_str();
    if surface.trim().is_empty() {
        return Piece::Space;
    }
    if surface.chars().all(|c| c.is_ascii_punctuation() || is_cjk_punct(c)) {
        let opening = surface.chars().all(|c| "「『（(［[【〈《“‘".contains(c));
        return Piece::Punct(surface.to_owned(), opening);
    }
    if token.pos == "助詞" {
        // The particles は/へ/を are read wa/e/o.
        match surface {
            "は" => return Piece::Kana("ワ".into()),
            "へ" => return Piece::Kana("エ".into()),
            "を" => return Piece::Kana("オ".into()),
            _ => {}
        }
    }
    if !token.reading.is_empty() {
        return Piece::Kana(token.reading.clone());
    }
    if surface.chars().all(|c| is_kana(c) || c == 'ー') {
        return Piece::Kana(surface.chars().map(to_katakana).collect());
    }
    // Full-width letters and digits (ＬＯＶＥ, ２０２４) read as their ASCII forms.
    Piece::Literal(surface.chars().map(halfwidth).collect())
}

fn halfwidth(c: char) -> char {
    if ('\u{FF01}'..='\u{FF5E}').contains(&c) {
        char::from_u32(c as u32 - 0xFEE0).unwrap_or(c)
    } else {
        c
    }
}

/// Whether a token continues the previous word instead of starting one.
fn attaches(token: &Token, prev_conjugates: bool, prev_verb: bool) -> bool {
    match token.pos.as_str() {
        // だ right after a verb is the past tense (込んだ, 飛んだ), not the copula; after anything
        // else だ/です stand alone (だけ だった, よかった でしょう).
        "助動詞" => {
            prev_conjugates && (prev_verb || !matches!(token.base.as_str(), "だ" | "です"))
        }
        "動詞" | "形容詞" => token.pos1 == "非自立" || token.pos1 == "接尾",
        "助詞" => {
            token.pos1 == "接続助詞" && matches!(token.surface.as_str(), "て" | "で" | "ば")
        }
        _ => false,
    }
}

fn is_kana(c: char) -> bool {
    ('\u{3041}'..='\u{3096}').contains(&c) || ('\u{30A1}'..='\u{30FA}').contains(&c)
}

/// CJK punctuation and full-width symbols, but not 々〆〇, which are written like kanji.
fn is_cjk_punct(c: char) -> bool {
    let symbols = ('\u{3000}'..='\u{303F}').contains(&c) && !('\u{3005}'..='\u{3007}').contains(&c);
    let fullwidth =
        matches!(c as u32, 0xFF01..=0xFF0F | 0xFF1A..=0xFF20 | 0xFF3B..=0xFF40 | 0xFF5B..=0xFF65);
    symbols || fullwidth || matches!(c, '…' | '・' | '♪' | '☆' | '★' | '〜')
}

fn to_katakana(c: char) -> char {
    if ('\u{3041}'..='\u{3096}').contains(&c) {
        char::from_u32(c as u32 + 0x60).unwrap_or(c)
    } else {
        c
    }
}

/// Hepburn-style romaji. っ doubles the next consonant, ー repeats the vowel before it.
fn katakana_to_romaji(kana: &str) -> String {
    let chars: Vec<char> = kana.chars().collect();
    let mut out = String::with_capacity(kana.len());
    let mut i = 0;
    let mut double = false;
    while i < chars.len() {
        let c = chars[i];
        if c == 'ッ' {
            double = true;
            i += 1;
            continue;
        }
        if c == 'ー' {
            if let Some(vowel) = out.chars().last().filter(|v| "aeiou".contains(*v)) {
                out.push(vowel);
            }
            i += 1;
            continue;
        }
        let (latin, len) = match chars.get(i + 1).and_then(|&n| digraph(c, n)) {
            Some(latin) => (latin, 2),
            None => (mora(c).unwrap_or(""), 1),
        };
        if c == 'ン' {
            // Hepburn marks a syllabic n before a vowel or y: ren'ai, kin'you.
            let next = chars.get(i + 1).and_then(|&n| mora(n)).unwrap_or("");
            out.push_str(if next.starts_with(['a', 'i', 'u', 'e', 'o', 'y']) { "n'" } else { "n" });
            double = false;
            i += 1;
            continue;
        }
        if latin.is_empty() {
            out.push(c);
        } else {
            if double {
                // ch is doubled as tch (マッチ → matchi).
                let first =
                    if latin.starts_with("ch") { 't' } else { latin.chars().next().unwrap_or('t') };
                if !"aeiou".contains(first) {
                    out.push(first);
                }
            }
            out.push_str(latin);
        }
        double = false;
        i += len;
    }
    out
}

/// Two-kana sounds: the yōon from PeekMusic's table plus the extended katakana loanwords use.
fn digraph(a: char, b: char) -> Option<&'static str> {
    Some(match (a, b) {
        ('キ', 'ャ') => "kya",
        ('キ', 'ュ') => "kyu",
        ('キ', 'ョ') => "kyo",
        ('シ', 'ャ') => "sha",
        ('シ', 'ュ') => "shu",
        ('シ', 'ョ') => "sho",
        ('チ', 'ャ') => "cha",
        ('チ', 'ュ') => "chu",
        ('チ', 'ョ') => "cho",
        ('ニ', 'ャ') => "nya",
        ('ニ', 'ュ') => "nyu",
        ('ニ', 'ョ') => "nyo",
        ('ヒ', 'ャ') => "hya",
        ('ヒ', 'ュ') => "hyu",
        ('ヒ', 'ョ') => "hyo",
        ('ミ', 'ャ') => "mya",
        ('ミ', 'ュ') => "myu",
        ('ミ', 'ョ') => "myo",
        ('リ', 'ャ') => "rya",
        ('リ', 'ュ') => "ryu",
        ('リ', 'ョ') => "ryo",
        ('ギ', 'ャ') => "gya",
        ('ギ', 'ュ') => "gyu",
        ('ギ', 'ョ') => "gyo",
        ('ジ', 'ャ') => "ja",
        ('ジ', 'ュ') => "ju",
        ('ジ', 'ョ') => "jo",
        ('ヂ', 'ャ') => "ja",
        ('ヂ', 'ュ') => "ju",
        ('ヂ', 'ョ') => "jo",
        ('ビ', 'ャ') => "bya",
        ('ビ', 'ュ') => "byu",
        ('ビ', 'ョ') => "byo",
        ('ピ', 'ャ') => "pya",
        ('ピ', 'ュ') => "pyu",
        ('ピ', 'ョ') => "pyo",
        ('シ', 'ェ') => "she",
        ('ジ', 'ェ') => "je",
        ('チ', 'ェ') => "che",
        ('フ', 'ァ') => "fa",
        ('フ', 'ィ') => "fi",
        ('フ', 'ェ') => "fe",
        ('フ', 'ォ') => "fo",
        ('テ', 'ィ') => "ti",
        ('デ', 'ィ') => "di",
        ('ト', 'ゥ') => "tu",
        ('ド', 'ゥ') => "du",
        ('ウ', 'ィ') => "wi",
        ('ウ', 'ェ') => "we",
        ('ウ', 'ォ') => "wo",
        ('ヴ', 'ァ') => "va",
        ('ヴ', 'ィ') => "vi",
        ('ヴ', 'ェ') => "ve",
        ('ヴ', 'ォ') => "vo",
        ('ツ', 'ァ') => "tsa",
        ('ツ', 'ィ') => "tsi",
        ('ツ', 'ェ') => "tse",
        ('ツ', 'ォ') => "tso",
        ('イ', 'ェ') => "ye",
        _ => return None,
    })
}

fn mora(c: char) -> Option<&'static str> {
    Some(match c {
        'ア' | 'ァ' => "a",
        'イ' | 'ィ' => "i",
        'ウ' | 'ゥ' => "u",
        'エ' | 'ェ' => "e",
        'オ' | 'ォ' => "o",
        'カ' => "ka",
        'キ' => "ki",
        'ク' => "ku",
        'ケ' => "ke",
        'コ' => "ko",
        'サ' => "sa",
        'シ' => "shi",
        'ス' => "su",
        'セ' => "se",
        'ソ' => "so",
        'タ' => "ta",
        'チ' => "chi",
        'ツ' => "tsu",
        'テ' => "te",
        'ト' => "to",
        'ナ' => "na",
        'ニ' => "ni",
        'ヌ' => "nu",
        'ネ' => "ne",
        'ノ' => "no",
        'ハ' => "ha",
        'ヒ' => "hi",
        'フ' => "fu",
        'ヘ' => "he",
        'ホ' => "ho",
        'マ' => "ma",
        'ミ' => "mi",
        'ム' => "mu",
        'メ' => "me",
        'モ' => "mo",
        'ヤ' | 'ャ' => "ya",
        'ユ' | 'ュ' => "yu",
        'ヨ' | 'ョ' => "yo",
        'ラ' => "ra",
        'リ' => "ri",
        'ル' => "ru",
        'レ' => "re",
        'ロ' => "ro",
        'ワ' => "wa",
        'ヲ' => "o",
        'ン' => "n",
        'ヴ' => "vu",
        'ガ' => "ga",
        'ギ' => "gi",
        'グ' => "gu",
        'ゲ' => "ge",
        'ゴ' => "go",
        'ザ' => "za",
        'ジ' => "ji",
        'ズ' => "zu",
        'ゼ' => "ze",
        'ゾ' => "zo",
        'ダ' => "da",
        'ヂ' => "ji",
        'ヅ' => "zu",
        'デ' => "de",
        'ド' => "do",
        'バ' => "ba",
        'ビ' => "bi",
        'ブ' => "bu",
        'ベ' => "be",
        'ボ' => "bo",
        'パ' => "pa",
        'ピ' => "pi",
        'プ' => "pu",
        'ペ' => "pe",
        'ポ' => "po",
        _ => return None,
    })
}

#[cfg(test)]
mod tests {
    use super::*;

    /// A token as IPADIC would tag it: surface, part of speech, subdivision, base form, reading.
    fn t(surface: &str, pos: &str, pos1: &str, base: &str, reading: &str) -> Token {
        Token {
            surface: surface.into(),
            pos: pos.into(),
            pos1: pos1.into(),
            base: base.into(),
            reading: reading.into(),
        }
    }

    #[test]
    fn kana_follow_hepburn_with_doubling_and_long_vowels() {
        assert_eq!(katakana_to_romaji("ウバッタ"), "ubatta");
        assert_eq!(katakana_to_romaji("マッチ"), "matchi");
        assert_eq!(katakana_to_romaji("トウキョウタワー"), "toukyoutawaa");
        assert_eq!(katakana_to_romaji("フェンス"), "fensu");
        assert_eq!(katakana_to_romaji("シャシン"), "shashin");
    }

    #[test]
    fn auxiliaries_join_their_verb_and_the_copula_stands_alone() {
        // 僕の心の全てを奪った
        let tokens = [
            t("僕", "名詞", "代名詞", "僕", "ボク"),
            t("の", "助詞", "連体化", "の", "ノ"),
            t("心", "名詞", "一般", "心", "ココロ"),
            t("の", "助詞", "連体化", "の", "ノ"),
            t("全て", "名詞", "副詞可能", "全て", "スベテ"),
            t("を", "助詞", "格助詞", "を", "ヲ"),
            t("奪っ", "動詞", "自立", "奪う", "ウバッ"),
            t("た", "助動詞", "", "た", "タ"),
        ];
        assert_eq!(assemble(&tokens), "boku no kokoro no subete o ubatta");
        // だけだった: だっ is the copula, so it does not glue onto だけ.
        let tokens = [
            t("だけ", "助詞", "副助詞", "だけ", "ダケ"),
            t("だっ", "助動詞", "", "だ", "ダッ"),
            t("た", "助動詞", "", "た", "タ"),
        ];
        assert_eq!(assemble(&tokens), "dake datta");
        // 重なっていた: て and the dependent いる join the verb.
        let tokens = [
            t("重なっ", "動詞", "自立", "重なる", "カサナッ"),
            t("て", "助詞", "接続助詞", "て", "テ"),
            t("い", "動詞", "非自立", "いる", "イ"),
            t("た", "助動詞", "", "た", "タ"),
        ];
        assert_eq!(assemble(&tokens), "kasanatteita");
    }

    #[test]
    fn particles_brackets_and_latin_keep_their_place() {
        let tokens = [
            t("君", "名詞", "代名詞", "君", "キミ"),
            t("の", "助詞", "連体化", "の", "ノ"),
            t("名", "名詞", "一般", "名", "ナ"),
            t("は", "助詞", "係助詞", "は", "ハ"),
        ];
        assert_eq!(assemble(&tokens), "kimi no na wa");
        let tokens = [
            t("「", "記号", "括弧開", "「", "「"),
            t("さよなら", "感動詞", "", "さよなら", "サヨナラ"),
            t("」", "記号", "括弧閉", "」", "」"),
            t("baby", "名詞", "固有名詞", "", ""),
            t("へ", "助詞", "格助詞", "へ", "ヘ"),
        ];
        assert_eq!(assemble(&tokens), "「sayonara」 baby e");
    }

    #[test]
    fn review_cases_interjections_quotes_fullwidth_and_syllabic_n() {
        // えっ何: the trailing っ of an interjection does not glue the next word.
        let tokens =
            [t("えっ", "感動詞", "", "えっ", "エッ"), t("何", "名詞", "代名詞", "何", "ナニ")];
        assert_eq!(assemble(&tokens), "e nani");
        // A straight quote opens, then closes.
        let tokens = [
            t("君", "名詞", "代名詞", "君", "キミ"),
            t("は", "助詞", "係助詞", "は", "ハ"),
            t("\"", "記号", "一般", "\"", ""),
            t("天使", "名詞", "一般", "天使", "テンシ"),
            t("\"", "記号", "一般", "\"", ""),
        ];
        assert_eq!(assemble(&tokens), "kimi wa \"tenshi\"");
        // Full-width letters read as ASCII; 〇 is not punctuation.
        assert_eq!(assemble(&[t("ＬＯＶＥ", "記号", "アルファベット", "", "")]), "LOVE");
        assert!(!is_cjk_punct('〇') && !is_cjk_punct('々') && is_cjk_punct('、'));
        assert_eq!(katakana_to_romaji("レンアイ"), "ren'ai");
        assert_eq!(katakana_to_romaji("キンヨウ"), "kin'you");
        assert_eq!(katakana_to_romaji("ミンナ"), "minna");
    }

    #[test]
    fn cache_keeps_the_most_recently_used_song() {
        // Keys no real song hashes to; only their relative order is checked, so tests running in
        // parallel may add entries meanwhile.
        let (older, newer) = (u64::MAX - 2, u64::MAX - 1);
        let position = |key| CACHE.lock().unwrap().iter().position(|(k, _)| *k == key);
        {
            let mut cache = CACHE.lock().unwrap();
            cache.push_back((older, Arc::new(Vec::new())));
            cache.push_back((newer, Arc::new(Vec::new())));
        }
        assert!(cached(older).is_some());
        assert!(position(older) > position(newer), "a hit moves the song to the back");
        CACHE.lock().unwrap().retain(|(k, _)| *k != older && *k != newer);
    }

    #[test]
    fn known_misreadings_are_corrected_from_context() {
        // 兄弟よ如何かしよう: IPADIC tags 如何 イカガ.
        let tokens = [
            t("兄弟", "名詞", "一般", "兄弟", "キョウダイ"),
            t("よ", "助詞", "終助詞", "よ", "ヨ"),
            t("如何", "名詞", "形容動詞語幹", "如何", "イカガ"),
            t("か", "助詞", "副助詞／並立助詞／終助詞", "か", "カ"),
            t("しよ", "動詞", "自立", "する", "シヨ"),
            t("う", "助動詞", "", "う", "ウ"),
        ];
        assert_eq!(assemble(&tokens), "kyoudai yo dou ka shiyou");
        // 僕ら手を: 手 tagged as the suffix シュ.
        let tokens = [
            t("僕ら", "名詞", "代名詞", "僕ら", "ボクラ"),
            t("手", "名詞", "接尾", "手", "シュ"),
            t("を", "助詞", "格助詞", "を", "ヲ"),
        ];
        assert_eq!(assemble(&tokens), "bokura te o");
        // A real suffix stays: 運転手を.
        let tokens = [
            t("運転", "名詞", "サ変接続", "運転", "ウンテン"),
            t("手", "名詞", "接尾", "手", "シュ"),
            t("を", "助詞", "格助詞", "を", "ヲ"),
        ];
        assert_eq!(assemble(&tokens), "unten shu o");
        let tokens = [t("前世", "名詞", "一般", "前世", "ゼンセイ")];
        assert_eq!(assemble(&tokens), "zense");
        let tokens = [
            t("愛し", "形容詞", "自立", "愛しい", "イトシ"),
            t("てる", "動詞", "非自立", "てる", "テル"),
        ];
        assert_eq!(assemble(&tokens), "aishiteru");
    }

    #[test]
    fn split_compounds_and_past_tense_da() {
        // 二人で: 二 + suffix 人 is futari.
        let tokens = [
            t("二", "名詞", "数", "二", "ニ"),
            t("人", "名詞", "接尾", "人", "ニン"),
            t("で", "助詞", "格助詞", "で", "デ"),
        ];
        assert_eq!(assemble(&tokens), "futari de");
        // 苛ついて: 苛 has no reading in IPADIC.
        let tokens = [
            t("苛", "名詞", "一般", "", ""),
            t("つい", "動詞", "自立", "つく", "ツイ"),
            t("て", "助詞", "接続助詞", "て", "テ"),
        ];
        assert_eq!(assemble(&tokens), "iratsuite");
        // 迷い込んだ: だ after a verb is the past tense.
        let tokens = [
            t("迷い", "動詞", "自立", "迷う", "マヨイ"),
            t("込ん", "動詞", "非自立", "込む", "コン"),
            t("だ", "助動詞", "", "だ", "ダ"),
        ];
        assert_eq!(assemble(&tokens), "mayoikonda");
        // …but stays apart after an auxiliary: よかったでしょう.
        let tokens = [
            t("よかっ", "形容詞", "自立", "よい", "ヨカッ"),
            t("た", "助動詞", "", "た", "タ"),
            t("でしょ", "助動詞", "", "です", "デショ"),
            t("う", "助動詞", "", "う", "ウ"),
        ];
        assert_eq!(assemble(&tokens), "yokatta deshou");
    }

    #[test]
    fn a_missing_dictionary_gives_no_reading() {
        assert!(
            romanize_lines_with(Path::new("/nonexistent/system.dic.zst"), &["君の名は"]).is_none()
        );
    }

    /// Runs only with the dictionary: `RYOTUNES_JA_DICT=…/system.dic.zst cargo test`, or with the
    /// package's copy installed.
    #[test]
    fn real_dictionary_reads_song_lines() {
        let path =
            std::env::var_os(DICT_ENV).map(PathBuf::from).unwrap_or_else(|| DICT_PATH.into());
        if !path.exists() {
            eprintln!("skipped: no Japanese dictionary at {}", path.display());
            return;
        }
        let lines = [
            "君の名は",
            "僕の心の全てを奪った",
            "日が沈み出した空と君の姿",
            "東京タワーへ行こう",
            "兄弟 よ 如何か しよう もう何も 考えない 様",
            "僕ら 手を叩いて笑い合う",
            "I love you 愛してる",
            "食べ尽くそう 二人 で くたばる まで",
            "邪魔 臭くて 苛ついて 迷い込んだ ニャンニャン",
        ];
        let readings = romanize_lines_with(&path, &lines).expect("dictionary loads");
        assert_eq!(readings[0], "kimi no na wa");
        assert_eq!(readings[1], "boku no kokoro no subete o ubatta");
        assert!(readings[2].starts_with("hi ga shizumi"), "{}", readings[2]);
        assert!(readings[3].contains("toukyou") && readings[3].contains(" e "), "{}", readings[3]);
        assert!(readings[4].contains("dou ka shiyou"), "{}", readings[4]);
        assert!(readings[5].starts_with("bokura te o tataite"), "{}", readings[5]);
        assert!(readings[6].ends_with("aishiteru"), "{}", readings[6]);
        assert!(readings[7].contains("futari de"), "{}", readings[7]);
        assert!(readings[8].contains("iratsuite mayoikonda"), "{}", readings[8]);
    }
}
