package listentogether

import (
	"errors"
	"net/url"
	"strings"
)

const (
	// DefaultServerURL is intentionally empty until the user chooses a relay.
	DefaultServerURL  = ""
	maxServerURLBytes = 2048
)

var (
	// ErrServerURLTooLong reports a relay URL beyond the settings limit.
	ErrServerURLTooLong = errors.New("Listen Together server URL is too long.")
	// ErrServerURLInvalid reports malformed URL syntax.
	ErrServerURLInvalid = errors.New("Enter a valid ws:// or wss:// server URL.")
	// ErrServerURLScheme reports a missing host or unsupported scheme.
	ErrServerURLScheme = errors.New("Listen Together server must use ws:// or wss:// and include a host.")
	// ErrServerURLCredentials reports credentials or a fragment in a relay URL.
	ErrServerURLCredentials = errors.New("Listen Together server URLs cannot contain credentials or fragments.")
)

// ValidateServerURL trims and validates a user-configured relay URL.
func ValidateServerURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if len(raw) > maxServerURLBytes {
		return "", ErrServerURLTooLong
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", ErrServerURLInvalid
	}
	if parsed.Scheme == "" {
		return "", ErrServerURLInvalid
	}
	if (parsed.Scheme != "ws" && parsed.Scheme != "wss") || parsed.Hostname() == "" {
		return "", ErrServerURLScheme
	}
	hasCredentials := false
	if parsed.User != nil {
		_, hasPassword := parsed.User.Password()
		hasCredentials = parsed.User.Username() != "" || hasPassword
	}
	if hasCredentials || parsed.Fragment != "" {
		return "", ErrServerURLCredentials
	}
	if parsed.Path == "" {
		parsed.Path = "/"
	}
	return parsed.String(), nil
}
