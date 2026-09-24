//! The visible SoundCloud sign-in window. SoundCloud's own web player keeps its OAuth access
//! token in a JS-readable `oauth_token` cookie and its session in `connect_session` +
//! `oauth_refresh_token` (httpOnly), and sends the bearer on every api-v2 call as
//! `Authorization: OAuth <token>`. WebKitGTK does not expose outgoing request headers, but its
//! cookie jar holds exactly those three cookies once the user has signed in — so this window
//! opens soundcloud.com/signin and harvests the jar, the same shape as the Google login in
//! `login.rs`. Unlike Google, the SPA writes the cookies without a navigation, so the jar is
//! watched through the cookie manager's `changed` signal as well as on finished loads.
//!
//! Persistent (`WebContext` default) on purpose: the webview keeps its SoundCloud session, so a
//! later re-login is one click.

use std::collections::BTreeMap;
use std::sync::Arc;

use parking_lot::Mutex;
use ryotunes_core::host::LoginError;
use ryotunes_soundcloud::SoundcloudAuth;
use tokio::sync::oneshot;

use crate::gtk_thread::Gtk;

/// A plain Linux Firefox UA: SoundCloud's login (and its IdP pages) behave normally under it,
/// and unlike the YouTube window there is no bot gate to spoof past.
const LOGIN_UA: &str = "Mozilla/5.0 (X11; Linux x86_64; rv:128.0) Gecko/20100101 Firefox/128.0";

/// The origin whose cookie jar carries the token triple.
const SC_ORIGIN: &str = "https://soundcloud.com";

/// Where the window starts: the site's own sign-in page (an account form, plus the Google /
/// Facebook / Apple buttons).
const LOGIN_URL: &str = "https://soundcloud.com/signin";

type Outcome = Result<SoundcloudAuth, LoginError>;
type Tx = Arc<Mutex<Option<oneshot::Sender<Outcome>>>>;

/// The host's SoundCloud sign-in: one visible window per call, on the GTK thread.
pub struct SoundcloudLogin {
    gtk: Gtk,
}

impl SoundcloudLogin {
    pub fn new(gtk: Gtk) -> Self {
        SoundcloudLogin { gtk }
    }

    /// Run the sign-in. `Ok(auth)` only after the harvested bearer parses; the caller persists
    /// it and installs it on the shared client.
    pub async fn sign_in(&self) -> Result<SoundcloudAuth, LoginError> {
        let (tx, rx) = oneshot::channel::<Outcome>();
        let tx: Tx = Arc::new(Mutex::new(Some(tx)));
        self.gtk.call(move || build_login_window(tx)).await;
        // The sender is dropped when the window is torn down without resolving → cancelled.
        rx.await.unwrap_or(Err(LoginError::Cancelled))
    }
}

/// True for https URLs the sign-in may live on: the site itself, its CDN, and the IdPs its
/// buttons open. Anything else is a phishing redirect and is refused.
fn allowed_navigation(url: &url::Url) -> bool {
    if url.scheme() != "https" {
        return false;
    }
    let Some(host) = url.host_str() else { return false };
    const HOSTS: [&str; 7] = [
        "soundcloud.com",
        "sndcdn.com",
        "google.com",
        "facebook.com",
        "facebook.net",
        "appleid.apple.com",
        "login.live.com",
    ];
    HOSTS.iter().any(|h| host == *h || host.ends_with(&format!(".{h}")))
}

/// Pick the token triple out of raw `(name, value, domain)` jar entries. Domain-matched by
/// hand like `login.rs` does for YouTube: anything outside soundcloud.com is dropped.
fn harvest(cookies: Vec<(String, String, String)>) -> Option<SoundcloudAuth> {
    let mut jar = BTreeMap::new();
    for (name, value, domain) in cookies {
        let domain = domain.strip_prefix('.').map(str::to_owned).unwrap_or(domain);
        if domain != "soundcloud.com" && !domain.ends_with(".soundcloud.com") {
            continue;
        }
        if matches!(name.as_str(), "oauth_token" | "oauth_refresh_token" | "connect_session") {
            jar.insert(name, value);
        }
    }
    let access = jar.remove("oauth_token")?;
    SoundcloudAuth::from_access_token(
        access,
        jar.remove("oauth_refresh_token"),
        jar.remove("connect_session"),
    )
}

fn build_login_window(tx: Tx) {
    use gtk::prelude::*;
    use webkit2gtk::{
        gio, CookieManagerExt, LoadEvent, NavigationPolicyDecision, NavigationPolicyDecisionExt,
        PolicyDecisionExt, PolicyDecisionType, SettingsExt, URIRequestExt, WebContext,
        WebContextExt, WebView, WebViewExt,
    };

    let view = WebView::new();
    crate::gtk_thread::install_clean_exit();
    if let Some(settings) = WebViewExt::settings(&view) {
        settings.set_user_agent(Some(LOGIN_UA));
    }

    view.connect_decide_policy(|_view, decision, kind| {
        if kind != PolicyDecisionType::NavigationAction
            && kind != PolicyDecisionType::NewWindowAction
        {
            return false;
        }
        let uri = decision
            .downcast_ref::<NavigationPolicyDecision>()
            .and_then(|d| d.navigation_action())
            .and_then(|a| a.request())
            .and_then(|r| r.uri());
        let allowed = uri
            .as_deref()
            .and_then(|u| url::Url::parse(u).ok())
            .map(|u| allowed_navigation(&u))
            .unwrap_or(false);
        if allowed {
            decision.use_();
        } else {
            decision.ignore();
        }
        true
    });

    // Resolve the flow from the jar: called on finished loads AND on every cookie change,
    // because the SPA lands the token without navigating.
    let resolve = {
        let view = view.clone();
        move |tx: &Tx| {
            let Some(cm) = WebContext::default().and_then(|c| c.cookie_manager()) else {
                return;
            };
            let tx = tx.clone();
            let view = view.clone();
            cm.cookies(SC_ORIGIN, None::<&gio::Cancellable>, move |res| {
                let Ok(cookies) = res else { return };
                let triples: Vec<(String, String, String)> = cookies
                    .into_iter()
                    .map(|mut c| {
                        (
                            c.name().map(|g| g.to_string()).unwrap_or_default(),
                            c.value().map(|g| g.to_string()).unwrap_or_default(),
                            c.domain().map(|g| g.to_string()).unwrap_or_default(),
                        )
                    })
                    .collect();
                let Some(auth) = harvest(triples) else {
                    return;
                };
                if let Some(tx) = tx.lock().take() {
                    let _ = tx.send(Ok(auth));
                }
                if let Some(w) = view.toplevel() {
                    unsafe { w.destroy() };
                }
            });
        }
    };

    let tx_load = tx.clone();
    let resolve_load = resolve.clone();
    view.connect_load_changed(move |_, event| {
        if event == LoadEvent::Finished {
            resolve_load(&tx_load);
        }
    });

    if let Some(cm) = WebContext::default().and_then(|c| c.cookie_manager()) {
        let tx_changed = tx.clone();
        cm.connect_changed(move |_| {
            resolve(&tx_changed);
        });
    }

    let window = gtk::Window::new(gtk::WindowType::Toplevel);
    window.set_title("Sign in to SoundCloud");
    window.set_default_size(480, 720);
    window.add(&view);

    window.connect_delete_event(move |_, _| {
        if let Some(tx) = tx.lock().take() {
            let _ = tx.send(Err(LoginError::Cancelled));
        }
        glib::Propagation::Proceed
    });

    window.show_all();
    view.load_uri(LOGIN_URL);
}

#[cfg(test)]
mod tests {
    use super::*;

    fn triple(name: &str, value: &str, domain: &str) -> (String, String, String) {
        (name.into(), value.into(), domain.into())
    }

    /// A JWT with a far-future exp so `from_access_token` keeps it alive.
    fn fake_jwt() -> String {
        use base64::Engine as _;
        let enc = |bytes: &[u8]| {
            base64::prelude::BASE64_URL_SAFE_NO_PAD.encode(bytes).trim_end_matches('=').to_string()
        };
        let exp =
            std::time::SystemTime::now().duration_since(std::time::UNIX_EPOCH).unwrap().as_secs()
                + 3600;
        format!(
            "{}.{}.sig",
            enc(b"{\"alg\":\"none\"}"),
            &enc(format!("{{\"exp\":{exp},\"client_id\":\"cid\"}}").as_bytes())
        )
    }

    #[test]
    fn harvest_needs_the_full_soundcloud_triple() {
        // An access token alone cannot rotate, but it is still a sign-in: harvest succeeds
        // with the refresh pair absent, and foreign-domain cookies never count.
        let auth = harvest(vec![triple("oauth_token", "elsewhere", ".example.com")]).is_none();
        assert!(auth, "a token outside soundcloud.com is not ours");
        let auth = harvest(vec![
            triple("oauth_token", &fake_jwt(), ".soundcloud.com"),
            triple("oauth_refresh_token", "rt", ".soundcloud.com"),
            triple("connect_session", "cs", ".soundcloud.com"),
        ])
        .expect("the triple harvests");
        assert_eq!(auth.refresh_token.as_deref(), Some("rt"));
        assert_eq!(auth.connect_session.as_deref(), Some("cs"));
    }

    #[test]
    fn navigation_stays_on_soundcloud_and_known_idps() {
        assert!(allowed_navigation(&url::Url::parse("https://soundcloud.com/signin").unwrap()));
        assert!(allowed_navigation(&url::Url::parse("https://a-v2.sndcdn.com/x.js").unwrap()));
        assert!(allowed_navigation(
            &url::Url::parse("https://accounts.google.com/o/oauth2/v2/auth").unwrap()
        ));
        assert!(!allowed_navigation(&url::Url::parse("https://evil.test/claim").unwrap()));
        assert!(!allowed_navigation(&url::Url::parse("http://soundcloud.com/insecure").unwrap()));
    }
}
