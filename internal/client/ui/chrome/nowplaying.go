package chrome

import (
	"image"
	"strings"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/anim"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/tokens"
	"github.com/ryoku-dev/ryotunes/internal/client/state"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/sheet"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/widgets"
	"github.com/ryoku-dev/ryotunes/internal/model"
)

// The now playing sheet's parts, placed as the frame places a page's
// (the Hub's sheet): the masthead 28 in and 4 down, 34 tall; the colophon at
// the foot, 26 tall and 6 off the edge, in a pane at least 600 tall.
const (
	nowMastheadX    unit.Dp = 28
	nowMastheadY    unit.Dp = 4
	nowMastheadTall unit.Dp = 34
	nowColophonTall unit.Dp = 26
	nowColophonFoot unit.Dp = 6
	nowColophonPane unit.Dp = 600
)

// nowAct is one of now playing's quiet action plaques.
type nowAct struct {
	plaque kit.IconPlaque
	glyph  widgets.PressGlyph
	tip    look.Tip
}

// Layout draws the plaque with icon and its tooltip.
func (a *nowAct) Layout(gtx layout.Context, e *env.Env, icon *widget.Icon, description string) layout.Dimensions {
	a.plaque.Kind, a.plaque.Description, a.plaque.Small = kit.PlaqueQuiet, description, false
	return a.tip.Layout(gtx, e.Th, e.Overlays, description, func(gtx layout.Context) layout.Dimensions {
		return a.glyph.Layout(gtx, e.Th, &a.plaque, icon)
	})
}

// NowPlaying renders the artwork-first playback view and its lyrics or queue
// as a sheet of the Hub: its masthead (§ 07 NOW PLAYING 再生 and the
// provider) at the head and its colophon at the foot, over the blurred
// cover (owner ruling: clearly visible), with the visualizer's sharp bars
// along the bottom.
type NowPlaying struct {
	art        widgets.PlayArt
	visualizer look.Visualizer
	masthead   sheet.Masthead
	colophon   sheet.Colophon
	tabs       kit.Tabs
	stage      look.Swap
	lyrics     Lyrics
	queue      Queue
	upNext     upNext
	like       likePlaque
	likeTip    look.Tip
	playlist   nowAct
	download   nowAct
	radio      nowAct
	more       nowAct
	artist     textLink
	album      textLink
	stageTab   int
	backdrop   look.Backdrop
}

// nowStageKeys name the stage's two bodies for the swap between them.
var nowStageKeys = [2]string{"lyrics", "queue"}

// Layout draws the complete source-field pane.
func (playing *NowPlaying) Layout(gtx layout.Context, e *env.Env) layout.Dimensions {
	cuts := [4]int{gtx.Dp(e.Th.Cut.Large), gtx.Dp(e.Th.Cut.Large), 0, 0}
	snapshot := e.Store.Snapshot()
	var cover string
	provider := model.Provider("")
	if snapshot.Playback.Now != nil {
		track := trackFromPlayback(snapshot.Playback.Now, e.Store.CurrentTrack())
		cover = optionalString(track.Thumbnail)
		provider = model.ProviderOf(track.VideoID)
	}
	// the field stands a stroke inside the pane's side hairlines where the
	// design draws them, so the blurred cover never covers them
	return widgets.InsidePaneStroke(gtx, e.Th, func(gtx layout.Context) layout.Dimensions {
		return playing.layoutField(gtx, e, cuts, cover, provider, snapshot)
	})
}

// layoutField draws the cover's field and the sheet on it.
func (playing *NowPlaying) layoutField(gtx layout.Context, e *env.Env, cuts [4]int, cover string, provider model.Provider, snapshot state.State) layout.Dimensions {
	return look.FieldBackdrop(gtx, e.Th, e.Th.Accent, cuts, &playing.backdrop, e.Art, cover, func(gtx layout.Context) layout.Dimensions {
		// a short pane gives the lyrics the colophon's room
		colophon := gtx.Constraints.Max.Y >= gtx.Dp(nowColophonPane)
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return playing.layoutMasthead(gtx, e, provider)
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				if snapshot.Playback.Now == nil {
					return widgets.EmptyState(e, look.Icon.NowPlaying, "Nothing playing", "Choose a track to see it here.")(gtx)
				}
				return playing.layoutBody(gtx, e, snapshot.Rating)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if !colophon {
					return layout.Dimensions{}
				}
				height := gtx.Dp(nowColophonTall)
				gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
				dims := layout.Inset{Left: nowMastheadX, Right: nowMastheadX}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return playing.colophon.Layout(gtx, e.Th, sheet.NowPlaying.ID)
				})
				dims.Size.Y += gtx.Dp(nowColophonFoot)
				return dims
			}),
		)
	})
}

// layoutMasthead draws the sheet's running head: "§ 07  NOW PLAYING  再生"
// and, at the right, the sheet's place, the provider and the clock.
func (playing *NowPlaying) layoutMasthead(gtx layout.Context, e *env.Env, provider model.Provider) layout.Dimensions {
	page := sheet.Page{Place: sheet.NowPlaying}
	if provider != "" {
		page.Provider = look.ProviderName(provider)
	}
	top, tall := gtx.Dp(nowMastheadY), gtx.Dp(nowMastheadTall)
	size := image.Pt(gtx.Constraints.Max.X, top+tall)
	child := gtx
	child.Constraints = layout.Exact(image.Pt(max(0, size.X-gtx.Dp(nowMastheadX)), tall))
	offset := op.Offset(image.Pt(gtx.Dp(nowMastheadX), top)).Push(gtx.Ops)
	playing.masthead.Layout(child, e.Th, page, -1)
	offset.Pop()
	return layout.Dimensions{Size: size}
}

func (playing *NowPlaying) layoutBody(gtx layout.Context, e *env.Env, rating model.Rating) layout.Dimensions {
	track := trackFromPlayback(e.Store.Snapshot().Playback.Now, e.Store.CurrentTrack())
	wide := gtx.Constraints.Max.X >= gtx.Dp(900)
	band, inset := int(float32(gtx.Constraints.Max.Y)*.14+.5), unit.Dp(21)
	band = min(max(band, gtx.Dp(56)), gtx.Dp(96))
	if wide {
		band = int(float32(gtx.Constraints.Max.Y)*.2 + .5)
		band = min(max(band, gtx.Dp(88)), gtx.Dp(144))
		inset = nowMastheadX
	}
	alpha := float32(.75)
	if e.Th.R.Dark {
		alpha = .85
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if wide {
				return playing.layoutWide(gtx, e, track, rating)
			}
			return playing.layoutNarrow(gtx, e, track, rating)
		}),
		layout.Rigid(layout.Spacer{Height: 21}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = band, band
			return layout.Inset{Left: inset, Right: inset}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return widgets.Visualizer(gtx, e, &playing.visualizer, look.VisualizerStyle{
					Ink: palette.WithAlpha(e.Th.Accent.Ink, alpha),
					Bar: 9, Gap: 3, Floor: 2,
				})
			})
		}),
		layout.Rigid(layout.Spacer{Height: 8}.Layout),
	)
}

func (playing *NowPlaying) layoutWide(gtx layout.Context, e *env.Env, track model.Track, rating model.Rating) layout.Dimensions {
	width, height := gtx.Constraints.Max.X, gtx.Constraints.Max.Y
	leftWidth := int(float32(width) * .42)
	artSide := nowPlayingWideArtSide(gtx, e, track, leftWidth, height)
	if artSide < gtx.Dp(120) {
		return playing.layoutNarrow(gtx, e, track, rating)
	}
	rightWidth := max(0, width-leftWidth)
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = leftWidth
			gtx.Constraints.Max.X = leftWidth
			return playing.layoutDetails(gtx, e, track, rating, leftWidth, height, true, artSide)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = rightWidth
			gtx.Constraints.Max.X = rightWidth
			column := layout.Inset{Right: nowMastheadX, Left: 13}
			return layout.Inset{Top: 13}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return column.Layout(gtx, playing.layoutTabs(e))
					}),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return playing.layoutStage(gtx, e, layout.Inset{Top: 13, Right: column.Right, Left: column.Left})
					}),
				)
			})
		}),
	)
}

// layoutTabs draws the stage's Lyrics and Queue tabs.
func (playing *NowPlaying) layoutTabs(e *env.Env) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		playing.tabs.Labels = []string{"Lyrics", "Queue"}
		playing.tabs.Selected = playing.stageTab
		dims := playing.tabs.Layout(gtx, e.Th.Theme)
		if selected, changed := playing.tabs.Changed(); changed {
			playing.stageTab = selected
		}
		return dims
	}
}

func (playing *NowPlaying) layoutNarrow(gtx layout.Context, e *env.Env, track model.Track, rating model.Rating) layout.Dimensions {
	width, height := gtx.Constraints.Max.X, gtx.Constraints.Max.Y
	header := func(gtx layout.Context) layout.Dimensions {
		// 720 of the old pane, less the masthead and colophon above and below
		if height < gtx.Dp(650) {
			return playing.layoutNarrowHeader(gtx, e, track, rating)
		}
		return playing.layoutDetails(gtx, e, track, rating, width, height, false, 0)
	}
	headAndTabs := func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(header),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Right: 13, Left: 13}.Layout(gtx, playing.layoutTabs(e))
			}),
		)
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if height >= gtx.Dp(650) || width < gtx.Dp(nowUpNextFrom) {
				return headAndTabs(gtx)
			}
			return playing.layoutHeadWithUpNext(gtx, e, track, headAndTabs)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return playing.layoutStage(gtx, e, layout.Inset{Top: 8, Right: 13, Left: 13})
		}),
	)
}

// Up next beside the short head: shown from a pane this wide, taking this
// share of it within these bounds, set this far from the head.
const (
	nowUpNextFrom  unit.Dp = 720
	nowUpNextShare         = .4
	nowUpNextMin   unit.Dp = 280
	nowUpNextMax   unit.Dp = 420
	nowUpNextGap   unit.Dp = 21
)

// layoutHeadWithUpNext lays the head and its tabs out in the left of the
// width and the up next block in the right, as tall as they are (from the
// cover's top to the tabs' foot), so the block fills what the head leaves
// empty and the lyric stage under them keeps its height.
func (playing *NowPlaying) layoutHeadWithUpNext(gtx layout.Context, e *env.Env, track model.Track, headAndTabs layout.Widget) layout.Dimensions {
	width := gtx.Constraints.Max.X
	block := int(float32(width) * nowUpNextShare)
	block = min(max(block, gtx.Dp(nowUpNextMin)), gtx.Dp(nowUpNextMax))
	right, top := gtx.Dp(nowMastheadX), gtx.Dp(8)
	left := width - block - gtx.Dp(nowUpNextGap) - right
	head := gtx
	head.Constraints = layout.Constraints{Max: image.Pt(max(0, left), gtx.Constraints.Max.Y)}
	dims := headAndTabs(head)
	if tall := dims.Size.Y - top; tall > 0 {
		offset := op.Offset(image.Pt(width-right-block, top)).Push(gtx.Ops)
		child := gtx
		child.Constraints = layout.Exact(image.Pt(block, tall))
		playing.upNext.Layout(child, e, track)
		offset.Pop()
	}
	return layout.Dimensions{Size: image.Pt(width, dims.Size.Y)}
}

// layoutStage draws the selected stage tab inside inset, or its empty state,
// which owns its spacing, in the whole area under the tabs; a change of tab
// crossfades with the Hub's short rise on OutCubic.
func (playing *NowPlaying) layoutStage(gtx layout.Context, e *env.Env, inset layout.Inset) layout.Dimensions {
	playing.stage.Ease = anim.EaseOutCubic
	return playing.stage.Layout(gtx, e.Th, nowStageKeys[min(max(playing.stageTab, 0), 1)], 8, e.Th.Motion.Move, func(gtx layout.Context, key string) layout.Dimensions {
		if key == nowStageKeys[1] {
			return playing.queue.LayoutList(gtx, e, inset)
		}
		return playing.lyrics.Layout(gtx, e, LyricsStage, inset)
	})
}

func (playing *NowPlaying) layoutNarrowHeader(gtx layout.Context, e *env.Env, track model.Track, rating model.Rating) layout.Dimensions {
	return layout.Inset{Top: 8, Right: 21, Bottom: 13, Left: nowMastheadX}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				size := image.Pt(gtx.Dp(120), gtx.Dp(120))
				return playing.art.Layout(gtx, e, optionalString(track.Thumbnail), size, look.Shape{Cut: 7}, e.Th.Motion.Field)
			}),
			layout.Rigid(layout.Spacer{Width: 21}.Layout),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical, Alignment: layout.Start}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return (kit.Label{Style: e.Th.T.Section, Color: e.Th.R.Fg, MaxLines: 2}).Layout(gtx, e.Th.Theme, nowPlayingTitle(track))
					}),
					layout.Rigid(layout.Spacer{Height: 2}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return playing.layoutArtist(gtx, e, track, text.Start)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return playing.layoutAlbum(gtx, e, track)
					}),
					layout.Rigid(layout.Spacer{Height: 8}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = gtx.Dp(40), gtx.Dp(40)
						// the acts stand under the words: the first glyph's edge on the
						// title's, its quiet plaque reaching into the gap before them
						return layout.W.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							lead := (gtx.Dp(unit.Dp(tokens.SizeIconPlaque)) - gtx.Dp(unit.Dp(tokens.SizeIconPlaqueIcon))) / 2
							defer op.Offset(image.Pt(-lead, 0)).Push(gtx.Ops).Pop()
							return playing.layoutActs(gtx, e, track, rating)
						})
					}),
				)
			}),
		)
	})
}

// layoutArtist draws the artists as a link to the first of them.
func (playing *NowPlaying) layoutArtist(gtx layout.Context, e *env.Env, track model.Track, align text.Alignment) layout.Dimensions {
	artists := strings.TrimSpace(track.Artists)
	if artists == "" {
		return layout.Dimensions{}
	}
	dims := playing.artist.Layout(gtx, e.Th, func(gtx layout.Context) layout.Dimensions {
		return (kit.Label{Style: e.Th.T.Proof, Color: e.Th.R.Fg2, MaxLines: 2, Align: align}).Layout(gtx, e.Th.Theme, artists)
	})
	if playing.artist.Clicked(gtx) {
		if id, name := firstNowPlayingArtist(track); id != "" {
			act.OpenArtist(e, id, name)
		}
	}
	return dims
}

// layoutAlbum draws the album as a link to it, 4 under the artists.
func (playing *NowPlaying) layoutAlbum(gtx layout.Context, e *env.Env, track model.Track) layout.Dimensions {
	album := strings.TrimSpace(optionalString(track.Album))
	if album == "" {
		return layout.Dimensions{}
	}
	return layout.Inset{Top: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		dims := playing.album.Layout(gtx, e.Th, kit.Text(e.Th.Theme, e.Th.T.Caption, e.Th.R.Fg3, album))
		if playing.album.Clicked(gtx) && track.AlbumID != nil && strings.TrimSpace(*track.AlbumID) != "" {
			act.OpenAlbum(e, strings.TrimSpace(*track.AlbumID), album)
		}
		return dims
	})
}

func (playing *NowPlaying) layoutDetails(gtx layout.Context, e *env.Env, track model.Track, rating model.Rating, columnWidth, paneHeight int, wide bool, artSide int) layout.Dimensions {
	if !wide {
		artSide = min(columnWidth-gtx.Dp(42), gtx.Dp(280), paneHeight-gtx.Dp(440))
		artSide = max(gtx.Dp(120), artSide)
	}
	edge, detailGap := unit.Dp(21), unit.Dp(21)
	if !wide {
		edge, detailGap = 13, 13
	}
	return layout.Inset{Top: edge, Right: edge, Left: nowMastheadX}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				size := image.Pt(artSide, artSide)
				return playing.art.Layout(gtx, e, optionalString(track.Thumbnail), size, look.Shape{Cut: 16}, e.Th.Motion.Field)
			}),
			layout.Rigid(layout.Spacer{Height: detailGap}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				// the title takes the column, not the cover's width, so a short
				// pane's smaller cover does not cut a title short
				gtx.Constraints.Min.X = gtx.Constraints.Max.X
				return (kit.Label{Style: e.Th.T.Sentence, Color: e.Th.R.Fg, MaxLines: 2, Align: text.Middle}).Layout(gtx, e.Th.Theme, nowPlayingTitle(track))
			}),
			layout.Rigid(layout.Spacer{Height: 4}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return playing.layoutArtist(gtx, e, track, text.Middle)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return playing.layoutAlbum(gtx, e, track)
				})
			}),
			layout.Rigid(layout.Spacer{Height: detailGap}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = gtx.Dp(40), gtx.Dp(40)
				return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return playing.layoutActs(gtx, e, track, rating)
				})
			}),
		)
	})
}

func nowPlayingWideArtSide(gtx layout.Context, e *env.Env, track model.Track, columnWidth, paneHeight int) int {
	maxSide := min(columnWidth-gtx.Dp(110), gtx.Dp(520))
	if maxSide <= 0 {
		return maxSide
	}
	// the words take the column inside its insets (layoutDetails)
	textWidth := max(0, columnWidth-gtx.Dp(nowMastheadX)-gtx.Dp(21))
	textHeight := nowPlayingTextHeight(gtx, e, track, textWidth)
	return min(maxSide, paneHeight-gtx.Dp(21+21+21+40)-textHeight)
}

func nowPlayingTextHeight(gtx layout.Context, e *env.Env, track model.Track, width int) int {
	measure := gtx
	measure.Constraints.Min = image.Point{}
	measure.Constraints.Max = image.Pt(max(0, width), 1<<20)
	record := op.Record(gtx.Ops)
	dims := layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(measure,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = min(gtx.Constraints.Max.X, width)
			gtx.Constraints.Max.X = gtx.Constraints.Min.X
			return (kit.Label{Style: e.Th.T.Sentence, Color: e.Th.R.Fg, MaxLines: 2, Align: text.Middle}).Layout(gtx, e.Th.Theme, nowPlayingTitle(track))
		}),
		layout.Rigid(layout.Spacer{Height: 4}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			artists := strings.TrimSpace(track.Artists)
			if artists == "" {
				return layout.Dimensions{}
			}
			return (kit.Label{Style: e.Th.T.Proof, Color: e.Th.R.Fg2, MaxLines: 2, Align: text.Middle}).Layout(gtx, e.Th.Theme, artists)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			album := strings.TrimSpace(optionalString(track.Album))
			if album == "" {
				return layout.Dimensions{}
			}
			return layout.Inset{Top: 4}.Layout(gtx, kit.Text(e.Th.Theme, e.Th.T.Caption, e.Th.R.Fg3, album))
		}),
	)
	record.Stop()
	return dims.Size.Y
}

func (playing *NowPlaying) layoutActs(gtx layout.Context, e *env.Env, track model.Track, rating model.Rating) layout.Dimensions {
	provider := model.ProviderOf(track.VideoID)
	canRate := provider != model.ProviderLocal && provider != model.ProviderRadio
	canDownload := canRate
	canRadio := provider != model.ProviderLocal && provider != model.ProviderRadio

	children := make([]layout.FlexChild, 0, 9)
	appendAction := func(widget layout.Widget) {
		if len(children) > 0 {
			children = append(children, layout.Rigid(layout.Spacer{Width: 8}.Layout))
		}
		children = append(children, layout.Rigid(widget))
	}
	if canRate {
		appendAction(func(gtx layout.Context) layout.Dimensions {
			liked := rating == model.RatingLike
			tip := "Like"
			if liked {
				tip = "Remove like"
			}
			dims := playing.likeTip.Layout(gtx, e.Th, e.Overlays, tip, func(gtx layout.Context) layout.Dimensions {
				return playing.like.Layout(gtx, e.Th, liked, false)
			})
			if playing.like.Clicked(gtx) {
				widgets.ToggleLike(e, track, liked)
			}
			return dims
		})
	}
	appendAction(func(gtx layout.Context) layout.Dimensions {
		dims := playing.playlist.Layout(gtx, e, look.Icon.PlaylistAdd, "Add to playlist")
		if playing.playlist.plaque.Clicked(gtx) && e.Menu != nil {
			copy := track
			e.Menu(env.MenuTarget{Track: &copy, Context: "add-to-playlist"})
		}
		return dims
	})
	if canDownload {
		appendAction(func(gtx layout.Context) layout.Dimensions {
			dims := playing.download.Layout(gtx, e, look.Icon.Download, "Download")
			if playing.download.plaque.Clicked(gtx) {
				act.Download(e, track)
			}
			return dims
		})
	}
	if canRadio {
		appendAction(func(gtx layout.Context) layout.Dimensions {
			dims := playing.radio.Layout(gtx, e, look.Icon.Radio, "Start radio")
			if playing.radio.plaque.Clicked(gtx) {
				act.StartRadio(e, "track", track.VideoID, track.Title)
			}
			return dims
		})
	}
	appendAction(func(gtx layout.Context) layout.Dimensions {
		dims := playing.more.Layout(gtx, e, look.Icon.More, "More actions")
		if playing.more.plaque.Clicked(gtx) && e.Menu != nil {
			copy := track
			e.Menu(env.MenuTarget{Track: &copy})
		}
		return dims
	})
	gtx.Constraints.Min.X = 0
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
}

func nowPlayingTitle(track model.Track) string {
	if title := strings.TrimSpace(track.Title); title != "" {
		return title
	}
	return "Untitled track"
}

func firstNowPlayingArtist(track model.Track) (string, string) {
	for _, artist := range track.ArtistRuns {
		if artist.ID != nil && strings.TrimSpace(*artist.ID) != "" {
			return strings.TrimSpace(*artist.ID), strings.TrimSpace(artist.Text)
		}
	}
	if track.ArtistID != nil && strings.TrimSpace(*track.ArtistID) != "" {
		return strings.TrimSpace(*track.ArtistID), strings.TrimSpace(track.Artists)
	}
	return "", ""
}
