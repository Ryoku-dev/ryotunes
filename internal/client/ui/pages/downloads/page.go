// Package downloads implements the event-driven downloads queue and history page.
package downloads

import (
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/pages/pagesheet"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/sheet"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/widgets"
)

// Page is the retained downloads queue and history surface.
type Page struct {
	route       env.Route
	scroll      look.Scroller
	header      widgets.Header
	filter      kit.Filters
	filterIndex int
	rows        map[string]*downloadRow
	collections map[string]*collectionCard

	openFolder kit.Plaque
	clear      kit.Plaque
	settings   kit.Plaque

	usedRevision int64
	usedBytes    int64

	reveal sheet.Reveal
	strip  pagesheet.Strip
}

// New constructs a downloads page backed entirely by the live client store.
func New(r env.Route) env.Page {
	return &Page{
		route:       r,
		rows:        make(map[string]*downloadRow),
		collections: make(map[string]*collectionCard),
	}
}

// Title returns the page title.
func (p *Page) Title() string { return "Downloads" }
