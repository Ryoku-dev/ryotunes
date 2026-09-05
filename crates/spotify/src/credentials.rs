//! Where the Spotify provider keeps its on-disk state: the librespot credential cache and the
//! Pathfinder query-hash registry. Both live directly under the provider's data directory.
//!
//! The root is set once, by [`crate::SpotifyProvider::new`], and read process-wide (the Pathfinder
//! hash cache reaches for it deep in the metadata path where threading a handle would be noise).

use std::path::{Path, PathBuf};
use std::sync::OnceLock;

/// The credential file librespot writes after a successful connect.
pub(crate) const FILE: &str = "credentials.json";

static ROOT: OnceLock<PathBuf> = OnceLock::new();

/// Record the provider's data directory. The first caller wins; later calls are ignored so a
/// second provider cannot silently repoint an already-open cache.
pub(crate) fn set_root(dir: PathBuf) {
    let _ = ROOT.set(dir);
}

/// The provider's data directory. Falls back to a temp dir when no provider was constructed
/// (only the offline unit tests hit that path).
pub(crate) fn root() -> PathBuf {
    ROOT.get().cloned().unwrap_or_else(|| std::env::temp_dir().join("ryotunes-spotify"))
}

/// Restricts an existing credential file to its owner. A missing file is left alone.
pub(crate) fn secure(path: &Path) {
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt as _;
        if let Err(error) = std::fs::set_permissions(path, std::fs::Permissions::from_mode(0o600))
            && error.kind() != std::io::ErrorKind::NotFound
        {
            log::warn!("credentials: cannot restrict {}: {error}", path.display());
        }
    }
    #[cfg(not(unix))]
    let _ = path;
}

/// Deletes a credential file, treating one that is already gone as success.
pub(crate) fn remove(path: &Path) {
    if let Err(error) = std::fs::remove_file(path)
        && error.kind() != std::io::ErrorKind::NotFound
    {
        log::warn!("credentials: cannot remove {}: {error}", path.display());
    }
}
