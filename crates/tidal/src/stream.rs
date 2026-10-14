//! Turning a playback-info response into something a player can open, and refusing the cases that
//! would need a content key.
//!
//! TIDAL answers `playbackinfopostpaywall` with a base64 `manifest` plus the mime type that says
//! how to read it. Two shapes reach a desktop client:
//!
//! * `application/vnd.tidal.bts` carries JSON with a `urls` list. LOW, HIGH and LOSSLESS arrive
//!   this way, and the first url is a progressive file a player opens unaided.
//! * `application/dash+xml` carries an MPEG-DASH presentation. HI_RES_LOSSLESS arrives this way.
//!
//! Either shape can come back protected: a BTS manifest names an `encryptionType` and a `keyId`,
//! and an MPD can carry `ContentProtection`. This module reads those markers and returns
//! `Error::Protected`. It does not hold, derive or apply a key, and nothing downstream should add
//! one: a protected asset is a refusal, not a slower path.

use base64::Engine as _;
use serde::Deserialize;

use crate::Error;

/// The four tiers TIDAL serves to a subscriber. Anything beyond them, the immersive formats above
/// all, is out of scope for this crate.
#[derive(
    Clone, Copy, Debug, PartialEq, Eq, PartialOrd, Ord, serde::Serialize, serde::Deserialize,
)]
#[serde(rename_all = "snake_case")]
pub enum Quality {
    Low,
    High,
    Lossless,
    HiResLossless,
}

impl Quality {
    /// The spelling the API expects in `audioquality`.
    pub fn as_str(self) -> &'static str {
        match self {
            Quality::Low => "LOW",
            Quality::High => "HIGH",
            Quality::Lossless => "LOSSLESS",
            Quality::HiResLossless => "HI_RES_LOSSLESS",
        }
    }

    pub fn parse(s: &str) -> Option<Self> {
        match s.trim().to_ascii_uppercase().as_str() {
            "LOW" => Some(Quality::Low),
            "HIGH" => Some(Quality::High),
            "LOSSLESS" => Some(Quality::Lossless),
            "HI_RES_LOSSLESS" | "HI_RES" => Some(Quality::HiResLossless),
            _ => None,
        }
    }
}

/// What the player is given. A direct url is opened as is; a presentation has to be written out
/// first, because the manifest arrives inline rather than at a url of its own.
#[derive(Clone, Debug, PartialEq, Eq)]
pub enum Source {
    Direct(String),
    Presentation { xml: String },
}

/// One resolved track, with whatever the manifest was willing to say about it.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct Stream {
    pub source: Source,
    /// What the server actually served, which can be below what was asked for.
    pub quality: Quality,
    pub codec: Option<String>,
    pub sample_rate: Option<u32>,
    pub bit_depth: Option<u32>,
}

/// Wire shape of the playback-info response, trimmed to the fields that decide anything.
#[derive(Debug, Deserialize)]
pub(crate) struct PlaybackInfo {
    #[serde(rename = "manifestMimeType")]
    pub manifest_mime_type: String,
    pub manifest: String,
    #[serde(rename = "audioQuality")]
    pub audio_quality: Option<String>,
    #[serde(rename = "assetPresentation")]
    pub asset_presentation: Option<String>,
    #[serde(rename = "sampleRate")]
    pub sample_rate: Option<u32>,
    #[serde(rename = "bitDepth")]
    pub bit_depth: Option<u32>,
}

/// The JSON inside a `vnd.tidal.bts` manifest.
#[derive(Debug, Deserialize)]
struct BtsManifest {
    #[serde(default)]
    urls: Vec<String>,
    #[serde(default)]
    codecs: Option<String>,
    #[serde(rename = "encryptionType", default)]
    encryption_type: Option<String>,
    #[serde(rename = "keyId", default)]
    key_id: Option<String>,
}

pub(crate) const MIME_BTS: &str = "application/vnd.tidal.bts";
pub(crate) const MIME_DASH: &str = "application/dash+xml";

/// Read a playback-info response into a playable source, or refuse it.
pub(crate) fn resolve(info: &PlaybackInfo) -> Result<Stream, Error> {
    if info.asset_presentation.as_deref().is_some_and(|a| !a.eq_ignore_ascii_case("FULL")) {
        return Err(Error::Preview(info.asset_presentation.clone().unwrap_or_default()));
    }
    let raw = base64::engine::general_purpose::STANDARD
        .decode(info.manifest.as_bytes())
        .map_err(|_| Error::Manifest("manifest is not base64".into()))?;
    let quality = info
        .audio_quality
        .as_deref()
        .and_then(Quality::parse)
        .ok_or_else(|| Error::Manifest("response named no audio quality".into()))?;

    match info.manifest_mime_type.as_str() {
        MIME_BTS => {
            let text = String::from_utf8(raw)
                .map_err(|_| Error::Manifest("direct manifest is not utf-8".into()))?;
            let bts: BtsManifest = serde_json::from_str(&text)?;
            guard_bts(&bts)?;
            let url = bts
                .urls
                .into_iter()
                .find(|u| !u.trim().is_empty())
                .ok_or_else(|| Error::Manifest("direct manifest carried no url".into()))?;
            Ok(Stream {
                source: Source::Direct(url),
                quality,
                codec: bts.codecs,
                sample_rate: info.sample_rate,
                bit_depth: info.bit_depth,
            })
        }
        MIME_DASH => {
            let xml = String::from_utf8(raw)
                .map_err(|_| Error::Manifest("presentation is not utf-8".into()))?;
            guard_dash(&xml)?;
            Ok(Stream {
                codec: attribute(&xml, "codecs"),
                sample_rate: info
                    .sample_rate
                    .or_else(|| attribute(&xml, "audioSamplingRate").and_then(|v| v.parse().ok())),
                bit_depth: info.bit_depth,
                source: Source::Presentation { xml },
                quality,
            })
        }
        other => Err(Error::Manifest(format!("unknown manifest type {other}"))),
    }
}

/// A direct manifest is protected when it names an encryption type other than NONE, or when it
/// carries a key id at all.
fn guard_bts(bts: &BtsManifest) -> Result<(), Error> {
    let kind = bts.encryption_type.as_deref().unwrap_or("NONE");
    let encrypted = !kind.eq_ignore_ascii_case("NONE") && !kind.trim().is_empty();
    let keyed = bts.key_id.as_deref().is_some_and(|k| !k.trim().is_empty());
    if encrypted || keyed {
        return Err(Error::Protected(kind.to_string()));
    }
    Ok(())
}

/// A presentation is protected when it declares any `ContentProtection`. The marker is checked
/// rather than parsed: the presence of the element is the whole answer, and reading further would
/// only serve an attempt to satisfy it.
fn guard_dash(xml: &str) -> Result<(), Error> {
    if xml.contains("ContentProtection") {
        return Err(Error::Protected("dash content protection".into()));
    }
    Ok(())
}

/// Pull the first value of an attribute out of an MPD without taking on an XML parser. Only used
/// for the two cosmetic fields, so a miss costs a label and nothing else.
fn attribute(xml: &str, name: &str) -> Option<String> {
    let needle = format!("{name}=\"");
    let start = xml.find(&needle)? + needle.len();
    let rest = &xml[start..];
    let end = rest.find('"')?;
    let value = rest[..end].trim();
    (!value.is_empty()).then(|| value.to_string())
}

#[cfg(test)]
mod tests {
    use super::*;

    fn b64(s: &str) -> String {
        base64::engine::general_purpose::STANDARD.encode(s.as_bytes())
    }

    fn info(mime: &str, manifest: &str, quality: &str) -> PlaybackInfo {
        PlaybackInfo {
            manifest_mime_type: mime.into(),
            manifest: b64(manifest),
            audio_quality: Some(quality.into()),
            asset_presentation: Some("FULL".into()),
            sample_rate: None,
            bit_depth: None,
        }
    }

    #[test]
    fn a_plain_direct_manifest_yields_its_first_url() {
        let body = r#"{"mimeType":"audio/flac","codecs":"flac","encryptionType":"NONE",
            "urls":["https://example.invalid/a.flac"]}"#;
        let s = resolve(&info(MIME_BTS, body, "LOSSLESS")).expect("plain manifests must resolve");
        assert_eq!(s.source, Source::Direct("https://example.invalid/a.flac".into()));
        assert_eq!(s.quality, Quality::Lossless);
        assert_eq!(s.codec.as_deref(), Some("flac"));
    }

    #[test]
    fn an_encrypted_direct_manifest_is_refused() {
        let body = r#"{"encryptionType":"OLD_AES","keyId":"abc",
            "urls":["https://example.invalid/a.flac"]}"#;
        match resolve(&info(MIME_BTS, body, "LOSSLESS")) {
            Err(Error::Protected(kind)) => assert_eq!(kind, "OLD_AES"),
            other => panic!("a keyed manifest must be refused, got {other:?}"),
        }
    }

    #[test]
    fn a_key_id_alone_is_enough_to_refuse() {
        let body =
            r#"{"encryptionType":"NONE","keyId":"abc","urls":["https://example.invalid/a"]}"#;
        assert!(
            matches!(resolve(&info(MIME_BTS, body, "HIGH")), Err(Error::Protected(_))),
            "a key id without an encryption type still means a key is expected"
        );
    }

    #[test]
    fn a_plain_presentation_is_handed_over_whole() {
        let xml = r#"<MPD><Representation codecs="flac" audioSamplingRate="96000"/></MPD>"#;
        let s = resolve(&info(MIME_DASH, xml, "HI_RES_LOSSLESS")).expect("plain mpd must resolve");
        assert!(matches!(s.source, Source::Presentation { .. }));
        assert_eq!(s.quality, Quality::HiResLossless);
        assert_eq!(s.sample_rate, Some(96_000));
        assert_eq!(s.codec.as_deref(), Some("flac"));
    }

    #[test]
    fn a_protected_presentation_is_refused() {
        let xml =
            r#"<MPD><ContentProtection schemeIdUri="urn:mpeg:dash:mp4protection:2011"/></MPD>"#;
        assert!(
            matches!(resolve(&info(MIME_DASH, xml, "HI_RES_LOSSLESS")), Err(Error::Protected(_))),
            "a presentation declaring protection must be refused"
        );
    }

    #[test]
    fn a_preview_asset_is_reported_as_such() {
        let body = r#"{"urls":["https://example.invalid/a"]}"#;
        let mut i = info(MIME_BTS, body, "LOSSLESS");
        i.asset_presentation = Some("PREVIEW".into());
        match resolve(&i) {
            Err(Error::Preview(p)) => assert_eq!(p, "PREVIEW"),
            other => panic!("a preview must not be mistaken for a stream, got {other:?}"),
        }
    }

    #[test]
    fn an_empty_url_list_is_an_error_not_a_panic() {
        let body = r#"{"encryptionType":"NONE","urls":[]}"#;
        assert!(matches!(resolve(&info(MIME_BTS, body, "HIGH")), Err(Error::Manifest(_))));
    }

    #[test]
    fn quality_round_trips_through_its_wire_spelling() {
        for q in [Quality::Low, Quality::High, Quality::Lossless, Quality::HiResLossless] {
            assert_eq!(Quality::parse(q.as_str()), Some(q));
        }
        assert_eq!(Quality::parse("nonsense"), None);
    }
}
