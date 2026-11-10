# Client UI contract, v3

Ryotunes is a music player. Its window is laid out like one: navigation on the
left, the music you browse in the middle, what plays next on the right, and the
player along the bottom where the hand rests. It is drawn in Lithic: flat stone
planes, 45 degree cuts, bone for where you are, gold for play, minerals with a
word for state, Space Grotesk for type and Space Mono only for data.

Lithic is not ours to copy. The client draws through the shared library
`github.com/ryoku-dev/ryoku/docs/lithic/library/go` (packages `palette`,
`theme`, `anim`, `kit`, `tokens`), pinned at
`v0.0.0-20261109022738-f273fa939a18`. Read, before writing code:

- `~/Work/hyperion/docs/lithic/README.md`, `working.md`, `map.md`,
  `components.md`, `motion.md`, `copy.md`, `checklist.md`
- `~/Work/hyperion/docs/lithic/library/go/README.md` (every kit API)
- the library source under `~/Work/hyperion/docs/lithic/library/go/kit/`
  (read-only for us; never edit it)

The library's `Rail`, `AppFrame`, `NewPlaque` and `BrandMark` are Ryoport's
window. Ryotunes does **not** use them: it has its own music frame below.
Everything else (Plaque, IconPlaque, SplitPlaque, Search, TextField, Toggle,
Slider, Stepper, Filters, Tabs, Disclosure, Tablet, GroupHeader, Skeleton,
Empty, Pane, MineralField, Section, Slab, Facts, Claim, Marks, Sentence,
BladeMeter, BladeProgress, Working, Menu, Dialog, Confirm, Placard,
SectionIndex, Click, CutPath, FillCut, StrokeCut, Notch, Blade, Flake, Hatch,
FocusRing) is used as is. When a library control cannot do a job, the
app-owned replacement lives in `look` and says why in one line.

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
owners.

`internal/client/lithic` (our old private copy) is deleted once nothing imports
it. No file may keep a copied cut, paint recipe or click wrapper.

## Packages

```
internal/client/
  conn/  state/  art/         kept; small additions named below
  ui/
    look/        app theme on the library, source colours, Ryoku mark,
                 provider glyphs, icons, frame geometry, scrolling,
                 scrubber, art view, micro-motion helpers
    env/         kept; Env.Th becomes *look.Theme
    act/         kept (daemon actions)
    widgets/     track table, card, shelf, grid, quick tile, hero,
                 page header, page states, item menu, playlist dialogs
    chrome/      topbar, provider switcher, sidebar, bottom nav,
                 player bar, sound popover, right panel (queue, lyrics,
                 together), now playing view, mini window content
    shell/       windows, frame, router, overlays, keys, single instance
    pages/       home search library album playlist artist list radio
                 downloads settings diagnostics
    demo/        fixtures for --demo renders
```

Nothing under `ui/` imports `shell`. Pages import `env`, `widgets`, `act`,
`look` and the library. Chrome imports the same plus `widgets`.

## Colour

Colour comes from the Ryoku palette through `palette.Load` /
`palette.FromPalette` (follow, dark, light) or a palette file named in the
client preferences. No hex value appears outside `look/brand.go`.

| Role | Used for |
|---|---|
| ground | the window behind the panes and the top bar |
| stone1 | sidebar, right panel, player bar, slabs and tiles inside the content |
| stone2 | content pane |
| stone3 | hover plane of tiles and rows; field and track backgrounds |
| stone4 | popovers, menus, sheets, the card play plate. Nothing sits above stone4, so a row on it hovers by mixing `fg` in at 0.06 and presses at 0.08; it never steps down to stone3, which reads as a hole in dark mode |
| sel / onSel | where you are: current nav row, selected row, active tab blade, a toggle that is on (shuffle, repeat, open panel) |
| gold / onGold | play. The theme points these roles at the **accent** (below), so every gold plaque is the colour of the music. The player bar's play/pause is the window's standing gold action; a page may spend one more gold on the action that plays it (hero Play, top search result Play). Nothing else is gold: not radio, not download, not navigation. |
| goldInk | line plaques that accompany a gold action (the accent's ink) |
| jade lapis amber jasper | state, always with a shape and a word: downloading (lapis), saved (jade), paused (amber), failed (jasper), live stream (jade) |
| fg fg2 fg3 vein | ink and hairlines |

**Accent.** One hue belongs to the music, and everything that means "this is
playing" or "play" wears it: the play plaques, the progress wave, the
visualizer, the playing row, liked hearts, the sung lyric words and the header
fields. `look.Theme.Accent` carries it:

- `Accent.Fill` the face of a gold plaque; `Accent.On` the ink on it
  (`palette.InkOn(Fill)`);
- `Accent.Ink` the fill re-toned along HSL lightness to 4.5:1 on stone2
  (lighter in dark mode, darker in light), for type, glyphs, the wave, the
  visualizer and hearts;
- `Accent.Field` `palette.Mix(R.Stone2, Accent.Fill, tokens.AlphaMineral)`, the
  header field plane.

The theme writes Fill, On and Ink into `R.Gold`, `R.OnGold` and `R.GoldInk`
every frame, so library gold plaques follow without a wrapper. Where the accent
comes from is the **Accent colour** preference (`ClientPreferences.Accent`):

1. **Album art** (default). The loaded track's cover, sampled off the frame
   goroutine by the art cache into an `art.Palette`. `Vibrant` is 1.x's rule: a
   24 × 24 downsample, pixels with alpha under 128 or saturation under .12
   skipped, colours bucketed by 3 bits per channel, each pixel scoring
   `saturation × (1 − |max − .65|)`, the bucket with the best total wins and
   its average is the sample. `Dominant` is the most populous bucket over all
   opaque pixels, whatever their saturation. The accent keeps `Vibrant`'s hue;
   saturation is clamped to .55–.85 and lightness set to .70 in dark mode and
   .42 in light, so it reads on every stone. A cover that is still loading
   keeps the previous accent; a track without a cover, a grey cover (no
   vibrant bucket) or nothing loaded uses the desktop accent.
2. **Source.** The active provider's brand (`look/brand.go`), as v2 did for
   everything but play.
3. **Desktop.** The Ryoku palette's gold roles as loaded.

Every accent change (a new track, a provider switch in Source mode, the
preference itself) tweens all four colours over `Motion.Field` (320) with the
chisel ease; a palette or mode change snaps. Album, playlist and artist heroes
tint only their own field (and its visualizer lane) with their own cover's
vibrant colour, tweening in when it arrives; their Play plaque stays the global
accent.

**Cover tint.** While the accent comes from the album art and **Tint the
window** is on (`ClientPreferences.CoverTintOn()`, default on), every surface
takes the cover's hue, the way Ryoku's own palette takes the wallpaper's
(`look/tint.go`, `look/oklch.go`). Ground, the four stones, vein, fg, fg2, fg3
and sel are re-hued in OKLCH: each keeps its own lightness, so every contrast
pair holds, and takes a small chroma (dark mode: ground .020, stone1 .026,
stone2 .030, stone3 .034, stone4 .036, vein .030, fg .010, fg2 .014, fg3 .018,
sel .018; light mode .8 of those) times a strength, reduced until it fits
sRGB. The minerals, scrim, onSel and onSel2 never change, so states read the
same on any cover. Hue and strength come from the palette: the dominant
colour's hue when its OKLCH chroma is at least .04 (strength
`min(1, chroma / .12)`), else the vibrant colour's hue at strength .35, else
no tint (a black and white cover keeps the desktop stones). The accent is then
derived against the tinted stone2, so `Accent.Ink` still meets 4.5:1. A track,
mode or preference change tweens the whole role set together with the accent
over `Motion.Field`; a palette change snaps. With the tint off, an accent mode
other than Album art, or nothing loaded, the stones are the desktop palette's.

**Cover backdrop** (`look/backdrop.go`). A hint of the cover's blur sits on the
planes where a cover is the subject: the now playing view's pane and Home's
now playing card (the loaded track's cover), the album, playlist and artist
heroes (their own cover) and the mini window (the loaded track's). Page
headers, the player bar, the compact player, cards and rows stay plain stone.
The art cache blurs the 24 × 24 palette sample on its worker (three box passes
of radius 2, edges held) and keeps it with the palette, in memory and in the
cover's palette file on disk; a palette file from before the blur is sampled
again once from the disk cover. Collages have none. `look.Backdrop` tones it
to the plane under it: each pixel keeps its hue, its OKLCH lightness stays
within .05 of the plane's and its chroma under .08, so text and veins keep the
plane's contrast on any cover. It is drawn at .35 opacity, scaled to cover the
plane (centre crop, linear filtering) inside the plane's cuts, and toned again
only when the plane's lightness moves by .02 (a tint tween). The first frame
shows it whole. A new cover fades in over `Motion.Field` with the chisel ease
while the old one fades out, and a cover that lands mid-fade carries on from
wherever the fade left the old one. No cover, or one not sampled yet, fades
the last one out.

**Source colour.** The active provider's brand is identity, not state, so it
may stand without a word, and since v3 it lives in one place: the provider
switcher. `look/brand.go` holds the fixed brand fills of YouTube Music,
Spotify, SoundCloud and TIDAL and the Ryoku mark orange. `look.Theme.Src`
carries the active provider's `Fill`, `Ink` (re-toned to 4.5:1 on stone2) and
`Field`, tweened over `Motion.Field` when the provider changes. It draws the
switcher's active cell (field plate, 1 dp `Src.Ink` hairline at alpha .55,
glyph in `Src.Ink`), the compact switcher's glyph, and the small provider
glyph beside the artists in the player bar. Local files and radio have no
brand: their source colour is `R.Lapis` and its re-toned ink.

**Corners.** Every pane cuts its two top corners by `th.Cut.Large`, and the
**Corners** preference (`ClientPreferences.Corners`, `CornerDp`) sets it:
Desktop (the Ryoku window corners: the `rounding` of the first
`desktop.appOverrides` entry whose `class` matches `ryotunes` with a value of 0
or more, else `desktop.appearance.rounding`, from
`$XDG_CONFIG_HOME/ryoku/desktop.json`; 16 when absent; watched while chosen, so
a change in Ryoku Hub reaches open windows), Square 0, Fine 8, Chisel 16
(default), Deep 24, or Custom 0–32 in whole dp. `Theme.SetCorner` snaps the
first value and tweens later ones over `Motion.Select` (240), so the panes
re-cut in place. Dialogs and empty-state frames share the same cut.

## Frame geometry (dp)

`W`, `H` are the window size in effective dp: window pixels divided by the
screen scale times the app's zoom. The player window has its own classes:
compact `W < 760`, medium `760 <= W < 1100`, expanded `W >= 1100`, so the
sidebar with playlist names survives a 1280 window at 110 % zoom. Structure
changes at a boundary; a wide layout is never squeezed.

```
expanded / medium
┌─────────────────────────────────────────────────────────────────────┐ y 0
│ top bar 56 on ground                                                │
├──────────┬──────────────────────────────────────────┬───────────────┤ y 56
│ sidebar  │ content (stone2, top corners cut 16)     │ right panel   │
│ stone1   │                                          │ stone1, 340   │
│ 240 / 72 │                                          │ (when docked) │
├──────────┴──────────────────────────────────────────┴───────────────┤ y H-8-88-8
│ player bar 88, stone1, top corners cut 16, x 8 .. W-8               │
└─────────────────────────────────────────────────────────────────────┘ y H-8

compact
┌──────────────────────────────┐
│ top bar 56                   │
│ content, full width          │
│ compact player 64 (stone1)   │
│ bottom nav 56 (stone1)       │
└──────────────────────────────┘
```

| Part | Expanded | Medium | Compact |
|---|---|---|---|
| outer margin and gap between panes | 8 | 8 | 8 (no side margin for the bottom nav) |
| top bar | 56, ground | 56 | 56 |
| sidebar | x 8, width 240, labels + playlists | x 8, width 72, icons + playlist covers | none: bottom nav 56 |
| content side inset | 34 | 21 | 13 |
| content top inset (pages without a hero) | 21 | 21 | 13 |
| right panel | docked 340 when `W - sidebar - 340 - 24 >= 520`, else an overlay sheet 340 wide (max `W - 72`) over the content's right edge | same rule | full sheet over the content, opened only by an explicit action |
| player bar | 88, three zones | 88, three zones, narrower | compact player 64 above the bottom nav |

Crossing into compact closes a docked panel (it would otherwise cover the
page) and remembers it; crossing back out restores it. A sheet never opens by
itself on a class change.

**Nothing loaded.** Until the daemon reports a current track there is no
player bar and no compact player: the panes run down to `H - 8` (compact: to
the bottom nav). When a track loads, the bar unfolds from the bottom over
`Motion.Unfold` while the panes' bottom edges glide up by the bar and its gap;
when nothing is current anymore it folds away the same way. Reaching the end of
the queue or stopping keeps the last track in the bar, as music players do. The first snapshot after
connecting snaps instead, so a launch with a restored track never shows the
bar arriving and a launch with nothing never flashes an empty one. Reduced
motion snaps.

Panes cut their two top corners by `th.Cut.Large` (the Corners preference,
16 by default) and keep the bottom square. Each pane pushes its own clip. The
window minimum is 480 × 560; the default is 1280 × 800.

### Top bar (chrome/topbar.go)

On ground, 56 high, every blank part drags the window (`system.ActionMove`;
double click toggles maximise).

- **Expanded:** over the sidebar column, the Ryoku mark (24; `look.RyokuMark`,
  the current v2 mark: its cut strokes in `fg`, its pointer in Ryoku orange) at x 21, y 16,
  then 8 and the word "Ryotunes" in `T.Title` `fg`. At the content's left
  edge: Back and Forward (`IconPlaque` small, quiet, 4 apart; disabled ink
  when the router cannot move). 13 to their right, the search field
  (`kit.Search`) flexes up to 520 wide. At the right: the provider switcher,
  13, then window controls (maximise or restore, close; small quiet icon
  plaques 4 apart) when the client draws its own decorations. There is no
  minimise: niri and Hyprland have no minimised state, and Gio cannot read
  the compositor's capabilities to show it only where it works.
- **Medium:** the mark alone, centred over the 72 rail; the rest as expanded.
- **Sidebar toggle** (expanded and medium): a small quiet icon plaque 4 left
  of Back, so toggle, Back and Forward ride the content's left edge together.
  Its glyph is drawn in `look` (a cut frame with its left column filled while
  the sidebar is open, hollow while folded) and morphs over `Motion.Notch`.
  Tip: "Collapse sidebar · Ctrl B", "Expand sidebar · Ctrl B", or in the
  medium class "Show sidebar · Ctrl B". While the sidebar is folded the word
  "Ryotunes" fades out and the mark glides to the rail's centre.
- **Compact:** mark at x 13; a search icon plaque that opens the field across
  the bar (Escape or blur with an empty field closes it); the provider
  switcher collapses to one 40 icon plaque showing the active provider glyph in
  `Src.Ink` that opens a `look.Overlays` menu (`kit.Menu`) of the four
  providers; window controls.

**Discord quick toggle.** Left of the provider switcher, 13 apart, a small icon
plaque with the Discord glyph: quiet when rich presence is off, `PlaqueOn` when
on. Its tip names the state ("Discord status: Connected", "Connecting",
"Discord is not running", "Off"). The same setting is the first row of
Settings → Integrations.

**Search has one entry point: this field.** There is no Search destination in
the sidebar or the bottom nav. `/` and Ctrl K focus it. While it is focused a
`look.Overlays` entry under it lists up to 8 recent searches when empty, or up to 6
suggestions while typing (debounced 200 ms, from the active provider); Enter or
a suggestion pushes `RouteSearch{Query}`. The field keeps the query while the
results page is current and clears when the user navigates elsewhere. Escape
clears, then blurs.

**Provider switcher** (chrome/providers.go): one cut container (stone1, cut 6,
height 40, inner padding 4) holding four 40 × 32 cells, 4 apart, in the order
YouTube Music, Spotify, SoundCloud, TIDAL. Each cell shows the provider glyph
(20). Inactive glyph `fg3`; hover steps the cell plane to stone3 and the glyph
to `fg2`; the active cell is a plate of `palette.Mix(R.Stone3, Src.Fill, .22)`
with a 1 dp `Src.Ink` hairline at alpha .55 and the glyph in `Src.Ink`. The
plate glides to a newly chosen cell over `Motion.Select` (240); the source
colours retint over `Motion.Field` (320); the content re-enters (navigate).
Every cell has a `look.Tip`: provider name and "Signed in" or "Not signed in".
Clicking an inactive cell sets the provider optimistically and calls the daemon
(`set_provider`); a failure reverts and shows a placard.

### Sidebar (chrome/sidebar.go)

stone1 pane. **Expanded (240):**

1. 13 top inset. Destinations as 40 high rows inset 8 from the pane sides,
   4 apart: Home, Library, Radio, Downloads. Icon 20 at x 13 in the row,
   label `T.Label` 13 to its right. Rest ink `fg2`; hover plane step and `fg`;
   current row is a `sel` slab (cut 6) with `onSel` ink that glides between rows
   over `Motion.Select`. Downloads shows the active job count in `T.Mono` `fg2`
   at the row's right with a lapis flake while jobs run.
2. 21 gap, then a group row: "Playlists" in `T.Group` `fg3` at x 21 and a small
   quiet "+" icon plaque at the right (New playlist dialog).
3. The playlist list fills the remaining height and scrolls (look.Scroller).
   Rows are 48 high: cover 32 (cut 3, 8 inside the hover plate), 13, title `T.Label` `fg` and a caption
   `fg3` ("Playlist", "Album", "Liked songs · 128" with the count in mono).
   Order: Liked songs (a heart glyph on an `Accent.Field` tile), then pins
   (`state.Personal.Pins`), then library playlists. The open collection's row
   is the sel slab; the playing collection shows a static `volume_up` glyph in
   `Accent.Ink` at the row's right. Right click opens the item menu.
4. Settings is a 40 row pinned 13 above the bottom, under a 1 dp vein hairline
   inset 8.

**Medium (72):** the same order with 56 × 48 icon tiles (`look.Tip` carries the
label), playlist covers 32 (cut 3) centred with tips, the "+" plaque
centred, Settings at the bottom.

**Folding (expanded class).** The sidebar folds to the 72 rail and back: the
top bar's sidebar toggle or Ctrl B, remembered in
`ClientPreferences.SidebarCollapsed` and applied only in the expanded class.
The pane's width tweens 240 ↔ 72 over `Motion.Unfold` (200) with the chisel
ease, and everything to its right (content, a docked panel, the top bar's
toggle, Back, Forward and search) lays out from the tweened width each frame.
Labels, captions, the Playlists group word and the Downloads count clip at the
pane edge and fade out over the first half of a fold (in over the second half
of an unfold); icons and covers glide to their rail places, and the sel slab
morphs from row to tile with them. Folded, the pane is exactly the medium
rail. The panel docking rule reads the target width, so a fold never makes a
docked panel flicker. In the medium class the toggle opens the labelled
sidebar as a 240 sheet over the content's left edge (stone1, the pane's top
cuts, entering like a popover with `Motion.MenuIn` and 6 dp travel from the
left); Escape, an outside press (swallowed), Ctrl B or choosing a destination
closes it. Compact has no toggle.

**Compact:** no sidebar. `chrome/bottomnav.go` draws a stone1 bar 56 high across
the full width: Home, Library, Radio, Downloads, Settings in five equal columns,
icon over `T.Caption`, current column a sel tile (cut 6) that glides.

### Player bar (chrome/playerbar.go)

stone1, 88 high, x 8 to W-8, top corners cut 16, side insets 21. Three zones:
`side = clamp(barW * .28, 220, 360)`; the centre column is
`min(560, barW - 2*side - 42)` wide and centred on the bar.

- **Left zone**, one `layout.Flex` so nothing can overlap: art 56 (cut 6) │ 13 │
  text column (flexed): title `T.Row` `fg` one line ellipsis (click opens the
  album, or the now playing view when there is no album), then 2 below a line
  with the provider glyph 14 in its brand ink, 4, artists `T.Caption` `fg2`
  ellipsis (click opens the first artist) │ 8 │ like (small quiet icon plaque;
  liked = filled heart in `Accent.Ink`) │ 4 │ download (small quiet icon plaque;
  downloading shows a lapis progress ring and its tip says
  "Downloading 42%"; saved shows `download_done` in jade with the tip "Saved").
  No provider word, no time, no badge in this zone. Local files and radio hide
  like and download.
- **Centre**, vertically centred stack: transport row 40 high, 6, seek row 20.
  Transport: shuffle, previous, play/pause, next, repeat, **13 apart**. All are
  small icon plaques (32) except play/pause, a normal `IconPlaque` (40) of kind
  gold. Shuffle and repeat are quiet when off and `PlaqueOn` (sel) when on;
  repeat one shows `repeat_one`. Seek row: elapsed time in `T.Mono` `fg3`
  right-aligned in a 44 box, 8, the seek scrubber (flexed), 8, the duration in a
  44 box. A live stream (no duration) replaces the scrubber and times with a
  jade `kit.Claim` "Live".
- **Right zone**, right-aligned: lyrics, queue, now playing (small icon plaques,
  4 apart, `PlaqueOn` while their panel or view is open) │ 13 │ mini player │
  13 │ volume icon │ 4 │ volume scrubber (120 expanded, 88 medium) │ 13 │ sound.
  The volume icon reflects the level (`volume_off` at 0, `volume_mute` under
  1/3, `volume_down` under 2/3, `volume_up` above) and toggles mute, restoring
  the previous level or 70.

Scrubbers (look.Scrubber): 24 high hit area across the full length. Every end
and corner is square or cut, never rounded. Seek uses a 2 thick track and a
3 × 14 vertical blade handle (square ends, `fg`) that fades in on hover or
drag without changing the track thickness. Volume uses a 2 thick track at
rest, 3 on hover, and an always-visible 10 square handle cut 3. Press anywhere
jumps there and grabs the pointer (`pointer.GrabCmd`) so the drag continues
outside the 24 band until release. A bubble (stone4, cut 4, `T.Mono`) floats 8
above the pointer while hovering or dragging, measured at its full label
height and clamped horizontally inside the scrubber: the time under the
pointer for seek, the percentage for volume. Seek
previews while dragging and commits on release; the position then holds at the
committed value until the daemon reports past it (no jump back). Volume sends
`set_volume` at most every 50 ms while dragging and persists on release. Wheel
or trackpad over volume: ±5 per notch, trackpad deltas accumulate to the same
step; over seek: ±5 s per notch. Keyboard when focused: Left/Right step.

**Wave progress** (`ScrubberStyle.Wave`, every seek bar: player bar, mini,
compact player). Android's wavy progress, chiselled: the played part is a
zigzag, a triangle wave with sharp vertices, never a sine.

- Played part: a zigzag 2 thick (perpendicular), amplitude 3, wavelength 20,
  in the fill colour. Build it as one filled polygon: the zigzag's points
  shifted up and down by `thickness / 2 × √(1 + slope²)`, which keeps parallel
  edges and sharp mitred vertices. Clip it at the track start and at the
  played position with a vertical cut.
- Unplayed part: a flat track 2 thick in the track ink (at least 1.5:1 against
  the bar plane in both modes), starting 3 after the played end while the wave
  has amplitude, square at both ends.
- While playing, the wave travels toward the played end at one wavelength per
  1200 ms. Pausing, buffering, a drag or reduced motion flattens it: amplitude
  eases to 0 over `Motion.Field`, then the phase stops and the bar is a flat 2
  stroke that asks for no frames. Resuming eases the amplitude back.
- Hover keeps the wave at 2 thick and shows the 3 × 14 blade handle; dragging
  flattens it so the position is exact.
- The compact player's top edge uses the same wave at 1.5 thick, amplitude 1.5,
  wavelength 12.

**Sound popover** (chrome/sound.go): a `look.Overlays` entry anchored to the
sound plaque's window rectangle, placed above the bar, right edges aligned and
clamped 8 inside the window. 340 wide, stone4. "Sound" in `T.Section`, 13, the presets
(Flat, Slowed and reverb, Nightcore, Bass boost, Wide) as choice chips that wrap onto a second line instead of scrolling sideways, 21, five rows of label
`T.Label`, value `T.Mono` right, `kit.Slider` below: Tempo 0.25 to 2.0 ×,
Pitch −12 to 12 st, Reverb 0 to 100 %, Bass −6 to 12 dB, Width 0 to 100 %;
changes debounce 80 ms into `set_audio_fx`; a quiet "Reset" plaque at the end.

**Compact player** (64, stone1, cut 16 top corners, above the bottom nav): art 44
│ 13 │ title and artist (flexed) │ 8 │ like │ 8 │ play/pause (gold 40) │ 4 │
next. A wave progress (1.5 thick) across the bar's top edge shows progress in
`Accent.Ink`. Tapping the art or text opens the now playing view.

Progress drawing: while the wave travels it asks for its next frame 1/30 s ahead
(1/60 s while its amplitude eases in or out; a visible visualizer drives 60 a
second anyway). When the wave is flat (paused, reduced motion, wave turned off) it asks
for its next frame only when the fill would move by one pixel
(`duration / trackPx`, at least 250 ms) and when the elapsed second changes.
Paused or hidden, it asks for none.

### Right panel (chrome/rightpanel.go, queue.go, lyrics.go, together.go)

stone1, 340 wide. A 48 header: `kit.Tabs` Queue, Lyrics, Together at x 13, a
small quiet close plaque 13 from the right. Docked, the panel's slot unfolds
its width over `Motion.Unfold` (200) with the content clipped and fading;
as a sheet it enters like a popover (`Motion.MenuIn`, 6 dp travel) and closes
on Escape or an outside press, which it swallows.

- **Queue:** sections with `kit.GroupHeader`: "Now playing" (the current track
  row: art 40, title `T.Row` in `Accent.Ink`, artists, and a caption "From
  <collection>"), "Up next" with its count and a quiet "Clear" plaque, then
  "Autoplay" for the continuation in `fg2`. Rows 56: art 40, title, artists,
  duration `T.Mono` `fg3`; hover reveals a drag grip on the left and a remove
  plaque in place of the duration. Click plays from that row; right click
  opens the item menu; drag reorders with a pointer grab. A removed row
  collapses over `Motion.Unfold`; an added row cuts a notch over
  `Motion.Notch`. The header's more menu: Stop after this track, Save queue as
  playlist, Clear. Empty: `widgets.EmptyState` "Nothing queued" / "Play
  something and the next tracks show here.", in the whole body, with no row
  inset around it.
- **Lyrics:** synced lines in fixed-height rows, romanized and translated
  lines under each in `T.Caption`. Every line is laid out at the active size
  (`T.Section` here, `T.Sentence` on the now playing stage); lines that are
  not active are scaled about their centre to the resting size (`T.Proof`,
  `T.Section` on the stage) in `fg3`. When a line becomes active it scales up
  to full size and brightens over `Motion.Field`, and the line it replaces
  settles back the same way.

  **Word by word** (`look.Karaoke`, while the Word by word preference is on,
  default): the active line's words flow and wrap centred (a line too long for
  its row scales down until it fits, never clipped), unsung words in `fg2`,
  sung words in `Accent.Ink`, and the word being sung fills from its left edge
  behind a 45° cut that follows the clock, rising 2 dp (3 on the stage) while
  it fills; sung words stay up and the line settles when it goes inactive.
  Word timing comes from the lyrics when the source times each word (TTML,
  enhanced LRC, KRC, yrc). Otherwise it is estimated from the line's own
  timing: words split at spaces (CJK runs per character), each weighted by
  its rune count plus one, sung across
  `min(gap − 120 ms, max(500 ms, 1.25 × 70 ms × total weight))` from the
  line's start, where the gap runs to the line's end time or else the next
  line's start. With the preference off the active line is whole in `fg`
  (still scaling). While a word fills, the lyrics ask for frames 1/30 s ahead;
  between words they schedule only the next boundary; paused, none.

  The active line sits one third from the top. Following moves
  the list's anchor (`layout.Position.First`) next to the active line at once,
  so the line is laid out on the first frame, and springs only the remaining
  distance (at most one viewport). Only real wheel, trackpad or drag input on
  the lyrics pauses following, for 5 s, and shows a quiet "Follow" plaque; the
  follow motion itself never counts as a manual scroll. A seek, a track change,
  a speed change or the surface becoming visible anchors at once. The clock is
  the playback position projected with the playback speed, the same clock
  that schedules the next line and word boundary. Clicking a line seeks to it.
  Caption proof: where the lyrics came from. No lyrics: `widgets.EmptyState`
  "No lyrics for this track" in the whole body, with no document inset around
  it; the mini player's square uses a bare `kit.Empty`, since it is not a page.
- **Together:** the Listen Together session (existing behaviour, rebuilt on
  kit components).

### Now playing view (chrome/nowplaying.go)

Opened by the player bar's now playing plaque or the compact player. It
replaces the sidebar, content and panel area with one stone2 pane (x 8 to
W-8, top corners cut by `Cut.Large`) whose whole plane is `Accent.Field`
under the cover backdrop (see Colour). The top bar and player bar stay.
Escape, Back or the plaque closes it. It enters over `Motion.Navigate` (fade,
8 rise).

- Pane width ≥ 900: left column 42 % with the art square
  `min(colW - 110, paneH - 208, 520)`, cut 16, centred; under it, 21, title
  `T.Sentence` (two lines), artists `T.Proof` `fg2` (links), album `T.Caption`
  `fg3`, then 21, an act row of normal quiet icon plaques **8 apart**: like,
  add to playlist, download, start radio, more. Nothing in this row is gold.
  Right column: `kit.Tabs` Lyrics, Queue, then the content; lyrics here sing
  at `T.Sentence` with the others resting at `T.Section` (see Lyrics).
- Narrower: a pane shorter than 720 puts art 120 (cut 7) beside the title,
  artists and act row; a taller pane stacks art `min(paneW - 42, 280,
  paneH - 440)` over them. The Lyrics and Queue tabs and their scroller fill
  the rest.
- The visualizer band runs along the pane's bottom edge (see Visualizer): 88
  high at ≥ 900, 56 narrower, inset 34 (21 narrower) at the sides and 13 from
  the bottom. The columns end 21 above it, so lyrics never run over bars.
- Facts appear only when they exist. Never print "Plays -", "Unknown", an
  empty album or a lone provider word.
- The art crossfades over `Motion.Field` when the track changes.
- The art is the play and pause control (`widgets.PlayArt`): hovering lays a
  stone4 plate (cut 6, alpha .9) with the action's glyph over its centre; a
  click toggles playback, the glyph morphs to the next action and the plate
  stays 700 ms more so a click without hover still answers. Home's now
  playing card and the mini share it.

### Mini window (chrome/mini.go)

Title "Ryotunes Mini", its own window, asked for at 520 × 176 dp times the
zoom; the niri rule opens it at 520 × 176. It is a landscape strip whenever
its width is at least 1.4 times its height or its height is below the portrait
layout's 251 dp fixed rows:

- The cover is a square as tall as the window, flush with its top, left and
  bottom edges (the compositor rounds the window; the art has no cut of its
  own on those sides and a `Cut.Mid` cut on its top-right corner), drawn by
  `widgets.PlayArt`.
- The column to its right has 13 insets (12 at the top for the controls). Row
  one: title `T.Row` `fg` (one line, ellipsis) and, at its right, the 24 square
  expand and close controls 6 apart (icons 16, cut 4, clear at rest, stone3 on
  hover, the plaque press recipe, the shared focus ring). Row two: artists
  `T.Caption` `fg2`. The rest of the column bottom-aligns: the seek row
  (elapsed and duration in `T.Mono` `fg3`, the wave scrubber between), 8,
  then the transport (`widgets.Transport`, 8 apart) at the left and like,
  lyrics and volume at the right. Volume is the icon plaque (click mutes, the
  wheel steps ±5, its bubble shows the percentage) plus a 72 scrubber when the
  column is at least 360 wide.
- At narrower forced-landscape sizes, the seek and action controls appear only
  when their complete row fits; the cover remains the play and pause control.
- Lyrics: the toggle crossfades the title and artists rows into the active
  synced line, sung word by word (`look.Karaoke` at `T.Label`, left aligned,
  one row: a long line scales down to .8, and past that wraps to two rows and
  hides the next line), and the next line (`T.Caption` `fg3`) over
  `Motion.Field`; the controls stay.
- Every blank part of the strip drags the window.
- The window is stone1 under the cover backdrop (see Colour).

A taller window that can fit those fixed rows keeps the v2 portrait layout: a
48 strip with the controls, title and artists centred, the seek row, the
transport (13 apart),
and a 48 bottom row (like, lyrics; volume at the right), with the art taking
what is left (`clamp(availableH - fixedRows, 96, 200)`, cut 7, centred) and
going, with the lyrics toggle, below 96. The two layouts crossfade when a
resize crosses the ratio. The window's content always fits at any size the
compositor gives and any zoom; nothing is ever cut at an edge. Expand returns
to the main window (re-creating it if it was closed) and closes the mini.

### Visualizer (look/visualizer.go, internal/client/spectrum)

Music made visible, carved from the field it sits on. It shows only while
something plays, the visualizer setting is on and motion is neither reduced
nor in low resource mode. **Visualizer style** (`ClientPreferences.Style()`,
`env.Fx.Style`) chooses how every visualizer draws: Bars (default), Curve,
Oscilloscope or Vectorscope. Changing it crossfades the old drawing into the
new over `Motion.Field`.

- **Spectrum feed.** `internal/client/spectrum` runs `cava` (the same analyser
  the Ryoku shell and Ryotunes 1.x use) while a visible Bars or Curve
  visualizer wants it and playback is playing: 64 bands at 60 frames a second,
  `noise_reduction = 30` so it answers quickly,
  mono, raw ASCII on stdout, `method = pipewire`. Its `source` is the sink the
  daemon's playback stream links into, found with `pw-dump` (the
  Stream/Output/Audio node whose `application.process.binary` is `ryotunesd`,
  then its link's input node), so filter sinks such as Easy Effects still feed
  it; `auto` when that cannot be found. The process stops within a second of
  the last visualizer leaving the screen or playback pausing, restarts after
  1.2 s if it dies, and gives up until the next play after three failures in
  30 s. Without `cava` the spectrum styles never show and Settings says why. A
  frame older than 250 ms counts as silence.
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
  (bar 6, gap 3) in headers. The 64 bands are resampled to the bar count.
  Each bar follows its band with a spring that rises fast (ω = 4 / 50 ms) and
  falls slower (ω = 4 / 180 ms). While playing a bar is at least 2 high, so
  the lane reads as a floor in a quiet passage. Blades paint in four alpha
  steps by level (.55, .7, .85 and 1 × the style's alpha), one path per step,
  so loud bands are brighter. **Peak caps**: a 2 thick cap rides 2 above each
  blade's highest point, holds 140 ms, then falls with constant acceleration
  (the lane's height in 550 ms) until it rests on its blade; one path, one
  paint, in the brightest step's ink.
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
  two axes) and draws at most 768 decimated samples as 1.5 dp squares, the two
  previous frames kept at alpha .45 and .18 for persistence. In a lane wider
  than tall it folds `Y` upward into a half diamond 2 × lane height wide,
  centred. Header lanes are too short for it and draw Bars instead.
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
  (header bottom padding 40, lane 32; compact 24 and 18), drawn after the field
  and before the text, so text never sits on it. The now playing band along
  the pane's bottom: `clamp(paneH × .2, 88, 144)` high when the pane is at
  least 900 wide, `clamp(paneH × .14, 56, 96)` narrower, with the columns
  ending 21 above it. The Home now playing card's instrument (see Pages); the
  Home greeting has no lane of its own.

## Pages

Every page fills its pane. Grids and shelves use the full content width; a
table spans it; nothing is a narrow column in a wide empty pane except reading
text (lyrics ≤ 560, empty-state body ≤ 420) and Settings (the Lithic settings
archetype). The header field (`Accent.Field`) sits behind the title or hero and
scrolls with the page.

- **Home:** greeting `T.Sentence` by time of day ("Good evening") and one proof
  line `T.Proof` `fg2` ("Picked from YouTube Music for you", or, signed out, "Sign
  in to shape your YouTube Music home" with a line plaque "Sign in" that starts
  the active provider's sign-in right there). Then the chips row:
  `kit.Filters` with "All" first and the provider's chips after it (no
  Podcasts, spec 09). All shows the quick grid: up to 8 quick tiles (recent and
  pinned collections), 4 columns at content width ≥ 1100, 3 at ≥ 820, else 2, 8
  apart, then shelves: Jump back in (recent), Picked for you, then the provider
  feed's shelves in order. Any other chip replaces everything under the row
  with that chip's shelves (`get_home` with the chip's params): a shelf skeleton
  while it loads, the shelves when they arrive, `widgets.EmptyState` "Nothing
  here yet" for an empty answer, and on failure a placard "Could not load
  <chip>" with the selection back on the previous chip. The page consumes the
  filter's change right after laying it out, in the same frame.

  **Now playing card** (`pages/home`). While a track is loaded (playing,
  paused, loading, or finished but still in the player, exactly when the
  player bar shows it), the greeting's field grows a card under the proof
  line, so greeting and card share the accent field and scroll with the page.
  With nothing loaded there is no card and no placeholder. The card unfolds
  (its height over `Motion.Unfold`, its contents rising 8 and fading in over
  `Motion.Navigate`) when a track first loads and folds away when the player
  empties.

  - A stone1 slab across the content width, 13 padding, its top-left and
    bottom-right corners cut by `Cut.Mid`, under the cover backdrop (see
    Colour).
  - Content width ≥ 720: art 144 (`widgets.PlayArt`, cut 7) │ 21 │ text column
    (flexed) │ 21 │ instrument. The text column's top: a state line (a flake
    and a word: "Playing" with an `Accent.Ink` flake, "Paused" amber, "Loading"
    lapis; then " · From <collection>" in `fg3`, a link, when the queue names
    its source), 8, the title `T.Sentence` `fg` (one line, ellipsis; click
    opens the now playing view), the artists `T.Proof` `fg2` (each a link) and
    the album `T.Caption` `fg3`. Its bottom: the seek row (mono times and the
    wave scrubber, at most 420 wide), 8, the act row: `widgets.Transport{Basic:
    true}` (previous, play in gold 40, next), 21, then like, queue (`PlaqueOn`
    while the queue panel is open), lyrics (`PlaqueOn` while lyrics are open)
    and now playing, small quiet icon plaques 4 apart. At the tight edge of
    this layout, the now playing plaque goes first and then lyrics rather than
    clipping the act row.
  - The instrument: `widgets.Visualizer` in a slot as tall as the art and
    `clamp(cardW × .3, 200, 360)` wide, a square of the art's height for the
    Vectorscope, on a stone2 plate (cut 6, 1 dp vein); the style's short name
    ("Bars", "Curve", "Scope", "Stereo") in `T.Mono` `fg3` at its top-left.
    Clicking it cycles the style and saves it like the Settings row; its tip
    is "Visualizer: Bars · click to change". With the visualizer off,
    unavailable or motion reduced, the slot goes and the text column takes
    the room.
  - Content width 600–719: art 112, no instrument. Narrower: art 72, title,
    artists and the gold play plaque only.
  - A track change crossfades the art (`Motion.Field`), swaps title and
    artists with an 8 rise (`Motion.Navigate`), and the accent retints the
    field and the play plaque.
- **Search results** (`RouteSearch`): "Results for “q”" `T.Section`, 13,
  `kit.Filters` All, Songs, Albums, Artists, Playlists with counts. All: at
  content width ≥ 1000 the top result tile (art 120, name `T.Section`, kind
  caption, a gold "Play" plaque) beside the first four songs (40 / 60 split,
  34 apart); narrower, stacked. Then shelves for Artists, Albums, Playlists.
  A filter shows a track table (Songs) or a card grid.
- **Library:** "Library" `T.Section`, then `kit.Tabs` Playlists, Songs, Albums,
  Artists, Local; at the tab row's right a filter icon plaque that opens a
  280 `kit.Search` narrowing the current tab (Ctrl F) and a line "New
  playlist" plaque on Playlists. Playlists: grid with Liked songs first. Songs:
  track table. Albums and Artists: grids. Local: folder slab rows (path, count,
  Remove) with "Add folder", then the local track table.
- **Album / playlist:** hero in the field, under its cover's backdrop (see
  Colour): art 200 when the hero is at least 760 wide, 160 at 560–759,
  otherwise 120; cut 7; 21; a bottom-aligned column whose natural height may
  make the hero taller than the art: kind
  `T.Group` `fg2`, title `T.Name` (falls to `T.Sentence` under 640 content
  width or past two lines), then meta `T.Proof` `fg2`. Meta wraps only between
  whole facts, uses at most two lines and drops trailing facts that do not fit;
  the artist is a link and counts and durations are mono ("Spiritbox · 2021 ·
  12 songs · 43 min"). Then 13 and the full-size act row 8 apart: Play (gold
  `Plaque` with the play icon, "Pause" while this collection plays), Shuffle,
  Save/like, Download (icon plaques, quiet) and More. When space runs out,
  fold Download, Save/like and Shuffle, in that order, into More; Play and More
  always remain. Then the track table.
- **Artist:** the same hero with the portrait octagon-cut and the act row
  Play (gold), Shuffle, Subscribe (line plaque "Subscribe"/"Subscribed", YouTube
  artists only, through `set_artist_subscribed`), Radio, More (Share).
  Then "Popular" (five table rows and a quiet "Show all"), then shelves:
  Albums, Singles and EPs, Videos, Featured on, Fans also like.
- **List:** a titled card grid for "Show all".
- **Radio:** "Radio" `T.Section` and a sentence; `kit.Filters` Popular,
  Favourites, Recent, By country, By genre; a station grid of tiles (favicon
  56, name `T.Label`, country and codec/bitrate in mono `fg3`); a field that
  narrows stations by name lives in the filter row (it filters this
  catalogue, it is not search). The playing station shows the `volume_up`
  glyph in `Accent.Ink`.
- **Downloads:** "Downloads" and a sentence ("3 downloading, 120 saved");
  `kit.Filters` All, Active, Saved, Failed with counts; `kit.Tablet` rows with
  art in the tablet's 32 mark, title and artists, claim (lapis Downloading with `kit.BladeProgress`,
  jade Saved, jasper Failed with the reason as proof) and one action: Cancel
  while active, Retry when failed or cancelled, and **Play** on a saved row.
  Double clicking a saved row also plays it, from its file on disk with no
  network; Show in folder moves to the row's right click menu (Play, Show in folder). A collection card's
  action plays its saved tracks in order. At content width ≥ 1000 a facts slab on the right:
  location, used space, format.
- **Settings:** the Lithic settings archetype from `layout.md` with
  `kit.SectionIndex` (at medium widths the sections are wrapping `look.FlowChoice` chips, never a sideways strip) and `kit.Section` + slabs, every row from spec 09
  §Settings (`~/Work/ryotunes-rebuild-spec/09-client-ui.md`), including
  **Discord rich presence** under Integrations. About carries the Ryoku mark,
  version in mono and a line plaque to Diagnostics.
- **Diagnostics:** reached from Settings → About only.
- **Equalizer** (`RouteEqualizer`, from the Sound popover and Settings →
  Sound): a parametric equalizer of up to 20 bands, after the one the
  listener asked for (Astra's): a response graph with draggable nodes over a
  strip of faders. Model, presets, the Equalizer APO format and the response
  maths live in `internal/model/equalizer.go` and `internal/eq`.

  - Title row: a small quiet back plaque ("Back"), then "Equalizer"
    `T.Section`; at its right a `kit.Toggle` whose word is "On" or
    "Bypassed", the preset plaque (a line plaque with the preset's name or
    "Custom" and a chevron, opening a `look.Overlays` `kit.Menu`: the
    built-ins, a vein, the saved presets, then "Save as preset"), and quiet
    plaques Import, Export and Reset. The back plaque, like Escape, goes back
    to where the equalizer was opened from; when the equalizer is the first
    page (restored at launch) it opens Settings at Sound instead, so the page
    always has a way out, also in the compact class whose top bar has no Back.
  - Graph: a stone1 slab (cut 6) `clamp(H × .4, 220, 360)` high. Verticals at
    20, 50, 100, 200, 500, 1k, 2k, 5k, 10k and 20k Hz (vein; labels `T.Mono`
    `fg3` under), horizontals at +12, +6, 0, −6 and −12 dB (0 in `fg3`, the
    rest vein; labels at the left). The summed response (`eq.Curve` over 384
    log-spaced frequencies) is a 2 thick line in `Accent.Ink` with the area
    between it and 0 dB filled at alpha .18. Bypassed, the line is `fg3` at .6
    with no fill and the slab's top-right says "Bypassed" with an amber flake.
    While playing, the spectrum feed's bands lie behind on the log axis as
    `fg3` blades at alpha .1.
  - Nodes: a 12 diamond per band at its frequency and gain (shapes without
    gain sit on the curve), `Accent.Fill` with a 1.5 stone1 rim; disabled
    bands are hollow `fg3`; the selected band's node is sel. Hover shows a
    bubble "1.2 kHz · +3.5 dB · Q 1.40" in mono. Dragging (with a pointer
    grab) moves frequency on the log axis and gain within ±12; the wheel over
    a node or the selected band steps Q by ×1.1 a notch (trackpad deltas
    accumulate); a double click on empty graph adds a peak band there while
    fewer than 20 exist; right click opens the band's menu: the seven shapes,
    Enable or Disable, Remove.
  - Strip (look.HScroller when it overflows): Preamp first (a 64 column: a
    vertical fader −24 to +12 dB, its value in mono above, "Pre" under), a
    vein, then a column per band whose controls stay 56 wide and centred in
    it: gain in mono above (an en space for shapes without gain), a vertical
    fader (2 track, a 14 × 3 blade handle; double click resets the gain to
    0), the frequency in mono ("1.2k"), a small quiet shape plaque (glyphs
    drawn in `look`: bell, the two shelves, the two passes, notch, band) that
    opens the shape menu, and "Q 1.40" in mono that steps with the wheel or a
    vertical drag. Every number is centred over its fader. The band columns
    share the strip's width so a few bands fill it: at least 60 apart, at
    most 180, and only when 60 apart does not fit does the strip scroll, with
    step plaques at its ends. A column unfolding or collapsing counts by its
    progress, so the others narrow or widen with it. The selected column's
    controls stand on a sel slab that glides over `Motion.Select`. After the
    strip, outside the scroller so it is always in reach, a "+ Band" tile
    adds a peak band in the widest gap; at 20 bands it is disabled with the
    tip "Twenty bands is the most".
  - Headroom: when the curve's peak is above 0 dB, an amber claim "May clip"
    with the proof "Peaks reach +3.2 dB" and a line plaque "Lower preamp" that
    lowers the preamp by that much. No claim otherwise.
  - Keys with the graph focused: ← → move the selected band a sixth of an
    octave, ↑ ↓ change its gain by 0.5 dB (Shift 0.1), [ and ] change Q,
    Delete removes it, Tab selects the next band.
  - Edits change a local working copy at once, so the curve never lags the
    pointer, and reach the daemon as complete `set_equalizer` states at most
    every 80 ms while dragging and once on release. An `equalizer` event
    replaces the working copy unless a drag is in flight. Any edit clears the
    preset name.
  - Import opens a `kit.Dialog` with a multi-line field ("Paste an AutoEQ or
    Equalizer APO parametric profile") and an optional "Save as" name;
    `import_equalizer` applies it and its warnings show in a placard
    ("Imported 10 bands. Skipped 1 line."). Export copies the Equalizer APO
    text to the clipboard (placard "Copied the profile"). Save as preset asks
    for a name in a dialog. Choosing a preset tweens every node and the curve
    to it over `Motion.Field`.

Page states: `widgets.Loading` (kit.Skeleton shaped like the page), `widgets.
Failed` (kit.Empty with the daemon's message and a Retry line plaque),
`widgets.EmptyState`. A page state owns its spacing: it sits the page's
content inset (34 / 21 / 13 by class) below whatever is above it (hero,
header, filter row; that element drops its own bottom gap) and never touches
a pane edge or a hero; when it is the only thing under a hero it is centred
in the remaining height, its top no closer than that inset, and scrolls with
the hero when that height is too short. Callers pass no insets of their own.
A skeleton is anchored where its content will appear, never centred: a
whole-page `widgets.Loading` sits top-start at the content inset, and
`LoadingHero` is the full-bleed hero at its natural height, flush. Inside a
list, the page lays `widgets.Skeleton` out in place of the rows it stands
for, with their insets and gaps, so nothing moves when they arrive.

## Widgets (widgets/)

```go
// TrackTable is a header row and track rows inside the page's Scroller.
type TrackTable struct{ ... }
type TrackColumns uint8 // bit set: ColArt, ColArtist, ColAlbum, ColPlays, ColAdded
type TrackTableOpts struct {
    Columns  TrackColumns
    Context  string        // menu context: playlist id, "album", "queue", "library"
    SourceID, SourceName string // for PlayCollection
    Number   bool          // index column shows track numbers (albums) or row order
}
func (t *TrackTable) Header(gtx layout.Context, e *env.Env, o TrackTableOpts) layout.Dimensions
func (t *TrackTable) Row(gtx layout.Context, e *env.Env, items []model.Track, i int, o TrackTableOpts) layout.Dimensions
```

Rows are `th.RowHeight()` high (56 comfortable, 48 compact). Columns: index
48 (number at rest; play icon on hover; static `volume_up` in `Accent.Ink` when
playing, pause icon when hovering the playing row) │ Title flexed (art 40 when
`ColArt`, 32 in compact rows, 13, title `T.Row` with an `E` mark for explicit, and the like heart
inline after the title: shown when liked or hovered) │ Artist 21 % │ Album
27 % │ Plays 108 `T.Mono` (only when some row has plays) │ Length 72 `T.Mono`
right aligned │ 40 more plaque (hover). Album pages use Title 66.5 % and
Artist 33.5 % and drop Artist when every row matches the album artist. Under
900 content width the Album column goes; under 720 Plays goes; under 560,
Title carries the artists under it and only Length remains on the right.
Header row 32, `T.Group` `fg3`, 1 dp vein under it. Hover: plane step and the index/trailing reveals (hover in/out).
Single click selects (sel slab, glides); double click or Enter plays from that
row; clicking the index play icon plays; right click opens the item menu.

```go
type Card struct{ ... }      // art (square, or octagon for artists), title 2 lines, subtitle 1 line
func (c *Card) Layout(gtx layout.Context, e *env.Env, item model.BrowseItem, width unit.Dp) layout.Dimensions
type Shelf struct{ ... }     // title row + horizontal look.HScroller of cards
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
    Acts layout.Widget     // the act row, built by the page
}
func (h *Hero) Layout(gtx layout.Context, e *env.Env, c HeroContent) layout.Dimensions
type Header struct{ ... } // retained per page: it owns the header's visualizer
func (h *Header) Layout(gtx layout.Context, e *env.Env, title, sentence string, trailing layout.Widget) layout.Dimensions
func Loading(e *env.Env, shape LoadingShape) layout.Widget
func Skeleton(e *env.Env, shape LoadingShape) layout.Widget
func Failed(e *env.Env, message string, retry *kit.Plaque) layout.Widget
func EmptyState(e *env.Env, icon *widget.Icon, title, body string, actions ...*kit.Plaque) layout.Widget
type ItemMenu struct{ ... } // spec 09 item menus per kind: kit.Menu inside a look.Overlays entry at the press point
type AddToPlaylistDialog struct{ Open bool; Tracks []model.Track }
type EditPlaylistDialog struct{ Open bool; Page *model.PlaylistPage }
func Duration(seconds float64) string // m:ss or h:mm:ss
func Count(n int) string              // 1.2K, 3.4M
```

Card: art square at the card width (cut 7; artists octagon), 8, title
`T.Label` `fg` two lines, subtitle `T.Caption` `fg3` one line ("Album · 2021",
"Artist", "Playlist · 42 songs"). Hover: a stone1 plate (cut 7) fades in
behind the card reaching 8 past the art and the copy on every side; the card's
resting box and its art's alignment with the section header do not move, and
shelves and grids leave the plate that room so it is never clipped. Every
hover plate in the app keeps its content at least 8 from its edges (quick
tiles keep their art flush, as the art is the plate's edge). The **play plate** (40, stone4, cut 6,
`fg` play icon) fades in rising 4 dp at the art's bottom right, 8 inset; under
its own hover the plate turns gold with `onGold` ink. Clicking it plays the
item; clicking the card opens it; right click opens the item menu. Card widths:
176 expanded, 152 medium, 136 compact; shelves space cards 21 apart.

Quick tile: 56 high, stone1, cut 6 on the top-left corner, art 56 flush left,
13, title `T.Label` two lines; hover steps the plane to stone3 and fades in a
32 play plate at the right, 12 inset.

Shared playback pieces, one implementation each:

```go
// Transport: shuffle, previous, play (gold, morphing), next, repeat; Basic keeps
// previous, play and next. Gap defaults to 13.
type Transport struct{ Basic bool; Gap unit.Dp }
func (t *Transport) Layout(gtx layout.Context, e *env.Env, snapshot state.State) layout.Dimensions
func PlayPlaque(gtx layout.Context, th *look.Theme, plaque *kit.IconPlaque, morph *look.Morph, icon *widget.Icon, description string) layout.Dimensions

// Seek: elapsed, the wave scrubber and the duration; previews while dragging,
// commits on release and holds until the daemon passes it. Radio shows "Live".
type Seek struct{}
func (s *Seek) Layout(gtx layout.Context, e *env.Env, snapshot state.State, track model.Track) layout.Dimensions
func (s *Seek) Reconcile(gtx layout.Context, e *env.Env, snapshot state.State, trackID string)
func ProgressFrame(gtx layout.Context, e *env.Env, duration float64, trackPixels int, paused bool)
func PlaybackDuration(reported float64, value *string) float64

// PlayArt: the loaded track's cover as its play and pause control.
type PlayArt struct{ View look.ArtView }
func (p *PlayArt) Layout(gtx layout.Context, e *env.Env, url string, size image.Point, shape look.Shape, fade time.Duration) layout.Dimensions

// Header: Below lays out under the title row inside the field; NoLane drops
// the visualizer lane.
type Header struct{ Sentence, NoLane bool; Below layout.Widget }
```

## look

```go
package look

// Theme is the library theme plus the app's source colours and settings.
type Theme struct {
    *theme.Theme                 // R, T, Cut, Sp, Motion, Density, Shaper
    Src          Source
    Class        Class            // the window's size class, set by the shell each frame
}
type Source struct{ Provider model.Provider; Fill, Ink, Field color.NRGBA }
type Class uint8 // Compact, Medium, Expanded
func ClassFor(widthDp float32) Class

func NewTheme(roles palette.Roles, density theme.Density) *Theme
func (t *Theme) SetRoles(roles palette.Roles)                 // snaps
func (t *Theme) SetProvider(gtx layout.Context, p model.Provider) // tweens Src over Motion.Field
func (t *Theme) Animate(gtx layout.Context)                    // advances the Src tween; requests frames only while moving

// Palette: appearance is "follow", "dark" or "light"; file, when set, is a
// palette JSON (same shape as Ryoku's colors.json) that replaces the source.
func Roles(appearance, file string) palette.Roles
func Watch(ctx context.Context, current func() (appearance, file string), changed func(palette.Roles))

func Fonts() []font.FontFace // embedded Space Grotesk and Space Mono for theme.WithFontCollection

// brand.go: the only file with colour literals.
var RyokuOrange color.NRGBA
func BrandFill(p model.Provider, r palette.Roles) color.NRGBA
func BrandInk(p model.Provider, r palette.Roles) color.NRGBA

// Marks
func RyokuMark(size unit.Dp, ink color.NRGBA) layout.Widget   // the v2 cut-stone 力: strokes in ink (fg), pointer in RyokuOrange
func ProviderGlyph(p model.Provider, size unit.Dp, ink color.NRGBA) layout.Widget
func ProviderName(p model.Provider) string                    // "YouTube Music", "Spotify", "SoundCloud", "TIDAL", "Local files", "Radio"

// Icons: materialdesign icons decoded once.
var Icon struct{ Play, Pause, Next, Previous, Shuffle, Repeat, RepeatOne,
    Heart, HeartOutline, Download, DownloadDone, Queue, Lyrics, NowPlaying,
    Mini, Expand, VolumeOff, VolumeMute, VolumeDown, VolumeUp, Tune, Back,
    Forward, Search, Filter, Close, More, Add, Home, Library, Radio, Downloads,
    Settings, Minimize, Maximize, Restore, Check, Drag, Remove, Folder, Person,
    Album, Playlist, PlaylistAdd, Refresh, Share, OpenInNew, Sparkle, Explicit,
    Equalizer, Sort *widget.Icon }

// Frame geometry, in dp.
const (TopBar = 56; Gap = 8; SidebarExpanded = 240; SidebarMedium = 72
    Panel = 340; PlayerBar = 88; PlayerCompact = 64; BottomNav = 56
    ContentMinWithPanel = 520)
func ContentInset(c Class) unit.Dp // 34, 21, 13

// Scrolling (app-owned: see Library limits). Every delta applies at once,
// with no easing: Wayland deltas times gtx.Metric.PxPerDp; on X11 (detected
// as the library does) a 10-unit notch scrolls 48 dp. Keep the pass-through
// catcher with its own clip, directional ScrollRange at the ends and
// fractional accumulation. The thumb is 3 wide, fg3, shown while scrolling or
// hovered and fading out over hover out.
type Scroller struct{ ... }
func (s *Scroller) Layout(gtx layout.Context, th *Theme, count int, el layout.ListElement) layout.Dimensions
func (s *Scroller) ScrollTo(i int)
func (s *Scroller) Offset() (first, px int)
type HScroller struct{ ... }  // same input rules horizontally; Page(±1) glides by the visible width
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
}
func (s *Scrubber) Layout(gtx layout.Context, th *Theme, value float32, st ScrubberStyle) layout.Dimensions
func (s *Scrubber) Dragging() (f float32, ok bool) // preview while held
func (s *Scrubber) Committed() (f float32, ok bool) // release, click or wheel

// Art: a cover from the cache at a size and shape; crossfades when the url
// changes (duration chosen by the caller).
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
// return a cover's palette and its blurred 24 × 24 sample, both made once per
// cover off the frame goroutine and kept beside it on disk.

// Micro motion
type Hover struct{ ... }   // pointer hover of an area as a tween 0..1 (hover in/out)
func (h *Hover) Layout(gtx layout.Context, th *Theme, w layout.Widget) (layout.Dimensions, float32)
type Swap struct{ ... }    // keyed crossfade with optional rise for text and art changes
func (s *Swap) Layout(gtx layout.Context, th *Theme, key string, rise unit.Dp, d time.Duration, w func(gtx layout.Context, key string) layout.Dimensions) layout.Dimensions
type Morph struct{ ... }   // icon change: outgoing scales to .85 and fades, incoming from .85 (Motion.Notch)
func (m *Morph) Layout(gtx layout.Context, th *Theme, icon *widget.Icon, ink color.NRGBA, size unit.Dp) layout.Dimensions
type Fill struct{ ... }    // a glyph filling from its centre outward as a clip grows (Motion.Notch): the liked heart
func (f *Fill) Layout(gtx layout.Context, th *Theme, on bool, off, onW layout.Widget) layout.Dimensions
func Bubble(gtx layout.Context, th *Theme, at image.Point, text string, alpha float32) // deferred overlay
func Field(gtx layout.Context, th *Theme, cuts [4]int, body layout.Widget) layout.Dimensions // Accent.Field plane
// FieldBackdrop is a field plane with a cover's backdrop; Backdrop.Paint lays
// one on any plane, toned to its colour.
func FieldBackdrop(gtx layout.Context, th *Theme, a Accent, cuts [4]int, b *Backdrop, cache *art.Cache, cover string, body layout.Widget) layout.Dimensions
type Backdrop struct{ ... }
func (b *Backdrop) Paint(gtx layout.Context, th *Theme, cache *art.Cache, cover string, plane color.NRGBA, bounds image.Rectangle, cuts [4]int)

// Overlays places popovers, menus, tips and choices in WINDOW coordinates.
// The shell owns one instance, lays out the frame, then calls Layout last
// with full-window constraints. It also registers a pass-through pointer area
// over the whole window, drawn on top, that records the last press and the
// pointer position in window coordinates.
type Overlays struct{ ... }
type Placement struct {
    Prefer  Side       // Below or Above; flips when the preferred side does not fit
    Align   Align      // Start (left edges) or End (right edges)
    Gap     unit.Dp    // default tokens.SizePopoverAnchorGap
    Width   unit.Dp    // 0: the content's own width
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
func (o *Overlays) Close(tag any)
func (o *Overlays) IsOpen(tag any) bool
func (o *Overlays) Top() (tag any, ok bool)           // for the Escape stack
func (o *Overlays) Layout(gtx layout.Context, th *Theme)
// Each entry: stone4 plane cut tokens.CutMenu, content inset by the menu
// tokens, measured with the window as its maximum, placed against the
// anchor, flipped, clamped 8 inside the window, menu in/out motion, an
// outside press closes it and is swallowed (it never reaches what is
// underneath), Escape closes the top entry, focus returns to the opener.

// Tip is the app's tooltip: after Motion.TooltipDelay on hover, a stone4 slab
// (cut tokens.CutTooltip, T.Caption) through Overlays, below the widget or
// above when there is no room.
type Tip struct{ ... }
func (t *Tip) Layout(gtx layout.Context, th *Theme, o *Overlays, text string, w layout.Widget) layout.Dimensions

// Choice replaces kit.Picker: a field-shaped plaque (stone3, cut 6, 40 high,
// value T.Label, a chevron) that opens a kit.Menu of options through Overlays.
type Choice struct{ ... }
func (c *Choice) Layout(gtx layout.Context, th *Theme, o *Overlays, options []string, selected int) layout.Dimensions
func (c *Choice) Changed() (int, bool)
```

`env.Env` changes: `Th *look.Theme` (was `*lithic.Theme`) and a new
`Overlays *look.Overlays`. Everything else in env stays.

## Micro-interactions

Every motion answers an input or a state change and stops. Durations are the
library's tokens; reduced motion (desktop or app setting) makes each one
instant.

| Interaction | Motion | Token |
|---|---|---|
| press any plaque | cut deepens 4, content sinks 1 (library) | press 90 |
| hover a control, row, tile or card | plane or ink fades in, out | hover in 160 / out 100 |
| play ↔ pause | `look.Morph`: icon scales from .85 and crossfades | notch 200 |
| like | `look.Fill`: the heart fills from its centre in `Accent.Ink`; unliking empties it | notch 200 |
| track changes | every cover showing it (player bar, now playing, Home card, mini) is revealed behind a 45° cut sweeping from its top-left corner past its bottom-right; title and artists rise 8 and fade in | the caller's duration: navigate 200 in the bar, field 320 elsewhere |
| hover a scrubber | blade handle and bubble fade in; only the volume track thickens 2 → 3 | hover in / out |
| drag a scrubber | handle follows the pointer exactly; the wave flattens; seek holds the committed value | field 320 |
| playback starts / pauses | the wave's amplitude eases in and travels / eases out and stops | field 320 |
| music plays | visualizer blades rise fast and fall slower with the bands, brighter when loud; peak caps hold and fall | springs, every frame |
| a beat lands | every visualizer kicks up to 14 % taller and brighter, strokes thicken | decays over 180 ms |
| a word is sung | its highlight fills it behind a 45° cut that follows the clock and it rises 2 | continuous while it fills, 30 frames a second |
| provider switch | active plate glides; source colours retint; content re-enters | select 240, field 320, navigate 200 |
| current destination or selection changes | sel slab glides (sidebar, bottom nav, tables, tabs blade) | select 240 |
| page navigation | new content fades in rising 8 | navigate 200 |
| right panel docks or leaves | slot width unfolds, content clipped and fading | unfold 200 |
| sheet, overlay, menu | fades in sliding the last 6, out retreating 4 | menu in 160 / out 100 |
| card hover | plate fades in; play plate rises 4 and fades in | hover in 160 |
| row hover | index number crossfades to the play icon; like and more fade in | hover in / out |
| queue row removed / added | collapses / notch cut | unfold 200 / notch 200 |
| lyric line becomes active | the list anchors next to it and springs the rest to one third; the line scales up from its resting size and brightens while the previous one settles back | spring, field 320 |
| download completes | ring fills to full, icon morphs to `download_done` | spring, notch 200 |
| shelf arrows | glide one page | wheel 140 |
| placard | rises 8, blade drains (library) | placard in 300 / life 5000 |
| now playing view opens / closes | fade with an 8 rise / fade | navigate 200 / hover out 100 |
| accent changes (new cover, source, preference) | fill, on, ink and field tween together, and with the window tint every stone and ink retints with them | field 320 |
| a cover backdrop's cover changes (new track, a hero's cover arrives) | the new blur fades in as the old one fades out, carrying on from any fade in progress | field 320 |
| corners preference changes | every pane re-cuts in place | select 240 |
| sidebar folds / unfolds | width tweens, labels clip and fade, icons glide to the rail | unfold 200 |
| medium sidebar sheet | slides 6 in from the left and fades / retreats | menu in 160 / out 100 |
| visualizer style changes | the old drawing crossfades into the new | field 320 |
| Home now playing card appears / leaves | height unfolds, contents rise 8 / folds | unfold 200, navigate 200 |
| hover or click the playing art | plate fades in; the glyph morphs; the plate lingers 700 ms after a click | hover in 160, notch 200 |
| equalizer preset chosen or reset | nodes and curve tween to it | field 320 |
| equalizer band added / removed | node grows from nothing, its column unfolds / shrinks and collapses while the other columns narrow / widen with it | notch 200, unfold 200 |
| equalizer bypassed / on | curve fades to fg3 and its fill out / back | field 320 |
| a track loads with nothing loaded / the queue empties | the player bar unfolds from the bottom while the panes' bottoms glide up / folds away | unfold 200 |
| mini resized across landscape and portrait | the two layouts crossfade | field 320 |

Nothing moves at rest: no pulsing, no idle frames. A paused, idle window draws
zero frames. While music plays, the wave progress, the visible visualizers and
the word being sung in visible lyrics are the only things that move without
being touched; they ask for frames at up to 60 a second (30 for the lyric
wipe) and stop as soon as playback pauses, the window hides them, or motion is
reduced. Reduced motion keeps the word by word colours but drops the wipe, the
lift and the line scaling. Position samples that only confirm the client's own
clock do not redraw.

## Settings

The full information architecture of spec 09 §Settings returns, laid out as
the Lithic settings archetype. Sections in order: Account, Playback, Sound,
Lyrics, Downloads, Local music, Playlists, Appearance, Integrations, Data and
storage, Diagnostics, About. Rows that were missing in 2.0.0 and come back:

- Integrations: Discord rich presence first (toggle with the live claim:
  Off, Connecting, Connected, Discord is not running), then the Discord title
  row and its save action; also reachable from the top-bar quick toggle.
- Sound: an "Open sound" line plaque (opens the same overlay as the player
  bar, anchored to that plaque) and the current preset as proof; then
  **Equalizer**, its state as proof ("On · Bass boost", "On · Custom",
  "Bypassed") and an "Open equalizer" line plaque. The Sound popover ends with
  the same row as a click target: the word, the state, and the curve drawn 48
  high (`look` draws it for both); choosing it opens the page and closes the
  popover.
- Local music: the watched folders as rows (path, track count in mono, Rescan,
  Remove) and "Add folder".
- Playlists: Import a playlist (link or file) and Export the queue.
- Diagnostics: a section with the daemon state summary and an "Open
  diagnostics" line plaque.
- About: App, Daemon, Client, and Engine (the playback engine's name and
  version) in mono; updates with Check, Install, View changelog and Show
  release notes; License: `GPL-3.0-or-later` in mono, the copyright line, and
  a "Licenses" line plaque that opens the license and the third-party notices.

The section index (left of the sections at content width ≥ 900) scrolls on its
own when it is taller than the pane and keeps the current section in view.

Skins and decor are not restored: colour comes from the Ryoku palette, or from
the playing cover when the window is tinted. The Appearance section offers
Colours (Follow Ryoku, Dark, Light), Palette file (choose a palette JSON, or
none), Density, Motion, UI scale, Low resource mode, **Visualizer** (on by
default; its detail says "Bars that move with the music in page headers and
the now playing view" or, without cava, "Install cava to see the visualizer"
with the toggle disabled), **Visualizer style** (`look.FlowChoice` Bars, Curve,
Oscilloscope, Vectorscope, disabled while the visualizer is off; without
`pw-record` the last two are disabled and the detail says "Install pw-record
to see the waveform and stereo field"), **Wave progress** (on by default),
**Accent colour** (`look.FlowChoice` Album art, Source, Desktop; details "The
colour of the cover that is playing", "The colour of the service you are
listening on", "Your Ryoku palette's accent"), **Tint the window** (a toggle,
on by default, detail "Every surface takes a tint of the cover that's
playing"; disabled with the detail "Follows the album art accent" while Accent
colour is not Album art) and **Corners** (`look.FlowChoice` Desktop, Square, Fine,
Chisel, Deep, Custom; Custom adds a `kit.Slider` 0 to 32 with the value in
mono; Desktop's detail names what it follows, "Ryoku window corners: 18 dp"),
then **Open as a floating window** (a toggle, on by default; detail "Opens at
its own size above your tiled windows" when on, "Opens tiled in your layout
like any other window" when off, and both add "from the next launch" once it
differs from how this window opened). Every other change applies live with its motion. Reduced motion and low resource
mode turn the visualizer and the wave off without changing the stored
settings.

The Lyrics section offers **Word by word** (the client preference, on by
default, detail "Highlight each word as it's sung, estimated when the lyrics
only time each line"), **Word-timed lyrics** (the daemon's `lyrics_boidu`,
which the daemon treats as on when unset, so the row reads on too; detail "Ask
Boidu first for lyrics timed word by word") and **Romanized lyrics**.

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
  URL opens once. A missing opener or a non-zero exit shows a placard "Could
  not open your browser" and leaves the link on the account row.
- While a flow waits, its account row says "Waiting for your browser" and
  offers "Open again" and "Copy link" (quiet plaques) and Cancel; TIDAL also
  shows its code in mono. The Home header's Sign in starts the active
  provider's flow directly.

## Input

- Scrolling: `look.Scroller` and `look.HScroller` everywhere content scrolls
  (pages, sidebar playlists, queue, lyrics, settings and its index). Every
  wheel or trackpad delta moves the content by exactly delta × PxPerDp in the
  frame it arrives, however fast the deltas come and however different the
  rows' heights are; nothing skips or jumps. Horizontal trackpad deltas move
  shelves; vertical wheel over a shelf scrolls the page.
- Sideways scrolling is only for shelves and the equalizer's band strip, and
  both show where more is: the cut-off item at the edge and step plaques that
  disable at the ends. Short choice sets (Sound presets, visualizer styles,
  accents, corners) wrap onto more lines instead.
- Every clickable is `kit.Click` (or a kit control). Pointer handlers push
  their own clip; catchers use `pointer.PassOp` where content beneath must
  still receive input.
- Keys: Space play/pause (unless a field has focus), Ctrl ← / → previous /
  next, Alt ← / → back / forward, / and Ctrl K search, Ctrl F filter the
  current list, Ctrl L like, Ctrl R refresh, Ctrl , settings, Ctrl Q quit,
  F11 full screen, Ctrl B fold or show the sidebar, Escape closes the topmost
  thing: overlay or menu, sheet, search suggestions, now playing view, then
  leaves the equalizer page the way its back plaque does, then nothing.

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
