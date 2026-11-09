package chrome

import (
	"image"
	"math"
	"time"

	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
	"github.com/ryoku-dev/ryotunes/internal/client/state"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/widgets"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

// Mini owns the complete Ryotunes Mini window content.
type Mini struct {
	expand    kit.IconPlaque
	expandTip look.Tip
	close     kit.IconPlaque
	closeTip  look.Tip

	artClick   kit.Click
	art        look.ArtView
	titleSwap  look.Swap
	artistSwap look.Swap
	lyrics     Lyrics
	showLyrics bool

	seek      look.Scrubber
	transport transport
	like      kit.IconPlaque
	likeFill  look.Fill
	likeTip   look.Tip
	lyricsAct kit.IconPlaque
	lyricsTip look.Tip
	volume    volumeControl

	trackID      string
	seekTarget   float64
	seekOrigin   float64
	seekUntil    time.Time
	seekHold     bool
	seekDragging bool

	pendingAction system.Action
	hasAction     bool
}

// Layout renders the 360 by 420 mini player window.
func (mini *Mini) Layout(gtx layout.Context, e *env.Env) layout.Dimensions {
	size := gtx.Constraints.Max
	paint.FillShape(gtx.Ops, e.Th.R.Stone1, clip.Rect(image.Rectangle{Max: size}).Op())
	snapshot := e.Store.Snapshot()
	mini.reconcileSeek(gtx, e, snapshot.Playback.Position, playbackTrackID(snapshot.Playback.Now))

	if snapshot.Playback.Now == nil {
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return mini.layoutStrip(gtx, e) }),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return layout.Center.Layout(gtx, kit.Text(e.Th.Theme, e.Th.T.Title, e.Th.R.Fg, "Nothing playing"))
			}),
		)
	}

	const detailsHeight = 40 + 13 + 18 + 13 + 40 + 5 + 48
	fixed := gtx.Dp(32 + 13 + detailsHeight + 13)
	stageSide, showStage := fitMiniStage(size.Y-fixed, size.X-2*gtx.Dp(13), gtx.Dp(96), gtx.Dp(200))
	if !showStage {
		mini.showLyrics = false
	}
	dimensions := layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return mini.layoutStrip(gtx, e) }),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if !showStage {
				return layout.Dimensions{Size: gtx.Constraints.Max}
			}
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return mini.layoutStage(gtx, e, snapshot.Playback.Now, stageSide)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: 13, Bottom: 13, Left: 13, Right: 13}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return mini.layoutDetails(gtx, e, snapshot, showStage)
			})
		}),
	)
	if mini.artClick.Clicked(gtx) {
		e.Nav.ToggleNowPlaying()
	}
	return dimensions
}

// WindowAction returns and clears a pending mini window-control action.
func (mini *Mini) WindowAction() (system.Action, bool) {
	action, ok := mini.pendingAction, mini.hasAction
	mini.pendingAction, mini.hasAction = 0, false
	return action, ok
}

func (mini *Mini) layoutStrip(gtx layout.Context, e *env.Env) layout.Dimensions {
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = gtx.Dp(32), gtx.Dp(32)
	mini.expand.Small, mini.expand.Kind, mini.expand.Icon, mini.expand.Description = true, kit.PlaqueQuiet, look.Icon.Expand, "Expand"
	mini.close.Small, mini.close.Kind, mini.close.Icon, mini.close.Description = true, kit.PlaqueQuiet, look.Icon.Close, "Close"
	dimensions := layout.Inset{Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				size := gtx.Constraints.Max
				area := clip.Rect(image.Rectangle{Max: size}).Push(gtx.Ops)
				system.ActionInputOp(system.ActionMove).Add(gtx.Ops)
				area.Pop()
				return layout.Dimensions{Size: size}
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return mini.expandTip.Layout(gtx, e.Th, e.Overlays, "Expand", func(gtx layout.Context) layout.Dimensions {
					return mini.expand.Layout(gtx, e.Th.Theme)
				})
			}),
			layout.Rigid(layout.Spacer{Width: 4}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return mini.closeTip.Layout(gtx, e.Th, e.Overlays, "Close", func(gtx layout.Context) layout.Dimensions {
					return mini.close.Layout(gtx, e.Th.Theme)
				})
			}),
		)
	})
	if mini.expand.Clicked(gtx) {
		e.Nav.ToggleMini()
	}
	if mini.close.Clicked(gtx) {
		mini.pendingAction, mini.hasAction = system.ActionClose, true
	}
	return dimensions
}

func (mini *Mini) layoutStage(gtx layout.Context, e *env.Env, now *model.PlaybackTrack, side int) layout.Dimensions {
	gtx.Constraints = layout.Exact(image.Pt(side, side))
	if mini.showLyrics {
		return mini.lyrics.Layout(gtx, e, LyricsCompact)
	}
	return mini.artClick.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return mini.art.Layout(gtx, e.Th, e.Art, optionalString(now.Thumbnail), image.Pt(side, side), look.Shape{Cut: 7}, e.Th.Motion.Navigate)
	})
}

func (mini *Mini) layoutDetails(gtx layout.Context, e *env.Env, snapshot state.State, showStage bool) layout.Dimensions {
	now := snapshot.Playback.Now
	key := now.VideoID
	track := trackFromPlayback(now, e.Store.CurrentTrack())
	provider := model.ProviderOf(track.VideoID)
	canRate := provider != model.ProviderLocal && provider != model.ProviderRadio
	return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(miniFixedHeight(gtx.Dp(40), func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return mini.titleSwap.Layout(gtx, e.Th, key, 8, e.Th.Motion.Navigate, func(gtx layout.Context, _ string) layout.Dimensions {
						return (kit.Label{Style: e.Th.T.Row, Color: e.Th.R.Fg, MaxLines: 1, Align: text.Middle}).Layout(gtx, e.Th.Theme, track.Title)
					})
				}),
				layout.Rigid(layout.Spacer{Height: 2}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return mini.artistSwap.Layout(gtx, e.Th, key, 8, e.Th.Motion.Navigate, func(gtx layout.Context, _ string) layout.Dimensions {
						return (kit.Label{Style: e.Th.T.Caption, Color: e.Th.R.Fg2, MaxLines: 1, Align: text.Middle}).Layout(gtx, e.Th.Theme, track.Artists)
					})
				}),
			)
		})),
		layout.Rigid(layout.Spacer{Height: 13}.Layout),
		layout.Rigid(miniFixedHeight(gtx.Dp(18), func(gtx layout.Context) layout.Dimensions {
			return mini.layoutSeek(gtx, e, snapshot, track, provider)
		})),
		layout.Rigid(layout.Spacer{Height: 13}.Layout),
		layout.Rigid(miniFixedHeight(gtx.Dp(40), func(gtx layout.Context) layout.Dimensions {
			return mini.transport.Layout(gtx, e, snapshot)
		})),
		layout.Rigid(layout.Spacer{Height: 5}.Layout),
		layout.Rigid(miniFixedHeight(gtx.Dp(48), func(gtx layout.Context) layout.Dimensions {
			return mini.layoutBottom(gtx, e, snapshot, track, canRate, showStage)
		})),
	)
}

func (mini *Mini) layoutSeek(gtx layout.Context, e *env.Env, snapshot state.State, track model.Track, provider model.Provider) layout.Dimensions {
	if provider == model.ProviderRadio {
		return layout.Center.Layout(gtx, kit.Claim(e.Th.Theme, palette.Alive, "Live"))
	}
	duration := playbackDisplayDuration(snapshot.Playback.Duration, track.Duration)
	knownDuration := duration > 0
	position := 0.0
	end := "--:--"
	if knownDuration {
		position = e.Store.PositionAt(gtx.Now)
		end = widgets.Duration(duration)
	}
	value := float32(0)
	if knownDuration {
		value = float32(min(max(position/duration, 0), 1))
	}
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(playerTimeBox(e, widgets.Duration(position), text.End)),
		layout.Rigid(layout.Spacer{Width: 8}.Layout),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			fill := e.Th.R.Fg2
			if !snapshot.Playback.Paused {
				fill = e.Th.Src.Ink
			}
			scrubGTX := gtx
			if !knownDuration {
				scrubGTX = gtx.Disabled()
			}
			dimensions := mini.seek.Layout(scrubGTX, e.Th, value, look.ScrubberStyle{
				Fill: fill,
				Bubble: func(fraction float32) string {
					if !knownDuration {
						return "--:--"
					}
					return widgets.Duration(float64(fraction) * duration)
				},
				WheelStep: float32(min(1, 5/max(1, duration))),
				Wave:      e.Fx.Wave,
				Live:      !snapshot.Playback.Paused,
			})
			if !knownDuration {
				if mini.seekDragging {
					e.Store.SetSeekDrag(nil)
				}
				mini.seekDragging = false
				return dimensions
			}
			fraction, dragging := mini.seek.Dragging()
			if dragging {
				preview := float64(fraction) * duration
				e.Store.SetSeekDrag(&preview)
			}
			committedFraction, committed := mini.seek.Committed()
			if committed {
				mini.seekTarget = float64(committedFraction) * duration
				mini.seekOrigin = snapshot.Playback.Position
				mini.seekUntil = gtx.Now.Add(seekSafetyTimeout)
				mini.seekHold = true
				e.Store.SetSeekDrag(&mini.seekTarget)
				callVoid(e, protocol.MethodSeek, protocol.SeekParams{Position: mini.seekTarget})
				gtx.Execute(op.InvalidateCmd{At: mini.seekUntil})
			} else if mini.seekDragging && !dragging {
				e.Store.SetSeekDrag(nil)
			}
			mini.seekDragging = dragging
			if !e.Fx.Wave || dragging {
				mini.requestProgressFrame(gtx, e, duration, dimensions.Size.X, snapshot.Playback.Paused)
			}
			return dimensions
		}),
		layout.Rigid(layout.Spacer{Width: 8}.Layout),
		layout.Rigid(playerTimeBox(e, end, text.Start)),
	)
}

func (mini *Mini) layoutBottom(gtx layout.Context, e *env.Env, snapshot state.State, track model.Track, canRate, showLyrics bool) layout.Dimensions {
	mini.lyricsAct.Small, mini.lyricsAct.Icon, mini.lyricsAct.Description = true, look.Icon.Lyrics, "Lyrics"
	mini.lyricsAct.Kind = kit.PlaqueQuiet
	if mini.showLyrics {
		mini.lyricsAct.Kind = kit.PlaqueOn
	}
	scrubberWidth := fitMiniScrubber(
		gtx.Constraints.Max.X, gtx.Dp(32), gtx.Dp(13),
		gtx.Dp(100), gtx.Dp(72), canRate, showLyrics,
	)
	dimensions := layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if !canRate {
				return layout.Dimensions{}
			}
			mini.like.Small, mini.like.Kind, mini.like.Icon, mini.like.Description = true, kit.PlaqueQuiet, nil, "Like"
			liked := snapshot.Rating == model.RatingLike
			return mini.likeTip.Layout(gtx, e.Th, e.Overlays, "Like", func(gtx layout.Context) layout.Dimensions {
				return layout.Stack{Alignment: layout.Center}.Layout(gtx,
					layout.Stacked(func(gtx layout.Context) layout.Dimensions { return mini.like.Layout(gtx, e.Th.Theme) }),
					layout.Expanded(func(gtx layout.Context) layout.Dimensions {
						return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return mini.likeFill.Layout(gtx, e.Th, liked,
								func(gtx layout.Context) layout.Dimensions { return look.Icon.HeartOutline.Layout(gtx, e.Th.R.Fg2) },
								func(gtx layout.Context) layout.Dimensions { return look.Icon.Heart.Layout(gtx, e.Th.Src.Ink) },
							)
						})
					}),
				)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if !canRate || !showLyrics {
				return layout.Dimensions{}
			}
			return layout.Spacer{Width: 13}.Layout(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if !showLyrics {
				return layout.Dimensions{}
			}
			return mini.lyricsTip.Layout(gtx, e.Th, e.Overlays, "Lyrics", func(gtx layout.Context) layout.Dimensions {
				return mini.lyricsAct.Layout(gtx, e.Th.Theme)
			})
		}),
		layout.Flexed(1, layout.Spacer{}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return mini.volume.Layout(gtx, e, scrubberWidth) }),
	)
	if canRate && mini.like.Clicked(gtx) {
		rating := model.RatingLike
		if snapshot.Rating == model.RatingLike {
			rating = model.RatingIndifferent
		}
		act.Rate(e, track, rating)
	}
	if showLyrics && mini.lyricsAct.Clicked(gtx) {
		mini.showLyrics = !mini.showLyrics
	}
	return dimensions
}

func miniFixedHeight(height int, content layout.Widget) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
		return layout.Center.Layout(gtx, content)
	}
}

func fitMiniStage(remaining, width, minimum, maximum int) (int, bool) {
	available := min(remaining, width)
	if available < minimum {
		return 0, false
	}
	return min(available, maximum), true
}

func fitMiniScrubber(available, plaque, gap, preferred, minimum int, showLike, showLyrics bool) int {
	fixed := plaque
	if showLyrics {
		fixed += plaque + gap
	}
	if showLike {
		fixed += plaque + gap
	}
	remaining := available - fixed
	if remaining >= preferred {
		return preferred
	}
	return max(0, min(minimum, remaining))
}

func (mini *Mini) reconcileSeek(gtx layout.Context, e *env.Env, position float64, trackID string) {
	if mini.trackID != trackID {
		mini.trackID = trackID
		mini.seekHold = false
		e.Store.SetSeekDrag(nil)
		return
	}
	if !mini.seekHold {
		return
	}
	passed := math.Abs(position-mini.seekTarget) <= .5
	if mini.seekTarget > mini.seekOrigin {
		passed = passed || position >= mini.seekTarget
	} else if mini.seekTarget < mini.seekOrigin {
		passed = passed || position <= mini.seekTarget
	}
	if passed || !gtx.Now.Before(mini.seekUntil) {
		mini.seekHold = false
		e.Store.SetSeekDrag(nil)
	}
}

func (mini *Mini) requestProgressFrame(gtx layout.Context, e *env.Env, duration float64, trackPixels int, paused bool) {
	if paused || duration <= 0 || trackPixels <= 0 {
		return
	}
	pixelDelay := time.Duration(duration / float64(trackPixels) * float64(time.Second))
	if pixelDelay < 250*time.Millisecond {
		pixelDelay = 250 * time.Millisecond
	}
	position := e.Store.PositionAt(gtx.Now)
	secondDelay := time.Duration((1 - (position - math.Floor(position))) * float64(time.Second))
	if secondDelay <= 0 {
		secondDelay = time.Second
	}
	gtx.Execute(op.InvalidateCmd{At: gtx.Now.Add(min(pixelDelay, secondDelay))})
}

func playbackTrackID(now *model.PlaybackTrack) string {
	if now == nil {
		return ""
	}
	return now.VideoID
}
