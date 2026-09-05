//! Spotify sign-in. Adapted from nolight132/sonora `spotify/auth.rs`.
//!
//! Change from Sonora: Sonora used `librespot_oauth::OAuthClientBuilder`, which either opens a
//! browser itself or prints the authorization URL to stdout and blocks. A daemon can do neither, so
//! the PKCE authorization-code flow is driven here (via the `oauth2` crate librespot-oauth wraps):
//! the authorization URL is handed to a caller-supplied sink, and the loopback redirect is caught by
//! a small one-shot listener. Everything else — the credential cache, the Premium gate, the error
//! classification — is Sonora's, near-verbatim.

use std::path::PathBuf;

use anyhow::{Context as _, Result, anyhow};
use librespot_core::authentication::Credentials;
use librespot_core::cache::Cache;
use librespot_core::{Session, SessionConfig};
use oauth2::basic::BasicClient;
use oauth2::{
    AuthUrl, AuthorizationCode, ClientId, CsrfToken, PkceCodeChallenge, RedirectUrl, Scope,
    TokenResponse, TokenUrl,
};

use crate::credentials;

pub const DEFAULT_CLIENT_ID: &str = "65b708073fc0480ea92a077233ca87bd";
pub const DEFAULT_REDIRECT_URI: &str = "http://127.0.0.1:8989/login";

const AUTHORIZE_URL: &str = "https://accounts.spotify.com/authorize";
const TOKEN_URL: &str = "https://accounts.spotify.com/api/token";

/// How long the browser gets to come back with the redirect before the flow gives up.
const REDIRECT_WAIT: std::time::Duration = std::time::Duration::from_secs(300);
const PRODUCT_WAIT: std::time::Duration = std::time::Duration::from_secs(5);
const PRODUCT_POLL: std::time::Duration = std::time::Duration::from_millis(100);

pub const SCOPES: &[&str] = &[
    "playlist-read-collaborative",
    "playlist-read-private",
    "streaming",
    "user-follow-read",
    "user-library-read",
    "user-read-email",
    "user-read-playback-state",
    "user-read-private",
    "user-read-recently-played",
    "user-top-read",
];

/// Why a sign-in failed, when the reason is one the caller can act on (as opposed to a raw transport
/// error). Lifted from Sonora's `music` crate.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum SignInProblem {
    Premium,
    Region,
    Credentials,
    Network,
    Cancelled,
    Refused,
}

/// A [`SignInProblem`] carried as an [`std::error::Error`] so it can ride inside `anyhow` and be
/// recovered with `downcast_ref`.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub struct SignInFailure(pub SignInProblem);

impl std::fmt::Display for SignInFailure {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        let reason = match self.0 {
            SignInProblem::Premium => "the account has no Spotify Premium",
            SignInProblem::Region => "the account is out of its home region",
            SignInProblem::Credentials => "the stored credentials are no longer valid",
            SignInProblem::Network => "Spotify could not be reached",
            SignInProblem::Cancelled => "authorization was cancelled in the browser",
            SignInProblem::Refused => "Spotify refused the session",
        };
        write!(f, "{reason}")
    }
}

impl std::error::Error for SignInFailure {}

#[derive(Clone, Debug)]
pub struct AuthConfig {
    pub client_id: String,
    pub redirect_uri: String,
    pub cache_dir: PathBuf,
}

impl AuthConfig {
    /// The stored credential librespot writes after a successful connect.
    pub fn file(&self) -> PathBuf {
        self.cache_dir.join(credentials::FILE)
    }
}

/// The loopback authority (`host:port`) a redirect URI redirects to, so the callback listener knows
/// where to bind.
fn socket_address(uri: &str) -> Option<String> {
    let rest = uri.strip_prefix("http://").or_else(|| uri.strip_prefix("https://"))?;
    let authority = rest.split('/').next().filter(|host| !host.is_empty())?;
    match authority.rsplit_once(':') {
        Some((_, port)) if port.chars().all(|digit| digit.is_ascii_digit()) => {
            Some(authority.to_owned())
        }
        _ => Some(format!("{authority}:80")),
    }
}

pub async fn restore(config: &AuthConfig) -> Result<Option<Session>> {
    let session = session(config)?;
    let Some(credentials) = session.cache().and_then(|cache| cache.credentials()) else {
        return Ok(None);
    };

    session.connect(credentials, true).await.map_err(denied)?;
    credentials::secure(&config.file());
    premium(&session).await?;
    Ok(Some(session))
}

/// Run the PKCE authorization-code flow. `on_url` receives the authorization URL the moment it is
/// known — the daemon hands it to the client to open — then this blocks on the loopback redirect and
/// exchanges the code for a session.
pub async fn sign_in<F>(config: &AuthConfig, on_url: F) -> Result<Session>
where
    F: FnOnce(String) + Send + 'static,
{
    let access_token = authorize(config, on_url).await?;

    let session = session(config)?;
    session.connect(Credentials::with_access_token(access_token), true).await.map_err(denied)?;
    credentials::secure(&config.file());
    premium(&session).await?;
    Ok(session)
}

async fn authorize<F>(config: &AuthConfig, on_url: F) -> Result<String>
where
    F: FnOnce(String) + Send + 'static,
{
    let address = socket_address(&config.redirect_uri)
        .ok_or_else(|| anyhow!("cannot read a socket address from {}", config.redirect_uri))?;

    let client = BasicClient::new(ClientId::new(config.client_id.clone()))
        .set_auth_uri(AuthUrl::new(AUTHORIZE_URL.to_owned()).context("invalid authorize URL")?)
        .set_token_uri(TokenUrl::new(TOKEN_URL.to_owned()).context("invalid token URL")?)
        .set_redirect_uri(
            RedirectUrl::new(config.redirect_uri.clone())
                .with_context(|| format!("invalid redirect URI {}", config.redirect_uri))?,
        );

    let (challenge, verifier) = PkceCodeChallenge::new_random_sha256();
    let (auth_url, _csrf) = client
        .authorize_url(CsrfToken::new_random)
        .add_scopes(SCOPES.iter().map(|scope| Scope::new((*scope).to_owned())))
        .set_pkce_challenge(challenge)
        .url();

    on_url(auth_url.to_string());

    let code = tokio::time::timeout(REDIRECT_WAIT, listen(&address))
        .await
        .map_err(|_| anyhow::Error::new(SignInFailure(SignInProblem::Cancelled)))
        .context("no Spotify redirect arrived in time")??;

    let http = reqwest::Client::new();
    let token = client
        .exchange_code(AuthorizationCode::new(code))
        .set_pkce_verifier(verifier)
        .request_async(&http)
        .await
        .map_err(|error| anyhow!("Spotify token exchange failed: {error}"))?;

    Ok(token.access_token().secret().to_owned())
}

/// Wait for a single loopback redirect, mimicking Spotify's own client: bind the redirect socket,
/// accept one connection, pull `code` out of the request target, and reply with a close-me message.
/// Async so the caller's deadline can drop it; a blocking accept would hold the port forever and
/// make every later sign-in a silent no-op.
async fn listen(address: &str) -> Result<String> {
    use tokio::io::{AsyncBufReadExt as _, AsyncWriteExt as _, BufReader};

    let listener = tokio::net::TcpListener::bind(address)
        .await
        .with_context(|| format!("cannot bind the OAuth callback listener on {address}"))?;
    log::info!("auth: OAuth callback listening on {address}");

    let (stream, _) = listener.accept().await.context("the OAuth callback listener failed")?;
    let mut stream = BufReader::new(stream);

    let mut request_line = String::new();
    stream
        .read_line(&mut request_line)
        .await
        .context("cannot read the OAuth redirect request")?;

    let target = request_line
        .split_whitespace()
        .nth(1)
        .context("the OAuth redirect request had no target")?;
    let redirect = format!("http://localhost{target}");

    let message = "Ryotunes has your Spotify authorization. You can close this tab.";
    let response =
        format!("HTTP/1.1 200 OK\r\ncontent-length: {}\r\n\r\n{}", message.len(), message);
    let _ = stream.get_mut().write_all(response.as_bytes()).await;

    if let Some(code) = query_param(&redirect, "code") {
        return Ok(code);
    }
    match query_param(&redirect, "error").as_deref() {
        Some("access_denied") => Err(anyhow::Error::new(SignInFailure(SignInProblem::Cancelled))),
        Some(code) => Err(anyhow!("Spotify refused authorization ({code})")),
        None => Err(anyhow!("the OAuth redirect carried no authorization code")),
    }
}

fn query_param(redirect_url: &str, key: &str) -> Option<String> {
    let url = url::Url::parse(redirect_url).ok()?;
    url.query_pairs().find(|(name, _)| name == key).map(|(_, value)| value.into_owned())
}

async fn premium(session: &Session) -> Result<()> {
    let deadline = tokio::time::Instant::now() + PRODUCT_WAIT;
    loop {
        if let Some(account) = session.user_data().attributes.get("type") {
            match account.as_str() {
                "premium" => return Ok(()),
                _ => {
                    session.shutdown();
                    return Err(anyhow::Error::new(SignInFailure(SignInProblem::Premium)));
                }
            }
        }
        if tokio::time::Instant::now() >= deadline {
            return Ok(());
        }
        tokio::time::sleep(PRODUCT_POLL).await;
    }
}

fn denied(error: librespot_core::Error) -> anyhow::Error {
    let problem = classify(&error.to_string());
    anyhow::Error::new(error).context(SignInFailure(problem))
}

fn classify(message: &str) -> SignInProblem {
    let message = message.to_lowercase();
    if message.contains("travel restriction") {
        return SignInProblem::Region;
    }
    if message.contains("bad credentials") || message.contains("invalid credentials") {
        return SignInProblem::Credentials;
    }
    if message.contains("connection")
        || message.contains("timed out")
        || message.contains("dns")
        || message.contains("network")
    {
        return SignInProblem::Network;
    }
    SignInProblem::Refused
}

pub fn forget(config: &AuthConfig) {
    credentials::remove(&config.file());
}

fn session(config: &AuthConfig) -> Result<Session> {
    let cache = Cache::new(Some(config.cache_dir.as_path()), None, None, None)
        .with_context(|| format!("cannot open cache at {}", config.cache_dir.display()))?;

    let session_config =
        SessionConfig { client_id: config.client_id.clone(), ..Default::default() };

    Ok(Session::new(session_config, Some(cache)))
}

#[cfg(test)]
mod tests {
    use super::{query_param, socket_address};

    #[test]
    fn reads_host_and_port() {
        assert_eq!(
            socket_address("http://127.0.0.1:8989/login").as_deref(),
            Some("127.0.0.1:8989")
        );
    }

    #[test]
    fn defaults_the_port_when_absent() {
        assert_eq!(socket_address("http://localhost/login").as_deref(), Some("localhost:80"));
    }

    #[test]
    fn pulls_the_code_from_a_redirect() {
        assert_eq!(
            query_param("http://localhost/login?code=abc123&state=x", "code").as_deref(),
            Some("abc123")
        );
    }

    #[test]
    fn reads_a_cancelled_error() {
        assert_eq!(
            query_param("http://localhost/login?error=access_denied", "error").as_deref(),
            Some("access_denied")
        );
        assert_eq!(query_param("http://localhost/login?error=access_denied", "code"), None);
    }
}
