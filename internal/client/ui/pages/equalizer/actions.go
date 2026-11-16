package equalizer

import (
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"
	"time"

	"gioui.org/layout"
	"gioui.org/op"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/anim"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

const equalizerDragInterval = 80 * time.Millisecond

// Leave goes back to where the equalizer was opened, or to Settings, Sound
// when it is the first page the window shows and there is no way back.
func (p *Page) Leave(e *env.Env) {
	if e.Nav.CanBack() {
		e.Nav.Back()
		return
	}
	e.Nav.Replace(env.Route{Kind: env.RouteSettings, Title: "Settings", Params: "sound"})
}

func (p *Page) reconcile(e *env.Env) {
	current := e.Store.Snapshot().Equalizer
	if !p.initialized {
		p.working = cloneEqualizer(current)
		p.initialized = true
		p.bands = make([]bandControl, len(p.working.Bands))
		for index := range p.bands {
			p.bands[index].existing = true
		}
		return
	}
	if p.dragging || p.presetAnimating {
		return
	}
	if p.pending {
		if equalizerEqual(current, p.submitted) {
			p.pending = false
			p.working = cloneEqualizer(current)
		}
		return
	}
	if !p.dirty && !equalizerEqual(current, p.working) {
		p.working = cloneEqualizer(current)
	}
}

func (p *Page) loadPresets(e *env.Env) {
	if p.presetsLoaded || p.presetsLoading {
		return
	}
	p.presetsLoading = true
	act.Call[[]model.EQPreset](e, protocol.MethodListEqualizerPresets, protocol.Null{}, func(presets []model.EQPreset, err error) {
		p.presetsLoading = false
		if err != nil {
			if e.Toast != nil {
				e.Toast("Could not load equalizer presets", env.ToastError)
			}
			return
		}
		p.presets = append(p.presets[:0], presets...)
		p.presetsLoaded = true
	})
}

func (p *Page) submit(e *env.Env) {
	p.dirty = false
	p.pending = true
	p.submitted = cloneEqualizer(p.working)
	if p.requestInFlight {
		p.requestQueued = true
		return
	}
	p.dispatchSubmit(e)
}

func (p *Page) dispatchSubmit(e *env.Env) {
	value := cloneEqualizer(p.submitted)
	p.requestInFlight = true
	p.requestSeq++
	sequence := p.requestSeq
	act.Call[model.Equalizer](e, protocol.MethodSetEqualizer, value, func(accepted model.Equalizer, err error) {
		if sequence != p.requestSeq {
			return
		}
		p.requestInFlight = false
		if err != nil {
			if e.Toast != nil {
				e.Toast(err.Error(), env.ToastError)
			}
			if p.requestQueued {
				p.requestQueued = false
				p.dispatchSubmit(e)
				return
			}
			p.pending = false
			if !p.dragging && !p.dirty {
				p.working = cloneEqualizer(e.Store.Snapshot().Equalizer)
			}
			return
		}
		if p.requestQueued {
			p.requestQueued = false
			p.dispatchSubmit(e)
			return
		}
		p.submitted = cloneEqualizer(accepted)
		if !p.dragging && !p.dirty && !p.presetAnimating {
			p.working = cloneEqualizer(accepted)
		}
	})
}

func (p *Page) markEdit(gtx layout.Context, e *env.Env, dragging, release bool) {
	p.working.Preset = ""
	p.dirty = true
	p.dragging = dragging
	if release {
		p.dragging = false
		p.applyAt = time.Time{}
		p.submit(e)
		return
	}
	if !dragging {
		p.submit(e)
		return
	}
	if p.applyAt.IsZero() || !gtx.Now.Before(p.applyAt) {
		p.submit(e)
		p.applyAt = gtx.Now.Add(equalizerDragInterval)
	}
	gtx.Execute(op.InvalidateCmd{At: p.applyAt})
}

func (p *Page) flushDrag(gtx layout.Context, e *env.Env) {
	if !p.dragging || !p.dirty || p.applyAt.IsZero() {
		return
	}
	if gtx.Now.Before(p.applyAt) {
		gtx.Execute(op.InvalidateCmd{At: p.applyAt})
		return
	}
	p.submit(e)
	p.applyAt = gtx.Now.Add(equalizerDragInterval)
	gtx.Execute(op.InvalidateCmd{At: p.applyAt})
}

func presetEqualizer(preset model.EQPreset, enabled bool) model.Equalizer {
	return model.Equalizer{Enabled: enabled, Preamp: preset.Preamp, Bands: append([]model.EQBand(nil), preset.Bands...), Preset: preset.Name}
}

// animateTo moves the curve to target as the Hub moves a value it sets: 240
// ms (th.Motion.Move) on OutCubic, the readouts rolling once to the values
// it lands on.
func (p *Page) animateTo(gtx layout.Context, e *env.Env, target model.Equalizer) {
	p.presetFrom = cloneEqualizer(p.working)
	p.presetTarget = cloneEqualizer(target)
	p.presetTween = anim.Tween{Duration: e.Th.Motion.Move, Ease: anim.EaseOutCubic}
	p.presetTween.Set(gtx, 0)
	p.presetTween.Set(gtx, 1)
	p.presetAnimating = true
	p.pending = false
}

// settledEqualizer is the equalizer as it stands once any preset still
// gliding has landed: what an action changes from.
func (p *Page) settledEqualizer() model.Equalizer {
	if p.presetAnimating {
		return cloneEqualizer(p.presetTarget)
	}
	return cloneEqualizer(p.working)
}

func interpolateBand(from, target model.EQBand, progress float64) model.EQBand {
	result := target
	result.Frequency = from.Frequency + (target.Frequency-from.Frequency)*progress
	result.Gain = from.Gain + (target.Gain-from.Gain)*progress
	result.Q = from.Q + (target.Q-from.Q)*progress
	if progress < .5 {
		result.Type, result.Enabled = from.Type, from.Enabled
	}
	return result
}

func (p *Page) advancePreset(gtx layout.Context, e *env.Env) {
	if !p.presetAnimating {
		return
	}
	progress := float64(p.presetTween.Value(gtx))
	if progress >= 1 {
		p.working = cloneEqualizer(p.presetTarget)
		p.presetAnimating = false
		p.submit(e)
		return
	}
	from, target := p.presetFrom, p.presetTarget
	work := target
	work.Preamp = from.Preamp + (target.Preamp-from.Preamp)*progress
	count := max(len(from.Bands), len(target.Bands))
	// the frames in between reuse one buffer; the landing clones the target
	p.presetBands = slices.Grow(p.presetBands[:0], count)[:count]
	work.Bands = p.presetBands
	for index := range count {
		var first, second model.EQBand
		switch {
		case index < len(from.Bands) && index < len(target.Bands):
			first, second = from.Bands[index], target.Bands[index]
		case index < len(target.Bands):
			second = target.Bands[index]
			first = second
			first.Gain = 0
			first.Enabled = false
		case index < len(from.Bands):
			first = from.Bands[index]
			second = first
			second.Gain = 0
			second.Enabled = false
		}
		work.Bands[index] = interpolateBand(first, second, progress)
	}
	p.working = work
}

func (p *Page) resetTarget() model.Equalizer {
	for _, preset := range p.presets {
		if strings.EqualFold(preset.Name, "Flat") {
			return presetEqualizer(preset, p.working.Enabled)
		}
	}
	return model.Equalizer{Enabled: p.working.Enabled, Preset: "Flat"}
}

func widestGapFrequency(bands []model.EQBand) float64 {
	frequencies := make([]float64, 0, len(bands)+2)
	frequencies = append(frequencies, model.EQMinFrequency)
	for _, band := range bands {
		frequencies = append(frequencies, min(max(band.Frequency, model.EQMinFrequency), model.EQMaxFrequency))
	}
	frequencies = append(frequencies, model.EQMaxFrequency)
	sort.Float64s(frequencies)
	bestRatio, best := 0.0, 1000.0
	for index := 0; index+1 < len(frequencies); index++ {
		ratio := frequencies[index+1] / frequencies[index]
		if ratio > bestRatio {
			bestRatio = ratio
			best = math.Sqrt(frequencies[index] * frequencies[index+1])
		}
	}
	return math.Round(best)
}

func (p *Page) addBand(gtx layout.Context, e *env.Env) {
	if len(p.working.Bands) >= model.EQMaxBands {
		return
	}
	p.working.Bands = append(p.working.Bands, model.EQBand{Type: model.EQPeak, Frequency: widestGapFrequency(p.working.Bands), Q: 1, Enabled: true})
	p.graph.Select(len(p.working.Bands) - 1)
	p.markEdit(gtx, e, false, true)
}

func (p *Page) removeBand(gtx layout.Context, e *env.Env, index int) {
	if index < 0 || index >= len(p.working.Bands) || p.removing >= 0 {
		return
	}
	control := &p.bands[index]
	control.unfold.Duration = e.Th.Motion.HubUnfold
	control.unfold.Ease = anim.EaseOutCubic
	if !control.ready {
		control.unfold.Set(gtx, 1)
		control.ready = true
	}
	control.unfold.Set(gtx, 0)
	p.removing = index
	p.removeBefore = cloneEqualizer(p.working)
	gtx.Execute(op.InvalidateCmd{})
}

func (p *Page) finishRemoval(gtx layout.Context, e *env.Env) {
	if !p.removeComplete || p.removing < 0 || p.removing >= len(p.working.Bands) {
		return
	}
	index := p.removing
	p.working.Bands = append(p.working.Bands[:index], p.working.Bands[index+1:]...)
	p.bands = append(p.bands[:index], p.bands[index+1:]...)
	p.removing, p.removeComplete = -1, false
	if p.graph.Selected() >= len(p.working.Bands) {
		p.graph.Select(len(p.working.Bands) - 1)
	}
	p.markEdit(gtx, e, false, true)
	p.offerUndo(e, "Removed the band at "+hertz(p.removeBefore.Bands[index].Frequency)+".", equalizerChange{before: p.removeBefore, after: cloneEqualizer(p.working), removed: index})
}

// hertz writes a frequency for a sentence: "60 Hz", "1.2 kHz".
func hertz(frequency float64) string {
	value := look.FormatFrequency(frequency)
	if kilo, ok := strings.CutSuffix(value, "k"); ok {
		return kilo + " kHz"
	}
	return value + " Hz"
}

func (p *Page) lowerPreamp(gtx layout.Context, e *env.Env, peak float64) {
	before := cloneEqualizer(p.working)
	p.working.Preamp = max(model.EQMinPreamp, p.working.Preamp-peak)
	p.markEdit(gtx, e, false, true)
	p.offerUndo(e, fmt.Sprintf("Lowered the preamp by %.1f dB.", peak), equalizerChange{before: before, after: cloneEqualizer(p.working), removed: -1})
}

// equalizerChange is one action an Undo can take back: the equalizer before
// and after it, and the column of a band it removed (-1 for none).
type equalizerChange struct {
	before, after model.Equalizer
	removed       int
}

// revertChange takes back only what change did to current, leaving every
// edit made since: the preamp and the preset name go back where they still
// read as the change left them; a removed band returns to its column (while
// there is room); bands the change set go back where nobody touched them
// since, bands it dropped return and bands it added leave, as long as the
// band count is still the one it left.
func revertChange(current model.Equalizer, change equalizerChange) model.Equalizer {
	out := cloneEqualizer(current)
	if out.Preamp == change.after.Preamp {
		out.Preamp = change.before.Preamp
	}
	if out.Preset == change.after.Preset {
		out.Preset = change.before.Preset
	}
	if change.removed >= 0 {
		if change.removed < len(change.before.Bands) && len(out.Bands) < model.EQMaxBands {
			at := min(change.removed, len(out.Bands))
			out.Bands = slices.Insert(out.Bands, at, change.before.Bands[change.removed])
		}
		return out
	}
	before, after := change.before.Bands, change.after.Bands
	for index := range min(len(out.Bands), len(after), len(before)) {
		if out.Bands[index] == after[index] {
			out.Bands[index] = before[index]
		}
	}
	if len(out.Bands) == len(after) {
		switch {
		case len(before) > len(after):
			out.Bands = append(out.Bands, before[len(after):]...)
		case len(before) < len(after):
			keep := len(after)
			for keep > len(before) && out.Bands[keep-1] == after[keep-1] {
				keep--
			}
			out.Bands = out.Bands[:keep]
		}
	}
	return out
}

// offerUndo shows the change as a Hub notice (3.17) whose Undo takes it back
// at once, whether or not the page is showing. Without the notice host it
// says what happened as a plain toast.
func (p *Page) offerUndo(e *env.Env, message string, change equalizerChange) {
	if e.Undoable == nil {
		if e.Toast != nil {
			e.Toast(message, env.ToastSuccess)
		}
		return
	}
	e.Undoable(message, func() { p.undoChange(e, change) })
}

// undoChange takes change back from the equalizer as it stands now and sends
// it to the daemon. A preset still gliding lands first; a band that returns
// unfolds in its own column the next time the strip is drawn.
func (p *Page) undoChange(e *env.Env, change equalizerChange) {
	p.reconcile(e)
	if p.presetAnimating {
		p.working = cloneEqualizer(p.presetTarget)
		p.presetAnimating = false
	}
	reverted := revertChange(p.working, change)
	if change.removed >= 0 && len(reverted.Bands) > len(p.working.Bands) {
		at := min(change.removed, len(p.working.Bands))
		if at <= len(p.bands) {
			p.bands = slices.Insert(p.bands, at, bandControl{})
		}
		if p.removing >= at {
			p.removing++
		}
		p.graph.Select(at)
	}
	p.working = reverted
	p.ensureBandControls()
	p.submit(e)
}
