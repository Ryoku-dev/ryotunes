package chrome

import (
	"image"
	"strconv"
	"strings"

	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryotunes/internal/client/state"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/widgets"
	"github.com/ryoku-dev/ryotunes/internal/model"
)

// Mini owns the complete Ryotunes Mini window content.
type Mini struct {
	expand    look.WindowControl
	expandTip look.Tip
	close     look.WindowControl
	closeTip  look.Tip

	art                 widgets.PlayArt
	layoutSwap          look.Swap
	titleSwap           look.Swap
	artistSwap          look.Swap
	landscapeTitleSwap  look.Swap
	landscapeArtistSwap look.Swap
	lyrics              Lyrics
	showLyrics          bool

	landscapeTrackID      string
	landscapeTrackKey     string
	landscapeLyricID      string
	landscapeLyricIndex   int
	landscapeLyricKey     string
	landscapeLyricReady   bool
	landscapeWraps        map[string]bool
	landscapeLyricWrapped bool

	seek      widgets.Seek
	transport widgets.Transport
	like      kit.IconPlaque
	likeFill  look.Fill
	likeTip   look.Tip
	lyricsAct kit.IconPlaque
	lyricsTip look.Tip
	volume    volumeControl

	pendingAction system.Action
	hasAction     bool
}

type miniFit struct {
	Landscape        bool
	ArtSide          int
	ColumnWidth      int
	ColumnInset      int
	InnerWidth       int
	CompactTransport bool
	ShowSeek         bool
	ShowTransport    bool
	ShowLike         bool
	ShowLyrics       bool
	ShowVolume       bool
	VolumeScrubber   int
}

// Layout renders the responsive mini player window.
func (mini *Mini) Layout(gtx layout.Context, e *env.Env) layout.Dimensions {
	size := gtx.Constraints.Max
	windowClip := clip.Rect(image.Rectangle{Max: size}).Push(gtx.Ops)
	paint.FillShape(gtx.Ops, e.Th.R.Stone1, clip.Rect(image.Rectangle{Max: size}).Op())

	snapshot := e.Store.Snapshot()
	mini.seek.Reconcile(gtx, e, snapshot, playbackTrackID(snapshot.Playback.Now))
	layoutKey := "portrait"
	if miniUsesLandscape(size.X, size.Y, gtx.Dp(251)) {
		layoutKey = "landscape"
	}
	dimensions := mini.layoutSwap.Layout(gtx, e.Th, layoutKey, 0, e.Th.Motion.Field, func(gtx layout.Context, key string) layout.Dimensions {
		gtx.Constraints = layout.Exact(size)
		if key == "landscape" {
			return mini.layoutLandscape(gtx, e, snapshot)
		}
		return mini.layoutPortrait(gtx, e, snapshot)
	})

	if snapshot.Playback.Now != nil {
		mini.handleTrackActions(gtx, e, snapshot)
	}
	if mini.expand.Clicked(gtx) {
		e.Nav.ToggleMini()
	}
	if mini.close.Clicked(gtx) {
		mini.pendingAction, mini.hasAction = system.ActionClose, true
	}
	windowClip.Pop()
	dimensions.Size = size
	return dimensions
}

// WindowAction returns and clears a pending mini window-control action.
func (mini *Mini) WindowAction() (system.Action, bool) {
	action, ok := mini.pendingAction, mini.hasAction
	mini.pendingAction, mini.hasAction = 0, false
	return action, ok
}

func (mini *Mini) layoutPortrait(gtx layout.Context, e *env.Env, snapshot state.State) layout.Dimensions {
	if snapshot.Playback.Now == nil {
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return mini.layoutStrip(gtx, e) }),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return layout.Center.Layout(gtx, kit.Text(e.Th.Theme, e.Th.T.Title, e.Th.R.Fg, "Nothing playing"))
			}),
		)
	}

	const detailsHeight = 40 + 13 + 18 + 13 + 40 + 5 + 48
	fixed := gtx.Dp(48 + 13 + detailsHeight + 13)
	stageSide, showStage := fitMiniStage(gtx.Constraints.Max.Y-fixed, gtx.Constraints.Max.X-2*gtx.Dp(13), gtx.Dp(96), gtx.Dp(200))
	if !showStage {
		mini.showLyrics = false
	}
	return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
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
}

func (mini *Mini) layoutLandscape(gtx layout.Context, e *env.Env, snapshot state.State) layout.Dimensions {
	size := gtx.Constraints.Max
	fit := fitMini(
		size.X, size.Y, gtx.Dp(251), gtx.Dp(13), gtx.Dp(54),
		gtx.Dp(120), gtx.Dp(112), gtx.Dp(32), gtx.Dp(4),
		gtx.Dp(360), gtx.Dp(72),
	)
	var url string
	if snapshot.Playback.Now != nil {
		url = optionalString(snapshot.Playback.Now.Thumbnail)
	}
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			side := fit.ArtSide
			gtx.Constraints = layout.Exact(image.Pt(side, side))
			return mini.art.Layout(
				gtx, e, url, image.Pt(side, side),
				look.Shape{Cut: e.Th.Cut.Mid, Corners: [4]bool{false, true, false, false}},
				e.Th.Motion.Field,
			)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			columnInset := gtx.Metric.PxToDp(fit.ColumnInset)
			return layout.Stack{}.Layout(gtx,
				layout.Expanded(miniMoveArea),
				layout.Stacked(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: 12, Right: columnInset, Bottom: 13, Left: columnInset}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return mini.layoutLandscapeColumn(gtx, e, snapshot, fit)
					})
				}),
			)
		}),
	)
}

func (mini *Mini) layoutLandscapeColumn(gtx layout.Context, e *env.Env, snapshot state.State, fit miniFit) layout.Dimensions {
	title, artists, key := "Nothing playing", "", "empty"
	track := model.Track{}
	provider := model.Provider("")
	if snapshot.Playback.Now != nil {
		track = trackFromPlayback(snapshot.Playback.Now, e.Store.CurrentTrack())
		title, artists = track.Title, track.Artists
		key = mini.landscapeHeadingKey(track.VideoID, false, 0)
		provider = model.ProviderOf(track.VideoID)
		if mini.showLyrics && provider != model.ProviderRadio {
			mini.lyrics.ensure(gtx, e)
			mini.lyrics.updateLineMotion(gtx, e.Th.Motion.Field)
			key = mini.landscapeHeadingKey(track.VideoID, true, mini.lyrics.active)
		}
	}

	mini.expand.Icon, mini.expand.Description = look.Icon.Expand, "Expand"
	mini.close.Icon, mini.close.Description = look.Icon.Close, "Close"
	mini.transport.Basic = true
	if fit.CompactTransport {
		mini.transport.Gap = 4
	} else {
		mini.transport.Gap = 8
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(miniFixedHeight(gtx.Dp(44), func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
			mini.landscapeLyricWrapped = false
			return layout.Stack{}.Layout(gtx,
				layout.Stacked(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Right: 54}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								dims := mini.landscapeTitleSwap.Layout(gtx, e.Th, key, 0, e.Th.Motion.Field, func(gtx layout.Context, swapKey string) layout.Dimensions {
									return mini.layoutLandscapePrimary(gtx, e, swapKey, title, swapKey == key)
								})
								height := gtx.Sp(e.Th.T.Label.Line)
								if mini.landscapeLyricWrapped {
									height *= 2
								}
								dims.Size.Y = min(gtx.Constraints.Max.Y, height)
								return dims
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layout.Inset{Top: 2}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return mini.landscapeArtistSwap.Layout(gtx, e.Th, key, 0, e.Th.Motion.Field, func(gtx layout.Context, swapKey string) layout.Dimensions {
										return mini.layoutLandscapeSecondary(gtx, e, swapKey, artists)
									})
								})
							}),
						)
					})
				}),
				layout.Stacked(func(gtx layout.Context) layout.Dimensions {
					size := gtx.Constraints.Max
					child := gtx
					child.Constraints.Min = image.Point{}
					child.Constraints.Max = image.Pt(gtx.Dp(54), gtx.Dp(24))
					move := op.Offset(image.Pt(max(0, size.X-child.Constraints.Max.X), 0)).Push(gtx.Ops)
					layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(child,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return mini.expandTip.Layout(gtx, e.Th, e.Overlays, "Expand", func(gtx layout.Context) layout.Dimensions {
								return mini.expand.Layout(gtx, e.Th)
							})
						}),
						layout.Rigid(layout.Spacer{Width: 6}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return mini.closeTip.Layout(gtx, e.Th, e.Overlays, "Close", func(gtx layout.Context) layout.Dimensions {
								return mini.close.Layout(gtx, e.Th)
							})
						}),
					)
					move.Pop()
					return layout.Dimensions{Size: size}
				}),
			)
		})),
		layout.Flexed(1, layout.Spacer{}.Layout),
		layout.Rigid(miniFixedHeight(gtx.Dp(18), func(gtx layout.Context) layout.Dimensions {
			if !fit.ShowSeek || snapshot.Playback.Now == nil {
				return layout.Dimensions{}
			}
			return mini.seek.Layout(gtx, e, snapshot, track)
		})),
		layout.Rigid(layout.Spacer{Height: 8}.Layout),
		layout.Rigid(miniFixedHeight(gtx.Dp(40), func(gtx layout.Context) layout.Dimensions {
			return mini.layoutLandscapeActions(gtx, e, snapshot, track, provider, fit)
		})),
	)
}

func (mini *Mini) layoutLandscapePrimary(gtx layout.Context, e *env.Env, key, title string, current bool) layout.Dimensions {
	mini.setLandscapeWrapped(key, current, false)
	if index, ok := miniLyricIndex(key); ok {
		if current {
			defer mini.lyrics.scheduleBoundary(gtx, e, LyricsCompact)
		}
		if mini.lyrics.lyrics == nil {
			return (kit.Label{Style: e.Th.T.Label, Color: e.Th.R.Fg, MaxLines: 1}).Layout(gtx, e.Th.Theme, "Resolving lyrics")
		}
		if index < 0 || index >= len(mini.lyrics.lyrics.Lines) {
			return (kit.Label{Style: e.Th.T.Label, Color: e.Th.R.Fg, MaxLines: 1}).Layout(gtx, e.Th.Theme, "No synced lyrics")
		}
		line := mini.lyrics.lyrics.Lines[index]
		if e.Store.Snapshot().Client.WordByWordOn() && index < len(mini.lyrics.words) && len(mini.lyrics.words[index]) > 0 {
			result := mini.lyrics.karaoke[index].Layout(gtx, e.Th, mini.lyrics.words[index], look.KaraokeStyle{
				Key: mini.lyrics.trackID, Text: e.Th.T.Label, Base: e.Th.R.Fg2, Highlight: e.Th.Accent.Ink,
				Align: text.Start, PreferredRows: 1, MaxRows: 2, MinScale: .8,
				Lift: unit.Dp(2 * mini.lyrics.lineProgress(index)), PositionMS: mini.lyrics.positionMS(gtx, e),
			})
			mini.setLandscapeWrapped(key, current, result.Rows > 1)
			return result.Dimensions
		}
		dims := (kit.Label{Style: e.Th.T.Label, Color: e.Th.R.Fg, MaxLines: 2}).Layout(gtx, e.Th.Theme, strings.TrimSpace(line.Text))
		mini.setLandscapeWrapped(key, current, dims.Size.Y > gtx.Sp(e.Th.T.Label.Line))
		return dims
	}
	return (kit.Label{Style: e.Th.T.Row, Color: e.Th.R.Fg, MaxLines: 1}).Layout(gtx, e.Th.Theme, title)
}

func (mini *Mini) layoutLandscapeSecondary(gtx layout.Context, e *env.Env, key, artists string) layout.Dimensions {
	if index, ok := miniLyricIndex(key); ok {
		if mini.landscapeWraps[key] || mini.lyrics.lyrics == nil || index+1 < 0 || index+1 >= len(mini.lyrics.lyrics.Lines) {
			return layout.Dimensions{}
		}
		next := strings.TrimSpace(mini.lyrics.lyrics.Lines[index+1].Text)
		return (kit.Label{Style: e.Th.T.Caption, Color: e.Th.R.Fg3, MaxLines: 1}).Layout(gtx, e.Th.Theme, next)
	}
	return (kit.Label{Style: e.Th.T.Caption, Color: e.Th.R.Fg2, MaxLines: 1}).Layout(gtx, e.Th.Theme, artists)
}

func (mini *Mini) setLandscapeWrapped(key string, current, wrapped bool) {
	if mini.landscapeWraps == nil {
		mini.landscapeWraps = make(map[string]bool)
	}
	mini.landscapeWraps[key] = wrapped
	if current {
		mini.landscapeLyricWrapped = wrapped
	}
}

func (mini *Mini) layoutStrip(gtx layout.Context, e *env.Env) layout.Dimensions {
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = gtx.Dp(48), gtx.Dp(48)
	mini.expand.Icon, mini.expand.Description = look.Icon.Expand, "Expand"
	mini.close.Icon, mini.close.Description = look.Icon.Close, "Close"
	return layout.Inset{Left: 12, Right: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Flexed(1, miniMoveArea),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return mini.expandTip.Layout(gtx, e.Th, e.Overlays, "Expand", func(gtx layout.Context) layout.Dimensions {
					return mini.expand.Layout(gtx, e.Th)
				})
			}),
			layout.Rigid(layout.Spacer{Width: 6}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return mini.closeTip.Layout(gtx, e.Th, e.Overlays, "Close", func(gtx layout.Context) layout.Dimensions {
					return mini.close.Layout(gtx, e.Th)
				})
			}),
		)
	})
}

func miniMoveArea(gtx layout.Context) layout.Dimensions {
	size := gtx.Constraints.Max
	area := clip.Rect(image.Rectangle{Max: size}).Push(gtx.Ops)
	system.ActionInputOp(system.ActionMove).Add(gtx.Ops)
	area.Pop()
	return layout.Dimensions{Size: size}
}

func (mini *Mini) layoutStage(gtx layout.Context, e *env.Env, now *model.PlaybackTrack, side int) layout.Dimensions {
	gtx.Constraints = layout.Exact(image.Pt(side, side))
	if mini.showLyrics {
		return mini.lyrics.Layout(gtx, e, LyricsCompact, layout.Inset{})
	}
	return mini.art.Layout(gtx, e, optionalString(now.Thumbnail), image.Pt(side, side), look.Shape{Cut: 7}, e.Th.Motion.Navigate)
}

func (mini *Mini) layoutDetails(gtx layout.Context, e *env.Env, snapshot state.State, showStage bool) layout.Dimensions {
	now := snapshot.Playback.Now
	key := now.VideoID
	track := trackFromPlayback(now, e.Store.CurrentTrack())
	provider := model.ProviderOf(track.VideoID)
	canRate := provider != model.ProviderLocal && provider != model.ProviderRadio
	mini.transport.Basic, mini.transport.Gap = false, 13
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
			return mini.seek.Layout(gtx, e, snapshot, track)
		})),
		layout.Rigid(layout.Spacer{Height: 13}.Layout),
		layout.Rigid(miniFixedHeight(gtx.Dp(40), func(gtx layout.Context) layout.Dimensions {
			return mini.transport.Layout(gtx, e, snapshot)
		})),
		layout.Rigid(layout.Spacer{Height: 5}.Layout),
		layout.Rigid(miniFixedHeight(gtx.Dp(48), func(gtx layout.Context) layout.Dimensions {
			return mini.layoutBottom(gtx, e, snapshot, canRate, showStage)
		})),
	)
}

func (mini *Mini) layoutBottom(gtx layout.Context, e *env.Env, snapshot state.State, canRate, showLyrics bool) layout.Dimensions {
	scrubberWidth := fitMiniScrubber(
		gtx.Constraints.Max.X, gtx.Dp(32), gtx.Dp(13),
		gtx.Dp(100), gtx.Dp(72), canRate, showLyrics,
	)
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if !canRate {
				return layout.Dimensions{}
			}
			return mini.layoutLike(gtx, e, snapshot)
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
			return mini.layoutLyricsAction(gtx, e)
		}),
		layout.Flexed(1, layout.Spacer{}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return mini.volume.Layout(gtx, e, scrubberWidth) }),
	)
}

func (mini *Mini) layoutLandscapeActions(gtx layout.Context, e *env.Env, snapshot state.State, track model.Track, provider model.Provider, fit miniFit) layout.Dimensions {
	canRate := provider != model.ProviderLocal && provider != model.ProviderRadio
	canLyrics := provider != model.ProviderRadio
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if !fit.ShowTransport {
				return layout.Dimensions{}
			}
			return mini.transport.Layout(gtx, e, snapshot)
		}),
		layout.Flexed(1, layout.Spacer{}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if !fit.ShowLike || !canRate {
				return layout.Dimensions{}
			}
			return mini.layoutLike(gtx, e, snapshot)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if !fit.ShowLyrics || !canLyrics {
				return layout.Dimensions{}
			}
			return layout.Inset{Left: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return mini.layoutLyricsAction(gtx, e)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if !fit.ShowVolume {
				return layout.Dimensions{}
			}
			return layout.Inset{Left: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return mini.volume.Layout(gtx, e, fit.VolumeScrubber)
			})
		}),
	)
}

func (mini *Mini) layoutLike(gtx layout.Context, e *env.Env, snapshot state.State) layout.Dimensions {
	mini.like.Small, mini.like.Kind, mini.like.Icon, mini.like.Description = true, kit.PlaqueQuiet, nil, "Like"
	liked := snapshot.Rating == model.RatingLike
	return mini.likeTip.Layout(gtx, e.Th, e.Overlays, "Like", func(gtx layout.Context) layout.Dimensions {
		return layout.Stack{Alignment: layout.Center}.Layout(gtx,
			layout.Stacked(func(gtx layout.Context) layout.Dimensions { return mini.like.Layout(gtx, e.Th.Theme) }),
			layout.Expanded(func(gtx layout.Context) layout.Dimensions {
				return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return mini.likeFill.Layout(gtx, e.Th, liked,
						func(gtx layout.Context) layout.Dimensions { return look.Icon.HeartOutline.Layout(gtx, e.Th.R.Fg2) },
						func(gtx layout.Context) layout.Dimensions { return look.Icon.Heart.Layout(gtx, e.Th.Accent.Ink) },
					)
				})
			}),
		)
	})
}

func (mini *Mini) layoutLyricsAction(gtx layout.Context, e *env.Env) layout.Dimensions {
	mini.lyricsAct.Small, mini.lyricsAct.Icon, mini.lyricsAct.Description = true, look.Icon.Lyrics, "Lyrics"
	mini.lyricsAct.Kind = kit.PlaqueQuiet
	if mini.showLyrics {
		mini.lyricsAct.Kind = kit.PlaqueOn
	}
	return mini.lyricsTip.Layout(gtx, e.Th, e.Overlays, "Lyrics", func(gtx layout.Context) layout.Dimensions {
		return mini.lyricsAct.Layout(gtx, e.Th.Theme)
	})
}

func (mini *Mini) handleTrackActions(gtx layout.Context, e *env.Env, snapshot state.State) {
	track := trackFromPlayback(snapshot.Playback.Now, e.Store.CurrentTrack())
	provider := model.ProviderOf(track.VideoID)
	if provider != model.ProviderLocal && provider != model.ProviderRadio && mini.like.Clicked(gtx) {
		rating := model.RatingLike
		if snapshot.Rating == model.RatingLike {
			rating = model.RatingIndifferent
		}
		act.Rate(e, track, rating)
	}
	if provider != model.ProviderRadio && mini.lyricsAct.Clicked(gtx) {
		mini.showLyrics = !mini.showLyrics
	}
}

func miniUsesLandscape(width, height, portraitMinHeight int) bool {
	return miniIsLandscape(width, height) || height < portraitMinHeight
}

func miniIsLandscape(width, height int) bool {
	return height > 0 && int64(width)*10 >= int64(height)*14
}

func (mini *Mini) landscapeHeadingKey(trackID string, lyrics bool, lyricIndex int) string {
	if !lyrics {
		if mini.landscapeTrackID != trackID {
			mini.landscapeTrackID = trackID
			mini.landscapeTrackKey = "track:" + trackID
		}
		return mini.landscapeTrackKey
	}
	if mini.landscapeLyricID != trackID && mini.landscapeWraps != nil {
		clear(mini.landscapeWraps)
	}
	if !mini.landscapeLyricReady || mini.landscapeLyricID != trackID || mini.landscapeLyricIndex != lyricIndex {
		mini.landscapeLyricID = trackID
		mini.landscapeLyricIndex = lyricIndex
		mini.landscapeLyricKey = "lyrics:" + strconv.Itoa(lyricIndex) + ":" + trackID
		mini.landscapeLyricReady = true
	}
	return mini.landscapeLyricKey
}

func fitMini(width, height, portraitMinHeight, inset, windowControls, transportWide, transportNarrow, plaque, gap, volumeThreshold, volumeWidth int) miniFit {
	fit := miniFit{Landscape: miniUsesLandscape(width, height, portraitMinHeight)}
	if !fit.Landscape {
		return fit
	}
	fit.ArtSide = min(max(height, 0), max(width, 0))
	fit.ColumnWidth = max(0, width-fit.ArtSide)
	fit.ColumnInset = min(inset, max(0, (fit.ColumnWidth-windowControls)/2))
	fit.InnerWidth = max(0, fit.ColumnWidth-2*fit.ColumnInset)
	fit.ShowSeek = fit.InnerWidth >= 3*plaque+gap

	transport := 0
	switch {
	case fit.InnerWidth >= transportWide:
		fit.ShowTransport = true
		transport = transportWide
	case fit.InnerWidth >= transportNarrow:
		fit.ShowTransport = true
		fit.CompactTransport = true
		transport = transportNarrow
	}
	volumeControlWidth := plaque + gap
	fit.ShowVolume = fit.InnerWidth >= transport+volumeControlWidth
	fit.ShowLyrics = fit.ShowVolume && fit.InnerWidth >= transport+plaque+gap+volumeControlWidth
	fit.ShowLike = fit.ShowLyrics && fit.InnerWidth >= transport+2*plaque+2*gap+volumeControlWidth

	rightWidth := 0
	if fit.ShowLike {
		rightWidth += plaque
	}
	if fit.ShowLyrics {
		if rightWidth > 0 {
			rightWidth += gap
		}
		rightWidth += plaque
	}
	if fit.ShowVolume {
		if rightWidth > 0 {
			rightWidth += gap
		}
		rightWidth += volumeControlWidth
	}
	if fit.ColumnWidth >= volumeThreshold && fit.ShowVolume && fit.InnerWidth >= transport+rightWidth+volumeWidth {
		fit.VolumeScrubber = volumeWidth
	}
	return fit
}

func miniLyricIndex(key string) (int, bool) {
	value, ok := strings.CutPrefix(key, "lyrics:")
	if !ok {
		return 0, false
	}
	if separator := strings.IndexByte(value, ':'); separator >= 0 {
		value = value[:separator]
	}
	index, err := strconv.Atoi(value)
	return index, err == nil
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

func playbackTrackID(now *model.PlaybackTrack) string {
	if now == nil {
		return ""
	}
	return now.VideoID
}
