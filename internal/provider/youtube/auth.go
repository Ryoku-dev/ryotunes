package youtube

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/provider"
	"github.com/ryoku-dev/ryotunes/internal/provider/browsercookies"
)

const youtubeSignInURL = "https://music.youtube.com"

// CookieSource supplies browser cookie stores without touching the live browser.
type CookieSource interface {
	Discover() []browsercookies.Store
	Read(store browsercookies.Store, hostSuffixes ...string) ([]browsercookies.Cookie, error)
	ChangedSince(store browsercookies.Store, since time.Time) bool
}

type systemCookies struct{}

func (systemCookies) Discover() []browsercookies.Store { return browsercookies.Discover() }
func (systemCookies) Read(store browsercookies.Store, suffixes ...string) ([]browsercookies.Cookie, error) {
	return browsercookies.Read(store, suffixes...)
}
func (systemCookies) ChangedSince(store browsercookies.Store, since time.Time) bool {
	return browsercookies.ChangedSince(store, since)
}

// SignInFlow reports the client-owned browser handoff and asynchronous completion.
type SignInFlow struct {
	URL  string
	Done <-chan error
}

// SignIn imports a YouTube browser session or starts watching for one.
func (c *Client) SignIn(ctx context.Context) (SignInFlow, error) {
	c.mu.RLock()
	signedIn := c.cookie != ""
	c.mu.RUnlock()
	if signedIn {
		return SignInFlow{}, nil
	}
	c.signInMu.Lock()
	if c.signingIn {
		c.signInMu.Unlock()
		return SignInFlow{URL: youtubeSignInURL}, nil
	}
	c.signingIn = true
	c.signInMu.Unlock()

	stores := c.cookies.Discover()
	if header := harvestYouTubeCookies(c.cookies, stores); sapisidCookie(header) != "" {
		err := c.applyCookie(ctx, header)
		c.signInMu.Lock()
		c.signingIn = false
		c.signInMu.Unlock()
		return SignInFlow{}, err
	}

	done := make(chan error, 1)
	flowCtx, cancel := context.WithTimeout(c.ctx, 5*time.Minute)
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		defer cancel()
		err := c.watchSignIn(flowCtx, stores)
		c.signInMu.Lock()
		c.signingIn = false
		c.signInMu.Unlock()
		done <- err
		close(done)
	}()
	return SignInFlow{URL: youtubeSignInURL, Done: done}, nil
}

func (c *Client) watchSignIn(ctx context.Context, stores []browsercookies.Store) error {
	deadline := time.NewTimer(5 * time.Minute)
	defer deadline.Stop()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	seen := make(map[string]time.Time)
	for _, store := range stores {
		seen[store.Path] = cookieStoreModTime(store)
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return errors.New("Sign-in didn't complete, try signing in again.")
		case <-ticker.C:
			stores = c.cookies.Discover()
			changed := false
			for _, store := range stores {
				if c.cookies.ChangedSince(store, seen[store.Path]) {
					changed = true
					seen[store.Path] = cookieStoreModTime(store)
				}
			}
			if changed {
				if header := harvestYouTubeCookies(c.cookies, stores); sapisidCookie(header) != "" {
					return c.applyCookie(ctx, header)
				}
			}
		}
	}
}

func cookieStoreModTime(store browsercookies.Store) time.Time {
	var latest time.Time
	for _, path := range []string{store.Path, store.Path + "-wal"} {
		if info, err := os.Stat(path); err == nil && info.ModTime().After(latest) {
			latest = info.ModTime()
		}
	}
	return latest
}

func harvestYouTubeCookies(source CookieSource, stores []browsercookies.Store) string {
	type value struct {
		cookie      browsercookies.Cookie
		specificity int
	}
	selected := make(map[string]value)
	for _, store := range stores {
		cookies, err := source.Read(store, "youtube.com")
		if err != nil {
			continue
		}
		for _, cookie := range cookies {
			if cookie.Name == "" {
				continue
			}
			specificity := len(strings.TrimPrefix(strings.ToLower(cookie.Host), "."))
			if old, ok := selected[cookie.Name]; !ok || specificity > old.specificity {
				selected[cookie.Name] = value{cookie: cookie, specificity: specificity}
			}
		}
	}
	names := make([]string, 0, len(selected))
	for name := range selected {
		names = append(names, name)
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, name := range names {
		parts = append(parts, name+"="+selected[name].cookie.Value)
	}
	return strings.Join(parts, "; ")
}

// ImportCookiesFile imports a Netscape cookies.txt session and validates it.
func (c *Client) ImportCookiesFile(path string) error {
	return c.ImportCookiesFileContext(context.Background(), path)
}

// ImportCookiesFileContext imports a Netscape cookies.txt session with cancellation.
func (c *Client) ImportCookiesFileContext(ctx context.Context, path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	var cookies []browsercookies.Cookie
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "#HttpOnly_") {
			line = strings.TrimPrefix(line, "#HttpOnly_")
		} else if strings.HasPrefix(line, "#") || strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) != 7 {
			continue
		}
		host, name, value := fields[0], fields[5], fields[6]
		normalized := strings.TrimPrefix(strings.ToLower(host), ".")
		if name != "" && (normalized == "youtube.com" || strings.HasSuffix(normalized, ".youtube.com")) {
			cookies = append(cookies, browsercookies.Cookie{Name: name, Value: value, Host: host})
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	sort.SliceStable(cookies, func(i, j int) bool {
		if cookies[i].Name == cookies[j].Name {
			return len(cookies[i].Host) < len(cookies[j].Host)
		}
		return cookies[i].Name < cookies[j].Name
	})
	values := make(map[string]string)
	for _, cookie := range cookies {
		values[cookie.Name] = cookie.Value
	}
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, name := range names {
		parts = append(parts, name+"="+values[name])
	}
	return c.applyCookie(ctx, strings.Join(parts, "; "))
}

func (c *Client) applyCookie(ctx context.Context, cookie string) error {
	if sapisidCookie(cookie) == "" {
		return errors.New("Sign-in didn't complete, try signing in again.")
	}
	c.mu.Lock()
	oldCookie, oldVisitor, oldDataSync := c.cookie, c.visitor, c.dataSync
	c.cookie, c.dataSync = cookie, ""
	c.mu.Unlock()
	account, dataSync, visitor, err := c.fetchAccount(ctx, "")
	if err != nil {
		c.mu.Lock()
		c.cookie, c.visitor, c.dataSync = oldCookie, oldVisitor, oldDataSync
		c.mu.Unlock()
		return err
	}
	if account.Name == nil {
		c.mu.Lock()
		c.cookie, c.visitor, c.dataSync = oldCookie, oldVisitor, oldDataSync
		c.mu.Unlock()
		return errors.New("That session didn't authenticate, sign in again.")
	}
	if visitor != "" {
		c.mu.Lock()
		c.visitor = visitor
		c.mu.Unlock()
	}
	identities, err := c.accountIdentities(ctx)
	if err == nil {
		account.CanSwitch = len(identities.rows) > 1
		if len(identities.rows) > 1 && dataSync == "" {
			account = model.Account{SignedIn: true, SelectionRequired: true, CanSwitch: true}
			_ = c.kv.Set(pendingKey, "true")
		} else {
			_ = c.kv.Delete(pendingKey)
			for _, row := range identities.rows {
				if row.public.Selected || dataSync != "" && row.dataSync == dataSync {
					selectedJSON, _ := json.Marshal(row.public)
					_ = c.kv.Set(selectionKey, string(selectedJSON))
					break
				}
			}
		}
	}
	if err := c.persistAccount(cookie, dataSync, visitor, account); err != nil {
		return err
	}
	return nil
}

func (c *Client) persistAccount(cookie, dataSync, visitor string, account model.Account) error {
	raw, err := json.Marshal(account)
	if visitor == "" {
		c.mu.RLock()
		visitor = c.visitor
		c.mu.RUnlock()
	}
	if err != nil {
		return err
	}
	for _, entry := range []struct{ key, value string }{{cookieKey, cookie}, {dataSyncKey, dataSync}, {visitorKey, visitor}, {accountKey, string(raw)}} {
		if entry.value == "" {
			if err := c.kv.Delete(entry.key); err != nil {
				return err
			}
		} else if err := c.kv.Set(entry.key, entry.value); err != nil {
			return err
		}
	}
	c.mu.Lock()
	c.cookie, c.dataSync, c.account = cookie, dataSync, account
	if visitor != "" {
		c.visitor = visitor
	}
	c.mu.Unlock()
	return nil
}

// SignOut clears every persisted YouTube session projection.
func (c *Client) SignOut() error {
	var failures []error
	for _, key := range []string{cookieKey, dataSyncKey, accountKey, selectionKey, pendingKey} {
		if err := c.kv.Delete(key); err != nil {
			failures = append(failures, err)
		}
	}
	c.mu.Lock()
	c.cookie, c.dataSync = "", ""
	c.account = model.Account{}
	c.mu.Unlock()
	return errors.Join(failures...)
}

// Account returns the current account projection, refreshing it when signed in.
func (c *Client) Account(ctx context.Context) (model.Account, error) {
	c.mu.RLock()
	cookie, cached := c.cookie, c.account
	c.mu.RUnlock()
	if cookie == "" {
		return model.Account{}, nil
	}
	account, dataSync, visitor, err := c.fetchAccount(ctx, "")
	if err != nil {
		if cached.SignedIn {
			return cached, err
		}
		return model.Account{}, err
	}
	if account.Name == nil {
		return model.Account{}, errors.New("That session didn't authenticate, sign in again.")
	}
	account.CanSwitch = cached.CanSwitch
	if err := c.persistAccount(cookie, dataSync, visitor, account); err != nil {
		return model.Account{}, err
	}
	return account, nil
}

// AccountIdentities returns renderer-safe identities and opaque selection keys.
func (c *Client) AccountIdentities(ctx context.Context) (model.AccountIdentities, error) {
	c.mu.RLock()
	signedIn := c.cookie != ""
	c.mu.RUnlock()
	if !signedIn {
		return nil, errors.New("Sign in before switching channels.")
	}
	result, err := c.accountIdentities(ctx)
	if err != nil {
		return nil, err
	}
	return result.public, nil
}

type identityRow struct {
	public   model.AccountIdentity
	dataSync string
}

type identityResult struct {
	public model.AccountIdentities
	rows   []identityRow
}

func (c *Client) accountIdentities(ctx context.Context) (identityResult, error) {
	root, err := c.do(ctx, "WEB_REMIX", "account/accounts_list", map[string]any{}, true)
	if err != nil {
		return identityResult{}, err
	}
	c.mu.RLock()
	selected := c.dataSync
	c.mu.RUnlock()
	var result identityResult
	walkMaps(root, func(m map[string]any) {
		item, ok := m["accountItem"].(map[string]any)
		if !ok || findBool(item, "isDisabled") {
			return
		}
		if hasChannel, ok := item["hasChannel"].(bool); ok && !hasChannel {
			return
		}
		dataSync := normalizeDataSync(findString(item, "datasyncIdToken", "dataSyncIdToken", "datasyncId", "dataSyncId", "pageId"))
		if dataSync == "" {
			return
		}
		name := deepText(item["accountName"])
		if name == "" {
			return
		}
		keyHash := sha256.Sum256([]byte(dataSync))
		row := model.AccountIdentity{SelectionKey: hex.EncodeToString(keyHash[:16]), Name: name, Selected: findBool(item, "isSelected") || dataSync == selected}
		setStringPtr(&row.Handle, deepText(item["channelHandle"]))
		setStringPtr(&row.Email, deepText(item["email"]))
		setStringPtr(&row.Thumbnail, deepThumbnail(item))
		channelID := findString(item, "browseId")
		if !strings.HasPrefix(channelID, "UC") {
			channelID = ""
		}
		setStringPtr(&row.ChannelID, channelID)
		result.public = append(result.public, row)
		result.rows = append(result.rows, identityRow{public: row, dataSync: dataSync})
	})
	return result, nil
}
func normalizeDataSync(value string) string {
	value = strings.TrimSpace(value)
	if before, after, ok := strings.Cut(value, "||"); ok {
		if before != "" {
			return before
		}
		return after
	}
	return value
}

// SwitchAccount validates and commits a server-issued account identity.
func (c *Client) SwitchAccount(ctx context.Context, selection string) error {
	c.mu.RLock()
	cookie := c.cookie
	c.mu.RUnlock()
	if cookie == "" {
		return errors.New("Sign in before switching channels.")
	}
	identities, err := c.accountIdentities(ctx)
	if err != nil {
		return fmt.Errorf("Couldn't switch YouTube channel: %w", err)
	}
	var chosen *identityRow
	for i := range identities.rows {
		if identities.rows[i].public.SelectionKey == selection {
			chosen = &identities.rows[i]
			break
		}
	}
	if chosen == nil {
		return errors.New("That YouTube channel is no longer available. Refresh the list and try again.")
	}
	account, dataSync, visitor, err := c.fetchAccount(ctx, chosen.dataSync)
	if err != nil {
		return fmt.Errorf("Couldn't switch YouTube channel: %w", err)
	}
	if account.Name == nil {
		return errors.New("YouTube did not confirm that channel. Try signing in again.")
	}
	if dataSync == "" {
		dataSync = chosen.dataSync
	}
	c.mu.RLock()
	sessionStillCurrent := c.cookie == cookie && cookie != ""
	c.mu.RUnlock()
	if !sessionStillCurrent {
		return errors.New("The YouTube session expired before the channel could be saved.")
	}
	account.CanSwitch = len(identities.rows) > 1
	if err := c.persistAccount(cookie, dataSync, visitor, account); err != nil {
		return fmt.Errorf("Couldn't save the selected YouTube channel: %w", err)
	}
	selectedJSON, _ := json.Marshal(chosen.public)
	if err := c.kv.Set(selectionKey, string(selectedJSON)); err != nil {
		return fmt.Errorf("Couldn't save the selected YouTube channel: %w", err)
	}
	_ = c.kv.Delete(pendingKey)
	return nil
}

func (c *Client) fetchAccount(ctx context.Context, candidate string) (model.Account, string, string, error) {
	body := map[string]any{}
	if candidate != "" {
		body["__dataSyncOverride"] = candidate
	}
	root, err := c.do(ctx, "WEB_REMIX", "account/account_menu", body, true)
	if err != nil {
		return model.Account{}, "", "", err
	}
	account := model.Account{SignedIn: true}
	walkMaps(root, func(m map[string]any) {
		if header, ok := m["activeAccountHeaderRenderer"].(map[string]any); ok && account.Name == nil {
			setStringPtr(&account.Name, deepText(header["accountName"]))
			setStringPtr(&account.Handle, deepText(header["channelHandle"]))
			setStringPtr(&account.Email, deepText(header["email"]))
			channelID := findString(header, "browseId")
			if strings.HasPrefix(channelID, "UC") {
				setStringPtr(&account.ChannelID, channelID)
			}
			setStringPtr(&account.Thumbnail, findThumbnail(header))
		}
	})
	dataSync := normalizeDataSync(findString(root, "datasyncId", "dataSyncId"))
	return account, dataSync, findString(root, "visitorData"), nil
}

var _ provider.Provider = (*Client)(nil)
var _ provider.Rater = (*Client)(nil)
var _ provider.AlbumSaver = (*Client)(nil)
var _ provider.PlaylistEditor = (*Client)(nil)
var _ provider.Lyricist = (*Client)(nil)
var _ provider.Radio = (*Client)(nil)
var _ provider.Grid = (*Client)(nil)
