package daemon

import (
	"errors"
	"strings"

	"github.com/ryoku-dev/ryotunes/internal/provider"
	"github.com/ryoku-dev/ryotunes/internal/store"
)

// userMessage turns package-level typed failures into stable renderer copy.
func userMessage(err error) string {
	if err == nil {
		return ""
	}
	switch {
	case errors.Is(err, provider.ErrNotPremium):
		return "the account has no Spotify Premium"
	case errors.Is(err, provider.ErrSignInRequired):
		message := strings.ToLower(err.Error())
		switch {
		case strings.Contains(message, "spotify"):
			return "Sign in to Spotify"
		case strings.Contains(message, "soundcloud"):
			return "Sign in to SoundCloud"
		case strings.Contains(message, "tidal"):
			return "Sign in to TIDAL"
		default:
			return "Sign in to YouTube Music"
		}
	case errors.Is(err, provider.ErrUnavailable):
		return "This item is unavailable."
	case errors.Is(err, provider.ErrBotGated):
		return "YouTube Music could not confirm this playback session. Try again."
	case errors.Is(err, provider.ErrNotFound):
		return "That item could not be found."
	case errors.Is(err, provider.ErrUnsupported):
		return "This action is not available for this provider."
	case errors.Is(err, store.ErrPersonalInvalid):
		return "Personal data must be a JSON object."
	case errors.Is(err, store.ErrPersonalTooLarge):
		return "Personal data must be no larger than 1 MiB."
	default:
		return err.Error()
	}
}
