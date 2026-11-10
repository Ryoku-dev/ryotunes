package soundcloud

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
	baseprovider "github.com/ryoku-dev/ryotunes/internal/provider"
	"github.com/ryoku-dev/ryotunes/internal/provider/browsercookies"
)

// legacyMiss remembers a failed source lookup for an auth that predates
// remembered sources, so it is not repeated until a store changes.
type legacyMiss struct {
	token  string
	at     time.Time
	stores []browsercookies.Store
}

var (
	errNoFreshBrowserAuth   = errors.New("the remembered SoundCloud browser session is signed out")
	errDifferentBrowserUser = errors.New("the remembered browser store now contains a different SoundCloud account")
)

type browserReadError struct{ err error }

func (e browserReadError) Error() string {
	return "read remembered SoundCloud browser session: " + e.err.Error()
}
func (e browserReadError) Unwrap() error { return e.err }

type soundCloudHTTPError struct {
	operation string
	status    int
}

func (e soundCloudHTTPError) Error() string {
	return fmt.Sprintf("SoundCloud answered %d for %s", e.status, e.operation)
}

type authCandidate struct {
	auth Auth
}

func (p *Provider) cookieAuthCandidates(stores []browsercookies.Store, changedAfter *time.Time) []authCandidate {
	var result []authCandidate
	for _, store := range stores {
		if changedAfter != nil && !p.cookies.ChangedSince(store, *changedAfter) {
			continue
		}
		cookies, err := p.cookies.Read(store, "soundcloud.com")
		if err != nil {
			continue
		}
		byContainer := make(map[string][]browsercookies.Cookie)
		for _, cookie := range cookies {
			if !cookie.Expires.IsZero() && !cookie.Expires.After(time.Now()) {
				continue
			}
			byContainer[cookie.Container] = append(byContainer[cookie.Container], cookie)
		}
		containers := make([]string, 0, len(byContainer))
		for container := range byContainer {
			containers = append(containers, container)
		}
		sort.Slice(containers, func(i, j int) bool {
			if containers[i] == "" {
				return containers[j] != ""
			}
			if containers[j] == "" {
				return false
			}
			return containers[i] < containers[j]
		})
		for _, container := range containers {
			var access string
			for _, cookie := range byContainer[container] {
				if cookie.Name == "oauth_token" {
					access = cookie.Value
				}
			}
			if strings.TrimSpace(access) == "" {
				continue
			}
			auth := authFromToken(access)
			auth.Source = &browsercookies.SessionSource{Kind: store.Kind, Path: store.Path, Container: container}
			result = append(result, authCandidate{auth: auth})
		}
	}
	return result
}

func (p *Provider) authFromSource(source browsercookies.SessionSource) (Auth, error) {
	store, err := p.cookies.Resolve(source)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) || errors.Is(err, browsercookies.ErrInvalidSource) {
			return Auth{}, fmt.Errorf("%w: %w", errNoFreshBrowserAuth, err)
		}
		return Auth{}, browserReadError{err: err}
	}
	cookies, err := p.cookies.Read(store, "soundcloud.com")
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Auth{}, fmt.Errorf("%w: %w", errNoFreshBrowserAuth, err)
		}
		return Auth{}, browserReadError{err: err}
	}
	var access string
	for _, cookie := range cookies {
		if cookie.Container != source.Container || !cookie.Expires.IsZero() && !cookie.Expires.After(time.Now()) {
			continue
		}
		if cookie.Name == "oauth_token" {
			access = cookie.Value
		}
	}
	if strings.TrimSpace(access) == "" {
		return Auth{}, errNoFreshBrowserAuth
	}
	auth := authFromToken(access)
	auth.Source = &source
	return auth, nil
}

func (p *Provider) refreshFromSourceLocked(ctx context.Context, requireDifferent bool) (bool, error) {
	p.mu.RLock()
	if p.auth == nil {
		p.mu.RUnlock()
		return false, baseprovider.ErrSignInRequired
	}
	stored := *p.auth
	epoch := p.epoch
	p.mu.RUnlock()

	if stored.Source == nil {
		source, err := p.findLegacySource(ctx, stored)
		if err != nil {
			return false, err
		}
		stored.Source = source
		p.mu.Lock()
		if p.auth == nil || p.epoch != epoch {
			p.mu.Unlock()
			return false, baseprovider.ErrSignInRequired
		}
		if err := p.persistAuth(stored); err != nil {
			p.mu.Unlock()
			return false, err
		}
		p.auth.Source = source
		p.mu.Unlock()
	}
	candidate, err := p.authFromSource(*stored.Source)
	if err != nil {
		return false, err
	}
	if candidate.AccessToken == stored.AccessToken {
		if requireDifferent || authExpired(candidate.ExpiresAt, time.Now().Unix()) {
			return false, errNoFreshBrowserAuth
		}
		return false, nil
	}
	if authExpired(candidate.ExpiresAt, time.Now().Unix()) {
		return false, errNoFreshBrowserAuth
	}
	if err := p.proveSameUser(ctx, stored, &candidate); err != nil {
		return false, err
	}
	candidate.Username = stored.Username
	if candidate.UserID == 0 {
		candidate.UserID = stored.UserID
	}
	p.mu.Lock()
	if p.auth == nil || p.epoch != epoch {
		p.mu.Unlock()
		return false, baseprovider.ErrSignInRequired
	}
	if err := p.persistAuth(candidate); err != nil {
		p.mu.Unlock()
		return false, err
	}
	p.auth = &candidate
	p.clientID = candidate.ClientID
	p.mu.Unlock()
	// a quiet re-adoption: the user signed in long ago and sees nothing
	p.bus.Emit(protocol.EventSoundCloudAuth, model.SoundCloudAuthEvent{State: "restored", Name: candidate.Username})
	return true, nil
}

// findLegacySource finds the store holding an auth saved before sources were
// remembered. A miss is not repeated until one of the stores changes.
func (p *Provider) findLegacySource(ctx context.Context, stored Auth) (*browsercookies.SessionSource, error) {
	if miss := p.legacyMiss; miss != nil && miss.token == stored.AccessToken {
		changed := false
		for _, store := range miss.stores {
			if p.cookies.ChangedSince(store, miss.at) {
				changed = true
				break
			}
		}
		if !changed {
			return nil, errNoFreshBrowserAuth
		}
	}
	started := time.Now()
	stores := p.cookies.Discover()
	for _, candidate := range p.cookieAuthCandidates(stores, nil) {
		if sameSoundCloudIdentity(stored, candidate.auth) {
			p.legacyMiss = nil
			return candidate.auth.Source, nil
		}
		if stored.UserID == 0 {
			continue
		}
		var me user
		if err := p.proveMe(ctx, candidate.auth, &me); err != nil {
			if rejectedStatus(err) {
				continue
			}
			return nil, err
		}
		if me.ID == stored.UserID {
			p.legacyMiss = nil
			return candidate.auth.Source, nil
		}
	}
	p.legacyMiss = &legacyMiss{token: stored.AccessToken, at: started, stores: stores}
	return nil, errNoFreshBrowserAuth
}

func (p *Provider) proveSameUser(ctx context.Context, stored Auth, candidate *Auth) error {
	if sameSoundCloudIdentity(stored, *candidate) {
		return nil
	}
	var me user
	if err := p.proveMe(ctx, *candidate, &me); err != nil {
		return err
	}
	candidate.UserID = me.ID
	candidate.Username = pointerIfSet(me.Username)
	if stored.UserID != 0 && me.ID == stored.UserID {
		return nil
	}
	return errDifferentBrowserUser
}

func sameSoundCloudIdentity(left, right Auth) bool {
	leftSubject := left.Subject
	if leftSubject == "" {
		leftSubject = tokenSubject(left.AccessToken)
	}
	rightSubject := right.Subject
	if rightSubject == "" {
		rightSubject = tokenSubject(right.AccessToken)
	}
	if leftSubject != "" && rightSubject != "" {
		return leftSubject == rightSubject
	}
	return left.UserID != 0 && right.UserID != 0 && left.UserID == right.UserID
}

func tokenSubject(token string) string {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return ""
	}
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[1], "="))
	if err != nil {
		payload, err = base64.URLEncoding.DecodeString(parts[1])
	}
	if err != nil {
		return ""
	}
	var claims struct {
		Subject string `json:"sub"`
	}
	if json.Unmarshal(payload, &claims) != nil {
		return ""
	}
	return claims.Subject
}

func (p *Provider) ensureFresh(ctx context.Context) error {
	p.mu.RLock()
	if p.auth == nil {
		p.mu.RUnlock()
		return fmt.Errorf("%w: SoundCloud", baseprovider.ErrSignInRequired)
	}
	expired := authExpired(p.auth.ExpiresAt, time.Now().Unix())
	p.mu.RUnlock()
	if !expired {
		return nil
	}
	return p.recoverSession(ctx, "")
}

// recoverRejected follows the browser after SoundCloud refused token.
func (p *Provider) recoverRejected(ctx context.Context, token string) error {
	return p.recoverSession(ctx, token)
}

// recoverSession re-reads the remembered browser store. rejected is the token
// SoundCloud just refused, or "" when the stored token has merely expired.
func (p *Provider) recoverSession(ctx context.Context, rejected string) error {
	p.refreshLock.Lock()
	defer p.refreshLock.Unlock()
	p.mu.RLock()
	if p.auth == nil {
		p.mu.RUnlock()
		return fmt.Errorf("%w: SoundCloud", baseprovider.ErrSignInRequired)
	}
	auth := *p.auth
	p.mu.RUnlock()
	if rejected != "" && auth.AccessToken != rejected {
		// another caller already replaced the refused token
		return nil
	}
	if rejected == "" && !authExpired(auth.ExpiresAt, time.Now().Unix()) {
		return nil
	}
	changed, browserErr := p.refreshFromSourceLocked(ctx, rejected != "")
	if changed {
		return nil
	}
	if browserErr != nil && !definitiveSoundCloudFailure(browserErr) {
		return browserErr
	}
	if rejected == "" {
		// The browser has not visited SoundCloud since the token expired. That is
		// not a rejection: keep the auth and its source so the next re-read
		// adopts the browser's next token quietly, and tell the client once.
		if p.staleAnnounced != auth.AccessToken {
			p.staleAnnounced = auth.AccessToken
			p.bus.Emit(protocol.EventSoundCloudAuth, model.SoundCloudAuthEvent{State: "stale"})
		}
		return fmt.Errorf("%w: SoundCloud", baseprovider.ErrSignInRequired)
	}
	p.expireAuth(auth.AccessToken, "Your SoundCloud session expired. Sign in again to see your playlists and likes.")
	return fmt.Errorf("%w: SoundCloud", baseprovider.ErrSignInRequired)
}

// Restore follows the remembered browser once at daemon start, adopting a
// newer token for the same account. A token SoundCloud has not refused is
// kept even when the browser no longer holds it.
func (p *Provider) Restore(ctx context.Context) error {
	p.refreshLock.Lock()
	defer p.refreshLock.Unlock()
	p.mu.RLock()
	signedIn := p.auth != nil
	p.mu.RUnlock()
	if !signedIn {
		return nil
	}
	if _, err := p.refreshFromSourceLocked(ctx, false); err != nil && !errors.Is(err, errNoFreshBrowserAuth) {
		return err
	}
	return nil
}

func (p *Provider) validateCurrentSession(ctx context.Context) error {
	if err := p.ensureFresh(ctx); err != nil {
		return err
	}
	p.mu.RLock()
	if p.auth == nil {
		p.mu.RUnlock()
		return fmt.Errorf("%w: SoundCloud", baseprovider.ErrSignInRequired)
	}
	auth := *p.auth
	p.mu.RUnlock()
	var me user
	err := p.proveMe(ctx, auth, &me)
	if err == nil {
		return nil
	}
	if rejectedStatus(err) {
		return p.recoverRejected(ctx, auth.AccessToken)
	}
	return err
}

func definitiveSoundCloudFailure(err error) bool {
	return errors.Is(err, errNoFreshBrowserAuth) || errors.Is(err, errDifferentBrowserUser) ||
		errors.Is(err, baseprovider.ErrSignInRequired) || rejectedStatus(err)
}

// rejectedStatus reports SoundCloud refusing the token itself; other 4xx
// answers (429, 408) say nothing about the session.
func rejectedStatus(err error) bool {
	var status soundCloudHTTPError
	return errors.As(err, &status) && (status.status == 401 || status.status == 403)
}

func subjectUserID(subject string) uint64 {
	id, err := strconv.ParseUint(subject, 10, 64)
	if err != nil {
		return 0
	}
	return id
}
