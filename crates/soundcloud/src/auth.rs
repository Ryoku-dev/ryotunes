//! Signed-in support: the OAuth bearer the host captures from the web player's cookie jar, its
//! JWT lifetime, and SoundCloud's rotating refresh-token exchange.
//!
//! SoundCloud's own web app keeps its access token in a JS-readable `oauth_token` cookie and a
//! single-use `oauth_refresh_token` (httpOnly) cookie, and refreshes through
//! `POST https://soundcloud.com/n/api/refresh-token` with `credentials: include`. The exchange
//! ROTATES the refresh token: the old one is dead the moment the new one is issued. A client that
//! spends one token twice (two concurrent refreshes) therefore logs itself out — so every refresh
//! runs under one async lock and re-checks whether a sibling already replaced the token before
//! spending another.

use std::time::{SystemTime, UNIX_EPOCH};

use base64::Engine as _;
use serde::{Deserialize, Serialize};

/// The refresh endpoint the site itself uses (cookie-authenticated, JSON body).
pub const REFRESH_URL: &str = "https://soundcloud.com/n/api/refresh-token";

/// Re-refresh this many seconds before the access token actually dies, so a request that starts
/// near the boundary never carries an expired bearer.
const EXPIRY_MARGIN_SECS: i64 = 60;
/// Fallback lifetime for a token whose JWT we cannot parse (the site's documented ~1 h).
const UNKNOWN_LIFETIME_SECS: i64 = 3600;
/// The web app's own client id, used as the fallback when a JWT carries no readable claim.
const FALLBACK_CLIENT_ID: &str = "3uJIGBRwdofKn6QKzONvDxUM1Vs4bTv9";

/// Everything needed to act as the signed-in user and to keep acting after the access token
/// expires. Serializable: the host persists this blob (and rewrites it on every rotation) under a
/// settings key the renderer can never read.
#[derive(Clone, Debug, PartialEq, Eq, Serialize, Deserialize)]
pub struct SoundcloudAuth {
    pub access_token: String,
    /// The single-use rotation token. `None` only if capture somehow saw no refresh cookie —
    /// then the auth dies with the access token and the user re-signs.
    pub refresh_token: Option<String>,
    /// The `client_id` claim inside the JWT: the refresh exchange must echo it back.
    pub client_id: String,
    /// Unix seconds when the access token dies.
    pub expires_at: i64,
    /// The `connect_session` cookie the refresh endpoint authenticates with.
    pub connect_session: Option<String>,
    /// The account's display name, filled in by the host after verifying the token against
    /// `/me`, so the status line survives restarts without another round trip.
    #[serde(default)]
    pub username: Option<String>,
}

impl SoundcloudAuth {
    /// Build auth from a captured bearer: the JWT carries its own expiry and client id, so a
    /// capture needs no extra round trip. Returns `None` only for an empty token.
    pub fn from_access_token(
        access_token: String,
        refresh_token: Option<String>,
        connect_session: Option<String>,
    ) -> Option<Self> {
        if access_token.trim().is_empty() {
            return None;
        }
        let claims = decode_jwt_claims(&access_token);
        let now = now_secs();
        let expires_at = claims.as_ref().and_then(|c| c.exp).unwrap_or(now + UNKNOWN_LIFETIME_SECS);
        let client_id = claims
            .as_ref()
            .and_then(|c| c.client_id.clone())
            .unwrap_or_else(|| FALLBACK_CLIENT_ID.to_string());
        Some(SoundcloudAuth {
            access_token,
            refresh_token,
            client_id,
            expires_at,
            connect_session,
            username: None,
        })
    }

    /// Whether the bearer is dead or about to be.
    pub fn is_expired(&self, now: i64) -> bool {
        self.expires_at <= now + EXPIRY_MARGIN_SECS
    }

    /// The `Cookie` header the refresh endpoint needs (it authenticates the browser session).
    pub fn refresh_cookie(&self) -> Option<String> {
        let session = self.connect_session.as_deref()?;
        let mut cookie = format!("connect_session={session}");
        if let Some(rt) = &self.refresh_token {
            cookie.push_str(&format!("; oauth_refresh_token={rt}"));
        }
        Some(cookie)
    }
}

/// The two JWT claims we care about.
#[derive(Deserialize)]
struct JwtClaims {
    exp: Option<i64>,
    #[serde(alias = "client_id", alias = "azp")]
    client_id: Option<String>,
}

fn decode_jwt_claims(token: &str) -> Option<JwtClaims> {
    let payload = token.split('.').nth(1)?;
    let bytes = base64::prelude::BASE64_URL_SAFE_NO_PAD
        .decode(payload.trim_end_matches('='))
        .or_else(|_| base64::prelude::BASE64_URL_SAFE.decode(payload))
        .ok()?;
    serde_json::from_slice(&bytes).ok()
}

fn now_secs() -> i64 {
    SystemTime::now().duration_since(UNIX_EPOCH).map(|d| d.as_secs() as i64).unwrap_or(0)
}

/// The refresh exchange's response.
#[derive(Deserialize)]
pub struct RefreshResponse {
    pub access_token: String,
    pub refresh_token: Option<String>,
    pub expires_in: Option<i64>,
}

#[cfg(test)]
mod tests {
    use super::*;

    /// A JWT-shaped token with a chosen exp/client_id payload, base64url-encoded like the site's.
    fn fake_jwt(payload: serde_json::Value) -> String {
        let enc = |bytes: &[u8]| {
            base64::prelude::BASE64_URL_SAFE_NO_PAD.encode(bytes).trim_end_matches('=').to_string()
        };
        format!("{}.{}.sig", enc(b"{\"alg\":\"none\"}"), &enc(payload.to_string().as_bytes()))
    }

    #[test]
    fn reads_expiry_and_client_id_from_the_jwt() {
        let auth = SoundcloudAuth::from_access_token(
            fake_jwt(serde_json::json!({"exp": 5_000_000_i64, "client_id": "abc123"})),
            None,
            None,
        )
        .unwrap();
        assert_eq!(auth.expires_at, 5_000_000);
        assert_eq!(auth.client_id, "abc123");
        assert!(auth.is_expired(5_000_000 - 30)); // inside the margin
        assert!(!auth.is_expired(5_000_000 - 3600));
    }

    #[test]
    fn an_unparseable_token_still_authenticates_with_a_fallback_lifetime() {
        let now = now_secs();
        let auth = SoundcloudAuth::from_access_token("not-a-jwt".into(), None, Some("sess".into()))
            .unwrap();
        assert!(auth.expires_at > now);
        assert_eq!(auth.client_id, FALLBACK_CLIENT_ID);
        assert_eq!(
            auth.refresh_cookie().as_deref(),
            Some("connect_session=sess"),
            "the refresh call authenticates with the session cookie"
        );
    }

    #[test]
    fn an_empty_token_is_no_auth() {
        assert!(SoundcloudAuth::from_access_token("  ".into(), None, None).is_none());
    }

    #[test]
    fn a_blob_without_a_username_round_trips() {
        let auth =
            SoundcloudAuth::from_access_token("not-a-jwt".into(), Some("rt".into()), None).unwrap();
        let json = serde_json::to_string(&auth).unwrap();
        let back: SoundcloudAuth = serde_json::from_str(&json).unwrap();
        assert_eq!(back, auth);
        // An older persisted blob (no username key at all) still parses.
        let v: serde_json::Value = serde_json::from_str(&json).unwrap();
        let mut obj = v.as_object().unwrap().clone();
        obj.remove("username");
        let old: SoundcloudAuth = serde_json::from_value(serde_json::Value::Object(obj)).unwrap();
        assert_eq!(old.username, None);
    }
}
