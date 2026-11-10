package daemon

import (
	"context"
	"errors"
	"fmt"

	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
	"github.com/ryoku-dev/ryotunes/internal/provider"
)

func init() {
	register(protocol.MethodGetProvider, typed(getProvider))
	register(protocol.MethodSetProvider, typed(setProvider))
	register(protocol.MethodSignIn, typed(signIn))
	register(protocol.MethodSignOut, typed(signOut))
	register(protocol.MethodGetAccount, typed(getAccount))
	register(protocol.MethodGetAccountIdentities, typed(getAccountIdentities))
	register(protocol.MethodSwitchAccount, typed(switchAccount))
	register(protocol.MethodSpotifyStatus, typed(spotifyStatus))
	register(protocol.MethodSpotifySignIn, typed(spotifySignIn))
	register(protocol.MethodSpotifySignOut, typed(spotifySignOut))
	register(protocol.MethodSoundCloudStatus, typed(soundCloudStatus))
	register(protocol.MethodSoundCloudSignIn, typed(soundCloudSignIn))
	register(protocol.MethodSoundCloudSignOut, typed(soundCloudSignOut))
	register(protocol.MethodTidalStatus, typed(tidalStatus))
	register(protocol.MethodTidalSignIn, typed(tidalSignIn))
	register(protocol.MethodTidalSignOut, typed(tidalSignOut))
	register(protocol.MethodTidalSetCredentials, typed(tidalSetCredentials))
}

func getProvider(_ context.Context, a *App, _ protocol.Null) (protocol.ProviderResult, error) {
	a.mu.Lock()
	selected := a.provider
	a.mu.Unlock()
	if selected == "" {
		selected = string(model.ProviderYouTube)
	}
	return protocol.ProviderResult{Provider: model.Provider(selected)}, nil
}
func setProvider(_ context.Context, a *App, p protocol.ProviderParams) (protocol.ProviderResult, error) {
	if _, ok := a.Providers.Get(p.Provider); !ok {
		return protocol.ProviderResult{}, fmt.Errorf("unknown provider: %s", p.Provider)
	}
	if err := a.Store.KV().Set("provider", string(p.Provider)); err != nil {
		return protocol.ProviderResult{}, err
	}
	a.mu.Lock()
	a.provider = string(p.Provider)
	a.mu.Unlock()
	result := protocol.ProviderResult{Provider: p.Provider}
	a.Bus.Emit(protocol.EventProviderChanged, result)
	return result, nil
}
func signIn(ctx context.Context, a *App, _ protocol.Null) (protocol.AuthFlowResult, error) {
	flow, err := a.YouTube.SignIn(ctx)
	if err != nil {
		return protocol.AuthFlowResult{}, err
	}
	finish := func(error) {
		defer a.background.Done()
		// a failed watch ends the flow, not a session the daemon may still hold
		a.Bus.Emit(protocol.EventAuthChanged, a.YouTube.CurrentAccount())
	}
	if flow.Done != nil {
		a.background.Add(1)
		go func() {
			select {
			case err := <-flow.Done:
				finish(err)
			case <-a.ctx.Done():
				// the watch only ends with the YouTube client, which closes after this is waited on
				a.background.Done()
			}
		}()
	} else if flow.URL == "" {
		a.background.Add(1)
		go finish(nil)
	}
	return protocol.AuthFlowResult{URL: flow.URL}, nil
}
func signOut(_ context.Context, a *App, _ protocol.Null) (protocol.Null, error) {
	if err := a.YouTube.SignOut(); err != nil {
		return protocol.Null{}, err
	}
	a.Bus.Emit(protocol.EventAuthChanged, model.Account{})
	return protocol.Null{}, nil
}
func getAccount(ctx context.Context, a *App, _ protocol.Null) (model.Account, error) {
	return a.YouTube.Account(ctx)
}
func getAccountIdentities(ctx context.Context, a *App, _ protocol.Null) (model.AccountIdentities, error) {
	return a.YouTube.AccountIdentities(ctx)
}
func switchAccount(ctx context.Context, a *App, p protocol.SwitchAccountParams) (model.Account, error) {
	if err := a.YouTube.SwitchAccount(ctx, p.SelectionKey); err != nil {
		return model.Account{}, err
	}
	account, err := a.YouTube.Account(ctx)
	if err == nil {
		a.Bus.Emit(protocol.EventAuthChanged, account)
	}
	return account, err
}
func spotifyStatus(_ context.Context, a *App, _ protocol.Null) (model.SpotifyStatus, error) {
	return a.Spotify.Status(), nil
}
func spotifySignIn(ctx context.Context, a *App, _ protocol.Null) (protocol.StartedResult, error) {
	_, err := a.Spotify.SignIn(ctx)
	return protocol.StartedResult{Started: true}, providerError(model.ProviderSpotify, err)
}
func spotifySignOut(_ context.Context, a *App, _ protocol.Null) (protocol.SignedOutResult, error) {
	return protocol.SignedOutResult{SignedIn: false}, providerError(model.ProviderSpotify, a.Spotify.SignOut())
}
func soundCloudStatus(_ context.Context, a *App, _ protocol.Null) (model.SoundCloudStatus, error) {
	return a.SoundCloud.Status(), nil
}
func soundCloudSignIn(ctx context.Context, a *App, _ protocol.Null) (protocol.AuthFlowResult, error) {
	url, err := a.SoundCloud.SignIn(ctx)
	if err != nil {
		return protocol.AuthFlowResult{}, providerError(model.ProviderSoundCloud, err)
	}
	return protocol.AuthFlowResult{URL: url}, nil
}
func soundCloudSignOut(_ context.Context, a *App, _ protocol.Null) (protocol.SignedOutResult, error) {
	return protocol.SignedOutResult{SignedIn: false}, providerError(model.ProviderSoundCloud, a.SoundCloud.SignOut())
}
func tidalStatus(_ context.Context, a *App, _ protocol.Null) (model.TidalStatus, error) {
	return a.Tidal.Status(), nil
}
func tidalSignIn(ctx context.Context, a *App, _ protocol.Null) (protocol.StartedResult, error) {
	return protocol.StartedResult{Started: a.Tidal.SignIn(ctx)}, nil
}
func tidalSignOut(_ context.Context, a *App, _ protocol.Null) (protocol.SignedOutResult, error) {
	return protocol.SignedOutResult{SignedIn: false}, providerError(model.ProviderTidal, a.Tidal.SignOut())
}
func tidalSetCredentials(_ context.Context, a *App, p protocol.TidalCredentialsParams) (model.TidalStatus, error) {
	if err := a.Tidal.SetCredentials(p); err != nil {
		return model.TidalStatus{}, providerError(model.ProviderTidal, err)
	}
	return a.Tidal.Status(), nil
}

func providerError(id model.Provider, err error) error {
	if err == nil {
		return nil
	}
	switch id {
	case model.ProviderSpotify:
		if errors.Is(err, provider.ErrSignInRequired) {
			return coded(protocol.ErrorSpotifySignedOut, "Sign in to Spotify")
		}
		return coded(protocol.ErrorSpotify, "%v", err)
	case model.ProviderSoundCloud:
		if errors.Is(err, provider.ErrSignInRequired) {
			return coded(protocol.ErrorSoundCloud, "Sign in to SoundCloud")
		}
		return coded(protocol.ErrorSoundCloud, "%v", err)
	case model.ProviderTidal:
		if errors.Is(err, provider.ErrSignInRequired) {
			return coded(protocol.ErrorTidal, "Sign in to TIDAL")
		}
		return coded(protocol.ErrorTidal, "%v", err)
	default:
		return err
	}
}
