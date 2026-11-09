package store

import (
	"encoding/json"
	"fmt"

	"github.com/ryoku-dev/ryotunes/internal/model"
)

const accountIdentitiesKey = "account_identities_json"

// AccountIdentities returns the persisted selectable YouTube identities.
func (s *Store) AccountIdentities() (model.AccountIdentities, error) {
	raw, ok := s.Setting(accountIdentitiesKey)
	if !ok || raw == "" {
		return model.AccountIdentities{}, nil
	}
	var identities model.AccountIdentities
	if err := json.Unmarshal([]byte(raw), &identities); err != nil {
		return nil, fmt.Errorf("decode account identities: %w", err)
	}
	return identities, nil
}

// SetAccountIdentities replaces the persisted selectable YouTube identities.
func (s *Store) SetAccountIdentities(identities model.AccountIdentities) error {
	raw, err := json.Marshal(identities)
	if err != nil {
		return fmt.Errorf("encode account identities: %w", err)
	}
	return s.setRaw(accountIdentitiesKey, string(raw))
}

// ClearAccountIdentities removes persisted selectable YouTube identities.
func (s *Store) ClearAccountIdentities() error {
	return s.deleteRaw(accountIdentitiesKey)
}
