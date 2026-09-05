//! Ryotunes SoundCloud provider.
//!
//! A guest-only client for SoundCloud's internal `api-v2` (no account, no OAuth). It discovers a
//! public `client_id` by scraping soundcloud.com's asset bundles, caches it in memory and on disk,
//! and re-scrapes once on a 401/403 (the id rotates). Tracks stream as HLS m3u8 playlists that mpv
//! plays directly; waveforms come from SoundCloud's own peak arrays, downsampled for the seek bar.
//!
//! Boundary: this crate knows nothing about GPUI, Tauri, mpv, or UI state. It maps SoundCloud's
//! (frequently field-absent) JSON into tolerant models and hands them to the daemon bridge.

mod client;
mod models;

pub use client::{Error, Result, SoundCloud};
pub use models::{
    DiscoverItem, Page, Playlist, PlaylistDetail, SearchResults, Selection, SystemPlaylist, Track,
    Transcoding, User, UserRef,
};

use std::sync::LazyLock;

use regex::Regex;

/// Asset-bundle `<script src>` matcher; the `client_id` lives in one of these bundles.
static SCRIPT_SRC_RE: LazyLock<Regex> =
    LazyLock::new(|| Regex::new(r#"src="(https://a-v2\.sndcdn\.com/assets/[^"]+\.js)""#).unwrap());
/// The `client_id` literal inside a bundle (`client_id:"…"` or `client_id="…"`, exactly 32 chars).
static CLIENT_ID_RE: LazyLock<Regex> =
    LazyLock::new(|| Regex::new(r#"client_id\s*[:=]\s*"([A-Za-z0-9]{32})""#).unwrap());

/// Rewrite a SoundCloud artwork/avatar URL to its 500×500 variant. SoundCloud names sizes with a
/// `-<size>.jpg` suffix; the API hands back `-large` (100 px). The `-large.` anchor (with the dot)
/// only matches the size token, never an artwork id, so a URL without it passes through unchanged.
pub fn upscale_artwork(url: &str) -> String {
    url.replace("-large.", "-t500x500.")
}

/// Downsample a SoundCloud waveform (`samples`, each `0..=height`) to `buckets` peaks scaled to
/// `0..=100`. Each output bucket is the **max** of the input samples that fall in it (peak-hold, so
/// the bar chart keeps the loud transients a mean would wash out); `height` is the full-scale
/// reference. Always returns exactly `buckets` values.
pub fn downsample_waveform(samples: &[u32], height: u32, buckets: usize) -> Vec<u8> {
    if buckets == 0 {
        return Vec::new();
    }
    if samples.is_empty() {
        return vec![0; buckets];
    }
    let n = samples.len();
    // Fall back to the observed peak when the payload omits/zeroes `height`, so scaling never
    // divides by zero and still spans the full 0..=100 range.
    let denom =
        u64::from(if height == 0 { samples.iter().copied().max().unwrap_or(1) } else { height })
            .max(1);
    let mut out = Vec::with_capacity(buckets);
    for i in 0..buckets {
        let start = i * n / buckets;
        let end = (((i + 1) * n) / buckets).max(start + 1).min(n);
        let peak = samples[start..end].iter().copied().max().unwrap_or(0);
        out.push((u64::from(peak) * 100 / denom).min(100) as u8);
    }
    out
}

/// Extract every SoundCloud asset-bundle `<script src>` from a soundcloud.com page. The `client_id`
/// lives in one of these; callers scan them last-first (the id sits in a late bundle).
pub(crate) fn asset_script_srcs(html: &str) -> Vec<String> {
    SCRIPT_SRC_RE.captures_iter(html).map(|c| c[1].to_string()).collect()
}

/// Pull the first `client_id":"…32 chars…"` (or `client_id=…`) out of a JS bundle body.
pub(crate) fn scrape_client_id_from_js(js: &str) -> Option<String> {
    CLIENT_ID_RE.captures(js).map(|c| c[1].to_string())
}

/// Parse SoundCloud's space-delimited `tag_list` (multi-word tags are double-quoted) into tags.
pub(crate) fn parse_tag_list(list: &str) -> Vec<String> {
    let mut tags = Vec::new();
    let mut buf = String::new();
    let mut quoted = false;
    for c in list.chars() {
        match c {
            '"' => {
                if quoted && !buf.is_empty() {
                    tags.push(std::mem::take(&mut buf));
                }
                quoted = !quoted;
            }
            ' ' | '\t' if !quoted => {
                if !buf.is_empty() {
                    tags.push(std::mem::take(&mut buf));
                }
            }
            _ => buf.push(c),
        }
    }
    if !buf.is_empty() {
        tags.push(buf);
    }
    tags
}

/// `None` for an absent *or empty* string — SoundCloud sends `""` for "unset" as often as `null`.
pub(crate) fn nonempty(s: Option<String>) -> Option<String> {
    s.filter(|v| !v.is_empty())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn artwork_upscales_large_to_t500() {
        assert_eq!(
            upscale_artwork("https://i1.sndcdn.com/artworks-abc123-0-large.jpg"),
            "https://i1.sndcdn.com/artworks-abc123-0-t500x500.jpg"
        );
        // Avatars share the convention.
        assert_eq!(
            upscale_artwork("https://i1.sndcdn.com/avatars-XYZ-large.png"),
            "https://i1.sndcdn.com/avatars-XYZ-t500x500.png"
        );
        // No size token -> untouched (default/gravatar art, and never mangles an id).
        let bare = "https://i1.sndcdn.com/artworks-large-id-nosize";
        assert_eq!(upscale_artwork(bare), bare);
    }

    #[test]
    fn waveform_downsamples_to_240_peaks_in_range() {
        // 1800 samples ramping 0..1800 (SoundCloud's usual width), full scale 1800.
        let samples: Vec<u32> = (0..1800).collect();
        let out = downsample_waveform(&samples, 1800, 240);
        assert_eq!(out.len(), 240);
        assert!(out.iter().all(|&s| s <= 100));
        // Peak-hold + ramp: monotonic non-decreasing; last bucket at (near) full scale. The ramp's
        // top sample is 1799/1800, so it lands at 99 — the point is it reaches the ceiling, not 0.
        assert!(out.windows(2).all(|w| w[0] <= w[1]));
        assert!(*out.last().unwrap() >= 99);
        assert!(out[0] < 5);
    }

    #[test]
    fn waveform_bucket_is_max_not_mean() {
        // A single loud spike in bucket 0 must survive as ~full scale (peak-hold, not averaging).
        let mut samples = vec![0u32; 240];
        samples[0] = 100;
        let out = downsample_waveform(&samples, 100, 240);
        assert_eq!(out.len(), 240);
        assert_eq!(out[0], 100);
        assert!(out[1..].iter().all(|&s| s == 0));
    }

    #[test]
    fn waveform_scales_relative_to_height() {
        // Samples top out at half of `height` -> peaks land at ~50, never above.
        let samples = vec![70u32; 240];
        let out = downsample_waveform(&samples, 140, 240);
        assert!(out.iter().all(|&s| s == 50));
    }

    #[test]
    fn client_id_regex_finds_id_in_bundle_snippet() {
        // The real minified bundle form: `client_id:"…"` in a JS object literal.
        let js = r#"...,e.exports={api_host:"api-v2.soundcloud.com",client_id:"Pb72ranhoyt6gw7hM7TkzUItXlMWSNSo",app_version:"1"},..."#;
        assert_eq!(
            scrape_client_id_from_js(js).as_deref(),
            Some("Pb72ranhoyt6gw7hM7TkzUItXlMWSNSo")
        );
        // The `=` assignment form (the regex accepts `[:=]`).
        let js2 = r#"var t;t.client_id="abcdefghij0123456789ABCDEFGHIJKL";"#;
        assert_eq!(
            scrape_client_id_from_js(js2).as_deref(),
            Some("abcdefghij0123456789ABCDEFGHIJKL")
        );
        // A wrong-length near-miss must not match (id is exactly 32).
        assert_eq!(scrape_client_id_from_js(r#"client_id:"tooShort01234567890123456789""#), None);
        assert_eq!(scrape_client_id_from_js("no client id here"), None);
    }

    #[test]
    fn asset_srcs_collect_bundle_scripts_only() {
        let html = r#"
            <script src="https://a-v2.sndcdn.com/assets/0-abc.js"></script>
            <script src="https://example.com/other.js"></script>
            <script src="https://a-v2.sndcdn.com/assets/49-xyz.js" crossorigin></script>
        "#;
        let srcs = asset_script_srcs(html);
        assert_eq!(
            srcs,
            vec![
                "https://a-v2.sndcdn.com/assets/0-abc.js".to_string(),
                "https://a-v2.sndcdn.com/assets/49-xyz.js".to_string(),
            ]
        );
    }

    #[test]
    fn tag_list_respects_quoted_multiword_tags() {
        assert_eq!(
            parse_tag_list(r#"dance "hip hop" pop "drum and bass""#),
            vec!["dance", "hip hop", "pop", "drum and bass"]
        );
        assert!(parse_tag_list("").is_empty());
    }
}
