<script lang="ts">
	import { fade, scale } from 'svelte/transition';
	import { cubicOut } from 'svelte/easing';
	import { beforeNavigate } from '$app/navigation';
	import { HugeiconsIcon } from '@hugeicons/svelte';
	import {
		Maximize01Icon,
		Minimize01Icon,
		Mic01Icon,
		MusicNote01Icon,
		PlayIcon,
		PauseIcon,
		Queue01Icon
	} from '@hugeicons/core-free-icons';
	import * as Tabs from '$lib/components/ui/tabs';
	import * as api from '$lib/api';
	import { np, playback, ui } from '$lib/player.svelte';
	import { appearance } from '$lib/theme.svelte';
	import { t } from '$lib/i18n.svelte';
	import ArtworkImage from './ArtworkImage.svelte';
	import QueueList from './QueueList.svelte';
	import LyricsView from './LyricsView.svelte';

	// Off in settings, this view drops its tabs and the queue/lyrics panels stay in charge of both
	// (see +layout): they paint above this (z-30 over z-20), so all this needs is to hand back the
	// width they take at lg+ instead of letting them cover a third of the artwork. Below lg they're
	// a scrimmed overlay and there's nothing to shrink into. In tabbed mode both are always closed.
	let { queueOpen, lyricsOpen }: { queueOpen: boolean; lyricsOpen: boolean } = $props();
	const tabbed = $derived(appearance.tabbedPlayer);
	// Note: mirrors QueuePanel / LyricsPanel's w-80, keep in sync if those change.
	const panels = $derived(Number(queueOpen) + Number(lyricsOpen));
	const inset = $derived(['', 'lg:right-80', 'lg:right-[40rem]'][panels]);

	// Going somewhere means the user wants that page, not this one: minimise. The player bar brings
	// it back. beforeNavigate (not a pathname effect) so clicking the tab you're already on counts.
	beforeNavigate(() => (np.open = false));

	// Enlarged lyrics take the whole view, artwork column and tab strip included. A class swap
	// rather than unmounting the tabs: LyricsView must survive it or it refetches and loses its
	// scroll position.
	let big = $state(false);
	$effect(() => {
		if (np.tab !== 'lyrics') {
			big = false;
			np.lyricsFocus = false;
		} else {
			big = np.lyricsFocus;
		}
	});

	// Large artwork uses the shared bounded preparation path. It starts from the same 120px
	// thumbnail the bottom player is likely to have in WebKit's cache, then swaps only after the
	// high-resolution image has decoded. Track changes cannot paint a stale high-res image.

	// Clicking the artwork toggles playback, and flashes the action just taken over it so the click
	// visibly did something. Read `paused` before the toggle: the backend event that flips it is a
	// round trip away, and the icon has to be right on the frame the user clicked.
	let flash: 'play' | 'pause' | null = $state(null);
	let flashTimer: ReturnType<typeof setTimeout>;
	function toggle() {
		flash = playback.paused ? 'play' : 'pause';
		clearTimeout(flashTimer);
		flashTimer = setTimeout(() => (flash = null), 220);
		api.togglePause();
	}

</script>


<div
    transition:fade={{ duration: 210 }}
    class="npstable-root absolute inset-y-0 left-16 right-0 z-20 overflow-hidden bg-background {ui.sidebarCollapsed ? '' : 'lg:left-[16.75rem]'} {inset}"
>
    <div class="npstable-shell {tabbed ? 'npstable-tabbed' : 'npstable-solo'} {big ? 'npstable-focus' : ''}">
        {#if !big}
            <section class="npstable-preview" aria-label={t('Now playing media')}>
                <div class="npstable-preview-head" aria-hidden="true">
                    <span>// LIVE PREVIEW</span>
                    <span>PLAYBACK · LOCAL</span>
                </div>
                <div class="npstable-media npstable-artwork">
                    <button type="button" onclick={toggle} aria-label={t('Play/pause')} class="npstable-media-button">
                        {#if flash}
                            <div
                                in:scale={{ start: 0.7, duration: 150, easing: cubicOut }}
                                out:scale={{ start: 1.3, duration: 320, easing: cubicOut }}
                                class="npstable-flash"
                            >
                                <div class="rounded-full bg-black/55 p-3.5 text-white">
                                    <HugeiconsIcon icon={PauseIcon} altIcon={PlayIcon} showAlt={flash === 'play'} class="h-7 w-7" />
                                </div>
                            </div>
                        {/if}
                        {#if playback.now?.thumbnail}
                            <ArtworkImage
                                source={playback.now.thumbnail}
                                size={960}
                                previewSize={160}
                                className="npstable-media-object ryo-now-art-swap"
                            />
                        {:else}
                            <div class="npstable-placeholder">
                                <HugeiconsIcon icon={MusicNote01Icon} class="h-16 w-16" />
                            </div>
                        {/if}
                    </button>
                </div>
            </section>
        {/if}

        {#if tabbed}
            <aside
                class="npstable-detail {big ? 'npstable-detail-focus' : ''}"
                style="background:var(--ryo-paper);color:var(--ryo-ink);"
            >
                <Tabs.Root value={np.tab} onValueChange={(v) => (np.tab = v as typeof np.tab)} class="npstable-tabs">
                    <div class="npstable-tabs-head {big ? 'npstable-focus-head' : ''}">
                        {#if big}<span class="npstable-focus-label">{t('// LYRICS / FOCUS')}</span>{/if}
                        <Tabs.List class={big ? 'hidden' : 'npstable-tabs-list'}>
                            <Tabs.Trigger value="queue" class="ryo-lane-tab gap-2.5">
                                <HugeiconsIcon icon={Queue01Icon} class="h-4 w-4" /> {t('Queue')}
                            </Tabs.Trigger>
                            <Tabs.Trigger value="lyrics" class="ryo-lane-tab gap-2.5">
                                <HugeiconsIcon icon={Mic01Icon} class="h-4 w-4" /> {t('Lyrics')}
                            </Tabs.Trigger>
                        </Tabs.List>
                        {#if np.tab === 'lyrics'}
                            <button
                                onclick={() => (np.lyricsFocus = !np.lyricsFocus)}
                                class="npstable-focus-toggle"
                                aria-label={t(big ? 'Shrink lyrics' : 'Enlarge lyrics')}
                            >
                                <HugeiconsIcon icon={Maximize01Icon} altIcon={Minimize01Icon} showAlt={big} class="h-4 w-4" />
                            </button>
                        {/if}
                    </div>
                    {#if np.tab === 'queue'}
                        <Tabs.Content value="queue" class="npstable-content" style="background:var(--ryo-paper);color:var(--ryo-ink);">
                            <QueueList />
                        </Tabs.Content>
                    {:else}
                        <Tabs.Content value="lyrics" class="npstable-content" style="background:var(--ryo-paper);color:var(--ryo-ink);">
                            <LyricsView expanded={big} />
                        </Tabs.Content>
                    {/if}
                </Tabs.Root>
            </aside>
        {/if}
    </div>
</div>
