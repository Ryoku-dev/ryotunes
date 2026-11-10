# Ryotunes design

Ryotunes draws with the shared Lithic library from the Ryoku repository,
`github.com/ryoku-dev/ryoku/docs/lithic/library/go` (`palette`, `theme`, `anim`,
`kit`, `tokens`). The language itself lives in the Ryoku repo under
`docs/lithic/`; its `layout.md` describes the player window archetype this app
is the reference for. `internal/client/ui/look` holds what is ours: the theme
with the accent and source colours, the Ryoku mark, provider glyphs, the frame
geometry, scrolling, scrubbers, artwork, the visualizer styles, the equalizer's
graph and faders, and the small motions playback needs.
`internal/client/ui/CONTRACT.md` has every measurement.

## Window layout

The window is laid out like a music player, not like a list and detail tool:

1. A top bar on the ground: the Ryoku mark, back and forward, the one search
   field, the Discord toggle, the provider switcher and window controls.
2. The sidebar: Home, Library, Radio and Downloads, then your playlists, with
   Settings at the bottom. On wide windows it folds to an icon rail and back
   (the toggle beside Back, or Ctrl B) and remembers the choice; on medium
   widths it is the rail and the toggle opens the full sidebar as a sheet; on
   compact ones it is a bottom bar.
3. The content pane with the current page. Home opens with a now playing card
   inside its greeting while something is loaded: the cover (click to play or
   pause), the track, the seek bar, the transport and an instrument showing
   the visualizer.
4. The side panel with Queue, Lyrics and Together. It docks while the content
   keeps 520 dp and becomes a sheet when it would not.
5. The player bar across the bottom: what plays on the left, transport over the
   progress in the centre, panels, volume and sound on the right.

The now playing view takes the space between the two bars with large artwork
(click it to play or pause) and lyrics. The mini player is a second window,
"Ryotunes Mini", in the same process and the same state: a landscape strip
with the cover flush along its left edge, falling back to a portrait card when
the window is made tall. The equalizer is a page of its own, reached from the
sound popover and Settings.

Structure changes at the size-class boundaries (760 and 1100 dp, measured
after the app's zoom) instead of squeezing the wide layout. Entering the
compact class closes a docked panel so it never covers the page.

## Selection, action and source

Bone is where you are: the current destination, the selected row, the active
tab and any toggle that is on.

Gold is play. The player bar's play and pause always carries it, and a page
may spend one more gold on the action that plays it, such as Play on an album.
Radio, download, navigation and state never take gold.

The accent is the music's colour, and gold is pointed at it. By default it is
sampled from the playing cover (1.x's rule: a 24 pixel downsample, the most
saturated mid-light bucket wins), kept to a hue with fixed lightness per mode
so it reads on every stone, with its ink re-toned to 4.5:1 on `stone2`. The
play buttons, the progress wave, the visualizer, liked hearts, the playing
track, the sung words and the header fields all wear it, so nothing on screen
disagrees. Settings can take it from the provider's brand or the desktop
palette instead; a grey or missing cover falls back to the desktop palette.
Album, playlist and artist heroes tint their own field with their own cover.

The window can take the cover's colour too, the way Ryoku's palette takes the
wallpaper's. With Tint the window on (the default while the accent follows the
album art), the ground, every stone, the veins and the inks are re-hued toward
the cover's dominant colour in OKLCH, each keeping its own lightness so every
contrast pair holds, with a chroma that grows with how colourful the cover is.
States keep their minerals on any cover, and a black and white cover leaves the
desktop palette alone. A new track retints the whole window together with the
accent.

The active provider's brand colour is identity, not state, and now lives only
in the provider switcher and the small provider glyphs. The brand colours and
the Ryoku orange are the only colour literals in the client, in
`look/brand.go`.

Panes cut their top corners by one length the listener can choose: Square,
Fine, Chisel (the default 16 dp), Deep, a custom value, or Desktop, which
follows the window corners set in Ryoku Hub (including a per-app override for
Ryotunes) and updates live when they change.

## State

| Tone | Ryotunes use |
|---|---|
| Jade | saved, connected, live stream |
| Lapis | downloading, connecting, scanning |
| Amber | paused, waiting, sign-in needed |
| Jasper | failed, expired, unavailable |
| Idle vein | stopped, not started |

Colour never carries a state alone; keep the word and the flake or notch. A
fact that is not known is left out instead of shown as a dash.

## Type

The page's title or hero name is the one large title in a view. Space Mono is
only for data: durations, counts, bitrates and versions.

## Motion

Motion answers a press, a hover, a navigation or a playback change and then
stops: the play button morphs, a liked heart fills, a new cover is revealed
behind a 45 degree cut that sweeps across the old one while the window
retints, a scrubber thickens and shows its handle under the pointer, the
provider plate glides, the panes re-cut when the corner setting changes, the
sidebar folds with its labels fading, the side panel unfolds, the Home card
unfolds when music starts, a lyric line grows into place as it becomes the one
being sung while the last one settles back, and equalizer nodes and curves
tween to a preset. Each uses the library's chisel curve and duration tokens and
is instant under reduced motion.

Nothing moves at rest. While music plays, three things move on their own: the
wave progress (the played part is a travelling zigzag with sharp vertices,
flattening when you pause or drag), the visualizer, and the word being sung.

Lyrics are sung word by word: the word being sung fills with the accent behind
a 45 degree cut that follows the clock and lifts a little, the words before it
stay lit, the words after it wait in a quieter ink. Real word timing is used
when the lyrics have it (the Boidu source asks for it first); otherwise the
line's own timing is shared across its words by length.

The visualizer lives in the bottom lane of page headers, along the bottom of
the now playing view and in the Home card's instrument. It has four styles:
Bars (Lithic blades), Curve (a line over the spectrum), Oscilloscope (the
waveform, triggered so a steady tone stands still) and Vectorscope (the stereo
field in a diamond, folded into a half diamond in lanes). Quiet bands sink and
loud ones stand out, loud blades are brighter, bars carry peak caps that hold
and fall, and a beat detector on the bass kicks every style taller and
brighter for a moment, so the music is felt, not just shown. They run at up to
60 frames a second only while playing and visible, and stop with reduced
motion, low resource mode or their own settings. Bars and Curve come from
`cava` on the sink Ryotunes plays into, the same analyser the Ryoku shell uses;
the scopes read that sink's stereo samples through `pw-record`, only while one
is on screen.

Scrolling applies every delta at once. Gio cannot tell a wheel from a
trackpad, so the client never guesses a wheel from the size of a delta. Each
delta is converted to pixels and accounted against the real height of every
row it crosses, never an average, so a fast flick over rows of different
heights lands exactly where the wheel says.

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

## Palette

Ryotunes has roles, not colour rules of its own. The library resolves the
Ryoku palette (`shell.json` named scheme, then the wallpaper role set, then
the Lithic signature fallback) and watches it. Settings can force dark or
light, or point at a palette JSON file of the same shape. A palette change
snaps to the new roles in both windows.

## Equalizer

The equalizer is an instrument, after the one the listener asked for (Astra's):
a response graph over 20 Hz to 20 kHz with a diamond node per band, and a
strip of faders below it. Drag a node for frequency and gain, use the wheel for
Q, double click the graph to add a band (up to 20), right click a node for its
shape (bell, shelves, passes, notch, band pass). The curve is computed with the
same filter maths ffmpeg runs, so what you see is what you hear, and a claim
warns when the curve's peak could clip, with a one-click fix. Presets,
Equalizer APO and AutoEQ import, and export to the clipboard sit in its title
row.
