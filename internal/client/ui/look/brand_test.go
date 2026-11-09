package look

import (
	"testing"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/tokens"
	"github.com/ryoku-dev/ryotunes/internal/model"
)

func TestBrandInkContrast(t *testing.T) {
	providers := []model.Provider{
		model.ProviderYouTube, model.ProviderSpotify, model.ProviderSoundCloud,
		model.ProviderTidal, model.ProviderLocal, model.ProviderRadio,
	}
	for _, dark := range []bool{false, true} {
		roles := palette.Signature(dark)
		for _, provider := range providers {
			ratio := palette.ContrastRatio(BrandInk(provider, roles), roles.Stone2)
			if ratio < float64(tokens.ContrastMinimum) {
				t.Errorf("dark=%v provider=%s contrast=%0.3f", dark, provider, ratio)
			}
		}
	}
}
