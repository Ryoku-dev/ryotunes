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
	"github.com/ryoku-dev/ryotunes/internal/client/ui/sheet"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/widgets"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
	"github.com/ryoku-dev/ryotunes/internal/version"
)

var (
	levelLabels  = []string{"All", "Warnings", "Errors"}
	sourceLabels = []string{"All", "Daemon", "Client"}
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
	if p.loading && !p.loaded {
		return widgets.Loading(e, widgets.LoadingTable)(gtx)
	}
	if p.error != "" {
		return widgets.Failed(e, "The daemon did not send its logs ("+p.error+"). Retry once it is running.", &p.retry)(gtx)
	}
	inset := look.ContentInset(e.Th.Class)
	count := 4 + max(1, len(p.logs.Entries))
	return p.scroll.Layout(gtx, e.Th, count, func(gtx layout.Context, index int) layout.Dimensions {
		return p.reveal.Group(gtx, e.Th, index, func(gtx layout.Context) layout.Dimensions {
			return p.layoutElement(gtx, e, index, count, inset)
		})
	})
}

// Masthead names Diagnostics on the frame's masthead and says how far the log is read.
func (p *Page) Masthead() (part, partGloss string, progress float32) {
	return "", "", p.scroll.Progress()
}

// layoutHeader is the page head with the log's counts as its spec strip, errors lit while
// there are any.
func (p *Page) layoutHeader(gtx layout.Context, e *env.Env) layout.Dimensions {
	p.header.Place = sheet.Diagnostics
	p.strip.Set(0, "Entries", p.logs.Total, false)
	p.strip.Set(1, "Errors", p.logs.Errors, p.logs.Errors > 0)
	p.strip.Set(2, "Warnings", p.logs.Warnings, false)
	p.header.Specs = p.strip.Specs(p.loaded)
	return p.header.Layout(gtx, e, "Diagnostics", "Recent daemon and client evidence.", p.headerActions(e))
}

func (p *Page) layoutElement(gtx layout.Context, e *env.Env, index, count int, inset unit.Dp) layout.Dimensions {
	if index == 0 {
		return layout.Inset{Top: e.Th.Sp.S2}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return p.layoutHeader(gtx, e)
		})
	}
	if index == 4 && len(p.logs.Entries) == 0 {
		return widgets.EmptyState(e, look.Icon.Search, "No log entries", "Try a broader level, source, or search filter.")(gtx)
	}
	var body layout.Widget
	switch index {
	case 1:
		body = func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(21)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return widgets.RuledHead(gtx, e, "Runtime", kit.Ordinal(1, 2), nil)
					}),
					layout.Rigid(layout.Spacer{Height: e.Th.Sp.S3}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return kit.SlabContent(gtx, e.Th.Theme, kit.Facts(e.Th.Theme, p.runtimeFacts(e)))
					}),
				)
			})
		}
	case 2:
		body = func(gtx layout.Context) layout.Dimensions { return p.filterLayout(gtx, e) }
	case 3:
		gap := unit.Dp(8)
		if len(p.logs.Entries) == 0 {
			gap = 0
		}
		body = func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(13), Bottom: gap}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical, Alignment: layout.Start}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return widgets.RuledHead(gtx, e, "Log entries", kit.Ordinal(2, 2), nil)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return kit.LayoutSpacedCaps(gtx, e.Th.Theme, e.Th.T.MonoSm, e.Th.R.Fg3, 1.2, p.totals())
					}),
				)
			})
		}
	default:
		entry := p.logs.Entries[index-4]
		key := logRowKey(entry)
		row := p.rows[key]
		if row == nil {
			row = new(logRow)
			p.rows[key] = row
		}
		body = func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return row.layout(gtx, e, entry)
			})
		}
	}
	bottom := unit.Dp(0)
	if index+1 == count {
		bottom = inset
	}
	return layout.Inset{Right: inset, Bottom: bottom, Left: inset}.Layout(gtx, body)
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
	p.levelCounts = [3]int{p.logs.Total, p.logs.Warnings, p.logs.Errors}
	p.levelFilters.Labels, p.levelFilters.Counts, p.levelFilters.Selected = levelLabels, p.levelCounts[:], p.levelIndex
	p.sourceFilters.Labels, p.sourceFilters.Selected = sourceLabels, p.sourceIndex
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
	if source := e.Th.Desktop.Palette.Source; source != "" {
		facts = append(facts, kit.Fact{Label: "Palette source", Value: source})
	}
	facts = append(facts,
		kit.Fact{Label: "Renderer", Value: p.renderer, Mono: true},
		kit.Fact{Label: "Log size", Value: formatBytes(p.logs.FileBytes), Mono: true},
	)
	return facts
}

// totals is the line under the log's head: what the filters show and the file's size, kept
// until the log changes.
func (p *Page) totals() string {
	key := totalsKey{shown: len(p.logs.Entries), total: p.logs.Total, bytes: p.logs.FileBytes}
	if key != p.totalsKey || p.totalsText == "" {
		p.totalsKey, p.totalsText = key, totalsLine(p.logs)
	}
	return p.totalsText
}

type totalsKey struct {
	shown, total int
	bytes        int64
}

func totalsLine(logs model.LogsResult) string {
	return fmt.Sprintf("Showing %s of %s · log %s", kit.Pad2(len(logs.Entries)), kit.Pad2(logs.Total), formatBytes(logs.FileBytes))
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
