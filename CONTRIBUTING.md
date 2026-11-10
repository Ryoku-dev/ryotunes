# Contributing to Ryotunes

Ryotunes is a Go application with a Gio client and a separate playback daemon. Keep changes within the existing package boundaries described in [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).

## License

Contributions are distributed under GPL-3.0-or-later, the same license as Ryotunes. Keep existing copyright, license and source credits when changing work that came from another project.

## Go conventions

- Run `gofmt` on changed Go files.
- Keep package APIs small and use the standard library where it is enough.
- Return typed provider errors when the daemon or queue must choose a user-visible outcome.
- Keep provider calls outside the daemon state lock.
- Add tests beside the package whose behaviour changes.
- Do not duplicate protocol method names or payload shapes outside `internal/protocol`.
- Do not add a second store, palette resolver or playback owner.

## Product contracts

Every change must preserve these rules:

- Ryotunes is audio only.
- `ryotunesd` owns playback, the queue, providers and integrations.
- Closing a window does not stop playback or active downloads.
- Nothing polls, animates or redraws when its surface is hidden and no live work needs it.
- Quit stops playback, unregisters MPRIS, closes integrations and exits the daemon.
- Package install, update and removal never remove `ryoku-desktop`.

## Lithic UI

Use the [Ryoku Lithic specification](https://github.com/ryoku-dev/ryoku/tree/main/docs/lithic) and the components in `internal/client/lithic`.

- Use semantic roles, not colour values.
- Use 45 degree cuts or square corners, not radii.
- Use bone for selection, gold for the task's consequential action and mineral plus text for state.
- Take type, spacing and motion from the shared Lithic theme.
- Motion must answer an action or state change and then stop.
- Reduced motion makes the same state change immediate.
- Pages do not create their own visual primitives when the kit already provides one.

## Development

```sh
go build ./...
go test ./...
```

For an isolated daemon:

```sh
root=$(mktemp -d)
mkdir -p "$root/run" "$root/data" "$root/cache" "$root/config" "$root/state"
XDG_RUNTIME_DIR="$root/run" \
XDG_DATA_HOME="$root/data" \
XDG_CACHE_HOME="$root/cache" \
XDG_CONFIG_HOME="$root/config" \
XDG_STATE_HOME="$root/state" \
go run ./cmd/ryotunesd
```

## Gate

Before sending a change, run the release gate from the repository root:

```sh
./release/check.sh
```

It checks formatting, vet, tests, the pure Go binaries, version alignment, tracked text, client role usage and the desktop file when its validator is installed.

A pull request should say what changed, which product contract it touches and how it was checked. Include a screenshot for a visible client change.
