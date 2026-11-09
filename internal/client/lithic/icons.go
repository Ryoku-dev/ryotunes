package lithic

import (
	"gioui.org/widget"
	mdicons "golang.org/x/exp/shiny/materialdesign/icons"
)

var iconData = map[string][]byte{
	"play":            mdicons.AVPlayArrow,
	"pause":           mdicons.AVPause,
	"skip_next":       mdicons.AVSkipNext,
	"skip_previous":   mdicons.AVSkipPrevious,
	"shuffle":         mdicons.AVShuffle,
	"repeat":          mdicons.AVRepeat,
	"repeat_one":      mdicons.AVRepeatOne,
	"volume_up":       mdicons.AVVolumeUp,
	"volume_down":     mdicons.AVVolumeDown,
	"volume_off":      mdicons.AVVolumeOff,
	"queue":           mdicons.AVQueueMusic,
	"lyrics":          mdicons.AVLibraryMusic,
	"favorite":        mdicons.ActionFavorite,
	"favorite_border": mdicons.ActionFavoriteBorder,
	"download":        mdicons.FileFileDownload,
	"downloading":     mdicons.ActionAutorenew,
	"check":           mdicons.ActionDone,
	"close":           mdicons.NavigationClose,
	"search":          mdicons.ActionSearch,
	"home":            mdicons.ActionHome,
	"library":         mdicons.AVLibraryMusic,
	"radio":           mdicons.ToggleRadioButtonChecked,
	"settings":        mdicons.ActionSettings,
	"more_vert":       mdicons.NavigationMoreVert,
	"more_horiz":      mdicons.NavigationMoreHoriz,
	"add":             mdicons.ContentAdd,
	"remove":          mdicons.ContentRemove,
	"delete":          mdicons.ActionDelete,
	"edit":            mdicons.ContentCreate,
	"open_in_new":     mdicons.ActionOpenInNew,
	"folder":          mdicons.FileFolder,
	"refresh":         mdicons.NavigationRefresh,
	"chevron_left":    mdicons.NavigationChevronLeft,
	"chevron_right":   mdicons.NavigationChevronRight,
	"expand_more":     mdicons.NavigationExpandMore,
	"expand_less":     mdicons.NavigationExpandLess,
	"arrow_back":      mdicons.NavigationArrowBack,
	"drag_handle":     mdicons.EditorDragHandle,
	"pin":             mdicons.MapsPinDrop,
	"cast":            mdicons.HardwareCast,
	"person":          mdicons.SocialPerson,
	"group":           mdicons.SocialGroup,
	"info":            mdicons.ActionInfo,
	"warning":         mdicons.ActionReportProblem,
	"error":           mdicons.ActionReportProblem,
	"lock":            mdicons.ActionLock,
	"visibility":      mdicons.ActionVisibility,
	"visibility_off":  mdicons.ActionVisibilityOff,
	"album":           mdicons.AVAlbum,
	"mic":             mdicons.AVMic,
	"headphones":      mdicons.HardwareHeadset,
	"speaker":         mdicons.HardwareSpeaker,
	"equalizer":       mdicons.AVEqualizer,
	"timer":           mdicons.ActionQueryBuilder,
	"history":         mdicons.ActionHistory,
	"star":            mdicons.ToggleStar,
	"link":            mdicons.ActionOpenInNew,
	"content_copy":    mdicons.ContentContentCopy,
	"share":           mdicons.ActionOpenInNew,
	"sort":            mdicons.ContentSort,
	"filter_list":     mdicons.ContentFilterList,
	"tune":            mdicons.ActionSettings,
	"logout":          mdicons.ActionExitToApp,
	"login":           mdicons.ActionInput,
	"cloud_download":  mdicons.FileCloudDownload,
	"cloud_off":       mdicons.FileCloudOff,
	"playlist_add":    mdicons.AVPlaylistAdd,
	"playlist_play":   mdicons.AVPlaylistPlay,
	"explicit":        mdicons.AVExplicit,
}

var iconSet = func() map[string]*widget.Icon {
	set := make(map[string]*widget.Icon, len(iconData))
	for name, data := range iconData {
		icon, err := widget.NewIcon(data)
		if err != nil {
			panic(err)
		}
		set[name] = icon
	}
	return set
}()

// Icon returns the embedded Material icon for name, or nil for an unknown name.
func Icon(name string) *widget.Icon { return iconSet[name] }
