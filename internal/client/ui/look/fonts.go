package look

import (
	_ "embed"
	"sync"

	"gioui.org/font"
	"gioui.org/font/opentype"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/tokens"
)

var (
	//go:embed fonts/SpaceGrotesk-Regular.otf
	spaceGroteskRegular []byte
	//go:embed fonts/SpaceGrotesk-Medium.otf
	spaceGroteskMedium []byte
	//go:embed fonts/SpaceGrotesk-Bold.otf
	spaceGroteskBold []byte
	//go:embed fonts/SpaceMono-Regular.ttf
	spaceMonoRegular []byte
	//go:embed fonts/SpaceMono-Bold.ttf
	spaceMonoBold []byte

	fontsOnce sync.Once
	fonts     []font.FontFace
)

// Fonts returns the embedded Space Grotesk and Space Mono faces.
func Fonts() []font.FontFace {
	fontsOnce.Do(func() {
		faces := []struct {
			data     []byte
			typeface font.Typeface
			weight   font.Weight
		}{
			{spaceGroteskRegular, font.Typeface(tokens.FontSans[0]), font.Normal},
			{spaceGroteskMedium, font.Typeface(tokens.FontSans[0]), font.Medium},
			{spaceGroteskBold, font.Typeface(tokens.FontSans[0]), font.Bold},
			{spaceMonoRegular, font.Typeface(tokens.FontMono[0]), font.Normal},
			{spaceMonoBold, font.Typeface(tokens.FontMono[0]), font.Bold},
		}
		for _, embedded := range faces {
			parsed, err := opentype.ParseCollection(embedded.data)
			if err != nil {
				panic(err)
			}
			for index := range parsed {
				parsed[index].Font.Typeface = embedded.typeface
				parsed[index].Font.Weight = embedded.weight
			}
			fonts = append(fonts, parsed...)
		}
	})
	return fonts
}
