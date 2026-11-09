package store

import (
	"database/sql"
	"fmt"
)

const schemaVersion = 1

var schemaStatements = []string{
	`CREATE TABLE IF NOT EXISTS settings (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS stream_url_cache (
		video_id TEXT PRIMARY KEY,
		url TEXT NOT NULL,
		itag INTEGER NOT NULL,
		expires_at INTEGER NOT NULL,
		loudness_db REAL,
		is_video INTEGER NOT NULL DEFAULT 0,
		ping_url TEXT,
		ping_client TEXT
	)`,
	`CREATE TABLE IF NOT EXISTS lyrics_cache (
		video_id TEXT PRIMARY KEY,
		lyrics TEXT,
		fetched_at INTEGER NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS plays (
		id INTEGER PRIMARY KEY,
		video_id TEXT NOT NULL,
		played_at INTEGER NOT NULL,
		song_json TEXT NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS plays_played_at ON plays(played_at)`,
	`CREATE TABLE IF NOT EXISTS local_tracks (
		path TEXT PRIMARY KEY,
		title TEXT NOT NULL,
		artist TEXT NOT NULL,
		album TEXT NOT NULL,
		album_key TEXT NOT NULL,
		track_no INTEGER NOT NULL DEFAULT 0,
		duration_secs INTEGER NOT NULL DEFAULT 0,
		cover TEXT,
		mtime INTEGER NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS local_tracks_album ON local_tracks(album_key)`,
	`CREATE TABLE IF NOT EXISTS playlist_track (
		playlist_id TEXT NOT NULL,
		video_id TEXT NOT NULL,
		PRIMARY KEY(playlist_id, video_id)
	) WITHOUT ROWID`,
	`CREATE INDEX IF NOT EXISTS playlist_track_video ON playlist_track(video_id)`,
	`CREATE TABLE IF NOT EXISTS local_playlists (
		id TEXT PRIMARY KEY,
		title TEXT NOT NULL,
		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS local_playlist_tracks (
		playlist_id TEXT NOT NULL,
		video_id TEXT NOT NULL,
		position INTEGER NOT NULL,
		song_json TEXT NOT NULL,
		PRIMARY KEY(playlist_id, video_id)
	) WITHOUT ROWID`,
	`CREATE INDEX IF NOT EXISTS local_playlist_tracks_position ON local_playlist_tracks(playlist_id, position)`,
	`CREATE TABLE IF NOT EXISTS radio_station_cache (
		station_id TEXT PRIMARY KEY,
		station_json TEXT NOT NULL,
		updated_at INTEGER NOT NULL
	)`,
}

func migrate(db *sql.DB) error {
	var version int
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin schema migration: %w", err)
	}
	defer tx.Rollback()
	for _, statement := range schemaStatements {
		if _, err := tx.Exec(statement); err != nil {
			return fmt.Errorf("create schema: %w", err)
		}
	}
	for _, column := range []struct {
		table string
		name  string
		decl  string
	}{
		{"stream_url_cache", "loudness_db", "REAL"},
		{"stream_url_cache", "is_video", "INTEGER NOT NULL DEFAULT 0"},
		{"stream_url_cache", "ping_url", "TEXT"},
		{"stream_url_cache", "ping_client", "TEXT"},
	} {
		exists, err := columnExists(tx, column.table, column.name)
		if err != nil {
			return err
		}
		if !exists {
			if _, err := tx.Exec("ALTER TABLE " + column.table + " ADD COLUMN " + column.name + " " + column.decl); err != nil {
				return fmt.Errorf("add %s.%s: %w", column.table, column.name, err)
			}
		}
	}
	if version < schemaVersion {
		if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version=%d", schemaVersion)); err != nil {
			return fmt.Errorf("record schema version: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit schema migration: %w", err)
	}
	return nil
}

func columnExists(tx *sql.Tx, table, column string) (bool, error) {
	rows, err := tx.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return false, fmt.Errorf("inspect %s schema: %w", table, err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, kind string
		var notNull, primaryKey int
		var defaultValue any
		if err := rows.Scan(&cid, &name, &kind, &notNull, &defaultValue, &primaryKey); err != nil {
			return false, fmt.Errorf("scan %s schema: %w", table, err)
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}
