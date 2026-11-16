package look

import (
	"image/color"

	"gioui.org/layout"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/anim"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
	libtheme "github.com/ryoku-dev/ryoku/docs/lithic/library/go/theme"
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
// field of that hue (Field). The progress wave, the visualizer, the playing
// row, hearts and hero fields read it.
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

// Theme is the shared Lithic theme the desktop asks for, plus Ryotunes'
// music colours (the provider source and the accent) and the frame class.
//
// The layers, bottom to top: the desktop's roles (Desktop.Roles, its design,
// colours, mode, contrast and tint), then the cover tint when the listener
// asks for it, then the accent over gold.
type Theme struct {
	*libtheme.Theme
	Src    Source
	Accent Accent
	Class  Class
	// Desktop is the look the desktop asks for: design, colours, light or
	// dark, contrast, tint, reduced motion and the gloss.
	Desktop palette.Desktop

	loadedRoles palette.Roles

	srcFrom, srcTo Source
	srcTween       anim.Tween
	providerSet    bool

	schemeFrom, schemeTo palette.Roles
	accentFrom, accentTo Accent
	schemeTween          anim.Tween
	schemeSet            bool
	schemeAnimating      bool

	schemeCache     [schemeCacheSize]schemeCacheEntry
	schemeCacheNext int
}

// NewThemeFor builds the theme the desktop asks for (theme.NewFor), which
// also sets the process's reduced motion from it.
func NewThemeFor(desktop palette.Desktop, density libtheme.Density, provider ...model.Provider) *Theme {
	return newTheme(libtheme.NewFor(desktop), desktop, density, provider)
}

// NewTheme builds a Lithic theme on caller-chosen roles (theme.New), for
// tests and galleries that read no desktop: Lithic design, gloss on, motion
// left as it is.
func NewTheme(roles palette.Roles, density libtheme.Density, provider ...model.Provider) *Theme {
	mode := palette.Dark
	if !roles.Dark {
		mode = palette.Light
	}
	desktop := palette.Desktop{
		Mode: mode, Gloss: true, Roles: roles,
		Palette: palette.Palette{Dark: roles.Dark, Source: roles.Source},
	}
	return newTheme(libtheme.New(roles), desktop, density, provider)
}

func newTheme(base *libtheme.Theme, desktop palette.Desktop, density libtheme.Density, provider []model.Provider) *Theme {
	base.Density = density
	var initial model.Provider
	if len(provider) > 0 {
		initial = provider[0]
	}
	roles := desktop.Roles
	source := sourceFor(initial, roles)
	accent := AccentFromDesktop(roles)
	theme := &Theme{
		Theme:       base,
		Src:         source,
		Accent:      accent,
		Desktop:     desktop,
		loadedRoles: roles,
		srcFrom:     source,
		srcTo:       source,
		srcTween:    anim.Tween{Duration: base.Motion.Field, Ease: anim.EaseChisel},
		schemeFrom:  roles,
		schemeTo:    roles,
		accentFrom:  accent,
		accentTo:    accent,
		schemeTween: anim.Tween{Duration: base.Motion.Field, Ease: anim.EaseChisel},
		providerSet: initial != "",
	}
	theme.applyScheme(roles, accent)
	return theme
}

// SetDesktop swaps in a new reading of the desktop: a theme built by
// theme.NewFor on the same shaper (so no face loads again), the density kept,
// and the roles snapped as SetRoles does. It sets reduced motion from the
// desktop. Call it on the window's goroutine.
func (theme *Theme) SetDesktop(desktop palette.Desktop) {
	density := theme.Density
	base := libtheme.NewFor(desktop, libtheme.WithShaper(theme.Shaper))
	base.Density = density
	theme.Theme = base
	theme.Desktop = desktop
	theme.SetRoles(desktop.Roles)
}

// LoadedRoles returns the untouched desktop roles, before the cover tint and
// the accent.
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
	switch {
	case cover.Vibrant.A != 0:
		accent = AccentFromSample(cover.Vibrant, roles)
	case cover.Dominant.A != 0:
		accent = NeutralAccent(roles)
	}
	theme.schemeCache[theme.schemeCacheNext] = schemeCacheEntry{
		key: key, roles: roles, accent: accent, used: true,
	}
	theme.schemeCacheNext = (theme.schemeCacheNext + 1) % len(theme.schemeCache)
	return roles, accent
}

// ArtAccent derives a hero accent from its cover without changing the window
// roles.
func (theme *Theme) ArtAccent(cover art.Palette) Accent {
	_, accent := theme.ArtScheme(cover, false)
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

// Animate advances active theme transitions.
func (theme *Theme) Animate(gtx layout.Context) {
	theme.animateSource(gtx)
	theme.animateScheme(gtx)
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

// applyScheme lays the accent over gold: the play plaque, gold ink and every
// kit gold control point at the music.
func (theme *Theme) applyScheme(roles palette.Roles, accent Accent) {
	theme.R = roles
	theme.Accent = accent
	theme.R.Gold = accent.Fill
	theme.R.OnGold = accent.On
	theme.R.GoldInk = accent.Ink
}

// mixRoles blends every colour role from from toward to. It starts from to,
// so every field the library adds (and the unexported mineral lightness
// MineralField reads) lands as the target's.
func mixRoles(from, to palette.Roles, progress float32) palette.Roles {
	mixed := to
	for _, pair := range [...]struct{ out, a, b *color.NRGBA }{
		{&mixed.Ground, &from.Ground, &to.Ground},
		{&mixed.Stone1, &from.Stone1, &to.Stone1},
		{&mixed.Stone2, &from.Stone2, &to.Stone2},
		{&mixed.Stone3, &from.Stone3, &to.Stone3},
		{&mixed.Stone4, &from.Stone4, &to.Stone4},
		{&mixed.Slab, &from.Slab, &to.Slab},
		{&mixed.Field, &from.Field, &to.Field},
		{&mixed.Vein, &from.Vein, &to.Vein},
		{&mixed.Edge, &from.Edge, &to.Edge},
		{&mixed.Fg, &from.Fg, &to.Fg},
		{&mixed.Fg2, &from.Fg2, &to.Fg2},
		{&mixed.Fg3, &from.Fg3, &to.Fg3},
		{&mixed.Sel, &from.Sel, &to.Sel},
		{&mixed.OnSel, &from.OnSel, &to.OnSel},
		{&mixed.OnSel2, &from.OnSel2, &to.OnSel2},
		{&mixed.Gold, &from.Gold, &to.Gold},
		{&mixed.OnGold, &from.OnGold, &to.OnGold},
		{&mixed.GoldInk, &from.GoldInk, &to.GoldInk},
		{&mixed.Jade, &from.Jade, &to.Jade},
		{&mixed.Lapis, &from.Lapis, &to.Lapis},
		{&mixed.Amber, &from.Amber, &to.Amber},
		{&mixed.Jasper, &from.Jasper, &to.Jasper},
		{&mixed.Scrim, &from.Scrim, &to.Scrim},
		{&mixed.Container, &from.Container, &to.Container},
		{&mixed.OnContainer, &from.OnContainer, &to.OnContainer},
	} {
		*pair.out = palette.Mix(*pair.a, *pair.b, progress)
	}
	return mixed
}

func sourceFor(provider model.Provider, roles palette.Roles) Source {
	fill := BrandFill(provider, roles)
	return Source{
		Provider: provider,
		Fill:     fill,
		Ink:      BrandInk(provider, roles),
		Field:    accentField(fill, roles),
	}
}
