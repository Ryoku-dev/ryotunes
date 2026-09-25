//! SoundCloud sign-in through the browser the user already uses, instead of a Ryotunes-owned
//! webview. The embedded WebKit window could not complete real sign-ins: hCaptcha challenges
//! and the Google/Facebook/Apple popup flows need a genuine browser (their own engine, their
//! own profile, working popups), and users rightly refuse to type passwords into an app-owned
//! window. So `soundcloud_sign_in` now opens soundcloud.com in the system default browser and
//! imports the session from that browser's own cookie store once the user has signed in there.
//!
//! What gets imported is exactly what the old window harvested — the `oauth_token`,
//! `oauth_refresh_token` and `connect_session` triple scoped to soundcloud.com — read directly
//! from the cookie databases of the Gecko and Chromium families (Firefox, Zen, LibreWolf, …,
//! Chromium, Chrome, Brave, Vivaldi, Edge), including Flatpak and snap layouts. Only rows whose
//! host matches soundcloud.com are ever selected; no other cookie is read, and values are never
//! logged. A running browser keeps its database locked, so each store is copied (with its WAL)
//! to a private temp dir and read from the copy.
//!
//! The user may already be signed in — the first pass then completes the sign-in immediately.
//! Otherwise the caller polls until a valid triple appears or the window times out.

use std::collections::BTreeMap;
use std::path::{Path, PathBuf};

use rusqlite::{Connection, OpenFlags};
use ryotunes_soundcloud::SoundcloudAuth;

/// Where the user is sent: the site's own sign-in page (account form + IdP buttons), which in a
/// real browser has working captchas and popups.
pub const SIGN_IN_URL: &str = "https://soundcloud.com/signin";

/// The three cookies that make a SoundCloud web session.
const COOKIE_NAMES: [&str; 3] = ["oauth_token", "oauth_refresh_token", "connect_session"];

/// Which cookie dialect a store uses.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum Kind {
    /// Firefox family: plaintext `value` in `moz_cookies`.
    Gecko,
    /// Chromium family: `encrypted_value` in `cookies`, AES-128-CBC under the public Linux key.
    Chromium,
}

#[derive(Debug)]
pub struct Store {
    pub kind: Kind,
    pub path: PathBuf,
}

/// Hand the sign-in page to the system default browser. True when the launcher spawned; the
/// user still has to actually sign in there.
pub fn open_in_default_browser() -> bool {
    let mut command = std::process::Command::new("xdg-open");
    command
        .arg(SIGN_IN_URL)
        .stdin(std::process::Stdio::null())
        .stdout(std::process::Stdio::null())
        .stderr(std::process::Stdio::null());
    match command.spawn() {
        Ok(mut child) => {
            // xdg-open usually exits immediately, but a chooser dialog may linger; reap it on a
            // blocking thread so a tokio worker never parks in `wait`.
            std::thread::spawn(move || {
                let _ = child.wait();
            });
            true
        }
        Err(error) => {
            tracing::warn!(error = %error, "could not launch the default browser for SoundCloud sign-in");
            false
        }
    }
}

/// Try every discovered cookie store and return the first complete, parseable SoundCloud
/// session. Cheap enough to poll: a store whose database (or WAL journal) has not been
/// touched since the last scan is skipped — a running browser writes every cookie change
/// through the WAL, so unchanged mtimes mean unchanged cookies. Side-effect free.
pub fn import_any() -> Option<SoundcloudAuth> {
    import_from(&discover())
}

fn import_from(stores: &[Store]) -> Option<SoundcloudAuth> {
    for store in stores {
        if !changed_since_last_look(&store.path) {
            continue;
        }
        let cookies = match store.kind {
            Kind::Gecko => read_gecko(&store.path),
            Kind::Chromium => read_chromium(&store.path),
        };
        let Some(cookies) = cookies else { continue };
        if let Some(auth) = harvest(cookies) {
            tracing::info!(store = %store.path.display(), "imported a SoundCloud session from the browser");
            return Some(auth);
        }
    }
    None
}

/// True when a store's database or WAL journal changed since the previous scan (or was never
/// scanned / cannot be stat'ed — err toward reading it). The memo is process-wide and only
/// ever consulted by the sign-in poller, so a stale entry costs one re-read, never a miss.
fn changed_since_last_look(path: &Path) -> bool {
    static SEEN: std::sync::LazyLock<parking_lot::Mutex<BTreeMap<PathBuf, (i64, i64)>>> =
        std::sync::LazyLock::new(|| parking_lot::Mutex::new(BTreeMap::new()));
    let stamp = |p: &Path| {
        std::fs::metadata(&p)
            .ok()
            .and_then(|m| m.modified().ok())
            .and_then(|t| t.duration_since(std::time::UNIX_EPOCH).ok())
            // Nanoseconds: a write that lands after our stat has a strictly later mtime on
            // every local filesystem; second granularity could miss one inside the tick.
            .map(|d| d.as_nanos() as i64)
            .unwrap_or(-1)
    };
    let current = (stamp(path), stamp(&PathBuf::from(format!("{}-wal", path.display()))));
    let mut seen = SEEN.lock();
    if seen.get(path) == Some(&current) {
        return false;
    }
    seen.insert(path.to_path_buf(), current);
    true
}

/// Pick the token triple out of raw `(name, value, domain)` cookie rows. Domain-matched by
/// hand: anything outside soundcloud.com is dropped even if a store somehow returned it.
pub fn harvest(cookies: Vec<(String, String, String)>) -> Option<SoundcloudAuth> {
    let mut jar = BTreeMap::new();
    for (name, value, domain) in cookies {
        let domain = domain.strip_prefix('.').map(str::to_owned).unwrap_or(domain);
        if domain != "soundcloud.com" && !domain.ends_with(".soundcloud.com") {
            continue;
        }
        if COOKIE_NAMES.contains(&name.as_str()) {
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

// --- discovery --------------------------------------------------------------------------------

/// Every plausible browser config root: `$XDG_CONFIG_HOME`, `~/.mozilla`, and each Flatpak app's
/// private config/home (which is where sandboxed Firefox and Zen actually keep their profiles).
fn candidate_roots() -> Vec<PathBuf> {
    let home = home_dir();
    let config = std::env::var_os("XDG_CONFIG_HOME")
        .filter(|p| Path::new(p).is_absolute())
        .map(PathBuf::from)
        .unwrap_or_else(|| home.join(".config"));
    let mut roots = vec![config, home.join(".mozilla")];
    if let Ok(entries) = std::fs::read_dir(home.join(".var/app")) {
        for entry in entries.flatten() {
            let app = entry.path();
            roots.push(app.join("config"));
            roots.push(app.join(".config"));
            roots.push(app.join(".mozilla"));
        }
    }
    if let Ok(entries) = std::fs::read_dir(home.join("snap")) {
        for entry in entries.flatten() {
            roots.push(entry.path().join("common/.mozilla"));
            roots.push(entry.path().join("common/.config"));
        }
    }
    roots
}

/// All cookie databases that could hold a SoundCloud session, Gecko stores first (their values
/// are readable without any key material). Bounded so a wild home directory cannot blow up a
/// poll tick.
pub fn discover() -> Vec<Store> {
    let mut gecko = Vec::new();
    let mut chromium = Vec::new();
    for root in candidate_roots() {
        find_named(&root, "cookies.sqlite", 4, &mut gecko);
        // Chromium-family profiles keep Cookies under the profile root or its Network/ dir.
        find_named(&root, "Cookies", 5, &mut chromium);
    }
    chromium.retain(|path| {
        // A file named "Cookies" is only interesting below a browser-ish profile directory;
        // this also drops the Network/ duplicates' parent noise.
        path.components().any(|c| {
            matches!(
                c.as_os_str().to_str(),
                Some("Default") | Some("Network") | Some("chromium") | Some("google-chrome")
            )
        })
    });
    let mut stores: Vec<Store> = gecko
        .into_iter()
        .map(|path| Store { kind: Kind::Gecko, path })
        .chain(chromium.into_iter().map(|path| Store { kind: Kind::Chromium, path }))
        .collect();
    stores.sort_by(|a, b| a.path.cmp(&b.path));
    stores.truncate(64);
    prefer_default_browser(&mut stores);
    stores
}

/// The user's default browser is the most likely place they just signed in; try it first.
fn prefer_default_browser(stores: &mut [Store]) {
    let Ok(output) =
        std::process::Command::new("xdg-settings").args(["get", "default-web-browser"]).output()
    else {
        return;
    };
    if !output.status.success() {
        return;
    }
    let desktop = String::from_utf8_lossy(&output.stdout).trim().to_owned();
    // "zen.desktop" / "firefox.desktop" / "chromium.desktop" … match path fragments.
    let name = desktop.trim_end_matches(".desktop").to_lowercase();
    if name.is_empty() {
        return;
    }
    stores.sort_by_key(|store| {
        let lower = store.path.to_string_lossy().to_lowercase();
        if lower.contains(&name) {
            0
        } else {
            1
        }
    });
}

fn find_named(root: &Path, filename: &str, max_depth: usize, out: &mut Vec<PathBuf>) {
    if max_depth == 0 {
        return;
    }
    let Ok(entries) = std::fs::read_dir(root) else { return };
    for entry in entries.flatten() {
        let path = entry.path();
        let Ok(file_type) = entry.file_type() else { continue };
        if file_type.is_file() {
            if path.file_name().is_some_and(|n| n == filename) {
                out.push(path);
            }
        } else if file_type.is_dir() && !path_is_symlink(&path) {
            find_named(&path, filename, max_depth - 1, out);
        }
    }
}

fn path_is_symlink(path: &Path) -> bool {
    std::fs::symlink_metadata(path).map(|m| m.file_type().is_symlink()).unwrap_or(false)
}

fn home_dir() -> PathBuf {
    std::env::var_os("HOME").map(PathBuf::from).unwrap_or_else(|| PathBuf::from("/"))
}

// --- readers ----------------------------------------------------------------------------------

/// Copy a (possibly locked, WAL-backed) SQLite database beside its journal files and open the
/// copy read-only, so a running browser never blocks or corrupts the import.
fn copy_and_open(path: &Path) -> Option<(tempfile::TempDir, Connection)> {
    let dir = tempfile::Builder::new().prefix("ryotunes-cookies-").tempdir().ok()?;
    let target = dir.path().join(path.file_name()?);
    std::fs::copy(path, &target).ok()?;
    for suffix in ["-wal", "-shm", "-journal"] {
        let side = PathBuf::from(format!("{}{suffix}", path.to_string_lossy()));
        if side.exists() {
            let _ = std::fs::copy(&side, dir.path().join(side.file_name()?));
        }
    }
    let connection = Connection::open_with_flags(
        &target,
        OpenFlags::SQLITE_OPEN_READ_ONLY | OpenFlags::SQLITE_OPEN_NO_MUTEX,
    )
    .ok()?;
    Some((dir, connection))
}

fn read_gecko(path: &Path) -> Option<Vec<(String, String, String)>> {
    let (_dir, connection) = copy_and_open(path)?;
    let mut statement = connection
        .prepare(
            "SELECT name, value, host FROM moz_cookies \
             WHERE host LIKE '%soundcloud.com' AND name IN (?1, ?2, ?3)",
        )
        .ok()?;
    let rows = statement
        .query_map(rusqlite::params![COOKIE_NAMES[0], COOKIE_NAMES[1], COOKIE_NAMES[2]], |row| {
            Ok((row.get::<_, String>(0)?, row.get::<_, String>(1)?, row.get::<_, String>(2)?))
        })
        .ok()?;
    Some(rows.flatten().collect())
}

fn read_chromium(path: &Path) -> Option<Vec<(String, String, String)>> {
    let (_dir, connection) = copy_and_open(path)?;
    let mut statement = connection
        .prepare(
            "SELECT name, host_key, encrypted_value, value FROM cookies \
             WHERE host_key LIKE '%soundcloud.com' AND name IN (?1, ?2, ?3)",
        )
        .ok()?;
    let rows = statement
        .query_map(rusqlite::params![COOKIE_NAMES[0], COOKIE_NAMES[1], COOKIE_NAMES[2]], |row| {
            Ok((
                row.get::<_, String>(0)?,
                row.get::<_, String>(1)?,
                row.get::<_, Vec<u8>>(2)?,
                row.get::<_, Vec<u8>>(3)?,
            ))
        })
        .ok()?;
    let mut out = Vec::new();
    for row in rows.flatten() {
        let (name, host, encrypted, plain) = row;
        if let Some(value) = chromium_value(&encrypted, &plain, &host) {
            out.push((name, value, host));
        }
    }
    Some(out)
}

/// Decode a Chromium cookie value. Linux Chromium stores `v10` values as AES-128-CBC under a
/// key derived from the public passphrase "peanuts" (the whole point is per-user file
/// permissions, not secrecy from the user's own processes); some builds keep plaintext in
/// `value`. `v11` (a keyring-held key) is not attemptable here and is skipped.
fn chromium_value(encrypted: &[u8], plain: &[u8], host: &str) -> Option<String> {
    if let Some(text) = non_utf8_none(plain) {
        return Some(text);
    }
    if encrypted.starts_with(b"v10") {
        let decrypted = chromium_decrypt_v10(&encrypted[3..])?;
        // Some versions prefix the SHA-256 of the cookie host to the plaintext.
        let host_hash = sha256(host.trim_start_matches('.'));
        let body = if decrypted.len() > 32 && decrypted[..32] == host_hash[..] {
            &decrypted[32..]
        } else {
            &decrypted[..]
        };
        return String::from_utf8(body.to_vec()).ok();
    }
    if !encrypted.is_empty() && !encrypted.starts_with(b"v1") {
        return String::from_utf8(encrypted.to_vec()).ok();
    }
    None
}

fn non_utf8_none(bytes: &[u8]) -> Option<String> {
    if bytes.is_empty() {
        return None;
    }
    String::from_utf8(bytes.to_vec()).ok()
}

fn chromium_decrypt_v10(ciphertext: &[u8]) -> Option<Vec<u8>> {
    use aes::Aes128;
    use cbc::cipher::{block_padding::Pkcs7, BlockDecryptMut, KeyIvInit};
    type Aes128CbcDec = cbc::Decryptor<Aes128>;

    let mut key = [0u8; 16];
    pbkdf2::pbkdf2_hmac::<sha1::Sha1>(b"peanuts", b"saltysalt", 1, &mut key);
    let decryptor = Aes128CbcDec::new(&key.into(), &[0x20; 16].into());
    decryptor.decrypt_padded_vec_mut::<Pkcs7>(ciphertext).ok()
}

fn sha256(input: &str) -> [u8; 32] {
    use sha2::Digest;
    sha2::Sha256::digest(input).into()
}

#[cfg(test)]
mod tests {
    use super::*;

    fn fake_jwt(expiry: i64) -> String {
        let header = base64::Engine::encode(
            &base64::engine::general_purpose::URL_SAFE_NO_PAD,
            br#"{"alg":"RS256"}"#,
        );
        let payload = base64::Engine::encode(
            &base64::engine::general_purpose::URL_SAFE_NO_PAD,
            format!(r#"{{"exp":{expiry},"client_id":"cid"}}"#).as_bytes(),
        );
        format!("{header}.{payload}.signature")
    }

    #[test]
    fn harvest_takes_the_soundcloud_triple_and_ignores_the_rest() {
        let expiry =
            std::time::SystemTime::now().duration_since(std::time::UNIX_EPOCH).unwrap().as_secs()
                as i64
                + 3600;
        let cookies = vec![
            ("oauth_token".into(), fake_jwt(expiry), ".soundcloud.com".into()),
            ("oauth_refresh_token".into(), "rot-1".into(), ".soundcloud.com".into()),
            ("connect_session".into(), "cs-1".into(), ".soundcloud.com".into()),
            ("spam".into(), "no".into(), ".evil.com".into()),
            ("oauth_token".into(), "wrong-domain".into(), ".notsoundcloud.com".into()),
        ];
        let auth = harvest(cookies).expect("a complete triple harvests");
        assert_eq!(auth.refresh_token.as_deref(), Some("rot-1"));
        assert_eq!(auth.connect_session.as_deref(), Some("cs-1"));
        assert_eq!(auth.client_id, "cid");
    }

    #[test]
    fn harvest_needs_the_access_token() {
        assert!(harvest(vec![("connect_session".into(), "cs".into(), ".soundcloud.com".into())])
            .is_none());
    }

    fn seed_gecko(root: &Path, profile: &str, rows: &[(&str, &str, &str)]) -> PathBuf {
        let dir = root.join(profile);
        std::fs::create_dir_all(&dir).unwrap();
        let path = dir.join("cookies.sqlite");
        let connection = Connection::open(&path).unwrap();
        connection
            .execute("CREATE TABLE moz_cookies (host TEXT, name TEXT, value TEXT)", [])
            .unwrap();
        for (host, name, value) in rows {
            connection
                .execute(
                    "INSERT INTO moz_cookies VALUES (?1, ?2, ?3)",
                    rusqlite::params![host, name, value],
                )
                .unwrap();
        }
        path
    }

    #[test]
    fn gecko_store_imports_over_a_locked_live_database() {
        let temp = tempfile::tempdir().unwrap();
        let expiry =
            std::time::SystemTime::now().duration_since(std::time::UNIX_EPOCH).unwrap().as_secs()
                as i64
                + 3600;
        let path = seed_gecko(
            temp.path(),
            "abc123.Default (release)",
            &[
                (".soundcloud.com", "oauth_token", &fake_jwt(expiry)),
                (".soundcloud.com", "oauth_refresh_token", "rot"),
                (".soundcloud.com", "connect_session", "cs"),
                (".example.com", "oauth_token", "decoy"),
            ],
        );
        // Hold the source open (like a running browser) to prove the copy path works anyway.
        let _live = Connection::open(&path).unwrap();
        let stores = vec![Store { kind: Kind::Gecko, path }];
        let auth = import_from(&stores).expect("the triple imports");
        assert_eq!(auth.refresh_token.as_deref(), Some("rot"));
    }

    #[test]
    fn chromium_v10_values_decrypt_with_the_public_linux_key() {
        use aes::Aes128;
        use cbc::cipher::{block_padding::Pkcs7, BlockEncryptMut, KeyIvInit};
        type Aes128CbcEnc = cbc::Encryptor<Aes128>;

        let mut key = [0u8; 16];
        pbkdf2::pbkdf2_hmac::<sha1::Sha1>(b"peanuts", b"saltysalt", 1, &mut key);
        let encrypt = |plain: &[u8]| -> Vec<u8> {
            Aes128CbcEnc::new(&key.into(), &[0x20; 16].into())
                .encrypt_padded_vec_mut::<Pkcs7>(plain)
        };

        let temp = tempfile::tempdir().unwrap();
        let dir = temp.path().join("chromium/Default");
        std::fs::create_dir_all(&dir).unwrap();
        let path = dir.join("Cookies");
        let connection = Connection::open(&path).unwrap();
        connection
            .execute(
                "CREATE TABLE cookies (host_key TEXT, name TEXT, encrypted_value BLOB, value BLOB)",
                [],
            )
            .unwrap();
        let expiry =
            std::time::SystemTime::now().duration_since(std::time::UNIX_EPOCH).unwrap().as_secs()
                as i64
                + 3600;
        let jwt = fake_jwt(expiry);
        connection
            .execute(
                "INSERT INTO cookies VALUES ('.soundcloud.com', 'oauth_token', ?1, x'')",
                rusqlite::params![format!("v10")
                    .into_bytes()
                    .into_iter()
                    .chain(encrypt(jwt.as_bytes()))
                    .collect::<Vec<u8>>()],
            )
            .unwrap();
        connection
            .execute(
                "INSERT INTO cookies VALUES ('.soundcloud.com', 'oauth_refresh_token', ?1, x'')",
                rusqlite::params!["v10"
                    .as_bytes()
                    .iter()
                    .cloned()
                    .chain(encrypt(b"rot-c"))
                    .collect::<Vec<u8>>()],
            )
            .unwrap();
        connection
            .execute(
                "INSERT INTO cookies VALUES ('.soundcloud.com', 'connect_session', ?1, x'')",
                rusqlite::params!["v10"
                    .as_bytes()
                    .iter()
                    .cloned()
                    .chain(encrypt(b"cs-c"))
                    .collect::<Vec<u8>>()],
            )
            .unwrap();
        drop(connection);

        let stores = vec![Store { kind: Kind::Chromium, path }];
        let auth = import_from(&stores).expect("v10 cookies import");
        assert_eq!(auth.refresh_token.as_deref(), Some("rot-c"));
        assert_eq!(auth.connect_session.as_deref(), Some("cs-c"));
    }

    #[test]
    fn v11_keyring_cookies_are_skipped_not_fatal() {
        assert_eq!(chromium_value(b"v11garbage", b"", ".soundcloud.com"), None);
    }

    #[test]
    fn discovery_finds_gecko_and_chromium_layouts_under_a_fake_home() {
        let temp = tempfile::tempdir().unwrap();
        let expiry =
            std::time::SystemTime::now().duration_since(std::time::UNIX_EPOCH).unwrap().as_secs()
                as i64
                + 3600;
        seed_gecko(
            &temp.path().join("zen"),
            "xyz.Default (release)",
            &[(".soundcloud.com", "oauth_token", &fake_jwt(expiry))],
        );
        let chrome_dir = temp.path().join("chromium/Default/Network");
        std::fs::create_dir_all(&chrome_dir).unwrap();
        std::fs::write(chrome_dir.join("Cookies"), b"not-a-db").unwrap();

        let mut gecko = Vec::new();
        let mut chromium = Vec::new();
        find_named(temp.path(), "cookies.sqlite", 4, &mut gecko);
        find_named(temp.path(), "Cookies", 5, &mut chromium);
        assert_eq!(gecko.len(), 1, "the zen profile db is found");
        assert!(gecko[0].to_string_lossy().contains("zen"));
        assert_eq!(chromium.len(), 1, "the chromium Network/Cookies is found");
    }
}
