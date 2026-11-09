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
	PanelOpen    bool   `json:"panelOpen"`
	PanelTab     string `json:"panelTab"`
	ZoomPercent  int    `json:"zoomPercent"`
	Appearance   string `json:"appearance"`
	Density      string `json:"density"`
	ReduceMotion bool   `json:"reduceMotion"`
	PaletteFile  string `json:"paletteFile,omitempty"`
	Decorations  bool   `json:"decorations"`
	Route        string `json:"route,omitempty"`
	MiniOpen     bool   `json:"miniOpen"`
	Visualizer   *bool  `json:"visualizer,omitempty"`
	WaveProgress *bool  `json:"waveProgress,omitempty"`
	// SidebarCollapsed folds the wide-window sidebar to its rail.
	SidebarCollapsed bool   `json:"sidebarCollapsed,omitempty"`
	Accent           string `json:"accent,omitempty"`
	VisualizerStyle  string `json:"visualizerStyle,omitempty"`
	Corners          string `json:"corners,omitempty"`
	CornerDp         int    `json:"cornerDp,omitempty"`
}

// VisualizerOn reports the stored visualizer preference; missing means on.
func (p ClientPreferences) VisualizerOn() bool {
	return p.Visualizer == nil || *p.Visualizer
}

// WaveOn reports the stored wave-progress preference; missing means on.
func (p ClientPreferences) WaveOn() bool {
	return p.WaveProgress == nil || *p.WaveProgress
}

// The accent sources: the playing cover, the provider's brand, or the
// desktop palette.
const (
	AccentArt     = "art"
	AccentSource  = "source"
	AccentDesktop = "desktop"
)

// AccentMode returns the stored accent source; missing or unknown means art.
func (p ClientPreferences) AccentMode() string {
	switch p.Accent {
	case AccentSource, AccentDesktop:
		return p.Accent
	default:
		return AccentArt
	}
}

// VisualizerStyle is how the visualizer draws the music.
type VisualizerStyle string

// The visualizer styles: spectrum blades, a spectrum curve, the waveform,
// and the stereo field.
const (
	VisualizerBars         VisualizerStyle = "bars"
	VisualizerCurve        VisualizerStyle = "curve"
	VisualizerOscilloscope VisualizerStyle = "oscilloscope"
	VisualizerVectorscope  VisualizerStyle = "vectorscope"
)

// VisualizerStyles lists the styles in their cycling order.
var VisualizerStyles = []VisualizerStyle{VisualizerBars, VisualizerCurve, VisualizerOscilloscope, VisualizerVectorscope}

// Style returns the stored visualizer style; missing or unknown means bars.
func (p ClientPreferences) Style() VisualizerStyle {
	for _, style := range VisualizerStyles {
		if string(style) == p.VisualizerStyle {
			return style
		}
	}
	return VisualizerBars
}

// The pane corner presets. CornersDesktop follows the Ryoku window corners;
// CornersCustom uses CornerDp.
const (
	CornersSquare  = "square"
	CornersFine    = "fine"
	CornersChisel  = "chisel"
	CornersDeep    = "deep"
	CornersDesktop = "desktop"
	CornersCustom  = "custom"
)

// CornerMaxDp is the deepest pane cut a listener can choose.
const CornerMaxDp = 32

// CornerPresetDp returns a fixed preset's cut in dp.
func CornerPresetDp(preset string) (int, bool) {
	switch preset {
	case CornersSquare:
		return 0, true
	case CornersFine:
		return 8, true
	case CornersChisel:
		return 16, true
	case CornersDeep:
		return 24, true
	default:
		return 0, false
	}
}

// CornerPreset returns the stored corner preset; missing or unknown means chisel.
func (p ClientPreferences) CornerPreset() string {
	switch p.Corners {
	case CornersSquare, CornersFine, CornersDeep, CornersDesktop, CornersCustom:
		return p.Corners
	default:
		return CornersChisel
	}
}

// DefaultClientPreferences returns safe defaults for malformed or absent state.
func DefaultClientPreferences() ClientPreferences {
	return ClientPreferences{
		WindowWidth: 1280, WindowHeight: 800, PanelTab: "queue", ZoomPercent: 100,
		Appearance: "follow", Density: "comfortable", Decorations: true,
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
	if preferences.WindowWidth < 480 || preferences.WindowHeight < 560 {
		preferences.WindowWidth, preferences.WindowHeight = defaults.WindowWidth, defaults.WindowHeight
	}
	if preferences.ZoomPercent < 80 || preferences.ZoomPercent > 140 || preferences.ZoomPercent%10 != 0 {
		preferences.ZoomPercent = defaults.ZoomPercent
	}
	if preferences.PanelTab != "queue" && preferences.PanelTab != "lyrics" && preferences.PanelTab != "listen-together" {
		preferences.PanelTab = defaults.PanelTab
	}
	switch preferences.Appearance {
	case "desktop":
		preferences.Appearance = "follow"
	case "follow", "dark", "light":
	default:
		preferences.Appearance = defaults.Appearance
	}
	switch preferences.Density {
	case "comfortable", "compact":
	default:
		preferences.Density = defaults.Density
	}
	preferences.CornerDp = min(max(preferences.CornerDp, 0), CornerMaxDp)
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
	s.publishLocked()
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
