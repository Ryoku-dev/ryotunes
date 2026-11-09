package store

import (
	"encoding/json"
	"errors"

	"github.com/ryoku-dev/ryotunes/internal/model"
)

const maxPersonalBytes = 1 << 20

var emptyPersonal = model.Personal(`{"picks":[],"saved":[],"pins":[],"recent":{},"artists":{},"dismissedSeeds":[],"home":{"order":[],"hidden":[],"seen":[]}}`)

// ErrPersonalInvalid reports a personal payload that is not a JSON object.
var ErrPersonalInvalid = errors.New("personal data must be a JSON object")

// ErrPersonalTooLarge reports a personal payload larger than one MiB.
var ErrPersonalTooLarge = errors.New("personal data exceeds 1 MiB")

// Personal returns the opaque renderer-owned object or the canonical empty shape.
func (s *Store) Personal() model.Personal {
	raw, ok := s.Setting("personal_json")
	if !ok || !validJSONObject([]byte(raw)) {
		return append(model.Personal(nil), emptyPersonal...)
	}
	return model.Personal([]byte(raw))
}

// SetPersonal validates and stores an opaque renderer-owned JSON object.
func (s *Store) SetPersonal(personal model.Personal) error {
	if len(personal) > maxPersonalBytes {
		return ErrPersonalTooLarge
	}
	if !validJSONObject(personal) {
		return ErrPersonalInvalid
	}
	return s.setRaw("personal_json", string(personal))
}

func validJSONObject(raw []byte) bool {
	if !json.Valid(raw) {
		return false
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return false
	}
	_, ok := value.(map[string]any)
	return ok
}
