package look

import (
	"gioui.org/widget"
	mdicons "golang.org/x/exp/shiny/materialdesign/icons"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
)

// Icon is the complete Ryotunes Material icon set, decoded once.
var Icon struct {
	Play, Pause, Next, Previous, Shuffle, Repeat, RepeatOne                    *widget.Icon
	Heart, HeartOutline, Download, DownloadDone, Queue, Lyrics, NowPlaying     *widget.Icon
	Mini, Expand, VolumeOff, VolumeMute, VolumeDown, VolumeUp, Tune, Back      *widget.Icon
	Forward, Search, Filter, Close, More, Add, Home, Library, Radio, Downloads *widget.Icon
	Settings, Minimize, Maximize, Restore, Check, Drag, Remove, Folder, Person *widget.Icon
	Album, Playlist, PlaylistAdd, Refresh, Share, OpenInNew, Sparkle, Explicit *widget.Icon
	Equalizer, Sort                                                            *widget.Icon
}

func init() {
	Icon.Play = kit.Icon(mdicons.AVPlayArrow)
	Icon.Pause = kit.Icon(mdicons.AVPause)
	Icon.Next = kit.Icon(mdicons.AVSkipNext)
	Icon.Previous = kit.Icon(mdicons.AVSkipPrevious)
	Icon.Shuffle = kit.Icon(mdicons.AVShuffle)
	Icon.Repeat = kit.Icon(mdicons.AVRepeat)
	Icon.RepeatOne = kit.Icon(mdicons.AVRepeatOne)
	Icon.Heart = kit.Icon(mdicons.ActionFavorite)
	Icon.HeartOutline = kit.Icon(mdicons.ActionFavoriteBorder)
	Icon.Download = kit.Icon(mdicons.FileFileDownload)
	Icon.DownloadDone = kit.Icon(mdicons.ActionDoneAll)
	Icon.Queue = kit.Icon(mdicons.AVQueueMusic)
	Icon.Lyrics = kit.Icon(mdicons.AVLibraryMusic)
	Icon.NowPlaying = kit.Icon(mdicons.NavigationFullscreen)
	Icon.Mini = kit.Icon(mdicons.ActionPictureInPictureAlt)
	Icon.Expand = kit.Icon(mdicons.NavigationFullscreen)
	Icon.VolumeOff = kit.Icon(mdicons.AVVolumeOff)
	Icon.VolumeMute = kit.Icon(mdicons.AVVolumeMute)
	Icon.VolumeDown = kit.Icon(mdicons.AVVolumeDown)
	Icon.VolumeUp = kit.Icon(mdicons.AVVolumeUp)
	Icon.Tune = kit.Icon(mdicons.ImageTune)
	Icon.Back = kit.Icon(mdicons.NavigationArrowBack)
	Icon.Forward = kit.Icon(mdicons.NavigationArrowForward)
	Icon.Search = kit.Icon(mdicons.ActionSearch)
	Icon.Filter = kit.Icon(mdicons.ContentFilterList)
	Icon.Close = kit.Icon(mdicons.NavigationClose)
	Icon.More = kit.Icon(mdicons.NavigationMoreVert)
	Icon.Add = kit.Icon(mdicons.ContentAdd)
	Icon.Home = kit.Icon(mdicons.ActionHome)
	Icon.Library = kit.Icon(mdicons.AVLibraryMusic)
	Icon.Radio = kit.Icon(mdicons.ToggleRadioButtonChecked)
	Icon.Downloads = kit.Icon(mdicons.FileFileDownload)
	Icon.Settings = kit.Icon(mdicons.ActionSettings)
	Icon.Minimize = kit.Icon(mdicons.ContentRemove)
	Icon.Maximize = kit.Icon(mdicons.NavigationFullscreen)
	Icon.Restore = kit.Icon(mdicons.NavigationFullscreenExit)
	Icon.Check = kit.Icon(mdicons.ActionDone)
	Icon.Drag = kit.Icon(mdicons.EditorDragHandle)
	Icon.Remove = kit.Icon(mdicons.ContentRemove)
	Icon.Folder = kit.Icon(mdicons.FileFolder)
	Icon.Person = kit.Icon(mdicons.SocialPerson)
	Icon.Album = kit.Icon(mdicons.AVAlbum)
	Icon.Playlist = kit.Icon(mdicons.AVPlaylistPlay)
	Icon.PlaylistAdd = kit.Icon(mdicons.AVPlaylistAdd)
	Icon.Refresh = kit.Icon(mdicons.NavigationRefresh)
	Icon.Share = kit.Icon(mdicons.ActionOpenInNew)
	Icon.OpenInNew = kit.Icon(mdicons.ActionOpenInNew)
	Icon.Sparkle = kit.Icon(mdicons.ToggleStar)
	Icon.Explicit = kit.Icon(mdicons.AVExplicit)
	Icon.Equalizer = kit.Icon(mdicons.AVEqualizer)
	Icon.Sort = kit.Icon(mdicons.ContentSort)
}
