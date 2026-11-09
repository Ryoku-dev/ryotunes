package diagnostics

import (
	"fmt"
	"strconv"
	"strings"

	"gioui.org/layout"
	"gioui.org/widget"

	"github.com/ryoku-dev/ryotunes/internal/client/lithic"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/widgets"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
	"github.com/ryoku-dev/ryotunes/internal/version"
)

// Layout renders filtered diagnostic evidence and runtime facts.
func (p *Page) Layout(gtx layout.Context, e *env.Env) layout.Dimensions {
	p.ensureLoaded(e)
	changed := false
	if index, ok := p.levelFilters.Changed(gtx); ok && index != p.levelIndex {
		p.levelIndex = index
		changed = true
	}
	if index, ok := p.sourceFilters.Changed(gtx); ok && index != p.sourceIndex {
		p.sourceIndex = index
		changed = true
	}
	for {
		event, ok := p.search.Update(gtx)
		if !ok {
			break
		}
		if submit, ok := event.(widget.SubmitEvent); ok {
			p.query = strings.TrimSpace(submit.Text)
			p.search.SetText(p.query)
			changed = true
		}
	}
	if strings.TrimSpace(p.search.Text()) == "" && p.query != "" {
		p.query = ""
		changed = true
	}
	if changed {
		p.fetch(e)
	}
	if p.retry.Clicked(gtx) {
		p.fetch(e)
	}
	if p.openLogs.Clicked(gtx) {
		act.Call[protocol.Null](e, protocol.MethodOpenLogsFolder, protocol.Null{}, func(_ protocol.Null, err error) {
			if err != nil && e.Toast != nil {
				e.Toast(err.Error(), env.ToastError)
			}
		})
	}
	if p.copyAll.Clicked(gtx) && len(p.logs.Entries) > 0 {
		act.CopyText(e, formatLogClipboard(p.logs.Entries), "Logs copied")
	}

	inset := layout.Inset{Top: e.Th.Sp.S4, Right: e.Th.Sp.S5, Bottom: e.Th.Sp.S5, Left: e.Th.Sp.S5}
	return inset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(widgets.PageHeader(e, "Diagnostics", "Recent daemon and client evidence.", p.headerActions(e))),
			layout.Rigid(layout.Spacer{Height: e.Th.Sp.S4}.Layout),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				if p.loading && !p.loaded {
					return widgets.Loading(e)(gtx)
				}
				if p.error != "" {
					return widgets.Failed(e, "Could not load diagnostics: "+p.error, &p.retry)(gtx)
				}
				return p.logsLayout(gtx, e)
			}),
		)
	})
}

func (p *Page) headerActions(e *env.Env) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		children := []layout.FlexChild{
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return p.openLogs.Layout(gtx, e.Th, lithic.Line, lithic.Icon("folder"), "Open logs folder")
			}),
		}
		if len(p.logs.Entries) > 0 {
			children = append(children,
				layout.Rigid(layout.Spacer{Width: e.Th.Sp.S2}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return p.copyAll.Layout(gtx, e.Th, lithic.Quiet, lithic.Icon("content_copy"), "Copy all")
				}),
			)
		}
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
	}
}

func (p *Page) logsLayout(gtx layout.Context, e *env.Env) layout.Dimensions {
	count := 3 + len(p.logs.Entries)
	if len(p.logs.Entries) == 0 {
		count++
	}
	return p.list.Layout(gtx, count, func(gtx layout.Context, index int) layout.Dimensions {
		switch index {
		case 0:
			return layout.Inset{Bottom: e.Th.Sp.S4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return lithic.Slab(gtx, e.Th.R.Stone1, e.Th.Cut.Mid, lithic.Facts(e.Th, p.runtimeFacts(e)))
			})
		case 1:
			return p.filterLayout(gtx, e)
		case 2:
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(lithic.GroupHeader(e.Th, "Log entries", p.logs.Total)),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Left: e.Th.Sp.S2, Right: e.Th.Sp.S2, Bottom: e.Th.Sp.S2}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return lithic.Text(e.Th, e.Th.T.MonoSm, e.Th.R.Fg3, totalsLine(p.logs))(gtx)
					})
				}),
			)
		default:
			if len(p.logs.Entries) == 0 {
				return widgets.EmptyState(e, "info", "No log entries.", "Try a broader level, source, or search filter.")(gtx)
			}
			entryIndex := index - 3
			entry := p.logs.Entries[entryIndex]
			key := logRowKey(entry)
			row := p.rows[key]
			if row == nil {
				row = &logRow{}
				p.rows[key] = row
			}
			return row.layout(gtx, e, entry)
		}
	})
}

func (p *Page) filterLayout(gtx layout.Context, e *env.Env) layout.Dimensions {
	return layout.Inset{Bottom: e.Th.Sp.S4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(520))
				return p.search.Layout(gtx, e.Th, "Search messages or targets…", "Enter")
			}),
			layout.Rigid(layout.Spacer{Height: e.Th.Sp.S3}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(lithic.Text(e.Th, e.Th.T.Caption, e.Th.R.Fg3, "Level")),
					layout.Rigid(layout.Spacer{Width: e.Th.Sp.S3}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return p.levelFilters.Layout(gtx, e.Th, []string{"All", "Warnings", "Errors"}, []int{p.logs.Total, p.logs.Warnings, p.logs.Errors}, p.levelIndex)
					}),
				)
			}),
			layout.Rigid(layout.Spacer{Height: e.Th.Sp.S2}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(lithic.Text(e.Th, e.Th.T.Caption, e.Th.R.Fg3, "Source")),
					layout.Rigid(layout.Spacer{Width: e.Th.Sp.S3}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return p.sourceFilters.Layout(gtx, e.Th, []string{"All", "Daemon", "Client"}, nil, p.sourceIndex)
					}),
				)
			}),
		)
	})
}

func (p *Page) runtimeFacts(e *env.Env) []lithic.Fact {
	daemon, protocolValue := "-", "-"
	if p.helloLoaded {
		daemon = p.hello.Daemon
		protocolValue = strconv.Itoa(p.hello.Protocol)
	}
	connected := "No"
	if e.Conn != nil && e.Conn.Connected() {
		connected = "Yes"
	}
	palette := p.palette
	if palette == "" {
		palette = "signature"
	}
	return []lithic.Fact{
		{Label: "Client version", Value: version.Version, Mono: true},
		{Label: "Daemon version", Value: daemon, Mono: true},
		{Label: "Protocol", Value: protocolValue, Mono: true},
		{Label: "Connected", Value: connected},
		{Label: "Socket", Value: protocol.SocketPath(), Mono: true},
		{Label: "Data directory", Value: p.dataDir, Mono: true},
		{Label: "Palette source", Value: palette},
		{Label: "Renderer", Value: p.renderer, Mono: true},
	}
}

func totalsLine(logs model.LogsResult) string {
	return fmt.Sprintf("%d entries · %d errors · %d warnings · %s", logs.Total, logs.Errors, logs.Warnings, formatBytes(logs.FileBytes))
}

func formatBytes(bytes int64) string {
	if bytes < 1024 {
		return fmt.Sprintf("%d B", bytes)
	}
	if bytes < 1024*1024 {
		return fmt.Sprintf("%.1f KiB", float64(bytes)/1024)
	}
	return fmt.Sprintf("%.1f MiB", float64(bytes)/(1024*1024))
}

func formatLogClipboard(entries []model.LogEntry) string {
	var text strings.Builder
	for i, entry := range entries {
		if i > 0 {
			text.WriteByte('\n')
		}
		fmt.Fprintf(&text, "%s [%s] %s %s: %s", entry.Timestamp, strings.ToUpper(entry.Level), entry.Source, entry.Target, entry.Message)
	}
	return text.String()
}
