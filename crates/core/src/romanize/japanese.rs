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

fn cached(key: u64) -> Option<Arc<Vec<String>>> {
    let cache = CACHE.lock().unwrap_or_else(|poisoned| poisoned.into_inner());
    cache.iter().find(|(k, _)| *k == key).map(|(_, readings)| readings.clone())
}

/// Load the dictionary, run `f`, and give the memory back before returning.
fn with_dictionary<T>(path: &Path, f: impl FnOnce(&vibrato::Tokenizer) -> T) -> Option<T> {
    let file = std::fs::File::open(path).ok()?;
    let decoder = match ruzstd::decoding::StreamingDecoder::new(std::io::BufReader::new(file)) {
        Ok(decoder) => decoder,
        Err(e) => {
            tracing::warn!(error = %e, path = %path.display(), "romanize: unreadable Japanese dictionary");
            return None;
        }
    };
    let dictionary = match vibrato::Dictionary::read(decoder) {
        Ok(dictionary) => dictionary,
        Err(e) => {
            tracing::warn!(error = %e, path = %path.display(), "romanize: invalid Japanese dictionary");
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
    let mut out = String::new();
    let mut word = String::new(); // katakana of the word being built
    let mut prev_conjugates = false;
    let mut glue_next = false; // after an opening bracket
    let flush = |word: &mut String, out: &mut String| {
        if !word.is_empty() {
            out.push_str(&katakana_to_romaji(word));
            word.clear();
        }
    };
    for token in tokens {
        match piece(token) {
            Piece::Space => {
                flush(&mut word, &mut out);
                if !out.is_empty() && !out.ends_with(' ') {
                    out.push(' ');
                }
                prev_conjugates = false;
            }
            Piece::Punct(text, opening) => {
                flush(&mut word, &mut out);
                if opening && !out.is_empty() && !out.ends_with(' ') {
                    out.push(' ');
                }
                out.push_str(&text);
                glue_next = opening;
                prev_conjugates = false;
            }
            Piece::Literal(text) => {
                flush(&mut word, &mut out);
                space_before(&mut out, glue_next);
                out.push_str(&text);
                glue_next = false;
                prev_conjugates = false;
            }
            Piece::Kana(kana) => {
                let joins =
                    !word.is_empty() && (word.ends_with('ッ') || attaches(token, prev_conjugates));
                if !joins {
                    flush(&mut word, &mut out);
                    space_before(&mut out, glue_next);
                }
                word.push_str(&kana);
                glue_next = false;
                prev_conjugates = matches!(token.pos.as_str(), "動詞" | "形容詞" | "助動詞");
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

fn piece(token: &Token) -> Piece {
    let surface = token.surface.as_str();
    if surface.trim().is_empty() {
        return Piece::Space;
    }
    if token.pos == "記号" || surface.chars().all(|c| c.is_ascii_punctuation() || is_cjk_punct(c))
    {
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
    Piece::Literal(surface.to_owned())
}

/// Whether a token continues the previous word instead of starting one.
fn attaches(token: &Token, prev_conjugates: bool) -> bool {
    match token.pos.as_str() {
        "助動詞" => prev_conjugates && !matches!(token.base.as_str(), "だ" | "です"),
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

fn is_cjk_punct(c: char) -> bool {
    ('\u{3000}'..='\u{303F}').contains(&c) || ('\u{FF01}'..='\u{FF0F}').contains(&c) || c == '…'
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
        let lines =
            ["君の名は", "僕の心の全てを奪った", "日が沈み出した空と君の姿", "東京タワーへ行こう"];
        let readings = romanize_lines_with(&path, &lines).expect("dictionary loads");
        assert_eq!(readings[0], "kimi no na wa");
        assert_eq!(readings[1], "boku no kokoro no subete o ubatta");
        assert!(readings[2].starts_with("hi ga shizumi"), "{}", readings[2]);
        assert!(readings[3].contains("toukyou") && readings[3].contains(" e "), "{}", readings[3]);
    }
}
