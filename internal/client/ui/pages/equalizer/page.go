// Package equalizer implements the retained parametric equalizer instrument.
package equalizer

import (
	"gioui.org/unit"
	"reflect"
	"time"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/anim"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/model"
)

type bandControl struct {
	fader    look.Fader
	shape    kit.Click
	q        look.ValueDrag
	unfold   anim.Tween
	ready    bool
	existing bool
}

// Page is the retained equalizer page.
type Page struct {
	route       env.Route
	scroll      look.Scroller
	graph       look.EqualizerGraph
	graphHeight unit.Dp

	working         model.Equalizer
	initialized     bool
	dragging        bool
	dirty           bool
	applyAt         time.Time
	pending         bool
	submitted       model.Equalizer
	requestInFlight bool
	requestQueued   bool
	requestSeq      uint64

	presets        []model.EQPreset
	presetsLoaded  bool
	presetsLoading bool
	presetButton   kit.Plaque
	presetMenu     kit.Menu
	presetTag      struct{}
	presetCommands []int

	toggle                                   kit.Toggle
	reset, importButton, exportButton, lower kit.Plaque
	add                                      kit.Plaque
	addTip                                   look.Tip
	shapeMenu                                kit.Menu
	shapeTag                                 struct{}
	shapeBand                                int
	contextBand                              int

	preamp              look.Fader
	bands               []bandControl
	strip               look.HScroller
	stripPrevious       kit.IconPlaque
	stripNext           kit.IconPlaque
	stripSteps          bool
	stripSelection      anim.Tween
	stripSelectionReady bool
	removing            int
	removeComplete      bool

	presetTween     anim.Tween
	presetAnimating bool
	presetFrom      model.Equalizer
	presetTarget    model.Equalizer

	saveDialog   savePresetDialog
	deleteDialog deletePresetDialog
	importDialog importProfileDialog
}

// New constructs the retained Equalizer page.
func New(route env.Route) env.Page {
	return &Page{route: route, shapeBand: -1, contextBand: -1, removing: -1}
}

// Title returns the destination title.
func (p *Page) Title() string { return "Equalizer" }

func cloneEqualizer(source model.Equalizer) model.Equalizer {
	clone := source
	clone.Bands = append([]model.EQBand(nil), source.Bands...)
	return clone
}

func equalizerEqual(first, second model.Equalizer) bool {
	return reflect.DeepEqual(first, second)
}

func (p *Page) ensureBandControls() {
	if len(p.bands) < len(p.working.Bands) {
		p.bands = append(p.bands, make([]bandControl, len(p.working.Bands)-len(p.bands))...)
	}
}
