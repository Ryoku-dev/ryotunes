# Ryotunes native client: visual system

Status: approved direction (2026-09-05), implementation in progress on `native/design`.

The slogan is "for the sake of power and beauty". The first pass ported the Svelte layout
literally, set in 9–11 px type with per-page margins, and laid it out for a 900 px window; at
1600 px it reads as scattered furniture. This spec replaces that with one system. The structural
reference is Sonora (github.com/nolight132/sonora): a fixed three-column frame, one spacing
rhythm, table-style track lists, page heroes with a fixed art-plus-title block, and the playing
artwork's colour as the adaptive theme. The skin is Ryoku's: ink/bone/paper tokens, Fraunces
display type, Space Grotesk body, tracked mono labels, one-pixel lines, kana marks, grain and
register crosses at the rich decor level.

## 1. One rule for colour

The artwork is the only saturated thing on screen, and it is allowed to bleed:

- `Style.accent`: the playing cover's sampled accent (`components/ArtAccent.qml` writes
  `Playback.artAccent`; same algorithm as the Svelte `artworkAccent`), else `Tokens.sun`.
  Used ONLY for state: the playing row, progress fills, the liked heart, the active tab
  underline, the spectrum ramp, the mini's glow. Never for buttons or text.
- `components/Backdrop.qml`: the 64 px cover blurred once (MultiEffect) and cross-faded on
  track change. App places one behind the whole content area at `strength` 0.22 (dark) /
  0.14 (light); Now Playing and the mini use 0.6. Static between tracks: zero per-frame cost.
- Everything else is Tokens: `paper`, `paperLift`, `ink`, `inkDim`, `inkMuted`, `inkFaint`,
  `line`, `lineSoft`, `lineStrong`, `bone`/`inkOnBone`, `tint5`/`tint10`/`tint16`.
  Primary buttons are bone (Ryoku), not accent (Sonora).
- Rich decor (`Style.decorRich`, Prefs, default on): `Ryoku.Ui.Grain` over the window,
  `Ryoku.Ui.Reg` register crosses behind page heroes, kana marks on section headings and the
  sidebar seals. Calm drops all three; nothing else changes.

## 2. Scale

`Style.sp(n)` = n x 4 px x uiScale. Spacing uses 8 / 12 / 16 / 24 / 32 (`sp(2/3/4/6/8)`).

Type roles (`Style.fs`, px at uiScale 1), at most four per screen:

| role   | px | face            | use                                   |
|--------|----|-----------------|---------------------------------------|
| micro  | 10 | SpaceMono, +1.4 | tracked labels, table headers, eyebrows |
| xs     | 11 | Space Grotesk   | timestamps, badges                     |
| sm     | 13 | Space Grotesk   | secondary text: artist, meta, captions |
| md     | 15 | Space Grotesk   | body: row titles, nav, buttons         |
| lg     | 18 | Space Grotesk 500 | section headings                     |
| xl     | 24 | Fraunces        | panel titles, the mini's track title   |
| title  | 36 | Fraunces        | page hero title                        |
| hero   | 44 | Fraunces        | Home greeting                          |

Geometry: `Style.radius` 8 (controls), `radiusCard` 12 (cards, hero art), row height 52,
control height 36, hero art 168, card 168, sidebar 248, right panel 340, title bar 44,
player bar 88.

## 3. Frame (App.qml)

```
+--------------------------------------------------------------+
| TitleBar 44: [rail] [< >]            [search] [panel] [mini] |
+-------+----------------------------------------+-------------+
| Side  | Content (Backdrop behind, 32 px pad)   | RightPanel  |
| 248   |                                        | 340         |
|       |                                        | Queue|Lyrics|
+-------+----------------------------------------+-------------+
| PlayerBar 88: [art title heart] [transport / seek] [tools]   |
+--------------------------------------------------------------+
```

- Sidebar: brand (力 RYOTUNES / RYOKU // MUSIC), nav rows 40 px (icon 18, label md,
  kana seal at the right when rich), sections DISCOVER / COLLECTION / SYSTEM as micro
  labels with a hairline, Library expands to Songs / Albums / Artists / Playlists / Local.
  Below: PINNED (Personal.picks) as 40 px art + name/kind rows. Edition tag at the foot.
- Content: `Backdrop` at the back, then the routed page over paper at 0.88 alpha (so the
  bloom shows through), padding 32 sides / 24 top / 32 bottom. Pages never set their own
  outer margins; they fill and use the shared components.
- RightPanel: persistent, `App.panelOpen` (default on at >= 1400 px width), tabs Queue |
  Lyrics as a segmented control in a 44 px header with a close chevron. Panel background
  paper 0.92. Hidden below 1100 px.
- PlayerBar: three zones. Left 280: art 48 + title md / artist sm + heart. Centre: transport
  row (shuffle, prev, play 40 primary, next, repeat) centred on the WINDOW, seek row below
  (time xs | slider | time xs) 480 wide max, centred. Right 280: lyrics, queue, volume 96,
  expand (Now Playing stage). The wave is gone: the seek is a hairline with an accent fill.
- Now Playing stage: replaces content + panel when open; big art left (max 480, accent
  glow), lyrics stage right, `Spectrum` ribbon across the foot.

## 4. Components (client/components)

- `PageHero { eyebrow, title, meta, art, round, primaryLabel, onPrimary, liked, onLike,
  onMore }`: art 168 (radiusCard, circle when `round`) | column: eyebrow micro, title
  `fs.title` (2 lines max, elide), meta sm muted, action row `Btn` primary + heart + more.
  Register crosses behind it when rich. Height 200.
- `Btn { text, icon, primary }`: height 36, radius 8, padding 16, md 500; primary = bone.
- `TrackList` + `TrackRow`: table. `showHeader` draws `#  TITLE  ARTIST  ALBUM  PLAYS  LENGTH`
  in micro over a hairline. Rows 52: index 40 (play icon on hover, accent note when
  playing) | art 40 | title md + artist sm | album sm (>= 900 px) | plays sm right (when
  present) | length xs mono 56 | actions 40. Hover `tint5`, playing row `tint10` + accent
  title. Hairlines between rows.
- `MediaCard` 168: art 168 radiusCard, title md, sub sm, hover lift (translate -2, border
  `lineStrong`) via `Ryoku.Ui.Anim`. `round` for artists.
- `Shelf { title, items, more }`: `SectionHeading` + horizontally flicking row of cards,
  gap 16. `CardGrid` for full-page grids: columns = max(2, floor(width / 184)).
- `SectionHeading { title, action, mark }`: lg 500 + optional "See all" + kana mark.
- `TileGrid` (Home pinned / shortcuts): 56 px art tiles, 3 columns >= 1200 else 2.
- `Backdrop`, `ArtAccent`, `MusicDeck` (fixed 560 x 160, art square 160).

## 5. Pages

- Home: greeting hero (`fs.hero`, caption sm, search field 320) with the deck at the right
  when width >= 1240; chips row; Pinned `TileGrid`; shelves: Listen again, Familiar
  artists (round), Forgotten favourites, then the feed's shelves. No inline artist inspector.
- Playlist / Album / Artist: `PageHero` then `TrackList showHeader` (artist: Popular 5 +
  Show all, then Releases as a `CardGrid`).
- Library: tabs (Songs / Albums / Artists / Playlists / Local) as a segmented control under
  a `PageHero`-less title row; grids/tables below.
- Search: field + suggestions; result sections as shelves and a `TrackList`.
- Radio: hero + `CardGrid` of stations (station art 96, name md, tags micro).
- Settings: two-column setting rows (label md + description sm | control), grouped with
  `SectionHeading`; includes Appearance (theme, decor level) and Sound (see 7).
- Queue panel: NOW PLAYING / UP NEXT groups, rows 52, drag to reorder, Clear.
- Lyrics: stage typography: current line `fs.xl` Fraunces in ink, past lines dim, upcoming
  muted, 24 px leading, word highlight in accent when timed. Centred column, max 560.

## 6. Motion and cost

`Ryoku.Ui.Anim`/`Entrance` with `Tokens.dur*` (reduce-motion zeroes them). Transform and
opacity only. Page change: crossfade 150 ms. Hover lift 90 ms. Shelf reveal: Entrance
stagger. The spectrum (`Spectrum` singleton, cava on the PipeWire monitor, 40 bands 30 fps)
runs only while a claiming surface is visible AND something is playing; surfaces claim with
`Spectrum.claim(id, on)`. Static layers re-render only on track change.

## 7. Sound (a Ryotunes feature normal players do not have)

Daemon: `set_audio_fx { speed, semitones, reverb, bass, width }` over the socket; the mpv
`af` chain gains `aecho` (reverb 0..1), `bass=g=` (bass -6..+12 dB) and `stereowiden`
(width 0..1) after the existing gain and rubberband pitch; `get_audio_fx` and an
`audio-fx` event carry the state so every client shows the same values. Client: a Sound
dialog from the player bar's tools and the track menu: sliders for Tempo (0.25-2.0),
Pitch (-12..+12 st), Reverb, Bass, Width, and presets: Flat, Slowed + Reverb (0.85x,
reverb 0.6), Nightcore (1.25x, +2 st), Bass Boost (+8 dB), Wide (width 0.8).

## 8. Mini

The Tauri widget's geometry: 724 x 356, layer-shell surface over the work area with the
widget as a draggable item inside it (position persists), above windows, no exclusive zone.
Artwork column 232 with the track over its foot, head with brand / tabs / like / maximize,
body (Now: title `fs.xl` Fraunces + artist, seek, next; Lyrics; Queue), footer transport +
volume. Backdrop 0.6 behind the main column, accent glow top-left, `Spectrum` ribbon along
the artwork's foot while playing.

## 9. Blur, motion and the resource budget

Beauty here is paid for once, not per frame.

Blur. Two kinds, both cheap by construction:
- *Static blur*: the `Backdrop` (64 px cover, one MultiEffect pass on track change). Frosted
  panels (sidebar, right panel, player bar, menus) are paper at alpha OVER that backdrop, so
  they read as glass without blurring live content. Never `MultiEffect` a live subtree.
- *Snapshot blur*: a modal (Sound dialog, command palette, add-to-playlist) blurs the page
  behind it once on open (`layer.enabled` + `layer.live: false` + blur), then shows a scrim.
  Closed dialogs unload (`Loader.active: false`).

Motion. Two kinds, each gated:
- *Active* (answers an input): hover lift and border, press scale 0.97, tab indicator slide,
  right panel slide-in, page crossfade + 8 px rise, art and title swap on track change,
  heart pop, play/pause morph, shelf `Entrance` stagger, queue reorder. Durations from
  `Tokens.dur*` (90-300 ms), curves from `Ryoku.Ui.Anim`; `Animator` types for opacity,
  scale, x/y so the render thread carries them.
- *Passive* (ambient, while listening): the backdrop bloom drifts (scale 1.0-1.06, 12 s,
  stepped by a 10 fps Timer, not per vsync), the LIVE dot breathes at 1 Hz, the spectrum
  ribbon on the mini and the Now Playing stage (30 fps). All of it runs only under
  `Style.ambient`: something is playing AND the surface is visible AND Tokens.reduceMotion
  is off AND the power profile is not power-saver (`Style.powerSaver`, polled from
  powerprofilesctl every 30 s). Paused or hidden: still, at once.

Budget (Ryzen 7940HS, measured with the tools in the plan docs): window open and idle,
paused: <= 0.3% CPU. Playing, ambient on, no spectrum: <= 3%. Spectrum surface visible:
<= 6% including cava. The static-layer rule: a `layer.enabled` item must either be static
(re-rendered on data change only) or small (the mini, the deck art).

## 10. Implementation notes

- `import Ryoku.Ui as RU` (qualified): Ryoku.Ui ships its own `Btn`, `Icon`-like names that
  shadow the client's components when imported bare. Use `RU.Reg`, `RU.Grain`, `RU.Anim`,
  `RU.Entrance`, `RU.SpectrumField`, `RU.DitherImage`.
- Client singletons: `Style` (scale, fs roles, accent, ambient, decorRich), `Prefs`,
  `Spectrum` (claim/levels/energy), `Playback`, `Daemon`, `Router`, `Personal`.
- Shared components: `Backdrop`, `ArtAccent` (one per window), `Btn`, `IconButton`
  (`outlined`), `PageHero`, `Artwork` (`cornerRadius`), `TrackList`/`TrackRow`,
  `MediaCard`, `Shelf`, `CardGrid`, `SectionHeading`, `Slider`, `Chip`, `Pill`, `Menu`.
- Icons: `Icon { name }` from `lib/glyphs.js` (baked 24 px stroke paths). Add a glyph by
  appending to the table (d, fill, cap, w); names now include more, sound, panel, sidebar,
  expand, chevron-down, chevron-right, sort.
