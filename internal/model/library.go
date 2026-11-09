package model

import "encoding/json"

// LocalLibrary is the complete on-device scan result.
type LocalLibrary struct {
	Folders []string     `json:"folders"`
	Albums  []BrowseItem `json:"albums"`
	Artists []BrowseItem `json:"artists"`
	Songs   []Track      `json:"songs"`
	Removed []string     `json:"removed"`
}

// DevicePlaylist is a playlist stored only on this device.
type DevicePlaylist struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	CreatedAt  int64  `json:"createdAt"`
	UpdatedAt  int64  `json:"updatedAt"`
	TrackCount int64  `json:"trackCount"`
}

// PlaylistIndex maps a track ID to every playlist containing it.
type PlaylistIndex map[string][]string

// PlayCounts maps a track ID to its trailing-window play count.
type PlayCounts map[string]int

// ListeningPeriod selects a listening-statistics window.
type ListeningPeriod string

const (
	// ListeningDay is the trailing 24-hour window.
	ListeningDay ListeningPeriod = "day"
	// ListeningWeek is the trailing seven-day window.
	ListeningWeek ListeningPeriod = "week"
	// ListeningMonth is the trailing 30-day window.
	ListeningMonth ListeningPeriod = "month"
)

// ArtistPlayCount is one listening-statistics artist row.
type ArtistPlayCount struct {
	Name  string `json:"name"`
	Plays int    `json:"plays"`
}

// TrackPlayCount is one listening-statistics track row.
type TrackPlayCount struct {
	Title   string `json:"title"`
	Artists string `json:"artists"`
	Plays   int    `json:"plays"`
}

// ListeningStats is the aggregate listening history for one period.
type ListeningStats struct {
	Period               ListeningPeriod   `json:"period"`
	Plays                int               `json:"plays"`
	KnownDurationSeconds int64             `json:"knownDurationSeconds"`
	TopArtists           []ArtistPlayCount `json:"topArtists"`
	TopTracks            []TrackPlayCount  `json:"topTracks"`
}

// RadioStation is a normalized Radio Browser station.
type RadioStation struct {
	StationUUID string `json:"stationUuid"`
	Name        string `json:"name"`
	StreamURL   string `json:"streamUrl"`
	URL         string `json:"url"`
	Homepage    string `json:"homepage"`
	Favicon     string `json:"favicon"`
	Country     string `json:"country"`
	CountryCode string `json:"countryCode"`
	Tags        string `json:"tags"`
	Codec       string `json:"codec"`
	Bitrate     int64  `json:"bitrate"`
	Votes       int64  `json:"votes"`
	ClickCount  int64  `json:"clickcount"`
}

// Account is the renderer-safe YouTube account snapshot.
type Account struct {
	SignedIn          bool    `json:"signedIn"`
	Name              *string `json:"name"`
	Handle            *string `json:"handle"`
	Email             *string `json:"email"`
	Thumbnail         *string `json:"thumbnail"`
	ChannelID         *string `json:"channelId"`
	CanSwitch         bool    `json:"canSwitch"`
	SelectionRequired bool    `json:"selectionRequired,omitempty"`
}

// MarshalJSON preserves the legacy compact signed-out and selection-required account shapes.
func (a Account) MarshalJSON() ([]byte, error) {
	if a.SelectionRequired {
		return json.Marshal(struct {
			SignedIn          bool `json:"signedIn"`
			SelectionRequired bool `json:"selectionRequired"`
			CanSwitch         bool `json:"canSwitch"`
		}{a.SignedIn, true, a.CanSwitch})
	}
	if a.Name == nil && a.Handle == nil && a.Email == nil && a.Thumbnail == nil &&
		a.ChannelID == nil && !a.CanSwitch {
		return json.Marshal(struct {
			SignedIn bool `json:"signedIn"`
		}{a.SignedIn})
	}
	type account Account
	return json.Marshal(account(a))
}

// AccountIdentity is a selectable renderer-safe YouTube identity.
type AccountIdentity struct {
	SelectionKey string  `json:"selectionKey"`
	Name         string  `json:"name"`
	Handle       *string `json:"handle"`
	Email        *string `json:"email"`
	Thumbnail    *string `json:"thumbnail"`
	ChannelID    *string `json:"channelId"`
	Selected     bool    `json:"selected"`
}

// AccountIdentities is the account picker result.
type AccountIdentities []AccountIdentity

// PlaylistTransfer is the portable version-one playlist-file shape.
type PlaylistTransfer struct {
	Version int     `json:"version"`
	Title   string  `json:"title"`
	Items   []Track `json:"items"`
}

// LocalChanged lists local entities removed since the preceding scan.
type LocalChanged struct {
	Removed []string `json:"removed"`
}

// LogEntry is one diagnostics row.
type LogEntry struct {
	Timestamp string `json:"ts"`
	Level     string `json:"level"`
	Source    string `json:"source"`
	Target    string `json:"target"`
	Message   string `json:"message"`
}

// LogsResult is a filtered diagnostics result.
type LogsResult struct {
	Entries   []LogEntry `json:"entries"`
	Total     int        `json:"total"`
	Errors    int        `json:"errors"`
	Warnings  int        `json:"warnings"`
	Path      string     `json:"path"`
	FileBytes int64      `json:"fileBytes"`
}
