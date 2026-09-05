//! The `SoundCloud` guest client: `client_id` discovery + cache + one-retry, and every endpoint
//! the daemon bridge needs. Pure transport — no UI, no mpv.

use std::collections::HashMap;
use std::path::PathBuf;
use std::time::Duration;

use serde::de::DeserializeOwned;
use serde::Deserialize;
use tokio::sync::RwLock;

use crate::models::*;
use crate::{asset_script_srcs, downsample_waveform, scrape_client_id_from_js};

const API: &str = "https://api-v2.soundcloud.com";
const HOME: &str = "https://soundcloud.com/";
/// A current desktop-Chrome UA; SoundCloud serves the client_id bundles only to browser-shaped UAs.
const UA: &str = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) \
                  Chrome/124.0.0.0 Safari/537.36";
/// The page limit used across listing endpoints.
const LIMIT: &str = "20";
/// Hydration batch size for `/tracks?ids=…`.
const HYDRATE_BATCH: usize = 50;

#[derive(Debug, thiserror::Error)]
pub enum Error {
    #[error("http: {0}")]
    Http(#[from] reqwest::Error),
    #[error("json: {0}")]
    Json(#[from] serde_json::Error),
    #[error("could not discover a SoundCloud client_id")]
    ClientId,
    #[error("SoundCloud returned HTTP {status} for {url}")]
    Api { status: u16, url: String },
    #[error("track {0} has no playable HLS transcoding")]
    NoStream(u64),
    #[error("track {0} has no waveform")]
    NoWaveform(u64),
}

pub type Result<T, E = Error> = std::result::Result<T, E>;

/// A guest SoundCloud client. Cheap to clone the way callers expect an `Arc` wrapper (holds a
/// pooled `reqwest::Client` and a shared, refreshable `client_id`).
pub struct SoundCloud {
    http: reqwest::Client,
    cache_dir: PathBuf,
    client_id: RwLock<Option<String>>,
}

impl SoundCloud {
    /// `cache_dir` is where the discovered `client_id` is persisted (`<cache_dir>/client_id`);
    /// the caller passes `<data>/soundcloud`.
    pub fn new(cache_dir: PathBuf) -> Self {
        let http = reqwest::Client::builder()
            .user_agent(UA)
            .connect_timeout(Duration::from_secs(30))
            .timeout(Duration::from_secs(60))
            .pool_idle_timeout(Duration::from_secs(90))
            .build()
            .expect("reqwest client builds with rustls");
        SoundCloud { http, cache_dir, client_id: RwLock::new(None) }
    }

    fn cache_file(&self) -> PathBuf {
        self.cache_dir.join("client_id")
    }

    // --- client_id lifecycle -------------------------------------------------

    /// The current `client_id`: in-memory, then disk cache, then a fresh scrape.
    async fn client_id(&self) -> Result<String> {
        if let Some(id) = self.client_id.read().await.clone() {
            return Ok(id);
        }
        if let Ok(disk) = tokio::fs::read_to_string(self.cache_file()).await {
            let disk = disk.trim().to_string();
            if disk.len() == 32 {
                *self.client_id.write().await = Some(disk.clone());
                return Ok(disk);
            }
        }
        self.refresh_client_id().await
    }

    /// Scrape a fresh `client_id`, replacing the cached value (memory + disk). Called on startup
    /// discovery and once on a 401/403 (the public id rotates).
    async fn refresh_client_id(&self) -> Result<String> {
        let id = self.scrape_client_id().await?;
        *self.client_id.write().await = Some(id.clone());
        let _ = tokio::fs::create_dir_all(&self.cache_dir).await;
        let _ = tokio::fs::write(self.cache_file(), &id).await;
        tracing::debug!(client_id = %id, "discovered SoundCloud client_id");
        Ok(id)
    }

    /// Fetch soundcloud.com, collect its asset bundles, and scan them (last first) for a client_id.
    async fn scrape_client_id(&self) -> Result<String> {
        let html = self.http.get(HOME).send().await?.text().await?;
        let mut srcs = asset_script_srcs(&html);
        srcs.reverse();
        for src in srcs {
            let js = match self.http.get(&src).send().await {
                Ok(r) => r.text().await.unwrap_or_default(),
                Err(_) => continue,
            };
            if let Some(id) = scrape_client_id_from_js(&js) {
                return Ok(id);
            }
        }
        Err(Error::ClientId)
    }

    // --- transport -----------------------------------------------------------

    /// GET `url` with `extra` query params plus the current `client_id`; on 401/403 re-scrape the
    /// id once and retry. Returns the raw (decompressed) body bytes.
    async fn get_bytes(&self, url: &str, extra: &[(&str, String)]) -> Result<Vec<u8>> {
        let id = self.client_id().await?;
        let resp = self.send(url, extra, &id).await?;
        let status = resp.status();
        if status.is_success() {
            return Ok(resp.bytes().await?.to_vec());
        }
        if status == reqwest::StatusCode::UNAUTHORIZED || status == reqwest::StatusCode::FORBIDDEN {
            let id = self.refresh_client_id().await?;
            let resp = self.send(url, extra, &id).await?;
            let status = resp.status();
            if status.is_success() {
                return Ok(resp.bytes().await?.to_vec());
            }
            return Err(Error::Api { status: status.as_u16(), url: url.to_string() });
        }
        Err(Error::Api { status: status.as_u16(), url: url.to_string() })
    }

    async fn send(
        &self,
        url: &str,
        extra: &[(&str, String)],
        client_id: &str,
    ) -> Result<reqwest::Response> {
        Ok(self.http.get(url).query(extra).query(&[("client_id", client_id)]).send().await?)
    }

    /// GET + deserialize JSON, with the client_id retry.
    async fn get_json<T: DeserializeOwned>(
        &self,
        url: &str,
        extra: &[(&str, String)],
    ) -> Result<T> {
        let bytes = self.get_bytes(url, extra).await?;
        Ok(serde_json::from_slice(&bytes)?)
    }

    // --- search --------------------------------------------------------------

    /// Mixed search (`/search`): tracks, users, and playlists (albums flagged via `is_album`).
    pub async fn search(&self, q: &str) -> Result<SearchResults> {
        let url = format!("{API}/search");
        let w: WireCollection<serde_json::Value> =
            self.get_json(&url, &[("q", q.into()), ("limit", LIMIT.into())]).await?;
        let mut out = SearchResults::default();
        for item in w.collection {
            let kind = item.get("kind").and_then(|k| k.as_str()).map(str::to_owned);
            match kind.as_deref() {
                Some("track") => {
                    if let Ok(t) = serde_json::from_value::<WireTrack>(item) {
                        out.tracks.push(map_track(t));
                    }
                }
                Some("user") => {
                    if let Ok(u) = serde_json::from_value::<WireUser>(item) {
                        out.users.push(map_user(u));
                    }
                }
                Some("playlist") => {
                    if let Ok(p) = serde_json::from_value::<WirePlaylist>(item) {
                        out.playlists.push(map_playlist(&p));
                    }
                }
                _ => {}
            }
        }
        Ok(out)
    }

    /// Paged track search (`/search/tracks`).
    pub async fn search_tracks(&self, q: &str, offset: u32) -> Result<Page<Track>> {
        let url = format!("{API}/search/tracks");
        let w: WireCollection<WireTrack> = self
            .get_json(
                &url,
                &[("q", q.into()), ("limit", LIMIT.into()), ("offset", offset.to_string())],
            )
            .await?;
        Ok(page(w, offset, map_track))
    }

    /// User search (`/search/users`).
    pub async fn search_users(&self, q: &str) -> Result<Vec<User>> {
        let url = format!("{API}/search/users");
        let w: WireCollection<WireUser> =
            self.get_json(&url, &[("q", q.into()), ("limit", LIMIT.into())]).await?;
        Ok(w.collection.into_iter().map(map_user).collect())
    }

    /// Playlist search (`/search/playlists`).
    pub async fn search_playlists(&self, q: &str) -> Result<Vec<Playlist>> {
        let url = format!("{API}/search/playlists");
        let w: WireCollection<WirePlaylist> =
            self.get_json(&url, &[("q", q.into()), ("limit", LIMIT.into())]).await?;
        Ok(w.collection.iter().map(map_playlist).collect())
    }

    // --- tracks --------------------------------------------------------------

    /// A single track (`/tracks/{id}`).
    pub async fn track(&self, id: u64) -> Result<Track> {
        let url = format!("{API}/tracks/{id}");
        let w: WireTrack = self.get_json(&url, &[]).await?;
        Ok(map_track(w))
    }

    /// Hydrate many tracks (`/tracks?ids=…`), 50 per call, preserving the caller's order.
    pub async fn tracks(&self, ids: &[u64]) -> Result<Vec<Track>> {
        let mut by_id: HashMap<u64, Track> = HashMap::with_capacity(ids.len());
        for chunk in ids.chunks(HYDRATE_BATCH) {
            let joined = chunk.iter().map(u64::to_string).collect::<Vec<_>>().join(",");
            let url = format!("{API}/tracks");
            let batch: Vec<WireTrack> = self.get_json(&url, &[("ids", joined)]).await?;
            for w in batch {
                by_id.insert(w.id, map_track(w));
            }
        }
        Ok(ids.iter().filter_map(|id| by_id.remove(id)).collect())
    }

    /// Resolve a playable HLS m3u8 URL for `track`, preferring aac (`audio/mp4`) then mp3
    /// (`audio/mpeg`). These URLs expire, so resolve at play time.
    pub async fn stream_url(&self, track: &Track) -> Result<String> {
        let hls = |mime_frag: &'static str| {
            track
                .transcodings
                .iter()
                .find(move |t| t.protocol == "hls" && t.mime_type.contains(mime_frag))
        };
        let chosen = hls("mp4")
            .or_else(|| hls("mpeg"))
            .or_else(|| track.transcodings.iter().find(|t| t.protocol == "hls"))
            .ok_or(Error::NoStream(track.id))?;

        let mut extra: Vec<(&str, String)> = Vec::new();
        if let Some(auth) = &track.authorization {
            extra.push(("track_authorization", auth.clone()));
        }
        let resolved: StreamUrl = self.get_json(&chosen.url, &extra).await?;
        Ok(resolved.url)
    }

    /// Fetch `track`'s waveform and downsample it to 240 peaks scaled `0..=100`. The waveform JSON
    /// is a public CDN payload (no client_id needed).
    pub async fn waveform(&self, track: &Track) -> Result<Vec<u8>> {
        let url = track.waveform_url.as_deref().ok_or(Error::NoWaveform(track.id))?;
        let bytes = self.http.get(url).send().await?.bytes().await?;
        let wave: WireWaveform = serde_json::from_slice(&bytes)?;
        Ok(downsample_waveform(&wave.samples, wave.height, 240))
    }

    // --- users ---------------------------------------------------------------

    /// A user profile (`/users/{id}`).
    pub async fn user(&self, id: u64) -> Result<User> {
        let url = format!("{API}/users/{id}");
        let w: WireUser = self.get_json(&url, &[]).await?;
        Ok(map_user(w))
    }

    /// A user's tracks, paged (`/users/{id}/tracks`).
    pub async fn user_tracks(&self, id: u64, offset: u32) -> Result<Page<Track>> {
        let url = format!("{API}/users/{id}/tracks");
        let w: WireCollection<WireTrack> =
            self.get_json(&url, &[("limit", LIMIT.into()), ("offset", offset.to_string())]).await?;
        Ok(page(w, offset, map_track))
    }

    /// A user's albums (`/users/{id}/albums`).
    pub async fn user_albums(&self, id: u64) -> Result<Vec<Playlist>> {
        let url = format!("{API}/users/{id}/albums");
        let w: WireCollection<WirePlaylist> =
            self.get_json(&url, &[("limit", "50".into())]).await?;
        Ok(w.collection.iter().map(map_playlist).collect())
    }

    /// A user's non-album playlists (`/users/{id}/playlists_without_albums`).
    pub async fn user_playlists(&self, id: u64) -> Result<Vec<Playlist>> {
        let url = format!("{API}/users/{id}/playlists_without_albums");
        let w: WireCollection<WirePlaylist> =
            self.get_json(&url, &[("limit", "50".into())]).await?;
        Ok(w.collection.iter().map(map_playlist).collect())
    }

    /// A user's liked tracks (`/users/{id}/likes`), unwrapping the `{track}` envelope and dropping
    /// liked playlists.
    pub async fn user_likes(&self, id: u64) -> Result<Vec<Track>> {
        let url = format!("{API}/users/{id}/likes");
        let w: WireCollection<serde_json::Value> =
            self.get_json(&url, &[("limit", "50".into())]).await?;
        let mut out = Vec::new();
        for item in w.collection {
            let track = match item.get("track") {
                Some(t) => Some(t.clone()),
                None if item.get("kind").and_then(|k| k.as_str()) == Some("track") => Some(item),
                None => None,
            };
            if let Some(t) = track {
                if let Ok(w) = serde_json::from_value::<WireTrack>(t) {
                    out.push(map_track(w));
                }
            }
        }
        Ok(out)
    }

    // --- playlists / related / charts ---------------------------------------

    /// A playlist with every track fully hydrated in order (`/playlists/{id}` hands back only the
    /// first few tracks hydrated; the rest are `{id}` stubs, hydrated in 50-id batches).
    pub async fn playlist(&self, id: u64) -> Result<PlaylistDetail> {
        let url = format!("{API}/playlists/{id}");
        let w: WirePlaylist = self.get_json(&url, &[]).await?;

        let order: Vec<u64> = w.tracks.iter().map(|t| t.id).collect();
        let mut by_id: HashMap<u64, Track> = HashMap::with_capacity(order.len());
        let mut missing: Vec<u64> = Vec::new();
        for wt in &w.tracks {
            if wt.is_hydrated() {
                by_id.insert(wt.id, map_track(wt.clone()));
            } else {
                missing.push(wt.id);
            }
        }
        if !missing.is_empty() {
            for t in self.tracks(&missing).await? {
                by_id.insert(t.id, t);
            }
        }
        let tracks = order.iter().filter_map(|id| by_id.remove(id)).collect();
        let description = crate::nonempty(w.description.clone());
        Ok(PlaylistDetail { playlist: map_playlist(&w), tracks, description })
    }

    /// Related / autoplay tracks (`/tracks/{id}/related`).
    pub async fn related(&self, id: u64) -> Result<Vec<Track>> {
        let url = format!("{API}/tracks/{id}/related");
        let w: WireCollection<WireTrack> = self.get_json(&url, &[("limit", LIMIT.into())]).await?;
        Ok(w.collection.into_iter().map(map_track).collect())
    }

    /// The guest Discover feed (`/mixed-selections`): titled shelves of playlists and system
    /// playlists. This replaces the retired `/charts` endpoint for guest Home.
    pub async fn discover(&self) -> Result<Vec<Selection>> {
        let url = format!("{API}/mixed-selections");
        let w: WireCollection<WireSelection> =
            self.get_json(&url, &[("limit", LIMIT.into())]).await?;
        let mut out = Vec::with_capacity(w.collection.len());
        for sel in w.collection {
            let slug =
                sel.urn.strip_prefix("soundcloud:selections:").unwrap_or(&sel.urn).to_string();
            let mut items = Vec::new();
            for item in sel.items.collection {
                match item.get("kind").and_then(|k| k.as_str()) {
                    Some("playlist") => {
                        if let Ok(p) = serde_json::from_value::<WirePlaylist>(item) {
                            items.push(DiscoverItem::Playlist(map_playlist(&p)));
                        }
                    }
                    Some("system-playlist") => {
                        if let Ok(s) = serde_json::from_value::<WireSystemPlaylist>(item) {
                            items.push(DiscoverItem::System(map_system_playlist(&s)));
                        }
                    }
                    _ => {}
                }
            }
            if !items.is_empty() {
                out.push(Selection { slug, title: sel.title, items });
            }
        }
        Ok(out)
    }

    /// Hydrate a system playlist (`/system-playlists/{urn}`) into a `PlaylistDetail`. `permalink`
    /// is e.g. "trending-by-genre:trap"; the response carries id-stub tracks, hydrated via
    /// `tracks()` in order. The synthetic playlist has id 0 and `set_type = "system"`.
    pub async fn system_playlist(&self, permalink: &str) -> Result<PlaylistDetail> {
        let urn = format!("soundcloud:system-playlists:{permalink}");
        let url = format!("{API}/system-playlists/{urn}");
        let w: WireSystemPlaylist = self.get_json(&url, &[]).await?;

        let ids: Vec<u64> = w.tracks.iter().map(|t| t.id).collect();
        let tracks = self.tracks(&ids).await?;
        let duration_ms = tracks.iter().map(|t| t.duration_ms).sum();
        let system = map_system_playlist(&w);
        let playlist = Playlist {
            id: 0,
            title: system.title.clone(),
            user: UserRef::default(),
            artwork: system.artwork.clone(),
            is_album: false,
            set_type: Some("system".to_string()),
            track_count: system.track_count,
            release_date: None,
            duration_ms,
        };
        Ok(PlaylistDetail { playlist, tracks, description: system.description.clone() })
    }
}

/// Turn a `{collection, next_href}` envelope into a `Page`, deriving the next offset from the
/// presence of `next_href` (SoundCloud paginates these endpoints by numeric offset).
fn page<W, T>(w: WireCollection<W>, offset: u32, map: impl Fn(W) -> T) -> Page<T> {
    let items: Vec<T> = w.collection.into_iter().map(map).collect();
    let next = w.next_href.map(|_| offset + items.len() as u32);
    Page { items, next }
}

/// The `/media/transcodings/…` resolution response.
#[derive(Deserialize)]
struct StreamUrl {
    url: String,
}

/// A waveform payload (`{width, height, samples}`).
#[derive(Deserialize, Default)]
struct WireWaveform {
    #[serde(default)]
    height: u32,
    #[serde(default)]
    samples: Vec<u32>,
}

/// A `/mixed-selections` shelf: `{urn:"soundcloud:selections:<slug>", title, items:{collection}}`.
#[derive(Deserialize, Default)]
struct WireSelection {
    #[serde(default)]
    urn: String,
    #[serde(default)]
    title: String,
    #[serde(default)]
    items: WireCollection<serde_json::Value>,
}
