package chrome

import (
	"image"
	"image/color"
	"math"
	"strconv"
	"strings"
	"time"

	"gioui.org/f32"
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

const seekSafetyTimeout = 3 * time.Second

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

	transport transport
	seek      look.Scrubber
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

	seekTrackID  string
	seekPending  bool
	seekDragging bool
	seekTarget   float64
	seekOrigin   float64
	seekUntil    time.Time
	lastToast    uint64

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

func playbackDisplayDuration(reported float64, value *string) float64 {
	if reported > 0 && !math.IsNaN(reported) && !math.IsInf(reported, 0) {
		return reported
	}
	if value == nil {
		return 0
	}
	parts := strings.Split(strings.TrimSpace(*value), ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0
	}
	seconds := 0
	for index, part := range parts {
		component, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || component < 0 || (index > 0 && component >= 60) {
			return 0
		}
		seconds = seconds*60 + component
	}
	return float64(seconds)
}

// Layout renders the 88 dp three-zone player interior.
func (bar *PlayerBar) Layout(gtx layout.Context, e *env.Env) layout.Dimensions {
	snapshot := e.Store.Snapshot()
	bar.forwardToast(e, snapshot.Toast)
	if snapshot.Playback.Now == nil {
		return layout.Dimensions{}
	}
	track := trackFromPlayback(snapshot.Playback.Now, e.Store.CurrentTrack())
	provider := model.ProviderOf(track.VideoID)
	bar.reconcileSeek(gtx, e, snapshot, track.VideoID)

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
						func(gtx layout.Context) layout.Dimensions { return look.Icon.Heart.Layout(gtx, e.Th.Src.Ink) },
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
				return bar.layoutSeek(gtx, e, snapshot, track, provider)
			}),
		)
	})
}

func (bar *PlayerBar) layoutSeek(gtx layout.Context, e *env.Env, snapshot state.State, track model.Track, provider model.Provider) layout.Dimensions {
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
			dimensions := bar.seek.Layout(scrubGTX, e.Th, value, look.ScrubberStyle{
				Fill: fill,
				Bubble: func(fraction float32) string {
					if !knownDuration {
						return "--:--"
					}
					return widgets.Duration(float64(fraction) * duration)
				},
				WheelStep: float32(min(1, 5/max(1, duration))),
			})
			if !knownDuration {
				if bar.seekDragging {
					e.Store.SetSeekDrag(nil)
				}
				bar.seekDragging = false
				return dimensions
			}
			fraction, dragging := bar.seek.Dragging()
			if dragging {
				preview := float64(fraction) * duration
				e.Store.SetSeekDrag(&preview)
			}
			committedFraction, committed := bar.seek.Committed()
			if committed {
				bar.commitSeek(gtx, e, snapshot, float64(committedFraction)*duration, duration)
			} else if bar.seekDragging && !dragging {
				e.Store.SetSeekDrag(nil)
			}
			bar.seekDragging = dragging
			bar.requestProgressFrame(gtx, e, duration, max(1, dimensions.Size.X), snapshot.Playback.Paused)
			return dimensions
		}),
		layout.Rigid(layout.Spacer{Width: 8}.Layout),
		layout.Rigid(playerTimeBox(e, end, text.Start)),
	)
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
		e.Nav.OpenSound(e.Overlays.AnchorOf(&bar.sound.Clickable, soundSize))
	}
	return dimensions
}

func (bar *PlayerBar) commitSeek(gtx layout.Context, e *env.Env, snapshot state.State, target, duration float64) {
	target = min(max(target, 0), duration)
	bar.seekPending = true
	bar.seekTarget = target
	bar.seekOrigin = snapshot.Playback.Position
	bar.seekUntil = gtx.Now.Add(seekSafetyTimeout)
	e.Store.SetSeekDrag(&target)
	gtx.Execute(op.InvalidateCmd{At: bar.seekUntil})
	callVoid(e, protocol.MethodSeek, protocol.SeekParams{Position: target})
}

func (bar *PlayerBar) reconcileSeek(gtx layout.Context, e *env.Env, snapshot state.State, trackID string) {
	if trackID != bar.seekTrackID {
		bar.seekTrackID = trackID
		bar.seekPending = false
		e.Store.SetSeekDrag(nil)
		return
	}
	if !bar.seekPending {
		return
	}
	position := snapshot.Playback.Position
	passed := math.Abs(position-bar.seekTarget) <= .5
	if bar.seekTarget > bar.seekOrigin {
		passed = passed || position >= bar.seekTarget
	} else if bar.seekTarget < bar.seekOrigin {
		passed = passed || position <= bar.seekTarget
	}
	if passed || !gtx.Now.Before(bar.seekUntil) {
		bar.seekPending = false
		e.Store.SetSeekDrag(nil)
	}
}

func (bar *PlayerBar) requestProgressFrame(gtx layout.Context, e *env.Env, duration float64, trackPixels int, paused bool) {
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
	delay := min(pixelDelay, secondDelay)
	gtx.Execute(op.InvalidateCmd{At: gtx.Now.Add(delay)})
}

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

func playerTimeBox(e *env.Env, value string, align text.Alignment) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return fixedPlayerWidth(gtx.Dp(44), func(gtx layout.Context) layout.Dimensions {
			return (kit.Label{Style: e.Th.T.Mono, Color: e.Th.R.Fg3, MaxLines: 1, Align: align}).Layout(gtx, e.Th.Theme, value)
		})(gtx)
	}
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
