// Package pagesheet holds the sheet parts Ryotunes' pages share: the spec strip's figures
// and the work pattern's "loading more" line.
package pagesheet

import (
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/sheet"
)

// Strip keeps a page's spec strip as figures: counts under a hundred are zero padded
// ("03"), larger ones plain ("1204"). The page head rolls a figure that changes
// (look.CountRoll), so the strip hands it the values as they are; the specs are rebuilt
// only when a figure changes, so a strip at rest allocates nothing.
type Strip struct {
	counts []count
	specs  []sheet.Spec
	dirty  bool
}

type count struct {
	label string
	value int
	lit   bool
}

// Set puts figure index of the strip; the strip grows to hold it.
func (s *Strip) Set(index int, label string, value int, lit bool) {
	for len(s.counts) <= index {
		s.counts = append(s.counts, count{})
		s.dirty = true
	}
	next := count{label: label, value: value, lit: lit}
	if s.counts[index] != next {
		s.counts[index] = next
		s.dirty = true
	}
}

// Len sets how many figures the strip shows, dropping those past n.
func (s *Strip) Len(n int) {
	if n < len(s.counts) {
		s.counts = s.counts[:n]
		s.dirty = true
	}
}

// Specs returns the strip to hand to the page head, or nil while ready says the counts
// have not arrived (so no zeros flash before the figures).
func (s *Strip) Specs(ready bool) []sheet.Spec {
	if !ready {
		return nil
	}
	if !s.dirty && len(s.specs) == len(s.counts) {
		return s.specs
	}
	s.dirty = false
	s.specs = s.specs[:0]
	for _, figure := range s.counts {
		s.specs = append(s.specs, sheet.Spec{Label: figure.label, Value: kit.Pad2(figure.value), Lit: figure.lit})
	}
	return s.specs
}
