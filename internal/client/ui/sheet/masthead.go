package sheet

import (
	"strings"

	"gioui.org/layout"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
)

// Page is what a page's masthead reads: its place, the part of it shown
// ("Songs" in the Library, sentence case, may be empty) with that part's
// gloss, and the provider the page draws from (its display name, empty for
// pages that use none).
type Page struct {
	Place     Place
	Part      string
	PartGloss string
	Provider  string
}

// Masthead is a page's running head (kit.Masthead) filled from Ryotunes'
// sheet: "§ 01  LIBRARY  /  SONGS  書庫・曲" at the left and "P 02/10 ·
// YOUTUBE MUSIC · 12:40" at the right, the clock showing seconds only while
// the window is focused and motion is not reduced (kit.Ambient, fed by the
// shell from the window's focus). Glosses follow the desktop. The strings
// are built only when the page changes, so a masthead at rest allocates
// nothing and asks for no frame, save the clock's ambient second.
type Masthead struct {
	head  kit.Masthead
	page  Page
	gloss bool
	ready bool
	facts [2]string
}

// Layout draws the masthead across the width given, 34 tall; progress is
// how far the page is read (0..1), negative for a page that does not scroll.
func (m *Masthead) Layout(gtx layout.Context, th *look.Theme, page Page, progress float32) layout.Dimensions {
	gloss := th.Desktop.Gloss
	if !m.ready || page != m.page || gloss != m.gloss {
		m.ready, m.page, m.gloss = true, page, gloss
		m.head.Number = page.Place.Number
		m.head.Where = page.Place.Name
		if page.Part != "" {
			m.head.Where += "  /  " + page.Part
		}
		m.head.Gloss = ""
		if gloss {
			m.head.Gloss = JoinGloss(page.Place.Gloss, page.PartGloss)
		}
		facts := m.facts[:0]
		if place := page.Place.PagePlace(); place != "" {
			facts = append(facts, place)
		}
		if page.Provider != "" {
			facts = append(facts, strings.ToUpper(page.Provider))
		}
		m.head.Facts = facts
		m.head.Clock = true
	}
	return m.head.Layout(gtx, th.Theme, progress)
}
