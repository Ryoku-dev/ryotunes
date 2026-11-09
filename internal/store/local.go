package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/ryoku-dev/ryotunes/internal/model"
	providerlocal "github.com/ryoku-dev/ryotunes/internal/provider/local"
)

var _ providerlocal.Index = (*Store)(nil)

// UpsertLocalTrack inserts or replaces scanner metadata for one path.
func (s *Store) UpsertLocalTrack(track providerlocal.LocalTrack) error {
	_, err := s.db.Exec(`INSERT INTO local_tracks
		(path,title,artist,album,album_key,track_no,duration_secs,cover,mtime)
		VALUES(?,?,?,?,?,?,?,?,?) ON CONFLICT(path) DO UPDATE SET
		title=excluded.title,artist=excluded.artist,album=excluded.album,
		album_key=excluded.album_key,track_no=excluded.track_no,
		duration_secs=excluded.duration_secs,cover=excluded.cover,mtime=excluded.mtime`,
		track.Path, track.Title, track.Artist, track.Album, track.AlbumKey,
		track.TrackNo, track.DurationSecs, nullableString(track.Cover), track.MTime)
	if err != nil {
		return fmt.Errorf("upsert local track: %w", err)
	}
	return nil
}

// DeleteLocalTrack deletes one scanner row by path.
func (s *Store) DeleteLocalTrack(path string) error {
	_, err := s.db.Exec("DELETE FROM local_tracks WHERE path=?", path)
	if err != nil {
		return fmt.Errorf("delete local track: %w", err)
	}
	return nil
}

// AllLocalTracks returns scanner rows ordered by album, disc position, and title.
func (s *Store) AllLocalTracks() ([]providerlocal.LocalTrack, error) {
	return s.queryLocalTracks("", nil)
}

// LocalTracksByAlbum returns scanner rows for one album key.
func (s *Store) LocalTracksByAlbum(albumKey string) ([]providerlocal.LocalTrack, error) {
	return s.queryLocalTracks(" WHERE album_key=?", []any{albumKey})
}

// PruneMissingLocalTracks removes rows absent from foundPaths and returns removed paths.
func (s *Store) PruneMissingLocalTracks(foundPaths []string) ([]string, error) {
	found := make(map[string]struct{}, len(foundPaths))
	for _, path := range foundPaths {
		found[path] = struct{}{}
	}
	tracks, err := s.AllLocalTracks()
	if err != nil {
		return nil, err
	}
	var removed []string
	for _, track := range tracks {
		if _, ok := found[track.Path]; !ok {
			removed = append(removed, track.Path)
		}
	}
	if len(removed) == 0 {
		return []string{}, nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin local-track prune: %w", err)
	}
	defer tx.Rollback()
	for _, path := range removed {
		if _, err := tx.Exec("DELETE FROM local_tracks WHERE path=?", path); err != nil {
			return nil, fmt.Errorf("prune local track: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit local-track prune: %w", err)
	}
	return removed, nil
}

// LocalFolders returns the persisted canonical scan roots.
func (s *Store) LocalFolders() ([]string, error) {
	raw, ok := s.Setting("local_folders")
	if !ok || raw == "" {
		return []string{}, nil
	}
	var folders []string
	if err := json.Unmarshal([]byte(raw), &folders); err != nil {
		return nil, fmt.Errorf("decode local folders: %w", err)
	}
	return folders, nil
}

// SetLocalFolders replaces the persisted canonical scan roots.
func (s *Store) SetLocalFolders(folders []string) error {
	folders = append([]string(nil), folders...)
	sort.Strings(folders)
	raw, err := json.Marshal(folders)
	if err != nil {
		return fmt.Errorf("encode local folders: %w", err)
	}
	return s.setRaw("local_folders", string(raw))
}

// LocalScanVersion returns the scanner metadata version.
func (s *Store) LocalScanVersion() (string, error) {
	value, _ := s.Setting("local_scan_version")
	return value, nil
}

// SetLocalScanVersion persists the scanner metadata version.
func (s *Store) SetLocalScanVersion(version string) error {
	return s.setRaw("local_scan_version", version)
}

// LocalPlaylists returns device playlists ordered by most recent update.
func (s *Store) LocalPlaylists() ([]model.DevicePlaylist, error) {
	rows, err := s.db.Query(`SELECT p.id,p.title,p.created_at,p.updated_at,COUNT(t.video_id)
		FROM local_playlists p LEFT JOIN local_playlist_tracks t ON t.playlist_id=p.id
		GROUP BY p.id,p.title,p.created_at,p.updated_at ORDER BY p.updated_at DESC,p.title COLLATE NOCASE`)
	if err != nil {
		return nil, fmt.Errorf("query local playlists: %w", err)
	}
	defer rows.Close()
	var out []model.DevicePlaylist
	for rows.Next() {
		var playlist model.DevicePlaylist
		if err := rows.Scan(&playlist.ID, &playlist.Title, &playlist.CreatedAt, &playlist.UpdatedAt, &playlist.TrackCount); err != nil {
			return nil, fmt.Errorf("scan local playlist: %w", err)
		}
		out = append(out, playlist)
	}
	if out == nil {
		out = []model.DevicePlaylist{}
	}
	return out, rows.Err()
}

// LocalPlaylist returns one device playlist and its membership count.
func (s *Store) LocalPlaylist(id string) (model.DevicePlaylist, bool, error) {
	var playlist model.DevicePlaylist
	err := s.db.QueryRow(`SELECT p.id,p.title,p.created_at,p.updated_at,COUNT(t.video_id)
		FROM local_playlists p LEFT JOIN local_playlist_tracks t ON t.playlist_id=p.id
		WHERE p.id=? GROUP BY p.id,p.title,p.created_at,p.updated_at`, id).Scan(
		&playlist.ID, &playlist.Title, &playlist.CreatedAt, &playlist.UpdatedAt, &playlist.TrackCount)
	if err == sql.ErrNoRows {
		return model.DevicePlaylist{}, false, nil
	}
	if err != nil {
		return model.DevicePlaylist{}, false, fmt.Errorf("get local playlist: %w", err)
	}
	return playlist, true, nil
}

// CreateLocalPlaylist inserts a device playlist.
func (s *Store) CreateLocalPlaylist(playlist model.DevicePlaylist) error {
	_, err := s.db.Exec("INSERT INTO local_playlists(id,title,created_at,updated_at) VALUES(?,?,?,?)",
		playlist.ID, playlist.Title, playlist.CreatedAt, playlist.UpdatedAt)
	if err != nil {
		return fmt.Errorf("create local playlist: %w", err)
	}
	return nil
}

// RenameLocalPlaylist changes a device playlist title and update timestamp.
func (s *Store) RenameLocalPlaylist(id, title string) error {
	_, err := s.db.Exec("UPDATE local_playlists SET title=?,updated_at=? WHERE id=?", title, s.now().Unix(), id)
	if err != nil {
		return fmt.Errorf("rename local playlist: %w", err)
	}
	return nil
}

// DeleteLocalPlaylist deletes a device playlist, members, and persisted presentation settings.
func (s *Store) DeleteLocalPlaylist(id string) error {
	coverPath, hasCover := s.Setting(playlistCoverKey(id))
	s.settingsWrite.Lock()
	defer s.settingsWrite.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin local playlist delete: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec("DELETE FROM local_playlist_tracks WHERE playlist_id=?", id); err != nil {
		return fmt.Errorf("delete local playlist tracks: %w", err)
	}
	if _, err := tx.Exec("DELETE FROM local_playlists WHERE id=?", id); err != nil {
		return fmt.Errorf("delete local playlist: %w", err)
	}
	for _, key := range []string{playlistCoverKey(id), playlistCoverSyncedKey(id), playlistSortKey(id)} {
		if _, err := tx.Exec("DELETE FROM settings WHERE key=?", key); err != nil {
			return fmt.Errorf("delete local playlist setting: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit local playlist delete: %w", err)
	}
	for _, key := range []string{playlistCoverKey(id), playlistCoverSyncedKey(id), playlistSortKey(id)} {
		s.settings.Delete(key)
	}
	if hasCover {
		if err := s.removePlaylistCoverFile(coverPath); err != nil {
			return err
		}
	}
	return nil
}

// AddLocalPlaylistTrack appends a track unless the playlist already contains it.
func (s *Store) AddLocalPlaylistTrack(playlistID string, track model.Track) (bool, error) {
	raw, err := json.Marshal(track)
	if err != nil {
		return false, fmt.Errorf("encode local playlist track: %w", err)
	}
	tx, err := s.db.Begin()
	if err != nil {
		return false, fmt.Errorf("begin local playlist append: %w", err)
	}
	defer tx.Rollback()
	var position int
	if err := tx.QueryRow("SELECT COALESCE(MAX(position)+1,0) FROM local_playlist_tracks WHERE playlist_id=?", playlistID).Scan(&position); err != nil {
		return false, fmt.Errorf("find local playlist tail: %w", err)
	}
	result, err := tx.Exec(`INSERT OR IGNORE INTO local_playlist_tracks(playlist_id,video_id,position,song_json)
		VALUES(?,?,?,?)`, playlistID, track.VideoID, position, string(raw))
	if err != nil {
		return false, fmt.Errorf("append local playlist track: %w", err)
	}
	added, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("inspect local playlist append: %w", err)
	}
	if added != 0 {
		if _, err := tx.Exec("UPDATE local_playlists SET updated_at=? WHERE id=?", s.now().Unix(), playlistID); err != nil {
			return false, fmt.Errorf("touch local playlist: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit local playlist append: %w", err)
	}
	return added != 0, nil
}

// RemoveLocalPlaylistTrack removes one member and compacts remaining positions.
func (s *Store) RemoveLocalPlaylistTrack(playlistID, trackID string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin local playlist removal: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec("DELETE FROM local_playlist_tracks WHERE playlist_id=? AND video_id=?", playlistID, trackID); err != nil {
		return fmt.Errorf("remove local playlist track: %w", err)
	}
	if err := compactPlaylistPositions(tx, playlistID); err != nil {
		return err
	}
	if _, err := tx.Exec("UPDATE local_playlists SET updated_at=? WHERE id=?", s.now().Unix(), playlistID); err != nil {
		return fmt.Errorf("touch local playlist: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit local playlist removal: %w", err)
	}
	return nil
}

// LocalPlaylistTracks returns members in playlist order.
func (s *Store) LocalPlaylistTracks(playlistID string) ([]model.Track, error) {
	rows, err := s.db.Query("SELECT song_json FROM local_playlist_tracks WHERE playlist_id=? ORDER BY position", playlistID)
	if err != nil {
		return nil, fmt.Errorf("query local playlist tracks: %w", err)
	}
	defer rows.Close()
	var out []model.Track
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("scan local playlist track: %w", err)
		}
		var track model.Track
		if err := json.Unmarshal([]byte(raw), &track); err != nil {
			return nil, fmt.Errorf("decode local playlist track: %w", err)
		}
		out = append(out, track)
	}
	if out == nil {
		out = []model.Track{}
	}
	return out, rows.Err()
}

// LocalPlaylistMemberships returns device-playlist memberships by track ID.
func (s *Store) LocalPlaylistMemberships() (model.PlaylistIndex, error) {
	rows, err := s.db.Query("SELECT video_id,playlist_id FROM local_playlist_tracks ORDER BY video_id,playlist_id")
	if err != nil {
		return nil, fmt.Errorf("query local playlist memberships: %w", err)
	}
	defer rows.Close()
	out := make(model.PlaylistIndex)
	for rows.Next() {
		var videoID, playlistID string
		if err := rows.Scan(&videoID, &playlistID); err != nil {
			return nil, fmt.Errorf("scan local playlist membership: %w", err)
		}
		out[videoID] = append(out[videoID], playlistID)
	}
	return out, rows.Err()
}

func (s *Store) queryLocalTracks(where string, args []any) ([]providerlocal.LocalTrack, error) {
	rows, err := s.db.Query(`SELECT path,title,artist,album,album_key,track_no,duration_secs,COALESCE(cover,''),mtime
		FROM local_tracks`+where+` ORDER BY album COLLATE NOCASE,track_no,title COLLATE NOCASE`, args...)
	if err != nil {
		return nil, fmt.Errorf("query local tracks: %w", err)
	}
	defer rows.Close()
	var out []providerlocal.LocalTrack
	for rows.Next() {
		var track providerlocal.LocalTrack
		if err := rows.Scan(&track.Path, &track.Title, &track.Artist, &track.Album, &track.AlbumKey,
			&track.TrackNo, &track.DurationSecs, &track.Cover, &track.MTime); err != nil {
			return nil, fmt.Errorf("scan local track: %w", err)
		}
		out = append(out, track)
	}
	if out == nil {
		out = []providerlocal.LocalTrack{}
	}
	return out, rows.Err()
}

func compactPlaylistPositions(tx *sql.Tx, playlistID string) error {
	rows, err := tx.Query("SELECT video_id FROM local_playlist_tracks WHERE playlist_id=? ORDER BY position", playlistID)
	if err != nil {
		return fmt.Errorf("query local playlist positions: %w", err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return fmt.Errorf("scan local playlist position: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close local playlist positions: %w", err)
	}
	for position, id := range ids {
		if _, err := tx.Exec("UPDATE local_playlist_tracks SET position=? WHERE playlist_id=? AND video_id=?", position, playlistID, id); err != nil {
			return fmt.Errorf("compact local playlist position: %w", err)
		}
	}
	return nil
}
