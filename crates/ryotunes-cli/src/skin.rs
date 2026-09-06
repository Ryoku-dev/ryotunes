//! `ryotunes-cli skin <check DIR...|list|show ID>`: a local, socket-free skin toolbox.
//!
//! Parses a skin manifest (docs/SKINS.md / skins/skin.schema.json) into a typed shape, enforces the
//! format-1 contract (id == folder, lowercase kebab; a mode with paper+ink; `#rrggbb` colours; WCAG
//! contrast on the ink pairs; type/shape/motion/decor/accent/wash ranges; fonts present) and prints
//! a swatch table with 24-bit ANSI blocks when stdout is a tty. Unknown keys warn; errors exit 1.
//! None of this needs the daemon.

use std::collections::BTreeMap;
use std::fs;
use std::io::IsTerminal;
use std::path::{Path, PathBuf};

use serde::Deserialize;
use serde_json::Value;

// --- the typed manifest -----------------------------------------------------------------------

const COLOR_KEYS: [&str; 8] =
    ["paper", "paperLift", "ink", "inkDim", "bone", "inkOnBone", "sun", "alert"];

#[derive(Deserialize, Default)]
struct Manifest {
    format: Option<u64>,
    id: Option<String>,
    name: Option<String>,
    author: Option<String>,
    version: Option<String>,
    license: Option<String>,
    homepage: Option<String>,
    description: Option<String>,
    #[serde(rename = "default")]
    default_mode: Option<String>,
    modes: Option<BTreeMap<String, Mode>>,
    #[serde(rename = "type")]
    type_: Option<TypeSpec>,
    shape: Option<Shape>,
    motion: Option<Motion>,
    decor: Option<String>,
    accent: Option<String>,
    wash: Option<f64>,
    fonts: Option<Vec<String>>,
    generated: Option<String>,
    // Captured only so `"$schema"` (paper/ember/mist reference the schema) is a known key.
    #[serde(rename = "$schema")]
    #[allow(dead_code)]
    schema: Option<String>,
    #[serde(flatten)]
    extra: BTreeMap<String, Value>,
}

#[derive(Deserialize, Default)]
struct Mode {
    paper: Option<String>,
    #[serde(rename = "paperLift")]
    paper_lift: Option<String>,
    ink: Option<String>,
    #[serde(rename = "inkDim")]
    ink_dim: Option<String>,
    bone: Option<String>,
    #[serde(rename = "inkOnBone")]
    ink_on_bone: Option<String>,
    sun: Option<String>,
    alert: Option<String>,
    #[serde(flatten)]
    extra: BTreeMap<String, Value>,
}

impl Mode {
    fn color(&self, key: &str) -> Option<&str> {
        match key {
            "paper" => self.paper.as_deref(),
            "paperLift" => self.paper_lift.as_deref(),
            "ink" => self.ink.as_deref(),
            "inkDim" => self.ink_dim.as_deref(),
            "bone" => self.bone.as_deref(),
            "inkOnBone" => self.ink_on_bone.as_deref(),
            "sun" => self.sun.as_deref(),
            "alert" => self.alert.as_deref(),
            _ => None,
        }
    }
}

#[derive(Deserialize, Default)]
struct TypeSpec {
    #[allow(dead_code)]
    display: Option<String>,
    #[allow(dead_code)]
    ui: Option<String>,
    #[allow(dead_code)]
    mono: Option<String>,
    #[serde(flatten)]
    extra: BTreeMap<String, Value>,
}

#[derive(Deserialize, Default)]
struct Shape {
    radius: Option<f64>,
    #[serde(rename = "radiusCard")]
    radius_card: Option<f64>,
    #[serde(flatten)]
    extra: BTreeMap<String, Value>,
}

#[derive(Deserialize, Default)]
struct Motion {
    snap: Option<f64>,
    #[serde(rename = "move")]
    move_: Option<f64>,
    swap: Option<f64>,
    slow: Option<f64>,
    #[serde(flatten)]
    extra: BTreeMap<String, Value>,
}

// --- colour + contrast ------------------------------------------------------------------------

/// Parse exactly `#rrggbb` (case-insensitive) into 8-bit channels; anything else is `None`.
fn parse_hex(s: &str) -> Option<(u8, u8, u8)> {
    let s = s.strip_prefix('#')?;
    if s.len() != 6 || !s.bytes().all(|b| b.is_ascii_hexdigit()) {
        return None;
    }
    let r = u8::from_str_radix(&s[0..2], 16).ok()?;
    let g = u8::from_str_radix(&s[2..4], 16).ok()?;
    let b = u8::from_str_radix(&s[4..6], 16).ok()?;
    Some((r, g, b))
}

/// WCAG 2.x relative luminance of an sRGB colour.
fn luminance((r, g, b): (u8, u8, u8)) -> f64 {
    fn lin(c: u8) -> f64 {
        let c = c as f64 / 255.0;
        if c <= 0.03928 {
            c / 12.92
        } else {
            ((c + 0.055) / 1.055).powf(2.4)
        }
    }
    0.2126 * lin(r) + 0.7152 * lin(g) + 0.0722 * lin(b)
}

/// WCAG contrast ratio (1..21) between two colours, order-independent.
fn contrast_ratio(a: (u8, u8, u8), b: (u8, u8, u8)) -> f64 {
    let (la, lb) = (luminance(a), luminance(b));
    let (hi, lo) = if la >= lb { (la, lb) } else { (lb, la) };
    (hi + 0.05) / (lo + 0.05)
}

/// Lowercase kebab-case: `a-z0-9` groups joined by single hyphens, no leading/trailing/double `-`.
fn is_kebab(s: &str) -> bool {
    if s.is_empty() {
        return false;
    }
    let ok_char = |c: char| c.is_ascii_lowercase() || c.is_ascii_digit() || c == '-';
    if !s.chars().all(ok_char) {
        return false;
    }
    if s.starts_with('-') || s.ends_with('-') || s.contains("--") {
        return false;
    }
    true
}

// --- validation -------------------------------------------------------------------------------

#[derive(Default)]
struct Report {
    errors: Vec<String>,
    warnings: Vec<String>,
}
impl Report {
    fn err(&mut self, m: String) {
        self.errors.push(m);
    }
    fn warn(&mut self, m: String) {
        self.warnings.push(m);
    }
}

fn contrast_checks(name: &str, mode: &Mode, r: &mut Report) {
    let g = |k: &str| mode.color(k).and_then(parse_hex);
    if let (Some(ink), Some(paper)) = (g("ink"), g("paper")) {
        let c = contrast_ratio(ink, paper);
        if c < 4.5 {
            r.err(format!("{name}: ink/paper contrast {c:.2} < 4.5 (WCAG AA body text)"));
        }
    }
    if let (Some(d), Some(paper)) = (g("inkDim"), g("paper")) {
        let c = contrast_ratio(d, paper);
        if c < 3.0 {
            r.warn(format!("{name}: inkDim/paper contrast {c:.2} < 3.0 (dim text hard to read)"));
        }
    }
    if let (Some(iob), Some(bone)) = (g("inkOnBone"), g("bone")) {
        let c = contrast_ratio(iob, bone);
        if c < 4.5 {
            r.err(format!("{name}: inkOnBone/bone contrast {c:.2} < 4.5 (WCAG AA body text)"));
        }
    }
    if let (Some(sun), Some(paper)) = (g("sun"), g("paper")) {
        let c = contrast_ratio(sun, paper);
        if c < 3.0 {
            r.warn(format!("{name}: sun/paper contrast {c:.2} < 3.0 (accent barely separates)"));
        }
    }
}

/// The heart of `skin check`: every rule the format-1 contract enforces. `folder` is the directory
/// name the id must equal; `dir` is where fonts and preview.png live.
fn validate(m: &Manifest, folder: &str, dir: &Path) -> Report {
    let mut r = Report::default();

    match m.format {
        Some(1) => {}
        Some(other) => r.err(format!("format must be 1, got {other}")),
        None => r.err("format is required and must be 1".into()),
    }

    match &m.id {
        Some(id) => {
            if !is_kebab(id) {
                r.err(format!(
                    "id \"{id}\" must be lowercase kebab-case (a-z, 0-9, single hyphens)"
                ));
            }
            if id != folder {
                r.err(format!("id \"{id}\" must equal its folder name \"{folder}\""));
            }
        }
        None => r.err(format!("id is required and must equal the folder name \"{folder}\"")),
    }

    if let Some(d) = &m.default_mode {
        if d != "dark" && d != "light" {
            r.err(format!("default \"{d}\" must be \"dark\" or \"light\""));
        }
    }

    match &m.modes {
        None => r.err("modes is required: at least one mode carrying paper and ink".into()),
        Some(modes) if modes.is_empty() => {
            r.err("modes is empty: need at least one mode with paper and ink".into());
        }
        Some(modes) => {
            let mut any_full = false;
            for (name, mode) in modes {
                if name != "dark" && name != "light" {
                    r.warn(format!(
                        "mode \"{name}\" is neither \"dark\" nor \"light\"; the client ignores it"
                    ));
                }
                for key in COLOR_KEYS {
                    if let Some(hex) = mode.color(key) {
                        if parse_hex(hex).is_none() {
                            r.err(format!("{name}.{key} \"{hex}\" is not #rrggbb"));
                        }
                    }
                }
                if mode.paper.is_some() && mode.ink.is_some() {
                    any_full = true;
                }
                contrast_checks(name, mode, &mut r);
                for k in mode.extra.keys() {
                    r.warn(format!("unknown key \"modes.{name}.{k}\""));
                }
            }
            if !any_full {
                r.err("no mode carries both paper and ink".into());
            }
        }
    }

    if let Some(d) = &m.decor {
        if d != "rich" && d != "calm" {
            r.err(format!("decor \"{d}\" must be \"rich\" or \"calm\""));
        }
    }

    if let Some(a) = &m.accent {
        let ok = matches!(a.as_str(), "artwork" | "provider" | "sun") || parse_hex(a).is_some();
        if !ok {
            r.err(format!("accent \"{a}\" must be \"artwork\", \"provider\", \"sun\", or #rrggbb"));
        }
    }

    if let Some(w) = m.wash {
        if !(0.0..=2.0).contains(&w) {
            r.err(format!("wash {w} is out of range 0..2"));
        }
    }

    if let Some(s) = &m.shape {
        for (k, v) in [("radius", s.radius), ("radiusCard", s.radius_card)] {
            if let Some(v) = v {
                if v < 0.0 {
                    r.err(format!("shape.{k} {v} must be >= 0"));
                }
            }
        }
        for k in s.extra.keys() {
            r.warn(format!("unknown key \"shape.{k}\""));
        }
    }

    if let Some(mo) = &m.motion {
        for (k, v) in [("snap", mo.snap), ("move", mo.move_), ("swap", mo.swap), ("slow", mo.slow)]
        {
            if let Some(v) = v {
                if v < 0.0 {
                    r.err(format!("motion.{k} {v}ms must be >= 0"));
                }
            }
        }
        for k in mo.extra.keys() {
            r.warn(format!("unknown key \"motion.{k}\""));
        }
    }

    if let Some(t) = &m.type_ {
        for k in t.extra.keys() {
            r.warn(format!("unknown key \"type.{k}\""));
        }
    }

    if let Some(fonts) = &m.fonts {
        for f in fonts {
            if !dir.join(f).exists() {
                r.err(format!("font \"{f}\" not found under {}", dir.display()));
            }
        }
    }

    if !dir.join("preview.png").exists() {
        r.warn("preview.png is missing (recommended: 800x500, scripts/dev/skin-preview.sh)".into());
    }

    for k in m.extra.keys() {
        r.warn(format!("unknown key \"{k}\""));
    }

    r
}

// --- rendering --------------------------------------------------------------------------------

fn block(rgb: (u8, u8, u8)) -> String {
    format!("\x1b[48;2;{};{};{}m  \x1b[0m", rgb.0, rgb.1, rgb.2)
}

/// Ordered (dark, light, then any extra) columns of the modes a manifest declares.
fn mode_columns(m: &Manifest) -> Vec<(String, &Mode)> {
    let mut cols = Vec::new();
    let modes = match &m.modes {
        Some(x) => x,
        None => return cols,
    };
    for name in ["dark", "light"] {
        if let Some(md) = modes.get(name) {
            cols.push((name.to_string(), md));
        }
    }
    for (name, md) in modes {
        if name != "dark" && name != "light" {
            cols.push((name.clone(), md));
        }
    }
    cols
}

fn print_swatches(m: &Manifest, tty: bool) {
    let cols = mode_columns(m);
    if cols.is_empty() {
        return;
    }
    let mut hdr = format!("  {:<11}", "colour");
    for (name, _) in &cols {
        hdr.push_str(&format!("   {:<9}", name));
    }
    println!("{}", hdr.trim_end());
    for key in COLOR_KEYS {
        let mut line = format!("  {key:<11}");
        for (_, md) in &cols {
            match md.color(key) {
                Some(hex) => {
                    let pre = match parse_hex(hex) {
                        Some(rgb) if tty => format!("{} ", block(rgb)),
                        _ => "   ".to_string(),
                    };
                    line.push_str(&pre);
                    line.push_str(&format!("{hex:<9}"));
                }
                None => line.push_str(&format!("   {:<9}", "—")),
            }
        }
        println!("{}", line.trim_end());
    }
}

// --- skin directory resolution (list / show) --------------------------------------------------

#[derive(Clone, Copy)]
enum Origin {
    Dev,
    User,
    Shipped,
}
impl Origin {
    fn label(self) -> &'static str {
        match self {
            Origin::Dev => "dev",
            Origin::User => "user",
            Origin::Shipped => "shipped",
        }
    }
}

/// The same search order the client uses (Skin.qml): dev dirs, the user dir, then the shipped dir
/// (`/usr/share/ryotunes/skins` when installed, `./skins` in a checkout). First hit per id wins.
fn skin_dirs() -> Vec<(Origin, PathBuf)> {
    let mut dirs = Vec::new();
    if let Ok(v) = std::env::var("RYOTUNES_SKIN_DIRS") {
        for d in v.split(':').filter(|s| !s.is_empty()) {
            dirs.push((Origin::Dev, PathBuf::from(d)));
        }
    }
    let cfg = std::env::var("XDG_CONFIG_HOME")
        .ok()
        .filter(|s| !s.is_empty())
        .unwrap_or_else(|| format!("{}/.config", std::env::var("HOME").unwrap_or_default()));
    dirs.push((Origin::User, PathBuf::from(format!("{cfg}/ryotunes/skins"))));
    let ship = PathBuf::from("/usr/share/ryotunes/skins");
    if ship.is_dir() {
        dirs.push((Origin::Shipped, ship));
    }
    let local = PathBuf::from("skins");
    if local.is_dir() {
        dirs.push((Origin::Shipped, local));
    }
    dirs
}

struct Cat {
    id: String,
    name: String,
    version: String,
    source: &'static str,
    dir: PathBuf,
    manifest: Manifest,
}

fn catalogue() -> Vec<Cat> {
    let mut out = Vec::new();
    let mut seen = std::collections::BTreeSet::new();
    for (origin, dir) in skin_dirs() {
        let rd = match fs::read_dir(&dir) {
            Ok(r) => r,
            Err(_) => continue,
        };
        let mut subs: Vec<PathBuf> =
            rd.filter_map(|e| e.ok().map(|e| e.path())).filter(|p| p.is_dir()).collect();
        subs.sort();
        for sub in subs {
            let json = sub.join("skin.json");
            if !json.is_file() {
                continue;
            }
            let text = match fs::read_to_string(&json) {
                Ok(t) => t,
                Err(_) => continue,
            };
            let m: Manifest = match serde_json::from_str(&text) {
                Ok(m) => m,
                Err(_) => continue,
            };
            let folder =
                sub.file_name().map(|s| s.to_string_lossy().into_owned()).unwrap_or_default();
            let id = m.id.clone().unwrap_or(folder);
            if !seen.insert(id.clone()) {
                continue;
            }
            out.push(Cat {
                name: m.name.clone().unwrap_or_else(|| id.clone()),
                version: m.version.clone().unwrap_or_default(),
                source: origin.label(),
                dir: sub,
                id,
                manifest: m,
            });
        }
    }
    out
}

// --- commands ---------------------------------------------------------------------------------

/// Resolve a `check` argument (a skin folder or a skin.json path) to (manifest, folder-name, dir).
fn load(path: &Path) -> Result<(Manifest, String, PathBuf), String> {
    let is_json_file =
        path.is_file() && path.file_name().map(|f| f == "skin.json").unwrap_or(false);
    let (json, dir) = if is_json_file {
        let dir = path.parent().unwrap_or(Path::new(".")).to_path_buf();
        (path.to_path_buf(), dir)
    } else {
        (path.join("skin.json"), path.to_path_buf())
    };
    let folder = dir
        .file_name()
        .map(|s| s.to_string_lossy().into_owned())
        .unwrap_or_else(|| dir.display().to_string());
    let text = fs::read_to_string(&json).map_err(|e| format!("{}: {e}", json.display()))?;
    let m: Manifest =
        serde_json::from_str(&text).map_err(|e| format!("{}: {e}", json.display()))?;
    Ok((m, folder, dir))
}

fn cmd_check(paths: &[String]) -> i32 {
    if paths.is_empty() {
        eprintln!("usage: ryotunes-cli skin check <dir-or-skin.json> [more...]");
        return 2;
    }
    let tty = std::io::stdout().is_terminal();
    let mut had_error = false;
    for (i, p) in paths.iter().enumerate() {
        if i > 0 {
            println!();
        }
        match load(Path::new(p)) {
            Err(e) => {
                println!("{p}: {e}");
                had_error = true;
            }
            Ok((m, folder, dir)) => {
                let title = m.name.clone().unwrap_or_else(|| folder.clone());
                let ver = m.version.clone().unwrap_or_default();
                let tag = if ver.is_empty() { String::new() } else { format!(" {ver}") };
                println!("{} — {title}{tag}", dir.display());
                print_swatches(&m, tty);
                let report = validate(&m, &folder, &dir);
                for w in &report.warnings {
                    println!("  warn:  {w}");
                }
                for e in &report.errors {
                    println!("  error: {e}");
                }
                let (ne, nw) = (report.errors.len(), report.warnings.len());
                if ne == 0 {
                    println!("  PASS  ({nw} warning{})", plural(nw));
                } else {
                    println!("  FAIL  ({ne} error{}, {nw} warning{})", plural(ne), plural(nw));
                    had_error = true;
                }
            }
        }
    }
    if had_error {
        1
    } else {
        0
    }
}

fn plural(n: usize) -> &'static str {
    if n == 1 {
        ""
    } else {
        "s"
    }
}

fn cmd_list() -> i32 {
    let cat = catalogue();
    if cat.is_empty() {
        println!("no skins found (looked in RYOTUNES_SKIN_DIRS, ~/.config/ryotunes/skins, ./skins, /usr/share/ryotunes/skins)");
        return 0;
    }
    println!("{:<18}{:<10}{:<10}{}", "id", "source", "version", "name");
    for c in &cat {
        println!("{:<18}{:<10}{:<10}{}", c.id, c.source, c.version, c.name);
    }
    0
}

fn cmd_show(id: Option<&str>) -> i32 {
    let id = match id {
        Some(i) => i,
        None => {
            eprintln!("usage: ryotunes-cli skin show <id>");
            return 2;
        }
    };
    let cat = catalogue();
    let c = match cat.iter().find(|c| c.id == id) {
        Some(c) => c,
        None => {
            eprintln!("skin \"{id}\" is not installed (try: ryotunes-cli skin list)");
            return 1;
        }
    };
    let tty = std::io::stdout().is_terminal();
    println!("{}  ({}, {})", c.id, c.source, c.dir.display());
    println!("  name     {}", c.name);
    if let Some(a) = c.manifest.author.as_deref().filter(|s| !s.is_empty()) {
        println!("  author   {a}");
    }
    if !c.version.is_empty() {
        println!("  version  {}", c.version);
    }
    if let Some(l) = c.manifest.license.as_deref().filter(|s| !s.is_empty()) {
        println!("  license  {l}");
    }
    if let Some(h) = c.manifest.homepage.as_deref().filter(|s| !s.is_empty()) {
        println!("  homepage {h}");
    }
    println!("  default  {}", c.manifest.default_mode.clone().unwrap_or_else(|| "dark".into()));
    if let Some(d) = c.manifest.description.as_deref().filter(|s| !s.is_empty()) {
        println!("  {d}");
    }
    if let Some(g) = c.manifest.generated.as_deref().filter(|s| !s.is_empty()) {
        println!("  generated by {g} — fork it from Settings to edit");
    }
    print_swatches(&c.manifest, tty);
    let report = validate(&c.manifest, &c.id, &c.dir);
    for w in &report.warnings {
        println!("  warn:  {w}");
    }
    for e in &report.errors {
        println!("  error: {e}");
    }
    0
}

pub fn run(args: &[String]) -> i32 {
    match args.first().map(String::as_str) {
        Some("check") => cmd_check(&args[1..]),
        Some("list") => cmd_list(),
        Some("show") => cmd_show(args.get(1).map(String::as_str)),
        Some(other) => {
            eprintln!("unknown skin command \"{other}\"");
            eprintln!("usage: ryotunes-cli skin <check DIR...|list|show ID>");
            2
        }
        None => {
            eprintln!("usage: ryotunes-cli skin <check DIR...|list|show ID>");
            2
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn contrast_known_pairs() {
        // Black on white is the WCAG maximum, 21:1; a colour on itself is 1:1.
        assert!((contrast_ratio((255, 255, 255), (0, 0, 0)) - 21.0).abs() < 0.01);
        assert!((contrast_ratio((10, 20, 30), (10, 20, 30)) - 1.0).abs() < 1e-9);
        // #777777 on white is a well-known ~4.48 (just under AA body text).
        let c = contrast_ratio((0x77, 0x77, 0x77), (255, 255, 255));
        assert!(c > 4.4 && c < 4.6, "expected ~4.48, got {c}");
        assert!(
            contrast_ratio((0, 0, 0), (255, 255, 255)) - contrast_ratio((255, 255, 255), (0, 0, 0))
                == 0.0
        );
    }

    #[test]
    fn kebab_rule() {
        for good in ["paper", "my-cool-skin", "skin2", "a1-b2-c3"] {
            assert!(is_kebab(good), "{good} should be kebab");
        }
        for bad in ["Paper", "-x", "x-", "a--b", "a_b", "", "café", "UPPER"] {
            assert!(!is_kebab(bad), "{bad} should not be kebab");
        }
    }

    #[test]
    fn parse_hex_is_strict() {
        assert_eq!(parse_hex("#050505"), Some((5, 5, 5)));
        assert_eq!(parse_hex("#FFFFFF"), Some((255, 255, 255)));
        assert!(parse_hex("#fff").is_none()); // shorthand not allowed
        assert!(parse_hex("050505").is_none()); // missing #
        assert!(parse_hex("#gggggg").is_none()); // non-hex
        assert!(parse_hex("#0505055").is_none()); // too long
    }

    #[test]
    fn bad_hex_is_an_error() {
        let json = r##"{
            "format": 1, "id": "demo",
            "modes": { "dark": { "paper": "#050505", "ink": "#gggggg" } }
        }"##;
        let m: Manifest = serde_json::from_str(json).unwrap();
        let report = validate(&m, "demo", Path::new("/nonexistent-skin-dir"));
        assert!(
            report.errors.iter().any(|e| e.contains("dark.ink") && e.contains("#rrggbb")),
            "bad ink hex should error, got {:?}",
            report.errors
        );
    }

    #[test]
    fn low_contrast_ink_fails() {
        // ink barely different from paper: below 4.5 -> error.
        let json = r##"{
            "format": 1, "id": "demo",
            "modes": { "dark": { "paper": "#202020", "ink": "#2a2a2a" } }
        }"##;
        let m: Manifest = serde_json::from_str(json).unwrap();
        let report = validate(&m, "demo", Path::new("/nonexistent-skin-dir"));
        assert!(
            report.errors.iter().any(|e| e.contains("ink/paper contrast")),
            "low ink/paper contrast should error, got {:?}",
            report.errors
        );
    }

    #[test]
    fn id_must_match_folder_and_format_must_be_one() {
        let json = r##"{
            "format": 2, "id": "Other",
            "modes": { "dark": { "paper": "#050505", "ink": "#d7cfc6" } }
        }"##;
        let m: Manifest = serde_json::from_str(json).unwrap();
        let report = validate(&m, "demo", Path::new("/nonexistent-skin-dir"));
        assert!(report.errors.iter().any(|e| e.contains("format must be 1")));
        assert!(report.errors.iter().any(|e| e.contains("kebab")));
        assert!(report.errors.iter().any(|e| e.contains("folder name")));
    }

    #[test]
    fn unknown_key_warns_but_does_not_fail() {
        let json = r##"{
            "format": 1, "id": "demo", "wobble": 3,
            "modes": { "dark": { "paper": "#050505", "ink": "#d7cfc6", "bogus": "x" } }
        }"##;
        let m: Manifest = serde_json::from_str(json).unwrap();
        let report = validate(&m, "demo", Path::new("/nonexistent-skin-dir"));
        assert!(report.warnings.iter().any(|w| w.contains("\"wobble\"")));
        assert!(report.warnings.iter().any(|w| w.contains("modes.dark.bogus")));
    }
}
