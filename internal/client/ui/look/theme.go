package look

import (
	"image/color"

	"gioui.org/layout"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/anim"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
	libtheme "github.com/ryoku-dev/ryoku/docs/lithic/library/go/theme"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/tokens"
	"github.com/ryoku-dev/ryotunes/internal/model"
)

// Source contains the active provider's identity colors: the provider
// switcher's plate and glyphs, nothing else.
type Source struct {
	Provider         model.Provider
	Fill, Ink, Field color.NRGBA
}

// Accent is the one hue the player borrows: the play plaque face (Fill) and
// its ink (On), the accent as a glyph or text on stone (Ink), and the mineral
// wash of stone toward it (Field). The progress wave, the visualizer, the
// playing row, hearts and hero fields read it.
type Accent struct {
	Fill, On, Ink, Field color.NRGBA
}

const accentCacheSize = 32

type accentCacheKey struct {
	sample color.NRGBA
	stone2 color.NRGBA
	dark   bool
}

type accentCacheEntry struct {
	key    accentCacheKey
	accent Accent
	used   bool
}

// Theme is the shared Lithic theme plus Ryotunes source colors and frame class.
type Theme struct {
	*libtheme.Theme
	Src    Source
	Accent Accent
	Class  Class

	loadedRoles palette.Roles

	srcFrom, srcTo Source
	srcTween       anim.Tween
	providerSet    bool

	accentFrom, accentTo Accent
	accentTween          anim.Tween
	accentSet            bool

	cornerTween anim.Tween
	cornerSet   bool

	accentCache     [accentCacheSize]accentCacheEntry
	accentCacheNext int
}

// NewTheme constructs the official Lithic theme with Ryotunes fonts.
func NewTheme(roles palette.Roles, density libtheme.Density, provider ...model.Provider) *Theme {
	base := libtheme.New(roles, libtheme.WithFontCollection(Fonts()))
	base.Density = density
	var initial model.Provider
	if len(provider) > 0 {
		initial = provider[0]
	}
	source := sourceFor(initial, roles)
	accent := AccentFromDesktop(roles)
	theme := &Theme{
		Theme:       base,
		Src:         source,
		Accent:      accent,
		loadedRoles: roles,
		srcFrom:     source,
		srcTo:       source,
		srcTween:    anim.Tween{Duration: base.Motion.Field, Ease: anim.EaseChisel},
		accentFrom:  accent,
		accentTo:    accent,
		accentTween: anim.Tween{Duration: base.Motion.Field, Ease: anim.EaseChisel},
		cornerTween: anim.Tween{Duration: base.Motion.Select, Ease: anim.EaseChisel},
		providerSet: initial != "",
	}
	theme.applyAccent(accent)
	return theme
}

// LoadedRoles returns the untouched roles loaded from the current palette.
func (theme *Theme) LoadedRoles() palette.Roles {
	return theme.loadedRoles
}

// SetRoles replaces palette roles and makes the next accent assignment snap.
func (theme *Theme) SetRoles(roles palette.Roles) {
	theme.loadedRoles = roles
	theme.R = roles
	theme.Src = sourceFor(theme.Src.Provider, roles)
	theme.srcFrom, theme.srcTo = theme.Src, theme.Src
	theme.srcTween = anim.Tween{Duration: theme.Motion.Field, Ease: anim.EaseChisel}
	theme.accentSet = false
	theme.accentCache = [accentCacheSize]accentCacheEntry{}
	theme.accentCacheNext = 0
	theme.applyAccent(theme.Accent)
}

// ArtAccent derives and caches a cover accent for the loaded palette.
func (theme *Theme) ArtAccent(sample color.NRGBA) Accent {
	key := accentCacheKey{sample: sample, stone2: theme.loadedRoles.Stone2, dark: theme.loadedRoles.Dark}
	for index := range theme.accentCache {
		entry := theme.accentCache[index]
		if entry.used && entry.key == key {
			return entry.accent
		}
	}
	accent := AccentFromSample(sample, theme.loadedRoles)
	theme.accentCache[theme.accentCacheNext] = accentCacheEntry{key: key, accent: accent, used: true}
	theme.accentCacheNext = (theme.accentCacheNext + 1) % len(theme.accentCache)
	return accent
}

// SetProvider snaps the first source assignment and transitions later switches.
func (theme *Theme) SetProvider(gtx layout.Context, provider model.Provider) {
	target := sourceFor(provider, theme.loadedRoles)
	if !theme.providerSet {
		theme.providerSet = true
		theme.Src, theme.srcFrom, theme.srcTo = target, target, target
		theme.srcTween = anim.Tween{Duration: theme.Motion.Field, Ease: anim.EaseChisel}
		return
	}
	theme.animateSource(gtx)
	if theme.Src.Provider == provider && theme.srcTo == target {
		return
	}
	theme.srcFrom, theme.srcTo = theme.Src, target
	theme.srcTween = anim.Tween{Duration: theme.Motion.Field, Ease: anim.EaseChisel}
	theme.srcTween.Set(gtx, 0)
	theme.srcTween.Set(gtx, 1)
	theme.animateSource(gtx)
}

// SetAccent snaps the first assignment and transitions later music changes.
func (theme *Theme) SetAccent(gtx layout.Context, target Accent) {
	if !theme.accentSet {
		theme.accentSet = true
		theme.accentFrom, theme.accentTo = target, target
		theme.accentTween = anim.Tween{Duration: theme.Motion.Field, Ease: anim.EaseChisel}
		theme.applyAccent(target)
		return
	}
	theme.animateAccent(gtx)
	if theme.accentTo == target {
		return
	}
	theme.accentFrom, theme.accentTo = theme.Accent, target
	theme.accentTween = anim.Tween{Duration: theme.Motion.Field, Ease: anim.EaseChisel}
	theme.accentTween.Set(gtx, 0)
	theme.accentTween.Set(gtx, 1)
	theme.animateAccent(gtx)
}

// SetCorner snaps the first pane cut and transitions later preference changes.
func (theme *Theme) SetCorner(gtx layout.Context, target unit.Dp) {
	if !theme.cornerSet {
		theme.cornerSet = true
		theme.cornerTween = anim.Tween{Duration: theme.Motion.Select, Ease: anim.EaseChisel}
		theme.cornerTween.Set(gtx, float32(target))
		theme.Cut.Large = target
		return
	}
	theme.cornerTween.Set(gtx, float32(target))
	theme.Cut.Large = unit.Dp(theme.cornerTween.Value(gtx))
}

// Animate advances the active provider transition.
func (theme *Theme) Animate(gtx layout.Context) {
	theme.animateSource(gtx)
	theme.animateAccent(gtx)
	if theme.cornerSet {
		theme.Cut.Large = unit.Dp(theme.cornerTween.Value(gtx))
	}
}

func (theme *Theme) animateSource(gtx layout.Context) {
	progress := theme.srcTween.Value(gtx)
	theme.Src = Source{
		Provider: theme.srcTo.Provider,
		Fill:     palette.Mix(theme.srcFrom.Fill, theme.srcTo.Fill, progress),
		Ink:      palette.Mix(theme.srcFrom.Ink, theme.srcTo.Ink, progress),
		Field:    palette.Mix(theme.srcFrom.Field, theme.srcTo.Field, progress),
	}
}

func (theme *Theme) animateAccent(gtx layout.Context) {
	if !theme.accentSet {
		return
	}
	progress := theme.accentTween.Value(gtx)
	theme.applyAccent(Accent{
		Fill:  palette.Mix(theme.accentFrom.Fill, theme.accentTo.Fill, progress),
		On:    palette.Mix(theme.accentFrom.On, theme.accentTo.On, progress),
		Ink:   palette.Mix(theme.accentFrom.Ink, theme.accentTo.Ink, progress),
		Field: palette.Mix(theme.accentFrom.Field, theme.accentTo.Field, progress),
	})
}

func (theme *Theme) applyAccent(accent Accent) {
	theme.Accent = accent
	theme.R.Gold = accent.Fill
	theme.R.OnGold = accent.On
	theme.R.GoldInk = accent.Ink
}

func sourceFor(provider model.Provider, roles palette.Roles) Source {
	fill := BrandFill(provider, roles)
	return Source{
		Provider: provider,
		Fill:     fill,
		Ink:      BrandInk(provider, roles),
		Field:    palette.Mix(roles.Stone2, fill, tokens.AlphaMineral),
	}
}
