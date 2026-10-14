//! The client: one request funnel, the two sign-in flows, token upkeep, and the catalogue calls.

use std::path::PathBuf;
use std::sync::Arc;

use base64::Engine as _;
use rand::Rng as _;
use serde::de::DeserializeOwned;
use sha2::{Digest as _, Sha256};

use crate::auth::{
    now_secs, Credentials, DeviceLogin, DeviceResponse, Entitlement, PkceChallenge, TidalAuth,
    TokenResponse, DEVICE_AUTH_URL, PKCE_AUTH_URL, SCOPE, TOKEN_URL,
};
use crate::models::{Album, Artist, Favourite, Page, Playlist, SearchResults, Track};
use crate::stream::{resolve, PlaybackInfo, Quality, Stream};
use crate::{Error, Result};

const API: &str = "https://api.tidal.com/v1";
const USER_AGENT: &str = concat!("Ryotunes/", env!("CARGO_PKG_VERSION"));

/// The hook the host registers to persist a rotated token.
type RotateHook = Arc<dyn Fn(TidalAuth) + Send + Sync>;

pub struct Tidal {
    http: reqwest::Client,
    cache_dir: PathBuf,
    creds: parking_lot::RwLock<Credentials>,
    auth: parking_lot::RwLock<Option<TidalAuth>>,
    /// One refresh at a time: TIDAL rotates the refresh token, so two concurrent exchanges would
    /// spend a token that is already dead and sign the user out.
    refresh_lock: tokio::sync::Mutex<()>,
    on_rotate: parking_lot::RwLock<Option<RotateHook>>,
}

impl Tidal {
    pub fn new(cache_dir: PathBuf) -> Self {
        Tidal {
            http: reqwest::Client::builder().user_agent(USER_AGENT).build().unwrap_or_default(),
            cache_dir,
            creds: parking_lot::RwLock::new(Credentials::default()),
            auth: parking_lot::RwLock::new(None),
            refresh_lock: tokio::sync::Mutex::new(()),
            on_rotate: parking_lot::RwLock::new(None),
        }
    }

    pub fn cache_dir(&self) -> &PathBuf {
        &self.cache_dir
    }

    pub fn set_credentials(&self, creds: Credentials) {
        *self.creds.write() = creds;
    }

    pub fn credentials(&self) -> Credentials {
        self.creds.read().clone()
    }

    pub fn set_auth(&self, auth: Option<TidalAuth>) {
        *self.auth.write() = auth;
    }

    pub fn auth(&self) -> Option<TidalAuth> {
        self.auth.read().clone()
    }

    pub fn signed_in(&self) -> bool {
        self.auth.read().is_some()
    }

    /// The host registers this to persist a rotated token. Called after every exchange.
    pub fn set_on_rotate<F: Fn(TidalAuth) + Send + Sync + 'static>(&self, f: F) {
        *self.on_rotate.write() = Some(Arc::new(f));
    }

    /// The best tier the current token can actually fetch, or `None` when signed out.
    pub fn ceiling(&self) -> Option<Quality> {
        self.auth.read().as_ref().map(|a| a.entitlement.ceiling())
    }

    // ---- sign-in -------------------------------------------------------------------------

    /// Start the device flow. The user opens the returned page; the caller then polls
    /// [`Tidal::device_poll`] at the interval TIDAL asked for.
    pub async fn device_start(&self) -> Result<DeviceLogin> {
        let (id, _) = self.device_creds()?;
        let res = self
            .http
            .post(DEVICE_AUTH_URL)
            .form(&[("client_id", id.as_str()), ("scope", SCOPE)])
            .send()
            .await?;
        let res = check(res, DEVICE_AUTH_URL).await?;
        let d: DeviceResponse = res.json().await?;
        let complete = d
            .verification_uri_complete
            .unwrap_or_else(|| format!("{}?userCode={}", d.verification_uri, d.user_code));
        Ok(DeviceLogin {
            device_code: d.device_code,
            user_code: d.user_code,
            verification_uri: if d.verification_uri.starts_with("http") {
                d.verification_uri
            } else {
                format!("https://{}", d.verification_uri)
            },
            verification_uri_complete: if complete.starts_with("http") {
                complete
            } else {
                format!("https://{complete}")
            },
            expires_in: d.expires_in.unwrap_or(300),
            interval: d.interval.unwrap_or(2).max(1),
        })
    }

    /// One poll of the device flow. [`Error::Pending`] means keep waiting;
    /// [`Error::LoginExpired`] means start over.
    pub async fn device_poll(&self, device_code: &str) -> Result<TidalAuth> {
        let (id, secret) = self.device_creds()?;
        let mut form = vec![
            ("client_id", id.as_str()),
            ("device_code", device_code),
            ("grant_type", "urn:ietf:params:oauth:grant-type:device_code"),
            ("scope", SCOPE),
        ];
        if let Some(s) = secret.as_deref() {
            form.push(("client_secret", s));
        }
        let res = self.http.post(TOKEN_URL).form(&form).send().await?;
        let status = res.status();
        if status.is_success() {
            let token: TokenResponse = res.json().await?;
            return self.adopt(token, Entitlement::Standard);
        }
        let body = res.text().await.unwrap_or_default();
        match pending_kind(&body) {
            Some(Pending::Waiting) => Err(Error::Pending),
            Some(Pending::Expired) => Err(Error::LoginExpired),
            None => Err(Error::Api { status: status.as_u16(), url: TOKEN_URL.into() }),
        }
    }

    /// Build the PKCE pair and the page the user has to open. Hi-res is only reachable through
    /// this flow: the device client is not entitled to it, whatever quality is later requested.
    pub fn pkce_start(&self) -> Result<(PkceChallenge, String)> {
        let creds = self.creds.read().clone();
        if !creds.has_pkce() {
            return Err(Error::NoCredentials("pkce"));
        }
        let id = creds.pkce_id.unwrap_or_default();
        let redirect = creds.pkce_redirect.unwrap_or_default();
        let verifier = random_token(64);
        let challenge = base64::engine::general_purpose::URL_SAFE_NO_PAD
            .encode(Sha256::digest(verifier.as_bytes()));
        let state = random_token(16);
        let url = format!(
            "{PKCE_AUTH_URL}?response_type=code&client_id={id}&redirect_uri={redirect}\
             &scope={}&code_challenge_method=S256&code_challenge={challenge}&state={state}",
            SCOPE.replace(' ', "+")
        );
        Ok((PkceChallenge { verifier, challenge, state }, url))
    }

    /// Redeem the code the host captured from the redirect.
    pub async fn pkce_finish(&self, code: &str, verifier: &str) -> Result<TidalAuth> {
        let creds = self.creds.read().clone();
        if !creds.has_pkce() {
            return Err(Error::NoCredentials("pkce"));
        }
        let id = creds.pkce_id.unwrap_or_default();
        let redirect = creds.pkce_redirect.unwrap_or_default();
        let mut form = vec![
            ("client_id", id.as_str()),
            ("code", code),
            ("code_verifier", verifier),
            ("grant_type", "authorization_code"),
            ("redirect_uri", redirect.as_str()),
            ("scope", SCOPE),
        ];
        if let Some(secret) = creds.pkce_secret.as_deref() {
            form.push(("client_secret", secret));
        }
        let res = self.http.post(TOKEN_URL).form(&form).send().await?;
        let res = check(res, TOKEN_URL).await?;
        let token: TokenResponse = res.json().await?;
        self.adopt(token, Entitlement::HiRes)
    }

    pub fn sign_out(&self) {
        *self.auth.write() = None;
    }

    // ---- catalogue -----------------------------------------------------------------------

    pub async fn search(&self, query: &str, limit: u32) -> Result<SearchResults> {
        self.get("/search", &[("query", query), ("limit", &limit.to_string())]).await
    }

    pub async fn track(&self, id: u64) -> Result<Track> {
        self.get(&format!("/tracks/{id}"), &[]).await
    }

    pub async fn album(&self, id: u64) -> Result<Album> {
        self.get(&format!("/albums/{id}"), &[]).await
    }

    pub async fn album_tracks(&self, id: u64, limit: u32) -> Result<Page<Track>> {
        self.get(&format!("/albums/{id}/tracks"), &[("limit", &limit.to_string())]).await
    }

    pub async fn artist(&self, id: u64) -> Result<Artist> {
        self.get(&format!("/artists/{id}"), &[]).await
    }

    pub async fn artist_top_tracks(&self, id: u64, limit: u32) -> Result<Page<Track>> {
        self.get(&format!("/artists/{id}/toptracks"), &[("limit", &limit.to_string())]).await
    }

    pub async fn artist_albums(&self, id: u64, limit: u32) -> Result<Page<Album>> {
        self.get(&format!("/artists/{id}/albums"), &[("limit", &limit.to_string())]).await
    }

    pub async fn playlist(&self, uuid: &str) -> Result<Playlist> {
        self.get(&format!("/playlists/{uuid}"), &[]).await
    }

    pub async fn playlist_tracks(&self, uuid: &str, limit: u32) -> Result<Page<Track>> {
        self.get(&format!("/playlists/{uuid}/tracks"), &[("limit", &limit.to_string())]).await
    }

    pub async fn my_tracks(&self, limit: u32) -> Result<Page<Favourite<Track>>> {
        let user = self.user_id()?;
        self.get(&format!("/users/{user}/favorites/tracks"), &[("limit", &limit.to_string())]).await
    }

    pub async fn my_albums(&self, limit: u32) -> Result<Page<Favourite<Album>>> {
        let user = self.user_id()?;
        self.get(&format!("/users/{user}/favorites/albums"), &[("limit", &limit.to_string())]).await
    }

    pub async fn my_playlists(&self, limit: u32) -> Result<Page<Playlist>> {
        let user = self.user_id()?;
        self.get(&format!("/users/{user}/playlists"), &[("limit", &limit.to_string())]).await
    }

    // ---- playback ------------------------------------------------------------------------

    /// Resolve a track to something openable at the best tier the token allows, never above it.
    ///
    /// Asking above the entitlement does not fail loudly: TIDAL answers with a preview, which this
    /// reports as [`Error::Preview`]. Asking for a protected asset yields [`Error::Protected`].
    /// Neither is retried here, and neither should be retried by the caller at a lower quality
    /// without telling the user what happened.
    pub async fn stream(&self, id: u64, want: Quality) -> Result<Stream> {
        let quality = match self.ceiling() {
            Some(ceiling) => want.min(ceiling),
            None => return Err(Error::SignedOut),
        };
        let info: PlaybackInfo = self
            .get(
                &format!("/tracks/{id}/playbackinfopostpaywall"),
                &[
                    ("audioquality", quality.as_str()),
                    ("playbackmode", "STREAM"),
                    ("assetpresentation", "FULL"),
                    // Without this TIDAL may answer with an immersive asset, which no plain player
                    // can open and which this crate does not handle.
                    ("immersiveaudio", "false"),
                ],
            )
            .await?;
        resolve(&info)
    }

    // ---- plumbing ------------------------------------------------------------------------

    fn device_creds(&self) -> Result<(String, Option<String>)> {
        let creds = self.creds.read().clone();
        if !creds.has_device() {
            return Err(Error::NoCredentials("device"));
        }
        Ok((creds.device_id.unwrap_or_default(), creds.device_secret))
    }

    fn user_id(&self) -> Result<u64> {
        self.auth.read().as_ref().map(|a| a.user_id).ok_or(Error::SignedOut)
    }

    fn adopt(&self, token: TokenResponse, entitlement: Entitlement) -> Result<TidalAuth> {
        let prior = self.auth.read().clone();
        let user = token.user.as_ref();
        let auth = TidalAuth {
            access_token: token.access_token,
            refresh_token: token
                .refresh_token
                .or_else(|| prior.as_ref().and_then(|p| p.refresh_token.clone())),
            expires_at: now_secs() + token.expires_in.unwrap_or(3600),
            user_id: user
                .and_then(|u| u.user_id)
                .or_else(|| prior.as_ref().map(|p| p.user_id))
                .unwrap_or(0),
            country_code: user
                .and_then(|u| u.country_code.clone())
                .or_else(|| prior.as_ref().map(|p| p.country_code.clone()))
                .unwrap_or_else(|| "US".into()),
            entitlement,
            username: user
                .and_then(|u| u.username.clone())
                .or_else(|| prior.as_ref().and_then(|p| p.username.clone())),
        };
        *self.auth.write() = Some(auth.clone());
        if let Some(hook) = self.on_rotate.read().clone() {
            hook(auth.clone());
        }
        Ok(auth)
    }

    /// Refresh under the lock, re-checking first in case a sibling already did it.
    async fn ensure_fresh(&self) -> Result<TidalAuth> {
        let current = self.auth.read().clone().ok_or(Error::SignedOut)?;
        if !current.is_expired(now_secs()) {
            return Ok(current);
        }
        let _guard = self.refresh_lock.lock().await;
        let current = self.auth.read().clone().ok_or(Error::SignedOut)?;
        if !current.is_expired(now_secs()) {
            return Ok(current);
        }
        let refresh = current.refresh_token.clone().ok_or(Error::SignedOut)?;
        let (id, secret) = match current.entitlement {
            Entitlement::Standard => self.device_creds()?,
            Entitlement::HiRes => {
                let creds = self.creds.read().clone();
                if !creds.has_pkce() {
                    return Err(Error::NoCredentials("pkce"));
                }
                (creds.pkce_id.unwrap_or_default(), creds.pkce_secret)
            }
        };
        let mut form = vec![
            ("client_id", id.as_str()),
            ("refresh_token", refresh.as_str()),
            ("grant_type", "refresh_token"),
            ("scope", SCOPE),
        ];
        if let Some(s) = secret.as_deref() {
            form.push(("client_secret", s));
        }
        let res = self.http.post(TOKEN_URL).form(&form).send().await?;
        let res = check(res, TOKEN_URL).await?;
        let token: TokenResponse = res.json().await?;
        self.adopt(token, current.entitlement)
    }

    /// Every catalogue call goes through here: refresh if needed, add the bearer and the country,
    /// and turn a refusal into a typed error.
    async fn get<T: DeserializeOwned>(&self, path: &str, query: &[(&str, &str)]) -> Result<T> {
        let auth = self.ensure_fresh().await?;
        let url = format!("{API}{path}");
        let mut req = self
            .http
            .get(&url)
            .bearer_auth(&auth.access_token)
            .query(&[("countryCode", auth.country_code.as_str())]);
        if !query.is_empty() {
            req = req.query(query);
        }
        let res = check(req.send().await?, &url).await?;
        Ok(res.json().await?)
    }
}

async fn check(res: reqwest::Response, url: &str) -> Result<reqwest::Response> {
    let status = res.status();
    if status.is_success() {
        return Ok(res);
    }
    Err(Error::Api { status: status.as_u16(), url: url.to_string() })
}

enum Pending {
    Waiting,
    Expired,
}

/// The device endpoint answers a pending login with an error body rather than a status a caller
/// can switch on, so the body decides whether to keep polling.
fn pending_kind(body: &str) -> Option<Pending> {
    let lower = body.to_ascii_lowercase();
    if lower.contains("authorization_pending") || lower.contains("slow_down") {
        return Some(Pending::Waiting);
    }
    if lower.contains("expired_token") || lower.contains("expired") {
        return Some(Pending::Expired);
    }
    None
}

fn random_token(len: usize) -> String {
    const ALPHABET: &[u8] = b"ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-._~";
    let mut rng = rand::thread_rng();
    (0..len).map(|_| ALPHABET[rng.gen_range(0..ALPHABET.len())] as char).collect()
}

#[cfg(test)]
mod tests {
    use super::*;

    fn client() -> Tidal {
        Tidal::new(PathBuf::from("/tmp"))
    }

    #[test]
    fn a_fresh_client_is_already_configured() {
        let t = client();
        assert!(t.device_creds().is_ok(), "signing in must need no setup from the user");
        assert!(t.pkce_start().is_ok());
    }

    #[test]
    fn a_host_that_clears_the_pair_is_told_rather_than_left_guessing() {
        let t = client();
        t.set_credentials(Credentials {
            device_id: None,
            device_secret: None,
            pkce_id: None,
            pkce_secret: None,
            pkce_redirect: None,
        });
        assert!(matches!(t.device_creds(), Err(Error::NoCredentials("device"))));
        assert!(matches!(t.pkce_start(), Err(Error::NoCredentials("pkce"))));
    }

    #[test]
    fn a_call_without_a_session_says_so() {
        let t = client();
        assert!(matches!(t.user_id(), Err(Error::SignedOut)));
        assert_eq!(t.ceiling(), None);
        assert!(!t.signed_in());
    }

    #[test]
    fn the_pkce_url_carries_a_challenge_and_not_the_verifier() {
        let t = client();
        t.set_credentials(Credentials {
            pkce_id: Some("id".into()),
            pkce_redirect: Some("http://127.0.0.1:9000/cb".into()),
            ..Default::default()
        });
        let (pair, url) = t.pkce_start().expect("configured pkce must start");
        assert!(url.contains(&pair.challenge));
        assert!(!url.contains(&pair.verifier), "the verifier must never be sent");
        assert!(url.contains("code_challenge_method=S256"));
        assert_ne!(pair.verifier, pair.challenge);
    }

    #[test]
    fn a_standard_token_cannot_ask_above_its_entitlement() {
        let t = client();
        t.set_auth(Some(TidalAuth {
            access_token: "a".into(),
            refresh_token: None,
            expires_at: i64::MAX,
            user_id: 1,
            country_code: "DE".into(),
            entitlement: Entitlement::Standard,
            username: None,
        }));
        assert_eq!(t.ceiling(), Some(Quality::Lossless));
        assert_eq!(
            Quality::HiResLossless.min(t.ceiling().unwrap()),
            Quality::Lossless,
            "a hi-res request on a standard token must be clamped, not sent and previewed"
        );
    }

    #[test]
    fn the_device_poll_reads_its_waiting_states_from_the_body() {
        assert!(matches!(
            pending_kind(r#"{"error":"authorization_pending"}"#),
            Some(Pending::Waiting)
        ));
        assert!(matches!(pending_kind(r#"{"error":"slow_down"}"#), Some(Pending::Waiting)));
        assert!(matches!(pending_kind(r#"{"error":"expired_token"}"#), Some(Pending::Expired)));
        assert!(pending_kind(r#"{"error":"invalid_client"}"#).is_none());
    }

    #[test]
    fn a_rotation_reaches_the_host() {
        let t = client();
        let seen = Arc::new(parking_lot::Mutex::new(None::<TidalAuth>));
        let sink = seen.clone();
        t.set_on_rotate(move |a| *sink.lock() = Some(a));
        t.adopt(
            TokenResponse {
                access_token: "new".into(),
                refresh_token: Some("r".into()),
                expires_in: Some(60),
                user: None,
            },
            Entitlement::Standard,
        )
        .unwrap();
        assert_eq!(seen.lock().as_ref().map(|a| a.access_token.clone()), Some("new".into()));
    }

    #[test]
    fn a_refreshed_token_keeps_the_identity_the_first_one_carried() {
        let t = client();
        t.set_auth(Some(TidalAuth {
            access_token: "old".into(),
            refresh_token: Some("r1".into()),
            expires_at: 0,
            user_id: 42,
            country_code: "DE".into(),
            entitlement: Entitlement::HiRes,
            username: Some("vlad".into()),
        }));
        let fresh = t
            .adopt(
                TokenResponse {
                    access_token: "new".into(),
                    refresh_token: None,
                    expires_in: Some(60),
                    user: None,
                },
                Entitlement::HiRes,
            )
            .unwrap();
        assert_eq!(fresh.user_id, 42, "a refresh response names no user; keep the one we had");
        assert_eq!(fresh.country_code, "DE");
        assert_eq!(fresh.refresh_token.as_deref(), Some("r1"), "keep the rotation token too");
        assert_eq!(fresh.username.as_deref(), Some("vlad"));
    }

    #[test]
    fn random_tokens_differ_and_stay_in_the_unreserved_set() {
        let a = random_token(64);
        let b = random_token(64);
        assert_eq!(a.len(), 64);
        assert_ne!(a, b);
        assert!(a.chars().all(|c| c.is_ascii_alphanumeric() || "-._~".contains(c)));
    }
}
