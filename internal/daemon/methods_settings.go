package daemon

import (
	"context"
	"strings"

	"github.com/ryoku-dev/ryotunes/internal/integrations/discord"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/netx"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

func init() {
	register(protocol.MethodGetSettings, typed(getSettings))
	register(protocol.MethodSetSetting, typed(setSetting))
	register(protocol.MethodGetPersonal, typed(getPersonal))
	register(protocol.MethodSetPersonal, typed(setPersonal))
	register(protocol.MethodGetStreamClients, typed(getStreamClients))
	register(protocol.MethodClearCaches, typed(clearCaches))
}

func getSettings(_ context.Context, a *App, _ protocol.Null) (model.SettingsSnapshot, error) {
	return a.Store.SettingsSnapshot(), nil
}
func setSetting(_ context.Context, a *App, p protocol.SetSettingParams) (protocol.Null, error) {
	if err := model.ValidateSetting(p.Key, p.Value); err != nil {
		return protocol.Null{}, err
	}
	if err := a.Store.SetSetting(p.Key, p.Value); err != nil {
		return protocol.Null{}, err
	}
	settings := a.Store.Settings()
	switch model.SettingKey(p.Key) {
	case model.SettingProxy:
		if err := netx.SetProxy(settings.Proxy); err != nil {
			return protocol.Null{}, err
		}
		a.YouTube.SetProxy(settings.Proxy)
	case model.SettingQuality:
		if err := a.Store.ClearStreams(); err != nil {
			return protocol.Null{}, err
		}
	case model.SettingLyricsBoidu:
		if err := a.Store.ClearLyrics(); err != nil {
			return protocol.Null{}, err
		}
	case model.SettingLyricsRomanize:
		a.Bus.Emit(protocol.EventLyricsChanged, protocol.Null{})
	case model.SettingDiscordRPC:
		a.mu.Lock()
		client := a.Discord
		if settings.DiscordRPC && client == nil {
			client = discord.New(a.ctx, discord.Options{Enabled: true, PresenceName: settings.DiscordPresenceName, Log: a.Log})
			a.Discord = client
		}
		a.mu.Unlock()
		if client != nil {
			client.SetEnabled(settings.DiscordRPC)
		}
	case model.SettingDiscordPresenceName:
		a.mu.Lock()
		client := a.Discord
		a.mu.Unlock()
		if client != nil {
			if err := client.SetPresenceName(settings.DiscordPresenceName); err != nil {
				return protocol.Null{}, err
			}
		}
	case model.SettingDisabledStreamClients:
		names := make([]string, len(settings.DisabledStreamClients))
		for i, client := range settings.DisabledStreamClients {
			names[i] = string(client)
		}
		a.YouTube.SetDisabledStreamClients(strings.Join(names, ","))
	case model.SettingVolume:
		if err := a.Queue.SetVolume(settings.Volume); err != nil {
			return protocol.Null{}, err
		}
	}
	return protocol.Null{}, nil
}
func getPersonal(_ context.Context, a *App, _ protocol.Null) (protocol.PersonalResult, error) {
	return protocol.PersonalResult{Personal: a.Store.Personal()}, nil
}
func setPersonal(_ context.Context, a *App, p protocol.SetPersonalParams) (protocol.Null, error) {
	if err := a.Store.SetPersonal(p.Personal); err != nil {
		return protocol.Null{}, err
	}
	a.Bus.Emit(protocol.EventPersonalChanged, p.Personal)
	return protocol.Null{}, nil
}
func getStreamClients(_ context.Context, a *App, _ protocol.Null) ([]model.StreamClient, error) {
	return a.YouTube.GetStreamClients(), nil
}
func clearCaches(ctx context.Context, a *App, p protocol.ClearCachesParams) (protocol.ClearCachesResult, error) {
	if err := a.Store.ClearCaches(); err != nil {
		return protocol.ClearCachesResult{}, err
	}
	rotate := true
	if p.Rotate != nil {
		rotate = *p.Rotate
	}
	refreshed := false
	if rotate {
		if err := a.YouTube.RefreshVisitorData(ctx); err != nil {
			return protocol.ClearCachesResult{}, err
		}
		refreshed = true
	}
	return protocol.ClearCachesResult{VisitorDataRefreshed: refreshed}, nil
}
