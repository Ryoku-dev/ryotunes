package settings

import (
	"gioui.org/layout"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/design"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
	libtheme "github.com/ryoku-dev/ryoku/docs/lithic/library/go/theme"
	"github.com/ryoku-dev/ryotunes/internal/client/state"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/model"
)

var (
	densityLabels            = []string{"Comfortable", "Compact"}
	densityValues            = []string{"comfortable", "compact"}
	visualizerStyleLabels    = []string{"Bars", "Curve", "Oscilloscope", "Vectorscope"}
	visualizerSpectrumLabels = visualizerStyleLabels[:2]
	visualizerScopeLabels    = visualizerStyleLabels[2:]
	accentLabels             = []string{"Album art", "Source", "Desktop"}
	accentValues             = []string{state.AccentArt, state.AccentSource, state.AccentDesktop}
)

// The desktop's look as Settings claims it: what Ryoku Settings' Style page
// (and Animations, for reduced motion) has chosen for every Ryoku app.
const (
	claimDesign = iota
	claimColours
	claimMode
	claimContrast
	claimTint
	claimMotion
	claimCount
)

// styleClaims are the claims' values in words; an empty value is a claim
// the desktop does not make now (tint outside Lithic on wallpaper colours,
// as the Hub drops its Tint row).
type styleClaims [claimCount]string

var claimTitles = [claimCount]string{
	claimDesign:   "Design",
	claimColours:  "Colours",
	claimMode:     "Light or dark",
	claimContrast: "Contrast",
	claimTint:     "Tint",
	claimMotion:   "Reduce motion",
}

var claimDetails = [claimCount]string{
	claimDesign:   "The shape of every Ryoku app: Lithic, Material, Adwaita or Fluent",
	claimColours:  "Ryoku's own colours, or the desktop's from your wallpaper",
	claimMode:     "For the whole desktop",
	claimContrast: "High makes text and the lines between things stronger",
	claimTint:     "How much of the wallpaper's hue the surfaces take",
	claimMotion:   "Stills Ryotunes' animations with the rest of the desktop",
}

var (
	designNames   = [...]string{design.Lithic: "Lithic", design.Material: "Material", design.Adwaita: "Adwaita", design.Fluent: "Fluent"}
	contrastNames = [...]string{palette.Standard: "Standard", palette.High: "High"}
	tintNames     = [...]string{palette.TintOff: "Off", palette.TintLow: "Low", palette.TintMedium: "Medium"}
)

// claimsOf reads the claims from the desktop's look as the library resolved
// it (palette.ReadDesktop, live through WatchDesktop).
func claimsOf(desktop palette.Desktop) styleClaims {
	var claims styleClaims
	claims[claimDesign] = nameOr(designNames[:], int(desktop.Design), "Lithic")
	claims[claimColours] = "Ryoku"
	if desktop.Colours == palette.Wallpaper {
		claims[claimColours] = "Wallpaper"
	}
	claims[claimMode] = "Dark"
	if !desktop.Palette.Dark {
		claims[claimMode] = "Light"
	}
	claims[claimContrast] = nameOr(contrastNames[:], int(desktop.Look.Contrast), "Standard")
	if desktop.Colours == palette.Wallpaper && desktop.Design == design.Lithic {
		claims[claimTint] = nameOr(tintNames[:], int(desktop.Look.Tint), "Off")
	}
	claims[claimMotion] = "Off"
	if desktop.ReduceMotion {
		claims[claimMotion] = "On"
	}
	return claims
}

func nameOr(names []string, index int, fallback string) string {
	if index >= 0 && index < len(names) && names[index] != "" {
		return names[index]
	}
	return fallback
}

// layoutAppearance draws Appearance in two slabs. The first claims the
// desktop's look, read-only and live: Ryotunes keeps no copy of it, so a
// change in Ryoku Settings rolls each value to its new word here as the
// window takes it on; a quiet hand-off opens the Style page. The second holds
// what is Ryotunes' own: the music's colour (the accent and tinting with the
// cover) and the player's surfaces.
func (p *page) layoutAppearance(gtx layout.Context, e *env.Env, snapshot state.State) layout.Dimensions {
	p.appearance.claims = claimsOf(e.Desktop())
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return p.layoutStyleClaims(gtx, e)
		}),
		layout.Rigid(layout.Spacer{Height: e.Th.Sp.S4}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return p.layoutOwnLook(gtx, e, snapshot)
		}),
	)
}

func (p *page) layoutStyleClaims(gtx layout.Context, e *env.Env) layout.Dimensions {
	p.newSlab()
	rows := make([]layout.Widget, 0, claimCount+1)
	for claim := range claimCount {
		value := p.appearance.claims[claim]
		if value == "" {
			continue
		}
		roll := &p.appearance.claimRoll[claim]
		rows = append(rows, p.row(e.Th, rowSpec{
			title:  claimTitles[claim],
			detail: claimDetails[claim],
			control: func(gtx layout.Context) layout.Dimensions {
				return roll.Layout(gtx, e.Th, e.Th.T.Body, e.Th.R.Fg, value)
			},
		}))
	}
	rows = append(rows, p.row(e.Th, rowSpec{
		title:    "Ryoku Settings",
		detail:   "The look is the desktop's: change it once in Style and every Ryoku app follows",
		control:  p.handoffControl(e),
		activate: func() { handOff(e, stylePage) },
	}))
	return p.settingRows(gtx, rows...)
}

// handoffControl is the quiet "Open Ryoku Settings" plaque.
func (p *page) handoffControl(e *env.Env) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		button := &p.appearance.handoff
		button.Kind, button.Small, button.Icon, button.Label = kit.PlaqueQuiet, true, look.Icon.OpenInNew, "Open Ryoku Settings"
		if button.Clicked(gtx) {
			handOff(e, stylePage)
		}
		return button.Layout(gtx, e.Th.Theme)
	}
}

func (p *page) layoutOwnLook(gtx layout.Context, e *env.Env, snapshot state.State) layout.Dimensions {
	p.newSlab()
	preferences := snapshot.Client
	reduced := e.Desktop().ReduceMotion
	density := indexOf(densityValues, preferences.Density, 0)
	scale := intOr(p.value(snapshot.Settings, model.SettingUIScale), 100)
	lowResource := p.valueBool(snapshot.Settings, model.SettingLowResourceMode)
	visualizerStyle := preferences.Style()
	spectrumInstalled := e.Spectrum != nil && e.Spectrum.Installed()
	scopeInstalled := e.Scope != nil && e.Scope.Installed()
	spectrumAvailable := e.Spectrum != nil && e.Spectrum.Available()
	scopeAvailable := e.Scope != nil && e.Scope.Available()
	anyVisualizerInstalled := spectrumInstalled || scopeInstalled
	visualizerDetail, selectedVisualizerAvailable := visualizerPreferenceDetail(
		visualizerStyle,
		spectrumInstalled,
		scopeInstalled,
		spectrumAvailable,
		scopeAvailable,
	)
	if selectedVisualizerAvailable && reduced {
		visualizerDetail += " Reduced motion pauses it without changing this setting."
	} else if selectedVisualizerAvailable && lowResource {
		visualizerDetail += " Low resource mode pauses it without changing this setting."
	}
	styleLabels, styleValues := visualizerStyleOptions(spectrumInstalled, scopeInstalled)
	visualizerStyleDetail := visualizerStylePreferenceDetail(spectrumInstalled, scopeInstalled)
	waveDetail := "A sharp travelling wave marks the played part of every seek bar."
	if reduced {
		waveDetail += " Reduced motion pauses it without changing this setting."
	} else if lowResource {
		waveDetail += " Low resource mode pauses it without changing this setting."
	}
	accentMode := preferences.AccentMode()
	openFloating := preferences.OpenFloatingOn()
	save := func(change func(*state.ClientPreferences)) func() {
		return func() {
			next := e.Store.Snapshot().Client
			change(&next)
			saveClientPreferences(e, next)
		}
	}
	return p.settingRows(gtx,
		p.row(e.Th, rowSpec{
			title: "Accent colour", detail: accentPreferenceDetail(accentMode), wide: true,
			changed: accentChanged(preferences), reset: save(func(p *state.ClientPreferences) { p.Accent = "" }),
			control: func(gtx layout.Context) layout.Dimensions {
				p.appearance.accent.Labels = accentLabels
				p.appearance.accent.Selected = indexOf(accentValues, accentMode, 0)
				dims := p.appearance.accent.Layout(gtx, e.Th)
				if selected, changed := p.appearance.accent.Changed(); changed && selected >= 0 && selected < len(accentValues) {
					preferences.Accent = accentValues[selected]
					saveClientPreferences(e, preferences)
				}
				return dims
			},
		}),
		p.toggleRow(e.Th, &p.appearance.coverTint, preferences.CoverTintOn(), accentMode != state.AccentArt, rowSpec{
			title: "Tint with the cover", detail: coverTintDetail(accentMode),
			changed: coverTintChanged(preferences), reset: save(func(p *state.ClientPreferences) { p.CoverTint = nil }),
		}, func(on bool) {
			save(func(p *state.ClientPreferences) { p.CoverTint = &on })()
		}),
		p.row(e.Th, rowSpec{
			title: "Density", detail: "Compact changes resource rows from 56 dp to 48 dp.",
			changed: densityChanged(preferences), reset: func() { setDensity(e, preferences, "") },
			control: func(gtx layout.Context) layout.Dimensions {
				dims := p.appearance.density.Layout(gtx, e.Th, e.Overlays, densityLabels, density)
				if selected, ok := p.appearance.density.Changed(); ok {
					setDensity(e, preferences, densityValues[selected])
				}
				return dims
			},
		}),
		p.row(e.Th, rowSpec{
			title: "UI scale", detail: "Preferred renderer scale, persisted for every Ryotunes client on this account.",
			changed: p.settingChanged(snapshot.Settings, model.SettingUIScale),
			reset:   func() { p.write(e, model.SettingUIScale, settingDefaults[model.SettingUIScale], "", nil) },
			control: func(gtx layout.Context) layout.Dimensions {
				p.appearance.scale.Value, p.appearance.scale.Min, p.appearance.scale.Max = scale, 80, 140
				p.appearance.scale.Step, p.appearance.scale.Unit = 10, "%"
				dims := p.appearance.scale.Layout(gtx, e.Th.Theme)
				if p.appearance.scale.Changed() {
					p.write(e, model.SettingUIScale, intString(p.appearance.scale.Value), "", nil)
				}
				return dims
			},
		}),
		p.toggleSettingRow(e, &p.appearance.lowResource, snapshot.Settings, model.SettingLowResourceMode,
			"Low resource mode", "Reduce automatic Home and network work, and pause visualizers and wave progress without changing their settings."),
		p.toggleRow(e.Th, &p.appearance.visualizer, preferences.VisualizerOn(), !anyVisualizerInstalled, rowSpec{
			title: "Visualizer", detail: visualizerDetail,
			changed: visualizerChanged(preferences), reset: save(func(p *state.ClientPreferences) { p.Visualizer = nil }),
		}, func(on bool) {
			save(func(p *state.ClientPreferences) { p.Visualizer = &on })()
		}),
		p.row(e.Th, rowSpec{
			title: "Visualizer style", detail: visualizerStyleDetail, wide: true,
			changed: visualizerStyleChanged(preferences), reset: save(func(p *state.ClientPreferences) { p.VisualizerStyle = "" }),
			control: func(gtx layout.Context) layout.Dimensions {
				p.appearance.visualizerStyle.Labels = styleLabels
				p.appearance.visualizerStyle.Selected = visualizerStyleIndex(styleValues, visualizerStyle)
				styleGTX := gtx
				if !preferences.VisualizerOn() || !anyVisualizerInstalled {
					styleGTX = styleGTX.Disabled()
				}
				dims := p.appearance.visualizerStyle.Layout(styleGTX, e.Th)
				if selected, changed := p.appearance.visualizerStyle.Changed(); changed && preferences.VisualizerOn() && selected >= 0 && selected < len(styleValues) {
					preferences.VisualizerStyle = string(styleValues[selected])
					saveClientPreferences(e, preferences)
				}
				return dims
			},
		}),
		p.toggleRow(e.Th, &p.appearance.waveProgress, preferences.WaveOn(), false, rowSpec{
			title: "Wave progress", detail: waveDetail,
			changed: waveChanged(preferences), reset: save(func(p *state.ClientPreferences) { p.WaveProgress = nil }),
		}, func(on bool) {
			save(func(p *state.ClientPreferences) { p.WaveProgress = &on })()
		}),
		p.toggleRow(e.Th, &p.appearance.openFloating, openFloating, false, rowSpec{
			title: "Open as a floating window", detail: floatingWindowDetail(openFloating, e.Store.MainWindowOpenedFloating()),
			changed: openFloatingChanged(preferences), reset: save(func(p *state.ClientPreferences) { p.OpenFloating = nil }),
		}, func(on bool) {
			save(func(p *state.ClientPreferences) { p.OpenFloating = &on })()
		}),
	)
}

// setDensity stores the row density and applies it to the theme at once.
func setDensity(e *env.Env, preferences state.ClientPreferences, value string) {
	preferences.Density = value
	if value == "compact" {
		e.Th.Density = libtheme.Compact
	} else {
		e.Th.Density = libtheme.Comfortable
	}
	saveClientPreferences(e, preferences)
}

// toggleRow is a row whose control is a toggle; the whole row toggles it, as
// the Hub's toggle rows do. A disabled row neither toggles nor activates.
func (p *page) toggleRow(th *look.Theme, toggle *kit.Toggle, value, disabled bool, spec rowSpec, set func(bool)) layout.Widget {
	spec.control = func(gtx layout.Context) layout.Dimensions {
		toggle.Value = value
		if disabled {
			gtx = gtx.Disabled()
		}
		dims := toggle.Layout(gtx, th.Theme)
		if toggle.Changed() && !disabled {
			set(toggle.Value)
		}
		return dims
	}
	if !disabled {
		spec.activate = func() { set(!value) }
	}
	return p.row(th, spec)
}

// coverTintDetail says what tinting with the cover does, or why it waits.
func coverTintDetail(accentMode string) string {
	if accentMode != state.AccentArt {
		return "Follows the album art accent"
	}
	return "The window takes a tint of the cover that is playing over the desktop's surfaces"
}

func floatingWindowDetail(preferred, opened bool) string {
	detail := "Opens tiled in your layout like any other window"
	if preferred {
		detail = "Opens at its own size above your tiled windows"
	}
	if preferred != opened {
		detail += " from the next launch"
	}
	return detail
}

func visualizerStyleIndex(styles []state.VisualizerStyle, style state.VisualizerStyle) int {
	for index, candidate := range styles {
		if candidate == style {
			return index
		}
	}
	return 0
}

func visualizerStyleOptions(spectrumInstalled, scopeInstalled bool) ([]string, []state.VisualizerStyle) {
	switch {
	case spectrumInstalled && !scopeInstalled:
		return visualizerSpectrumLabels, state.VisualizerStyles[:2]
	case !spectrumInstalled && scopeInstalled:
		return visualizerScopeLabels, state.VisualizerStyles[2:]
	default:
		return visualizerStyleLabels, state.VisualizerStyles
	}
}

func visualizerStylePreferenceDetail(spectrumInstalled, scopeInstalled bool) string {
	switch {
	case spectrumInstalled && !scopeInstalled:
		return "Install pw-record to see the waveform and stereo field"
	case !spectrumInstalled && scopeInstalled:
		return "Install cava to see Bars and Curve"
	case !spectrumInstalled && !scopeInstalled:
		return "Install cava or pw-record to choose a visualizer style"
	default:
		return "Choose how the visualizer draws the music."
	}
}

func visualizerPreferenceDetail(style state.VisualizerStyle, spectrumInstalled, scopeInstalled, spectrumAvailable, scopeAvailable bool) (string, bool) {
	if style == state.VisualizerBars || style == state.VisualizerCurve {
		switch {
		case !spectrumInstalled:
			return "Install cava to see Bars and Curve", false
		case !spectrumAvailable:
			return "cava stopped; play again to retry", false
		case style == state.VisualizerCurve:
			return "A curve that moves with the music in page headers and the now playing view.", true
		default:
			return "Bars that move with the music in page headers and the now playing view.", true
		}
	}
	switch {
	case !scopeInstalled:
		return "Install pw-record to see the waveform and stereo field", false
	case !scopeAvailable:
		return "pw-record stopped; play again to retry", false
	case style == state.VisualizerVectorscope:
		return "The stereo field moves with the music in page headers and the now playing view.", true
	default:
		return "A waveform that moves with the music in page headers and the now playing view.", true
	}
}

func accentPreferenceDetail(mode string) string {
	switch mode {
	case state.AccentSource:
		return "The colour of the service you are listening on"
	case state.AccentDesktop:
		return "Your Ryoku palette's accent"
	default:
		return "The colour of the cover that is playing"
	}
}
