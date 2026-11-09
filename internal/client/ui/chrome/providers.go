package chrome

import (
	"fmt"
	"image"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/anim"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
	"github.com/ryoku-dev/ryotunes/internal/client/state"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

var topProviders = [...]model.Provider{
	model.ProviderYouTube,
	model.ProviderSpotify,
	model.ProviderSoundCloud,
	model.ProviderTidal,
}

type providerCell struct {
	click kit.Click
	hover look.Hover
	tip   look.Tip
}

type providerSwitcher struct {
	cells   [len(topProviders)]providerCell
	compact providerCell
	menu    kit.Menu
	menuTag struct{}
	plate   anim.Tween

	display     model.Provider
	previous    model.Provider
	pending     bool
	acknowledge bool
	generation  uint64
}

func providerIndex(provider model.Provider) int {
	for index, candidate := range topProviders {
		if provider == candidate {
			return index
		}
	}
	return -1
}

func providerSignedIn(snapshot state.State, provider model.Provider) bool {
	switch provider {
	case model.ProviderYouTube:
		return snapshot.Auth.SignedIn
	case model.ProviderSpotify:
		return snapshot.Spotify.SignedIn
	case model.ProviderSoundCloud:
		return snapshot.SoundCloud.SignedIn
	case model.ProviderTidal:
		return snapshot.Tidal.SignedIn
	default:
		return true
	}
}

func providerTip(snapshot state.State, provider model.Provider) string {
	status := "Not signed in"
	if providerSignedIn(snapshot, provider) {
		status = "Signed in"
	}
	return look.ProviderName(provider) + " · " + status
}

func (switcher *providerSwitcher) active(gtx layout.Context, e *env.Env, snapshot state.State) model.Provider {
	if switcher.display == "" {
		switcher.display = snapshot.Provider
	}
	if switcher.pending && switcher.acknowledge && snapshot.Provider == switcher.display {
		switcher.pending = false
		switcher.acknowledge = false
	}
	if !switcher.pending {
		switcher.display = snapshot.Provider
	}
	if switcher.display == "" {
		switcher.display = model.ProviderYouTube
	}
	e.Th.SetProvider(gtx, switcher.display)
	return switcher.display
}

func (switcher *providerSwitcher) choose(gtx layout.Context, e *env.Env, snapshot state.State, provider model.Provider) {
	if provider == switcher.display || e == nil {
		return
	}
	switcher.generation++
	generation := switcher.generation
	switcher.previous = snapshot.Provider
	if switcher.pending {
		switcher.previous = switcher.display
	}
	switcher.display = provider
	switcher.pending = true
	switcher.acknowledge = false
	e.Th.SetProvider(gtx, provider)
	act.Call[protocol.ProviderResult](e, protocol.MethodSetProvider, protocol.ProviderParams{Provider: provider}, func(result protocol.ProviderResult, err error) {
		if generation != switcher.generation {
			return
		}
		if err != nil {
			switcher.display = switcher.previous
			switcher.pending = false
			switcher.acknowledge = false
			if e.Toast != nil {
				e.Toast(fmt.Sprintf("Could not switch to %s: %v", look.ProviderName(provider), err), env.ToastError)
			}
			return
		}
		switcher.acknowledge = true
		if result.Provider != "" {
			switcher.display = result.Provider
		}
	})
}

func providerGlyphCell(gtx layout.Context, e *env.Env, cell *providerCell, provider model.Provider, active bool, size image.Point) layout.Dimensions {
	gtx.Constraints = layout.Exact(size)
	return cell.click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		record := op.Record(gtx.Ops)
		dimensions, hover := cell.hover.Layout(gtx, e.Th, func(gtx layout.Context) layout.Dimensions {
			ink := e.Th.R.Fg3
			if cell.click.Hovered() {
				ink = e.Th.R.Fg2
			}
			if active {
				ink = e.Th.Src.Ink
			}
			return layout.Center.Layout(gtx, look.ProviderGlyph(provider, 20, ink))
		})
		content := record.Stop()
		area := clip.Rect(image.Rectangle{Max: dimensions.Size}).Push(gtx.Ops)
		if hover > 0 && !active {
			kit.FillCut(gtx.Ops, image.Rectangle{Max: dimensions.Size}, gtx.Dp(e.Th.Cut.Small), palette.WithAlpha(e.Th.R.Stone3, hover))
		}
		content.Add(gtx.Ops)
		area.Pop()
		if gtx.Focused(&cell.click.Clickable) {
			kit.FocusRing(gtx, e.Th.Theme, image.Rectangle{Max: dimensions.Size}, gtx.Dp(e.Th.Cut.Small))
		}
		return dimensions
	})
}

func (switcher *providerSwitcher) handleExpanded(gtx layout.Context, e *env.Env, snapshot state.State) {
	for index := range switcher.cells {
		if switcher.cells[index].click.Clicked(gtx) {
			switcher.choose(gtx, e, snapshot, topProviders[index])
			gtx.Execute(op.InvalidateCmd{})
		}
	}
}

func (switcher *providerSwitcher) layoutExpanded(gtx layout.Context, e *env.Env) layout.Dimensions {
	snapshot := e.Store.Snapshot()
	active := switcher.active(gtx, e, snapshot)

	width, height := gtx.Dp(180), gtx.Dp(40)
	size := image.Pt(width, height)
	gtx.Constraints = layout.Exact(size)
	area := clip.Rect(image.Rectangle{Max: size}).Push(gtx.Ops)
	kit.FillCut(gtx.Ops, image.Rectangle{Max: size}, gtx.Dp(6), e.Th.R.Stone1)

	cellWidth, cellHeight, gap := gtx.Dp(40), gtx.Dp(32), gtx.Dp(4)
	activeIndex := providerIndex(active)
	if activeIndex >= 0 {
		switcher.plate.Duration = e.Th.Motion.Select
		switcher.plate.Ease = anim.EaseChisel
		switcher.plate.Set(gtx, float32(activeIndex))
		plateAt := switcher.plate.Value(gtx)
		plateX := gtx.Dp(4) + int(plateAt*float32(cellWidth+gap))
		plateRect := image.Rect(plateX, gtx.Dp(4), plateX+cellWidth, gtx.Dp(4)+cellHeight)
		kit.FillCut(gtx.Ops, plateRect, gtx.Dp(6), palette.Mix(e.Th.R.Stone3, e.Th.Src.Fill, .22))
		kit.StrokeCut(gtx.Ops, plateRect, gtx.Dp(6), max(1, gtx.Dp(1)), palette.WithAlpha(e.Th.Src.Ink, .55))
	}

	for index, provider := range topProviders {
		x := gtx.Dp(4) + index*(cellWidth+gap)
		offset := op.Offset(image.Pt(x, gtx.Dp(4))).Push(gtx.Ops)
		cell := &switcher.cells[index]
		cell.tip.Layout(gtx, e.Th, e.Overlays, providerTip(snapshot, provider), func(gtx layout.Context) layout.Dimensions {
			return providerGlyphCell(gtx, e, cell, provider, provider == active, image.Pt(cellWidth, cellHeight))
		})
		offset.Pop()
	}
	switcher.handleExpanded(gtx, e, snapshot)
	area.Pop()
	return layout.Dimensions{Size: size}
}

func (switcher *providerSwitcher) openCompactMenu(gtx layout.Context, e *env.Env, anchor image.Rectangle, snapshot state.State) {
	items := make([]kit.MenuItem, len(topProviders))
	for index, provider := range topProviders {
		label := look.ProviderName(provider)
		if providerSignedIn(snapshot, provider) {
			label += " · Signed in"
		} else {
			label += " · Not signed in"
		}
		items[index] = kit.MenuItem{Label: label}
	}
	content := func(gtx layout.Context) layout.Dimensions {
		dimensions := switcher.menu.Layout(gtx, e.Th.Theme, items)
		if index, ok := switcher.menu.Clicked(gtx); ok && index >= 0 && index < len(topProviders) {
			switcher.choose(gtx, e, e.Store.Snapshot(), topProviders[index])
			e.Overlays.Close(&switcher.menuTag)
			gtx.Execute(op.InvalidateCmd{})
		}
		return dimensions
	}
	e.Overlays.Open(&switcher.menuTag, anchor, look.Placement{Prefer: look.Below, Align: look.End, Gap: 8, Width: unit.Dp(260)}, content)
}

func (switcher *providerSwitcher) layoutCompact(gtx layout.Context, e *env.Env, anchor image.Rectangle) layout.Dimensions {
	snapshot := e.Store.Snapshot()
	active := switcher.active(gtx, e, snapshot)
	size := image.Pt(gtx.Dp(40), gtx.Dp(40))
	dimensions := switcher.compact.tip.Layout(gtx, e.Th, e.Overlays, providerTip(snapshot, active), func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints = layout.Exact(size)
		record := op.Record(gtx.Ops)
		dimensions := providerGlyphCell(gtx, e, &switcher.compact, active, true, size)
		content := record.Stop()
		kit.FillCut(gtx.Ops, image.Rectangle{Max: dimensions.Size}, gtx.Dp(6), e.Th.R.Stone1)
		kit.StrokeCut(gtx.Ops, image.Rectangle{Max: dimensions.Size}, gtx.Dp(6), max(1, gtx.Dp(1)), e.Th.R.Vein)
		content.Add(gtx.Ops)
		return dimensions
	})
	if switcher.compact.click.Clicked(gtx) && e.Overlays != nil {
		if e.Overlays.IsOpen(&switcher.menuTag) {
			e.Overlays.Close(&switcher.menuTag)
		} else {
			switcher.openCompactMenu(gtx, e, anchor, snapshot)
		}
		gtx.Execute(op.InvalidateCmd{})
	}
	return dimensions
}
