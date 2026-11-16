package chrome

import (
	"image"
	"strings"
	"time"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/anim"
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
	wash  look.RowWash
	tip   look.Tip
	on    anim.Tween
	onSet bool

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
			th := e.Th
			bounds := image.Rectangle{Max: size}
			cut := gtx.Dp(6)
			usable := gtx.Source.Enabled()
			hover, _ := discord.wash.Update(gtx, th, usable && discord.click.Hovered(), usable && discord.click.Pressed())
			// on: the selection's wash and the accent glyph; off: a hairline. The two
			// cross over th.Motion.Quick.
			discord.on.Duration, discord.on.Ease = th.Motion.Quick, anim.EaseOutCubic
			target := float32(0)
			if enabled {
				target = 1
			}
			if !discord.onSet {
				discord.onSet = true
				discord.on.Set(gtx, target)
			}
			discord.on.Set(gtx, target)
			on := discord.on.Value(gtx)
			if on < 1 {
				kit.Stroke(gtx, th.Theme, bounds, cut, max(1, gtx.Dp(1)), palette.WithAlpha(th.R.Vein, 1-on))
			}
			if on > 0 {
				kit.Fill(gtx, th.Theme, bounds, cut, palette.WithAlpha(th.R.Select(), on))
			}
			discord.wash.Paint(gtx, th, bounds, cut)
			ink := palette.Mix(palette.Mix(th.R.Fg2, th.R.Fg, hover), th.R.Sel, on)
			glyphSize := gtx.Dp(18)
			scale := discord.wash.GlyphScale()
			at := op.Offset(image.Pt((size.X-glyphSize)/2, (size.Y-glyphSize)/2)).Push(gtx.Ops)
			glyphGtx := gtx
			glyphGtx.Constraints = layout.Exact(image.Pt(glyphSize, glyphSize))
			if scale != 1 {
				centre := f32.Pt(float32(glyphSize)/2, float32(glyphSize)/2)
				stack := op.Affine(f32.AffineId().Scale(centre, f32.Pt(scale, scale))).Push(gtx.Ops)
				look.DiscordGlyph(18, ink)(glyphGtx)
				stack.Pop()
			} else {
				look.DiscordGlyph(18, ink)(glyphGtx)
			}
			at.Pop()
			if gtx.Focused(&discord.click.Clickable) {
				kit.FocusRing(gtx, th.Theme, bounds, cut)
			}
			return layout.Dimensions{Size: size}
		})
	})
	if discord.click.Clicked(gtx) {
		discord.toggle(e, enabled)
		gtx.Execute(op.InvalidateCmd{})
	}
	return dimensions
}
