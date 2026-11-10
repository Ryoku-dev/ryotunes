package chrome

import (
	"image"
	"image/color"
	"math"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryotunes/internal/client/state"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/widgets"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

// PlayerBar owns the persistent and compact playback bars.
type PlayerBar struct {
	artClick    kit.Click
	titleClick  kit.Click
	artistClick kit.Click
	art         look.ArtView
	titleSwap   look.Swap
	artistSwap  look.Swap

	like     kit.IconPlaque
	likeFill look.Fill
	likeTip  look.Tip
	download kit.IconPlaque
	downTip  look.Tip

	transport widgets.Transport
	seek      widgets.Seek
	volume    volumeControl

	lyrics    kit.IconPlaque
	lyricsTip look.Tip
	queue     kit.IconPlaque
	queueTip  look.Tip
	now       kit.IconPlaque
	nowTip    look.Tip
	mini      kit.IconPlaque
	miniTip   look.Tip
	sound     kit.IconPlaque
	soundTip  look.Tip

	lastToast     uint64
	lastSnapshot  state.State
	lastTrack     model.Track
	lastProvider  model.Provider
	hasPlayback   bool
	frameTop      int
	soundTop      int
	soundAnchor   image.Rectangle
	soundAnchored bool

	compact compactPlayer
}

type playerBarFitMetrics struct {
	Plaque       int
	SmallGap     int
	MajorGap     int
	CompactGap   int
	MaxScrubber  int
	MinScrubber  int
	TransportMin int
}

type playerBarFit struct {
	Side       int
	Centre     int
	Scrubber   int
	Gap        int
	ShowNow    bool
	TotalWidth int
}

func fitPlayerBar(side, centre int, metrics playerBarFitMetrics) playerBarFit {
	fit := playerBarFit{
		Side: side, Centre: centre, Scrubber: metrics.MaxScrubber,
		Gap: metrics.MajorGap, ShowNow: true,
	}
	fit.TotalWidth = playerBarTrailingWidth(fit, metrics)
	if fit.TotalWidth > fit.Side {
		fit.Scrubber = metrics.MinScrubber
		fit.TotalWidth = playerBarTrailingWidth(fit, metrics)
	}
	if fit.TotalWidth > fit.Side {
		fit.ShowNow = false
		fit.TotalWidth = playerBarTrailingWidth(fit, metrics)
	}
	if fit.TotalWidth > fit.Side {
		fit.Gap = metrics.CompactGap
		fit.TotalWidth = playerBarTrailingWidth(fit, metrics)
	}
	if fit.TotalWidth > fit.Side {
		needed := fit.TotalWidth - fit.Side
		available := max(0, (fit.Centre-metrics.TransportMin)/2)
		grow := min(needed, available)
		fit.Side += grow
		fit.Centre -= grow * 2
	}
	return fit
}

func playerBarTrailingWidth(fit playerBarFit, metrics playerBarFitMetrics) int {
	plaques, smallGaps := 6, 3
	if !fit.ShowNow {
		plaques, smallGaps = 5, 2
	}
	return plaques*metrics.Plaque + smallGaps*metrics.SmallGap + 3*fit.Gap + fit.Scrubber
}

// Layout renders the 88 dp three-zone player interior.
func (bar *PlayerBar) Layout(gtx layout.Context, e *env.Env) layout.Dimensions {
	snapshot, track, provider, ok := bar.playback(e)
	if !ok {
		return layout.Dimensions{}
	}

	barWidth := gtx.Constraints.Max.X
	height := min(gtx.Constraints.Max.Y, gtx.Dp(88))
	if barWidth <= 0 || height <= 0 {
		return layout.Dimensions{}
	}
	gtx.Constraints = layout.Exact(image.Pt(barWidth, height))

	side := barWidth * 28 / 100
	side = min(max(side, gtx.Dp(220)), gtx.Dp(360))
	centre := min(gtx.Dp(560), barWidth-2*side-gtx.Dp(42))
	centre = max(0, centre)
	fit := fitPlayerBar(side, centre, playerBarFitMetrics{
		Plaque:       gtx.Dp(32),
		SmallGap:     gtx.Dp(4),
		MajorGap:     gtx.Dp(13),
		CompactGap:   gtx.Dp(8),
		MaxScrubber:  gtx.Dp(120),
		MinScrubber:  gtx.Dp(72),
		TransportMin: gtx.Dp(220),
	})

	dimensions := layout.Inset{Left: 21, Right: 21}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(fixedPlayerWidth(fit.Side, func(gtx layout.Context) layout.Dimensions {
				return bar.layoutTrack(gtx, e, snapshot, track, provider)
			})),
			layout.Rigid(fixedPlayerWidth(fit.Centre, func(gtx layout.Context) layout.Dimensions {
				return bar.layoutCentre(gtx, e, snapshot, track, provider)
			})),
			layout.Rigid(fixedPlayerWidth(fit.Side, func(gtx layout.Context) layout.Dimensions {
				return bar.layoutTrailing(gtx, e, provider, fit)
			})),
		)
	})
	return dimensions
}

func (bar *PlayerBar) layoutTrack(gtx layout.Context, e *env.Env, snapshot state.State, track model.Track, provider model.Provider) layout.Dimensions {
	canAct := provider != model.ProviderLocal && provider != model.ProviderRadio
	key := track.VideoID
	dimensions := layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return bar.artClick.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				size := image.Pt(gtx.Dp(56), gtx.Dp(56))
				return bar.art.Layout(gtx, e.Th, e.Art, optionalString(track.Thumbnail), size, look.Shape{Cut: 6}, e.Th.Motion.Navigate)
			})
		}),
		layout.Rigid(layout.Spacer{Width: 13}.Layout),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return bar.titleSwap.Layout(gtx, e.Th, key, 8, e.Th.Motion.Navigate, func(gtx layout.Context, _ string) layout.Dimensions {
						return bar.titleClick.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return (kit.Label{Style: e.Th.T.Row, Color: e.Th.R.Fg, MaxLines: 1}).Layout(gtx, e.Th.Theme, track.Title)
						})
					})
				}),
				layout.Rigid(layout.Spacer{Height: 2}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return bar.artistSwap.Layout(gtx, e.Th, key, 8, e.Th.Motion.Navigate, func(gtx layout.Context, _ string) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(look.ProviderGlyph(provider, 14, look.BrandInk(provider, e.Th.R))),
							layout.Rigid(layout.Spacer{Width: 4}.Layout),
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								return bar.artistClick.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return (kit.Label{Style: e.Th.T.Caption, Color: e.Th.R.Fg2, MaxLines: 1}).Layout(gtx, e.Th.Theme, track.Artists)
								})
							}),
						)
					})
				}),
			)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if !canAct {
				return layout.Dimensions{}
			}
			return layout.Inset{Left: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return bar.layoutLike(gtx, e, snapshot.Rating == model.RatingLike)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if !canAct {
				return layout.Dimensions{}
			}
			return layout.Inset{Left: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return bar.layoutDownload(gtx, e, currentDownload(snapshot.Downloads, track.VideoID))
			})
		}),
	)

	if bar.artClick.Clicked(gtx) {
		e.Nav.ToggleNowPlaying()
	}
	if bar.titleClick.Clicked(gtx) {
		if track.AlbumID != nil && *track.AlbumID != "" {
			act.OpenAlbum(e, *track.AlbumID, optionalString(track.Album))
		} else {
			e.Nav.ToggleNowPlaying()
		}
	}
	if bar.artistClick.Clicked(gtx) {
		if id, name := firstTrackArtist(track); id != "" {
			act.OpenArtist(e, id, name)
		}
	}
	if bar.like.Clicked(gtx) {
		rating := model.RatingLike
		if snapshot.Rating == model.RatingLike {
			rating = model.RatingIndifferent
		}
		act.Rate(e, track, rating)
	}
	if bar.download.Clicked(gtx) {
		job := currentDownload(snapshot.Downloads, track.VideoID)
		switch {
		case job == nil:
			act.Download(e, track)
		case job.Status == model.DownloadStatusFailed || job.Status == model.DownloadStatusCancelled:
			callVoid(e, protocol.MethodRetryDownload, protocol.DownloadIDParams{ID: job.ID})
		default:
			e.Nav.Push(env.Route{Kind: env.RouteDownloads, Title: "Downloads"})
		}
	}
	return dimensions
}

func (bar *PlayerBar) layoutLike(gtx layout.Context, e *env.Env, liked bool) layout.Dimensions {
	bar.like.Small = true
	bar.like.Kind = kit.PlaqueQuiet
	bar.like.Icon = nil
	bar.like.Description = "Like"
	return bar.likeTip.Layout(gtx, e.Th, e.Overlays, "Like", func(gtx layout.Context) layout.Dimensions {
		return layout.Stack{Alignment: layout.Center}.Layout(gtx,
			layout.Stacked(func(gtx layout.Context) layout.Dimensions { return bar.like.Layout(gtx, e.Th.Theme) }),
			layout.Expanded(func(gtx layout.Context) layout.Dimensions {
				return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return bar.likeFill.Layout(gtx, e.Th, liked,
						func(gtx layout.Context) layout.Dimensions { return look.Icon.HeartOutline.Layout(gtx, e.Th.R.Fg2) },
						func(gtx layout.Context) layout.Dimensions { return look.Icon.Heart.Layout(gtx, e.Th.Accent.Ink) },
					)
				})
			}),
		)
	})
}

func (bar *PlayerBar) layoutDownload(gtx layout.Context, e *env.Env, job *model.DownloadEntry) layout.Dimensions {
	icon := look.Icon.Download
	ink := e.Th.R.Fg2
	tip := "Download"
	progress := -1
	if job != nil {
		switch job.Status {
		case model.DownloadStatusQueued:
			progress = max(job.Progress, 1)
			tip = "Downloading " + widgets.Count(progress) + "%"
		case model.DownloadStatusDownloading:
			progress = max(0, min(100, job.Progress))
			tip = "Downloading " + widgets.Count(progress) + "%"
		case model.DownloadStatusCompleted:
			icon = look.Icon.DownloadDone
			ink = e.Th.R.Jade
			tip = "Saved"
		case model.DownloadStatusFailed:
			tip = "Download failed. Retry"
		case model.DownloadStatusCancelled:
			tip = "Download cancelled. Retry"
		}
	}
	bar.download.Small = true
	bar.download.Kind = kit.PlaqueQuiet
	bar.download.Icon = nil
	bar.download.Description = tip
	return bar.downTip.Layout(gtx, e.Th, e.Overlays, tip, func(gtx layout.Context) layout.Dimensions {
		return layout.Stack{Alignment: layout.Center}.Layout(gtx,
			layout.Stacked(func(gtx layout.Context) layout.Dimensions { return bar.download.Layout(gtx, e.Th.Theme) }),
			layout.Expanded(func(gtx layout.Context) layout.Dimensions {
				return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					size := gtx.Dp(18)
					gtx.Constraints = layout.Exact(image.Pt(size, size))
					return icon.Layout(gtx, ink)
				})
			}),
			layout.Expanded(func(gtx layout.Context) layout.Dimensions {
				if progress >= 0 {
					drawProgressRing(gtx, float32(progress)/100, e.Th.R.Lapis)
				}
				return layout.Dimensions{Size: gtx.Constraints.Min}
			}),
		)
	})
}

func (bar *PlayerBar) layoutCentre(gtx layout.Context, e *env.Env, snapshot state.State, track model.Track, provider model.Provider) layout.Dimensions {
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return bar.transport.Layout(gtx, e, snapshot) }),
			layout.Rigid(layout.Spacer{Height: 6}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return bar.seek.Layout(gtx, e, snapshot, track)
			}),
		)
	})
}

func (bar *PlayerBar) layoutTrailing(gtx layout.Context, e *env.Env, provider model.Provider, fit playerBarFit) layout.Dimensions {
	panel := e.Nav.Panel()
	bar.lyrics.Small, bar.lyrics.Icon, bar.lyrics.Description = true, look.Icon.Lyrics, "Lyrics"
	bar.lyrics.Kind = kit.PlaqueQuiet
	if panel == env.PanelLyrics {
		bar.lyrics.Kind = kit.PlaqueOn
	}
	bar.queue.Small, bar.queue.Icon, bar.queue.Description = true, look.Icon.Queue, "Queue"
	bar.queue.Kind = kit.PlaqueQuiet
	if panel == env.PanelQueue {
		bar.queue.Kind = kit.PlaqueOn
	}
	bar.now.Small, bar.now.Icon, bar.now.Description = true, look.Icon.NowPlaying, "Now playing"
	bar.now.Kind = kit.PlaqueQuiet
	if e.Nav.NowPlaying() {
		bar.now.Kind = kit.PlaqueOn
	}
	bar.mini.Small, bar.mini.Icon, bar.mini.Description, bar.mini.Kind = true, look.Icon.Mini, "Mini player", kit.PlaqueQuiet
	bar.sound.Small, bar.sound.Icon, bar.sound.Description, bar.sound.Kind = true, look.Icon.Tune, "Sound", kit.PlaqueQuiet

	volumeWidth := fit.Scrubber

	children := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if provider == model.ProviderRadio {
				gtx = gtx.Disabled()
			}
			return bar.lyricsTip.Layout(gtx, e.Th, e.Overlays, "Lyrics", func(gtx layout.Context) layout.Dimensions {
				return bar.lyrics.Layout(gtx, e.Th.Theme)
			})
		}),
		layout.Rigid(layout.Spacer{Width: 4}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return bar.queueTip.Layout(gtx, e.Th, e.Overlays, "Queue", func(gtx layout.Context) layout.Dimensions {
				return bar.queue.Layout(gtx, e.Th.Theme)
			})
		}),
	}
	if fit.ShowNow {
		children = append(children,
			layout.Rigid(layout.Spacer{Width: 4}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return bar.nowTip.Layout(gtx, e.Th, e.Overlays, "Now playing", func(gtx layout.Context) layout.Dimensions {
					return bar.now.Layout(gtx, e.Th.Theme)
				})
			}),
		)
	}
	children = append(children,
		layout.Rigid(fixedPlayerWidth(fit.Gap, layout.Spacer{}.Layout)),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return bar.miniTip.Layout(gtx, e.Th, e.Overlays, "Mini player", func(gtx layout.Context) layout.Dimensions {
				return bar.mini.Layout(gtx, e.Th.Theme)
			})
		}),
	)
	var soundSize image.Point
	children = append(children,
		layout.Rigid(fixedPlayerWidth(fit.Gap, layout.Spacer{}.Layout)),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return bar.volume.Layout(gtx, e, volumeWidth) }),
		layout.Rigid(fixedPlayerWidth(fit.Gap, layout.Spacer{}.Layout)),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			dimensions := bar.soundTip.Layout(gtx, e.Th, e.Overlays, "Sound", func(gtx layout.Context) layout.Dimensions {
				return bar.sound.Layout(gtx, e.Th.Theme)
			})
			soundSize = dimensions.Size
			return dimensions
		}),
	)
	dimensions := layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceEnd}.Layout(gtx, children...)

	if provider != model.ProviderRadio && bar.lyrics.Clicked(gtx) {
		e.Nav.TogglePanel(env.PanelLyrics)
	}
	if bar.queue.Clicked(gtx) {
		e.Nav.TogglePanel(env.PanelQueue)
	}
	if fit.ShowNow && bar.now.Clicked(gtx) {
		e.Nav.ToggleNowPlaying()
	}
	if bar.mini.Clicked(gtx) {
		e.Nav.ToggleMini()
	}
	if bar.sound.Clicked(gtx) && e.Overlays != nil {
		bar.soundAnchor = e.Overlays.AnchorOf(&bar.sound.Clickable, soundSize)
		bar.soundTop = bar.frameTop
		bar.soundAnchored = !bar.soundAnchor.Empty()
		e.Nav.OpenSound(bar.soundAnchor)
	}
	return dimensions
}

func (bar *PlayerBar) playback(e *env.Env) (state.State, model.Track, model.Provider, bool) {
	snapshot := e.Store.Snapshot()
	bar.forwardToast(e, snapshot.Toast)
	if snapshot.Playback.Now == nil {
		return bar.lastSnapshot, bar.lastTrack, bar.lastProvider, bar.hasPlayback
	}
	track := trackFromPlayback(snapshot.Playback.Now, e.Store.CurrentTrack())
	provider := model.ProviderOf(track.VideoID)
	bar.lastSnapshot, bar.lastTrack, bar.lastProvider = snapshot, track, provider
	bar.hasPlayback = true
	return snapshot, track, provider, true
}

func (bar *PlayerBar) SetTop(top int) { bar.frameTop = top }

func (bar *PlayerBar) SoundAnchor() (image.Rectangle, bool) {
	if !bar.soundAnchored {
		return image.Rectangle{}, false
	}
	return bar.soundAnchor.Add(image.Pt(0, bar.frameTop-bar.soundTop)), true
}

func (bar *PlayerBar) ResetSoundAnchor() { bar.soundAnchored = false }

func (bar *PlayerBar) forwardToast(e *env.Env, toast *state.Toast) {
	if toast == nil || toast.Serial == bar.lastToast || e.Toast == nil {
		return
	}
	bar.lastToast = toast.Serial
	kind := env.ToastInfo
	if toast.Kind == state.ToastError {
		kind = env.ToastError
	} else if toast.Kind == state.ToastSuccess {
		kind = env.ToastSuccess
	}
	e.Toast(toast.Message, kind)
}

func fixedPlayerWidth(width int, widget layout.Widget) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		width = max(0, min(width, gtx.Constraints.Max.X))
		gtx.Constraints.Min.X, gtx.Constraints.Max.X = width, width
		return widget(gtx)
	}
}

func firstTrackArtist(track model.Track) (string, string) {
	for _, artist := range track.ArtistRuns {
		if artist.ID != nil && *artist.ID != "" {
			return *artist.ID, artist.Text
		}
	}
	if track.ArtistID != nil {
		return *track.ArtistID, track.Artists
	}
	return "", ""
}

func drawProgressRing(gtx layout.Context, fraction float32, ink color.NRGBA) {
	fraction = min(max(fraction, 0), 1)
	if fraction <= 0 {
		return
	}
	size := gtx.Constraints.Min
	centre := f32.Pt(float32(size.X)/2, float32(size.Y)/2)
	radius := float32(min(size.X, size.Y))/2 - float32(gtx.Dp(2))
	steps := max(2, int(math.Ceil(float64(32*fraction))))
	var path clip.Path
	path.Begin(gtx.Ops)
	for index := range steps + 1 {
		angle := -math.Pi/2 + 2*math.Pi*float64(index)/float64(steps)*float64(fraction)
		point := f32.Pt(centre.X+radius*float32(math.Cos(angle)), centre.Y+radius*float32(math.Sin(angle)))
		if index == 0 {
			path.MoveTo(point)
		} else {
			path.LineTo(point)
		}
	}
	paint.FillShape(gtx.Ops, ink, clip.Stroke{Path: path.End(), Width: float32(max(1, gtx.Dp(2)))}.Op())
}
