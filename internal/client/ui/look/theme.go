package look

import (
	"image/color"

	"gioui.org/layout"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/anim"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
	libtheme "github.com/ryoku-dev/ryoku/docs/lithic/library/go/theme"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/tokens"
	"github.com/ryoku-dev/ryotunes/internal/model"
)

// Source contains the active provider's identity colors.
type Source struct {
	Provider         model.Provider
	Fill, Ink, Field color.NRGBA
}

// Theme is the shared Lithic theme plus Ryotunes source colors and frame class.
type Theme struct {
	*libtheme.Theme
	Src   Source
	Class Class

	srcFrom, srcTo Source
	srcTween       anim.Tween
}

// NewTheme constructs the official Lithic theme with Ryotunes fonts.
func NewTheme(roles palette.Roles, density libtheme.Density) *Theme {
	base := libtheme.New(roles, libtheme.WithFontCollection(Fonts()))
	base.Density = density
	source := sourceFor("", roles)
	return &Theme{
		Theme:    base,
		Src:      source,
		srcFrom:  source,
		srcTo:    source,
		srcTween: anim.Tween{Duration: base.Motion.Field, Ease: anim.EaseChisel},
	}
}

// SetRoles replaces palette roles and snaps all source colors to them.
func (theme *Theme) SetRoles(roles palette.Roles) {
	theme.R = roles
	theme.Src = sourceFor(theme.Src.Provider, roles)
	theme.srcFrom, theme.srcTo = theme.Src, theme.Src
	theme.srcTween = anim.Tween{Duration: theme.Motion.Field, Ease: anim.EaseChisel}
}

// SetProvider starts a source-color transition from the colors visible now.
func (theme *Theme) SetProvider(gtx layout.Context, provider model.Provider) {
	theme.Animate(gtx)
	target := sourceFor(provider, theme.R)
	if theme.Src.Provider == provider && theme.srcTo == target {
		return
	}
	theme.srcFrom, theme.srcTo = theme.Src, target
	theme.srcTween = anim.Tween{Duration: theme.Motion.Field, Ease: anim.EaseChisel}
	theme.srcTween.Set(gtx, 0)
	theme.srcTween.Set(gtx, 1)
	theme.Animate(gtx)
}

// Animate advances the active provider transition.
func (theme *Theme) Animate(gtx layout.Context) {
	progress := theme.srcTween.Value(gtx)
	theme.Src = Source{
		Provider: theme.srcTo.Provider,
		Fill:     palette.Mix(theme.srcFrom.Fill, theme.srcTo.Fill, progress),
		Ink:      palette.Mix(theme.srcFrom.Ink, theme.srcTo.Ink, progress),
		Field:    palette.Mix(theme.srcFrom.Field, theme.srcTo.Field, progress),
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
