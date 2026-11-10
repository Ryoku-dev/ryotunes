package youtube

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
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
	// Resolve validates a remembered source before it is read.
	Resolve(source browsercookies.SessionSource) (browsercookies.Store, error)
	Read(store browsercookies.Store, hostSuffixes ...string) ([]browsercookies.Cookie, error)
	ChangedSince(store browsercookies.Store, since time.Time) bool
}

type systemCookies struct{}

func (systemCookies) Discover() []browsercookies.Store { return browsercookies.Discover() }
func (systemCookies) Resolve(source browsercookies.SessionSource) (browsercookies.Store, error) {
	return browsercookies.ValidateSource(source)
}
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
		account, err := c.Account(ctx)
		if err == nil && account.SignedIn {
			return SignInFlow{}, nil
		}
		var readErr sourceReadError
		if err != nil && !definitiveSessionFailure(err) && !errors.As(err, &readErr) {
			return SignInFlow{}, err
		}
	}
	c.signInMu.Lock()
	if c.signingIn {
		c.signInMu.Unlock()
		return SignInFlow{URL: youtubeSignInURL}, nil
	}
	c.signingIn = true
	c.signInMu.Unlock()

	// sessions YouTube refused stay refused for the life of this flow
	refused := make(map[string]bool)
	stores := c.cookies.Discover()
	signedIn, failures := c.tryCookieCandidates(ctx, buildYouTubeCookieCandidates(c.cookies, stores, time.Now()), refused)
	if signedIn {
		c.signInMu.Lock()
		c.signingIn = false
		c.signInMu.Unlock()
		return SignInFlow{}, nil
	}
	c.warnCookieCandidateFailures(failures, nil)

	done := make(chan error, 1)
	flowCtx, cancel := context.WithCancel(c.ctx)
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		defer cancel()
		err := c.watchSignIn(flowCtx, stores, refused)
		c.signInMu.Lock()
		c.signingIn = false
		c.signInMu.Unlock()
		done <- err
		close(done)
	}()
	return SignInFlow{URL: youtubeSignInURL, Done: done}, nil
}

type cookieCandidateFailure struct {
	candidate cookieCandidate
	err       error
}

func (c *Client) tryCookieCandidates(ctx context.Context, candidates []cookieCandidate, refused map[string]bool) (bool, []cookieCandidateFailure) {
	attemptCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	failures := make([]cookieCandidateFailure, 0, len(candidates))
	for _, candidate := range candidates {
		if refused[candidate.header] {
			continue
		}
		source := browsercookies.SessionSource{Kind: candidate.store.Kind, Path: candidate.store.Path, Container: candidate.container}
		err := c.applyCookie(attemptCtx, candidate.header, source)
		if err == nil {
			return true, failures
		}
		if definitiveSessionFailure(err) {
			refused[candidate.header] = true
		}
		failures = append(failures, cookieCandidateFailure{candidate: candidate, err: err})
		if attemptCtx.Err() != nil {
			break
		}
	}
	return false, failures
}

func (c *Client) warnCookieCandidateFailures(failures []cookieCandidateFailure, seenReasons map[string]struct{}) {
	for _, failure := range failures {
		reason := failure.err.Error()
		if seenReasons != nil {
			if _, seen := seenReasons[reason]; seen {
				continue
			}
			seenReasons[reason] = struct{}{}
		}
		c.log.Warn(
			"YouTube browser cookie import failed",
			"store", failure.candidate.store.Path,
			"container", failure.candidate.container,
			"error", failure.err,
		)
	}
}

func (c *Client) watchSignIn(ctx context.Context, stores []browsercookies.Store, refused map[string]bool) error {
	deadline := time.NewTimer(5 * time.Minute)
	defer deadline.Stop()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	seenModifications := make(map[string]time.Time)
	for _, store := range stores {
		seenModifications[store.Path] = cookieStoreModTime(store)
	}
	loggedReasons := make(map[string]struct{})
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
				if c.cookies.ChangedSince(store, seenModifications[store.Path]) {
					changed = true
					seenModifications[store.Path] = cookieStoreModTime(store)
				}
			}
			if !changed {
				continue
			}
			signedIn, failures := c.tryCookieCandidates(ctx, buildYouTubeCookieCandidates(c.cookies, stores, time.Now()), refused)
			if signedIn {
				return nil
			}
			c.warnCookieCandidateFailures(failures, loggedReasons)
			if err := ctx.Err(); err != nil {
				return err
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

func (c *Client) applyCookie(ctx context.Context, cookie string, source browsercookies.SessionSource) error {
	if sapisidCookie(cookie) == "" {
		return errors.New("Sign-in didn't complete, try signing in again.")
	}
	account, dataSync, visitor, err := c.fetchAccountWithCookie(ctx, cookie, "")
	if err != nil {
		return err
	}
	if account.Name == nil {
		return fmt.Errorf("%w: sign in again", errSessionRejected)
	}
	identity := ""
	identities, err := c.candidateIdentities(ctx, cookie)
	c.commitMu.Lock()
	defer c.commitMu.Unlock()
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
		// the account this session is using, for re-adoption when the account
		// menu carries no channel, email or handle; a browser signed in to two
		// Google accounts lists both, so only the server's selection counts
		for _, row := range identities.rows {
			if row.serverSelected {
				identity = row.public.SelectionKey
				break
			}
		}
	}
	if err := c.persistAccount(cookie, dataSync, visitor, account); err != nil {
		return err
	}
	if err := c.persistIdentity(identity); err != nil {
		return err
	}
	return c.persistCookieSource(source)
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
	if c.cookie != cookie {
		// a ping minted under another session must not go out with this one
		clear(c.playbackPings)
	}
	c.cookie, c.dataSync, c.account = cookie, dataSync, account
	if visitor != "" {
		c.visitor = visitor
	}
	c.mu.Unlock()
	return nil
}

// SignOut clears every persisted YouTube session projection.
func (c *Client) SignOut() error {
	c.commitMu.Lock()
	defer c.commitMu.Unlock()
	var failures []error
	for _, key := range []string{cookieKey, cookieSourceKey, dataSyncKey, accountKey, identityKey, selectionKey, pendingKey} {
		if err := c.kv.Delete(key); err != nil {
			failures = append(failures, err)
		}
	}
	c.mu.Lock()
	c.cookie, c.dataSync, c.identity = "", "", ""
	clear(c.playbackPings)
	c.cookieSource = nil
	c.account = model.Account{}
	c.mu.Unlock()
	return errors.Join(failures...)
}

// Account asks the account menu for the current account. It re-reads the
// remembered browser store only when the menu reports no active account.
func (c *Client) Account(ctx context.Context) (model.Account, error) {
	c.mu.RLock()
	cookie, cached := c.cookie, c.account
	c.mu.RUnlock()
	if cookie == "" {
		return model.Account{}, nil
	}
	account, dataSync, visitor, err := c.fetchAccount(ctx, "")
	if err != nil {
		if definitiveSessionFailure(err) {
			// do has already forgotten the rejected session
			return c.CurrentAccount(), err
		}
		if cached.SignedIn {
			return cached, err
		}
		return model.Account{}, err
	}
	if account.Name == nil {
		changed, refreshErr := c.refreshFromSource(ctx, cookie)
		if changed {
			c.mu.RLock()
			cookie = c.cookie
			c.mu.RUnlock()
			account, dataSync, visitor, err = c.fetchAccount(ctx, "")
			if err != nil {
				return c.CurrentAccount(), err
			}
		} else if ctxErr := ctx.Err(); ctxErr != nil {
			return cached, ctxErr
		}
		if account.Name == nil {
			// the menu itself proves this cookie has no account
			if _, err := c.markSessionRejected(cookie); err != nil {
				return model.Account{}, err
			}
			return c.CurrentAccount(), sessionExpiredError(refreshErr)
		}
	}
	account.CanSwitch = cached.CanSwitch
	c.commitMu.Lock()
	defer c.commitMu.Unlock()
	c.mu.RLock()
	current, storedDataSync := c.cookie, c.dataSync
	c.mu.RUnlock()
	if current != cookie {
		return c.CurrentAccount(), nil
	}
	if dataSync == "" {
		dataSync = storedDataSync
	}
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
	// serverSelected is isSelected from the response, not our stored choice.
	serverSelected bool
}

type identityResult struct {
	public model.AccountIdentities
	rows   []identityRow
}

func (c *Client) accountIdentities(ctx context.Context) (identityResult, error) {
	return c.listIdentities(ctx, map[string]any{})
}

// candidateIdentities lists the accounts of a session being proven or
// imported, without acting as the stored session's channel, so the server's
// selection is the account that session itself uses.
func (c *Client) candidateIdentities(ctx context.Context, cookie string) (identityResult, error) {
	return c.listIdentities(withSessionOverride(ctx, cookie), map[string]any{"__dataSyncOverride": ""})
}

func (c *Client) listIdentities(ctx context.Context, body map[string]any) (identityResult, error) {
	root, err := c.do(ctx, "WEB_REMIX", "account/accounts_list", body, true)
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
		result.rows = append(result.rows, identityRow{public: row, dataSync: dataSync, serverSelected: findBool(item, "isSelected")})
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
	c.commitMu.Lock()
	defer c.commitMu.Unlock()
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
	if err := c.persistIdentity(chosen.public.SelectionKey); err != nil {
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
	return c.fetchAccountMenu(ctx, body)
}

// fetchAccountWithoutChannel asks the account menu for cookie's own account,
// not acting as any remembered channel.
func (c *Client) fetchAccountWithoutChannel(ctx context.Context, cookie string) (model.Account, string, string, error) {
	return c.fetchAccountMenu(withSessionOverride(ctx, cookie), map[string]any{"__dataSyncOverride": ""})
}

func (c *Client) fetchAccountMenu(ctx context.Context, body map[string]any) (model.Account, string, string, error) {
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
