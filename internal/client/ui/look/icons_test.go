package look

import (
	"reflect"
	"testing"

	"gioui.org/widget"
)

func TestEveryIconIsDecoded(t *testing.T) {
	v := reflect.ValueOf(Icon)
	for i := range v.NumField() {
		if v.Field(i).IsNil() {
			t.Errorf("Icon.%s is nil", v.Type().Field(i).Name)
		}
	}
}

func TestFilledMarksDestinationsOnly(t *testing.T) {
	for _, c := range []struct {
		name          string
		outline, fill *widget.Icon
	}{
		{"home", Icon.Home, Icon.HomeFill},
		{"library", Icon.Library, Icon.LibraryFill},
		{"downloads", Icon.Downloads, Icon.DownloadsFill},
		{"settings", Icon.Settings, Icon.SettingsFill},
		{"folder", Icon.Folder, Icon.FolderFill},
		{"sidebar", Icon.Sidebar, Icon.SidebarFill},
		{"heart", Icon.HeartOutline, Icon.Heart},
	} {
		if c.outline == c.fill {
			t.Errorf("%s: outline and fill are the same glyph", c.name)
		}
		if got := Filled(c.outline); got != c.fill {
			t.Errorf("Filled(%s) is not its fill variant", c.name)
		}
		if Filled(c.fill) != c.fill {
			t.Errorf("Filled(%s fill) changed an already filled glyph", c.name)
		}
	}
	for _, icon := range []*widget.Icon{Icon.Radio, Icon.Search, Icon.Play, Icon.Download, nil} {
		if Filled(icon) != icon {
			t.Error("Filled changed a glyph that has no destination fill")
		}
	}
	if allocs := testing.AllocsPerRun(100, func() { _ = Filled(Icon.Library) }); allocs != 0 {
		t.Errorf("Filled allocates %v per call", allocs)
	}
}
