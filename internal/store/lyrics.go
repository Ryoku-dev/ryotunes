package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ryoku-dev/ryotunes/internal/model"
)

// GetLyrics returns a positive or negative cache hit; a hit with nil lyrics is negative.
func (s *Store) GetLyrics(videoID string, now time.Time, missTTL time.Duration) (*model.Lyrics, bool, error) {
	var raw sql.NullString
	var fetchedAt int64
	err := s.db.QueryRow("SELECT lyrics,fetched_at FROM lyrics_cache WHERE video_id=?", videoID).Scan(&raw, &fetchedAt)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("get cached lyrics: %w", err)
	}
	if !raw.Valid {
		if now.Sub(time.Unix(fetchedAt, 0)) > missTTL {
			if _, err := s.db.Exec("DELETE FROM lyrics_cache WHERE video_id=?", videoID); err != nil {
				return nil, false, fmt.Errorf("expire cached lyrics miss: %w", err)
			}
			return nil, false, nil
		}
		return nil, true, nil
	}
	var lyrics model.Lyrics
	if err := json.Unmarshal([]byte(raw.String), &lyrics); err != nil {
		return nil, false, fmt.Errorf("decode cached lyrics: %w", err)
	}
	return &lyrics, true, nil
}

// PutLyrics stores a positive result or a nil negative verdict.
func (s *Store) PutLyrics(videoID string, lyrics *model.Lyrics, fetchedAt time.Time) error {
	var raw any
	if lyrics != nil {
		encoded, err := json.Marshal(lyrics)
		if err != nil {
			return fmt.Errorf("encode cached lyrics: %w", err)
		}
		raw = string(encoded)
	}
	_, err := s.db.Exec(`INSERT INTO lyrics_cache(video_id,lyrics,fetched_at) VALUES(?,?,?)
		ON CONFLICT(video_id) DO UPDATE SET lyrics=excluded.lyrics,fetched_at=excluded.fetched_at`,
		videoID, raw, fetchedAt.Unix())
	if err != nil {
		return fmt.Errorf("put cached lyrics: %w", err)
	}
	return nil
}

// ClearLyrics deletes every positive and negative lyrics cache row.
func (s *Store) ClearLyrics() error {
	_, err := s.db.Exec("DELETE FROM lyrics_cache")
	if err != nil {
		return fmt.Errorf("clear cached lyrics: %w", err)
	}
	return nil
}
