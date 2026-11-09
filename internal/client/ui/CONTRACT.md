# Client UI contract

Package layout under `internal/client/ui/` and the exported surface each
package owns. Pages and chrome import `env`, `widgets`, `act` and the kit;
the shell imports all of them. Nothing below `ui/` imports `shell`.

## env (exists)

`env.Env`, `env.Page`, `env.Nav`, `env.Route`, `env.MenuTarget`, toasts, panels.

## act: daemon actions every surface reuses

Each function runs the call through `e.Go` (never on the frame goroutine),
toasts failures with the daemon's message, and returns immediately.

```go
func Play(e *env.Env, t model.Track)
func PlayCollection(e *env.Env, items []model.Track, start int, sourceID, sourceName string, shuffle bool, continuation *string)
func AddToQueue(e *env.Env, items []model.Track, from string)
func PlayNext(e *env.Env, items []model.Track, from string)
func StartRadio(e *env.Env, kind, id, name string)
func Rate(e *env.Env, t model.Track, r model.Rating)
func Download(e *env.Env, t model.Track)
func DownloadCollection(e *env.Env, title string, items []model.Track)
func OpenCard(e *env.Env, c model.BrowseItem)          // routes album/artist/playlist/song(plays)
func OpenArtist(e *env.Env, id, name string)
func OpenAlbum(e *env.Env, id, title string)
func AddToPlaylist(e *env.Env, playlistID string, t model.Track)
func RemoveFromPlaylist(e *env.Env, playlistID string, t model.Track)
func CopyLink(e *env.Env, t model.Track)               // share URL to the clipboard via gtx-free clipboard request
func Call[R any](e *env.Env, method string, params any, done func(R, error)) // generic: runs conn.Do in e.Go, done runs on the frame (via invalidate)
```

## widgets: shared composed widgets

```go
// TrackRow is one track tablet: number or art, title, artists (+album), provider badge,
// explicit mark, like, duration, menu; hover shows play; double click plays.
type TrackRow struct { ... }
type TrackRowOpts struct { Index int; ShowArt, ShowAlbum, Playing, Selected bool; Context string }
func (r *TrackRow) Layout(gtx layout.Context, e *env.Env, t model.Track, o TrackRowOpts) layout.Dimensions
func (r *TrackRow) Activated() bool            // double click or Enter this frame

// TrackList is a scrolling list of TrackRows with the moving selection slab and
// keyboard Up/Down/Enter; onPlay(i) is called when a row is activated.
type TrackList struct { ... }
func (l *TrackList) Layout(gtx layout.Context, e *env.Env, items []model.Track, o TrackListOpts) layout.Dimensions
type TrackListOpts struct { ShowArt, ShowAlbum bool; Context string; Header layout.Widget; Footer layout.Widget; OnPlay func(i int); OnEnd func() }

// Card is one browse card (square art, title, subtitle); artists are octagon-cut.
type Card struct { ... }
func (c *Card) Layout(gtx layout.Context, e *env.Env, item model.BrowseItem, size unit.Dp) layout.Dimensions
func (c *Card) Clicked() bool

// Shelf is a titled horizontal row of cards with a "More" plaque when the shelf has a browse id.
type Shelf struct { ... }
func (s *Shelf) Layout(gtx layout.Context, e *env.Env, shelf model.Shelf) layout.Dimensions

// CardGrid lays cards in responsive columns.
type CardGrid struct { ... }
func (g *CardGrid) Layout(gtx layout.Context, e *env.Env, items []model.BrowseItem) layout.Dimensions

// Hero is the mineral-field header of a detail page: 44dp mark or cover, name at T.Name
// (T.Sentence under 640dp), proof line, marks line, act row.
type Hero struct { ... }
type HeroContent struct { Art string; Collage bool; Octagon bool; Kicker, Name, Proof string; Facts []string; Tone lithic.Tone; Acts layout.Widget }
func (h *Hero) Layout(gtx layout.Context, e *env.Env, c HeroContent) layout.Dimensions

// Art draws a cover from the art cache, cut at the given cut; collage urls draw the 2x2 grid.
func Art(gtx layout.Context, e *env.Env, url string, size unit.Dp, cut unit.Dp) layout.Dimensions
// ProviderBadge is the small source mark (YouTube, Spotify, SoundCloud, Tidal, Local, Radio) as a flake + word.
func ProviderBadge(e *env.Env, p model.Provider) layout.Widget
// PageHeader is the plain page title (T.Section) with an optional sentence under it and trailing acts.
func PageHeader(e *env.Env, title, sentence string, trailing layout.Widget) layout.Widget
// Loading, Failed and EmptyState are the three page states, with the spec copy passed in.
func Loading(e *env.Env) layout.Widget
func Failed(e *env.Env, message string, retry *lithic.Button) layout.Widget
func EmptyState(e *env.Env, icon, title, body string, actions ...lithic.EmptyAction) layout.Widget

// ItemMenu builds the menu entries for a target (spec 09 item menus per kind) and runs the picked one.
type ItemMenu struct { ... }
func (m *ItemMenu) Layout(gtx layout.Context, e *env.Env, t env.MenuTarget) layout.Dimensions
func (m *ItemMenu) Done() bool                 // an entry ran or the menu was dismissed

// AddToPlaylistDialog and EditPlaylistDialog are the two playlist dialogs.
type AddToPlaylistDialog struct { Open bool; Track *model.Track }
func (d *AddToPlaylistDialog) Layout(gtx layout.Context, e *env.Env) layout.Dimensions
type EditPlaylistDialog struct { Open bool; Page *model.PlaylistPage }
func (d *EditPlaylistDialog) Layout(gtx layout.Context, e *env.Env) layout.Dimensions

// Duration formats seconds as m:ss or h:mm:ss; Count formats 1.2K / 3.4M.
func Duration(seconds float64) string
func Count(n int) string
```

## pages: one package each, `func New(r env.Route) env.Page`

`pages/home`, `pages/search`, `pages/library`, `pages/artist`, `pages/album`,
`pages/playlist`, `pages/radio`, `pages/downloads`, `pages/settings`,
`pages/diagnostics`, `pages/list` (generic browse grid).

## chrome

```go
type PlayerBar struct { ... }   func (b *PlayerBar) Layout(gtx, e) layout.Dimensions
type QueuePanel struct { ... }  func (p *QueuePanel) Layout(gtx, e) layout.Dimensions
type LyricsPanel struct { ... } func (p *LyricsPanel) Layout(gtx, e) layout.Dimensions
                                func (p *LyricsPanel) LayoutCompact(gtx, e) layout.Dimensions // mini player
type NowPlaying struct { ... }  func (p *NowPlaying) Layout(gtx, e) layout.Dimensions
type ListenPanel struct { ... } func (p *ListenPanel) Layout(gtx, e) layout.Dimensions
type SoundPanel struct { ... }  func (p *SoundPanel) Layout(gtx, e) layout.Dimensions // volume, speed/pitch, fx; opened from the player bar in a popover
type Mini struct { ... }        func (m *Mini) Layout(gtx, e) layout.Dimensions       // the whole "Ryotunes Mini" window
```

## shell

`shell.Run(opts)` owns windows, the event loop, `env.Nav`, the rail, the
title bar, the right panel host, placards, dialogs, menus, the command
palette, keyboard shortcuts, the Escape stack, palette watching and the
`--png`/`--demo` modes. `cmd/ryotunes` parses flags and calls it.

## Layout of the main window

Rail (88/72 at expanded/medium; bottom bar under 760) with Home, Search,
Library, Radio, Downloads, Settings, the brand mark as Home at the top; the
work pane (stone2, top corners cut 16) with the routed page; the right panel
(stone1, 360 wide, cut top-left) when open; the player bar along the bottom
of the window under all panes (stone1, 88 tall, top corners cut 16).
