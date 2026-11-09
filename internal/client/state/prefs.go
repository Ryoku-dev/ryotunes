package state

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// ClientPreferences is the client-only window, panel, appearance, and zoom state.
type ClientPreferences struct {
	WindowWidth  int    `json:"windowWidth"`
	WindowHeight int    `json:"windowHeight"`
	WindowX      int    `json:"windowX,omitempty"`
	WindowY      int    `json:"windowY,omitempty"`
	PanelOpen    bool   `json:"panelOpen"`
	PanelTab     string `json:"panelTab"`
	SidebarOpen  bool   `json:"sidebarOpen"`
	ZoomPercent  int    `json:"zoomPercent"`
	Appearance   string `json:"appearance"`
	Density      string `json:"density"`
	ReduceMotion bool   `json:"reduceMotion"`
	PaletteFile  string `json:"paletteFile,omitempty"`
	Skin         string `json:"skin,omitempty"`
	Decorations  bool   `json:"decorations"`
	MiniRight    int    `json:"miniRight"`
	MiniBottom   int    `json:"miniBottom"`
	Route        string `json:"route,omitempty"`
	MiniOpen     bool   `json:"miniOpen"`
}

// DefaultClientPreferences returns safe defaults for malformed or absent state.
func DefaultClientPreferences() ClientPreferences {
	return ClientPreferences{
		PanelOpen: true, PanelTab: "queue", SidebarOpen: true, ZoomPercent: 110,
		Appearance: "desktop", Density: "comfortable", Decorations: true,
	}
}

// ClientPreferencesPath returns the XDG client.json location.
func ClientPreferencesPath() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" || !filepath.IsAbs(base) {
		home, err := os.UserHomeDir()
		if err != nil {
			return filepath.Join(".config", "ryotunes", "client.json")
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "ryotunes", "client.json")
}

// LoadClientPreferences reads client.json, using defaults for missing or malformed data.
func LoadClientPreferences() (ClientPreferences, error) {
	defaults := DefaultClientPreferences()
	data, err := os.ReadFile(ClientPreferencesPath())
	if errors.Is(err, os.ErrNotExist) {
		return defaults, nil
	}
	if err != nil {
		return defaults, err
	}
	preferences := defaults
	if json.Unmarshal(data, &preferences) != nil {
		return defaults, nil
	}
	if preferences.ZoomPercent < 80 || preferences.ZoomPercent > 140 || preferences.ZoomPercent%10 != 0 {
		preferences.ZoomPercent = defaults.ZoomPercent
	}
	if preferences.PanelTab != "queue" && preferences.PanelTab != "lyrics" {
		preferences.PanelTab = defaults.PanelTab
	}
	switch preferences.Appearance {
	case "desktop", "dark", "light":
	default:
		preferences.Appearance = defaults.Appearance
	}
	switch preferences.Density {
	case "comfortable", "compact":
	default:
		preferences.Density = defaults.Density
	}
	return preferences, nil
}

// SaveClientPreferences atomically writes client.json with private permissions.
func SaveClientPreferences(preferences ClientPreferences) error {
	path := ClientPreferencesPath()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	file, err := os.CreateTemp(dir, ".client-*.json")
	if err != nil {
		return err
	}
	temporary := file.Name()
	committed := false
	defer func() {
		if !committed {
			_ = os.Remove(temporary)
		}
	}()
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return err
	}
	encoder := json.NewEncoder(file)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(preferences); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporary, path); err != nil {
		return err
	}
	committed = true
	return nil
}

// SetClientPreferences replaces client-only state and invalidates subscribers.
func (s *Store) SetClientPreferences(preferences ClientPreferences) {
	s.mu.Lock()
	s.state.Client = preferences
	callbacks := s.callbacksLocked()
	s.mu.Unlock()
	notify(callbacks)
}

// LoadClientPreferences replaces the store's client state from client.json.
func (s *Store) LoadClientPreferences() error {
	preferences, err := LoadClientPreferences()
	if err != nil {
		return err
	}
	s.SetClientPreferences(preferences)
	return nil
}

// SaveClientPreferences writes the store's current client state to client.json.
func (s *Store) SaveClientPreferences() error {
	s.mu.RLock()
	preferences := s.state.Client
	s.mu.RUnlock()
	return SaveClientPreferences(preferences)
}
