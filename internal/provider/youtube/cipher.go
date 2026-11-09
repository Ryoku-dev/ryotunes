package youtube

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dop251/goja"
)

const playerUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"

var (
	playerHashPatterns = []*regexp.Regexp{
		regexp.MustCompile(`player\\?/([0-9A-Za-z_-]+)\\?/`),
		regexp.MustCompile(`/s/player/([0-9A-Za-z_-]+)/`),
	}
	stsPattern      = regexp.MustCompile(`(?:signatureTimestamp|sts)"?\s*[:=]\s*(\d{4,})`)
	playerHashValid = regexp.MustCompile(`^[0-9A-Za-z_-]+$`)
	sigNamePatterns = []*regexp.Regexp{
		regexp.MustCompile(`\.sig\|\|([A-Za-z_$][\w$]*)\(`),
		regexp.MustCompile(`["']signature["']\s*,\s*([A-Za-z_$][\w$]*)\(`),
		regexp.MustCompile(`\.set\([^,]+,\s*([A-Za-z_$][\w$]*)\(`),
	}
	nNamePatterns = []*regexp.Regexp{
		regexp.MustCompile(`\.get\(["']n["']\)\)\s*&&\s*\([^=]+\s*=\s*([A-Za-z_$][\w$]*)\(`),
		regexp.MustCompile(`\bn\s*&&\s*\([^=]+\s*=\s*([A-Za-z_$][\w$]*)\(`),
	}
)

type cipherState struct {
	mu        sync.Mutex
	hash      string
	js        string
	loadedAt  time.Time
	sts       int
	sigSource string
	nSource   string
}

func (c *Client) signatureTimestamp(ctx context.Context) (int, error) {
	if err := c.ensurePlayerJS(ctx, false); err != nil {
		return 0, err
	}
	c.cipher.mu.Lock()
	defer c.cipher.mu.Unlock()
	return c.cipher.sts, nil
}

func (c *Client) decipherURL(ctx context.Context, encoded string) (string, error) {
	values, err := url.ParseQuery(encoded)
	if err != nil {
		return "", err
	}
	rawURL, signature := values.Get("url"), values.Get("s")
	if rawURL == "" || signature == "" {
		return "", errors.New("youtube: incomplete signatureCipher")
	}
	deciphered, err := c.runCipher(ctx, true, signature, false)
	if err != nil {
		return "", err
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	query := parsed.Query()
	key := values.Get("sp")
	if key == "" {
		key = "signature"
	}
	query.Set(key, deciphered)
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func (c *Client) transformN(ctx context.Context, rawURL string) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	query := parsed.Query()
	n := query.Get("n")
	if n == "" {
		return rawURL, nil
	}
	changed, err := c.runCipher(ctx, false, n, false)
	if err != nil {
		return rawURL, nil
	}
	if changed == "" {
		return rawURL, nil
	}
	query.Set("n", changed)
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func (c *Client) runCipher(ctx context.Context, signature bool, input string, retried bool) (string, error) {
	if err := c.ensurePlayerJS(ctx, retried); err != nil {
		return "", err
	}
	c.cipher.mu.Lock()
	source := c.cipher.nSource
	name := "_n"
	if signature {
		source, name = c.cipher.sigSource, "_sig"
	}
	c.cipher.mu.Unlock()
	if source == "" {
		return "", errors.New("youtube: player JS transform not found")
	}
	vm := goja.New()
	timer := time.AfterFunc(5*time.Second, func() { vm.Interrupt("cipher timeout") })
	defer timer.Stop()
	if _, err := vm.RunString(source); err != nil {
		if !retried {
			c.invalidatePlayerJS()
			return c.runCipher(ctx, signature, input, true)
		}
		return "", fmt.Errorf("youtube cipher: %w", err)
	}
	fn, ok := goja.AssertFunction(vm.Get(name))
	if !ok {
		return "", errors.New("youtube: transform export missing")
	}
	result, err := fn(goja.Undefined(), vm.ToValue(input))
	if err != nil {
		if !retried {
			c.invalidatePlayerJS()
			return c.runCipher(ctx, signature, input, true)
		}
		return "", fmt.Errorf("youtube cipher: %w", err)
	}
	return result.String(), nil
}

func (c *Client) ensurePlayerJS(ctx context.Context, force bool) error {
	c.cipher.mu.Lock()
	defer c.cipher.mu.Unlock()
	if !force && c.cipher.js != "" && time.Since(c.cipher.loadedAt) < 6*time.Hour {
		return nil
	}
	cacheDir := c.cachePath("cipher_cache")
	if !force {
		if hashBytes, err := os.ReadFile(filepath.Join(cacheDir, "current_hash.txt")); err == nil {
			hash := strings.TrimSpace(string(hashBytes))
			if playerHashValid.MatchString(hash) {
				path := filepath.Join(cacheDir, "player_"+hash+".js")
				if info, err := os.Stat(path); err == nil && time.Since(info.ModTime()) < 6*time.Hour {
					if data, err := os.ReadFile(path); err == nil {
						return c.analyzePlayerJS(hash, string(data), info.ModTime())
					}
				}
			}
		}
	}
	hash, js, err := c.fetchPlayerJS(ctx)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(cacheDir, 0o700); err == nil {
		_ = os.WriteFile(filepath.Join(cacheDir, "current_hash.txt"), []byte(hash+"\n"), 0o600)
		_ = os.WriteFile(filepath.Join(cacheDir, "player_"+hash+".js"), []byte(js), 0o600)
	}
	return c.analyzePlayerJS(hash, js, time.Now())
}

func (c *Client) analyzePlayerJS(hash, js string, loaded time.Time) error {
	c.cipher.hash, c.cipher.js, c.cipher.loadedAt = hash, js, loaded
	if match := stsPattern.FindStringSubmatch(js); len(match) == 2 {
		c.cipher.sts, _ = strconv.Atoi(match[1])
	}
	if name := firstPatternName(js, sigNamePatterns); name != "" {
		c.cipher.sigSource = transformSource(js, name, "_sig")
	}
	if name := firstPatternName(js, nNamePatterns); name != "" {
		c.cipher.nSource = transformSource(js, name, "_n")
	}
	if c.cipher.sts == 0 {
		return errors.New("youtube: signatureTimestamp not found")
	}
	return nil
}

func firstPatternName(js string, patterns []*regexp.Regexp) string {
	for _, pattern := range patterns {
		if match := pattern.FindStringSubmatch(js); len(match) == 2 {
			return match[1]
		}
	}
	return ""
}

func transformSource(js, name, export string) string {
	declaration := extractDeclaration(js, name)
	if declaration == "" {
		return ""
	}
	var helpers strings.Builder
	seen := make(map[string]bool)
	for _, match := range regexp.MustCompile(`([A-Za-z_$][\w$]*)\.`).FindAllStringSubmatch(declaration, -1) {
		helper := match[1]
		if helper == name || seen[helper] {
			continue
		}
		if source := extractDeclaration(js, helper); source != "" {
			seen[helper] = true
			helpers.WriteString(source)
			helpers.WriteByte(';')
		}
	}
	return helpers.String() + declaration + ";var " + export + "=" + name + ";"
}

func extractDeclaration(js, name string) string {
	quoted := regexp.QuoteMeta(name)
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?:var\s+)?` + quoted + `\s*=\s*function\s*\([^)]*\)\s*\{`),
		regexp.MustCompile(`function\s+` + quoted + `\s*\([^)]*\)\s*\{`),
		regexp.MustCompile(`(?:var\s+)?` + quoted + `\s*=\s*\{`),
	}
	for _, pattern := range patterns {
		loc := pattern.FindStringIndex(js)
		if loc == nil {
			continue
		}
		brace := strings.IndexByte(js[loc[0]:loc[1]], '{') + loc[0]
		end := matchingBrace(js, brace)
		if end < 0 {
			continue
		}
		start := loc[0]
		if strings.HasPrefix(js[start:], "function ") {
			return js[start : end+1]
		}
		source := strings.TrimSpace(js[start : end+1])
		source = strings.TrimPrefix(source, "var ")
		return "var " + source
	}
	return ""
}

func matchingBrace(source string, start int) int {
	depth := 0
	quote := byte(0)
	escaped := false
	for i := start; i < len(source); i++ {
		ch := source[i]
		if quote != 0 {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == quote {
				quote = 0
			}
			continue
		}
		if ch == '\'' || ch == '"' || ch == '`' {
			quote = ch
			continue
		}
		switch ch {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func (c *Client) fetchPlayerJS(ctx context.Context) (string, string, error) {
	iframe, err := c.fetchText(ctx, "https://www.youtube.com/iframe_api")
	if err != nil {
		return "", "", err
	}
	hash := firstPatternName(iframe, playerHashPatterns)
	if hash == "" {
		return "", "", errors.New("youtube: player hash not found")
	}
	js, err := c.fetchText(ctx, "https://www.youtube.com/s/player/"+hash+"/player_ias.vflset/en_GB/base.js")
	return hash, js, err
}

func (c *Client) fetchText(ctx context.Context, target string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", playerUserAgent)
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return "", fmt.Errorf("youtube player JS: %s", resp.Status)
	}
	data, err := io.ReadAll(resp.Body)
	return string(data), err
}

func (c *Client) invalidatePlayerJS() {
	c.cipher.mu.Lock()
	defer c.cipher.mu.Unlock()
	hash := c.cipher.hash
	c.cipher.js, c.cipher.sigSource, c.cipher.nSource = "", "", ""
	_ = os.Remove(c.cachePath("cipher_cache", "current_hash.txt"))
	if hash != "" {
		_ = os.Remove(c.cachePath("cipher_cache", "player_"+hash+".js"))
	}
}
