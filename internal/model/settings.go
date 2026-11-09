package model

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"
)

// SettingKey is a renderer-visible string setting key.
type SettingKey string

const (
	// SettingVolume is the persisted volume percent.
	SettingVolume SettingKey = "volume"
	// SettingProxy is the optional HTTP(S) proxy.
	SettingProxy SettingKey = "proxy"
	// SettingQuality is the stream quality policy.
	SettingQuality SettingKey = "quality"
	// SettingEnableHistory enables YouTube watch-history registration.
	SettingEnableHistory SettingKey = "enable_history"
	// SettingDiscordRPC enables Discord Rich Presence.
	SettingDiscordRPC SettingKey = "discord_rpc"
	// SettingCloseToTray keeps playback resident when the window closes.
	SettingCloseToTray SettingKey = "close_to_tray"
	// SettingAutostart records the desktop autostart preference.
	SettingAutostart SettingKey = "autostart"
	// SettingAutoplay enables queue radio extension.
	SettingAutoplay SettingKey = "autoplay"
	// SettingPreventDuplicates enables queue duplicate prevention.
	SettingPreventDuplicates SettingKey = "prevent_duplicates"
	// SettingLyricsBoidu enables the Boidu lyrics source.
	SettingLyricsBoidu SettingKey = "lyrics_boidu"
	// SettingLyricsRomanize enables client lyric readings.
	SettingLyricsRomanize SettingKey = "lyrics_romanize"
	// SettingLowResourceMode enables reduced daemon and client cadence.
	SettingLowResourceMode SettingKey = "low_resource_mode"
	// SettingDisabledStreamClients is the comma-separated disabled-client list.
	SettingDisabledStreamClients SettingKey = "disabled_stream_clients"
	// SettingUIScale is the interface scale percentage.
	SettingUIScale SettingKey = "ui_scale"
	// SettingDiscordPresenceName is the Discord activity title.
	SettingDiscordPresenceName SettingKey = "discord_presence_name"
)

// Settings is the daemon's typed view of every renderer-visible setting.
type Settings struct {
	Volume                int            `json:"volume"`
	Proxy                 string         `json:"proxy"`
	Quality               Quality        `json:"quality"`
	EnableHistory         bool           `json:"enable_history"`
	DiscordRPC            bool           `json:"discord_rpc"`
	CloseToTray           bool           `json:"close_to_tray"`
	Autostart             bool           `json:"autostart"`
	Autoplay              bool           `json:"autoplay"`
	PreventDuplicates     bool           `json:"prevent_duplicates"`
	LyricsBoidu           bool           `json:"lyrics_boidu"`
	LyricsRomanize        bool           `json:"lyrics_romanize"`
	LowResourceMode       bool           `json:"low_resource_mode"`
	DisabledStreamClients []StreamClient `json:"disabled_stream_clients"`
	UIScale               int            `json:"ui_scale"`
	DiscordPresenceName   string         `json:"discord_presence_name"`
}

// SettingsSnapshot preserves the legacy string-to-string settings wire.
type SettingsSnapshot map[string]string

// Personal is an opaque renderer-owned JSON object; the daemon validates and stores it but never interprets it.
type Personal = json.RawMessage

// StreamClient identifies one YouTube player client used for stream resolution.
type StreamClient string

const (
	// StreamClientWebRemix is the primary metadata and stream client.
	StreamClientWebRemix StreamClient = "WEB_REMIX"
	// StreamClientVisionOS is the first ordinary stream fallback.
	StreamClientVisionOS StreamClient = "VISIONOS"
	// StreamClientAndroidVR165 is the newer Android VR fallback.
	StreamClientAndroidVR165 StreamClient = "ANDROID_VR_1_65_10"
	// StreamClientAndroidVR143 is the older Android VR fallback.
	StreamClientAndroidVR143 StreamClient = "ANDROID_VR_1_43_32"
	// StreamClientTVHTML5 is the first upload fallback.
	StreamClientTVHTML5 StreamClient = "TVHTML5"
	// StreamClientWebCreator is the authenticated upload fallback.
	StreamClientWebCreator StreamClient = "WEB_CREATOR"
)

// KnownStreamClients lists stream clients in resolver order.
var KnownStreamClients = []StreamClient{
	StreamClientWebRemix,
	StreamClientVisionOS,
	StreamClientAndroidVR165,
	StreamClientAndroidVR143,
	StreamClientTVHTML5,
	StreamClientWebCreator,
}

// SettingError is a user-visible setting validation error.
type SettingError struct {
	Message string
}

// Error returns the exact legacy validation sentence.
func (e *SettingError) Error() string {
	return e.Message
}

// DefaultSettings returns the effective values used when settings are absent.
func DefaultSettings() Settings {
	return Settings{
		Volume:              100,
		Quality:             QualityHigh,
		EnableHistory:       true,
		Autoplay:            true,
		UIScale:             100,
		DiscordPresenceName: "Ryotunes",
	}
}

// ValidateSetting validates one legacy string setting and returns its exact user-visible error.
func ValidateSetting(key, value string) error {
	switch key {
	case "volume":
		n, err := strconv.Atoi(value)
		if err != nil {
			return settingError("Volume must be a number from 0 to 100.")
		}
		if n < 0 || n > 100 {
			return settingError("Volume must be between 0 and 100.")
		}
	case "proxy":
		return validateProxy(value)
	case "quality":
		if value != string(QualityLow) && value != string(QualityAuto) && value != string(QualityHigh) {
			return settingError("Unknown audio quality.")
		}
	case "enable_history", "discord_rpc", "close_to_tray", "autostart", "autoplay",
		"prevent_duplicates", "lyrics_boidu", "lyrics_romanize", "low_resource_mode":
		if value != "true" && value != "false" {
			return settingError(fmt.Sprintf("%s must be true or false.", key))
		}
	case "disabled_stream_clients":
		known := make(map[string]struct{}, len(KnownStreamClients))
		for _, client := range KnownStreamClients {
			known[string(client)] = struct{}{}
		}
		for _, name := range strings.Split(value, ",") {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			if _, ok := known[name]; !ok {
				return settingError("Unknown stream client in disabled-client list.")
			}
		}
	case "ui_scale":
		n, err := strconv.Atoi(value)
		if err != nil {
			return settingError("Interface scale must be a percentage.")
		}
		if n < 80 || n > 140 || n%10 != 0 {
			return settingError("Interface scale must be 80–140% in 10% steps.")
		}
	case "discord_presence_name":
		name := strings.TrimSpace(value)
		if name != "" {
			length := utf8.RuneCountInString(name)
			if length < 2 || length > 128 {
				return settingError("Discord presence title must be between 2 and 128 characters.")
			}
		}
	default:
		return settingError(fmt.Sprintf("unknown setting: %s", key))
	}
	return nil
}

func validateProxy(value string) error {
	raw := strings.TrimSpace(value)
	if raw == "" {
		return nil
	}
	if len(raw) > 2048 {
		return settingError("Proxy URL is too long.")
	}
	if !strings.Contains(raw, "://") {
		return settingError("Enter a valid HTTP or HTTPS proxy URL.")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return settingError("Enter a valid HTTP or HTTPS proxy URL.")
	}
	if (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" {
		return settingError("Proxy must use http:// or https:// and include a host.")
	}
	if parsed.User != nil {
		return settingError("Authenticated proxy URLs are not supported in renderer-visible settings.")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return settingError("Proxy URL must contain only scheme, host and port.")
	}
	return nil
}

func settingError(message string) error {
	return &SettingError{Message: message}
}
