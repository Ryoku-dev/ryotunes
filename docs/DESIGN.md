# Ryotunes design

Ryotunes draws with the shared Lithic library from the Ryoku repository,
`github.com/ryoku-dev/ryoku/docs/lithic/library/go` (`palette`, `theme`, `anim`,
`kit`, `tokens`). The language itself lives in the Ryoku repo under
`docs/lithic/`; its `layout.md` describes the player window archetype this app
is the reference for. `internal/client/ui/look` holds what is ours: the theme
with the source colour, the Ryoku mark, provider glyphs, the frame geometry,
scrolling, scrubbers, artwork and the small motions playback needs.
`internal/client/ui/CONTRACT.md` has every measurement.

## Window layout

The window is laid out like a music player, not like a list and detail tool:

1. A top bar on the ground: the Ryoku mark, back and forward, the one search
   field, the Discord toggle, the provider switcher and window controls.
2. The sidebar: Home, Library, Radio and Downloads, then your playlists, with
   Settings at the bottom. It becomes an icon rail on medium widths and a
   bottom bar on compact ones.
3. The content pane with the current page.
4. The side panel with Queue, Lyrics and Together. It docks while the content
   keeps 520 dp and becomes a sheet when it would not.
5. The player bar across the bottom: what plays on the left, transport over the
   progress in the centre, panels, volume and sound on the right.

The now playing view takes the space between the two bars with large artwork
and lyrics. The mini player is a second window, "Ryotunes Mini", in the same
process and the same state.

Structure changes at the size-class boundaries (760 and 1100 dp, measured
after the app's zoom) instead of squeezing the wide layout. Entering the
compact class closes a docked panel so it never covers the page.

## Selection, action and source

Bone is where you are: the current destination, the selected row, the active
tab and any toggle that is on.

Gold is play. The player bar's play and pause always carries it, and a page
may spend one more gold on the action that plays it, such as Play on an album.
Radio, download, navigation and state never take gold.

The active provider's brand colour is identity, not state. It tints the
switcher's active cell, the field behind page titles and the now playing view,
the progress, liked hearts, the playing track and the sung words in lyrics.
Text and marks use the brand re-toned to 4.5:1 on `stone2`. Local files and
radio use lapis. The brand colours and the Ryoku orange are the only colour
literals in the client, in `look/brand.go`.

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
stops: the play button morphs, a liked heart fills, artwork crossfades when
the track changes, a scrubber thickens and shows its handle under the pointer,
the provider plate glides and the source colour retints, the side panel
unfolds, the active lyric line glides into place. Each uses the library's
chisel curve and duration tokens and is instant under reduced motion.

Nothing moves at rest. While playing, only the progress and the elapsed time
ask for frames, and only when the fill would move by a pixel or the second
changes.

Scrolling applies every delta at once. Gio cannot tell a wheel from a
trackpad, so the client never guesses a wheel from the size of a delta.

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
