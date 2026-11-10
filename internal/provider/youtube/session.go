package youtube

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"reflect"
	"strings"
	"time"

	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
	"github.com/ryoku-dev/ryotunes/internal/provider/browsercookies"
)

var (
	errSessionRejected         = errors.New("YouTube Music rejected the session")
	errNoBrowserSession        = errors.New("the remembered browser session is signed out")
	errDifferentBrowserAccount = errors.New("the remembered browser store now contains a different YouTube account")
)

type sourceReadError struct{ err error }

func (e sourceReadError) Error() string { return "read remembered browser session: " + e.err.Error() }
func (e sourceReadError) Unwrap() error { return e.err }

type sessionOverrideKey struct{}

type sessionOverride struct{ cookie string }

// legacyMiss remembers a failed source lookup for a session that predates
// remembered sources, so it is not repeated until a store changes.
type legacyMiss struct {
	cookie string
	at     time.Time
	stores []browsercookies.Store
}

func withSessionOverride(ctx context.Context, cookie string) context.Context {
	return context.WithValue(ctx, sessionOverrideKey{}, sessionOverride{cookie: cookie})
}

func (c *Client) fetchAccountWithCookie(ctx context.Context, cookie, dataSync string) (model.Account, string, string, error) {
	return c.fetchAccount(withSessionOverride(ctx, cookie), dataSync)
}

// CurrentAccount returns the account the daemon holds, without network or
// browser work.
func (c *Client) CurrentAccount() model.Account {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.account
}

// Restore follows the remembered browser once at daemon start: it adopts the
// browser's current cookies when they prove the same account, then confirms
// the session with the account menu. Changes are announced on the bus.
func (c *Client) Restore(ctx context.Context) error {
	c.mu.RLock()
	signedIn := c.cookie != ""
	c.mu.RUnlock()
	if !signedIn {
		return nil
	}
	if _, err := c.refreshFromSource(ctx, ""); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		// the stored cookie may still work; the account menu decides
		c.log.Info("YouTube browser session was not re-read", "error", err)
	}
	before := c.CurrentAccount()
	account, err := c.Account(ctx)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(before, account) {
		c.emitAccount(account)
	}
	return nil
}

// refreshFromSource re-reads the remembered browser store and adopts its
// cookies when they prove the stored account. rejected is the cookie a
// provider just refused; when another caller already replaced it, that
// counts as a change.
func (c *Client) refreshFromSource(ctx context.Context, rejected string) (bool, error) {
	c.refreshMu.Lock()
	defer c.refreshMu.Unlock()

	c.mu.RLock()
	cookie := c.cookie
	source := c.cookieSource
	storedAccount := c.account
	dataSync := c.dataSync
	identity := c.identity
	c.mu.RUnlock()
	if cookie == "" {
		return false, errNoBrowserSession
	}
	if rejected != "" && cookie != rejected {
		return true, nil
	}
	if source == nil {
		matched, err := c.findLegacyCookieSource(cookie)
		if err != nil {
			return false, err
		}
		source = matched
	}
	candidate, err := c.cookieCandidateFromSource(*source)
	if err != nil {
		return false, err
	}
	if candidate.header == cookie {
		return false, nil
	}
	account, refreshedDataSync, visitor, err := c.fetchAccountWithCookie(ctx, candidate.header, dataSync)
	if err != nil {
		return false, err
	}
	if account.Name == nil {
		return false, errNoBrowserSession
	}
	same, err := c.provesSameAccount(ctx, storedAccount, identity, account, candidate.header)
	if err != nil {
		return false, err
	}
	if !same {
		return false, errDifferentBrowserAccount
	}
	if refreshedDataSync == "" {
		refreshedDataSync = dataSync
	}
	account.CanSwitch = storedAccount.CanSwitch
	c.commitMu.Lock()
	defer c.commitMu.Unlock()
	c.mu.RLock()
	current := c.cookie
	c.mu.RUnlock()
	if current != cookie {
		// signed out or replaced while the browser's cookie was being proven
		if current == "" {
			return false, errNoBrowserSession
		}
		return true, nil
	}
	if err := c.persistAccount(candidate.header, refreshedDataSync, visitor, account); err != nil {
		return false, err
	}
	if err := c.persistCookieSource(*source); err != nil {
		return false, err
	}
	c.emitAccount(account)
	return true, nil
}

// provesSameAccount checks the candidate session against the stored account.
// The identity fallback compares only the account the candidate is using
// now: a browser signed in to two Google accounts lists both in its switcher.
func (c *Client) provesSameAccount(ctx context.Context, stored model.Account, identity string, fresh model.Account, cookie string) (bool, error) {
	if same, known := sameYouTubeAccount(stored, fresh); known {
		return same, nil
	}
	if identity == "" {
		return false, nil
	}
	identities, err := c.candidateIdentities(ctx, cookie)
	if err != nil {
		return false, err
	}
	for _, row := range identities.rows {
		if row.serverSelected {
			return row.public.SelectionKey == identity, nil
		}
	}
	return false, nil
}

func (c *Client) findLegacyCookieSource(cookie string) (*browsercookies.SessionSource, error) {
	wanted := sapisidCookie(cookie)
	if wanted == "" {
		return nil, errNoBrowserSession
	}
	if miss := c.legacyMiss; miss != nil && miss.cookie == cookie {
		changed := false
		for _, store := range miss.stores {
			if c.cookies.ChangedSince(store, miss.at) {
				changed = true
				break
			}
		}
		if !changed {
			return nil, errNoBrowserSession
		}
	}
	started := time.Now()
	stores := c.cookies.Discover()
	for _, candidate := range buildYouTubeCookieCandidates(c.cookies, stores, started) {
		if sapisidCookie(candidate.header) != wanted {
			continue
		}
		source := browsercookies.SessionSource{Kind: candidate.store.Kind, Path: candidate.store.Path, Container: candidate.container}
		if err := c.persistCookieSourceFor(cookie, source); err != nil {
			return nil, err
		}
		c.legacyMiss = nil
		return &source, nil
	}
	c.legacyMiss = &legacyMiss{cookie: cookie, at: started, stores: stores}
	return nil, errNoBrowserSession
}

func (c *Client) cookieCandidateFromSource(source browsercookies.SessionSource) (cookieCandidate, error) {
	store, err := c.cookies.Resolve(source)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) || errors.Is(err, browsercookies.ErrInvalidSource) {
			return cookieCandidate{}, fmt.Errorf("%w: %w", errNoBrowserSession, err)
		}
		return cookieCandidate{}, sourceReadError{err: err}
	}
	cookies, err := c.cookies.Read(store, "youtube.com")
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return cookieCandidate{}, fmt.Errorf("%w: %w", errNoBrowserSession, err)
		}
		return cookieCandidate{}, sourceReadError{err: err}
	}
	var selected []browsercookies.Cookie
	now := time.Now()
	for _, cookie := range cookies {
		if cookie.Container == source.Container && youtubeCookieEligible(cookie, now) {
			selected = append(selected, cookie)
		}
	}
	header := youtubeCookieHeader(selected)
	if sapisidCookie(header) == "" {
		return cookieCandidate{}, errNoBrowserSession
	}
	return cookieCandidate{store: store, container: source.Container, header: header}, nil
}

func (c *Client) persistCookieSource(source browsercookies.SessionSource) error {
	raw, err := json.Marshal(source)
	if err != nil {
		return err
	}
	if err := c.kv.Set(cookieSourceKey, string(raw)); err != nil {
		return err
	}
	c.mu.Lock()
	copy := source
	c.cookieSource = &copy
	c.mu.Unlock()
	return nil
}

// persistCookieSourceFor remembers source only while cookie is still the
// session, so a sign out during the lookup stays complete.
func (c *Client) persistCookieSourceFor(cookie string, source browsercookies.SessionSource) error {
	c.commitMu.Lock()
	defer c.commitMu.Unlock()
	c.mu.RLock()
	current := c.cookie
	c.mu.RUnlock()
	if current != cookie {
		return errNoBrowserSession
	}
	return c.persistCookieSource(source)
}

func (c *Client) persistIdentity(identity string) error {
	if identity == "" {
		if err := c.kv.Delete(identityKey); err != nil {
			return err
		}
	} else if err := c.kv.Set(identityKey, identity); err != nil {
		return err
	}
	c.mu.Lock()
	c.identity = identity
	c.mu.Unlock()
	return nil
}

// markSessionRejected forgets the session the provider refused and reports
// whether it did; a cookie another caller already replaced is kept. The
// remembered source stays so the next sign in can follow the same browser.
func (c *Client) markSessionRejected(rejected string) (bool, error) {
	c.refreshMu.Lock()
	defer c.refreshMu.Unlock()
	c.commitMu.Lock()
	defer c.commitMu.Unlock()
	c.mu.Lock()
	if c.cookie != rejected {
		c.mu.Unlock()
		return false, nil
	}
	c.cookie = ""
	clear(c.playbackPings)
	c.dataSync = ""
	c.identity = ""
	c.account = model.Account{}
	c.mu.Unlock()
	var failures []error
	for _, key := range []string{cookieKey, dataSyncKey, accountKey, identityKey, selectionKey, pendingKey} {
		if err := c.kv.Delete(key); err != nil {
			failures = append(failures, err)
		}
	}
	c.emitAccount(model.Account{})
	if err := errors.Join(failures...); err != nil {
		return true, fmt.Errorf("forget rejected YouTube session: %w", err)
	}
	return true, nil
}

func (c *Client) emitAccount(account model.Account) {
	c.bus.Emit(protocol.EventAuthChanged, account)
}

// sameYouTubeAccount compares the strongest identity both projections carry:
// channel id, then email, then handle. Each is unique to one account, so the
// first one present on both sides decides. A display name never proves
// identity.
func sameYouTubeAccount(stored, fresh model.Account) (same, known bool) {
	if left, right := trimmed(stored.ChannelID), trimmed(fresh.ChannelID); left != "" && right != "" {
		return left == right, true
	}
	for _, pair := range [][2]*string{{stored.Email, fresh.Email}, {stored.Handle, fresh.Handle}} {
		if left, right := trimmed(pair[0]), trimmed(pair[1]); left != "" && right != "" {
			return strings.EqualFold(left, right), true
		}
	}
	return false, false
}

func trimmed(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func definitiveSessionFailure(err error) bool {
	return errors.Is(err, errSessionRejected) || errors.Is(err, errNoBrowserSession) || errors.Is(err, errDifferentBrowserAccount)
}

func sessionExpiredError(cause error) error {
	if cause == nil {
		return fmt.Errorf("%w: sign in again", errSessionRejected)
	}
	if errors.Is(cause, errSessionRejected) {
		return cause
	}
	return fmt.Errorf("%w: %v", errSessionRejected, cause)
}
