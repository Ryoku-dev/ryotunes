package soundcloud

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
	baseprovider "github.com/ryoku-dev/ryotunes/internal/provider"
	"github.com/ryoku-dev/ryotunes/internal/provider/browsercookies"
)

const soundCloudSignInURL = "https://soundcloud.com/signin"

// Auth is the persisted SoundCloud browser authorization. Only the access
// token is kept: the browser's refresh token and session cookie are never
// used, so they are not stored.
type Auth struct {
	AccessToken string                        `json:"access_token"`
	ClientID    string                        `json:"client_id"`
	ExpiresAt   int64                         `json:"expires_at"`
	Username    *string                       `json:"username"`
	UserID      uint64                        `json:"user_id,omitempty"`
	Subject     string                        `json:"subject,omitempty"`
	Source      *browsercookies.SessionSource `json:"source,omitempty"`
}

// CookieSource supplies browser cookies without touching the live browser.
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

// Status returns the current SoundCloud authorization snapshot.
func (p *Provider) Status() model.SoundCloudStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.auth == nil || authExpired(p.auth.ExpiresAt, time.Now().Unix()) {
		return model.SoundCloudStatus{SignedIn: false}
	}
	return model.SoundCloudStatus{SignedIn: true, Name: p.auth.Username}
}

// SignIn adopts a browser SoundCloud session or starts watching for one, and
// returns the sign-in page the client should open ("" when already signed in).
func (p *Provider) SignIn(ctx context.Context) (string, error) {
	p.mu.RLock()
	hasAuth := p.auth != nil
	p.mu.RUnlock()
	if hasAuth {
		err := p.validateCurrentSession(ctx)
		if err == nil {
			return "", nil
		}
		var readErr browserReadError
		if !definitiveSoundCloudFailure(err) && !errors.As(err, &readErr) {
			return "", err
		}
	}

	p.mu.Lock()
	if p.signingIn {
		url := p.signInURL
		p.mu.Unlock()
		return url, nil
	}
	p.signingIn = true
	epoch := p.epoch
	p.mu.Unlock()

	attempts := make(candidateAttempts)
	signedIn, lastErr := p.tryCandidates(ctx, p.cookieAuthCandidates(p.cookies.Discover(), nil), epoch, attempts)
	if signedIn {
		p.mu.Lock()
		if p.epoch == epoch {
			p.signingIn = false
		}
		p.mu.Unlock()
		return "", nil
	}

	flowCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Minute)
	p.mu.Lock()
	if p.epoch != epoch {
		p.mu.Unlock()
		cancel()
		return "", nil
	}
	p.signInURL = soundCloudSignInURL
	p.signInCancel = cancel
	p.mu.Unlock()
	go func() {
		defer cancel()
		defer func() {
			p.mu.Lock()
			if p.epoch == epoch {
				p.signingIn = false
				p.signInURL = ""
				p.signInCancel = nil
			}
			p.mu.Unlock()
		}()
		if !p.signInEpoch(epoch) {
			return
		}
		watchSince := time.Now()
		p.bus.Emit(protocol.EventSoundCloudAuth, model.SoundCloudAuthEvent{State: "waiting"})
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-flowCtx.Done():
				if !p.signInEpoch(epoch) {
					return
				}
				if lastErr != nil {
					message := "SoundCloud sign-in failed: " + lastErr.Error()
					p.bus.Emit(protocol.EventSoundCloudAuth, model.SoundCloudAuthEvent{State: "error", Message: &message})
					return
				}
				message := "No SoundCloud sign-in was completed in the browser."
				p.bus.Emit(protocol.EventSoundCloudAuth, model.SoundCloudAuthEvent{State: "expired", Message: &message})
				return
			case <-ticker.C:
				candidates := p.cookieAuthCandidates(p.cookies.Discover(), &watchSince)
				if len(candidates) == 0 {
					// a failure from a store that has not changed since is not this flow's outcome
					lastErr = nil
					continue
				}
				done, err := p.tryCandidates(flowCtx, candidates, epoch, attempts)
				if done || !p.signInEpoch(epoch) {
					return
				}
				if err != nil {
					lastErr = err
				}
			}
		}
	}()
	return soundCloudSignInURL, nil
}

// candidateAttempts remembers, for the life of one sign-in flow, which tokens
// SoundCloud refused and when a token that failed for another reason may be
// tried again.
type candidateAttempts map[string]*candidateAttempt

type candidateAttempt struct {
	refused bool
	wait    time.Duration
	next    time.Time
}

const (
	candidateRetryFirst = 4 * time.Second
	candidateRetryMax   = time.Minute
)

// tryCandidates installs the first unexpired candidate SoundCloud accepts.
// Failures stay quiet: the flow reports only its final outcome.
func (p *Provider) tryCandidates(ctx context.Context, candidates []authCandidate, epoch uint64, attempts candidateAttempts) (bool, error) {
	var lastErr error
	for _, candidate := range candidates {
		token := candidate.auth.AccessToken
		now := time.Now()
		attempt := attempts[token]
		if attempt != nil && (attempt.refused || now.Before(attempt.next)) || authExpired(candidate.auth.ExpiresAt, now.Unix()) {
			continue
		}
		err := p.installAndProve(ctx, candidate.auth, epoch)
		if err == nil {
			return true, nil
		}
		if errors.Is(err, baseprovider.ErrSignInRequired) {
			return false, nil
		}
		if attempt == nil {
			attempt = &candidateAttempt{}
			attempts[token] = attempt
		}
		if rejectedStatus(err) {
			attempt.refused = true
		} else {
			attempt.wait = min(max(candidateRetryFirst, 2*attempt.wait), candidateRetryMax)
			attempt.next = time.Now().Add(attempt.wait)
		}
		lastErr = err
		if ctx.Err() != nil {
			break
		}
	}
	return false, lastErr
}

// SignOut clears SoundCloud authorization and emits signed_out.
func (p *Provider) SignOut() error {
	p.mu.Lock()
	p.auth = nil
	p.epoch++
	cancel := p.signInCancel
	p.signInCancel = nil
	p.signingIn = false
	p.signInURL = ""
	p.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	err := p.kv.Delete(soundCloudAuthKey)
	p.bus.Emit(protocol.EventSoundCloudAuth, model.SoundCloudAuthEvent{State: "signed_out"})
	return err
}

func authFromToken(access string) Auth {
	now := time.Now().Unix()
	auth := Auth{AccessToken: access, ClientID: fallbackClientID, ExpiresAt: now + 3600}
	parts := strings.Split(access, ".")
	if len(parts) < 2 {
		return auth
	}
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[1], "="))
	if err != nil {
		payload, err = base64.URLEncoding.DecodeString(parts[1])
	}
	if err != nil {
		return auth
	}
	var claims struct {
		ExpiresAt *int64  `json:"exp"`
		ClientID  *string `json:"client_id"`
		AZP       *string `json:"azp"`
		Subject   string  `json:"sub"`
	}
	if json.Unmarshal(payload, &claims) != nil {
		return auth
	}
	if claims.ExpiresAt != nil {
		auth.ExpiresAt = *claims.ExpiresAt
	}
	if claims.ClientID != nil && *claims.ClientID != "" {
		auth.ClientID = *claims.ClientID
	} else if claims.AZP != nil && *claims.AZP != "" {
		auth.ClientID = *claims.AZP
	}
	auth.Subject = claims.Subject
	auth.UserID = subjectUserID(claims.Subject)
	return auth
}

// installAndProve adopts auth once /me accepts it. Failures are returned, not
// announced, so a sign-in flow reports only its final outcome.
func (p *Provider) installAndProve(ctx context.Context, auth Auth, epoch uint64) error {
	p.mu.Lock()
	if p.epoch != epoch {
		p.mu.Unlock()
		return baseprovider.ErrSignInRequired
	}
	prior := p.auth
	p.auth = &auth
	p.clientID = auth.ClientID
	p.mu.Unlock()
	var me user
	err := p.proveMe(ctx, auth, &me)
	if err != nil {
		p.mu.Lock()
		if p.epoch == epoch {
			p.auth = prior
		}
		p.mu.Unlock()
		return err
	}
	auth.Username = pointerIfSet(me.Username)
	auth.UserID = me.ID
	p.mu.Lock()
	if p.epoch != epoch {
		p.mu.Unlock()
		return baseprovider.ErrSignInRequired
	}
	if err := p.persistAuth(auth); err != nil {
		p.auth = prior
		p.mu.Unlock()
		return err
	}
	p.auth = &auth
	p.mu.Unlock()
	p.bus.Emit(protocol.EventSoundCloudAuth, model.SoundCloudAuthEvent{State: "signed_in", Name: auth.Username})
	return nil
}

func (p *Provider) signInEpoch(epoch uint64) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.epoch == epoch
}

func (p *Provider) proveMe(ctx context.Context, auth Auth, me *user) error {
	endpoint := p.apiBase + "/me?client_id=" + url.QueryEscape(auth.ClientID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "OAuth "+auth.AccessToken)
	req.Header.Set("User-Agent", userAgent)
	resp, err := p.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return soundCloudHTTPError{operation: "/me", status: resp.StatusCode}
	}
	return json.NewDecoder(resp.Body).Decode(me)
}

func (p *Provider) persistAuth(auth Auth) error {
	raw, err := json.Marshal(auth)
	if err != nil {
		return err
	}
	return p.kv.Set(soundCloudAuthKey, string(raw))
}

func authExpired(expiresAt, now int64) bool { return expiresAt <= now+expiryMarginSeconds }

func pointerIfSet(v string) *string {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return &v
}
