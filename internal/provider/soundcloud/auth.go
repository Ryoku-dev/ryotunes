package soundcloud

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
	baseprovider "github.com/ryoku-dev/ryotunes/internal/provider"
	"github.com/ryoku-dev/ryotunes/internal/provider/browsercookies"
)

const refreshURL = "https://soundcloud.com/n/api/refresh-token"

// Auth is the persisted SoundCloud browser authorization.
type Auth struct {
	AccessToken    string  `json:"access_token"`
	RefreshToken   *string `json:"refresh_token"`
	ClientID       string  `json:"client_id"`
	ExpiresAt      int64   `json:"expires_at"`
	ConnectSession *string `json:"connect_session"`
	Username       *string `json:"username"`
	UserID         uint64  `json:"-"`
}

// CookieSource supplies browser cookies and opens the system browser.
type CookieSource interface {
	Discover() []browsercookies.Store
	Read(store browsercookies.Store, hostSuffixes ...string) ([]browsercookies.Cookie, error)
	ChangedSince(store browsercookies.Store, since time.Time) bool
	OpenURL(url string) error
}

type systemCookies struct{}

func (systemCookies) Discover() []browsercookies.Store { return browsercookies.Discover() }
func (systemCookies) Read(store browsercookies.Store, suffixes ...string) ([]browsercookies.Cookie, error) {
	return browsercookies.Read(store, suffixes...)
}
func (systemCookies) ChangedSince(store browsercookies.Store, since time.Time) bool {
	return browsercookies.ChangedSince(store, since)
}
func (systemCookies) OpenURL(rawURL string) error { return browsercookies.OpenURL(rawURL) }

// Status returns the current SoundCloud authorization snapshot.
func (p *Provider) Status() model.SoundCloudStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.auth == nil {
		return model.SoundCloudStatus{SignedIn: false}
	}
	return model.SoundCloudStatus{SignedIn: true, Name: p.auth.Username}
}

// SignIn starts one browser-cookie import flow and reports whether it started.
func (p *Provider) SignIn(ctx context.Context) bool {
	p.mu.Lock()
	if p.signingIn {
		p.mu.Unlock()
		return false
	}
	p.signingIn = true
	epoch := p.epoch
	p.mu.Unlock()
	flowCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Minute)
	go func() {
		defer cancel()
		defer func() { p.mu.Lock(); p.signingIn = false; p.mu.Unlock() }()
		stores := p.cookies.Discover()
		if auth, ok := p.importCookies(stores, nil); ok {
			_ = p.installAndProve(flowCtx, auth, epoch)
			return
		}
		if !p.signInEpoch(epoch) {
			return
		}
		watchSince := time.Now()
		if err := p.cookies.OpenURL("https://soundcloud.com/signin"); err != nil {
			message := "Could not open your browser. Sign in at soundcloud.com, then press the button again."
			p.bus.Emit(protocol.EventSoundCloudAuth, model.SoundCloudAuthEvent{State: "error", Message: &message})
			return
		}
		p.bus.Emit(protocol.EventSoundCloudAuth, model.SoundCloudAuthEvent{State: "waiting"})
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-flowCtx.Done():
				if !p.signInEpoch(epoch) {
					return
				}
				message := "No SoundCloud sign-in was completed in the browser."
				p.bus.Emit(protocol.EventSoundCloudAuth, model.SoundCloudAuthEvent{State: "expired", Message: &message})
				return
			case <-ticker.C:
				stores = p.cookies.Discover()
				auth, ok := p.importCookies(stores, &watchSince)
				if !ok {
					continue
				}
				_ = p.installAndProve(flowCtx, auth, epoch)
				return
			}
		}
	}()
	return true
}

// SignOut clears SoundCloud authorization and emits signed_out.
func (p *Provider) SignOut() error {
	p.mu.Lock()
	p.auth = nil
	p.epoch++
	p.mu.Unlock()
	err := p.kv.Delete(soundCloudAuthKey)
	p.bus.Emit(protocol.EventSoundCloudAuth, model.SoundCloudAuthEvent{State: "signed_out"})
	return err
}

func (p *Provider) importCookies(stores []browsercookies.Store, changedAfter *time.Time) (Auth, bool) {
	for _, store := range stores {
		if changedAfter != nil && !p.cookies.ChangedSince(store, *changedAfter) {
			continue
		}
		cookies, err := p.cookies.Read(store, "soundcloud.com")
		if err != nil {
			continue
		}
		var access, refresh, session string
		for _, cookie := range cookies {
			switch cookie.Name {
			case "oauth_token":
				access = cookie.Value
			case "oauth_refresh_token":
				refresh = cookie.Value
			case "connect_session":
				session = cookie.Value
			}
		}
		if strings.TrimSpace(access) == "" {
			continue
		}
		return authFromToken(access, pointerIfSet(refresh), pointerIfSet(session)), true
	}
	return Auth{}, false
}

func authFromToken(access string, refresh, session *string) Auth {
	now := time.Now().Unix()
	auth := Auth{
		AccessToken: access, RefreshToken: refresh, ConnectSession: session,
		ClientID: fallbackClientID, ExpiresAt: now + 3600,
	}
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
	return auth
}

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
		active := p.epoch == epoch
		if active {
			p.auth = prior
		}
		p.mu.Unlock()
		if active {
			message := "SoundCloud sign-in failed: " + err.Error()
			p.bus.Emit(protocol.EventSoundCloudAuth, model.SoundCloudAuthEvent{State: "error", Message: &message})
		}
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
		message := "SoundCloud sign-in failed: " + err.Error()
		p.bus.Emit(protocol.EventSoundCloudAuth, model.SoundCloudAuthEvent{State: "error", Message: &message})
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
		return fmt.Errorf("SoundCloud answered %d for /me", resp.StatusCode)
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

func (p *Provider) ensureFresh(ctx context.Context) error {
	p.mu.RLock()
	if p.auth == nil {
		p.mu.RUnlock()
		return fmt.Errorf("%w: SoundCloud", baseprovider.ErrSignInRequired)
	}
	auth := *p.auth
	p.mu.RUnlock()
	if !authExpired(auth.ExpiresAt, time.Now().Unix()) {
		return nil
	}
	if auth.RefreshToken == nil || auth.ConnectSession == nil {
		return nil
	}
	p.refreshLock.Lock()
	defer p.refreshLock.Unlock()
	p.mu.RLock()
	if p.auth == nil {
		p.mu.RUnlock()
		return baseprovider.ErrSignInRequired
	}
	auth = *p.auth
	epoch := p.epoch
	p.mu.RUnlock()
	if !authExpired(auth.ExpiresAt, time.Now().Unix()) {
		return nil
	}
	rotated, err := p.refresh(ctx, auth)
	if err != nil {
		if auth.ExpiresAt <= time.Now().Unix() {
			p.expireAuth("Your SoundCloud session expired. Sign in again to see your playlists and likes.")
			return baseprovider.ErrSignInRequired
		}
		return nil
	}
	p.mu.Lock()
	if p.epoch != epoch || p.auth == nil {
		p.mu.Unlock()
		return baseprovider.ErrSignInRequired
	}
	if err := p.persistAuth(rotated); err != nil {
		p.mu.Unlock()
		return err
	}
	p.auth = &rotated
	p.mu.Unlock()
	p.log.Info("soundcloud: refreshed the access token")
	return nil
}

func (p *Provider) refresh(ctx context.Context, auth Auth) (Auth, error) {
	body, err := json.Marshal(map[string]any{
		"client_id": auth.ClientID, "grant_type": "refresh_token", "refresh_token": auth.RefreshToken,
	})
	if err != nil {
		return Auth{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, refreshURL, bytes.NewReader(body))
	if err != nil {
		return Auth{}, err
	}
	cookie := "connect_session=" + *auth.ConnectSession
	if auth.RefreshToken != nil {
		cookie += "; oauth_refresh_token=" + *auth.RefreshToken
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", cookie)
	req.Header.Set("Referer", homeURL)
	req.Header.Set("Origin", "https://soundcloud.com")
	req.Header.Set("User-Agent", userAgent)
	resp, err := p.http.Do(req)
	if err != nil {
		return Auth{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Auth{}, fmt.Errorf("SoundCloud refresh answered %d", resp.StatusCode)
	}
	var result struct {
		AccessToken  string  `json:"access_token"`
		RefreshToken *string `json:"refresh_token"`
		ExpiresIn    *int64  `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return Auth{}, err
	}
	rotated := authFromToken(result.AccessToken, result.RefreshToken, auth.ConnectSession)
	if rotated.RefreshToken == nil {
		rotated.RefreshToken = auth.RefreshToken
	}
	rotated.Username = auth.Username
	rotated.UserID = auth.UserID
	if result.ExpiresIn != nil {
		promise := time.Now().Unix() + *result.ExpiresIn
		if promise < rotated.ExpiresAt {
			rotated.ExpiresAt = promise
		}
	}
	return rotated, nil
}

func authExpired(expiresAt, now int64) bool { return expiresAt <= now+expiryMarginSeconds }

func pointerIfSet(v string) *string {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return &v
}
