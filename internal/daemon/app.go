package daemon

import (
	"context"
	"errors"
	"log/slog"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/ryoku-dev/ryotunes/internal/bus"
	"github.com/ryoku-dev/ryotunes/internal/diag"
	"github.com/ryoku-dev/ryotunes/internal/downloads"
	"github.com/ryoku-dev/ryotunes/internal/integrations/discord"
	"github.com/ryoku-dev/ryotunes/internal/integrations/lastfm"
	"github.com/ryoku-dev/ryotunes/internal/integrations/listentogether"
	"github.com/ryoku-dev/ryotunes/internal/integrations/mpris"
	"github.com/ryoku-dev/ryotunes/internal/integrations/tray"
	"github.com/ryoku-dev/ryotunes/internal/lyrics"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/netx"
	"github.com/ryoku-dev/ryotunes/internal/player"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
	"github.com/ryoku-dev/ryotunes/internal/provider"
	"github.com/ryoku-dev/ryotunes/internal/provider/browsercookies"
	"github.com/ryoku-dev/ryotunes/internal/provider/local"
	"github.com/ryoku-dev/ryotunes/internal/provider/radio"
	"github.com/ryoku-dev/ryotunes/internal/provider/soundcloud"
	"github.com/ryoku-dev/ryotunes/internal/provider/spotify"
	"github.com/ryoku-dev/ryotunes/internal/provider/tidal"
	"github.com/ryoku-dev/ryotunes/internal/provider/youtube"
	"github.com/ryoku-dev/ryotunes/internal/queue"
	"github.com/ryoku-dev/ryotunes/internal/store"
	"github.com/ryoku-dev/ryotunes/internal/version"
)

// New constructs a complete daemon application. Network-backed restoration is
// started only after every local dependency has been installed on App.
func New(ctx context.Context, bootstrapLog *slog.Logger) (_ *App, err error) {
	if bootstrapLog == nil {
		bootstrapLog = slog.Default()
	}
	dirs, err := store.ResolveDirs()
	if err != nil {
		return nil, err
	}
	diagnostics, err := diag.New(filepath.Join(dirs.Data, "logs"))
	if err != nil {
		return nil, err
	}
	log := slog.New(diagnostics)
	slog.SetDefault(log)

	appCtx, cancel := context.WithCancel(ctx)
	a := &App{Log: log, Diag: diagnostics, Dirs: dirs, Bus: &bus.Fanout{}, ctx: appCtx, cancel: cancel}
	defer func() {
		if err != nil {
			cancel()
			if closeErr := a.Close(context.Background()); closeErr != nil {
				bootstrapLog.Error("closing partially constructed daemon", "error", closeErr)
			}
		}
	}()

	a.Store, err = store.Open(dirs, log)
	if err != nil {
		return nil, err
	}
	settings := a.Store.Settings()
	if err := netx.SetProxy(settings.Proxy); err != nil {
		return nil, err
	}
	a.Player, err = player.New(player.Options{CacheDir: dirs.Cache, Log: log, LowResource: settings.LowResourceMode})
	if err != nil {
		return nil, err
	}
	a.Player.SetPositionCadence(time.Second)
	a.EngineVersion = a.Player.EngineVersion()

	kv := a.Store.KV()
	a.equalizerPlayer = a.Player
	a.equalizerStore = kv
	if err := a.restoreEqualizer(); err != nil {
		return nil, err
	}
	disabled := make([]string, len(settings.DisabledStreamClients))
	for i, client := range settings.DisabledStreamClients {
		disabled[i] = string(client)
	}
	a.YouTube, err = youtube.New(youtube.Options{
		KV: kv, Log: log, DataDir: dirs.Data, Proxy: settings.Proxy, Bus: a.Bus,
		DisabledStreamClients: strings.Join(disabled, ","),
	})
	if err != nil {
		return nil, err
	}
	a.Spotify, err = spotify.New(spotify.Options{DataDir: dirs.Data, Events: a.Bus, Log: log})
	if err != nil {
		return nil, err
	}
	a.SoundCloud, err = soundcloud.New(soundcloud.Options{DataDir: dirs.Data, KV: kv, Bus: a.Bus, Log: log})
	if err != nil {
		return nil, err
	}
	a.Tidal, err = tidal.New(tidal.Options{KV: kv, Bus: a.Bus, Log: log, UserAgent: "Ryotunes/" + version.Version})
	if err != nil {
		return nil, err
	}
	a.Local, err = local.New(local.Options{Index: a.Store, CacheDir: filepath.Join(dirs.Cache, "local"), Log: log})
	if err != nil {
		return nil, err
	}
	a.Radio = radio.New(radio.Options{Cache: a.Store, Log: log})
	a.Providers = provider.NewRegistry(a.YouTube, a.Spotify, a.SoundCloud, a.Tidal, a.Local, a.Radio)

	a.Queue, err = queue.New(queue.Options{
		Player: a.Player, Registry: a.Providers, Persist: queuePersist{kv: kv},
		Hooks: queueHooks{app: a}, Bus: a.Bus, Log: log,
	})
	if err != nil {
		return nil, err
	}
	a.Lyrics = lyrics.New(lyrics.Options{
		Cache: lyricsCache{store: a.Store}, Settings: kv, YouTube: a.YouTube,
		Spotify: a.Spotify, Log: log, Version: version.Version,
		Romanize: settings.LyricsRomanize, LocalPath: local.PathFromID,
	})
	a.Downloads, err = downloads.New(downloads.Options{
		DataDir: dirs.Data, Sink: a.Bus, Searcher: youtubeDownloadSearcher{youtube: a.YouTube},
		SoundCloud: soundCloudDownloadResolver{soundcloud: a.SoundCloud},
		Lyrics:     downloadLyricsSource{lyrics: a.Lyrics}, Log: log,
	})
	if err != nil {
		return nil, err
	}
	serverURL, _ := kv.Get("lt_server_url")
	a.LT = listentogether.New(listentogether.Options{ServerURL: serverURL, Host: listenTogetherHost{app: a}, Sink: a.Bus, Log: log})
	a.Lastfm, err = lastfm.New(appCtx, lastfm.Options{Store: kv, Events: a.Bus, Log: log})
	if err != nil {
		return nil, err
	}
	if settings.DiscordRPC {
		a.Discord = discord.New(appCtx, discord.Options{Enabled: true, PresenceName: settings.DiscordPresenceName, Log: log})
	}
	if a.MPRIS, err = mpris.New(appCtx, mprisController{app: a}, log); err != nil {
		log.Warn("MPRIS unavailable", "error", err)
		a.MPRIS = nil
	}
	if a.Tray, err = tray.New(appCtx, trayActions{app: a}, log); err != nil {
		log.Warn("tray unavailable", "error", err)
		a.Tray = nil
	}

	a.provider = string(model.ProviderYouTube)
	if selected, ok := kv.Get("provider"); ok {
		if _, valid := a.Providers.Get(model.Provider(selected)); valid {
			a.provider = selected
		}
	}

	a.Bus.Attach(bus.SinkFunc(func(event string, _ any) {
		switch event {
		case protocol.EventPosition, protocol.EventDuration:
			a.syncIntegrations(true)
		case protocol.EventPlaybackState, protocol.EventNowPlaying, protocol.EventQueueChanged,
			protocol.EventQueueIndex, protocol.EventStopAfterCurrent, protocol.EventVolume:
			a.syncIntegrations(true)
		}
	}))
	a.syncIntegrations(false)

	a.background.Add(1)
	go func() {
		defer a.background.Done()
		if restoreErr := a.Spotify.Restore(appCtx); restoreErr != nil && !errors.Is(restoreErr, context.Canceled) {
			log.Warn("Spotify credential restore failed", "error", restoreErr)
		}
	}()
	a.background.Add(1)
	go func() {
		defer a.background.Done()
		a.restoreBrowserSessions(appCtx)
	}()
	if folders, foldersErr := a.Store.LocalFolders(); foldersErr != nil {
		log.Warn("reading local music folders", "error", foldersErr)
	} else if len(folders) != 0 {
		a.background.Add(1)
		go func() {
			defer a.background.Done()
			a.scanLocalOnStart()
		}()
	}
	return a, nil
}

func (a *App) scanLocalOnStart() {
	if _, err := a.ScanLocal(a.ctx); err != nil && !errors.Is(err, context.Canceled) {
		a.Log.Warn("scanning local music at startup", "error", err)
	}
}

// browserRestoreTimeout bounds the start-up re-read so a black-holed network
// or a slow keyring never keeps it running.
const browserRestoreTimeout = 30 * time.Second

// restoreBrowserSessions is the start-up re-read of the remembered browser
// stores for YouTube and SoundCloud. Subscribers are answered from memory
// meanwhile; a changed session arrives as an auth event.
func (a *App) restoreBrowserSessions(ctx context.Context) {
	if err := browsercookies.RemoveStaleCopies(); err != nil {
		a.Log.Warn("removing stale browser cookie copies", "error", err)
	}
	ctx, cancel := context.WithTimeout(ctx, browserRestoreTimeout)
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := a.SoundCloud.Restore(ctx); err != nil && !errors.Is(err, context.Canceled) {
			a.Log.Info("SoundCloud browser session was not re-read at start", "error", err)
		}
	}()
	if err := a.YouTube.Restore(ctx); err != nil && !errors.Is(err, context.Canceled) {
		a.Log.Info("YouTube session was not confirmed at start", "error", err)
	}
	<-done
}

// ScanLocal refreshes configured folders and emits local-changed when the index changes.
func (a *App) ScanLocal(ctx context.Context) (model.LocalLibrary, error) {
	return a.scanLocal(ctx, func() (model.LocalLibrary, error) {
		return a.Local.LocalLibrary(ctx)
	})
}

// AddLocalFolder adds, scans, and reports changes to the local index.
func (a *App) AddLocalFolder(ctx context.Context, path string) (model.LocalLibrary, error) {
	return a.scanLocal(ctx, func() (model.LocalLibrary, error) {
		return a.Local.AddFolder(ctx, path)
	})
}

// RemoveLocalFolder removes, scans, and reports changes to the local index.
func (a *App) RemoveLocalFolder(ctx context.Context, path string) (model.LocalLibrary, error) {
	return a.scanLocal(ctx, func() (model.LocalLibrary, error) {
		return a.Local.RemoveFolder(ctx, path)
	})
}

func (a *App) scanLocal(_ context.Context, operation func() (model.LocalLibrary, error)) (model.LocalLibrary, error) {
	before, err := a.Store.AllLocalTracks()
	if err != nil {
		return model.LocalLibrary{}, err
	}
	library, err := operation()
	if err != nil {
		return model.LocalLibrary{}, err
	}
	after, err := a.Store.AllLocalTracks()
	if err != nil {
		return model.LocalLibrary{}, err
	}
	if reflect.DeepEqual(before, after) {
		return library, nil
	}
	present := make(map[string]struct{}, len(after))
	for _, track := range after {
		present[track.Path] = struct{}{}
	}
	removed := make([]string, 0)
	for _, track := range before {
		if _, ok := present[track.Path]; !ok {
			removed = append(removed, local.SongID(track.Path))
		}
	}
	a.Bus.Emit(protocol.EventLocalChanged, model.LocalChanged{Removed: removed})
	return library, nil
}

func (a *App) syncIntegrations(broadcastLT bool) {
	if a.Queue == nil {
		return
	}
	playback := a.Queue.Playback()
	queueState := a.Queue.Snapshot()
	var track *model.Track
	if queueState.CurrentIndex >= 0 && queueState.CurrentIndex < len(queueState.Items) {
		copy := queueState.Items[queueState.CurrentIndex]
		track = &copy
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.MPRIS != nil {
		a.MPRIS.Update(playback, queueState)
	}
	if a.Tray != nil {
		a.Tray.SetPlaying(playback.Now != nil && !playback.Paused)
	}
	if a.Discord != nil {
		a.Discord.Update(playback.Now, playback.Paused, playback.Position, playback.Duration)
	}
	if a.Lastfm != nil {
		a.Lastfm.Update(track, playback.Position, playback.Duration)
	}
	if broadcastLT && a.LT != nil {
		a.LT.BroadcastSnapshot()
	}
}

// Quit requests the lifecycle loop to tear down the daemon.
func (a *App) Quit() {
	if a == nil {
		return
	}
	if a.Queue != nil {
		_ = a.Queue.Stop()
	}
	if a.cancel != nil {
		a.cancel()
	}
}

// Close tears the daemon down in dependency order.
func (a *App) Close(ctx context.Context) error {
	if a == nil {
		return nil
	}
	var errs []error
	if a.Server != nil {
		if err := a.Server.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if a.cancel != nil {
		a.cancel()
	}
	if a.Server != nil {
		a.Server.Wait()
	}
	a.background.Wait()
	if a.Downloads != nil {
		if err := a.Downloads.Shutdown(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	a.mu.Lock()
	ltClient := a.LT
	mprisService := a.MPRIS
	trayService := a.Tray
	lastfmClient := a.Lastfm
	discordClient := a.Discord
	a.LT = nil
	a.MPRIS = nil
	a.Tray = nil
	a.Lastfm = nil
	a.Discord = nil
	a.mu.Unlock()
	if ltClient != nil {
		ltClient.Leave()
		ltClient.Close()
	}
	if mprisService != nil {
		mprisService.Close()
	}
	if trayService != nil {
		trayService.Close()
	}
	if lastfmClient != nil {
		lastfmClient.Close()
	}
	if discordClient != nil {
		discordClient.Close()
	}
	if a.Queue != nil {
		a.Queue.Close()
	}
	if a.YouTube != nil {
		a.YouTube.Close()
	}
	if a.Spotify != nil {
		if err := a.Spotify.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if a.Player != nil {
		a.Player.Close()
	}
	if a.Store != nil {
		if err := a.Store.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
