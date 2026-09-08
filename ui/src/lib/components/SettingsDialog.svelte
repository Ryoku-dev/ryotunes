<script lang="ts">
	import { untrack } from 'svelte';
	import { HugeiconsIcon } from '@hugeicons/svelte';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Switch } from '$lib/components/ui/switch';
	import * as Dialog from '$lib/components/ui/dialog';
	import * as api from '$lib/api';
	import { ui, toast, playback, auth } from '$lib/player.svelte';
	import { appearance, setAppearance, resolvedTheme, type ThemeMode } from '$lib/theme.svelte';
	import { getVersion } from '@tauri-apps/api/app';
	import { thumb } from '$lib/thumb';
	import RyokuSpecimen from '$lib/components/RyokuSpecimen.svelte';
	import { setInterfaceScale } from '$lib/zoom';
	import { KEYBIND_GROUPS } from '$lib/shortcuts';
	import { normalizeSearchText } from '$lib/localsearch';

	type TabId = 'general' | 'playback' | 'data' | 'keybinds' | 'about';
	const TABS: { id: TabId; label: string }[] = [
		{ id: 'general', label: 'General' },
		{ id: 'playback', label: 'Playback' },
		{ id: 'data', label: 'Data & storage' },
		{ id: 'keybinds', label: 'Keybinds' },
		{ id: 'about', label: 'About' }
	];

	const TAB_META: Record<TabId, { jp: string; group: string; blurb: string; code: string }> = {
		general: { jp: '全般', group: 'APPLICATION', blurb: 'Session behaviour, desktop integration and the things that should stay out of your way.', code: 'APP-01' },
		playback: { jp: '再生', group: 'PLAYBACK', blurb: 'How the listening engine resolves, queues and carries a session forward.', code: 'PLAY-02' },
		data: { jp: '保存', group: 'DATA', blurb: 'Network routing and local storage used to keep the instrument responsive.', code: 'DATA-03' },
		keybinds: { jp: '鍵', group: 'KEYBINDS', blurb: 'Every Ryotunes shortcut, grouped by intent and read from the same registry the app executes.', code: 'KEY-04' },
		about: { jp: '力', group: 'ABOUT', blurb: 'Build identity and the small set of components that make Ryotunes a Ryoku music instrument.', code: 'INFO-05' }
	};

	let tab = $state<TabId>('general');
	const metaFor = (id: TabId) => TAB_META[id];
	const activeMeta = $derived(metaFor(tab));
	const activeLabel = $derived(TABS.find((t) => t.id === tab)?.label ?? 'General');
	let buildVersion = $state('');
	getVersion().then((v) => (buildVersion = v)).catch(() => {});
	// Software updates (About). Never polled: a check runs only when the button is pressed, and the
	// long install RPC has no client-side timeout, so the promise simply resolves when it is done.
	let update = $state<api.UpdateInfo | null>(null);
	let updateStatus = $state<'idle' | 'checking' | 'installing' | 'installed' | 'error'>('idle');
	let updateError = $state('');
	let installedVersion = $state('');
	let showReleaseNotes = $state(false);
	const appVersion = $derived(update?.currentVersion || buildVersion);
	const updateBusy = $derived(updateStatus === 'checking' || updateStatus === 'installing');
	const updateAvailable = $derived(!!update?.available && !!update?.latestVersion);
	const canInstallUpdate = $derived(updateAvailable && !!update?.canInstall && updateStatus !== 'installed');
	// Always-available changelog target: the release the check found, else the project's releases page.
	const RELEASES_URL = 'https://github.com/ryoku-dev/ryotunes/releases';
	let settings = $state<Record<string, string>>({});
	let clients = $state<string[]>([]);
	let proxyInput = $state('');
	let discordNameInput = $state('Ryotunes');
	let savingDiscordName = $state(false);
	let loaded = $state(false);
	let clearing = $state(false);
	let discordState = $state<api.DiscordStatus>({ enabled: false, status: 'disabled' });
	const settingsArt = $derived.by(() => {
		if (tab === 'general' && auth.account?.thumbnail) return thumb(auth.account.thumbnail, 384);
		if (tab === 'playback' && playback.now?.thumbnail) return thumb(playback.now.thumbnail, 384);
		return '';
	});
	const settingsCaption = $derived.by(() => {
		if (tab === 'general') {
			return auth.account?.signedIn
				? `Session ready${auth.account.name ? ` — ${auth.account.name}` : ''}. Desktop services stay local.`
				: 'Local desktop session. Sign in only when your YouTube library needs it.';
		}
		if (tab === 'playback') {
			return playback.now
				? `${playback.now.title} — ${playback.now.artists || 'Now playing'}`
				: 'Playback engine ready. No stream is active.';
		}
		if (tab === 'data') return 'Tape, cache and transport — the local path that keeps playback immediate.';
		if (tab === 'keybinds') return 'Keyboard flow is part of the instrument: searchable, grouped and always in sync with the live bindings.';
		return `Ryotunes${appVersion ? ` ${appVersion}` : ''} — a Ryoku-native music instrument built around Rust, mpv and WebKitGTK.`;
	});
	const settingsReadout = $derived.by(() => {
		if (tab === 'general') {
			return [
				`SESSION|${auth.account?.signedIn ? 'SIGNED IN' : 'LOCAL'}`,
				`HISTORY|${settings.enable_history !== 'false' ? 'ON' : 'OFF'}`,
				`AUTOSTART|${settings.autostart === 'true' ? 'ON' : 'OFF'}`,
				`TRAY|${settings.close_to_tray !== 'false' ? 'ON' : 'OFF'}`
			];
		}
		if (tab === 'playback') {
			return [
				`ENGINE|MPV`,
				`QUALITY|${settings.quality ?? 'HIGH'}`,
				`SPEED|${playback.speed.toFixed(2)}×`,
				`QUEUE|${Math.max(0, playback.queue.items.length - playback.queue.currentIndex - 1)}`
			];
		}
		if (tab === 'data') {
			return [
				`PROXY|${settings.proxy?.trim() ? 'CUSTOM' : 'DIRECT'}`,
				`CLIENTS|${clients.length || 'AUTO'}`,
				`CACHE|LOCAL`,
				`TRANSPORT|RUST`
			];
		}
		if (tab === 'keybinds') return [`BINDINGS|${KEYBIND_GROUPS.reduce((n, g) => n + g.rows.length, 0)}`, `GROUPS|${KEYBIND_GROUPS.length}`, `ESCAPE|PEEL`, `SEARCH|GLOBAL`];
		return [
			`VERSION|${appVersion || '—'}`,
			`SHELL|RYOKU`,
			`ENGINE|RUST + MPV`,
			`UI|WEBKITGTK`
		];
	});
	// Reload persisted settings whenever the instrument opens.
	$effect(() => {
		if (!ui.settingsOpen) return;
		untrack(() => { load(); });
	});
	$effect(() => {
		if (!ui.settingsOpen || tab !== 'general') return;
		let stopped = false;
		const refresh = async () => {
			try {
				const value = await api.discordStatus();
				if (!stopped) discordState = value;
			} catch {}
		};
		void refresh();
		const timer = window.setInterval(refresh, 3000);
		return () => { stopped = true; window.clearInterval(timer); };
	});
	async function load() {
		try {
			const [s, c] = await Promise.all([api.getSettings(), api.getStreamClients()]);
			settings = s;
			clients = c;
			proxyInput = s.proxy ?? '';
			discordNameInput = s.discord_presence_name?.trim() || 'Ryotunes';
			if (s.low_resource_mode === 'true' && !appearance.lowResourceMode) setAppearance({ lowResourceMode: true });
			if (s.low_resource_mode !== 'true' && appearance.lowResourceMode) {
				settings.low_resource_mode = 'true';
				void api.setSetting('low_resource_mode', 'true');
			}
		} catch (e) {
			toast.error(String(e));
		}
		loaded = true;
	}

	const quality = $derived(settings.quality ?? 'HIGH');
	const historyOn = $derived(settings.enable_history !== 'false');
	const autoplayOn = $derived(settings.autoplay !== 'false');
	const boiduOn = $derived(settings.lyrics_boidu !== 'false');
	const preventDuplicatesOn = $derived(settings.prevent_duplicates === 'true');
	const discordOn = $derived(settings.discord_rpc === 'true');
	const trayOn = $derived(settings.close_to_tray !== 'false');
	const autostartOn = $derived(settings.autostart === 'true');
	const uiScale = $derived(Number(settings.ui_scale ?? '110'));
	const UI_SCALES = [80, 90, 100, 110, 120, 130, 140];
	let keybindFilter = $state('');
	const visibleKeybindGroups = $derived.by(() => {
		const q = normalizeSearchText(keybindFilter);
		if (!q) return KEYBIND_GROUPS;
		return KEYBIND_GROUPS.map((group) => ({
			...group,
			rows: group.rows.filter((row) => normalizeSearchText(`${row.label} ${row.display} ${group.title}`).includes(q))
		})).filter((group) => group.rows.length);
	});
	const disabled = $derived(
		new Set(
			(settings.disabled_stream_clients ?? '')
				.split(',')
				.map((s) => s.trim())
				.filter(Boolean)
		)
	);

	const QUALITIES = [
		{ id: 'LOW', label: 'Low' },
		{ id: 'AUTO', label: 'Auto' },
		{ id: 'HIGH', label: 'High' }
	];

	async function setQuality(q: string) {
		settings.quality = q;
		await api.setSetting('quality', q);
		// Cached URLs are keyed by video only, so clear them to apply the new quality everywhere.
		await api.clearCaches();
		toast.success('Audio quality updated');
	}

	async function setHistory(on: boolean) {
		settings.enable_history = on ? 'true' : 'false';
		await api.setSetting('enable_history', settings.enable_history);
	}

	async function setAutoplay(on: boolean) {
		settings.autoplay = on ? 'true' : 'false';
		await api.setSetting('autoplay', settings.autoplay);
	}


	async function setBoidu(on: boolean) {
		settings.lyrics_boidu = on ? 'true' : 'false';
		await api.setSetting('lyrics_boidu', settings.lyrics_boidu);
	}

	async function setPreventDuplicates(on: boolean) {
		settings.prevent_duplicates = on ? 'true' : 'false';
		await api.setSetting('prevent_duplicates', settings.prevent_duplicates);
	}
	async function setDiscord(on: boolean) {
		settings.discord_rpc = on ? 'true' : 'false';
		await api.setSetting('discord_rpc', settings.discord_rpc);
		discordState = await api.discordStatus().catch(() => ({ enabled: on, status: on ? 'connecting' : 'disabled' } as api.DiscordStatus));
	}

	async function saveDiscordName() {
		if (savingDiscordName) return;
		const value = discordNameInput.trim() || 'Ryotunes';
		const length = [...value].length;
		if (length < 2 || length > 128) {
			toast.error('Discord presence title must be between 2 and 128 characters');
			return;
		}
		savingDiscordName = true;
		try {
			await api.setSetting('discord_presence_name', value);
			settings.discord_presence_name = value;
			discordNameInput = value;
			toast.success(`Discord now shows “Listening to ${value}”`);
		} catch (e) {
			toast.error(String(e));
		} finally {
			savingDiscordName = false;
		}
	}

	async function resetDiscordName() {
		discordNameInput = 'Ryotunes';
		await saveDiscordName();
	}

	async function setLowResource(on: boolean) {
		setAppearance({ lowResourceMode: on });
		settings.low_resource_mode = on ? 'true' : 'false';
		await api.setSetting('low_resource_mode', settings.low_resource_mode);
	}

	function setTheme(mode: ThemeMode) {
		setAppearance({ themeMode: mode });
	}

	async function setTray(on: boolean) {
		settings.close_to_tray = on ? 'true' : 'false';
		await api.setSetting('close_to_tray', settings.close_to_tray);
	}

	async function setAutostart(on: boolean) {
		settings.autostart = on ? 'true' : 'false';
		try {
			await api.setSetting('autostart', settings.autostart);
		} catch (e) {
			settings.autostart = on ? 'false' : 'true'; // registration failed — revert the switch
			toast.error(String(e));
		}
	}

	async function setUiScale(percent: number) {
		settings.ui_scale = String(percent);
		try {
			await setInterfaceScale(percent);
		} catch (e) {
			toast.error(String(e));
		}
	}

	async function toggleClient(name: string) {
		const set = new Set(disabled);
		if (set.has(name)) set.delete(name);
		else set.add(name);
		settings.disabled_stream_clients = [...set].join(',');
		await api.setSetting('disabled_stream_clients', settings.disabled_stream_clients);
	}

	async function saveProxy() {
		const value = proxyInput.trim();
		try {
			await api.setSetting('proxy', value);
			settings.proxy = value;
			proxyInput = value;
			toast.success('Proxy saved — restart to apply');
		} catch (e) {
			toast.error(String(e));
		}
	}

	async function doClearCaches() {
		clearing = true;
		try {
			await api.clearCaches();
			toast.success('Caches cleared');
		} finally {
			clearing = false;
		}
	}

	async function checkForUpdates() {
		if (updateStatus === 'checking' || updateStatus === 'installing') return;
		updateStatus = 'checking';
		updateError = '';
		showReleaseNotes = false;
		try {
			update = await api.checkForUpdates();
			updateStatus = 'idle';
		} catch (e) {
			updateError = String(e);
			updateStatus = 'error';
		}
	}

	async function installUpdate() {
		if (updateBusy) return;
		const version = update?.latestVersion;
		if (!version || !update?.available || !update?.canInstall) return;
		updateStatus = 'installing';
		updateError = '';
		try {
			const result = await api.installUpdate(version);
			installedVersion = result.version;
			updateStatus = 'installed';
		} catch (e) {
			updateError = String(e);
			updateStatus = 'error';
		}
	}
</script>

<Dialog.Root bind:open={ui.settingsOpen}>
	<Dialog.Content class="ryo-settings-hub gap-0 overflow-hidden p-0">
		<div class="ryo-settings-register">
			<div class="ryo-settings-register-left">
				<span class="ryo-settings-register-rule"></span>
				<span class="ryo-settings-register-mark">力</span>
				<Dialog.Title>RYOTUNES</Dialog.Title>
				<span>// SETTINGS_</span>
			</div>
			<Dialog.Description class="sr-only">Ryotunes application settings</Dialog.Description>
			<div class="ryo-settings-register-right">
				<span>{activeMeta.code}</span><i>///</i>
			</div>
		</div>

		<div class="ryo-settings-shell">
			<aside class="ryo-settings-rail">
				<div class="ryo-settings-masthead">
					<div class="ryo-settings-mast-row">
						<span class="ryo-settings-mast-mark">力</span>
						<div>
							<strong>RYOTUNES</strong>
							<small>// MUSIC CONTROL</small>
						</div>
					</div>
					<span class="ryo-settings-mast-slash">///</span>
				</div>

				<div class="ryo-settings-nav-group"><span>01</span><strong>APP</strong><i></i><b>設定</b></div>
				<nav class="ryo-settings-hub-nav">
					{#each TABS as t, i (t.id)}
						{@const meta = metaFor(t.id)}
						<button type="button" onclick={() => (tab = t.id)} class:active={tab === t.id}>
							<span class="ryo-settings-nav-index">{String(i + 1).padStart(2, '0')}</span>
							<span class="ryo-settings-nav-lead">{tab === t.id ? '//' : ''}</span>
							<span class="ryo-settings-nav-label">{t.label}</span>
							<span class="ryo-settings-nav-kana">{meta.jp}</span>
						</button>
					{/each}
				</nav>

				<div class="ryo-settings-rail-foot">
					<div class="ryo-settings-edition"><span>RYOKU</span><b>// MUSIC</b><em>LIVE</em></div>
					<div class="ryo-settings-mini-barcode"></div>
					<small>RYOTUNES · CONTROL</small>
				</div>
			</aside>

			<section class="ryo-settings-page">
				<div class="ryo-settings-watermark" aria-hidden="true">{activeMeta.jp}</div>
				<header class="ryo-settings-page-head">
					<div class="ryo-settings-eyebrow">
						<span class="ryo-settings-eyebrow-rule"></span>
						<span class="ryo-settings-eyebrow-mark">力</span>
						<span>{activeMeta.group}</span>
						<i></i><b>+</b><em>///</em>
					</div>
					<h2>{activeLabel}</h2>
					<p>{activeMeta.blurb}</p>
				</header>

				<div class="ryo-settings-scroll" data-ryo-own-scroll>
					{#key tab}
						<div class="ryo-settings-tabview">
							<div class="ryo-settings-card">
				{#if !loaded}
					<p class="text-sm text-muted-foreground">Loading…</p>
				{:else if tab === 'general'}
					<div class="border-b py-3">
						<div class="font-medium">Appearance</div>
						<p class="mt-0.5 mb-3 text-sm text-muted-foreground">Follow the desktop automatically, or keep Ryotunes in its comfortable light or dark palette.</p>
						<div class="flex flex-wrap gap-2" role="radiogroup" aria-label="Appearance">
							{#each [['system','Follow system'], ['light','Light'], ['dark','Dark']] as option}
								<Button variant={appearance.themeMode === option[0] ? 'default' : 'outline'} size="sm" role="radio" aria-checked={appearance.themeMode === option[0]} onclick={() => setTheme(option[0] as ThemeMode)}>{option[1]}</Button>
							{/each}
						</div>
						<p class="mt-2 text-xs text-muted-foreground">Currently {resolvedTheme()}. Ryoku accent and reduced-motion preferences still apply.</p>
					</div>
					<div class="flex items-start justify-between gap-4 border-b py-3">
						<div class="min-w-0">
							<div class="font-medium">Watch history</div>
							<p class="mt-0.5 text-sm text-muted-foreground">
								{auth.account?.signedIn ? 'Register completed plays in your YouTube Music history.' : 'Sign in to register completed plays in your YouTube Music history.'}
							</p>
						</div>
						<Switch checked={historyOn} onCheckedChange={setHistory} />
					</div>
					<div class="flex items-start justify-between gap-4 border-b py-3">
						<div class="min-w-0">
							<div class="font-medium">Discord rich presence</div>
							<p class="mt-0.5 text-sm text-muted-foreground">Show what you're listening to on your Discord profile through the local Discord desktop client.</p>
							<p class="mt-1 text-xs font-medium" data-discord-status={discordState.status}>Status: {discordState.status === 'connected' ? 'Connected' : discordState.status === 'connecting' ? 'Connecting…' : discordState.status === 'unavailable' ? 'Discord not running / unavailable' : 'Disabled'}</p>
						</div>
						<Switch checked={discordOn} onCheckedChange={setDiscord} />
					</div>
					<div class="border-b py-3">
						<div class="font-medium">Discord presence title</div>
						<p class="mt-0.5 text-sm text-muted-foreground">
							Customize the text Discord renders as “Listening to …”. Track, artist and Ryotunes'
							application identity stay unchanged.
						</p>
						<div class="mt-3 flex max-w-xl items-center gap-2">
							<Input
								bind:value={discordNameInput}
								maxlength={128}
								placeholder="Ryotunes"
								aria-label="Discord presence title"
							/>
							<Button
								variant="outline"
								size="sm"
								disabled={savingDiscordName || !discordNameInput.trim()}
								onclick={saveDiscordName}
							>
								{savingDiscordName ? 'Saving…' : 'Save'}
							</Button>
							<Button
								variant="ghost"
								size="sm"
								disabled={savingDiscordName || discordNameInput === 'Ryotunes'}
								onclick={resetDiscordName}
							>
								Reset
							</Button>
						</div>
						<p class="mt-2 text-xs text-muted-foreground">
							Preview: Listening to {discordNameInput.trim() || 'Ryotunes'}
						</p>
					</div>
					<div class="flex items-start justify-between gap-4 border-b py-3">
						<div class="min-w-0">
							<div class="font-medium">Close to tray</div>
							<p class="mt-0.5 text-sm text-muted-foreground">
								Closing the window keeps music playing in the background. Restore or quit from the
								tray icon.
							</p>
						</div>
						<Switch checked={trayOn} onCheckedChange={setTray} />
					</div>
					<div class="flex items-start justify-between gap-4 border-b py-3">
						<div class="min-w-0">
							<div class="font-medium">Low resource mode</div>
							<p class="mt-0.5 text-sm text-muted-foreground">
								Keep playback quality unchanged while disabling speculative stream warming, reducing automatic Home/network work, slowing nonessential UI updates and suppressing decorative motion.
							</p>
						</div>
						<Switch checked={appearance.lowResourceMode} onCheckedChange={setLowResource} />
					</div>
					<div class="flex items-start justify-between gap-4 border-b py-3">
						<div class="min-w-0">
							<div class="font-medium">Start on login</div>
							<p class="mt-0.5 text-sm text-muted-foreground">
								Launch Ryotunes automatically when you log in.
							</p>
						</div>
						<Switch checked={autostartOn} onCheckedChange={setAutostart} />
					</div>
					<div class="py-3">
						<div class="font-medium">Interface scale</div>
						<p class="mt-0.5 mb-3 text-sm text-muted-foreground">
							Adjust the WebKit interface without changing your desktop scaling. Ctrl+0 restores 110%.
						</p>
						<div class="flex flex-wrap gap-2">
							{#each UI_SCALES as n (n)}
								<Button variant={uiScale === n ? 'default' : 'outline'} size="sm" onclick={() => setUiScale(n)}>{n}%</Button>
							{/each}
						</div>
					</div>
				{:else if tab === 'playback'}
					<div class="ryo-settings-subhead"><span>// PLAYER BEHAVIOUR</span><b>LOCAL</b></div>
					<div class="flex items-start justify-between gap-4 border-b py-3">
						<div class="min-w-0"><div class="font-medium">Open the player when playback starts</div><p class="mt-0.5 text-sm text-muted-foreground">Bring the full listening view forward when you choose a track, album or playlist.</p></div>
						<Switch checked={appearance.openPlayerOnPlay} onCheckedChange={(on) => setAppearance({ openPlayerOnPlay: on })} />
					</div>
					<div class="flex items-start justify-between gap-4 border-b py-3">
						<div class="min-w-0"><div class="font-medium">Queue and lyrics inside the player</div><p class="mt-0.5 text-sm text-muted-foreground">Keep Queue and Lyrics as tabs in Now Playing. Turn off to use the floating side panels instead.</p></div>
						<Switch checked={appearance.tabbedPlayer} onCheckedChange={(on) => setAppearance({ tabbedPlayer: on })} />
					</div>
					<div class="border-b py-3">
						<div class="font-medium">Audio quality</div>
						<p class="mt-0.5 mb-3 text-sm text-muted-foreground">
							Preferred stream quality when resolving a track.
						</p>
						<div class="flex gap-2">
							{#each QUALITIES as q (q.id)}
								<Button
									variant={quality === q.id ? 'default' : 'outline'}
									size="sm"
									onclick={() => setQuality(q.id)}
								>
									{q.label}
								</Button>
							{/each}
						</div>
					</div>
					<div class="flex items-start justify-between gap-4 border-b py-3">
						<div class="min-w-0">
							<div class="font-medium">Autoplay</div>
							<p class="mt-0.5 text-sm text-muted-foreground">
								Keep the music going with similar songs when your queue ends.
							</p>
						</div>
						<Switch checked={autoplayOn} onCheckedChange={setAutoplay} />
					</div>
					<div class="flex items-start justify-between gap-4 border-b py-3">
						<div class="min-w-0">
							<div class="font-medium">Prevent duplicate tracks in queue</div>
							<p class="mt-0.5 text-sm text-muted-foreground">
								Adding a track that's already in the queue moves it from its old position instead of
								adding a second copy.
							</p>
						</div>
						<Switch checked={preventDuplicatesOn} onCheckedChange={setPreventDuplicates} />
					</div>
					<div class="flex items-start justify-between gap-4 border-b py-3">
						<div class="min-w-0">
							<div class="font-medium">Word-by-word lyrics</div>
							<p class="mt-0.5 text-sm text-muted-foreground">
								Asks lyrics-api.boidu.dev first, the only source here with per-word timings, so
								lyrics can highlight as they're sung. It's checked for every track, so turning
								this off keeps your listening off that service. Other sources still provide
								line-by-line lyrics.
							</p>
						</div>
						<Switch checked={boiduOn} onCheckedChange={setBoidu} />
					</div>
					<div class="py-3">
						<div class="font-medium">Stream clients</div>
						<p class="mt-0.5 mb-2 text-sm text-muted-foreground">
							Advanced — turn a client off to skip it when resolving streams. Overridden by a
							RYOTUNES_DISABLED_CLIENTS environment value when one is configured.
						</p>
						<div class="flex flex-col gap-2">
							{#each clients as name (name)}
								<div class="flex items-center justify-between">
									<span class="font-mono text-sm">{name}</span>
									<Switch
										checked={!disabled.has(name)}
										onCheckedChange={() => toggleClient(name)}
									/>
								</div>
							{/each}
						</div>
					</div>
				{:else if tab === 'data'}
					<div class="border-b py-3">
						<div class="font-medium">Proxy</div>
						<p class="mt-0.5 mb-3 text-sm text-muted-foreground">
							HTTP or HTTPS proxy for all YouTube traffic. Authenticated proxy URLs are not
							stored. Takes effect on restart.
						</p>
						<form
							class="flex gap-2"
							onsubmit={(e) => {
								e.preventDefault();
								saveProxy();
							}}
						>
							<Input bind:value={proxyInput} placeholder="http://host:port (blank = none)" />
							<Button type="submit" variant="outline">Save</Button>
						</form>
					</div>
					<div class="py-3">
						<div class="font-medium">Cache</div>
						<p class="mt-0.5 mb-3 text-sm text-muted-foreground">
							Clear cached stream URLs and downloaded audio bytes.
						</p>
						<Button variant="destructive" size="sm" onclick={doClearCaches} disabled={clearing}>
							{clearing ? 'Clearing…' : 'Clear caches'}
						</Button>
					</div>
				{:else if tab === 'keybinds'}
					<div class="ryo-settings-subhead"><span>// KEYBOARD MAP</span><b>{KEYBIND_GROUPS.reduce((n, g) => n + g.rows.length, 0)} BINDINGS</b></div>
					<label class="ryo-keybind-search"><span>⌕</span><input bind:value={keybindFilter} placeholder="Filter shortcuts…" autocomplete="off" spellcheck="false" /></label>
					<div class="ryo-keybind-groups">
						{#each visibleKeybindGroups as group (group.title)}
							<section class="ryo-keybind-group">
								<header><span>{group.title}</span><b>{String(group.rows.length).padStart(2, '0')}</b></header>
								{#each group.rows as row (row.id)}
									<div class="ryo-keybind-row"><span>{row.label}</span><kbd>{row.display}</kbd></div>
								{/each}
							</section>
						{/each}
						{#if !visibleKeybindGroups.length}<p class="py-4 text-sm text-muted-foreground">No shortcut matches that filter.</p>{/if}
					</div>
				{:else if tab === 'about'}
					<div class="border-b py-3">
						<div class="font-heading text-lg font-bold">Ryotunes</div>
						<p class="mt-1 text-sm text-muted-foreground">
							A focused Ryoku desktop music instrument: your YouTube Music library, local media
							controls, queue, lyrics and playback engine in one paper-and-ink surface.
						</p>
						<div class="ryo-about-build"><span>VERSION</span><strong>{appVersion || '—'}</strong><span>ENGINE</span><strong>RUST + MPV</strong><span>UI</span><strong>TAURI / WEBKITGTK</strong></div>
					</div>
					<div class="border-b py-3">
						<div class="font-medium">Software updates</div>
						<p class="mt-0.5 text-sm text-muted-foreground">
							You're on {appVersion || 'this build'}. Ryotunes checks GitHub only when you ask — nothing runs in the background.
						</p>
						<div class="mt-3 flex flex-wrap items-center gap-2">
							<Button variant="outline" size="sm" onclick={checkForUpdates} disabled={updateBusy || updateStatus === 'installed'}>
								{updateStatus === 'checking' ? 'Checking…' : 'Check for new version'}
							</Button>
							<Button variant="ghost" size="sm" onclick={() => api.openExternal(update?.releaseUrl || RELEASES_URL)}>View changelog</Button>
							{#if canInstallUpdate}
								<Button variant="default" size="sm" onclick={installUpdate} disabled={updateBusy}>
									{updateStatus === 'installing' ? 'Installing…' : `Update to ${update?.latestVersion}`}
								</Button>
							{/if}
						</div>
						<p class="mt-2 text-xs" role="status" aria-live="polite" data-ryo-update-state={updateStatus}>
							{#if updateStatus === 'checking'}
								Checking for a new version…
							{:else if updateStatus === 'installing'}
								Downloading and verifying the update, then asking for administrator approval. Keep Ryotunes open until it finishes.
							{:else if updateStatus === 'installed'}
								Ryotunes {installedVersion} is installed. Quit Ryotunes and open it again to finish — closing the window isn't enough; the background service reloads only on a full restart.
							{:else if updateStatus === 'error'}
								Update failed: {updateError}
							{:else if update}
								{#if update.latestVersion === null}
									No release has been published yet.
								{:else if update.available}
									Version {update.latestVersion} is available.{#if !update.canInstall && update.unsupportedReason} {update.unsupportedReason}{/if}
								{:else}
									You're on the latest version.
								{/if}
							{/if}
						</p>
						{#if update?.notes}
							<div class="mt-2">
								<Button variant="ghost" size="sm" aria-expanded={showReleaseNotes} onclick={() => (showReleaseNotes = !showReleaseNotes)}>
									{showReleaseNotes ? 'Hide release notes' : 'Show release notes'}
								</Button>
								{#if showReleaseNotes}
									<pre class="mt-2 max-h-48 overflow-auto whitespace-pre-wrap rounded border p-2 font-mono text-xs text-muted-foreground">{update.notes}</pre>
								{/if}
							</div>
						{/if}
					</div>
					<div class="border-b py-3">
						<div class="font-medium">Made by</div>
						<p class="mt-0.5 text-sm text-muted-foreground">Ryotunes is built by two developers. Open their GitHub profiles:</p>
						<div class="mt-2 flex flex-wrap gap-2">
							<Button variant="outline" size="sm" onclick={() => api.openExternal('https://github.com/ashmitvoid')}>ashmitvoid</Button>
							<Button variant="outline" size="sm" onclick={() => api.openExternal('https://github.com/neur0map')}>neur0map</Button>
						</div>
					</div>
					<div class="py-3">
						<div class="font-medium">Open source</div>
						<p class="mt-0.5 text-sm text-muted-foreground">
							Ryotunes is a fork of LiMusic, released under the GNU General Public License v3.0 or later.
						</p>
						<div class="mt-2 flex flex-wrap gap-2">
							<Button variant="ghost" size="sm" onclick={() => api.openExternal('https://github.com/SimoHypers/limusic')}>LiMusic upstream</Button>
							<Button variant="ghost" size="sm" onclick={() => api.openExternal('https://www.gnu.org/licenses/gpl-3.0.html')}>GPL-3.0-or-later</Button>
						</div>
					</div>
				{/if}
							</div>
						</div>
					{/key}
				</div>
			</section>

			<div class="ryo-settings-specimen-wrap">
				<RyokuSpecimen
					image={settingsArt}
					artMode={tab === 'data' ? 'data' : tab === 'about' ? 'about' : 'auto'}
					code={activeMeta.code}
					title={activeMeta.jp}
					sub={activeLabel.toUpperCase()}
					caption={settingsCaption}
					status={loaded ? 'LIVE' : 'LOADING'}
					readout={settingsReadout}
				/>
			</div>
		</div>
	</Dialog.Content>
</Dialog.Root>
