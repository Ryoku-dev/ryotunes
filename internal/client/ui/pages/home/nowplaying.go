package home

import (
	"image"
	"image/color"
	"math"
	"strings"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/anim"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/tokens"
	"github.com/ryoku-dev/ryotunes/internal/client/state"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/widgets"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

type nowPlayingArtistLink struct {
	click kit.Click
	hover look.Hover
	value float32
}

type nowPlayingCard struct {
	ready  bool
	open   bool
	height anim.Tween
	reveal anim.Tween

	snapshot state.State
	track    model.Track
	previous model.Track

	art              widgets.PlayArt
	identity         look.Swap
	title            kit.Click
	source           kit.Click
	sourceHover      look.Hover
	sourceHoverValue float32
	artists          map[string]*nowPlayingArtistLink

	seek      widgets.Seek
	transport widgets.Transport

	like      kit.IconPlaque
	likeFill  look.Fill
	likeTip   look.Tip
	queue     kit.IconPlaque
	queueTip  look.Tip
	lyrics    kit.IconPlaque
	lyricsTip look.Tip
	now       kit.IconPlaque
	nowTip    look.Tip

	compactPlay    kit.IconPlaque
	compactMorph   look.Morph
	compactPlayTip look.Tip

	instrument    kit.Click
	instrumentTip look.Tip
	visualizer    look.Visualizer
}

// Layout grows the card into the Home field only while a track is loaded.
func (card *nowPlayingCard) Layout(gtx layout.Context, e *env.Env) layout.Dimensions {
	if e == nil || e.Th == nil || e.Store == nil {
		return layout.Dimensions{}
	}
	snapshot := e.Store.Snapshot()
	loaded := snapshot.Playback.Now != nil
	if loaded {
		track := homeTrack(snapshot.Playback.Now, e.Store.CurrentTrack())
		if track.VideoID != card.track.VideoID {
			if card.track.VideoID != "" {
				card.previous = card.track
			}
			card.artists = nil
		}
		card.track = track
		card.snapshot = snapshot
	}

	card.height.Duration, card.height.Ease = e.Th.Motion.Unfold, anim.EaseChisel
	card.reveal.Duration, card.reveal.Ease = e.Th.Motion.Navigate, anim.EaseChisel
	if !card.ready {
		card.ready = true
		card.height.Set(gtx, 0)
		card.reveal.Set(gtx, 0)
	}
	if loaded != card.open {
		card.open = loaded
		target := float32(0)
		if loaded {
			target = 1
		}
		card.height.Set(gtx, target)
		card.reveal.Set(gtx, target)
	}
	heightProgress := card.height.Value(gtx)
	revealProgress := card.reveal.Value(gtx)
	if !loaded && heightProgress <= 0 && revealProgress <= 0 {
		card.track = model.Track{}
		card.previous = model.Track{}
		card.artists = nil
		return layout.Dimensions{}
	}

	contentGTX := gtx
	contentGTX.Constraints.Min = image.Point{}
	contentGTX.Constraints.Max.Y = 1 << 20
	if !loaded {
		contentGTX = contentGTX.Disabled()
	}
	record := op.Record(gtx.Ops)
	full := layout.Inset{Top: 21}.Layout(contentGTX, func(gtx layout.Context) layout.Dimensions {
		return card.layoutSlab(gtx, e)
	})
	call := record.Stop()
	visibleHeight := int(math.Round(float64(float32(full.Size.Y) * heightProgress)))
	visibleHeight = min(max(visibleHeight, 0), full.Size.Y)
	if visibleHeight == 0 {
		return layout.Dimensions{Size: image.Pt(full.Size.X, 0)}
	}

	area := clip.Rect(image.Rect(0, 0, full.Size.X, visibleHeight)).Push(gtx.Ops)
	opacity := paint.PushOpacity(gtx.Ops, min(max(revealProgress, 0), 1))
	rise := int(math.Round(float64(float32(gtx.Dp(8)) * (1 - revealProgress))))
	offset := op.Offset(image.Pt(0, rise)).Push(gtx.Ops)
	call.Add(gtx.Ops)
	offset.Pop()
	opacity.Pop()
	area.Pop()
	return layout.Dimensions{Size: image.Pt(full.Size.X, visibleHeight)}
}

func (card *nowPlayingCard) layoutSlab(gtx layout.Context, e *env.Env) layout.Dimensions {
	gtx.Constraints.Min.X = gtx.Constraints.Max.X
	record := op.Record(gtx.Ops)
	dimensions := layout.UniformInset(13).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		width := int(gtx.Metric.PxToDp(gtx.Constraints.Max.X + gtx.Dp(26)))
		switch {
		case width >= 720:
			return card.layoutFull(gtx, e, 144, true)
		case width >= 600:
			return card.layoutFull(gtx, e, 112, false)
		default:
			return card.layoutCompact(gtx, e)
		}
	})
	content := record.Stop()
	bounds := image.Rectangle{Max: dimensions.Size}
	cut := gtx.Dp(e.Th.Cut.Mid)
	kit.FillCutCorners(gtx.Ops, bounds, cut, 0, cut, 0, e.Th.R.Stone1)
	content.Add(gtx.Ops)
	return dimensions
}

func (card *nowPlayingCard) layoutFull(gtx layout.Context, e *env.Env, artDp int, instrument bool) layout.Dimensions {
	artSide := gtx.Dp(unit.Dp(artDp))
	children := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			size := image.Pt(artSide, artSide)
			return card.art.Layout(gtx, e, homeString(card.track.Thumbnail), size, look.Shape{Cut: 7}, e.Th.Motion.Field)
		}),
		layout.Rigid(layout.Spacer{Width: 21}.Layout),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return card.layoutText(gtx, e)
		}),
	}
	if instrument && e.Fx.Visualizer {
		cardWidth := gtx.Constraints.Max.X + gtx.Dp(26)
		instrumentWidth := min(max(cardWidth*3/10, gtx.Dp(200)), gtx.Dp(360))
		if e.Fx.Style == state.VisualizerVectorscope {
			instrumentWidth = artSide
		}
		children = append(children,
			layout.Rigid(layout.Spacer{Width: 21}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints = layout.Exact(image.Pt(instrumentWidth, artSide))
				return card.layoutInstrument(gtx, e)
			}),
		)
	}
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
}

func (card *nowPlayingCard) layoutText(gtx layout.Context, e *env.Env) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical, Alignment: layout.Start}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return card.layoutState(gtx, e)
		}),
		layout.Rigid(layout.Spacer{Height: 8}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return card.layoutIdentity(gtx, e, false)
		}),
		layout.Rigid(layout.Spacer{Height: 13}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = 0
			gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(420))
			return card.seek.Layout(gtx, e, card.snapshot, card.track)
		}),
		layout.Rigid(layout.Spacer{Height: 8}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return card.layoutActions(gtx, e)
		}),
	)
}

func (card *nowPlayingCard) layoutState(gtx layout.Context, e *env.Env) layout.Dimensions {
	word, flake := "Playing", e.Th.Accent.Ink
	if card.snapshot.PendingVideoID != "" {
		word, flake = "Loading", e.Th.R.Lapis
	} else if card.snapshot.Playback.Paused {
		word, flake = "Paused", e.Th.R.Amber
	}
	children := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return playingFlake(gtx, flake) }),
		layout.Rigid(layout.Spacer{Width: 8}.Layout),
		layout.Rigid(kit.Text(e.Th.Theme, e.Th.T.Label, e.Th.R.Fg2, word)),
	}
	if source := homeString(card.snapshot.Queue.SourceName); source != "" {
		children = append(children,
			layout.Rigid(layout.Spacer{Width: 4}.Layout),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				dimensions, nextHover := card.sourceHover.Layout(gtx, e.Th, func(gtx layout.Context) layout.Dimensions {
					return card.source.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						ink := palette.Mix(e.Th.R.Fg3, e.Th.R.Fg, card.sourceHoverValue)
						return (kit.Label{Style: e.Th.T.Caption, Color: ink, MaxLines: 1}).Layout(gtx, e.Th.Theme, "· From "+source)
					})
				})
				card.sourceHoverValue = nextHover
				if card.source.Clicked(gtx) {
					card.openSource(e, source)
				}
				return dimensions
			}),
		)
	}
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
}

func playingFlake(gtx layout.Context, ink color.NRGBA) layout.Dimensions {
	height := max(1, gtx.Dp(9)*7/10)
	width := max(1, gtx.Dp(9)*14/10)
	dimensions := layout.Dimensions{Size: gtx.Constraints.Constrain(image.Pt(width, height))}
	cut := min(dimensions.Size.Y, dimensions.Size.X/2)
	kit.FillCutCorners(gtx.Ops, image.Rectangle{Max: dimensions.Size}, cut, 0, cut, 0, ink)
	return dimensions
}

func (card *nowPlayingCard) layoutIdentity(gtx layout.Context, e *env.Env, compact bool) layout.Dimensions {
	key := card.track.VideoID
	return card.identity.Layout(gtx, e.Th, key, 8, e.Th.Motion.Navigate, func(gtx layout.Context, requested string) layout.Dimensions {
		track := card.trackForKey(requested)
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.Start}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				dimensions := card.title.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return (kit.Label{Style: e.Th.T.Sentence, Color: e.Th.R.Fg, MaxLines: 1}).Layout(gtx, e.Th.Theme, homeTrackTitle(track))
				})
				if card.title.Clicked(gtx) {
					e.Nav.ToggleNowPlaying()
				}
				return dimensions
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return card.layoutArtists(gtx, e, track)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if compact {
					return layout.Dimensions{}
				}
				album := strings.TrimSpace(homeString(track.Album))
				if album == "" {
					return layout.Dimensions{}
				}
				return (kit.Label{Style: e.Th.T.Caption, Color: e.Th.R.Fg3, MaxLines: 1}).Layout(gtx, e.Th.Theme, album)
			}),
		)
	})
}

func (card *nowPlayingCard) layoutArtists(gtx layout.Context, e *env.Env, track model.Track) layout.Dimensions {
	if card.artists == nil {
		card.artists = make(map[string]*nowPlayingArtistLink)
	}
	runs := track.ArtistRuns
	if len(runs) == 0 && strings.TrimSpace(track.Artists) != "" {
		runs = []model.Artist{{Text: strings.TrimSpace(track.Artists), ID: track.ArtistID}}
	}
	children := make([]layout.FlexChild, 0, len(runs)*2)
	for _, artist := range runs {
		name := strings.TrimSpace(artist.Text)
		if name == "" {
			continue
		}
		if len(children) > 0 {
			children = append(children, layout.Rigid(kit.Text(e.Th.Theme, e.Th.T.Proof, e.Th.R.Fg2, ", ")))
		}
		id := strings.TrimSpace(homeString(artist.ID))
		if id == "" {
			children = append(children, layout.Rigid(kit.Text(e.Th.Theme, e.Th.T.Proof, e.Th.R.Fg2, name)))
			continue
		}
		key := id + "\x00" + name
		link := card.artists[key]
		if link == nil {
			link = new(nowPlayingArtistLink)
			card.artists[key] = link
		}
		artistID, artistName, artistLink := id, name, link
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			dimensions, nextHover := artistLink.hover.Layout(gtx, e.Th, func(gtx layout.Context) layout.Dimensions {
				return artistLink.click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					ink := palette.Mix(e.Th.R.Fg2, e.Th.R.Fg, artistLink.value)
					return (kit.Label{Style: e.Th.T.Proof, Color: ink, MaxLines: 1}).Layout(gtx, e.Th.Theme, artistName)
				})
			})
			artistLink.value = nextHover
			if artistLink.click.Clicked(gtx) {
				act.OpenArtist(e, artistID, artistName)
			}
			return dimensions
		}))
	}
	if len(children) == 0 {
		return layout.Dimensions{}
	}
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
}

func (card *nowPlayingCard) layoutActions(gtx layout.Context, e *env.Env) layout.Dimensions {
	card.transport.Basic, card.transport.Gap = true, 13
	card.like.Small, card.like.Kind, card.like.Icon, card.like.Description = true, kit.PlaqueQuiet, nil, "Like"
	liked := card.snapshot.Rating == model.RatingLike
	like := func(gtx layout.Context) layout.Dimensions {
		return card.likeTip.Layout(gtx, e.Th, e.Overlays, "Like", func(gtx layout.Context) layout.Dimensions {
			return layout.Stack{Alignment: layout.Center}.Layout(gtx,
				layout.Stacked(func(gtx layout.Context) layout.Dimensions { return card.like.Layout(gtx, e.Th.Theme) }),
				layout.Expanded(func(gtx layout.Context) layout.Dimensions {
					return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return card.likeFill.Layout(gtx, e.Th, liked,
							func(gtx layout.Context) layout.Dimensions { return look.Icon.HeartOutline.Layout(gtx, e.Th.R.Fg2) },
							func(gtx layout.Context) layout.Dimensions { return look.Icon.Heart.Layout(gtx, e.Th.Accent.Ink) },
						)
					})
				}),
			)
		})
	}
	panel := e.Nav.Panel()
	card.queue.Small, card.queue.Icon, card.queue.Description = true, look.Icon.Queue, "Queue"
	card.queue.Kind = kit.PlaqueQuiet
	if panel == env.PanelQueue {
		card.queue.Kind = kit.PlaqueOn
	}
	card.lyrics.Small, card.lyrics.Icon, card.lyrics.Description = true, look.Icon.Lyrics, "Lyrics"
	card.lyrics.Kind = kit.PlaqueQuiet
	if panel == env.PanelLyrics {
		card.lyrics.Kind = kit.PlaqueOn
	}
	card.now.Small, card.now.Icon, card.now.Description = true, look.Icon.NowPlaying, "Now playing"
	card.now.Kind = kit.PlaqueQuiet
	if e.Nav.NowPlaying() {
		card.now.Kind = kit.PlaqueOn
	}

	smallPlaque := unit.Dp(tokens.SizeIconPlaqueSmall)
	basicTransport := smallPlaque*2 + unit.Dp(tokens.SizeIconPlaque) + 26
	baseActions := basicTransport + 21 + smallPlaque*2 + 4
	showLyrics := gtx.Constraints.Max.X >= gtx.Dp(baseActions+4+smallPlaque)
	showNow := gtx.Constraints.Max.X >= gtx.Dp(baseActions+8+smallPlaque*2)
	var children [9]layout.FlexChild
	count := 0
	add := func(child layout.FlexChild) {
		children[count] = child
		count++
	}
	add(layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return card.transport.Layout(gtx, e, card.snapshot)
	}))
	add(layout.Rigid(layout.Spacer{Width: 21}.Layout))
	add(layout.Rigid(like))
	add(layout.Rigid(layout.Spacer{Width: 4}.Layout))
	add(layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return card.queueTip.Layout(gtx, e.Th, e.Overlays, "Queue", func(gtx layout.Context) layout.Dimensions {
			return card.queue.Layout(gtx, e.Th.Theme)
		})
	}))
	if showLyrics {
		add(layout.Rigid(layout.Spacer{Width: 4}.Layout))
		add(layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			buttonGTX := gtx
			if model.ProviderOf(card.track.VideoID) == model.ProviderRadio {
				buttonGTX = buttonGTX.Disabled()
			}
			return card.lyricsTip.Layout(buttonGTX, e.Th, e.Overlays, "Lyrics", func(gtx layout.Context) layout.Dimensions {
				return card.lyrics.Layout(gtx, e.Th.Theme)
			})
		}))
	}
	if showNow {
		add(layout.Rigid(layout.Spacer{Width: 4}.Layout))
		add(layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return card.nowTip.Layout(gtx, e.Th, e.Overlays, "Now playing", func(gtx layout.Context) layout.Dimensions {
				return card.now.Layout(gtx, e.Th.Theme)
			})
		}))
	}

	dimensions := layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children[:count]...)
	if card.like.Clicked(gtx) {
		rating := model.RatingLike
		if liked {
			rating = model.RatingIndifferent
		}
		act.Rate(e, card.track, rating)
	}
	if card.queue.Clicked(gtx) {
		e.Nav.TogglePanel(env.PanelQueue)
	}
	if showLyrics && model.ProviderOf(card.track.VideoID) != model.ProviderRadio && card.lyrics.Clicked(gtx) {
		e.Nav.TogglePanel(env.PanelLyrics)
	}
	if showNow && card.now.Clicked(gtx) {
		e.Nav.ToggleNowPlaying()
	}
	return dimensions
}

func (card *nowPlayingCard) layoutCompact(gtx layout.Context, e *env.Env) layout.Dimensions {
	artSide := gtx.Dp(72)
	playIcon, playDescription := look.Icon.Pause, "Pause"
	if card.snapshot.Playback.Paused {
		playIcon, playDescription = look.Icon.Play, "Play"
	}
	dimensions := layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			size := image.Pt(artSide, artSide)
			return card.art.Layout(gtx, e, homeString(card.track.Thumbnail), size, look.Shape{Cut: 7}, e.Th.Motion.Field)
		}),
		layout.Rigid(layout.Spacer{Width: 13}.Layout),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return card.layoutIdentity(gtx, e, true)
		}),
		layout.Rigid(layout.Spacer{Width: 13}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return card.compactPlayTip.Layout(gtx, e.Th, e.Overlays, playDescription, func(gtx layout.Context) layout.Dimensions {
				return widgets.PlayPlaque(gtx, e.Th, &card.compactPlay, &card.compactMorph, playIcon, playDescription)
			})
		}),
	)
	if card.compactPlay.Clicked(gtx) {
		act.Call[protocol.Null](e, protocol.MethodTogglePause, protocol.Null{}, func(_ protocol.Null, err error) {
			if err != nil && e.Toast != nil {
				e.Toast(err.Error(), env.ToastError)
			}
		})
	}
	return dimensions
}

func (card *nowPlayingCard) layoutInstrument(gtx layout.Context, e *env.Env) layout.Dimensions {
	styleName := visualizerStyleName(e.Fx.Style)
	tip := "Visualizer: " + styleName + " · click to change"
	dimensions := card.instrumentTip.Layout(gtx, e.Th, e.Overlays, tip, func(gtx layout.Context) layout.Dimensions {
		return card.instrument.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			size := gtx.Constraints.Max
			gtx.Constraints = layout.Exact(size)
			record := op.Record(gtx.Ops)
			inset := gtx.Dp(13)
			labelTop := gtx.Dp(8)
			labelGTX := gtx
			labelGTX.Constraints.Min = image.Point{}
			labelGTX.Constraints.Max = image.Pt(max(0, size.X-2*inset), size.Y)
			labelRecord := op.Record(gtx.Ops)
			labelDimensions := kit.Text(e.Th.Theme, e.Th.T.Mono, e.Th.R.Fg3, styleName)(labelGTX)
			labelCall := labelRecord.Stop()

			drawingTop := labelTop + labelDimensions.Size.Y + gtx.Dp(5)
			visualizerGTX := gtx
			visualizerGTX.Constraints = layout.Exact(image.Pt(
				max(0, size.X-2*inset),
				max(0, size.Y-drawingTop-inset),
			))
			offset := op.Offset(image.Pt(inset, drawingTop)).Push(gtx.Ops)
			widgets.Visualizer(visualizerGTX, e, &card.visualizer, look.VisualizerStyle{
				Ink: e.Th.Accent.Ink, Bar: 7, Gap: 3, Floor: 2,
			})
			offset.Pop()

			labelOffset := op.Offset(image.Pt(inset, labelTop)).Push(gtx.Ops)
			labelCall.Add(gtx.Ops)
			labelOffset.Pop()
			content := record.Stop()
			bounds := image.Rectangle{Max: size}
			cut := gtx.Dp(e.Th.Cut.Small)
			kit.FillCut(gtx.Ops, bounds, cut, e.Th.R.Stone2)
			kit.StrokeCut(gtx.Ops, bounds, cut, max(1, gtx.Dp(1)), e.Th.R.Vein)
			content.Add(gtx.Ops)
			return layout.Dimensions{Size: size}
		})
	})
	if card.instrument.Clicked(gtx) {
		spectrumInstalled := e.Spectrum != nil && e.Spectrum.Installed()
		scopeInstalled := e.Scope != nil && e.Scope.Installed()
		preferences := card.snapshot.Client
		preferences.VisualizerStyle = string(nextVisualizerStyle(preferences.Style(), spectrumInstalled, scopeInstalled))
		e.Store.SetClientPreferences(preferences)
		if err := e.Store.SaveClientPreferences(); err != nil && e.Toast != nil {
			e.Toast("Could not save visualizer style", env.ToastError)
		}
	}
	return dimensions
}

func (card *nowPlayingCard) trackForKey(key string) model.Track {
	if card.previous.VideoID == key {
		return card.previous
	}
	return card.track
}

func (card *nowPlayingCard) openSource(e *env.Env, source string) {
	stack := e.Nav.Stack()
	for index := len(stack) - 1; index >= 0; index-- {
		route := stack[index]
		if strings.EqualFold(strings.TrimSpace(route.Title), source) && route.ID != "" {
			e.Nav.Push(route)
			return
		}
	}
	e.Nav.OpenPanel(env.PanelQueue)
}

func nextVisualizerStyle(current state.VisualizerStyle, spectrumInstalled, scopeInstalled bool) state.VisualizerStyle {
	styles := state.VisualizerStyles
	if len(styles) == 0 {
		return current
	}
	currentIndex := -1
	for index, style := range styles {
		if style == current {
			currentIndex = index
			break
		}
	}
	for step := 1; step <= len(styles); step++ {
		candidate := styles[(currentIndex+step)%len(styles)]
		if visualizerStyleInstalled(candidate, spectrumInstalled, scopeInstalled) {
			return candidate
		}
	}
	return current
}

func visualizerStyleInstalled(style state.VisualizerStyle, spectrumInstalled, scopeInstalled bool) bool {
	switch style {
	case state.VisualizerOscilloscope, state.VisualizerVectorscope:
		return scopeInstalled
	default:
		return spectrumInstalled
	}
}

func visualizerStyleName(style state.VisualizerStyle) string {
	switch style {
	case state.VisualizerCurve:
		return "Curve"
	case state.VisualizerOscilloscope:
		return "Scope"
	case state.VisualizerVectorscope:
		return "Stereo"
	default:
		return "Bars"
	}
}

func homeTrack(now *model.PlaybackTrack, current *model.Track) model.Track {
	if now == nil {
		return model.Track{}
	}
	if current != nil && current.VideoID == now.VideoID {
		track := *current
		track.Title, track.Artists = now.Title, now.Artists
		track.ArtistID, track.ArtistRuns = now.ArtistID, now.ArtistRuns
		track.Thumbnail, track.Duration, track.Rating = now.Thumbnail, now.Duration, now.Rating
		track.Plays, track.Likes, track.Genre = now.Plays, now.Likes, now.Genre
		return track
	}
	return model.Track{
		VideoID: now.VideoID, Title: now.Title, Artists: now.Artists,
		ArtistID: now.ArtistID, ArtistRuns: now.ArtistRuns,
		Thumbnail: now.Thumbnail, Duration: now.Duration, Rating: now.Rating,
		Plays: now.Plays, Likes: now.Likes, Genre: now.Genre,
	}
}

func homeTrackTitle(track model.Track) string {
	if title := strings.TrimSpace(track.Title); title != "" {
		return title
	}
	return "Untitled track"
}

func homeString(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}
