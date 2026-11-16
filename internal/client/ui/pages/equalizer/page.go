// Package equalizer implements the retained parametric equalizer instrument.
package equalizer

import (
	"reflect"
	"time"

	"gioui.org/unit"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/anim"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/sheet"
	"github.com/ryoku-dev/ryotunes/internal/model"
)

type bandControl struct {
	fader    look.Fader
	shape    kit.Click
	q        look.ValueDrag
	unfold   anim.Tween
	ready    bool
	existing bool

	// the column's hover and press wash, the shape button's, and the
	// readouts that roll when their value steps
	hoverTag  struct{}
	hovered   bool
	pressed   bool
	wash      look.RowWash
	shapeWash look.RowWash
	gainRoll  look.ValueRoll
	freqRoll  look.ValueRoll
	qRoll     look.ValueRoll
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

	back                                     kit.IconPlaque
	toggle                                   kit.Toggle
	reset, importButton, exportButton, lower kit.Plaque
	add                                      kit.Plaque
	addTip                                   look.Tip
	shapeMenu                                kit.Menu
	shapeTag                                 struct{}
	shapeBand                                int
	contextBand                              int

	preamp              look.Fader
	preampRoll          look.ValueRoll
	bands               []bandControl
	strip               look.HScroller
	stripPrevious       kit.IconPlaque
	stripNext           kit.IconPlaque
	stripSteps          bool
	stripPitch          int
	stripUnfold         []float32
	stripSelection      anim.Tween
	stripSelectionReady bool
	removing            int
	removeComplete      bool

	presetTween     anim.Tween
	presetAnimating bool
	presetFrom      model.Equalizer
	presetTarget    model.Equalizer
	presetBands     []model.EQBand

	// the curve a band's removal started from, for its Undo
	removeBefore model.Equalizer

	// the sheet: the page's entrance, its tategaki, and the spec strip and
	// headroom words, rebuilt only when what they say changes
	reveal       sheet.Reveal
	tategaki     sheet.Tategaki
	figures      figures
	specs        []sheet.Spec
	headroomText string

	saveDialog   savePresetDialog
	deleteDialog deletePresetDialog
	importDialog importProfileDialog
}

// figures is what the page head's spec strip and the headroom line say.
type figures struct {
	ready   bool
	enabled bool
	preset  string
	bands   int
	preamp  float64
	peak    float64
}

// New constructs the retained Equalizer page.
func New(route env.Route) env.Page {
	return &Page{route: route, shapeBand: -1, contextBand: -1, removing: -1}
}

// Title returns the destination title.
func (p *Page) Title() string { return "Equalizer" }

// Masthead feeds the frame's running head: the equalizer is one place with
// no parts, and the ruler's marker follows how far the page is read.
func (p *Page) Masthead() (part, partGloss string, progress float32) {
	return "", "", scrollProgress(p.scroll.Position.Offset, p.scroll.Position.OffsetLast)
}

// scrollProgress is how far a one-item page is read, 0..1, from its list
// position: offset is how far its top has gone above the viewport, last how
// far its foot still lies below it (negative). A page that fits reads -1,
// a page that does not scroll.
func scrollProgress(offset, last int) float32 {
	total := offset - last
	if total <= 0 {
		return -1
	}
	return min(max(float32(offset)/float32(total), 0), 1)
}

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
