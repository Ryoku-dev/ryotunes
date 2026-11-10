package model

// LyricsWord is one word-level karaoke timing.
type LyricsWord struct {
	Text    string `json:"text"`
	StartMS uint64 `json:"start_ms"`
	EndMS   uint64 `json:"end_ms"`
}

// LyricsLine is one plain or timed lyric line.
type LyricsLine struct {
	TimeMS      *uint64      `json:"time_ms,omitempty"`
	EndTimeMS   *uint64      `json:"end_time_ms,omitempty"`
	Text        string       `json:"text"`
	Words       []LyricsWord `json:"words,omitempty"`
	Translation *string      `json:"translation,omitempty"`
	Romanized   *string      `json:"romanized,omitempty"`
}

// Lyrics is a resolved lyric document.
type Lyrics struct {
	Source       string       `json:"source"`
	Synced       bool         `json:"synced"`
	Instrumental bool         `json:"instrumental"`
	Lines        []LyricsLine `json:"lines"`
}

// PlaybackPing identifies a provider watch-history request.
type PlaybackPing struct {
	URL    string
	Client string
}

// DownloadFormat selects publication encoding.
type DownloadFormat string

const (
	// DownloadOriginal keeps the selected source stream.
	DownloadOriginal DownloadFormat = "original"
	// DownloadMP3 transcodes to MP3.
	DownloadMP3 DownloadFormat = "mp3"
	// DownloadOpus transcodes to Opus.
	DownloadOpus DownloadFormat = "opus"
	// DownloadFLAC decodes into FLAC.
	DownloadFLAC DownloadFormat = "flac"
)

// DownloadStatus identifies a download lifecycle state.
type DownloadStatus string

const (
	// DownloadStatusQueued is waiting for a worker.
	DownloadStatusQueued DownloadStatus = "queued"
	// DownloadStatusDownloading is actively running.
	DownloadStatusDownloading DownloadStatus = "downloading"
	// DownloadStatusCompleted was published successfully.
	DownloadStatusCompleted DownloadStatus = "completed"
	// DownloadStatusFailed may be retried.
	DownloadStatusFailed DownloadStatus = "failed"
	// DownloadStatusCancelled may be retried.
	DownloadStatusCancelled DownloadStatus = "cancelled"
)

// DownloadSettings is the user-editable download policy.
type DownloadSettings struct {
	Path             string         `json:"path"`
	Format           DownloadFormat `json:"format"`
	Workers          int            `json:"workers"`
	OrganizeByArtist bool           `json:"organizeByArtist"`
	EmbedMetadata    bool           `json:"embedMetadata"`
}

// DownloadEntry is one queued, active, or historical download.
type DownloadEntry struct {
	ID             string           `json:"id"`
	VideoID        string           `json:"videoId"`
	Title          string           `json:"title"`
	Artists        string           `json:"artists"`
	Thumbnail      string           `json:"thumbnail"`
	Status         DownloadStatus   `json:"status"`
	Progress       int              `json:"progress"`
	FilePath       *string          `json:"filePath"`
	Error          *string          `json:"error"`
	Source         string           `json:"source"`
	Collection     *string          `json:"collection"`
	CollectionKind *string          `json:"collectionKind"`
	CreatedAt      int64            `json:"createdAt"`
	FinishedAt     *int64           `json:"finishedAt"`
	Settings       DownloadSettings `json:"settings"`
	Warnings       []string         `json:"warnings"`
}

// DownloadCollectionEntry is one track admitted by a collection enqueue.
type DownloadCollectionEntry struct {
	VideoID        string  `json:"videoId"`
	Title          string  `json:"title"`
	Artists        string  `json:"artists"`
	Thumbnail      string  `json:"thumbnail"`
	Collection     *string `json:"collection,omitempty"`
	CollectionKind *string `json:"collectionKind,omitempty"`
}

// DownloadCollectionResult summarizes a collection enqueue.
type DownloadCollectionResult struct {
	Added             int      `json:"added"`
	AlreadyDownloaded int      `json:"alreadyDownloaded"`
	Skipped           int      `json:"skipped"`
	AlreadyPaths      []string `json:"alreadyPaths"`
}

// DownloadSnapshot is the full downloads event and request result.
type DownloadSnapshot struct {
	Jobs        []DownloadEntry `json:"jobs"`
	ActiveCount int             `json:"activeCount"`
	QueuedCount int             `json:"queuedCount"`
}
