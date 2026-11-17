package update

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
		{"2.0.10", "2.0.9", 1},
		{"1.2.6", "2.0.0", -1},
		{"2.0.0", "1.99.99", 1},
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
	for _, invalid := range []string{"0.1.0", "01.0.0", "1.01.0", "1.0.01", "+1.0.0", "1..0", "2.0.0-rc1", "1.0.0/../../bad", "v1.2.3", "1.a.0", ""} {
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
	rel.Draft = true
	if got := releaseInfo(rel, "1.2.3", managerPacman, nil); got.LatestVersion != nil {
		t.Fatalf("draft exposed as latest: %#v", got)
	}
	rel.Draft = false
	rel.Assets = rel.Assets[:1]
	if got := releaseInfo(rel, "1.2.3", managerPacman, nil); got.CanInstall || got.UnsupportedReason == nil {
		t.Fatalf("release without checksum installable: %#v", got)
	}
}

// fakeReleases mirrors the GitHub releases list: newest first, the 1.x line still the
// stable "latest", and Ryotunes 2 published only as prereleases.
func fakeReleases() []release {
	complete := func(tag string, prerelease bool) release {
		name := packageName(strings.TrimPrefix(tag, "v"), managerPacman)
		return release{TagName: tag, Prerelease: prerelease, Assets: []asset{
			{Name: name, Size: 10, URL: "https://example/" + name},
			{Name: name + ".sha256", Size: 70, URL: "https://example/" + name + ".sha256"},
		}}
	}
	draft := complete("v2.1.0", true)
	draft.Draft = true
	return []release{
		draft,
		complete("v2.0.9", true),
		complete("v2.0.10", true),
		{TagName: "v2.0.11-rc1", Prerelease: true},
		{TagName: "nightly", Prerelease: true},
		complete("v1.2.6", false),
		complete("v2.0.0", true),
		complete("v1.2.5", false),
	}
}

func TestSelectReleaseFollowsTheRunningLine(t *testing.T) {
	tests := []struct {
		current   string
		want      string
		available bool
	}{
		// 2.x installs follow prereleases and never go back to the stable 1.x latest.
		{"2.0.0", "v2.0.10", true},
		{"2.0.9", "v2.0.10", true},
		{"2.0.10", "v2.0.10", false},
		{"2.0.11", "v2.0.10", false},
		{"2.1.0", "v2.0.10", false},
		// 1.x installs keep stable releases only and stay on their line.
		{"1.2.5", "v1.2.6", true},
		{"1.2.6", "v1.2.6", false},
		// A development build is never offered a prerelease.
		{"dev", "v1.2.6", true},
	}
	for _, test := range tests {
		rel := selectRelease(fakeReleases(), test.current)
		if rel == nil || rel.TagName != test.want {
			t.Fatalf("selectRelease(%q) = %#v, want %s", test.current, rel, test.want)
		}
		info := releaseInfo(rel, test.current, managerPacman, nil)
		if info.Available != test.available || info.LatestVersion == nil || "v"+*info.LatestVersion != test.want {
			t.Fatalf("releaseInfo for %q = %#v, want %s available=%v", test.current, info, test.want, test.available)
		}
		if test.available && !info.CanInstall {
			t.Fatalf("complete release %s not installable from %q: %#v", test.want, test.current, info)
		}
	}

	// With no 2.x release published, a 2.x install sees nothing rather than v1.2.6.
	stableOnly := []release{{TagName: "v1.2.6"}, {TagName: "v1.2.5"}}
	if rel := selectRelease(stableOnly, "2.0.0"); rel != nil {
		t.Fatalf("2.x install offered %s", rel.TagName)
	}
	if info := releaseInfo(nil, "2.0.0", managerPacman, nil); info.Available || info.LatestVersion != nil {
		t.Fatalf("no release reported an update: %#v", info)
	}
	// A stable 3.x release is not a 2.x update.
	if rel := selectRelease([]release{{TagName: "v3.0.0"}, {TagName: "v2.0.1"}}, "2.0.0"); rel == nil || rel.TagName != "v2.0.1" {
		t.Fatalf("2.x install selected %#v", rel)
	}
}

func TestFetchReleaseReadsTheReleasesList(t *testing.T) {
	payload, err := json.Marshal(fakeReleases())
	if err != nil {
		t.Fatal(err)
	}
	status := http.StatusOK
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/ryoku-dev/ryotunes/releases" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(status)
		_, _ = w.Write(payload)
	}))
	defer server.Close()
	previous := releasesURL
	releasesURL = server.URL + "/repos/ryoku-dev/ryotunes/releases?per_page=50"
	defer func() { releasesURL = previous }()

	rel, err := fetchRelease(context.Background(), "2.0.0")
	if err != nil || rel == nil || rel.TagName != "v2.0.10" {
		t.Fatalf("fetchRelease(2.0.0) = %#v, %v", rel, err)
	}
	rel, err = fetchRelease(context.Background(), "1.2.5")
	if err != nil || rel == nil || rel.TagName != "v1.2.6" {
		t.Fatalf("fetchRelease(1.2.5) = %#v, %v", rel, err)
	}
	status = http.StatusNotFound
	if rel, err := fetchRelease(context.Background(), "2.0.0"); err != nil || rel != nil {
		t.Fatalf("missing repository = %#v, %v", rel, err)
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

func TestInstalledTwoLineVersionsParse(t *testing.T) {
	info := "Name            : ryotunes\nVersion         : 1:2.0.10-1\nURL             : https://github.com/ryoku-dev/ryotunes\nPackager        : Ryoku Build System\n"
	if version, err := pacmanInfoVersion(info); err != nil || version != "2.0.10" {
		t.Fatalf("pacmanInfoVersion() = %q, %v", version, err)
	}
	if version, err := xbpsVersion("ryotunes-2.0.10_1"); err != nil || version != "2.0.10" {
		t.Fatalf("xbpsVersion() = %q, %v", version, err)
	}
}
