package look

import (
	"testing"
	"time"

	"gioui.org/layout"
	"gioui.org/op"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
	libtheme "github.com/ryoku-dev/ryoku/docs/lithic/library/go/theme"
	"github.com/ryoku-dev/ryotunes/internal/model"
)

func TestThemeFirstProviderAssignmentSnaps(t *testing.T) {
	roles := palette.Signature(true)
	theme := NewTheme(roles, libtheme.Comfortable)
	var operations op.Ops
	gtx := layout.Context{Ops: &operations, Now: time.Unix(1000, 0)}

	theme.SetProvider(gtx, model.ProviderYouTube)
	if want := sourceFor(model.ProviderYouTube, roles); theme.Src != want {
		t.Fatalf("first provider source = %#v, want %#v", theme.Src, want)
	}

	before := theme.Src
	theme.SetProvider(gtx, model.ProviderSpotify)
	if theme.Src.Fill != before.Fill || theme.Src.Ink != before.Ink || theme.Src.Field != before.Field {
		t.Fatalf("real provider switch snapped colors to %#v, want tween start %#v", theme.Src, before)
	}
	if want := sourceFor(model.ProviderSpotify, roles); theme.srcTo != want {
		t.Fatalf("provider switch target = %#v, want %#v", theme.srcTo, want)
	}
}

func TestThemeKnownProviderStartsAtProviderSource(t *testing.T) {
	roles := palette.Signature(true)
	theme := NewTheme(roles, libtheme.Comfortable, model.ProviderTidal)
	if want := sourceFor(model.ProviderTidal, roles); theme.Src != want {
		t.Fatalf("initial provider source = %#v, want %#v", theme.Src, want)
	}
}
