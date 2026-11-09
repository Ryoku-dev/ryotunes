package look

import (
	"image"
	"image/color"
	"math"
	"strings"

	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"golang.org/x/image/math/fixed"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/anim"
	libtheme "github.com/ryoku-dev/ryoku/docs/lithic/library/go/theme"
)

// KaraokeStyle describes one retained word-by-word lyric line.
type KaraokeStyle struct {
	Key                    string
	Text                   libtheme.TextStyle
	Base, Highlight        color.NRGBA
	Align                  text.Alignment
	PreferredRows, MaxRows int
	MinScale               float32
	Lift                   unit.Dp
	PositionMS             int64
}

// KaraokeResult reports the fitted line geometry.
type KaraokeResult struct {
	Dimensions layout.Dimensions
	Rows       int
}

type karaokeMeasureKey struct {
	line       string
	face       font.Font
	size       int
	lineHeight int
}

type karaokeGlyphWord struct {
	glyphs   []text.Glyph
	path     clip.PathSpec
	width    int
	baseline f32.Point
}

type karaokeFlowRow struct {
	first, last int
	width       int
}

type karaokeFlowLayout struct {
	positions []image.Point
	rows      []karaokeFlowRow
	scale     float32
	height    int
}

type karaokeFlowKey struct {
	width, lineHeight, preferred, maximum int
	minimumScale                          float32
	align                                 text.Alignment
	rtl                                   bool
}

// Karaoke retains shaped words and flow buffers across playback frames.
type Karaoke struct {
	measureKey karaokeMeasureKey
	flowKey    karaokeFlowKey
	measured   []karaokeGlyphWord
	widths     []int
	spaces     []int
	positions  []image.Point
	rows       []karaokeFlowRow
	flow       karaokeFlowLayout
	spaceWidth int
	label      string
	flowReady  bool
}

// Layout draws one line without reshaping its words after the cache is warm.
func (karaoke *Karaoke) Layout(gtx layout.Context, th *Theme, words []KaraokeWord, style KaraokeStyle) KaraokeResult {
	if len(words) == 0 || gtx.Constraints.Max.X <= 0 {
		return KaraokeResult{}
	}
	preferred, maximum := max(1, style.PreferredRows), max(1, style.MaxRows)
	if preferred > maximum {
		preferred = maximum
	}
	minimumScale := style.MinScale
	if minimumScale <= 0 || minimumScale > 1 {
		minimumScale = .72
	}
	lineHeight := max(1, gtx.Sp(style.Text.Line))
	key := karaokeMeasureKey{
		line: style.Key, face: style.Text.Face, size: gtx.Sp(style.Text.Size), lineHeight: lineHeight,
	}
	if key != karaoke.measureKey || len(karaoke.measured) != len(words) {
		karaoke.shape(gtx, th, words, style.Text, key)
	}

	rtl := words[0].RTL
	flowKey := karaokeFlowKey{
		width: gtx.Constraints.Max.X, lineHeight: lineHeight, preferred: preferred, maximum: maximum,
		minimumScale: minimumScale, align: style.Align, rtl: rtl,
	}
	if !karaoke.flowReady || flowKey != karaoke.flowKey {
		karaoke.flow = layoutKaraokeFlowInto(
			karaoke.positions, karaoke.rows, karaoke.widths, karaoke.spaces,
			flowKey.width, flowKey.lineHeight, flowKey.preferred, flowKey.maximum, flowKey.minimumScale, flowKey.align, flowKey.rtl,
		)
		karaoke.positions, karaoke.rows = karaoke.flow.positions, karaoke.flow.rows
		karaoke.flowKey, karaoke.flowReady = flowKey, true
	}
	flow := karaoke.flow

	lift := gtx.Dp(style.Lift)
	if anim.Reduced() {
		lift = 0
	}
	contentHeight := flow.height + int(math.Ceil(float64(float32(lift)*flow.scale)))
	size := gtx.Constraints.Constrain(image.Pt(gtx.Constraints.Max.X, contentHeight))
	transform := op.Affine(f32.AffineId().Scale(f32.Point{}, f32.Pt(flow.scale, flow.scale))).Push(gtx.Ops)
	for index := range words {
		progress := KaraokeFillProgress(words[index], style.PositionMS)
		active := style.PositionMS >= int64(words[index].StartMS) && style.PositionMS < int64(words[index].EndMS)
		if anim.Reduced() && active {
			progress = 1
		}
		rise := float32(lift) * anim.EaseChisel(progress)
		point := karaoke.positions[index]
		point.Y += lift - int(math.Round(float64(rise)))
		drawKaraokeWord(gtx, karaoke.measured[index], point, style.Base)
		if progress <= 0 {
			continue
		}
		if progress >= 1 {
			drawKaraokeWord(gtx, karaoke.measured[index], point, style.Highlight)
			continue
		}
		drawKaraokeWordFill(gtx, karaoke.measured[index], point, lineHeight, progress, style.Highlight, rtl)
	}
	transform.Pop()
	semantic.LabelOp(karaoke.label).Add(gtx.Ops)
	return KaraokeResult{
		Dimensions: layout.Dimensions{Size: size, Baseline: contentHeight - int(math.Ceil(float64(float32(lineHeight)*flow.scale/4)))},
		Rows:       len(flow.rows),
	}
}

func (karaoke *Karaoke) shape(gtx layout.Context, th *Theme, words []KaraokeWord, style libtheme.TextStyle, key karaokeMeasureKey) {
	karaoke.measureKey = key
	karaoke.measured = resizeKaraokeGlyphWords(karaoke.measured, len(words))
	karaoke.widths = resizeInts(karaoke.widths, len(words))
	karaoke.spaces = resizeInts(karaoke.spaces, len(words))
	karaoke.positions = resizeImagePoints(karaoke.positions, len(words))
	karaoke.rows = karaoke.rows[:0]
	karaoke.spaceWidth = karaokeSpaceWidth(gtx, th, style)
	karaoke.label = karaokeWordsLabel(words)
	karaoke.flowReady = false
	for index := range words {
		karaoke.measured[index] = shapeKaraokeWord(gtx, th, style, strings.TrimSpace(words[index].Text), karaoke.measured[index].glyphs)
		karaoke.measured[index].path = th.Shaper.Shape(karaoke.measured[index].glyphs)
		karaoke.widths[index] = karaoke.measured[index].width
		karaoke.spaces[index] = 0
		if words[index].SpaceBefore {
			karaoke.spaces[index] = karaoke.spaceWidth
		}
	}
}

func shapeKaraokeWord(gtx layout.Context, th *Theme, style libtheme.TextStyle, value string, storage []text.Glyph) karaokeGlyphWord {
	th.Shaper.LayoutString(text.Parameters{
		Font: style.Face, PxPerEm: fixed.I(gtx.Sp(style.Size)), MaxLines: 1,
		MaxWidth: 1 << 20, Locale: gtx.Locale, LineHeight: fixed.I(gtx.Sp(style.Line)), LineHeightScale: 1,
		DisableSpaceTrim: true,
	}, value)
	glyphs := storage[:0]
	width := 0
	baseline := f32.Point{}
	for glyph, ok := th.Shaper.NextGlyph(); ok; glyph, ok = th.Shaper.NextGlyph() {
		if len(glyphs) == 0 {
			baseline = f32.Pt(float32(glyph.X)/64, float32(glyph.Y))
		}
		glyphs = append(glyphs, glyph)
		width = max(width, (glyph.X + glyph.Advance).Ceil())
		if glyph.Flags&text.FlagLineBreak != 0 {
			break
		}
	}
	return karaokeGlyphWord{glyphs: glyphs, width: max(1, width), baseline: baseline}
}

func karaokeSpaceWidth(gtx layout.Context, th *Theme, style libtheme.TextStyle) int {
	th.Shaper.LayoutString(text.Parameters{
		Font: style.Face, PxPerEm: fixed.I(gtx.Sp(style.Size)), MaxLines: 1,
		MaxWidth: 1 << 20, Locale: gtx.Locale, LineHeight: fixed.I(gtx.Sp(style.Line)), LineHeightScale: 1,
		DisableSpaceTrim: true,
	}, " ")
	width := 0
	for glyph, ok := th.Shaper.NextGlyph(); ok; glyph, ok = th.Shaper.NextGlyph() {
		width = max(width, (glyph.X + glyph.Advance).Ceil())
		if glyph.Flags&text.FlagLineBreak != 0 {
			break
		}
	}
	return max(1, width)
}

func drawKaraokeWord(gtx layout.Context, word karaokeGlyphWord, point image.Point, ink color.NRGBA) {
	if len(word.glyphs) == 0 || ink.A == 0 {
		return
	}
	move := op.Affine(f32.AffineId().Offset(f32.Pt(float32(point.X)+word.baseline.X, float32(point.Y)+word.baseline.Y))).Push(gtx.Ops)
	paint.FillShape(gtx.Ops, ink, clip.Outline{Path: word.path}.Op())
	move.Pop()
}

func drawKaraokeWordFill(gtx layout.Context, word karaokeGlyphWord, point image.Point, lineHeight int, progress float32, ink color.NRGBA, rtl bool) {
	points := karaokeFillPolygon(point, word.width, lineHeight, progress, rtl)
	var path clip.Path
	path.Begin(gtx.Ops)
	path.MoveTo(points[0])
	path.LineTo(points[1])
	path.LineTo(points[2])
	path.LineTo(points[3])
	path.Close()
	mask := clip.Outline{Path: path.End()}.Op().Push(gtx.Ops)
	drawKaraokeWord(gtx, word, point, ink)
	mask.Pop()
}

func karaokeFillPolygon(point image.Point, width, lineHeight int, progress float32, rtl bool) [4]f32.Point {
	left := float32(point.X)
	height := float32(lineHeight)
	right := left + float32(width)
	top := float32(point.Y) - height
	bottom := float32(point.Y+lineHeight) + height
	edge := left - 2*height + progress*(float32(width)+3*height)
	if rtl {
		sum := left + right
		return [4]f32.Point{
			f32.Pt(right+2*height, top),
			f32.Pt(sum-(edge-height), top),
			f32.Pt(sum-(edge+2*height), bottom),
			f32.Pt(right+2*height, bottom),
		}
	}
	return [4]f32.Point{
		f32.Pt(left-2*height, top),
		f32.Pt(edge-height, top),
		f32.Pt(edge+2*height, bottom),
		f32.Pt(left-2*height, bottom),
	}
}

// KaraokeFillProgress returns the clock position of the wipe through one word.
func KaraokeFillProgress(word KaraokeWord, positionMS int64) float32 {
	if positionMS >= int64(word.EndMS) {
		return 1
	}
	if word.EndMS <= word.StartMS || positionMS <= int64(word.StartMS) {
		return 0
	}
	return float32(positionMS-int64(word.StartMS)) / float32(word.EndMS-word.StartMS)
}

func layoutKaraokeFlowInto(positions []image.Point, rows []karaokeFlowRow, widths, spaces []int, maxWidth, lineHeight, preferredRows, maxRows int, minScale float32, align text.Alignment, rtl bool) karaokeFlowLayout {
	positions = resizeImagePoints(positions, len(widths))
	rows = karaokeRows(rows[:0], widths, spaces, maxWidth, 1)
	scale := float32(1)
	if !karaokeRowsFit(rows, maxWidth, scale, preferredRows) {
		atMinimum := karaokeRows(rows[:0], widths, spaces, maxWidth, minScale)
		if karaokeRowsFit(atMinimum, maxWidth, minScale, preferredRows) {
			low, high := minScale, float32(1)
			for range 18 {
				middle := (low + high) / 2
				candidate := karaokeRows(rows[:0], widths, spaces, maxWidth, middle)
				if karaokeRowsFit(candidate, maxWidth, middle, preferredRows) {
					low = middle
				} else {
					high = middle
				}
			}
			scale = low
			rows = karaokeRows(rows[:0], widths, spaces, maxWidth, scale)
		} else {
			scale, rows = minScale, atMinimum
		}
	}
	if !karaokeRowsFit(rows, maxWidth, scale, maxRows) {
		low, high := float32(.001), scale
		for range 24 {
			middle := (low + high) / 2
			candidate := karaokeRows(rows[:0], widths, spaces, maxWidth, middle)
			if karaokeRowsFit(candidate, maxWidth, middle, maxRows) {
				low = middle
			} else {
				high = middle
			}
		}
		scale = low
		rows = karaokeRows(rows[:0], widths, spaces, maxWidth, scale)
	}
	available := max(1, int(float32(maxWidth)/max(scale, .001)))
	for rowIndex := range rows {
		row := rows[rowIndex]
		x := 0
		rowAlign := align
		if rtl {
			if rowAlign == text.Start {
				rowAlign = text.End
			} else if rowAlign == text.End {
				rowAlign = text.Start
			}
		}
		if rowAlign == text.Middle {
			x = max(0, (available-row.width)/2)
		} else if rowAlign == text.End {
			x = max(0, available-row.width)
		}
		if rtl {
			cursor := x + row.width
			for index := row.first; index < row.last; index++ {
				if index > row.first {
					cursor -= spaces[index]
				}
				cursor -= widths[index]
				positions[index] = image.Pt(cursor, rowIndex*lineHeight)
			}
			continue
		}
		for index := row.first; index < row.last; index++ {
			if index > row.first {
				x += spaces[index]
			}
			positions[index] = image.Pt(x, rowIndex*lineHeight)
			x += widths[index]
		}
	}
	return karaokeFlowLayout{positions: positions, rows: rows, scale: scale, height: int(math.Ceil(float64(float32(len(rows)*lineHeight) * scale)))}
}

func karaokeRows(rows []karaokeFlowRow, widths, spaces []int, maxWidth int, scale float32) []karaokeFlowRow {
	available := max(1, int(float32(maxWidth)/max(scale, .001)))
	first, width := 0, 0
	for index := range widths {
		advance := widths[index]
		if index > first {
			advance += spaces[index]
		}
		if index > first && width+advance > available {
			rows = append(rows, karaokeFlowRow{first: first, last: index, width: width})
			first, width, advance = index, 0, widths[index]
		}
		width += advance
	}
	if first < len(widths) {
		rows = append(rows, karaokeFlowRow{first: first, last: len(widths), width: width})
	}
	return rows
}

func karaokeRowsFit(rows []karaokeFlowRow, maxWidth int, scale float32, maximum int) bool {
	if len(rows) > maximum {
		return false
	}
	available := max(1, int(float32(maxWidth)/max(scale, .001)))
	for _, row := range rows {
		if row.width > available {
			return false
		}
	}
	return true
}

func karaokeWordsLabel(words []KaraokeWord) string {
	var value strings.Builder
	for index := range words {
		if words[index].SpaceBefore && value.Len() > 0 {
			value.WriteByte(' ')
		}
		value.WriteString(strings.TrimSpace(words[index].Text))
	}
	return value.String()
}

func resizeKaraokeGlyphWords(values []karaokeGlyphWord, length int) []karaokeGlyphWord {
	if cap(values) < length {
		return make([]karaokeGlyphWord, length)
	}
	return values[:length]
}

func resizeInts(values []int, length int) []int {
	if cap(values) < length {
		return make([]int, length)
	}
	return values[:length]
}

func resizeImagePoints(values []image.Point, length int) []image.Point {
	if cap(values) < length {
		return make([]image.Point, length)
	}
	return values[:length]
}
