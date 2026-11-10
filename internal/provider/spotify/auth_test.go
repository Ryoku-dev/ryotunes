package spotify

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/devgianlu/go-librespot/ap"
	"github.com/devgianlu/go-librespot/login5"
	spotifypb "github.com/devgianlu/go-librespot/proto/spotify"
	devicespb "github.com/devgianlu/go-librespot/proto/spotify/connectstate/devices"
	login5pb "github.com/devgianlu/go-librespot/proto/spotify/login5/v3"
	"github.com/devgianlu/go-librespot/session"
	"github.com/ryoku-dev/ryotunes/internal/bus"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
	providerapi "github.com/ryoku-dev/ryotunes/internal/provider"
)

const legacyCredentialFixture = `{"username":"fixture-user","auth_type":1,"auth_data":"AQIDBAU="}`
const legacyDeviceFixture = "01234567-89AB-cdef-0123-456789abcdef"
const storedCredentialFixture = `{"username":"fixture","data":"AQI="}`

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func offlineClient() *http.Client {
	return &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("connection refused")
	})}
}

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func writeLegacyCache(t *testing.T, dataDir, credentials, device string) string {
	t.Helper()
	legacyRoot := filepath.Join(dataDir, "spotify", "spotify")
	if err := os.MkdirAll(legacyRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyRoot, "credentials.json"), []byte(credentials), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyRoot, "device.id"), []byte(device), 0o600); err != nil {
		t.Fatal(err)
	}
	return legacyRoot
}

func newTestProvider(t *testing.T, opts Options) *Spotify {
	t.Helper()
	if opts.HTTPClient == nil {
		opts.HTTPClient = offlineClient()
	}
	provider, err := New(opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		provider.mu.Lock()
		provider.sess = nil
		provider.mu.Unlock()
		if err := provider.Close(); err != nil {
			t.Error(err)
		}
	})
	return provider
}

// assertLibrespotAccepts runs go-librespot's own validation on the device id
// the provider would use. The client cannot reach anything, so the session
// fails later, never on the id.
func assertLibrespotAccepts(t *testing.T, provider *Spotify) string {
	t.Helper()
	id, err := provider.deviceID()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err = session.NewSessionFromOptions(ctx, &session.Options{
		Log: provider.liblog, DeviceType: devicespb.DeviceType_COMPUTER, DeviceId: id,
		Credentials: session.StoredCredentials{Username: "fixture", Data: []byte{1}},
		ClientToken: "fixture", Client: offlineClient(),
	})
	if err == nil {
		t.Fatal("offline session unexpectedly connected")
	}
	if strings.Contains(err.Error(), "invalid device id") {
		t.Fatalf("go-librespot rejected device id %q: %v", id, err)
	}
	return id
}

func derivedDeviceID(uuid string) string {
	sum := sha1.Sum([]byte(strings.ToLower(uuid)))
	return hex.EncodeToString(sum[:])
}

func TestNewMigratesSpotify1xSessionToADeviceLibrespotAccepts(t *testing.T) {
	dataDir := t.TempDir()
	legacyRoot := writeLegacyCache(t, dataDir, legacyCredentialFixture, legacyDeviceFixture+"\n")
	provider := newTestProvider(t, Options{DataDir: dataDir})

	creds, err := readCredentials(filepath.Join(dataDir, "spotify", "credentials.json"))
	if err != nil {
		t.Fatal(err)
	}
	if creds.Username != "fixture-user" || string(creds.Data) != "\x01\x02\x03\x04\x05" {
		t.Fatalf("migrated credentials = %#v", creds)
	}
	if !provider.Status().Stored {
		t.Fatal("migrated credentials are not reported as stored")
	}
	if id := assertLibrespotAccepts(t, provider); id != derivedDeviceID(legacyDeviceFixture) {
		t.Fatalf("device id = %q, want the one derived from the 1.x id", id)
	}
	legacy, err := os.ReadFile(filepath.Join(legacyRoot, "credentials.json"))
	if err != nil || string(legacy) != legacyCredentialFixture {
		t.Fatalf("1.x rollback copy changed: %v", err)
	}
}

func TestDeviceIDReplacesUUIDCopiedByEarlierBuild(t *testing.T) {
	dataDir := t.TempDir()
	provider := newTestProvider(t, Options{DataDir: dataDir})
	if err := os.WriteFile(provider.devicePath, []byte(legacyDeviceFixture+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	id := assertLibrespotAccepts(t, provider)
	if id != derivedDeviceID(legacyDeviceFixture) {
		t.Fatalf("device id = %q, want the one derived from the UUID", id)
	}
	raw, err := os.ReadFile(provider.devicePath)
	if err != nil || strings.TrimSpace(string(raw)) != id {
		t.Fatalf("device.id was not rewritten: %v", err)
	}
}

func TestNewWarnsAndContinuesOnUnreadable1xCache(t *testing.T) {
	for _, test := range []struct {
		name        string
		credentials string
		device      string
		migrated    bool
	}{
		{name: "empty credentials", credentials: ``},
		{name: "null username", credentials: `{"username":null,"auth_type":1,"auth_data":"AQIDBAU="}`},
		{name: "empty device id", credentials: legacyCredentialFixture, device: "", migrated: true},
		{name: "32 character device id", credentials: legacyCredentialFixture, device: "0123456789abcdef0123456789abcdef", migrated: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			dataDir := t.TempDir()
			writeLegacyCache(t, dataDir, test.credentials, test.device)
			var logs lockedBuffer
			provider := newTestProvider(t, Options{DataDir: dataDir, Log: slog.New(slog.NewTextHandler(&logs, nil))})
			if !strings.Contains(logs.String(), "level=WARN") || !strings.Contains(logs.String(), "Spotify 1.x session") {
				t.Fatalf("no migration warning was logged: %q", logs.String())
			}
			if provider.Status().Stored != test.migrated {
				t.Fatalf("stored = %v, want %v", provider.Status().Stored, test.migrated)
			}
			assertLibrespotAccepts(t, provider)
		})
	}
}

func TestMigrationNeverOverwritesCurrentAccount(t *testing.T) {
	dataDir := t.TempDir()
	root := filepath.Join(dataDir, "spotify")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := writeCredentials(filepath.Join(root, "credentials.json"), session.StoredCredentials{Username: "current-user", Data: []byte("current-data")}); err != nil {
		t.Fatal(err)
	}
	writeLegacyCache(t, dataDir, legacyCredentialFixture, legacyDeviceFixture)
	newTestProvider(t, Options{DataDir: dataDir})
	creds, err := readCredentials(filepath.Join(root, "credentials.json"))
	if err != nil {
		t.Fatal(err)
	}
	if creds.Username != "current-user" || string(creds.Data) != "current-data" {
		t.Fatalf("current credentials were replaced: %#v", creds)
	}
}

func TestSignOutThenRestartDoesNotResurrect1xSession(t *testing.T) {
	dataDir := t.TempDir()
	writeLegacyCache(t, dataDir, legacyCredentialFixture, legacyDeviceFixture)
	provider := newTestProvider(t, Options{DataDir: dataDir})
	if err := provider.SignOut(); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{provider.credPath, provider.legacyCredPath} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("credential copy still exists at %s", path)
		}
	}
	if _, err := os.Stat(provider.devicePath); err != nil {
		t.Fatalf("device identity missing: %v", err)
	}
	if restarted := newTestProvider(t, Options{DataDir: dataDir}); restarted.Status().Stored {
		t.Fatal("restart restored a signed-out session")
	}
}

func TestSignOutThatCannotRemove1xCopyStaysSignedOutAfterRestart(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	dataDir := t.TempDir()
	legacyRoot := writeLegacyCache(t, dataDir, legacyCredentialFixture, legacyDeviceFixture)
	provider := newTestProvider(t, Options{DataDir: dataDir})
	if err := os.Chmod(legacyRoot, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(legacyRoot, 0o700); err != nil {
			t.Error(err)
		}
	})
	if err := provider.SignOut(); err == nil {
		t.Fatal("sign out reported removing a read-only 1.x copy")
	}
	if restarted := newTestProvider(t, Options{DataDir: dataDir}); restarted.Status().Stored {
		t.Fatal("restart restored the session from the 1.x copy")
	}
}

func writeStoredCredentials(t *testing.T, provider *Spotify) {
	t.Helper()
	if err := os.WriteFile(provider.credPath, []byte(storedCredentialFixture), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestRestoreFailureKeepsStoredCredentialsAndEmitsReason(t *testing.T) {
	events := make(chan model.SpotifyAuthEvent, 1)
	provider := newTestProvider(t, Options{
		DataDir: t.TempDir(),
		Events: bus.SinkFunc(func(event string, value any) {
			if event == protocol.EventSpotifyAuth {
				events <- value.(model.SpotifyAuthEvent)
			}
		}),
	})
	writeStoredCredentials(t, provider)
	if err := provider.Restore(context.Background()); err == nil {
		t.Fatal("restore unexpectedly succeeded")
	}
	if _, err := os.Stat(provider.credPath); err != nil {
		t.Fatalf("startup failure removed credentials: %v", err)
	}
	event := <-events
	if event.State != "restore_failed" || event.Message == nil || !strings.Contains(*event.Message, "could not be reached") {
		t.Fatalf("restore event = %#v", event)
	}
}

func TestRestoreIsBoundedWithoutCallerDeadline(t *testing.T) {
	hanging := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		<-request.Context().Done()
		return nil, request.Context().Err()
	})}
	provider := newTestProvider(t, Options{DataDir: t.TempDir(), HTTPClient: hanging, RestoreTimeout: 100 * time.Millisecond})
	writeStoredCredentials(t, provider)
	done := make(chan error, 1)
	go func() { done <- provider.Restore(context.Background()) }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("restore against a silent network succeeded")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("restore held restoreMu past its deadline")
	}
}

func TestStartupRestoreAndDroppedSessionRecoveryAreSerialized(t *testing.T) {
	started := make(chan struct{}, 2)
	release := make(chan struct{})
	blocking := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		started <- struct{}{}
		<-release
		return nil, errors.New("connection refused")
	})}
	provider := newTestProvider(t, Options{DataDir: t.TempDir(), HTTPClient: blocking})
	writeStoredCredentials(t, provider)
	dead := new(session.Session)
	provider.mu.Lock()
	provider.sess = dead
	provider.mu.Unlock()

	restoreDone := make(chan error, 1)
	go func() { restoreDone <- provider.Restore(context.Background()) }()
	<-started
	recoveryDone := make(chan error, 1)
	go func() {
		_, err := provider.recoverSession(context.Background(), dead)
		recoveryDone <- err
	}()
	select {
	case <-started:
		t.Fatal("recovery entered while startup restore was running")
	case <-time.After(50 * time.Millisecond):
	}
	release <- struct{}{}
	if err := <-restoreDone; err == nil {
		t.Fatal("startup restore unexpectedly succeeded")
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("recovery did not start after startup restore finished")
	}
	release <- struct{}{}
	if err := <-recoveryDone; err == nil {
		t.Fatal("recovery unexpectedly succeeded")
	}
}

func TestEachDroppedSessionGetsOneRecovery(t *testing.T) {
	var calls atomic.Int32
	// The client-token endpoint answers, but refuses: a definitive answer.
	counting := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		calls.Add(1)
		return &http.Response{StatusCode: http.StatusBadRequest, Body: io.NopCloser(strings.NewReader("")), Request: request}, nil
	})}
	provider := newTestProvider(t, Options{DataDir: t.TempDir(), HTTPClient: counting})
	writeStoredCredentials(t, provider)
	first := new(session.Session)
	provider.mu.Lock()
	provider.sess = first
	provider.mu.Unlock()
	for range 2 {
		if _, err := provider.recoverSession(context.Background(), first); err == nil {
			t.Fatal("recovery unexpectedly succeeded")
		}
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("recovery attempts for one dropped session = %d, want 1", got)
	}

	// A session installed later (after a recovery, a restore or a sign in)
	// that drops on the next suspend must be recovered again.
	second := new(session.Session)
	provider.mu.Lock()
	provider.sess = second
	provider.mu.Unlock()
	if _, err := provider.recoverSession(context.Background(), second); err == nil {
		t.Fatal("recovery unexpectedly succeeded")
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("recovery attempts after a second drop = %d, want 2", got)
	}
}

func TestTransientRecoveryFailureKeepsSessionRecoverable(t *testing.T) {
	var calls atomic.Int32
	unreachable := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return nil, &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ENETUNREACH}
	})}
	provider := newTestProvider(t, Options{DataDir: t.TempDir(), HTTPClient: unreachable})
	writeStoredCredentials(t, provider)
	dead := new(session.Session)
	provider.mu.Lock()
	provider.sess = dead
	provider.mu.Unlock()
	for attempt := 1; attempt <= 3; attempt++ {
		_, err := provider.recoverSession(context.Background(), dead)
		if err == nil {
			t.Fatal("recovery unexpectedly succeeded")
		}
		if errors.Is(err, providerapi.ErrSignInRequired) {
			t.Fatalf("attempt %d: an unreachable network was reported as signed out: %v", attempt, err)
		}
		if got := calls.Load(); got != int32(attempt) {
			t.Fatalf("attempt %d: recovery requests = %d, want one per attempt", attempt, got)
		}
	}
}

func TestTransientFailureSeparatesLibrespotRefusalsFromNetwork(t *testing.T) {
	for _, test := range []struct {
		name      string
		err       error
		transient bool
	}{
		{name: "accesspoint refusal", err: fmt.Errorf("failed authenticating accesspoint with stored credentials: %w",
			&ap.AccesspointLoginError{Message: &spotifypb.APLoginFailed{ErrorCode: spotifypb.ErrorCode_BadCredentials.Enum()}})},
		{name: "login5 refusal", err: fmt.Errorf("failed authenticating with login5: %w", &login5.LoginError{Code: login5pb.LoginError_INVALID_CREDENTIALS})},
		{name: "login5 outage", err: fmt.Errorf("failed authenticating with login5: %w", &login5.HTTPError{StatusCode: http.StatusServiceUnavailable}), transient: true},
		{name: "login5 client error", err: fmt.Errorf("failed authenticating with login5: %w", &login5.HTTPError{StatusCode: http.StatusBadRequest})},
		{name: "accesspoint dial", err: fmt.Errorf("failed getting accesspoint from resolver: %w",
			&url.Error{Op: "Get", URL: "https://apresolve.spotify.com", Err: &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}}), transient: true},
		{name: "restore deadline", err: fmt.Errorf("failed connecting: %w", context.DeadlineExceeded), transient: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := transientFailure(classifySignIn(test.err)); got != test.transient {
				t.Fatalf("transient = %v, want %v", got, test.transient)
			}
		})
	}
}

func TestAccesspointRefusalSaysWhy(t *testing.T) {
	refusal := func(code spotifypb.ErrorCode) string {
		err := fmt.Errorf("failed authenticating with accesspoint: %w", &ap.AccesspointLoginError{Message: &spotifypb.APLoginFailed{ErrorCode: code.Enum()}})
		return classifySignIn(err).Error()
	}
	if got := refusal(spotifypb.ErrorCode_PremiumAccountRequired); !strings.Contains(got, "Premium") {
		t.Fatalf("a free account reads %q, it should say Premium is needed", got)
	}
	if bad, other := refusal(spotifypb.ErrorCode_BadCredentials), refusal(spotifypb.ErrorCode_InvalidAppKey); bad == other {
		t.Fatalf("bad credentials read like any other refusal: %q", bad)
	}
}
