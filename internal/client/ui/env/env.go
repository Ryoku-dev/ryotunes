// Package env is what every page and chrome surface of the client receives
// to draw and act: the theme, the daemon connection, the shared state, the
// router, artwork and the toast line. Pages live in their own packages and
// import only this one, so the shell can import them without a cycle.
package env

import (
	"context"
	"image"
	"time"

	"gioui.org/layout"

	"github.com/ryoku-dev/ryotunes/internal/client/art"
	"github.com/ryoku-dev/ryotunes/internal/client/conn"
	"github.com/ryoku-dev/ryotunes/internal/client/spectrum"
	"github.com/ryoku-dev/ryotunes/internal/client/state"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/model"
)

// RouteKind names a destination the router can show in the work area.
type RouteKind string

// The routes. Detail routes carry an id; Search carries the query.
const (
	RouteHome        RouteKind = "home"
	RouteSearch      RouteKind = "search"
	RouteLibrary     RouteKind = "library"
	RouteRadio       RouteKind = "radio"
	RouteDownloads   RouteKind = "downloads"
	RouteSettings    RouteKind = "settings"
	RouteDiagnostics RouteKind = "diagnostics"
	RouteArtist      RouteKind = "artist"
	RouteAlbum       RouteKind = "album"
	RoutePlaylist    RouteKind = "playlist"
	RouteList        RouteKind = "list" // a generic browse grid: id + params
	RouteEqualizer   RouteKind = "equalizer"
)

// Route is one entry of the navigation stack.
type Route struct {
	Kind   RouteKind
	ID     string
	Params string
	Query  string
	Title  string
}

// ToastKind selects the placard tone.
type ToastKind string

// Toast kinds, matching the daemon's notice kinds.
const (
	ToastInfo    ToastKind = "info"
	ToastSuccess ToastKind = "success"
	ToastError   ToastKind = "error"
)

// Panel is what the right panel shows.
type Panel string

// Right panel modes; PanelNone hides it.
const (
	PanelNone   Panel = ""
	PanelQueue  Panel = "queue"
	PanelLyrics Panel = "lyrics"
	PanelListen Panel = "listen-together"
)

// Nav is the router surface pages use. The shell implements it.
type Nav interface {
	Push(r Route)
	Replace(r Route)
	Back() bool
	Forward() bool
	CanBack() bool
	CanForward() bool
	Current() Route
	Stack() []Route
	OpenPanel(p Panel)
	TogglePanel(p Panel)
	Panel() Panel
	NowPlaying() bool
	ToggleNowPlaying()
	ToggleMini()
	FocusSearch()
	OpenSound(anchor image.Rectangle)
	// SidebarCollapsed reports the wide-window sidebar folded to its rail.
	SidebarCollapsed() bool
	// ToggleSidebar folds or unfolds the sidebar and remembers the choice.
	ToggleSidebar()
}

// Fx are the motion-sensitive playback effects enabled for this frame.
type Fx struct {
	Visualizer bool
	Wave       bool
	// Style is how every visualizer draws.
	Style state.VisualizerStyle
}

// Env is passed to every Layout call. Pages keep their own widget state
// and read everything else from here.
type Env struct {
	Th       *look.Theme
	Overlays *look.Overlays
	Store    *state.Store
	Conn     *conn.Client
	Nav      Nav
	Art      *art.Cache
	Spectrum *spectrum.Feed
	// Scope is the stereo sample feed behind the oscilloscope and vectorscope.
	Scope *spectrum.Scope
	Fx    Fx
	Now   time.Time
	// Toast shows a placard; the shell owns the stack.
	Toast func(message string, kind ToastKind)
	// Go runs fn off the UI goroutine and invalidates the window when it
	// returns; pages use it for daemon calls so a frame never blocks.
	Go func(fn func(ctx context.Context))
	// Confirm asks a yes/no question in a dialog and calls back with the
	// answer; typeToConfirm, when non-empty, must be typed to enable the act.
	Confirm func(title, body, action, typeToConfirm string, onConfirm func())
	// Menu opens an item menu for a track or card at the pointer position.
	Menu func(target MenuTarget)
}

// MenuTarget is what an item menu acts on.
type MenuTarget struct {
	Track    *model.Track
	Card     *model.BrowseItem
	Playlist *model.PlaylistPage
	// Context tells the menu where the item sits (a playlist id when the
	// row can be removed from it, "queue" when it can be removed from the
	// queue, or "add-to-playlist" for the dedicated full-window dialog).
	Context    string
	QueueIndex int
}

// Page is one work-area surface. The router keeps instances alive while
// they are on the stack so scroll and selection survive navigation.
type Page interface {
	Layout(gtx layout.Context, e *Env) layout.Dimensions
	// Title is what the title bar and route history show.
	Title() string
}

// Refresher is a page that can reload itself (Ctrl R).
type Refresher interface {
	Refresh(e *Env)
}

// Filterer is a page whose own collection filter can take keyboard focus.
type Filterer interface {
	FocusFilter(gtx layout.Context)
}

// Leaver is a page Escape leaves, a tool opened from somewhere else.
type Leaver interface {
	Leave(e *Env)
}
