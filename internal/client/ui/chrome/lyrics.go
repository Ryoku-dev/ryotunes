// Package chrome implements the persistent playback surfaces shared by Ryotunes windows.
package chrome

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"strings"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	libtheme "github.com/ryoku-dev/ryoku/docs/lithic/library/go/theme"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

const lyricsManualHold = 5 * time.Second

// LyricsSize selects typography and density for each lyrics host.
type LyricsSize uint8

const (
	LyricsPanel LyricsSize = iota
	LyricsStage
	LyricsCompact
)

// Lyrics resolves, caches, and follows the current track's lyric document.
type Lyrics struct {
	scroll      look.Scroller
	lineClicks  map[string]*kit.Click
	follow      kit.Plaque
	retry       kit.Plaque
	lyrics      *model.Lyrics
	timed       []int
	trackID     string
	requestedID uint64
	revision    uint64
	loading     bool
	stale       bool
	err         string
	offsetMS    int64
	active      int
	lastActive  int
	manualUntil time.Time
	followAt    time.Time
	lastTarget  int
	romanized   bool
}

// Refresh reloads the current document and rejects stale replies.
func (lyrics *Lyrics) Refresh(e *env.Env) {
	if lyrics.trackID == "" {
		return
	}
	if lyrics.loading {
		lyrics.stale = true
		return
	}
	lyrics.fetch(e, lyrics.trackID, lyrics.requestedID)
}

// Layout draws lyrics in the typography and follow mode of their host.
func (lyrics *Lyrics) Layout(gtx layout.Context, e *env.Env, size LyricsSize) layout.Dimensions {
	lyrics.ensure(gtx, e)
	lyrics.romanized = settingEnabled(e.Store.Snapshot().Settings, model.SettingLyricsRomanize, false)
	if size == LyricsCompact {
		return lyrics.layoutCompact(gtx, e)
	}
	inset := layout.Inset{Top: 13, Bottom: 13}
	if size == LyricsPanel {
		inset.Left, inset.Right = 13, 13
	} else {
		inset.Left, inset.Right = 21, 21
	}
	return inset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return lyrics.layoutDocument(gtx, e, size)
	})
}

func (lyrics *Lyrics) ensure(gtx layout.Context, e *env.Env) {
	snapshot := e.Store.Snapshot()
	trackID := ""
	if snapshot.Playback.Now != nil {
		trackID = snapshot.Playback.Now.VideoID
	}
	if trackID != lyrics.trackID {
		lyrics.trackID = trackID
		lyrics.requestedID++
		lyrics.loading = false
		lyrics.stale = false
		lyrics.err = ""
		lyrics.lyrics = nil
		lyrics.timed = lyrics.timed[:0]
		lyrics.lineClicks = nil
		lyrics.offsetMS = 0
		lyrics.active = -1
		lyrics.lastActive = -2
		lyrics.manualUntil = time.Time{}
		lyrics.followAt = time.Time{}
		lyrics.lastTarget = 0
		lyrics.scroll.ScrollTo(0)
		lyrics.revision = snapshot.LyricsRevision
		if trackID != "" {
			lyrics.fetch(e, trackID, lyrics.requestedID)
		}
	}
	if snapshot.LyricsRevision != lyrics.revision {
		lyrics.revision = snapshot.LyricsRevision
		lyrics.Refresh(e)
	}
	if lyrics.lyrics != nil && lyrics.lyrics.Synced {
		lyrics.active = activeLyricLine(lyrics.lyrics.Lines, lyrics.timed, lyrics.positionMS(gtx, e))
		lyrics.scheduleBoundary(gtx, e)
	} else {
		lyrics.active = -1
	}
}

func (lyrics *Lyrics) fetch(e *env.Env, trackID string, requestID uint64) {
	snapshot := e.Store.Snapshot()
	if snapshot.Playback.Now == nil || snapshot.Playback.Now.VideoID != trackID {
		return
	}
	params := protocol.LyricsParams{VideoID: trackID, Title: snapshot.Playback.Now.Title, Artists: snapshot.Playback.Now.Artists}
	if snapshot.Playback.Duration > 0 {
		duration := snapshot.Playback.Duration
		params.Duration = &duration
	}
	if snapshot.Queue.CurrentIndex >= 0 && snapshot.Queue.CurrentIndex < len(snapshot.Queue.Items) {
		track := snapshot.Queue.Items[snapshot.Queue.CurrentIndex]
		if track.VideoID == trackID {
			params.Album = track.Album
		}
	}
	lyrics.loading = true
	lyrics.err = ""
	act.Call[*model.Lyrics](e, protocol.MethodGetLyrics, params, func(document *model.Lyrics, err error) {
		if requestID != lyrics.requestedID || trackID != lyrics.trackID {
			return
		}
		lyrics.loading = false
		if err != nil {
			lyrics.err = err.Error()
		} else {
			lyrics.err = ""
			lyrics.lyrics = document
			lyrics.timed = lyrics.timed[:0]
			if document != nil {
				for index := range document.Lines {
					if document.Lines[index].TimeMS != nil {
						lyrics.timed = append(lyrics.timed, index)
					}
				}
			}
		}
		if lyrics.stale {
			lyrics.stale = false
			lyrics.fetch(e, trackID, requestID)
		}
	})
}

func (lyrics *Lyrics) positionMS(gtx layout.Context, e *env.Env) int64 {
	return int64(math.Round(e.Store.PositionAt(gtx.Now)*1000)) - lyrics.offsetMS
}

func activeLyricLine(lines []model.LyricsLine, timed []int, positionMS int64) int {
	low, high := 0, len(timed)
	for low < high {
		middle := low + (high-low)/2
		if int64(*lines[timed[middle]].TimeMS) > positionMS {
			high = middle
		} else {
			low = middle + 1
		}
	}
	if low == 0 {
		return -1
	}
	return timed[low-1]
}

func activeLyricWord(words []model.LyricsWord, positionMS int64) int {
	low, high := 0, len(words)
	for low < high {
		middle := low + (high-low)/2
		if int64(words[middle].StartMS) > positionMS {
			high = middle
		} else {
			low = middle + 1
		}
	}
	return low - 1
}

func (lyrics *Lyrics) scheduleBoundary(gtx layout.Context, e *env.Env) {
	snapshot := e.Store.Snapshot()
	if snapshot.Playback.Paused || lyrics.lyrics == nil || !lyrics.lyrics.Synced {
		return
	}
	positionMS := lyrics.positionMS(gtx, e)
	const noBoundary = int64(1<<63 - 1)
	nextMS := noBoundary
	for _, index := range lyrics.timed {
		line := lyrics.lyrics.Lines[index]
		if line.TimeMS != nil && int64(*line.TimeMS) > positionMS {
			nextMS = min(nextMS, int64(*line.TimeMS))
			break
		}
	}
	if lyrics.active >= 0 && lyrics.active < len(lyrics.lyrics.Lines) {
		line := lyrics.lyrics.Lines[lyrics.active]
		if line.EndTimeMS != nil && int64(*line.EndTimeMS) > positionMS {
			nextMS = min(nextMS, int64(*line.EndTimeMS))
		}
		for _, word := range line.Words {
			if int64(word.StartMS) > positionMS {
				nextMS = min(nextMS, int64(word.StartMS))
				break
			}
			if int64(word.EndMS) > positionMS {
				nextMS = min(nextMS, int64(word.EndMS))
				break
			}
		}
	}
	if nextMS == noBoundary {
		return
	}
	speed := snapshot.AudioFx.Speed
	if speed <= 0 {
		speed = 1
	}
	delay := time.Duration(float64(nextMS-positionMS)/speed) * time.Millisecond
	if delay < time.Millisecond {
		delay = time.Millisecond
	}
	gtx.Execute(op.InvalidateCmd{At: gtx.Now.Add(delay)})
}

func (lyrics *Lyrics) layoutDocument(gtx layout.Context, e *env.Env, size LyricsSize) layout.Dimensions {
	if e.Store.Snapshot().Playback.Now == nil {
		empty := kit.Empty{Icon: look.Icon.Lyrics, Title: "No lyrics for this track"}
		return empty.Layout(gtx, e.Th.Theme)
	}
	if lyrics.lyrics == nil {
		if lyrics.loading {
			return lyricsCentered(gtx, e, "Resolving lyrics", lyricsTrackProof(e))
		}
		if lyrics.err != "" {
			lyrics.retry.Kind = kit.PlaqueLine
			lyrics.retry.Icon = look.Icon.Refresh
			lyrics.retry.Label = "Try again"
			empty := kit.Empty{Icon: look.Icon.Lyrics, Title: "Lyrics could not be loaded", Body: lyrics.err, Actions: []*kit.Plaque{&lyrics.retry}}
			dims := empty.Layout(gtx, e.Th.Theme)
			if lyrics.retry.Clicked(gtx) {
				lyrics.Refresh(e)
			}
			return dims
		}
		empty := kit.Empty{Icon: look.Icon.Lyrics, Title: "No lyrics for this track"}
		return empty.Layout(gtx, e.Th.Theme)
	}
	if lyrics.lyrics.Instrumental {
		empty := kit.Empty{Icon: look.Icon.Lyrics, Title: "No lyrics for this track", Body: "This recording is instrumental."}
		return empty.Layout(gtx, e.Th.Theme)
	}
	if len(lyrics.lyrics.Lines) == 0 {
		empty := kit.Empty{Icon: look.Icon.Lyrics, Title: "No lyrics for this track"}
		return empty.Layout(gtx, e.Th.Theme)
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if lyrics.lyrics.Synced {
				return lyrics.layoutSynced(gtx, e, size)
			}
			return lyrics.layoutPlain(gtx, e, size)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if strings.TrimSpace(lyrics.lyrics.Source) == "" {
				return layout.Dimensions{}
			}
			return layout.Inset{Top: 8}.Layout(gtx, kit.Text(e.Th.Theme, e.Th.T.Caption, e.Th.R.Fg3, "Lyrics from "+strings.TrimSpace(lyrics.lyrics.Source)))
		}),
	)
}

func (lyrics *Lyrics) layoutPlain(gtx layout.Context, e *env.Env, size LyricsSize) layout.Dimensions {
	return lyrics.scroll.Layout(gtx, e.Th, len(lyrics.lyrics.Lines), func(gtx layout.Context, index int) layout.Dimensions {
		line := lyrics.lyrics.Lines[index]
		return layout.Inset{Top: 8, Bottom: 13}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return lyrics.layoutLine(gtx, e, line, false, size)
		})
	})
}

func (lyrics *Lyrics) layoutSynced(gtx layout.Context, e *env.Env, size LyricsSize) layout.Dimensions {
	viewport := gtx.Constraints.Max.Y
	rowHeight := gtx.Dp(88)
	if size == LyricsStage {
		rowHeight = gtx.Dp(108)
	}
	topPad := max(gtx.Dp(13), viewport/3-rowHeight/2)
	bottomPad := max(gtx.Dp(13), viewport-topPad-rowHeight)
	beforeFirst, beforeOffset := lyrics.scroll.Offset()
	beforeAbsolute := lyricsAbsolute(beforeFirst, beforeOffset, topPad, rowHeight)
	manual := gtx.Now.Before(lyrics.manualUntil)
	if !manual && lyrics.active >= 0 && (lyrics.active != lyrics.lastActive || lyrics.lastTarget != viewport) {
		target := max(0, topPad+lyrics.active*rowHeight+rowHeight/2-viewport/3)
		delta := target - beforeAbsolute
		lyrics.scroll.SpringTo(beforeOffset + delta)
		lyrics.followAt = gtx.Now
		lyrics.lastActive = lyrics.active
		lyrics.lastTarget = viewport
	}
	count := len(lyrics.lyrics.Lines) + 2
	dims := lyrics.scroll.Layout(gtx, e.Th, count, func(gtx layout.Context, index int) layout.Dimensions {
		if index == 0 {
			return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, topPad)}
		}
		if index == count-1 {
			return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, bottomPad)}
		}
		lineIndex := index - 1
		line := lyrics.lyrics.Lines[lineIndex]
		rowGTX := gtx
		rowGTX.Constraints.Min.Y = rowHeight
		rowGTX.Constraints.Max.Y = rowHeight
		click := lyrics.lineClick(line, lineIndex, size)
		dims := click.Layout(rowGTX, func(gtx layout.Context) layout.Dimensions {
			return lyrics.layoutLine(gtx, e, line, lineIndex == lyrics.active, size)
		})
		if click.Clicked(gtx) && line.TimeMS != nil {
			lyrics.seek(e, float64(int64(*line.TimeMS)+lyrics.offsetMS)/1000)
			lyrics.manualUntil = time.Time{}
		}
		return dims
	})
	afterFirst, afterOffset := lyrics.scroll.Offset()
	afterAbsolute := lyricsAbsolute(afterFirst, afterOffset, topPad, rowHeight)
	if afterAbsolute != beforeAbsolute && gtx.Now.Sub(lyrics.followAt) > 500*time.Millisecond {
		lyrics.manualUntil = gtx.Now.Add(lyricsManualHold)
		lyrics.lastActive = -2
		gtx.Execute(op.InvalidateCmd{At: lyrics.manualUntil})
	}
	if gtx.Now.Before(lyrics.manualUntil) {
		buttonGTX := gtx
		buttonGTX.Constraints.Min = image.Point{}
		buttonGTX.Constraints.Max.X = min(gtx.Dp(120), dims.Size.X)
		lyrics.follow.Kind = kit.PlaqueQuiet
		lyrics.follow.Label = "Follow"
		lyrics.follow.Small = true
		offset := op.Offset(image.Pt(max(0, dims.Size.X-gtx.Dp(120)), max(0, dims.Size.Y-gtx.Dp(40)))).Push(gtx.Ops)
		lyrics.follow.Layout(buttonGTX, e.Th.Theme)
		if lyrics.follow.Clicked(buttonGTX) {
			lyrics.manualUntil = time.Time{}
			lyrics.lastActive = -2
		}
		offset.Pop()
	}
	return dims
}

func lyricsAbsolute(first, offset, topPad, rowHeight int) int {
	if first <= 0 {
		return offset
	}
	return topPad + (first-1)*rowHeight + offset
}

func (lyrics *Lyrics) lineClick(line model.LyricsLine, index int, size LyricsSize) *kit.Click {
	if lyrics.lineClicks == nil {
		lyrics.lineClicks = make(map[string]*kit.Click)
	}
	key := fmt.Sprintf("%d:%d:%d", size, valueOrZero(line.TimeMS), index)
	click := lyrics.lineClicks[key]
	if click == nil {
		click = new(kit.Click)
		lyrics.lineClicks[key] = click
	}
	return click
}

func (lyrics *Lyrics) layoutLine(gtx layout.Context, e *env.Env, line model.LyricsLine, active bool, size LyricsSize) layout.Dimensions {
	style, ink := lyricsStyle(e, size, active)
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		maxWidth := gtx.Dp(560)
		if size == LyricsStage {
			maxWidth = gtx.Dp(760)
		}
		gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, maxWidth)
		children := []layout.FlexChild{
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if active && len(line.Words) > 0 {
					return lyrics.layoutWords(gtx, e, style, line.Words)
				}
				return (kit.Label{Style: style, Color: ink, MaxLines: 2, Align: text.Middle}).Layout(gtx, e.Th.Theme, strings.TrimSpace(line.Text))
			}),
		}
		if lyrics.romanized && line.Romanized != nil && strings.TrimSpace(*line.Romanized) != "" {
			children = append(children,
				layout.Rigid(layout.Spacer{Height: 4}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return (kit.Label{Style: e.Th.T.Caption, Color: ink, MaxLines: 2, Align: text.Middle}).Layout(gtx, e.Th.Theme, strings.TrimSpace(*line.Romanized))
				}),
			)
		}
		if line.Translation != nil && strings.TrimSpace(*line.Translation) != "" {
			children = append(children,
				layout.Rigid(layout.Spacer{Height: 4}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return (kit.Label{Style: e.Th.T.Caption, Color: ink, MaxLines: 2, Align: text.Middle}).Layout(gtx, e.Th.Theme, strings.TrimSpace(*line.Translation))
				}),
			)
		}
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx, children...)
	})
}

func lyricsStyle(e *env.Env, size LyricsSize, active bool) (libtheme.TextStyle, color.NRGBA) {
	if !active {
		if size == LyricsStage {
			return e.Th.T.Section, e.Th.R.Fg3
		}
		return e.Th.T.Proof, e.Th.R.Fg3
	}
	switch size {
	case LyricsStage:
		return e.Th.T.Sentence, e.Th.R.Fg
	case LyricsCompact:
		return e.Th.T.Row, e.Th.R.Fg
	default:
		return e.Th.T.Section, e.Th.R.Fg
	}
}

func (lyrics *Lyrics) layoutWords(gtx layout.Context, e *env.Env, style libtheme.TextStyle, words []model.LyricsWord) layout.Dimensions {
	activeWord := activeLyricWord(words, lyrics.positionMS(gtx, e))
	children := make([]layout.FlexChild, 0, len(words))
	for index := range words {
		index := index
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			word := words[index].Text
			if index+1 < len(words) {
				word += " "
			}
			ink := e.Th.R.Fg
			if index <= activeWord {
				ink = e.Th.Src.Ink
			}
			return kit.Text(e.Th.Theme, style, ink, word)(gtx)
		}))
	}
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
}

func (lyrics *Lyrics) layoutCompact(gtx layout.Context, e *env.Env) layout.Dimensions {
	if lyrics.lyrics == nil || lyrics.lyrics.Instrumental || !lyrics.lyrics.Synced || len(lyrics.lyrics.Lines) == 0 {
		empty := kit.Empty{Icon: look.Icon.Lyrics, Title: "No lyrics for this track"}
		return empty.Layout(gtx, e.Th.Theme)
	}
	center := max(0, lyrics.active)
	return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return lyrics.compactLine(gtx, e, center-1) }),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return lyrics.compactLine(gtx, e, center) }),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return lyrics.compactLine(gtx, e, center+1) }),
	)
}

func (lyrics *Lyrics) compactLine(gtx layout.Context, e *env.Env, index int) layout.Dimensions {
	if index < 0 || index >= len(lyrics.lyrics.Lines) {
		return layout.Dimensions{Size: gtx.Constraints.Min}
	}
	line := lyrics.lyrics.Lines[index]
	click := lyrics.lineClick(line, index, LyricsCompact)
	dims := click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return lyrics.layoutLine(gtx, e, line, index == lyrics.active, LyricsCompact)
	})
	if click.Clicked(gtx) && line.TimeMS != nil {
		lyrics.seek(e, float64(int64(*line.TimeMS)+lyrics.offsetMS)/1000)
	}
	return dims
}

func (lyrics *Lyrics) seek(e *env.Env, position float64) {
	act.Call[protocol.Null](e, protocol.MethodSeek, protocol.SeekParams{Position: max(0, position)}, func(_ protocol.Null, err error) {
		if err != nil && e.Toast != nil {
			e.Toast(err.Error(), env.ToastError)
		}
	})
}

func lyricsCentered(gtx layout.Context, e *env.Env, title, body string) layout.Dimensions {
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(420))
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(kit.Text(e.Th.Theme, e.Th.T.Section, e.Th.R.Fg, title)),
			layout.Rigid(layout.Spacer{Height: 8}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return (kit.Label{Style: e.Th.T.Proof, Color: e.Th.R.Fg2, MaxLines: 3, Align: text.Middle}).Layout(gtx, e.Th.Theme, body)
			}),
		)
	})
}

func lyricsTrackProof(e *env.Env) string {
	now := e.Store.Snapshot().Playback.Now
	if now == nil {
		return ""
	}
	parts := make([]string, 0, 2)
	if title := strings.TrimSpace(now.Title); title != "" {
		parts = append(parts, title)
	}
	if artists := strings.TrimSpace(now.Artists); artists != "" {
		parts = append(parts, artists)
	}
	return strings.Join(parts, " · ")
}

func valueOrZero(value *uint64) uint64 {
	if value == nil {
		return 0
	}
	return *value
}

var _ env.Refresher = (*Lyrics)(nil)
