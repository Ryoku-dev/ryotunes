//! Explicit, package-managed updates shared by both desktop hosts. GitHub is the release
//! authority; the renderer can select a version, never a URL, command or destination.
use std::path::Path;
use std::process::Stdio;
use std::time::Duration;

use serde::{Deserialize, Serialize};
use sha2::{Digest, Sha256};
use tokio::io::AsyncWriteExt;
use tokio::process::Command;

const REPOSITORY: &str = "https://github.com/ryoku-dev/ryotunes";
const LATEST: &str = "https://api.github.com/repos/ryoku-dev/ryotunes/releases/latest";
const MAX_PACKAGE: u64 = 1024 * 1024 * 1024;
static INSTALL: tokio::sync::Mutex<()> = tokio::sync::Mutex::const_new(());

#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct UpdateInfo {
    pub current_version: String,
    pub latest_version: Option<String>,
    pub available: bool,
    pub release_url: String,
    pub notes: String,
    pub can_install: bool,
    pub unsupported_reason: Option<String>,
}

#[derive(Serialize)]
#[serde(rename_all = "camelCase")]
pub struct InstalledUpdate {
    pub version: String,
    pub restart_required: bool,
}

#[derive(Deserialize)]
struct Release {
    tag_name: String,
    draft: bool,
    prerelease: bool,
    body: Option<String>,
    assets: Vec<Asset>,
}

#[derive(Deserialize)]
struct Asset {
    name: String,
    size: u64,
}

// Canonical v1 versions only: no traversal, prereleases, leading zeroes or legacy v2 tags.
fn version(raw: &str) -> Option<(u64, u64, u64)> {
    let mut parts = raw.split('.');
    let (major, minor, patch) = (parts.next()?, parts.next()?, parts.next()?);
    if parts.next().is_some() || major != "1" {
        return None;
    }
    if (minor.len() > 1 && minor.starts_with('0'))
        || !minor.bytes().all(|byte| byte.is_ascii_digit())
        || patch.len() != 1
        || !patch.as_bytes()[0].is_ascii_digit()
    {
        return None;
    }
    Some((1, minor.parse::<u64>().ok()?, (patch.as_bytes()[0] - b'0') as u64))
}

fn package_name(version: &str) -> String {
    format!("ryotunes-{version}-1-x86_64.pkg.tar.zst")
}

fn release_info(release: Option<Release>, current: &str, reason: Option<String>) -> UpdateInfo {
    let mut info = UpdateInfo {
        current_version: current.into(),
        latest_version: None,
        available: false,
        release_url: format!("{REPOSITORY}/releases"),
        notes: String::new(),
        can_install: false,
        unsupported_reason: reason,
    };
    let Some(release) = release else { return info };
    let Some(latest) = release.tag_name.strip_prefix('v') else { return info };
    let Some(next) = version(latest) else { return info };
    if release.draft || release.prerelease {
        return info;
    }
    info.latest_version = Some(latest.into());
    // v1 is a deliberate new release series, not a numeric downgrade of the legacy v2 line.
    info.available = version(current).is_none_or(|installed| next > installed);
    info.release_url = format!("{REPOSITORY}/releases/tag/v{latest}");
    info.notes = release.body.unwrap_or_default();
    let name = package_name(latest);
    let checksum = format!("{name}.sha256");
    let has_package =
        release.assets.iter().any(|a| a.name == name && a.size > 0 && a.size <= MAX_PACKAGE);
    let has_checksum =
        release.assets.iter().any(|a| a.name == checksum && a.size > 0 && a.size <= 4096);
    if !has_package || !has_checksum {
        info.unsupported_reason = Some("This release has no complete x86_64 Arch package and checksum. View the changelog or check again after the release build finishes.".into());
    }
    info.can_install = info.unsupported_reason.is_none() && has_package && has_checksum;
    info
}

async fn response(url: &str, timeout: Duration) -> Result<reqwest::Response, String> {
    crate::http::client()
        .get(url)
        .header(reqwest::header::USER_AGENT, concat!("Ryotunes/", env!("CARGO_PKG_VERSION")))
        .header(reqwest::header::ACCEPT, "application/vnd.github+json")
        .timeout(timeout)
        .send()
        .await
        .map_err(|e| format!("Could not contact GitHub: {e}"))
}

async fn bounded_body(mut response: reqwest::Response, limit: usize) -> Result<Vec<u8>, String> {
    response = response.error_for_status().map_err(|e| format!("GitHub request failed: {e}"))?;
    let mut bytes = Vec::new();
    while let Some(chunk) = response.chunk().await.map_err(|e| e.to_string())? {
        if chunk.len() > limit.saturating_sub(bytes.len()) {
            return Err("GitHub response exceeded the allowed size.".into());
        }
        bytes.extend_from_slice(&chunk);
    }
    Ok(bytes)
}

async fn installation_reason() -> Option<String> {
    if !cfg!(all(target_os = "linux", target_arch = "x86_64")) {
        return Some("Self update supports the x86_64 Arch/Ryoku package. Download a release for your platform instead.".into());
    }
    if !Path::new("/usr/bin/pacman").is_file() || !Path::new("/usr/bin/pkexec").is_file() {
        return Some(
            "Self update needs pacman and polkit (pkexec), with a desktop authentication agent."
                .into(),
        );
    }
    let exe = match std::env::current_exe() {
        Ok(exe) => exe,
        Err(e) => return Some(format!("Cannot determine the running installation: {e}")),
    };
    let owner = Command::new("/usr/bin/pacman").args(["-Qoq"]).arg(exe).kill_on_drop(true).output();
    match tokio::time::timeout(Duration::from_secs(10), owner).await {
        Ok(Ok(output)) if output.status.success() && output.stdout == b"ryotunes\n" => None,
        _ => Some("This is not a pacman-managed Ryotunes installation. Install the release package first; development builds are never overwritten.".into()),
    }
}

pub async fn check_for_updates() -> Result<UpdateInfo, String> {
    let response = response(LATEST, Duration::from_secs(30)).await?;
    let release = if response.status() == reqwest::StatusCode::NOT_FOUND {
        None
    } else {
        let body = bounded_body(response, 1024 * 1024).await?;
        Some(
            serde_json::from_slice::<Release>(&body)
                .map_err(|e| format!("Invalid GitHub release metadata: {e}"))?,
        )
    };
    Ok(release_info(release, env!("CARGO_PKG_VERSION"), installation_reason().await))
}

fn checksum(bytes: &[u8], name: &str) -> Result<String, String> {
    let text = std::str::from_utf8(bytes).map_err(|_| "Invalid package checksum encoding.")?;
    let mut fields = text.split_whitespace();
    let hash = fields.next().unwrap_or_default();
    if fields.next() != Some(name)
        || fields.next().is_some()
        || hash.len() != 64
        || !hash.bytes().all(|byte| byte.is_ascii_hexdigit())
    {
        return Err(
            "Invalid package checksum: expected one SHA-256 entry for this release package.".into(),
        );
    }
    Ok(hash.to_ascii_lowercase())
}

async fn download(url: &str, path: &Path, expected: &str) -> Result<(), String> {
    let mut response = response(url, Duration::from_secs(15 * 60))
        .await?
        .error_for_status()
        .map_err(|e| format!("Package download failed: {e}"))?;
    if response.content_length().is_some_and(|length| length > MAX_PACKAGE) {
        return Err("Release package is too large.".into());
    }
    let mut file = tokio::fs::OpenOptions::new()
        .write(true)
        .create_new(true)
        .open(path)
        .await
        .map_err(|e| format!("Could not stage update: {e}"))?;
    let mut size = 0u64;
    let mut hash = Sha256::new();
    while let Some(chunk) =
        response.chunk().await.map_err(|e| format!("Package download interrupted: {e}"))?
    {
        size += chunk.len() as u64;
        if size > MAX_PACKAGE {
            return Err("Release package is too large.".into());
        }
        hash.update(&chunk);
        file.write_all(&chunk).await.map_err(|e| format!("Could not write update: {e}"))?;
    }
    if size == 0 || format!("{:x}", hash.finalize()) != expected {
        return Err("Package SHA-256 verification failed. Nothing was installed.".into());
    }
    file.sync_all().await.map_err(|e| format!("Could not save update: {e}"))?;
    Ok(())
}

pub async fn install_update(requested: String) -> Result<InstalledUpdate, String> {
    if version(&requested).is_none() {
        return Err("Select a valid v1 release.".into());
    }
    let _guard = INSTALL.try_lock().map_err(|_| "An update is already in progress.")?;
    let info = check_for_updates().await?;
    if info.latest_version.as_deref() != Some(&requested) {
        return Err(
            "The latest release changed. Check for new versions again before updating.".into()
        );
    }
    if !info.available {
        return Err("This version is already running, or a newer version is installed.".into());
    }
    if !info.can_install {
        return Err(info
            .unsupported_reason
            .unwrap_or_else(|| "Self update is not available for this installation.".into()));
    }
    // Ryoku may already have upgraded the files while this older daemon was still running.
    let installed = tokio::time::timeout(
        Duration::from_secs(10),
        Command::new("/usr/bin/pacman").args(["-Q", "ryotunes"]).kill_on_drop(true).output(),
    )
    .await
    .map_err(|_| "Timed out reading the installed package.")?
    .map_err(|e| format!("Could not query the installed package: {e}"))?;
    if !installed.status.success() {
        return Err("Could not query the installed Ryotunes package.".into());
    }
    let installed_text = String::from_utf8_lossy(&installed.stdout);
    if let Some(installed_version) = installed_text
        .trim()
        .strip_prefix("ryotunes 1:")
        .and_then(|value| value.rsplit_once('-').map(|(v, _)| v))
    {
        if version(installed_version) > version(&requested) {
            return Err(
                "A newer package is already installed. Quit Ryotunes and open it again.".into()
            );
        }
        if installed_version == requested {
            return Ok(InstalledUpdate { version: requested, restart_required: true });
        }
    }
    // Private random staging, removed on every success/error. No user-controlled path reaches root.
    let staging = tempfile::Builder::new()
        .prefix("ryotunes-update-")
        .tempdir()
        .map_err(|e| format!("Could not create private update staging: {e}"))?;
    let name = package_name(&requested);
    let url = format!("{REPOSITORY}/releases/download/v{requested}/{name}");
    let expected = checksum(
        &bounded_body(response(&format!("{url}.sha256"), Duration::from_secs(30)).await?, 4096)
            .await?,
        &name,
    )?;
    let path = staging.path().join(&name);
    download(&url, &path, &expected).await?;

    // Verify the package's internal identity, not just its download filename, before privilege.
    let metadata =
        Command::new("/usr/bin/pacman").arg("-Qp").arg(&path).kill_on_drop(true).output();
    let output = tokio::time::timeout(Duration::from_secs(30), metadata)
        .await
        .map_err(|_| "Timed out reading the package identity.")?
        .map_err(|e| format!("Could not inspect the package: {e}"))?;
    let expected_identity = format!("ryotunes 1:{requested}-1\n");
    if !output.status.success() || output.stdout != expected_identity.as_bytes() {
        return Err(
            "Downloaded package identity does not match the selected Ryotunes release.".into()
        );
    }
    // Explicit Update click + polkit approval authorizes just this package transaction. Retain
    // pacman's signature/dependency policy; never use --overwrite, --nodeps or bypass signatures.
    // Do not kill pacman on a timeout: interrupting a committed transaction can damage the install.
    let output = Command::new("/usr/bin/pkexec")
        .arg("/usr/bin/pacman")
        .args(["-U", "--noconfirm"])
        .arg(&path)
        .stdin(Stdio::null())
        .output()
        .await
        .map_err(|e| format!("Could not start the administrator approval: {e}"))?;
    if !output.status.success() {
        let detail = String::from_utf8_lossy(&output.stderr);
        return Err(format!(
            "Update was cancelled or pacman could not install the package ({}). {}",
            output.status,
            detail.trim()
        ));
    }
    Ok(InstalledUpdate { version: requested, restart_required: true })
}

#[cfg(test)]
mod tests {
    use super::*;

    fn release(tag: &str) -> Release {
        let name = package_name(tag.trim_start_matches('v'));
        Release {
            tag_name: tag.into(),
            draft: false,
            prerelease: false,
            body: Some("Changes".into()),
            assets: vec![
                Asset { name: name.clone(), size: 100 },
                Asset { name: format!("{name}.sha256"), size: 120 },
            ],
        }
    }

    #[test]
    fn release_series_and_numeric_order() {
        assert!(release_info(Some(release("v1.1.0")), "1.0.9", None).available);
        assert!(!release_info(Some(release("v1.0.9")), "1.1.0", None).available);
        assert!(!release_info(Some(release("v1.1.0")), "1.1.0", None).available);
        assert!(release_info(Some(release("v1.0.0")), "2.5.1", None).available);
        assert!(release_info(Some(release("v2.5.1")), "1.0.0", None).latest_version.is_none());
        assert!(version("1.0.10").is_none());
        assert!(version("1.01.0").is_none());
        assert!(version("1.0.0/../../bad").is_none());
    }

    #[test]
    fn incomplete_or_unpublished_releases_cannot_install() {
        let mut value = release("v1.0.1");
        value.assets.pop();
        let info = release_info(Some(value), "1.0.0", None);
        assert!(info.available && !info.can_install);
        let mut value = release("v1.0.1");
        value.prerelease = true;
        assert!(release_info(Some(value), "1.0.0", None).latest_version.is_none());
        assert!(
            !release_info(Some(release("v1.0.1")), "1.0.0", Some("Unmanaged".into())).can_install
        );
    }

    #[tokio::test]
    async fn downloaded_bytes_must_match_and_never_replace_an_existing_file() {
        use tokio::io::AsyncReadExt;
        let listener = tokio::net::TcpListener::bind("127.0.0.1:0").await.unwrap();
        let url = format!("http://{}/package", listener.local_addr().unwrap());
        let server = tokio::spawn(async move {
            for _ in 0..3 {
                let (mut stream, _) = listener.accept().await.unwrap();
                let mut request = [0u8; 4096];
                stream.read(&mut request).await.unwrap();
                stream
                    .write_all(
                        b"HTTP/1.1 200 OK\r\nContent-Length: 7\r\nConnection: close\r\n\r\npackage",
                    )
                    .await
                    .unwrap();
            }
        });
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("verified");
        let hash = format!("{:x}", Sha256::digest(b"package"));
        download(&url, &path, &hash).await.unwrap();
        assert_eq!(tokio::fs::read(&path).await.unwrap(), b"package");
        assert!(download(&url, &dir.path().join("corrupt"), &"0".repeat(64))
            .await
            .unwrap_err()
            .contains("SHA-256"));
        tokio::fs::write(&path, b"keep").await.unwrap();
        assert!(download(&url, &path, &hash).await.is_err());
        assert_eq!(tokio::fs::read(&path).await.unwrap(), b"keep");
        server.await.unwrap();
    }

    #[test]
    fn checksum_binds_exactly_one_asset() {
        let hash = "a".repeat(64);
        assert_eq!(checksum(format!("{hash}  package\n").as_bytes(), "package").unwrap(), hash);
        assert!(checksum(format!("{hash}  other\n").as_bytes(), "package").is_err());
        assert!(checksum(format!("{hash}  package\n{hash} other").as_bytes(), "package").is_err());
        assert!(checksum(b"not-a-hash package", "package").is_err());
    }
}
