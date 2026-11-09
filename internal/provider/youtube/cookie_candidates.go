package youtube

import (
	"sort"
	"strings"
	"time"

	"github.com/ryoku-dev/ryotunes/internal/provider/browsercookies"
	"golang.org/x/net/http/httpguts"
)

type cookieCandidate struct {
	store     browsercookies.Store
	container string
	header    string
}

func buildYouTubeCookieCandidates(source CookieSource, stores []browsercookies.Store, now time.Time) []cookieCandidate {
	var candidates []cookieCandidate
	for _, store := range stores {
		cookies, err := source.Read(store, "youtube.com")
		if err != nil {
			continue
		}
		byContainer := make(map[string][]browsercookies.Cookie)
		for _, cookie := range cookies {
			if youtubeCookieEligible(cookie, now) {
				byContainer[cookie.Container] = append(byContainer[cookie.Container], cookie)
			}
		}
		containers := make([]string, 0, len(byContainer))
		for container := range byContainer {
			containers = append(containers, container)
		}
		sort.Slice(containers, func(i, j int) bool {
			if containers[i] == "" {
				return containers[j] != ""
			}
			if containers[j] == "" {
				return false
			}
			return containers[i] < containers[j]
		})
		for _, container := range containers {
			if header := youtubeCookieHeader(byContainer[container]); sapisidCookie(header) != "" {
				candidates = append(candidates, cookieCandidate{store: store, container: container, header: header})
			}
		}
	}
	return candidates
}

func youtubeCookieEligible(cookie browsercookies.Cookie, now time.Time) bool {
	if cookie.Partitioned || !cookie.Expires.IsZero() && !cookie.Expires.After(now) {
		return false
	}
	switch strings.ToLower(cookie.Host) {
	case ".youtube.com", ".music.youtube.com", "music.youtube.com":
	default:
		return false
	}
	if cookie.Path != "" && cookie.Path != "/" && !strings.HasPrefix("/youtubei/", cookie.Path) {
		return false
	}
	if !httpguts.ValidHeaderFieldName(cookie.Name) {
		return false
	}
	pair := cookie.Name + "=" + cookie.Value
	return !strings.Contains(pair, ";") && httpguts.ValidHeaderFieldValue(pair)
}

func youtubeCookieHeader(cookies []browsercookies.Cookie) string {
	type selectedCookie struct {
		cookie          browsercookies.Cookie
		hostSpecificity int
	}
	selected := make(map[string]selectedCookie)
	for _, cookie := range cookies {
		specificity := len(strings.TrimPrefix(strings.ToLower(cookie.Host), "."))
		old, exists := selected[cookie.Name]
		if exists && (specificity < old.hostSpecificity || specificity == old.hostSpecificity && len(cookie.Path) <= len(old.cookie.Path)) {
			continue
		}
		selected[cookie.Name] = selectedCookie{cookie: cookie, hostSpecificity: specificity}
	}
	names := make([]string, 0, len(selected))
	for name := range selected {
		names = append(names, name)
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, name := range names {
		parts = append(parts, name+"="+selected[name].cookie.Value)
	}
	return strings.Join(parts, "; ")
}
