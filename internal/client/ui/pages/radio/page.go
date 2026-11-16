// Package radio implements the internet radio catalogue page.
package radio

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryotunes/internal/client/state"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/pages/pagesheet"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/sheet"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/widgets"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

const pageSize = 36

var radioFilters = []string{"Popular", "Favourites", "Recent", "By country", "By genre"}

// Page is the retained internet-radio directory surface.
type Page struct {
	scroll   look.Scroller
	header   widgets.Header
	filter   kit.TextField
	filters  kit.Filters
	tiles    map[string]*stationTile
	retry    kit.Plaque
	moreTry  kit.Plaque
	saver    radioSaver
	selected int
	stations []model.RadioStation
	error    string
	moreErr  string
	loaded   bool
	loading  bool
	more     bool
	hasMore  bool
	request  uint64
	reveal   sheet.Reveal
	strip    pagesheet.Strip
	moreLine pagesheet.More
}

// New constructs a retained radio page without fetching the directory.
func New(r env.Route) env.Page {
	p := &Page{tiles: make(map[string]*stationTile)}
	p.filter.SingleLine = true
	p.filter.Label = "Filter stations"
	p.filter.Placeholder = "Station name"
	if query := normalizedQuery(r.Query); query != "" {
		p.filter.SetText(query)
	}
	return p
}

func (p *Page) Title() string { return "Radio" }

func (p *Page) Refresh(e *env.Env) { p.load(e) }

func normalizedQuery(value string) string { return strings.Join(strings.Fields(value), " ") }

func (p *Page) ensureLoaded(e *env.Env) {
	if !p.loaded && !p.loading {
		p.load(e)
	}
}

func (p *Page) load(e *env.Env) {
	p.request++
	request := p.request
	p.error = ""
	p.moreErr = ""
	p.loaded = false
	p.loading = true
	p.more = false
	p.hasMore = false
	p.stations = nil
	offset, limit := 0, pageSize
	act.Call[[]model.RadioStation](e, protocol.MethodRadioStations, protocol.RadioStationsParams{Offset: &offset, Limit: &limit}, func(stations []model.RadioStation, err error) {
		if request != p.request {
			return
		}
		p.loading = false
		p.loaded = true
		if err != nil {
			p.error = err.Error()
			return
		}
		p.stations = dedupeStations(stations)
		p.hasMore = len(stations) == pageSize
	})
}

func (p *Page) loadMore(e *env.Env) {
	if p.loading || p.more || !p.hasMore {
		return
	}
	p.more = true
	p.moreErr = ""
	request := p.request
	offset, limit := len(p.stations), pageSize
	act.Call[[]model.RadioStation](e, protocol.MethodRadioStations, protocol.RadioStationsParams{Offset: &offset, Limit: &limit}, func(stations []model.RadioStation, err error) {
		if request != p.request {
			return
		}
		p.more = false
		if err != nil {
			p.moreErr = err.Error()
			return
		}
		p.stations = dedupeStations(append(p.stations, stations...))
		p.hasMore = len(stations) == pageSize
	})
}

func dedupeStations(stations []model.RadioStation) []model.RadioStation {
	seen := make(map[string]struct{}, len(stations))
	out := make([]model.RadioStation, 0, len(stations))
	for _, station := range stations {
		if station.StationUUID == "" {
			continue
		}
		if _, exists := seen[station.StationUUID]; exists {
			continue
		}
		seen[station.StationUUID] = struct{}{}
		out = append(out, station)
	}
	return out
}

func (p *Page) favourite(personal state.Personal, station model.RadioStation) bool {
	id := stationPersonalID(station.StationUUID)
	for _, item := range personal.Saved {
		if item.Kind == "station" && item.ID == id {
			return true
		}
	}
	return false
}

func (p *Page) toggleFavourite(e *env.Env, station model.RadioStation) {
	card := stationBrowse(station)
	for _, item := range e.Store.Snapshot().Personal.Saved {
		if item.Kind == card.Kind && item.ID == card.ID {
			card = item
			break
		}
	}
	if !p.favourite(e.Store.Snapshot().Personal, station) {
		widgets.SetPersonalSaved(e, card, true)
		p.saver.queue(e)
		return
	}
	at := widgets.SetPersonalSaved(e, card, false)
	if at < 0 {
		return
	}
	p.saver.queue(e)
	message := "Removed " + station.Name + " from your favourites."
	// Undo puts the station back where it stood, silently, as the item menu's Undo does
	restore := func() {
		widgets.RestorePersonalSaved(e, card, at)
		p.saver.queue(e)
	}
	pagesheet.OfferUndo(e, message, restore)
}

func (p *Page) recordRecent(e *env.Env, station model.RadioStation) {
	now := e.Now
	if now.IsZero() {
		now = time.Now()
	}
	item := stationBrowse(station)
	changed := e.Store.UpdatePersonal(func(personal *state.Personal) bool {
		if personal.Recent == nil {
			personal.Recent = make(map[string]state.RecentItem)
		}
		personal.Recent[item.ID] = state.RecentItem{BrowseItem: item, At: now.UnixMilli()}
		return true
	})
	if changed {
		p.saver.queue(e)
	}
}

func (p *Page) playStation(e *env.Env, station model.RadioStation) {
	act.Call[protocol.Null](e, protocol.MethodPlayRadioStation, protocol.PlayRadioStationParams{StationUUID: station.StationUUID}, func(_ protocol.Null, err error) {
		if err != nil {
			if e.Toast != nil {
				e.Toast(err.Error(), env.ToastError)
			}
			return
		}
		p.recordRecent(e, station)
		if e.Toast != nil {
			e.Toast("Playing "+station.Name, env.ToastSuccess)
		}
	})
}

type radioSaver struct {
	saving bool
	queued bool
}

func (s *radioSaver) queue(e *env.Env) {
	if s.saving {
		s.queued = true
		return
	}
	s.start(e)
}

func (s *radioSaver) start(e *env.Env) {
	raw, err := state.MarshalPersonal(e.Store.Snapshot().Personal)
	if err != nil {
		if e.Toast != nil {
			e.Toast("Could not save", env.ToastError)
		}
		return
	}
	e.Store.MarkPersonalSave(json.RawMessage(raw))
	s.saving = true
	s.queued = false
	act.Call[protocol.Null](e, protocol.MethodSetPersonal, protocol.SetPersonalParams{Personal: raw}, func(_ protocol.Null, err error) {
		s.saving = false
		if err != nil && e.Toast != nil {
			e.Toast("Could not save", env.ToastError)
		}
		if s.queued {
			s.start(e)
		}
	})
}
