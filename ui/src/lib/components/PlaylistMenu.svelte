<script lang="ts">
	// The ⋯ menu on a sidebar library row, a card, or an artist row. Positioned `fixed` and moved to
	// <body> like TrackMenu: the playlist list is a scroll container, so an absolute popup would be
	// clipped by it. Right-clicking the surrounding `[data-ctx]` element opens it at the pointer.
	import { HugeiconsIcon } from '@hugeicons/svelte';
	import {
		MoreHorizontalIcon,
		MoreVerticalIcon,
		PinIcon,
		PinOffIcon,
		Radio02Icon,
		ArrowUpNarrowWideIcon,
		ArrowDownWideNarrowIcon,
		BookmarkMinus02Icon,
		DashboardSquare02Icon,
		Share08Icon
	} from '@hugeicons/core-free-icons';
	import * as api from '$lib/api';
	import type { BrowseItem } from '$lib/api';
	import { enqueueItem } from '$lib/browse';
	import { anchorMenu, ctxHost, fitMenu, NO_ANCHOR, toBody } from '$lib/menu';
	import {
		addPick,
		auth,
		isSaved,
		isSynced,
		openShare,
		personal,
		removePick,
		startRadio,
		toast,
		togglePin,
		toggleSaved
	} from '$lib/player.svelte';
	import { t } from '$lib/i18n.svelte';

	let {
		item,
		showPin = true,
		vertical = false,
		iconClass = 'h-4 w-4',
		// The ⋯ is always discoverable; host-specific classes control geometry, never visibility.
		triggerClass = 'absolute right-1 top-1/2 flex h-7 w-7 -translate-y-1/2 cursor-pointer items-center justify-center rounded-md text-muted-foreground transition hover:bg-sidebar-accent hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring'
	}: {
		item: BrowseItem;
		showPin?: boolean;
		vertical?: boolean;
		iconClass?: string;
		triggerClass?: string;
	} = $props();

	const pinned = $derived(personal.pins.includes(item.id));
	const isPick = $derived(personal.picks.some((p) => p.id === item.id));
	// A synced row is on the account too, and dropping only the local copy would leave the card on
	// screen with a "removed" toast under it. Signed out, the local copy is the whole library again.
	const savedHere = $derived(
		isSaved(item.id) && !(auth.account?.signedIn && isSynced(item.id))
	);
	// Radio and Share both need a YouTube item behind them: local folders and the locally-built
	// On Repeat have none.
	const onYouTube = $derived(
		!api.isLocalId(item.id) &&
			!api.isLocalPlaylistId(item.id) &&
			!api.isSmartPlaylistId(item.id)
	);
	// An artist isn't a track list — there's nothing unambiguous to queue. Songs, albums and
	// playlists (local ones included) all are.
	const canQueue = $derived(item.kind === 'song' || item.kind === 'album' || item.kind === 'playlist');

	// The tracks have to be fetched before anything can be queued, so the menu stays open and the
	// row shows it's working. Guards a second click from queueing the album twice.
	let queueing = $state(false);
	async function queue(next: boolean) {
		if (queueing) return;
		queueing = true;
		try {
			await enqueueItem(item, next);
			menuOpen = false;
		} finally {
			queueing = false;
		}
	}

	let menuOpen = $state(false);
	let anchor = $state(NO_ANCHOR);

	// Click on the ⋯ opens under the button; right-click on the host card or row opens at the pointer.
	function openMenu(e: Event) {
		e.preventDefault(); // a right-click must not also raise WebKit's own menu
		e.stopPropagation();
		anchor = anchorMenu(e, { align: 'right' });
		menuOpen = true;
	}
	// stopPropagation everywhere: the trigger sits over a clickable host (a card's whole surface is a
	// play/navigate target), so its click must not reach the host's handler. The popup itself now
	// lives at <body> and no longer bubbles into the host, but these stay: the trigger needs them.
	function run(e: MouseEvent, action?: () => void) {
		e.stopPropagation();
		menuOpen = false;
		action?.();
	}
	// Right-clicking off the menu dismisses it, same as a left click.
	function close(e: MouseEvent) {
		e.preventDefault();
		e.stopPropagation();
		menuOpen = false;
	}
</script>

<button
	class="ryo-action-menu-trigger {triggerClass} {menuOpen ? 'opacity-100' : ''}"
	onclick={openMenu}
	aria-label={t('Item options')}
	{@attach ctxHost(openMenu)}
>
	
	<HugeiconsIcon
		icon={MoreHorizontalIcon}
		altIcon={MoreVerticalIcon}
		showAlt={vertical}
		class={iconClass}
	/>
</button>

{#if menuOpen}
	<button
		class="fixed inset-0 z-40 cursor-default"
		onclick={close}
		oncontextmenu={close}
		aria-label={t('Close menu')}
		{@attach toBody}
	></button>
	<div
		class="ryo-float-menu fixed z-50 min-w-48 p-1"
		style={anchor.style}
		{@attach toBody}
		{@attach fitMenu(anchor)}
	>
		{#if showPin}
			<button
				class="flex w-full cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm hover:bg-accent/10"
				onclick={(e) => run(e, () => togglePin(item.id))}
			>
				<HugeiconsIcon icon={pinned ? PinOffIcon : PinIcon} class="h-4 w-4" />
				{t(pinned ? 'Unpin' : 'Pin to top')}
			</button>
		{/if}
		{#if canQueue}
			<button
				class="flex w-full cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm hover:bg-accent/10 disabled:opacity-50"
				disabled={queueing}
				onclick={(e) => {
					e.stopPropagation();
					queue(true);
				}}
			>
				<HugeiconsIcon icon={ArrowUpNarrowWideIcon} class="h-4 w-4" /> {t('Play next')}
			</button>
			<button
				class="flex w-full cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm hover:bg-accent/10 disabled:opacity-50"
				disabled={queueing}
				onclick={(e) => {
					e.stopPropagation();
					queue(false);
				}}
			>
				<HugeiconsIcon icon={ArrowDownWideNarrowIcon} class="h-4 w-4" /> {t('Add to queue')}
			</button>
		{/if}
		{#if onYouTube}
			<button
				class="flex w-full cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm hover:bg-accent/10"
				onclick={(e) => run(e, () => startRadio(item.kind as 'artist' | 'album' | 'playlist', item.id, item.title))}
			>
				<HugeiconsIcon icon={Radio02Icon} class="h-4 w-4" /> {t('Start radio')}
			</button>
		{/if}
		<button
			class="flex w-full cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm hover:bg-accent/10"
			onclick={(e) => run(e, () => (isPick ? removePick(item.id) : addPick(item)))}
		>
			<HugeiconsIcon icon={DashboardSquare02Icon} class="h-4 w-4" />
			{t(isPick ? 'Remove from shortcuts' : 'Add to shortcuts')}
		</button>
		{#if onYouTube}
			<button
				class="flex w-full cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm hover:bg-accent/10"
				onclick={(e) => run(e, () => openShare(item))}
			>
				<HugeiconsIcon icon={Share08Icon} class="h-4 w-4" /> {t('Share')}
			</button>
		{/if}
		
		{#if savedHere}
			<button
				class="flex w-full cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm hover:bg-accent/10"
				onclick={(e) =>
					run(e, () => {
						toggleSaved(item);
						toast.success(t('Removed from library'));
					})}
			>
				<HugeiconsIcon icon={BookmarkMinus02Icon} class="h-4 w-4" /> {t('Remove from library')}
			</button>
		{/if}
	</div>
{/if}
