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

// intOr is value read as an integer, fallback when it does not parse.
func intOr(value string, fallback int) int {
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

// listSet is a comma separated list as a set.
func listSet(list string) map[string]bool {
	set := make(map[string]bool)
	for _, value := range strings.Split(list, ",") {
		value = strings.TrimSpace(value)
		if value != "" {
			set[value] = true
		}
	}
	return set
}

func boolString(value bool) string { return strconv.FormatBool(value) }

func intString(value int) string { return strconv.Itoa(value) }

func indexOf(values []string, value string, fallback int) int {
	for i, candidate := range values {
		if candidate == value {
			return i
		}
	}
	return fallback
}
