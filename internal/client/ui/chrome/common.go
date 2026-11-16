package chrome

import (
	"image"
	"math"
	"strings"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/anim"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/tokens"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/widgets"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

func callVoid(e *env.Env, method string, params any) {
	act.Call[protocol.Null](e, method, params, func(_ protocol.Null, err error) {
		if err != nil && e.Toast != nil {
			e.Toast(err.Error(), env.ToastError)
		}
	})
}

func trackFromPlayback(now *model.PlaybackTrack, current *model.Track) model.Track {
	if now == nil {
		return model.Track{}
	}
	if current != nil && current.VideoID == now.VideoID {
		track := *current
		track.Title = now.Title
		track.Artists = now.Artists
		track.ArtistID = now.ArtistID
		track.ArtistRuns = now.ArtistRuns
		track.Thumbnail = now.Thumbnail
		track.Duration = now.Duration
		track.Rating = now.Rating
		track.Plays = now.Plays
		track.Likes = now.Likes
		track.Genre = now.Genre
		return track
	}
	return model.Track{
		VideoID: now.VideoID, Title: now.Title, Artists: now.Artists,
		ArtistID: now.ArtistID, ArtistRuns: now.ArtistRuns,
		Thumbnail: now.Thumbnail, Duration: now.Duration, Rating: now.Rating,
		Plays: now.Plays, Likes: now.Likes, Genre: now.Genre,
	}
}

func optionalString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func currentDownload(snapshot model.DownloadSnapshot, videoID string) *model.DownloadEntry {
	for index := range snapshot.Jobs {
		if snapshot.Jobs[index].VideoID == videoID {
			return &snapshot.Jobs[index]
		}
	}
	return nil
}

func settingEnabled(settings model.SettingsSnapshot, key model.SettingKey, fallback bool) bool {
	value, ok := settings[string(key)]
	if !ok {
		return fallback
	}
	return strings.EqualFold(value, "true")
}

// heartRise is how far the heart grows at the height of its pop.
const heartRise = .24

// heart is the like glyph with its two micro animations: the filled heart
// grows out of its centre (look.Fill, the one round shape the owner kept),
// and a like pops it, the glyph rising by a quarter and settling over
// th.Motion.Field on OutCubic. An unlike only empties it. A heart at rest
// asks for no frame; under reduced motion it neither pops nor fills over
// time.
type heart struct {
	fill  look.Fill
	pop   anim.Tween
	liked bool
	ready bool
}

// scale is the glyph's scale for liked this frame.
func (h *heart) scale(gtx layout.Context, th *look.Theme, liked bool) float32 {
	if !h.ready {
		h.ready, h.liked = true, liked
		h.pop = anim.Tween{Duration: th.Motion.Field, Ease: anim.EaseOutCubic}
		h.pop.Set(gtx, 1)
		return 1
	}
	if liked != h.liked {
		h.liked = liked
		if liked {
			h.pop = anim.Tween{Duration: th.Motion.Field, Ease: anim.EaseOutCubic}
			h.pop.Set(gtx, 0)
			h.pop.Set(gtx, 1)
		}
	}
	progress := h.pop.Value(gtx)
	if progress >= 1 {
		return 1
	}
	return 1 + heartRise*float32(math.Sin(math.Pi*float64(progress)))
}

// Layout draws the heart side pixels square: the outline in fg2, the filled
// heart in the music's accent.
func (h *heart) Layout(gtx layout.Context, th *look.Theme, liked bool, side int) layout.Dimensions {
	scale := h.scale(gtx, th, liked)
	gtx.Constraints = layout.Exact(image.Pt(side, side))
	if scale != 1 {
		centre := f32.Pt(float32(side)/2, float32(side)/2)
		defer op.Affine(f32.AffineId().Scale(centre, f32.Pt(scale, scale))).Push(gtx.Ops).Pop()
	}
	return h.fill.Layout(gtx, th, liked,
		func(gtx layout.Context) layout.Dimensions { return look.Icon.HeartOutline.Layout(gtx, th.R.Fg2) },
		func(gtx layout.Context) layout.Dimensions { return look.Icon.Heart.Layout(gtx, th.Accent.Ink) },
	)
}

// likePlaque is the like control: a quiet icon plaque whose glyph is the
// heart, pressed as the Hub's icon buttons are.
type likePlaque struct {
	plaque kit.IconPlaque
	heart  heart
	press  widgets.PressGlyph
}

// Layout draws the plaque, small or not, with the heart for liked.
func (l *likePlaque) Layout(gtx layout.Context, th *look.Theme, liked, small bool) layout.Dimensions {
	l.plaque.Small, l.plaque.Kind, l.plaque.Icon = small, kit.PlaqueQuiet, nil
	l.plaque.Description = "Like"
	if liked {
		l.plaque.Description = "Liked"
	}
	dimensions := l.plaque.Layout(gtx, th.Theme)
	pressed := l.press.Value(gtx, th, l.plaque.Pressed())
	size := tokens.SizeIconPlaqueIcon
	if small {
		size = tokens.SizeIconPlaqueIconSmall
	}
	side := gtx.Dp(unit.Dp(size))
	widgets.DrawPressedGlyph(gtx, dimensions.Size, pressed, func(gtx layout.Context) layout.Dimensions {
		return l.heart.Layout(gtx, th, liked, side)
	}, side)
	return dimensions
}

// Clicked reports one activation.
func (l *likePlaque) Clicked(gtx layout.Context) bool { return l.plaque.Clicked(gtx) }

// textLink is a text that opens something (the bar's title and artists):
// hovered, it takes the Hub's row wash (fg at .06 dark / .07 light, 120 in
// and 240 out on OutCubic) a little beyond its letters, stronger while
// pressed.
type textLink struct {
	click kit.Click
	wash  look.RowWash
}

// Layout lays w out as the link.
func (l *textLink) Layout(gtx layout.Context, th *look.Theme, w layout.Widget) layout.Dimensions {
	record := op.Record(gtx.Ops)
	dimensions := l.click.Layout(gtx, w)
	call := record.Stop()
	l.wash.Update(gtx, th, l.click.Hovered() && gtx.Source.Enabled(), l.click.Pressed())
	padX, padY := gtx.Dp(4), gtx.Dp(1)
	l.wash.Paint(gtx, th, image.Rect(-padX, -padY, dimensions.Size.X+padX, dimensions.Size.Y+padY), gtx.Dp(th.Cut.Token))
	call.Add(gtx.Ops)
	return dimensions
}

// Clicked reports one activation.
func (l *textLink) Clicked(gtx layout.Context) bool { return l.click.Clicked(gtx) }

// Ruled group heads, from the Hub's tree folders and ruled sections: the
// name in the group face's spaced capitals, tracking 1.4, a vein leader to
// the count in the smallest mono (Pad2), all fg3.
const (
	groupTracking = 1.4
	groupGap      = unit.Dp(8)
	groupHeadTall = unit.Dp(40)
)

// ruledHead lays out one ruled group head across the width given, tall
// high: name, then lead (a sentence-case phrase such as the queue's source,
// fg2), the leader, the mark in the smallest mono (a count "06", a place
// "31 / 67"; none when empty) and trailing (may be nil), all centred.
func ruledHead(gtx layout.Context, th *look.Theme, name, lead, mark string, tall unit.Dp, trailing layout.Widget) layout.Dimensions {
	width := gtx.Constraints.Max.X
	height := gtx.Dp(tall)
	gtx.Constraints = layout.Exact(image.Pt(width, height))
	gap := gtx.Dp(groupGap)

	// the trailing control and the count, laid right to left
	right := width
	if trailing != nil {
		record := op.Record(gtx.Ops)
		child := gtx
		child.Constraints = layout.Constraints{Max: image.Pt(width/2, height)}
		dims := trailing(child)
		call := record.Stop()
		right -= dims.Size.X
		offset := op.Offset(image.Pt(right, (height-dims.Size.Y)/2)).Push(gtx.Ops)
		call.Add(gtx.Ops)
		offset.Pop()
		right -= gap
	}
	measure := gtx
	measure.Constraints = layout.Constraints{Max: image.Pt(width, height)}
	if mark != "" {
		record := op.Record(gtx.Ops)
		dims := kit.LayoutSpacedCaps(measure, th.Theme, th.T.MonoSm, th.R.Fg3, 0, mark)
		call := record.Stop()
		right -= dims.Size.X
		offset := op.Offset(image.Pt(right, (height-dims.Size.Y)/2)).Push(gtx.Ops)
		call.Add(gtx.Ops)
		offset.Pop()
		right -= gap
	}

	// the name and its lead, left to right
	record := op.Record(gtx.Ops)
	nameDims := kit.LayoutSpacedCaps(measure, th.Theme, th.T.Group, th.R.Fg3, groupTracking, name)
	call := record.Stop()
	offset := op.Offset(image.Pt(0, (height-nameDims.Size.Y)/2)).Push(gtx.Ops)
	call.Add(gtx.Ops)
	offset.Pop()
	left := nameDims.Size.X
	if lead != "" && right-left-gap > gtx.Dp(24) {
		leadGTX := gtx
		leadGTX.Constraints = layout.Constraints{Max: image.Pt(right-left-gap, height)}
		record := op.Record(gtx.Ops)
		leadDims := kit.Label{Style: th.T.Caption, Color: th.R.Fg2, MaxLines: 1}.Layout(leadGTX, th.Theme, lead)
		call := record.Stop()
		offset := op.Offset(image.Pt(left+gap, (height-leadDims.Size.Y)/2)).Push(gtx.Ops)
		call.Add(gtx.Ops)
		offset.Pop()
		left += gap + leadDims.Size.X
	}

	// the vein leader between them
	start, end := left+gap, right
	if end-start > gap {
		hair := max(1, gtx.Dp(unit.Dp(tokens.SizeHairline)))
		top := (height - hair) / 2
		paint.FillShape(gtx.Ops, th.R.Vein, clip.Rect(image.Rect(start, top, end, top+hair)).Op())
	}
	return layout.Dimensions{Size: image.Pt(width, height)}
}

// hairlineSlab lays body out on a card of the desktop's planes, as the Hub
// lays its slabs: in Lithic the card is the pane's own tone, drawn by its
// vein hairline alone; Material lifts it a step with no line; Adwaita and
// Fluent fill it, Fluent with a fainter line. The cut is the mid cut.
func hairlineSlab(gtx layout.Context, th *look.Theme, inset unit.Dp, body layout.Widget) layout.Dimensions {
	record := op.Record(gtx.Ops)
	dimensions := layout.UniformInset(inset).Layout(gtx, body)
	call := record.Stop()
	bounds := image.Rectangle{Max: dimensions.Size}
	cut := gtx.Dp(th.Cut.Mid)
	planes := th.Planes()
	if planes.Card != planes.Pane {
		kit.Fill(gtx, th.Theme, bounds, cut, planes.Card)
	}
	if planes.CardStroke.A > 0 {
		kit.Stroke(gtx, th.Theme, bounds, cut, max(1, gtx.Dp(unit.Dp(tokens.SizeHairline))), planes.CardStroke)
	}
	call.Add(gtx.Ops)
	return dimensions
}
