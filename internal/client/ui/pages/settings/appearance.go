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
	appearanceLabels = []string{"Follow Ryoku", "Dark", "Light"}
	appearanceValues = []string{"follow", "dark", "light"}
	densityLabels    = []string{"Comfortable", "Compact"}
	densityValues    = []string{"comfortable", "compact"}
	motionLabels     = []string{"Full", "Reduced"}
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
	return settingRows(gtx,
		settingRow(e.Th, "Colours", "Follow Ryoku, or pin dark or light stones while keeping the live palette accents.", func(gtx layout.Context) layout.Dimensions {
			dims := p.appearance.colours.Layout(gtx, e.Th, e.Overlays, appearanceLabels, mode)
			if selected, ok := p.appearance.colours.Changed(); ok {
				preferences.Appearance = appearanceValues[selected]
				saveClientPreferences(e, preferences)
				applyAppearance(e, preferences.Appearance, preferences.PaletteFile)
			}
			return dims
		}),
		settingRow(e.Th, "Palette file", "Choose a Ryoku palette JSON, or clear it to use the live desktop palette.", func(gtx layout.Context) layout.Dimensions {
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
		settingRow(e.Th, "Density", "Compact changes resource rows from 56 dp to 48 dp.", func(gtx layout.Context) layout.Dimensions {
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
		settingRow(e.Th, "Motion", "Reduced motion makes state and selection changes immediate.", func(gtx layout.Context) layout.Dimensions {
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
		settingRow(e.Th, "UI scale", "Preferred renderer scale, persisted for every Ryotunes client on this account.", func(gtx layout.Context) layout.Dimensions {
			p.appearance.scale.Value, p.appearance.scale.Min, p.appearance.scale.Max = scale, 80, 140
			p.appearance.scale.Step, p.appearance.scale.Unit = 10, "%"
			dims := p.appearance.scale.Layout(gtx, e.Th.Theme)
			if p.appearance.scale.Changed() {
				setSetting(e, model.SettingUIScale, intString(p.appearance.scale.Value), "", nil)
			}
			return dims
		}),
		toggleSettingRow(e, &p.appearance.lowResource, snapshot.Settings, model.SettingLowResourceMode, false,
			"Low resource mode", "Disable speculative stream warming and reduce automatic Home and network work."),
	)
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
