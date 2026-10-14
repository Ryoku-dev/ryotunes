//! Sign-in state and the two OAuth flows TIDAL exposes to a desktop client.
//!
//! TIDAL splits entitlement across two OAuth clients, and the split is not a detail: the device
//! authorization flow yields a token good for LOW through LOSSLESS, while HI_RES_LOSSLESS is only
//! served to a token minted through the PKCE authorization-code flow. The entitlement is baked in
//! when the token is issued, so swapping client ids on an existing token silently keeps serving
//! 16-bit audio. A host that wants hi-res has to sign the user in again through the other flow.
//!
//! TIDAL publishes no client credentials for third-party playback, so the pairs below are the ones
//! its own applications carry, as every open-source TIDAL client uses them. They are defaults, the
//! same way `crates/spotify` carries Spotify's: a host that has its own registered application
//! overrides them through [`Credentials`], and nothing here reads a user-visible setting.

use std::time::{SystemTime, UNIX_EPOCH};

use serde::{Deserialize, Serialize};

/// Where the device flow starts and where both flows redeem their codes.
pub const DEVICE_AUTH_URL: &str = "https://auth.tidal.com/v1/oauth2/device_authorization";
pub const TOKEN_URL: &str = "https://auth.tidal.com/v1/oauth2/token";
/// The PKCE flow sends the user here; the host captures the redirect and hands back the code.
pub const PKCE_AUTH_URL: &str = "https://login.tidal.com/authorize";

/// Scope both flows request. `r_usr` reads the account's own catalogue, `w_usr` writes favourites.
pub const SCOPE: &str = "r_usr w_usr";

/// The device-authorization pair. Entitled up to LOSSLESS.
pub const DEFAULT_DEVICE_ID: &str = "fX2JxdmntZWK0ixT";
pub const DEFAULT_DEVICE_SECRET: &str = "1Nn9AfDAjxrgJFJbKNWLeAyKGVGmINuXPPLHVXAvxAg=";
/// The PKCE pair, the only one TIDAL serves HI_RES_LOSSLESS to, with the redirect it expects.
pub const DEFAULT_PKCE_ID: &str = "6BDSRdpK9hqEBTgU";
pub const DEFAULT_PKCE_SECRET: &str = "xeuPmY7nbpZ9IIbLAcQ93shka1VNheUAqN6IcszjTG8=";
pub const DEFAULT_PKCE_REDIRECT: &str = "https://tidal.com/android/login/auth";

/// Refresh this many seconds before the access token dies, so a request that starts near the
/// boundary never carries a stale bearer.
const EXPIRY_MARGIN_SECS: i64 = 60;

/// Which OAuth client minted a token, and therefore what it is entitled to stream.
#[derive(Clone, Copy, Debug, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum Entitlement {
    /// Device authorization flow: LOW, HIGH and LOSSLESS.
    Standard,
    /// PKCE authorization-code flow: adds HI_RES_LOSSLESS.
    HiRes,
}

impl Entitlement {
    /// The best quality a token of this kind can actually fetch. Asking for more than this
    /// returns a preview asset rather than an error, which is why the ceiling is explicit here.
    pub fn ceiling(self) -> crate::Quality {
        match self {
            Entitlement::Standard => crate::Quality::Lossless,
            Entitlement::HiRes => crate::Quality::HiResLossless,
        }
    }
}

/// The client credentials in force. [`Credentials::default`] is the shipped pair, so a host that
/// sets nothing still signs in; a host with its own registered application replaces them.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct Credentials {
    /// Client id and secret for the device authorization flow.
    pub device_id: Option<String>,
    pub device_secret: Option<String>,
    /// Client id and secret for the PKCE flow, plus the redirect TIDAL sends the user back to.
    pub pkce_id: Option<String>,
    pub pkce_secret: Option<String>,
    pub pkce_redirect: Option<String>,
}

impl Default for Credentials {
    fn default() -> Self {
        Credentials {
            device_id: Some(DEFAULT_DEVICE_ID.to_string()),
            device_secret: Some(DEFAULT_DEVICE_SECRET.to_string()),
            pkce_id: Some(DEFAULT_PKCE_ID.to_string()),
            pkce_secret: Some(DEFAULT_PKCE_SECRET.to_string()),
            pkce_redirect: Some(DEFAULT_PKCE_REDIRECT.to_string()),
        }
    }
}

impl Credentials {
    pub fn has_device(&self) -> bool {
        self.device_id.as_deref().is_some_and(|s| !s.trim().is_empty())
    }

    pub fn has_pkce(&self) -> bool {
        self.pkce_id.as_deref().is_some_and(|s| !s.trim().is_empty())
            && self.pkce_redirect.as_deref().is_some_and(|s| !s.trim().is_empty())
    }
}

/// Everything needed to act as the signed-in user and to keep acting after the access token
/// expires. Serializable: the host persists this blob and rewrites it on every rotation.
#[derive(Clone, Debug, PartialEq, Eq, Serialize, Deserialize)]
pub struct TidalAuth {
    pub access_token: String,
    pub refresh_token: Option<String>,
    /// Unix seconds when the access token dies.
    pub expires_at: i64,
    /// The account's numeric id, needed for every personal collection endpoint.
    pub user_id: u64,
    /// The account's country, which TIDAL requires on nearly every catalogue request and which
    /// decides what the catalogue contains.
    pub country_code: String,
    /// Which flow minted this token. Persisted so a restart knows whether hi-res is reachable
    /// without a round trip.
    pub entitlement: Entitlement,
    /// Filled in by the host after a `/users/me` call so the status line survives restarts.
    #[serde(default)]
    pub username: Option<String>,
}

impl TidalAuth {
    /// Whether the bearer is dead or about to be.
    pub fn is_expired(&self, now: i64) -> bool {
        self.expires_at <= now + EXPIRY_MARGIN_SECS
    }
}

/// What the device flow hands back for the user to act on.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct DeviceLogin {
    pub device_code: String,
    pub user_code: String,
    /// The bare page the user opens, and the same page with the code already filled in.
    pub verification_uri: String,
    pub verification_uri_complete: String,
    pub expires_in: i64,
    /// Seconds TIDAL asks the client to wait between polls.
    pub interval: u64,
}

/// The PKCE pair. The verifier never leaves the host; only the challenge is sent.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct PkceChallenge {
    pub verifier: String,
    pub challenge: String,
    pub state: String,
}

pub fn now_secs() -> i64 {
    SystemTime::now().duration_since(UNIX_EPOCH).map(|d| d.as_secs() as i64).unwrap_or(0)
}

/// Wire shape of a successful token exchange, shared by both flows and by refresh.
#[derive(Debug, Deserialize)]
pub(crate) struct TokenResponse {
    pub access_token: String,
    #[serde(default)]
    pub refresh_token: Option<String>,
    #[serde(default)]
    pub expires_in: Option<i64>,
    #[serde(default)]
    pub user: Option<TokenUser>,
}

#[derive(Debug, Deserialize)]
pub(crate) struct TokenUser {
    #[serde(rename = "userId")]
    pub user_id: Option<u64>,
    #[serde(rename = "countryCode")]
    pub country_code: Option<String>,
    #[serde(default)]
    pub username: Option<String>,
}

#[derive(Debug, Deserialize)]
pub(crate) struct DeviceResponse {
    #[serde(rename = "deviceCode")]
    pub device_code: String,
    #[serde(rename = "userCode")]
    pub user_code: String,
    #[serde(rename = "verificationUri")]
    pub verification_uri: String,
    #[serde(rename = "verificationUriComplete")]
    pub verification_uri_complete: Option<String>,
    #[serde(rename = "expiresIn")]
    pub expires_in: Option<i64>,
    #[serde(default)]
    pub interval: Option<u64>,
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn expiry_keeps_a_margin() {
        let auth = TidalAuth {
            access_token: "t".into(),
            refresh_token: None,
            expires_at: 1_000,
            user_id: 1,
            country_code: "DE".into(),
            entitlement: Entitlement::Standard,
            username: None,
        };
        assert!(!auth.is_expired(900), "still good well before the margin");
        assert!(auth.is_expired(941), "inside the 60 s margin counts as expired");
        assert!(auth.is_expired(1_000));
    }

    #[test]
    fn a_blob_without_a_username_still_parses() {
        let json = r#"{"access_token":"a","refresh_token":"r","expires_at":5,"user_id":7,
            "country_code":"DE","entitlement":"standard"}"#;
        let auth: TidalAuth = serde_json::from_str(json).expect("older blobs must keep parsing");
        assert_eq!(auth.username, None);
        assert_eq!(auth.entitlement, Entitlement::Standard);
    }

    #[test]
    fn the_standard_flow_cannot_reach_hi_res() {
        assert_eq!(Entitlement::Standard.ceiling(), crate::Quality::Lossless);
        assert_eq!(Entitlement::HiRes.ceiling(), crate::Quality::HiResLossless);
    }

    #[test]
    fn the_shipped_pair_is_enough_to_sign_in() {
        let c = Credentials::default();
        assert!(c.has_device(), "a user must not have to paste anything to sign in");
        assert!(c.has_pkce(), "the hi-res flow is configured out of the box too");
    }

    #[test]
    fn credentials_ignore_blank_strings() {
        let mut c = Credentials { device_id: Some("  ".into()), ..Default::default() };
        assert!(!c.has_device());
        c.device_id = Some("real".into());
        assert!(c.has_device());
    }
}
