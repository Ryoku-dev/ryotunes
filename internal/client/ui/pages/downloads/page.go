// Package downloads implements the event-driven downloads queue and history page.
package downloads

import (
	"gioui.org/layout"
	"gioui.org/widget"

	"github.com/ryoku-dev/ryotunes/internal/client/lithic"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
)

// Page is the retained downloads queue and history surface.
type Page struct {
	route env.Route
	list  widget.List

	rows        map[string]*downloadRow
	collections map[string]*collectionCard

	openFolder lithic.Button
	clear      lithic.Button
	settings   lithic.Button
}

// New constructs a downloads page backed entirely by the live client store.
func New(r env.Route) env.Page {
	return &Page{
		route:       r,
		list:        widget.List{List: layout.List{Axis: layout.Vertical}},
		rows:        make(map[string]*downloadRow),
		collections: make(map[string]*collectionCard),
	}
}

// Title returns the page title.
func (p *Page) Title() string { return "Downloads" }
