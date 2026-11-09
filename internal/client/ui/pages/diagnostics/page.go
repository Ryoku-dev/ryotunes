// Package diagnostics implements the client and daemon diagnostic evidence page.
package diagnostics

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

const diagnosticsLimit = 2000

// Page is the retained diagnostics query and evidence surface.
type Page struct {
	route env.Route

	search        kit.Search
	levelFilters  kit.Filters
	sourceFilters kit.Filters
	scroll        look.Scroller
	rows          map[string]*logRow

	levelIndex  int
	sourceIndex int
	query       string
	logs        model.LogsResult
	hello       protocol.Hello
	dataDir     string
	renderer    string

	loaded      bool
	loading     bool
	helloLoaded bool
	error       string
	request     uint64

	openLogs kit.Plaque
	copyAll  kit.Plaque
	retry    kit.Plaque
}

// New constructs a lazy diagnostics page.
func New(r env.Route) env.Page {
	p := &Page{
		route: r, rows: make(map[string]*logRow), dataDir: dataDirectory(),
		renderer: "Gio · " + runtime.GOOS + "/" + runtime.GOARCH,
	}
	p.search.Submit = true
	return p
}

// Title returns the page title.
func (p *Page) Title() string { return "Diagnostics" }

// Refresh reloads the current log query and handshake facts.
func (p *Page) Refresh(e *env.Env) {
	p.query = strings.TrimSpace(p.search.Text())
	p.search.SetText(p.query)
	p.fetch(e)
}

func (p *Page) ensureLoaded(e *env.Env) {
	if !p.loaded && !p.loading {
		p.fetch(e)
	}
}

func (p *Page) fetch(e *env.Env) {
	p.request++
	request := p.request
	p.loading, p.error = true, ""
	level, source, limit := levelValue(p.levelIndex), sourceValue(p.sourceIndex), diagnosticsLimit
	params := protocol.GetLogsParams{Level: &level, Source: &source, Limit: &limit}
	if query := strings.TrimSpace(p.query); query != "" {
		params.Query = &query
	}
	act.Call[model.LogsResult](e, protocol.MethodGetLogs, params, func(result model.LogsResult, err error) {
		if request != p.request {
			return
		}
		p.loading, p.loaded = false, true
		if err != nil {
			p.error = err.Error()
			return
		}
		p.logs = result
	})
	act.Call[protocol.Hello](e, protocol.MethodHello, protocol.Null{}, func(result protocol.Hello, err error) {
		if request == p.request {
			p.helloLoaded = err == nil
			if err == nil {
				p.hello = result
			}
		}
	})
}

func levelValue(index int) string {
	values := [...]string{"all", "warn", "error"}
	if index < 0 || index >= len(values) {
		return values[0]
	}
	return values[index]
}

func sourceValue(index int) string {
	values := [...]string{"all", "daemon", "client"}
	if index < 0 || index >= len(values) {
		return values[0]
	}
	return values[index]
}

func dataDirectory() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	root := os.Getenv("XDG_DATA_HOME")
	if !filepath.IsAbs(root) {
		root = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(filepath.Clean(root), "dev.ryoku.ryotunes")
}
