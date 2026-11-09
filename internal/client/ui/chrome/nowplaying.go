package chrome

import (
	"image"
	"strings"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/model"
)

// NowPlaying renders the artwork-first playback view and its lyrics or queue.
type NowPlaying struct {
	art      look.ArtView
	tabs     kit.Tabs
	lyrics   Lyrics
	queue    Queue
	like     kit.IconPlaque
	likeFill look.Fill
	playlist kit.IconPlaque
	download kit.IconPlaque
	radio    kit.IconPlaque
	more     kit.IconPlaque
	artist   kit.Click
	album    kit.Click
	stageTab int
}

// Layout draws the complete source-field pane.
func (playing *NowPlaying) Layout(gtx layout.Context, e *env.Env) layout.Dimensions {
	cuts := [4]int{gtx.Dp(e.Th.Cut.Large), gtx.Dp(e.Th.Cut.Large), 0, 0}
	return look.Field(gtx, e.Th, cuts, func(gtx layout.Context) layout.Dimensions {
		snapshot := e.Store.Snapshot()
		if snapshot.Playback.Now == nil {
			empty := kit.Empty{Icon: look.Icon.NowPlaying, Title: "Nothing playing", Body: "Choose a track to see it here."}
			return empty.Layout(gtx, e.Th.Theme)
		}
		track := trackFromPlayback(snapshot.Playback.Now, e.Store.CurrentTrack())
		if gtx.Constraints.Max.X >= gtx.Dp(900) {
			return playing.layoutWide(gtx, e, track, snapshot.Rating)
		}
		return playing.layoutNarrow(gtx, e, track, snapshot.Rating)
	})
}

func (playing *NowPlaying) layoutWide(gtx layout.Context, e *env.Env, track model.Track, rating model.Rating) layout.Dimensions {
	width, height := gtx.Constraints.Max.X, gtx.Constraints.Max.Y
	leftWidth := int(float32(width) * .42)
	rightWidth := max(0, width-leftWidth)
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = leftWidth
			gtx.Constraints.Max.X = leftWidth
			return playing.layoutDetails(gtx, e, track, rating, leftWidth, height, true)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = rightWidth
			gtx.Constraints.Max.X = rightWidth
			return layout.Inset{Top: 21, Right: 21, Bottom: 21, Left: 13}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						playing.tabs.Labels = []string{"Lyrics", "Queue"}
						playing.tabs.Selected = playing.stageTab
						dims := playing.tabs.Layout(gtx, e.Th.Theme)
						if selected, changed := playing.tabs.Changed(); changed {
							playing.stageTab = selected
						}
						return dims
					}),
					layout.Rigid(layout.Spacer{Height: 13}.Layout),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						if playing.stageTab == 1 {
							return playing.queue.LayoutList(gtx, e)
						}
						return playing.lyrics.Layout(gtx, e, LyricsStage)
					}),
				)
			})
		}),
	)
}

func (playing *NowPlaying) layoutNarrow(gtx layout.Context, e *env.Env, track model.Track, rating model.Rating) layout.Dimensions {
	width, height := gtx.Constraints.Max.X, gtx.Constraints.Max.Y
	header := func(gtx layout.Context) layout.Dimensions {
		if height < gtx.Dp(720) {
			return playing.layoutNarrowHeader(gtx, e, track, rating)
		}
		return playing.layoutDetails(gtx, e, track, rating, width, height, false)
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(header),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Right: 13, Left: 13}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				playing.tabs.Labels = []string{"Lyrics", "Queue"}
				playing.tabs.Selected = playing.stageTab
				dims := playing.tabs.Layout(gtx, e.Th.Theme)
				if selected, changed := playing.tabs.Changed(); changed {
					playing.stageTab = selected
				}
				return dims
			})
		}),
		layout.Rigid(layout.Spacer{Height: 8}.Layout),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Right: 13, Bottom: 13, Left: 13}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				if playing.stageTab == 1 {
					return playing.queue.LayoutList(gtx, e)
				}
				return playing.lyrics.Layout(gtx, e, LyricsStage)
			})
		}),
	)
}

func (playing *NowPlaying) layoutNarrowHeader(gtx layout.Context, e *env.Env, track model.Track, rating model.Rating) layout.Dimensions {
	return layout.Inset{Top: 13, Right: 21, Bottom: 13, Left: 21}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				size := image.Pt(gtx.Dp(120), gtx.Dp(120))
				return playing.art.Layout(gtx, e.Th, e.Art, optionalString(track.Thumbnail), size, look.Shape{Cut: 7}, e.Th.Motion.Field)
			}),
			layout.Rigid(layout.Spacer{Width: 21}.Layout),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical, Alignment: layout.Start}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return (kit.Label{Style: e.Th.T.Section, Color: e.Th.R.Fg, MaxLines: 2}).Layout(gtx, e.Th.Theme, nowPlayingTitle(track))
					}),
					layout.Rigid(layout.Spacer{Height: 2}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						artists := strings.TrimSpace(track.Artists)
						if artists == "" {
							return layout.Dimensions{}
						}
						dims := playing.artist.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return (kit.Label{Style: e.Th.T.Proof, Color: e.Th.R.Fg2, MaxLines: 2}).Layout(gtx, e.Th.Theme, artists)
						})
						if playing.artist.Clicked(gtx) {
							if id, name := firstNowPlayingArtist(track); id != "" {
								act.OpenArtist(e, id, name)
							}
						}
						return dims
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						album := strings.TrimSpace(optionalString(track.Album))
						if album == "" {
							return layout.Dimensions{}
						}
						return layout.Inset{Top: 2}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							dims := playing.album.Layout(gtx, kit.Text(e.Th.Theme, e.Th.T.Caption, e.Th.R.Fg3, album))
							if playing.album.Clicked(gtx) && track.AlbumID != nil && strings.TrimSpace(*track.AlbumID) != "" {
								act.OpenAlbum(e, strings.TrimSpace(*track.AlbumID), album)
							}
							return dims
						})
					}),
					layout.Rigid(layout.Spacer{Height: 8}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return playing.layoutActs(gtx, e, track, rating)
					}),
				)
			}),
		)
	})
}

func (playing *NowPlaying) layoutDetails(gtx layout.Context, e *env.Env, track model.Track, rating model.Rating, columnWidth, paneHeight int, wide bool) layout.Dimensions {
	artSide := min(columnWidth-gtx.Dp(110), paneHeight-gtx.Dp(208), gtx.Dp(520))
	if !wide {
		artSide = min(columnWidth-gtx.Dp(42), gtx.Dp(280), paneHeight-gtx.Dp(440))
	}
	artSide = max(gtx.Dp(120), artSide)
	edge, detailGap := unit.Dp(21), unit.Dp(21)
	if !wide {
		edge, detailGap = 13, 13
	}
	return layout.Inset{Top: edge, Right: edge, Bottom: edge, Left: edge}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				size := image.Pt(artSide, artSide)
				return playing.art.Layout(gtx, e.Th, e.Art, optionalString(track.Thumbnail), size, look.Shape{Cut: 16}, e.Th.Motion.Field)
			}),
			layout.Rigid(layout.Spacer{Height: detailGap}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.X = min(gtx.Constraints.Max.X, artSide)
				gtx.Constraints.Max.X = gtx.Constraints.Min.X
				return (kit.Label{Style: e.Th.T.Sentence, Color: e.Th.R.Fg, MaxLines: 2, Align: text.Middle}).Layout(gtx, e.Th.Theme, nowPlayingTitle(track))
			}),
			layout.Rigid(layout.Spacer{Height: 4}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				artists := strings.TrimSpace(track.Artists)
				if artists == "" {
					return layout.Dimensions{}
				}
				dims := playing.artist.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return (kit.Label{Style: e.Th.T.Proof, Color: e.Th.R.Fg2, MaxLines: 2, Align: text.Middle}).Layout(gtx, e.Th.Theme, artists)
				})
				if playing.artist.Clicked(gtx) {
					if id, name := firstNowPlayingArtist(track); id != "" {
						act.OpenArtist(e, id, name)
					}
				}
				return dims
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				album := strings.TrimSpace(optionalString(track.Album))
				if album == "" {
					return layout.Dimensions{}
				}
				return layout.Inset{Top: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						dims := playing.album.Layout(gtx, kit.Text(e.Th.Theme, e.Th.T.Caption, e.Th.R.Fg3, album))
						if playing.album.Clicked(gtx) && track.AlbumID != nil && strings.TrimSpace(*track.AlbumID) != "" {
							act.OpenAlbum(e, strings.TrimSpace(*track.AlbumID), album)
						}
						return dims
					})
				})
			}),
			layout.Rigid(layout.Spacer{Height: detailGap}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return playing.layoutActs(gtx, e, track, rating)
			}),
		)
	})
}

func (playing *NowPlaying) layoutActs(gtx layout.Context, e *env.Env, track model.Track, rating model.Rating) layout.Dimensions {
	provider := model.ProviderOf(track.VideoID)
	canRate := provider != model.ProviderLocal && provider != model.ProviderRadio
	canDownload := canRate
	canRadio := provider != model.ProviderLocal && provider != model.ProviderRadio

	playing.like.Kind = kit.PlaqueQuiet
	playing.like.Icon = nil
	playing.like.Description = "Like"
	playing.like.Small = false
	playing.playlist.Kind = kit.PlaqueQuiet
	playing.playlist.Icon = look.Icon.PlaylistAdd
	playing.playlist.Description = "Add to playlist"
	playing.playlist.Small = false
	playing.download.Kind = kit.PlaqueQuiet
	playing.download.Icon = look.Icon.Download
	playing.download.Description = "Download"
	playing.download.Small = false
	playing.radio.Kind = kit.PlaqueQuiet
	playing.radio.Icon = look.Icon.Radio
	playing.radio.Description = "Start radio"
	playing.radio.Small = false
	playing.more.Kind = kit.PlaqueQuiet
	playing.more.Icon = look.Icon.More
	playing.more.Description = "More actions"
	playing.more.Small = false

	children := make([]layout.FlexChild, 0, 9)
	appendAction := func(widget layout.Widget) {
		if len(children) > 0 {
			children = append(children, layout.Rigid(layout.Spacer{Width: 8}.Layout))
		}
		children = append(children, layout.Rigid(widget))
	}
	if canRate {
		appendAction(func(gtx layout.Context) layout.Dimensions {
			dims := playing.like.Layout(gtx, e.Th.Theme)
			iconSize := gtx.Dp(20)
			offset := op.Offset(image.Pt((dims.Size.X-iconSize)/2, (dims.Size.Y-iconSize)/2)).Push(gtx.Ops)
			iconGTX := gtx
			iconGTX.Constraints = layout.Exact(image.Pt(iconSize, iconSize))
			liked := rating == model.RatingLike
			playing.likeFill.Layout(iconGTX, e.Th, liked,
				func(gtx layout.Context) layout.Dimensions { return look.Icon.HeartOutline.Layout(gtx, e.Th.R.Fg2) },
				func(gtx layout.Context) layout.Dimensions { return look.Icon.Heart.Layout(gtx, e.Th.Src.Ink) },
			)
			offset.Pop()
			if playing.like.Clicked(gtx) {
				next := model.RatingLike
				if liked {
					next = model.RatingIndifferent
				}
				act.Rate(e, track, next)
			}
			return dims
		})
	}
	appendAction(func(gtx layout.Context) layout.Dimensions {
		dims := playing.playlist.Layout(gtx, e.Th.Theme)
		if playing.playlist.Clicked(gtx) && e.Menu != nil {
			copy := track
			e.Menu(env.MenuTarget{Track: &copy, Context: "add-to-playlist"})
		}
		return dims
	})
	if canDownload {
		appendAction(func(gtx layout.Context) layout.Dimensions {
			dims := playing.download.Layout(gtx, e.Th.Theme)
			if playing.download.Clicked(gtx) {
				act.Download(e, track)
			}
			return dims
		})
	}
	if canRadio {
		appendAction(func(gtx layout.Context) layout.Dimensions {
			dims := playing.radio.Layout(gtx, e.Th.Theme)
			if playing.radio.Clicked(gtx) {
				act.StartRadio(e, "track", track.VideoID, track.Title)
			}
			return dims
		})
	}
	appendAction(func(gtx layout.Context) layout.Dimensions {
		dims := playing.more.Layout(gtx, e.Th.Theme)
		if playing.more.Clicked(gtx) && e.Menu != nil {
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
