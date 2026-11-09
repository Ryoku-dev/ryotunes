// Package browsercookies discovers and reads browser cookie databases without
// touching a browser's live SQLite files.
package browsercookies

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// Kind identifies a browser cookie database family.
type Kind int

const (
	// Gecko is Firefox's moz_cookies database.
	Gecko Kind = iota
	// Chromium is Chrome's cookies database.
	Chromium
)

// Store identifies one discovered browser cookie database.
type Store struct {
	Kind Kind
	Path string
}

// Cookie is the portable subset of a browser cookie.
type Cookie struct {
	Name  string
	Value string
	Host  string
}

// Discover returns eligible cookie stores in default-browser-first path order.
func Discover() []Store {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return nil
	}
	config := os.Getenv("XDG_CONFIG_HOME")
	if !filepath.IsAbs(config) {
		config = filepath.Join(home, ".config")
	}
	roots := []string{config, filepath.Join(home, ".mozilla")}
	roots = appendContainerRoots(roots, filepath.Join(home, ".var", "app"), []string{"config", ".config", ".mozilla"})
	roots = appendContainerRoots(roots, filepath.Join(home, "snap"), []string{filepath.Join("common", ".mozilla"), filepath.Join("common", ".config")})

	seen := make(map[string]bool)
	var stores []Store
	for _, root := range roots {
		walkStores(root, 0, 5, func(path string, entry os.DirEntry, depth int) {
			kind, ok := storeKind(path, entry.Name(), depth)
			if !ok || seen[path] {
				return
			}
			seen[path] = true
			stores = append(stores, Store{Kind: kind, Path: path})
		})
	}
	browser := strings.ToLower(DefaultBrowser())
	sort.SliceStable(stores, func(i, j int) bool {
		pi, pj := storePriority(stores[i].Path, browser), storePriority(stores[j].Path, browser)
		if pi != pj {
			return pi < pj
		}
		return stores[i].Path < stores[j].Path
	})
	if len(stores) > 64 {
		stores = stores[:64]
	}
	return stores
}

func appendContainerRoots(roots []string, parent string, suffixes []string) []string {
	entries, err := os.ReadDir(parent)
	if err != nil {
		return roots
	}
	for _, entry := range entries {
		if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		for _, suffix := range suffixes {
			roots = append(roots, filepath.Join(parent, entry.Name(), suffix))
		}
	}
	return roots
}

func walkStores(dir string, depth, maxDepth int, found func(string, os.DirEntry, int)) {
	if depth > maxDepth {
		return
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())
		if entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		if entry.IsDir() {
			walkStores(path, depth+1, maxDepth, found)
			continue
		}
		found(path, entry, depth)
	}
}

func storeKind(path, name string, depth int) (Kind, bool) {
	if name == "cookies.sqlite" && depth <= 4 {
		return Gecko, true
	}
	if name != "Cookies" || depth > 5 {
		return 0, false
	}
	lower := strings.ToLower(path)
	if !strings.Contains(path, "Default") && !strings.Contains(path, "Network") &&
		!strings.Contains(lower, "chromium") && !strings.Contains(lower, "google-chrome") {
		return 0, false
	}
	return Chromium, true
}

func storePriority(path, browser string) int {
	if browser == "" {
		return 1
	}
	p := strings.ToLower(path)
	for _, token := range strings.FieldsFunc(browser, func(r rune) bool { return r == '-' || r == '.' || r == '_' }) {
		if len(token) > 2 && strings.Contains(p, token) {
			return 0
		}
	}
	return 1
}

// Read copies a store and its SQLite sidecars, then returns matching cookies.
func Read(st Store, hostSuffixes ...string) ([]Cookie, error) {
	tmp, err := os.MkdirTemp("", "ryotunes-browser-cookies-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	if err := os.Chmod(tmp, 0o700); err != nil {
		return nil, err
	}
	copyPath := filepath.Join(tmp, "cookies.db")
	if err := copyFile(st.Path, copyPath); err != nil {
		return nil, err
	}
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		if err := copyFile(st.Path+suffix, copyPath+suffix); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(copyPath)+"?mode=ro")
	if err != nil {
		return nil, err
	}
	defer db.Close()
	if st.Kind == Gecko {
		return readGecko(db, hostSuffixes)
	}
	return readChromium(db, st, hostSuffixes)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func readGecko(db *sql.DB, suffixes []string) ([]Cookie, error) {
	rows, err := db.Query(`SELECT name, value, host FROM moz_cookies`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Cookie
	for rows.Next() {
		var cookie Cookie
		if err := rows.Scan(&cookie.Name, &cookie.Value, &cookie.Host); err != nil {
			return nil, err
		}
		if hostMatches(cookie.Host, suffixes) {
			result = append(result, cookie)
		}
	}
	return result, rows.Err()
}

func readChromium(db *sql.DB, st Store, suffixes []string) ([]Cookie, error) {
	rows, err := db.Query(`SELECT name, value, host_key, encrypted_value FROM cookies`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	v10 := chromiumKey("peanuts")
	var v11 []byte
	v11Tried := false
	var result []Cookie
	for rows.Next() {
		var name, value, host string
		var encrypted []byte
		if err := rows.Scan(&name, &value, &host, &encrypted); err != nil {
			return nil, err
		}
		if !hostMatches(host, suffixes) {
			continue
		}
		if value == "" && len(encrypted) >= 3 {
			var key []byte
			switch string(encrypted[:3]) {
			case "v10":
				key = v10
			case "v11":
				if !v11Tried {
					v11, _ = keyringKey(st.Path)
					v11Tried = true
				}
				key = v11
			}
			if len(key) != 0 {
				value, _ = decryptChromium(encrypted[3:], key, host)
			}
		}
		if value != "" {
			result = append(result, Cookie{Name: name, Value: value, Host: host})
		}
	}
	return result, rows.Err()
}

func chromiumKey(passphrase string) []byte {
	return pbkdf2SHA1([]byte(passphrase), []byte("saltysalt"), 1, 16)
}

func pbkdf2SHA1(password, salt []byte, iterations, length int) []byte {
	prf := func() hash.Hash { return hmac.New(sha1.New, password) }
	key := make([]byte, 0, length)
	for block := 1; len(key) < length; block++ {
		mac := prf()
		mac.Write(salt)
		mac.Write([]byte{byte(block >> 24), byte(block >> 16), byte(block >> 8), byte(block)})
		u := mac.Sum(nil)
		t := append([]byte(nil), u...)
		for i := 1; i < iterations; i++ {
			mac = prf()
			mac.Write(u)
			u = mac.Sum(nil)
			for j := range t {
				t[j] ^= u[j]
			}
		}
		key = append(key, t...)
	}
	return key[:length]
}

func keyringKey(path string) ([]byte, error) {
	apps := []string{"chrome", "chromium"}
	if strings.Contains(strings.ToLower(path), "chromium") {
		apps[0], apps[1] = apps[1], apps[0]
	}
	for _, app := range apps {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		out, err := exec.CommandContext(ctx, "secret-tool", "lookup", "application", app).Output()
		cancel()
		if err == nil && strings.TrimSpace(string(out)) != "" {
			return chromiumKey(strings.TrimSpace(string(out))), nil
		}
	}
	return nil, errors.New("browser keyring password unavailable")
}

func decryptChromium(ciphertext, key []byte, host string) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	if len(ciphertext) == 0 || len(ciphertext)%aes.BlockSize != 0 {
		return "", errors.New("invalid Chromium cookie ciphertext")
	}
	plain := make([]byte, len(ciphertext))
	cipher.NewCBCDecrypter(block, []byte("                ")).CryptBlocks(plain, ciphertext)
	pad := int(plain[len(plain)-1])
	if pad == 0 || pad > aes.BlockSize || pad > len(plain) {
		return "", errors.New("invalid Chromium cookie padding")
	}
	for _, b := range plain[len(plain)-pad:] {
		if int(b) != pad {
			return "", errors.New("invalid Chromium cookie padding")
		}
	}
	plain = plain[:len(plain)-pad]
	digest := sha256.Sum256([]byte(strings.TrimPrefix(host, ".")))
	if len(plain) >= len(digest) && string(plain[:len(digest)]) == string(digest[:]) {
		plain = plain[len(digest):]
	}
	return string(plain), nil
}

func hostMatches(host string, suffixes []string) bool {
	if len(suffixes) == 0 {
		return true
	}
	host = strings.ToLower(strings.TrimPrefix(host, "."))
	for _, suffix := range suffixes {
		suffix = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(suffix), "."))
		if suffix != "" && (host == suffix || strings.HasSuffix(host, "."+suffix)) {
			return true
		}
	}
	return false
}

// ChangedSince reports whether the database or WAL changed after since.
func ChangedSince(st Store, since time.Time) bool {
	for _, path := range []string{st.Path, st.Path + "-wal"} {
		info, err := os.Stat(path)
		if err == nil && info.ModTime().After(since) {
			return true
		}
	}
	return false
}

// DefaultBrowser returns the desktop file selected by xdg-settings.
func DefaultBrowser() string {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "xdg-settings", "get", "default-web-browser").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// OpenURL asks the desktop to open an HTTPS URL.
func OpenURL(url string) error {
	if !strings.HasPrefix(url, "https://") {
		return fmt.Errorf("refusing non-HTTPS URL")
	}
	return exec.Command("xdg-open", url).Start()
}
