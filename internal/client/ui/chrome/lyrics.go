// Package chrome implements the persistent playback surfaces shared by Ryotunes windows.
package chrome

import (
	"fmt"
	"image"
	"math"
	"strings"
	"time"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"

	"github.com/ryoku-dev/ryotunes/internal/client/lithic"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

const lyricsManualHold = 5 * time.Second

// LyricsPanel resolves and follows the current track's lyrics.
type LyricsPanel struct {
	list          lithic.List
	lineClicks    map[string]*lithic.Click
	current       lithic.Button
	retry         lithic.Button
	romanize      lithic.Button
	lyrics        *model.Lyrics
	timed         []int
	trackID       string
	requestedID   uint64
	revision      uint64
	loading       bool
	stale         bool
	err           string
	offsetMS      int64
	active        int
	manualUntil   time.Time
	scroll        lithic.Tween
	scrollReady   bool
	lastScrollY   float32
	showRomanized bool
}

// Refresh reloads the visible track without discarding the current document or scroll position.
func (p *LyricsPanel) Refresh(e *env.Env) {
	if p.trackID == "" {
		return
	}
	if p.loading {
		p.stale = true
		return
	}
	p.fetch(e, p.trackID, p.requestedID)
}

// Layout renders the full lyrics panel.
func (p *LyricsPanel) Layout(gtx layout.Context, e *env.Env) layout.Dimensions {
	p.ensure(gtx, e)
	romanize := settingEnabled(e.Store.Snapshot().Settings, model.SettingLyricsRomanize, false)
	if p.romanize.Clicked(gtx) {
		romanize = !romanize
		value := "false"
		if romanize {
			value = "true"
		}
		callVoid(e, protocol.MethodSetSetting, protocol.SetSettingParams{Key: string(model.SettingLyricsRomanize), Value: value})
	}
	p.showRomanized = romanize
	return layout.Inset{Top: 21, Bottom: 13, Left: 13, Right: 13}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Flexed(1, lithic.Text(e.Th, e.Th.T.Section, e.Th.R.Fg, "Lyrics")),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if !p.hasRomanizableLine() {
							return layout.Dimensions{}
						}
						p.romanize.Small = true
						return p.romanize.Layout(gtx, e.Th, activeKind(romanize), lithic.Icon("translate"), "Romanize")
					}),
				)
			}),
			layout.Rigid(layout.Spacer{Height: 13}.Layout),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return p.layoutFull(gtx, e)
			}),
		)
	})
}

// LayoutCompact renders the three-line follow view used by the mini player.
func (p *LyricsPanel) LayoutCompact(gtx layout.Context, e *env.Env) layout.Dimensions {
	p.ensure(gtx, e)
	p.showRomanized = settingEnabled(e.Store.Snapshot().Settings, model.SettingLyricsRomanize, false)
	return p.layoutCompact(gtx, e)
}

func (p *LyricsPanel) hasRomanizableLine() bool {
	if p.lyrics == nil {
		return false
	}
	for _, line := range p.lyrics.Lines {
		if line.Romanized != nil && strings.TrimSpace(*line.Romanized) != "" {
			return true
		}
	}
	return false
}

func (p *LyricsPanel) ensure(gtx layout.Context, e *env.Env) {
	state := e.Store.Snapshot()
	trackID := ""
	if state.Playback.Now != nil {
		trackID = state.Playback.Now.VideoID
	}
	if trackID != p.trackID {
		p.trackID = trackID
		p.requestedID++
		p.loading = false
		p.stale = false
		p.err = ""
		p.lyrics = nil
		p.timed = p.timed[:0]
		p.offsetMS = 0
		p.active = -1
		p.manualUntil = time.Time{}
		p.list.Position = layout.Position{}
		p.scrollReady = false
		p.revision = state.LyricsRevision
		if trackID != "" {
			p.fetch(e, trackID, p.requestedID)
		}
	}
	if state.LyricsRevision != p.revision {
		p.revision = state.LyricsRevision
		if trackID != "" {
			p.Refresh(e)
		}
	}
	if p.lyrics != nil && p.lyrics.Synced {
		p.active = activeLyricLine(p.lyrics.Lines, p.timed, p.positionMS(gtx, e))
		p.scheduleBoundary(gtx, e)
	} else {
		p.active = -1
	}
}

func (p *LyricsPanel) fetch(e *env.Env, trackID string, requestID uint64) {
	state := e.Store.Snapshot()
	if state.Playback.Now == nil || state.Playback.Now.VideoID != trackID {
		return
	}
	params := protocol.LyricsParams{
		VideoID: trackID,
		Title:   state.Playback.Now.Title,
		Artists: state.Playback.Now.Artists,
	}
	if state.Playback.Duration > 0 {
		duration := state.Playback.Duration
		params.Duration = &duration
	}
	if state.Queue.CurrentIndex >= 0 && state.Queue.CurrentIndex < len(state.Queue.Items) {
		item := state.Queue.Items[state.Queue.CurrentIndex]
		if item.VideoID == trackID {
			params.Album = item.Album
		}
	}
	p.loading = true
	p.err = ""
	act.Call[*model.Lyrics](e, protocol.MethodGetLyrics, params, func(lyrics *model.Lyrics, err error) {
		if requestID != p.requestedID || trackID != p.trackID {
			return
		}
		p.loading = false
		if err != nil {
			p.err = err.Error()
		} else {
			p.err = ""
			p.lyrics = lyrics
			p.timed = p.timed[:0]
			if lyrics != nil {
				for i := range lyrics.Lines {
					if lyrics.Lines[i].TimeMS != nil {
						p.timed = append(p.timed, i)
					}
				}
			}
		}
		if p.stale {
			p.stale = false
			p.fetch(e, trackID, requestID)
		}
	})
}

func (p *LyricsPanel) positionMS(gtx layout.Context, e *env.Env) int64 {
	return int64(math.Round(e.Store.PositionAt(gtx.Now)*1000)) - p.offsetMS
}

func activeLyricLine(lines []model.LyricsLine, timed []int, positionMS int64) int {
	lo, hi := 0, len(timed)
	for lo < hi {
		mid := lo + (hi-lo)/2
		line := timed[mid]
		if int64(*lines[line].TimeMS) > positionMS {
			hi = mid
		} else {
			lo = mid + 1
		}
	}
	if lo == 0 {
		return -1
	}
	return timed[lo-1]
}

func activeLyricWord(words []model.LyricsWord, positionMS int64) int {
	lo, hi := 0, len(words)
	for lo < hi {
		mid := lo + (hi-lo)/2
		if int64(words[mid].StartMS) > positionMS {
			hi = mid
		} else {
			lo = mid + 1
		}
	}
	return lo - 1
}

func (p *LyricsPanel) scheduleBoundary(gtx layout.Context, e *env.Env) {
	snapshot := e.Store.Snapshot()
	if snapshot.Playback.Paused || p.lyrics == nil || !p.lyrics.Synced {
		return
	}
	positionMS := p.positionMS(gtx, e)
	const noBoundary = int64(1<<63 - 1)
	nextMS := noBoundary
	lo, hi := 0, len(p.timed)
	for lo < hi {
		mid := lo + (hi-lo)/2
		line := p.lyrics.Lines[p.timed[mid]]
		if int64(*line.TimeMS) <= positionMS {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	if lo < len(p.timed) {
		nextMS = int64(*p.lyrics.Lines[p.timed[lo]].TimeMS)
	}
	if p.active >= 0 && p.active < len(p.lyrics.Lines) {
		line := p.lyrics.Lines[p.active]
		if line.EndTimeMS != nil && int64(*line.EndTimeMS) > positionMS {
			nextMS = min(nextMS, int64(*line.EndTimeMS))
		}
		for i := range line.Words {
			word := line.Words[i]
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

func (p *LyricsPanel) layoutFull(gtx layout.Context, e *env.Env) layout.Dimensions {
	if e.Store.Snapshot().Playback.Now == nil {
		return centeredMessage(gtx, e.Th, "Nothing playing.", "Play a track to see its lyrics.")
	}
	if p.lyrics == nil {
		if p.loading {
			return centeredMessage(gtx, e.Th, "Resolving lyrics", currentTrackProof(e))
		}
		if p.err != "" {
			return p.layoutError(gtx, e)
		}
		return p.layoutEmpty(gtx, e, false)
	}
	if p.lyrics.Instrumental {
		return p.layoutEmpty(gtx, e, true)
	}
	if len(p.lyrics.Lines) == 0 {
		return p.layoutEmpty(gtx, e, false)
	}
	if !p.lyrics.Synced {
		return p.layoutPlain(gtx, e)
	}
	return p.layoutSynced(gtx, e)
}

func (p *LyricsPanel) layoutError(gtx layout.Context, e *env.Env) layout.Dimensions {
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(lithic.Text(e.Th, e.Th.T.Title, e.Th.R.Fg, "Lyrics could not be loaded.")),
			layout.Rigid(layout.Spacer{Height: e.Th.Sp.S2}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				label := lithic.Label{Style: e.Th.T.Body, Color: e.Th.R.Fg3, MaxLines: 3, Align: text.Middle}
				return label.Layout(gtx, e.Th, p.err)
			}),
			layout.Rigid(layout.Spacer{Height: e.Th.Sp.S3}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				dims := p.retry.Layout(gtx, e.Th, lithic.Line, lithic.Icon("refresh"), "Retry lookup")
				if p.retry.Clicked(gtx) {
					p.Refresh(e)
				}
				return dims
			}),
		)
	})
}

func (p *LyricsPanel) layoutEmpty(gtx layout.Context, e *env.Env, instrumental bool) layout.Dimensions {
	if instrumental {
		return centeredMessage(gtx, e.Th, "Instrumental track.", "There are no vocal lines to follow for this recording.")
	}
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(420))
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(lithic.Text(e.Th, e.Th.T.Title, e.Th.R.Fg, "No lyrics found for this track.")),
			layout.Rigid(layout.Spacer{Height: e.Th.Sp.S2}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return (lithic.Label{Style: e.Th.T.Body, Color: e.Th.R.Fg3, MaxLines: 3, Align: text.Middle}).Layout(gtx, e.Th, currentTrackProof(e))
			}),
			layout.Rigid(layout.Spacer{Height: e.Th.Sp.S3}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				dims := p.retry.Layout(gtx, e.Th, lithic.Line, lithic.Icon("refresh"), "Retry lookup")
				if p.retry.Clicked(gtx) {
					p.Refresh(e)
				}
				return dims
			}),
		)
	})
}

func currentTrackProof(e *env.Env) string {
	now := e.Store.Snapshot().Playback.Now
	if now == nil {
		return "Current track · Unknown artist"
	}
	title, artists := strings.TrimSpace(now.Title), strings.TrimSpace(now.Artists)
	if title == "" {
		title = "Current track"
	}
	if artists == "" {
		artists = "Unknown artist"
	}
	return title + " · " + artists
}

func centeredMessage(gtx layout.Context, th *lithic.Theme, title, body string) layout.Dimensions {
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(420))
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(lithic.Text(th, th.T.Title, th.R.Fg, title)),
			layout.Rigid(layout.Spacer{Height: th.Sp.S2}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return (lithic.Label{Style: th.T.Body, Color: th.R.Fg3, MaxLines: 4, Align: text.Middle}).Layout(gtx, th, body)
			}),
		)
	})
}

func (p *LyricsPanel) layoutPlain(gtx layout.Context, e *env.Env) layout.Dimensions {
	inset := layout.Inset{Top: e.Th.Sp.S4, Right: e.Th.Sp.S4, Bottom: e.Th.Sp.S4, Left: e.Th.Sp.S4}
	return inset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return p.list.Layout(gtx, e.Th, len(p.lyrics.Lines), func(gtx layout.Context, index int) layout.Dimensions {
					line := p.lyrics.Lines[index]
					return layout.Inset{Bottom: e.Th.Sp.S3}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layoutPlainLyricLine(gtx, e.Th, line, p.showRomanized)
					})
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return sourceLine(gtx, e.Th, p.lyrics.Source) }),
		)
	})
}

func layoutPlainLyricLine(gtx layout.Context, th *lithic.Theme, line model.LyricsLine, showRomanized bool) layout.Dimensions {
	children := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return (lithic.Label{Style: th.T.Body, Color: th.R.Fg, MaxLines: 0}).Layout(gtx, th, line.Text)
		}),
	}
	if showRomanized && line.Romanized != nil && strings.TrimSpace(*line.Romanized) != "" {
		children = append(children,
			layout.Rigid(layout.Spacer{Height: th.Sp.S1}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return (lithic.Label{Style: th.T.Caption, Color: th.R.Fg, MaxLines: 0}).Layout(gtx, th, *line.Romanized)
			}),
		)
	}
	if line.Translation != nil && strings.TrimSpace(*line.Translation) != "" {
		children = append(children,
			layout.Rigid(layout.Spacer{Height: th.Sp.S1}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return (lithic.Label{Style: th.T.Caption, Color: th.R.Fg, MaxLines: 0}).Layout(gtx, th, *line.Translation)
			}),
		)
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

func (p *LyricsPanel) layoutSynced(gtx layout.Context, e *env.Env) layout.Dimensions {
	viewport := gtx.Constraints.Max.Y
	rowHeight := max(gtx.Dp(88), viewport/5)
	header := int(float32(viewport) * .34)
	if p.lineClicks == nil {
		p.lineClicks = make(map[string]*lithic.Click)
	}
	manual := gtx.Now.Before(p.manualUntil)
	target := p.targetScroll(header, rowHeight, viewport)
	if !manual && p.active >= 0 {
		if !p.scrollReady {
			p.scroll.Duration = 0
			p.scroll.Set(gtx, target)
			p.scrollReady = true
		} else {
			p.scroll.Duration = e.Th.Motion.Select
			p.scroll.Ease = lithic.EaseChisel
			p.scroll.Set(gtx, target)
		}
		y := p.scroll.Value(gtx)
		p.list.Position = lyricsPosition(y, header, rowHeight)
		p.lastScrollY = y
	}
	beforeY := lyricsScrollY(p.list.Position, header, rowHeight)
	count := len(p.lyrics.Lines) + 2
	dims := p.list.Layout(gtx, e.Th, count, func(gtx layout.Context, index int) layout.Dimensions {
		if index == 0 || index == count-1 {
			return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, header)}
		}
		lineIndex := index - 1
		line := p.lyrics.Lines[lineIndex]
		row := gtx
		row.Constraints.Min.Y = rowHeight
		row.Constraints.Max.Y = rowHeight
		key := fmt.Sprintf("%d:%d", valueOrZero(line.TimeMS), lineIndex)
		click := p.lineClicks[key]
		if click == nil {
			click = new(lithic.Click)
			p.lineClicks[key] = click
		}
		dims := click.Layout(row, func(gtx layout.Context) layout.Dimensions {
			return p.layoutLine(gtx, e, line, lineIndex == p.active, false)
		})
		if click.Clicked(gtx) && line.TimeMS != nil {
			p.seek(e, float64(int64(*line.TimeMS)+p.offsetMS)/1000)
			p.manualUntil = time.Time{}
		}
		return dims
	})
	afterY := lyricsScrollY(p.list.Position, header, rowHeight)
	if math.Abs(float64(afterY-beforeY)) > 1 && math.Abs(float64(afterY-p.lastScrollY)) > 1 {
		p.manualUntil = gtx.Now.Add(lyricsManualHold)
		p.lastScrollY = afterY
		p.scroll.Duration = e.Th.Motion.Select
		p.scroll.Ease = lithic.EaseChisel
		p.scroll.Set(gtx, afterY)
	}
	if strings.TrimSpace(p.lyrics.Source) != "" {
		sourceGTX := gtx
		sourceGTX.Constraints.Min = image.Point{}
		sourceGTX.Constraints.Max.X = max(0, dims.Size.X-gtx.Dp(26))
		offset := op.Offset(image.Pt(gtx.Dp(13), max(0, dims.Size.Y-gtx.Dp(20)))).Push(gtx.Ops)
		sourceLine(sourceGTX, e.Th, p.lyrics.Source)
		offset.Pop()
	}
	if gtx.Now.Before(p.manualUntil) {
		buttonGTX := gtx
		buttonGTX.Constraints.Min = image.Point{}
		buttonGTX.Constraints.Max.X = min(buttonGTX.Constraints.Max.X, gtx.Dp(160))
		offset := op.Offset(image.Pt(max(0, dims.Size.X-gtx.Dp(174)), max(0, dims.Size.Y-gtx.Dp(48)))).Push(gtx.Ops)
		p.current.Small = true
		p.current.Layout(buttonGTX, e.Th, lithic.Quiet, lithic.Icon("lyrics"), "Current line")
		if p.current.Clicked(buttonGTX) {
			p.manualUntil = time.Time{}
		}
		offset.Pop()
		gtx.Execute(op.InvalidateCmd{At: p.manualUntil})
	}
	return dims
}

func (p *LyricsPanel) targetScroll(header, rowHeight, viewport int) float32 {
	lineCenter := header + p.active*rowHeight + rowHeight/2
	return float32(max(0, lineCenter-viewport/2))
}

func lyricsPosition(scrollY float32, header, rowHeight int) layout.Position {
	y := max(0, int(math.Round(float64(scrollY))))
	if y < header {
		return layout.Position{First: 0, Offset: y}
	}
	y -= header
	return layout.Position{First: 1 + y/rowHeight, Offset: y % rowHeight}
}

func lyricsScrollY(position layout.Position, header, rowHeight int) float32 {
	if position.First <= 0 {
		return float32(position.Offset)
	}
	return float32(header + (position.First-1)*rowHeight + position.Offset)
}

func (p *LyricsPanel) layoutLine(gtx layout.Context, e *env.Env, line model.LyricsLine, active, compact bool) layout.Dimensions {
	color := e.Th.R.Fg3
	style := e.Th.T.Body
	if active {
		color = e.Th.R.Fg
		style = e.Th.T.Title
		style.Face.Weight = font.Bold
	}
	if compact {
		style = e.Th.T.Body
		if active {
			style = e.Th.T.Row
			style.Face.Weight = font.Bold
		}
	}
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(760))
		children := make([]layout.FlexChild, 0, 5)
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if active && len(line.Words) > 0 {
				return p.layoutWords(gtx, e, style, line.Words)
			}
			return (lithic.Label{Style: style, Color: color, MaxLines: 2, Align: text.Middle}).Layout(gtx, e.Th, line.Text)
		}))
		if p.showRomanized && line.Romanized != nil && strings.TrimSpace(*line.Romanized) != "" {
			children = append(children,
				layout.Rigid(layout.Spacer{Height: e.Th.Sp.S1}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return (lithic.Label{Style: e.Th.T.Caption, Color: color, MaxLines: 2, Align: text.Middle}).Layout(gtx, e.Th, *line.Romanized)
				}),
			)
		}
		if line.Translation != nil && strings.TrimSpace(*line.Translation) != "" {
			children = append(children,
				layout.Rigid(layout.Spacer{Height: e.Th.Sp.S1}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return (lithic.Label{Style: e.Th.T.Caption, Color: color, MaxLines: 2, Align: text.Middle}).Layout(gtx, e.Th, *line.Translation)
				}),
			)
		}
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx, children...)
	})
}

func (p *LyricsPanel) layoutWords(gtx layout.Context, e *env.Env, style lithic.TextStyle, words []model.LyricsWord) layout.Dimensions {
	active := activeLyricWord(words, p.positionMS(gtx, e))
	children := make([]layout.FlexChild, 0, len(words))
	for i := range words {
		index := i
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			word := words[index].Text
			if index+1 < len(words) {
				word += " "
			}
			if index != active {
				return lithic.Text(e.Th, style, e.Th.R.Fg, word)(gtx)
			}
			macro := op.Record(gtx.Ops)
			dims := layout.Inset{Top: 1, Right: 3, Bottom: 1, Left: 3}.Layout(gtx, lithic.Text(e.Th, style, e.Th.R.OnSel, word))
			call := macro.Stop()
			lithic.FillCut(gtx.Ops, image.Rectangle{Max: dims.Size}, gtx.Dp(e.Th.Cut.Token), e.Th.R.Sel)
			call.Add(gtx.Ops)
			return dims
		}))
	}
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
}

func (p *LyricsPanel) layoutCompact(gtx layout.Context, e *env.Env) layout.Dimensions {
	state := e.Store.Snapshot()
	if state.Playback.Now == nil {
		return centeredMessage(gtx, e.Th, "Current track", "Waiting for metadata")
	}
	if p.lyrics == nil {
		if p.loading {
			return centeredMessage(gtx, e.Th, "Resolving lyrics", "Auto follow")
		}
		return centeredMessage(gtx, e.Th, "No lyrics", "Nothing found for this track.")
	}
	if p.lyrics.Instrumental {
		return centeredMessage(gtx, e.Th, "Instrumental", "No vocal lines to follow.")
	}
	if !p.lyrics.Synced || len(p.lyrics.Lines) == 0 {
		return centeredMessage(gtx, e.Th, "No lyrics", "Nothing found for this track.")
	}
	center := p.active
	if center < 0 {
		center = 0
	}
	return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return p.compactLine(gtx, e, center-1) }),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return p.compactLine(gtx, e, center) }),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return p.compactLine(gtx, e, center+1) }),
	)
}

func (p *LyricsPanel) compactLine(gtx layout.Context, e *env.Env, index int) layout.Dimensions {
	if index < 0 || p.lyrics == nil || index >= len(p.lyrics.Lines) {
		return layout.Dimensions{Size: gtx.Constraints.Min}
	}
	line := p.lyrics.Lines[index]
	key := fmt.Sprintf("compact:%d:%d", valueOrZero(line.TimeMS), index)
	if p.lineClicks == nil {
		p.lineClicks = make(map[string]*lithic.Click)
	}
	click := p.lineClicks[key]
	if click == nil {
		click = new(lithic.Click)
		p.lineClicks[key] = click
	}
	dims := click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return p.layoutLine(gtx, e, line, index == p.active, true)
	})
	if click.Clicked(gtx) && line.TimeMS != nil {
		p.seek(e, float64(int64(*line.TimeMS)+p.offsetMS)/1000)
	}
	return dims
}

func (p *LyricsPanel) seek(e *env.Env, position float64) {
	position = max(0, position)
	act.Call[protocol.Null](e, protocol.MethodSeek, protocol.SeekParams{Position: position}, func(_ protocol.Null, err error) {
		if err != nil && e.Toast != nil {
			e.Toast(err.Error(), env.ToastError)
		}
	})
}

func sourceLine(gtx layout.Context, th *lithic.Theme, source string) layout.Dimensions {
	if strings.TrimSpace(source) == "" {
		return layout.Dimensions{}
	}
	return lithic.Text(th, th.T.MonoSm, th.R.Fg3, "Source · "+source)(gtx)
}

func valueOrZero(value *uint64) uint64 {
	if value == nil {
		return 0
	}
	return *value
}

var _ env.Refresher = (*LyricsPanel)(nil)
