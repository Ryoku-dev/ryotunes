package model

// Artist is one linked artist credit in a track or card.
type Artist struct {
	Text string  `json:"text"`
	ID   *string `json:"id,omitempty"`
}

// AlbumRef is a compact album identity used where a full album page is unnecessary.
type AlbumRef struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// Rating is the signed-in or local rating of a track.
type Rating string

const (
	// RatingLike marks a liked track.
	RatingLike Rating = "like"
	// RatingDislike marks a disliked track.
	RatingDislike Rating = "dislike"
	// RatingIndifferent removes a rating.
	RatingIndifferent Rating = "indifferent"
)

// Track is the SongItem wire shape shared by providers, queues, playlists, and downloads.
type Track struct {
	VideoID       string   `json:"video_id"`
	Title         string   `json:"title"`
	Artists       string   `json:"artists"`
	ArtistID      *string  `json:"artist_id,omitempty"`
	ArtistRuns    []Artist `json:"artist_runs,omitempty"`
	Album         *string  `json:"album,omitempty"`
	AlbumID       *string  `json:"album_id,omitempty"`
	Duration      *string  `json:"duration,omitempty"`
	PlayCount     *string  `json:"play_count,omitempty"`
	Thumbnail     *string  `json:"thumbnail,omitempty"`
	SetVideoID    *string  `json:"set_video_id,omitempty"`
	AddedBy       *string  `json:"added_by,omitempty"`
	AddedByAvatar *string  `json:"added_by_avatar,omitempty"`
	Rating        *Rating  `json:"rating,omitempty"`
	QueuedBy      *string  `json:"queued_by,omitempty"`
	Queued        bool     `json:"queued"`
	QueuedEnd     bool     `json:"queued_end"`
	QueuedFrom    *string  `json:"queued_from,omitempty"`
	Autoplay      bool     `json:"autoplay"`
	IsVideo       bool     `json:"is_video"`
	IsUpload      bool     `json:"is_upload"`
	Explicit      bool     `json:"explicit"`
	Plays         *string  `json:"plays,omitempty"`
	Likes         *string  `json:"likes,omitempty"`
	Genre         *string  `json:"genre,omitempty"`
}

// BrowseItem is a kind-tagged card on home, search, and library surfaces.
type BrowseItem struct {
	Kind       string   `json:"kind"`
	ID         string   `json:"id"`
	Title      string   `json:"title"`
	Subtitle   *string  `json:"subtitle,omitempty"`
	Thumbnail  *string  `json:"thumbnail,omitempty"`
	Duration   *string  `json:"duration,omitempty"`
	ArtistRuns []Artist `json:"artistRuns,omitempty"`
	PlayCount  *string  `json:"playCount,omitempty"`
	IsVideo    bool     `json:"isVideo"`
	IsUpload   bool     `json:"isUpload"`
	Explicit   bool     `json:"explicit"`
}

// Shelf is one titled row of browse cards.
type Shelf struct {
	Title         string       `json:"title"`
	TitleIsArtist bool         `json:"titleIsArtist"`
	Items         []BrowseItem `json:"items"`
	MoreBrowseID  *string      `json:"moreBrowseId,omitempty"`
	MoreParams    *string      `json:"moreParams,omitempty"`
}

// FeedChip is a home-feed filter chip.
type FeedChip struct {
	Title  string `json:"title"`
	Params string `json:"params"`
}

// Feed is a home feed and its continuation token.
type Feed struct {
	Chips        []FeedChip `json:"chips"`
	Sections     []Shelf    `json:"sections"`
	Continuation *string    `json:"continuation,omitempty"`
}

// SearchKind selects a provider search result family.
type SearchKind string

const (
	// SearchKindAll requests mixed results.
	SearchKindAll SearchKind = "all"
	// SearchKindSongs requests tracks.
	SearchKindSongs SearchKind = "songs"
	// SearchKindAlbums requests albums.
	SearchKindAlbums SearchKind = "albums"
	// SearchKindArtists requests artists.
	SearchKindArtists SearchKind = "artists"
	// SearchKindPlaylists requests community playlists.
	SearchKindPlaylists SearchKind = "playlists"
)

// SearchPage is a page of track search results.
type SearchPage struct {
	Items        []Track `json:"items"`
	Continuation *string `json:"continuation,omitempty"`
}

// SearchAll is the mixed-search wire shape.
type SearchAll struct {
	Top          []BrowseItem `json:"top"`
	Songs        []BrowseItem `json:"songs"`
	Albums       []BrowseItem `json:"albums"`
	Artists      []BrowseItem `json:"artists"`
	Playlists    []BrowseItem `json:"playlists"`
	Continuation *string      `json:"continuation,omitempty"`
}

// CardsPage is a page of album, artist, or playlist cards.
type CardsPage struct {
	Items        []BrowseItem `json:"items"`
	Continuation *string      `json:"continuation,omitempty"`
}

// PlaylistSort is a provider playlist ordering.
type PlaylistSort string

const (
	// PlaylistSortDefault preserves provider order.
	PlaylistSortDefault PlaylistSort = "default"
	// PlaylistSortNewest orders newest additions first.
	PlaylistSortNewest PlaylistSort = "newest"
	// PlaylistSortOldest orders oldest additions first.
	PlaylistSortOldest PlaylistSort = "oldest"
	// PlaylistSortTitle orders by title.
	PlaylistSortTitle PlaylistSort = "title"
	// PlaylistSortArtist orders by artist.
	PlaylistSortArtist PlaylistSort = "artist"
	// PlaylistSortAlbum orders by album.
	PlaylistSortAlbum PlaylistSort = "album"
	// PlaylistSortTop orders by provider popularity.
	PlaylistSortTop PlaylistSort = "top"
)

// PlaylistSortMenu describes available server-side playlist ordering.
type PlaylistSortMenu struct {
	Selected *PlaylistSort `json:"selected"`
	Editable bool          `json:"editable"`
}

// PlaylistPage is the playlist detail wire shape.
type PlaylistPage struct {
	Title         *string           `json:"title"`
	Subtitle      *string           `json:"subtitle"`
	Thumbnail     *string           `json:"thumbnail"`
	Description   *string           `json:"description"`
	Privacy       *string           `json:"privacy"`
	Cover         *string           `json:"cover"`
	Items         []Track           `json:"items"`
	Continuation  *string           `json:"continuation"`
	Owned         bool              `json:"owned"`
	Collaborative bool              `json:"collaborative"`
	SortMenu      *PlaylistSortMenu `json:"sortMenu"`
}

// AlbumPage is the album detail shape, which is wire-compatible with PlaylistPage.
type AlbumPage = PlaylistPage

// PlaylistTracksPage is a continuation page of playlist tracks.
type PlaylistTracksPage struct {
	Items        []Track `json:"items"`
	Continuation *string `json:"continuation"`
}

// ArtistSection is one artist-page card row and its optional continuation target.
type ArtistSection struct {
	Title        string       `json:"title"`
	Items        []BrowseItem `json:"items"`
	MoreBrowseID *string      `json:"moreBrowseId,omitempty"`
	MoreParams   *string      `json:"moreParams,omitempty"`
}

// ArtistPage is an artist header, top tracks, and card sections.
type ArtistPage struct {
	Name             *string         `json:"name,omitempty"`
	Thumbnail        *string         `json:"thumbnail,omitempty"`
	Description      *string         `json:"description,omitempty"`
	Subscribers      *string         `json:"subscribers,omitempty"`
	MonthlyListeners *string         `json:"monthlyListeners,omitempty"`
	ChannelID        string          `json:"channelId,omitempty"`
	Subscribed       bool            `json:"subscribed"`
	RadioPlaylistID  *string         `json:"radioPlaylistId,omitempty"`
	TopSongs         []Track         `json:"topSongs"`
	TopSongsID       *string         `json:"topSongsId,omitempty"`
	Sections         []ArtistSection `json:"sections"`
}

// Library groups provider library collections for daemon-internal provider calls.
type Library struct {
	Liked     []Track      `json:"liked"`
	Playlists []BrowseItem `json:"playlists"`
	Albums    []BrowseItem `json:"albums"`
	Artists   []BrowseItem `json:"artists"`
}
