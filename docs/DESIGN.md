# Ryotunes design

Ryotunes is part of Ryoku and looks like it. It speaks the Ryoku Hub's
language (the Ryoku Settings app) and follows the Style page of Ryoku Settings
for everything the desktop decides: design, colours, light or dark, contrast,
tint, reduced motion and the Japanese gloss. Change the style once in Ryoku
Settings and Ryotunes changes with every other Ryoku app, live.

It draws with the shared Lithic library from the Ryoku repository,
`github.com/ryoku-dev/ryoku/docs/lithic/library/go`. The library reads the
desktop (`palette.ReadDesktop`, `palette.WatchDesktop`), builds the theme for
it (`theme.NewFor`), draws every control in the chosen design, and carries the
Hub's sheet parts, washes, motion tokens, fonts and Material Symbols.
`internal/client/ui/look` holds what is ours: the music's colours (accent,
cover tint, blurred cover), the Ryoku mark, provider glyphs, the frame
geometry, washes and motion helpers on top of the library's, scrolling,
scrubbers, artwork, the visualizer styles, and the equalizer's graph and
faders. `internal/client/ui/sheet` holds Ryotunes' places on the Hub's sheet.
`internal/client/ui/CONTRACT.md` has every measurement.

## Window layout

The window is laid out like a music player, not like a list and detail tool:

1. The sidebar tree on the window paper at the left edge: the Ryoku mark and
   RYOTUNES, then the numbered places Home, Library, Radio and Downloads, the
   Playlists folder with your collections as leaves, and Settings at the foot.
   On wide windows it folds to a rail and back (the toggle beside Back, or
   Ctrl B) and remembers the choice. On medium widths it is the rail and the
   toggle opens the full tree as a drawer. On compact ones it is a bottom bar.
2. A top bar on the same paper: back and forward, the one search field, the
   Discord toggle, the provider switcher and window controls.
3. The page pane with the current page on its sheet: masthead on top,
   colophon at the foot, the registration marks under it. Home opens with a
   now playing card inside its greeting while something is loaded: the cover
   (click to play or pause), the track, the seek bar, the transport and an
   instrument showing the visualizer.
4. The side panel with Queue, Lyrics and Together. It docks while the page
   keeps 520 dp and becomes a drawer when it would not.
5. The player bar across the bottom: what plays on the left, transport over
   the progress in the centre, panels, volume and sound on the right.

The now playing view takes the page pane with large artwork (click it to play
or pause) and lyrics, with its own masthead. The mini player is a second
window, "Ryotunes Mini", in the same process and the same state: a landscape
strip with the cover flush along its left edge, falling back to a portrait
card when the window is made tall. The equalizer is a page of its own,
reached from the sound popover and Settings.

Structure changes at the size-class boundaries (760 and 1100 dp, measured
after the app's zoom) instead of squeezing the wide layout. Entering the
compact class closes a docked panel so it never covers the page.

## The desktop's look

Ryotunes keeps no copy of the Style page. Settings, Appearance shows the
desktop's Design, Colours, Light or dark, Contrast, Tint (for Lithic on the
wallpaper's colours) and Reduce motion as read-only claims, with an "Open
Ryoku Settings" row that opens the Style page (`ryoku-shell hub open style`).
Those words roll to the new value when the desktop changes. The old Colours,
Palette file, Corners and Motion rows are gone. `--mode` and the library's
`LITHIC_MODE`, `LITHIC_DESIGN` and `LITHIC_PALETTE_JSON` stay for demo renders
and tests.

A style change snaps: the next frame wears it in both windows. Only the
music's colours tween.

## The Hub's language

### One paper

The window is one paper (the design's side plane). The sidebar tree, the top
bar and the compact bottom nav stand bare on it. The page, the docked panel,
the player bar and the mini window are panes: the design's pane plane with
its hairline, cut 16 at the top and 7 at the foot. Slabs inside a pane (Home's
card, tiles, Settings and equalizer cards) use the design's card plane.

### The four styles

Every shape Ryotunes draws goes through the library's style-aware painters,
so each design changes corners and planes exactly as the Hub does.

| Design | Corners | Window paper | Pane | Card |
|---|---|---|---|---|
| Lithic | 45 degree cuts (16 top, 7 foot on panes) | ground | stone2 with a vein hairline | slab with a vein hairline |
| Material | round, cut × 2.6, at most 28, pill under 56 | ground | stone1 | stone3 |
| Adwaita | round, cut × 1.6, at most 14, pill under 26 | stone3 | ground | stone4 |
| Fluent | round, cut × .75, at most 8, pill under 22 | ground | stone3 with a vein hairline | stone4 with a vein hairline at .7 |

Covers follow the round designs as rounded squares, never circles: the
corner is capped at a quarter of the cover. Artist portraits become circles.
Some things stay angular in every design on purpose: the visualizer's blades
(an owner ruling), the 45 degree sweep that reveals a new cover, notches,
blades and flakes, and the equalizer's diamond nodes.

### The sidebar tree

The Hub's tree, 272 wide (72 folded). Each place is a numbered line: its
numeral in mono, its name in spaced capitals, a leader, its gloss while the
gloss is on, and a count at the tail. The Playlists folder opens with a
chevron that turns with a small overshoot, its leaves sliding in one after
another on a guide line with branch ticks, the covers kept (a music player
shows its covers). New playlist is the folder's first leaf. One selection
wash travels between lines and lands when it changes region. While Settings
is open its sections hang under it as an outline whose node grows for the
section being read.

### The sheet

Every page sits on a sheet: the registration marks (a dot grid with pluses
and four print-register crosses), the masthead at the top, the colophon at
the foot.

- **Masthead:** "§" and the place's number in the accent, the place and its
  part in spaced capitals ("SETTINGS / APPEARANCE"), the gloss, and at the
  right the page place ("P 06/10"), the provider and the clock, with a tick
  ruler whose marker follows how far you have read.
- **Colophon:** the place's id as Code 39 bars with its words under them, and
  Ryoku's motto 力と美のために beside them while the gloss is on.
- **Page head:** the title at 32/40, a lead sentence, a spec strip of counted
  facts in mono capitals ("PLAYLISTS 24 · SONGS 128"), and the tategaki (the
  page's kanji down the right edge) while the gloss is on.
- **Sections:** ruled heads, a title, a hairline and a mark "02 / 07".
- **Waits:** nothing for the first second, then the working flakes with a
  word, then past ten seconds a blade and a way back.

### Numbering and glosses

| Place | Number | Gloss |
|---|---|---|
| Home | 00 | 表紙 |
| Library | 01 | 書庫 |
| Radio | 02 | 放送 |
| Downloads | 03 | 保存 |
| Playlists | 04 | 曲集 |
| Settings | 05 (sections 05.1 to 05.12) | 設定 |
| Search | 06 | 検索 |
| Now playing | 07 | 再生 |
| Equalizer | 08 | 調音 |
| Diagnostics | 09 | none |

Ryotunes itself is 音楽. Settings' sections: Account 利用者, Playback 再生,
Sound 音, Lyrics 歌詞, Downloads 保存, Local music 手元, Playlists 曲集,
Appearance 外観, Integrations 連携, Data and storage 保管, Diagnostics 診断,
About 概要. Library's tabs: Playlists 曲集, Songs 曲, Albums 盤, Artists 演者,
Local 手元. Home, Settings, Sound, Appearance and About reuse the Hub's own
words; the rest are proposals for the owner to confirm. Every gloss follows
the desktop's Japanese gloss setting.

### Washes

Where you are is a wash, not a solid slab: the accent at .16 (.14 in light
mode) with ordinary ink, and the accent only on the marks (the numeral, the
glyph, the index). A hover is fg at .06 (.07), a press .10 (.12), and an
app-owned row leans 2 dp toward you while hovered. A solid accent plane is
kept for a toggle that is on (shuffle, repeat, an open panel, a switch).

### Motion

Micro animations answer every touch, at display rate, and stop. The Hub's
durations and its OutCubic curve carry the Hub's parts; the library's chisel
curve stays on library controls and on the music's colours. Reduced motion
from the desktop makes every one land at once.

| Surface | Micro interaction | Duration, curve |
|---|---|---|
| Any row, tile, card, link, tree line | hover wash in, row leans 2 dp; out | 120 in, 240 out, OutCubic |
| Any press | press wash, glyph shrinks to .86 (.85 on window buttons), plaques grow their cut by 4 and sink 1 | 90 |
| Tree, tables, queue, bottom nav, search list, equalizer strip | the selection wash travels, and lands (fading in, widening from .6) in a new region | 240 OutCubic, landing 240 and 320 |
| Playlists folder | chevron turns with overshoot; leaves unfold, sliding in 18 ms apart | 240 OutBack 1.6; 260 and 240 OutCubic |
| Settings outline | unfolds; the current node grows 5 to 7 with overshoot | 260 OutCubic; 240 OutBack 2 |
| Sidebar fold | width 272 to 72, names out over the first half, rail tiles in over the second | 240 OutCubic |
| Page change, provider switch, now playing | fade in and slide 14 dp from the right | 240 fade, 300 slide, OutCubic |
| Page opening | first five groups rise 8 and fade, 40 ms apart | 200 chisel |
| Masthead | the ruler marker follows the scroll; seconds tick only while focused | 120 OutCubic |
| Counts and words | roll in from half a line below; Home's readings count up once | 240 OutCubic; 640 |
| Drawers (tree, panel) | slide over a fading scrim | 240 OutCubic |
| Docked panel, player bar, Home's card | unfold | 260 OutCubic |
| Window close | glyph turns a quarter on hover | 240 OutCubic |
| Discord toggle | wash and accent glyph cross over | 120 OutCubic |
| Play and pause | glyph morphs | 200 chisel |
| Like | heart fills and pops to 1.24 and back | 200; pop 320 OutCubic |
| Track change | new cover revealed behind a 45 degree sweep, title and artists rise 8 | 200 or 320 chisel; 200 OutCubic |
| Volume | an outside change is chased, a drag follows exactly | Chase |
| Equalizer | node hover with guides, a register ring lands on the selected node, a dragged node chases, presets and Undo glide the curve, readouts roll, band columns unfold | 120 and 240, Chase, 240 and 260 OutCubic |
| Lyrics | the sung word fills behind a 45 degree cut and lifts, the line grows into place | clock-driven, 320 chisel |
| Accent and cover tint | the whole window retints with a new cover | 320 chisel |

Nothing moves at rest. While music plays, three things move on their own: the
wave progress (the played part is a travelling zigzag with sharp vertices,
flattening when you pause or drag), the visualizer, and the word being sung.
The masthead's seconds are the only ornament, and only while the window is
focused and motion is not reduced. A paused, unfocused window draws nothing.

### Notices and Undo

Notices are the Hub's placards above the player. A reversible removal offers
Undo: unliking a song (from any heart, the item menu or Ctrl L), removing a
collection from your library, unsaving an album or playlist, unsubscribing
from an artist, dropping a radio favourite, stopping watching a folder,
resetting a setting, and the equalizer's band removal, lower preamp, reset and
preset switch. The same notice repeated replaces the one showing.

Undo is left out where it could not put things back exactly or where
something better already guards the action:

- Liking, saving, subscribing and favouriting: the button takes it back.
- Removing from the queue and clearing it: the daemon cannot put a track
  back at its old place, so an Undo would turn autoplay rows into manual ones
  and lose their order.
- Removing from a playlist: adding takes no position, so the song could only
  come back at the end.
- Deleting a playlist, clearing caches or download history, signing out,
  disconnecting Last.fm: these ask first.
- Changing a setting: the control is its own undo, only a reset offers one.

### Tooltips

The Hub has no tooltips. Ryotunes keeps them because its transport and window
controls are icons only and need names.

## Colour

Colour comes from the desktop. Ryotunes adds only the music's.

Gold is play. The player bar's play and pause always carries it, and a page
may spend one more gold on the action that plays it, such as Play on an album.
A dialog's one consequential action is gold too. Radio, download, navigation
and state never take gold.

The accent is the music's colour, and gold is pointed at it. By default it is
sampled from the playing cover (1.x's rule: a 24 pixel downsample, the most
saturated mid-light bucket wins), kept to a hue with fixed lightness per mode
so it reads on every plane, with its ink re-toned to 4.5:1. The play buttons,
the progress wave, the visualizer, liked hearts, the playing track, the sung
words and the header fields all wear it, so nothing on screen disagrees.
Settings can take it from the provider's brand or the desktop instead. A
black and white cover takes a grey accent, and a missing cover falls back to
the desktop. Album, playlist and artist heroes tint their own field with
their own cover.

Where a cover is the subject, the cover itself, heavily blurred, fills the
plane behind it: the now playing view, Home's now playing card, the album,
playlist and artist heroes, and the mini player. Its big shapes and colours
show, but its tones are pressed toward the plane's own lightness, a little
toward the text and further away from it, so type over it reads as it does on
a bare plane. On the black Lithic ladder the band sits a little lighter so the
cover still shows. It changes with the cover and never moves at rest. Pages
without a cover, the player bar, cards and rows stay plain.

Tint with the cover is off by default, so the window wears the system's
planes. Turned on (it needs the album art accent), the planes, hairlines and
inks are re-hued toward the cover's dominant colour in OKLCH, each keeping its
own lightness so every contrast pair holds, with a chroma that grows with how
colourful the cover is. The desktop's selection colour and the state minerals
never change, and a black and white cover turns the window grey instead of
leaving the desktop's hue on it.

The active provider's brand colour is identity, not state, and lives in the
provider switcher and the small provider glyphs. The brand colours and the
Ryoku orange are the only colour literals in the client, in `look/brand.go`.

## State

| Tone | Ryotunes use |
|---|---|
| Jade | saved, connected, live stream |
| Lapis | downloading, connecting, scanning, loading |
| Amber | paused, waiting, sign-in needed, may clip |
| Jasper | failed, expired, unavailable |
| Idle vein | stopped, not started |

Colour never carries a state alone; keep the word and the flake or notch. A
fact that is not known is left out instead of shown as a dash.

## Type and icons

Geologica for text and Martian Mono for data, both from the library's
embedded faces, so the type is the same on any system. Mono is only for data:
durations, counts, bitrates, versions, ordinals and the sheet's strips. The
page's title or hero name is the one large title in a view.

Icons are the library's Material Symbols Sharp. Play, pause, next, previous,
the liked heart and explicit use the Fill variants, and the current place in
the tree and the bottom nav wears its filled glyph.

## Home readings

Home ends with "Your listening": a datasheet (plays this week, time
listening, top artist, top song) and two engraved charts over the last 14
days, Plays and Listening time, whose figures count up once. The series comes
from the daemon's `listening_days` method, one row per local calendar day,
oldest first, ending today, with zeros for quiet days, up to the 30 days of
play history the daemon keeps. A daemon without it draws no chart, and
nothing is ever invented.

## Playback surfaces

Lyrics are sung word by word: the word being sung fills with the accent behind
a 45 degree cut that follows the clock and lifts a little, the words before it
stay lit, the words after it wait in a quieter ink. Real word timing is used
when the lyrics have it (the Boidu source asks for it first); otherwise the
line's own timing is shared across its words by length.

The visualizer lives in the bottom lane of page headers, along the bottom of
the now playing view and in the Home card's instrument. It has four styles:
Bars (sharp blades), Curve (a line over the spectrum), Oscilloscope (the
waveform, triggered so a steady tone stands still) and Vectorscope (the stereo
field in a diamond, folded into a half diamond in lanes). Quiet bands sink and
loud ones stand out, loud blades are brighter, bars carry peak caps that hold
and fall, and a beat detector on the bass kicks every style taller and
brighter for a moment, so the music is felt, not just shown. They run at up to
60 frames a second only while playing and visible, and stop with reduced
motion, low resource mode or their own settings. Bars and Curve come from
`cava` on the sink Ryotunes plays into, the same analyser the Ryoku shell
uses; the scopes read that sink's stereo samples through `pw-record`, only
while one is on screen.

Scrolling applies every delta at once. Gio cannot tell a wheel from a
trackpad, so the client never guesses a wheel from the size of a delta. Each
delta is converted to pixels and accounted against the real height of every
row it crosses, never an average, so a fast flick over rows of different
heights lands exactly where the wheel says. Animated jumps (Settings sections,
shelf arrows) glide through every row on the way.

Floating or tiled is the app's own choice. A floating main window maps with
its minimum and maximum both at its size, which tiling window managers float
the way they float a dialog, and then becomes resizable; a tiled one maps
with only its minimum. The desktop carries no rule for Ryotunes, so the
setting is the only thing deciding.

## Overlays

Popovers, menus, tips and pickers are placed in window coordinates by the
client's own overlay host, so a popover from the player bar opens above it and
stays inside the window. The library's popover, tooltip and picker measure
inside the slot that opens them, which squeezes them in nested layouts, and
its row list eases trackpad deltas that look like wheel notches; Ryotunes uses
neither.

## Equalizer

The equalizer is an instrument, after the one the listener asked for (Astra's):
a response graph over 20 Hz to 20 kHz with a diamond node per band, drawn the
way the Hub draws its charts (a line over a 45 degree hatch), and a strip of
faders below it. Drag a node for frequency and gain, use the wheel for Q,
double click the graph to add a band (up to 20), right click a node for its
shape (bell, shelves, passes, notch, band pass). The curve is computed with
the same filter maths ffmpeg runs, so what you see is what you hear, and a
claim warns when the curve's peak could clip, with a one-click fix. Presets,
Equalizer APO and AutoEQ import, and export to the clipboard sit in its title
row.

## Owner rulings

These hold through every redesign.

| # | Ruling | How the code keeps it |
|---|---|---|
| R1 | Music-focused interaction, distinct from Ryoport, with deliberate placement and spacing | the three-zone player bar stays the hand rest, covers stay in the tree, the Hub's ornaments are chrome only, gold stays play |
| R2 | Volume is easy to drag precisely | the 24 dp hit band, the pointer grab and the bubble are untouched; equalizer values stay exact under the pointer while the drawing chases |
| R3 | Provider switching keeps the provider icons on top in the provider's colours | the switcher keeps its brand glyphs and plate; its frame is a hairline |
| R4 | No search tab next to a search bar | one field in the top bar, no Search place in the tree or bottom nav; `/` and Ctrl K focus it |
| R5 | A Discord Rich Presence toggle | the top bar toggle stays (on is the selection wash) and leads Settings, Integrations |
| R6 | Compact mode is reopenable and rectangular around the album art | the mini and compact classes keep their layouts |
| R7 | Now playing lyrics stay live | the follow logic and the playback clock are untouched |
| R8 | Horizontal scroll areas show there is more | shelves and the equalizer strip keep the cut-off item and step plaques |
| R9 | Fast scrolling never skips | only `look.Scroller` and `look.HScroller`; jumps glide through every row |
| R10 | Visualizer bars are wavy, Android 17 like, sharp, never rounded | blades and caps stay angular in every design |
| R11 | The blurred cover is clearly visible behind now playing | the backdrop's band sits on a lighter floor on the black ladder |
| R12 | Grey covers stay grey | the tint's grey path is kept and the desktop's selection colour is never re-hued |
