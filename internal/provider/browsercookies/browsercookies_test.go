package browsercookies

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStoreKindPathFiltering(t *testing.T) {
	tests := []struct {
		path  string
		name  string
		depth int
		kind  Kind
		ok    bool
	}{
		{"/home/me/.mozilla/firefox/a/cookies.sqlite", "cookies.sqlite", 3, Gecko, true},
		{"/home/me/.mozilla/a/b/c/d/e/cookies.sqlite", "cookies.sqlite", 5, Gecko, false},
		{"/home/me/.config/google-chrome/Default/Cookies", "Cookies", 3, Chromium, true},
		{"/home/me/.config/chromium/Profile 1/Network/Cookies", "Cookies", 4, Chromium, true},
		{"/home/me/.config/random/Profile 1/Cookies", "Cookies", 3, Chromium, false},
		{"/home/me/.config/google-chrome/Default/Cookies", "Cookies", 6, Chromium, false},
	}
	for _, test := range tests {
		kind, ok := storeKind(test.path, test.name, test.depth)
		if ok != test.ok || ok && kind != test.kind {
			t.Fatalf("storeKind(%q): got (%v,%v), want (%v,%v)", test.path, kind, ok, test.kind, test.ok)
		}
	}
}

func TestWalkStoresRefusesSymlinkDirectories(t *testing.T) {
	root := t.TempDir()
	real := filepath.Join(root, "real", "Default")
	if err := os.MkdirAll(real, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(real, "Cookies"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "real"), filepath.Join(root, "linked")); err != nil {
		t.Fatal(err)
	}
	var found []string
	walkStores(root, 0, 5, func(path string, _ os.DirEntry, _ int) { found = append(found, path) })
	if len(found) != 1 || found[0] != filepath.Join(real, "Cookies") {
		t.Fatalf("walked through symlink: %v", found)
	}
}

func TestReadGeckoCookieMetadata(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cookies.sqlite")
	db := openCookieDB(t, path)
	if _, err := db.Exec(`CREATE TABLE moz_cookies (
		name TEXT, value TEXT, host TEXT, path TEXT, expiry INTEGER, originAttributes TEXT
	)`); err != nil {
		t.Fatal(err)
	}
	rows := []struct {
		name, attributes string
		expiry           int64
	}{
		{name: "seconds", expiry: 1_900_000_000},
		{name: "milliseconds", expiry: 1_900_000_000_123, attributes: "^userContextId=4&partitionKey=%28https%2Cyoutube.com%29"},
		{name: "session", expiry: 0},
	}
	for _, row := range rows {
		if _, err := db.Exec(
			`INSERT INTO moz_cookies VALUES (?, ?, ?, ?, ?, ?)`,
			row.name, "value", ".youtube.com", "/youtubei/", row.expiry, row.attributes,
		); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	cookies, err := Read(Store{Kind: Gecko, Path: path}, "youtube.com")
	if err != nil {
		t.Fatal(err)
	}
	got := cookiesByName(cookies)
	if len(got) != 3 {
		t.Fatalf("cookies = %#v", cookies)
	}
	if got["seconds"].Path != "/youtubei/" || !got["seconds"].Expires.Equal(time.Unix(1_900_000_000, 0)) {
		t.Fatalf("seconds metadata = %#v", got["seconds"])
	}
	milliseconds := got["milliseconds"]
	if !milliseconds.Expires.Equal(time.UnixMilli(1_900_000_000_123)) || milliseconds.Container != "4" || !milliseconds.Partitioned {
		t.Fatalf("milliseconds metadata = %#v", milliseconds)
	}
	if !got["session"].Expires.IsZero() {
		t.Fatalf("session expiry = %v", got["session"].Expires)
	}
}

func TestReadChromiumCookieMetadataWithOptionalPartitionColumn(t *testing.T) {
	const chromiumEpoch = int64(11_644_473_600_000_000)
	expires := time.Unix(1_900_000_000, 123_000_000)
	path := filepath.Join(t.TempDir(), "Cookies")
	db := openCookieDB(t, path)
	if _, err := db.Exec(`CREATE TABLE cookies (
		name TEXT, value TEXT, host_key TEXT, path TEXT, expires_utc INTEGER,
		encrypted_value BLOB, top_frame_site_key TEXT
	)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(
		`INSERT INTO cookies VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"partitioned", "value", ".youtube.com", "/", chromiumEpoch+expires.UnixMicro(), []byte(nil), "https://music.youtube.com",
	); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	cookies, err := Read(Store{Kind: Chromium, Path: path}, "youtube.com")
	if err != nil {
		t.Fatal(err)
	}
	if len(cookies) != 1 || !cookies[0].Expires.Equal(expires) || !cookies[0].Partitioned || cookies[0].Container != "" {
		t.Fatalf("cookie metadata = %#v", cookies)
	}

	withoutPartition := filepath.Join(t.TempDir(), "Cookies")
	db = openCookieDB(t, withoutPartition)
	if _, err := db.Exec(`CREATE TABLE cookies (
		name TEXT, value TEXT, host_key TEXT, path TEXT, expires_utc INTEGER, encrypted_value BLOB
	)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(
		`INSERT INTO cookies VALUES (?, ?, ?, ?, ?, ?)`,
		"session", "value", ".youtube.com", "/", 0, []byte(nil),
	); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	cookies, err = Read(Store{Kind: Chromium, Path: withoutPartition}, "youtube.com")
	if err != nil {
		t.Fatal(err)
	}
	if len(cookies) != 1 || !cookies[0].Expires.IsZero() || cookies[0].Partitioned {
		t.Fatalf("cookie metadata without partition column = %#v", cookies)
	}
}

func openCookieDB(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func cookiesByName(cookies []Cookie) map[string]Cookie {
	result := make(map[string]Cookie, len(cookies))
	for _, cookie := range cookies {
		result[cookie.Name] = cookie
	}
	return result
}
