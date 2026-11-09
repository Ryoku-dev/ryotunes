// This file defines Lithic's exact type, cut, spacing, density, and motion scales.
package lithic

import (
	_ "embed"
	"sync"
	"time"

	"gioui.org/font"
	"gioui.org/font/opentype"
	"gioui.org/text"
	"gioui.org/unit"
)

// Sans and Mono are the two embedded Lithic typefaces.
const (
	Sans = "Space Grotesk"
	Mono = "Space Mono"
)

// TextStyle fixes a face, size, and line height from the Lithic type scale.
type TextStyle struct {
	Face font.Font
	Size unit.Sp
	Line unit.Sp
}

// Type separates human-readable text from literal monospace data.
type Type struct {
	Name     TextStyle
	Sentence TextStyle
	Section  TextStyle
	Title    TextStyle
	Row      TextStyle
	Body     TextStyle
	Proof    TextStyle
	Label    TextStyle
	Caption  TextStyle
	Group    TextStyle
	Mono     TextStyle
	MonoSm   TextStyle
}

func defaultType() Type {
	sans := func(weight font.Weight, size, line float32) TextStyle {
		return TextStyle{Face: font.Font{Typeface: Sans, Weight: weight}, Size: unit.Sp(size), Line: unit.Sp(line)}
	}
	mono := func(size, line float32) TextStyle {
		return TextStyle{Face: font.Font{Typeface: Mono}, Size: unit.Sp(size), Line: unit.Sp(line)}
	}
	return Type{
		Name: sans(font.Bold, 52, 56), Sentence: sans(font.Bold, 34, 40),
		Section: sans(font.Bold, 21, 28), Title: sans(font.Medium, 15, 20),
		Row: sans(font.Medium, 15, 20), Body: sans(font.Normal, 14, 20),
		Proof: sans(font.Normal, 15, 21), Label: sans(font.Medium, 13, 16),
		Caption: sans(font.Normal, 12, 16), Group: sans(font.Medium, 12, 16),
		Mono: mono(12, 16), MonoSm: mono(11, 15),
	}
}

// Cut contains the 45-degree cut sizes for each component scale.
type Cut struct {
	Token, Toggle, Small, Mid unit.Dp
	Notch, Large              unit.Dp
	Menu, Placard, Tooltip    unit.Dp
}

// Space is Lithic's non-linear spacing run: 4, 8, 13, 21, 34, 55.
type Space struct {
	S1, S2, S3, S4, S5, S6 unit.Dp
}

// Density controls only tablet height.
type Density int

// Densities select comfortable or compact tablet height.
const (
	Comfortable Density = iota
	Compact
)

// Motion is the duration scale; EaseChisel is the only curve.
type Motion struct {
	Press, HoverIn, HoverOut, FocusIn, FocusOut time.Duration
	Select, Field, Notch, Unfold, Navigate      time.Duration
	MenuIn, MenuOut, DialogIn, DialogOut        time.Duration
	TooltipIn, TooltipDelay, PlacardIn          time.Duration
	PlacardLife, Wheel, Working, Sweep          time.Duration
}

// Theme owns every Lithic type, shape, spacing, motion, and density token.
type Theme struct {
	R       Roles
	T       Type
	Cut     Cut
	Sp      Space
	Motion  Motion
	Density Density
	Shaper  *text.Shaper
}

// New constructs a complete Lithic theme for roles and an optional shaper.
func New(roles Roles, shaper *text.Shaper) *Theme {
	if shaper == nil {
		shaper = Shaper()
	}
	return &Theme{
		R: roles, T: defaultType(),
		Cut: Cut{
			Token: 3, Toggle: 4, Small: 6, Mid: 7, Notch: 12, Large: 16,
			Menu: 8, Placard: 8, Tooltip: 4,
		},
		Sp: Space{S1: 4, S2: 8, S3: 13, S4: 21, S5: 34, S6: 55},
		Motion: Motion{
			Press: 90 * time.Millisecond, HoverIn: 160 * time.Millisecond, HoverOut: 100 * time.Millisecond,
			FocusIn: 120 * time.Millisecond, FocusOut: 100 * time.Millisecond,
			Select: 240 * time.Millisecond, Field: 320 * time.Millisecond, Notch: 200 * time.Millisecond,
			Unfold: 200 * time.Millisecond, Navigate: 200 * time.Millisecond,
			MenuIn: 160 * time.Millisecond, MenuOut: 100 * time.Millisecond,
			DialogIn: 300 * time.Millisecond, DialogOut: 160 * time.Millisecond,
			TooltipIn: 300 * time.Millisecond, TooltipDelay: 600 * time.Millisecond,
			PlacardIn: 300 * time.Millisecond, PlacardLife: 5 * time.Second,
			Wheel: 140 * time.Millisecond, Working: 900 * time.Millisecond, Sweep: 1280 * time.Millisecond,
		},
		Shaper: shaper,
	}
}

// RowHeight returns the tablet height for the active density.
func (t *Theme) RowHeight() unit.Dp {
	if t.Density == Compact {
		return 48
	}
	return 56
}

var (
	//go:embed fonts/SpaceGrotesk-Regular.otf
	spaceGroteskRegular []byte
	//go:embed fonts/SpaceGrotesk-Medium.otf
	spaceGroteskMedium []byte
	//go:embed fonts/SpaceGrotesk-Bold.otf
	spaceGroteskBold []byte
	//go:embed fonts/SpaceMono-Regular.ttf
	spaceMonoRegular []byte
	//go:embed fonts/SpaceMono-Bold.ttf
	spaceMonoBold  []byte
	collectionOnce sync.Once
	collection     []font.FontFace
)

func fontCollection() []font.FontFace {
	collectionOnce.Do(func() {
		faces := []struct {
			data     []byte
			typeface font.Typeface
			weight   font.Weight
		}{
			{spaceGroteskRegular, Sans, font.Normal},
			{spaceGroteskMedium, Sans, font.Medium},
			{spaceGroteskBold, Sans, font.Bold},
			{spaceMonoRegular, Mono, font.Normal},
			{spaceMonoBold, Mono, font.Bold},
		}
		for _, embedded := range faces {
			parsed, err := opentype.ParseCollection(embedded.data)
			if err != nil {
				panic(err)
			}
			for i := range parsed {
				parsed[i].Font.Typeface = embedded.typeface
				parsed[i].Font.Weight = embedded.weight
			}
			collection = append(collection, parsed...)
		}
	})
	return collection
}

// Shaper registers only the embedded Space Grotesk and Space Mono faces.
func Shaper() *text.Shaper {
	return text.NewShaper(text.NoSystemFonts(), text.WithCollection(fontCollection()))
}
