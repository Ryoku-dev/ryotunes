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
	import { t } from '$lib/i18n.svelte';

	type TabId = 'general' | 'playback' | 'data' | 'keybinds' | 'about';
	const TABS: { id: TabId; label: string }[] = [
		{ id: 'general', label: t('General') },
		{ id: 'playback', label: t('Playback') },
		{ id: 'data', label: t('Data & storage') },
		{ id: 'keybinds', label: t('Keybinds') },
		{ id: 'about', label: t('About') }
	];

	const TAB_META: Record<TabId, { jp: string; group: string; blurb: string; code: string }> = {
		general: { jp: '全般', group: 'APPLICATION', blurb: t('Session behaviour, desktop integration and the things that should stay out of your way.'), code: 'APP-01' },
		playback: { jp: '再生', group: 'PLAYBACK', blurb: t('How the listening engine resolves, queues and carries a session forward.'), code: 'PLAY-02' },
		data: { jp: '保存', group: 'DATA', blurb: t('Network routing and local storage used to keep the instrument responsive.'), code: 'DATA-03' },
		keybinds: { jp: '鍵', group: 'KEYBINDS', blurb: t('Every Ryotunes shortcut, grouped by intent and read from the same registry the app executes.'), code: 'KEY-04' },
		about: { jp: '力', group: 'ABOUT', blurb: t('Build identity and the small set of components that make Ryotunes a Ryoku music instrument.'), code: 'INFO-05' }
	};

	let tab = $state<TabId>('general');
	const metaFor = (id: TabId) => TAB_META[id];
	const activeMeta = $derived(metaFor(tab));
	const activeLabel = $derived(TABS.find((t) => t.id === tab)?.label ?? t('General'));
	const PRODUCT_VERSION = 'v2.4';
	let buildVersion = $state('2.4.1');
	getVersion().then((v) => (buildVersion = v)).catch(() => {});
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
					? `${t('Session ready')}${auth.account.name ? ` — ${auth.account.name}` : ''}. ${t('Desktop services stay local.')}`
					: t('Local desktop session. Sign in only when your YouTube library needs it.');
			}
			if (tab === 'playback') {
				return playback.now
					? `${playback.now.title} — ${playback.now.artists || t('Now playing')}`
					: t('Playback engine ready. No stream is active.');
			}
			if (tab === 'data') return t('Tape, cache and transport — the local path that keeps playback immediate.');
			if (tab === 'keybinds') return t('Keyboard flow is part of the instrument: searchable, grouped and always in sync with the live bindings.');
			return `${t('Ryotunes')} ${PRODUCT_VERSION} — ${t('a Ryoku-native music instrument built around Rust, mpv and WebKitGTK.')}`;
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
			`VERSION|${PRODUCT_VERSION}`,
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
		{ id: 'LOW', label: t('Low') },
		{ id: 'AUTO', label: t('Auto') },
		{ id: 'HIGH', label: t('High') }
	];

	async function setQuality(q: string) {
		settings.quality = q;
		await api.setSetting('quality', q);
		// Cached URLs are keyed by video only, so clear them to apply the new quality everywhere.
		await api.clearCaches();
		toast.success(t('Audio quality updated'));
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
			toast.error(t('Discord presence title must be between 2 and 128 characters'));
			return;
		}
		savingDiscordName = true;
		try {
			await api.setSetting('discord_presence_name', value);
			settings.discord_presence_name = value;
			discordNameInput = value;
			toast.success(`${t('Discord now shows')} “${t('Listening to')} ${value}”`);
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
			toast.success(t('Proxy saved — restart to apply'));
		} catch (e) {
			toast.error(String(e));
		}
	}

	async function doClearCaches() {
		clearing = true;
		try {
			await api.clearCaches();
			toast.success(t('Caches cleared'));
		} finally {
			clearing = false;
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
			<Dialog.Description class="sr-only">{t("Ryotunes application settings")}</Dialog.Description>
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
					<p class="text-sm text-muted-foreground">{t('Loading…')}</p>
				{:else if tab === 'general'}
					<div class="border-b py-3">
						<div class="font-medium">{t('Appearance')}</div>
						<p class="mt-0.5 mb-3 text-sm text-muted-foreground">{t("Follow the desktop automatically, or keep Ryotunes in its comfortable light or dark palette.")}</p>
						<div class="flex flex-wrap gap-2" role="radiogroup" aria-label={t('Appearance')}>
							{#each [['system', t('Follow system')], ['light', t('Light')], ['dark', t('Dark')]] as option}
								<Button variant={appearance.themeMode === option[0] ? 'default' : 'outline'} size="sm" role="radio" aria-checked={appearance.themeMode === option[0]} onclick={() => setTheme(option[0] as ThemeMode)}>{option[1]}</Button>
							{/each}
						</div>
						<p class="mt-2 text-xs text-muted-foreground">{t('Currently')} {resolvedTheme()}. {t("Ryoku accent and reduced-motion preferences still apply.")}</p>
						</div>
					<div class="flex items-start justify-between gap-4 border-b py-3">
						<div class="min-w-0">
							<div class="font-medium">{t('Watch history')}</div>
							<p class="mt-0.5 text-sm text-muted-foreground">
								{t(auth.account?.signedIn ? 'Register completed plays in your YouTube Music history.' : 'Sign in to register completed plays in your YouTube Music history.')}
							</p>
						</div>
						<Switch checked={historyOn} onCheckedChange={setHistory} />
					</div>
					<div class="flex items-start justify-between gap-4 border-b py-3">
						<div class="min-w-0">
							<div class="font-medium">{t('Discord rich presence')}</div>
							<p class="mt-0.5 text-sm text-muted-foreground">{t("Show what you're listening to on your Discord profile through the local Discord desktop client.")}</p>
							<p class="mt-1 text-xs font-medium" data-discord-status={discordState.status}>{t('Status:')} {t(discordState.status === 'connected' ? 'Connected' : discordState.status === 'connecting' ? 'Connecting…' : discordState.status === 'unavailable' ? 'Discord not running / unavailable' : 'Disabled')}</p>
						</div>
						<Switch checked={discordOn} onCheckedChange={setDiscord} />
					</div>
					<div class="border-b py-3">
						<div class="font-medium">{t('Discord presence title')}</div>
						<p class="mt-0.5 text-sm text-muted-foreground">
							{t("Customize the text Discord renders as “Listening to …”. Track, artist and Ryotunes' application identity stay unchanged.")}
						</p>
						<div class="mt-3 flex max-w-xl items-center gap-2">
							<Input
								bind:value={discordNameInput}
								maxlength={128}
								placeholder="Ryotunes"
								aria-label={t('Discord presence title')}
							/>
							<Button
								variant="outline"
								size="sm"
								disabled={savingDiscordName || !discordNameInput.trim()}
								onclick={saveDiscordName}
							>
								{t(savingDiscordName ? 'Saving…' : 'Save')}
							</Button>
							<Button
								variant="ghost"
								size="sm"
								disabled={savingDiscordName || discordNameInput === 'Ryotunes'}
								onclick={resetDiscordName}
							>
								{t('Reset')}
							</Button>
						</div>
						<p class="mt-2 text-xs text-muted-foreground">
							{t('Preview:')} {t('Listening to')} {discordNameInput.trim() || 'Ryotunes'}
						</p>
					</div>
					<div class="flex items-start justify-between gap-4 border-b py-3">
						<div class="min-w-0">
							<div class="font-medium">{t('Close to tray')}</div>
							<p class="mt-0.5 text-sm text-muted-foreground">
								{t("Closing the window keeps music playing in the background. Restore or quit from the tray icon.")}
							</p>
						</div>
						<Switch checked={trayOn} onCheckedChange={setTray} />
					</div>
					<div class="flex items-start justify-between gap-4 border-b py-3">
						<div class="min-w-0">
							<div class="font-medium">{t('Low resource mode')}</div>
							<p class="mt-0.5 text-sm text-muted-foreground">
								{t("Keep playback quality unchanged while disabling speculative stream warming, reducing automatic Home/network work, slowing nonessential UI updates and suppressing decorative motion.")}
							</p>
						</div>
						<Switch checked={appearance.lowResourceMode} onCheckedChange={setLowResource} />
					</div>
					<div class="flex items-start justify-between gap-4 border-b py-3">
						<div class="min-w-0">
							<div class="font-medium">{t('Start on login')}</div>
							<p class="mt-0.5 text-sm text-muted-foreground">
								{t('Launch Ryotunes automatically when you log in.')}
							</p>
						</div>
						<Switch checked={autostartOn} onCheckedChange={setAutostart} />
					</div>
					<div class="py-3">
						<div class="font-medium">{t('Interface scale')}</div>
						<p class="mt-0.5 mb-3 text-sm text-muted-foreground">
							{t("Adjust the WebKit interface without changing your desktop scaling. Ctrl+0 restores 110%.")}
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
						<div class="min-w-0"><div class="font-medium">{t('Open the player when playback starts')}</div><p class="mt-0.5 text-sm text-muted-foreground">{t("Bring the full listening view forward when you choose a track, album or playlist.")}</p></div>
						<Switch checked={appearance.openPlayerOnPlay} onCheckedChange={(on) => setAppearance({ openPlayerOnPlay: on })} />
					</div>
					<div class="flex items-start justify-between gap-4 border-b py-3">
						<div class="min-w-0"><div class="font-medium">{t('Queue and lyrics inside the player')}</div><p class="mt-0.5 text-sm text-muted-foreground">{t("Keep Queue and Lyrics as tabs in Now Playing. Turn off to use the floating side panels instead.")}</p></div>
						<Switch checked={appearance.tabbedPlayer} onCheckedChange={(on) => setAppearance({ tabbedPlayer: on })} />
					</div>
					<div class="border-b py-3">
						<div class="font-medium">{t('Audio quality')}</div>
						<p class="mt-0.5 mb-3 text-sm text-muted-foreground">
							{t("Preferred stream quality when resolving a track.")}
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
							<div class="font-medium">{t('Autoplay')}</div>
							<p class="mt-0.5 text-sm text-muted-foreground">
								{t("Keep the music going with similar songs when your queue ends.")}
							</p>
						</div>
						<Switch checked={autoplayOn} onCheckedChange={setAutoplay} />
					</div>
					<div class="flex items-start justify-between gap-4 border-b py-3">
						<div class="min-w-0">
							<div class="font-medium">{t('Prevent duplicate tracks in queue')}</div>
							<p class="mt-0.5 text-sm text-muted-foreground">
								{t("Adding a track that's already in the queue moves it from its old position instead of adding a second copy.")}
							</p>
						</div>
						<Switch checked={preventDuplicatesOn} onCheckedChange={setPreventDuplicates} />
					</div>
					<div class="flex items-start justify-between gap-4 border-b py-3">
						<div class="min-w-0">
							<div class="font-medium">{t('Word-by-word lyrics')}</div>
							<p class="mt-0.5 text-sm text-muted-foreground">
								{t("Asks lyrics-api.boidu.dev first, the only source here with per-word timings, so lyrics can highlight as they're sung. It's checked for every track, so turning this off keeps your listening off that service. Other sources still provide line-by-line lyrics.")}
							</p>
						</div>
						<Switch checked={boiduOn} onCheckedChange={setBoidu} />
					</div>
					<div class="py-3">
						<div class="font-medium">{t('Stream clients')}</div>
						<p class="mt-0.5 mb-2 text-sm text-muted-foreground">
							{t("Advanced — turn a client off to skip it when resolving streams. Overridden by a RYOTUNES_DISABLED_CLIENTS environment value when one is configured.")}
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
						<div class="font-medium">{t('Proxy')}</div>
						<p class="mt-0.5 mb-3 text-sm text-muted-foreground">
							{t("HTTP or HTTPS proxy for all YouTube traffic. Authenticated proxy URLs are not stored. Takes effect on restart.")}
						</p>
						<form
							class="flex gap-2"
							onsubmit={(e) => {
								e.preventDefault();
								saveProxy();
							}}
						>
							<Input bind:value={proxyInput} placeholder={t("http://host:port (blank = none)")} />
							<Button type="submit" variant="outline">{t('Save')}</Button>
						</form>
					</div>
					<div class="py-3">
						<div class="font-medium">{t('Cache')}</div>
						<p class="mt-0.5 mb-3 text-sm text-muted-foreground">
							{t("Clear cached stream URLs and downloaded audio bytes.")}
						</p>
						<Button variant="destructive" size="sm" onclick={doClearCaches} disabled={clearing}>
							{t(clearing ? 'Clearing…' : 'Clear caches')}
						</Button>
					</div>
				{:else if tab === 'keybinds'}
					<div class="ryo-settings-subhead"><span>// KEYBOARD MAP</span><b>{KEYBIND_GROUPS.reduce((n, g) => n + g.rows.length, 0)} BINDINGS</b></div>
					<label class="ryo-keybind-search"><span>⌕</span><input bind:value={keybindFilter} placeholder={t('Filter shortcuts…')} autocomplete="off" spellcheck="false" /></label>
					<div class="ryo-keybind-groups">
						{#each visibleKeybindGroups as group (group.title)}
							<section class="ryo-keybind-group">
								<header><span>{group.title}</span><b>{String(group.rows.length).padStart(2, '0')}</b></header>
								{#each group.rows as row (row.id)}
									<div class="ryo-keybind-row"><span>{row.label}</span><kbd>{row.display}</kbd></div>
								{/each}
							</section>
						{/each}
						{#if !visibleKeybindGroups.length}<p class="py-4 text-sm text-muted-foreground">{t('No shortcut matches that filter.')}</p>{/if}
					</div>
				{:else if tab === 'about'}
					<div class="border-b py-3">
						<div class="font-heading text-lg font-bold">Ryotunes</div>
						<p class="mt-1 text-sm text-muted-foreground">
							{t("A focused Ryoku desktop music instrument: your YouTube Music library, local media controls, queue, lyrics and playback engine in one paper-and-ink surface.")}
						</p>
						<div class="ryo-about-build"><span>RELEASE</span><strong>{PRODUCT_VERSION}</strong><span>BUILD</span><strong>{buildVersion}</strong><span>ENGINE</span><strong>RUST + MPV</strong><span>UI</span><strong>TAURI / WEBKITGTK</strong></div>
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
