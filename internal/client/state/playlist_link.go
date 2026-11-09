package state

import (
	"net/url"
	"regexp"
	"strings"
)

var (
	playlistIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
	hostPattern       = regexp.MustCompile(`^[a-z0-9.-]+$`)
	schemePattern     = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9+.-]*):`)
)

var playlistHosts = map[string]struct{}{
	"youtube.com": {}, "www.youtube.com": {}, "m.youtube.com": {},
	"music.youtube.com": {}, "youtu.be": {},
}

// PlaylistLink is the pure parse result for a pasted playlist link.
type PlaylistLink struct {
	OK      bool
	ID      string
	RawID   string
	Reason  string
	Message string
}

// ParsePlaylistLink resolves a YouTube share URL without fetching it.
func ParsePlaylistLink(input string) PlaylistLink {
	text := strings.TrimSpace(input)
	if text == "" {
		return playlistLinkFailure("empty", "")
	}
	if len(text) > 4096 {
		return playlistLinkFailure("invalid", "That link is too long.")
	}

	rest := text
	if match := schemePattern.FindStringSubmatch(text); match != nil {
		scheme := strings.ToLower(match[1])
		afterColon := text[len(match[0]):]
		bareHostPort := strings.Contains(scheme, ".") && leadingDecimal(afterColon)
		if !bareHostPort {
			if scheme != "http" && scheme != "https" {
				return playlistLinkFailure("scheme", "Only http and https links are supported.")
			}
			if !strings.HasPrefix(afterColon, "//") {
				return playlistLinkFailure("invalid", "That does not look like a link.")
			}
			rest = afterColon[2:]
		}
	}

	cut := strings.IndexAny(rest, "/?#")
	authority, tail := rest, ""
	if cut >= 0 {
		authority, tail = rest[:cut], rest[cut:]
	}
	if strings.Contains(authority, "@") {
		return playlistLinkFailure("invalid", "Paste a public share link without login details.")
	}
	host := strings.ToLower(authority)
	if colon := strings.LastIndexByte(host, ':'); colon >= 0 {
		port := host[colon+1:]
		if port == "" || strings.Trim(port, "0123456789") != "" {
			return playlistLinkFailure("invalid", "That does not look like a link.")
		}
		host = host[:colon]
	}
	if host == "" || !hostPattern.MatchString(host) {
		return playlistLinkFailure("invalid", "That does not look like a link.")
	}
	if _, accepted := playlistHosts[host]; !accepted {
		return playlistLinkFailure("host", "Only YouTube and YouTube Music links are supported.")
	}

	if hash := strings.IndexByte(tail, '#'); hash >= 0 {
		tail = tail[:hash]
	}
	path, query := tail, ""
	if question := strings.IndexByte(tail, '?'); question >= 0 {
		path, query = tail[:question], tail[question+1:]
	}
	params := parsePlaylistQuery(query)
	segments := splitPath(path)

	var raw string
	if lists := params["list"]; len(lists) != 0 {
		distinct := make(map[string]struct{}, len(lists))
		for _, list := range lists {
			distinct[list] = struct{}{}
		}
		if len(distinct) > 1 {
			return playlistLinkFailure("ambiguous", "This link points at more than one playlist.")
		}
		raw = lists[0]
	} else if len(segments) > 1 && segments[0] == "browse" {
		browseID := segments[1]
		switch {
		case strings.HasPrefix(browseID, "VL"):
			raw = browseID
		case strings.HasPrefix(browseID, "MPRE"):
			return playlistLinkFailure("no-playlist", "That link is an album, not a playlist.")
		case strings.HasPrefix(browseID, "UC"):
			return playlistLinkFailure("no-playlist", "That link is an artist, not a playlist.")
		default:
			return playlistLinkFailure("no-playlist", "That link is not a playlist.")
		}
	} else {
		return playlistLinkFailure("no-playlist", "That link has no playlist, paste a playlist or a “Share playlist” link.")
	}

	raw = strings.TrimSpace(raw)
	rawID := strings.TrimPrefix(raw, "VL")
	if rawID == "" || len(rawID) > 256 || !playlistIDPattern.MatchString(rawID) {
		return playlistLinkFailure("bad-id", "That does not look like a valid playlist id.")
	}
	return PlaylistLink{OK: true, ID: "VL" + rawID, RawID: rawID}
}

func parsePlaylistQuery(query string) map[string][]string {
	out := make(map[string][]string)
	for _, part := range strings.Split(query, "&") {
		if part == "" {
			continue
		}
		key, value, found := strings.Cut(part, "=")
		if !found {
			value = ""
		}
		key = tolerantQueryUnescape(key)
		out[key] = append(out[key], tolerantQueryUnescape(value))
	}
	return out
}

func tolerantQueryUnescape(value string) string {
	decoded, err := url.QueryUnescape(value)
	if err != nil {
		return strings.ReplaceAll(value, "+", " ")
	}
	return decoded
}

func splitPath(path string) []string {
	parts := strings.Split(strings.TrimLeft(path, "/"), "/")
	out := parts[:0]
	for _, part := range parts {
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func leadingDecimal(value string) bool {
	if value == "" || value[0] < '0' || value[0] > '9' {
		return false
	}
	for i := 1; i < len(value); i++ {
		if value[i] == '/' || value[i] == '?' || value[i] == '#' {
			return true
		}
		if value[i] < '0' || value[i] > '9' {
			return false
		}
	}
	return true
}

func playlistLinkFailure(reason, message string) PlaylistLink {
	return PlaylistLink{Reason: reason, Message: message}
}
