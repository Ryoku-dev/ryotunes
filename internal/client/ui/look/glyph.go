package look

import (
	"image"
	"image/color"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryotunes/internal/model"
)

const (
	youtubeGlyphPath    = "M12 3a9 9 0 1 0 0 18a9 9 0 1 0 0-18z M12 7.5a4.5 4.5 0 1 0 0 9a4.5 4.5 0 1 0 0-9z M10.5 9.8v4.4l3.6-2.2z"
	spotifyGlyphPath    = "M12 3a9 9 0 1 0 0 18a9 9 0 1 0 0-18z M7 9.4c3.4-1 7.2-0.7 10.2 1 M7.6 12.3c2.8-0.8 5.9-0.5 8.4 0.9 M8.2 15c2.2-0.6 4.6-0.4 6.6 0.7"
	soundCloudGlyphPath = "M17.5 19H9a7 7 0 1 1 6.71-9h1.79a4.5 4.5 0 1 1 0 9Z"
	tidalGlyphPath      = "M4 14L8 10L12 14L8 18Z M8 10L12 6L16 10L12 14Z M12 14L16 10L20 14L16 18Z"
	discordGlyphPath    = "M15.5 17.5C16.5 19 17.3333 19.6667 18 20C19.3333 19.6667 22 18.2 22 15C22 11.8 20.6667 7.33333 20 5.5C18 4.3 15.8333 4 15 4L14.198 5.60393C13.4135 5.28708 12.4058 5.25438 12 5.27763C11.5942 5.25438 10.5865 5.28708 9.80197 5.60393L9 4C8.16667 4 6 4.3 4 5.5C3.33333 7.33333 2 11.8 2 15C2 18.2 4.66667 19.6667 6 20C6.66667 19.6667 7.5 19 8.5 17.5 M17.3652 11.5C17.3652 12.6046 16.5817 13.5 15.6152 13.5C14.6487 13.5 13.8652 12.6046 13.8652 11.5C13.8652 10.3954 14.6487 9.5 15.6152 9.5C16.5817 9.5 17.3652 10.3954 17.3652 11.5Z M10 11.5C10 12.6046 9.2165 13.5 8.25 13.5C7.2835 13.5 6.5 12.6046 6.5 11.5C6.5 10.3954 7.2835 9.5 8.25 9.5C9.2165 9.5 10 10.3954 10 11.5Z M17.5 16.5C16.4022 17.3967 14.3502 18 12 18C9.64981 18 7.59785 17.3967 6.5 16.5"
)

// ProviderGlyph draws the provider's 24-unit stroke mark.
func ProviderGlyph(provider model.Provider, size unit.Dp, ink color.NRGBA) layout.Widget {
	if provider == model.ProviderLocal || provider == model.ProviderRadio {
		icon := Icon.Folder
		if provider == model.ProviderRadio {
			icon = Icon.Radio
		}
		return func(gtx layout.Context) layout.Dimensions {
			pixels := gtx.Dp(size)
			gtx.Constraints = layout.Exact(image.Pt(pixels, pixels))
			return icon.Layout(gtx, ink)
		}
	}
	path := youtubeGlyphPath
	switch provider {
	case model.ProviderSpotify:
		path = spotifyGlyphPath
	case model.ProviderSoundCloud:
		path = soundCloudGlyphPath
	case model.ProviderTidal:
		path = tidalGlyphPath
	}
	return strokedGlyph(path, size, ink)
}

// DiscordGlyph draws the legacy client's verbatim Discord stroke mark.
func DiscordGlyph(size unit.Dp, ink color.NRGBA) layout.Widget {
	return strokedGlyph(discordGlyphPath, size, ink)
}

// ProviderName returns the provider's human-facing name.
func ProviderName(provider model.Provider) string {
	switch provider {
	case model.ProviderYouTube:
		return "YouTube Music"
	case model.ProviderSpotify:
		return "Spotify"
	case model.ProviderSoundCloud:
		return "SoundCloud"
	case model.ProviderTidal:
		return "TIDAL"
	case model.ProviderLocal:
		return "Local files"
	case model.ProviderRadio:
		return "Radio"
	default:
		return "Unknown"
	}
}

func strokedGlyph(data string, size unit.Dp, ink color.NRGBA) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		pixels := gtx.Dp(size)
		dimensions := image.Pt(pixels, pixels)
		path, geometry, err := buildSVGPath(gtx.Ops, data, f32.Pt(24, 24), dimensions)
		if err != nil {
			return layout.Dimensions{Size: dimensions}
		}
		width := max(float32(1), 1.5*float32(pixels)/24)
		paint.FillShape(gtx.Ops, ink, clip.Stroke{Path: path, Width: width}.Op())
		drawRoundCaps(gtx, geometry, pixels, width, ink)
		return layout.Dimensions{Size: dimensions}
	}
}

func drawRoundCaps(gtx layout.Context, geometry svgGeometry, pixels int, width float32, ink color.NRGBA) {
	scale := float32(pixels) / 24
	var start, end f32.Point
	open := false
	flush := func() {
		if !open {
			return
		}
		radius := width / 2
		for _, point := range []f32.Point{start, end} {
			centre := point.Mul(scale)
			bounds := image.Rect(
				int(centre.X-radius), int(centre.Y-radius),
				int(centre.X+radius+1), int(centre.Y+radius+1),
			)
			paint.FillShape(gtx.Ops, ink, clip.Ellipse(bounds).Op(gtx.Ops))
		}
		open = false
	}
	for _, operation := range geometry.operations {
		switch operation.kind {
		case 'M':
			flush()
			start, end, open = operation.points[0], operation.points[0], true
		case 'L':
			end = operation.points[0]
		case 'Q':
			end = operation.points[1]
		case 'C':
			end = operation.points[2]
		case 'Z':
			open = false
		}
	}
	flush()
}
