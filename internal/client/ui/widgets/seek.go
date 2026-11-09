package widgets

import (
	"math"
	"strconv"
	"strings"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
	"github.com/ryoku-dev/ryotunes/internal/client/state"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

// seekSafetyTimeout releases a held seek if the daemon never reports reaching it.
const seekSafetyTimeout = 3 * time.Second

// Seek is the elapsed time, the seek scrubber and the duration in one row.
// Dragging previews the position; releasing commits it and holds that value
// until the daemon reports past it, so the handle never jumps back.
type Seek struct {
	scrub    look.Scrubber
	trackID  string
	pending  bool
	dragging bool
	target   float64
	origin   float64
	until    time.Time
}

// Layout draws the row for the loaded track; radio shows the Live claim.
func (s *Seek) Layout(gtx layout.Context, e *env.Env, snapshot state.State, track model.Track) layout.Dimensions {
	s.Reconcile(gtx, e, snapshot, track.VideoID)
	if model.ProviderOf(track.VideoID) == model.ProviderRadio {
		return layout.Center.Layout(gtx, kit.Claim(e.Th.Theme, palette.Alive, "Live"))
	}
	duration := PlaybackDuration(snapshot.Playback.Duration, track.Duration)
	known := duration > 0
	position, end, value := 0.0, "--:--", float32(0)
	if known {
		position = e.Store.PositionAt(gtx.Now)
		end = Duration(duration)
		value = float32(min(max(position/duration, 0), 1))
	}
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(seekTime(e, Duration(position), text.End)),
		layout.Rigid(layout.Spacer{Width: 8}.Layout),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return s.layoutScrubber(gtx, e, snapshot, value, duration, known)
		}),
		layout.Rigid(layout.Spacer{Width: 8}.Layout),
		layout.Rigid(seekTime(e, end, text.Start)),
	)
}

func (s *Seek) layoutScrubber(gtx layout.Context, e *env.Env, snapshot state.State, value float32, duration float64, known bool) layout.Dimensions {
	fill := e.Th.R.Fg2
	if !snapshot.Playback.Paused {
		fill = e.Th.Accent.Ink
	}
	scrubGTX := gtx
	if !known {
		scrubGTX = gtx.Disabled()
	}
	dimensions := s.scrub.Layout(scrubGTX, e.Th, value, look.ScrubberStyle{
		Fill: fill,
		Bubble: func(fraction float32) string {
			if !known {
				return "--:--"
			}
			return Duration(float64(fraction) * duration)
		},
		WheelStep: float32(min(1, 5/max(1, duration))),
		Wave:      e.Fx.Wave,
		Live:      !snapshot.Playback.Paused,
	})
	if !known {
		if s.dragging {
			e.Store.SetSeekDrag(nil)
		}
		s.dragging = false
		return dimensions
	}
	fraction, dragging := s.scrub.Dragging()
	if dragging {
		preview := float64(fraction) * duration
		e.Store.SetSeekDrag(&preview)
	}
	if committed, ok := s.scrub.Committed(); ok {
		s.commit(gtx, e, snapshot, float64(committed)*duration, duration)
	} else if s.dragging && !dragging {
		e.Store.SetSeekDrag(nil)
	}
	s.dragging = dragging
	if !e.Fx.Wave || dragging {
		ProgressFrame(gtx, e, duration, max(1, dimensions.Size.X), snapshot.Playback.Paused)
	}
	return dimensions
}

func (s *Seek) commit(gtx layout.Context, e *env.Env, snapshot state.State, target, duration float64) {
	target = min(max(target, 0), duration)
	s.pending, s.target, s.origin = true, target, snapshot.Playback.Position
	s.until = gtx.Now.Add(seekSafetyTimeout)
	e.Store.SetSeekDrag(&target)
	gtx.Execute(op.InvalidateCmd{At: s.until})
	transportCall(e, protocol.MethodSeek, protocol.SeekParams{Position: target})
}

// Reconcile releases a held seek once the daemon reports past it, its safety
// timeout passes, or the track changes. Surfaces that keep a Seek but do not
// lay it out this frame (a compact player) call it so the hold still ends.
func (s *Seek) Reconcile(gtx layout.Context, e *env.Env, snapshot state.State, trackID string) {
	if trackID != s.trackID {
		s.trackID, s.pending = trackID, false
		e.Store.SetSeekDrag(nil)
		return
	}
	if !s.pending {
		return
	}
	position := snapshot.Playback.Position
	passed := math.Abs(position-s.target) <= .5
	if s.target > s.origin {
		passed = passed || position >= s.target
	} else if s.target < s.origin {
		passed = passed || position <= s.target
	}
	if passed || !gtx.Now.Before(s.until) {
		s.pending = false
		e.Store.SetSeekDrag(nil)
	}
}

// ProgressFrame asks for the next frame when a flat progress fill would move
// by one pixel or the elapsed second changes, and never while paused.
func ProgressFrame(gtx layout.Context, e *env.Env, duration float64, trackPixels int, paused bool) {
	if paused || duration <= 0 || trackPixels <= 0 {
		return
	}
	pixelDelay := max(time.Duration(duration/float64(trackPixels)*float64(time.Second)), 250*time.Millisecond)
	position := e.Store.PositionAt(gtx.Now)
	secondDelay := time.Duration((1 - (position - math.Floor(position))) * float64(time.Second))
	if secondDelay <= 0 {
		secondDelay = time.Second
	}
	gtx.Execute(op.InvalidateCmd{At: gtx.Now.Add(min(pixelDelay, secondDelay))})
}

// PlaybackDuration is the daemon's duration, or the track's "m:ss" length
// until the daemon knows it (a restored track reports 0 until it plays).
func PlaybackDuration(reported float64, value *string) float64 {
	if reported > 0 && !math.IsNaN(reported) && !math.IsInf(reported, 0) {
		return reported
	}
	if value == nil {
		return 0
	}
	parts := strings.Split(strings.TrimSpace(*value), ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0
	}
	seconds := 0
	for index, part := range parts {
		component, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || component < 0 || (index > 0 && component >= 60) {
			return 0
		}
		seconds = seconds*60 + component
	}
	return float64(seconds)
}

func seekTime(e *env.Env, value string, align text.Alignment) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		width := min(gtx.Dp(unit.Dp(44)), max(0, gtx.Constraints.Max.X))
		gtx.Constraints.Min.X, gtx.Constraints.Max.X = width, width
		return kit.Label{Style: e.Th.T.Mono, Color: e.Th.R.Fg3, MaxLines: 1, Align: align}.Layout(gtx, e.Th.Theme, value)
	}
}
