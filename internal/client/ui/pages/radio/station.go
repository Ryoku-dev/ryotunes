package radio

import (
	"fmt"
	"image"
	"image/color"
	"strings"

	"gioui.org/layout"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/model"
)

// tileSlab lays a station tile on the design's card plane, drawn by its hairline in Lithic
// and Fluent, with the Hub's hover wash over it (hover 0..1).
func tileSlab(gtx layout.Context, e *env.Env, hover float32, body layout.Widget) layout.Dimensions {
	th := e.Th
	size := gtx.Constraints.Max
	bounds := image.Rectangle{Max: size}
	cut := gtx.Dp(th.Cut.Small)
	planes := th.Planes()
	kit.Fill(gtx, th.Theme, bounds, cut, planes.Card)
	if hover > 0 {
		kit.Fill(gtx, th.Theme, bounds, cut, palette.WithAlpha(th.R.Hover(), hover))
	}
	if stroke := planes.CardStroke; stroke.A != 0 {
		kit.Stroke(gtx, th.Theme, bounds, cut, max(1, gtx.Dp(1)), stroke)
	} else {
		kit.Stroke(gtx, th.Theme, bounds, cut, max(1, gtx.Dp(1)), th.R.Line())
	}
	body(gtx)
	return layout.Dimensions{Size: size}
}

type stationTile struct {
	click     kit.Click
	favourite kit.Click
	hover     look.Hover
	fill      look.Fill
	art       look.ArtView
	reveal    float32
}

func (t *stationTile) layout(gtx layout.Context, e *env.Env, page *Page, station model.RadioStation, playing, favourite bool) layout.Dimensions {
	favouriteClicked := t.favourite.Clicked(gtx)
	if favouriteClicked {
		page.toggleFavourite(e, station)
	}
	if t.click.Clicked(gtx) && !favouriteClicked {
		page.playStation(e, station)
	}

	t.hover.Hub = true
	size := gtx.Constraints.Constrain(image.Pt(gtx.Constraints.Max.X, gtx.Dp(88)))
	tileGTX := gtx
	tileGTX.Constraints = layout.Exact(size)
	dims, reveal := t.hover.Layout(tileGTX, e.Th, func(gtx layout.Context) layout.Dimensions {
		return t.click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return tileSlab(gtx, e, t.reveal, func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: unit.Dp(16), Right: unit.Dp(13), Bottom: unit.Dp(16), Left: unit.Dp(13)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					artSize := gtx.Dp(56)
					children := []layout.FlexChild{
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return t.art.Layout(gtx, e.Th, e.Art, station.Favicon, image.Pt(artSize, artSize), look.Shape{Cut: 6}, e.Th.Motion.Navigate)
						}),
						layout.Rigid(layout.Spacer{Width: unit.Dp(13)}.Layout),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							copy := []layout.FlexChild{layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return kit.Label{Style: e.Th.T.Label, Color: e.Th.R.Fg, MaxLines: 1}.Layout(gtx, e.Th.Theme, station.Name)
							})}
							if facts := stationFacts(station); facts != "" {
								copy = append(copy,
									layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										return kit.Label{Style: e.Th.T.MonoSm, Color: e.Th.R.Fg3, MaxLines: 1}.Layout(gtx, e.Th.Theme, facts)
									}),
								)
							}
							return layout.Flex{Axis: layout.Vertical, Alignment: layout.Start}.Layout(gtx, copy...)
						}),
					}
					if playing {
						children = append(children,
							layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
							layout.Rigid(iconLayout(look.Icon.VolumeUp, e.Th.Accent.Ink, unit.Dp(20))),
						)
					}
					if favourite || t.reveal > 0 {
						children = append(children,
							layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								alpha := t.reveal
								if favourite {
									alpha = 1
								}
								opacity := paint.PushOpacity(gtx.Ops, alpha)
								gtx.Constraints = layout.Exact(image.Pt(gtx.Dp(32), gtx.Dp(32)))
								dims := t.favourite.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return t.fill.Layout(gtx, e.Th, favourite,
											iconLayout(look.Icon.HeartOutline, e.Th.R.Fg2, unit.Dp(18)),
											iconLayout(look.Icon.Heart, e.Th.Accent.Ink, unit.Dp(18)),
										)
									})
								})
								opacity.Pop()
								return dims
							}),
						)
					}
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
				})
			})
		})
	})
	t.reveal = reveal
	return dims
}

func iconLayout(icon *widget.Icon, ink paletteColor, size unit.Dp) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		pixels := gtx.Dp(size)
		gtx.Constraints = layout.Exact(image.Pt(pixels, pixels))
		if icon == nil {
			return layout.Dimensions{Size: image.Pt(pixels, pixels)}
		}
		return icon.Layout(gtx, ink)
	}
}

type paletteColor = color.NRGBA

func stationFacts(station model.RadioStation) string {
	facts := make([]string, 0, 2)
	if country := strings.TrimSpace(station.Country); country != "" {
		facts = append(facts, country)
	}
	codec := strings.TrimSpace(station.Codec)
	if codec != "" && station.Bitrate > 0 {
		facts = append(facts, fmt.Sprintf("%s · %d kbps", codec, station.Bitrate))
	} else if codec != "" {
		facts = append(facts, codec)
	} else if station.Bitrate > 0 {
		facts = append(facts, fmt.Sprintf("%d kbps", station.Bitrate))
	}
	return strings.Join(facts, " · ")
}

func stationPersonalID(uuid string) string { return "RYOTUNES_RADIO:" + strings.TrimSpace(uuid) }

func stationBrowse(station model.RadioStation) model.BrowseItem {
	item := model.BrowseItem{Kind: "station", ID: stationPersonalID(station.StationUUID), Title: station.Name}
	if country := strings.TrimSpace(station.Country); country != "" {
		item.Subtitle = &country
	}
	if favicon := strings.TrimSpace(station.Favicon); favicon != "" {
		item.Thumbnail = &favicon
	}
	if facts := strings.TrimSpace(stationFacts(model.RadioStation{Codec: station.Codec, Bitrate: station.Bitrate})); facts != "" {
		item.PlayCount = &facts
	}
	return item
}

func stationFromBrowse(item model.BrowseItem) model.RadioStation {
	station := model.RadioStation{StationUUID: strings.TrimPrefix(item.ID, "RYOTUNES_RADIO:"), Name: item.Title}
	if item.Subtitle != nil {
		station.Country = strings.TrimSpace(*item.Subtitle)
	}
	if item.Thumbnail != nil {
		station.Favicon = strings.TrimSpace(*item.Thumbnail)
	}
	if item.PlayCount != nil {
		station.Codec = strings.TrimSpace(*item.PlayCount)
	}
	return station
}
