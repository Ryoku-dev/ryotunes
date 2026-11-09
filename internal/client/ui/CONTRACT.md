# Client UI contract, v2

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
| stone4 | popovers, menus, sheets, the card play plate |
| sel / onSel | where you are: current nav row, selected row, active tab blade, a toggle that is on (shuffle, repeat, open panel) |
| gold / onGold | play. The player bar's play/pause is the window's standing gold action; a page may spend one more gold on the action that plays it (hero Play, top search result Play). Nothing else is gold: not radio, not download, not navigation. |
| goldInk | line plaques that accompany a gold action |
| jade lapis amber jasper | state, always with a shape and a word: downloading (lapis), saved (jade), paused (amber), failed (jasper), live stream (jade) |
| fg fg2 fg3 vein | ink and hairlines |

**Source colour.** The active provider has a brand colour. It is identity,
not state, so it may stand without a word. `look/brand.go` holds the fixed
brand fills of YouTube Music, Spotify, SoundCloud and TIDAL and the Ryoku mark
orange, the same values 1.x used. `look.Theme.Src` carries
the active provider's colours, tweened over `Motion.Field` (320 ms) when the
provider changes:

- `Src.Fill` the brand colour, for planes;
- `Src.Ink` the brand re-toned along HSL lightness to 4.5:1 on stone2 (the
  same rule the library applies to goldInk and jasper), for type, glyphs and
  fills;
- `Src.Field` `palette.Mix(R.Stone2, Src.Fill, tokens.AlphaMineral)`, the
  header field plane.

Where the source colour shows (and nowhere else):

1. the provider switcher's active cell (field plate, 1 dp `Src.Ink` hairline at
   alpha .55, glyph in `Src.Ink`);
2. the header field behind every page title and hero, and behind the now
   playing view;
3. the progress fill of the seek scrubber while playing (fg2 while paused);
4. a liked heart (filled, `Src.Ink`);
5. the playing track: its title and index glyph in track tables, the queue's
   now-playing row, the sidebar row of the collection that is playing;
6. the active lyric line's sung words.

Local files and radio have no brand: when local or radio is the active
provider the source colour is `R.Lapis` (fill) and its re-toned ink.

## Frame geometry (dp)

`W`, `H` are the window size. Classes come from `tokens.Breakpoint*`:
compact `W < 760`, medium `760 <= W < 1280`, expanded `W >= 1280`. Structure
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
| right panel | docked 340 when `W - sidebar - 340 - 24 >= 560`, else an overlay sheet 340 wide (max `W - 72`) over the content's right edge | same rule | full sheet over the content, opened only by an explicit action |
| player bar | 88, three zones | 88, three zones, narrower | compact player 64 above the bottom nav |

Crossing into compact closes a docked panel (it would otherwise cover the
page) and remembers it; crossing back out restores it. A sheet never opens by
itself on a class change.

Panes cut their two top corners at 16 (`th.Cut.Large`) and keep the bottom
square. Each pane pushes its own clip. The window minimum is 480 × 560; the
default is 1280 × 800.

### Top bar (chrome/topbar.go)

On ground, 56 high, every blank part drags the window (`system.ActionMove`;
double click toggles maximise).

- **Expanded:** over the sidebar column, the Ryoku mark (24) at x 21, y 16,
  then 8 and the word "Ryotunes" in `T.Title` `fg`. At the content's left
  edge: Back and Forward (`IconPlaque` small, quiet, 4 apart; disabled ink
  when the router cannot move). 13 to their right, the search field
  (`kit.Search`) flexes up to 520 wide. At the right: the provider switcher,
  13, then window controls (maximise or restore, close; small quiet icon
  plaques 4 apart) when the client draws its own decorations. There is no
  minimise: niri and Hyprland have no minimised state, and Gio cannot read
  the compositor's capabilities to show it only where it works.
- **Medium:** the mark alone, centred over the 72 rail; the rest as expanded.
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
   Rows are 48 high: cover 36 (cut 3), 13, title `T.Label` `fg` and a caption
   `fg3` ("Playlist", "Album", "Liked songs · 128" with the count in mono).
   Order: Liked songs (a heart glyph on a `Src.Field` tile), then pins
   (`state.Personal.Pins`), then library playlists. The open collection's row
   is the sel slab; the playing collection shows a static `volume_up` glyph in
   `Src.Ink` at the row's right. Right click opens the item menu.
4. Settings is a 40 row pinned 13 above the bottom, under a 1 dp vein hairline
   inset 8.

**Medium (72):** the same order with 56 × 48 icon tiles (`look.Tip` carries the
label), playlist covers 40 (cut 3) centred with tips, the "+" plaque
centred, Settings at the bottom.

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
  liked = filled heart in `Src.Ink`) │ 4 │ download (small quiet icon plaque;
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

Scrubbers (look.Scrubber): 24 high hit area across the full length, 4 dp track
(stone3) that thickens to 6 on hover, fill from the left, a 12 square handle
(cut 3, `fg`). The seek handle fades in on hover or drag; the volume handle is
always shown. Press anywhere jumps there and grabs the pointer
(`pointer.GrabCmd`) so the drag continues outside the 24 band until release. A
bubble (stone4, cut 4, `T.Mono`) floats 8 above the pointer while hovering or
dragging: the time under the pointer for seek, the percentage for volume. Seek
previews while dragging and commits on release; the position then holds at the
committed value until the daemon reports past it (no jump back). Volume sends
`set_volume` at most every 50 ms while dragging and persists on release. Wheel
or trackpad over volume: ±5 per notch, trackpad deltas accumulate to the same
step; over seek: ±5 s per notch. Keyboard when focused: Left/Right step.

**Sound popover** (chrome/sound.go): a `look.Overlays` entry anchored to the
sound plaque's window rectangle, placed above the bar, right edges aligned and
clamped 8 inside the window. 340 wide, stone4. "Sound" in `T.Section`, 13, preset `kit.Filters`
(Flat, Slowed and reverb, Nightcore, Bass boost, Wide), 21, five rows of label
`T.Label`, value `T.Mono` right, `kit.Slider` below: Tempo 0.25 to 2.0 ×,
Pitch −12 to 12 st, Reverb 0 to 100 %, Bass −6 to 12 dB, Width 0 to 100 %;
changes debounce 80 ms into `set_audio_fx`; a quiet "Reset" plaque at the end.

**Compact player** (64, stone1, cut 16 top corners, above the bottom nav): art 44
│ 13 │ title and artist (flexed) │ 8 │ like │ 8 │ play/pause (gold 40) │ 4 │
next. A 2 dp blade across the bar's top edge shows progress in `Src.Ink`.
Tapping the art or text opens the now playing view.

Progress drawing: while playing, the bar asks for its next frame only when the
fill would move by one pixel (`duration / trackPx`, at least 250 ms) and when
the elapsed second changes. Paused or hidden, it asks for none.

### Right panel (chrome/rightpanel.go, queue.go, lyrics.go, together.go)

stone1, 340 wide. A 48 header: `kit.Tabs` Queue, Lyrics, Together at x 13, a
small quiet close plaque 13 from the right. Docked, the panel's slot unfolds
its width over `Motion.Unfold` (200) with the content clipped and fading;
as a sheet it enters like a popover (`Motion.MenuIn`, 6 dp travel) and closes
on Escape or an outside press, which it swallows.

- **Queue:** sections with `kit.GroupHeader`: "Now playing" (the current track
  row: art 40, title `T.Row` in `Src.Ink`, artists, and a caption "From
  <collection>"), "Up next" with its count and a quiet "Clear" plaque, then
  "Autoplay" for the continuation in `fg2`. Rows 56: art 40, title, artists,
  duration `T.Mono` `fg3`; hover reveals a drag grip on the left and a remove
  plaque in place of the duration. Click plays from that row; right click
  opens the item menu; drag reorders with a pointer grab. A removed row
  collapses over `Motion.Unfold`; an added row cuts a notch over
  `Motion.Notch`. The header's more menu: Stop after this track, Save queue as
  playlist, Clear. Empty: `kit.Empty` "Nothing queued" / "Play something and
  the next tracks show here."
- **Lyrics:** synced lines, the active one `T.Section` `fg` with sung words in
  `Src.Ink`, others `T.Proof` `fg3`, romanized line under each in
  `T.Caption`. The active line glides to one third from the top with the
  library spring; a manual scroll pauses following for 5 s and shows a quiet
  "Follow" plaque. Clicking a line seeks to it. Caption proof: where the lyrics
  came from. No lyrics: `kit.Empty` "No lyrics for this track".
- **Together:** the Listen Together session (existing behaviour, rebuilt on
  kit components).

### Now playing view (chrome/nowplaying.go)

Opened by the player bar's now playing plaque or the compact player. It
replaces the sidebar, content and panel area with one stone2 pane (x 8 to
W-8, top corners cut 16) whose whole plane is `Src.Field`. The top bar and
player bar stay. Escape, Back or the plaque closes it. It enters over
`Motion.Navigate` (fade, 8 rise).

- Pane width ≥ 900: left column 42 % with the art square
  `min(colW - 110, paneH - 208, 520)`, cut 16, centred; under it, 21, title
  `T.Sentence` (two lines), artists `T.Proof` `fg2` (links), album `T.Caption`
  `fg3`, then 21, an act row of normal quiet icon plaques **8 apart**: like,
  add to playlist, download, start radio, more. Nothing in this row is gold.
  Right column: `kit.Tabs` Lyrics, Queue, then the content; lyrics here use
  `T.Sentence` for the active line and `T.Section` `fg3` for the others.
- Narrower: one scrolling column: art `min(paneW - 42, 360)`, title, act row,
  then the lyrics.
- Facts appear only when they exist. Never print "Plays -", "Unknown", an
  empty album or a lone provider word.
- The art crossfades over `Motion.Field` when the track changes.

### Mini window (chrome/mini.go)

Title "Ryotunes Mini", 360 × 420, its own window. 32 strip on top (drag;
"expand" and close small quiet plaques at the right). Art 200 centred (cut 7);
21; title `T.Row` and artists `T.Caption` centred; 13; seek row; 13; transport
row (as the player bar, 13 apart); 13; bottom row: like, lyrics toggle (swaps
the art for compact lyrics), volume icon and a 100 scrubber. Expand returns to
the main window (re-creating it if it was closed) and closes the mini.

## Pages

Every page fills its pane. Grids and shelves use the full content width; a
table spans it; nothing is a narrow column in a wide empty pane except reading
text (lyrics ≤ 560, empty-state body ≤ 420) and Settings (the Lithic settings
archetype). The header field (`Src.Field`) sits behind the title or hero and
scrolls with the page.

- **Home:** greeting `T.Sentence` by time of day ("Good evening") and one proof
  line `T.Proof` `fg2` ("Picked from YouTube Music for you", or a sign-in
  line with a line plaque "Sign in" when signed out). Then the quick grid: up to
  8 quick tiles (recent and pinned collections), 4 columns at content width
  ≥ 1100, 3 at ≥ 820, else 2, 8 apart. Then shelves: Jump back in (recent),
  Picked for you, then the provider feed's shelves in order.
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
- **Album / playlist:** hero in the field: art 200 (160 medium, 120 compact),
  cut 7; 21; a bottom-aligned column: kind `T.Group` `fg2`, title `T.Name`
  (falls to `T.Sentence` under 640 content width or past two lines), meta
  `T.Proof` `fg2` with the artist as a link and counts and durations in mono
  ("Spiritbox · 2021 · 12 songs · 43 min"), then 13 and the act row 8 apart:
  Play (gold `Plaque` with the play icon, "Pause" while this collection
  plays), Shuffle, Save/like, Download (icon plaques, quiet) and More. Then the
  track table.
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
  glyph in `Src.Ink`.
- **Downloads:** "Downloads" and a sentence ("3 downloading, 120 saved");
  `kit.Filters` All, Active, Saved, Failed with counts; `kit.Tablet` rows with
  art in the tablet's 32 mark, title and artists, claim (lapis Downloading with `kit.BladeProgress`,
  jade Saved, jasper Failed with the reason as proof), one action (Cancel,
  Retry, Show in folder). At content width ≥ 1000 a facts slab on the right:
  location, used space, format.
- **Settings:** the Lithic settings archetype from `layout.md` with
  `kit.SectionIndex` and `kit.Section` + slabs, every row from spec 09
  §Settings (`~/Work/ryotunes-rebuild-spec/09-client-ui.md`), including
  **Discord rich presence** under Integrations. About carries the Ryoku mark,
  version in mono and a line plaque to Diagnostics.
- **Diagnostics:** reached from Settings → About only.

Page states: `widgets.Loading` (kit.Skeleton shaped like the page), `widgets.
Failed` (kit.Empty with the daemon's message and a Retry line plaque),
`widgets.EmptyState`.

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
48 (number at rest; play icon on hover; static `volume_up` in `Src.Ink` when
playing, pause icon when hovering the playing row) │ Title flexed (art 40 when
`ColArt`, 13, title `T.Row` with an `E` mark for explicit, and the like heart
inline after the title: shown when liked or hovered) │ Artist 21 % │ Album
27 % │ Plays 108 `T.Mono` (only when some row has plays) │ Length 72 `T.Mono`
right aligned │ 40 more plaque (hover). Album pages use Title 66.5 % and
Artist 33.5 % and drop Artist when every row matches the album artist. Under 900
content width the Album column goes; in compact, Title carries the artists
under it and only Length remains. Header row 32, `T.Group` `fg3`, 1 dp vein
under it. Hover: plane step and the index/trailing reveals (hover in/out).
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
func PageHeader(e *env.Env, title, sentence string, trailing layout.Widget) layout.Widget
func Loading(e *env.Env, shape LoadingShape) layout.Widget
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
behind the card with 8 padding, and the **play plate** (40, stone4, cut 6,
`fg` play icon) fades in rising 4 dp at the art's bottom right, 8 inset; under
its own hover the plate turns gold with `onGold` ink. Clicking it plays the
item; clicking the card opens it; right click opens the item menu. Card widths:
176 expanded, 152 medium, 136 compact; shelves space cards 21 apart.

Quick tile: 56 high, stone1, cut 6 on the top-left corner, art 56 flush left,
13, title `T.Label` two lines; hover steps the plane to stone3 and fades in a
32 play plate at the right, 12 inset.

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
func RyokuMark(size unit.Dp) layout.Widget                    // 力, in RyokuOrange
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
    ContentMinWithPanel = 560)
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
func Field(gtx layout.Context, th *Theme, cuts [4]int, body layout.Widget) layout.Dimensions // Src.Field plane

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
| like | `look.Fill`: the heart fills from its centre in `Src.Ink`; unliking empties it | notch 200 |
| track changes | player-bar art crossfades; title and artists rise 8 and fade in | navigate 200 |
| track changes in the now playing view | art crossfades | field 320 |
| hover a scrubber | track 4 → 6, handle and bubble fade in | hover in / out |
| drag a scrubber | handle follows the pointer exactly; seek holds the committed value | none |
| provider switch | active plate glides; source colours retint; content re-enters | select 240, field 320, navigate 200 |
| current destination or selection changes | sel slab glides (sidebar, bottom nav, tables, tabs blade) | select 240 |
| page navigation | new content fades in rising 8 | navigate 200 |
| right panel docks or leaves | slot width unfolds, content clipped and fading | unfold 200 |
| sheet, overlay, menu | fades in sliding the last 6, out retreating 4 | menu in 160 / out 100 |
| card hover | plate fades in; play plate rises 4 and fades in | hover in 160 |
| row hover | index number crossfades to the play icon; like and more fade in | hover in / out |
| queue row removed / added | collapses / notch cut | unfold 200 / notch 200 |
| lyric line becomes active | glides to one third with the spring; ink fg3 → fg | spring, hover in |
| download completes | ring fills to full, icon morphs to `download_done` | spring, notch 200 |
| shelf arrows | glide one page | wheel 140 |
| placard | rises 8, blade drains (library) | placard in 300 / life 5000 |
| now playing view opens / closes | fade with an 8 rise / fade | navigate 200 / hover out 100 |

Nothing moves at rest: no pulsing, no looping equalizers, no idle frames. A
paused, idle window draws zero frames; while playing, only the progress and
the elapsed time ask for frames, at the cadence above.

## Settings

The full information architecture of spec 09 §Settings returns, laid out as
the Lithic settings archetype. Sections in order: Account, Playback, Sound,
Lyrics, Downloads, Local music, Playlists, Appearance, Integrations, Data and
storage, Diagnostics, About. Rows that were missing in 2.0.0 and come back:

- Integrations: Discord rich presence first (toggle with the live claim:
  Off, Connecting, Connected, Discord is not running), then the Discord title
  row and its save action; also reachable from the top-bar quick toggle.
- Sound: an "Open sound" line plaque (opens the same overlay as the player
  bar, anchored to that plaque) and the current preset as proof.
- Local music: the watched folders as rows (path, track count in mono, Rescan,
  Remove) and "Add folder".
- Playlists: Import a playlist (link or file) and Export the queue.
- Diagnostics: a section with the daemon state summary and an "Open
  diagnostics" line plaque.
- About: App, Daemon, Client, and Engine (the playback engine's name and
  version) in mono; updates with Check, Install, View changelog and Show
  release notes.

Skins and decor are not restored: colour comes from the Ryoku palette. The
Appearance section offers Colours (Follow Ryoku, Dark, Light), Palette file
(choose a palette JSON, or none), Density, Motion, UI scale and Low resource
mode.

## Input

- Scrolling: `look.Scroller` and `look.HScroller` everywhere content scrolls
  (pages, sidebar playlists, queue, lyrics, settings). Horizontal trackpad
  deltas move shelves; vertical wheel over a shelf scrolls the page.
- Every clickable is `kit.Click` (or a kit control). Pointer handlers push
  their own clip; catchers use `pointer.PassOp` where content beneath must
  still receive input.
- Keys: Space play/pause (unless a field has focus), Ctrl ← / → previous /
  next, Alt ← / → back / forward, / and Ctrl K search, Ctrl F filter the
  current list, Ctrl L like, Ctrl R refresh, Ctrl , settings, Ctrl Q quit,
  F11 full screen, Escape closes the topmost thing: overlay or menu, sheet,
  search suggestions, now playing view, then nothing.

## Windows and lifetime (shell)

- app id `ryotunes`, title "Ryotunes", min 480 × 560, default 1280 × 800,
  decorations per preference (default: the client draws its own). The niri
  provider floats `^ryotunes$` at 1280 × 800 and "Ryotunes Mini" at 360 × 420;
  Hyprland already floats `^ryotunes$`.
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
  lock and spawns `ryotunes`; when it cannot, it broadcasts `show` to its
  subscribers. `ryotunes-cli events` never counts as a window.
- The main window's title is "Ryotunes", the mini's "Ryotunes Mini"; both use
  app id `ryotunes`, so compositor rules tell them apart by title.
