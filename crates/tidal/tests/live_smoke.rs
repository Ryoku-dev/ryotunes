//! Live TIDAL checks. Not in the default run, because they need a real signed-in account.

use ryotunes_tidal::{Entitlement, Tidal, TidalAuth};

/// The profile name is the one thing the token response never carries, so the only honest check
/// is against a real account. A `None` is a pass too: it means the account never made a profile,
/// which is the common case, and the status line then shows the username.
///
///   RYOTUNES_TIDAL_TOKEN=… RYOTUNES_TIDAL_USER=… \
///     cargo test -p ryotunes-tidal --test live_smoke -- --ignored --nocapture
#[tokio::test]
#[ignore]
async fn the_profile_name_comes_off_the_account() {
    let (Ok(token), Ok(user)) =
        (std::env::var("RYOTUNES_TIDAL_TOKEN"), std::env::var("RYOTUNES_TIDAL_USER"))
    else {
        eprintln!("set RYOTUNES_TIDAL_TOKEN and RYOTUNES_TIDAL_USER to run this");
        return;
    };
    let t = Tidal::new(std::env::temp_dir().join("ryotunes-tidal-live"));
    t.set_auth(Some(TidalAuth {
        access_token: token,
        refresh_token: None,
        expires_at: i64::MAX,
        user_id: user.parse().expect("RYOTUNES_TIDAL_USER is the numeric account id"),
        country_code: std::env::var("RYOTUNES_TIDAL_COUNTRY").unwrap_or_else(|_| "DE".into()),
        entitlement: Entitlement::Standard,
        username: None,
        profile_name: None,
    }));

    let auth = t.load_profile_name().await.expect("the account endpoint answered");
    eprintln!("profile_name = {:?}", auth.profile_name);
    assert_eq!(auth.profile_name, t.auth().unwrap().profile_name, "the read is kept, not dropped");
    assert!(
        auth.profile_name.as_deref() != Some(""),
        "an unset profile name must arrive as None, never as an empty string the UI would print"
    );
}
