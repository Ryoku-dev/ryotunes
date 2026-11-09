// This file defines the palette-derived roles and state tones.
package lithic

import "image/color"

// Tone is a state meaning, independent of its rendered colour.
type Tone int

// Tones cover idle, healthy, working, attention, paused, and failed states.
const (
	Idle Tone = iota
	Alive
	Busy
	Attention
	Paused
	Bad
)

// Roles are semantic colours resolved from the live Ryoku palette.
type Roles struct {
	Ground color.NRGBA
	Stone1 color.NRGBA
	Stone2 color.NRGBA
	Stone3 color.NRGBA
	Stone4 color.NRGBA
	Vein   color.NRGBA

	Fg  color.NRGBA
	Fg2 color.NRGBA
	Fg3 color.NRGBA

	Sel    color.NRGBA
	OnSel  color.NRGBA
	OnSel2 color.NRGBA

	Gold    color.NRGBA
	OnGold  color.NRGBA
	GoldInk color.NRGBA

	Jade      color.NRGBA
	Lapis     color.NRGBA
	Amber     color.NRGBA
	Jasper    color.NRGBA
	OnJasper  color.NRGBA
	JasperInk color.NRGBA
	Scrim     color.NRGBA
	Dark      bool
}

func rgb(hex uint32) color.NRGBA {
	return color.NRGBA{R: uint8(hex >> 16), G: uint8(hex >> 8), B: uint8(hex), A: 255}
}

// WithAlpha returns c at alpha a of its current alpha.
func WithAlpha(c color.NRGBA, a float32) color.NRGBA {
	c.A = uint8(float32(c.A) * a)
	return c
}

// Mix blends a toward b by t, including alpha.
func Mix(a, b color.NRGBA, t float32) color.NRGBA {
	lerp := func(x, y uint8) uint8 { return uint8(float32(x) + (float32(y)-float32(x))*t + 0.5) }
	return color.NRGBA{R: lerp(a.R, b.R), G: lerp(a.G, b.G), B: lerp(a.B, b.B), A: lerp(a.A, b.A)}
}

// SetStones installs Lithic's fixed minerals and no-palette gold values.
func (r *Roles) SetStones() {
	if r.Dark {
		r.Jade, r.Lapis, r.Amber, r.Jasper = rgb(0x7ab8a0), rgb(0x8aa2d6), rgb(0xe0954a), rgb(0xe0553a)
		r.Gold, r.OnGold, r.GoldInk = rgb(0xc9a861), rgb(0x17120a), rgb(0xd6b877)
	} else {
		r.Jade, r.Lapis, r.Amber, r.Jasper = rgb(0x2f7a64), rgb(0x3d5f99), rgb(0x94520f), rgb(0xb8321e)
		r.Gold, r.OnGold, r.GoldInk = rgb(0xc9a45a), rgb(0x1d1b18), rgb(0x85651f)
	}
}

// Stone returns the mineral ink for a state.
func (r *Roles) Stone(t Tone) color.NRGBA {
	switch t {
	case Alive:
		return r.Jade
	case Busy:
		return r.Lapis
	case Attention, Paused:
		return r.Amber
	case Bad:
		return r.JasperInk
	default:
		return r.Vein
	}
}

// Field returns the mineral field behind a header in the given state.
func (r *Roles) Field(t Tone) color.NRGBA {
	switch t {
	case Alive:
		return Mix(r.Stone2, r.Jade, 0.17)
	case Busy, Attention, Paused:
		return Mix(r.Stone2, r.Stone(t), 0.15)
	case Bad:
		return Mix(r.Stone2, r.Jasper, 0.15)
	default:
		return r.Stone3
	}
}

// Signature is the only neutral fallback used when no Ryoku palette is available.
func Signature(dark bool) Roles {
	var r Roles
	if dark {
		r = Roles{
			Ground: rgb(0x0e0e0e), Stone1: rgb(0x141414), Stone2: rgb(0x1b1b1b),
			Stone3: rgb(0x232323), Stone4: rgb(0x2c2c2c), Vein: rgb(0x393939),
			Fg: rgb(0xe3e3e3), Fg2: rgb(0xc6c6c6), Fg3: rgb(0x8f8f8f),
			Sel: rgb(0xe3e3e3), OnSel: rgb(0x121212), OnSel2: rgb(0x474747),
			Gold: rgb(0xc9a861), OnGold: rgb(0x17120a), GoldInk: rgb(0xd6b877),
			Scrim: color.NRGBA{A: 0x9e}, Dark: true,
		}
	} else {
		r = Roles{
			Ground: rgb(0xe2ddd4), Stone1: rgb(0xeae6df), Stone2: rgb(0xf4f1eb),
			Stone3: rgb(0xe9e5de), Stone4: rgb(0xfbf9f5), Vein: rgb(0xcbc4b8),
			Fg: rgb(0x1d1b18), Fg2: rgb(0x4a4640), Fg3: rgb(0x6f6a62),
			Sel: rgb(0x1d1b18), OnSel: rgb(0xf4f1eb), OnSel2: rgb(0xb9b2a7),
			Gold: rgb(0xc9a45a), OnGold: rgb(0x1d1b18), GoldInk: rgb(0x85651f),
			Scrim: color.NRGBA{A: 0x73},
		}
	}
	r.SetStones()
	r.OnJasper = highestContrastInk(r.Jasper)
	r.JasperInk = contrastTone(r.Jasper, r.Stone2, r.Dark)
	return r
}
