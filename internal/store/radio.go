package store

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/ryoku-dev/ryotunes/internal/model"
)

// PutRadioStation stores station metadata and retains the 64 newest rows.
func (s *Store) PutRadioStation(station model.RadioStation) error {
	raw, err := json.Marshal(station)
	if err != nil {
		return fmt.Errorf("encode radio station: %w", err)
	}
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin radio cache write: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`INSERT INTO radio_station_cache(station_id,station_json,updated_at) VALUES(?,?,?)
		ON CONFLICT(station_id) DO UPDATE SET station_json=excluded.station_json,updated_at=excluded.updated_at`,
		station.StationUUID, string(raw), s.now().UnixNano()); err != nil {
		return fmt.Errorf("put radio station: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM radio_station_cache WHERE station_id IN (
		SELECT station_id FROM radio_station_cache ORDER BY updated_at DESC LIMIT -1 OFFSET 64)`); err != nil {
		return fmt.Errorf("bound radio cache: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit radio cache write: %w", err)
	}
	return nil
}

// RadioStation returns cached metadata for one station UUID.
func (s *Store) RadioStation(stationUUID string) (model.RadioStation, bool, error) {
	var raw string
	err := s.db.QueryRow("SELECT station_json FROM radio_station_cache WHERE station_id=?", stationUUID).Scan(&raw)
	if err == sql.ErrNoRows {
		return model.RadioStation{}, false, nil
	}
	if err != nil {
		return model.RadioStation{}, false, fmt.Errorf("get radio station: %w", err)
	}
	var station model.RadioStation
	if err := json.Unmarshal([]byte(raw), &station); err != nil {
		return model.RadioStation{}, false, fmt.Errorf("decode radio station: %w", err)
	}
	return station, true, nil
}
