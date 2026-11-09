package update

import (
	"strings"
	"testing"
)

func TestVersionComparisonAndValidation(t *testing.T) {
	tests := []struct {
		left  string
		right string
		want  int
	}{
		{"1.0.9", "1.1.0", -1},
		{"1.12.3", "1.2.9", 1},
		{"1.2.3", "1.2.3", 0},
	}
	for _, test := range tests {
		left, leftOK := parseVersion(test.left)
		right, rightOK := parseVersion(test.right)
		if !leftOK || !rightOK {
			t.Fatalf("valid versions rejected: %q %q", test.left, test.right)
		}
		if got := compareVersion(left, right); got != test.want {
			t.Fatalf("compareVersion(%q, %q) = %d, want %d", test.left, test.right, got, test.want)
		}
	}
	for _, invalid := range []string{"2.0.0", "1.01.0", "1.0.10", "1.0.0/../../bad", "v1.2.3", "1.a.0"} {
		if _, ok := parseVersion(invalid); ok {
			t.Fatalf("parseVersion(%q) accepted invalid version", invalid)
		}
	}
}

func TestAssetSelection(t *testing.T) {
	archName := "ryotunes-1.2.4-1-x86_64.pkg.tar.zst"
	rpmName := "ryotunes-1.2.4-1.x86_64.rpm"
	rel := &release{Assets: []asset{
		{Name: archName, Size: 100, URL: "https://example/arch"},
		{Name: archName + ".sha256", Size: 100, URL: "https://example/arch.sum"},
		{Name: rpmName, Size: 200, URL: "https://example/rpm"},
		{Name: rpmName + ".sha256", Size: 100, URL: "https://example/rpm.sum"},
		{Name: "ryotunes-1.2.5-1-x86_64.pkg.tar.zst", Size: 100},
	}}
	for _, mgr := range []manager{managerPacman, managerXBPS} {
		pkg, sum, ok := selectAssets(rel, "1.2.4", mgr)
		if !ok || pkg.Name != archName || sum.Name != archName+".sha256" {
			t.Fatalf("Arch assets for manager %d = %#v %#v %v", mgr, pkg, sum, ok)
		}
	}
	pkg, sum, ok := selectAssets(rel, "1.2.4", managerDNF)
	if !ok || pkg.Name != rpmName || sum.Name != rpmName+".sha256" {
		t.Fatalf("RPM assets = %#v %#v %v", pkg, sum, ok)
	}

	rel.Assets[0].Size = maxPackage + 1
	if _, _, ok := selectAssets(rel, "1.2.4", managerPacman); ok {
		t.Fatal("oversized package was selectable")
	}
}

func TestReleaseInfoRejectsDraftAndIncompleteRelease(t *testing.T) {
	notes := "Changes"
	name := packageName("1.2.4", managerPacman)
	rel := &release{
		TagName: "v1.2.4", Body: &notes,
		Assets: []asset{{Name: name, Size: 10}, {Name: name + ".sha256", Size: 70}},
	}
	info := releaseInfo(rel, "1.2.3", managerPacman, nil)
	if !info.Available || !info.CanInstall || info.LatestVersion == nil || *info.LatestVersion != "1.2.4" {
		t.Fatalf("complete release info = %#v", info)
	}
	rel.Prerelease = true
	if got := releaseInfo(rel, "1.2.3", managerPacman, nil); got.LatestVersion != nil {
		t.Fatalf("prerelease exposed as latest: %#v", got)
	}
}

func TestPacmanInfoAcceptsRyokuOrigin(t *testing.T) {
	info := "Name            : ryotunes\nVersion         : 1:1.2.3-1\nURL             : https://github.com/ryoku-dev/ryotunes\nPackager        : Ryoku Build System\n"
	version, err := pacmanInfoVersion(info)
	if err != nil || version != "1.2.3" {
		t.Fatalf("pacmanInfoVersion() = %q, %v", version, err)
	}
	untrusted := strings.ReplaceAll(info, "https://github.com/ryoku-dev/ryotunes", "https://example.invalid")
	untrusted = strings.ReplaceAll(untrusted, "Ryoku Build System", "Unknown")
	if _, err := pacmanInfoVersion(untrusted); err == nil {
		t.Fatal("untrusted package origin was accepted")
	}
}
