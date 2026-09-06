# Ryotunes skins

A **skin** is one file, `skin.json`, that names every colour, type face, radius and duration the
Ryotunes chrome reads. You pick one in Settings (or let it follow the desktop); editing its file
re-paints the running app live. This is the format reference. The machine-checkable version is
[`skins/skin.schema.json`](../skins/skin.schema.json); validate a skin with
`ryotunes-cli skin check <dir>`.

## What a skin controls — and what it does not

A skin sets:

- **Colour** — a `dark` and/or `light` palette of eight roles (below), applied as a Material scheme
  on `Tokens` so every surface re-renders at once.
- **Type** — the display, UI and mono font families.
- **Shape** — corner radii.
- **Motion** — the four base durations (they still pass Ryoku's reduce-motion gate).
- **Decor** — `rich` (full grain/texture) or `calm` (flatter, quieter).
- **Accent** — where the highlight colour comes from: the artwork, the provider brand, the skin's
  own `sun`, or a fixed hex.
- **Wash** — how strongly the playing cover tints the pages and the stage.

A skin **cannot**:

- Change the **layout** — the sidebar, the grid, where the player bar sits. Layout is not skinnable.
- Add new colours. **Lines, dividers and tints are alpha of `ink`** by design; there is no separate
  "border colour" to set. Tune `ink` and they follow.
- Restyle individual widgets or override per-page anything. A skin is a palette and a few globals,
  not a stylesheet.

## The manifest, key by key

Everything is optional **except** `format` and a `modes` object with at least one mode carrying
`paper` and `ink`. Any missing key falls back to the **Paper** skin's value (shown as the default).

| Key | Type | Default | Notes |
| --- | --- | --- | --- |
| `format` | `1` | — (required) | Manifest version. Always `1`. |
| `id` | string | — (required) | Lowercase kebab-case, **equal to the folder name**. |
| `name` | string | the `id` | Shown in Settings. |
| `author` | string | `""` | |
| `version` | string | `""` | |
| `license` | string | `""` | SPDX id; use `CC0-1.0` or `MIT` to ship. |
| `homepage` | string | `""` | |
| `description` | string | `""` | One line; shown in Settings. |
| `default` | `"dark"` \| `"light"` | `"dark"` | Which mode **Light/Dark = System** pins to. |
| `modes.dark` / `modes.light` | palette | Paper's | See below; at least one needs `paper`+`ink`. |
| `type.display` | string | `"Fraunces"` | Titles and headings. |
| `type.ui` | string | `"Space Grotesk"` | Body and controls. |
| `type.mono` | string | `"SpaceMono Nerd Font"` | Numbers, timers. |
| `shape.radius` | px | `8` | Buttons, fields, tiles. |
| `shape.radiusCard` | px | `12` | Cards, sheets. |
| `motion.snap` | ms | `90` | Presses, toggles. |
| `motion.move` | ms | `170` | Slides, reorders. |
| `motion.swap` | ms | `210` | Page/tab swaps. |
| `motion.slow` | ms | `200` | Fades, ambient. |
| `decor` | `"rich"` \| `"calm"` | `"rich"` | Texture level. |
| `accent` | `"artwork"` \| `"provider"` \| `"sun"` \| `#rrggbb` | `"artwork"` | Highlight source. |
| `wash` | `0`..`2` | `1.0` | Cover-wash multiplier; `0` keeps paper flat. |
| `fonts` | `["fonts/Foo.ttf"]` | `[]` | Bundled faces, relative to the folder, `FontLoader`-loaded. |
| `generated` | string | unset | Set by a generator (e.g. `matugen`); the UI marks it *regenerated, fork to edit*. |

### The eight colour roles

Each mode is a palette of `#rrggbb` colours. `paper`+`ink` are the minimum; the rest fall back to
Paper's per-mode values.

| Role | Is | Paper `dark` | Paper `light` |
| --- | --- | --- | --- |
| `paper` | the base surface | `#050505` | `#c8c4bc` |
| `paperLift` | a slightly lifted surface | `#0d0d0c` | `#d5d0c7` |
| `ink` | primary text on paper | `#d7cfc6` | `#211f1c` |
| `inkDim` | secondary text on paper | `#b6aea5` | `#403b35` |
| `bone` | the inverse surface | `#d7cfc6` | `#292620` |
| `inkOnBone` | text on bone | `#090807` | `#eee8de` |
| `sun` | the primary / accent | `#e2342a` | `#e2342a` |
| `alert` | error / destructive | `#d33b32` | `#d33b32` |

`ryotunes-cli skin check` enforces WCAG contrast on the text pairs:

| Pair | Threshold | Severity |
| --- | --- | --- |
| `ink` / `paper` | ≥ 4.5 | **error** |
| `inkOnBone` / `bone` | ≥ 4.5 | **error** |
| `inkDim` / `paper` | ≥ 3.0 | warn |
| `sun` / `paper` | ≥ 3.0 | warn |

## Modes and the Light/Dark pin

A skin may ship both `dark` and `light`, or just one. In Settings, **Light / Dark / System**:

- **System** uses the skin's `default` mode (or, for the `system` skin, whatever the desktop is).
- **Light** / **Dark** pin that mode. If the skin lacks it, its other mode is used and Settings says
  so (`Skin.modeMissing`).

## Resolution order and paths

`Prefs.skin` is either `"system"` or a skin **id**. An id is looked up in these directories, and the
**first hit per id wins**:

1. `$RYOTUNES_SKIN_DIRS` — colon-separated, dev/preview only.
2. `~/.config/ryotunes/skins/<id>/skin.json` — **user** skins; the `matugen` skin lives here.
3. `<shellDir>/../skins/<id>/skin.json` — **shipped**: `/usr/share/ryotunes/skins` when installed,
   `./skins` in a checkout.

So a user skin shadows a shipped one of the same id. `RYOTUNES_SKIN=<id>` pins a skin for one run
(previews use this).

```
/usr/share/ryotunes/skins/<id>/     shipped skins (skin.json, preview.png)
/usr/share/ryotunes/matugen/ryotunes.json   the matugen template
~/.config/ryotunes/skins/<id>/      user skins
~/.config/ryotunes/skins/matugen/   the matugen-generated skin
~/.config/ryotunes/client.json      prefs (skin, decor, themeMode, mini position)
```

## Live reload

The **active** skin's `skin.json` is watched. Save it in your editor and the app re-paints in about
a second — no restart, no relaunch. This is how skins are authored: open the file, tweak a colour,
watch it land.

## The `system` skin

`system` is the default on a Ryoku desktop. It **follows the desktop theme** through `Tokens`:

- **Follows:** the colour scheme (the wallpaper palette or a named scheme), plus `decor` and
  `motion` from the shell.
- **Does not follow:** **type**. The desktop's bar font is often a mono Nerd font and would put the
  whole app in it, so `system` keeps Paper's type. It also does not follow `accent`/`wash` policy —
  those stay Paper's.

`system` has no file of its own; Paper's `modes` are what **Light/Dark** pin to while following the
desktop.

## matugen

The `matugen` skin is just a skin whose file matugen writes from your wallpaper. The template shipped
at `/usr/share/ryotunes/matugen/ryotunes.json` maps Material 3 roles to the eight skin colours in
both modes, e.g. `"paper": "{{colors.surface.dark.hex}}"`, `"sun": "{{colors.primary.dark.hex}}"`.

Add this to `~/.config/matugen/config.toml`:

```toml
[templates.ryotunes]
input_path = "/usr/share/ryotunes/matugen/ryotunes.json"
output_path = "~/.config/ryotunes/skins/matugen/skin.json"
```

Then render it by hand from any image:

```sh
matugen -t scheme-fidelity --prefer saturation image ~/Pictures/wallpaper.png
```

That writes `~/.config/ryotunes/skins/matugen/skin.json`; select **Matugen** in Settings (it is
marked *regenerated — fork to edit*, since the next palette change overwrites it). On **Ryoku**, the
Hub's **Theme apps** toggle does all of this automatically, fanning the wallpaper palette into
Ryotunes on every change.

## Authoring a skin

1. **Start from something.** In Settings, **Fork** the current skin (it writes
   `~/.config/ryotunes/skins/<id>/skin.json` from the painted palette and opens it), or copy
   `skins/paper/` to `skins/<id>/` and rename `id` to `<id>`.
2. **Edit live.** Change colours, type, `shape`, `motion`, `decor`, `accent`, `wash`. The app
   re-paints on save. Keep `id` equal to the folder name.
3. **Check it.** `ryotunes-cli skin check skins/<id>` — fix every **error** (contrast, hex, ranges);
   address warnings where you can. Errors block a PR.
4. **Do both modes.** If you ship `dark` and `light`, tune each; the validator checks contrast per
   mode. Pin Light and Dark in Settings and look at both.
5. **Capture a preview.** `scripts/dev/skin-preview.sh <id>` renders the skin in the test rig and
   writes `skins/<id>/preview.png` (800×500).

## Submitting a skin

Open a PR that adds `skins/<id>/`. Checklist:

- [ ] `skin.json` with `"$schema": "../skin.schema.json"`, `format: 1`, `id` = the folder name.
- [ ] `ryotunes-cli skin check skins/<id>` passes with **no errors**.
- [ ] `license` is `CC0-1.0` or `MIT`.
- [ ] `preview.png`, 800×500, produced by `scripts/dev/skin-preview.sh`.
- [ ] Any bundled `fonts/` are freely redistributable — **no proprietary fonts**. Prefer naming a
      common family in `type` over bundling.
- [ ] Add a row to the gallery in [`skins/README.md`](../skins/README.md).
