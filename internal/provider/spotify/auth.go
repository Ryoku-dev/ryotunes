package spotify

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	librespot "github.com/devgianlu/go-librespot"
	"github.com/devgianlu/go-librespot/ap"
	"github.com/devgianlu/go-librespot/login5"
	spotifypb "github.com/devgianlu/go-librespot/proto/spotify"
	pbdata "github.com/devgianlu/go-librespot/proto/spotify/clienttoken/data/v0"
	pbhttp "github.com/devgianlu/go-librespot/proto/spotify/clienttoken/http/v0"
	devicespb "github.com/devgianlu/go-librespot/proto/spotify/connectstate/devices"
	"github.com/devgianlu/go-librespot/session"
	providerapi "github.com/ryoku-dev/ryotunes/internal/provider"
	"golang.org/x/oauth2"
	"google.golang.org/protobuf/proto"
)

const (
	oauthClientID     = "65b708073fc0480ea92a077233ca87bd"
	oauthRedirect     = "http://127.0.0.1:8989/login"
	oauthAuthorizeURL = "https://accounts.spotify.com/authorize"
	oauthTokenURL     = "https://accounts.spotify.com/api/token"
	callbackMessage   = "Ryotunes has your Spotify authorization. You can close this tab."
)

var oauthScopes = []string{
	"playlist-read-collaborative", "playlist-read-private", "streaming", "user-follow-read",
	"user-library-read", "user-read-email", "user-read-playback-state", "user-read-private",
	"user-read-recently-played", "user-top-read",
}

type storedCredentials struct {
	Username string `json:"username"`
	Data     string `json:"data"`
}

type accountProfile struct {
	DisplayName string `json:"display_name"`
	ID          string `json:"id"`
	Email       string `json:"email"`
	Product     string `json:"product"`
	// recheckAt is set while the product is unknown: when to ask again.
	recheckAt time.Time
}

const (
	profileURL          = "https://api.spotify.com/v1/me"
	profileRetryDefault = time.Minute
	profileRetryMax     = time.Hour
)

// Restore restores cached Spotify credentials and emits the startup result event.
func (p *Spotify) Restore(ctx context.Context) error {
	p.restoreMu.Lock()
	defer p.restoreMu.Unlock()
	p.authMu.Lock()
	generation := p.authGeneration
	p.authMu.Unlock()
	if !fileExists(p.credPath) {
		return nil
	}
	sess, profile, err := p.openStoredSession(ctx)
	if err != nil {
		p.log.Warn("Spotify restore failed", "error", err, "cause", errorCause(err))
		message := "Spotify sign-in no longer works: " + err.Error()
		p.emitAuth("restore_failed", nil, nil, &message)
		return err
	}
	p.authMu.Lock()
	if generation != p.authGeneration {
		p.authMu.Unlock()
		sess.Close()
		return nil
	}
	if err := p.saveCredentials(sess); err != nil {
		p.authMu.Unlock()
		sess.Close()
		message := "Spotify sign-in no longer works: " + err.Error()
		p.emitAuth("restore_failed", nil, nil, &message)
		return err
	}
	current := p.installSession(sess, profile)
	p.authMu.Unlock()
	if !current {
		return nil
	}
	p.emitAuth("restored", nil, nil, nil)
	return nil
}

// SignIn starts one PKCE authorization flow and returns immediately.
func (p *Spotify) SignIn(_ context.Context) (bool, error) {
	p.mu.RLock()
	closed := p.closed
	p.mu.RUnlock()
	if closed {
		return false, errors.New("Spotify provider is closed")
	}
	p.authMu.Lock()
	if p.signing {
		url := p.authURL
		p.authMu.Unlock()
		if url != "" {
			p.emitAuth("url", &url, nil, nil)
		}
		return true, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	p.authGeneration++
	generation := p.authGeneration
	p.signing = true
	p.authURL = ""
	p.authCancel = cancel
	p.authMu.Unlock()

	go func() {
		defer cancel()
		err := p.signIn(ctx, generation)
		p.authMu.Lock()
		p.signing = false
		p.authURL = ""
		p.authCancel = nil
		p.authMu.Unlock()
		if err != nil && ctx.Err() == nil {
			p.log.Warn("Spotify sign-in failed", "error", err, "cause", errorCause(err))
			message := "Spotify sign-in failed: " + err.Error()
			p.emitAuth("error", nil, nil, &message)
		}
	}()
	return true, nil
}

// SignOut closes the session, removes account credentials, and emits signed_out.
func (p *Spotify) SignOut() error {
	p.authMu.Lock()
	if p.authCancel != nil {
		p.authCancel()
	}
	p.authGeneration++
	p.authMu.Unlock()
	old := p.clearSession()
	if old != nil {
		old.Close()
	}
	p.closeActiveStream()
	var joined error
	for _, path := range []string{p.credPath, p.legacyCredPath} {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			joined = errors.Join(joined, err)
		}
	}
	p.emitAuth("signed_out", nil, nil, nil)
	return joined
}

func (p *Spotify) signIn(ctx context.Context, generation uint64) error {
	accessToken, username, err := p.authorize(ctx)
	if err != nil {
		return err
	}
	deviceID, err := p.deviceID()
	if err != nil {
		return err
	}
	clientToken, err := requestClientToken(ctx, p.http, deviceID)
	if err != nil {
		return classifySignIn(err)
	}
	sess, err := session.NewSessionFromOptions(ctx, &session.Options{
		Log: p.liblog, DeviceType: devicespb.DeviceType_COMPUTER, DeviceId: deviceID,
		Credentials: session.SpotifyTokenCredentials{Username: username, Token: accessToken},
		ClientToken: clientToken, Client: p.http,
	})
	if err != nil {
		return classifySignIn(err)
	}
	profile, err := p.premiumProfile(ctx, spclientToken(sess))
	if err != nil {
		sess.Close()
		return err
	}
	if profile.DisplayName == "" {
		profile.DisplayName = sess.Username()
	}
	p.authMu.Lock()
	if generation != p.authGeneration || ctx.Err() != nil {
		p.authMu.Unlock()
		sess.Close()
		return context.Canceled
	}
	if err := p.saveCredentials(sess); err != nil {
		p.authMu.Unlock()
		sess.Close()
		return err
	}
	p.mu.Lock()
	p.clientToken = clientToken
	p.mu.Unlock()
	if !p.installSession(sess, profile) {
		p.authMu.Unlock()
		return errors.New("Spotify provider is closed")
	}
	p.authMu.Unlock()
	name := profile.DisplayName
	p.emitAuth("signed_in", nil, &name, nil)
	return nil
}

func (p *Spotify) restoreSession(ctx context.Context) (*session.Session, accountProfile, error) {
	creds, err := readCredentials(p.credPath)
	if err != nil {
		return nil, accountProfile{}, fmt.Errorf("the stored credentials are no longer valid")
	}
	deviceID, err := p.deviceID()
	if err != nil {
		return nil, accountProfile{}, err
	}
	clientToken, err := requestClientToken(ctx, p.http, deviceID)
	if err != nil {
		return nil, accountProfile{}, classifySignIn(err)
	}
	sess, err := session.NewSessionFromOptions(ctx, &session.Options{
		Log: p.liblog, DeviceType: devicespb.DeviceType_COMPUTER, DeviceId: deviceID,
		Credentials: session.StoredCredentials{Username: creds.Username, Data: creds.Data},
		ClientToken: clientToken, Client: p.http,
	})
	if err != nil {
		return nil, accountProfile{}, classifySignIn(err)
	}
	profile, err := p.premiumProfile(ctx, spclientToken(sess))
	if err != nil {
		sess.Close()
		return nil, accountProfile{}, err
	}
	if profile.DisplayName == "" {
		profile.DisplayName = sess.Username()
	}

	p.mu.Lock()
	p.clientToken = clientToken
	p.mu.Unlock()
	return sess, profile, nil
}

// openStoredSession bounds the restore itself, because callers hold restoreMu
// and may pass a context without a deadline.
func (p *Spotify) openStoredSession(ctx context.Context) (*session.Session, accountProfile, error) {
	ctx, cancel := context.WithTimeout(ctx, p.restoreTimeout)
	defer cancel()
	return p.restoreSession(ctx)
}

func (p *Spotify) authorize(ctx context.Context) (string, string, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:8989")
	if err != nil {
		return "", "", fmt.Errorf("cannot bind the OAuth callback listener on 127.0.0.1:8989: %w", err)
	}
	defer listener.Close()

	verifierBytes := make([]byte, 64)
	stateBytes := make([]byte, 24)
	if _, err := rand.Read(verifierBytes); err != nil {
		return "", "", err
	}
	if _, err := rand.Read(stateBytes); err != nil {
		return "", "", err
	}
	verifier := base64.RawURLEncoding.EncodeToString(verifierBytes)
	state := base64.RawURLEncoding.EncodeToString(stateBytes)
	challengeSum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(challengeSum[:])
	config := oauth2.Config{
		ClientID: oauthClientID, RedirectURL: oauthRedirect, Scopes: oauthScopes,
		Endpoint: oauth2.Endpoint{AuthURL: oauthAuthorizeURL, TokenURL: oauthTokenURL},
	}
	authURL := config.AuthCodeURL(state, oauth2.SetAuthURLParam("code_challenge", challenge), oauth2.SetAuthURLParam("code_challenge_method", "S256"))
	p.authMu.Lock()
	if ctx.Err() != nil {
		p.authMu.Unlock()
		return "", "", ctx.Err()
	}
	p.authURL = authURL
	p.authMu.Unlock()
	p.emitAuth("url", &authURL, nil, nil)

	type callback struct {
		code, state, oauthError string
		err                     error
	}
	result := make(chan callback, 1)
	server := &http.Server{ReadHeaderTimeout: 5 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/login" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = io.WriteString(w, callbackMessage)
		select {
		case result <- callback{code: r.URL.Query().Get("code"), state: r.URL.Query().Get("state"), oauthError: r.URL.Query().Get("error")}:
		default:
		}
	})}
	go func() { _ = server.Serve(listener) }()
	defer server.Close()

	wait, cancel := context.WithTimeout(ctx, 300*time.Second)
	defer cancel()
	var cb callback
	select {
	case cb = <-result:
	case <-wait.Done():
		return "", "", fmt.Errorf("authorization was cancelled in the browser")
	}
	if cb.oauthError == "access_denied" {
		return "", "", fmt.Errorf("authorization was cancelled in the browser")
	}
	if cb.oauthError != "" {
		return "", "", fmt.Errorf("Spotify refused authorization (%s)", cb.oauthError)
	}
	if cb.state != state || cb.code == "" {
		return "", "", &signInError{message: "Spotify refused the session", cause: errors.New("the OAuth callback had a different state or no code")}
	}
	token, err := config.Exchange(ctx, cb.code, oauth2.SetAuthURLParam("code_verifier", verifier))
	if err != nil {
		return "", "", &signInError{message: "Spotify refused the session", cause: err}
	}
	username, _ := token.Extra("username").(string)
	return token.AccessToken, username, nil
}

// premiumProfile reads the account's product right after login. A rate
// limit, an outage or an unreadable answer says nothing about the account,
// so the session is kept with the product unknown and asked again later.
func (p *Spotify) premiumProfile(ctx context.Context, accessToken func(context.Context) (string, error)) (accountProfile, error) {
	deadline := time.Now().Add(5 * time.Second)
	for {
		token, err := accessToken(ctx)
		if err != nil {
			return accountProfile{}, classifySignIn(err)
		}
		profile, retry, err := p.fetchProfile(ctx, token)
		if err != nil {
			return profile, err
		}
		if retry > 0 {
			profile.recheckAt = time.Now().Add(retry)
			return profile, nil
		}
		if profile.Product == "premium" {
			return profile, nil
		}
		if profile.Product != "" {
			return profile, providerapi.ErrNotPremium
		}
		if time.Now().After(deadline) {
			profile.recheckAt = time.Now()
			return profile, nil
		}
		select {
		case <-ctx.Done():
			return profile, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// fetchProfile asks /v1/me once. A positive retry means Spotify did not
// answer (429, 5xx, unreadable body) and says when to ask again.
func (p *Spotify) fetchProfile(ctx context.Context, token string) (accountProfile, time.Duration, error) {
	var profile accountProfile
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, profileURL, nil)
	if err != nil {
		return profile, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := p.http.Do(req)
	if err != nil {
		return profile, 0, &signInError{message: "Spotify could not be reached", cause: err}
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return profile, 0, &signInError{message: "the stored credentials are no longer valid", cause: fmt.Errorf("profile answered %s", resp.Status)}
	case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
		retry := retryAfter(resp.Header.Get("Retry-After"), time.Now())
		p.log.Warn("Spotify profile check deferred", "status", resp.StatusCode, "retry", retry)
		return profile, retry, nil
	case resp.StatusCode < 200 || resp.StatusCode >= 300:
		return profile, 0, &signInError{message: "Spotify refused the session", cause: fmt.Errorf("profile answered %s", resp.Status)}
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&profile); err != nil {
		p.log.Warn("Spotify profile check deferred", "error", err, "retry", profileRetryDefault)
		return accountProfile{}, profileRetryDefault, nil
	}
	if profile.DisplayName == "" {
		profile.DisplayName = profile.ID
	}
	return profile, 0, nil
}

// retryAfter reads a Retry-After header (seconds or an HTTP date), clamped
// to between one second and an hour; anything else means a minute.
func retryAfter(header string, now time.Time) time.Duration {
	header = strings.TrimSpace(header)
	wait := time.Duration(-1)
	if seconds, err := strconv.Atoi(header); err == nil {
		wait = time.Duration(seconds) * time.Second
	} else if at, err := http.ParseTime(header); err == nil {
		wait = at.Sub(now)
	}
	switch {
	case wait < 0:
		return profileRetryDefault
	case wait < time.Second:
		return time.Second
	case wait > profileRetryMax:
		return profileRetryMax
	default:
		return wait
	}
}

func spclientToken(sess *session.Session) func(context.Context) (string, error) {
	return func(ctx context.Context) (string, error) { return sess.Spclient().GetAccessToken(ctx, false) }
}

func errorCause(err error) string {
	var classified *signInError
	if errors.As(err, &classified) && classified.cause != nil {
		return classified.cause.Error()
	}
	return err.Error()
}

func (p *Spotify) deviceID() (string, error) {
	if raw, err := os.ReadFile(p.devicePath); err == nil {
		if id, ok := librespotDeviceID(string(raw)); ok {
			if id != strings.TrimSpace(string(raw)) {
				if err := atomicWrite(p.devicePath, []byte(id+"\n"), 0o600); err != nil {
					return "", err
				}
			}
			return id, nil
		}
	}
	raw := make([]byte, 20)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	id := hex.EncodeToString(raw)
	if err := atomicWrite(p.devicePath, []byte(id+"\n"), 0o600); err != nil {
		return "", err
	}
	return id, nil
}

// librespotDeviceID returns the 40 hex id go-librespot requires. A 1.x UUID
// maps to the SHA-1 of its lower-case text, so a migrated device keeps one
// stable id; a UUID copied verbatim by an earlier 2.x build maps the same way.
func librespotDeviceID(raw string) (string, bool) {
	id := strings.TrimSpace(raw)
	if decoded, err := hex.DecodeString(id); err == nil && len(decoded) == 20 {
		return id, true
	}
	if len(id) != 36 {
		return "", false
	}
	for i := range id {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if id[i] != '-' {
				return "", false
			}
			continue
		}
		if !((id[i] >= '0' && id[i] <= '9') || (id[i] >= 'a' && id[i] <= 'f') || (id[i] >= 'A' && id[i] <= 'F')) {
			return "", false
		}
	}
	sum := sha1.Sum([]byte(strings.ToLower(id)))
	return hex.EncodeToString(sum[:]), true
}

// migrateLegacyCache copies the 1.x session into 2.x once. The marker stops a
// later sign out from being undone by the 1.x copy on the next start; 2.x
// never writes the 1.x copy.
func (p *Spotify) migrateLegacyCache() error {
	if fileExists(p.migratedPath) || !fileExists(p.legacyCredPath) {
		return nil
	}
	var problems error
	if !fileExists(p.credPath) {
		creds, err := readCredentials(p.legacyCredPath)
		if err != nil {
			problems = fmt.Errorf("read credentials: %w", err)
		} else {
			if err := p.migrateLegacyDeviceID(); err != nil {
				problems = fmt.Errorf("device id, a new one is made at sign in: %w", err)
			}
			if err := writeCredentials(p.credPath, creds); err != nil {
				return errors.Join(problems, fmt.Errorf("write credentials: %w", err))
			}
		}
	}
	if err := atomicWrite(p.migratedPath, []byte("1\n"), 0o600); err != nil {
		problems = errors.Join(problems, fmt.Errorf("record migration: %w", err))
	}
	return problems
}

func (p *Spotify) migrateLegacyDeviceID() error {
	if raw, err := os.ReadFile(p.devicePath); err == nil {
		if _, ok := librespotDeviceID(string(raw)); ok {
			return nil
		}
	}
	raw, err := os.ReadFile(p.legacyDevicePath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	id, ok := librespotDeviceID(string(raw))
	if !ok {
		return fmt.Errorf("invalid device id")
	}
	return atomicWrite(p.devicePath, []byte(id+"\n"), 0o600)
}

func (p *Spotify) saveCredentials(sess *session.Session) error {
	return writeCredentials(p.credPath, session.StoredCredentials{Username: sess.Username(), Data: sess.StoredCredentials()})
}

func writeCredentials(path string, creds session.StoredCredentials) error {
	body, err := json.Marshal(storedCredentials{Username: creds.Username, Data: base64.StdEncoding.EncodeToString(creds.Data)})
	if err != nil {
		return err
	}
	return atomicWrite(path, append(body, '\n'), 0o600)
}

func readCredentials(path string) (session.StoredCredentials, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return session.StoredCredentials{}, err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return session.StoredCredentials{}, err
	}
	pick := func(names ...string) json.RawMessage {
		for _, name := range names {
			if value := raw[name]; value != nil {
				return value
			}
		}
		return nil
	}
	var username string
	if err := json.Unmarshal(pick("username", "Username"), &username); err != nil {
		return session.StoredCredentials{}, err
	}
	if authTypeRaw := pick("auth_type"); authTypeRaw != nil {
		var authType int
		if err := json.Unmarshal(authTypeRaw, &authType); err != nil || authType != 1 {
			return session.StoredCredentials{}, fmt.Errorf("invalid credential type")
		}
	}
	dataRaw := pick("data", "Data", "auth_data")
	var data []byte
	var encoded string
	if json.Unmarshal(dataRaw, &encoded) == nil {
		data, err = base64.StdEncoding.DecodeString(encoded)
	} else {
		var numbers []int
		err = json.Unmarshal(dataRaw, &numbers)
		if err == nil {
			data = make([]byte, len(numbers))
			for i, number := range numbers {
				if number < 0 || number > 255 {
					err = fmt.Errorf("invalid credential byte")
					break
				}
				data[i] = byte(number)
			}
		}
	}
	if err != nil || username == "" || len(data) == 0 {
		return session.StoredCredentials{}, fmt.Errorf("invalid credentials")
	}
	return session.StoredCredentials{Username: username, Data: data}, nil
}

func atomicWrite(path string, body []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmp := file.Name()
	defer os.Remove(tmp)
	if err := file.Chmod(mode); err != nil {
		file.Close()
		return err
	}
	if _, err := file.Write(body); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// signInError is the sentence shown to the user; the cause stays reachable so
// recovery can tell a refusal from an unreachable network.
type signInError struct {
	message string
	cause   error
}

func (e *signInError) Error() string { return e.message }
func (e *signInError) Unwrap() error { return e.cause }

func classifySignIn(err error) error {
	var refused *ap.AccesspointLoginError
	if errors.As(err, &refused) && refused.Message != nil {
		switch refused.Message.GetErrorCode() {
		case spotifypb.ErrorCode_PremiumAccountRequired:
			return &signInError{message: "Spotify Premium is required", cause: err}
		case spotifypb.ErrorCode_BadCredentials, spotifypb.ErrorCode_CouldNotValidateCredentials:
			return &signInError{message: "the stored credentials are no longer valid", cause: err}
		case spotifypb.ErrorCode_TravelRestriction:
			return &signInError{message: "the account is out of its home region", cause: err}
		}
	}
	message := err.Error()
	switch {
	case containsAny(message, "travel restriction", "country", "region"):
		return &signInError{message: "the account is out of its home region", cause: err}
	case containsAny(message, "bad credentials", "invalid credentials", "unauthorized"):
		return &signInError{message: "the stored credentials are no longer valid", cause: err}
	case transientFailure(err) || containsAny(message, "connection", "timed out", "timeout", "dns", "network", "resolve"):
		return &signInError{message: "Spotify could not be reached", cause: err}
	default:
		return &signInError{message: "Spotify refused the session", cause: err}
	}
}

// transientFailure reports whether a failure says nothing about the
// credentials: a dial, DNS or TLS error, a timeout, or a 5xx from login5.
// go-librespot wraps those with %w; its refusals are
// *ap.AccesspointLoginError and *login5.LoginError, which are not network
// errors.
func transientFailure(err error) bool {
	var netErr net.Error
	var login5HTTP *login5.HTTPError
	switch {
	case errors.Is(err, context.DeadlineExceeded), errors.As(err, &netErr):
		return true
	case errors.As(err, &login5HTTP):
		return login5HTTP.StatusCode >= 500
	default:
		return false
	}
}

func requestClientToken(ctx context.Context, client *http.Client, deviceID string) (string, error) {
	body, err := proto.Marshal(&pbhttp.ClientTokenRequest{
		RequestType: pbhttp.ClientTokenRequestType_REQUEST_CLIENT_DATA_REQUEST,
		Request: &pbhttp.ClientTokenRequest_ClientData{ClientData: &pbhttp.ClientDataRequest{
			ClientId: librespot.ClientIdHex, ClientVersion: librespot.SpotifyLikeClientVersion(),
			Data: &pbhttp.ClientDataRequest_ConnectivitySdkData{ConnectivitySdkData: &pbdata.ConnectivitySdkData{DeviceId: deviceID, PlatformSpecificData: librespot.GetPlatformSpecificData()}},
		}},
	})
	if err != nil {
		return "", err
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "https://clienttoken.spotify.com/v1/clienttoken", bytes.NewReader(body))
	req.Header.Set("Accept", "application/x-protobuf")
	req.Header.Set("User-Agent", librespot.UserAgent())
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("client token status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	var answer pbhttp.ClientTokenResponse
	if err := proto.Unmarshal(data, &answer); err != nil {
		return "", err
	}
	if answer.GetResponseType() != pbhttp.ClientTokenResponseType_RESPONSE_GRANTED_TOKEN_RESPONSE {
		return "", fmt.Errorf("client token refused")
	}
	return answer.GetGrantedToken().GetToken(), nil
}
