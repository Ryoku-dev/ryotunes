package diagnostics

import (
	"strings"

	"gioui.org/layout"
	"gioui.org/widget"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/model"
)

type logRow struct {
	tablet kit.Tablet
}

func (r *logRow) layout(gtx layout.Context, e *env.Env, entry model.LogEntry) layout.Dimensions {
	tone, claim, icon := logLevel(entry.Level)
	facts := entry.Timestamp
	if entry.Source != "" {
		facts += " · " + entry.Source
	}
	if entry.Target != "" {
		facts += " · " + entry.Target
	}
	return r.tablet.Layout(gtx, e.Th.Theme, kit.TabletContent{
		Mark:  func(gtx layout.Context) layout.Dimensions { return icon.Layout(gtx, e.Th.R.Stone(tone)) },
		Title: entry.Message, Facts: facts, FactsMono: true, Claim: claim, Tone: tone, Progress: -1,
	}, false, false)
}

func logLevel(level string) (palette.Tone, string, *widget.Icon) {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "error":
		return palette.Failed, "Error", look.Icon.Close
	case "warn", "warning":
		return palette.Attention, "Warning", look.Icon.Tune
	default:
		return palette.Alive, "Info", look.Icon.Check
	}
}

func logRowKey(entry model.LogEntry) string {
	return entry.Timestamp + "\x00" + entry.Level + "\x00" + entry.Source + "\x00" + entry.Target + "\x00" + entry.Message
}
