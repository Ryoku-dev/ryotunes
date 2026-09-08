//! Local artwork + lyrics enrichment for finished downloads.
//!
//! Runs once, after yt-dlp reports success and before publication, only when `embed_metadata` is
//! on. Everything here is best-effort and bounded: a lookup that fails, times out, or finds no
//! confident match is a warning on the job's history, never a download failure.
//!
//! Cover art is taken from the track's own provider thumbnail first (only over HTTPS from the
//! catalogue CDNs the app already serves — never localhost, a private address, or an arbitrary
//! host). When that is unavailable it falls back to the free iTunes Search API, and accepts a
//! result only on an exact normalized title+artist match whose length is within five seconds of
//! the file we actually downloaded — not merely the first search hit, which ranks remixes and
//! live cuts next to the original.
//!
//! Lyrics reuse [`ryotunes_core::lyrics::get_lyrics`] and its cache/provider chain (LRCLIB and the
//! rest) under one overall timeout. The plain text is embedded in the audio tag; a synced `.lrc`
//! (or plain `.txt`) sidecar is written next to it.
//!
//! Tag mutation happens on an isolated temp copy inside the staging directory and only replaces
//! the staged audio on success, so a failed embed can never corrupt the download. No re-encode.

use std::path::{Path, PathBuf};
use std::time::Duration;

use lofty::config::WriteOptions;
use lofty::file::{AudioFile, TaggedFileExt};
use lofty::picture::{MimeType, Picture, PictureType};
use lofty::tag::{Accessor, ItemKey, Tag};
use ryotunes_core::lyrics::{get_lyrics, Lyrics, LyricsRequest};
use ryotunes_core::state::AppState;
use serde::Deserialize;

use crate::downloads::DownloadJob;

/// What enrichment produced: sidecar files written beside the staged audio (cover art and/or
/// lyrics) that publication should relocate next to the final track, plus best-effort warnings
/// worth surfacing in the job's history.
pub struct Enrichment {
    pub sidecars: Vec<PathBuf>,
    pub warnings: Vec<String>,
}

/// Provider image CDNs the app itself already serves thumbnails from. Matched as domain suffixes
/// (leading dot), so `evil-scdn.co` and bare IPs/localhost never qualify.
const TRUSTED_ART_SUFFIXES: [&str; 5] =
    [".ytimg.com", ".ggpht.com", ".googleusercontent.com", ".sndcdn.com", ".scdn.co"];

/// Apple's artwork CDN. The iTunes Search API returns URLs here; we still verify the host before
/// fetching rather than trusting whatever the response points at.
const MZSTATIC_SUFFIX: &str = ".mzstatic.com";

/// A search hit's length may sit at most this far from the file we downloaded. Same tolerance the
/// core lyrics matcher uses, for the same reason: catalogues rank remixes beside originals.
const MATCH_TOLERANCE_SECS: f64 = 5.0;

const MAX_IMAGE_BYTES: usize = 12 * 1024 * 1024;
const MAX_JSON_BYTES: usize = 1024 * 1024;
const IMAGE_TIMEOUT: Duration = Duration::from_secs(15);
const API_TIMEOUT: Duration = Duration::from_secs(12);
/// Whole-artwork budget (provider thumbnail then the iTunes fallback chain).
const ARTWORK_TIMEOUT: Duration = Duration::from_secs(30);
/// Whole-lookup budget for the lyrics provider chain.
const LYRICS_TIMEOUT: Duration = Duration::from_secs(20);

/// Validated cover art ready to embed and write as a sidecar.
struct Artwork {
    bytes: Vec<u8>,
    mime: MimeType,
    ext: &'static str,
}

/// Enrich a freshly downloaded, staged audio file with cover art and lyrics.
///
/// `audio` is the staged `audio.<ext>` yt-dlp produced. Sidecars are written with
/// `audio.with_extension(...)` in the same directory and returned for publication. The staged
/// audio's title/artist are corrected to the catalogue values on `job` (yt-dlp tags them from the
/// uploader/video title), and artwork + plain lyrics are embedded when found.
pub async fn enrich(state: &AppState, audio: &Path, job: &DownloadJob) -> Enrichment {
    let mut warnings = Vec::new();

    // The real downloaded length, from the file itself — the trustworthy signal for matching.
    let duration = read_duration(audio).await;

    let artwork_fut = async {
        match tokio::time::timeout(
            ARTWORK_TIMEOUT,
            resolve_artwork(&job.thumbnail, &job.title, &job.artists, duration),
        )
        .await
        {
            Ok(pair) => pair,
            Err(_) => (None, Some("Cover art lookup timed out.".to_string())),
        }
    };
    let lyrics_fut = resolve_lyrics(state, job, duration);
    let ((artwork, art_warn), (lyrics, lyric_warn)) = tokio::join!(artwork_fut, lyrics_fut);
    warnings.extend(art_warn);
    warnings.extend(lyric_warn);

    // Shape the lyric outputs: the tag always carries plain text; the sidecar is synced `.lrc`
    // when we have timings, otherwise plain `.txt`.
    let mut lyric_tag: Option<String> = None;
    let mut lyric_sidecar: Option<(PathBuf, String)> = None;
    if let Some(found) = &lyrics {
        let plain = plain_text(found);
        if plain.trim().is_empty() {
            warnings.push("No lyrics were found for this track.".to_string());
        } else {
            lyric_tag = Some(plain.clone());
            lyric_sidecar = Some(match to_lrc(found) {
                Some(lrc) => (audio.with_extension("lrc"), lrc),
                None => (audio.with_extension("txt"), plain),
            });
        }
    }

    // File and tag work is blocking; do it off the async runtime and only report sidecars that
    // were actually written.
    let audio_buf = audio.to_path_buf();
    let title = job.title.clone();
    let artist = job.artists.clone();
    match tokio::task::spawn_blocking(move || {
        write_media(&audio_buf, &title, &artist, artwork, lyric_tag, lyric_sidecar)
    })
    .await
    {
        Ok((sidecars, mut blocking_warnings)) => {
            warnings.append(&mut blocking_warnings);
            Enrichment { sidecars, warnings }
        }
        Err(error) => {
            warnings.push(format!("Metadata enrichment did not finish: {error}"));
            Enrichment { sidecars: Vec::new(), warnings }
        }
    }
}

/// The staged file's real length in seconds, or `None` when lofty can't read it (unknown, so
/// matching falls back to title+artist alone).
async fn read_duration(audio: &Path) -> Option<f64> {
    let path = audio.to_path_buf();
    tokio::task::spawn_blocking(move || {
        lofty::read_from_path(&path)
            .ok()
            .map(|tagged| tagged.properties().duration().as_secs_f64())
            .filter(|secs| *secs > 0.0)
    })
    .await
    .ok()
    .flatten()
}

/// Cover art, provider thumbnail first then the iTunes fallback. Returns the art plus an optional
/// honest warning when nothing usable was found.
async fn resolve_artwork(
    thumbnail: &str,
    title: &str,
    artists: &str,
    duration: Option<f64>,
) -> (Option<Artwork>, Option<String>) {
    if let Some(url) = trusted_thumbnail(thumbnail) {
        match fetch_image(&url).await {
            Ok(art) => return (Some(art), None),
            Err(error) => {
                tracing::debug!(%error, "download: provider thumbnail unusable, trying iTunes");
            }
        }
    }
    match itunes_artwork(title, artists, duration).await {
        Ok(Some(art)) => (Some(art), None),
        Ok(None) => (None, Some("No matching cover art was found for this track.".to_string())),
        Err(error) => {
            tracing::debug!(%error, "download: iTunes artwork lookup failed");
            (None, Some("Cover art could not be downloaded.".to_string()))
        }
    }
}

/// Lyrics via the core cache/provider chain under one overall timeout. Instrumental verdicts yield
/// no files and no warning (that is the expected answer, not a failure).
async fn resolve_lyrics(
    state: &AppState,
    job: &DownloadJob,
    duration: Option<f64>,
) -> (Option<Lyrics>, Option<String>) {
    let req = LyricsRequest {
        video_id: job.video_id.clone(),
        title: job.title.clone(),
        artists: job.artists.clone(),
        album: None,
        duration,
    };
    match tokio::time::timeout(LYRICS_TIMEOUT, get_lyrics(state, req)).await {
        Ok(Some(lyrics)) if lyrics.instrumental => (None, None),
        Ok(Some(lyrics)) => (Some(lyrics), None),
        Ok(None) => (None, Some("No lyrics were found for this track.".to_string())),
        Err(_) => (None, Some("Lyrics lookup timed out.".to_string())),
    }
}

/// A trusted, HTTPS provider thumbnail URL to fetch, or `None` when the URL is missing, insecure,
/// or points somewhere we won't fetch cover art from.
fn trusted_thumbnail(raw: &str) -> Option<String> {
    let raw = raw.trim();
    (!raw.is_empty() && trusted_image_host(raw, &TRUSTED_ART_SUFFIXES)).then(|| raw.to_string())
}

/// HTTPS URL whose host ends with one of `suffixes`. Rejects non-HTTPS, IP literals, localhost,
/// and suffix-spoofing hosts (`evil-scdn.co`) by requiring the dotted-suffix boundary.
fn trusted_image_host(raw: &str, suffixes: &[&str]) -> bool {
    let Ok(url) = url::Url::parse(raw) else { return false };
    if url.scheme() != "https" {
        return false;
    }
    match url.host_str() {
        Some(host) => {
            let host = host.to_ascii_lowercase();
            suffixes.iter().any(|suffix| host.len() > suffix.len() && host.ends_with(suffix))
        }
        None => false,
    }
}

/// iTunes Search fallback. Picks the first result on an exact normalized title+artist match within
/// the duration window (when the length is known), then fetches and validates its cover.
async fn itunes_artwork(
    title: &str,
    artists: &str,
    duration: Option<f64>,
) -> Result<Option<Artwork>, String> {
    let term = format!("{} {}", artists.trim(), title.trim());
    if term.trim().is_empty() {
        return Ok(None);
    }
    let mut resp = ryotunes_core::http::client()
        .get("https://itunes.apple.com/search")
        .query(&[
            ("media", "music"),
            ("entity", "song"),
            ("limit", "25"),
            ("country", "US"),
            ("term", term.as_str()),
        ])
        .timeout(API_TIMEOUT)
        .send()
        .await
        .map_err(|error| format!("iTunes search failed: {error}"))?;
    if !resp.status().is_success() {
        return Err(format!("iTunes search returned HTTP {}", resp.status().as_u16()));
    }
    let mut buf = Vec::new();
    while let Some(chunk) =
        resp.chunk().await.map_err(|error| format!("iTunes search failed: {error}"))?
    {
        if buf.len() + chunk.len() > MAX_JSON_BYTES {
            return Err("iTunes response was too large".into());
        }
        buf.extend_from_slice(&chunk[..]);
    }
    let parsed: ItunesResponse = serde_json::from_slice(&buf)
        .map_err(|error| format!("iTunes response was not valid JSON: {error}"))?;

    let Some(chosen) = choose_itunes(&parsed.results, title, artists, duration) else {
        return Ok(None);
    };
    let Some(raw_art) = chosen.artwork_url100.as_deref() else {
        return Ok(None);
    };
    let art_url = upscale_itunes(raw_art);
    if !trusted_image_host(&art_url, &[MZSTATIC_SUFFIX]) {
        return Ok(None);
    }
    fetch_image(&art_url).await.map(Some)
}

/// The one search hit that confidently identifies our track: exact normalized title, exact
/// normalized artist (whole credit or its primary artist), and — when we know our length — a
/// length within tolerance. A candidate with no length loses when ours is known, rather than being
/// accepted on title+artist alone.
fn choose_itunes<'a>(
    results: &'a [ItunesResult],
    title: &str,
    artists: &str,
    duration: Option<f64>,
) -> Option<&'a ItunesResult> {
    let want_title = normalize(title);
    let want_artist_full = normalize(artists);
    let want_artist_primary = normalize(&primary_artist(artists));
    results.iter().find(|candidate| {
        let (Some(track), Some(artist)) =
            (candidate.track_name.as_deref(), candidate.artist_name.as_deref())
        else {
            return false;
        };
        if normalize(track) != want_title {
            return false;
        }
        let their_artist = normalize(artist);
        if their_artist != want_artist_full && their_artist != want_artist_primary {
            return false;
        }
        match duration {
            Some(ours) => match candidate.track_time_millis {
                Some(ms) => (ms as f64 / 1000.0 - ours).abs() <= MATCH_TOLERANCE_SECS,
                None => false,
            },
            None => true,
        }
    })
}

/// Ask iTunes for a larger render of the standard 100px artwork URL. Untouched if the size token
/// isn't there.
fn upscale_itunes(url: &str) -> String {
    url.replace("100x100bb", "600x600bb")
}

/// Casefolded alphanumerics only — drops spacing and punctuation so "Song (Remix)" and "Song"
/// stay distinct while "The Weeknd" matches regardless of stray punctuation.
fn normalize(s: &str) -> String {
    s.chars().filter(|c| c.is_alphanumeric()).flat_map(|c| c.to_lowercase()).collect()
}

/// The first credited artist, before any secondary/feature separator. Used so a multi-artist
/// credit still matches iTunes' primary `artistName`.
fn primary_artist(artists: &str) -> String {
    let trimmed = artists.trim();
    let lower = trimmed.to_ascii_lowercase();
    let mut end = trimmed.len();
    for sep in [",", ";", " & ", " feat.", " feat ", " ft.", " ft ", " x "] {
        if let Some(index) = lower.find(sep) {
            end = end.min(index);
        }
    }
    trimmed[..end].trim().to_string()
}

/// Download `url`, bounded by size and time, and confirm it is a JPEG or PNG by signature.
async fn fetch_image(url: &str) -> Result<Artwork, String> {
    let bytes = fetch_bounded(url, MAX_IMAGE_BYTES, IMAGE_TIMEOUT).await?;
    let mime =
        detect_image(&bytes).ok_or("the downloaded cover art was not a JPEG or PNG image")?;
    let ext = match mime {
        MimeType::Png => "png",
        _ => "jpg",
    };
    Ok(Artwork { bytes, mime, ext })
}

/// A GET whose body is capped at `max` bytes (via `Content-Length` and while streaming) and whose
/// total time is capped by `timeout`.
async fn fetch_bounded(url: &str, max: usize, timeout: Duration) -> Result<Vec<u8>, String> {
    let mut resp = ryotunes_core::http::client()
        .get(url)
        .timeout(timeout)
        .send()
        .await
        .map_err(|error| format!("request failed: {error}"))?;
    if !resp.status().is_success() {
        return Err(format!("host returned HTTP {}", resp.status().as_u16()));
    }
    if resp.content_length().is_some_and(|len| len > max as u64) {
        return Err("response was too large".into());
    }
    let mut buf = Vec::new();
    while let Some(chunk) =
        resp.chunk().await.map_err(|error| format!("download failed: {error}"))?
    {
        if buf.len() + chunk.len() > max {
            return Err("response was too large".into());
        }
        buf.extend_from_slice(&chunk[..]);
    }
    Ok(buf)
}

/// JPEG/PNG detection by magic bytes. Deliberately narrow: only these two formats are embedded and
/// written as sidecars.
fn detect_image(bytes: &[u8]) -> Option<MimeType> {
    if bytes.starts_with(&[0xFF, 0xD8, 0xFF]) {
        Some(MimeType::Jpeg)
    } else if bytes.starts_with(&[0x89, b'P', b'N', b'G', 0x0D, 0x0A, 0x1A, 0x0A]) {
        Some(MimeType::Png)
    } else {
        None
    }
}

/// Synced lyrics rendered as LRC (`[mm:ss.xx]text`). Lines without a timestamp are skipped; `None`
/// when the lyrics aren't synced or nothing timed survived.
fn to_lrc(lyrics: &Lyrics) -> Option<String> {
    if !lyrics.synced {
        return None;
    }
    let mut out = String::new();
    for line in &lyrics.lines {
        let Some(ms) = line.time_ms else { continue };
        let centis_total = ms / 10;
        let centis = centis_total % 100;
        let secs_total = centis_total / 100;
        let secs = secs_total % 60;
        let mins = secs_total / 60;
        out.push_str(&format!("[{mins:02}:{secs:02}.{centis:02}]{}\n", line.text));
    }
    (!out.is_empty()).then_some(out)
}

/// The lyrics as plain text, one line per cue, for the audio tag and the plain sidecar.
fn plain_text(lyrics: &Lyrics) -> String {
    let mut out = String::new();
    for (index, line) in lyrics.lines.iter().enumerate() {
        if index > 0 {
            out.push('\n');
        }
        out.push_str(&line.text);
    }
    out
}

/// Write the sidecars and embed the tags. Returns the sidecar paths that were actually written and
/// any warnings; a failed step degrades to a warning without failing the download.
fn write_media(
    audio: &Path,
    title: &str,
    artist: &str,
    artwork: Option<Artwork>,
    lyric_tag: Option<String>,
    lyric_sidecar: Option<(PathBuf, String)>,
) -> (Vec<PathBuf>, Vec<String>) {
    let mut sidecars = Vec::new();
    let mut warnings = Vec::new();

    if let Some(art) = &artwork {
        let path = audio.with_extension(art.ext);
        match std::fs::write(&path, &art.bytes) {
            Ok(()) => sidecars.push(path),
            Err(error) => warnings.push(format!("Could not save the cover art file: {error}")),
        }
    }
    if let Some((path, content)) = &lyric_sidecar {
        match std::fs::write(path, content) {
            Ok(()) => sidecars.push(path.clone()),
            Err(error) => warnings.push(format!("Could not save the lyrics file: {error}")),
        }
    }
    if let Err(error) = embed_tags(audio, title, artist, artwork, lyric_tag.as_deref()) {
        warnings.push(format!("Could not embed metadata into the audio file: {error}"));
    }
    (sidecars, warnings)
}

/// Embed title/artist (+ artwork/lyrics when present) on an isolated copy, replacing the staged
/// audio only if the whole write succeeds. A failure leaves the original file untouched.
fn embed_tags(
    audio: &Path,
    title: &str,
    artist: &str,
    artwork: Option<Artwork>,
    lyrics: Option<&str>,
) -> Result<(), String> {
    let ext = audio
        .extension()
        .and_then(|value| value.to_str())
        .ok_or("the audio file has no extension")?;
    let temp = audio.with_extension(format!("rytag.{ext}"));
    std::fs::copy(audio, &temp).map_err(|error| format!("could not stage a copy: {error}"))?;

    match embed_into(&temp, title, artist, artwork, lyrics) {
        Ok(()) => std::fs::rename(&temp, audio)
            .map_err(|error| format!("could not replace the staged audio: {error}")),
        Err(error) => {
            let _ = std::fs::remove_file(&temp);
            Err(error)
        }
    }
}

/// Apply the tags in place on `path`. Reuses the file's existing tag, adds the format's primary
/// tag when there is none, and writes without re-encoding.
fn embed_into(
    path: &Path,
    title: &str,
    artist: &str,
    artwork: Option<Artwork>,
    lyrics: Option<&str>,
) -> Result<(), String> {
    let mut tagged =
        lofty::read_from_path(path).map_err(|error| format!("could not read tags: {error}"))?;
    let primary = tagged.primary_tag_type();
    let write_type = if tagged.contains_tag_type(primary) {
        primary
    } else if let Some(first) = tagged.first_tag() {
        first.tag_type()
    } else {
        tagged.insert_tag(Tag::new(primary));
        primary
    };
    let tag = tagged.tag_mut(write_type).ok_or("no writable tag on the audio file")?;

    tag.set_title(title.to_string());
    tag.set_artist(artist.to_string());
    if let Some(art) = artwork {
        tag.remove_picture_type(PictureType::CoverFront);
        tag.push_picture(Picture::new_unchecked(
            PictureType::CoverFront,
            Some(art.mime),
            None,
            art.bytes,
        ));
    }
    if let Some(text) = lyrics {
        tag.insert_text(ItemKey::Lyrics, text.to_string());
    }

    tagged
        .save_to_path(path, WriteOptions::default())
        .map_err(|error| format!("could not write tags: {error}"))
}

#[derive(Deserialize)]
struct ItunesResponse {
    #[serde(default)]
    results: Vec<ItunesResult>,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct ItunesResult {
    #[serde(default)]
    track_name: Option<String>,
    #[serde(default)]
    artist_name: Option<String>,
    #[serde(default)]
    track_time_millis: Option<i64>,
    #[serde(default)]
    artwork_url100: Option<String>,
}

#[cfg(test)]
mod tests {
    use super::*;
    use ryotunes_core::lyrics::LyricLine;

    #[test]
    fn staged_tag_write_preserves_a_recognizable_audio_format() {
        struct Temp(PathBuf);
        impl Drop for Temp {
            fn drop(&mut self) {
                let _ = std::fs::remove_dir_all(&self.0);
            }
        }
        let nonce =
            std::time::SystemTime::now().duration_since(std::time::UNIX_EPOCH).unwrap().as_nanos();
        let dir = Temp(
            std::env::temp_dir().join(format!("ryotunes-tags-{}-{nonce}", std::process::id())),
        );
        std::fs::create_dir(&dir.0).unwrap();
        let audio = dir.0.join("audio.wav");
        // One PCM sample: a real, minimal RIFF/WAVE file rather than a mocked tag writer.
        let mut wav = b"RIFF".to_vec();
        wav.extend(38u32.to_le_bytes());
        wav.extend(b"WAVEfmt ");
        wav.extend(16u32.to_le_bytes());
        wav.extend(1u16.to_le_bytes());
        wav.extend(1u16.to_le_bytes());
        wav.extend(8000u32.to_le_bytes());
        wav.extend(16000u32.to_le_bytes());
        wav.extend(2u16.to_le_bytes());
        wav.extend(16u16.to_le_bytes());
        wav.extend(b"data");
        wav.extend(2u32.to_le_bytes());
        wav.extend(0i16.to_le_bytes());
        std::fs::write(&audio, wav).unwrap();
        embed_tags(&audio, "Saved title", "Saved artist", None, None).unwrap();
        let tagged = lofty::read_from_path(&audio).unwrap();
        assert!(tagged.tags().iter().any(|tag| tag.title().as_deref() == Some("Saved title")));
        assert!(tagged.tags().iter().any(|tag| tag.artist().as_deref() == Some("Saved artist")));
    }

    fn result(track: &str, artist: &str, ms: Option<i64>, art: Option<&str>) -> ItunesResult {
        ItunesResult {
            track_name: Some(track.to_string()),
            artist_name: Some(artist.to_string()),
            track_time_millis: ms,
            artwork_url100: art.map(str::to_string),
        }
    }

    #[test]
    fn itunes_pick_needs_exact_title_and_rejects_the_remix_even_within_window() {
        let results = vec![
            result("Blinding Lights (Remix)", "The Weeknd", Some(200_000), Some("remix")),
            result("Blinding Lights", "The Weeknd", Some(200_040), Some("real")),
        ];
        // The remix is inside the duration window too; only the exact title separates it.
        let chosen = choose_itunes(&results, "Blinding Lights", "The Weeknd", Some(200.0)).unwrap();
        assert_eq!(chosen.artwork_url100.as_deref(), Some("real"));
    }

    #[test]
    fn itunes_pick_enforces_the_duration_window() {
        let results = vec![result("Song", "Artist", Some(180_000), Some("x"))];
        assert!(choose_itunes(&results, "Song", "Artist", Some(200.0)).is_none());
        assert!(choose_itunes(&results, "Song", "Artist", Some(182.0)).is_some());
    }

    #[test]
    fn itunes_pick_matches_primary_of_a_multi_artist_credit() {
        let results = vec![result("Save Your Tears", "The Weeknd", Some(215_000), Some("y"))];
        assert!(choose_itunes(
            &results,
            "Save Your Tears",
            "The Weeknd, Ariana Grande",
            Some(215.0)
        )
        .is_some());
    }

    #[test]
    fn itunes_pick_wants_duration_when_ours_is_known_but_falls_back_to_title_artist_when_not() {
        let results = vec![result("Song", "Artist", None, Some("z"))];
        // We know our length, the candidate hides its own: refuse rather than guess.
        assert!(choose_itunes(&results, "Song", "Artist", Some(200.0)).is_none());
        // Our length is unknown: an exact title+artist is all there is to go on.
        assert!(choose_itunes(&results, "Song", "Artist", None).is_some());
    }

    #[test]
    fn itunes_pick_rejects_a_wrong_artist() {
        let results = vec![result("Song", "Somebody Else", Some(200_000), Some("no"))];
        assert!(choose_itunes(&results, "Song", "Artist", Some(200.0)).is_none());
    }

    #[test]
    fn lrc_encodes_timestamps_and_skips_untimed_lines() {
        let lyrics = Lyrics {
            source: "LRCLIB".into(),
            synced: true,
            instrumental: false,
            lines: vec![
                LyricLine::simple(Some(0), "first".into()),
                LyricLine::simple(Some(61_230), "second".into()),
                LyricLine::simple(None, "no cue".into()),
            ],
        };
        assert_eq!(to_lrc(&lyrics).unwrap(), "[00:00.00]first\n[01:01.23]second\n");
    }

    #[test]
    fn lrc_is_none_for_unsynced_lyrics() {
        let lyrics = Lyrics {
            source: "x".into(),
            synced: false,
            instrumental: false,
            lines: vec![LyricLine::simple(None, "plain".into())],
        };
        assert!(to_lrc(&lyrics).is_none());
    }

    #[test]
    fn plain_text_joins_lines_with_newlines() {
        let lyrics = Lyrics {
            source: "x".into(),
            synced: false,
            instrumental: false,
            lines: vec![
                LyricLine::simple(None, "one".into()),
                LyricLine::simple(None, "two".into()),
            ],
        };
        assert_eq!(plain_text(&lyrics), "one\ntwo");
    }

    #[test]
    fn image_detection_accepts_only_jpeg_and_png() {
        assert!(matches!(detect_image(&[0xFF, 0xD8, 0xFF, 0xE0, 0, 0]), Some(MimeType::Jpeg)));
        assert!(matches!(
            detect_image(&[0x89, b'P', b'N', b'G', 0x0D, 0x0A, 0x1A, 0x0A, 0]),
            Some(MimeType::Png)
        ));
        assert!(detect_image(b"GIF89a1234").is_none());
        assert!(detect_image(b"<!doctype html>").is_none());
        assert!(detect_image(&[]).is_none());
    }

    #[test]
    fn thumbnail_trust_accepts_provider_cdns_over_https() {
        assert!(trusted_thumbnail("https://i.ytimg.com/vi/abc/maxresdefault.jpg").is_some());
        assert!(trusted_thumbnail("https://lh3.googleusercontent.com/x=w512").is_some());
        assert!(trusted_thumbnail("https://yt3.ggpht.com/y=s512").is_some());
        assert!(trusted_thumbnail("https://i1.sndcdn.com/artworks-x-t500x500.jpg").is_some());
        assert!(trusted_thumbnail("https://i.scdn.co/image/abc").is_some());
    }

    #[test]
    fn thumbnail_trust_rejects_insecure_local_and_spoofed_hosts() {
        assert!(trusted_thumbnail("http://i.ytimg.com/vi/abc.jpg").is_none());
        assert!(trusted_thumbnail("https://127.0.0.1/cover.jpg").is_none());
        assert!(trusted_thumbnail("https://localhost/cover.jpg").is_none());
        assert!(trusted_thumbnail("https://evil.example.com/cover.jpg").is_none());
        assert!(trusted_thumbnail("https://evil-scdn.co/cover.jpg").is_none());
        assert!(trusted_thumbnail("").is_none());
    }

    #[test]
    fn itunes_art_host_must_be_mzstatic() {
        assert!(trusted_image_host(
            "https://is1-ssl.mzstatic.com/image/thumb/x/600x600bb.jpg",
            &[MZSTATIC_SUFFIX]
        ));
        assert!(!trusted_image_host("https://evil.com/600x600bb.jpg", &[MZSTATIC_SUFFIX]));
    }

    #[test]
    fn itunes_artwork_url_is_upscaled() {
        assert_eq!(
            upscale_itunes("https://is1-ssl.mzstatic.com/image/thumb/x/100x100bb.jpg"),
            "https://is1-ssl.mzstatic.com/image/thumb/x/600x600bb.jpg"
        );
    }
}
