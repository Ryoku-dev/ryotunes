// Package sheet holds Ryotunes' own parts of the Hub's sheet, built on the
// Lithic kit's sheet parts (kit.Masthead, kit.TickRuler, kit.RegistrationSheet,
// kit.SectionStyle, kit.Folio, kit.SpacedCaps, kit.Clock): the numbering and
// glosses of Ryotunes' places, the page masthead adapter, the colophon with
// the page's code in bars and Ryoku's motto, the tategaki, the spec strip and
// the page's reveal groups.
package sheet

import (
	"strconv"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
)

// Place is one numbered place of Ryotunes' sheet, as the Hub numbers its
// pages (registry/numbering.js): its id (the colophon's code), its number,
// its name in sentence case (set in spaced capitals where the sheet shows
// it) and its kanji gloss.
type Place struct {
	ID     string
	Number string
	Name   string
	Gloss  string
	// order is the place's position on the sheet, from 1, for the
	// masthead's page place
	order int
}

// The places, in the sheet's order. Single-page places carry only their
// number, as the Hub's single-page categories do; Settings' sections are
// numbered under it (SettingsNumber). The equalizer, a tool reached from
// Sound, and Diagnostics follow Now playing, so no two places share a number.
var (
	Home        = Place{ID: "home", Number: "00", Name: "Home", Gloss: "表紙", order: 1}
	Library     = Place{ID: "library", Number: "01", Name: "Library", Gloss: "書庫", order: 2}
	Radio       = Place{ID: "radio", Number: "02", Name: "Radio", Gloss: "放送", order: 3}
	Downloads   = Place{ID: "downloads", Number: "03", Name: "Downloads", Gloss: "保存", order: 4}
	Playlists   = Place{ID: "playlists", Number: "04", Name: "Playlists", Gloss: "曲集", order: 5}
	Settings    = Place{ID: "settings", Number: "05", Name: "Settings", Gloss: "設定", order: 6}
	Search      = Place{ID: "search", Number: "06", Name: "Search", Gloss: "検索", order: 7}
	NowPlaying  = Place{ID: "now-playing", Number: "07", Name: "Now playing", Gloss: "再生", order: 8}
	Equalizer   = Place{ID: "equalizer", Number: "08", Name: "Equalizer", Gloss: "調音", order: 9}
	Diagnostics = Place{ID: "diagnostics", Number: "09", Name: "Diagnostics", order: 10}
)

// Places lists every place in the sheet's order.
var Places = []Place{Home, Library, Radio, Downloads, Playlists, Settings, Search, NowPlaying, Equalizer, Diagnostics}

// pagePlaces holds "P 01/10" .. so a masthead at rest builds no string.
var pagePlaces = func() []string {
	out := make([]string, len(Places)+1)
	for at := 1; at <= len(Places); at++ {
		out[at] = "P " + kit.Pad2(at) + "/" + kit.Pad2(len(Places))
	}
	return out
}()

// PagePlace is the place's position on the sheet as the masthead writes it,
// "P 02/10".
func (p Place) PagePlace() string {
	if p.order <= 0 || p.order >= len(pagePlaces) {
		return ""
	}
	return pagePlaces[p.order]
}

// GlossIf is the place's gloss while glosses are on (env.Env.Gloss), else
// empty.
func (p Place) GlossIf(on bool) string {
	if !on {
		return ""
	}
	return p.Gloss
}

// ForRoute is the place a route belongs to and the part of it the route
// shows (sentence case, empty for the place itself): an album, an artist or
// a browse list sits in the Library, a playlist among the Playlists.
func ForRoute(kind env.RouteKind) (place Place, part string) {
	switch kind {
	case env.RouteHome:
		return Home, ""
	case env.RouteLibrary:
		return Library, ""
	case env.RouteAlbum:
		return Library, "Album"
	case env.RouteArtist:
		return Library, "Artist"
	case env.RouteList:
		return Library, "Browse"
	case env.RouteRadio:
		return Radio, ""
	case env.RouteDownloads:
		return Downloads, ""
	case env.RoutePlaylist:
		return Playlists, "Playlist"
	case env.RouteSettings:
		return Settings, ""
	case env.RouteSearch:
		return Search, ""
	case env.RouteEqualizer:
		return Equalizer, ""
	case env.RouteDiagnostics:
		return Diagnostics, ""
	default:
		return Home, ""
	}
}

// settingsNumbers holds "05.1" .. "05.32".
var settingsNumbers = func() []string {
	out := make([]string, 33)
	for n := 1; n < len(out); n++ {
		out[n] = Settings.Number + "." + strconv.Itoa(n)
	}
	return out
}()

// SettingsNumber is the number of Settings' nth section, from 1: "05.4".
func SettingsNumber(n int) string {
	if n > 0 && n < len(settingsNumbers) {
		return settingsNumbers[n]
	}
	return Settings.Number + "." + strconv.Itoa(n)
}

// sectionGlosses are the Hub's own words for the Settings sections it has
// too (registry.js): sounds, appearance and about.
var sectionGlosses = map[string]string{
	"sound":      "音",
	"appearance": "外観",
	"about":      "概要",
}

// SectionGloss is the gloss of a Settings section by its id ("sound",
// "appearance", "about") while glosses are on; a section with no Hub word has
// none.
func SectionGloss(id string, on bool) string {
	if !on {
		return ""
	}
	return sectionGlosses[id]
}

// Motto is Ryoku's motto, "for strength and beauty", set at the sheet's foot
// while glosses are on.
const Motto = "力と美のために"

// JoinGloss joins a place's gloss and its part's with the Hub's middle dot,
// "設定・音"; either may be empty.
func JoinGloss(place, part string) string {
	switch {
	case place == "":
		return part
	case part == "":
		return place
	default:
		return place + "・" + part
	}
}
