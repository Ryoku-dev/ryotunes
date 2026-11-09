# Ryotunes release checklist

Use this checklist for a numbered release from the default branch.

## Prepare the version

1. Choose a semantic version `X.Y.Z`.
2. Synchronize the version files and create the release heading:

   ```sh
   ./release/sync-version.sh X.Y.Z --release
   ```

3. Confirm `VERSION`, `release/arch/PKGBUILD`, `release/aur/PKGBUILD` and the metainfo release entry all use `X.Y.Z`.
4. Move the user-visible changes into `## vX.Y.Z - YYYY-MM-DD` in `CHANGELOG.md`. Leave `## Unreleased` present and empty when there are no pending changes.
5. Confirm the first numbered changelog heading matches `VERSION`.

## Check the tree

Run the release gate from the repository root:

```sh
./release/check.sh
```

The gate covers formatting, vet, tests, pure Go binary builds, version alignment, the no-em-dash rule, client role literals and desktop file validation when available.

## Build the package

Build the exact checkout with the release package script:

```sh
rm -rf dist
./release/build-arch-package.sh dist
```

The output must include:

```text
dist/ryotunes-X.Y.Z-1-x86_64.pkg.tar.zst
dist/ryotunes-X.Y.Z-1-x86_64.pkg.tar.zst.sha256
```

The package filename is epochless. Its metadata must report `ryotunes 1:X.Y.Z-1`.

The package builder runs the glibc ceiling check. Run it explicitly against the final package when reviewing a copied or rebuilt artifact:

```sh
./release/check-glibc.sh dist/ryotunes-X.Y.Z-1-x86_64.pkg.tar.zst 2.41
```

Verify the sidecar and inspect the package contents:

```sh
(
  cd dist
  sha256sum -c ryotunes-X.Y.Z-1-x86_64.pkg.tar.zst.sha256
)
bsdtar -tf dist/ryotunes-X.Y.Z-1-x86_64.pkg.tar.zst
```

The package must contain `ryotunes`, `ryotunesd`, `ryotunes-cli`, `ryotunes-sync`, `ryotunes-diagnostics`, the systemd user units, desktop entry, metainfo, icons, Matugen template, license, README and upstream notice. Its install hooks must not restart the daemon during an update and must never remove `ryoku-desktop`.

## Publish and tag

1. Commit the version, changelog and package metadata together.
2. Push the commit containing the `VERSION` change to `main`.
3. The Release workflow builds the source archive and package, then creates or updates GitHub release tag `vX.Y.Z` at that commit.
4. Confirm tag `vX.Y.Z` points to the intended commit.
5. Confirm the release has the package, package checksum, source archive and source checksum.
6. Confirm the release notes come from the matching changelog section.
7. Install the published package on an Arch or Ryoku system, Quit Ryotunes, reopen it and confirm the client and daemon report the new version.
8. Run `./release/void/install.sh` on Void and confirm the glibc check and xbps upgrade complete.
