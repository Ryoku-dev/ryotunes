<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { HugeiconsIcon } from '@hugeicons/svelte';
	import { MusicNote01Icon } from '@hugeicons/core-free-icons';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import { Button } from '$lib/components/ui/button';
	import MediaCardSkeleton from '$lib/components/MediaCardSkeleton.svelte';
	import ErrorState from '$lib/components/ErrorState.svelte';
	import HomeHero from '$lib/components/HomeHero.svelte';
	import Shortcuts from '$lib/components/Shortcuts.svelte';
	import RecentRail from '$lib/components/RecentRail.svelte';
	import Shelf from '$lib/components/Shelf.svelte';
	import ForgottenFavourites from '$lib/components/ForgottenFavourites.svelte';
	import ArtistIndex from '$lib/components/ArtistIndex.svelte';
	import HomeLayoutDialog from '$lib/components/HomeLayoutDialog.svelte';
	import * as api from '$lib/api';
	import type { BrowseItem, HomeChip, HomePage, HomeSection } from '$lib/api';
	import {
		auth,
		library,
		noteHomeSections,
		personal,
		playback,
		seedOnRepeatPick,
		toast
	} from '$lib/player.svelte';
	import {
		arrangeSections,
		freshen,
		hiddenSections,
		recentItems
	} from '$lib/personal';
	import { getCached, putCached } from '$lib/pagecache';
	import { appearance } from '$lib/theme.svelte';
	import { buildHomeRegistry, HOME_LOCAL_SECTIONS, homeSectionKey, homeSectionTitle, unsupportedHomeSection } from '$lib/home-sections';
	import { t } from '$lib/i18n.svelte';

	const FORGOTTEN_KEY = 'home:forgotten';

	let home = $state<HomePage | null>(null);
	let loading = $state(true);
	let error = $state<string | null>(null);
	// The mood chips + which one is active. Kept out of `home` so the row survives a filter switch's
	// loading state (every home response carries the same chips anyway). Ryotunes is music-only.
	let chips = $state<HomeChip[]>([]);
	let selected = $state<string | null>(null);
	let loadingMore = $state(false);
	let moreError = $state(false);
	// Anything already on the Shortcuts grid is dropped: a shortcut is something you play, so the two
	// lists otherwise converge on the same handful of items and the top of home shows them twice in
	// two different shapes. Recents earn their space by being what Shortcuts *isn't*. Nine survivors
	// = three full columns; the window is generous because most of it gets filtered away.
	const pinned = $derived(new Set(personal.picks.map((p) => p.id)));
	// Same snapshot problem as the Shortcuts tiles: the stored card is what it looked like when it
	// was last played from, so the live library row wins where there is one (#67).
	const recent = $derived(
		recentItems(personal, 100)
			.filter((r) => !pinned.has(r.id))
			.slice(0, 9)
			.map((r) => freshen(r, library.items))
	);

	// Outlined at rest, filled with the accent when on. Grey-on-grey pills that go black when
	// selected are YouTube Music's chip row exactly, and they carry no colour of the app at all;
	// this way the one active filter is the only saturated thing above the feed.
	const chipClass = (active: boolean) =>
		`shrink-0 cursor-pointer rounded-md border px-3.5 py-1.5 text-sm font-medium transition-colors ${
			active
				? 'border-primary bg-primary text-primary-foreground'
				: 'border-border text-muted-foreground hover:border-foreground/25 hover:text-foreground'
		}`;

	// "Forgotten favourites" is pulled out of the feed and rendered as a list above it (see the
	// markup) — the shelf's cards say nothing about a song, and this one is meant to be read.
	// Songs only: if YouTube ever fills that shelf with something else, it stays a normal card row.
	const isForgotten = (s: HomeSection) =>
		/forgotten/i.test(s.title) && s.items.some((i) => i.kind === 'song');
	// Held separately from `home`, not derived from it: YouTube sends the shelf a page or two into the
	// feed, so it survives the revalidating `home = fresh` that drops back to page one, and a revisit
	// reads it from the cache instead of walking continuations again.
	let forgotten = $state<HomeSection | null>(null);
	const feed = $derived(home?.sections.filter((s) => !isForgotten(s) && !unsupportedHomeSection(s.title)) ?? []);
	const knownArtistNames = $derived(Object.values(personal.artists).map((entry) => entry.name).filter(Boolean));

	// --- the arrangement the user set in the Edit modal (personal.ts) ---------------------------
	// The two sections the app builds itself get reserved keys — a YouTube shelf title can't start
	// with "@" — so they keep their slot even before (or without) any content to show.
	const RECENT = HOME_LOCAL_SECTIONS[0].key;
	const FAMILIAR = HOME_LOCAL_SECTIONS[1].key;
	const FORGOTTEN = HOME_LOCAL_SECTIONS[2].key;
	type Block =
		| { id: string; key: string; title: string; shelf?: undefined }
		| { id: string; key: string; title: string; shelf: HomeSection };
	let editing = $state(false);
	const hidden = $derived(hiddenSections(personal));
	/**
	 * Every section home can show, in the user's order, hidden ones included — the modal lists those
	 * to offer them back. Shelves are keyed on their title (all YouTube gives us that survives a
	 * restart) but rendered under a positional id, because a feed walked far enough does repeat one.
	 */
	const blocks = $derived.by(() => {
		const local: Block[] = selected
			? [] // a mood feed is the chip's: neither of ours belongs in it
			: HOME_LOCAL_SECTIONS.map((entry) => ({ id: entry.key, key: entry.key, title: entry.title }));
		const shelves = feed.map((section, i) => ({
			id: `${i}:${homeSectionKey(section)}`,
			key: homeSectionKey(section),
			title: homeSectionTitle(section, knownArtistNames),
			shelf: section
		}));
		return arrangeSections([...local, ...shelves], personal);
	});
	const visible = $derived(blocks.filter((b) => !hidden.has(b.key)));
	/**
	 * What the Edit modal lists. Not `blocks`: the feed arrives a page at a time, so `blocks` holds
	 * only the shelves scrolled to so far, and the modal showed five rows before a scroll and
	 * fifteen after one. Every shelf home has ever rendered is remembered (`noteSections`), and the
	 * ones this visit hasn't fetched yet are listed alongside the loaded ones — a section can be
	 * hidden or moved before the page has got to it, which is the whole point of the modal.
	 *
	 * Kept apart from `blocks` deliberately: these carry no shelf, so they must never reach the
	 * feed's renderer. Unranked ones sort to the end, since where they belong is exactly what
	 * hasn't loaded.
	 */
	const known = $derived.by(() => {
		if (selected) return blocks.map((b) => ({ ...b, available: true }));
		const registry = buildHomeRegistry(feed, personal.home.seen, knownArtistNames);
		const blockByKey = new Map(blocks.map((block) => [block.key, block]));
		// Keep the registry in product/feed order here. The editor applies the saved personal order
		// only to its working copy, so “Reset default” can genuinely return to the authoritative
		// Home order rather than resetting to the user's already-customised order.
		return registry.map((entry) => ({
			...(blockByKey.get(entry.key) ?? { id: `registry:${entry.key}`, key: entry.key, title: entry.title }),
			title: entry.title,
			available: entry.available
		}));
	});

	// Every page of the feed adds to that memory. Only the unfiltered feed: a mood chip's shelves
	// belong to the chip, not to home's arrangement.
	$effect(() => {
		if (selected) return;
		const titles = feed.map((s) => homeSectionKey(s)).filter(Boolean);
		if (titles.length) noteHomeSections(titles);
	});

	/** Latch the shelf whenever a page turns out to carry it. Called after every `home` change. */
	function noteForgotten() {
		const found = home?.sections.find(isForgotten);
		if (found) {
			forgotten = found;
			putCached(FORGOTTEN_KEY, found);
		}
		return !!found;
	}

	function showMore(section: { title: string; moreBrowseId?: string; moreParams?: string }) {
		const q = new URLSearchParams({ id: section.moreBrowseId!, title: section.title });
		if (section.moreParams) q.set('params', section.moreParams);
		goto(`/list?${q.toString()}`);
	}

	async function load(params: string | null = selected) {
		selected = params;
		const key = params ? `home:${params}` : 'home';
		const hit = getCached<HomePage>(key);
		forgotten = params ? null : getCached<HomeSection>(FORGOTTEN_KEY);
		if (hit) {
			home = hit;
			loading = false;
			error = null;
			noteForgotten();
			// Session cache is deliberately authoritative for revisits. Revalidating page one while the
			// user scrolls caused visible shelf replacement/reshuffling; continuation remains append-only.
			return;
		}
		loading = true;
		error = null;
		try {
			const fresh = await api.getHome(params ?? undefined);
			// A stale response from a chip the user already clicked away from must not win.
			if (selected !== params) return;
			home = fresh;
			putCached(key, fresh);
			noteForgotten();
		} catch (e) {
			if (!hit) error = String(e);
		} finally {
			loading = false;
		}
	}

	async function loadMore() {
		const token = home?.continuation;
		if (!token || loadingMore) return;
		loadingMore = true;
		moreError = false;
		const params = selected; // guard against chip switches mid-flight
		try {
			const more = await api.getHomeMore(token);
			if (selected !== params || home?.continuation !== token) return; // stale
			home = {
				...home!,
				sections: [...home!.sections, ...more.sections],
				// An empty page would leave the sentinel in view with nothing to show — treat it as the end.
				continuation: more.sections.length ? more.continuation : undefined
			};
			noteForgotten();
		} catch (e) {
			// Stop auto-loading and offer a retry — auto-retrying a visible sentinel would spin.
			moreError = true;
			toast.error(t('Could not load more'));
		} finally {
			loadingMore = false;
		}
	}

	// Home uses the layout's <main> as its scroll container. The observer only marks active
	// scrolling and handles shallow-window resize correction.
	function watchScroll(node: HTMLElement) {
		const el = node.closest('main');
		if (!el) return;
		let settle: number | undefined;
		const onScroll = () => {
			el.classList.add('ryo-is-scrolling');
			if (settle) window.clearTimeout(settle);
			settle = window.setTimeout(() => el.classList.remove('ryo-is-scrolling'), 110);
		};


		/*
		 * Precision touchpads stay completely native.  The app-level WebKitGTK safety net in
		 * `initPrecisionScrollFallback()` only intervenes one frame later when native scrolling
		 * genuinely made no progress.  Do not preventDefault() here: doing so used to steal
		 * two-finger gestures that began over artwork, buttons, or horizontal shelves.
		 */

		// Hyprland's Super+A changes the tiled/floating geometry in one compositor transaction.
		// WebKit can preserve a small pre-resize scroll offset while the Home hero becomes shorter,
		// which makes the greeting/search appear to vanish even though they are merely above the new
		// viewport. If the user was still in the Home header region, resize returns them to its top; a
		// deliberate deep scroll is left untouched.
		let lastWidth = el.clientWidth;
		let lastHeight = el.clientHeight;
		const resize = new ResizeObserver(() => {
			const changed = el.clientWidth !== lastWidth || el.clientHeight !== lastHeight;
			lastWidth = el.clientWidth;
			lastHeight = el.clientHeight;
			if (changed && el.scrollTop > 0 && el.scrollTop < 320) el.scrollTop = 0;
		});

		el.addEventListener('scroll', onScroll, { passive: true });
		resize.observe(el);
		return () => {
			el.removeEventListener('scroll', onScroll);
			resize.disconnect();
			if (settle) window.clearTimeout(settle);
			el.classList.remove('ryo-is-scrolling');
		};
	}

	// One page per approach to the bottom: the observer only fires when the sentinel *enters* view, so
	// an appended page that pushes it back out is required before the next fetch. rootMargin starts
	// the fetch early enough that the content is usually there by the time you scroll to it.
	function sentinel(node: HTMLElement) {
		// Low Resource mode keeps pagination demand-driven instead of performing network work just
		// because a large/short window happens to put the sentinel near the viewport.
		if (appearance.lowResourceMode) return () => {};
		const io = new IntersectionObserver(([e]) => e.isIntersecting && loadMore(), {
			rootMargin: '400px 0px'
		});
		io.observe(node);
		return () => io.disconnect();
	}

	// Chips only refresh when a response actually carries them (never blank the row mid-switch).
	$effect(() => {
		if (home?.chips?.length) chips = home.chips.filter((c) => c.title !== 'Podcasts');
	});

	onMount(() => load(null));

	// On Repeat crosses its threshold while you listen, so re-check on every track change rather
	// than once per visit: sitting on home through your fifth song should be enough to see the tile.
	// The check is a local SQLite read, and `seedPick` is what actually decides.
	$effect(() => {
		playback.now?.videoId;
		seedOnRepeatPick();
	});
</script>

<div class="ryo-home-page" {@attach watchScroll}>
	<HomeHero />
	
	{#if chips.length}
		<div class="ryo-toolbar sticky top-0 z-20 border-b bg-background px-6 pt-2.5">
			<div class="ryo-chip-rail flex gap-2 overflow-x-auto pb-2">
				
				<button onclick={() => load(null)} class={chipClass(!selected)}>{t('All')}</button>
				{#each chips as chip (chip.params)}
					<button
						onclick={() => load(selected === chip.params ? null : chip.params)}
						class={chipClass(selected === chip.params)}
					>
						{chip.title}
					</button>
				{/each}
			</div>
		</div>
	{:else if loading}
		
		<div class="ryo-toolbar sticky top-0 z-20 border-b bg-background px-6 pt-2.5" aria-hidden="true">
			<div class="flex gap-2 overflow-hidden pb-2">
				{#each ['w-10', 'w-16', 'w-20', 'w-14', 'w-24', 'w-16'] as w, i (i)}
					<Skeleton class="h-8 shrink-0 rounded-full {w}" />
				{/each}
			</div>
		</div>
	{/if}
	<div class="ryo-home-body px-6 pb-6 pt-6">
		
		{#if !selected}
			<div class="mb-10 border-b pb-8">
				<Shortcuts onEdit={() => (editing = true)} />
			</div>
		{/if}
		{#snippet shelfSkeletons(n: number)}
			{#each Array(n) as _, s (s)}
				<section aria-hidden="true">
					<Skeleton class="mb-3 h-5 w-40 rounded" />
					<div class="flex gap-2 overflow-hidden pb-2">
						{#each Array(6) as _, i (i)}
							<div class="w-40 shrink-0"><MediaCardSkeleton /></div>
						{/each}
					</div>
				</section>
			{/each}
		{/snippet}
		
		<div class="content-in flex flex-col gap-10">
			{#each visible as block (block.id)}
				{#if block.shelf}
					<Shelf
						title={block.title}
						items={block.shelf.items}
						community={/community/i.test(block.shelf.title)}
						onMore={block.shelf.moreBrowseId ? () => showMore(block.shelf!) : undefined}
					/>
				{:else if block.key === RECENT}
					{#if recent.length}<RecentRail items={recent} />{/if}
				{:else if block.key === FAMILIAR}
					<ArtistIndex />
				{:else if forgotten}
					<ForgottenFavourites
						section={forgotten}
						onMore={forgotten.moreBrowseId ? () => showMore(forgotten!) : undefined}
					/>
				{/if}
			{/each}
			{#if loading}
				{@render shelfSkeletons(3)}
			{:else if error}
				<ErrorState message={error} onRetry={() => load(selected)} />
			{:else if !home?.sections.length}
				
				<div class="flex flex-col items-center gap-3 py-20 text-center">
					<HugeiconsIcon icon={MusicNote01Icon} class="h-8 w-8 text-muted-foreground/40" />
					<p class="max-w-sm text-sm text-muted-foreground">
						{t(auth.account?.signedIn
							? 'Your home feed came back empty this time.'
							: 'Sign in and home fills up with mixes and playlists built from what you listen to.')}
					</p>
					{#if auth.account?.signedIn}
						<Button variant="outline" size="sm" onclick={() => load(selected)}>{t('Try again')}</Button>
					{:else}
						<Button size="sm" onclick={() => api.loginWebview()}>{t('Sign in with Google')}</Button>
					{/if}
				</div>
			{:else if home.continuation}
				{#if moreError || appearance.lowResourceMode}
					<div class="p-3 text-center">
						<Button variant="outline" size="sm" onclick={loadMore} disabled={loadingMore}>
							{t(loadingMore ? 'Loading…' : moreError ? 'Try again' : 'Load more')}
						</Button>
					</div>
				{:else}
					<div class="flex flex-col gap-10" aria-busy={loadingMore}>
						<div {@attach sentinel}></div>
						{#if loadingMore}{@render shelfSkeletons(2)}{/if}
					</div>
				{/if}
			{/if}
		</div>
	</div>
</div>

<HomeLayoutDialog bind:open={editing} sections={known} />
