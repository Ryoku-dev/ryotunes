package look

import (
	"image/color"

	"gioui.org/layout"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/anim"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
	libtheme "github.com/ryoku-dev/ryoku/docs/lithic/library/go/theme"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/tokens"
	"github.com/ryoku-dev/ryotunes/internal/client/art"
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

const schemeCacheSize = 32

type schemeCacheKey struct {
	cover  art.Palette
	base   palette.Roles
	tinted bool
}

type schemeCacheEntry struct {
	key    schemeCacheKey
	roles  palette.Roles
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

	schemeFrom, schemeTo palette.Roles
	accentFrom, accentTo Accent
	schemeTween          anim.Tween
	schemeSet            bool
	schemeAnimating      bool

	cornerTween anim.Tween
	cornerSet   bool

	schemeCache     [schemeCacheSize]schemeCacheEntry
	schemeCacheNext int
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
		schemeFrom:  roles,
		schemeTo:    roles,
		accentFrom:  accent,
		accentTo:    accent,
		schemeTween: anim.Tween{Duration: base.Motion.Field, Ease: anim.EaseChisel},
		cornerTween: anim.Tween{Duration: base.Motion.Select, Ease: anim.EaseChisel},
		providerSet: initial != "",
	}
	theme.applyScheme(roles, accent)
	return theme
}

// LoadedRoles returns the untouched roles loaded from the current palette.
func (theme *Theme) LoadedRoles() palette.Roles {
	return theme.loadedRoles
}

// SetRoles replaces palette roles and makes the next accent assignment snap.
func (theme *Theme) SetRoles(roles palette.Roles) {
	theme.loadedRoles = roles
	theme.Src = sourceFor(theme.Src.Provider, roles)
	theme.srcFrom, theme.srcTo = theme.Src, theme.Src
	theme.srcTween = anim.Tween{Duration: theme.Motion.Field, Ease: anim.EaseChisel}
	theme.schemeSet = false
	theme.schemeAnimating = false
	theme.schemeCache = [schemeCacheSize]schemeCacheEntry{}
	theme.schemeCacheNext = 0
	accent := AccentFromDesktop(roles)
	theme.schemeFrom, theme.schemeTo = roles, roles
	theme.accentFrom, theme.accentTo = accent, accent
	theme.schemeTween = anim.Tween{Duration: theme.Motion.Field, Ease: anim.EaseChisel}
	theme.applyScheme(roles, accent)
}

// ArtScheme derives and caches the cover-tinted roles and accent together.
func (theme *Theme) ArtScheme(cover art.Palette, tinted bool) (palette.Roles, Accent) {
	key := schemeCacheKey{cover: cover, base: theme.loadedRoles, tinted: tinted}
	for index := range theme.schemeCache {
		entry := theme.schemeCache[index]
		if entry.used && entry.key == key {
			return entry.roles, entry.accent
		}
	}
	roles := theme.loadedRoles
	if tinted {
		if tintedRoles, ok := CoverTint(roles, cover); ok {
			roles = tintedRoles
		}
	}
	accent := AccentFromDesktop(roles)
	if cover.Vibrant.A != 0 {
		accent = AccentFromSample(cover.Vibrant, roles)
	}
	theme.schemeCache[theme.schemeCacheNext] = schemeCacheEntry{
		key: key, roles: roles, accent: accent, used: true,
	}
	theme.schemeCacheNext = (theme.schemeCacheNext + 1) % len(theme.schemeCache)
	return roles, accent
}

// ArtAccent derives a hero accent without changing the window roles.
func (theme *Theme) ArtAccent(sample color.NRGBA) Accent {
	_, accent := theme.ArtScheme(art.Palette{Vibrant: sample}, false)
	return accent
}

// SetProvider snaps the first source assignment and transitions later switches.
func (theme *Theme) SetProvider(gtx layout.Context, provider model.Provider) {
	target := sourceFor(provider, theme.schemeTo)
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

// SetScheme transitions the window roles and accent as one field change.
func (theme *Theme) SetScheme(gtx layout.Context, roles palette.Roles, accent Accent) {
	targetSource := sourceFor(theme.srcTo.Provider, roles)
	if !theme.schemeSet {
		theme.schemeSet = true
		theme.schemeAnimating = false
		theme.schemeFrom, theme.schemeTo = roles, roles
		theme.accentFrom, theme.accentTo = accent, accent
		theme.schemeTween = anim.Tween{Duration: theme.Motion.Field, Ease: anim.EaseChisel}
		theme.Src, theme.srcFrom, theme.srcTo = targetSource, targetSource, targetSource
		theme.srcTween = anim.Tween{Duration: theme.Motion.Field, Ease: anim.EaseChisel}
		theme.applyScheme(roles, accent)
		return
	}
	if theme.schemeTo == roles && theme.accentTo == accent && theme.srcTo == targetSource {
		return
	}
	theme.animateScheme(gtx)
	theme.animateSource(gtx)
	theme.schemeFrom, theme.schemeTo = theme.R, roles
	theme.accentFrom, theme.accentTo = theme.Accent, accent
	theme.schemeTween = anim.Tween{Duration: theme.Motion.Field, Ease: anim.EaseChisel}
	theme.schemeTween.Set(gtx, 0)
	theme.schemeTween.Set(gtx, 1)
	theme.srcFrom, theme.srcTo = theme.Src, targetSource
	theme.srcTween = anim.Tween{Duration: theme.Motion.Field, Ease: anim.EaseChisel}
	theme.srcTween.Set(gtx, 0)
	theme.srcTween.Set(gtx, 1)
	theme.schemeAnimating = true
}

// SetAccent transitions an accent while restoring the untouched desktop roles.
func (theme *Theme) SetAccent(gtx layout.Context, accent Accent) {
	theme.SetScheme(gtx, theme.loadedRoles, accent)
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

// Animate advances active theme transitions.
func (theme *Theme) Animate(gtx layout.Context) {
	theme.animateSource(gtx)
	theme.animateScheme(gtx)
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

func (theme *Theme) animateScheme(gtx layout.Context) {
	if !theme.schemeAnimating {
		return
	}
	progress := theme.schemeTween.Value(gtx)
	roles := mixRoles(theme.schemeFrom, theme.schemeTo, progress)
	accent := Accent{
		Fill:  palette.Mix(theme.accentFrom.Fill, theme.accentTo.Fill, progress),
		On:    palette.Mix(theme.accentFrom.On, theme.accentTo.On, progress),
		Ink:   palette.Mix(theme.accentFrom.Ink, theme.accentTo.Ink, progress),
		Field: palette.Mix(theme.accentFrom.Field, theme.accentTo.Field, progress),
	}
	theme.applyScheme(roles, accent)
	if progress >= 1 {
		theme.schemeAnimating = false
	}
}

func (theme *Theme) applyScheme(roles palette.Roles, accent Accent) {
	theme.R = roles
	theme.Accent = accent
	theme.R.Gold = accent.Fill
	theme.R.OnGold = accent.On
	theme.R.GoldInk = accent.Ink
}

func mixRoles(from, to palette.Roles, progress float32) palette.Roles {
	return palette.Roles{
		Ground:  palette.Mix(from.Ground, to.Ground, progress),
		Stone1:  palette.Mix(from.Stone1, to.Stone1, progress),
		Stone2:  palette.Mix(from.Stone2, to.Stone2, progress),
		Stone3:  palette.Mix(from.Stone3, to.Stone3, progress),
		Stone4:  palette.Mix(from.Stone4, to.Stone4, progress),
		Vein:    palette.Mix(from.Vein, to.Vein, progress),
		Fg:      palette.Mix(from.Fg, to.Fg, progress),
		Fg2:     palette.Mix(from.Fg2, to.Fg2, progress),
		Fg3:     palette.Mix(from.Fg3, to.Fg3, progress),
		Sel:     palette.Mix(from.Sel, to.Sel, progress),
		OnSel:   palette.Mix(from.OnSel, to.OnSel, progress),
		OnSel2:  palette.Mix(from.OnSel2, to.OnSel2, progress),
		Gold:    palette.Mix(from.Gold, to.Gold, progress),
		OnGold:  palette.Mix(from.OnGold, to.OnGold, progress),
		GoldInk: palette.Mix(from.GoldInk, to.GoldInk, progress),
		Jade:    palette.Mix(from.Jade, to.Jade, progress),
		Lapis:   palette.Mix(from.Lapis, to.Lapis, progress),
		Amber:   palette.Mix(from.Amber, to.Amber, progress),
		Jasper:  palette.Mix(from.Jasper, to.Jasper, progress),
		Scrim:   palette.Mix(from.Scrim, to.Scrim, progress),
		Dark:    to.Dark,
		Source:  to.Source,
	}
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
