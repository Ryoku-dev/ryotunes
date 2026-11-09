package store

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

const databaseName = "ryotunes.sqlite"

// Store is the durable Ryotunes state store.
type Store struct {
	db            *sql.DB
	dirs          Dirs
	log           *slog.Logger
	settings      sync.Map
	settingsWrite sync.Mutex
	now           func() time.Time
}

// Open opens or creates the version-one-compatible database.
func Open(dirs Dirs, log *slog.Logger) (*Store, error) {
	if log == nil {
		log = slog.Default()
	}
	if err := dirs.ensure(); err != nil {
		return nil, err
	}
	dbPath := filepath.Join(dirs.Data, databaseName)
	if err := adoptSiblingDatabase(dirs.Data, dbPath); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(dbPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("create database: %w", err)
	}
	if err := file.Close(); err != nil {
		return nil, fmt.Errorf("close database file: %w", err)
	}
	if err := os.Chmod(dbPath, 0o600); err != nil {
		return nil, fmt.Errorf("secure database: %w", err)
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	for _, pragma := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA busy_timeout=5000",
	} {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("configure database: %w", err)
		}
	}
	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	s := &Store{db: db, dirs: dirs, log: log, now: time.Now}
	if err := s.loadSettings(); err != nil {
		db.Close()
		return nil, err
	}
	if err := s.pruneStartupCaches(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// Close closes the SQLite connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// Dirs returns the resolved store directories.
func (s *Store) Dirs() Dirs {
	return s.dirs
}

func adoptSiblingDatabase(dataDir, target string) error {
	if _, err := os.Stat(target); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect database: %w", err)
	}
	entries, err := os.ReadDir(dataDir)
	if err != nil {
		return fmt.Errorf("list data directory: %w", err)
	}
	var candidate string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".sqlite") || entry.Name() == databaseName {
			continue
		}
		if candidate != "" {
			return nil
		}
		candidate = filepath.Join(dataDir, entry.Name())
	}
	if candidate == "" {
		return nil
	}
	if err := os.Rename(candidate, target); err != nil {
		return fmt.Errorf("adopt sibling database: %w", err)
	}
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		if err := os.Rename(candidate+suffix, target+suffix); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("adopt sibling database sidecar: %w", err)
		}
	}
	return nil
}

func (s *Store) loadSettings() error {
	rows, err := s.db.Query("SELECT key, value FROM settings")
	if err != nil {
		return fmt.Errorf("load settings: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return fmt.Errorf("scan setting: %w", err)
		}
		s.settings.Store(key, value)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read settings: %w", err)
	}
	return nil
}

func (s *Store) pruneStartupCaches() error {
	now := s.now().Unix()
	if _, err := s.db.Exec("DELETE FROM stream_url_cache WHERE expires_at <= ?", now); err != nil {
		return fmt.Errorf("prune expired streams: %w", err)
	}
	if _, err := s.db.Exec("DELETE FROM plays WHERE video_id LIKE 'LOCAL:%'"); err != nil {
		return fmt.Errorf("prune legacy local plays: %w", err)
	}
	return nil
}
