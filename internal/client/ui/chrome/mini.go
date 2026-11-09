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
	"gioui.org/widget"

	"github.com/ryoku-dev/ryotunes/internal/client/lithic"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

// Mini owns the complete Ryotunes Mini window content.
type Mini struct {
	lyrics          LyricsPanel
	seek            lithic.Slider
	seekWasDragging bool
	artMode         lithic.Button
	lyricsMode      lithic.Button
	previous        lithic.IconButton
	playPause       lithic.IconButton
	next            lithic.IconButton
	restore         lithic.IconButton
	showLyrics      bool
}

// Layout renders the compact artwork, lyrics, seek, and transport surface.
func (m *Mini) Layout(gtx layout.Context, e *env.Env) layout.Dimensions {
	state := e.Store.Snapshot()
	lithic.FillCutTop(gtx.Ops, image.Rectangle{Max: gtx.Constraints.Max}, gtx.Dp(e.Th.Cut.Large), e.Th.R.Stone1)
	if state.Playback.Now == nil {
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(lithic.Text(e.Th, e.Th.T.Title, e.Th.R.Fg, "Nothing playing")),
				layout.Rigid(layout.Spacer{Height: e.Th.Sp.S2}.Layout),
				layout.Rigid(lithic.Text(e.Th, e.Th.T.Body, e.Th.R.Fg3, "Play a track from the main window.")),
				layout.Rigid(layout.Spacer{Height: e.Th.Sp.S4}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					dims := m.restore.Layout(gtx, e.Th, lithic.Quiet, lithic.Icon("open_in_new"), "Restore main window")
					if m.restore.Clicked(gtx) {
						e.Nav.ToggleMini()
					}
					return dims
				}),
			)
		})
	}

	height := gtx.Constraints.Max.Y
	stageHeight := max(gtx.Dp(150), int(float32(height)*.48))
	stageHeight = min(stageHeight, max(0, height-gtx.Dp(190)))
	if stageHeight <= 0 {
		stageHeight = max(0, height/2)
	}
	bodyInset := layout.Inset{Top: e.Th.Sp.S3, Right: e.Th.Sp.S4, Bottom: e.Th.Sp.S3, Left: e.Th.Sp.S4}
	title := state.Playback.Now.Title
	if title == "" {
		title = "Current track"
	}
	artists := state.Playback.Now.Artists
	if artists == "" {
		artists = "Waiting for metadata"
	}

	dims := layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.Y = stageHeight
			gtx.Constraints.Max.Y = stageHeight
			if m.showLyrics {
				return layout.Inset{Top: e.Th.Sp.S3, Right: e.Th.Sp.S3, Bottom: e.Th.Sp.S3, Left: e.Th.Sp.S3}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return m.lyrics.LayoutCompact(gtx, e)
				})
			}
			return m.layoutArt(gtx, e, state.Playback.Now.Thumbnail)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return bodyInset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
									layout.Rigid(lithic.Text(e.Th, e.Th.T.Title, e.Th.R.Fg, title)),
									layout.Rigid(lithic.Text(e.Th, e.Th.T.Caption, e.Th.R.Fg3, artists)),
								)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								dims := m.restore.Layout(gtx, e.Th, lithic.Quiet, lithic.Icon("open_in_new"), "Restore main window")
								if m.restore.Clicked(gtx) {
									e.Nav.ToggleMini()
								}
								return dims
							}),
						)
					}),
					layout.Rigid(layout.Spacer{Height: e.Th.Sp.S2}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions { return m.layoutSeek(gtx, e) }),
					layout.Rigid(layout.Spacer{Height: e.Th.Sp.S2}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								kind := lithic.Quiet
								if !m.showLyrics {
									kind = lithic.On
								}
								m.artMode.Small = true
								dims := m.artMode.Layout(gtx, e.Th, kind, lithic.Icon("album"), "Art")
								if m.artMode.Clicked(gtx) {
									m.showLyrics = false
								}
								return dims
							}),
							layout.Rigid(layout.Spacer{Width: e.Th.Sp.S2}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								kind := lithic.Quiet
								if m.showLyrics {
									kind = lithic.On
								}
								m.lyricsMode.Small = true
								dims := m.lyricsMode.Layout(gtx, e.Th, kind, lithic.Icon("lyrics"), "Lyrics")
								if m.lyricsMode.Clicked(gtx) {
									m.showLyrics = true
								}
								return dims
							}),
							layout.Flexed(1, func(layout.Context) layout.Dimensions { return layout.Dimensions{} }),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions { return m.layoutTransport(gtx, e) }),
						)
					}),
				)
			})
		}),
	)
	if !state.Playback.Paused && !m.showLyrics {
		nextSecond := gtx.Now.Truncate(time.Second).Add(time.Second)
		gtx.Execute(op.InvalidateCmd{At: nextSecond})
	}
	return dims
}

func (m *Mini) layoutArt(gtx layout.Context, e *env.Env, thumbnail *string) layout.Dimensions {
	size := gtx.Constraints.Max
	cut := gtx.Dp(e.Th.Cut.Large)
	path := lithic.CutPath(gtx.Ops, image.Rectangle{Max: size}, cut, cut, 0, 0)
	mask := clip.Outline{Path: path}.Op().Push(gtx.Ops)
	paint.Fill(gtx.Ops, e.Th.R.Stone3)
	if thumbnail != nil && *thumbnail != "" && e.Art != nil {
		imageOp, ok := e.Art.Get(*thumbnail, max(size.X, size.Y))
		if ok {
			widget.Image{Src: imageOp, Fit: widget.Cover, Position: layout.Center}.Layout(gtx)
		} else {
			m.layoutArtPlaceholder(gtx, e)
		}
	} else {
		m.layoutArtPlaceholder(gtx, e)
	}
	system.ActionInputOp(system.ActionMove).Add(gtx.Ops)
	mask.Pop()
	return layout.Dimensions{Size: size}
}

func (m *Mini) layoutArtPlaceholder(gtx layout.Context, e *env.Env) layout.Dimensions {
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		size := min(gtx.Constraints.Max.X, gtx.Constraints.Max.Y) / 4
		if size <= 0 {
			return layout.Dimensions{}
		}
		gtx.Constraints = layout.Exact(image.Pt(size, size))
		return lithic.Icon("album").Layout(gtx, e.Th.R.Fg3)
	})
}

func (m *Mini) layoutSeek(gtx layout.Context, e *env.Env) layout.Dimensions {
	state := e.Store.Snapshot()
	duration := state.Playback.Duration
	position := e.Store.PositionAt(gtx.Now)
	wasDragging := m.seek.Dragging()
	if !wasDragging && !m.seekWasDragging {
		if duration > 0 {
			m.seek.Value = float32(min(1, max(0, position/duration)))
		} else {
			m.seek.Value = 0
		}
	}
	before := m.seek.Value
	dims := m.seek.Layout(gtx, e.Th, 0, float32(duration))
	dragging := m.seek.Dragging()
	changedClick := !wasDragging && !dragging && math.Abs(float64(m.seek.Value-before)) > .0001
	released := m.seekWasDragging && !dragging
	if duration > 0 && (changedClick || released) {
		miniCall(e, protocol.MethodSeek, protocol.SeekParams{Position: float64(m.seek.Value) * duration})
	}
	m.seekWasDragging = dragging
	return dims
}

func (m *Mini) layoutTransport(gtx layout.Context, e *env.Env) layout.Dimensions {
	paused := e.Store.Snapshot().Playback.Paused
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			m.previous.Small = true
			dims := m.previous.Layout(gtx, e.Th, lithic.Quiet, lithic.Icon("skip_previous"), "Previous")
			if m.previous.Clicked(gtx) {
				miniCall(e, protocol.MethodPrevTrack, protocol.Null{})
			}
			return dims
		}),
		layout.Rigid(layout.Spacer{Width: e.Th.Sp.S2}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			icon, description := "pause", "Pause"
			if paused {
				icon, description = "play", "Play"
			}
			dims := m.playPause.Layout(gtx, e.Th, lithic.Gold, lithic.Icon(icon), description)
			if m.playPause.Clicked(gtx) {
				miniCall(e, protocol.MethodTogglePause, protocol.Null{})
			}
			return dims
		}),
		layout.Rigid(layout.Spacer{Width: e.Th.Sp.S2}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			m.next.Small = true
			dims := m.next.Layout(gtx, e.Th, lithic.Quiet, lithic.Icon("skip_next"), "Next")
			if m.next.Clicked(gtx) {
				miniCall(e, protocol.MethodNextTrack, protocol.Null{})
			}
			return dims
		}),
	)
}
func miniCall(e *env.Env, method string, params any) {
	callVoid(e, method, params)
}
