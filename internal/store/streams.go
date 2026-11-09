package store

import (
	"database/sql"
	"fmt"
	"time"
)

// CachedStream is one provider stream URL and its playback metadata.
type CachedStream struct {
	VideoID    string
	URL        string
	Itag       int
	ExpiresAt  time.Time
	LoudnessDB *float64
	IsVideo    bool
	PingURL    string
	PingClient string
}

// GetStream returns an unexpired cached stream.
func (s *Store) GetStream(videoID string, now time.Time) (*CachedStream, error) {
	var out CachedStream
	var loudness sql.NullFloat64
	var isVideo int
	var pingURL, pingClient sql.NullString
	err := s.db.QueryRow(`SELECT video_id,url,itag,expires_at,loudness_db,is_video,ping_url,ping_client
		FROM stream_url_cache WHERE video_id=? AND expires_at>?`, videoID, now.Unix()).Scan(
		&out.VideoID, &out.URL, &out.Itag, (*unixTime)(&out.ExpiresAt), &loudness,
		&isVideo, &pingURL, &pingClient,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get cached stream: %w", err)
	}
	if loudness.Valid {
		out.LoudnessDB = &loudness.Float64
	}
	out.IsVideo = isVideo != 0
	out.PingURL = pingURL.String
	out.PingClient = pingClient.String
	return &out, nil
}

// PutStream stores a stream and prunes every expired stream in the same transaction.
func (s *Store) PutStream(stream CachedStream) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin stream cache write: %w", err)
	}
	defer tx.Rollback()
	var loudness any
	if stream.LoudnessDB != nil {
		loudness = *stream.LoudnessDB
	}
	if _, err := tx.Exec(`INSERT INTO stream_url_cache
		(video_id,url,itag,expires_at,loudness_db,is_video,ping_url,ping_client)
		VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(video_id) DO UPDATE SET
		url=excluded.url,itag=excluded.itag,expires_at=excluded.expires_at,
		loudness_db=excluded.loudness_db,is_video=excluded.is_video,
		ping_url=excluded.ping_url,ping_client=excluded.ping_client`,
		stream.VideoID, stream.URL, stream.Itag, stream.ExpiresAt.Unix(), loudness,
		boolInt(stream.IsVideo), nullableString(stream.PingURL), nullableString(stream.PingClient)); err != nil {
		return fmt.Errorf("put cached stream: %w", err)
	}
	if _, err := tx.Exec("DELETE FROM stream_url_cache WHERE expires_at<=?", s.now().Unix()); err != nil {
		return fmt.Errorf("prune cached streams: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit stream cache write: %w", err)
	}
	return nil
}

// DeleteStream deletes one cached stream.
func (s *Store) DeleteStream(videoID string) error {
	_, err := s.db.Exec("DELETE FROM stream_url_cache WHERE video_id=?", videoID)
	if err != nil {
		return fmt.Errorf("delete cached stream: %w", err)
	}
	return nil
}

// PruneStreams deletes streams expired at or before now and returns the deleted count.
func (s *Store) PruneStreams(now time.Time) (int64, error) {
	result, err := s.db.Exec("DELETE FROM stream_url_cache WHERE expires_at<=?", now.Unix())
	if err != nil {
		return 0, fmt.Errorf("prune cached streams: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("count pruned streams: %w", err)
	}
	return count, nil
}

// ClearStreams deletes every cached stream.
func (s *Store) ClearStreams() error {
	_, err := s.db.Exec("DELETE FROM stream_url_cache")
	if err != nil {
		return fmt.Errorf("clear cached streams: %w", err)
	}
	return nil
}

type unixTime time.Time

func (t *unixTime) Scan(src any) error {
	var seconds int64
	switch value := src.(type) {
	case int64:
		seconds = value
	case int:
		seconds = int64(value)
	default:
		return fmt.Errorf("invalid unix timestamp %T", src)
	}
	*t = unixTime(time.Unix(seconds, 0))
	return nil
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}
