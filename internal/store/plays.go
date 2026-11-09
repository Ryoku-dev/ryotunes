package store

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ryoku-dev/ryotunes/internal/model"
)

const playRetention = 30 * 24 * time.Hour

// TrackPlay pairs a track with its aggregate play count.
type TrackPlay struct {
	Track model.Track
	Plays int
}

type playRow struct {
	track    model.Track
	playedAt int64
}

// RecordPlay appends one eligible play and bounds history to the retention window.
func (s *Store) RecordPlay(track model.Track) error {
	if !playEligible(track.VideoID) {
		return nil
	}
	raw, err := json.Marshal(track)
	if err != nil {
		return fmt.Errorf("encode played track: %w", err)
	}
	now := s.now()
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin play write: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec("INSERT INTO plays(video_id,played_at,song_json) VALUES(?,?,?)", track.VideoID, now.Unix(), string(raw)); err != nil {
		return fmt.Errorf("record play: %w", err)
	}
	if _, err := tx.Exec("DELETE FROM plays WHERE played_at<?", now.Add(-playRetention).Unix()); err != nil {
		return fmt.Errorf("prune plays: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit play write: %w", err)
	}
	return nil
}

// PlayCounts returns grouped counts at or after since.
func (s *Store) PlayCounts(since time.Time) (model.PlayCounts, error) {
	rows, err := s.db.Query("SELECT video_id,COUNT(*) FROM plays WHERE played_at>=? GROUP BY video_id", since.Unix())
	if err != nil {
		return nil, fmt.Errorf("query play counts: %w", err)
	}
	defer rows.Close()
	out := make(model.PlayCounts)
	for rows.Next() {
		var videoID string
		var count int
		if err := rows.Scan(&videoID, &count); err != nil {
			return nil, fmt.Errorf("scan play count: %w", err)
		}
		out[videoID] = count
	}
	return out, rows.Err()
}

// ListeningStats returns the aggregate day, week, or month listening summary.
func (s *Store) ListeningStats(period model.ListeningPeriod) (model.ListeningStats, error) {
	window := 7 * 24 * time.Hour
	if period == model.ListeningDay {
		window = 24 * time.Hour
	} else if period == model.ListeningMonth {
		window = 30 * 24 * time.Hour
	} else if period != model.ListeningWeek {
		period = model.ListeningWeek
	}
	plays, err := s.playRows(s.now().Add(-window), s.now())
	if err != nil {
		return model.ListeningStats{}, err
	}
	stats := model.ListeningStats{Period: period, Plays: len(plays), TopArtists: []model.ArtistPlayCount{}, TopTracks: []model.TrackPlayCount{}}
	artists := make(map[string]int)
	tracks := make(map[string]*model.TrackPlayCount)
	for _, play := range plays {
		artists[play.track.Artists]++
		row := tracks[play.track.VideoID]
		if row == nil {
			row = &model.TrackPlayCount{Title: play.track.Title, Artists: play.track.Artists}
			tracks[play.track.VideoID] = row
		}
		row.Plays++
		stats.KnownDurationSeconds += parseDuration(play.track.Duration)
	}
	for name, count := range artists {
		stats.TopArtists = append(stats.TopArtists, model.ArtistPlayCount{Name: name, Plays: count})
	}
	sort.Slice(stats.TopArtists, func(i, j int) bool {
		if stats.TopArtists[i].Plays != stats.TopArtists[j].Plays {
			return stats.TopArtists[i].Plays > stats.TopArtists[j].Plays
		}
		return stats.TopArtists[i].Name < stats.TopArtists[j].Name
	})
	for _, row := range tracks {
		stats.TopTracks = append(stats.TopTracks, *row)
	}
	sort.Slice(stats.TopTracks, func(i, j int) bool {
		if stats.TopTracks[i].Plays != stats.TopTracks[j].Plays {
			return stats.TopTracks[i].Plays > stats.TopTracks[j].Plays
		}
		return stats.TopTracks[i].Title < stats.TopTracks[j].Title
	})
	if len(stats.TopArtists) > 8 {
		stats.TopArtists = stats.TopArtists[:8]
	}
	if len(stats.TopTracks) > 8 {
		stats.TopTracks = stats.TopTracks[:8]
	}
	return stats, nil
}

// TopPlays returns tracks ranked by count and then latest play.
func (s *Store) TopPlays(since time.Time, limit int) ([]TrackPlay, error) {
	plays, err := s.playRows(since, s.now())
	if err != nil {
		return nil, err
	}
	type aggregate struct {
		TrackPlay
		latest int64
	}
	byID := make(map[string]*aggregate)
	for _, play := range plays {
		entry := byID[play.track.VideoID]
		if entry == nil {
			entry = &aggregate{TrackPlay: TrackPlay{Track: play.track}}
			byID[play.track.VideoID] = entry
		}
		entry.Plays++
		if play.playedAt > entry.latest {
			entry.latest = play.playedAt
			entry.Track = play.track
		}
	}
	ranked := make([]*aggregate, 0, len(byID))
	for _, entry := range byID {
		ranked = append(ranked, entry)
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].Plays != ranked[j].Plays {
			return ranked[i].Plays > ranked[j].Plays
		}
		return ranked[i].latest > ranked[j].latest
	})
	if limit > 0 && len(ranked) > limit {
		ranked = ranked[:limit]
	}
	out := make([]TrackPlay, len(ranked))
	for i, entry := range ranked {
		out[i] = entry.TrackPlay
	}
	return out, nil
}

// RecentUniquePlays returns the latest track metadata per ID, newest first.
func (s *Store) RecentUniquePlays(since time.Time, limit int) ([]model.Track, error) {
	plays, err := s.playRows(since, s.now())
	if err != nil {
		return nil, err
	}
	sort.Slice(plays, func(i, j int) bool { return plays[i].playedAt > plays[j].playedAt })
	return uniqueTracks(plays, limit), nil
}

// RediscoverPlays returns unique tracks last played in an interval, oldest first.
func (s *Store) RediscoverPlays(older, newer time.Time, limit int) ([]model.Track, error) {
	plays, err := s.playRows(time.Unix(0, 0), s.now())
	if err != nil {
		return nil, err
	}
	latest := make(map[string]playRow)
	for _, play := range plays {
		if current, ok := latest[play.track.VideoID]; !ok || play.playedAt > current.playedAt {
			latest[play.track.VideoID] = play
		}
	}
	rows := make([]playRow, 0, len(latest))
	for _, play := range latest {
		if play.playedAt >= older.Unix() && play.playedAt <= newer.Unix() {
			rows = append(rows, play)
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].playedAt < rows[j].playedAt })
	return uniqueTracks(rows, limit), nil
}

func (s *Store) playRows(since, until time.Time) ([]playRow, error) {
	rows, err := s.db.Query("SELECT song_json,played_at FROM plays WHERE played_at>=? AND played_at<=?", since.Unix(), until.Unix())
	if err != nil {
		return nil, fmt.Errorf("query plays: %w", err)
	}
	defer rows.Close()
	var out []playRow
	for rows.Next() {
		var raw string
		var play playRow
		if err := rows.Scan(&raw, &play.playedAt); err != nil {
			return nil, fmt.Errorf("scan play: %w", err)
		}
		if err := json.Unmarshal([]byte(raw), &play.track); err != nil {
			s.log.Warn("ignoring malformed play row", "error", err)
			continue
		}
		out = append(out, play)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read plays: %w", err)
	}
	return out, nil
}

func uniqueTracks(rows []playRow, limit int) []model.Track {
	seen := make(map[string]struct{})
	out := make([]model.Track, 0, len(rows))
	for _, row := range rows {
		if _, ok := seen[row.track.VideoID]; ok {
			continue
		}
		seen[row.track.VideoID] = struct{}{}
		out = append(out, row.track)
		if limit > 0 && len(out) == limit {
			break
		}
	}
	return out
}

func playEligible(id string) bool {
	return id != "" && !strings.HasPrefix(id, "LOCAL:") && !strings.HasPrefix(id, "RYOTUNES_RADIO:") &&
		!strings.HasPrefix(id, "spotify:") && !strings.HasPrefix(id, "sc:")
}

func parseDuration(value *string) int64 {
	if value == nil {
		return 0
	}
	parts := strings.Split(*value, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0
	}
	var seconds int64
	for _, part := range parts {
		n, err := strconv.ParseInt(part, 10, 64)
		if err != nil || n < 0 {
			return 0
		}
		seconds = seconds*60 + n
	}
	return seconds
}
