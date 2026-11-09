package daemon

import (
	"errors"
	"net/url"
	"strings"

	"github.com/ryoku-dev/ryotunes/internal/provider"
	"github.com/ryoku-dev/ryotunes/internal/store"
)

// userMessage turns package-level typed failures into stable renderer copy.
func userMessage(err error) string {
	message, _ := userMessageMapping(err)
	return message
}

func userMessageMapping(err error) (string, bool) {
	if err == nil {
		return "", false
	}
	switch {
	case errors.Is(err, provider.ErrNotPremium):
		return "the account has no Spotify Premium", false
	case errors.Is(err, provider.ErrSignInRequired):
		message := strings.ToLower(err.Error())
		switch {
		case strings.Contains(message, "spotify"):
			return "Sign in to Spotify", false
		case strings.Contains(message, "soundcloud"):
			return "Sign in to SoundCloud", false
		case strings.Contains(message, "tidal"):
			return "Sign in to TIDAL", false
		default:
			return "Sign in to YouTube Music", false
		}
	case errors.Is(err, provider.ErrUnavailable):
		return "This item is unavailable.", false
	case errors.Is(err, provider.ErrBotGated):
		return "YouTube Music could not confirm this playback session. Try again.", false
	case errors.Is(err, provider.ErrNotFound):
		return "That item could not be found.", false
	case errors.Is(err, provider.ErrUnsupported):
		return "This action is not available for this provider.", false
	case errors.Is(err, store.ErrPersonalInvalid):
		return "Personal data must be a JSON object.", false
	case errors.Is(err, store.ErrPersonalTooLarge):
		return "Personal data must be no larger than 1 MiB.", false
	}
	var requestError *url.Error
	if errors.As(err, &requestError) {
		service := requestService(requestError.URL)
		if requestError.Timeout() {
			return service + " didn't answer in time. Try again.", true
		}
		return "Couldn't reach " + service + ". Check your connection and try again.", true
	}
	return err.Error(), true
}

func requestService(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "the server"
	}
	host := strings.ToLower(parsed.Hostname())
	switch {
	case host == "youtube.com" || host == "youtu.be" || strings.HasSuffix(host, ".youtube.com"):
		return "YouTube Music"
	case host == "spotify.com" || strings.HasSuffix(host, ".spotify.com"):
		return "Spotify"
	case host == "soundcloud.com" || strings.HasSuffix(host, ".soundcloud.com"):
		return "SoundCloud"
	case host == "tidal.com" || strings.HasSuffix(host, ".tidal.com"):
		return "TIDAL"
	default:
		return "the server"
	}
}
