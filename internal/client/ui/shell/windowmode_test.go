package shell

import (
	"image"
	"testing"

	gioapp "gioui.org/app"
	"gioui.org/unit"
)

func TestMainWindowMode(t *testing.T) {
	floating := mainWindowMode(1280, 800, true)
	if floating.width != 1280 || floating.height != 800 {
		t.Fatalf("floating map size = %vx%v, want 1280x800", floating.width, floating.height)
	}
	if !floating.fixedAtMap || !floating.persistSize {
		t.Fatalf("floating plan = %+v, want fixed map with size persistence", floating)
	}
	if floating.minimumWidth != 480 || floating.minimumHeight != 560 {
		t.Fatalf("floating relaxed minimum = %vx%v, want 480x560", floating.minimumWidth, floating.minimumHeight)
	}
	floatingConfig := applyWindowOptions(floating.creationOptions())
	if floatingConfig.Size != image.Pt(1280, 800) || floatingConfig.MinSize != floatingConfig.Size || floatingConfig.MaxSize != floatingConfig.Size {
		t.Fatalf("floating creation config = %+v, want fixed 1280x800", floatingConfig)
	}

	tiled := mainWindowMode(1280, 800, false)
	if tiled.fixedAtMap || tiled.persistSize {
		t.Fatalf("tiled plan = %+v, want unconstrained map without size persistence", tiled)
	}
	if tiled.minimumWidth != 480 || tiled.minimumHeight != 560 {
		t.Fatalf("tiled minimum = %vx%v, want 480x560", tiled.minimumWidth, tiled.minimumHeight)
	}
	tiledConfig := applyWindowOptions(tiled.creationOptions())
	if tiledConfig.Size != image.Pt(1280, 800) || tiledConfig.MinSize != image.Pt(480, 560) || tiledConfig.MaxSize != (image.Point{}) {
		t.Fatalf("tiled creation config = %+v, want 1280x800 with only a 480x560 minimum", tiledConfig)
	}
	relaxed := applyWindowOptions(relaxedWindowOptions(480, 560))
	if relaxed.MinSize != image.Pt(480, 560) || relaxed.MaxSize != image.Pt(int(relaxedWindowMax), int(relaxedWindowMax)) {
		t.Fatalf("relaxed config = %+v, want 480x560 with a practical unbounded maximum", relaxed)
	}
}

func TestWindowMapPadding(t *testing.T) {
	var padding windowPaddingTracker
	for index, test := range []struct {
		observed int
		want     int
	}{
		{observed: 840, want: 800},
		{observed: 800, want: 800},
		{observed: 900, want: 900},
	} {
		if got := padding.contentHeight(test.observed, 800); got != test.want {
			t.Fatalf("step %d content height = %d, want %d", index, got, test.want)
		}
	}

	var persistentPadding windowPaddingTracker
	if got := persistentPadding.contentHeight(840, 800); got != 800 {
		t.Fatalf("first padded height = %d, want 800", got)
	}
	if got := persistentPadding.contentHeight(900, 800); got != 860 {
		t.Fatalf("resized padded height = %d, want 860", got)
	}
}

func TestMiniWindowMode(t *testing.T) {
	mini := miniWindowMode(1.1)
	if mini.width != unit.Dp(572) || mini.height != unit.Dp(193.6) {
		t.Fatalf("mini map size = %vx%v, want 572x193.6 at 110%% zoom", mini.width, mini.height)
	}
	if !mini.fixedAtMap || mini.persistSize {
		t.Fatalf("mini plan = %+v, want fixed map without size persistence", mini)
	}
	if mini.minimumWidth != unit.Dp(330) || mini.minimumHeight != unit.Dp(176) {
		t.Fatalf("mini relaxed minimum = %vx%v, want 330x176 at 110%% zoom", mini.minimumWidth, mini.minimumHeight)
	}
	config := applyWindowOptions(mini.creationOptions())
	if config.Size != image.Pt(572, 194) || config.MinSize != config.Size || config.MaxSize != config.Size {
		t.Fatalf("mini creation config = %+v, want fixed 572x194 at 110%% zoom", config)
	}
}

func applyWindowOptions(options []gioapp.Option) gioapp.Config {
	var config gioapp.Config
	metric := unit.Metric{PxPerDp: 1, PxPerSp: 1}
	for _, option := range options {
		option(metric, &config)
	}
	return config
}
