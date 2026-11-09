package spotify

import (
	"bytes"
	"context"
	"crypto/rand"
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
	"strings"
	"time"

	librespot "github.com/devgianlu/go-librespot"
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
}

// Restore restores cached Spotify credentials and emits the startup result event.
func (p *Spotify) Restore(ctx context.Context) error {
	p.authMu.Lock()
	generation := p.authGeneration
	p.authMu.Unlock()
	if !fileExists(p.credPath) {
		return nil
	}
	sess, profile, err := p.restoreSession(ctx)
	if err != nil {
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
	p.mu.Lock()
	p.recoverUsed = false
	p.mu.Unlock()
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
			message := "Spotify sign-in failed: " + err.Error()
			p.emitAuth("error", nil, nil, &message)
		}
	}()
	return true, nil
}

// SignOut closes the session, removes credentials and device identity, and emits signed_out.
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
	for _, path := range []string{p.credPath, p.devicePath} {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			joined = errors.Join(joined, err)
		}
	}
	p.recoverMu.Lock()
	p.recoverUsed = false
	p.recoverMu.Unlock()
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
	profile, err := p.premiumProfile(ctx, sess)
	if err != nil {
		sess.Close()
		return err
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
	p.recoverUsed = false
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
	profile, err := p.premiumProfile(ctx, sess)
	if err != nil {
		sess.Close()
		return nil, accountProfile{}, err
	}

	p.mu.Lock()
	p.clientToken = clientToken
	p.mu.Unlock()
	return sess, profile, nil
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
		return "", "", fmt.Errorf("Spotify refused the session")
	}
	token, err := config.Exchange(ctx, cb.code, oauth2.SetAuthURLParam("code_verifier", verifier))
	if err != nil {
		return "", "", fmt.Errorf("Spotify refused the session")
	}
	username, _ := token.Extra("username").(string)
	return token.AccessToken, username, nil
}

func (p *Spotify) premiumProfile(ctx context.Context, sess *session.Session) (accountProfile, error) {
	deadline := time.Now().Add(5 * time.Second)
	var profile accountProfile
	for {
		token, err := sess.Spclient().GetAccessToken(ctx, false)
		if err != nil {
			return profile, classifySignIn(err)
		}
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.spotify.com/v1/me", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := p.http.Do(req)
		if err != nil {
			return profile, fmt.Errorf("Spotify could not be reached")
		}
		if resp.StatusCode == http.StatusUnauthorized {
			_ = resp.Body.Close()
			return profile, fmt.Errorf("the stored credentials are no longer valid")
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			_ = resp.Body.Close()
			return profile, fmt.Errorf("Spotify refused the session")
		}
		err = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&profile)
		_ = resp.Body.Close()
		if err != nil {
			return profile, fmt.Errorf("Spotify refused the session")
		}
		if profile.DisplayName == "" {
			profile.DisplayName = profile.ID
		}
		if profile.Product == "premium" {
			return profile, nil
		}
		if profile.Product != "" {
			return profile, providerapi.ErrNotPremium
		}
		if time.Now().After(deadline) {
			return profile, nil
		}
		select {
		case <-ctx.Done():
			return profile, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func (p *Spotify) deviceID() (string, error) {
	if raw, err := os.ReadFile(p.devicePath); err == nil {
		id := strings.TrimSpace(string(raw))
		if decoded, err := hex.DecodeString(id); err == nil && len(decoded) == 20 {
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

func (p *Spotify) saveCredentials(sess *session.Session) error {
	body, err := json.Marshal(storedCredentials{Username: sess.Username(), Data: base64.StdEncoding.EncodeToString(sess.StoredCredentials())})
	if err != nil {
		return err
	}
	return atomicWrite(p.credPath, append(body, '\n'), 0o600)
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
		for _, n := range names {
			if v := raw[n]; v != nil {
				return v
			}
		}
		return nil
	}
	var username string
	_ = json.Unmarshal(pick("username", "Username"), &username)
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

func classifySignIn(err error) error {
	message := err.Error()
	switch {
	case containsAny(message, "travel restriction", "country", "region"):
		return fmt.Errorf("the account is out of its home region")
	case containsAny(message, "bad credentials", "invalid credentials", "unauthorized"):
		return fmt.Errorf("the stored credentials are no longer valid")
	case containsAny(message, "connection", "timed out", "timeout", "dns", "network", "resolve"):
		return fmt.Errorf("Spotify could not be reached")
	default:
		return fmt.Errorf("Spotify refused the session")
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
