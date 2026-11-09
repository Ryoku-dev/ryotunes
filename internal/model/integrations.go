package model

// SpotifyStatus is the Spotify account and credential snapshot.
type SpotifyStatus struct {
	SignedIn bool    `json:"signedIn"`
	Stored   bool    `json:"stored"`
	Name     *string `json:"name"`
	Premium  *bool   `json:"premium"`
}

// SoundCloudStatus is the SoundCloud account snapshot.
type SoundCloudStatus struct {
	SignedIn bool    `json:"signedIn"`
	Name     *string `json:"name"`
}

// TidalStatus is the TIDAL account and configured-flow snapshot.
type TidalStatus struct {
	SignedIn       bool    `json:"signedIn"`
	Name           *string `json:"name"`
	Quality        *string `json:"quality"`
	HasCredentials bool    `json:"hasCredentials"`
}

// SpotifyAuthEvent is a state-tagged Spotify authorization event.
type SpotifyAuthEvent struct {
	State   string  `json:"state"`
	URL     *string `json:"url,omitempty"`
	Name    *string `json:"name,omitempty"`
	Message *string `json:"message,omitempty"`
}

// SoundCloudAuthEvent is a state-tagged SoundCloud authorization event.
type SoundCloudAuthEvent struct {
	State   string  `json:"state"`
	Name    *string `json:"name,omitempty"`
	Message *string `json:"message,omitempty"`
}

// TidalAuthEvent is a state-tagged TIDAL authorization event.
type TidalAuthEvent struct {
	State   string  `json:"state"`
	URL     *string `json:"url,omitempty"`
	Code    *string `json:"code,omitempty"`
	Name    *string `json:"name,omitempty"`
	Quality *string `json:"quality,omitempty"`
	Message *string `json:"message,omitempty"`
}

// DiscordStatus is the Discord RPC runtime snapshot.
type DiscordStatus struct {
	Enabled bool   `json:"enabled"`
	Status  string `json:"status"`
}

// LastfmStatus is the stable Last.fm status request result.
type LastfmStatus struct {
	Configured bool    `json:"configured"`
	Connected  bool    `json:"connected"`
	Username   *string `json:"username"`
}

// LastfmState is the Last.fm event payload, including a transient error.
type LastfmState struct {
	Configured bool    `json:"configured"`
	Connected  bool    `json:"connected"`
	Username   *string `json:"username"`
	Error      *string `json:"error"`
}

// LTTrack is a track relayed through a Listen Together room.
type LTTrack struct {
	ID         string  `json:"id"`
	Title      string  `json:"title"`
	Artist     string  `json:"artist"`
	Thumbnail  *string `json:"thumbnail"`
	DurationMS int64   `json:"duration_ms"`
	QueuedBy   *string `json:"queued_by,omitempty"`
}

// LTUser is one Listen Together room participant.
type LTUser struct {
	UserID      string `json:"user_id"`
	Username    string `json:"username"`
	IsHost      bool   `json:"is_host"`
	IsConnected bool   `json:"is_connected"`
}

// LTRoom is the relay's authoritative room state.
type LTRoom struct {
	RoomCode     string    `json:"room_code"`
	HostID       string    `json:"host_id"`
	Users        []LTUser  `json:"users"`
	CurrentTrack *LTTrack  `json:"current_track"`
	IsPlaying    bool      `json:"is_playing"`
	PositionMS   int64     `json:"position_ms"`
	LastUpdateMS int64     `json:"last_update_ms"`
	Volume       float64   `json:"volume"`
	Queue        []LTTrack `json:"queue"`
}

// LTPendingJoin is a host-visible pending room join.
type LTPendingJoin struct {
	UserID   string `json:"userId"`
	Username string `json:"username"`
}

// LTSuggestion is a guest track suggestion awaiting host action.
type LTSuggestion struct {
	ID           string  `json:"id"`
	FromUserID   string  `json:"from_user_id"`
	FromUsername string  `json:"from_username"`
	Track        LTTrack `json:"track"`
}

// LTState is the complete client-facing Listen Together snapshot.
type LTState struct {
	Status       string          `json:"status"`
	Role         string          `json:"role"`
	Requesting   bool            `json:"requesting"`
	RoomCode     *string         `json:"roomCode"`
	MyID         *string         `json:"myId"`
	ServerURL    string          `json:"serverUrl"`
	Users        []LTUser        `json:"users"`
	CurrentTrack *LTTrack        `json:"currentTrack"`
	Queue        []LTTrack       `json:"queue"`
	PendingJoins []LTPendingJoin `json:"pendingJoins"`
	Suggestions  []LTSuggestion  `json:"suggestions"`
}

// LTNotice is a bare Listen Together notice string.
type LTNotice string

// UpdateInfo is the self-update availability result.
type UpdateInfo struct {
	CurrentVersion    string  `json:"currentVersion"`
	LatestVersion     *string `json:"latestVersion"`
	Available         bool    `json:"available"`
	ReleaseURL        string  `json:"releaseUrl"`
	Notes             string  `json:"notes"`
	CanInstall        bool    `json:"canInstall"`
	UnsupportedReason *string `json:"unsupportedReason"`
	RestartPending    bool    `json:"restartPending"`
}

// UpdateResult is a successfully installed update result.
type UpdateResult struct {
	Version         string `json:"version"`
	RestartRequired bool   `json:"restartRequired"`
}
