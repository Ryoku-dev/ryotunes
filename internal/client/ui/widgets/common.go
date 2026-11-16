package widgets

import (
	"image"
	"strings"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/model"
)

// RuledHead draws a ruled section head as the Hub's Section: the title in
// section type, a vein hairline 13 after it running to 13 before the mark
// (an ordinal such as kit.Ordinal(2, 6), small mono fg3), then trailing (a
// shelf's Show all and steps) 13 after the mark. It fills the width given;
// mark and trailing are optional.
func RuledHead(gtx layout.Context, e *env.Env, title, mark string, trailing layout.Widget) layout.Dimensions {
	th := e.Th
	gtx.Constraints.Min = image.Point{}
	width := gtx.Constraints.Max.X
	gap := gtx.Dp(th.Sp.S3)
	end := width
	trailingSize := image.Point{}
	var trailingCall op.CallOp
	if trailing != nil {
		record := op.Record(gtx.Ops)
		trailingSize = trailing(gtx).Size
		trailingCall = record.Stop()
		end -= trailingSize.X
	}
	markWidth := 0
	var markCall op.CallOp
	markSize := image.Point{}
	if mark != "" {
		record := op.Record(gtx.Ops)
		markSize = kit.LayoutSpacedCaps(gtx, th.Theme, th.T.MonoSm, th.R.Fg3, 0, mark).Size
		markCall = record.Stop()
		markWidth = markSize.X
		if trailing != nil {
			end -= gap
		}
		end -= markWidth
	}
	titleGTX := gtx
	titleGTX.Constraints.Max.X = max(0, end-gap)
	record := op.Record(gtx.Ops)
	titleDims := kit.Label{Style: th.T.Section, Color: th.R.Fg, MaxLines: 1}.Layout(titleGTX, th.Theme, title)
	titleCall := record.Stop()
	height := max(titleDims.Size.Y, markSize.Y, trailingSize.Y)
	middle := height / 2
	place := func(call op.CallOp, x, h int) {
		offset := op.Offset(image.Pt(x, middle-h/2)).Push(gtx.Ops)
		call.Add(gtx.Ops)
		offset.Pop()
	}
	place(titleCall, 0, titleDims.Size.Y)
	start := titleDims.Size.X + gap
	ruleEnd := end - gap
	if mark == "" && trailing == nil {
		ruleEnd = width
	}
	if title != "" && ruleEnd > start {
		hair := max(1, gtx.Dp(1))
		paint.FillShape(gtx.Ops, th.R.Vein, clip.Rect(image.Rect(start, middle-hair/2, ruleEnd, middle-hair/2+hair)).Op())
	}
	if mark != "" {
		place(markCall, end, markSize.Y)
	}
	if trailing != nil {
		place(trailingCall, width-trailingSize.X, trailingSize.Y)
	}
	return layout.Dimensions{Size: image.Pt(width, height)}
}

func textValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func browseKey(item model.BrowseItem) string {
	if item.ID != "" {
		return strings.ToLower(item.Kind) + "\x00" + item.ID
	}
	return strings.ToLower(item.Kind) + "\x00" + item.Title
}

func browseKind(item model.BrowseItem) string {
	kind := strings.ToLower(strings.TrimSpace(item.Kind))
	switch kind {
	case "song", "track", "video":
		return "Song"
	case "album", "single", "ep":
		return "Album"
	case "artist", "channel", "user":
		return "Artist"
	case "playlist", "mix":
		return "Playlist"
	case "station", "radio":
		return "Station"
	default:
		if kind == "" {
			return ""
		}
		return strings.ToUpper(kind[:1]) + kind[1:]
	}
}

func browseSubtitle(item model.BrowseItem) string {
	if subtitle := textValue(item.Subtitle); subtitle != "" {
		return subtitle
	}
	return browseKind(item)
}

func browseIsArtist(item model.BrowseItem) bool {
	switch strings.ToLower(strings.TrimSpace(item.Kind)) {
	case "artist", "channel", "user":
		return true
	default:
		return false
	}
}

func browseIsTrack(item model.BrowseItem) bool {
	switch strings.ToLower(strings.TrimSpace(item.Kind)) {
	case "song", "track", "video", "station", "radio":
		return true
	default:
		return false
	}
}

// IsLikedCollection reports provider and local aliases for the built-in liked-songs collection.
func IsLikedCollection(item model.BrowseItem, provider model.Provider) bool {
	id := strings.TrimSpace(item.ID)
	switch {
	case id == model.LocalLikedPlaylistID:
		return true
	case strings.EqualFold(id, "LOCAL:liked"), strings.EqualFold(id, "RYOTUNES_LIKED"):
		return true
	}
	switch provider {
	case model.ProviderYouTube:
		return strings.EqualFold(id, "VLLM") || strings.EqualFold(id, "LM")
	case model.ProviderSpotify:
		return strings.EqualFold(id, "spotify:playlist:liked") || strings.EqualFold(id, "liked")
	default:
		return false
	}
}
