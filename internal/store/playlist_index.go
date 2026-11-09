package store

import (
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/ryoku-dev/ryotunes/internal/model"
)

const playlistIndexSyncedAtKey = "playlist_index_synced_at"

// SetPlaylistTracks replaces one YouTube playlist's membership rows.
func (s *Store) SetPlaylistTracks(playlistID string, videoIDs []string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin playlist-index replacement: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec("DELETE FROM playlist_track WHERE playlist_id=?", playlistID); err != nil {
		return fmt.Errorf("clear playlist-index rows: %w", err)
	}
	seen := make(map[string]struct{}, len(videoIDs))
	for _, videoID := range videoIDs {
		if videoID == "" {
			continue
		}
		if _, ok := seen[videoID]; ok {
			continue
		}
		seen[videoID] = struct{}{}
		if _, err := tx.Exec("INSERT INTO playlist_track(playlist_id,video_id) VALUES(?,?)", playlistID, videoID); err != nil {
			return fmt.Errorf("insert playlist-index row: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit playlist-index replacement: %w", err)
	}
	return nil
}

// PlaylistMemberships returns YouTube playlist IDs containing one track.
func (s *Store) PlaylistMemberships(videoID string) ([]string, error) {
	rows, err := s.db.Query("SELECT playlist_id FROM playlist_track WHERE video_id=? ORDER BY playlist_id", videoID)
	if err != nil {
		return nil, fmt.Errorf("query playlist memberships: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var playlistID string
		if err := rows.Scan(&playlistID); err != nil {
			return nil, fmt.Errorf("scan playlist membership: %w", err)
		}
		out = append(out, playlistID)
	}
	if out == nil {
		out = []string{}
	}
	return out, rows.Err()
}

// ClearPlaylistTracks clears all YouTube playlist membership rows.
func (s *Store) ClearPlaylistTracks() error {
	_, err := s.db.Exec("DELETE FROM playlist_track")
	if err != nil {
		return fmt.Errorf("clear playlist index: %w", err)
	}
	return nil
}

// PrunePlaylistIndex removes YouTube playlists not present in observedIDs.
func (s *Store) PrunePlaylistIndex(observedIDs []string) error {
	rows, err := s.db.Query("SELECT DISTINCT playlist_id FROM playlist_track")
	if err != nil {
		return fmt.Errorf("query indexed playlists: %w", err)
	}
	var existing []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return fmt.Errorf("scan indexed playlist: %w", err)
		}
		existing = append(existing, id)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close indexed playlists: %w", err)
	}
	observed := make(map[string]struct{}, len(observedIDs))
	for _, id := range observedIDs {
		observed[id] = struct{}{}
	}
	for _, id := range existing {
		if _, ok := observed[id]; !ok {
			if _, err := s.db.Exec("DELETE FROM playlist_track WHERE playlist_id=?", id); err != nil {
				return fmt.Errorf("prune indexed playlist: %w", err)
			}
		}
	}
	return nil
}

// PlaylistIndex returns combined YouTube and device playlist memberships.
func (s *Store) PlaylistIndex() (model.PlaylistIndex, error) {
	out, err := s.LocalPlaylistMemberships()
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query("SELECT video_id,playlist_id FROM playlist_track ORDER BY video_id,playlist_id")
	if err != nil {
		return nil, fmt.Errorf("query playlist index: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var videoID, playlistID string
		if err := rows.Scan(&videoID, &playlistID); err != nil {
			return nil, fmt.Errorf("scan playlist index: %w", err)
		}
		out[videoID] = append(out[videoID], playlistID)
	}
	for videoID := range out {
		sort.Strings(out[videoID])
	}
	return out, rows.Err()
}

// PlaylistIndexSyncedAt returns the last successful YouTube index sync time.
func (s *Store) PlaylistIndexSyncedAt() (time.Time, bool) {
	value, ok := s.Setting(playlistIndexSyncedAtKey)
	if !ok {
		return time.Time{}, false
	}
	seconds, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return time.Time{}, false
	}
	return time.Unix(seconds, 0), true
}

// SetPlaylistIndexSyncedAt records the last successful YouTube index sync time.
func (s *Store) SetPlaylistIndexSyncedAt(at time.Time) error {
	return s.setRaw(playlistIndexSyncedAtKey, strconv.FormatInt(at.Unix(), 10))
}
