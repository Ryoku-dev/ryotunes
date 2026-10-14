//! A standalone check that the provider talks to TIDAL: sign in once, then resolve a track and
//! print what came back, including a refusal.
//!
//! The client id is not supplied here and is not in the crate. TIDAL publishes none for third
//! party playback, so the host decides where one comes from; this example reads the environment.
//!
//!   RYOTUNES_TIDAL_CLIENT_ID=... RYOTUNES_TIDAL_CLIENT_SECRET=... \
//!     cargo run -p ryotunes-tidal --example probe -- [track-id] [quality]
//!
//! With no track id it signs in and reports the account. The session is cached next to the
//! example's own data so a second run does not sign in again.

use std::path::PathBuf;

use ryotunes_tidal::{Credentials, Error, Quality, Source, Tidal, TidalAuth};

#[tokio::main(flavor = "current_thread")]
async fn main() {
    let dir = cache_dir();
    if let Err(e) = std::fs::create_dir_all(&dir) {
        eprintln!("cannot use {}: {e}", dir.display());
        std::process::exit(1);
    }
    let tidal = Tidal::new(dir.clone());
    tidal.set_credentials(Credentials {
        device_id: std::env::var("RYOTUNES_TIDAL_CLIENT_ID").ok(),
        device_secret: std::env::var("RYOTUNES_TIDAL_CLIENT_SECRET").ok(),
        pkce_id: std::env::var("RYOTUNES_TIDAL_PKCE_ID").ok(),
        pkce_secret: std::env::var("RYOTUNES_TIDAL_PKCE_SECRET").ok(),
        pkce_redirect: std::env::var("RYOTUNES_TIDAL_PKCE_REDIRECT").ok(),
    });

    let session = dir.join("auth.json");
    if let Ok(text) = std::fs::read_to_string(&session) {
        match serde_json::from_str::<TidalAuth>(&text) {
            Ok(auth) => tidal.set_auth(Some(auth)),
            Err(e) => eprintln!("the cached session did not parse ({e}); signing in again"),
        }
    }
    let keep = session.clone();
    tidal.set_on_rotate(move |auth| match serde_json::to_string_pretty(&auth) {
        Ok(text) => {
            if let Err(e) = std::fs::write(&keep, text) {
                eprintln!("could not keep the session: {e}");
            }
        }
        Err(e) => eprintln!("could not serialise the session: {e}"),
    });

    if !tidal.signed_in() {
        if let Err(e) = sign_in(&tidal).await {
            eprintln!("sign-in failed: {e}");
            std::process::exit(1);
        }
    }

    match tidal.auth() {
        Some(auth) => println!(
            "signed in as {} (user {}, country {}), best reachable quality {:?}",
            auth.username.as_deref().unwrap_or("unknown"),
            auth.user_id,
            auth.country_code,
            auth.entitlement.ceiling()
        ),
        None => {
            eprintln!("no session after sign-in");
            std::process::exit(1);
        }
    }

    let mut args = std::env::args().skip(1);
    let Some(track) = args.next() else {
        println!("pass a track id to resolve one, for example: ... --example probe -- 12345678");
        return;
    };
    let Ok(track) = track.parse::<u64>() else {
        eprintln!("'{track}' is not a track id");
        std::process::exit(1);
    };
    let want = args.next().and_then(|q| Quality::parse(&q)).unwrap_or(Quality::HiResLossless);

    match tidal.track(track).await {
        Ok(t) => println!("track: {} - {}", t.artist_line(), t.title),
        Err(e) => eprintln!("could not read the track: {e}"),
    }

    match tidal.stream(track, want).await {
        Ok(s) => {
            println!("served quality: {:?}", s.quality);
            println!("codec: {}", s.codec.as_deref().unwrap_or("not stated"));
            if let Some(rate) = s.sample_rate {
                println!("sample rate: {rate} Hz");
            }
            if let Some(depth) = s.bit_depth {
                println!("bit depth: {depth}");
            }
            match s.source {
                Source::Direct(url) => println!("direct stream, open with: mpv '{url}'"),
                Source::Presentation { xml } => {
                    let path = dir.join("stream.mpd");
                    match std::fs::write(&path, xml) {
                        Ok(()) => {
                            println!("presentation written, open with: mpv '{}'", path.display())
                        }
                        Err(e) => eprintln!("could not write the presentation: {e}"),
                    }
                }
            }
        }
        Err(Error::Protected(kind)) => {
            println!("refused: TIDAL returned a protected asset ({kind}).");
            println!("this build does not play protected streams, by design.");
        }
        Err(Error::Preview(kind)) => {
            println!("refused: TIDAL returned a {kind} asset rather than the full track.");
            println!("the account is not entitled to the quality that was asked for.");
        }
        Err(e) => eprintln!("could not resolve the track: {e}"),
    }
}

async fn sign_in(tidal: &Tidal) -> Result<(), Error> {
    let login = tidal.device_start().await?;
    println!("open {} and enter {}", login.verification_uri, login.user_code);
    println!("or open {} directly", login.verification_uri_complete);
    let deadline =
        std::time::Instant::now() + std::time::Duration::from_secs(login.expires_in.max(0) as u64);
    loop {
        tokio::time::sleep(std::time::Duration::from_secs(login.interval)).await;
        match tidal.device_poll(&login.device_code).await {
            Ok(_) => return Ok(()),
            Err(Error::Pending) => {
                if std::time::Instant::now() >= deadline {
                    return Err(Error::LoginExpired);
                }
            }
            Err(e) => return Err(e),
        }
    }
}

fn cache_dir() -> PathBuf {
    std::env::var_os("XDG_CACHE_HOME")
        .map(PathBuf::from)
        .or_else(|| std::env::var_os("HOME").map(|h| PathBuf::from(h).join(".cache")))
        .unwrap_or_else(std::env::temp_dir)
        .join("ryotunes-tidal-probe")
}
