package chrome

import (
	"image"
	"strings"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

type discordToggle struct {
	click kit.Click
	hover look.Hover
	tip   look.Tip

	status     model.DiscordStatus
	loaded     bool
	loading    bool
	lastStatus time.Time
	nextStatus time.Time
	generation uint64
}

func discordEnabled(settings model.SettingsSnapshot) bool {
	return strings.EqualFold(strings.TrimSpace(settings[string(model.SettingDiscordRPC)]), "true")
}

func discordStateWord(enabled bool, status model.DiscordStatus) string {
	if !enabled {
		return "Off"
	}
	switch strings.ToLower(strings.TrimSpace(status.Status)) {
	case "connected":
		return "Connected"
	case "connecting", "starting":
		return "Connecting"
	default:
		return "Discord is not running"
	}
}

func (discord *discordToggle) refresh(gtx layout.Context, e *env.Env, enabled bool) {
	if !enabled || discord.loading {
		return
	}
	if !discord.nextStatus.IsZero() && gtx.Now.Before(discord.nextStatus) {
		gtx.Execute(op.InvalidateCmd{At: discord.nextStatus})
		return
	}
	discord.loading = true
	discord.nextStatus = time.Time{}
	act.Call[model.DiscordStatus](e, protocol.MethodDiscordStatus, protocol.Null{}, func(status model.DiscordStatus, err error) {
		discord.loading = false
		discord.loaded = true
		discord.lastStatus = time.Now()
		if err == nil {
			discord.status = status
		}
		if err == nil && strings.EqualFold(status.Status, "connecting") {
			discord.nextStatus = time.Now().Add(time.Second)
		}
	})
}

func (discord *discordToggle) toggle(e *env.Env, enabled bool) {
	discord.generation++
	generation := discord.generation
	next := !enabled
	e.Store.SetSetting(string(model.SettingDiscordRPC), boolString(next))
	if next {
		discord.status.Status = "connecting"
		discord.loaded = false
		discord.nextStatus = time.Time{}
	} else {
		discord.status = model.DiscordStatus{}
		discord.loaded = false
	}
	act.Call[protocol.Null](e, protocol.MethodSetSetting, protocol.SetSettingParams{Key: string(model.SettingDiscordRPC), Value: boolString(next)}, func(_ protocol.Null, err error) {
		if generation != discord.generation || err == nil {
			return
		}
		e.Store.SetSetting(string(model.SettingDiscordRPC), boolString(enabled))
		if e.Toast != nil {
			e.Toast("Could not change Discord status: "+err.Error(), env.ToastError)
		}
	})
}

func boolString(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

func (discord *discordToggle) layout(gtx layout.Context, e *env.Env) layout.Dimensions {
	snapshot := e.Store.Snapshot()
	enabled := discordEnabled(snapshot.Settings)
	staleOnHover := discord.click.Hovered() && (discord.lastStatus.IsZero() || gtx.Now.Sub(discord.lastStatus) >= 2*time.Second)
	if enabled && (!discord.loaded || strings.EqualFold(discord.status.Status, "connecting") || staleOnHover) {
		discord.refresh(gtx, e, enabled)
	}
	word := discordStateWord(enabled, discord.status)
	size := image.Pt(gtx.Dp(32), gtx.Dp(32))
	dimensions := discord.tip.Layout(gtx, e.Th, e.Overlays, "Discord status: "+word, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints = layout.Exact(size)
		return discord.click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			record := op.Record(gtx.Ops)
			dimensions, hover := discord.hover.Layout(gtx, e.Th, func(gtx layout.Context) layout.Dimensions {
				ink := e.Th.R.Fg2
				if discord.click.Hovered() {
					ink = e.Th.R.Fg
				}
				if enabled {
					ink = e.Th.R.OnSel
				}
				return layout.Center.Layout(gtx, look.DiscordGlyph(18, ink))
			})
			content := record.Stop()
			area := clip.Rect(image.Rectangle{Max: dimensions.Size}).Push(gtx.Ops)
			if enabled {
				kit.FillCut(gtx.Ops, image.Rectangle{Max: dimensions.Size}, gtx.Dp(6), e.Th.R.Sel)
			} else {
				kit.StrokeCut(gtx.Ops, image.Rectangle{Max: dimensions.Size}, gtx.Dp(6), max(1, gtx.Dp(1)), e.Th.R.Vein)
				if hover > 0 {
					kit.FillCut(gtx.Ops, image.Rectangle{Max: dimensions.Size}, gtx.Dp(6), palette.WithAlpha(e.Th.R.Stone3, hover))
				}
			}
			content.Add(gtx.Ops)
			area.Pop()
			if gtx.Focused(&discord.click.Clickable) {
				kit.FocusRing(gtx, e.Th.Theme, image.Rectangle{Max: dimensions.Size}, gtx.Dp(6))
			}
			return dimensions
		})
	})
	if discord.click.Clicked(gtx) {
		discord.toggle(e, enabled)
		gtx.Execute(op.InvalidateCmd{})
	}
	return dimensions
}
