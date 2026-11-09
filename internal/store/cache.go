package store

import (
	"fmt"
	"os"
	"path/filepath"
)

// ClearCaches clears playback URL, lyrics, PoToken, and disk audio caches.
func (s *Store) ClearCaches() error {
	s.settingsWrite.Lock()
	defer s.settingsWrite.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin cache clear: %w", err)
	}
	defer tx.Rollback()
	for _, statement := range []string{
		"DELETE FROM stream_url_cache",
		"DELETE FROM lyrics_cache",
		"DELETE FROM settings WHERE key='potoken_session'",
	} {
		if _, err := tx.Exec(statement); err != nil {
			return fmt.Errorf("clear database cache: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit cache clear: %w", err)
	}
	s.settings.Delete("potoken_session")
	audioDir := s.dirs.Cache
	entries, err := os.ReadDir(audioDir)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("list audio cache: %w", err)
	}
	for _, entry := range entries {
		if err := os.RemoveAll(filepath.Join(audioDir, entry.Name())); err != nil {
			return fmt.Errorf("clear audio cache: %w", err)
		}
	}
	return nil
}
