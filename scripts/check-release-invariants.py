#!/usr/bin/env python3
from pathlib import Path
import json
import re, re

root = Path(__file__).resolve().parents[1]
def read(rel): return (root / rel).read_text()
def req(ok, msg):
    if not ok:
        raise SystemExit(f'release invariant failed: {msg}')

cargo = read('Cargo.toml')
lock = read('Cargo.lock')
config = read('src-tauri/tauri.conf.json')
main = read('src-tauri/src/main_window.rs')
lib = read('src-tauri/src/lib.rs')
commands = read('src-tauri/src/commands.rs')
state = read('crates/core/src/state.rs')
media = read('crates/core/src/media.rs')
tray = read('src-tauri/src/tray.rs')
mini_rs = read('src-tauri/src/mini.rs')
layout = read('ui/src/routes/+layout.svelte')
home = read('ui/src/routes/+page.svelte')
registry = read('ui/src/lib/home-sections.ts')
editor = read('ui/src/lib/components/HomeLayoutDialog.svelte')
search = read('ui/src/routes/search/+page.svelte')
search_more = read('ui/src/routes/search-more/+page.svelte')
suggest = read('ui/src/lib/components/SearchSuggest.svelte')
pager = read('ui/src/lib/search-pager.ts')
scroll = read('ui/src/lib/ryoku-scroll.ts')
menu = read('ui/src/lib/menu.ts')
trackmenu = read('ui/src/lib/components/TrackMenu.svelte')
playlistmenu = read('ui/src/lib/components/PlaylistMenu.svelte')
mini = read('ui/src/lib/components/MiniPlayer.svelte')
lyrics = read('ui/src/lib/components/LyricsView.svelte')
queue = read('ui/src/lib/components/QueueList.svelte')
shortcuts = read('ui/src/lib/shortcuts.ts')
css = read('ui/src/lib/ryotunes.css')
theme = read('ui/src/lib/theme.svelte.ts')
art_cache = read('ui/src/lib/artwork-cache.ts')
art_img = read('ui/src/lib/components/ArtworkImage.svelte')
now = read('ui/src/lib/components/NowPlaying.svelte')
diag = read('scripts/diagnostics.sh')
rule = read('scripts/ryoku-window-rule.sh')
packaged_rule = read('packaging/ryoku/ryotunes-window-rule.lua')
endpoints = read('crates/innertube/src/endpoints.rs')
browse = read('crates/innertube/src/models/browse.rs')
settings = read('ui/src/lib/components/SettingsDialog.svelte')
zoom = read('ui/src/lib/zoom.ts')
playerbar = read('ui/src/lib/components/PlayerBar.svelte')
layout_css = read('ui/src/routes/layout.css')
ryoku_live = read('ui/src/lib/ryoku-live.ts')
ryoku_theme = read('src-tauri/src/ryoku_theme.rs')
radio = read('crates/core/src/radio.rs')
db = read('crates/core/src/db.rs')
add_to_playlist_ui = read('ui/src/lib/components/AddToPlaylist.svelte')
radio_ui = read('ui/src/routes/radio/+page.svelte')
package_builder = read('scripts/build-replacement-package.sh')
lastfm = read('crates/core/src/lastfm.rs')
local_rs = read('crates/core/src/local.rs')
default_cap = json.loads(read('src-tauri/capabilities/default.json'))
mini_cap = json.loads(read('src-tauri/capabilities/mini.json'))
security_workflow = read('.github/workflows/security.yml')
sync_server = read('crates/sync-server/src/main.rs')
session = read('crates/core/src/session.rs')
local_ui = read('ui/src/lib/components/LocalMusic.svelte')
edit_playlist_ui = read('ui/src/lib/components/EditPlaylistDialog.svelte')
library_page = read('ui/src/routes/library/+page.svelte')
playlist_page = read('ui/src/routes/playlist/[id]/+page.svelte')
local_scope = read('src-tauri/src/local_scope.rs')
login_webview = read('src-tauri/src/login_webview.rs')
core_lib = read('crates/core/src/lib.rs')
discord = read('crates/core/src/discord.rs')

# --- one version, everywhere ----------------------------------------------------
# Cargo.toml's workspace version is the source of truth (scripts/release.sh bumps
# it); every other manifest must agree, so a release never ships a mismatched id.
VERSION = re.search(r'^version = "([0-9]+\.[0-9]+\.[0-9]+)"', cargo, re.M).group(1)
MAJOR_MINOR = '.'.join(VERSION.split('.')[:2])
req(f'name = "ryotunes"\nversion = "{VERSION}"' in lock, f'Cargo.lock Ryotunes version not {VERSION}')
req(f'name = "sync-server"\nversion = "{VERSION}"' in lock, f'Cargo.lock sync-server version not {VERSION}')
parsed = json.loads(config)
req(parsed.get('version') == VERSION and parsed.get('identifier') == 'dev.ryoku.ryotunes', 'Tauri identity incorrect')
req(json.loads(read('ui/package.json')).get('version') == VERSION, 'UI version incorrect')
req(f'pkgver={VERSION}' in read('packaging/arch/PKGBUILD'), 'Arch source pkgver incorrect')
# httpdate is no longer a stale-lock tell: librespot-core (crates/spotify) pulls it legitimately.
req('name = "tauri-plugin-window-state"' not in lock, 'stale v2.0 lock entries returned')

# --- audio-only / background architecture ----------------------------------
active = '\n'.join(read(p) for p in [
    'src-tauri/src/commands.rs','src-tauri/src/lib.rs','crates/core/src/state.rs',
    'ui/src/lib/api.ts','ui/src/lib/player.svelte.ts','ui/src/lib/components/NowPlaying.svelte'])
for forbidden in ['video_stream', 'set_webkit_media_enabled', 'videoproxy', 'hide_videos']:
    req(forbidden not in active, f'audio-only path regressed: {forbidden}')
req('<video ' not in now and '<video>' not in now, 'video element returned to Now Playing')

# --- native/WebKit security boundary ---------------------------------------
tauri_version = re.search(r'name = "tauri"\nversion = "(\d+)\.(\d+)\.(\d+)"', lock)
req(tauri_version is not None and tuple(map(int, tauri_version.groups())) >= (2, 11, 1),
    'Tauri regressed into the GHSA-7gmj-67g7-phm9 affected range')
req('normalize_ui_setting' in commands and 'normalize_proxy_setting' in commands,
    'renderer-writable settings are not validated natively')
req('Authenticated proxy URLs are not supported' in commands
    and 'discarding invalid persisted proxy setting' in lib,
    'proxy credentials can cross the WebKit settings boundary')
req('normalize_lt_server_url' in commands and 'ws://' in commands and 'wss://' in commands,
    'Listen Together server URL validation missing')
req('canonical_music_folder' in commands and 'filesystem root' in commands,
    'local music folder path guard missing')
req('has_local_track' in db and 'if !self.db.has_local_track(path)' in state,
    'renderer-forged local media ids can reach native file playback')
req('std::process::Command::new("cmd")' not in lastfm and 'explorer.exe' in lastfm and 'browser_url' in lastfm,
    'external browser URL can still pass through a command shell')
req('const SCAN_VERSION: &str = "5"' in local_rs and 'sidecar_cover' in local_rs,
    'local cover migration/copy hardening missing')
req('for dir in folders(db)' not in local_scope and 'scope.allow_directory(&covers, true)' in local_scope,
    'watched music folders are recursively exposed through the asset protocol')
req(default_cap.get('windows') == ['main'] and 'dialog:allow-open' not in default_cap.get('permissions', []),
    'main renderer regained direct native file-dialog capability')
req("DialogExt" in commands and "blocking_pick_folder" in commands and "blocking_save_file" in commands,
    'filesystem pickers are not owned by native commands')
req('portable_song' in commands and 'YouTube Music tracks only' in commands,
    'portable playlist transfer can leak local file identifiers')
req('@tauri-apps/plugin-dialog' not in local_ui and '@tauri-apps/plugin-dialog' not in edit_playlist_ui
    and '@tauri-apps/plugin-dialog' not in library_page and '@tauri-apps/plugin-dialog' not in playlist_page,
    'renderer still owns a filesystem picker path')
req('allowed_login_navigation' in session and '.on_navigation(allowed_login_navigation)' in login_webview,
    'Google login WebView navigation allowlist missing')
req(mini_cap.get('windows') == ['mini'] and 'dialog:allow-open' not in mini_cap.get('permissions', []),
    'mini player has unnecessary file-dialog capability')
req('cargo audit' in security_workflow and 'pnpm audit' in security_workflow,
    'scheduled dependency security audit workflow missing')
req('accept_async_with_config' in sync_server and 'MAX_WS_MESSAGE_BYTES' in sync_server,
    'Listen Together relay WebSocket message limits missing')
req('MAX_QUEUE_TRACKS' in sync_server and 'MAX_SUGGESTIONS_PER_ROOM' in sync_server,
    'Listen Together retained room-state limits missing')
req('mpsc::channel::<ServerMessage>(OUTBOUND_QUEUE_CAPACITY)' in sync_server,
    'Listen Together relay outbound channel is unbounded')
req("DEFAULT_PRESENCE_NAME: &str = \"Ryotunes\"" in discord,
    'Discord default presence title is not Ryotunes')

# --- v2.4 Radio / device playlists / Discord title -------------------------
req('pub mod radio;' in core_lib and 'commands::radio_stations' in lib and 'commands::play_radio_station' in lib,
    'Internet Radio command wiring missing')
req('RADIO_ID_PREFIX' in radio and 'hidebroken=true' in radio and 'count_click' in radio,
    'bounded Radio Browser integration missing')
req('station_by_uuid' in radio and 'STATION_CACHE_MAX' in radio and 'station_uuid: String' in commands,
    'radio playback trusts renderer-supplied stream metadata')
req('.and_then(crate::radio::normalize_station)' in state,
    'restored radio cache bypasses current native URL validation')
req('192.168.1.5' in radio and 'localhost' in radio,
    'radio native stream URL local-network rejection missing')
req('MAX_RADIO_RESPONSE_BYTES' in radio and 'official_mirror_host' in radio,
    'Radio Browser response/mirror trust bounds missing')
req('setInterval(' not in radio_ui and 'radioStations' in radio_ui and 'playRadioStation' in radio_ui,
    'Radio UI is not demand-driven')
req('CREATE TABLE IF NOT EXISTS local_playlists' in db and 'CREATE TABLE IF NOT EXISTS local_playlist_tracks' in db,
    'persistent device playlist storage missing')
req('LOCAL_PLAYLIST_PREFIX' in state and 'add_to_local_playlist' in commands,
    'device playlist native command path missing')
req('Create + add' in add_to_playlist_ui and 'addToLocalPlaylist' in add_to_playlist_ui,
    'Add-to-playlist create-and-add flow missing')
req('discord_presence_name' in commands and 'DEFAULT_PRESENCE_NAME' in discord,
    'custom Discord presence title backend missing')
req('Discord presence title' in settings and "api.setSetting('discord_presence_name'" in settings,
    'custom Discord presence title UI missing')
req('unsupportedHomeSection' in home and r'music\s*videos?' in registry and 'video\\s+for\\s+you' in registry, 'central Home video rejection missing')
req('destroy()' in main and 'hibernate_main' in main and 'trim_after_hibernate' in main, 'Linux WebKit hibernation missing')
req('const IDLE_EXIT_GRACE: Duration = Duration::from_secs(5 * 60);' in main, 'five-minute tray grace changed')
req('background && !has_ui && !playing' in main and 'schedule_idle_exit' in main and 'cancel_idle_exit()' in main, 'tray idle policy regressed')
req('pub fn request_quit(app: &AppHandle)' in main and 'shutdown_for_quit().await' in main, 'explicit Quit path missing')
req('MediaUpdate::Shutdown' in media and 'MediaPlayback::Stopped' in media, 'MPRIS teardown missing')
req('pub async fn shutdown_for_quit(&self)' in state and 'self.player.stop()' in state, 'native playback Quit teardown missing')
req('"quit" => crate::main_window::request_quit(app)' in tray, 'tray Quit bypasses graceful shutdown')

# --- cold start / mini restore / Ryoku-native floating ----------------------
windows = parsed['app']['windows']
w = next((x for x in windows if x.get('label','main') == 'main'), windows[0])
req(parsed['app'].get('enableGTKAppId') is True, 'GTK/Wayland app id is not enabled')
req(w.get('visible') is False and w.get('fullscreen') is False and w.get('center') is True, 'main surface startup flags incorrect')
req("function acknowledgeFrontend(label: 'main' | 'mini', beforeReveal?: Promise<void>)" in layout and 'const delays = [0, 45, 120, 260, 520, 900]' in layout, 'cold-start first-paint retry handshake missing')
req('void attempt(0);' in layout and 'requestAnimationFrame(() => void attempt(0))' not in layout, 'hidden WebView readiness is incorrectly gated by requestAnimationFrame')
req("const teardownReady = acknowledgeFrontend('main', ryokuTokens.ready)" in layout and "const teardownReady = acknowledgeFrontend('mini', ryokuTokens.ready)" in layout, 'main/mini theme-prime/reveal handshake not shared')
req('pub fn frontend_ready(app: &AppHandle)' in main and 'w.show().map_err' in main and 'crate::mini::close(app);' in main, 'main reveal does not close mini only after successful show')
req('pub fn arm_reveal_failsafe(app: &AppHandle, delay: Duration)' in main and 'frontend readiness deadline expired' in main, 'native hidden-window reveal failsafe missing')
req('arm_reveal_failsafe(app.handle(), Duration::from_millis(4000))' in lib and 'arm_reveal_failsafe(app, Duration::from_millis(220))' in main, 'cold-start/second-launch reveal recovery missing')
req('crate::tray::show_main(&app);' in commands and 'pub async fn close_mini' in commands, 'mini restore button does not use shared main restore path')
req('RYOTUNES_APP_ID: &str = "dev.ryoku.ryotunes"' in main and 'RYOTUNES_MAIN_TITLE: &str = "Ryotunes"' in main, 'stable main window identity missing')
req('args(["keyword", "windowrulev2"' not in main and "arg(\"windowrulev2\")" not in main, 'runtime Hyprland windowrule injection returned')
req('hl.window_rule({' in rule and 'title = "^(Ryotunes)$"' in rule and 'float  = true' in rule and 'center = true' in rule, 'managed Ryoku floating rule missing')
req('hl.window_rule({' in packaged_rule and 'title = "^(Ryotunes)$"' in packaged_rule and 'float  = true' in packaged_rule and 'size   = { 1760, 1000 }' in packaged_rule and 'center = true' in packaged_rule, 'packaged Ryoku rule missing')
req('Ryotunes Mini' in packaged_rule, 'floating rule does not document mini exclusion')
req('0.92' in main and '0.84' in main and 'monitor.work_area()' in main and 'unmaximize()' in main, 'adaptive 92% x 84% work-area geometry missing')
req('setfloating' in main and 'clients", "-j"' in main, 'Hyprland defensive fallback missing')
req('tauri_plugin_window_state' not in main and 'tauri_plugin_window_state' not in lib, 'persisted geometry can override cold-start default')

# --- Ryoku live theme parity -------------------------------------------------
req('mod ryoku_theme;' in lib and 'ryoku_theme::spawn_watcher(sink.clone())' in lib, 'native Ryoku theme watcher is not wired')
for name in ['theme.json', 'shell.json', 'colors.json']:
    req(name in ryoku_theme, f'Ryoku theme watcher missing {name}')
req('inotify_init1' in ryoku_theme and 'IN_CLOSE_WRITE' in ryoku_theme and 'IN_MOVED_TO' in ryoku_theme, 'Ryoku theme watcher is not event-driven')
req('named.and_then' in ryoku_theme and 'if follow' in ryoku_theme and '"surfaceContainerLow"' in ryoku_theme, 'Ryoku Material role precedence drifted')
req('setInterval(' not in ryoku_live, 'Ryoku theme polling clock returned')
req('onRyokuThemeChanged' in ryoku_live and "'ryoku-theme-changed'" in read('ui/src/lib/api.ts'), 'live Ryoku theme event bridge missing')
for token in ['--ryo-paper', '--ryo-paper-lift', '--ryo-panel', '--ryo-card', '--ryo-sidebar-surface', '--ryo-player-surface', '--ryo-ink', '--ryo-bone']:
    req(token in ryoku_live, f'Ryoku palette does not drive {token}')
req('t.secondaryContainer' in ryoku_live and 't.primaryContainer' in ryoku_live and 't.tertiaryContainer' in ryoku_live, 'v2.3 accent families are not Material-role driven')
req('v2.3 Ryoku live-palette compatibility.' in css and 'html[data-ryo-theme] .ryo-command-dialog' in css and 'html[data-ryo-theme] .ryo-search-suggest' in css, 'legacy fixed-color interaction surfaces are not theme-semantic')
req('setRyokuSystemTheme(t.light)' in ryoku_live, 'Follow System does not follow Ryoku surface luminance')
req("const ryokuTokens = initRyokuLiveTokens();" in layout and layout.count('initRyokuLiveTokens()') >= 2, 'main and mini do not both initialise Ryoku live tokens')

# --- Home stability / Edit Home / headings ---------------------------------
req('buildHomeRegistry' in home and 'HOME_LOCAL_SECTIONS' in home and 'homeSectionTitle' in home, 'Home registry not authoritative')
req('Reset default' in editor and 'Cancel' in editor and 'saveHomeLayout' in editor and 'ryo-home-insert-rule' in editor, 'Edit Home transactional/reorder/reset behavior missing')
req("return 'You might also like'" in registry and 'section.titleIsArtist' in registry, 'artist recommendation heading normalization missing')
req('title_is_artist' in browse and 'MUSIC_PAGE_TYPE_ARTIST' in browse, 'backend artist-heading marker missing')
req('Session cache is deliberately authoritative for revisits' in home and 'if (hit)' in home and 'return;' in home, 'Home revisit revalidation returned')
req('sections: [...home!.sections, ...more.sections]' in home, 'Home continuation is not append-only')
req('content-visibility:auto' in css, 'off-screen paint containment missing')
for forbidden in ['unmountSection', 'mountedSections.delete', 'visibleSections.delete']:
    req(forbidden not in home, f'physical Home virtualization returned: {forbidden}')

# --- Search continuation and nested scroll ownership ------------------------
for name in ['search_page_more','search_all_more','search_cards_page','search_cards_more']:
    req(name in commands, f'native Search continuation command missing: {name}')
for name in ['search_songs_continuation','search_all_continuation','search_cards_continuation']:
    req(name in endpoints, f'InnerTube Search continuation path missing: {name}')
req("const STREAMS: SearchStream[] = ['mixed', 'songs', 'albums', 'artists', 'playlists']" in pager, 'bounded multi-stream Search pager missing')
req('at most one request is made per call' in pager and 'nextSearchPage' in pager, 'Search pager is not explicitly bounded to one request')
req('const QUICK_BATCH = 12' in search and '.slice(0, quickLimit)' in search and '.slice(0, 8)' not in search, 'full Search still has a fixed legacy cap')
req('nextSearchPage(q, quickPager)' in search and 'Loading more…' in search and 'No more results' in search, 'full Search incremental states missing')
req('ownNestedVerticalScroll' in search and '{@attach ownNestedVerticalScroll}' in search, 'full Search nested scroll attachment missing')
req('.ryo-search-quick-list { min-height:0;' in css and 'overflow-y:auto; overscroll-behavior:contain;' in css, 'Search result pane is not a contained vertical scroller')
req('e.preventDefault();' in scroll and 'e.stopPropagation();' in scroll and 'export function ownNestedVerticalScroll' in scroll, 'nested scroll chain suppression missing')
req('nextSearchPage(q, pager)' in suggest and 'onscroll={panelScroll}' in suggest and '{@attach ownNestedVerticalScroll}' in suggest, 'Home/global quick Search is still fixed-page')
req('All results for “{value.trim()}”' in suggest, 'quick Search continuation-to-full-search entry missing')
req('api.searchPageMore(token)' in search_more and 'api.searchCardsMore(token)' in search_more and 'IntersectionObserver' in search_more, 'category Search More pagination missing')
req('lastQuickScroll' in search and 'lastActiveResultId' in search and 'lastQuickPager' in search, 'full Search navigation state preservation missing')
req('const seen = new Set' in search and 'const seen = new Set' in suggest, 'Search de-duplication missing')

# --- shared menus / discoverability ----------------------------------------
req('ctxHost(openMenu)' in trackmenu and 'ctxHost(openMenu)' in playlistmenu, 'shared context-menu host missing')
req("key.key === 'ContextMenu'" in menu and "key.shiftKey && key.key === 'F10'" in menu, 'keyboard context-menu support missing')
req("e.type === 'contextmenu'" in menu and 'clientX' in menu and 'clientY' in menu, 'pointer context anchoring missing')
req('.ryo-action-menu-trigger { opacity:.62 !important; visibility:visible !important; }' in css, 'always-visible action-menu baseline missing')
for p in (root/'ui/src').rglob('*.svelte'):
    for line in p.read_text().splitlines():
        if 'triggerClass' in line:
            req('opacity-0' not in line, f'hover-hidden action menu remains in {p.relative_to(root)}')

# --- mini-player premium/self-contained behavior ---------------------------
req("type MiniView = 'now' | 'lyrics' | 'queue'" in mini, 'mini three-view model missing')
req('<LyricsView compact />' in mini and '<QueueList compact showMenus={false} />' in mini, 'mini Lyrics/Queue or menu suppression missing')
req('TrackMenu' not in mini and 'PlaylistMenu' not in mini, 'action menu exists directly in mini-player')
req('ryo-mini-v2-tabs' in mini and 'aria-label="Now playing"' in mini and 'aria-label="Lyrics"' in mini and 'aria-label="Queue"' in mini, 'mini icon-led view switcher missing')
req('const W: f64 = 724.0;' in mini_rs and 'const H: f64 = 356.0;' in mini_rs and '.visible(false)' in mini_rs, 'mini geometry/hidden startup incorrect')
req('compact ? 100 : appearance.lowResourceMode ? 125 : 67' in lyrics and 'hasWordTiming' in lyrics, 'bounded compact lyrics interpolation missing')
req('ryo-queue-compact' in queue, 'compact queue path missing')
# TrackRow's mini/queue menu suppression flags are typed component props; without these,
# Svelte semantic checking rejects every QueueList caller even though syntax-only checks pass.
trackrow = read('ui/src/lib/components/TrackRow.svelte')
req('showMenu?: boolean;' in trackrow and 'contextMenu?: boolean;' in trackrow, 'TrackRow menu suppression props missing from typed component contract')

# --- artwork + theme + smoothness ------------------------------------------
req('const MAX_READY_ARTWORK = 36' in art_cache and 'new Map<string, true>()' in art_cache, 'bounded artwork readiness cache missing')
req("typeof image.decode === 'function'" in art_img and 'needsSwap' in art_img and 'fullFailed' in art_img and 'class:sharp={!needsSwap || fullFailed}' in art_img, 'sharp artwork fallback/decode-before-swap guard missing')
req('size={960}' in now and 'previewSize={160}' in now, 'Now Playing artwork resolution regressed')
req('ArtworkImage' in now and 'ArtworkImage' in mini, 'large artwork surfaces do not share artwork pipeline')
for token in ['--ryo-paper:#d2cabd','--ryo-paper-lift:#e2d8c9','--ryo-panel:#c4baab','--ryo-card:#dbd1c3','--ryo-sidebar-surface:#bec6b8','--ryo-player-surface:#c1c7cc','--ryo-ink:#28231e','--ryo-light-sage:#9faa94','--ryo-light-blue:#9aaabc','--ryo-light-clay:#c58f73']:
    req(token in css, f'v2.3 Light token missing: {token}')
req('professional Light compatibility for legacy dark-only overlay chrome' in css, 'Light-theme legacy surface compatibility pass missing')
req("'system' | 'light' | 'dark'" in theme and 'prefers-color-scheme: dark' in theme, 'Follow System/Light/Dark engine missing')
req('@media (prefers-reduced-motion: reduce)' in css and 'data-low-resource="true"' in css, 'Reduced Motion/Low Resource global guards missing')
req('content-visibility:auto' in css, 'deep Home shelves must stay content-visibility:auto')
req('ryo-is-scrolling' not in css and 'ryo-is-scrolling' not in home and 'ryo-is-scrolling' not in scroll, 'a scroll-time class toggle restyles the whole page on every wheel notch; do not reintroduce it')
# Prevent accidental permanent frontend clocks in the shell/search/home/mini paths.
for rel in ['ui/src/routes/+layout.svelte','ui/src/routes/+page.svelte','ui/src/routes/search/+page.svelte','ui/src/lib/components/MiniPlayer.svelte','ui/src/lib/player.svelte.ts']:
    req('setInterval(' not in read(rel), f'permanent interval introduced in hot frontend path: {rel}')

# --- keybind architecture ---------------------------------------------------
req('export const KEYBINDINGS: Keybind[]' in shortcuts and 'KEYBIND_GROUPS' in shortcuts, 'single executable keybind registry missing')
req("initShortcuts(scope: 'main' | 'mini' = 'main')" in shortcuts and "scope === 'mini'" in shortcuts, 'mini/full shortcut scopes missing')
for key in ['search.global','search.link','search.page','playback.toggle','playback.previous','playback.next','playback.seekBack','playback.seekForward','playback.mute','playback.shuffle','playback.repeat','playback.queue','playback.lyrics','playback.now','nav.back','nav.forward','nav.escape','interface.settings','interface.shortcuts']:
    req(key in shortcuts, f'keybind registry lost {key}')
req('MediaControlEvent::Play' in lib and 'MediaControlEvent::Pause' in lib and 'MediaControlEvent::Next' in lib and 'MediaControlEvent::Previous' in lib, 'system media-key/MPRIS command coverage missing')

# --- v2.3 final-user field fixes -------------------------------------------
req('const DEFAULT = 1.1;' in zoom, 'default UI scale is not 110%')
req("settings.ui_scale ?? '110'" in settings and 'Ctrl+0 restores 110%.' in settings, 'Settings 110% default/reset copy missing')
req('PhysicalSize::new(1760u32, 1000u32)' in main, 'large pre-map main-window fallback missing')
req(w.get('width') == 1760 and w.get('height') == 1000, 'Tauri pre-map main-window size is not 1760x1000')
req('.ryo-wave-seek:focus-within::after' not in css, 'rectangular seek focus pseudo-element returned')
req('.ryo-wave-seek::after { content:none !important; }' in css, 'seek focus rectangle suppression missing')
req('ryo-playerbar-center' in playerbar and '.ryo-playerbar-center { gap:7px !important;' in css, 'bottom transport breathing room missing')
req('.ryo-music-deck-actions {' in css and 'min-height:44px !important' in css and 'padding:7px 0 7px !important' in css, 'Home deck transport breathing room missing')
req('.ryo-settings-register::after' in css and 'right:52px' in css and 'border-bottom:0 !important' in css, 'Settings close-button divider gap missing')
req("if (compact) return;" in lyrics and 'const pauseMs = compact ? 1400 : 5000' in lyrics, 'compact lyrics auto-follow ownership missing')
req('scroller.getBoundingClientRect()' in lyrics and 'line.getBoundingClientRect()' in lyrics, 'compact lyrics robust active-line centering missing')
req('ryo-lyrics-scroller-compact' in lyrics and 'padding-top:54px !important' in css and 'padding-bottom:54px !important' in css, 'compact lyrics reading-zone padding missing')
req('grid-template-columns:232px minmax(0,1fr)' in mini and 'gap:7px' in mini and 'width:38px' in mini, 'mini-player final spacing/redesign missing')
req('--background: oklch(0.80 0.008 80)' in layout_css and '--card: oklch(0.83 0.009 80)' in layout_css, 'pre-theme light surfaces can still flash pure white')

# --- Tauri application-command ACL ------------------------------------------
build_rs = read('src-tauri/build.rs')
ui_perm = read('src-tauri/permissions/ui.toml')
default_cap = json.loads(read('src-tauri/capabilities/default.json'))
mini_cap = json.loads(read('src-tauri/capabilities/mini.json'))
handler_block = lib.split('.invoke_handler(tauri::generate_handler![', 1)[1].split('])', 1)[0]
handler_commands = set(re.findall(r'commands::([A-Za-z0-9_]+)', handler_block))
manifest_block = build_rs.split('const APP_COMMANDS:', 1)[1].split('];', 1)[0]
manifest_commands = set(re.findall(r'"([A-Za-z0-9_]+)"', manifest_block))
permission_block = ui_perm.split('commands.allow = [', 1)[1].split(']', 1)[0]
permission_commands = set(re.findall(r'"([A-Za-z0-9_]+)"', permission_block))
req(handler_commands == manifest_commands == permission_commands,
    f'Tauri app-command ACL drift: handler={len(handler_commands)} manifest={len(manifest_commands)} permission={len(permission_commands)}')
req(default_cap.get('windows') == ['main'] and mini_cap.get('windows') == ['mini'],
    'main/mini capability windows are broader than their intended surfaces')
req('allow-ui-commands' in default_cap.get('permissions', []) and 'allow-ui-commands' in mini_cap.get('permissions', []),
    'bundled UI surfaces lost their application-command permission')
req('*' not in default_cap.get('windows', []) and '*' not in mini_cap.get('windows', []),
    'wildcard WebView capability would expose application commands to helper/login surfaces')
for cap_name, cap in [('main', default_cap), ('mini', mini_cap)]:
    perms = set(cap.get('permissions', []))
    req('core:default' not in perms, f'{cap_name} capability re-enabled broad core:default')
    req('core:app:allow-version' in perms and 'core:event:default' in perms,
        f'{cap_name} capability lost the minimal app/event APIs used by the bundled UI')
    for forbidden in ['core:image:default', 'core:path:default', 'core:tray:default',
                      'core:menu:default', 'core:resources:default']:
        req(forbidden not in perms, f'{cap_name} capability exposes unnecessary {forbidden}')
req('AppManifest::new().commands(APP_COMMANDS)' in build_rs,
    'application commands are not registered with Tauri runtime authority')

# --- diagnostics / package integration -------------------------------------
req("local expected='/usr/lib/ryotunes-v2.4/ryotunes'" in diag, 'diagnostics expected binary path incorrect')
req('for p in /proc/[0-9]*' in diag and 'readlink -f "$p/exe"' in diag, 'diagnostics do not identify process through /proc/<pid>/exe')
req('/home/' not in diag and '/Users/' not in diag, 'diagnostics contain private user paths')
req('state_home="${XDG_DATA_HOME:-$HOME/.local/share}/ryotunes-v2.4"' in rule, 'Ryoku rule state path is not versioned for v2.4')
req('adopt_previous_backup' in package_builder and '/var/lib/ryotunes-v2.3/stock' in package_builder,
    'v2.4 replacement package does not inherit the previous stock rollback backup')
req("conflicts=('ryotunes-v2.3'" in package_builder,
    'v2.4 package does not declare the previous custom package generation as a conflict')

# Frontend settings whitelist stays synchronized with the Rust command boundary. UI_SETTINGS lives
# in the core (shared by the Tauri host and the daemon).
ui_block = state.split('pub const UI_SETTINGS:',1)[1].split('];',1)[0]
allowed = set(re.findall(r'"([^"]+)"', ui_block))
written = set()
for p in (root/'ui/src').rglob('*'):
    if p.suffix in {'.svelte','.ts'}:
        written.update(re.findall(r"api\.setSetting\('([^']+)'", p.read_text()))
req(written <= allowed, f'frontend writes unwhitelisted settings: {sorted(written-allowed)}')

print(f'Release invariants v{VERSION}: OK')
