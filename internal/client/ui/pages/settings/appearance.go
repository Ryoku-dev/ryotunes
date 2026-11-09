package settings

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"strings"

	"gioui.org/layout"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/anim"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	libtheme "github.com/ryoku-dev/ryoku/docs/lithic/library/go/theme"
	"github.com/ryoku-dev/ryotunes/internal/client/state"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/model"
)

var (
	appearanceLabels         = []string{"Follow Ryoku", "Dark", "Light"}
	appearanceValues         = []string{"follow", "dark", "light"}
	densityLabels            = []string{"Comfortable", "Compact"}
	densityValues            = []string{"comfortable", "compact"}
	motionLabels             = []string{"Full", "Reduced"}
	visualizerStyleLabels    = []string{"Bars", "Curve", "Oscilloscope", "Vectorscope"}
	visualizerSpectrumLabels = visualizerStyleLabels[:2]
	visualizerScopeLabels    = visualizerStyleLabels[2:]
	accentLabels             = []string{"Album art", "Source", "Desktop"}
	accentValues             = []string{state.AccentArt, state.AccentSource, state.AccentDesktop}
	cornerLabels             = []string{"Desktop", "Square", "Fine", "Chisel", "Deep", "Custom"}
	cornerValues             = []string{state.CornersDesktop, state.CornersSquare, state.CornersFine, state.CornersChisel, state.CornersDeep, state.CornersCustom}
)

func (p *page) layoutAppearance(gtx layout.Context, e *env.Env, snapshot state.State) layout.Dimensions {
	preferences := snapshot.Client
	p.appearance.palette.sync(preferences.PaletteFile)
	mode := indexOf(appearanceValues, preferences.Appearance, 0)
	if preferences.Appearance == "desktop" {
		mode = 0
	}
	density := indexOf(densityValues, preferences.Density, 0)
	scale := settingInt(snapshot.Settings, model.SettingUIScale, 100)
	lowResource := settingBool(snapshot.Settings, model.SettingLowResourceMode, false)
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
	if selectedVisualizerAvailable && preferences.ReduceMotion {
		visualizerDetail += " Reduced motion pauses it without changing this setting."
	} else if selectedVisualizerAvailable && lowResource {
		visualizerDetail += " Low resource mode pauses it without changing this setting."
	}
	styleLabels, styleValues := visualizerStyleOptions(spectrumInstalled, scopeInstalled)
	visualizerStyleDetail := visualizerStylePreferenceDetail(spectrumInstalled, scopeInstalled)
	waveDetail := "A sharp travelling wave marks the played part of every seek bar."
	if preferences.ReduceMotion {
		waveDetail += " Reduced motion pauses it without changing this setting."
	} else if lowResource {
		waveDetail += " Low resource mode pauses it without changing this setting."
	}
	accentMode := preferences.AccentMode()
	accentDetail := accentPreferenceDetail(accentMode)
	cornerPreset := preferences.CornerPreset()
	cornerDetail := "Set the cut of every pane corner."
	if cornerPreset == state.CornersDesktop {
		desktopCorner, _ := look.DesktopCorner()
		cornerDetail = "Ryoku window corners: " + intString(desktopCorner) + " dp"
	}
	return p.settingRows(gtx,
		p.settingRow(e.Th, "Colours", "Follow Ryoku, or pin dark or light stones while keeping the live palette accents.", func(gtx layout.Context) layout.Dimensions {
			dims := p.appearance.colours.Layout(gtx, e.Th, e.Overlays, appearanceLabels, mode)
			if selected, ok := p.appearance.colours.Changed(); ok {
				preferences.Appearance = appearanceValues[selected]
				saveClientPreferences(e, preferences)
				applyAppearance(e, preferences.Appearance, preferences.PaletteFile)
			}
			return dims
		}),
		p.settingRow(e.Th, "Palette file", "Choose a Ryoku palette JSON, or clear it to use the live desktop palette.", func(gtx layout.Context) layout.Dimensions {
			p.appearance.palette.field.Mono = true
			p.appearance.palette.field.Placeholder = "/path/to/colors.json"
			p.appearance.usePalette.Kind, p.appearance.usePalette.Label = kit.PlaqueLine, "Use palette"
			p.appearance.clearPalette.Kind, p.appearance.clearPalette.Label = kit.PlaqueQuiet, "None"
			if p.appearance.usePalette.Clicked(gtx) {
				path := strings.TrimSpace(p.appearance.palette.field.Text())
				if err := validatePaletteFile(path); err != nil {
					e.Toast(err.Error(), env.ToastError)
				} else {
					preferences.PaletteFile = path
					saveClientPreferences(e, preferences)
					applyAppearance(e, preferences.Appearance, path)
				}
			}
			if p.appearance.clearPalette.Clicked(gtx) {
				preferences.PaletteFile = ""
				p.appearance.palette.field.SetText("")
				saveClientPreferences(e, preferences)
				applyAppearance(e, preferences.Appearance, "")
			}
			return layout.Flex{Axis: layout.Vertical, Alignment: layout.End}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return p.appearance.palette.field.Layout(gtx, e.Th.Theme) }),
				layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions { return p.appearance.usePalette.Layout(gtx, e.Th.Theme) }),
						layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions { return p.appearance.clearPalette.Layout(gtx, e.Th.Theme) }),
					)
				}),
			)
		}),
		p.settingRow(e.Th, "Density", "Compact changes resource rows from 56 dp to 48 dp.", func(gtx layout.Context) layout.Dimensions {
			dims := p.appearance.density.Layout(gtx, e.Th, e.Overlays, densityLabels, density)
			if selected, ok := p.appearance.density.Changed(); ok {
				preferences.Density = densityValues[selected]
				if preferences.Density == "compact" {
					e.Th.Density = libtheme.Compact
				} else {
					e.Th.Density = libtheme.Comfortable
				}
				saveClientPreferences(e, preferences)
			}
			return dims
		}),
		p.settingRow(e.Th, "Motion", "Reduced motion makes state and selection changes immediate and pauses visualizers and wave progress without changing their settings.", func(gtx layout.Context) layout.Dimensions {
			selected := 0
			if preferences.ReduceMotion {
				selected = 1
			}
			dims := p.appearance.motion.Layout(gtx, e.Th, e.Overlays, motionLabels, selected)
			if changed, ok := p.appearance.motion.Changed(); ok {
				preferences.ReduceMotion = changed == 1
				anim.SetReduce(preferences.ReduceMotion)
				saveClientPreferences(e, preferences)
			}
			return dims
		}),
		p.settingRow(e.Th, "UI scale", "Preferred renderer scale, persisted for every Ryotunes client on this account.", func(gtx layout.Context) layout.Dimensions {
			p.appearance.scale.Value, p.appearance.scale.Min, p.appearance.scale.Max = scale, 80, 140
			p.appearance.scale.Step, p.appearance.scale.Unit = 10, "%"
			dims := p.appearance.scale.Layout(gtx, e.Th.Theme)
			if p.appearance.scale.Changed() {
				setSetting(e, model.SettingUIScale, intString(p.appearance.scale.Value), "", nil)
			}
			return dims
		}),
		p.toggleSettingRow(e, &p.appearance.lowResource, snapshot.Settings, model.SettingLowResourceMode, false,
			"Low resource mode", "Reduce automatic Home and network work, and pause visualizers and wave progress without changing their settings."),
		p.settingRow(e.Th, "Visualizer", visualizerDetail, func(gtx layout.Context) layout.Dimensions {
			p.appearance.visualizer.Value = preferences.VisualizerOn()
			toggleGTX := gtx
			if !anyVisualizerInstalled {
				toggleGTX = toggleGTX.Disabled()
			}
			dims := p.appearance.visualizer.Layout(toggleGTX, e.Th.Theme)
			if anyVisualizerInstalled && p.appearance.visualizer.Changed() {
				value := p.appearance.visualizer.Value
				preferences.Visualizer = &value
				saveClientPreferences(e, preferences)
			}
			return dims
		}),
		p.settingRowWide(e.Th, "Visualizer style", visualizerStyleDetail, func(gtx layout.Context) layout.Dimensions {
			p.appearance.visualizerStyle.Labels = styleLabels
			p.appearance.visualizerStyle.Selected = visualizerStyleIndex(styleValues, visualizerStyle)
			styleGTX := gtx
			if !preferences.VisualizerOn() || !anyVisualizerInstalled {
				styleGTX = styleGTX.Disabled()
			}
			return p.appearance.visualizerStyleScroll.Layout(styleGTX, e.Th, 1, func(styleGTX layout.Context, _ int) layout.Dimensions {
				dims := p.appearance.visualizerStyle.Layout(styleGTX, e.Th.Theme)
				if selected, changed := p.appearance.visualizerStyle.Changed(); changed && preferences.VisualizerOn() && selected >= 0 && selected < len(styleValues) {
					preferences.VisualizerStyle = string(styleValues[selected])
					saveClientPreferences(e, preferences)
				}
				return dims
			})
		}),
		p.settingRow(e.Th, "Wave progress", waveDetail, func(gtx layout.Context) layout.Dimensions {
			p.appearance.waveProgress.Value = preferences.WaveOn()
			dims := p.appearance.waveProgress.Layout(gtx, e.Th.Theme)
			if p.appearance.waveProgress.Changed() {
				value := p.appearance.waveProgress.Value
				preferences.WaveProgress = &value
				saveClientPreferences(e, preferences)
			}
			return dims
		}),
		p.settingRowWide(e.Th, "Accent colour", accentDetail, func(gtx layout.Context) layout.Dimensions {
			p.appearance.accent.Labels = accentLabels
			p.appearance.accent.Selected = indexOf(accentValues, accentMode, 0)
			return p.appearance.accentScroll.Layout(gtx, e.Th, 1, func(gtx layout.Context, _ int) layout.Dimensions {
				dims := p.appearance.accent.Layout(gtx, e.Th.Theme)
				if selected, changed := p.appearance.accent.Changed(); changed && selected >= 0 && selected < len(accentValues) {
					preferences.Accent = accentValues[selected]
					saveClientPreferences(e, preferences)
				}
				return dims
			})
		}),
		p.settingRowWide(e.Th, "Corners", cornerDetail, func(gtx layout.Context) layout.Dimensions {
			p.appearance.corners.Labels = cornerLabels
			p.appearance.corners.Selected = indexOf(cornerValues, cornerPreset, 3)
			return layout.Flex{Axis: layout.Vertical, Alignment: layout.End}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return p.appearance.cornersScroll.Layout(gtx, e.Th, 1, func(gtx layout.Context, _ int) layout.Dimensions {
						dims := p.appearance.corners.Layout(gtx, e.Th.Theme)
						if selected, changed := p.appearance.corners.Changed(); changed && selected >= 0 && selected < len(cornerValues) {
							previousPreset := cornerPreset
							cornerPreset = cornerValues[selected]
							preferences.Corners = cornerPreset
							if cornerPreset == state.CornersCustom {
								desktopCorner, _ := look.DesktopCorner()
								preferences.CornerDp = cornerDpForCustom(previousPreset, preferences.CornerDp, desktopCorner)
							}
							saveClientPreferences(e, preferences)
						}
						return dims
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if cornerPreset != state.CornersCustom {
						return layout.Dimensions{}
					}
					return layout.Inset{Top: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						p.appearance.corner.Min = 0
						p.appearance.corner.Max = state.CornerMaxDp
						p.appearance.corner.Step = 1
						p.appearance.corner.Value = float32(preferences.CornerDp)
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								dims := p.appearance.corner.Layout(gtx, e.Th.Theme)
								if p.appearance.corner.Changed() {
									preferences.CornerDp = int(p.appearance.corner.Value + .5)
									e.Store.SetClientPreferences(preferences)
									if p.appearance.corner.Pressed() {
										p.appearance.cornerSaveArmed = true
									} else {
										saveClientPreferences(e, preferences)
										p.appearance.cornerSaveArmed = false
									}
								} else if p.appearance.cornerSaveArmed && !p.appearance.corner.Pressed() {
									saveClientPreferences(e, preferences)
									p.appearance.cornerSaveArmed = false
								}
								return dims
							}),
							layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
							layout.Rigid(kit.Text(e.Th.Theme, e.Th.T.Mono, e.Th.R.Fg, intString(preferences.CornerDp)+" dp")),
						)
					})
				}),
			)
		}))
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

func cornerDpForCustom(previousPreset string, stored, desktop int) int {
	if stored != 0 {
		return stored
	}
	if previousPreset == state.CornersDesktop {
		return desktop
	}
	if dp, ok := state.CornerPresetDp(previousPreset); ok {
		return dp
	}
	return stored
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

func applyAppearance(e *env.Env, appearance, paletteFile string) {
	e.Th.SetRoles(look.Roles(appearance, paletteFile))
}

func validatePaletteFile(path string) error {
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return errors.New("Could not read that palette file.")
	}
	var value any
	if json.Unmarshal(data, &value) != nil {
		return errors.New("That palette file is not valid JSON.")
	}
	if !hasPaletteColour(value) {
		return errors.New("That palette file has no #rrggbb role values.")
	}
	return nil
}

func hasPaletteColour(value any) bool {
	switch value := value.(type) {
	case string:
		if len(value) != 7 || value[0] != '#' {
			return false
		}
		_, err := hex.DecodeString(value[1:])
		return err == nil
	case []any:
		for _, item := range value {
			if hasPaletteColour(item) {
				return true
			}
		}
	case map[string]any:
		for _, item := range value {
			if hasPaletteColour(item) {
				return true
			}
		}
	}
	return false
}
