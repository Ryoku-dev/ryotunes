package chrome

import (
	"image"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/anim"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/tokens"
	"github.com/ryoku-dev/ryotunes/internal/client/state"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/widgets"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

// The bar's facts line, set as the Hub's masthead sets its facts: the
// smallest mono in spaced capitals with a 3 by 3 square between, the last in
// fg2 and the others in fg3. The masthead's facts stand 12 apart; the bar's
// track zone is a third of its width, so these stand 8 apart, the gap of the
// Hub's tree between a name and its leader.
const (
	factTracking         = 1.2
	factGap      unit.Dp = 8
	factSquare   unit.Dp = 3
)

// PlayerBar owns the persistent and compact playback bars.
type PlayerBar struct {
	artClick   kit.Click
	title      textLink
	artist     textLink
	art        look.ArtView
	titleSwap  look.Swap
	artistSwap look.Swap
	factsSwap  look.Swap
	facts      playerFacts

	like      likePlaque
	likeTip   look.Tip
	download  kit.IconPlaque
	downGlyph widgets.PressGlyph
	downTip   look.Tip

	transport widgets.Transport
	seek      widgets.Seek
	volume    volumeControl

	lyrics      kit.IconPlaque
	lyricsGlyph widgets.PressGlyph
	lyricsTip   look.Tip
	queue       kit.IconPlaque
	queueGlyph  widgets.PressGlyph
	queueTip    look.Tip
	now         kit.IconPlaque
	nowGlyph    widgets.PressGlyph
	nowTip      look.Tip
	mini        kit.IconPlaque
	miniGlyph   widgets.PressGlyph
	miniTip     look.Tip
	sound       kit.IconPlaque
	soundGlyph  widgets.PressGlyph
	soundTip    look.Tip

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

// playerFacts is the bar's facts line: the provider in capitals and the
// track's place in the queue ("04/12"). Its strings are built only when one
// of them changes, so a bar at rest builds none.
type playerFacts struct {
	provider model.Provider
	index    int
	count    int
	ready    bool
	key      string
	name     string
	place    string
}

// update rebuilds the facts for provider and the queue's position.
func (f *playerFacts) update(provider model.Provider, index, count int) {
	if f.ready && provider == f.provider && index == f.index && count == f.count {
		return
	}
	f.ready, f.provider, f.index, f.count = true, provider, index, count
	f.name = look.ProviderName(provider)
	f.place = ""
	if index >= 0 && count > 0 && provider != model.ProviderRadio {
		f.place = queuePlace(index+1, count)
	}
	f.key = string(provider) + "\x1e" + f.place
}

// queuePlace writes a track's place in its queue as the Hub writes a page's
// place in its sheet: "04/12", padded to two figures.
func queuePlace(at, of int) string {
	return kit.Pad2(at) + "/" + kit.Pad2(of)
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

// Layout renders the 88 dp three-zone player interior: the track and its
// facts at the left, the transport over the seek row in the centre, the
// panel toggles, mini, volume and sound at the right.
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
	bar.titleSwap.Ease, bar.artistSwap.Ease, bar.factsSwap.Ease = anim.EaseOutCubic, anim.EaseOutCubic, anim.EaseOutCubic
	bar.facts.update(provider, snapshot.Queue.CurrentIndex, len(snapshot.Queue.Items))
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
						return bar.title.Layout(gtx, e.Th, func(gtx layout.Context) layout.Dimensions {
							return (kit.Label{Style: e.Th.T.Row, Color: e.Th.R.Fg, MaxLines: 1}).Layout(gtx, e.Th.Theme, track.Title)
						})
					})
				}),
				layout.Rigid(layout.Spacer{Height: 2}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return bar.artistSwap.Layout(gtx, e.Th, key, 8, e.Th.Motion.Navigate, func(gtx layout.Context, _ string) layout.Dimensions {
						return bar.artist.Layout(gtx, e.Th, func(gtx layout.Context) layout.Dimensions {
							return (kit.Label{Style: e.Th.T.Caption, Color: e.Th.R.Fg2, MaxLines: 1}).Layout(gtx, e.Th.Theme, track.Artists)
						})
					})
				}),
				layout.Rigid(layout.Spacer{Height: 5}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return bar.factsSwap.Layout(gtx, e.Th, bar.facts.key, 0, e.Th.Motion.Navigate, func(gtx layout.Context, _ string) layout.Dimensions {
						return layoutFacts(gtx, e.Th, provider, bar.facts.name, bar.facts.place)
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
	if bar.title.Clicked(gtx) {
		if track.AlbumID != nil && *track.AlbumID != "" {
			act.OpenAlbum(e, *track.AlbumID, optionalString(track.Album))
		} else {
			e.Nav.ToggleNowPlaying()
		}
	}
	if bar.artist.Clicked(gtx) {
		if id, name := firstTrackArtist(track); id != "" {
			act.OpenArtist(e, id, name)
		}
	}
	if bar.like.Clicked(gtx) {
		widgets.ToggleLike(e, track, snapshot.Rating == model.RatingLike)
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

// layoutFacts draws the facts line: the provider's glyph in its brand ink,
// its name in spaced capitals, then the track's place in the queue, as the
// masthead draws its facts. A narrow bar drops the place before the name,
// and the line never draws past its width.
func layoutFacts(gtx layout.Context, th *look.Theme, provider model.Provider, name, place string) layout.Dimensions {
	width := gtx.Constraints.Max.X
	gtx.Constraints.Min = image.Point{}
	free := gtx
	free.Constraints.Max.X = 1 << 20
	glyph := gtx.Dp(12)
	line := gtx.Sp(th.T.MonoSm.Line)
	height := max(glyph, line)
	gap, square := gtx.Dp(factGap), gtx.Dp(factSquare)
	defer clip.Rect(image.Rect(0, 0, width, height)).Push(gtx.Ops).Pop()

	offset := op.Offset(image.Pt(0, (height-glyph)/2)).Push(gtx.Ops)
	glyphGTX := gtx
	glyphGTX.Constraints = layout.Exact(image.Pt(glyph, glyph))
	look.ProviderGlyph(provider, 12, look.BrandInk(provider, th.R))(glyphGTX)
	offset.Pop()
	x := glyph + gtx.Dp(6)

	record := op.Record(gtx.Ops)
	placeDims := kit.LayoutSpacedCaps(free, th.Theme, th.T.MonoSm, th.R.Fg2, factTracking, place)
	placeCall := record.Stop()
	record = op.Record(gtx.Ops)
	nameInk := th.R.Fg2
	if place != "" {
		nameInk = th.R.Fg3
	}
	nameDims := kit.LayoutSpacedCaps(free, th.Theme, th.T.MonoSm, nameInk, factTracking, name)
	nameCall := record.Stop()
	showPlace := place != "" && x+nameDims.Size.X+2*gap+square+placeDims.Size.X <= width
	if place != "" && !showPlace {
		// the name stands alone, as the last fact, in fg2
		record = op.Record(gtx.Ops)
		nameDims = kit.LayoutSpacedCaps(free, th.Theme, th.T.MonoSm, th.R.Fg2, factTracking, name)
		nameCall = record.Stop()
	}

	offset = op.Offset(image.Pt(x, (height-line)/2)).Push(gtx.Ops)
	nameCall.Add(gtx.Ops)
	offset.Pop()
	x += nameDims.Size.X
	if showPlace {
		x += gap
		top := (height - square) / 2
		paint.FillShape(gtx.Ops, th.R.Fg3, clip.Rect(image.Rect(x, top, x+square, top+square)).Op())
		x += square + gap
		offset = op.Offset(image.Pt(x, (height-line)/2)).Push(gtx.Ops)
		placeCall.Add(gtx.Ops)
		offset.Pop()
		x += placeDims.Size.X
	}
	return layout.Dimensions{Size: image.Pt(min(x, width), height)}
}

func (bar *PlayerBar) layoutLike(gtx layout.Context, e *env.Env, liked bool) layout.Dimensions {
	tip := "Like"
	if liked {
		tip = "Remove like"
	}
	return bar.likeTip.Layout(gtx, e.Th, e.Overlays, tip, func(gtx layout.Context) layout.Dimensions {
		return bar.like.Layout(gtx, e.Th, liked, true)
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
		dims := bar.download.Layout(gtx, e.Th.Theme)
		pressed := bar.downGlyph.Value(gtx, e.Th, bar.download.Pressed())
		side := gtx.Dp(unit.Dp(tokens.SizeIconPlaqueIconSmall))
		widgets.DrawPressedGlyph(gtx, dims.Size, pressed, func(gtx layout.Context) layout.Dimensions {
			return icon.Layout(gtx, ink)
		}, side)
		if progress >= 0 {
			drawProgressRule(gtx, e.Th, dims.Size, float32(progress)/100)
		}
		return dims
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

// toggle sets a panel toggle's plaque: on while its panel shows, as the
// Hub's toggles and segments mark an on state with sel.
func toggle(plaque *kit.IconPlaque, description string, on bool) {
	plaque.Small, plaque.Description = true, description
	plaque.Kind = kit.PlaqueQuiet
	if on {
		plaque.Kind = kit.PlaqueOn
	}
}

func (bar *PlayerBar) layoutTrailing(gtx layout.Context, e *env.Env, provider model.Provider, fit playerBarFit) layout.Dimensions {
	panel := e.Nav.Panel()
	toggle(&bar.lyrics, "Lyrics", panel == env.PanelLyrics)
	toggle(&bar.queue, "Queue", panel == env.PanelQueue)
	toggle(&bar.now, "Now playing", e.Nav.NowPlaying())
	toggle(&bar.mini, "Mini player", false)
	toggle(&bar.sound, "Sound", false)

	volumeWidth := fit.Scrubber
	plaque := func(tip *look.Tip, glyph *widgets.PressGlyph, plaque *kit.IconPlaque, icon *widget.Icon, size *image.Point) layout.Widget {
		return func(gtx layout.Context) layout.Dimensions {
			dims := tip.Layout(gtx, e.Th, e.Overlays, plaque.Description, func(gtx layout.Context) layout.Dimensions {
				return glyph.Layout(gtx, e.Th, plaque, icon)
			})
			if size != nil {
				*size = dims.Size
			}
			return dims
		}
	}

	children := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if provider == model.ProviderRadio {
				gtx = gtx.Disabled()
			}
			return plaque(&bar.lyricsTip, &bar.lyricsGlyph, &bar.lyrics, look.Icon.Lyrics, nil)(gtx)
		}),
		layout.Rigid(layout.Spacer{Width: 4}.Layout),
		layout.Rigid(plaque(&bar.queueTip, &bar.queueGlyph, &bar.queue, look.Icon.Queue, nil)),
	}
	if fit.ShowNow {
		children = append(children,
			layout.Rigid(layout.Spacer{Width: 4}.Layout),
			layout.Rigid(plaque(&bar.nowTip, &bar.nowGlyph, &bar.now, look.Icon.NowPlaying, nil)),
		)
	}
	var soundSize image.Point
	children = append(children,
		layout.Rigid(fixedPlayerWidth(fit.Gap, layout.Spacer{}.Layout)),
		layout.Rigid(plaque(&bar.miniTip, &bar.miniGlyph, &bar.mini, look.Icon.Mini, nil)),
		layout.Rigid(fixedPlayerWidth(fit.Gap, layout.Spacer{}.Layout)),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return bar.volume.Layout(gtx, e, volumeWidth) }),
		layout.Rigid(fixedPlayerWidth(fit.Gap, layout.Spacer{}.Layout)),
		layout.Rigid(plaque(&bar.soundTip, &bar.soundGlyph, &bar.sound, look.Icon.Tune, &soundSize)),
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

// drawProgressRule draws a download's progress along the foot of its plaque
// of size: a 2 dp rule from 6 in on each side, the guide wash under it and
// lapis over the share done. It is square in every design, as the Hub's
// progress blades are.
func drawProgressRule(gtx layout.Context, th *look.Theme, size image.Point, fraction float32) {
	fraction = min(max(fraction, 0), 1)
	inset, tall := gtx.Dp(6), max(1, gtx.Dp(2))
	left, right := inset, size.X-inset
	bottom := size.Y - gtx.Dp(4)
	if right <= left {
		return
	}
	paint.FillShape(gtx.Ops, th.R.Guide(), clip.Rect(image.Rect(left, bottom-tall, right, bottom)).Op())
	done := left + int(float32(right-left)*fraction+.5)
	if done > left {
		paint.FillShape(gtx.Ops, th.R.Lapis, clip.Rect(image.Rect(left, bottom-tall, done, bottom)).Op())
	}
}
