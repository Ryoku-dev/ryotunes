package look

import (
	"gioui.org/widget"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/symbols"
)

// Icon is the complete Ryotunes icon set, decoded once: Material Symbols Sharp from the
// Lithic library, the Hub's own glyphs. Outline by default; the transport (play, pause,
// skip), a liked heart and the explicit badge wear the filled form, and the *Fill fields
// are the filled destinations the navigation marks as current (see Filled). Provider marks
// stay Ryotunes' own strokes (glyph.go).
var Icon struct {
	Play, Pause, Next, Previous, Shuffle, Repeat, RepeatOne                    *widget.Icon
	Heart, HeartOutline, Download, DownloadDone, Queue, Lyrics, NowPlaying     *widget.Icon
	Mini, Expand, VolumeOff, VolumeMute, VolumeDown, VolumeUp, Tune, Back      *widget.Icon
	Forward, Search, Filter, Close, More, Add, Home, Library, Radio, Downloads *widget.Icon
	Settings, Minimize, Maximize, Restore, Check, Drag, Remove, Folder, Person *widget.Icon
	Album, Playlist, PlaylistAdd, Refresh, Share, OpenInNew, Sparkle, Explicit *widget.Icon
	Equalizer, Sort, ChevronDown, Sidebar                                      *widget.Icon

	// Filled destinations: the current navigation destination's glyph.
	HomeFill, LibraryFill, DownloadsFill, SettingsFill, FolderFill, SidebarFill *widget.Icon
}

// filled maps an outline destination glyph to its filled form.
var filled map[*widget.Icon]*widget.Icon

func init() {
	Icon.Play = kit.Icon(symbols.PlayArrowFill)
	Icon.Pause = kit.Icon(symbols.PauseFill)
	Icon.Next = kit.Icon(symbols.SkipNextFill)
	Icon.Previous = kit.Icon(symbols.SkipPreviousFill)
	Icon.Shuffle = kit.Icon(symbols.Shuffle)
	Icon.Repeat = kit.Icon(symbols.Repeat)
	Icon.RepeatOne = kit.Icon(symbols.RepeatOne)
	Icon.Heart = kit.Icon(symbols.FavoriteFill)
	Icon.HeartOutline = kit.Icon(symbols.Favorite)
	Icon.Download = kit.Icon(symbols.Download)
	Icon.DownloadDone = kit.Icon(symbols.DownloadDone)
	Icon.Queue = kit.Icon(symbols.QueueMusic)
	Icon.Lyrics = kit.Icon(symbols.Lyrics)
	Icon.NowPlaying = kit.Icon(symbols.Fullscreen)
	Icon.Mini = kit.Icon(symbols.PictureInPictureAlt)
	Icon.Expand = kit.Icon(symbols.Fullscreen)
	Icon.VolumeOff = kit.Icon(symbols.VolumeOff)
	Icon.VolumeMute = kit.Icon(symbols.VolumeMute)
	Icon.VolumeDown = kit.Icon(symbols.VolumeDown)
	Icon.VolumeUp = kit.Icon(symbols.VolumeUp)
	Icon.Tune = kit.Icon(symbols.Tune)
	Icon.Back = kit.Icon(symbols.ArrowBack)
	Icon.Forward = kit.Icon(symbols.ArrowForward)
	Icon.Search = kit.Icon(symbols.Search)
	Icon.Filter = kit.Icon(symbols.FilterList)
	Icon.Close = kit.Icon(symbols.Close)
	Icon.More = kit.Icon(symbols.MoreVert)
	Icon.Add = kit.Icon(symbols.Add)
	Icon.Home = kit.Icon(symbols.Home)
	Icon.Library = kit.Icon(symbols.LibraryMusic)
	Icon.Radio = kit.Icon(symbols.Radio)
	Icon.Downloads = kit.Icon(symbols.Download)
	Icon.Settings = kit.Icon(symbols.Settings)
	Icon.Minimize = kit.Icon(symbols.Remove)
	Icon.Maximize = kit.Icon(symbols.Fullscreen)
	Icon.Restore = kit.Icon(symbols.FullscreenExit)
	Icon.Check = kit.Icon(symbols.Check)
	Icon.Drag = kit.Icon(symbols.DragHandle)
	Icon.Remove = kit.Icon(symbols.Remove)
	Icon.Folder = kit.Icon(symbols.Folder)
	Icon.Person = kit.Icon(symbols.Person)
	Icon.Album = kit.Icon(symbols.Album)
	Icon.Playlist = kit.Icon(symbols.PlaylistPlay)
	Icon.PlaylistAdd = kit.Icon(symbols.PlaylistAdd)
	Icon.Refresh = kit.Icon(symbols.Refresh)
	Icon.Share = kit.Icon(symbols.Share)
	Icon.OpenInNew = kit.Icon(symbols.OpenInNew)
	Icon.Sparkle = kit.Icon(symbols.Stars)
	Icon.Explicit = kit.Icon(symbols.ExplicitFill)
	Icon.Equalizer = kit.Icon(symbols.Equalizer)
	Icon.Sort = kit.Icon(symbols.Sort)
	Icon.ChevronDown = kit.Icon(symbols.ExpandMore)
	Icon.Sidebar = kit.Icon(symbols.SideNavigation)

	Icon.HomeFill = kit.Icon(symbols.HomeFill)
	Icon.LibraryFill = kit.Icon(symbols.LibraryMusicFill)
	Icon.DownloadsFill = kit.Icon(symbols.DownloadFill)
	Icon.SettingsFill = kit.Icon(symbols.SettingsFill)
	Icon.FolderFill = kit.Icon(symbols.FolderFill)
	Icon.SidebarFill = kit.Icon(symbols.SideNavigationFill)

	filled = map[*widget.Icon]*widget.Icon{
		Icon.Home:         Icon.HomeFill,
		Icon.Library:      Icon.LibraryFill,
		Icon.Downloads:    Icon.DownloadsFill,
		Icon.Settings:     Icon.SettingsFill,
		Icon.Folder:       Icon.FolderFill,
		Icon.Sidebar:      Icon.SidebarFill,
		Icon.HeartOutline: Icon.Heart,
	}
}

// Filled returns the filled form of a destination glyph, the Hub's mark for the current
// destination, or the glyph itself when it has none (radio, search, the player's own).
// A map lookup: safe to call every frame.
func Filled(icon *widget.Icon) *widget.Icon {
	if fill, ok := filled[icon]; ok {
		return fill
	}
	return icon
}
