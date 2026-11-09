package youtube

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/ryoku-dev/ryotunes/internal/model"
)

var durationPattern = regexp.MustCompile(`^(?:\d+:)?\d{1,2}:\d{2}$`)

func walkMaps(value any, visit func(map[string]any)) {
	switch value := value.(type) {
	case map[string]any:
		visit(value)
		for _, child := range value {
			walkMaps(child, visit)
		}
	case []any:
		for _, child := range value {
			walkMaps(child, visit)
		}
	}
}

func findString(value any, keys ...string) string {
	for _, key := range keys {
		var result string
		walkMaps(value, func(m map[string]any) {
			if result != "" {
				return
			}
			if s, ok := m[key].(string); ok && s != "" {
				result = s
			}
		})
		if result != "" {
			return result
		}
	}
	return ""
}
func findPrefixedString(value any, key, prefix string) string {
	var result string
	walkMaps(value, func(m map[string]any) {
		if result == "" {
			if candidate, ok := m[key].(string); ok && strings.HasPrefix(candidate, prefix) {
				result = candidate
			}
		}
	})
	return result
}

func findTextForKey(value any, key string) string {
	var result string
	walkMaps(value, func(m map[string]any) {
		if result == "" {
			result = deepText(m[key])
		}
	})
	return result
}

func firstString(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := m[key].(string); ok && value != "" {
			return value
		}
	}
	return ""
}

func deepText(value any) string {
	if value == nil {
		return ""
	}
	if s, ok := value.(string); ok {
		return s
	}
	if m, ok := value.(map[string]any); ok {
		if s, ok := m["simpleText"].(string); ok {
			return s
		}
		if runs, ok := m["runs"].([]any); ok {
			var b strings.Builder
			for _, raw := range runs {
				if run, ok := raw.(map[string]any); ok {
					b.WriteString(firstString(run, "text"))
				}
			}
			return b.String()
		}
	}
	return ""
}

func textRuns(value any) []map[string]any {
	m, _ := value.(map[string]any)
	raw, _ := m["runs"].([]any)
	result := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		if run, ok := item.(map[string]any); ok {
			result = append(result, run)
		}
	}
	return result
}

func deepThumbnail(value any) string {
	var result string
	walkMaps(value, func(m map[string]any) {
		if raw, ok := m["thumbnails"].([]any); ok {
			for _, item := range raw {
				if thumb, ok := item.(map[string]any); ok {
					if url := firstString(thumb, "url"); url != "" {
						result = url
					}
				}
			}
		}
	})
	return result
}

func findThumbnail(value any) string { return deepThumbnail(value) }

func setStringPtr(dst **string, value string) {
	if value != "" {
		*dst = &value
	}
}

func endpointIDs(value any) (videoID, browseID, params string) {
	walkMaps(value, func(m map[string]any) {
		if videoID == "" {
			videoID = firstString(m, "videoId")
		}
		if browseID == "" {
			browseID = firstString(m, "browseId")
		}
		if params == "" {
			params = firstString(m, "params")
		}
	})
	return
}

func continuation(value any) *string {
	token := findString(value, "continuation", "token")
	if token == "" {
		return nil
	}
	return &token
}

func parseTracks(root any) []model.Track {
	seen := make(map[string]bool)
	var result []model.Track
	walkMaps(root, func(m map[string]any) {
		for _, key := range []string{"musicResponsiveListItemRenderer", "playlistPanelVideoRenderer", "musicPlaylistShelfRenderer"} {
			renderer, ok := m[key].(map[string]any)
			if !ok {
				continue
			}
			track, ok := parseTrack(renderer)
			if ok && !seen[track.VideoID+"\x00"+valueOrEmpty(track.SetVideoID)] {
				seen[track.VideoID+"\x00"+valueOrEmpty(track.SetVideoID)] = true
				result = append(result, track)
			}
		}
	})
	return result
}

func parseTrack(renderer map[string]any) (model.Track, bool) {
	videoID, _, _ := endpointIDs(renderer)
	if videoID == "" {
		videoID = firstString(renderer, "videoId")
	}
	if videoID == "" {
		return model.Track{}, false
	}
	track := model.Track{VideoID: videoID}
	if columns, ok := renderer["flexColumns"].([]any); ok {
		for i, raw := range columns {
			column, _ := raw.(map[string]any)
			text := findFirstText(column)
			if i == 0 {
				track.Title = text
			}
			for _, run := range findRuns(column) {
				label := firstString(run, "text")
				_, browseID, _ := endpointIDs(run)
				switch {
				case strings.HasPrefix(browseID, "UC"):
					id := browseID
					track.ArtistRuns = append(track.ArtistRuns, model.Artist{Text: label, ID: &id})
				case strings.HasPrefix(browseID, "MPRE"), strings.HasPrefix(browseID, "OLAK"):
					if label != "" {
						setStringPtr(&track.Album, label)
						setStringPtr(&track.AlbumID, browseID)
					}
				case durationPattern.MatchString(label):
					setStringPtr(&track.Duration, label)
				}
			}
		}
	}
	if track.Title == "" {
		track.Title = deepText(renderer["title"])
	}
	if track.Title == "" {
		track.Title = findFirstText(renderer)
	}
	if len(track.ArtistRuns) != 0 {
		names := make([]string, 0, len(track.ArtistRuns))
		for _, artist := range track.ArtistRuns {
			names = append(names, artist.Text)
		}
		track.Artists = strings.Join(names, ", ")
		track.ArtistID = track.ArtistRuns[0].ID
	}
	if track.Artists == "" {
		track.Artists = findByLabel(renderer, "ARTIST")
	}
	setStringPtr(&track.Thumbnail, deepThumbnail(renderer))
	setStringPtr(&track.SetVideoID, findString(renderer, "playlistSetVideoId", "setVideoId"))
	setStringPtr(&track.AddedBy, findString(renderer, "name"))
	if duration := findDuration(renderer); duration != "" {
		setStringPtr(&track.Duration, duration)
	}
	musicVideoType := strings.ToUpper(findString(renderer, "musicVideoType"))
	track.IsVideo = findBool(renderer, "isVideo") || musicVideoType != "" && !strings.Contains(musicVideoType, "ATV")
	track.IsUpload = findBool(renderer, "isUpload") || strings.Contains(musicVideoType, "UGC") || strings.Contains(musicVideoType, "UPLOAD")
	track.Explicit = hasBadge(renderer, "EXPLICIT")
	if rating := parseRating(findString(renderer, "likeStatus")); rating != "" {
		track.Rating = &rating
	}
	return track, true
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func findFirstText(value any) string {
	var result string
	walkMaps(value, func(m map[string]any) {
		if result == "" {
			result = deepText(m)
		}
	})
	return result
}

func findRuns(value any) []map[string]any {
	var result []map[string]any
	walkMaps(value, func(m map[string]any) {
		if raw, ok := m["runs"].([]any); ok {
			for _, item := range raw {
				if run, ok := item.(map[string]any); ok {
					result = append(result, run)
				}
			}
		}
	})
	return result
}

func findByLabel(value any, label string) string {
	var result string
	walkMaps(value, func(m map[string]any) {
		if result != "" || firstString(m, "musicPageType") != label {
			return
		}
		result = deepText(m)
	})
	return result
}

func findDuration(value any) string {
	for _, run := range findRuns(value) {
		if text := firstString(run, "text"); durationPattern.MatchString(text) {
			return text
		}
	}
	return ""
}

func findBool(value any, key string) bool {
	result := false
	walkMaps(value, func(m map[string]any) {
		if b, ok := m[key].(bool); ok && b {
			result = true
		}
	})
	return result
}

func hasBadge(value any, marker string) bool {
	found := false
	walkMaps(value, func(m map[string]any) {
		for _, key := range []string{"label", "iconType"} {
			if text, ok := m[key].(string); ok && strings.Contains(strings.ToUpper(text), marker) {
				found = true
			}
		}
	})
	return found
}

func parseRating(value string) model.Rating {
	switch strings.ToUpper(value) {
	case "LIKE", "LIKED":
		return model.RatingLike
	case "DISLIKE", "DISLIKED":
		return model.RatingDislike
	case "INDIFFERENT":
		return model.RatingIndifferent
	default:
		return ""
	}
}

func parseBrowseItems(root any) []model.BrowseItem {
	seen := make(map[string]bool)
	var result []model.BrowseItem
	walkMaps(root, func(m map[string]any) {
		for _, key := range []string{"musicTwoRowItemRenderer", "musicResponsiveListItemRenderer", "musicNavigationButtonRenderer"} {
			renderer, ok := m[key].(map[string]any)
			if !ok {
				continue
			}
			item, ok := parseBrowseItem(renderer)
			if ok && !seen[item.Kind+"\x00"+item.ID] {
				seen[item.Kind+"\x00"+item.ID] = true
				result = append(result, item)
			}
		}
	})
	return result
}

func parseSearchRows(root any) []model.BrowseItem {
	seen := make(map[string]bool)
	var result []model.BrowseItem
	walkMaps(root, func(m map[string]any) {
		section, ok := m["itemSectionRenderer"].(map[string]any)
		if !ok {
			return
		}
		for _, item := range parseBrowseItems(section["contents"]) {
			key := item.Kind + "\x00" + item.ID
			if !seen[key] {
				seen[key] = true
				result = append(result, item)
			}
		}
	})
	return result
}

func parseTopItems(root any) []model.BrowseItem {
	seen := make(map[string]bool)
	var result []model.BrowseItem
	walkMaps(root, func(m map[string]any) {
		card, ok := m["musicCardShelfRenderer"].(map[string]any)
		if !ok {
			return
		}
		if item, ok := parseBrowseItem(card); ok && !item.IsVideo {
			key := item.Kind + "\x00" + item.ID
			if !seen[key] {
				seen[key] = true
				result = append(result, item)
			}
		}
		for _, item := range parseBrowseItems(card["contents"]) {
			key := item.Kind + "\x00" + item.ID
			if !item.IsVideo && !seen[key] {
				seen[key] = true
				result = append(result, item)
			}
		}
	})
	return result
}

func itemNavigation(renderer map[string]any) map[string]any {
	for _, key := range []string{"navigationEndpoint", "onTap", "clickCommand"} {
		if endpoint, ok := renderer[key].(map[string]any); ok {
			return endpoint
		}
	}
	if title, ok := renderer["title"].(map[string]any); ok {
		for _, run := range textRuns(title) {
			if endpoint, ok := run["navigationEndpoint"].(map[string]any); ok {
				return endpoint
			}
		}
	}
	return nil
}

func itemTarget(renderer map[string]any) (videoID, browseID string) {
	if data, ok := renderer["playlistItemData"].(map[string]any); ok {
		videoID = firstString(data, "videoId")
	}
	if videoID == "" {
		videoID = firstString(renderer, "videoId")
	}
	endpoint := itemNavigation(renderer)
	if videoID == "" && endpoint != nil {
		if watch, ok := endpoint["watchEndpoint"].(map[string]any); ok {
			videoID = firstString(watch, "videoId")
		}
	}
	if endpoint != nil {
		if browse, ok := endpoint["browseEndpoint"].(map[string]any); ok {
			browseID = firstString(browse, "browseId")
		}
	}
	if videoID == "" {
		videoID = findString(renderer["overlay"], "videoId")
	}
	return videoID, browseID
}

func itemTitleAndSubtitle(renderer map[string]any) (string, string) {
	if columns, ok := renderer["flexColumns"].([]any); ok {
		title := ""
		subtitle := ""
		if len(columns) > 0 {
			title = findFirstText(columns[0])
		}
		if len(columns) > 1 {
			subtitle = findFirstText(columns[1])
		}
		return title, subtitle
	}
	return deepText(renderer["title"]), deepText(renderer["subtitle"])
}

func parseBrowseItem(renderer map[string]any) (model.BrowseItem, bool) {
	videoID, browseID := itemTarget(renderer)
	id := browseID
	kind := browseKind(browseID, videoID, renderer)
	if videoID != "" && (kind == "song" || id == "") {
		id = videoID
	}
	if id == "" {
		return model.BrowseItem{}, false
	}
	title, subtitle := itemTitleAndSubtitle(renderer)
	item := model.BrowseItem{Kind: kind, ID: id, Title: title}
	if item.Title == "" {
		item.Title = findFirstText(renderer)
	}
	setStringPtr(&item.Subtitle, subtitle)
	setStringPtr(&item.Thumbnail, deepThumbnail(renderer))
	setStringPtr(&item.Duration, findDuration(renderer))
	endpoint := itemNavigation(renderer)
	musicVideoType := strings.ToUpper(findString(endpoint, "musicVideoType"))
	item.IsVideo = findBool(renderer, "isVideo") || musicVideoType != "" && !strings.Contains(musicVideoType, "ATV")
	item.IsUpload = findBool(renderer, "isUpload") || strings.Contains(musicVideoType, "UGC") || strings.Contains(musicVideoType, "UPLOAD")
	item.Explicit = hasBadge(renderer, "EXPLICIT")
	for _, run := range findRuns(renderer) {
		_, artistID, _ := endpointIDs(run)
		if strings.HasPrefix(artistID, "UC") {
			id := artistID
			item.ArtistRuns = append(item.ArtistRuns, model.Artist{Text: firstString(run, "text"), ID: &id})
		}
	}
	return item, item.Title != ""
}

func browseKind(browseID, videoID string, value any) string {
	switch {
	case videoID != "":
		return "song"
	case strings.HasPrefix(browseID, "UC"):
		return "artist"
	case strings.HasPrefix(browseID, "MPRE"), strings.HasPrefix(browseID, "OLAK"):
		return "album"
	case strings.HasPrefix(browseID, "VL"), strings.HasPrefix(browseID, "PL"), strings.HasPrefix(browseID, "RD"):
		return "playlist"
	}
	pageType := strings.ToUpper(findString(value, "pageType", "musicPageType"))
	switch {
	case strings.Contains(pageType, "ALBUM"):
		return "album"
	case strings.Contains(pageType, "ARTIST"):
		return "artist"
	case strings.Contains(pageType, "PLAYLIST"):
		return "playlist"
	}
	if videoID != "" {
		return "song"
	}
	return "playlist"
}

func parseShelves(root any) []model.Shelf {
	var result []model.Shelf
	walkMaps(root, func(m map[string]any) {
		for _, key := range []string{"musicCarouselShelfRenderer", "musicShelfRenderer"} {
			renderer, ok := m[key].(map[string]any)
			if !ok {
				continue
			}
			title := findTextForKey(renderer["header"], "title")
			if title == "" {
				title = findFirstText(renderer["title"])
			}
			items := parseBrowseItems(renderer["contents"])
			if title == "" || len(items) == 0 {
				continue
			}
			shelf := model.Shelf{Title: title, Items: items}
			moreID, params := "", ""
			for _, endpointKey := range []string{"moreContentButton", "bottomEndpoint", "bottomButton"} {
				_, candidateID, candidateParams := endpointIDs(renderer[endpointKey])
				if moreID == "" {
					moreID = candidateID
				}
				if params == "" {
					params = candidateParams
				}
			}
			setStringPtr(&shelf.MoreBrowseID, moreID)
			setStringPtr(&shelf.MoreParams, params)
			shelf.TitleIsArtist = strings.HasPrefix(moreID, "UC")
			result = append(result, shelf)
		}
	})
	return result
}

func parseChips(root any) []model.FeedChip {
	seen := make(map[string]bool)
	var chips []model.FeedChip
	walkMaps(root, func(m map[string]any) {
		renderer, ok := m["chipCloudChipRenderer"].(map[string]any)
		if !ok {
			return
		}
		title := deepText(renderer["text"])
		_, _, params := endpointIDs(renderer)
		if title != "" && params != "" && !seen[params] {
			seen[params] = true
			chips = append(chips, model.FeedChip{Title: title, Params: params})
		}
	})
	return chips
}

func parsePlaylistPage(root any) model.PlaylistPage {
	page := model.PlaylistPage{Items: parseTracks(root), Continuation: continuation(root)}
	setStringPtr(&page.Title, findHeaderText(root, "title"))
	setStringPtr(&page.Subtitle, findHeaderText(root, "subtitle"))
	setStringPtr(&page.Description, findHeaderText(root, "description"))
	setStringPtr(&page.Thumbnail, findThumbnail(root))
	page.Cover = page.Thumbnail
	privacy := strings.ToUpper(findString(root, "privacy"))
	if privacy != "" {
		page.Privacy = &privacy
	}
	page.Owned = findBool(root, "isEditable") || findBool(root, "owned")
	page.Collaborative = findBool(root, "isCollaborative")
	selected := model.PlaylistSortDefault
	if value := findString(root, "selectedSortOption"); value != "" {
		selected = parsePlaylistSort(value)
	}
	page.SortMenu = &model.PlaylistSortMenu{Selected: &selected, Editable: page.Owned}
	return page
}

func findHeaderText(root any, key string) string {
	var result string
	walkMaps(root, func(m map[string]any) {
		if result != "" {
			return
		}
		for _, rendererKey := range []string{"musicDetailHeaderRenderer", "musicEditablePlaylistDetailHeaderRenderer", "musicImmersiveHeaderRenderer", "musicVisualHeaderRenderer"} {
			if renderer, ok := m[rendererKey].(map[string]any); ok {
				result = deepText(renderer[key])
			}
		}
	})
	return result
}

func parsePlaylistSort(value string) model.PlaylistSort {
	value = strings.ToLower(value)
	for _, sort := range []model.PlaylistSort{model.PlaylistSortNewest, model.PlaylistSortOldest, model.PlaylistSortTitle, model.PlaylistSortArtist, model.PlaylistSortAlbum, model.PlaylistSortTop} {
		if strings.Contains(value, string(sort)) {
			return sort
		}
	}
	return model.PlaylistSortDefault
}

func parseArtistPage(root any) model.ArtistPage {
	page := model.ArtistPage{Sections: nil}
	setStringPtr(&page.Name, findHeaderText(root, "title"))
	setStringPtr(&page.Thumbnail, findThumbnail(root))
	setStringPtr(&page.Description, findHeaderText(root, "description"))
	setStringPtr(&page.Subscribers, findTextForKey(root, "subscriberCountText"))
	setStringPtr(&page.MonthlyListeners, findTextForKey(root, "monthlyAudience"))
	page.ChannelID = findString(root, "channelId", "externalChannelId")
	page.Subscribed = findBool(root, "subscribed")
	page.TopSongs = parseTracks(root)
	if id := findPrefixedString(root, "playlistId", "RD"); id != "" {
		page.RadioPlaylistID = &id
	}
	for _, shelf := range parseShelves(root) {
		items := make([]model.BrowseItem, 0, len(shelf.Items))
		for _, item := range shelf.Items {
			if !item.IsVideo {
				items = append(items, item)
			}
		}
		if len(items) != 0 {
			page.Sections = append(page.Sections, model.ArtistSection{Title: shelf.Title, Items: items, MoreBrowseID: shelf.MoreBrowseID, MoreParams: shelf.MoreParams})
		}
	}
	return page
}

func parseSuggestions(root any) []string {
	seen := make(map[string]bool)
	var result []string
	walkMaps(root, func(m map[string]any) {
		for _, key := range []string{"query", "suggestion"} {
			if value := firstString(m, key); value != "" && !seen[value] {
				seen[value] = true
				result = append(result, value)
			}
		}
	})
	return result
}

func parseTimedLyrics(root any) []model.LyricsLine {
	var lines []model.LyricsLine
	walkMaps(root, func(m map[string]any) {
		text := firstString(m, "text", "lyricLine")
		if text == "" {
			return
		}
		var ms uint64
		switch value := m["timeMs"].(type) {
		case float64:
			ms = uint64(value)
		case string:
			ms, _ = strconv.ParseUint(value, 10, 64)
		}
		if ms != 0 || m["timeMs"] != nil {
			value := ms
			lines = append(lines, model.LyricsLine{TimeMS: &value, Text: text})
		}
	})
	return lines
}

func parsePlainLyrics(root any) []model.LyricsLine {
	var text string
	walkMaps(root, func(m map[string]any) {
		if text == "" {
			if renderer, ok := m["musicDescriptionShelfRenderer"].(map[string]any); ok {
				text = deepText(renderer["description"])
			}
		}
	})
	if text == "" {
		return nil
	}
	parts := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	lines := make([]model.LyricsLine, 0, len(parts))
	for _, part := range parts {
		lines = append(lines, model.LyricsLine{Text: part})
	}
	return lines
}
