//! Spotify color-lyrics. Lifted from nolight132/sonora `spotify/lyrics.rs`.
//!
//! Change from Sonora: the synced lines are normalized locally (sort by start, close open ends
//! from the following line) instead of running Sonora's 970-line LRC pipeline — Spotify's payload
//! is already ordered, one clean line per verse with syllable timing, so the heavy pass (which also
//! pulled in kakasi romanization) buys nothing here.

use anyhow::{Context as _, Result};
use bytes::Bytes;
use http::{Method, Request, header};
use librespot_core::{Session, spclient::CLIENT_TOKEN};
use serde::Deserialize;

use crate::{Lyrics, LyricsLine, LyricsWord, Voice};

const ENDPOINT: &str = "https://spclient.wg.spotify.com/color-lyrics/v2/track";
const APP_PLATFORM: &str = "WebPlayer";

#[derive(Deserialize)]
struct Answer {
    lyrics: Option<Sheet>,
}

#[derive(Deserialize)]
struct Sheet {
    #[serde(default, rename = "syncType")]
    sync: String,
    #[serde(default)]
    lines: Vec<Verse>,
}

#[derive(Deserialize)]
struct Verse {
    #[serde(default, rename = "startTimeMs")]
    start: String,
    #[serde(default, rename = "endTimeMs")]
    end: String,
    #[serde(default)]
    words: String,
    #[serde(default)]
    syllables: Vec<Syllable>,
}

#[derive(Deserialize)]
struct Syllable {
    #[serde(default, rename = "startTimeMs")]
    start: String,
    #[serde(default, rename = "endTimeMs")]
    end: String,
    #[serde(default)]
    text: String,
}

pub async fn lyrics(session: &Session, track_id: &str) -> Result<Option<Lyrics>> {
    let token =
        session.login5().auth_token().await.context("cannot obtain Spotify access token")?;
    let client_token =
        session.spclient().client_token().await.context("cannot obtain Spotify client token")?;
    let request = Request::builder()
        .method(Method::GET)
        .uri(format!("{ENDPOINT}/{track_id}?format=json&vocalRemoval=false&market=from_token"))
        .header(header::ACCEPT, "application/json")
        .header("app-platform", APP_PLATFORM)
        .header(header::AUTHORIZATION, format!("{} {}", token.token_type, token.access_token))
        .header(CLIENT_TOKEN, client_token)
        .body(Bytes::new())
        .context("cannot build the Spotify lyrics request")?;
    let body = match session.http_client().request_body(request).await {
        Ok(body) => body,
        Err(error) => {
            log::debug!("lyrics: spotify has none for {track_id}: {error}");
            return Ok(None);
        }
    };
    let answer: Answer =
        serde_json::from_slice(&body).context("cannot decode the Spotify lyrics response")?;
    Ok(answer.lyrics.and_then(sheet))
}

fn sheet(sheet: Sheet) -> Option<Lyrics> {
    if sheet.sync == "UNSYNCED" {
        let text = sheet
            .lines
            .iter()
            .map(|verse| verse.words.trim())
            .filter(|words| !words.is_empty())
            .collect::<Vec<_>>()
            .join("\n");
        return (!text.trim().is_empty()).then(|| Lyrics::plain(text));
    }

    let mut lines: Vec<LyricsLine> = sheet.lines.iter().filter_map(verse).collect();
    normalize(&mut lines);
    (!lines.is_empty()).then(|| Lyrics::Synced { lines: lines.into() })
}

/// Order the lines and, where a verse gave no end cue, close it at the next verse's start. Spotify
/// verses are already clean and mostly carry an `endTimeMs`, so this is all the shaping they need.
fn normalize(lines: &mut [LyricsLine]) {
    lines.sort_by_key(|line| line.start);
    let starts: Vec<_> = lines.iter().map(|line| line.start).collect();
    for (index, line) in lines.iter_mut().enumerate() {
        if line.end.is_none()
            && let Some(&next) = starts.get(index + 1)
            && next > line.start
        {
            line.end = Some(next);
        }
    }
}

fn verse(verse: &Verse) -> Option<LyricsLine> {
    let text = verse.words.trim();
    if text.is_empty() || text == "♪" {
        return None;
    }
    let start = stamp(&verse.start)?;
    let words: Vec<LyricsWord> = verse
        .syllables
        .iter()
        .filter_map(|syllable| {
            Some(LyricsWord {
                start: stamp(&syllable.start)?,
                end: stamp(&syllable.end)?,
                text: syllable.text.clone(),
            })
        })
        .collect();
    Some(LyricsLine {
        start,
        end: stamp(&verse.end).filter(|end| *end > start),
        text: text.to_owned(),
        romanized: None,
        words: (!words.is_empty()).then_some(words),
        secondary: Vec::new(),
        voice: Voice::Lead,
    })
}

fn stamp(millis: &str) -> Option<std::time::Duration> {
    millis.trim().parse().ok().map(std::time::Duration::from_millis)
}

#[cfg(test)]
mod tests {
    use super::*;

    fn verse_of(start: u64, end: Option<u64>) -> LyricsLine {
        LyricsLine {
            start: std::time::Duration::from_millis(start),
            end: end.map(std::time::Duration::from_millis),
            text: format!("line {start}"),
            romanized: None,
            words: None,
            secondary: Vec::new(),
            voice: Voice::Lead,
        }
    }

    #[test]
    fn normalize_orders_and_closes_open_ends() {
        let mut lines = vec![verse_of(2000, None), verse_of(0, None), verse_of(4000, None)];
        normalize(&mut lines);

        let stamps: Vec<_> = lines.iter().map(|line| line.start.as_millis()).collect();
        assert_eq!(stamps, [0, 2000, 4000]);
        // Each open line is closed at the next line's start; the last one stays open.
        assert_eq!(lines[0].end, Some(std::time::Duration::from_millis(2000)));
        assert_eq!(lines[1].end, Some(std::time::Duration::from_millis(4000)));
        assert_eq!(lines[2].end, None);
    }

    #[test]
    fn normalize_keeps_a_real_end_cue() {
        let mut lines = vec![verse_of(0, Some(1500)), verse_of(2000, None)];
        normalize(&mut lines);
        assert_eq!(lines[0].end, Some(std::time::Duration::from_millis(1500)));
    }

    #[test]
    fn parses_a_synced_sheet_with_syllables() {
        let raw = serde_json::json!({
            "lyrics": {
                "syncType": "LINE_SYNCED",
                "lines": [
                    { "startTimeMs": "1000", "endTimeMs": "2000", "words": "hello world",
                      "syllables": [
                          { "startTimeMs": "1000", "endTimeMs": "1400", "text": "hello" },
                          { "startTimeMs": "1400", "endTimeMs": "2000", "text": "world" }
                      ] },
                    { "startTimeMs": "0", "endTimeMs": "1000", "words": "♪", "syllables": [] }
                ]
            }
        });
        let answer: Answer = serde_json::from_value(raw).unwrap();
        let Some(Lyrics::Synced { lines }) = answer.lyrics.and_then(sheet) else {
            panic!("expected synced lyrics");
        };
        // The "♪" filler line is dropped; the sung line survives with its two syllables.
        assert_eq!(lines.len(), 1);
        assert_eq!(lines[0].text, "hello world");
        assert!(lines[0].worded());
    }

    #[test]
    fn unsynced_becomes_plain_text() {
        let raw = serde_json::json!({
            "lyrics": {
                "syncType": "UNSYNCED",
                "lines": [
                    { "words": "first" },
                    { "words": "" },
                    { "words": "second" }
                ]
            }
        });
        let answer: Answer = serde_json::from_value(raw).unwrap();
        let Some(Lyrics::Plain { text, .. }) = answer.lyrics.and_then(sheet) else {
            panic!("expected plain lyrics");
        };
        assert_eq!(text, "first\nsecond");
    }
}
