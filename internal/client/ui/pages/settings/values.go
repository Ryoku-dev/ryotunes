package settings

import (
	"strconv"
	"strings"

	"github.com/ryoku-dev/ryotunes/internal/model"
)

func settingString(settings model.SettingsSnapshot, key model.SettingKey, fallback string) string {
	if value, ok := settings[string(key)]; ok {
		return value
	}
	return fallback
}

func settingBool(settings model.SettingsSnapshot, key model.SettingKey, fallback bool) bool {
	value, ok := settings[string(key)]
	if !ok {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func settingInt(settings model.SettingsSnapshot, key model.SettingKey, fallback int) int {
	value, ok := settings[string(key)]
	if !ok {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func settingSet(settings model.SettingsSnapshot, key model.SettingKey) map[string]bool {
	set := make(map[string]bool)
	for _, value := range strings.Split(settings[string(key)], ",") {
		value = strings.TrimSpace(value)
		if value != "" {
			set[value] = true
		}
	}
	return set
}

func boolString(value bool) string { return strconv.FormatBool(value) }

func indexOf(values []string, value string, fallback int) int {
	for i, candidate := range values {
		if candidate == value {
			return i
		}
	}
	return fallback
}
