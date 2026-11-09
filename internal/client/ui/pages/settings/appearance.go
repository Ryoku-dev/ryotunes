package settings

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"strings"

	"gioui.org/layout"

	"github.com/ryoku-dev/ryotunes/internal/client/lithic"
	"github.com/ryoku-dev/ryotunes/internal/client/state"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/model"
)

var (
	appearanceLabels = []string{"Follow desktop", "Dark", "Light"}
	appearanceValues = []string{"desktop", "dark", "light"}
	densityLabels    = []string{"Comfortable", "Compact"}
	densityValues    = []string{"comfortable", "compact"}
)

func (p *page) layoutAppearance(gtx layout.Context, e *env.Env, snapshot state.State) layout.Dimensions {
	preferences := snapshot.Client
	p.appearance.palette.sync(preferences.PaletteFile)
	p.appearance.palette.field.Mono = true

	mode := indexOf(appearanceValues, preferences.Appearance, 0)
	density := indexOf(densityValues, preferences.Density, 0)
	scale := settingInt(snapshot.Settings, model.SettingUIScale, 100)
	romanize := settingBool(snapshot.Settings, model.SettingLyricsRomanize, false)

	return settingRows(gtx,
		settingRow(e.Th, "Colour mode", "Follow the Ryoku desktop, or pin dark or light neutral stones.", func(gtx layout.Context) layout.Dimensions {
			dims := p.appearance.mode.Layout(gtx, e.Th, "", appearanceLabels, mode)
			if selected, ok := p.appearance.mode.Changed(gtx); ok {
				preferences.Appearance = appearanceValues[selected]
				saveClientPreferences(e, preferences)
				applyAppearance(e, preferences.Appearance, preferences.PaletteFile)
			}
			return dims
		}),
		settingRow(e.Th, "Density", "Compact changes resource rows from 56 dp to 48 dp. Controls and type stay the same.", func(gtx layout.Context) layout.Dimensions {
			dims := p.appearance.density.Layout(gtx, e.Th, "", densityLabels, density)
			if selected, ok := p.appearance.density.Changed(gtx); ok {
				preferences.Density = densityValues[selected]
				if preferences.Density == "compact" {
					e.Th.Density = lithic.Compact
				} else {
					e.Th.Density = lithic.Comfortable
				}
				saveClientPreferences(e, preferences)
			}
			return dims
		}),
		settingRow(e.Th, "Reduce motion", "Make selection, hover and state changes immediate. Nothing animates at rest either way.", func(gtx layout.Context) layout.Dimensions {
			p.appearance.reduce.Value = preferences.ReduceMotion
			before := p.appearance.reduce.Value
			dims := p.appearance.reduce.Layout(gtx, e.Th, "")
			if p.appearance.reduce.Value != before {
				preferences.ReduceMotion = p.appearance.reduce.Value
				lithic.SetReduce(preferences.ReduceMotion)
				saveClientPreferences(e, preferences)
			}
			return dims
		}),
		toggleSettingRow(e, &p.appearance.lowResource, snapshot.Settings, model.SettingLowResourceMode, false,
			"Low resource mode", "Disable speculative stream warming and reduce automatic Home, network work and decorative motion."),
		settingRow(e.Th, "Interface scale", "Preferred renderer scale, persisted for every Ryotunes client on this account.", func(gtx layout.Context) layout.Dimensions {
			p.appearance.scale.Value = scale
			p.appearance.scale.Min = 80
			p.appearance.scale.Max = 140
			p.appearance.scale.Step = 10
			dims := p.appearance.scale.Layout(gtx, e.Th, "%")
			if p.appearance.scale.Changed() {
				setSetting(e, model.SettingUIScale, intString(p.appearance.scale.Value), "", nil)
			}
			return dims
		}),
		settingRow(e.Th, "Romanized lyrics", "Show romanized readings when the lyrics service provides them.", func(gtx layout.Context) layout.Dimensions {
			p.appearance.romanize.Value = romanize
			before := p.appearance.romanize.Value
			dims := p.appearance.romanize.Layout(gtx, e.Th, "")
			if p.appearance.romanize.Value != before {
				setSetting(e, model.SettingLyricsRomanize, boolString(p.appearance.romanize.Value), "", nil)
			}
			return dims
		}),
		settingRow(e.Th, "Palette file", "Optional Material role-set file. Leave blank to use the live Ryoku palette.", func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical, Alignment: layout.End, Gap: gtx.Dp(8)}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return p.appearance.palette.field.Layout(gtx, e.Th, "", "/path/to/colors.json", "", "")
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if p.appearance.savePalette.Clicked(gtx) {
						path := strings.TrimSpace(p.appearance.palette.field.Text())
						if err := validatePaletteFile(path); err != nil {
							e.Toast(err.Error(), env.ToastError)
						} else {
							preferences.PaletteFile = path
							saveClientPreferences(e, preferences)
							applyAppearance(e, preferences.Appearance, preferences.PaletteFile)
						}
					}
					return p.appearance.savePalette.Layout(gtx, e.Th, lithic.Line, nil, "Use palette")
				}),
			)
		}),
	)
}

func applyAppearance(e *env.Env, value, paletteFile string) {
	mode := lithic.Follow
	if value == "dark" {
		mode = lithic.Dark
	} else if value == "light" {
		mode = lithic.Light
	}
	e.Th.R = lithic.FromPalette(lithic.LoadPalette(mode, paletteFile))
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

func intString(value int) string {
	return strconv.Itoa(value)
}
