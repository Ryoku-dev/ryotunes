// Package chrome implements the persistent playback surfaces shared by Ryotunes windows.
package chrome

import (
	"image"
	"math"
	"strings"
	"time"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/text"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/anim"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
	libtheme "github.com/ryoku-dev/ryoku/docs/lithic/library/go/theme"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/widgets"
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

type lyricLineKey struct {
	size   LyricsSize
	timeMS uint64
	index  int
}

type lyricFollowPlan struct {
	anchor       bool
	anchorIndex  int
	anchorOffset int
	springDelta  int
}

type lyricLineMotion struct {
	current, previous        int
	currentValue, priorValue float32
	currentTween, priorTween anim.Tween
	ready                    bool
}

// Lyrics resolves, caches, and follows the current track's lyric document.
type Lyrics struct {
	scroll      look.Scroller
	lineClicks  map[lyricLineKey]*kit.Click
	follow      kit.Plaque
	retry       kit.Plaque
	lyrics      *model.Lyrics
	timed       []int
	words       [][]look.KaraokeWord
	karaoke     []look.Karaoke
	rowTops     []int
	rowSeen     []uint64
	laidRows    []int
	trackID     string
	requestedID uint64
	revision    uint64
	loading     bool
	stale       bool
	err         string
	offsetMS    int64
	active      int
	lastActive  int
	lineMotion  lyricLineMotion
	manualUntil time.Time
	lastTarget  int
	romanized   bool
	forceAnchor bool
	generation  uint64
	clockAt     time.Time
	clockMS     int64
	clockSpeed  float64
	clockReady  bool
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

// Layout draws lyrics in the typography and follow mode of their host, the
// document inside outer and the host's inset; an empty state takes the area.
func (lyrics *Lyrics) Layout(gtx layout.Context, e *env.Env, size LyricsSize, outer layout.Inset) layout.Dimensions {
	lyrics.ensure(gtx, e)
	lyrics.romanized = settingEnabled(e.Store.Snapshot().Settings, model.SettingLyricsRomanize, false)
	var dims layout.Dimensions
	if size == LyricsCompact {
		dims = lyrics.layoutCompact(gtx, e)
	} else if state, shown := lyrics.layoutState(gtx, e); shown {
		dims = state
	} else {
		inset := layout.Inset{Top: 13, Bottom: 13}
		if size == LyricsPanel {
			inset.Left, inset.Right = 13, 13
		} else {
			inset.Left, inset.Right = 21, 21
		}
		dims = outer.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return inset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return lyrics.layoutDocument(gtx, e, size)
			})
		})
	}
	lyrics.scheduleBoundary(gtx, e, size)
	return dims
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
		lyrics.words = nil
		lyrics.karaoke = nil
		lyrics.lineClicks = nil
		lyrics.rowTops = lyrics.rowTops[:0]
		lyrics.rowSeen = lyrics.rowSeen[:0]
		lyrics.laidRows = lyrics.laidRows[:0]
		lyrics.offsetMS = 0
		lyrics.active = -1
		lyrics.lastActive = -2
		lyrics.lineMotion = lyricLineMotion{}
		lyrics.manualUntil = time.Time{}
		lyrics.lastTarget = 0
		lyrics.forceAnchor = true
		lyrics.clockReady = false
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
		positionMS := lyrics.positionMS(gtx, e)
		speed := snapshot.AudioFx.Speed
		if speed <= 0 {
			speed = 1
		}
		if lyrics.clockDiscontinuity(gtx.Now, positionMS, speed, snapshot.Playback.Paused) {
			lyrics.forceAnchor = true
		}
		lyrics.active = activeLyricLine(lyrics.lyrics.Lines, lyrics.timed, positionMS)
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
			lyrics.words = nil
			lyrics.karaoke = nil
			lyrics.timed = lyrics.timed[:0]
			lineCount := 0
			if document != nil {
				lineCount = len(document.Lines)
				for index := range document.Lines {
					if document.Lines[index].TimeMS != nil {
						lyrics.timed = append(lyrics.timed, index)
					}
				}
			}
			if document != nil && document.Synced {
				lyrics.words = look.PrepareKaraoke(document.Lines)
				lyrics.karaoke = make([]look.Karaoke, lineCount)
			}
			lyrics.rowTops = make([]int, lineCount)
			lyrics.rowSeen = make([]uint64, lineCount)
			lyrics.laidRows = make([]int, 0, min(lineCount, 16))
			lyrics.generation = 0
			lyrics.forceAnchor = true
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

func (lyrics *Lyrics) clockDiscontinuity(now time.Time, positionMS int64, speed float64, paused bool) bool {
	if !lyrics.clockReady {
		lyrics.clockAt = now
		lyrics.clockMS = positionMS
		lyrics.clockSpeed = speed
		lyrics.clockReady = true
		return true
	}
	expected := lyrics.clockMS
	if !paused && now.After(lyrics.clockAt) {
		expected += int64(float64(now.Sub(lyrics.clockAt).Milliseconds()) * lyrics.clockSpeed)
	}
	speedChanged := math.Abs(speed-lyrics.clockSpeed) > .001
	jumped := math.Abs(float64(positionMS-expected)) > 500
	lyrics.clockAt = now
	lyrics.clockMS = positionMS
	lyrics.clockSpeed = speed
	return speedChanged || jumped
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

func (lyrics *Lyrics) scheduleBoundary(gtx layout.Context, e *env.Env, size LyricsSize) {
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
	visible := size == LyricsCompact ||
		lyrics.active >= 0 && lyrics.active < len(lyrics.rowSeen) && lyrics.rowSeen[lyrics.active] == lyrics.generation
	wiping := false
	if lyrics.active >= 0 && lyrics.active < len(lyrics.lyrics.Lines) {
		line := lyrics.lyrics.Lines[lyrics.active]
		if line.EndTimeMS != nil && int64(*line.EndTimeMS) > positionMS {
			nextMS = min(nextMS, int64(*line.EndTimeMS))
		}
		if snapshot.Client.WordByWordOn() && lyrics.active < len(lyrics.words) {
			for _, word := range lyrics.words[lyrics.active] {
				if int64(word.StartMS) > positionMS {
					nextMS = min(nextMS, int64(word.StartMS))
					break
				}
				if int64(word.EndMS) > positionMS {
					nextMS = min(nextMS, int64(word.EndMS))
					wiping = visible && !anim.Reduced()
					break
				}
			}
		}
	}
	speed := snapshot.AudioFx.Speed
	if speed <= 0 {
		speed = 1
	}
	delay := time.Duration(0)
	if nextMS != noBoundary {
		delay = time.Duration(float64(nextMS-positionMS)/speed) * time.Millisecond
	}
	if wiping && (delay <= 0 || delay > time.Second/30) {
		delay = time.Second / 30
	}
	if delay <= 0 {
		return
	}
	if delay < time.Millisecond {
		delay = time.Millisecond
	}
	gtx.Execute(op.InvalidateCmd{At: gtx.Now.Add(delay)})
}

// layoutState draws the empty or failed state, which owns its spacing, in the
// whole area, and reports whether one applied.
func (lyrics *Lyrics) layoutState(gtx layout.Context, e *env.Env) (layout.Dimensions, bool) {
	switch {
	case e.Store.Snapshot().Playback.Now == nil:
	case lyrics.lyrics == nil && lyrics.loading:
		return layout.Dimensions{}, false
	case lyrics.lyrics == nil && lyrics.err != "":
		lyrics.retry.Kind = kit.PlaqueLine
		lyrics.retry.Icon = look.Icon.Refresh
		lyrics.retry.Label = "Try again"
		dims := widgets.EmptyState(e, look.Icon.Lyrics, "Lyrics could not be loaded", lyrics.err, &lyrics.retry)(gtx)
		if lyrics.retry.Clicked(gtx) {
			lyrics.Refresh(e)
		}
		return dims, true
	case lyrics.lyrics == nil:
	case lyrics.lyrics.Instrumental:
		return widgets.EmptyState(e, look.Icon.Lyrics, "No lyrics for this track", "This recording is instrumental.")(gtx), true
	case len(lyrics.lyrics.Lines) == 0:
	default:
		return layout.Dimensions{}, false
	}
	return widgets.EmptyState(e, look.Icon.Lyrics, "No lyrics for this track", "")(gtx), true
}

func (lyrics *Lyrics) layoutDocument(gtx layout.Context, e *env.Env, size LyricsSize) layout.Dimensions {
	if lyrics.lyrics == nil {
		return lyricsCentered(gtx, e, "Resolving lyrics", lyricsTrackProof(e))
	}
	source := strings.TrimSpace(lyrics.lyrics.Source)
	if source == "" {
		return lyrics.layoutLines(gtx, e, size)
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return lyrics.layoutLines(gtx, e, size)
		}),
		layout.Rigid(layout.Spacer{Height: 13}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			height := gtx.Dp(18)
			gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
			return layout.Center.Layout(gtx, kit.Text(e.Th.Theme, e.Th.T.Caption, e.Th.R.Fg3, "Lyrics from "+source))
		}),
	)
}

func (lyrics *Lyrics) layoutLines(gtx layout.Context, e *env.Env, size LyricsSize) layout.Dimensions {
	if lyrics.lyrics.Synced {
		return lyrics.layoutSynced(gtx, e, size)
	}
	return lyrics.layoutPlain(gtx, e, size)
}

func (lyrics *Lyrics) layoutPlain(gtx layout.Context, e *env.Env, size LyricsSize) layout.Dimensions {
	return lyrics.scroll.Layout(gtx, e.Th, len(lyrics.lyrics.Lines), func(gtx layout.Context, index int) layout.Dimensions {
		line := lyrics.lyrics.Lines[index]
		return layout.Inset{Top: 8, Bottom: 13}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return lyrics.layoutLine(gtx, e, line, index, false, size)
		})
	})
}

func (lyrics *Lyrics) layoutSynced(gtx layout.Context, e *env.Env, size LyricsSize) layout.Dimensions {
	lyrics.updateLineMotion(gtx, e.Th.Motion.Field)
	viewport := gtx.Constraints.Max.Y
	rowHeight := gtx.Dp(88)
	if size == LyricsStage {
		rowHeight = gtx.Dp(108)
	}
	topPad := max(gtx.Dp(13), viewport/3-rowHeight/2)
	bottomPad := max(gtx.Dp(13), viewport-topPad-rowHeight)
	lyrics.scroll.UpdateInput(gtx)
	if lyrics.scroll.UserScrolled() {
		lyrics.manualUntil = lyricManualDeadline(gtx.Now, lyrics.manualUntil, true)
		lyrics.lastActive = -2
		lyrics.forceAnchor = true
		gtx.Execute(op.InvalidateCmd{At: lyrics.manualUntil})
	}
	manual := gtx.Now.Before(lyrics.manualUntil)
	activeLaid := lyrics.generation != 0 &&
		lyrics.active >= 0 && lyrics.active < len(lyrics.rowSeen) &&
		lyrics.rowSeen[lyrics.active] == lyrics.generation
	activeTop := 0
	if activeLaid {
		activeTop = lyrics.rowTops[lyrics.active]
	}
	if !manual && lyrics.active >= 0 &&
		(lyrics.forceAnchor || !activeLaid || lyrics.active != lyrics.lastActive || lyrics.lastTarget != viewport) {
		desiredTop := max(0, viewport/3-rowHeight/2)
		plan := planLyricsFollow(lyrics.active, activeTop, desiredTop, activeLaid, lyrics.forceAnchor)
		if plan.anchor {
			lyrics.scroll.AnchorTo(plan.anchorIndex, plan.anchorOffset)
		} else if plan.springDelta != 0 {
			lyrics.scroll.SpringBy(plan.springDelta)
		}
		lyrics.lastActive = lyrics.active
		lyrics.lastTarget = viewport
		lyrics.forceAnchor = false
	}

	lyrics.generation++
	generation := lyrics.generation
	lyrics.laidRows = lyrics.laidRows[:0]
	count := len(lyrics.lyrics.Lines) + 2
	dims := lyrics.scroll.Layout(gtx, e.Th, count, func(gtx layout.Context, index int) layout.Dimensions {
		if index == 0 {
			return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, topPad)}
		}
		if index == count-1 {
			return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, bottomPad)}
		}
		lineIndex := index - 1
		rowGTX := gtx
		rowGTX.Constraints.Min.Y = rowHeight
		rowGTX.Constraints.Max.Y = rowHeight
		lyrics.rowSeen[lineIndex] = generation
		lyrics.laidRows = append(lyrics.laidRows, lineIndex)
		line := lyrics.lyrics.Lines[lineIndex]
		click := lyrics.lineClick(line, lineIndex, size)
		dims := click.Layout(rowGTX, func(gtx layout.Context) layout.Dimensions {
			return lyrics.layoutLine(gtx, e, line, lineIndex, true, size)
		})
		if click.Clicked(gtx) && line.TimeMS != nil {
			lyrics.seek(e, float64(int64(*line.TimeMS)+lyrics.offsetMS)/1000)
			lyrics.manualUntil = time.Time{}
			lyrics.forceAnchor = true
		}
		return dims
	})
	first, offset := lyrics.scroll.Offset()
	absolute := lyricsAbsolute(first, offset, topPad, rowHeight)
	for _, index := range lyrics.laidRows {
		lyrics.rowTops[index] = topPad + index*rowHeight - absolute
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
			lyrics.forceAnchor = true
		}
		offset.Pop()
	}
	return dims
}

func planLyricsFollow(active, activeTop, desiredTop int, laid, forceAnchor bool) lyricFollowPlan {
	if forceAnchor || !laid {
		return lyricFollowPlan{anchor: true, anchorIndex: active + 1, anchorOffset: desiredTop}
	}
	return lyricFollowPlan{springDelta: activeTop - desiredTop}
}

func lyricManualDeadline(now, current time.Time, userScrolled bool) time.Time {
	if userScrolled {
		return now.Add(lyricsManualHold)
	}
	return current
}

func lyricsAbsolute(first, offset, topPad, rowHeight int) int {
	if first <= 0 {
		return offset
	}
	return topPad + (first-1)*rowHeight + offset
}

func (lyrics *Lyrics) lineClick(line model.LyricsLine, index int, size LyricsSize) *kit.Click {
	if lyrics.lineClicks == nil {
		lyrics.lineClicks = make(map[lyricLineKey]*kit.Click)
	}
	key := lyricLineKey{size: size, timeMS: valueOrZero(line.TimeMS), index: index}
	click := lyrics.lineClicks[key]
	if click == nil {
		click = new(kit.Click)
		lyrics.lineClicks[key] = click
	}
	return click
}

func (lyrics *Lyrics) layoutLine(gtx layout.Context, e *env.Env, line model.LyricsLine, index int, synced bool, size LyricsSize) layout.Dimensions {
	activeStyle, restingStyle := lyricStyles(e, size)
	progress := float32(0)
	if synced {
		progress = lyrics.lineProgress(index)
	} else {
		activeStyle = restingStyle
	}
	wordByWord := synced && e.Store.Snapshot().Client.WordByWordOn()
	targetInk := e.Th.R.Fg
	if wordByWord {
		targetInk = e.Th.R.Fg2
	}
	ink := palette.Mix(e.Th.R.Fg3, targetInk, progress)
	highlight := palette.Mix(e.Th.R.Fg3, e.Th.Accent.Ink, progress)
	captionInk := palette.Mix(e.Th.R.Fg3, e.Th.R.Fg2, progress)

	render := func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if wordByWord && index >= 0 && index < len(lyrics.words) && len(lyrics.words[index]) > 0 {
					lift := unit.Dp(2)
					preferredRows, maximumRows, minimumScale := 2, 2, float32(.72)
					if size == LyricsStage {
						lift = 3
					} else if size == LyricsCompact {
						preferredRows, maximumRows, minimumScale = 1, 2, .8
					}
					result := lyrics.karaoke[index].Layout(gtx, e.Th, lyrics.words[index], look.KaraokeStyle{
						Key: lyrics.trackID, Text: activeStyle, Base: ink, Highlight: highlight,
						Align: text.Middle, PreferredRows: preferredRows, MaxRows: maximumRows,
						MinScale: minimumScale, Lift: unit.Dp(float32(lift) * progress),
						PositionMS: lyrics.positionMS(gtx, e),
					})
					return result.Dimensions
				}
				return (kit.Label{Style: activeStyle, Color: ink, MaxLines: 2, Align: text.Middle}).Layout(gtx, e.Th.Theme, strings.TrimSpace(line.Text))
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if !lyrics.romanized || line.Romanized == nil || strings.TrimSpace(*line.Romanized) == "" {
					return layout.Dimensions{}
				}
				return layout.Inset{Top: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return (kit.Label{Style: e.Th.T.Caption, Color: captionInk, MaxLines: 2, Align: text.Middle}).Layout(gtx, e.Th.Theme, strings.TrimSpace(*line.Romanized))
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if line.Translation == nil || strings.TrimSpace(*line.Translation) == "" {
					return layout.Dimensions{}
				}
				return layout.Inset{Top: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return (kit.Label{Style: e.Th.T.Caption, Color: captionInk, MaxLines: 2, Align: text.Middle}).Layout(gtx, e.Th.Theme, strings.TrimSpace(*line.Translation))
				})
			}),
		)
	}

	maxWidth := gtx.Dp(560)
	if size == LyricsStage {
		maxWidth = gtx.Dp(760)
	}
	if !synced {
		return layout.Inset{Left: 13, Right: 13}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, maxWidth)
			return layout.Center.Layout(gtx, render)
		})
	}

	slot := gtx.Constraints.Max
	sideRoom := gtx.Dp(13)
	canvas := image.Pt(max(1, min(maxWidth, slot.X-2*sideRoom)), max(1, slot.Y))
	restingScale := float32(1)
	if activeStyle.Size > 0 {
		restingScale = float32(restingStyle.Size) / float32(activeStyle.Size)
	}
	if restingScale <= 0 || restingScale > 1 {
		restingScale = 1
	}
	restingWrap := index != lyrics.lineMotion.current && index != lyrics.lineMotion.previous
	if restingWrap && restingScale < 1 {
		canvas.X = int(math.Ceil(float64(float32(canvas.X) / restingScale)))
		canvas.Y = int(math.Ceil(float64(float32(canvas.Y) / restingScale)))
	}

	contentGTX := gtx
	contentGTX.Constraints.Min = canvas
	contentGTX.Constraints.Max = canvas
	record := op.Record(gtx.Ops)
	dims := layout.Center.Layout(contentGTX, render)
	call := record.Stop()

	scale := restingScale + (1-restingScale)*progress
	if anim.Reduced() {
		scale = 1
		if restingWrap {
			scale = restingScale
		}
	}
	mask := clip.Rect{Max: slot}.Push(gtx.Ops)
	move := op.Offset(image.Pt((slot.X-dims.Size.X)/2, (slot.Y-dims.Size.Y)/2)).Push(gtx.Ops)
	centre := f32.Pt(float32(dims.Size.X)/2, float32(dims.Size.Y)/2)
	transform := op.Affine(f32.AffineId().Scale(centre, f32.Pt(scale, scale))).Push(gtx.Ops)
	call.Add(gtx.Ops)
	transform.Pop()
	move.Pop()
	mask.Pop()
	return layout.Dimensions{Size: slot}
}

func lyricStyles(e *env.Env, size LyricsSize) (active, resting libtheme.TextStyle) {
	switch size {
	case LyricsStage:
		return e.Th.T.Sentence, e.Th.T.Section
	case LyricsCompact:
		return e.Th.T.Label, e.Th.T.Caption
	default:
		return e.Th.T.Section, e.Th.T.Proof
	}
}

func (lyrics *Lyrics) updateLineMotion(gtx layout.Context, duration time.Duration) {
	motion := &lyrics.lineMotion
	if !motion.ready {
		motion.current, motion.previous = lyrics.active, -1
		motion.currentTween = anim.Tween{Duration: duration, Ease: anim.EaseChisel}
		motion.currentTween.Set(gtx, 1)
		motion.currentValue, motion.priorValue, motion.ready = 1, 0, true
		return
	}
	if lyrics.active != motion.current {
		motion.previous = motion.current
		motion.priorTween = anim.Tween{Duration: duration, Ease: anim.EaseChisel}
		motion.priorTween.Set(gtx, motion.currentValue)
		motion.priorTween.Set(gtx, 0)
		motion.current = lyrics.active
		motion.currentTween = anim.Tween{Duration: duration, Ease: anim.EaseChisel}
		motion.currentTween.Set(gtx, 0)
		motion.currentTween.Set(gtx, 1)
	}
	motion.currentValue = motion.currentTween.Value(gtx)
	motion.priorValue = motion.priorTween.Value(gtx)
	if motion.priorValue <= 0 {
		motion.previous = -1
	}
}

func (lyrics *Lyrics) lineProgress(index int) float32 {
	if index == lyrics.lineMotion.current {
		return lyrics.lineMotion.currentValue
	}
	if index == lyrics.lineMotion.previous {
		return lyrics.lineMotion.priorValue
	}
	return 0
}

func (lyrics *Lyrics) layoutCompact(gtx layout.Context, e *env.Env) layout.Dimensions {
	if lyrics.lyrics == nil || lyrics.lyrics.Instrumental || !lyrics.lyrics.Synced || len(lyrics.lyrics.Lines) == 0 {
		// The mini stage is a small square, not a page: no content inset.
		empty := kit.Empty{Icon: look.Icon.Lyrics, Title: "No lyrics for this track"}
		return empty.Layout(gtx, e.Th.Theme)
	}
	lyrics.updateLineMotion(gtx, e.Th.Motion.Field)
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
		return lyrics.layoutLine(gtx, e, line, index, true, LyricsCompact)
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
