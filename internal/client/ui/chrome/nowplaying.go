package chrome

import (
	"gioui.org/layout"
	"gioui.org/widget"

	"github.com/ryoku-dev/ryotunes/internal/client/lithic"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/widgets"
	"github.com/ryoku-dev/ryotunes/internal/model"
)

// NowPlaying renders artwork, metadata, stream facts, and current-track acts.
type NowPlaying struct {
	list     lithic.List
	like     lithic.IconButton
	download lithic.IconButton
	playlist lithic.IconButton
	radio    lithic.IconButton
	artist   lithic.IconButton
	album    lithic.IconButton
}

// Layout renders the current track as an artwork-first detail panel.
func (p *NowPlaying) Layout(gtx layout.Context, e *env.Env) layout.Dimensions {
	snapshot := e.Store.Snapshot()
	now := snapshot.Playback.Now
	if now == nil {
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(lithic.Text(e.Th, e.Th.T.Title, e.Th.R.Fg, "Nothing playing")),
				layout.Rigid(layout.Spacer{Height: 8}.Layout),
				layout.Rigid(lithic.Text(e.Th, e.Th.T.Body, e.Th.R.Fg2, "Choose a track to see it here.")),
			)
		})
	}
	current := e.Store.CurrentTrack()
	track := trackFromPlayback(now, current)
	provider := model.ProviderOf(track.VideoID)

	return p.list.Layout(gtx, e.Th, 1, func(gtx layout.Context, _ int) layout.Dimensions {
		return layout.Inset{Top: 21, Bottom: 34, Left: 21, Right: 21}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			artSide := max(0, min(gtx.Constraints.Max.X, gtx.Dp(420)))
			facts := nowPlayingFacts(track, now, provider)
			return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return topCutArtwork(gtx, e, optionalString(track.Thumbnail), artSide)
				}),
				layout.Rigid(layout.Spacer{Height: 21}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					label := lithic.Label{Style: e.Th.T.Sentence, Color: e.Th.R.Fg, MaxLines: 2}
					return label.Layout(gtx, e.Th, track.Title)
				}),
				layout.Rigid(layout.Spacer{Height: 8}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					label := lithic.Label{Style: e.Th.T.Proof, Color: e.Th.R.Fg2, MaxLines: 2}
					return label.Layout(gtx, e.Th, track.Artists)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if track.Album == nil || *track.Album == "" {
						return layout.Dimensions{}
					}
					return layout.Inset{Top: 4}.Layout(gtx, lithic.Text(e.Th, e.Th.T.Caption, e.Th.R.Fg3, *track.Album))
				}),
				layout.Rigid(layout.Spacer{Height: 13}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Center.Layout(gtx, widgets.ProviderBadge(e, provider))
				}),
				layout.Rigid(layout.Spacer{Height: 21}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layoutActionRows(gtx, e, p, snapshot.Rating, provider, track)
				}),
				layout.Rigid(layout.Spacer{Height: 21}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.X = min(gtx.Constraints.Max.X, gtx.Dp(320))
					return lithic.Facts(e.Th, facts)(gtx)
				}),
			)
		})
	})
}

func layoutActionRows(gtx layout.Context, e *env.Env, p *NowPlaying, rating model.Rating, provider model.Provider, track model.Track) layout.Dimensions {
	if p.like.Clicked(gtx) {
		next := model.RatingLike
		if rating == model.RatingLike {
			next = model.RatingIndifferent
		}
		act.Rate(e, track, next)
	}
	if p.download.Clicked(gtx) {
		act.Download(e, track)
	}
	if p.playlist.Clicked(gtx) && e.Menu != nil {
		copy := track
		e.Menu(env.MenuTarget{Track: &copy})
	}
	if p.radio.Clicked(gtx) {
		act.StartRadio(e, "track", track.VideoID, track.Title)
	}
	if p.artist.Clicked(gtx) {
		if id, name := firstArtist(track); id != "" {
			act.OpenArtist(e, id, name)
		}
	}
	if p.album.Clicked(gtx) && track.AlbumID != nil {
		act.OpenAlbum(e, *track.AlbumID, optionalString(track.Album))
	}

	canRate := provider != model.ProviderLocal && provider != model.ProviderRadio
	canDownload := provider != model.ProviderLocal && provider != model.ProviderRadio
	canArtist := firstArtistID(track) != ""
	canAlbum := track.AlbumID != nil && *track.AlbumID != ""
	likeKind := lithic.Quiet
	likeIcon := lithic.Icon("favorite_border")
	if rating == model.RatingLike {
		likeKind = lithic.On
		likeIcon = lithic.Icon("favorite")
	}

	type action struct {
		button  *lithic.IconButton
		kind    lithic.ButtonKind
		icon    *widget.Icon
		tooltip string
		enabled bool
	}
	actions := []action{
		{button: &p.like, kind: likeKind, icon: likeIcon, tooltip: "Like", enabled: canRate},
		{button: &p.download, kind: lithic.Quiet, icon: lithic.Icon("download"), tooltip: "Download", enabled: canDownload},
		{button: &p.playlist, kind: lithic.Quiet, icon: lithic.Icon("playlist_add"), tooltip: "Add to playlist", enabled: true},
		{button: &p.radio, kind: lithic.Gold, icon: lithic.Icon("radio"), tooltip: "Start radio", enabled: true},
		{button: &p.artist, kind: lithic.Quiet, icon: lithic.Icon("person"), tooltip: "Go to artist", enabled: canArtist},
		{button: &p.album, kind: lithic.Quiet, icon: lithic.Icon("album"), tooltip: "Go to album", enabled: canAlbum},
	}
	children := make([]layout.FlexChild, 0, len(actions))
	for _, item := range actions {
		if !item.enabled {
			continue
		}
		item := item
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			item.button.Small = true
			return lithic.Tooltip(gtx, e.Th, item.tooltip, func(gtx layout.Context) layout.Dimensions {
				return item.button.Layout(gtx, e.Th, item.kind, item.icon, item.tooltip)
			})
		}))
	}
	gtx.Constraints.Min.X = 0
	gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(320))
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceBetween}.Layout(gtx, children...)
}

func nowPlayingFacts(track model.Track, now *model.PlaybackTrack, provider model.Provider) []lithic.Fact {
	return []lithic.Fact{
		{Label: "Plays", Value: optionalString(track.Plays), Mono: true},
		{Label: "Likes", Value: optionalString(track.Likes), Mono: true},
		{Label: "Genre", Value: optionalString(track.Genre), Mono: true},
		{Label: "Provider", Value: providerName(provider)},
		{Label: "Stream client", Value: now.StreamClient, Mono: true},
	}
}

func firstArtist(track model.Track) (string, string) {
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

func firstArtistID(track model.Track) string {
	id, _ := firstArtist(track)
	return id
}
