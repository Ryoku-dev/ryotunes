<script lang="ts">
	import './layout.css';
	import '$lib/ryotunes.css';
	import favicon from '$lib/assets/favicon.svg';
	import { HugeiconsIcon } from '@hugeicons/svelte';
	import {
		CheckmarkCircle02Icon,
		AlertCircleIcon,
		InformationCircleIcon
	} from '@hugeicons/core-free-icons';
	import { browser } from '$app/environment';
	import { onMount } from 'svelte';
	import { beforeNavigate, afterNavigate } from '$app/navigation';
	import { page } from '$app/state';
	import { getCurrentWindow } from '@tauri-apps/api/window';
	import { fade } from 'svelte/transition';
	import { appearance, initAppearance } from '$lib/theme.svelte';
	import { blockForeignDrag, dragScroll } from '$lib/dnd';
	import { suppressNative } from '$lib/menu';
	import Sidebar from '$lib/components/Sidebar.svelte';
	import Titlebar from '$lib/components/Titlebar.svelte';
	import ResizeBorders from '$lib/components/ResizeBorders.svelte';
	import PlayerBar from '$lib/components/PlayerBar.svelte';
	import QueuePanel from '$lib/components/QueuePanel.svelte';
	import LyricsPanel from '$lib/components/LyricsPanel.svelte';
	import AddToPlaylist from '$lib/components/AddToPlaylist.svelte';
	import SettingsDialog from '$lib/components/SettingsDialog.svelte';
	import ShareDialog from '$lib/components/ShareDialog.svelte';
	import ChannelPicker from '$lib/components/ChannelPicker.svelte';
	import ListenTogether from '$lib/components/ListenTogether.svelte';
	import LinkDialog from '$lib/components/LinkDialog.svelte';
	import MiniPlayer from '$lib/components/MiniPlayer.svelte';
	import NowPlaying from '$lib/components/NowPlaying.svelte';
	import CommandPalette from '$lib/components/CommandPalette.svelte';
	import KeyboardShortcuts from '$lib/components/KeyboardShortcuts.svelte';
	import RyokuAtmosphere from '$lib/components/RyokuAtmosphere.svelte';
	import { auth, initApp, np, playback, ui } from '$lib/player.svelte';
	import { win, initWin } from '$lib/win.svelte';
	import { initZoom } from '$lib/zoom';
	import { initShortcuts } from '$lib/shortcuts';
	import { initRyokuLiveTokens } from '$lib/ryoku-live';
	import { initPrecisionScrollFallback } from '$lib/ryoku-scroll';
	import * as api from '$lib/api';
	import { loadRouteScroll, rememberRoute, saveRouteScroll } from '$lib/session';
	import { t } from '$lib/i18n.svelte';

	let { children } = $props();
	// Two ways the now-playing view and these panels can divide the same two buttons, picked in
	// settings (#62). Tabbed (the default): the view carries queue and lyrics itself, so the panels
	// step aside for it and the bar's buttons switch its tabs. Off: these are the only owner, the
	// buttons always mean the panels, and the panels float over that view like they float over a
	// page, so opening it costs you nothing you had open.
	const tabbed = $derived(np.open && appearance.tabbedPlayer);
	$effect(() => {
		if (tabbed) ui.queueOpen = ui.lyricsOpen = false;
	});

	// All portal surfaces use the same Ryoku safe rectangle. Keep its left inset in sync with
	// the rail instead of letting individual dialogs invent viewport coordinates.
	$effect(() => {
		if (!browser) return;
		document.documentElement.style.setProperty('--ryo-overlay-left', ui.sidebarCollapsed ? '64px' : '268px');
		document.documentElement.style.setProperty('--ryo-overlay-bottom', playback.now ? '60px' : '0px');
	});

	// The mini player runs this same SPA in a second window (Rust `mini.rs`), so the window label is
	// what tells the two apart: `mini` gets the widget instead of the app chrome, and none of the
	// routes below it are ever rendered. Constant for the window's lifetime.
	const isMini = browser && getCurrentWindow().label === 'mini';

	let mainEl = $state<HTMLElement>();
	let windowFocused = $state(true);

	// Browser-like route restoration. SvelteKit preserves data well, but WebKitGTK can still return
	// a long library/search page to the top after an overlay or detail page. Store per-URL positions
	// and restore after the destination has painted.
	beforeNavigate(() => {
		if (browser && mainEl) saveRouteScroll(page.url, mainEl.scrollTop);
	});
	afterNavigate(() => {
		if (!browser) return;
		rememberRoute(page.url);
		requestAnimationFrame(() => requestAnimationFrame(() => {
			if (mainEl) mainEl.scrollTop = loadRouteScroll(page.url);
		}));
	});

	// Apply the saved accent color before the first paint (ssr=false → nothing renders until now).
	if (browser) initAppearance();

	// First-paint acknowledgement is a short startup handshake, not a permanent timer. WebKitGTK can
	// occasionally mount the Svelte tree a frame before Tauri's invoke bridge is ready on a cold
	// process. v2.0 fired once and swallowed that transient failure, leaving the real window hidden
	// until a second launch. Retry a handful of times while this WebView is alive; native reveal is
	// idempotent and still happens only after the UI tree exists, so this cannot reintroduce a blank
	// boot frame.
	function acknowledgeFrontend(label: 'main' | 'mini', beforeReveal?: Promise<void>) {
		let cancelled = false;
		let timer: ReturnType<typeof setTimeout> | undefined;
		const delays = [0, 45, 120, 260, 520, 900];
		const attempt = async (index: number) => {
			if (cancelled) return;
			try {
				await api.frontendReady(label);
			} catch {
				if (cancelled || index + 1 >= delays.length) return;
				timer = setTimeout(() => void attempt(index + 1), delays[index + 1]);
			}
		};
		// `onMount` already means the Svelte tree exists. Do NOT gate this invoke behind
		// requestAnimationFrame: WebKitGTK may suspend rAF for a Tauri window created
		// `visible:false`, which deadlocks the only callback capable of revealing that window.
		// Start the bridge handshake immediately; retries use ordinary timers, which continue
		// to run for a hidden WebView. Native code also owns a bounded reveal failsafe.
		// Palette-before-reveal is worth a short wait, not the whole IPC round-trip: gating on
		// the token invoke alone routinely outran the native 1.5 s failsafe on a cold WebKit
		// process, so the window was revealed natively, unhandshaken. Bound the wait.
		const begin = () => { if (!cancelled) void attempt(0); };
		if (beforeReveal) {
			const bounded = Promise.race([beforeReveal, new Promise<void>((r) => setTimeout(r, 250))]);
			void bounded.then(begin, begin);
		} else begin();
		return () => { cancelled = true; if (timer) clearTimeout(timer); };
	}

	// Wire the Tauri event bridge once for the whole app and tear it down with the window.
	onMount(() => {
		if (isMini) {
			document.getElementById('ryotunes-boot')?.remove();
			const teardownApp = initApp(true);
			const teardownShortcuts = initShortcuts('mini');
			const ryokuTokens = initRyokuLiveTokens();
			// Paint the current Ryoku palette before exposing the widget. The native reveal failsafe
			// remains authoritative if the local token invoke ever fails.
			const teardownReady = acknowledgeFrontend('mini', ryokuTokens.ready);
			return () => { teardownReady(); ryokuTokens.destroy(); teardownShortcuts(); teardownApp(); };
		}
		windowFocused = document.hasFocus();
		const teardownWin = initWin();
		const teardownApp = initApp();
		const teardownZoom = initZoom();
		const teardownShortcuts = initShortcuts();
		const ryokuTokens = initRyokuLiveTokens();
		const teardownPrecisionScroll = initPrecisionScrollFallback();
		// Cold start and tray reconstruction share one first-paint handshake. Geometry and the current
		// Ryoku palette are applied while hidden; reveal cannot expose a stale/default colour frame.
		const teardownReady = acknowledgeFrontend('main', ryokuTokens.ready);
		// Keep the native half of Low Resource mode in sync from the first window, not only after
		// Settings has been opened. This is one local SQLite write and never blocks first paint.
		void api.setSetting('low_resource_mode', appearance.lowResourceMode ? 'true' : 'false').catch(() => {});
		const setOnline = () => (ui.offline = false);
		const setOffline = () => (ui.offline = true);
		window.addEventListener('online', setOnline);
		window.addEventListener('offline', setOffline);
		return () => {
			teardownReady();
			teardownApp();
			teardownWin();
			teardownZoom();
			teardownShortcuts();
			ryokuTokens.destroy();
			teardownPrecisionScroll();
			window.removeEventListener('online', setOnline);
			window.removeEventListener('offline', setOffline);
		};
	});
</script>


<svelte:window
	onfocus={() => (windowFocused = true)}
	onblur={() => (windowFocused = false)}
	ondragover={blockForeignDrag}
	ondrop={blockForeignDrag}
	oncontextmenu={suppressNative}
/>

<svelte:head><link rel="icon" href={favicon} /></svelte:head>

{#if isMini}
	<MiniPlayer />
{:else}
	
	<div
		class="flex h-screen flex-col overflow-hidden bg-background text-foreground {win.maximized
			? ''
			: 'rounded-lg'}"
	>
		<ResizeBorders />
		<Titlebar />
		
		<div class="ryo-workspace relative flex min-h-0 flex-1 overflow-hidden" class:ryo-sidebar-collapsed={ui.sidebarCollapsed}>
			<Sidebar />
			<RyokuAtmosphere active={windowFocused && !np.open && !!playback.now && !playback.paused} />
			{#if ui.offline}<div class="ryo-offline-strip" role="status">{t('OFFLINE · cached and local music remain available')}</div>{/if}
			
			<main bind:this={mainEl} class="ryo-main relative z-[1] min-w-0 flex-1 overflow-y-auto" data-ryo-own-scroll class:ryo-main-suppressed={np.open && !!playback.now} {@attach dragScroll}>
				
				{#key auth.epoch}
					{@render children()}
				{/key}
			</main>
			{#if np.open && playback.now}<NowPlaying queueOpen={ui.queueOpen} lyricsOpen={ui.lyricsOpen} />{/if}
			
			{#if ui.lyricsOpen}<LyricsPanel onClose={() => (ui.lyricsOpen = false)} queueOpen={ui.queueOpen} />{/if}
			{#if ui.queueOpen}<QueuePanel onClose={() => (ui.queueOpen = false)} />{/if}
		</div>
		{#if playback.now}
			
			<div class="relative z-20" in:fade={{ duration: 170 }}>
				<PlayerBar
					onToggleQueue={() => (tabbed ? (np.tab = 'queue') : (ui.queueOpen = !ui.queueOpen))}
					queueOpen={tabbed ? np.tab === 'queue' : ui.queueOpen}
					onToggleLyrics={() => (tabbed ? (np.tab = 'lyrics') : (ui.lyricsOpen = !ui.lyricsOpen))}
					lyricsOpen={tabbed ? np.tab === 'lyrics' : ui.lyricsOpen}
				/>
			</div>
		{/if}
	</div>

	<!-- Expensive overlays mount only while they are in use; their state lives in the shared UI store. -->
	{#if ui.paletteOpen}<CommandPalette />{/if}
	{#if ui.shortcutsOpen}<KeyboardShortcuts />{/if}
	{#if ui.addSongs}<AddToPlaylist />{/if}
	{#if ui.share}<ShareDialog />{/if}
	{#if ui.settingsOpen}<SettingsDialog />{/if}
	{#if ui.channelPickerOpen || ui.channelPickerRequired}<ChannelPicker />{/if}
	{#if ui.ltOpen}<ListenTogether />{/if}
	{#if ui.linkOpen}<LinkDialog />{/if}

	
	{#if ui.toast}
		{@const t = ui.toast}
		<div
			transition:fade={{ duration: 90 }}
			class="ryo-toast fixed bottom-24 left-1/2 z-[100] flex -translate-x-1/2 items-center gap-2" data-kind={t.kind}
		>
			
			{#if t.kind === 'success'}
				<HugeiconsIcon icon={CheckmarkCircle02Icon} class="h-4 w-4 shrink-0 text-primary" />
			{:else if t.kind === 'error'}
				<HugeiconsIcon icon={AlertCircleIcon} class="h-4 w-4 shrink-0 text-destructive" />
			{:else}
				<HugeiconsIcon
					icon={InformationCircleIcon}
					class="h-4 w-4 shrink-0 text-muted-foreground"
				/>
			{/if}
			{t.msg}
		</div>
	{/if}
{/if}
