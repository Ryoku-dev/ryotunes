// Package state owns the Gio client's daemon mirror and pure client-side reducers.
package state

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/ryoku-dev/ryotunes/internal/model"
)

const (
	maxPicks   = 18
	maxPins    = 3
	maxRecent  = 100
	maxArtists = 100
)

// Personal is the hydrated client-owned personalization document.
type Personal struct {
	Picks          []Pick                `json:"picks"`
	Saved          []model.BrowseItem    `json:"saved"`
	Pins           []string              `json:"pins"`
	Recent         map[string]RecentItem `json:"recent"`
	Artists        map[string]ArtistStat `json:"artists"`
	DismissedSeeds []string              `json:"dismissedSeeds"`
	Home           HomeArrangement       `json:"home"`
}

// Pick is a BrowseItem shortcut with client usage metadata.
type Pick struct {
	model.BrowseItem
	ArtistID   *string `json:"artistId,omitempty"`
	LastUsedAt int64   `json:"lastUsedAt"`
	Manual     *bool   `json:"manual,omitempty"`
}

// RecentItem is a BrowseItem stamped with the latest use time.
type RecentItem struct {
	model.BrowseItem
	ArtistID *string `json:"artistId,omitempty"`
	At       int64   `json:"at"`
}

// ArtistStat is one local artist-affinity counter.
type ArtistStat struct {
	Name         string `json:"name"`
	Count        int    `json:"count"`
	LastPlayedAt int64  `json:"lastPlayedAt,omitempty"`
}

// HomeArrangement is the persisted Home shelf order and visibility state.
type HomeArrangement struct {
	Order  []string `json:"order"`
	Hidden []string `json:"hidden"`
	Seen   []string `json:"seen"`
}

// EmptyPersonal returns the canonical empty personalization document.
func EmptyPersonal() Personal {
	return Personal{
		Picks:          []Pick{},
		Saved:          []model.BrowseItem{},
		Pins:           []string{},
		Recent:         make(map[string]RecentItem),
		Artists:        make(map[string]ArtistStat),
		DismissedSeeds: []string{},
		Home:           HomeArrangement{Order: []string{}, Hidden: []string{}, Seen: []string{}},
	}
}

// HydratePersonal tolerantly converts an opaque daemon document into the canonical shape.
func HydratePersonal(raw json.RawMessage) Personal {
	out := EmptyPersonal()
	if len(raw) == 0 || string(raw) == "null" {
		return out
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) != nil || object == nil {
		return out
	}
	for _, rawPick := range rawElements(object["picks"]) {
		var pick Pick
		if json.Unmarshal(rawPick, &pick) != nil || pick.ID == "" || (pick.Manual != nil && !*pick.Manual) {
			continue
		}
		pick.ID = normalizePersonalID(pick.ID)
		out.Picks = append(out.Picks, pick)
	}
	for _, rawItem := range rawElements(object["saved"]) {
		var item model.BrowseItem
		if json.Unmarshal(rawItem, &item) != nil || item.ID == "" || item.Kind == "" {
			continue
		}
		item.ID = normalizePersonalID(item.ID)
		out.Saved = append(out.Saved, item)
	}
	for _, id := range rawStrings(object["pins"]) {
		if id == "" {
			continue
		}
		out.Pins = append(out.Pins, normalizePersonalID(id))
		if len(out.Pins) == maxPins {
			break
		}
	}
	var recent map[string]json.RawMessage
	if json.Unmarshal(object["recent"], &recent) == nil {
		for id, rawItem := range recent {
			var item RecentItem
			if id != "" && json.Unmarshal(rawItem, &item) == nil && item.ID != "" {
				out.Recent[id] = item
			}
		}
	}
	var artists map[string]json.RawMessage
	if json.Unmarshal(object["artists"], &artists) == nil {
		for id, rawArtist := range artists {
			var artist ArtistStat
			if id != "" && json.Unmarshal(rawArtist, &artist) == nil && artist.Count > 0 {
				out.Artists[id] = artist
			}
		}
	}
	for _, id := range rawStrings(object["dismissedSeeds"]) {
		if id != "" {
			out.DismissedSeeds = append(out.DismissedSeeds, normalizePersonalID(id))
		}
	}
	var home map[string]json.RawMessage
	if json.Unmarshal(object["home"], &home) == nil {
		out.Home.Order = rawStrings(home["order"])
		out.Home.Hidden = rawStrings(home["hidden"])
		out.Home.Seen = rawStrings(home["seen"])
		if len(out.Home.Order) != 0 && !contains(out.Home.Order, "@familiar") {
			at := len(out.Home.Order)
			for i, key := range out.Home.Order {
				if key == "@recent" {
					at = i + 1
					break
				}
			}
			out.Home.Order = append(out.Home.Order, "")
			copy(out.Home.Order[at+1:], out.Home.Order[at:])
			out.Home.Order[at] = "@familiar"
		}
	}
	trimPersonal(&out)
	return out
}

// MarshalPersonal returns the renderer-owned JSON document stored by the daemon.
func MarshalPersonal(personal Personal) (model.Personal, error) {
	data, err := json.Marshal(personal)
	return model.Personal(data), err
}

// AddPick appends a manual shortcut or refreshes an existing shortcut in place.
func AddPick(personal *Personal, item Pick, nowMS int64) bool {
	for i := range personal.Picks {
		if personal.Picks[i].ID == item.ID {
			personal.Picks[i].LastUsedAt = nowMS
			return false
		}
	}
	item.LastUsedAt = nowMS
	personal.Picks = append(personal.Picks, item)
	for len(personal.Picks) > maxPicks {
		oldest := 0
		for i := 1; i < len(personal.Picks); i++ {
			if personal.Picks[i].LastUsedAt < personal.Picks[oldest].LastUsedAt {
				oldest = i
			}
		}
		personal.Picks = append(personal.Picks[:oldest], personal.Picks[oldest+1:]...)
	}
	return true
}

// SeedPick adds an automatic shortcut without displacing a manual shortcut.
func SeedPick(personal *Personal, item Pick, nowMS int64) bool {
	if len(personal.Picks) >= maxPicks || contains(personal.DismissedSeeds, item.ID) {
		return false
	}
	for _, pick := range personal.Picks {
		if pick.ID == item.ID {
			return false
		}
	}
	item.LastUsedAt = nowMS
	personal.Picks = append(personal.Picks, item)
	return true
}

// RemovePick removes a shortcut and prevents automatic reseeding.
func RemovePick(personal *Personal, id string) {
	kept := personal.Picks[:0]
	for _, pick := range personal.Picks {
		if pick.ID != id {
			kept = append(kept, pick)
		}
	}
	personal.Picks = kept
	if !contains(personal.DismissedSeeds, id) {
		personal.DismissedSeeds = append(personal.DismissedSeeds, id)
	}
}

// TouchPick refreshes a shortcut's usage stamp without reordering it.
func TouchPick(personal *Personal, id string, nowMS int64) bool {
	for i := range personal.Picks {
		if personal.Picks[i].ID == id {
			personal.Picks[i].LastUsedAt = nowMS
			return true
		}
	}
	return false
}

// TogglePin toggles a sidebar pin and returns pinned, unpinned, or full.
func TogglePin(personal *Personal, id string) string {
	for i, pin := range personal.Pins {
		if pin == id {
			personal.Pins = append(personal.Pins[:i], personal.Pins[i+1:]...)
			return "unpinned"
		}
	}
	if len(personal.Pins) >= maxPins {
		return "full"
	}
	personal.Pins = append(personal.Pins, id)
	return "pinned"
}

// NoteRecent records or refreshes a song, album, playlist, or artist recent.
func NoteRecent(personal *Personal, item RecentItem, nowMS int64) {
	if personal.Recent == nil {
		personal.Recent = make(map[string]RecentItem)
	}
	item.At = nowMS
	personal.Recent[item.ID] = item
	trimPersonal(personal)
}

// RecentItems returns recents newest first, capped by limit when positive.
func RecentItems(personal Personal, limit int) []RecentItem {
	items := make([]RecentItem, 0, len(personal.Recent))
	for _, item := range personal.Recent {
		items = append(items, item)
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].At > items[j].At })
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return items
}

// ContinueListening returns recent items not already present in shortcuts.
func ContinueListening(personal Personal, limit int) []RecentItem {
	if limit <= 0 {
		limit = 9
	}
	picked := make(map[string]struct{}, len(personal.Picks))
	for _, pick := range personal.Picks {
		picked[pick.ID] = struct{}{}
	}
	out := make([]RecentItem, 0, limit)
	for _, item := range RecentItems(personal, maxRecent) {
		if _, exists := picked[item.ID]; exists {
			continue
		}
		out = append(out, item)
		if len(out) == limit {
			break
		}
	}
	return out
}

// NoteArtist increments an artist counter and records its latest play time.
func NoteArtist(personal *Personal, key, name string, nowMS int64) {
	if key == "" {
		return
	}
	if personal.Artists == nil {
		personal.Artists = make(map[string]ArtistStat)
	}
	artist := personal.Artists[key]
	artist.Count++
	if name != "" {
		artist.Name = name
	}
	if nowMS != 0 {
		artist.LastPlayedAt = nowMS
	}
	personal.Artists[key] = artist
	trimPersonal(personal)
}

// RecordNowPlaying records only a genuine new non-radio now-playing event.
func RecordNowPlaying(personal *Personal, now *model.PlaybackTrack, lastID *string, nowMS int64) bool {
	if now == nil || now.VideoID == "" || model.ProviderOf(now.VideoID) == model.ProviderRadio {
		*lastID = ""
		return false
	}
	if *lastID == now.VideoID {
		return false
	}
	*lastID = now.VideoID
	item := RecentItem{BrowseItem: model.BrowseItem{
		Kind: "song", ID: now.VideoID, Title: now.Title, Subtitle: stringPointer(now.Artists),
		Thumbnail: now.Thumbnail, ArtistRuns: now.ArtistRuns,
	}, ArtistID: now.ArtistID}
	NoteRecent(personal, item, nowMS)
	TouchPick(personal, now.VideoID, nowMS)
	if now.Artists != "" {
		key := now.Artists
		if now.ArtistID != nil {
			key = *now.ArtistID
		}
		NoteArtist(personal, key, FirstArtist(now.Artists), nowMS)
	}
	return true
}

// ForgetYouTubeRecents removes only account-private YouTube recents.
func ForgetYouTubeRecents(personal *Personal) int {
	removed := 0
	for id, item := range personal.Recent {
		if strings.HasPrefix(id, "spotify:") {
			continue
		}
		private := id == "LM" || id == "VLLM"
		if item.Kind == "playlist" && item.Subtitle != nil && strings.HasPrefix(*item.Subtitle, "Your") {
			private = true
		}
		if private {
			delete(personal.Recent, id)
			removed++
		}
	}
	return removed
}

// ForgetSpotifyRecents removes only provider-qualified Spotify recents.
func ForgetSpotifyRecents(personal *Personal) int {
	removed := 0
	for id := range personal.Recent {
		if strings.HasPrefix(id, "spotify:") {
			delete(personal.Recent, id)
			removed++
		}
	}
	return removed
}

// FirstArtist returns the lead credit from a joined artist string.
func FirstArtist(artists string) string {
	lower := strings.ToLower(artists)
	cut := len(artists)
	for _, separator := range []string{",", "&", "•", " feat. ", " feat ", " ft. ", " ft "} {
		if at := strings.Index(lower, separator); at >= 0 && at < cut {
			cut = at
		}
	}
	return strings.TrimSpace(artists[:cut])
}

func trimPersonal(personal *Personal) {
	for len(personal.Picks) > maxPicks {
		oldest := 0
		for i := 1; i < len(personal.Picks); i++ {
			if personal.Picks[i].LastUsedAt < personal.Picks[oldest].LastUsedAt {
				oldest = i
			}
		}
		personal.Picks = append(personal.Picks[:oldest], personal.Picks[oldest+1:]...)
	}
	if len(personal.Recent) > maxRecent {
		items := RecentItems(*personal, 0)
		for _, item := range items[maxRecent:] {
			delete(personal.Recent, item.ID)
		}
	}
	if len(personal.Artists) > maxArtists {
		keys := make([]string, 0, len(personal.Artists))
		for key := range personal.Artists {
			keys = append(keys, key)
		}
		sort.SliceStable(keys, func(i, j int) bool {
			return personal.Artists[keys[i]].Count > personal.Artists[keys[j]].Count
		})
		for _, key := range keys[maxArtists:] {
			delete(personal.Artists, key)
		}
	}
}

func normalizePersonalID(id string) string {
	if strings.HasSuffix(id, "_ON_REPEAT") {
		return "RYOTUNES_ON_REPEAT"
	}
	return id
}

func rawElements(raw json.RawMessage) []json.RawMessage {
	var values []json.RawMessage
	if json.Unmarshal(raw, &values) != nil {
		return []json.RawMessage{}
	}
	return values
}

func rawStrings(raw json.RawMessage) []string {
	var values []json.RawMessage
	if json.Unmarshal(raw, &values) != nil {
		return []string{}
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		var text string
		if json.Unmarshal(value, &text) == nil && text != "" {
			out = append(out, text)
		}
	}
	return out
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func stringPointer(value string) *string {
	if value == "" {
		return nil
	}
	copy := value
	return &copy
}
