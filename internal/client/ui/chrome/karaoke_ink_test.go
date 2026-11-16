package chrome

import (
	"image/color"
	"testing"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
	libtheme "github.com/ryoku-dev/ryoku/docs/lithic/library/go/theme"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/model"
)

// TestKaraokeSungWordsStandApart checks that the words still to sing differ
// from the sung words (the accent's ink) in every mode: a grey cover's
// colourless accent by lightness, coloured accents by their hue, keeping
// fg2 as before; the unsung words stay readable on the pane either way.
func TestKaraokeSungWordsStandApart(t *testing.T) {
	for _, dark := range []bool{true, false} {
		roles := palette.Signature(dark)
		accents := map[string]look.Accent{
			"grey cover":    look.NeutralAccent(roles),
			"warm cover":    look.AccentFromSample(color.NRGBA{R: 0xd0, G: 0x58, B: 0x30, A: 0xff}, roles),
			"cool cover":    look.AccentFromSample(color.NRGBA{R: 0x40, G: 0x80, B: 0xd0, A: 0xff}, roles),
			"youtube music": look.AccentFromSource(model.ProviderYouTube, roles),
		}
		for name, accent := range accents {
			th := look.NewTheme(roles, libtheme.Comfortable)
			th.Accent = accent
			rest := karaokeRest(th)
			ink := accent.Ink
			if spread := max(ink.R, ink.G, ink.B) - min(ink.R, ink.G, ink.B); spread < colourGrey {
				if ratio := palette.ContrastRatio(ink, rest); ratio < karaokeSeparation {
					t.Errorf("dark %v, %s: colourless sung words against unsung are %.2f:1", dark, name, ratio)
				}
			} else if rest != roles.Fg2 {
				t.Errorf("dark %v, %s: a coloured accent should keep fg2 for the unsung words", dark, name)
			}
			if ratio := palette.ContrastRatio(rest, roles.Stone2); ratio < 3 {
				t.Errorf("dark %v, %s: unsung words read %.2f:1 on the pane", dark, name, ratio)
			}
		}
	}
}
