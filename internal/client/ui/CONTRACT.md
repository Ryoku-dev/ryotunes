# Client UI contract, v4

Ryotunes is a music player. Its window is laid out like one: navigation on the
left, the music you browse in the middle, what plays next on the right, and the
player along the bottom where the hand rests. It is drawn in the Ryoku Hub's
language on the Lithic library: one paper drawn by hairlines, a numbered tree,
a masthead and colophon on every page, washes for where you are, gold for
play, minerals with a word for state, Geologica for type and Martian Mono only
for data.

Ryotunes keeps no copy of the desktop's look. Design, colours, light or dark,
contrast, tint, reduced motion and the Japanese gloss are what the Style page
of Ryoku Settings says, read and followed through the library. What Ryotunes
adds on top is the music's: the accent, the optional cover tint and the
blurred cover behind the music.

Lithic is not ours to copy. The client draws through the shared library
`github.com/ryoku-dev/ryoku/docs/lithic/library/go` (packages `palette`,
`theme`, `design`, `anim`, `kit`, `symbols`, `tokens`, and `fonts` through
`theme`), pinned in `go.mod`. Read, before writing code:

- `~/Work/hyperion/docs/lithic/README.md`, `working.md`, `map.md`,
  `components.md`, `motion.md`, `copy.md`, `checklist.md`, `styles.md`
- the library's `README.md` (every kit API and how a reading resolves)
- the library source under `kit/`, `palette/`, `theme/`, `anim/`
  (read-only for us; never edit it)

The library's `Rail`, `AppFrame`, `NewPlaque` and `BrandMark` are Ryoport's
window. Ryotunes does **not** use them: it has its own music frame below.
The kit parts used as they are: Plaque and its kinds (quiet, line, gold, on,
tonal, danger, danger line), IconPlaque, Search, TextField, Toggle, Slider,
Stepper, Filters, Tabs, Disclosure, Tablet, Empty, Facts, Claim, Sentence,
Label, Text, Span, SlabContent, SectionStyle, BladeProgress, Working, Menu,
Dialog, Confirm, Placard, Click, FocusRing, Notch, Flake, Hatch, the
style-aware painters `Fill`, `Stroke`, `FillCorners`, `StrokeCorners`,
`Shape`, the sheet parts `Masthead`, `TickRuler`, `RegistrationSheet`,
`Clock`, `Ambient`, `SetFocused`, `Ordinal`, `Pad2`, `SpacedCaps`,
`LayoutSpacedCaps`, and `EngravedChart`. When a library control cannot do a
job, the app-owned replacement lives in `look` and says why in one line.

### Library limits we work around

Measured in the hidden harness (`~/Work/ryotunes-rebuild-spec/13-defects.md`):

- `kit.Popover`, `kit.Tooltip` and `kit.Picker` place their overlay inside the
  **caller's local constraints**, treating them as the window. Called from a
  nested slot (a player-bar button, a settings row, a top-bar field) the
  overlay is squeezed to the slot and cannot flip or clamp to the real window.
  Ryotunes never uses these three; it uses `look.Overlays`, `look.Tip` and
  `look.Choice`, which place in window coordinates. `kit.Menu` is fine as the
  content of a `look.Overlays` entry. `kit.Dialog`, `kit.Confirm` and
  `kit.Placard` are laid out by the shell with full-window constraints and are
  used as is.
- `kit.RowList`'s scroller treats any delta within 0.5 of a multiple of 150 as
  a wheel notch and eases it. Gio does not expose the Wayland axis source, so a
  trackpad stream that happens to send 150 glides behind the fingers. Ryotunes
  scrolls only through `look.Scroller` / `look.HScroller`, which apply every
  delta at once. Do not use `kit.RowList`.

Never edit the library from this repository; these limits are reported to its
owners. No file may keep a copied cut, paint recipe or click wrapper.

## Packages

```
internal/client/
  conn/  state/  art/  spectrum/
  ui/
    look/        app theme on the library: accent and source colours, cover
                 tint and backdrop, Ryoku mark, provider glyphs, icons, frame
                 geometry, washes, motion helpers, drawers, scrolling,
                 scrubbers, art view, visualizers, the equalizer's graph and
                 faders
    sheet/       Ryotunes' places on the Hub's sheet: numbering and glosses,
                 masthead adapter, colophon, tategaki, spec strip, reveal
    env/         Env: theme, overlays, navigation, notices, the desktop
    act/         daemon actions
    widgets/     track table, card, shelf, grid, quick tile, hero, page head,
                 ruled head, page states, item menu, playlist dialogs,
                 transport, seek, play art
    chrome/      top bar, provider switcher, Discord toggle, search, sidebar
                 tree, bottom nav, player bar, compact player, volume, sound
                 popover, right panel (queue, lyrics, together), now playing,
                 mini window content
    shell/       windows, frame, router, overlays, notices, keys, single
                 instance
    pages/       home search library album playlist artist list radio
                 downloads settings diagnostics equalizer, and pagesheet
                 (spec strip and the "loading more" line)
    demo/        fixtures and the in-process daemon for --demo renders
```

Nothing under `ui/` imports `shell`. Pages import `env`, `sheet`, `widgets`,
`act`, `look` and the library. Chrome imports the same plus `widgets`.

## Style and the desktop

`shell/run.go` reads the desktop once with `palette.ReadDesktop()` and builds
the window's theme with `look.NewThemeFor(desktop, density, provider)`, which
wraps `theme.NewFor`. `palette.WatchDesktop` runs on its own goroutine for the
life of the client and hands each new reading to the window goroutine, where
`shell/theme.go` `applyClientTheme` calls `Theme.SetDesktop`. The main and
mini windows share one theme.

The reading (`palette.Desktop`): `Design` (`lithic`, `material`, `adwaita`,
`fluent` from `shell.json theme.design`), `Colours` (Ryoku's or the
wallpaper's, `theme.colours`), `Mode`, `Look` (`theme.contrast`,
`theme.tint`), `ReduceMotion` (`theme.motion.reduce`), `Gloss`
(`showJapaneseGloss`), `Palette` and the `Roles` the design wears. The watcher
covers `shell.json`, `theme.json` and `matugen.json` in
`$XDG_CONFIG_HOME/ryoku/` and `colors.json` in `$XDG_CACHE_HOME/ryoku/`, all
the time, coalesces events over 150 ms and calls back only when the reading
changed. How each key resolves is the library README's table; Ryotunes does
not resolve anything itself.

- A desktop change snaps: roles, planes, design and the motion flag change in
  the next frame, both windows together (`look/theme.go` `SetDesktop`,
  `SetRoles`). Only the music's own colours tween (Accent, below).
- Reduced motion has one home: `theme.NewFor` sets `anim.SetReduce` from the
  desktop and `env.Env.ReduceMotion()` reads it. Tweens, springs and chases
  then land at once; the `th.Motion` durations themselves are untouched.
- The gloss gates every kanji in the chrome: `env.Env.Gloss()` is
  `Th.Desktop.Gloss`.
- `state/prefs.go` stores none of these. The old `appearance`,
  `paletteFile`, `reduceMotion`, `corners` and `cornerDp` keys are gone and
  ignored when an old file still has them.
- `--mode follow|dark|light` (`cmd/ryotunes`) sets `LITHIC_MODE` before the
  first reading (`shell/run.go`), and the library honours `LITHIC_MODE`,
  `LITHIC_DESIGN` and `LITHIC_PALETTE_JSON` in every reading, so demo renders
  and tests choose their look without touching `~/.config/ryoku`. PNG renders
  force reduced motion unless `--demo-playing` (`shell/headless.go`).

No hex value appears outside `look/brand.go`.

### Planes and shapes: the four styles

The design decides two things: which role each plane wears and how a cut
corner is drawn. Ryotunes draws every shape through the style-aware kit
painters (`kit.Fill`, `Stroke`, `FillCorners`, `StrokeCorners`, `Shape`), so
Material, Adwaita and Fluent change corners and planes exactly as the Hub
does. It never calls `th.Corner` itself.

`th.Planes()` (library `palette/planes.go`):

| Design | Side (window paper) | Pane | Pane stroke | Card | Card stroke |
|---|---|---|---|---|---|
| Lithic | ground | stone2 | vein, 1 px | slab | vein |
| Material | ground | stone1 | none | stone3 | none |
| Adwaita | stone3 | ground | none | stone4 | none |
| Fluent | ground | stone3 | vein | stone4 | vein at .7 |

Corners (library `design/design.go`): Lithic cuts at 45 degrees by the cut
itself. The round designs turn a cut into a radius of `cut × Scale`, at most
`Max` and half the shorter side, and a shape whose shorter side is at most
`Pill` becomes a pill:

| Design | Scale | Max | Pill | Pane top 16 / foot 7 become |
|---|---|---|---|---|
| Material | 2.6 | 28 | 56 | 28 / 18 |
| Adwaita | 1.6 | 14 | 26 | 14 / 11 |
| Fluent | .75 | 8 | 22 | 8 / 5 |

The cut tokens are fixed: Large 16, Notch 12, Menu 8, Mid 7, Small 6,
Tooltip 4, Token 3. A cut of 0 stays square in every design.

- Covers (`look/art.go` `artRadius`) take the radius rule without the pill:
  `min(cut × Scale, Max)`, capped at a quarter of the cover's shorter side, so
  a cover is a rounded square in the round designs and never a circle. Artist
  portraits (octagons in Lithic) are circles there. `look.FillArt` draws the
  Liked songs tile in the same shape.
- Stay angular in every design: the visualizer's blades and peak caps (R10,
  `chrome/doc.go`), the 45 degree art reveal sweep, `kit.Notch`, blades and
  flakes, the seek handle (cut 0) and the download rule on the player bar.
- The volume handle (10 dp, cut 3) and equalizer fader handles fall under the
  pill size in the round designs and turn into a circle and a pill; the
  equalizer's nodes stay diamonds.

### Roles

| Role | Used for |
|---|---|
| side (`Planes().Side`) | the window paper: painted first in both windows (`shell/layout.go`, `shell/window.go`); the top bar, the sidebar tree and the bottom nav stand bare on it |
| pane (`Planes().Pane` + stroke) | the page pane, the docked panel, the panel and sidebar drawers, the player bar, the compact player and the mini window |
| card (`Planes().Card` + stroke) | slabs inside a pane: Home's now playing card, quick tiles, card hover planes, radio tiles, Settings and equalizer cards, Listen Together groups |
| stone2 | the reference plane for `Accent.Ink` and `Src.Ink` contrast; the Home card's instrument plate |
| stone3 | the art placeholder and the `look.Choice` field |
| stone4 | overlays (menus, popovers, tips; with a 1 dp vein hairline), the scrubber bubble, the card play plate at rest, the play art plate at .9 |
| slab, field, edge | Lithic's card, fields and fader tracks, a field's edge |
| washes | `Roles.Line()` .08 / .10, `Guide()` .16 / .20, `GuideLit()` .36 / .42, `Hover()` .06 / .07, `Press()` .10 / .12 (fg at that alpha, dark / light, solved over ground), `Select()` sel at .16 / .14 |
| sel | the selection wash's hue and the marks of where you are: the selected index numeral, the masthead's number and ruler marker, the tategaki rule, a lit spec value, the equalizer's curve and handles |
| sel / onSel as a plane | only a toggle that is on: `PlaqueOn` (shuffle, repeat, open panels), `kit.Toggle`, the slider handle |
| gold / onGold | play. The theme points these roles at the **accent** (below). The player bar's play and pause is the window's standing gold action; a page may spend one more gold on the action that plays it (hero Play, top search result Play), and a dialog's one consequential action (Create, Save, Import, Export, Add folder) is gold. Nothing else is gold: not radio, not download, not navigation. |
| jade lapis amber jasper | state, always with a shape and a word: downloading (lapis), saved (jade), paused (amber), failed (jasper), live stream (jade) |
| fg fg2 fg3 vein | ink and hairlines |
| scrim | behind drawers and dialogs |

**Accent.** One hue belongs to the music, and everything that means "this is
playing" or "play" wears it: the play plaques, the progress wave, the
visualizer, the playing row, liked hearts, the sung lyric words and the header
fields. `look.Theme.Accent` carries it (`look/accent.go`):

- `Accent.Fill` the face of a gold plaque; `Accent.On` the ink on it
  (`palette.InkOn(Fill)`);
- `Accent.Ink` the fill re-toned along HSL lightness to 4.5:1 on stone2
  (lighter in dark mode, darker in light), for type, glyphs, the wave, the
  visualizer and hearts;
- `Accent.Field` the header field plane: OKLCH with the lightness of
  `roles.MineralField`, the accent's hue and a chroma of at most .05
  (`tokens.DyeMineral["amber"]`); a colourless accent gives a grey field.

The theme writes Fill, On and Ink into `R.Gold`, `R.OnGold` and `R.GoldInk`
whenever the scheme changes and on every frame of a scheme tween
(`look/theme.go` `applyScheme`), so library gold plaques follow without a
wrapper. Where the accent comes from is the **Accent colour** preference
(`ClientPreferences.Accent`):

1. **Album art** (default). The loaded track's cover, sampled off the frame
   goroutine by the art cache into an `art.Palette`. `Vibrant`: a 24 × 24
   downsample, pixels with alpha under 128 or a channel spread (max − min)
   under 40 of 255 skipped, colours bucketed by 3 bits per channel, each pixel
   scoring `spread / 255 × (1 − |max − .65|)`, the bucket with the best total
   wins and its average is the sample. (1.x divided the spread by the maximum,
   which called blue-tinted blacks vivid, so dark covers took a bright blue
   accent from their shadows.) `Dominant` is the most populous bucket over all
   opaque pixels, whatever their colour. The accent keeps `Vibrant`'s hue;
   saturation is clamped to .55–.85 and lightness set to .70 in dark mode and
   .42 in light, so it reads on every plane. A grey cover (sampled, no vibrant
   bucket) takes the neutral accent: the grey at that lightness, with `fg` as
   its ink so sung words stand apart from the rest of the line in `fg2`. A
   cover that is still loading keeps the previous accent; a track without a
   cover or nothing loaded uses the desktop accent.
2. **Source.** The active provider's brand (`look/brand.go`).
3. **Desktop.** The desktop's gold roles as loaded.

Every accent change (a new track, a provider switch in Source mode, the
preference itself) tweens the whole role set, the accent and the source
colours together over `Motion.Field` (320) with the chisel ease; a desktop
change snaps. `ArtScheme` caches 32 schemes keyed by cover, loaded roles and
tint. Album, playlist and artist heroes tint only their own field (and its
visualizer lane) with their own cover's accent (`Theme.ArtAccent`, the neutral
one for a grey cover), tweening in over `Motion.Field`; their Play plaque stays
the global accent.

**Cover tint.** While the accent comes from the album art and **Tint with the
cover** is on (`ClientPreferences.CoverTintOn()`, JSON key `coverTint`,
**default off**), the window takes the cover's hue over the desktop's planes,
the way the desktop's palette takes the wallpaper's (`look/tint.go`,
`look/oklch.go`). Ground, the four stones, slab, field, vein, edge, fg, fg2
and fg3 are re-hued in OKLCH: each keeps its own lightness, so every contrast
pair holds, and takes a small chroma (dark mode: ground .020, stone1 .026,
stone2 .030, stone3 .034, stone4 .036, slab .030, field .034, vein .030,
edge .030, fg .010, fg2 .014, fg3 .018; light mode .8 of those) times a
strength, reduced until it fits sRGB. Sel is the desktop's accent and is never
tinted; the minerals, gold, scrim, onSel, onSel2, container and onContainer
never change, so states read the same on any cover. Hue and strength come from
the palette: the dominant colour's hue when its OKLCH chroma is at least .04
(strength `min(1, chroma / .12)`), else the vibrant colour's hue at strength
.35, else, for a black and white cover, chroma 0: every one of those roles
turns grey at its own lightness, so the cover is never framed in the desktop's
hue. The accent is then derived against the tinted stone2, so `Accent.Ink`
still meets 4.5:1. With the tint off, an accent mode other than Album art, or
nothing loaded, the planes are the desktop's (`shell/theme.go`
`reconcileProvider`).

**Cover backdrop** (`look/backdrop.go`). The cover, heavily blurred, sits on
the planes where a cover is the subject: the now playing view's field and
Home's now playing card (the loaded track's cover), the album, playlist and
artist heroes (their own cover) and the mini window (the loaded track's).
Page headers, the player bar, the compact player, cards and rows stay plain.
The art cache scales the cover to 48 × 48 and blurs it on its worker (three
box passes of radius 2, edges held), in grey when the sampler found no colour
in it, and keeps it with the palette, in memory and in the cover's palette
file on disk; a palette file without a 48 × 48 backdrop is sampled again once
from the disk cover. A cover's first sample stays: loading the same cover at
another size keeps it, and only a background refresh that brings a changed
cover replaces it. Collages have none. `look.Backdrop` tones it to the plane
under it (the field for now playing and heroes, `Planes().Card` on the Home
card, `Planes().Pane` in the mini): each pixel keeps its hue and the order of
its tones, its OKLCH lightness is pressed into a band around the plane's, .16
deep on the side away from the text and .05 toward it (darker in dark mode,
lighter in light), and its chroma is capped at .12, so the cover's big shapes
and colours show while type over it keeps 4.5:1. A dark plane below .22
centres the band on .22 instead (`backdropFloorDark`), so the black Lithic
ladder still shows the cover (R11). It is drawn at .9 opacity, scaled to cover
the plane (centre crop, linear filtering) inside the plane's shape, and toned
again only when the plane's lightness moves by .02 (a tint tween) or the mode
changes. The first frame shows it whole. A new cover fades in over
`Motion.Field` with the chisel ease while the old one fades out, and a cover
that lands mid-fade carries on from wherever the fade left the old one. No
cover, or one not sampled yet, fades the last one out.

**Source colour.** The active provider's brand is identity, not state, so it
may stand without a word, and it lives in the provider switcher and the small
provider glyphs. `look/brand.go` holds the fixed brand fills of YouTube Music,
Spotify, SoundCloud and TIDAL and the Ryoku mark orange. `look.Theme.Src`
carries the active provider's `Fill`, `Ink` (re-toned to 4.5:1 on stone2) and
`Field`, tweened over `Motion.Field` when the provider changes. It draws the
switcher's active cell, the compact switcher's glyph, and the provider glyphs
in the player bar's facts line and mixed track lists. Local files and radio
have no brand: their source colour is `R.Lapis` and its re-toned ink.

## Hub language

### The sheet (`shell/layout.go`, `sheet/`)

Every page pane at least 240 dp tall (`sheetMinimum`) is a sheet, drawn in
this order:

1. `kit.RegistrationSheet` over the whole pane: a 1 px dot every 24 dp
   (fg .07 dark / .10 light), a 7 px plus on every fourth crossing
   (.13 / .16), 13 px print-register crosses 14 in from each corner
   (.32 / .36). Recorded once per size, scale and ink and replayed.
2. The page on the stage, from y 38 (4 + 34) to 6 + 26 above the pane foot,
   the pane's full width. The shell applies no side inset; pages keep their
   own content insets.
3. The masthead (`sheet/masthead.go` over `kit.Masthead`), x 28, y 4, 34
   tall, to 10 short of the pane's right edge.
4. The colophon (`sheet/colophon.go`), x 28, 26 tall, 6 above the pane foot,
   to 20 short of the right edge.

A shorter pane shows only the page. Now playing draws its own head (Now
playing view).

**Masthead.** Left: "§ " and the number in sel (`05.8` for a Settings
section), the place in spaced caps with its part after "  /  "
("SETTINGS  /  APPEARANCE"), then the gloss at 11 sp in fg3 joined with "・"
("設定・外観"), only while the desktop's gloss is on. Right, 12 apart with a
3 × 3 fg3 square between, the last in fg2 and the rest fg3: the page place
("P 06/10"), the provider in caps (Home, Library, Search, Album, Artist,
Playlist and List only, and only while the pane is at least 720 dp wide), and
the clock. The clock is `kit.Clock`: "HH:MM", with seconds only while
`kit.Ambient` (the window is focused, motion is not reduced); it asks for a
frame at the next second only then. Under it, the tick ruler: a 1 px tick
every 8 dp, every fifth 7 tall and the rest 4, on a 1 px vein baseline, and a
24 × 3 sel marker that eases over `Motion.Quick` (120, OutCubic) to how far
the page is read; hidden when the page fits.

Pages feed the masthead through an optional method: `Masthead() (part,
partGloss string, progress float32)`, or Settings' `Masthead() (number, part,
partGloss string, progress float32)`. An empty part keeps the route's own part
(Album, Artist, Browse, Playlist). Library's part is its tab (and the tab's
gloss); Downloads, Radio and Search name their filter when it is not the
first.

**Colophon.** The place's id in Code 39 bars (upper case, framed by `*`,
9 dp tall, 1 dp unit), its words 3 under them in `MonoSm` spaced caps,
tracking 1.6, fg3, right aligned; 16 to its left, while the gloss is on,
Ryoku's motto 力と美のために at 11 sp, tracking 2, fg3. Bars are recorded once
per code, scale and ink.

**Page head** (`widgets/hero.go` `Header`). Title `T.Sentence` overridden to
32/40 (Home keeps 34/40 for its greeting), at most two lines; lead `T.Body`
fg2, 4 below, at most 720 wide; an optional state line `T.Detail` fg; the spec
strip 16 below (`sheet/specstrip.go`): labels `MonoSm` spaced caps, tracking
1.2, fg3, 6 to a value in `MonoSm` fg (sel when lit), figures 18 apart,
values zero padded below 100 (`kit.Pad2`) and rolling when they change
(`look.CountRoll`, 240 OutCubic). A page passes no strip until its counts are
known, so no zeros flash. The tategaki (`sheet/tategaki.go`) stands at the
head's right in a 36 dp room while the gloss is on: an 18 dp sel rule, the
page's kanji at 20/22 fg2, a 3 dp fg3 dot, the place's kanji at 11 fg3.

**Ruled heads** (`widgets.RuledHead`): title `T.Section`, a 1 dp vein rule 13
after it to 13 before the mark, the mark `kit.Ordinal` in `MonoSm` fg3
("02 / 07"), an optional trailing control 13 after. Shelves, Home's readings,
Artist's Popular, Radio's groups, Diagnostics, Downloads' Storage, Library's
watched folders and the Add to playlist list use it; Settings and the
equalizer use `kit.SectionStyle{Ruled, Mark}`.

**Reveal** (`sheet/reveal.go`): the first five groups of a page fade and rise
8 dp over `Motion.Navigate` (200, chisel), 40 ms apart
(`Motion.RevealStagger`), once per page; refreshes and later groups stand.
Reduced motion plays nothing.

**Work pattern.** Waits show nothing for the first second (the room is
kept), then `kit.Working` flakes with a word; whole-page loads add a
`kit.BladeProgress` and a quiet "Go back" past 10 s (`widgets/states.go`).
Lists that load more use `pagesheet.More`: nothing for a second, then flakes
and "Finding more songs", "Finding more stations", "Loading the rest of the
album" and so on.

### Numbering and glosses (`sheet/numbering.go`, `pages/settings/page.go`, `pages/library/page.go`)

| Place | Number | Gloss | Page place |
|---|---|---|---|
| Home | 00 | 表紙 | P 01/10 |
| Library | 01 | 書庫 | P 02/10 |
| Radio | 02 | 放送 | P 03/10 |
| Downloads | 03 | 保存 | P 04/10 |
| Playlists | 04 | 曲集 | P 05/10 |
| Settings | 05 | 設定 | P 06/10 |
| Search | 06 | 検索 | P 07/10 |
| Now playing | 07 | 再生 | P 08/10 |
| Equalizer | 08 | 調音 | P 09/10 |
| Diagnostics | 09 | none | P 10/10 |

Routes map to places in `sheet.ForRoute`: an album, artist or browse list sits
in the Library with the part "Album", "Artist" or "Browse", a playlist among
the Playlists with the part "Playlist". Ryotunes' own head gloss is 音楽.

Settings' sections are `05.1` to `05.12`: Account 利用者, Playback 再生, Sound
音, Lyrics 歌詞, Downloads 保存, Local music 手元, Playlists 曲集, Appearance
外観, Integrations 連携, Data and storage 保管, Diagnostics 診断, About 概要.
Sound, Appearance and About are the Hub's own words. Library's tabs: Playlists
曲集, Songs 曲, Albums 盤, Artists 演者, Local 手元. Every word without a Hub
precedent is a proposal for the owner to confirm.

### Washes (`look/wash.go`, `look/selectwash.go`)

- `look.RowWash` is an app-owned row's hover and press: `Roles.Hover()` in
  over `Motion.Quick` (120) and out over `Motion.Move` (240) on OutCubic, the
  row leaning 2 dp right while hovered, `Roles.Press()` under the pointer over
  `Motion.Press` (90) with the glyph at .86 (icon buttons .85). Library plaques
  keep their own hover. A settled wash paints nothing and asks for no frame.
- `look.SelectWash` is where you are: one `Roles.Select()` wash, fg ink,
  travelling between lines of a group over `Motion.Move` (240, OutCubic).
  Arriving from none or from another group it lands: it stands at the new
  line, fades in over 240 and widens from .6 over 320. The first placement
  stands. Used by the sidebar tree, the bottom nav, track tables, the queue,
  the search suggestions and the equalizer strip; the Discord toggle's on
  state wears the same wash.
- Solid `sel` slabs are not used for navigation anywhere.

### Notices and Undo (`shell/environment.go`, `env/env.go`)

`e.Toast(message, kind)` and `e.Undoable(message, undo)` push a
`kit.Placard`: 48 tall, at most 560 wide, 24 above the panes' foot (so above
the player), in over 300, life 5000, three at most. `noticeText` trims and
capitalises the first letter, and an error ends with a period. The same text
pushed while the last one still shows replaces it (and drops its Undo).
`Undoable` adds the action "Undo"; pressing it runs `undo` once on the UI
goroutine. Only the newest three notices keep a live Undo. Without a notice
host (headless renders) the message shows as a plain success toast.

Undo is offered for every reversible removal the daemon can put back exactly:

| Action | Notice | Undo |
|---|---|---|
| Unlike, from any heart (player bar, compact player, now playing, mini, Home card, track rows), the item menu or Ctrl L | "Removed from liked songs." | likes it again (the daemon rates by video id) |
| Remove from library (item menu) | "Removed <title> from your library." | albums are saved again; other collections go back to their old place in the app's list, never twice |
| Album or playlist heart off | "Removed from your library." | saves it again |
| Unsubscribe from an artist | "Unsubscribed from <name>." | subscribes again |
| A radio favourite off | "Removed <station> from your favourites." | favourites it again |
| Stop watching a local folder | "Stopped watching <folder>." | watches it again |
| Reset a setting | "<Setting> is back to its default." | restores every value the reset moved |
| Equalizer: remove a band, lower the preamp, reset, switch preset | "Removed the band at 1.2 kHz.", "Lowered the preamp by 3.2 dB.", "Reset to a flat curve.", "Switched to <preset>." | the curve glides back over 240 OutCubic; a removed band's column unfolds again |

Where Undo is not offered, and why:

- **A like, a save, a subscribe, a favourite:** the control that did it takes
  it back, so a notice would only repeat the button.
- **Remove from queue and Clear:** the protocol has no way to put a track
  back at a position (`add_to_queue` appends, `play_next` inserts after the
  current row) or to restore rows, so an Undo would turn an autoplay row into
  a manual one, lose its queue context and its shuffle order.
- **Remove from playlist:** adding to a playlist takes no position, so an
  Undo could only put the song at the end.
- **Delete playlist, Clear caches, Clear download history, Sign out,
  Disconnect Last.fm, Stop watching a folder from Settings:** these ask first
  through `e.Confirm` with a danger plaque; a remote playlist's delete says
  "It can't be undone."
- **Toggling or choosing a setting:** the control is its own undo; only a
  reset, which can move several values at once, offers one.
- **Import an equalizer profile:** it replaces the working curve directly
  and keeps the import's own notice ("Imported 10 bands. Skipped 1 line.").

## Frame geometry (dp)

`W`, `H` are the window size in effective dp: window pixels divided by the
screen scale times the app's zoom. The player window has its own classes:
compact `W < 760`, medium `760 <= W < 1100`, expanded `W >= 1100`
(`look/geometry.go`). Structure changes at a boundary; a wide layout is never
squeezed.

```
expanded / medium
┌──────────┬──────────────────────────────────────────────────────────┐ y 0
│ sidebar  │ top bar 56, bare on the window paper                     │
│ tree     ├─────────────────────────────────────────┬────────────────┤ y 56
│ bare on  │ page pane (pane plane + stroke,         │ right panel    │
│ the side │ top 16, foot 7)                         │ pane, 340      │
│ 272 / 72 │ masthead / sheet / colophon             │ (when docked)  │
│          ├─────────────────────────────────────────┴────────────────┤ y H-8-88-8
│          │ player bar 88, pane, top 16, foot 7, x 8 .. W-8          │
└──────────┴──────────────────────────────────────────────────────────┘ y H-8

compact
┌──────────────────────────────┐
│ top bar 56                   │
│ page pane, x 8 .. W-8        │
│ compact player 64 (pane)     │
│ bottom nav 56 under a vein   │
└──────────────────────────────┘
```

| Part | Expanded | Medium | Compact |
|---|---|---|---|
| window | `Planes().Side` | same | same |
| sidebar | the tree, x 0, full height, 272 (72 folded) | the tree as a 72 rail | none: bottom nav 56 |
| page pane | from the sidebar's right edge to `W - 8` (less the docked panel and its 8 gap), y 56 to the player's gap | same | x 8 to `W - 8` |
| content side inset (pages) | 34 | 21 | 13 |
| right panel | docked 340 when `W - sidebar - 340 - 24 >= 520` (sidebar read at its fold target), else a drawer | same rule | a drawer the pane's width, opened only by an explicit action |
| player bar | 88, three zones, x 8 to `W - 8` | 88, narrower | compact player 64 above the bottom nav |

Every pane is `Planes().Pane` with the design's stroke 1 px inside, its top
corners cut `Cut.Large` (16) and its foot `Cut.Mid` (7), through
`kit.FillCorners`; a pane too small to carry both cuts drops them
(`shell/layout.go` `paneCuts`). Each pane pushes its own clip. The window
minimum is 480 × 560; the default is 1280 × 800.

Crossing into compact closes a docked panel (it would otherwise cover the
page) and remembers it; crossing back out restores it. A drawer never opens by
itself on a class change. While now playing is open the page pane runs to
`W - 8` and the docked panel is hidden.

The docked panel's width unfolds 0 to 340 over `Motion.HubUnfold` (260,
OutCubic) with its opacity following. When it cannot dock it is a
`look.Drawer` from the right over the page pane: `min(340, pane width)` wide
(the whole pane in compact), the pane plane with the design's stroke (vein
where the design draws none), cut 16 / 7, sliding over `Motion.Move` (240,
OutCubic) above a `Roles.Scrim` that fades with it; a press on the scrim
closes it.

**Nothing loaded.** Until the daemon reports a current track there is no
player bar and no compact player: the panes run down to `H - 8` (compact: to
the bottom nav). When a track loads, the bar unfolds from the bottom over
`Motion.HubUnfold` (260, OutCubic), clipped so it slides up whole, while the
panes' bottom edges glide up by the bar and its gap; when nothing is current
anymore it folds away the same way. Reaching the end of the queue or stopping
keeps the last track in the bar, as music players do. The first snapshot after
connecting snaps instead, so a launch with a restored track never shows the
bar arriving and a launch with nothing never flashes an empty one. Reduced
motion snaps.

**Page change.** `look.PageRise` (`shell/layout.go`, `look/pagerise.go`): the
new page fades in over 240 and slides 14 dp in from the right over 300, both
OutCubic; the old one leaves at once; the first page stands; reduced motion
plays nothing. The key is the route and the provider, so a provider switch
re-rises the page; now playing rises again on every track change.

### Top bar (chrome/topbar.go)

Bare on the window paper, 56 high, every blank part drags the window
(`system.ActionMove`; double click toggles maximise while the client draws its
own decorations). The Ryoku mark and the app's name live in the sidebar
tree's head, not here.

- **Expanded and medium:** at the page pane's left edge (tweening with the
  fold) the sidebar toggle, Back and Forward: small quiet `IconPlaque`s, 32 at
  y 12, 36 apart; Back and Forward show disabled ink when the router cannot
  move. 13 to their right the search field flexes up to 520, stopping 13
  short of the Discord toggle. At the right: the Discord toggle, 13, the
  provider switcher (180 × 40), 13, then window controls when the client
  draws its own decorations, ending 13 from the window edge.
- **Window controls** (`look.WindowControl`): Maximise or Restore, then
  Close, 32 square, glyph 18, 4 apart. Hover is the row wash, a press grows
  the cut by 4 and sinks 1 with the glyph at .85. Close turns its glyph a
  quarter clockwise while hovered over `Motion.Move` (240, OutCubic), not
  under reduced motion. There is no minimise: niri and Hyprland have no
  minimised state, and Gio cannot read the compositor's capabilities to show
  it only where it works.
- **Sidebar toggle** (`look.SidebarPlaque`): its glyph is a frame with its
  left column filled while the sidebar is open and hollow while folded,
  morphing over `Motion.Notch`, corners per design. Tips: "Collapse sidebar ·
  Ctrl B", "Expand sidebar · Ctrl B", or in the medium class "Show sidebar ·
  Ctrl B".
- **Compact:** the mark (24) at x 13; a search icon plaque at x 50 that opens
  the field across the bar (it closes when it loses focus with an empty
  query); the provider switcher collapses to one 40 cell with a vein hairline
  (cut 6) showing the active provider glyph in `Src.Ink`, opening a
  `look.Overlays` `kit.Menu` (below, end aligned, 260 wide) of the four
  providers, "<Provider> · Signed in" or "<Provider> · Not signed in"; window
  controls.

**Discord quick toggle** (chrome/discord.go). 13 left of the provider
switcher in every class: 32 square, cut 6, glyph 18. Off: a vein hairline.
On: the `Select` wash with the glyph in sel; the two cross over `Motion.Quick`
(OutCubic). Row wash under the pointer, glyph .86 pressed. Tip "Discord
status: " and "Off", "Connected", "Connecting" or "Discord is not running".
A failed change reverts and says "Could not change Discord status: <error>."
The same setting is the first row of Settings → Integrations.

**Search has one entry point: this field** (chrome/search.go). `kit.Search`,
placeholder "Search tracks, albums, artists…", hint "/". There is no Search
place in the tree or the bottom nav. `/` and Ctrl K focus it. While it is
focused a passive `look.Overlays` entry under it, as wide as the field, lists
up to 8 recent searches under "Recent searches" when empty, or up to 6
suggestions while typing (debounced 200 ms, from the active provider),
grouped by kind under "Songs", "Albums", "Artists", "Playlists" (`T.Label`
fg2, 28 tall). A suggestion is its title in `T.Row` with every query word
underlined 1 dp at its first match (accent folded, case insensitive) over a
crumb in `T.Detail` fg2 ("Song › Kite Assembly"); a recent is a search glyph
and the query. Rows hover with the row wash; ↑ and ↓ move a travelling select
wash; the recents and suggestions swap over `Motion.Quick`. Enter submits the
highlighted row or the typed text and pushes `RouteSearch{Query}`. The field
keeps the query while the results page is current and clears when the user
navigates elsewhere. Escape closes the list and clears the field, then blurs.

**Provider switcher** (chrome/providers.go): a vein hairline box (cut 6, no
fill), 180 × 40, holding four 40 × 32 cells 4 apart in the order YouTube
Music, Spotify, SoundCloud, TIDAL. Each cell shows the provider glyph (20).
Inactive glyph fg3, mixing to fg2 under the row wash; .86 pressed. The active
cell is a plate of `palette.Mix(Planes().Side, Src.Fill, .22)` with a 1 dp
`Src.Ink` hairline at alpha .55 and the glyph in `Src.Ink`; it glides to a
newly chosen cell over `Motion.Move` (240, OutCubic); the source colours
retint over `Motion.Field` (320); the page rises again. Every cell has a
`look.Tip`: "<Provider> · Signed in" or "<Provider> · Not signed in".
Clicking an inactive cell sets the provider optimistically and calls the
daemon (`set_provider`); a failure reverts and says "Could not switch to
<Name>: <error>."

### Sidebar tree (chrome/tree.go, tree_row.go, sidebar.go)

The Hub's tree, bare on the window paper at x 0, 272 wide. Rows are inset 4
from its edges.

- **Head** (40, at y 8): `look.RyokuMark` 20 at x 14, then 10 and "RYOTUNES"
  in `T.Group` spaced caps, tracking 1.6, fg2, with "音楽" after it while the
  gloss is on.
- **Body** (from y 56, scrolling through `look.Scroller`): Home `00`, Library
  `01`, Radio `02`, Downloads `03` as numbered lines, then the Playlists
  folder `04` and its leaves.
- **Foot** (8 above the bottom): Settings `05`, and its outline while
  Settings is open. A 1 px line wash at x 12 to `W - 12` sits above the foot
  only while the body overflows.

A **numbered line** is 34 tall: the numeral in `MonoSm` (fg3, sel when lit),
10, the name in `T.Group` spaced caps, tracking 1.4 (bold when selected and
for the folder), a 1 px leader (`Guide`, `GuideLit` on hover or open), the
gloss at 12 sp, tracking 1, while the gloss is on, and a count in `MonoSm` at
the tail. Downloads counts active and queued jobs (fg3, padded below 100)
with a lapis flake while they run. The current place's glyph is the filled
Material Symbol (`look.Filled`).

The **Playlists folder** shows its count only while folded (at .6 of hover
while open). Its chevron turns a quarter over `Motion.Move` (240) on OutBack
1.6 (`look.EaseOutBackChevron`). Open state starts open and lives in memory.
The leaves unfold over `Motion.HubUnfold` (260, OutCubic), each sliding 8 dp
in from the left and fading over `Motion.Move`, 18 ms apart
(`Motion.Stagger`, capped at 24 leaves). Leaves are indented 22 on a 1 px
guide at x 16 (`GuideLit` while the open page is in the folder) with 8 dp
branch ticks (sel when selected); the last leaf's guide stops at its middle.

- **New playlist** (first leaf, 30 tall): the Add glyph and "New playlist"
  in `T.Detail`; opens the New playlist dialog.
- **Collections** (44 tall): cover 32 (cut 3), title `T.Detail` (fg2, fg on
  hover, bold when selected), caption `T.Caption` fg3: "No songs yet" or
  "<N> songs" for Liked songs, "Playlist · <N> songs", "Album · <artist>",
  "Artist". Order: Liked songs (a `look.FillArt` tile of `Accent.Field` with a
  heart in `Accent.Ink`), pins (`state.Personal.Pins`), library playlists.
  The playing collection draws its title in `Accent.Ink` with a `volume_up`
  glyph 16 at the right. Right click opens the item menu.

Hover and press are the row wash (a selected row does not lean); a line's ink
turns toward the selection over `Motion.Quick`. Where you are is one select
wash per region (body and foot) that travels within a group and lands when it
moves between the places and the leaves. A leaf selected inside a folded
folder rests the wash on the folder line. Search, the equalizer and
Diagnostics select no line.

**Settings outline.** While Settings is open, the class is expanded and the
sidebar is unfolded, the Settings line hangs its section outline: rows 26, a
guide thread at x 36, captions at x 55 in `T.Caption` (bold for the section
being read), ink fg3, fg2 on hover, fg when current. Nodes are 1 px rings of
5 dp that grow to 7 and fill for the section being read, over `Motion.Move`
on OutBack 2 (`look.EaseOutBackNode`). The outline unfolds over
`Motion.HubUnfold`. Pressing a row glides the page to that section.

**Folding (expanded class).** The tree folds to a 72 rail and back: the top
bar's toggle or Ctrl B, remembered in `ClientPreferences.SidebarCollapsed`.
The width tweens 272 ↔ 72 over `Motion.Move` (240, OutCubic), and everything
to its right lays out from the tweened width each frame; the docking rule
reads the target width, so a fold never makes a docked panel flicker. Names,
glosses, leaders and counts fade out over the first half of a fold; the rail's
tiles fade in over the second half. Folded, each line is a 48 tile: its glyph
20 over its numeral in `MonoSm` fg3, a tip naming it, covers 32 centred, the
New playlist leaf as the Add glyph; the select wash narrows to an 8 dp inset.
In the medium class the rail is all there is, and the toggle opens the full
tree as a `look.Drawer` from the window's left edge over everything,
`min(272, W)` wide, on the side plane with a vein hairline on its right edge,
sliding over `Motion.Move` (240, OutCubic) above a fading scrim; a press on
the scrim, Escape, Ctrl B, choosing a place, or leaving the medium class
closes it. Compact has no toggle.

**Compact:** no tree. `chrome/bottomnav.go` draws Home, Library, Radio,
Downloads, Settings in five equal columns, bare on the window paper under a
1 dp vein hairline. Each tile is the icon 20 over a `T.Caption` label; the
current tile is a select wash (inset 4, cut 6) that travels, with the filled
glyph in sel and the label in fg.

### Player bar (chrome/playerbar.go)

A pane (above), 88 high, x 8 to `W - 8`, side insets 21. Three zones:
`side = clamp(barW × .28, 220, 360)`; the centre column is
`min(560, barW - 2·side - 42)` wide and centred on the bar.

- **Left zone**, one `layout.Flex` so nothing can overlap: art 56 (cut 6) │
  13 │ text column (flexed): title `T.Row` fg, one line (a link: opens the
  album, or the now playing view when there is no album), 2 below the artists
  `T.Caption` fg2 (the first artist is a link; links wash 4 × 1 around their
  letters, cut 3), 5 below a facts line in `MonoSm` spaced caps, tracking
  1.2: the provider glyph 12 in its brand ink, 6, the provider's name, and,
  when it fits, a 3 × 3 fg3 square and the queue place ("04/12"); a narrow
  zone drops the place before the name │ 8 │ like (small quiet icon plaque;
  liked = filled heart in `Accent.Ink`, outline fg2) │ 4 │ download (small
  quiet icon plaque; while downloading a 2 dp lapis rule runs along its foot
  over a guide wash and its tip says "Downloading 42%"; saved shows
  `download_done` in jade with the tip "Saved"; failed "Download failed.
  Retry"). Local files and radio hide like and download.
- **Centre**, vertically centred stack: transport row 40, 6, seek row 24.
  Transport: shuffle, previous, play/pause, next, repeat, **13 apart**. All are
  small icon plaques (32) except play/pause, a normal gold `IconPlaque` (40).
  Shuffle and repeat are quiet when off and `PlaqueOn` when on; repeat one
  shows `repeat_one`. Seek row: elapsed time in `T.Mono` fg3, right aligned,
  8, the seek scrubber (flexed), 8, the duration, left aligned; both boxes
  take the width of the longer time's zero-filled template, measured once per
  digit count, so Martian Mono never clips; "--:--" and a disabled scrubber
  while the duration is unknown. A live stream replaces the scrubber and
  times with a jade `kit.Claim` "Live".
- **Right zone**, right aligned: lyrics, queue, now playing (small icon
  plaques, 4 apart, `PlaqueOn` while their panel or view is open; lyrics is
  disabled for radio) │ 13 │ mini player │ 13 │ volume icon │ 4 │ volume
  scrubber │ 13 │ sound. `fitPlayerBar` fits the zone: the scrubber 120, then
  72, then the now playing plaque goes, then the major gaps 13 → 8, then the
  side zones borrow from the centre down to a 220 transport. The volume icon
  reflects the level (`volume_off` at 0, `volume_mute` under 34 %,
  `volume_down` under 67 %, `volume_up` above) and toggles mute, restoring the
  previous level or 70. A volume change from anywhere but the pointer (mute,
  wheel, another client) is chased (`anim.Chase`); a drag shows the pointer's
  value exactly.

Every icon plaque draws its glyph through `widgets.PressGlyph`: it sinks 1 and
scales to .86 over `Motion.Press` (OutCubic) while the plaque grows its cut by
4 and sinks 1. A like pops the heart: it scales up to 1.24 and back over
`Motion.Field` (OutCubic) while `look.Fill` fills it; an unlike only empties
it, and offers Undo ("Removed from liked songs.").

Scrubbers (look.Scrubber): 24 high hit area across the full length; tracks
end square. Seek uses a 2 thick track and a 3 × 14 vertical blade
handle (cut 0, fg) that fades in on hover or drag without changing the track
thickness; its fill is `Accent.Ink` while playing, fg2 while paused. Volume
uses a 2 thick track at rest, 3 on hover, and an always visible 10 square
handle cut 3 (a circle in the round designs). The track ink is
`Mix(ground, fg, .25)` so it reads on the one paper in every design. Press
anywhere jumps there and grabs the pointer (`pointer.GrabCmd`) so the drag
continues outside the 24 band until release. A bubble (stone4, cut 4,
`T.Mono`) floats 8 above the pointer while hovering or dragging, clamped
inside the scrubber: the time under the pointer for seek, the percentage for
volume. Hover fades over `HoverIn` / `HoverOut` (chisel). Seek previews while
dragging and commits on release; the position then holds at the committed
value until the daemon reports past it, at most 3 s (no jump back). Volume
sends `set_volume` at most every 50 ms while dragging and persists on release.
Wheel or trackpad over volume: ±5 per notch, trackpad deltas accumulate to
the same step, a wheel change commits after 300 ms; over seek: ±5 s per notch.
Keyboard when focused: Left/Right step.

**Wave progress** (`ScrubberStyle.Wave`, every seek bar: player bar, mini,
compact player). Android's wavy progress, chiselled: the played part is a
zigzag, a triangle wave with sharp vertices, never a sine.

- Played part: a zigzag 2 thick (perpendicular), amplitude 3, wavelength 20,
  in the fill colour. Build it as one filled polygon: the zigzag's points
  shifted up and down by `thickness / 2 × √(1 + slope²)`, which keeps parallel
  edges and sharp mitred vertices. Clip it at the track start and at the
  played position with a vertical cut.
- Unplayed part: a flat track 2 thick in the track ink, starting 3 after the
  played end while the wave has amplitude, square at both ends.
- While playing, the wave travels toward the played end at one wavelength per
  1200 ms. Pausing, buffering, a drag or reduced motion flattens it: the
  amplitude follows a spring (ω = 4 / `Motion.Field`) to 0, then the phase
  stops and the bar is a flat 2 stroke that asks for no frames. Resuming
  brings the amplitude back; a wave that first appears while music plays
  starts moving.
- Hover keeps the wave at 2 thick and shows the 3 × 14 blade handle; dragging
  flattens it so the position is exact.
- The compact player's top edge uses the same wave at 1.5 thick, amplitude
  1.5, wavelength 12, unplayed in fg3; it is not interactive.

Progress drawing: while the wave travels it asks for its next frame 1/30 s
ahead (1/60 s while its amplitude eases in or out; a visible visualizer drives
60 a second anyway). When the wave is flat (paused, reduced motion, wave
turned off) it asks for its next frame only when the fill would move by one
pixel (`duration / trackPx`, at least 250 ms) and when the elapsed second
changes. Paused or hidden, it asks for none.

**Sound popover** (chrome/sound.go): a `look.Overlays` entry anchored to the
sound plaque's window rectangle, placed above the bar, right edges aligned
and clamped 8 inside the window. 340 wide, stone4 with a vein hairline, cut 8.
"Sound" in `T.Section`, 13, the presets (Flat, Slowed and reverb, Nightcore,
Bass boost, Wide) as `look.FlowChoice` chips that wrap, 21, five rows 8 apart
of label `T.Label` fg2, value `T.Mono` fg3 right, `kit.Slider` below: Tempo
0.25 to 2.0 ×, Pitch −12 to 12 st, Reverb 0 to 100 %, Bass −6 to 12 dB,
Width 0 to 100 %; changes debounce 80 ms into `set_audio_fx`; a quiet
"Reset"; then 13 and the Equalizer row: 64 tall, row wash and lean, cut 6,
"Equalizer" in `T.Label` over "Bypassed" or "On · <preset or Custom>" in
`T.Caption` fg3, the curve (`look.EqualizerPreview`, 120 × 48, sel, fg3 at
.6 when bypassed) at its right. Choosing it opens the equalizer and closes the
popover. The popover closes when nothing is loaded.

**Compact player** (chrome/compactplayer.go): a pane 64 high, x 8 to `W - 8`,
above the bottom nav: art 44 (cut 6) │ 13 │ title `T.Row` and artist
`T.Caption` fg2 (flexed) │ 8 │ like │ 8 │ play/pause (gold 40) │ 4 │ next. The
wave (1.5 thick, `Accent.Ink`) runs along its top edge between the cuts.
The art toggles the now playing view; the text opens the album, else toggles
the view.

### Right panel (chrome/rightpanel.go, queue.go, lyrics.go, together.go)

A pane 340 wide (above), 8 from the page while docked. A 48 head: `kit.Tabs`
Queue, Lyrics, Together, flexed, inset 13, 8, a small quiet close plaque
("Close panel"), and a line wash across the head's foot from 13 to `W - 13`.
Switching tabs swaps the body over `Motion.Move` with an 8 rise (OutCubic).

- **Queue:** ruled group heads 40 tall (`chrome/common.go` `ruledHead`): the
  name in `T.Group` spaced caps, tracking 1.4, fg3, an optional lead, a vein
  leader, a padded count in `MonoSm` fg3, and a trailing control. "Now
  playing" (lead "From <collection>", a more plaque "Queue actions"), "Up
  next" with its count and a quiet "Clear", then "Autoplay" with its count.
  Rows 56, inset 8: a 24 lead column holding the row's numeral (`MonoSm`
  fg3, padded) that crossfades into the drag grip under hover, art 40
  (cut 3), 13, title `T.Row` (fg2 for autoplay rows) and artists `T.Caption`
  fg2, then the duration `T.Mono` fg3, which a remove plaque replaces while
  hovered. A line wash separates rows from x 40. The playing row is a select
  wash with a play glyph in `Accent.Ink` (fg3 when paused) in its lead. Row
  wash and lean under the pointer. Click plays from that row; right click
  opens the item menu; dragging the grip reorders with a pointer grab and the
  press wash, applied on release with `move_in_queue`. A removed row
  collapses over `Motion.HubUnfold` (260, OutCubic) before the daemon is told;
  an added row shows an `Accent.Ink` notch (12) at its top-left that fades
  over `Motion.Notch`. The head's more menu: Stop after this track (or Keep
  playing after this track), Save queue as playlist, Clear. Empty:
  `widgets.EmptyState` "Nothing queued" / "Play something and the next tracks
  show here.", in the whole body, with no row inset around it.
- **Lyrics:** synced lines in fixed-height rows (88 in the panel, 108 on the
  stage), romanized and translated lines under each in `T.Caption`. Every
  line is laid out at the active size (`T.Section` here, `T.Sentence` on the
  now playing stage); lines that are not active are scaled about their centre
  to the resting size (`T.Proof`, `T.Section` on the stage) in fg3. When a
  line becomes active it scales up to full size and brightens over
  `Motion.Field` (chisel), and the line it replaces settles back the same way.
  The source sits 13 under the lines as "Lyrics from <source>" in `MonoSm`
  spaced caps, tracking 1.2, fg3. While resolving: "Resolving lyrics" over the
  track's title and artists. A failure: "Lyrics could not be loaded", the
  error and a "Try again" plaque.

  **Word by word** (`look.Karaoke`, while the Word by word preference is on,
  default): the active line's words flow and wrap centred (a line too long for
  its row scales down until it fits, never clipped), unsung words in fg2,
  sung words in `Accent.Ink`, and the word being sung fills from its left edge
  behind a 45° cut that follows the clock, rising 2 dp (3 on the stage) while
  it fills; sung words stay up and the line settles when it goes inactive.
  Word timing comes from the lyrics when the source times each word (TTML,
  enhanced LRC, KRC, yrc). Otherwise it is estimated from the line's own
  timing: words split at spaces (CJK runs per character), each weighted by
  its rune count plus one, sung across
  `min(gap − 120 ms, max(500 ms, 1.25 × 70 ms × total weight))` from the
  line's start, where the gap runs to the line's end time or else the next
  line's start. With the preference off the active line is whole in fg
  (still scaling). While a word fills, the lyrics ask for frames 1/30 s ahead;
  between words they schedule only the next boundary; paused, none.

  The active line sits one third from the top. Following moves the list's
  anchor (`layout.Position.First`) next to the active line at once, so the
  line is laid out on the first frame, and springs only the remaining
  distance (at most one viewport). Only real wheel, trackpad or drag input on
  the lyrics pauses following, for 5 s, and shows a quiet "Follow" plaque;
  the follow motion itself never counts as a manual scroll. A seek, a track
  change, a speed change or the surface becoming visible anchors at once. The
  clock is the playback position projected with the playback speed, the same
  clock that schedules the next line and word boundary. Clicking a line seeks
  to it. No lyrics: `widgets.EmptyState` "No lyrics for this track" (or "This
  recording is instrumental.") in the whole body, with no document inset
  around it; the mini player's square uses a bare `kit.Empty`, since it is
  not a page.
- **Together:** the Listen Together session. Groups are hairline slabs (the
  design's card plane and stroke, cut 7, inset 13); "Members", "Pending
  joins" and "Suggestions" are ruled heads with padded counts; "Room code"
  and "Host" facts in `MonoSm` spaced caps; the room code in mono.

### Now playing view (chrome/nowplaying.go)

Opened by the player bar's now playing plaque or the compact player. It takes
the page pane, widened to `W - 8` (the tree stays at its left, the docked
panel goes), drawn like every pane and filled with `Accent.Field` under the
cover backdrop (see Style). The top bar and player bar stay. Escape, Back or
the plaque closes it. It rises in like a page, and again on every track
change.

- **Head:** its own masthead (x 28, y 4, 34): "§ 07  NOW PLAYING  再生" (the
  gloss only while on), "P 08/10", the track's provider in caps and the
  clock. A colophon (`now-playing`) at its foot when the pane is at least
  600 tall. The body is the pane less the head and colophon.
- Pane width ≥ 900: left column 42 % with the art square
  `min(colW - 110, 520, body height - 103 - text height)`, cut 16, centred;
  under it, 21, title `T.Sentence` (two lines, centred), 4, artists `T.Proof`
  fg2 (links), album `T.Caption` fg3 (a link), then an act row of normal quiet
  icon plaques **8 apart**: like, add to playlist, download, start radio,
  more. Nothing in this row is gold. Right column: `kit.Tabs` Lyrics, Queue,
  then the stage (inset 13 left, 28 right); lyrics here sing at `T.Sentence`
  with the others resting at `T.Section`. An art under 120 falls back to the
  narrow layout.
- Narrower: a body shorter than 650 puts art 120 (cut 7) beside the title,
  artists and act row; a taller one stacks art `max(120, min(colW - 42, 280,
  body height - 440))` (cut 16) over them. The Lyrics and Queue tabs and their
  scroller fill the rest.
- The visualizer band runs along the body's bottom edge (see Visualizer):
  `clamp(body × .2, 88, 144)` high at ≥ 900, `clamp(body × .14, 56, 96)`
  narrower, inset 28 (21 narrower) at the sides and 8 from the bottom. The
  columns end 21 above it, so lyrics never run over bars.
- Facts appear only when they exist. Never print "Plays -", "Unknown", an
  empty album or a lone provider word.
- The art is revealed behind the 45° sweep over `Motion.Field` when the track
  changes.
- The art is the play and pause control (`widgets.PlayArt`): hovering lays a
  stone4 plate at .9 (`min(56, 2/5 of the art's shorter side)`, cut 6) with
  the action's glyph over its centre, in over `Motion.Quick` and out over
  `Motion.Move` (OutCubic); a click toggles playback, the glyph morphs to the
  next action and the plate stays 700 ms more so a click without hover still
  answers. Home's now playing card and the mini share it.
- Nothing loaded: "Nothing playing" / "Choose a track to see it here.", with
  the head still shown.

### Mini window (chrome/mini.go)

Title "Ryotunes Mini", its own window, asked for at 520 × 176 dp times the
zoom; the niri rule opens it at 520 × 176. It is a landscape strip whenever
its width is at least 1.4 times its height or its height is below the portrait
layout's 251 dp fixed rows:

- The cover is a square as tall as the window, flush with its top, left and
  bottom edges (the compositor rounds the window; the art has no cut of its
  own on those sides and a `Cut.Mid` corner on its top-right, rounded in the
  round designs), drawn by `widgets.PlayArt`.
- The column to its right has 13 insets (12 at the top for the controls). Row
  one (44): title `T.Row` fg (one line, ellipsis) and, at its right, the 24
  square expand and close controls 6 apart (`look.WindowControl`, glyph 16,
  the row wash and press, the focus ring; the mini's close does not turn).
  Row two: artists `T.Caption` fg2. The rest of the column bottom-aligns: the
  seek row (18; elapsed and duration in `T.Mono` fg3, the wave scrubber
  between), 8, then the transport (`widgets.Transport`, 8 apart) at the left
  and like, lyrics and volume at the right. Volume is the icon plaque (click
  mutes, the wheel steps ±5, its bubble shows the percentage) plus a 72
  scrubber when the column is at least 360 wide.
- At narrower forced-landscape sizes, the seek and action controls appear
  only when their complete row fits; the cover remains the play and pause
  control.
- Lyrics: the toggle crossfades the title and artists rows into the active
  synced line, sung word by word (`look.Karaoke` at `T.Label`, left aligned,
  one row: a long line scales down to .8, and past that wraps to two rows and
  hides the next line), and the next line (`T.Caption` fg3) over
  `Motion.Field`; the controls stay.
- Every blank part of the strip drags the window.
- The window is the side paper, then the pane plane under the cover backdrop,
  with the design's stroke square at its edge.

A taller window that can fit those fixed rows keeps the portrait layout: a 48
strip with the controls, title and artists centred, the seek row, the
transport (13 apart), and a 48 bottom row (like, lyrics; volume at the right),
with the art taking what is left (`clamp(availableH - fixedRows, 96, 200)`,
cut 7, centred) and going, with the lyrics toggle, below 96. The two layouts
crossfade over `Motion.Field` when a resize crosses the ratio. The window's
content always fits at any size the compositor gives and any zoom; nothing is
ever cut at an edge. Expand returns to the main window (re-creating it if it
was closed) and closes the mini.

### Visualizer (look/visualizer.go, internal/client/spectrum)

Music made visible, carved from the field it sits on. It shows only while
something plays, the visualizer setting is on and motion is neither reduced
nor in low resource mode. **Visualizer style** (`ClientPreferences.Style()`,
`env.Fx.Style`) chooses how every visualizer draws: Bars (default), Curve,
Oscilloscope or Vectorscope. Changing it crossfades the old drawing into the
new over `Motion.Field`. Its blades stay sharp in every design (R10).

- **Spectrum feed.** `internal/client/spectrum` runs `cava` (the same analyser
  the Ryoku shell and Ryotunes 1.x use) while a visible Bars or Curve
  visualizer wants it and playback is playing: 64 bands at 60 frames a
  second, `noise_reduction = 30` so it answers quickly, mono, raw ASCII on
  stdout, `method = pipewire`. Its `source` is the sink the daemon's playback
  stream links into, found with `pw-dump` (the Stream/Output/Audio node whose
  `application.process.binary` is `ryotunesd`, then its link's input node), so
  filter sinks such as Easy Effects still feed it; `auto` when that cannot be
  found. The process stops within a second of the last visualizer leaving the
  screen or playback pausing, restarts after 1.2 s if it dies, and gives up
  until the next play after three failures in 30 s. Without `cava` the
  spectrum styles never show and Settings says why. A frame older than 250 ms
  counts as silence.
- **Sample feed** (`spectrum.Scope`, `env.Scope`). The Oscilloscope and
  Vectorscope read real stereo samples: `pw-record` capturing the same sink's
  monitor (`--target <sink>` with `stream.capture.sink = true`), 48 kHz, two
  channels, f32, raw on stdout, into a ring of 8192 frames. It follows the
  spectrum feed's lifecycle rules exactly (wanted by a visible scope style
  while playing, stopped within a second after, the same restart and give-up
  rules, `Available()` false without `pw-record`), never writes to disk, and
  only one of the two feeds runs for a given style.
- **Dynamics** (`look/pulse.go`). Each frame's band levels are shaped
  `(max(level − .03, 0) / .97) ^ 1.35`, so quiet bands sink and loud ones
  stand out. A beat detector follows the bass (the mean of the lowest eighth
  of the bands, or the RMS of the sample window for the scopes) against its
  own moving average (time constant 1.2 s): a rise past 1.35 × the average,
  above .25 and at least 160 ms after the last beat, sets a kick to 1 that
  decays with a 180 ms time constant. The kick lifts every bar and the curve
  by up to 14 % (never past the lane), adds up to .25 alpha, and thickens the
  curve and oscilloscope strokes by up to 1 dp and the vectorscope points by
  .75 dp.
- **Bars** (`look.Visualizer`). Vertical blades rising from the bottom of
  their lane: square at the bottom, the top-right corner cut at 45° by
  `min(barWidth, height)`. Pitch 12 (bar 9, gap 3) in the now playing band, 9
  (bar 6, gap 3) in headers, 10 (bar 7, gap 3) in the Home card. The 64 bands
  are resampled to the bar count. Each bar follows its band with a spring
  that rises fast (ω = 4 / 50 ms) and falls slower (ω = 4 / 180 ms). While
  playing a bar is at least 2 high, so the lane reads as a floor in a quiet
  passage. Blades paint in four alpha steps by level (.55, .7, .85 and 1 ×
  the style's alpha), one path per step, so loud bands are brighter. **Peak
  caps**: a 2 thick cap rides 2 above each blade's highest point, holds
  140 ms, then falls with constant acceleration (the lane's height in 550 ms)
  until it rests on its blade; one path, one paint, in the brightest step's
  ink.
- **Curve.** The same springs, drawn as one line through the band tops across
  the lane (monotone cubic, so it never dips under the floor), 2 thick with
  square ends, and the area under it filled at a third of the line's alpha.
  One fill and one stroke.
- **Oscilloscope.** The mono mix `(L + R) / 2` over a 2048-sample window
  starting at a rising zero crossing (with a little hysteresis) so a steady
  tone stands still; one point per 2 px; height follows a peak meter that
  rises at once and falls over 600 ms (floor −40 dB) so quiet passages still
  read. A 2 thick polyline about the lane's middle, over a 1 dp vein centre
  line.
- **Vectorscope.** Each sample plotted at `X = (L − R) / √2`,
  `Y = (L + R) / √2`: mono stands vertical, wide stereo spreads sideways. In a
  square slot it sits in a diamond (a square turned 45°, 1 dp vein, with its
  two axes) and draws at most 768 decimated samples as 1.5 dp squares, the
  two previous frames kept at alpha .45 and .18 for persistence. In a lane
  wider than tall it folds `Y` upward into a half diamond 2 × lane height
  wide, centred. Header lanes are too short for it and draw Bars instead.
- **Ink.** `Accent.Ink` at alpha .70 in dark mode and .55 in light in header
  lanes, .85 and .75 in the now playing band, full alpha on the Home card's
  instrument.
- **Frames.** While anything moves (springs, falling caps, a decaying kick) a
  visualizer asks for its next frame 1/60 s ahead (60 a second even on faster
  screens; the feeds only wake a resting visualizer). Paused, bars and curve
  fall to nothing, caps settle, the scope traces flatten over `Motion.Field`,
  and then they ask for none; nothing is drawn at rest.
- **Where.** Page headers (`widgets.Header`, `widgets.Hero`): a lane along the
  field's bottom edge, the field's full width, inside the bottom padding
  (header bottom padding 40, lane 32; compact 24 and 18), drawn after the
  field and before the text, so text never sits on it. The now playing band
  (Now playing view). The Home now playing card's instrument (see Pages); the
  Home greeting has no lane of its own.

## Pages

Every page fills its pane's stage. Grids and shelves use the full content
width; a table spans it; nothing is a narrow column in a wide empty pane
except reading text (lyrics ≤ 560, empty-state body ≤ 420) and the page sheets
whose column the Hub caps (Settings and the equalizer, 960). The header field
(`Accent.Field`, cut 16 at the top) sits behind the title or hero and scrolls
with the page. Every page head is the Hub's (Page head, above): number and
gloss in the masthead, title, lead and spec strip, tategaki while the gloss is
on.

| Page | Lead | Spec strip |
|---|---|---|
| Library | "What you saved on <Provider>, and the music in your folders." (local: "The music in your folders and the playlists you keep on this computer.") | PLAYLISTS, SONGS, ALBUMS, ARTISTS, and FOLDERS once local music has loaded |
| Search | "From <Provider>." | RESULTS of the selected filter |
| Radio | "Live stations from around the world." | STATIONS, FAVOURITES |
| Downloads | "Saved songs play from their files, even offline." | ACTIVE (lit while > 0), SAVED, FAILED (lit while > 0) |
| List | none | ITEMS |
| Diagnostics | "Recent daemon and client evidence." | ENTRIES, ERRORS (lit while > 0), WARNINGS |
| Equalizer | "Shape everything Ryotunes plays. Drag a node or a fader, and double-click the curve to add a band." | STATE, PRESET, BANDS, PREAMP, PEAK |
| Settings | "How Ryotunes signs in, plays, keeps your music and wears the desktop's look." | SECTIONS 12, CHANGED |

- **Home** (`pages/home/`): the greeting `T.Sentence` 34/40 by time of day
  ("Still up" before 4, "Good morning", "Good afternoon" from 12, "Good
  evening" from 17, "Good night" from 21), with ", <name>" when the account
  has one; the lead "Picked from <Provider> for you", or, signed out, "Sign in
  to shape your <Provider> home" with a small line plaque "Sign in" at the end
  of the title line that starts the active provider's sign-in right there.
  No visualizer lane and no spec strip; the tategaki 表紙. Under the lead: 8,
  Ryoku's motto 力と美のために in `T.Caption` fg3 while the gloss is on, 8,
  the motto tick ruler (`kit.TickRuler`, 233 wide, ticks every 8, its first
  21 × 2 segment in sel), then the now playing card. Then the chips row:
  `kit.Filters` with "All" first and the provider's chips after it. All shows
  the quick grid: up to 8 quick tiles (pins, picks, then recent), 4 columns
  at content width ≥ 1100, 3 at ≥ 820, else 2, 8 apart, then shelves (Jump
  back in, Picked for you, then the provider feed's shelves) as numbered
  ruled heads, then **Your listening**. Any other chip replaces everything
  under the row with that chip's shelves (`get_home` with the chip's params):
  a shelf skeleton while it loads, `widgets.EmptyState` "Nothing here yet"
  for an empty answer, and on failure a notice "Could not load <chip>" with
  the selection back on the previous chip. The page consumes the filter's
  change right after laying it out, in the same frame.

  **Now playing card.** While a track is loaded (playing, paused, loading, or
  finished but still in the player, exactly when the player bar shows it),
  the greeting's field grows a card under the ruler, so greeting and card
  share the accent field and scroll with the page. With nothing loaded there
  is no card and no placeholder. The card's height unfolds over
  `Motion.HubUnfold` (260, OutCubic) and its contents fade and rise 8 over
  `Motion.Navigate` when a track first loads; it folds away when the player
  empties.

  - The design's card plane (`Planes().Card` and its stroke) across the
    content width, 21 above it, 13 padding, its top-left and bottom-right
    corners cut by `Cut.Mid`, under the cover backdrop.
  - Content width ≥ 720: art 144 (`widgets.PlayArt`, cut 7) │ 21 │ text
    column (flexed) │ 21 │ instrument. The text column's top: a state line (a
    flake and a word in `T.Label`: "Playing" with an `Accent.Ink` flake,
    "Paused" amber, "Loading" lapis; then "· From <collection>" in fg3, a
    link, when the queue names its source), 8, the title `T.Sentence` fg (one
    line, ellipsis; click opens the now playing view), the artists `T.Proof`
    fg2 (each a link) and the album `T.Caption` fg3. Then 13, the seek row
    (mono times and the wave scrubber, at most 420 wide), 8, the act row:
    `widgets.Transport{Basic: true}` (previous, play in gold 40, next), 21,
    then like, queue (`PlaqueOn` while the queue panel is open), lyrics
    (`PlaqueOn` while lyrics are open) and now playing, small quiet icon
    plaques 4 apart. At the tight edge of this layout, the now playing plaque
    goes first and then lyrics rather than clipping the act row.
  - The instrument: `widgets.Visualizer` in a slot as tall as the art and
    `clamp(cardW × .3, 200, 360)` wide, a square of the art's height for the
    Vectorscope, on a stone2 plate (cut 6, 1 dp vein); the style's short name
    ("BARS", "CURVE", "SCOPE", "STEREO") in `MonoSm` spaced caps, tracking
    1.2, fg3, at its top-left. Clicking it cycles the style and saves it like
    the Settings row; its tip is "Visualizer: Bars · click to change". With
    the visualizer off, unavailable or motion reduced, the slot goes and the
    text column takes the room.
  - Content width 600–719: art 112, no instrument. Narrower: art 72, title,
    artists and the gold play plaque only.
  - A track change reveals the new art behind the sweep (`Motion.Field`),
    swaps title and artists with an 8 rise (`Motion.Navigate`, OutCubic), and
    the accent retints the field and the play plaque.

  **Your listening** (`pages/home/insights.go`), in the All view only: a
  ruled head "Your listening" marked "14 days", then a datasheet (keys in
  `MonoSm` spaced caps fg3, one vein column rule 13 after the widest key,
  values `T.Detail` fg): Plays this week, Time listening, Top artist, Top
  song, each only when it is known. Then, 21 below, two readings side by side
  (one per row when narrow): "Plays" and "Listening time", each a name, a
  figure in `T.Section`, a `kit.EngravedChart` 44 tall over 14 days (ticks on
  the first day, the middle and "Today") and a proof ("Last 14 days, most on
  Tuesday, Nov 10", "About 31 min a day"). The figures count up once over
  `Motion.CountUp` (640) when the data first arrives and roll on later
  changes; each chart is recorded once per revision and width. The series
  comes from the daemon's `listening_days {days: 14}`; a daemon without it
  draws no chart, and fourteen days without a play show "Play something and
  your days draw here." Nothing is ever invented.
- **Search results** (`RouteSearch`): title "Results for “q”", then
  `kit.Filters` All, Songs, Albums, Artists, Playlists with counts. All: at
  content width ≥ 1000 the top result slab (art 120, name `T.Section`, kind in
  `T.Group` spaced caps, a gold "Play" plaque) beside the first four songs
  (40 / 60 split, 34 apart); narrower, stacked. Group labels "Top result" and
  "Songs" in `T.Label` fg2. Then numbered shelves for Artists, Albums,
  Playlists. A filter shows a track table (Songs) or a card grid. No results:
  "No results for “q”" / "Nothing on <Provider> matches. Try another
  spelling, or switch provider."
- **Library:** `kit.Tabs` Playlists, Songs, Albums, Artists, Local (the tab is
  the masthead's part); at the tab row's right a filter icon plaque
  (`PlaqueOn` while open) that opens a 280 `kit.Search` narrowing the current
  tab (Ctrl F), and on Playlists a quiet "Import playlist" icon plaque and a
  line "New playlist" plaque. Playlists: grid with Liked songs first. Songs:
  track table. Albums and Artists: grids. Local: a ruled head "Watched
  folders" with a padded count and a quiet "Add folder", hairline-separated
  folder rows (path in mono, "03 songs" in `MonoSm` caps, quiet Rescan and
  Remove), then the local track table.
- **Album / playlist:** hero in the field, under its cover's backdrop: art 200
  when the hero is at least 760 wide, 160 at 560–759, otherwise 120; cut 7;
  21; a bottom-aligned column whose natural height may make the hero taller
  than the art: kind in `T.Group` spaced caps, tracking 1.2, fg2 ("ALBUM",
  "EP", "PLAYLIST", "SMART PLAYLIST"...), 4, title `T.Name` (falls to
  `T.Sentence` under 640 content width or in the compact class; at most two
  lines), 8, meta `T.Proof` fg2. Meta wraps only between whole facts, uses at
  most two lines and drops trailing facts that do not fit; the artist is a
  link and counts and durations are mono ("Spiritbox · 2021 · 12 songs · 43
  min"). Then 13 and the full-size act row 8 apart: Play (gold `Plaque` with
  the play icon, "Pause" while this collection plays), Shuffle, Save/like,
  Download (icon plaques, quiet) and More (playlists add a sort choice at
  width ≥ 800). When space runs out, fold Download, Save/like and Shuffle, in
  that order, into More; Play and More always remain. Then the track table and
  a "Loading the rest of the album" line while it pages; a failure there is a
  jasper `T.Detail` sentence with a quiet Retry.
- **Artist:** the same hero with the portrait octagon-cut (a circle in the
  round designs) and the act row Play (gold), Shuffle, Subscribe (line plaque
  "Subscribe"/"Subscribed", YouTube artists only, through
  `set_artist_subscribed`), Radio, More (folded actions and "Copy artist
  link"). Then the ruled head "Popular" (five table rows and a quiet "Show
  all"), then numbered shelves: Albums, Singles and EPs, Videos, Featured on,
  Fans also like.
- **List:** the page head (the route's title, or "Browse") over a card grid
  for "Show all".
- **Radio:** `kit.Filters` Popular, Favourites, Recent, By country, By genre;
  a field "Filter stations" that narrows stations by name lives in the filter
  row (it filters this catalogue, it is not search); numbered country and
  genre heads; a station grid of 88 tiles on the design's card plane with the
  Hub's hover (favicon 56, name `T.Label`, country and codec/bitrate in
  `MonoSm` fg3). The playing station shows the `volume_up` glyph in
  `Accent.Ink`; a favourite's heart stays, others fade in on hover.
- **Downloads:** `kit.Filters` All, Active, Saved, Failed with counts, a quiet
  "Open folder", and outside compact a quiet "Clear history" (behind a
  confirm) and "Settings"; `kit.Tablet` rows with art in the tablet's mark,
  title and artists, claim (lapis Downloading with `kit.BladeProgress`, jade
  Saved, jasper Failed with the reason as proof) and one action: Cancel while
  active, Retry when failed or cancelled, and **Play** on a saved row. Double
  clicking a saved row also plays it, from its file on disk with no network;
  Show in folder lives in the row's right click menu (Play, Show in folder). A
  collection card's action plays its saved tracks in order. At content width
  ≥ 1000 a 300 wide ruled "Storage" head and facts slab on the right:
  location, used space, format.
- **Settings:** see Settings.
- **Diagnostics:** reached from Settings → Diagnostics. Ruled heads "Runtime"
  and "Log entries" (01 / 02, 02 / 02), "Open logs folder" and "Copy all".
- **Equalizer** (`RouteEqualizer`, from the Sound popover and Settings →
  Sound): a parametric equalizer of up to 20 bands, after the one the
  listener asked for (Astra's): a response graph with draggable nodes over a
  strip of faders. Model, presets, the Equalizer APO format and the response
  maths live in `internal/model/equalizer.go` and `internal/eq`.

  - Page sheet: a column of at most 960, centred, 8 from the masthead. The
    title row: a small quiet back plaque (icon only, "Back"), 8, "Equalizer"
    at 32/40; at its right, when the head is at least 760 wide (else under
    it), a `kit.Toggle` whose word is "On" or "Bypassed", 13, the preset
    plaque (a small line plaque with the preset's name or "Custom" and a
    chevron, opening a `look.Overlays` `kit.Menu` 260 wide: the built-ins, a
    divider, each saved preset with a danger "Remove <name>", a divider, "Save
    as preset"), and small quiet plaques Import, Export and Reset, 6 apart.
    The back plaque, like Escape, goes back to where the equalizer was opened
    from; when the equalizer is the first page (restored at launch) it opens
    Settings at Sound instead. Then the lead and the spec strip, and the
    tategaki 調音 while the gloss is on.
  - Sections: "Response" (01 / 02) holds the graph and the headroom claim,
    "Bands" (02 / 02) holds the strip, both on `look.EQCard`: the design's
    card plane and stroke at `Cut.Mid`.
  - Graph: `clamp(H × .32, 184, 320)` high, plot inset 38 / 13 / 16 / 30.
    Frequency lines at 20, 50, 100, 200, 500, 1k, 2k, 5k, 10k and 20k Hz and
    gain lines at +12, +6, 0, −6 and −12 dB as line washes, 0 dB in vein;
    labels `MonoSm` fg3 under 4 dp ticks and at the left. The summed response
    (`eq.Curve` over 384 log-spaced frequencies) is a 2 thick line in sel
    over a 45° hatch (6 apart, 1 dp, fg at .3) between it and 0 dB, the hatch
    recorded once. Bypassed, the line fades to fg3 at .7 over `Motion.Field`
    (OutCubic), the hatch greys, and "Bypassed" with an amber flake stands at
    the card's top right. While playing, the spectrum feed's bands lie behind
    on the log axis as sharp line-wash blades.
  - Nodes: a 12 diamond per band (shapes without gain sit on the curve),
    hollow with a rim in the curve's ink; disabled bands have an fg3 rim; the
    selected band's node is filled sel and a register ring lands around it
    over `Motion.Move`. Hover grows a node by 30 % and draws guide lines to
    both scales (120 in, 240 out, OutCubic) and a bubble "1.2 kHz · +3.5 dB ·
    Q 1.40". Dragging (with a pointer grab) moves frequency on the log axis
    and gain within ±12; a dragged or keyed node glides after the value
    (`anim.Chase`), the value itself exact (R2). The wheel over a node steps Q
    by ×1.1 a notch (trackpad deltas accumulate); a double click on empty
    graph adds a peak band there while fewer than 20 exist; right click on a
    node opens its menu: the seven shapes, Enable or Disable, Remove.
  - Strip (look.HScroller when it overflows): Preamp first (a 64 column: a
    vertical fader −24 to +12 dB, its value in mono above, "PRE" under in
    spaced caps), a vein hairline, 13, then a column per band whose controls
    stay 56 wide and centred in it: gain in `MonoSm` above, a vertical fader
    (`look.Fader`: a 4 field track that brightens on hover, the stretch from
    0 dB in fg2, an engraved scale with a long tick at 0, a 20 × 8 sel handle,
    a lozenge in Lithic and a pill in the round designs, growing on hover;
    double click resets the gain to 0), the frequency in mono ("1.2k"), a
    32 × 28 shape button outlined in vein with the row wash (glyphs drawn in
    `look`) that opens the shape menu, and "Q 1.40" that steps with the wheel
    or a vertical drag. Every number is centred over its fader and rolls on a
    discrete change, following a held control exactly. The band columns share
    the strip's width: at least 60 apart, at most 180, and only when 60 apart
    does not fit does the strip scroll, with step plaques at its ends that
    page over `Motion.Move` (OutCubic). A column unfolding or collapsing over
    `Motion.HubUnfold` counts by its progress, so the others narrow or widen
    with it. Each column has the row wash; the selected column wears the
    select wash, travelling over `Motion.Move`. After the strip, outside the
    scroller so it is always in reach, a quiet "Band" plaque adds a peak band
    in the widest gap; at 20 bands it is disabled with the tip "Twenty bands
    is the most".
  - Headroom: when the curve's peak is above 0 dB and the equalizer is on, an
    amber claim "May clip", "Peaks reach +3.2 dB, so loud passages may clip."
    in `T.Detail` fg2, and a quiet "Lower preamp" that lowers the preamp by
    that much. No claim otherwise.
  - Keys with the graph focused: ← → move the selected band a sixth of an
    octave, ↑ ↓ change its gain by 0.5 dB (Shift 0.1), [ and ] change Q,
    Delete or Backspace removes it, Tab and Shift Tab select the next and
    previous band.
  - Edits change a local working copy at once, so the curve never lags the
    pointer, and reach the daemon as complete `set_equalizer` states at most
    every 80 ms while dragging and once on release. An `equalizer` event
    replaces the working copy unless a drag, a preset glide or a pending write
    is in flight. Any edit clears the preset name.
  - Choosing a preset, Reset and Undo glide every node and the curve to the
    target over `Motion.Move` (240, OutCubic) and roll the readouts once.
  - Import opens a `kit.Dialog` 640 wide with a mono field ("Paste an AutoEQ
    or Equalizer APO parametric profile") and an optional "Save as" name;
    `import_equalizer` applies it and its warnings show in a notice
    ("Imported 10 bands. Skipped 1 line."). Export copies the Equalizer APO
    text to the clipboard ("Copied the profile"). Save as preset and Remove
    saved preset are 520 wide dialogs; Save and Import are gold, Remove is
    danger, Cancel is quiet.

Page states (`widgets/states.go`): `widgets.Loading` (the work pattern:
nothing for a second, then `kit.Working` with "Loading" or "Loading the
tracks" over vein outlines of the page's shape, past 10 s a blade and a quiet
"Go back"), `widgets.Failed` ("This page did not load" in `T.Section`, the
daemon's message in jasper `T.Detail`, a quiet Retry), `widgets.EmptyState`
(glyph 24 fg3, title `T.Section`, body `T.Body` fg2 at most 420, actions 8
apart). A page state owns its spacing: it sits the page's content inset (34 /
21 / 13 by class) below whatever is above it (hero, header, filter row; that
element drops its own bottom gap) and never touches a pane edge or a hero;
when it is the only thing under a hero it is centred in the remaining height,
its top no closer than that inset, and scrolls with the hero when that height
is too short. Callers pass no insets of their own. A loading shape is anchored
where its content will appear, never centred: a whole-page `widgets.Loading`
sits top-start at the content inset, and `LoadingHero` is the full-bleed hero
at its natural height, flush. Inside a list, the page lays `widgets.Skeleton`
out in place of the rows it stands for, with their insets and gaps, so
nothing moves when they arrive.

## Widgets (widgets/)

```go
// TrackTable is a header row and track rows inside the page's Scroller.
type TrackTable struct{ ... }
type TrackColumns uint8 // bit set: ColArt, ColArtist, ColAlbum, ColPlays, ColAdded
type TrackTableOpts struct {
    Columns  TrackColumns
    Context  string        // menu context: playlist id, "album", "queue", "library"
    SourceID, SourceName string // for PlayCollection
    SourceItems   []model.Track // the whole collection a filtered table plays
    SourceIndices []int
    DisplayItems  []model.Track // the visible rows; empty columns are dropped
    Continuation  *string
}
func (t *TrackTable) Header(gtx layout.Context, e *env.Env, o TrackTableOpts) layout.Dimensions
func (t *TrackTable) Row(gtx layout.Context, e *env.Env, items []model.Track, i int, o TrackTableOpts) layout.Dimensions
```

Rows are `th.RowHeight()` high (56 comfortable, 48 compact). Columns: index 48
(the row's ordinal in `T.Mono` fg3, zero padded to the list's digits, `01` to
`128`, sel on the selected row; a play glyph while the row is hot, pause in
`Accent.Ink` on the playing row; a static `volume_up` in `Accent.Ink` when
playing at rest) │ Title flexed (art 40 when `ColArt`, 32 in compact rows,
cut 3, 13, a provider glyph 14 in mixed lists, title `T.Row` (`Accent.Ink`
when playing) with an explicit mark, and the like heart inline after the
title: shown when liked, hovered or focused) │ Artist 21 % │ Album 27 % │
Plays 108 `T.Mono` (only when some row has plays) │ Added 132 │ Length 72
`T.Mono` right aligned │ 40 more plaque (hot rows). Album pages use Title
66.5 % and Artist 33.5 % and drop Artist when every row matches the album
artist. Under 900 content width the Album column goes; under 720 Plays goes;
under 560, Title carries the artists under it and only Length remains on the
right. Header row 32, labels in `MonoSm` spaced caps, tracking 1.2, fg3, a
1 dp vein under it. Hover is the row wash with the 2 dp lean; the index, like
and more appear at once. Single click selects (the select wash, travelling;
the first selection lands); double click or Enter plays from that row; ↑ ↓
move the selection; clicking the index play icon plays; right click opens the
item menu. Unliking from the row's heart offers Undo.

```go
type Card struct{ ... }      // art (square, or octagon for artists), title 2 lines, subtitle 1 line
func (c *Card) Layout(gtx layout.Context, e *env.Env, item model.BrowseItem, width unit.Dp) layout.Dimensions
type Shelf struct{ Number, Of int; ... } // ruled head with its ordinal + look.HScroller of cards
func (s *Shelf) Layout(gtx layout.Context, e *env.Env, title string, items []model.BrowseItem, more *env.Route) layout.Dimensions
type Grid struct{ ... }      // wrapping cards, columns from width (card 160 to 200)
func (g *Grid) Row(gtx layout.Context, e *env.Env, items []model.BrowseItem, row int) layout.Dimensions
func (g *Grid) Rows(width int, count int) int
type QuickTile struct{ ... } // home quick grid tile, 56 high
func (q *QuickTile) Layout(gtx layout.Context, e *env.Env, item model.BrowseItem) layout.Dimensions
type Hero struct{ ... }
type HeroContent struct {
    Art string; Octagon, Collage bool
    Kind, Title string
    Meta []kit.Span        // the meta line; numbers Mono
    MetaAction func()      // the first fact as a link
    Acts layout.Widget     // the act row, built by the page with HeroActions
    Place sheet.Place; PartGloss string
}
func (h *Hero) Layout(gtx layout.Context, e *env.Env, c HeroContent) layout.Dimensions
type HeroActions struct{ ... } // folds trailing actions into More as space runs out
// Header is retained per page: it owns the head's visualizer lane, its rolls
// and its tategaki.
type Header struct{
    Sentence, NoLane bool; Below layout.Widget
    Place sheet.Place; Part, PartGloss, State string
    Specs []sheet.Spec; Preview layout.Widget
}
func (h *Header) Layout(gtx layout.Context, e *env.Env, title, sentence string, trailing layout.Widget) layout.Dimensions
func (h *Header) Masthead() (part, partGloss string)
func RuledHead(...)                   // title, vein rule, NN / NN mark, trailing
func Loading(e *env.Env, shape LoadingShape) layout.Widget
func Skeleton(e *env.Env, shape LoadingShape) layout.Widget
func Failed(e *env.Env, message string, retry *kit.Plaque) layout.Widget
func EmptyState(e *env.Env, icon *widget.Icon, title, body string, actions ...*kit.Plaque) layout.Widget
type ItemMenu struct{ ... } // item menus per kind: kit.Menu inside a look.Overlays entry at the press point
type AddToPlaylistDialog struct{ Open bool; Tracks []model.Track }
type EditPlaylistDialog struct{ Open bool; Page *model.PlaylistPage }
func Duration(seconds float64) string // m:ss or h:mm:ss
func Count(n int) string              // 1.2K, 3.4M
```

Card: art square at the card width (cut 7; artists octagon), 8, title
`T.Label` fg two lines, subtitle `T.Caption` fg3 one line ("Album · 2021",
"Artist", "Playlist · 42 songs"). Hover: the design's card plane (cut 7, with
its stroke) fades in with the row wash behind the card, reaching 8 past the
art and the copy on every side; the card's resting box and its art's
alignment with the section head do not move, and shelves and grids leave the
plane that room so it is never clipped. Every hover plane in the app keeps its
content at least 8 from its edges (quick tiles keep their art flush, as the
art is the plane's edge). The **play plate** (40, stone4, cut 6, fg play
glyph) fades in rising 4 dp at the art's bottom right, 8 inset; under its own
hover it turns gold with `onGold` ink, and a press grows its cut by 4 with the
glyph at .86. Clicking it plays the item; clicking the card opens it; right
click opens the item menu. Card widths: 176 expanded, 152 medium, 136
compact; shelves space cards 21 apart.

Quick tile: 56 high on the design's card plane, cut 6 on the top-left corner,
art 56 flush left, 13, title `T.Label` two lines; hover lays the row wash over
it and fades in a 32 play plate at the right, 12 inset.

Item menu: a head with the kind in `MonoSm` spaced caps ("SONG", "IN THE
QUEUE", "ALBUM"), the title and a detail over a vein rule; rows 40 tall,
glyph 20, `T.Label`, the row wash and lean; danger rows in jasper after a vein
rule; ↑ ↓ wrap, Home and End.

Dialogs: `kit.Dialog` 520 wide (the owned playlist editor and the equalizer
import 640), the one consequential action gold, Cancel quiet, destructive
actions danger. Add to playlist lists "Your
playlists" under a ruled head with a padded count, rows 44 with the row wash.

Shared playback pieces, one implementation each:

```go
// Transport: shuffle, previous, play (gold, morphing), next, repeat; Basic keeps
// previous, play and next. Gap defaults to 13. Every button has a tip.
type Transport struct{ Basic bool; Gap unit.Dp }
func (t *Transport) Layout(gtx layout.Context, e *env.Env, snapshot state.State) layout.Dimensions
// PlayButton is the gold play plaque whose glyph morphs and presses.
type PlayButton struct{ Plaque kit.IconPlaque; ... }
func (b *PlayButton) Layout(gtx layout.Context, th *look.Theme, icon *widget.Icon, description string) layout.Dimensions
func PressGlyph(...)       // a plaque's glyph sinking 1 and scaling to .86 while pressed
func DrawPressedGlyph(...)
func PlaqueInk(...)        // a plaque kind's glyph ink

// Seek: elapsed, the wave scrubber and the duration; previews while dragging,
// commits on release and holds until the daemon passes it. Radio shows "Live".
type Seek struct{}
func (s *Seek) Layout(gtx layout.Context, e *env.Env, snapshot state.State, track model.Track) layout.Dimensions
func (s *Seek) Reconcile(gtx layout.Context, e *env.Env, snapshot state.State, trackID string)
func ProgressFrame(gtx layout.Context, e *env.Env, duration float64, trackPixels int, paused bool)
func PlaybackDuration(reported float64, value *string) float64

// PlayArt: the loaded track's cover as its play and pause control.
type PlayArt struct{ View look.ArtView; ... }
func (p *PlayArt) Layout(gtx layout.Context, e *env.Env, url string, size image.Point, shape look.Shape, fade time.Duration) layout.Dimensions
```

## look

```go
package look

// Theme is the library theme (with the desktop's Design) plus the app's
// accent, source colours and the desktop reading.
type Theme struct {
    *theme.Theme                 // R, T, Cut, Sp, Motion, Density, Shaper, Design
    Src          Source
    Accent       Accent
    Desktop      palette.Desktop
    Class        Class            // the window's size class, set by the shell each frame
}
type Accent struct{ Fill, On, Ink, Field color.NRGBA }
type Source struct{ Provider model.Provider; Fill, Ink, Field color.NRGBA }
type Class uint8 // Compact, Medium, Expanded
func ClassFor(widthDp float32) Class

func NewThemeFor(desktop palette.Desktop, density theme.Density, provider ...model.Provider) *Theme
func NewTheme(roles palette.Roles, density theme.Density, provider ...model.Provider) *Theme // tests and galleries: Lithic, gloss on
func (t *Theme) SetDesktop(d palette.Desktop)                  // snaps
func (t *Theme) SetRoles(roles palette.Roles)                  // snaps
func (t *Theme) LoadedRoles() palette.Roles
func (t *Theme) ArtScheme(cover art.Palette, tinted bool) (palette.Roles, Accent)
func (t *Theme) ArtAccent(cover art.Palette) Accent
func (t *Theme) SetScheme(gtx layout.Context, roles palette.Roles, accent Accent) // tweens over Motion.Field
func (t *Theme) SetAccent(gtx layout.Context, accent Accent)
func (t *Theme) SetProvider(gtx layout.Context, p model.Provider) // tweens Src over Motion.Field
func (t *Theme) Animate(gtx layout.Context)                    // advances the tweens; frames only while moving

// brand.go: the only file with colour literals.
var RyokuOrange color.NRGBA
func BrandFill(p model.Provider, r palette.Roles) color.NRGBA
func BrandInk(p model.Provider, r palette.Roles) color.NRGBA

// Marks
func RyokuMark(size unit.Dp, ink color.NRGBA) layout.Widget   // the current cut-stone 力: strokes in ink (fg), pointer in RyokuOrange
func ProviderGlyph(p model.Provider, size unit.Dp, ink color.NRGBA) layout.Widget
func ProviderName(p model.Provider) string                    // "YouTube Music", "Spotify", "SoundCloud", "TIDAL", "Local files", "Radio"

// Icons: the library's Material Symbols Sharp (package symbols), decoded once.
// Play, Pause, Next, Previous, Heart and Explicit are the Fill variants.
var Icon struct{ Play, Pause, Next, Previous, Shuffle, Repeat, RepeatOne,
    Heart, HeartOutline, Download, DownloadDone, Queue, Lyrics, NowPlaying,
    Mini, Expand, VolumeOff, VolumeMute, VolumeDown, VolumeUp, Tune, Back,
    Forward, Search, Filter, Close, More, Add, Home, Library, Radio, Downloads,
    Settings, Minimize, Maximize, Restore, Check, Drag, Remove, Folder, Person,
    Album, Playlist, PlaylistAdd, Refresh, Share, OpenInNew, Sparkle, Explicit,
    Equalizer, Sort, ChevronDown, Sidebar,
    HomeFill, LibraryFill, DownloadsFill, SettingsFill, FolderFill, SidebarFill *widget.Icon }
func Filled(icon *widget.Icon) *widget.Icon // the current destination's filled form, else icon

// Frame geometry, in dp.
const (TopBar = 56; Gap = 8; SidebarExpanded = 272; SidebarMedium = 72
    Panel = 340; PlayerBar = 88; PlayerCompact = 64; BottomNav = 56
    ContentMinWithPanel = 520)
func ContentInset(c Class) unit.Dp // 34, 21, 13

// Washes and motion (the Hub's)
type RowWash struct{ ... }   // hover 120 in / 240 out OutCubic, 2 dp lean, press wash, glyph .86
func WashInk(roles palette.Roles, hover, press float32) color.NRGBA
type SelectWash struct{ ... } // one travelling Roles.Select wash per group, landing between groups
type PageRise struct{ ... }   // page change: fade 240, 14 dp from the right over 300, OutCubic
type Drawer struct{ ... }     // a sheet sliding over Motion.Move OutCubic above a fading scrim
type CountUp struct{ ... }    // a figure running up once over Motion.CountUp
type CountRoll struct{ ... }  // a changed figure or word rolling in over Motion.Move OutCubic
type Chevron struct{ ... }    // a quarter turn on OutBack 1.6
type CloseTurn struct{ ... }  // the close glyph's quarter turn on hover
func EaseOutBackChevron(t float32) float32 // OutBack 1.6
func EaseOutBackNode(t float32) float32    // OutBack 2
type SidebarPlaque struct{ ... }
type WindowControl struct{ Icon *widget.Icon; Description string; Close bool; Size unit.Dp }
type FlowChoice struct{ ... } // short choice sets that wrap onto more lines
func FillArt(gtx layout.Context, th *Theme, size image.Point, shape Shape, ink color.NRGBA) layout.Dimensions

// Scrolling (app-owned: see Library limits). Every delta applies at once,
// with no easing: Wayland deltas times gtx.Metric.PxPerDp; on X11 (no
// WAYLAND_DISPLAY) a 10-unit notch scrolls 48 dp. Keep the pass-through
// catcher with its own clip, directional ScrollRange at the ends and
// fractional accumulation. The thumb is 3 wide, fg3, in the design's shape,
// shown while scrolling or hovered and fading out over hover out.
type Scroller struct{ layout.List; ... }
func (s *Scroller) Layout(gtx layout.Context, th *Theme, count int, el layout.ListElement) layout.Dimensions
func (s *Scroller) ScrollTo(i int)
func (s *Scroller) AnchorTo(index, offsetPx int)
func (s *Scroller) GlideTo(index int, d time.Duration) // OutCubic, laying out every row it passes
func (s *Scroller) SpringBy(px float32)
func (s *Scroller) Offset() (first, px int)
func (s *Scroller) Progress() float32 // how far the list is read, -1 when it fits
func (s *Scroller) UserScrolled() bool
type HScroller struct{ ... }  // same input rules horizontally; Page(±1) glides a viewport over Motion.Move OutCubic
func (s *HScroller) Layout(gtx layout.Context, th *Theme, count int, el layout.ListElement) layout.Dimensions
func (s *HScroller) Page(gtx layout.Context, dir int)
func (s *HScroller) CanPage(dir int) bool

// Scrubber: seek and volume (app-owned: the library Slider has a 21-ish
// hit band, no pointer grab and no bubble).
type Scrubber struct{ ... }
type ScrubberStyle struct {
    Fill        color.NRGBA
    AlwaysHandle bool
    Bubble      func(f float32) string
    WheelStep   float32    // fraction per notch
    Disabled    bool
    Wave, Live  bool
    Thickness, Amplitude, Wavelength unit.Dp
}
func (s *Scrubber) Layout(gtx layout.Context, th *Theme, value float32, st ScrubberStyle) layout.Dimensions
func (s *Scrubber) Dragging() (f float32, ok bool) // preview while held
func (s *Scrubber) Committed() (f float32, ok bool) // release, click or wheel

// Art: a cover from the cache at a size and shape; reveals the new cover
// behind a 45° sweep when the url changes (duration chosen by the caller).
// In the round designs Cut is a radius (artRadius) and Octagon a circle.
type Shape struct{ Cut unit.Dp; Octagon bool; Corners [4]bool }
type ArtView struct{ ... }
func (a *ArtView) Layout(gtx layout.Context, th *Theme, cache *art.Cache, url string, size image.Point, sh Shape, fade time.Duration) layout.Dimensions
func Art(gtx layout.Context, th *Theme, cache *art.Cache, url string, size image.Point, sh Shape) layout.Dimensions
// art.Cache keeps every decoded cover from the network in a disk cache under
// os.UserCacheDir()/ryotunes/art (keyed by URL and art.RequestSize, the one
// size ladder; written atomically; about 256 MB, pruned least recently read
// first to 90% once over), so a cover seen in any earlier session shows from
// disk in the first frames after launch. Without a cache directory it runs in
// memory only. A disk entry older than 30 days is still shown, and refetched
// in the background to pick up a cover that changed at the same URL. Covers
// made only of local files are not stored. Settings "Force clear caches"
// empties it (Cache.Clear) and Cache.Forget drops one URL. Network loads run
// with enough workers that one slow host cannot starve the rest, the covers
// being drawn this frame go first, and queued loads for rows that scrolled
// far away are dropped before they fetch. Cache.Palette and Cache.Backdrop
// return a cover's palette and its blurred 48 × 48 backdrop, both made once per
// cover off the frame goroutine and kept beside it on disk.

// Micro motion
type Hover struct{ Hub bool; ... } // hover of an area as a tween 0..1: library 160/100 chisel, Hub 120/240 OutCubic
func (h *Hover) Layout(gtx layout.Context, th *Theme, w layout.Widget) (layout.Dimensions, float32)
type Swap struct{ Ease func(float32) float32; ... } // keyed crossfade with optional rise for text and art changes
func (s *Swap) Layout(gtx layout.Context, th *Theme, key string, rise unit.Dp, d time.Duration, w func(gtx layout.Context, key string) layout.Dimensions) layout.Dimensions
type Morph struct{ ... }   // icon change: outgoing scales to .85 and fades, incoming from .85 (Motion.Notch)
func (m *Morph) Layout(gtx layout.Context, th *Theme, icon *widget.Icon, ink color.NRGBA, size unit.Dp) layout.Dimensions
type Fill struct{ ... }    // a glyph filling from its centre outward as a clip grows (Motion.Notch): the liked heart
func (f *Fill) Layout(gtx layout.Context, th *Theme, on bool, off, onW layout.Widget) layout.Dimensions
func Bubble(gtx layout.Context, th *Theme, at image.Point, text string, alpha float32) // deferred overlay
func Field(gtx layout.Context, th *Theme, cuts [4]int, body layout.Widget) layout.Dimensions // Accent.Field plane
func FieldAccent(gtx layout.Context, th *Theme, a Accent, cuts [4]int, body layout.Widget) layout.Dimensions
// FieldBackdrop is a field plane with a cover's backdrop; Backdrop.Paint lays
// one on any plane, toned to its colour.
func FieldBackdrop(gtx layout.Context, th *Theme, a Accent, cuts [4]int, b *Backdrop, cache *art.Cache, cover string, body layout.Widget) layout.Dimensions
type Backdrop struct{ ... }
func (b *Backdrop) Paint(gtx layout.Context, th *Theme, cache *art.Cache, cover string, field color.NRGBA, bounds image.Rectangle, cuts [4]int)

// Overlays places popovers, menus, tips and choices in WINDOW coordinates.
// The shell owns one instance, lays out the frame, then calls Layout last
// with full-window constraints. It also registers a pass-through pointer area
// over the whole window, drawn on top, that records the last press and the
// pointer position in window coordinates.
type Overlays struct{ ... }
type Placement struct {
    Prefer   Side       // Below or Above; flips when the preferred side does not fit
    Align    Align      // Start (left edges) or End (right edges)
    Gap      unit.Dp    // default tokens.SizePopoverAnchorGap
    Width    unit.Dp    // 0: the content's own width
    MinWidth unit.Dp    // at least the trigger's width, still growing
    Passive  bool       // Escape and Top skip it (search suggestions)
    Scrim    bool
    Return   any
}
func (o *Overlays) Window() image.Point               // window size in px
func (o *Overlays) Pointer() image.Point              // last pointer position, window px
func (o *Overlays) Press() image.Point                // last press position, window px
// AnchorOf returns the window rectangle of a clickable of the given size,
// from the last press (window position minus the clickable's local press
// position, widget.Clickable.History). Keyboard activation has no press: it
// returns the rectangle recorded by Mark for the same tag, if any.
func (o *Overlays) AnchorOf(c *widget.Clickable, size image.Point) image.Rectangle
func (o *Overlays) Mark(tag any, r image.Rectangle)    // chrome with known geometry records window rects
func (o *Overlays) Rect(tag any) (image.Rectangle, bool)
func (o *Overlays) Open(tag any, anchor image.Rectangle, p Placement, content layout.Widget)
func (o *Overlays) OpenAt(tag any, at image.Point, content layout.Widget) // menus at a press point
func (o *Overlays) Reanchor(tag any, anchor image.Rectangle)
func (o *Overlays) Close(tag any)
func (o *Overlays) IsOpen(tag any) bool
func (o *Overlays) Top() (tag any, ok bool)           // for the Escape stack
func (o *Overlays) Layout(gtx layout.Context, th *Theme)
// Each entry: stone4 plane with a 1 dp vein hairline, cut Cut.Menu (tips
// Cut.Tooltip), content inset 16 × 6 (tips 8 × 6), measured with the window
// as its maximum, placed against the anchor, flipped, clamped 8 inside the
// window, menu in 160 / out 100 sliding 6 in and 4 out (tips in 300, 8), an
// outside press closes it and is swallowed (it never reaches what is
// underneath), Escape closes the top entry, focus returns to the opener.

// Tip is the app's tooltip: after Motion.TooltipDelay on hover, a stone4 slab
// (cut tokens.CutTooltip, T.Caption) through Overlays, below the widget or
// above when there is no room. The Hub has no tooltips; Ryotunes keeps them
// because its transport and window controls are icon only.
type Tip struct{ ... }
func (t *Tip) Layout(gtx layout.Context, th *Theme, o *Overlays, text string, w layout.Widget) layout.Dimensions

// Choice replaces kit.Picker: a field-shaped plaque (stone3, cut 6, 40 high,
// value T.Label, a chevron, the row wash) that opens a kit.Menu of options
// through Overlays, at least as wide as itself.
type Choice struct{ ... }
func (c *Choice) Layout(gtx layout.Context, th *Theme, o *Overlays, options []string, selected int) layout.Dimensions
func (c *Choice) Changed() (int, bool)
```

`env.Env` carries `Th *look.Theme`, `Overlays *look.Overlays`, the notices
`Toast` and `Undoable`, `Confirm`, `Menu`, `Fx{Visualizer, Wave, Style}`,
`Spectrum`, `Scope`, and the desktop through `Desktop()`, `Design()`,
`Gloss()` and `ReduceMotion()`; it keeps no copy of the look. `Fx` turns the
visualizer and the wave off while motion is reduced or the daemon's low
resource mode is on, without changing the stored settings
(`shell/environment.go`).

## Micro-interactions

Every motion answers an input or a state change and stops. Durations are the
library's tokens: Lithic's (press 90, hover 160 / 100, focus 120 / 100,
select 240, field 320, notch 200, unfold 200, navigate 200, menu 160 / 100,
dialog 300 / 160, tooltip 300 after 600, placard 300 / 5000, count up 640,
reveal stagger 40) and the Hub's (quick 120, move 240, hub unfold 260,
stagger 18). The Hub's curve, OutCubic (`anim.EaseOutCubic`), carries every
Hub motion; chisel (`anim.EaseChisel`, cubic-bezier .2, 0, 0, 1) stays on the
library's own controls and on the music's colours; OutBack only turns the
chevron (1.6) and grows the outline node (2). Reduced motion (the desktop's)
makes each one land at once.

| Interaction | Motion | Duration, curve |
|---|---|---|
| press any plaque | cut deepens 4, plaque sinks 1 (library); glyph sinks 1 and scales to .86 | press 90; glyph OutCubic |
| hover a row, tile, card, link, tree line, provider cell, window control | the row wash fades in, the row leans 2 dp; out the same way | quick 120 in, move 240 out, OutCubic |
| press a row | the press wash deepens, glyph .86 | press 90 |
| hover a library plaque or scrubber | plane or handle and bubble fade in; only the volume track thickens 2 → 3 | hover 160 / 100, chisel |
| where you are changes (tree, bottom nav, tables, queue, search list, equalizer strip) | the select wash travels; arriving from another group it lands, widening from .6 | move 240 OutCubic; land 240 and 320 |
| page navigation, provider switch, now playing opens or changes track | the page fades in and slides 14 dp from the right | 240 fade, 300 slide, OutCubic |
| a page opens | its first five groups rise 8 and fade, one after another | navigate 200 chisel, 40 apart |
| the page scrolls | the masthead's ruler marker follows | quick 120 OutCubic |
| a figure or a word changes (spec strip, Settings claims, readouts, folder counts) | the new one rolls in from half a line below while the old leaves | move 240 OutCubic |
| Home's readings first arrive | the figures count up | count up 640, chisel |
| Playlists folder opens / closes | chevron turns a quarter, overshooting; leaves unfold and slide 8 in from the left, 18 ms apart | move 240 OutBack 1.6; hub unfold 260, move 240, OutCubic |
| Settings outline appears; the section being read changes | the outline unfolds; the node grows 5 → 7, overshooting | hub unfold 260 OutCubic; move 240 OutBack 2 |
| sidebar folds / unfolds | width tweens 272 ↔ 72, names fade over the first half, rail tiles over the second | move 240 OutCubic |
| medium tree drawer, panel drawer | slides in over a fading scrim / out | move 240 OutCubic |
| right panel docks or leaves | slot width unfolds, content fading | hub unfold 260 OutCubic |
| a track loads with nothing loaded / the queue empties | the player bar unfolds from the bottom while the panes' bottoms glide up / folds away | hub unfold 260 OutCubic |
| Home now playing card appears / leaves | height unfolds, contents rise 8 / folds | hub unfold 260 OutCubic; navigate 200 chisel |
| hover the window's close | its glyph turns a quarter | move 240 OutCubic |
| Discord toggle on / off | the select wash and the sel glyph cross over | quick 120 OutCubic |
| search suggestions replace recents | the new set fades in | quick 120 OutCubic |
| overlay, menu | fades in sliding the last 6, out retreating 4 | menu in 160 / out 100, chisel |
| tooltip | after 600 on hover, fades in sliding 8 | tooltip 300 |
| dialog | library | dialog 300 / 160 |
| notice | rises, blade drains; Undo while it shows | placard 300 / 5000 |
| play ↔ pause | `look.Morph`: icon scales from .85 and crossfades | notch 200, chisel |
| like | the heart fills from its centre in `Accent.Ink` and pops to 1.24 and back; unliking only empties it | notch 200; pop field 320 OutCubic |
| track changes | every cover showing it (player bar, now playing, Home card, mini) is revealed behind a 45° cut sweeping from its top-left corner past its bottom-right; title and artists rise 8 and fade in | the caller's duration (navigate 200 in the bar, field 320 elsewhere), chisel; swap navigate 200 OutCubic |
| hover or click the playing art | the plate fades in; the glyph morphs; the plate lingers 700 ms after a click | quick 120 / move 240 OutCubic; notch 200 |
| drag a scrubber | handle follows the pointer exactly; the wave flattens; seek holds the committed value | spring, field 320 |
| volume changes from outside the pointer | the handle chases the new value | `anim.Chase` |
| playback starts / pauses | the wave's amplitude eases in and travels / eases out and stops | spring, field 320 |
| music plays | visualizer blades rise fast and fall slower with the bands, brighter when loud; peak caps hold and fall | springs, every frame |
| a beat lands | every visualizer kicks up to 14 % taller and brighter, strokes thicken | decays with a 180 ms time constant |
| a word is sung | its highlight fills it behind a 45° cut that follows the clock and it rises 2 (3 on the stage) | continuous while it fills, 30 frames a second |
| lyric line becomes active | the list anchors next to it and springs the rest to one third; the line scales up from its resting size and brightens while the previous one settles back | spring; field 320 chisel |
| queue row removed / added | collapses / an accent notch fades | hub unfold 260 / notch 200, OutCubic |
| download progress | a 2 dp lapis rule fills along the plaque's foot; saved shows `download_done` in jade | per progress event |
| shelf and strip arrows | glide one viewport | move 240 OutCubic |
| card hover | the card plane and wash fade in; the play plate rises 4 and fades in, turning gold under its own hover | quick 120 / move 240 OutCubic |
| tab body changes (panel, now playing stage) | crossfade with an 8 rise | move 240 OutCubic |
| accent changes (new cover, source, preference) | fill, on, ink and field tween together, and with the cover tint every plane and ink retints with them | field 320, chisel |
| a cover backdrop's cover changes | the new blur fades in as the old one fades out, carrying on from any fade in progress | field 320, chisel |
| the desktop's style changes | design, colours, light or dark, contrast, tint, reduced motion and gloss apply in the next frame | snaps |
| visualizer style changes | the old drawing crossfades into the new | field 320 |
| equalizer preset, reset or Undo | nodes and curve glide to it; readouts roll once | move 240 OutCubic |
| equalizer node hover, selection, drag | grows 30 % with guides; a register ring lands; the node chases the pointer | quick 120 / move 240; move 240; Chase |
| equalizer band added / removed | node grows from nothing, its column unfolds / collapses while the others narrow / widen with it | notch 200, hub unfold 260, OutCubic |
| equalizer bypassed / on | curve fades to fg3 and its hatch greys / back | field 320 OutCubic |
| mini resized across landscape and portrait | the two layouts crossfade | field 320 |

Nothing moves at rest: no pulsing, no idle frames. A paused, idle window draws
zero frames. While music plays, the wave progress, the visible visualizers and
the word being sung in visible lyrics are the only things that move without
being touched; they ask for frames at up to 60 a second (30 for the lyric
wipe and the travelling wave) and stop as soon as playback pauses, the window
hides them, or motion is reduced. The one ornament allowed to move is the
masthead clock's seconds, and only while the window is focused and motion is
not reduced (`kit.Ambient`, fed by `kit.SetFocused` from every
`ConfigEvent`); an unfocused window shows minutes and draws nothing. Reduced
motion keeps the word by word colours but drops the wipe, the lift and the
line scaling. The registration sheet, rulers, colophon bars, charts and the
equalizer's hatch are recorded once and replayed. Position samples that only
confirm the client's own clock do not redraw.

## Settings

Settings is a Hub page sheet (`pages/settings/`). The sections, in order:
Account, Playback, Sound, Lyrics, Downloads, Local music, Playlists,
Appearance, Integrations, Data and storage, Diagnostics, About (`Sections`,
numbered `05.1` to `05.12` with the glosses in Numbering). The masthead names
the section being read ("§ 05.8  SETTINGS  /  APPEARANCE  設定・外観").

- **Head:** "Settings" at 32/40, the lead, and the spec strip "SECTIONS 12"
  and "CHANGED", whose count rolls and turns sel while it is not zero. The
  column is `min(usable, 960)`, centred, 28 from the stage's left (13 under a
  640 stage), clear of the scroll thumb, 8 under the masthead. The first five
  items rise once when the page opens.
- **Sections:** `kit.SectionStyle{Ruled, Mark: "08 / 12"}` and a card of rows
  (the design's card plane and stroke, cut 7, inset 21). On the expanded
  class with the tree unfolded the sections are the tree's Settings outline;
  otherwise `look.FlowChoice` chips at the top do the same job, wrapping,
  never a sideways strip. A jump glides through the list (`Scroller.GlideTo`
  over `Motion.Move`, OutCubic), laying out every row on the way (R9), and the
  section stays current until the reader scrolls. The current section is the
  last one whose top passed a third of the viewport.
- **Rows** (`row.go`): at least 44 tall, 13 apart with a vein hairline
  between; title `T.Row`, detail `T.Detail` fg2 under it, wrapping, never
  elided; the control at most 320 (chips 520), stacking under the words in a
  narrow row. The row wash bleeds 8 × 4 (cut 6) with the 2 dp lean; toggle
  rows and rows that open something work from anywhere on the row. A
  "Changed" tag in `T.Detail` fg3 stands beside a title whose value differs
  from its default (`defaults.go`, daemon and client values together), and a
  quiet reset plaque ("Reset <title>") fades in on hover or focus; a reset
  offers Undo. A daemon setting shows the new value at once, so a toggle
  moves once.
- **Plaques:** every action in Settings is a small quiet plaque. Clear
  caches, Sign out, Disconnect Last.fm and Stop watching a folder ask first
  through the shell's confirm with a danger plaque.
- **More options** folds (a chevron on OutBack 1.6, the rows unfolding like
  `kit.Disclosure`) hold the rare rows: TIDAL credentials, stream clients,
  the Listen Together server, the proxy, release notes.

Rows that matter to the contract:

- Sound: "Playback sound" (the current preset in its detail) with "Open
  sound" (opens the same overlay as the player bar, anchored to that plaque),
  then **Equalizer** ("Bypassed" or "On · <preset>") with "Open equalizer".
- Lyrics: **Word by word** (the client preference, on by default, detail
  "Highlight each word as it's sung, estimated when the lyrics only time each
  line"), **Word-timed lyrics** (the daemon's `lyrics_boidu`, which the daemon
  treats as on when unset, so the row reads on too; detail "Ask Boidu first
  for lyrics timed word by word") and **Romanized lyrics**.
- Local music: the watched folders as rows (name, path, a rolling track count
  in mono, Remove), Rescan and "Add folder".
- Playlists: Import a playlist (a Ryotunes playlist file) and Export the
  queue.
- Integrations: **Discord rich presence** first (off by default; a claim Off,
  Connecting, Connected or Discord is not running), then the Discord title
  row and its save action; also reachable from the top-bar quick toggle.
- Diagnostics: "Daemon summary" ("Connected · daemon <v> · protocol <n> ·
  <engine>") with "Open diagnostics".
- About: the Ryoku mark, App, Daemon, Client, and Engine (the playback
  engine's name and version) in mono; updates with Check, Install, View
  changelog and the release notes; License: `GPL-3.0-or-later` in mono, the
  copyright line, and "Licenses", which opens the license and the third-party
  notices; Website, Issues and Ryoku links.

**Appearance** is two cards (`appearance.go`, `handoff.go`):

1. **The desktop's look, read only.** Claims, not controls: no Changed tag,
   no reset, nothing to press but the hand-off. Each value rolls to its new
   word when the desktop changes.

   | Row | Detail | Value |
   |---|---|---|
   | Design | The shape of every Ryoku app: Lithic, Material, Adwaita or Fluent | Lithic, Material, Adwaita, Fluent |
   | Colours | Ryoku's own colours, or the desktop's from your wallpaper | Ryoku, Wallpaper |
   | Light or dark | For the whole desktop | Dark, Light |
   | Contrast | High makes text and the lines between things stronger | Standard, High |
   | Tint | How much of the wallpaper's hue the surfaces take (only for Lithic on the wallpaper's colours, as in the Hub) | Off, Low, Medium |
   | Reduce motion | Stills Ryotunes' animations with the rest of the desktop | Off, On |
   | Ryoku Settings | The look is the desktop's: change it once in Style and every Ryoku app follows | "Open Ryoku Settings" |

   The hand-off runs `ryoku-shell hub open style` through `spawn.Command`
   (its own scope, so Ryoku Settings outlives Ryotunes), off the frame
   goroutine; a failure says "Could not open Ryoku Settings".
2. **Ryotunes' own,** in order: **Accent colour** (`look.FlowChoice` Album
   art, Source, Desktop; details "The colour of the cover that is playing",
   "The colour of the service you are listening on", "Your Ryoku palette's
   accent"), **Tint with the cover** (a toggle, **off by default**, detail "The
   window takes a tint of the cover that is playing over the desktop's
   surfaces"; disabled with the detail "Follows the album art accent" while
   Accent colour is not Album art), **Density** (Comfortable, Compact), **UI
   scale** (80 to 140 %), **Low resource mode**, **Visualizer** (on by
   default; its detail names what the style draws, or "Install cava to see
   Bars and Curve" / "Install pw-record to see the waveform and stereo field"
   with the toggle disabled), **Visualizer style** (`look.FlowChoice` of the
   installed styles among Bars, Curve, Oscilloscope, Vectorscope, disabled
   while the visualizer is off), **Wave progress** (on by default), then
   **Open as a floating window** (a toggle, on by default; detail "Opens at
   its own size above your tiled windows" when on, "Opens tiled in your
   layout like any other window" when off, and both add "from the next
   launch" once it differs from how this window opened).

There are no Colours, Palette file, Corners or Motion rows: those belong to
the Style page. Every other change applies live with its motion. Reduced
motion and low resource mode pause the visualizer and the wave without
changing the stored settings.

## Sign in

The client opens every sign-in page; the daemon never launches a browser. The
client runs inside the graphical session that started it, so its opener has
the session's display and browser, and it can see when opening fails.

YouTube Music imports one eligible browser session at a time, in discovered
store order with the default container first. If every session fails it returns
the sign-in page without an RPC error and keeps watching all stores until one
succeeds or five minutes pass. SoundCloud watches its cookie stores the same
way; a token SoundCloud refuses is not tried again in that flow, a token that
fails for another reason waits longer each time, and only the flow's final
outcome is announced (`signed_in`, `error` with the last reason from a store
that changed during the flow, or `expired` when nothing was found). When a
YouTube watch ends, the daemon announces the account it still holds. Spotify
uses the `spotify-auth` `url` event, and TIDAL's `tidal-auth` `waiting` event
carries the complete verification `url` and `code`.

A browser session is followed, not copied. The daemon remembers which store
and container a YouTube or SoundCloud session came from. Once in the
background when it starts, whenever the provider rejects the session (401 or
403, YouTube's account menu without an active account), and while a
SoundCloud token is expired, it re-reads that store and adopts the browser's
current cookies if they prove the same account by channel id, email or
handle (never by display name alone), then retries once; the user sees
nothing, and a quiet SoundCloud re-adoption arrives as `restored`, without a
toast. The subscribe snapshot answers from what the daemon holds, so an
offline launch still shows the last known account. Only when the browser has
no working session does the account row fall back to signed out, with the
provider's reason: YouTube first asks its account menu whether the stored
session still works, and SoundCloud reports `session_expired` with the
daemon's message. A SoundCloud token that merely expired, with nothing newer
in the browser, keeps its source: the daemon announces `stale` once and the
row reads signed out without a toast, searches and playback go out as a
guest without the token, and the next re-read adopts quietly. Sign in never
short-circuits on a session the provider rejected: it runs the full import or
watch flow, also when the remembered store is missing or unreadable, and an
offline Sign in reports its error. The
daemon never refreshes, rotates or writes a browser's cookies itself, so it
can never sign the user out of their browser.

- `act.OpenURL(e, url)` starts `xdg-open url` through `spawn.Command`
  (its own session, and under a systemd user manager its own scope in
  `app.slice`, so a browser it starts never lives inside a service that can
  stop) and waits for it off the frame goroutine. Each flow's
  URL opens once. A missing opener or a non-zero exit shows a notice "Could
  not open your browser" and leaves the link on the account row.
- While a flow waits, its account row says "Waiting for your browser" and
  offers "Open again" and "Copy link" (quiet plaques) and Cancel; TIDAL also
  shows its code in mono. The Home header's Sign in starts the active
  provider's flow directly.

## Input

- Scrolling: `look.Scroller` and `look.HScroller` everywhere content scrolls
  (pages, the sidebar tree, queue, lyrics, settings). Every wheel or trackpad
  delta moves the content by exactly delta × PxPerDp in the frame it
  arrives, however fast the deltas come and however different the rows'
  heights are; nothing skips or jumps. An animated jump (`GlideTo`) lays out
  every row it passes. Horizontal trackpad deltas move shelves; vertical
  wheel over a shelf scrolls the page.
- Sideways scrolling is only for shelves and the equalizer's band strip, and
  both show where more is: the cut-off item at the edge and step plaques that
  disable at the ends. Short choice sets (Sound presets, visualizer styles,
  accents, Settings sections) wrap onto more lines instead.
- Every clickable is `kit.Click` (or a kit control). Pointer handlers push
  their own clip; catchers use `pointer.PassOp` where content beneath must
  still receive input.
- Keys: Space play/pause (unless a field has focus or a confirm is open),
  Ctrl ← / → previous / next, Alt ← / → back / forward, / and Ctrl K search,
  Ctrl F filter the current list, Ctrl L like (an unlike offers Undo), Ctrl R
  refresh, Ctrl , settings, Ctrl Q quit, F11 full screen, Ctrl B fold or show
  the sidebar (not in the mini), Escape closes the topmost thing: a confirm,
  an overlay or menu (search suggestions are passive and close with the
  field), the tree drawer, the panel drawer, then the field handles its own
  Escape while focused, then the now playing view, then it leaves the
  equalizer page the way its back plaque does, then nothing.

## Windows and lifetime (shell)

- app id `ryotunes`, title "Ryotunes", min 480 × 560, default 1280 × 800,
  decorations per preference (default: the client draws its own).
- **Floating or tiled is the app's choice** (Settings → Appearance → Open as a
  floating window, on by default). Floating, the main window maps with its
  minimum and maximum both set to its remembered size (default 1280 × 800):
  niri, Hyprland and Sway float a fixed-size window the way they float a
  dialog, and floating desktops simply open it at that size. On the first
  configure, before the first buffer, the shell re-issues those options so
  Gio drops its 40 dp fallback title bar padding, and after the first
  presented frame the window becomes freely resizable again (minimum
  480 × 560). Tiled, it maps with only its minimum, so tiling window managers
  tile it. The mini always maps fixed at its own size (scaled with the zoom),
  so it floats either way. The size is remembered only while the main window
  floats: on niri and Hyprland the shell asks the compositor (debounced, off
  the UI goroutine) and skips tiled, fullscreen and maximized sizes, so none of
  them becomes the next floating size. No compositor rule decides this for us:
  Ryoku's niri and Hyprland providers carry no Ryotunes rule.
- Focus: every `ConfigEvent` feeds `kit.SetFocused`, so ambient motion runs
  only in the focused window.
- Single instance: the client holds an exclusive `flock` on
  `$XDG_RUNTIME_DIR/ryotunes/client.lock` for its lifetime. A second
  `ryotunes` that cannot take the lock asks the daemon to `show` and exits 0.
  The daemon's client count no longer decides anything (an events subscriber
  or a dead client must never block a launch). `show` raises the main window,
  re-creating it when only the mini is open.
- Mini: opening the mini closes the main window and opens "Ryotunes Mini";
  expand reverses it. Closing the last window exits the client and releases the
  lock. Both directions work any number of times.
- Resizing across every class boundary, down to the minimum, never panics,
  never loops and never leaves a blank frame.
- The daemon's `Show` (MPRIS Raise, tray, a second launch) tries the same lock
  without blocking: when it can take it, no client runs, so it releases the
  lock and spawns `ryotunes` through `spawn.Command`, so the window and what
  it opens live outside `ryotunesd.service`; when it cannot, it broadcasts `show` to its
  subscribers. `ryotunes-cli events` never counts as a window.
- The daemon streams events to a subscriber before it answers `subscribe`.
  The client holds those events until the snapshot is published and then
  delivers them after it; the read loop never waits on the handshake.
- The main window's title is "Ryotunes", the mini's "Ryotunes Mini"; both use
  app id `ryotunes`, so compositor rules tell them apart by title.
