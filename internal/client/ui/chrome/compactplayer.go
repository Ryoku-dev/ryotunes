package chrome

import (
	"image"

	"gioui.org/layout"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/anim"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/widgets"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

type compactPlayer struct {
	artClick   kit.Click
	text       textLink
	art        look.ArtView
	titleSwap  look.Swap
	artistSwap look.Swap
	play       widgets.PlayButton
	playTip    look.Tip
	next       kit.IconPlaque
	nextGlyph  widgets.PressGlyph
	nextTip    look.Tip
	wave       look.WaveState
}

// LayoutCompact renders the 64 dp player above compact bottom navigation: a
// rectangular strip with the cover square at its left, the progress wave on
// its top edge, the track, like, play and next. The cover opens now playing
// and now playing closes back to it, so the full view can always be
// reopened.
func (bar *PlayerBar) LayoutCompact(gtx layout.Context, e *env.Env) layout.Dimensions {
	snapshot, track, provider, ok := bar.playback(e)
	if !ok {
		return layout.Dimensions{}
	}
	bar.seek.Reconcile(gtx, e, snapshot, track.VideoID)

	width := gtx.Constraints.Max.X
	height := min(gtx.Constraints.Max.Y, gtx.Dp(64))
	if width <= 0 || height <= 0 {
		return layout.Dimensions{}
	}
	gtx.Constraints = layout.Exact(image.Pt(width, height))
	bar.layoutCompactProgress(gtx, e, widgets.PlaybackDuration(snapshot.Playback.Duration, track.Duration), snapshot.Playback.Paused)

	compact := &bar.compact
	compact.titleSwap.Ease, compact.artistSwap.Ease = anim.EaseOutCubic, anim.EaseOutCubic
	canRate := provider != model.ProviderLocal && provider != model.ProviderRadio
	key := track.VideoID
	dimensions := layout.Inset{Top: 8, Left: 13, Right: 13}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return compact.artClick.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					size := image.Pt(gtx.Dp(44), gtx.Dp(44))
					return compact.art.Layout(gtx, e.Th, e.Art, optionalString(track.Thumbnail), size, look.Shape{Cut: 6}, e.Th.Motion.Navigate)
				})
			}),
			layout.Rigid(layout.Spacer{Width: 13}.Layout),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return compact.text.Layout(gtx, e.Th, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return compact.titleSwap.Layout(gtx, e.Th, key, 8, e.Th.Motion.Navigate, func(gtx layout.Context, _ string) layout.Dimensions {
								return (kit.Label{Style: e.Th.T.Row, Color: e.Th.R.Fg, MaxLines: 1}).Layout(gtx, e.Th.Theme, track.Title)
							})
						}),
						layout.Rigid(layout.Spacer{Height: 2}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return compact.artistSwap.Layout(gtx, e.Th, key, 8, e.Th.Motion.Navigate, func(gtx layout.Context, _ string) layout.Dimensions {
								return (kit.Label{Style: e.Th.T.Caption, Color: e.Th.R.Fg2, MaxLines: 1}).Layout(gtx, e.Th.Theme, track.Artists)
							})
						}),
					)
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if !canRate {
					return layout.Dimensions{}
				}
				return layout.Inset{Left: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return bar.layoutLike(gtx, e, snapshot.Rating == model.RatingLike)
				})
			}),
			layout.Rigid(layout.Spacer{Width: 8}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				icon, description := look.Icon.Play, "Play"
				if !snapshot.Playback.Paused {
					icon, description = look.Icon.Pause, "Pause"
				}
				return compact.playTip.Layout(gtx, e.Th, e.Overlays, description, func(gtx layout.Context) layout.Dimensions {
					return compact.play.Layout(gtx, e.Th, icon, description)
				})
			}),
			layout.Rigid(layout.Spacer{Width: 4}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				compact.next.Small = true
				compact.next.Kind = kit.PlaqueQuiet
				compact.next.Description = "Next"
				return compact.nextTip.Layout(gtx, e.Th, e.Overlays, "Next", func(gtx layout.Context) layout.Dimensions {
					return compact.nextGlyph.Layout(gtx, e.Th, &compact.next, look.Icon.Next)
				})
			}),
		)
	})

	if compact.artClick.Clicked(gtx) {
		e.Nav.ToggleNowPlaying()
	}
	if compact.text.Clicked(gtx) {
		if track.AlbumID != nil && *track.AlbumID != "" {
			act.OpenAlbum(e, *track.AlbumID, optionalString(track.Album))
		} else {
			e.Nav.ToggleNowPlaying()
		}
	}
	if canRate && bar.like.Clicked(gtx) {
		widgets.ToggleLike(e, track, snapshot.Rating == model.RatingLike)
	}
	if compact.play.Clicked(gtx) {
		callVoid(e, protocol.MethodTogglePause, protocol.Null{})
	}
	if compact.next.Clicked(gtx) {
		callVoid(e, protocol.MethodNextTrack, protocol.Null{})
	}
	return dimensions
}

// layoutCompactProgress draws the progress wave along the strip's top edge,
// between its two top corners (cut in Lithic, rounded in the round designs)
// so it never crosses the plane's hairline.
func (bar *PlayerBar) layoutCompactProgress(gtx layout.Context, e *env.Env, duration float64, paused bool) {
	if duration <= 0 {
		return
	}
	width := gtx.Constraints.Max.X
	corner := gtx.Dp(e.Th.Cut.Large)
	if width <= 2*corner {
		corner = 0
	}
	track := image.Rect(corner, gtx.Dp(2), width-corner, gtx.Dp(2)+max(1, gtx.Dp(8)))
	fraction := float32(min(max(e.Store.PositionAt(gtx.Now)/duration, 0), 1))
	live := e.Fx.Wave && !paused
	look.DrawWave(
		gtx, e.Th, &bar.compact.wave, track,
		fraction, e.Th.Accent.Ink, e.Th.R.Fg3, 1.5, 1.5, 12, live,
	)
	if !live {
		widgets.ProgressFrame(gtx, e, duration, max(1, track.Dx()), paused)
	}
}
