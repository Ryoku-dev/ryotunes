package diagnostics

import (
	"fmt"
	"strconv"
	"strings"

	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/widgets"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
	"github.com/ryoku-dev/ryotunes/internal/version"
)

// Layout renders filtered diagnostic evidence and runtime facts.
func (p *Page) Layout(gtx layout.Context, e *env.Env) layout.Dimensions {
	p.ensureLoaded(e)
	changed := false
	if index, ok := p.levelFilters.Changed(); ok && index != p.levelIndex {
		p.levelIndex, changed = index, true
	}
	if index, ok := p.sourceFilters.Changed(); ok && index != p.sourceIndex {
		p.sourceIndex, changed = index, true
	}
	for {
		event, ok := p.search.Update(gtx)
		if !ok {
			break
		}
		if submit, ok := event.(widget.SubmitEvent); ok {
			p.query = strings.TrimSpace(submit.Text)
			changed = true
		}
	}
	if strings.TrimSpace(p.search.Text()) == "" && p.query != "" {
		p.query, changed = "", true
	}
	if changed {
		p.fetch(e)
	}
	if p.retry.Clicked(gtx) {
		p.fetch(e)
	}
	if p.openLogs.Clicked(gtx) {
		act.Call[protocol.Null](e, protocol.MethodOpenLogsFolder, protocol.Null{}, func(_ protocol.Null, err error) {
			if err != nil {
				e.Toast(err.Error(), env.ToastError)
			}
		})
	}
	if p.copyAll.Clicked(gtx) && len(p.logs.Entries) > 0 {
		act.CopyText(e, formatLogClipboard(p.logs.Entries), "Logs copied")
	}
	inset := look.ContentInset(e.Th.Class)
	return layout.Inset{Right: inset, Bottom: inset, Left: inset}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		if p.loading && !p.loaded {
			return widgets.Loading(e, widgets.LoadingTable)(gtx)
		}
		if p.error != "" {
			p.retry.Kind, p.retry.Label = kit.PlaqueLine, "Retry"
			return widgets.Failed(e, "Could not load diagnostics: "+p.error, &p.retry)(gtx)
		}
		count := 4 + max(1, len(p.logs.Entries))
		return p.scroll.Layout(gtx, e.Th, count, func(gtx layout.Context, index int) layout.Dimensions {
			switch index {
			case 0:
				return widgets.PageHeader(e, "Diagnostics", "Recent daemon and client evidence.", p.headerActions(e))(gtx)
			case 1:
				return layout.Inset{Top: unit.Dp(21)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return kit.Section(e.Th.Theme, "Runtime", func(gtx layout.Context) layout.Dimensions {
						return kit.SlabContent(gtx, e.Th.Theme, kit.Facts(e.Th.Theme, p.runtimeFacts(e)))
					})(gtx)
				})
			case 2:
				return p.filterLayout(gtx, e)
			case 3:
				return layout.Inset{Top: unit.Dp(13), Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical, Alignment: layout.Start}.Layout(gtx,
						layout.Rigid(kit.GroupHeader(e.Th.Theme, "Log entries", p.logs.Total)),
						layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
						layout.Rigid(kit.Text(e.Th.Theme, e.Th.T.MonoSm, e.Th.R.Fg3, totalsLine(p.logs))),
					)
				})
			default:
				if len(p.logs.Entries) == 0 {
					return widgets.EmptyState(e, look.Icon.Search, "No log entries", "Try a broader level, source, or search filter.")(gtx)
				}
				entry := p.logs.Entries[index-4]
				key := logRowKey(entry)
				row := p.rows[key]
				if row == nil {
					row = new(logRow)
					p.rows[key] = row
				}
				return layout.Inset{Bottom: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions { return row.layout(gtx, e, entry) })
			}
		})
	})
}

func (p *Page) headerActions(e *env.Env) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		p.openLogs.Kind, p.openLogs.Icon, p.openLogs.Label = kit.PlaqueLine, look.Icon.Folder, "Open logs folder"
		p.copyAll.Kind, p.copyAll.Label = kit.PlaqueQuiet, "Copy all"
		children := []layout.FlexChild{layout.Rigid(func(gtx layout.Context) layout.Dimensions { return p.openLogs.Layout(gtx, e.Th.Theme) })}
		if len(p.logs.Entries) > 0 {
			children = append(children,
				layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return p.copyAll.Layout(gtx, e.Th.Theme) }),
			)
		}
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
	}
}

func (p *Page) filterLayout(gtx layout.Context, e *env.Env) layout.Dimensions {
	p.search.Placeholder, p.search.Hint = "Search messages or targets", "Enter"
	p.levelFilters.Labels, p.levelFilters.Counts, p.levelFilters.Selected = []string{"All", "Warnings", "Errors"}, []int{p.logs.Total, p.logs.Warnings, p.logs.Errors}, p.levelIndex
	p.sourceFilters.Labels, p.sourceFilters.Selected = []string{"All", "Daemon", "Client"}, p.sourceIndex
	return layout.Inset{Top: unit.Dp(13), Bottom: unit.Dp(13)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.Start}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(520))
				return p.search.Layout(gtx, e.Th.Theme)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(13)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return p.levelFilters.Layout(gtx, e.Th.Theme) }),
			layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return p.sourceFilters.Layout(gtx, e.Th.Theme) }),
		)
	})
}

func (p *Page) runtimeFacts(e *env.Env) []kit.Fact {
	facts := []kit.Fact{{Label: "Client version", Value: version.Version, Mono: true}}
	if p.helloLoaded {
		facts = append(facts,
			kit.Fact{Label: "Daemon version", Value: p.hello.Daemon, Mono: true},
			kit.Fact{Label: "Protocol", Value: strconv.Itoa(p.hello.Protocol), Mono: true},
		)
	}
	connected := "No"
	if e.Conn != nil && e.Conn.Connected() {
		connected = "Yes"
	}
	facts = append(facts,
		kit.Fact{Label: "Connected", Value: connected},
		kit.Fact{Label: "Socket", Value: protocol.SocketPath(), Mono: true},
	)
	if p.logs.EngineVersion != "" {
		facts = append(facts, kit.Fact{Label: "Engine", Value: p.logs.EngineVersion, Mono: true})
	}
	mpris := "Unavailable"
	if p.logs.MPRIS {
		mpris = "Connected"
	}
	facts = append(facts, kit.Fact{Label: "MPRIS", Value: mpris})
	if p.dataDir != "" {
		facts = append(facts, kit.Fact{Label: "Data directory", Value: p.dataDir, Mono: true})
	}
	preferences := e.Store.Snapshot().Client
	paletteSource := preferences.Appearance
	if preferences.PaletteFile != "" {
		paletteSource = preferences.PaletteFile
	}
	if paletteSource != "" {
		facts = append(facts, kit.Fact{Label: "Palette source", Value: paletteSource, Mono: preferences.PaletteFile != ""})
	}
	facts = append(facts,
		kit.Fact{Label: "Renderer", Value: p.renderer, Mono: true},
		kit.Fact{Label: "Log size", Value: formatBytes(p.logs.FileBytes), Mono: true},
	)
	return facts
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
