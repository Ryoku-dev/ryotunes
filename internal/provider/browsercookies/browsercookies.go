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
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/ryoku-dev/ryotunes/internal/protocol"
	_ "modernc.org/sqlite"
)

// ErrInvalidSource reports a remembered session source that names something
// Discover would never return as a cookie store.
var ErrInvalidSource = errors.New("not a browser cookie store")

const copyPrefix = "ryotunes-browser-cookies-"

// Kind identifies a browser cookie database family.
type Kind int

const (
	// Gecko is Firefox's moz_cookies database.
	Gecko Kind = iota
	// Chromium is Chrome's cookies database.
	Chromium
	unknownKind Kind = -1
)

// MarshalJSON persists a kind by name so reordering the constants never
// re-types a stored session.
func (k Kind) MarshalJSON() ([]byte, error) {
	switch k {
	case Gecko:
		return []byte(`"gecko"`), nil
	case Chromium:
		return []byte(`"chromium"`), nil
	default:
		return []byte(`"unknown"`), nil
	}
}

// UnmarshalJSON reads a kind by name, or the integer an earlier build stored.
// An unknown kind decodes without error so ValidateSource can refuse it.
func (k *Kind) UnmarshalJSON(data []byte) error {
	var name string
	if json.Unmarshal(data, &name) == nil {
		switch name {
		case "gecko":
			*k = Gecko
		case "chromium":
			*k = Chromium
		default:
			*k = unknownKind
		}
		return nil
	}
	var number int
	if err := json.Unmarshal(data, &number); err != nil {
		return fmt.Errorf("cookie store kind: %w", err)
	}
	switch number {
	case 0:
		*k = Gecko
	case 1:
		*k = Chromium
	default:
		*k = unknownKind
	}
	return nil
}

// Store identifies one discovered browser cookie database.
type Store struct {
	Kind Kind
	Path string
}

// SessionSource remembers which store and container a provider session was
// imported from, so the daemon can follow that browser.
type SessionSource struct {
	Kind      Kind   `json:"kind"`
	Path      string `json:"path"`
	Container string `json:"container,omitempty"`
}

// Cookie is the portable subset of a browser cookie.
type Cookie struct {
	Name        string
	Value       string
	Host        string
	Path        string
	Expires     time.Time
	Container   string
	Partitioned bool
}

// Discover returns eligible cookie stores in default-browser-first path order.
func Discover() []Store {
	seen := make(map[string]bool)
	var stores []Store
	for _, root := range Roots() {
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

// Roots returns the directories Discover walks for cookie stores.
func Roots() []string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return nil
	}
	config := os.Getenv("XDG_CONFIG_HOME")
	if !filepath.IsAbs(config) {
		config = filepath.Join(home, ".config")
	}
	roots := []string{filepath.Clean(config), filepath.Join(home, ".mozilla")}
	roots = appendContainerRoots(roots, filepath.Join(home, ".var", "app"), []string{"config", ".config", ".mozilla"})
	return appendContainerRoots(roots, filepath.Join(home, "snap"), []string{filepath.Join("common", ".mozilla"), filepath.Join("common", ".config")})
}

// ValidateSource returns the store a remembered source names, refusing any
// path Discover would not have found: wrong kind or name, a symlink or
// non-regular file, or a file outside every discovery root.
func ValidateSource(source SessionSource) (Store, error) {
	if source.Kind != Gecko && source.Kind != Chromium {
		return Store{}, fmt.Errorf("%w: unknown kind", ErrInvalidSource)
	}
	if !filepath.IsAbs(source.Path) || filepath.Clean(source.Path) != source.Path {
		return Store{}, fmt.Errorf("%w: %s is not a clean absolute path", ErrInvalidSource, source.Path)
	}
	info, err := os.Lstat(source.Path)
	if err != nil {
		return Store{}, err
	}
	if !info.Mode().IsRegular() {
		return Store{}, fmt.Errorf("%w: %s is not a regular file", ErrInvalidSource, source.Path)
	}
	for _, root := range Roots() {
		rel, err := filepath.Rel(root, source.Path)
		if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		parts := strings.Split(rel, string(filepath.Separator))
		depth := len(parts) - 1
		if depth > 5 || !realDirectories(root, parts[:depth]) {
			continue
		}
		if kind, ok := storeKind(source.Path, parts[depth], depth); ok && kind == source.Kind {
			return Store{Kind: kind, Path: source.Path}, nil
		}
	}
	return Store{}, fmt.Errorf("%w: %s", ErrInvalidSource, source.Path)
}

func realDirectories(root string, parts []string) bool {
	dir := root
	for i := -1; i < len(parts); i++ {
		if i >= 0 {
			dir = filepath.Join(dir, parts[i])
		}
		info, err := os.Lstat(dir)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return false
		}
	}
	return true
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
// The copy lives in the private runtime directory and is removed on return.
func Read(st Store, hostSuffixes ...string) ([]Cookie, error) {
	tmp, err := makeCopyDir()
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
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

func makeCopyDir() (string, error) {
	parent := protocol.RuntimeDir()
	if err := os.MkdirAll(parent, 0o700); err != nil || !privateDir(parent) {
		// without a private runtime directory, MkdirTemp in the sticky temp
		// directory is the one place no other user can rename
		parent = ""
	}
	tmp, err := os.MkdirTemp(parent, copyPrefix)
	if err != nil {
		return "", err
	}
	if err := os.Chmod(tmp, 0o700); err != nil {
		os.RemoveAll(tmp)
		return "", err
	}
	return tmp, nil
}

// privateDir reports a real directory owned by this user and closed to others.
func privateDir(path string) bool {
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0o700 {
		return false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && int(stat.Uid) == os.Getuid()
}

// staleCopyAge is far longer than any Read, keyring lookup included.
const staleCopyAge = 15 * time.Second

// RemoveStaleCopies deletes store copies a killed daemon left behind, in the
// runtime directory and in the temporary directory older builds used.
func RemoveStaleCopies() error {
	var failures []error
	cutoff := time.Now().Add(-staleCopyAge)
	for _, dir := range []string{protocol.RuntimeDir(), os.TempDir()} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				failures = append(failures, err)
			}
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() || !strings.HasPrefix(entry.Name(), copyPrefix) {
				continue
			}
			info, err := entry.Info()
			if err != nil || info.ModTime().After(cutoff) {
				continue
			}
			if err := os.RemoveAll(filepath.Join(dir, entry.Name())); err != nil {
				failures = append(failures, err)
			}
		}
	}
	return errors.Join(failures...)
}

func copyFile(src, dst string) error {
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	// a FIFO or device would block the copy forever
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%w: %s is not a regular file", ErrInvalidSource, src)
	}
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
	rows, err := db.Query(`SELECT name, value, host, path, expiry, originAttributes FROM moz_cookies`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Cookie
	for rows.Next() {
		var cookie Cookie
		var expiry int64
		var originAttributes string
		if err := rows.Scan(
			&cookie.Name,
			&cookie.Value,
			&cookie.Host,
			&cookie.Path,
			&expiry,
			&originAttributes,
		); err != nil {
			return nil, err
		}
		cookie.Expires = geckoExpiry(expiry)
		cookie.Container = geckoContainer(originAttributes)
		cookie.Partitioned = strings.Contains(originAttributes, "partitionKey=")
		if hostMatches(cookie.Host, suffixes) {
			result = append(result, cookie)
		}
	}
	return result, rows.Err()
}

func geckoExpiry(value int64) time.Time {
	if value <= 0 {
		return time.Time{}
	}
	if value > 1e12 {
		return time.UnixMilli(value)
	}
	return time.Unix(value, 0)
}

func geckoContainer(originAttributes string) string {
	for attribute := range strings.SplitSeq(strings.TrimPrefix(originAttributes, "^"), "&") {
		name, value, ok := strings.Cut(attribute, "=")
		if ok && name == "userContextId" {
			return value
		}
	}
	return ""
}

func readChromium(db *sql.DB, st Store, suffixes []string) ([]Cookie, error) {
	hasPartitionKey, err := tableHasColumn(db, "cookies", "top_frame_site_key")
	if err != nil {
		return nil, err
	}
	query := `SELECT name, value, host_key, path, expires_utc, encrypted_value, '' FROM cookies`
	if hasPartitionKey {
		query = `SELECT name, value, host_key, path, expires_utc, encrypted_value, top_frame_site_key FROM cookies`
	}
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	v10 := chromiumKey("peanuts")
	var v11 []byte
	v11Tried := false
	var result []Cookie
	for rows.Next() {
		var cookie Cookie
		var expiry int64
		var encrypted []byte
		var topFrameSiteKey sql.NullString
		if err := rows.Scan(
			&cookie.Name,
			&cookie.Value,
			&cookie.Host,
			&cookie.Path,
			&expiry,
			&encrypted,
			&topFrameSiteKey,
		); err != nil {
			return nil, err
		}
		if !hostMatches(cookie.Host, suffixes) {
			continue
		}
		if cookie.Value == "" && len(encrypted) >= 3 {
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
				cookie.Value, _ = decryptChromium(encrypted[3:], key, cookie.Host)
			}
		}
		if cookie.Value != "" {
			cookie.Expires = chromiumExpiry(expiry)
			cookie.Partitioned = topFrameSiteKey.Valid && topFrameSiteKey.String != ""
			result = append(result, cookie)
		}
	}
	return result, rows.Err()
}

func tableHasColumn(db *sql.DB, table, column string) (bool, error) {
	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, notNull, primaryKey int
		var name, columnType string
		var defaultValue any
		if err := rows.Scan(&id, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}

func chromiumExpiry(value int64) time.Time {
	if value == 0 {
		return time.Time{}
	}
	const unixEpochInChromiumMicroseconds = int64(11_644_473_600_000_000)
	unixMicroseconds := value - unixEpochInChromiumMicroseconds
	return time.Unix(unixMicroseconds/1_000_000, unixMicroseconds%1_000_000*1_000)
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
