package store

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/provider"
)

var rendererSettingKeys = []model.SettingKey{
	model.SettingVolume,
	model.SettingProxy,
	model.SettingQuality,
	model.SettingEnableHistory,
	model.SettingDiscordRPC,
	model.SettingCloseToTray,
	model.SettingAutostart,
	model.SettingAutoplay,
	model.SettingPreventDuplicates,
	model.SettingLyricsBoidu,
	model.SettingLyricsRomanize,
	model.SettingLowResourceMode,
	model.SettingDisabledStreamClients,
	model.SettingUIScale,
	model.SettingDiscordPresenceName,
}

// Setting returns one persisted setting without applying a default.
func (s *Store) Setting(key string) (string, bool) {
	value, ok := s.settings.Load(key)
	if !ok {
		return "", false
	}
	return value.(string), true
}

// SetSetting validates, normalizes, and persists one renderer-visible setting.
func (s *Store) SetSetting(key, value string) error {
	if err := model.ValidateSetting(key, value); err != nil {
		return err
	}
	return s.setRaw(key, normalizeSetting(key, value))
}

// Settings returns the typed settings view with effective defaults.
func (s *Store) Settings() model.Settings {
	out := model.DefaultSettings()
	if value, ok := s.validSetting(model.SettingVolume); ok {
		out.Volume, _ = strconv.Atoi(value)
	}
	if value, ok := s.validSetting(model.SettingProxy); ok {
		out.Proxy = value
	}
	if value, ok := s.validSetting(model.SettingQuality); ok {
		out.Quality = model.Quality(value)
	}
	out.EnableHistory = s.boolSetting(model.SettingEnableHistory, out.EnableHistory)
	out.DiscordRPC = s.boolSetting(model.SettingDiscordRPC, out.DiscordRPC)
	out.CloseToTray = s.boolSetting(model.SettingCloseToTray, out.CloseToTray)
	out.Autostart = s.boolSetting(model.SettingAutostart, out.Autostart)
	out.Autoplay = s.boolSetting(model.SettingAutoplay, out.Autoplay)
	out.PreventDuplicates = s.boolSetting(model.SettingPreventDuplicates, out.PreventDuplicates)
	out.LyricsBoidu = s.boolSetting(model.SettingLyricsBoidu, out.LyricsBoidu)
	out.LyricsRomanize = s.boolSetting(model.SettingLyricsRomanize, out.LyricsRomanize)
	out.LowResourceMode = s.boolSetting(model.SettingLowResourceMode, out.LowResourceMode)
	if value, ok := s.validSetting(model.SettingDisabledStreamClients); ok {
		for _, name := range strings.Split(value, ",") {
			if name != "" {
				out.DisabledStreamClients = append(out.DisabledStreamClients, model.StreamClient(name))
			}
		}
	}
	if value, ok := s.validSetting(model.SettingUIScale); ok {
		out.UIScale, _ = strconv.Atoi(value)
	}
	if value, ok := s.validSetting(model.SettingDiscordPresenceName); ok {
		out.DiscordPresenceName = value
	}
	return out
}

// SettingsSnapshot returns only persisted renderer-visible string settings.
func (s *Store) SettingsSnapshot() model.SettingsSnapshot {
	out := make(model.SettingsSnapshot)
	for _, key := range rendererSettingKeys {
		if value, ok := s.Setting(string(key)); ok {
			out[string(key)] = value
		}
	}
	return out
}

// KV returns the unvalidated key-value view used by providers and daemon state.
func (s *Store) KV() provider.KV {
	return storeKV{s}
}

func (s *Store) boolSetting(key model.SettingKey, fallback bool) bool {
	if value, ok := s.validSetting(key); ok {
		return value == "true"
	}
	return fallback
}

func (s *Store) validSetting(key model.SettingKey) (string, bool) {
	value, ok := s.Setting(string(key))
	if !ok || model.ValidateSetting(string(key), value) != nil {
		return "", false
	}
	return value, true
}

func normalizeSetting(key, value string) string {
	switch key {
	case string(model.SettingProxy), string(model.SettingDiscordPresenceName):
		return strings.TrimSpace(value)
	case string(model.SettingDisabledStreamClients):
		seen := make(map[string]struct{})
		for _, name := range strings.Split(value, ",") {
			name = strings.TrimSpace(name)
			if name != "" {
				seen[name] = struct{}{}
			}
		}
		names := make([]string, 0, len(seen))
		for name := range seen {
			names = append(names, name)
		}
		sort.Strings(names)
		return strings.Join(names, ",")
	default:
		return value
	}
}

func (s *Store) setRaw(key, value string) error {
	s.settingsWrite.Lock()
	defer s.settingsWrite.Unlock()
	if _, err := s.db.Exec(`INSERT INTO settings(key,value) VALUES(?,?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value); err != nil {
		return fmt.Errorf("persist setting %q: %w", key, err)
	}
	s.settings.Store(key, value)
	return nil
}

func (s *Store) deleteRaw(key string) error {
	s.settingsWrite.Lock()
	defer s.settingsWrite.Unlock()
	if _, err := s.db.Exec("DELETE FROM settings WHERE key=?", key); err != nil {
		return fmt.Errorf("delete setting %q: %w", key, err)
	}
	s.settings.Delete(key)
	return nil
}

type storeKV struct{ store *Store }

func (kv storeKV) Get(key string) (string, bool) { return kv.store.Setting(key) }
func (kv storeKV) Set(key, value string) error   { return kv.store.setRaw(key, value) }
func (kv storeKV) Delete(key string) error       { return kv.store.deleteRaw(key) }
