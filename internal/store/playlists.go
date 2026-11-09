package store

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/ryoku-dev/ryotunes/internal/model"
)

const (
	// PlaylistOnRepeat is the synthetic 30-day most-played collection.
	PlaylistOnRepeat = "RYOTUNES_ON_REPEAT"
	// PlaylistRecentlyPlayed is the synthetic seven-day recent collection.
	PlaylistRecentlyPlayed = "RYOTUNES_RECENTLY_PLAYED"
	// PlaylistRediscover is the synthetic seven-to-fourteen-day collection.
	PlaylistRediscover = "RYOTUNES_REDISCOVER"
	// PlaylistLiked is the fixed on-device liked-songs collection.
	PlaylistLiked       = "RYOTUNES_LOCAL_PLAYLIST:liked"
	localPlaylistPrefix = "RYOTUNES_LOCAL_PLAYLIST:"
)

// CreateDevicePlaylist validates a title and creates a randomized on-device playlist.
func (s *Store) CreateDevicePlaylist(title string) (model.DevicePlaylist, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return model.DevicePlaylist{}, errors.New("playlist title must not be empty")
	}
	title = truncateRunes(title, 150)
	var suffix [12]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return model.DevicePlaylist{}, fmt.Errorf("generate playlist id: %w", err)
	}
	now := s.now().Unix()
	playlist := model.DevicePlaylist{
		ID: localPlaylistPrefix + hex.EncodeToString(suffix[:]), Title: title,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := s.CreateLocalPlaylist(playlist); err != nil {
		return model.DevicePlaylist{}, err
	}
	return playlist, nil
}

// ReorderLocalPlaylistTrack moves one member to a zero-based position.
func (s *Store) ReorderLocalPlaylistTrack(playlistID, trackID string, position int) error {
	tracks, err := s.LocalPlaylistTracks(playlistID)
	if err != nil {
		return err
	}
	current := -1
	for i := range tracks {
		if tracks[i].VideoID == trackID {
			current = i
			break
		}
	}
	if current < 0 {
		return sql.ErrNoRows
	}
	if position < 0 {
		position = 0
	}
	if position >= len(tracks) {
		position = len(tracks) - 1
	}
	moved := tracks[current]
	tracks = append(tracks[:current], tracks[current+1:]...)
	tracks = append(tracks, model.Track{})
	copy(tracks[position+1:], tracks[position:len(tracks)-1])
	tracks[position] = moved
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin local playlist reorder: %w", err)
	}
	defer tx.Rollback()
	for i, track := range tracks {
		if _, err := tx.Exec("UPDATE local_playlist_tracks SET position=? WHERE playlist_id=? AND video_id=?", i, playlistID, track.VideoID); err != nil {
			return fmt.Errorf("reorder local playlist track: %w", err)
		}
	}
	if _, err := tx.Exec("UPDATE local_playlists SET updated_at=? WHERE id=?", s.now().Unix(), playlistID); err != nil {
		return fmt.Errorf("touch local playlist: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit local playlist reorder: %w", err)
	}
	return nil
}

// SetPlaylistSort stores the local presentation sort for one playlist.
func (s *Store) SetPlaylistSort(playlistID string, sort model.PlaylistSort) error {
	return s.setRaw(playlistSortKey(playlistID), string(sort))
}

// PlaylistSort returns the stored local presentation sort.
func (s *Store) PlaylistSort(playlistID string) (model.PlaylistSort, bool) {
	value, ok := s.Setting(playlistSortKey(playlistID))
	return model.PlaylistSort(value), ok
}

// SetPlaylistCover validates and copies a custom JPEG or PNG playlist cover.
func (s *Store) SetPlaylistCover(playlistID, source string) (string, error) {
	ext := strings.ToLower(filepath.Ext(source))
	if ext != ".jpg" && ext != ".jpeg" && ext != ".png" {
		return "", errors.New("Pick a JPEG or PNG image.")
	}
	file, err := os.Open(source)
	if err != nil {
		return "", errors.New("Artwork must be a readable JPEG/PNG up to 8 MB.")
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 8<<20 {
		return "", errors.New("Artwork must be a readable JPEG/PNG up to 8 MB.")
	}
	data, err := io.ReadAll(io.LimitReader(file, (8<<20)+1))
	if err != nil || len(data) > 8<<20 {
		return "", errors.New("Artwork must be a readable JPEG/PNG up to 8 MB.")
	}
	if !validCoverMagic(data, ext) {
		return "", errors.New("That file is not a valid JPEG or PNG image.")
	}
	digest := sha256.Sum256([]byte(playlistID))
	destination := filepath.Join(s.dirs.Data, "covers", "playlists", hex.EncodeToString(digest[:12])+ext)
	old, hadOld := s.Setting(playlistCoverKey(playlistID))
	if err := WriteFileAtomic(destination, data, 0o600); err != nil {
		return "", fmt.Errorf("write playlist cover: %w", err)
	}
	if err := s.setRaw(playlistCoverKey(playlistID), destination); err != nil {
		return "", err
	}
	if hadOld && old != destination {
		_ = s.removePlaylistCoverFile(old)
	}
	return destination, nil
}

// ClearPlaylistCover removes a custom cover and its settings.
func (s *Store) ClearPlaylistCover(playlistID string) error {
	path, hadPath := s.Setting(playlistCoverKey(playlistID))
	if err := s.deleteRaw(playlistCoverKey(playlistID)); err != nil {
		return err
	}
	if err := s.deleteRaw(playlistCoverSyncedKey(playlistID)); err != nil {
		return err
	}
	if hadPath {
		return s.removePlaylistCoverFile(path)
	}
	return nil
}

// PlaylistArtwork returns a custom cover or derived automatic artwork.
func (s *Store) PlaylistArtwork(playlistID string) (string, error) {
	if path, ok := s.Setting(playlistCoverKey(playlistID)); ok {
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
			return path, nil
		}
	}
	tracks, err := s.LocalPlaylistTracks(playlistID)
	if err != nil {
		return "", err
	}
	covers := make([]string, 0, len(tracks))
	for _, track := range tracks {
		if track.Thumbnail != nil {
			covers = append(covers, *track.Thumbnail)
		}
	}
	return DerivePlaylistArtwork(covers), nil
}

// DerivePlaylistArtwork derives the legacy single-cover or exact four-cover marker.
func DerivePlaylistArtwork(covers []string) string {
	unique := make([]string, 0, 4)
	seen := make(map[string]struct{}, 4)
	for _, cover := range covers {
		if cover == "" {
			continue
		}
		if _, ok := seen[cover]; ok {
			continue
		}
		seen[cover] = struct{}{}
		unique = append(unique, cover)
		if len(unique) == 4 {
			break
		}
	}
	if len(unique) == 0 {
		return ""
	}
	if len(unique) < 4 {
		return unique[0]
	}
	raw, _ := json.Marshal(unique)
	return "ryotunes-collage:" + string(raw)
}

func (s *Store) removePlaylistCoverFile(path string) error {
	base := filepath.Join(s.dirs.Data, "covers", "playlists")
	relative, err := filepath.Rel(base, path)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return nil
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove playlist cover: %w", err)
	}
	return nil
}

func validCoverMagic(data []byte, ext string) bool {
	if ext == ".png" {
		return len(data) >= 8 && string(data[:8]) == "\x89PNG\r\n\x1a\n"
	}
	return len(data) >= 3 && data[0] == 0xff && data[1] == 0xd8 && data[2] == 0xff
}

func playlistCoverKey(id string) string {
	return "playlist_cover:" + strings.TrimPrefix(id, "VL")
}

func playlistCoverSyncedKey(id string) string { return playlistCoverKey(id) + ":synced" }
func playlistSortKey(id string) string        { return "playlist_sort:" + id }

func truncateRunes(value string, max int) string {
	if utf8.RuneCountInString(value) <= max {
		return value
	}
	runes := []rune(value)
	return string(runes[:max])
}
