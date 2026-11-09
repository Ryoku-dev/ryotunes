package shell

import (
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/pages/album"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/pages/artist"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/pages/diagnostics"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/pages/downloads"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/pages/home"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/pages/library"
	listpage "github.com/ryoku-dev/ryotunes/internal/client/ui/pages/list"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/pages/playlist"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/pages/radio"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/pages/search"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/pages/settings"
)

func (s *uiShell) page(route env.Route) env.Page {
	key := routeKey(route)
	if page := s.pages[key]; page != nil {
		return page
	}
	var page env.Page
	switch route.Kind {
	case env.RouteHome:
		page = home.New(route)
	case env.RouteSearch:
		page = search.New(route)
	case env.RouteLibrary:
		page = library.New(route)
	case env.RouteRadio:
		page = radio.New(route)
	case env.RouteDownloads:
		page = downloads.New(route)
	case env.RouteSettings:
		page = settings.New(route)
	case env.RouteDiagnostics:
		page = diagnostics.New(route)
	case env.RouteArtist:
		page = artist.New(route)
	case env.RouteAlbum:
		page = album.New(route)
	case env.RoutePlaylist:
		page = playlist.New(route)
	case env.RouteList:
		page = listpage.New(route)
	default:
		page = home.New(env.Route{Kind: env.RouteHome})
	}
	s.pages[key] = page
	return page
}
