package settings

import (
	"strings"

	"github.com/ryoku-dev/ryotunes/internal/client/state"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

func errorMessage(err error, fallback string) string {
	if err != nil && strings.TrimSpace(err.Error()) != "" {
		return err.Error()
	}
	return fallback
}

func toastError(e *env.Env, err error, fallback string) {
	if e.Toast != nil {
		e.Toast(errorMessage(err, fallback), env.ToastError)
	}
}

func setSetting(e *env.Env, key model.SettingKey, value, success string, done func()) {
	act.Call[protocol.Null](e, protocol.MethodSetSetting, protocol.SetSettingParams{Key: string(key), Value: value}, func(_ protocol.Null, err error) {
		if err != nil {
			toastError(e, err, "Could not save setting")
			return
		}
		e.Store.SetSetting(string(key), value)
		if success != "" && e.Toast != nil {
			e.Toast(success, env.ToastSuccess)
		}
		if done != nil {
			done()
		}
	})
}

func saveClientPreferences(e *env.Env, preferences state.ClientPreferences) {
	e.Store.SetClientPreferences(preferences)
	if err := e.Store.SaveClientPreferences(); err != nil && e.Toast != nil {
		e.Toast(errorMessage(err, "Could not save client preferences"), env.ToastError)
	}
}

func callNull(e *env.Env, method string, params any, fallback string, done func()) {
	act.Call[protocol.Null](e, method, params, func(_ protocol.Null, err error) {
		if err != nil {
			toastError(e, err, fallback)
			return
		}
		if done != nil {
			done()
		}
	})
}

// Refresh reloads page-owned settings data while preserving the current page.
func (p *page) Refresh(e *env.Env) {
	p.refreshed = true
	p.generation++
	generation := p.generation
	p.loading = true
	p.loadError = ""

	act.Call[model.SettingsSnapshot](e, protocol.MethodGetSettings, protocol.Null{}, func(settings model.SettingsSnapshot, err error) {
		if generation != p.generation {
			return
		}
		p.loading = false
		if err != nil {
			p.loadError = errorMessage(err, "Could not load settings")
			return
		}
		e.Store.ReplaceSettings(settings)
	})
	act.Call[[]model.StreamClient](e, protocol.MethodGetStreamClients, protocol.Null{}, func(clients []model.StreamClient, err error) {
		if generation == p.generation && err == nil {
			p.streamClients = append(p.streamClients[:0], clients...)
		}
	})
	act.Call[model.DownloadSettings](e, protocol.MethodGetDownloadSettings, protocol.Null{}, func(settings model.DownloadSettings, err error) {
		if generation == p.generation && err == nil {
			p.downloads = settings
			p.download.path.initialized = false
		}
	})
	act.Call[model.AccountIdentities](e, protocol.MethodGetAccountIdentities, protocol.Null{}, func(identities model.AccountIdentities, err error) {
		if generation == p.generation && err == nil {
			p.identities = append(p.identities[:0], identities...)
		}
	})
	act.Call[model.DiscordStatus](e, protocol.MethodDiscordStatus, protocol.Null{}, func(status model.DiscordStatus, err error) {
		if generation == p.generation && err == nil {
			p.discord = status
		}
	})
	act.Call[model.LastfmStatus](e, protocol.MethodLastfmStatus, protocol.Null{}, func(status model.LastfmStatus, err error) {
		if generation == p.generation && err == nil {
			p.lastfm = status
		}
	})
	act.Call[protocol.Hello](e, protocol.MethodHello, protocol.Null{}, func(hello protocol.Hello, err error) {
		if generation == p.generation && err == nil {
			p.daemonVersion, p.protocol = hello.Daemon, hello.Protocol
		}
	})
	limit := 1
	act.Call[model.LogsResult](e, protocol.MethodGetLogs, protocol.GetLogsParams{Limit: &limit}, func(result model.LogsResult, err error) {
		if generation == p.generation && err == nil {
			p.diagnostics = result
		}
	})
	p.local.busy = false
	p.loadLocal(e, generation)
}

func (p *page) loadLocal(e *env.Env, generation uint64) {
	if p.local.busy {
		return
	}
	p.local.busy = true
	act.Call[model.LocalLibrary](e, protocol.MethodGetLocalLibrary, protocol.Null{}, func(local model.LocalLibrary, err error) {
		if generation != p.generation {
			return
		}
		p.local.busy, p.local.loaded = false, true
		if err != nil {
			p.local.errorMessage = err.Error()
			return
		}
		p.local.errorMessage = ""
		p.local.library = local
	})
}
