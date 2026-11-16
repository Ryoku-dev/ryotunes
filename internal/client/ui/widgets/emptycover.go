package widgets

import (
	"image"
	"strings"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
)

// EmptyCover is the deliberate plate of a collection that has no cover at all (a playlist
// with no songs yet): the design's field plane in the cover's own shape (cut in Lithic,
// rounded in the round designs, look.FillArt), its kind's glyph centred in fg3. The hatch
// stays what it means elsewhere, a cover still on its way.
func EmptyCover(gtx layout.Context, th *look.Theme, size image.Point, shape look.Shape, icon *widget.Icon) layout.Dimensions {
	look.FillArt(gtx, th, size, shape, th.R.Field)
	side := min(size.X, size.Y)
	glyph := min(max(side*2/5, 1), gtx.Dp(48))
	if icon != nil && side > 0 {
		offset := op.Offset(image.Pt((size.X-glyph)/2, (size.Y-glyph)/2)).Push(gtx.Ops)
		look.DrawGlyph(gtx, icon, th.R.Fg3, glyph, 0, 1)
		offset.Pop()
	}
	return layout.Dimensions{Size: size}
}

// emptyCoverIcon is the glyph an empty plate wears for a collection of kind: a record for an
// album, a person for an artist, the heart for liked songs, else a playlist.
func emptyCoverIcon(kind string, liked bool) *widget.Icon {
	switch {
	case liked:
		return look.Icon.Heart
	case strings.EqualFold(kind, "album"), strings.EqualFold(kind, "single"), strings.EqualFold(kind, "ep"):
		return look.Icon.Album
	case strings.EqualFold(kind, "artist"), strings.EqualFold(kind, "channel"), strings.EqualFold(kind, "user"):
		return look.Icon.Person
	default:
		return look.Icon.Playlist
	}
}

// coverMissing reports a collection with no cover to wait for.
func coverMissing(url string) bool { return strings.TrimSpace(url) == "" }
