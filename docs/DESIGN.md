# Ryotunes design

Ryotunes applies the [Ryoku Lithic specification](https://github.com/ryoku-dev/ryoku/tree/main/docs/lithic) through `internal/client/lithic`. The specification defines the roles, cuts, type, components and motion. This document describes how those rules map to the music player.

## Window layout

The main window has four persistent regions:

1. The rail holds Home, Search, Library, Radio, Downloads and Settings.
2. The work pane holds the current route and its list or detail content.
3. The right panel holds Queue or Lyrics and may be closed when more work-pane space is needed.
4. The player bar spans the bottom and keeps transport, progress, volume and the current track available across routes.

The mini player is a second window in the same process. It reads the same client store and does not create a second playback state.

At narrow widths, regions change structure rather than shrinking the wide layout until it clips. Window and pane corners use the shared cut primitives. Content such as album art may remain square.

## Selection and action

Bone means the current place. Use the selection role for:

- the current route in the rail;
- the playing row in a track list;
- the current Queue or Lyrics panel choice;
- the selected filter, tab or picker value.

Gold is reserved for Play and Save. It does not mark navigation, playback state, decoration or passive information. Other actions use the line, quiet or danger variants from the Lithic kit.

## Mineral fields

The selected album, artist or playlist begins with a mineral field. The field contains artwork, the name, a short proof such as artist, year or track count, and the relevant actions. It is a state-bearing header, not a decorative gradient or artwork wash.

Downloads, authentication and playback states use the Lithic state minerals with a shape and a plain label:

| Tone | Ryotunes use |
|---|---|
| Jade | ready, connected, playing or downloaded |
| Lapis | connecting, buffering, scanning or downloading |
| Amber | paused, waiting, sign-in required or attention needed |
| Jasper | failed, expired, unavailable or destructive |
| Idle vein | stopped, disconnected or not started |

Colour never carries a state by itself. Keep the word and the notch, flake or field when space becomes tight.

## Type

The selected album, artist, playlist or route may own the one large title in a view. Use the shared working sizes everywhere else.

Space Mono is for literal data:

- track and elapsed durations;
- track, result and download counts;
- provider, playlist and room identifiers;
- bitrate, sample rate and other measured values.

Titles, labels and prose use the shared Lithic text roles rather than the data face.

## Motion and idle work

Motion answers a press, selection, navigation or state change and then stops. Use the shared chisel curve and duration tokens. Do not add ambient movement, stagger list rows, spin idle icons or request frames for a hidden surface.

Reduced motion makes the same state transition immediate. An idle window with a still pointer and no live work requests no frames. Queue position and playback time come from daemon events; the client does not run a permanent transport loop when hidden.

## Palette

Ryotunes has roles, not standalone colour rules. The palette resolver follows the Ryoku sources in this order:

1. `$XDG_CONFIG_HOME/ryoku/shell.json` `themePalette` for a named scheme;
2. `$XDG_CACHE_HOME/ryoku/colors.json` for the live wallpaper role set;
3. `$XDG_CONFIG_HOME/ryoku/theme.json` for whether wallpaper colours are active;
4. the Lithic signature fallback when no Ryoku palette is available.

Settings can pin dark or light neutrals while continuing to use palette accents. An optional Material role-set JSON file can override the palette roles. That palette file replaces the 1.x skin system; it does not change layout, type, cuts or motion.

Palette changes are watched and applied to both windows. A palette change under the same state snaps to the new roles. Mineral field motion is for a state change, not a wallpaper change.
