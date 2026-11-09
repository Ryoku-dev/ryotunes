// This file resolves Ryoku's XDG palette into Lithic's semantic roles.
package lithic

import (
	"encoding/hex"
	"encoding/json"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Mode controls whether neutral roles follow the palette or use a forced signature mode.
type Mode int

// Palette modes select automatic, forced-dark, or forced-light neutrals.
const (
	Follow Mode = iota
	Dark
	Light
)

// Palette is the decoded Matugen role set and its provenance.
type Palette struct {
	Roles  map[string]color.NRGBA
	Dark   bool
	Forced bool
	Source string
}

// LoadPalette follows Ryoku's source precedence, with an optional app palette override.
func LoadPalette(mode Mode, paletteFile ...string) Palette {
	configHome := xdgHome("XDG_CONFIG_HOME", ".config")
	cacheHome := xdgHome("XDG_CACHE_HOME", ".cache")
	named := nestedRoles(readObject(filepath.Join(configHome, "ryoku", "shell.json")), "themePalette")
	follow := true
	if theme := readObject(filepath.Join(configHome, "ryoku", "theme.json")); theme != nil {
		if value, ok := theme["followWallpaper"].(bool); ok {
			follow = value
		}
	}

	roles := make(map[string]color.NRGBA)
	source := "signature"
	if follow || mode != Follow {
		wallpaper := colorRoles(readObject(filepath.Join(cacheHome, "ryoku", "colors.json")))
		for key, value := range wallpaper {
			roles[key] = value
		}
		if len(wallpaper) != 0 {
			source = "wallpaper"
		}
	}
	for key, value := range named {
		roles[key] = value
	}
	if len(named) != 0 {
		source = "scheme"
	}
	if override := colorRoles(readObject(paletteOverride(paletteFile))); len(override) != 0 {
		for key, value := range override {
			roles[key] = value
		}
		source = "scheme"
	}
	if source == "signature" {
		if accent, ok := editionColor("/etc/os-release"); ok {
			roles["primary"] = accent
		}
	}

	forced := mode != Follow
	dark := mode != Light
	if !forced {
		dark = true
		if surface, ok := roles["surface"]; ok {
			luma := (0.299*float64(surface.R) + 0.587*float64(surface.G) + 0.114*float64(surface.B)) / 255
			dark = luma <= 0.5
		}
	}
	return Palette{Roles: roles, Dark: dark, Forced: forced, Source: source}
}

// FromPalette maps Matugen roles to Lithic planes, selection, action, and state roles.
func FromPalette(p Palette) Roles {
	r := Signature(p.Dark)
	if p.Source != "wallpaper" && p.Source != "scheme" {
		accent, ok := p.Roles["primary"]
		if !ok {
			return r
		}
		r.Gold = accent
		r.OnGold = highestContrastInk(accent)
		r.GoldInk = contrastTone(accent, r.Stone2, p.Dark)
		return r
	}

	role := func(name string, fallback color.NRGBA) color.NRGBA {
		if resolved, ok := p.Roles[name]; ok {
			return resolved
		}
		return fallback
	}
	if !p.Forced {
		r.Fg = role("onSurface", r.Fg)
		r.Fg2 = role("onSurfaceVariant", r.Fg2)
		r.Fg3 = role("outline", r.Fg3)
		if p.Dark {
			r.Ground = role("surfaceContainerLowest", r.Ground)
			r.Stone1 = role("surface", r.Stone1)
			r.Stone2 = role("surfaceContainerLow", r.Stone2)
			r.Stone3 = role("surfaceContainer", r.Stone3)
			r.Stone4 = role("surfaceContainerHigh", r.Stone4)
			r.Vein = role("surfaceContainerHighest", r.Vein)
			if scrim, ok := p.Roles["scrim"]; ok {
				r.Scrim = WithAlpha(scrim, 0.62)
			}
		} else {
			r.Ground = role("surfaceContainerHighest", r.Ground)
			r.Stone1 = role("surfaceContainerHigh", r.Stone1)
			r.Stone2 = role("surfaceContainerLowest", r.Stone2)
			r.Stone3 = role("surfaceContainer", r.Stone3)
			r.Stone4 = role("surfaceContainerLowest", r.Stone4)
			r.Vein = role("outlineVariant", r.Vein)
			if scrim, ok := p.Roles["scrim"]; ok {
				r.Scrim = WithAlpha(scrim, 0.45)
			}
		}
	}

	r.Gold = role("primary", r.Gold)
	r.OnGold = role("onPrimary", r.OnGold)
	r.GoldInk = contrastTone(r.Gold, r.Stone2, p.Dark)
	r.Sel = role("secondaryContainer", r.Sel)
	r.OnSel = role("onSecondaryContainer", r.OnSel)
	r.OnSel2 = Mix(r.OnSel, r.Sel, 0.45)
	r.Jasper = role("error", r.Jasper)
	r.OnJasper = role("onError", highestContrastInk(r.Jasper))
	r.JasperInk = contrastTone(r.Jasper, r.Stone2, p.Dark)
	return r
}

func paletteOverride(paths []string) string {
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		if filepath.IsAbs(path) {
			return path
		}
		return filepath.Join(xdgHome("XDG_CONFIG_HOME", ".config"), "ryotunes", filepath.Clean(path))
	}
	return ""
}

func xdgHome(env, suffix string) string {
	if value := os.Getenv(env); value != "" && filepath.IsAbs(value) {
		return value
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join("/", suffix)
	}
	return filepath.Join(home, suffix)
}

func readObject(path string) map[string]any {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var object map[string]any
	if json.Unmarshal(data, &object) != nil {
		return nil
	}
	return object
}

func nestedRoles(object map[string]any, key string) map[string]color.NRGBA {
	if object == nil {
		return nil
	}
	nested, ok := object[key].(map[string]any)
	if !ok {
		return nil
	}
	return colorRoles(nested)
}

func colorRoles(object map[string]any) map[string]color.NRGBA {
	roles := make(map[string]color.NRGBA)
	for key, raw := range object {
		value, ok := raw.(string)
		if !ok {
			continue
		}
		if parsed, ok := parseColor(value); ok {
			roles[key] = parsed
		}
	}
	return roles
}

func parseColor(value string) (color.NRGBA, bool) {
	if (len(value) != 7 && len(value) != 9) || value[0] != '#' {
		return color.NRGBA{}, false
	}
	decoded, err := hex.DecodeString(value[1:])
	if err != nil {
		return color.NRGBA{}, false
	}
	out := color.NRGBA{R: decoded[0], G: decoded[1], B: decoded[2], A: 255}
	if len(decoded) == 4 {
		out.A = decoded[3]
	}
	return out, true
}

func editionColor(path string) (color.NRGBA, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return color.NRGBA{}, false
	}
	var id, idLike string
	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch strings.TrimSpace(key) {
		case "ID":
			id = osReleaseValue(value)
		case "ID_LIKE":
			idLike = osReleaseValue(value)
		}
	}
	if value, ok := edition(strings.ToLower(id)); ok {
		return value, true
	}
	for like := range strings.FieldsSeq(strings.ToLower(idLike)) {
		if value, ok := edition(like); ok {
			return value, true
		}
	}
	return color.NRGBA{}, false
}

func osReleaseValue(value string) string {
	value = strings.TrimSpace(value)
	if len(value) < 2 {
		return value
	}
	if value[0] == '"' && value[len(value)-1] == '"' {
		if decoded, err := strconv.Unquote(value); err == nil {
			return decoded
		}
	}
	if value[0] == '\'' && value[len(value)-1] == '\'' {
		return value[1 : len(value)-1]
	}
	return value
}

// edition returns Ryoku's distro accent for signature fallback.
func edition(id string) (color.NRGBA, bool) {
	var value uint32
	switch id {
	case "arch":
		value = 1545169
	case "cachyos":
		value = 45985
	case "fedora":
		value = 5350106
	case "void":
		value = 4685921
	case "nixos":
		value = 8305380
	case "ubuntu":
		value = 15291424
	case "debian":
		value = 14092883
	case "opensuse":
		value = 7584293
	case "manjaro":
		value = 3522396
	case "endeavouros":
		value = 8339391
	case "linuxmint":
		value = 8830531
	case "pop":
		value = 4766151
	case "gentoo":
		value = 8022973
	case "alpine":
		value = 884656
	default:
		return color.NRGBA{}, false
	}
	return rgb(value), true
}

func highestContrastInk(background color.NRGBA) color.NRGBA {
	black, white := color.NRGBA{A: 255}, color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	if contrastRatio(black, background) >= contrastRatio(white, background) {
		return black
	}
	return white
}

func contrastTone(ink, background color.NRGBA, lighter bool) color.NRGBA {
	if contrastRatio(ink, background) >= 4.5 {
		return ink
	}
	hue, saturation, lightness := rgbToHSL(ink)
	for step := 1; step <= 255; step++ {
		at := float64(step) / 255
		next := lightness * (1 - at)
		if lighter {
			next = lightness + (1-lightness)*at
		}
		candidate := hslToNRGBA(hue, saturation, next, ink.A)
		if contrastRatio(candidate, background) >= 4.5 {
			return candidate
		}
	}
	if lighter {
		return color.NRGBA{R: 255, G: 255, B: 255, A: ink.A}
	}
	return color.NRGBA{A: ink.A}
}

func contrastRatio(a, b color.NRGBA) float64 {
	lighter, darker := relativeLuminance(a), relativeLuminance(b)
	if lighter < darker {
		lighter, darker = darker, lighter
	}
	return (lighter + 0.05) / (darker + 0.05)
}

func relativeLuminance(c color.NRGBA) float64 {
	channel := func(value uint8) float64 {
		v := float64(value) / 255
		if v <= 0.04045 {
			return v / 12.92
		}
		return math.Pow((v+0.055)/1.055, 2.4)
	}
	return 0.2126*channel(c.R) + 0.7152*channel(c.G) + 0.0722*channel(c.B)
}

func rgbToHSL(c color.NRGBA) (hue, saturation, lightness float64) {
	red, green, blue := float64(c.R)/255, float64(c.G)/255, float64(c.B)/255
	highest, lowest := max(red, green, blue), min(red, green, blue)
	lightness = (highest + lowest) / 2
	delta := highest - lowest
	if delta == 0 {
		return 0, 0, lightness
	}
	saturation = delta / (1 - math.Abs(2*lightness-1))
	switch highest {
	case red:
		hue = math.Mod((green-blue)/delta, 6)
	case green:
		hue = (blue-red)/delta + 2
	default:
		hue = (red-green)/delta + 4
	}
	hue /= 6
	if hue < 0 {
		hue++
	}
	return hue, saturation, lightness
}

func hslToNRGBA(hue, saturation, lightness float64, alpha uint8) color.NRGBA {
	chroma := (1 - math.Abs(2*lightness-1)) * saturation
	x := chroma * (1 - math.Abs(math.Mod(hue*6, 2)-1))
	offset := lightness - chroma/2
	var red, green, blue float64
	switch int(hue * 6) {
	case 0:
		red, green = chroma, x
	case 1:
		red, green = x, chroma
	case 2:
		green, blue = chroma, x
	case 3:
		green, blue = x, chroma
	case 4:
		red, blue = x, chroma
	default:
		red, blue = chroma, x
	}
	channel := func(value float64) uint8 {
		return uint8(math.Round(min(max(value+offset, 0), 1) * 255))
	}
	return color.NRGBA{R: channel(red), G: channel(green), B: channel(blue), A: alpha}
}
