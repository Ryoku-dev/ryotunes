package tidal

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
	baseprovider "github.com/ryoku-dev/ryotunes/internal/provider"
)

const (
	deviceAuthURL       = "https://auth.tidal.com/v1/oauth2/device_authorization"
	tokenURL            = "https://auth.tidal.com/v1/oauth2/token"
	pkceAuthURL         = "https://login.tidal.com/authorize"
	oauthScope          = "r_usr w_usr"
	defaultDeviceID     = "fX2JxdmntZWK0ixT"
	defaultDeviceSecret = "1Nn9AfDAjxrgJFJbKNWLeAyKGVGmINuXPPLHVXAvxAg="
	defaultPKCEID       = "6BDSRdpK9hqEBTgU"
	defaultPKCESecret   = "xeuPmY7nbpZ9IIbLAcQ93shka1VNheUAqN6IcszjTG8="
	defaultPKCERedirect = "https://tidal.com/android/login/auth"
	tidalAuthKey        = "tidal_auth"
	tidalCredentialsKey = "tidal_credentials"
	tokenExpiryMargin   = int64(60)
)

// Credentials configures the TIDAL device and PKCE OAuth clients.
type Credentials struct {
	DeviceID     string `json:"deviceId"`
	DeviceSecret string `json:"deviceSecret"`
	PKCEID       string `json:"pkceId"`
	PKCESecret   string `json:"pkceSecret"`
	PKCERedirect string `json:"pkceRedirect"`
}

func defaultCredentials() Credentials {
	return Credentials{
		DeviceID: defaultDeviceID, DeviceSecret: defaultDeviceSecret,
		PKCEID: defaultPKCEID, PKCESecret: defaultPKCESecret, PKCERedirect: defaultPKCERedirect,
	}
}

type storedCredentials struct {
	DeviceID     *string `json:"device_id"`
	DeviceSecret *string `json:"device_secret"`
	PKCEID       *string `json:"pkce_id"`
	PKCESecret   *string `json:"pkce_secret"`
	PKCERedirect *string `json:"pkce_redirect"`
}

func credentialsFromStored(stored storedCredentials) Credentials {
	creds := defaultCredentials()
	if stored.DeviceID != nil && strings.TrimSpace(*stored.DeviceID) != "" {
		creds.DeviceID = *stored.DeviceID
	}
	if stored.DeviceSecret != nil && strings.TrimSpace(*stored.DeviceSecret) != "" {
		creds.DeviceSecret = *stored.DeviceSecret
	}
	if stored.PKCEID != nil && strings.TrimSpace(*stored.PKCEID) != "" {
		creds.PKCEID = *stored.PKCEID
	}
	if stored.PKCESecret != nil && strings.TrimSpace(*stored.PKCESecret) != "" {
		creds.PKCESecret = *stored.PKCESecret
	}
	if stored.PKCERedirect != nil && strings.TrimSpace(*stored.PKCERedirect) != "" {
		creds.PKCERedirect = *stored.PKCERedirect
	}
	return creds
}

// Auth is persisted TIDAL authorization.
type Auth struct {
	AccessToken  string  `json:"access_token"`
	RefreshToken *string `json:"refresh_token"`
	ExpiresAt    int64   `json:"expires_at"`
	UserID       uint64  `json:"user_id"`
	CountryCode  string  `json:"country_code"`
	Entitlement  string  `json:"entitlement"`
	Username     *string `json:"username"`
	ProfileName  *string `json:"profile_name"`
}

// PKCEChallenge is the private verifier and request state for a PKCE flow.
type PKCEChallenge struct {
	Verifier  string
	Challenge string
	State     string
	epoch     uint64
}

type deviceResponse struct {
	DeviceCode              string  `json:"deviceCode"`
	UserCode                string  `json:"userCode"`
	VerificationURI         string  `json:"verificationUri"`
	VerificationURIComplete *string `json:"verificationUriComplete"`
	ExpiresIn               *int64  `json:"expiresIn"`
	Interval                *int64  `json:"interval"`
}

type tokenResponse struct {
	AccessToken  string  `json:"access_token"`
	RefreshToken *string `json:"refresh_token"`
	ExpiresIn    *int64  `json:"expires_in"`
	User         *struct {
		UserID      *uint64 `json:"userId"`
		CountryCode *string `json:"countryCode"`
		Username    *string `json:"username"`
	} `json:"user"`
}

type tokenEndpointError struct {
	Status      int
	Code        string
	Description string
}

func (e *tokenEndpointError) Error() string {
	detail := strings.TrimSpace(e.Description)
	if detail == "" {
		detail = strings.TrimSpace(e.Code)
	}
	if detail == "" {
		return fmt.Sprintf("TIDAL answered %d", e.Status)
	}
	return fmt.Sprintf("TIDAL answered %d: %s", e.Status, detail)
}

// Status returns the current TIDAL authorization and quality ceiling.
func (p *Provider) Status() model.TidalStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()
	status := model.TidalStatus{HasCredentials: p.credentials.DeviceID != "" || (p.credentials.PKCEID != "" && p.credentials.PKCERedirect != "")}
	if !reusableAuth(p.auth) {
		return status
	}
	status.SignedIn = true
	status.Name = p.auth.ProfileName
	if status.Name == nil {
		status.Name = p.auth.Username
	}
	quality := ceilingForEntitlement(p.auth.Entitlement).String()
	status.Quality = &quality
	return status
}

// SetCredentials applies non-nil credential overrides and persists the resulting set.
func (p *Provider) SetCredentials(params protocol.TidalCredentialsParams) error {
	stored := storedCredentials{
		DeviceID: params.DeviceID, DeviceSecret: params.DeviceSecret, PKCEID: params.PKCEID,
		PKCESecret: params.PKCESecret, PKCERedirect: params.PKCERedirect,
	}
	raw, err := json.Marshal(stored)
	if err != nil {
		return err
	}
	p.mu.Lock()
	if err := p.kv.Set(tidalCredentialsKey, string(raw)); err != nil {
		p.mu.Unlock()
		return err
	}
	p.credentials = credentialsFromStored(stored)
	p.mu.Unlock()
	return nil
}

// SignIn starts one TIDAL device-code flow and reports whether it started.
func (p *Provider) SignIn(ctx context.Context) bool {
	p.mu.Lock()
	if p.signingIn {
		p.mu.Unlock()
		return false
	}
	p.signingIn = true
	epoch := p.epoch
	p.mu.Unlock()
	flowCtx := context.WithoutCancel(ctx)
	go func() {
		defer func() { p.mu.Lock(); p.signingIn = false; p.mu.Unlock() }()
		if err := p.deviceFlow(flowCtx, epoch); err != nil {
			if !p.signInEpoch(epoch) {
				return
			}
			message := err.Error()
			p.bus.Emit(protocol.EventTidalAuth, model.TidalAuthEvent{State: "error", Message: &message})
		}
	}()
	return true
}

func (p *Provider) deviceFlow(ctx context.Context, epoch uint64) error {
	p.mu.RLock()
	creds := p.credentials
	p.mu.RUnlock()
	if strings.TrimSpace(creds.DeviceID) == "" {
		return fmt.Errorf("no tidal client credentials configured for the device flow")
	}
	form := url.Values{"client_id": {creds.DeviceID}, "scope": {oauthScope}}
	var login deviceResponse
	if err := p.postForm(ctx, deviceAuthURL, form, &login); err != nil {
		return err
	}
	if !p.signInEpoch(epoch) {
		return nil
	}
	complete := ""
	if login.VerificationURIComplete != nil {
		complete = *login.VerificationURIComplete
	}
	if complete == "" {
		complete = login.VerificationURI
	}
	if !strings.HasPrefix(complete, "http") {
		complete = "https://" + complete
	}
	code := login.UserCode
	p.bus.Emit(protocol.EventTidalAuth, model.TidalAuthEvent{State: "waiting", URL: &complete, Code: &code})
	expires := int64(300)
	if login.ExpiresIn != nil {
		expires = *login.ExpiresIn
	}
	interval := int64(2)
	if login.Interval != nil {
		interval = max(1, *login.Interval)
	}
	deadline := time.NewTimer(time.Duration(expires) * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			if !p.signInEpoch(epoch) {
				return nil
			}
			message := "The TIDAL code expired before it was approved."
			p.bus.Emit(protocol.EventTidalAuth, model.TidalAuthEvent{State: "expired", Message: &message})
			return nil
		case <-ticker.C:
			if !p.signInEpoch(epoch) {
				return nil
			}
			form := url.Values{
				"client_id": {creds.DeviceID}, "device_code": {login.DeviceCode},
				"grant_type": {"urn:ietf:params:oauth:grant-type:device_code"}, "scope": {oauthScope},
			}
			if creds.DeviceSecret != "" {
				form.Set("client_secret", creds.DeviceSecret)
			}
			var token tokenResponse
			pending, err := p.pollToken(ctx, form, &token)
			if pending {
				continue
			}
			if err != nil {
				if strings.Contains(strings.ToLower(err.Error()), "expired") {
					message := "The TIDAL code expired before it was approved."
					p.bus.Emit(protocol.EventTidalAuth, model.TidalAuthEvent{State: "expired", Message: &message})
					return nil
				}
				return err
			}
			auth := p.adoptToken(token, "standard", nil)
			return p.completeSignIn(ctx, auth, epoch)
		}
	}
}

// StartPKCE returns a PKCE authorization URL and its private challenge.
func (p *Provider) StartPKCE() (PKCEChallenge, string, error) {
	p.mu.RLock()
	creds := p.credentials
	epoch := p.epoch
	p.mu.RUnlock()
	if creds.PKCEID == "" || creds.PKCERedirect == "" {
		return PKCEChallenge{}, "", fmt.Errorf("no tidal client credentials configured for the pkce flow")
	}
	verifier, err := randomToken(64)
	if err != nil {
		return PKCEChallenge{}, "", err
	}
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	state, err := randomToken(16)
	if err != nil {
		return PKCEChallenge{}, "", err
	}
	values := url.Values{
		"response_type": {"code"}, "client_id": {creds.PKCEID}, "redirect_uri": {creds.PKCERedirect},
		"scope": {oauthScope}, "code_challenge_method": {"S256"}, "code_challenge": {challenge}, "state": {state},
	}
	return PKCEChallenge{Verifier: verifier, Challenge: challenge, State: state, epoch: epoch}, pkceAuthURL + "?" + values.Encode(), nil
}

// FinishPKCE exchanges a PKCE code and emits the signed-in event.
func (p *Provider) FinishPKCE(ctx context.Context, code, state string, challenge PKCEChallenge) error {
	if state == "" || state != challenge.State {
		return fmt.Errorf("tidal PKCE state did not match")
	}
	p.mu.RLock()
	creds := p.credentials
	p.mu.RUnlock()
	form := url.Values{
		"client_id": {creds.PKCEID}, "code": {code}, "code_verifier": {challenge.Verifier},
		"grant_type": {"authorization_code"}, "redirect_uri": {creds.PKCERedirect}, "scope": {oauthScope},
	}
	if creds.PKCESecret != "" {
		form.Set("client_secret", creds.PKCESecret)
	}
	var token tokenResponse
	if err := p.postForm(ctx, p.tokenURL, form, &token); err != nil {
		return err
	}
	auth := p.adoptToken(token, "hires", nil)
	return p.completeSignIn(ctx, auth, challenge.epoch)
}

// SignOut clears TIDAL authorization and emits signed_out.
func (p *Provider) SignOut() error {
	p.mu.Lock()
	p.auth = nil
	p.epoch++
	p.mu.Unlock()
	err := p.kv.Delete(tidalAuthKey)
	p.bus.Emit(protocol.EventTidalAuth, model.TidalAuthEvent{State: "signed_out"})
	return err
}

func (p *Provider) completeSignIn(ctx context.Context, auth Auth, epoch uint64) error {
	if !reusableAuth(&auth) {
		return fmt.Errorf("TIDAL did not return a reusable session")
	}
	if auth.UserID != 0 {
		var profile struct {
			ProfileName *string `json:"profileName"`
		}
		if p.profile(ctx, auth, &profile) == nil {
			auth.ProfileName = profile.ProfileName
		}
	}
	p.mu.Lock()
	if p.epoch != epoch {
		p.mu.Unlock()
		return baseprovider.ErrSignInRequired
	}
	if err := p.persistAuth(auth); err != nil {
		p.mu.Unlock()
		return err
	}
	p.auth = &auth
	p.mu.Unlock()
	name := auth.ProfileName
	if name == nil {
		name = auth.Username
	}
	quality := ceilingForEntitlement(auth.Entitlement).String()
	p.bus.Emit(protocol.EventTidalAuth, model.TidalAuthEvent{State: "signed_in", Name: name, Quality: &quality})
	return nil
}

func (p *Provider) profile(ctx context.Context, auth Auth, out any) error {
	endpoint := fmt.Sprintf("%s/users/%d?countryCode=%s", p.apiBase, auth.UserID, url.QueryEscape(auth.CountryCode))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+auth.AccessToken)
	req.Header.Set("User-Agent", p.userAgent)
	resp, err := p.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("TIDAL answered %d for profile", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (p *Provider) signInEpoch(epoch uint64) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.epoch == epoch
}

func (p *Provider) persistAuth(auth Auth) error {
	raw, err := json.Marshal(auth)
	if err != nil {
		return err
	}
	return p.kv.Set(tidalAuthKey, string(raw))
}

func (p *Provider) adoptToken(token tokenResponse, entitlement string, prior *Auth) Auth {
	now := time.Now().Unix()
	expires := int64(3600)
	if token.ExpiresIn != nil {
		expires = *token.ExpiresIn
	}
	auth := Auth{AccessToken: token.AccessToken, RefreshToken: token.RefreshToken, ExpiresAt: now + expires, CountryCode: "US", Entitlement: entitlement}
	if prior != nil {
		auth.UserID, auth.CountryCode, auth.Username, auth.ProfileName = prior.UserID, prior.CountryCode, prior.Username, prior.ProfileName
		if auth.RefreshToken == nil || strings.TrimSpace(*auth.RefreshToken) == "" {
			auth.RefreshToken = prior.RefreshToken
		}
	}
	if token.User != nil {
		if token.User.UserID != nil {
			auth.UserID = *token.User.UserID
		}
		if token.User.CountryCode != nil {
			auth.CountryCode = *token.User.CountryCode
		}
		if token.User.Username != nil {
			auth.Username = token.User.Username
		}
	}
	return auth
}

func reusableAuth(auth *Auth) bool {
	return auth != nil && auth.RefreshToken != nil && strings.TrimSpace(*auth.RefreshToken) != ""
}

func (p *Provider) ensureFresh(ctx context.Context) (Auth, error) {
	p.mu.RLock()
	if !reusableAuth(p.auth) {
		p.mu.RUnlock()
		return Auth{}, fmt.Errorf("%w: TIDAL", baseprovider.ErrSignInRequired)
	}
	current := *p.auth
	p.mu.RUnlock()
	if current.AccessToken != "" && !tokenExpired(current.ExpiresAt, time.Now().Unix()) {
		return current, nil
	}
	p.refreshLock.Lock()
	defer p.refreshLock.Unlock()
	p.mu.RLock()
	if !reusableAuth(p.auth) {
		p.mu.RUnlock()
		return Auth{}, baseprovider.ErrSignInRequired
	}
	current = *p.auth
	creds := p.credentials
	epoch := p.epoch
	p.mu.RUnlock()
	if current.AccessToken != "" && !tokenExpired(current.ExpiresAt, time.Now().Unix()) {
		return current, nil
	}
	clientID, secret := creds.DeviceID, creds.DeviceSecret
	if strings.EqualFold(current.Entitlement, "hires") {
		clientID, secret = creds.PKCEID, creds.PKCESecret
	}
	form := url.Values{"client_id": {clientID}, "refresh_token": {*current.RefreshToken}, "grant_type": {"refresh_token"}, "scope": {oauthScope}}
	if secret != "" {
		form.Set("client_secret", secret)
	}
	var token tokenResponse
	if err := p.postForm(ctx, p.tokenURL, form, &token); err != nil {
		if tokenRejectsSession(err) {
			message := "TIDAL could not renew this session. Sign in again."
			deleteErr := p.expireAuth(message)
			return Auth{}, errors.Join(fmt.Errorf("%w: %s", baseprovider.ErrSignInRequired, message), deleteErr)
		}
		if clientRefused(err) {
			return Auth{}, fmt.Errorf("TIDAL refused the client id or secret in Settings: %w", err)
		}
		return Auth{}, err
	}
	rotated := p.adoptToken(token, current.Entitlement, &current)
	if !reusableAuth(&rotated) || rotated.AccessToken == "" {
		return Auth{}, fmt.Errorf("TIDAL returned an incomplete refreshed session")
	}
	p.mu.Lock()
	if p.epoch != epoch || !reusableAuth(p.auth) {
		p.mu.Unlock()
		return Auth{}, baseprovider.ErrSignInRequired
	}
	if err := p.persistAuth(rotated); err != nil {
		p.mu.Unlock()
		return Auth{}, err
	}
	p.auth = &rotated
	p.mu.Unlock()
	p.log.Info("tidal: refreshed the access token")
	return rotated, nil
}

// tokenRejectsSession reports whether TIDAL says the refresh token itself is
// dead. invalid_client and unauthorized_client mean the client id or secret
// was refused, for example after an edit in Settings; the refresh token is
// kept so restoring the old client works again.
func tokenRejectsSession(err error) bool {
	var endpointErr *tokenEndpointError
	if !errors.As(err, &endpointErr) {
		return false
	}
	return strings.EqualFold(endpointErr.Code, "invalid_grant") || strings.EqualFold(endpointErr.Code, "access_denied")
}

func clientRefused(err error) bool {
	var endpointErr *tokenEndpointError
	return errors.As(err, &endpointErr) &&
		(strings.EqualFold(endpointErr.Code, "invalid_client") || strings.EqualFold(endpointErr.Code, "unauthorized_client"))
}

func (p *Provider) postForm(ctx context.Context, endpoint string, form url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", p.userAgent)
	resp, err := p.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 4096))
		if readErr != nil {
			return readErr
		}
		var payload struct {
			Code        string `json:"error"`
			Description string `json:"error_description"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			payload.Description = strings.TrimSpace(string(body))
		}
		return &tokenEndpointError{Status: resp.StatusCode, Code: payload.Code, Description: payload.Description}
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (p *Provider) pollToken(ctx context.Context, form url.Values, out any) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", p.userAgent)
	resp, err := p.http.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return false, json.NewDecoder(resp.Body).Decode(out)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return false, err
	}
	lower := strings.ToLower(string(body))
	if strings.Contains(lower, "authorization_pending") || strings.Contains(lower, "slow_down") {
		return true, nil
	}
	if strings.Contains(lower, "expired_token") || strings.Contains(lower, "expired") {
		return false, fmt.Errorf("the tidal device login expired before it was approved")
	}
	return false, fmt.Errorf("TIDAL answered %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
}

func tokenExpired(expiresAt, now int64) bool { return expiresAt <= now+tokenExpiryMargin }

func randomToken(length int) (string, error) {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-._~"
	raw := make([]byte, length)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	for i := range raw {
		raw[i] = alphabet[int(raw[i])%len(alphabet)]
	}
	return string(raw), nil
}
