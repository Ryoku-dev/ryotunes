# Eye-candy + spacing pass (2026-09-05)

The user's verdict on the current native client: "looks bad, overlaps, worse than the old
design". The old design is the reference, captured live from the Tauri build on the rig:
`docs/superpowers/refs/tauri-home-2026-09-05.png` (1762x938, window at 1:1). Read it first and
match its proportions; the QML client must look like *that* app, in Ryoku's flat-paper skin.

Two skills inform this pass: Emil Kowalski's design-eng skill (motion, polish) and
tastemaker's spacing system (interface-quality-rules, style-tokens). Distilled rules:

## Spacing system (non-negotiable)

- One scale: `Style.sp(n)` = n x 4 px. Legal steps: 1 2 3 4 6 8 12 16 (4/8/12/16/24/32/48/64 px).
  Never 5, 7, 10, 14, 0.5 (except hairline). Reach for a gap between steps only with a reason.
- **Internal <= external.** The space *around* a group is >= the space *within* it. Card padding
  >= gap between cards is wrong; section gap (32-48) > heading-to-content gap (16) > row gap (8).
- Page inset: `Style.pagePad` (32) on every page edge, 48 on Home's hero column (the pivotal
  surface). Sections separated by `sp(10)` (40 px) of vertical rhythm, heading -> content `sp(4)`.
- Compact rows (nav, queue, track rows): 12-16 px internal padding, row height 48-56; never let
  a title elide to under ~12 characters at the panel's default width - give the title
  `Layout.fillWidth` + priority, and shrink meta columns first.
- Radius scale: `Style.radius` (8) for controls, `Style.radiusCard` (12) for cards, pill for chips.
  No other radii.
- Type roles only from `Style.fs` (micro xs sm md lg xl hero) + the two families
  (`Style.fontDisplay` Fraunces for titles, `Style.fontUi` for everything, `Style.fontMono` for
  meta/eyebrows). Mono eyebrows are `Style.fs.micro`, tracked, uppercase, `Tokens.inkFaint`.

## Motion (Emil)

- Only `opacity` and `x/y/scale` animate. Durations 150-250 ms, `Easing.OutCubic`; exits faster
  than enters. No animation on keyboard-driven actions. No always-on loops except the spectrum
  ribbon while playing. Hover lifts are subtle (`scale 1.0 -> 1.01`, or a paper-lift tint), press
  is `scale 0.97` 160 ms.
- Page enter: 8 px rise + fade, 200 ms (already in App). Lists may stagger 30-50 ms per row on
  first load, capped at the first 8 rows.

## What each surface must look like (from the reference)

- **Home hero** (ref, top): eyebrow `— 力 HOME / LISTEN ··· 01`, display "Good afternoon", one
  line of muted copy, the search field with a `Ctrl+K` kbd chip inside its right edge, the
  hint row under it. To the RIGHT of the hero, a **Now Playing card** at ~550x125: art on the
  left (square, full height), mono eyebrow `PLAYBACK ··· STREAM`, title in Fraunces, artist,
  three mono stat cells (RATIO / SPEED / TEMPO or whatever Playback exposes), a tiny transport
  (prev / play / next) bottom-left, a pill button bottom-right. Hidden when nothing is playing.
- **Chips row** under the hero, full width, 36 px chips, clipped with a fade at the right edge
  (never a hard cut on "Commute").
- **Section headings**: small glyph + `// Title` in `Style.fs.md` + a hairline running to the
  right; optional action pill on the right ("Edit home"). 40 px above each section.
- **Shortcuts / Pinned**: 56 px rows in a bordered card (icon tile 40, title, `N songs`); an
  "Add shortcut" dashed card of the same height.
- **Jump back in**: two-column grid of 48 px art + title + meta rows, `···` menu on hover.
- **Familiar artists**: ONE bordered panel: left column `// ARTIST INDEX` list (01 Yeat ...),
  center the artist photo (square, bleeds to the panel's edges), right column
  `SELECTED ARTIST · 63M MONTHLY AUDIENCE`, name in Fraunces hero size, `TOP TRACKS` list rows.
- **Sidebar** (ref, left): brand card (glyph tile + RYOTUNES / RYOKU // MUSIC) with a collapse
  button, groups `01 DISCOVER / 02 COLLECTION / 03 SYSTEM` as mono eyebrows with a hairline,
  nav rows 40 px with icon + label, active row = paper-lift fill + accent left border. Then
  `04 PLAYLISTS` with a full-width `+ New playlist` outlined button and playlist rows (40 px
  art tile, title, `N songs`). Footer: `RYOKU // MUSIC ··· LIVE` and a tiny spectrum strip.
- **Queue panel** rows: 48 px art, title (fillWidth, single line, elide), artist under it,
  duration mono right, drag handle 24 px on hover only. Explicit badge sits after the title,
  never steals width from it. Header: NOW PLAYING row highlighted, `UP NEXT` eyebrow.
- **Now Playing stage**: art column max 480 px with its glow, title xl Fraunces, artist, meta
  eyebrow; lyrics take the rest with 48 px left gap; spectrum ribbon 56 px at the foot at 0.6
  opacity; a real close affordance: a 36 px pill top-right `Collapse ⌄` (label + chevron), plus
  Escape and the player bar's expand button toggling (done in App).

## Working rules

- Rig only: `scripts/dev/ryotest.sh up|run NAME|ctl NAME <cmd>|shot NAME out.png`. Use your OWN
  instance name; never kill siblings' `qs` processes. Capture, crop to the window
  (`(122,63,1722,1063)` at the rig's default placement - measure if in doubt), and READ your
  capture before reporting. Compare against the reference image side by side.
- `qmllint -I /usr/lib/qt6/qml -I ~/.local/lib/qt6/qml <file>` clean on every touched file.
- Commit only your own files. Do not run the release gate or install; Main does that.
