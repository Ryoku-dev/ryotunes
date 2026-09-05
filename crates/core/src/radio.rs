//! Internet-radio directory integration for v2.4.
//!
//! Radio Browser is queried only when the Radio surface is opened/searched. There is no polling
//! task, no startup fetch, and no renderer-side CORS dependency. Playback itself remains native:
//! the selected station's resolved HTTP(S) stream is handed straight to libmpv.

use std::collections::{HashSet, VecDeque};
use std::sync::{Mutex, OnceLock};
use std::time::Duration;

use innertube::SongItem;
use rand::seq::SliceRandom;
use serde::{Deserialize, Serialize};

use crate::orchestrator::PlaybackData;

pub const RADIO_ID_PREFIX: &str = "RYOTUNES_RADIO:";
const DISCOVERY_URL: &str = "https://all.api.radio-browser.info/json/servers";
const FALLBACK_SERVERS: [&str; 3] = [
    "https://de1.api.radio-browser.info",
    "https://nl1.api.radio-browser.info",
    "https://at1.api.radio-browser.info",
];
const USER_AGENT: &str = "Ryotunes/2.4 (+https://github.com/ashmitvoid/ryotunes)";
const STATION_CACHE_MAX: usize = 256;
const MAX_QUERY_CHARS: usize = 128;
const MAX_RADIO_RESPONSE_BYTES: usize = 2 * 1024 * 1024;
static STATION_CACHE: OnceLock<Mutex<VecDeque<RadioStation>>> = OnceLock::new();

fn station_cache() -> &'static Mutex<VecDeque<RadioStation>> {
    STATION_CACHE.get_or_init(|| Mutex::new(VecDeque::new()))
}

fn remember_stations(stations: &[RadioStation]) {
    let Ok(mut cache) = station_cache().lock() else { return };
    for station in stations {
        cache.retain(|s| s.station_uuid != station.station_uuid);
        cache.push_back(station.clone());
        while cache.len() > STATION_CACHE_MAX {
            cache.pop_front();
        }
    }
}

fn cached_station(uuid: &str) -> Option<RadioStation> {
    station_cache().lock().ok()?.iter().rev().find(|station| station.station_uuid == uuid).cloned()
}

fn valid_station_uuid(uuid: &str) -> bool {
    let len = uuid.len();
    (1..=96).contains(&len)
        && uuid.chars().all(|c| c.is_ascii_alphanumeric() || matches!(c, '-' | '_'))
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct RadioStation {
    #[serde(alias = "stationuuid")]
    pub station_uuid: String,
    #[serde(default)]
    pub name: String,
    #[serde(alias = "url_resolved", default)]
    pub stream_url: String,
    #[serde(default)]
    pub url: String,
    #[serde(default)]
    pub homepage: String,
    #[serde(default)]
    pub favicon: String,
    #[serde(default)]
    pub country: String,
    #[serde(alias = "countrycode", default)]
    pub country_code: String,
    #[serde(default)]
    pub tags: String,
    #[serde(default)]
    pub codec: String,
    #[serde(default)]
    pub bitrate: i64,
    #[serde(default)]
    pub votes: i64,
    #[serde(default)]
    pub clickcount: i64,
}

#[derive(Debug, Deserialize)]
struct Mirror {
    name: String,
}

fn official_mirror_host(raw: &str) -> Option<String> {
    let host = raw.trim().trim_end_matches('.').to_ascii_lowercase();
    (host == "api.radio-browser.info" || host.ends_with(".api.radio-browser.info")).then_some(host)
}

async fn bounded_response_bytes(mut response: reqwest::Response) -> Result<Vec<u8>, String> {
    if response.content_length().is_some_and(|length| length > MAX_RADIO_RESPONSE_BYTES as u64) {
        return Err("Radio Browser response was too large.".into());
    }
    let mut bytes = Vec::new();
    while let Some(chunk) = response.chunk().await.map_err(|e| e.to_string())? {
        if bytes.len().saturating_add(chunk.len()) > MAX_RADIO_RESPONSE_BYTES {
            return Err("Radio Browser response was too large.".into());
        }
        bytes.extend_from_slice(&chunk);
    }
    Ok(bytes)
}

pub fn is_radio_id(id: &str) -> bool {
    id.starts_with(RADIO_ID_PREFIX)
}

fn client() -> Result<reqwest::Client, String> {
    reqwest::Client::builder()
        .user_agent(USER_AGENT)
        .timeout(Duration::from_secs(10))
        .build()
        .map_err(|e| format!("radio client: {e}"))
}

async fn mirrors(client: &reqwest::Client) -> Vec<String> {
    let mut out = match client.get(DISCOVERY_URL).send().await {
        Ok(response) if response.status().is_success() => {
            match bounded_response_bytes(response).await {
                Ok(bytes) => serde_json::from_slice::<Vec<Mirror>>(&bytes)
                    .unwrap_or_default()
                    .into_iter()
                    .filter_map(|mirror| official_mirror_host(&mirror.name))
                    .map(|host| format!("https://{host}"))
                    .collect::<Vec<_>>(),
                Err(_) => Vec::new(),
            }
        }
        _ => Vec::new(),
    };

    // Discovery itself can be unavailable. A short multi-server fallback is still materially more
    // robust than pinning the entire feature to one mirror, and the next request retries all of it.
    out.extend(FALLBACK_SERVERS.into_iter().map(str::to_owned));
    let mut seen = HashSet::new();
    out.retain(|url| seen.insert(url.clone()));
    out.shuffle(&mut rand::thread_rng());
    out
}

async fn get_json<T: serde::de::DeserializeOwned>(path: &str) -> Result<T, String> {
    let client = client()?;
    let mut errors = Vec::new();
    for base in mirrors(&client).await {
        let url = format!("{base}{path}");
        match client.get(&url).send().await {
            Ok(response) if response.status().is_success() => {
                match bounded_response_bytes(response).await {
                    Ok(bytes) => match serde_json::from_slice::<T>(&bytes) {
                        Ok(value) => return Ok(value),
                        Err(e) => errors.push(format!("{base}: invalid response ({e})")),
                    },
                    Err(e) => errors.push(format!("{base}: response body ({e})")),
                }
            }
            Ok(response) => errors.push(format!("{base}: HTTP {}", response.status())),
            Err(e) => errors.push(format!("{base}: {e}")),
        }
    }
    Err(if errors.is_empty() {
        "Radio Browser is unavailable right now".into()
    } else {
        format!("Radio Browser is unavailable right now ({})", errors.join("; "))
    })
}

fn public_ipv4(ip: std::net::Ipv4Addr) -> bool {
    let [a, b, c, _] = ip.octets();
    !(a == 0
        || a == 10
        || a == 127
        || (a == 100 && (64..=127).contains(&b))
        || (a == 169 && b == 254)
        || (a == 172 && (16..=31).contains(&b))
        || (a == 192 && b == 0 && c == 0)
        || (a == 192 && b == 0 && c == 2)
        || (a == 192 && b == 168)
        || (a == 198 && (18..=19).contains(&b))
        || (a == 198 && b == 51 && c == 100)
        || (a == 203 && b == 0 && c == 113)
        || a >= 224)
}

fn public_ip(ip: std::net::IpAddr) -> bool {
    match ip {
        std::net::IpAddr::V4(ip) => public_ipv4(ip),
        std::net::IpAddr::V6(ip) => {
            if let Some(mapped) = ip.to_ipv4_mapped() {
                return public_ipv4(mapped);
            }
            let seg = ip.segments();
            !ip.is_loopback()
                && !ip.is_unspecified()
                && (seg[0] & 0xfe00) != 0xfc00
                && (seg[0] & 0xffc0) != 0xfe80
                && (seg[0] & 0xff00) != 0xff00
                && !(seg[0] == 0x2001 && seg[1] == 0x0db8)
                // Deprecated IPv4-compatible space (::/96) is not a public radio endpoint.
                && !(seg[..6].iter().all(|part| *part == 0))
        }
    }
}

fn http_url(value: &str) -> bool {
    if value.len() > 4_096 {
        return false;
    }
    reqwest::Url::parse(value).ok().is_some_and(|url| {
        let Some(host) = url.host_str() else { return false };
        if !matches!(url.scheme(), "http" | "https")
            || !url.username().is_empty()
            || url.password().is_some()
            || host.eq_ignore_ascii_case("localhost")
            || host.ends_with(".localhost")
            || host.ends_with(".local")
        {
            return false;
        }
        let ip_host = host.strip_prefix('[').and_then(|h| h.strip_suffix(']')).unwrap_or(host);
        match ip_host.parse::<std::net::IpAddr>() {
            Ok(ip) => public_ip(ip),
            // Radio Browser should point at an Internet hostname, not an mDNS/single-label LAN name.
            Err(_) => host.contains('.'),
        }
    })
}

pub fn normalize_station(mut station: RadioStation) -> Option<RadioStation> {
    let clean = |value: &str, max: usize| {
        value.trim().chars().filter(|c| !c.is_control()).take(max).collect::<String>()
    };
    station.station_uuid = station.station_uuid.trim().to_owned();
    station.name = clean(&station.name, 200);
    station.stream_url = station.stream_url.trim().to_owned();
    station.url = station.url.trim().to_owned();
    station.homepage = station.homepage.trim().to_owned();
    station.favicon = station.favicon.trim().to_owned();
    station.country = clean(&station.country, 100);
    station.country_code = clean(&station.country_code, 8).to_ascii_uppercase();
    station.tags = clean(&station.tags, 512);
    station.codec = clean(&station.codec, 32).to_ascii_uppercase();

    if !valid_station_uuid(&station.station_uuid) {
        return None;
    }
    if !http_url(&station.stream_url) {
        station.stream_url = station.url.clone();
    }
    if station.station_uuid.is_empty() || station.name.is_empty() || !http_url(&station.stream_url)
    {
        return None;
    }
    if !station.homepage.is_empty() && !http_url(&station.homepage) {
        station.homepage.clear();
    }
    if !station.favicon.is_empty() && !http_url(&station.favicon) {
        station.favicon.clear();
    }
    Some(station)
}

/// Top stations for an empty query, or a name search otherwise. Limit is intentionally bounded:
/// Radio Browser can return tens of thousands of rows and the Radio page never needs them at once.
pub async fn stations(
    query: Option<&str>,
    offset: usize,
    limit: usize,
) -> Result<Vec<RadioStation>, String> {
    let limit = limit.clamp(1, 50);
    let offset = offset.min(10_000);
    let query = query.unwrap_or_default().trim();
    if query.chars().count() > MAX_QUERY_CHARS {
        return Err(format!("Radio search is limited to {MAX_QUERY_CHARS} characters."));
    }
    let path = if query.is_empty() {
        format!("/json/stations/topvote?offset={offset}&limit={limit}&hidebroken=true")
    } else {
        format!(
            "/json/stations/search?name={}&order=votes&reverse=true&hidebroken=true&offset={offset}&limit={limit}",
            urlencoding::encode(query)
        )
    };
    let rows: Vec<RadioStation> = get_json(&path).await?;
    let rows: Vec<RadioStation> = rows.into_iter().filter_map(normalize_station).collect();
    remember_stations(&rows);
    Ok(rows)
}

/// Resolve a station id entirely in native code. The normal path is the bounded cache populated by
/// `stations()`; a direct lookup covers stale UI cards without trusting renderer-supplied URLs.
pub async fn station_by_uuid(raw: &str) -> Result<RadioStation, String> {
    let uuid = raw.trim();
    if !valid_station_uuid(uuid) {
        return Err("Invalid radio station id.".into());
    }
    if let Some(station) = cached_station(uuid) {
        return Ok(station);
    }
    let path = format!("/json/stations/byuuid/{}", urlencoding::encode(uuid));
    let rows: Vec<RadioStation> = get_json(&path).await?;
    let station = rows
        .into_iter()
        .filter_map(normalize_station)
        .find(|station| station.station_uuid == uuid)
        .ok_or_else(|| "That radio station is no longer available.".to_string())?;
    remember_stations(std::slice::from_ref(&station));
    Ok(station)
}

/// Radio Browser asks clients to register a click when a station is actually played. Best effort:
/// playback is never delayed or failed because analytics for the public directory were unavailable.
pub async fn count_click(station_uuid: &str) {
    let uuid = station_uuid.trim();
    if uuid.is_empty() {
        return;
    }
    let path = format!("/json/url/{}", urlencoding::encode(uuid));
    let _: Result<serde_json::Value, String> = get_json(&path).await;
}

pub fn song_item(station: &RadioStation) -> SongItem {
    let live = match station.country_code.as_str() {
        "" => "Live Radio".to_owned(),
        code => format!("Live Radio · {code}"),
    };
    SongItem {
        video_id: format!("{RADIO_ID_PREFIX}{}", station.station_uuid),
        title: station.name.clone(),
        artists: live,
        album: Some("Internet Radio".into()),
        thumbnail: (!station.favicon.is_empty()).then(|| station.favicon.clone()),
        is_video: false,
        ..Default::default()
    }
}

pub fn playback_data(station: &RadioStation) -> PlaybackData {
    PlaybackData {
        video_id: format!("{RADIO_ID_PREFIX}{}", station.station_uuid),
        stream_url: station.stream_url.clone(),
        itag: 0,
        mpv_options: Vec::new(),
        headers: Default::default(),
        expires_in_seconds: 0,
        loudness_db: None,
        playback_ping: None,
        title: Some(station.name.clone()),
        artists: Some("Live Radio".into()),
        duration: None,
        thumbnail: (!station.favicon.is_empty()).then(|| station.favicon.clone()),
        is_video: Some(false),
        stream_client: "radio".into(),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn station_normalization_prefers_resolved_and_rejects_non_http_streams() {
        let good = normalize_station(RadioStation {
            station_uuid: " abc ".into(),
            name: " Test ".into(),
            stream_url: "https://stream.example/live".into(),
            url: "https://fallback.example/live".into(),
            homepage: "javascript:bad".into(),
            favicon: "https://example.test/icon.png".into(),
            country: String::new(),
            country_code: "in".into(),
            tags: String::new(),
            codec: "mp3".into(),
            bitrate: 128,
            votes: 1,
            clickcount: 0,
        })
        .unwrap();
        assert_eq!(good.station_uuid, "abc");
        assert_eq!(good.country_code, "IN");
        assert_eq!(good.codec, "MP3");
        assert!(good.homepage.is_empty());

        let bad = normalize_station(RadioStation {
            stream_url: "file:///tmp/a".into(),
            url: String::new(),
            ..good
        });
        assert!(bad.is_none());
    }

    #[test]
    fn radio_mirror_discovery_accepts_only_official_hosts() {
        assert_eq!(
            official_mirror_host("de1.api.radio-browser.info."),
            Some("de1.api.radio-browser.info".into())
        );
        assert!(official_mirror_host("localhost").is_none());
        assert!(official_mirror_host("radio-browser.info.evil.example").is_none());
    }

    #[test]
    fn radio_streams_reject_local_network_and_credentials() {
        assert!(http_url("https://radio.example/live"));
        assert!(!http_url("http://127.0.0.1:8000/live"));
        assert!(!http_url("http://192.168.1.5/live"));
        assert!(!http_url("http://169.254.169.254/latest/meta-data"));
        assert!(!http_url("http://[::1]/live"));
        assert!(!http_url("http://[::ffff:127.0.0.1]/live"));
        assert!(!http_url("http://[::ffff:192.168.1.5]/live"));
        assert!(!http_url("http://speaker.local/live"));
        assert!(!http_url("http://intranet/live"));
        assert!(!http_url("https://user:pass@example.com/live"));
        assert!(!http_url("file:///tmp/music"));
    }

    #[test]
    fn radio_station_ids_are_small_opaque_tokens() {
        assert!(valid_station_uuid("0f0f0f0f-1234-5678-90ab-abcdefabcdef"));
        assert!(!valid_station_uuid(""));
        assert!(!valid_station_uuid("../etc/passwd"));
        assert!(!valid_station_uuid(&"x".repeat(97)));
    }

    #[test]
    fn radio_browser_input_serializes_to_camel_case_for_the_ui() {
        let station: RadioStation = serde_json::from_value(serde_json::json!({
            "stationuuid": "station-1",
            "name": "Example",
            "url_resolved": "https://example.test/live",
            "countrycode": "IN"
        }))
        .unwrap();
        assert_eq!(station.station_uuid, "station-1");
        assert_eq!(station.stream_url, "https://example.test/live");
        assert_eq!(station.country_code, "IN");

        let json = serde_json::to_value(&station).unwrap();
        assert_eq!(json["stationUuid"], "station-1");
        assert_eq!(json["streamUrl"], "https://example.test/live");
        assert_eq!(json["countryCode"], "IN");
        assert!(json.get("stationuuid").is_none());
        assert!(json.get("url_resolved").is_none());
    }

    #[test]
    fn radio_song_ids_are_namespaced() {
        let station = RadioStation {
            station_uuid: "id".into(),
            name: "Station".into(),
            stream_url: "https://example.test/live".into(),
            url: String::new(),
            homepage: String::new(),
            favicon: String::new(),
            country: String::new(),
            country_code: "US".into(),
            tags: String::new(),
            codec: String::new(),
            bitrate: 0,
            votes: 0,
            clickcount: 0,
        };
        let song = song_item(&station);
        assert_eq!(song.video_id, "RYOTUNES_RADIO:id");
        assert_eq!(song.artists, "Live Radio · US");
    }
}
