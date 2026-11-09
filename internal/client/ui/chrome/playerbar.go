package chrome

import (
	"image"
	"math"
	"time"

	"gioui.org/f32"
	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryotunes/internal/client/lithic"
	"github.com/ryoku-dev/ryotunes/internal/client/state"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/widgets"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

// PlayerBar renders the persistent transport, seek, track, panel, and sound controls.
type PlayerBar struct {
	art     lithic.Click
	artists map[string]*lithic.Click

	shuffle  lithic.IconButton
	previous lithic.IconButton
	play     lithic.IconButton
	next     lithic.IconButton
	repeat   lithic.IconButton
	like     lithic.IconButton
	download lithic.IconButton
	stop     lithic.IconButton
	queue    lithic.IconButton
	lyrics   lithic.IconButton
	now      lithic.IconButton
	mini     lithic.IconButton
	mute     lithic.IconButton
	sound    lithic.IconButton

	seek            lithic.Slider
	seekWasDragging bool
	seekPending     bool
	seekTarget      float64
	volume          lithic.Slider
	volumeWasDrag   bool
	volumePending   bool
	volumeTarget    int
	preMute         int
	volumeWheelTag  int

	soundPopover lithic.Popover
	soundPanel   SoundPanel

	waveformID      string
	waveformLoading bool
	waveform        []int
	wavePoints      []f32.Point
	waveWidth       int
	waveHeight      int
	lastToast       uint64
}

// Layout renders an 88 dp playback bar when a track is active.
func (b *PlayerBar) Layout(gtx layout.Context, e *env.Env) layout.Dimensions {
	snapshot := e.Store.Snapshot()
	b.forwardToast(e, snapshot.Toast)
	now := snapshot.Playback.Now
	if now == nil {
		return layout.Dimensions{}
	}
	current := e.Store.CurrentTrack()
	track := trackFromPlayback(now, current)
	provider := model.ProviderOf(track.VideoID)
	b.requestWaveform(e, track.VideoID, provider)
	b.reconcile(snapshot)
	b.pollActions(gtx, e, snapshot, track, provider)

	if !snapshot.Playback.Paused {
		// A visible player owns one coarse playback-clock redraw; hidden windows
		// do not lay out and therefore never arm this invalidation.
		gtx.Execute(op.InvalidateCmd{At: gtx.Now.Truncate(time.Second).Add(time.Second)})
	}

	height := min(gtx.Constraints.Max.Y, gtx.Dp(88))
	width := gtx.Constraints.Max.X
	if width <= 0 || height <= 0 {
		return layout.Dimensions{}
	}
	gtx.Constraints = layout.Exact(image.Pt(width, height))
	lithic.FillCutTop(gtx.Ops, image.Rect(0, 0, width, height), gtx.Dp(16), e.Th.R.Stone1)

	compact := width < gtx.Dp(760)
	leftWidth := min(gtx.Dp(340), width*28/100)
	rightWidth := min(gtx.Dp(420), width*36/100)
	if compact {
		rightWidth = gtx.Dp(176)
		leftWidth = min(gtx.Dp(360), max(gtx.Dp(240), width-rightWidth-gtx.Dp(204)))
	} else if width-leftWidth-rightWidth < gtx.Dp(220) {
		shortfall := gtx.Dp(220) - (width - leftWidth - rightWidth)
		leftWidth = max(gtx.Dp(150), leftWidth-shortfall/2)
		rightWidth = max(gtx.Dp(220), rightWidth-(shortfall-shortfall/2))
	}

	return layout.Inset{Top: 8, Bottom: 8, Left: 13, Right: 13}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(fixedWidth(leftWidth, func(gtx layout.Context) layout.Dimensions {
				return b.layoutTrack(gtx, e, snapshot, track, provider, compact)
			})),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return b.layoutTransport(gtx, e, snapshot)
			}),
			layout.Rigid(fixedWidth(rightWidth, func(gtx layout.Context) layout.Dimensions {
				return b.layoutTrailing(gtx, e, snapshot, track, provider, compact)
			})),
		)
	})
}

func (b *PlayerBar) forwardToast(e *env.Env, toast *state.Toast) {
	if toast == nil || toast.Serial == b.lastToast || e.Toast == nil {
		return
	}
	b.lastToast = toast.Serial
	kind := env.ToastInfo
	if toast.Kind == state.ToastError {
		kind = env.ToastError
	} else if toast.Kind == state.ToastSuccess {
		kind = env.ToastSuccess
	}
	e.Toast(toast.Message, kind)
}

func (b *PlayerBar) pollActions(gtx layout.Context, e *env.Env, snapshot state.State, track model.Track, provider model.Provider) {
	if b.art.Clicked(gtx) {
		e.Nav.TogglePanel(env.PanelNowPlaying)
	}
	if b.shuffle.Clicked(gtx) {
		callVoid(e, protocol.MethodToggleShuffle, protocol.Null{})
	}
	if b.previous.Clicked(gtx) {
		callVoid(e, protocol.MethodPrevTrack, protocol.Null{})
	}
	if b.play.Clicked(gtx) {
		callVoid(e, protocol.MethodTogglePause, protocol.Null{})
	}
	if b.next.Clicked(gtx) {
		callVoid(e, protocol.MethodNextTrack, protocol.Null{})
	}
	if b.repeat.Clicked(gtx) {
		next := model.RepeatAll
		if snapshot.Queue.Repeat == model.RepeatAll {
			next = model.RepeatOne
		} else if snapshot.Queue.Repeat == model.RepeatOne {
			next = model.RepeatOff
		}
		callVoid(e, protocol.MethodSetRepeat, protocol.SetRepeatParams{Mode: next})
	}
	if b.like.Clicked(gtx) {
		rating := model.RatingLike
		if snapshot.Rating == model.RatingLike {
			rating = model.RatingIndifferent
		}
		act.Rate(e, track, rating)
	}
	if b.download.Clicked(gtx) {
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
	if b.stop.Clicked(gtx) {
		callVoid(e, protocol.MethodSetStopAfterCurrent, protocol.SetStopAfterCurrentParams{Enabled: !snapshot.Playback.StopAfterCurrent})
	}
	if b.queue.Clicked(gtx) {
		e.Nav.TogglePanel(env.PanelQueue)
	}
	if provider != model.ProviderRadio && b.lyrics.Clicked(gtx) {
		e.Nav.TogglePanel(env.PanelLyrics)
	}
	if b.now.Clicked(gtx) {
		e.Nav.TogglePanel(env.PanelNowPlaying)
	}
	if b.mini.Clicked(gtx) {
		e.Nav.ToggleMini()
	}
	if b.mute.Clicked(gtx) {
		if b.volumeTarget == 0 {
			if b.preMute <= 0 {
				b.preMute = 70
			}
			b.setVolume(e, b.preMute)
		} else {
			b.preMute = b.volumeTarget
			b.setVolume(e, 0)
		}
	}
	if b.sound.Clicked(gtx) {
		b.soundPopover.Open = !b.soundPopover.Open
		if !b.soundPopover.Open {
			e.Store.SetVolumeDragging(false)
			b.soundPanel.volumeOn = false
		}
	}
}

func (b *PlayerBar) layoutTrack(gtx layout.Context, e *env.Env, snapshot state.State, track model.Track, provider model.Provider, compact bool) layout.Dimensions {
	canRate := provider != model.ProviderLocal && provider != model.ProviderRadio
	canDownload := provider != model.ProviderLocal && provider != model.ProviderRadio
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return b.art.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return widgets.Art(gtx, e, optionalString(track.Thumbnail), 56, 3)
			})
		}),
		layout.Rigid(layout.Spacer{Width: 13}.Layout),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return (lithic.Label{Style: e.Th.T.Row, Color: e.Th.R.Fg, MaxLines: 1}).Layout(gtx, e.Th, track.Title)
				}),
				layout.Rigid(layout.Spacer{Height: 4}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if compact {
						return b.layoutArtists(gtx, e, track)
					}
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return b.layoutArtists(gtx, e, track)
						}),
						layout.Rigid(layout.Spacer{Width: 8}.Layout),
						layout.Rigid(widgets.ProviderBadge(e, provider)),
					)
				}),
			)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if !canRate {
				return layout.Dimensions{}
			}
			b.like.Small = true
			icon := lithic.Icon("favorite_border")
			kind := lithic.Quiet
			if snapshot.Rating == model.RatingLike {
				icon, kind = lithic.Icon("favorite"), lithic.On
			}
			return b.like.Layout(gtx, e.Th, kind, icon, "Like")
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if !canDownload {
				return layout.Dimensions{}
			}
			return b.layoutDownload(gtx, e, currentDownload(snapshot.Downloads, track.VideoID))
		}),
	)
}

func (b *PlayerBar) layoutArtists(gtx layout.Context, e *env.Env, track model.Track) layout.Dimensions {
	if b.artists == nil {
		b.artists = make(map[string]*lithic.Click)
	}
	if len(track.ArtistRuns) == 0 {
		if track.ArtistID == nil {
			return (lithic.Label{Style: e.Th.T.Caption, Color: e.Th.R.Fg2, MaxLines: 1}).Layout(gtx, e.Th, track.Artists)
		}
		track.ArtistRuns = []model.Artist{{Text: track.Artists, ID: track.ArtistID}}
	}
	children := make([]layout.FlexChild, 0, len(track.ArtistRuns)*2)
	for index, artist := range track.ArtistRuns {
		if index > 0 {
			children = append(children, layout.Rigid(lithic.Text(e.Th, e.Th.T.Caption, e.Th.R.Fg3, " · ")))
		}
		artist := artist
		if artist.ID == nil || *artist.ID == "" {
			children = append(children, layout.Rigid(lithic.Text(e.Th, e.Th.T.Caption, e.Th.R.Fg2, artist.Text)))
			continue
		}
		click := b.artists[*artist.ID]
		if click == nil {
			click = new(lithic.Click)
			b.artists[*artist.ID] = click
		}
		if click.Clicked(gtx) {
			act.OpenArtist(e, *artist.ID, artist.Text)
		}
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return click.Layout(gtx, lithic.Text(e.Th, e.Th.T.Caption, e.Th.R.Fg2, artist.Text))
		}))
	}
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
}

func (b *PlayerBar) layoutTransport(gtx layout.Context, e *env.Env, snapshot state.State) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				b.shuffle.Small, b.previous.Small, b.next.Small, b.repeat.Small = true, true, true, true
				playIcon := lithic.Icon("play")
				playDesc := "Play"
				if !snapshot.Playback.Paused {
					playIcon, playDesc = lithic.Icon("pause"), "Pause"
				}
				repeatIcon, repeatDesc := lithic.Icon("repeat"), "Repeat off"
				if snapshot.Queue.Repeat == model.RepeatAll {
					repeatIcon, repeatDesc = lithic.Icon("repeat"), "Repeat all"
				} else if snapshot.Queue.Repeat == model.RepeatOne {
					repeatIcon, repeatDesc = lithic.Icon("repeat_one"), "Repeat one"
				}
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return b.shuffle.Layout(gtx, e.Th, activeKind(snapshot.Queue.Shuffle), lithic.Icon("shuffle"), "Shuffle")
					}),
					layout.Rigid(layout.Spacer{Width: 4}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return b.previous.Layout(gtx, e.Th, lithic.Quiet, lithic.Icon("skip_previous"), "Previous")
					}),
					layout.Rigid(layout.Spacer{Width: 4}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return b.play.Layout(gtx, e.Th, lithic.Gold, playIcon, playDesc)
					}),
					layout.Rigid(layout.Spacer{Width: 4}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return b.next.Layout(gtx, e.Th, lithic.Quiet, lithic.Icon("skip_next"), "Next")
					}),
					layout.Rigid(layout.Spacer{Width: 4}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return b.repeat.Layout(gtx, e.Th, activeKind(snapshot.Queue.Repeat != model.RepeatOff), repeatIcon, repeatDesc)
					}),
				)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return b.layoutSeek(gtx, e, snapshot)
		}),
	)
}

func (b *PlayerBar) layoutSeek(gtx layout.Context, e *env.Env, snapshot state.State) layout.Dimensions {
	duration := max(0, snapshot.Playback.Duration)
	position := e.Store.PositionAt(gtx.Now)
	if !b.seek.Dragging() && !b.seekPending {
		if duration > 0 {
			b.seek.Value = float32(position / duration)
		} else {
			b.seek.Value = 0
		}
	}
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(fixedWidth(gtx.Dp(50), lithic.Text(e.Th, e.Th.T.MonoSm, e.Th.R.Fg3, formatClock(position)))),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			before := b.seek.Value
			size := image.Pt(gtx.Constraints.Max.X, min(gtx.Constraints.Max.Y, gtx.Dp(28)))
			gtx.Constraints = layout.Exact(size)
			b.drawWaveform(gtx, e)
			dims := b.seek.Layout(gtx, e.Th, 0, float32(duration))
			dragging := b.seek.Dragging()
			target := float64(b.seek.Value) * duration
			changed := math.Abs(float64(b.seek.Value-before)) > 0.0001
			if dragging {
				e.Store.SetSeekDrag(&target)
			} else if b.seekWasDragging {
				e.Store.SetSeekDrag(nil)
				b.seekTarget, b.seekPending = target, true
				callVoid(e, protocol.MethodSeek, protocol.SeekParams{Position: target})
			} else if changed {
				b.seekTarget, b.seekPending = target, true
				callVoid(e, protocol.MethodSeek, protocol.SeekParams{Position: target})
			}
			b.seekWasDragging = dragging
			return dims
		}),
		layout.Rigid(fixedWidth(gtx.Dp(50), func(gtx layout.Context) layout.Dimensions {
			return layout.E.Layout(gtx, lithic.Text(e.Th, e.Th.T.MonoSm, e.Th.R.Fg3, formatClock(duration)))
		})),
	)
}

func (b *PlayerBar) layoutTrailing(gtx layout.Context, e *env.Env, snapshot state.State, track model.Track, provider model.Provider, compact bool) layout.Dimensions {
	panel := e.Nav.Panel()
	children := make([]layout.FlexChild, 0, 19)
	appendGap := func(width unit.Dp) {
		if len(children) > 0 {
			children = append(children, layout.Rigid(layout.Spacer{Width: width}.Layout))
		}
	}
	children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		b.stop.Small = true
		return b.stop.Layout(gtx, e.Th, activeKind(snapshot.Playback.StopAfterCurrent), lithic.Icon("timer"), "Stop after current")
	}))
	appendGap(4)
	children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		b.queue.Small = true
		return b.queue.Layout(gtx, e.Th, activeKind(panel == env.PanelQueue), lithic.Icon("queue"), "Queue")
	}))
	if !compact {
		appendGap(4)
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			b.lyrics.Small = true
			ctx := gtx
			if provider == model.ProviderRadio {
				ctx = ctx.Disabled()
			}
			return b.lyrics.Layout(ctx, e.Th, activeKind(panel == env.PanelLyrics), lithic.Icon("lyrics"), "Lyrics")
		}))
		appendGap(4)
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			b.now.Small = true
			return b.now.Layout(gtx, e.Th, activeKind(panel == env.PanelNowPlaying), lithic.Icon("equalizer"), "Now playing")
		}))
	}
	appendGap(4)
	children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		b.mini.Small = true
		return b.mini.Layout(gtx, e.Th, lithic.Quiet, lithic.Icon("open_in_new"), "Mini player")
	}))
	appendGap(8)
	children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		b.mute.Small = true
		icon := lithic.Icon("volume_up")
		if b.volumeTarget == 0 {
			icon = lithic.Icon("volume_off")
		} else if b.volumeTarget < 50 {
			icon = lithic.Icon("volume_down")
		}
		return b.mute.Layout(gtx, e.Th, lithic.Quiet, icon, "Mute")
	}))
	if !compact {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X, gtx.Constraints.Max.X = gtx.Dp(84), gtx.Dp(104)
			return b.layoutVolume(gtx, e)
		}))
	}
	appendGap(4)
	children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return b.soundPopover.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				b.sound.Small = true
				return b.sound.Layout(gtx, e.Th, activeKind(b.soundPopover.Open), lithic.Icon("tune"), "Sound")
			},
			func(gtx layout.Context) layout.Dimensions { return b.soundPanel.Layout(gtx, e) },
		)
	}))
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceEnd}.Layout(gtx, children...)
}

func (b *PlayerBar) layoutVolume(gtx layout.Context, e *env.Env) layout.Dimensions {
	for {
		raw, ok := gtx.Event(pointer.Filter{
			Target: &b.volumeWheelTag, Kinds: pointer.Scroll,
			ScrollY: pointer.ScrollRange{Min: -1e6, Max: 1e6},
		})
		if !ok {
			break
		}
		event, ok := raw.(pointer.Event)
		if !ok || event.Scroll.Y == 0 {
			continue
		}
		value := b.volumeTarget
		if event.Scroll.Y < 0 {
			value += 5
		} else {
			value -= 5
		}
		b.setVolume(e, value)
	}
	before := b.volume.Value
	dims := b.volume.Layout(gtx, e.Th, 0, 100)
	region := clip.Rect(image.Rectangle{Max: dims.Size}).Push(gtx.Ops)
	event.Op(gtx.Ops, &b.volumeWheelTag)
	region.Pop()
	dragging := b.volume.Dragging()
	if dragging != b.volumeWasDrag {
		e.Store.SetVolumeDragging(dragging)
		b.volumeWasDrag = dragging
	}
	if math.Abs(float64(b.volume.Value-before)) > 0.0001 {
		b.setVolume(e, int(math.Round(float64(b.volume.Value*100))))
	}
	return dims
}

func (b *PlayerBar) layoutDownload(gtx layout.Context, e *env.Env, job *model.DownloadEntry) layout.Dimensions {
	b.download.Small = true
	icon, desc := lithic.Icon("download"), "Download"
	if job != nil {
		switch job.Status {
		case model.DownloadStatusQueued:
			icon, desc = lithic.Icon("download"), "Queued. Open downloads"
		case model.DownloadStatusDownloading:
			icon, desc = lithic.Icon("download"), "Downloading. Open downloads"
		case model.DownloadStatusCompleted:
			icon, desc = lithic.Icon("check"), "Downloaded. Open downloads"
		case model.DownloadStatusFailed:
			icon, desc = lithic.Icon("error"), "Download failed. Retry"
		case model.DownloadStatusCancelled:
			icon, desc = lithic.Icon("refresh"), "Download cancelled. Retry"
		}
	}
	dims := b.download.Layout(gtx, e.Th, lithic.Quiet, icon, desc)
	if job == nil || (job.Status != model.DownloadStatusQueued && job.Status != model.DownloadStatusDownloading) {
		return dims
	}
	progress := job.Progress
	if job.Status == model.DownloadStatusQueued && progress == 0 {
		progress = 4
	}
	height := max(1, gtx.Dp(4))
	y := max(0, dims.Size.Y-height)
	lithic.FillCut(gtx.Ops, image.Rect(0, y, dims.Size.X, dims.Size.Y), 0, e.Th.R.Stone3)
	end := dims.Size.X * max(0, min(100, progress)) / 100
	if end > 0 {
		lithic.Blade(gtx.Ops, image.Rect(0, y, end, dims.Size.Y), e.Th.R.Lapis)
	}
	return dims
}

func (b *PlayerBar) reconcile(snapshot state.State) {
	if b.volumeTarget == 0 && !b.volumePending && snapshot.Playback.Volume > 0 {
		b.volumeTarget = snapshot.Playback.Volume
		b.preMute = snapshot.Playback.Volume
	}
	if b.volumePending {
		if snapshot.Playback.Volume == b.volumeTarget {
			b.volumePending = false
		} else {
			b.volume.Value = float32(b.volumeTarget) / 100
		}
	} else if !b.volume.Dragging() {
		b.volumeTarget = max(0, min(100, snapshot.Playback.Volume))
		b.volume.Value = float32(b.volumeTarget) / 100
	}
	if b.seekPending && math.Abs(snapshot.Playback.Position-b.seekTarget) < 1.5 {
		b.seekPending = false
	}
}

func (b *PlayerBar) setVolume(e *env.Env, value int) {
	value = max(0, min(100, value))
	b.volumeTarget = value
	b.volume.Value = float32(value) / 100
	b.volumePending = true
	callVoid(e, protocol.MethodSetVolume, protocol.SetVolumeParams{Volume: value})
}

func (b *PlayerBar) requestWaveform(e *env.Env, videoID string, provider model.Provider) {
	if provider != model.ProviderSoundCloud {
		b.waveformID, b.waveformLoading, b.waveform = "", false, nil
		return
	}
	if videoID == b.waveformID {
		return
	}
	b.waveformID, b.waveformLoading, b.waveform = videoID, true, nil
	b.waveWidth, b.waveHeight = 0, 0
	act.Call[model.Waveform](e, protocol.MethodGetWaveform, protocol.IDParams{ID: videoID}, func(wave model.Waveform, err error) {
		if b.waveformID != videoID {
			return
		}
		b.waveformLoading = false
		if err == nil {
			b.waveform = append(b.waveform[:0], wave.Samples...)
		}
	})
}

func (b *PlayerBar) drawWaveform(gtx layout.Context, e *env.Env) {
	if len(b.waveform) < 2 {
		return
	}
	width, height := gtx.Constraints.Max.X, gtx.Constraints.Max.Y
	if width <= 0 || height <= 0 {
		return
	}
	if width != b.waveWidth || height != b.waveHeight || len(b.wavePoints) != len(b.waveform) {
		b.wavePoints = make([]f32.Point, len(b.waveform))
		last := max(1, len(b.waveform)-1)
		for index, sample := range b.waveform {
			x := float32(index*max(0, width-1)) / float32(last)
			y := float32(height-2) - float32(max(0, min(100, sample)))*float32(max(1, height-4))/100
			b.wavePoints[index] = f32.Pt(x, y)
		}
		b.waveWidth, b.waveHeight = width, height
	}
	var area clip.Path
	area.Begin(gtx.Ops)
	area.MoveTo(b.wavePoints[0])
	for _, point := range b.wavePoints[1:] {
		area.LineTo(point)
	}
	area.LineTo(f32.Pt(float32(width), float32(height)))
	area.LineTo(f32.Pt(0, float32(height)))
	area.Close()
	lithic.Hatch(gtx.Ops, area.End(), image.Rect(0, 0, width, height), max(1, gtx.Dp(6)), max(1, gtx.Dp(1)), lithic.WithAlpha(e.Th.R.Fg, .18))
	lithic.Polyline(gtx.Ops, b.wavePoints, false, float32(max(1, gtx.Dp(unit.Dp(1)))), e.Th.R.Fg3)
}

func fixedWidth(width int, widget layout.Widget) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		width = max(0, min(width, gtx.Constraints.Max.X))
		gtx.Constraints.Min.X, gtx.Constraints.Max.X = width, width
		return widget(gtx)
	}
}
