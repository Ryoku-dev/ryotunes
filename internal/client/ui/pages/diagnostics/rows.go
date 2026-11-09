package diagnostics

import (
	"strings"

	"gioui.org/layout"

	"github.com/ryoku-dev/ryotunes/internal/client/lithic"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/model"
)

type logRow struct {
	row lithic.Row
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
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return r.row.Layout(gtx, e.Th, lithic.RowContent{
				Mark: func(gtx layout.Context) layout.Dimensions {
					return lithic.Icon(icon).Layout(gtx, e.Th.R.Stone(tone))
				},
				Title:     entry.Message,
				Facts:     facts,
				FactsMono: true,
				Claim:     claim,
				Tone:      tone,
				Progress:  -1,
			}, false, false)
		}),
		layout.Rigid(layout.Spacer{Height: e.Th.Sp.S1}.Layout),
	)
}

func logLevel(level string) (lithic.Tone, string, string) {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "error":
		return lithic.Bad, "Error", "error"
	case "warn", "warning":
		return lithic.Attention, "Warning", "warning"
	default:
		return lithic.Alive, "Info", "info"
	}
}

func logRowKey(entry model.LogEntry) string {
	return entry.Timestamp + "\x00" + entry.Level + "\x00" + entry.Source + "\x00" + entry.Target + "\x00" + entry.Message
}
