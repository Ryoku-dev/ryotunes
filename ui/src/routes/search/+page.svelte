<script module lang="ts">
	import type { BrowseItem as ModuleBrowseItem } from '$lib/api';
	import type { SearchPagerState as ModuleSearchPagerState } from '$lib/search-pager';

	// Survives remounts (module scope), so coming back to /search — from a result you clicked, or
	// from the sidebar — shows the last search instead of a blank page. The results themselves come
	// back from the page cache, so the rerun paints instantly and just revalidates.
	let lastQuery = '';
	let lastActiveResultId = '';
	let lastQuickLimit = 12;
	let lastQuickScroll = 0;
	let lastExtraQuick: ModuleBrowseItem[] = [];
	let lastQuickPager: ModuleSearchPagerState | null = null;
</script>

<script lang="ts">
	import { onMount, tick } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { HugeiconsIcon } from '@hugeicons/svelte';
	import { Search01Icon, PlayIcon, ArrowRight01Icon, Cancel01Icon } from '@hugeicons/core-free-icons';
	import { Button } from '$lib/components/ui/button';
	import SearchSuggest from '$lib/components/SearchSuggest.svelte';
	import RyokuPageHeader from '$lib/components/RyokuPageHeader.svelte';
	import TrackRow from '$lib/components/TrackRow.svelte';
	import TrackMenu from '$lib/components/TrackMenu.svelte';
	import PlaylistMenu from '$lib/components/PlaylistMenu.svelte';
	import ErrorState from '$lib/components/ErrorState.svelte';
	import Shelf from '$lib/components/Shelf.svelte';
	import * as api from '$lib/api';
	import type { SearchResults, SongItem, BrowseItem } from '$lib/api';
	import { getCached, putCached } from '$lib/pagecache';
	import { openAddToPlaylist, playSong, personal, library } from '$lib/player.svelte';
	import { asSong, openItem, playItem } from '$lib/browse';
	import { thumb } from '$lib/thumb';
	import { recentItems, freshen } from '$lib/personal';
	import { cloneSearchPager, createSearchPager, nextSearchPage, searchPagerDone, type SearchPagerState } from '$lib/search-pager';
	import { ownNestedVerticalScroll } from '$lib/ryoku-scroll';
	import { t } from '$lib/i18n.svelte';

	type Cached = { res: SearchResults; songs: SongItem[]; songContinuation?: string };

	let query = $state(lastQuery);
	let res = $state<SearchResults | null>(null);
	// The Songs shelf comes from the songs-filtered search, not from `res.songs`: an unfiltered
	// response gives a song row either its artist or its length, never both, so those rows land
	// duration-less. The filtered endpoint returns "Artist • Album • 3:58" on every row.
	let songs = $state<SongItem[]>([]);
	let searched = $state('');
	let searching = $state(false);
	let error = $state<string | null>(null);
	let searchHistory = $state<string[]>([]);
	let extraQuick = $state<BrowseItem[]>(lastExtraQuick);
	let quickLimit = $state(lastQuickLimit);
	let quickLoading = $state(false);
	let quickPager = $state<SearchPagerState>(lastQuickPager ? cloneSearchPager(lastQuickPager) : createSearchPager());
	let quickListEl: HTMLElement | undefined = $state();
	let restoredQuickScroll = $state(false);
	const QUICK_BATCH = 12;

	// The query of the most recent runSearch call, so an older in-flight one can't clobber it.
	let latest = '';
	let mounted = false;
	let debounceTimer: ReturnType<typeof setTimeout> | undefined;

	const recentListening = $derived(
		recentItems(personal, 12).slice(0, 5).map((item) => freshen(item, library.items))
	);

	function rememberQuery(q: string) {
		const value = q.trim();
		if (!value) return;
		searchHistory = [value, ...searchHistory.filter((x) => x.toLowerCase() !== value.toLowerCase())].slice(0, 6);
		try { localStorage.setItem('ryotunes_search_history', JSON.stringify(searchHistory)); } catch {}
	}

	function clearHistory() {
		searchHistory = [];
		try { localStorage.removeItem('ryotunes_search_history'); } catch {}
	}

	function searchAgain(value: string) {
		query = value;
		runSearch();
	}

	async function runSearch() {
		if (!query.trim()) return;
		const q = query.trim().replace(/\s+/g, ' ');
		if (!q) return;
		const queryChanged = q !== lastQuery;
		if (queryChanged) {
			extraQuick = [];
			lastExtraQuick = [];
			quickLimit = QUICK_BATCH;
			quickPager = createSearchPager();
			lastQuickPager = null;
			restoredQuickScroll = false;
			activeResultId = '';
			lastActiveResultId = '';
			lastQuickScroll = 0;
		}
		latest = q;
		lastQuery = q;
		rememberQuery(q);
		const key = `search:${q}`;
		const hit = getCached<Cached>(key);
		if (hit) {
			res = hit.res;
			songs = hit.songs;
			searched = q;
			searching = false;
			if (queryChanged || !lastQuickPager) {
				quickPager = createSearchPager({
					mixedContinuation: hit.res.continuation,
					songContinuation: hit.songContinuation,
					songsStarted: !!hit.songs.length
				});
			}
		} else {
			searching = true;
		}
		error = null;
		try {
			// One mixed page + one songs page gives the inspector rich metadata immediately. Every
			// continuation after this is demand-driven by the result pane, one request at a time.
			const [fresh, freshSongs] = await Promise.all([
				api.searchAll(q),
				api.searchPage(q).catch(() => ({ items: [] as SongItem[], continuation: undefined }))
			]);
			if (latest !== q) return;
			res = fresh;
			songs = freshSongs.items;
			searched = q;
			quickPager = createSearchPager({
				mixedContinuation: fresh.continuation,
				songContinuation: freshSongs.continuation,
				songsStarted: true
			});
			lastQuickPager = cloneSearchPager(quickPager);
			putCached(key, { res: fresh, songs: freshSongs.items, songContinuation: freshSongs.continuation });
		} catch (e) {
			if (latest !== q) return;
			if (!hit) error = String(e);
		} finally {
			if (latest === q) searching = false;
		}
	}

	function showMore(cat: 'songs' | 'albums' | 'artists' | 'playlists') {
		goto(`/search-more?${new URLSearchParams({ q: searched, cat }).toString()}`);
	}

	// Run the search when arriving with a ?q= (e.g. from the Home search box). Keyed on the URL
	// alone: typing a new query in the field must not look like a URL change and bounce us back.
	const urlQuery = $derived(page.url.searchParams.get('q') ?? '');
	let lastUrlQuery = '';
	$effect(() => {
		if (urlQuery && urlQuery !== lastUrlQuery) {
			lastUrlQuery = urlQuery;
			query = urlQuery;
			runSearch();
		}
	});

	// Arriving without a ?q= (back from a result, or the sidebar link): rerun whatever was last
	// searched. onMount, not the effect above, so a ?q= arrival still wins.
	onMount(() => {
		mounted = true;
		try {
			const saved = JSON.parse(localStorage.getItem('ryotunes_search_history') ?? '[]');
			if (Array.isArray(saved)) searchHistory = saved.filter((x): x is string => typeof x === 'string').slice(0, 6);
		} catch {}
		if (!urlQuery && query) runSearch();
		return () => {
			clearTimeout(debounceTimer);
			lastActiveResultId = activeResultId;
			lastQuickLimit = quickLimit;
			lastExtraQuick = extraQuick;
			lastQuickPager = cloneSearchPager(quickPager);
			lastQuickScroll = quickListEl?.scrollTop ?? lastQuickScroll;
		};
	});

	// Daily-driver search: pause briefly after typing, then search automatically. Submission still
	// works immediately, and emptying the field returns to the useful recent-search state.
	$effect(() => {
		if (!mounted) return;
		const value = query.trim();
		clearTimeout(debounceTimer);
		if (!value || value === searched || value === urlQuery) return;
		debounceTimer = setTimeout(() => runSearch(), 160);
	});

	const songRows = $derived((songs.length ? songs : (res?.songs ?? []).map(asSong)).filter((song) => !song.is_video));

	// The search page is an instrument, not a stack of shelves. Keep a small working set pinned
	// above the categories: keyboard / hover changes the inspector, Enter performs the natural action.
	let activeResultId = $state(lastActiveResultId);
	const songCards = $derived(songRows.map((song): BrowseItem => ({
		kind: 'song', id: song.video_id, title: song.title, subtitle: song.artists,
		thumbnail: song.thumbnail, duration: song.duration, playCount: song.play_count,
		artistRuns: song.artist_runs, explicit: song.explicit, isUpload: song.is_upload
	})));
	const quickPool = $derived.by(() => {
		if (!res) return [];
		const out: BrowseItem[] = [];
		const seen = new Set<string>();
		for (const item of [...res.top, ...songCards, ...res.songs, ...res.albums, ...res.artists, ...res.playlists, ...extraQuick]) {
			const key = `${item.kind}:${item.id}`;
			if (seen.has(key)) continue;
			seen.add(key);
			out.push(item);
		}
		return out;
	});
	const quickItems = $derived(quickPool.slice(0, quickLimit));
	const quickDone = $derived(searchPagerDone(quickPager) && quickLimit >= quickPool.length);
	const activeResult = $derived(quickItems.find((i) => i.id === activeResultId) ?? quickItems[0] ?? null);
	$effect(() => {
		if (quickItems.length && !quickItems.some((i) => i.id === activeResultId)) activeResultId = quickItems[0].id;
		lastActiveResultId = activeResultId;
		lastQuickLimit = quickLimit;
		lastExtraQuick = extraQuick;
		lastQuickPager = cloneSearchPager(quickPager);
	});

	async function loadNextQuickPage() {
		if (!res || quickLoading || quickDone) return;
		if (quickLimit < quickPool.length) {
			quickLimit = Math.min(quickPool.length, quickLimit + QUICK_BATCH);
			return;
		}
		quickLoading = true;
		const q = searched;
		try {
			const batch = await nextSearchPage(q, quickPager);
			if (searched !== q) return;
			const seen = new Set(quickPool.map((item) => `${item.kind}:${item.id}`));
			const unique = batch.filter((item) => {
				const key = `${item.kind}:${item.id}`;
				if (seen.has(key)) return false;
				seen.add(key);
				return true;
			});
			extraQuick = [...extraQuick, ...unique];
			quickLimit = Math.min(quickPool.length + unique.length, quickLimit + QUICK_BATCH);
			lastQuickPager = cloneSearchPager(quickPager);
		} catch {
			// Fail soft: the existing working set stays usable and the next scroll can retry.
		} finally {
			quickLoading = false;
		}
	}

	function quickScroll(e: Event) {
		const el = e.currentTarget as HTMLElement;
		lastQuickScroll = el.scrollTop;
		if (el.scrollTop + el.clientHeight >= el.scrollHeight - 120) void loadNextQuickPage();
	}

	async function moveResult(delta: number) {
		if (!quickItems.length) return;
		let at = Math.max(0, quickItems.findIndex((i) => i.id === activeResult?.id));
		if (delta > 0 && at === quickItems.length - 1 && !quickDone) {
			await loadNextQuickPage();
			await tick();
			at = Math.max(0, quickItems.findIndex((i) => i.id === activeResult?.id));
		}
		const next = Math.max(0, Math.min(quickItems.length - 1, at + delta));
		activeResultId = quickItems[next].id;
		requestAnimationFrame(() => {
			const row = Array.from(quickListEl?.querySelectorAll<HTMLElement>('[data-search-id]') ?? [])
				.find((node) => node.dataset.searchId === activeResultId);
			row?.scrollIntoView({ block: 'nearest' });
		});
	}
	function naturalAction(item = activeResult) {
		if (!item) return;
		if (item.kind === 'song') {
			void playSong(asSong(item));
			return;
		}
		openItem(item);
	}
	async function playSelection(item = activeResult) {
		if (!item || item.kind === 'artist') return;
		await playItem(item);
	}
	function searchKeys(e: KeyboardEvent) {
		if (e.ctrlKey || e.metaKey || e.altKey) return;
		const target = e.target as HTMLElement | null;
		if (target?.matches('input,textarea,select,[contenteditable=true]')) return;
		if (!quickItems.length) return;
		if (e.key === 'ArrowDown' || e.key === 'j' || e.key === 'J') { e.preventDefault(); void moveResult(1); }
		else if (e.key === 'ArrowUp' || e.key === 'k' || e.key === 'K') { e.preventDefault(); void moveResult(-1); }
		else if (e.key === 'Enter') { e.preventDefault(); naturalAction(); }
	}


	// Restore only once after the asynchronous result list exists. Doing this in onMount races the
	// cached/network result paint and silently loses the old position on most back navigations.
	$effect(() => {
		if (restoredQuickScroll || !quickListEl || !quickItems.length || !lastQuickScroll) return;
		restoredQuickScroll = true;
		requestAnimationFrame(() => {
			if (!quickListEl) return;
			quickListEl.scrollTop = Math.min(lastQuickScroll, Math.max(0, quickListEl.scrollHeight - quickListEl.clientHeight));
		});
	});

	// Sections are horizontal card rows, except Songs which is a vertical list. `top` has no "show more".
	const sections = $derived(
		res
			? [
					{ key: 'top', label: 'Top results', items: res.top, max: 4, more: false, list: false },
					{ key: 'songs', label: 'Songs', items: res.songs, max: 6, more: true, list: true },
					{ key: 'albums', label: 'Albums', items: res.albums, max: 5, more: true, list: false },
					{ key: 'artists', label: 'Artists', items: res.artists, max: 3, more: true, list: false },
					{ key: 'playlists', label: 'Playlists', items: res.playlists, max: 5, more: true, list: false }
				].filter((s) => (s.list ? songRows.length : s.items.length))
			: []
	);
	const searchReadout = $derived([
		`QUERY|${searched ? searched.slice(0, 18).toUpperCase() : 'READY'}`,
		`SONGS|${songRows.length}`,
		`SETS|${sections.length}`
	]);

</script>

<svelte:window onkeydown={searchKeys} />

<div class="ryo-route-page flex h-full flex-col">
	<RyokuPageHeader
		eyebrow="MUSIC / DISCOVERY"
		title={t('Search')}
		blurb={t('Tune into a track, artist, album or playlist — one discovery index, built for keyboard flow.')}
		artMode="search"
		code="SEARCH · INDEX"
		artTitle="検索"
		artSub="DISCOVERY"
		tate="音を探す"
		seal="探"
		readout={searchReadout}
	/>
	<div class="ryo-page-toolbar">
		<form
			class="ryo-search-page-form"
			onsubmit={(e) => {
				e.preventDefault();
				runSearch();
			}}
		>
			<SearchSuggest
				bind:value={query}
				placeholder={t('Search songs, albums, artists, playlists…')}
				onpick={() => (lastQuery = query)}
				inputClass="ryo-search-input"
				panelClass="left-0 w-[32rem]"
			/>
			{#if query}
				<button type="button" class="ryo-search-clear" aria-label={t('Clear search')} title={t('Clear search')} onclick={() => { query = ''; res = null; songs = []; searched = ''; error = null; }}><HugeiconsIcon icon={Cancel01Icon} class="h-4 w-4" /></button>
			{/if}
			<Button type="submit" variant="outline" class="ryo-sheet-action gap-2" disabled={searching}>
				<HugeiconsIcon icon={Search01Icon} class="h-4 w-4" />
				{t(searching ? 'SEARCHING' : 'SEARCH')}
			</Button>
		</form>
		{#if error}<div class="mt-2"><ErrorState message={error} onRetry={runSearch} /></div>{/if}
	</div>

	<div class="ryo-page-scroll min-h-0 flex-1 overflow-y-auto">
		{#if searching}
			<section class="ryo-search-scanning" aria-live="polite">
				<div class="ryo-search-scanning-copy">
					<span>{t('// DISCOVERY / QUERY')}</span>
					<strong>{query}</strong>
					<p>{t('Resolving songs, artists, albums and playlists in parallel.')}</p>
					<div><b>INDEX</b><em>YOUTUBE MUSIC</em><b>STATE</b><em>SEARCHING</em></div>
				</div>
				<div class="ryo-search-scanning-field" aria-hidden="true">
					<div class="ryo-search-scanning-mark">検索</div>
					{#each Array(9) as _, i (i)}<i style="--w:{48 + ((i * 17) % 46)}%"></i>{/each}
					<span></span>
				</div>
			</section>
		{:else if !res}
			<section class="ryo-search-idle ryo-search-index" aria-label={t('Search start page')}>
				<div class="ryo-search-idle-column">
					<div class="ryo-search-idle-head"><span>{t('// RECENT SEARCHES')}</span>{#if searchHistory.length}<button type="button" onclick={clearHistory}>{t('CLEAR')}</button>{/if}</div>
					{#if searchHistory.length}
						<div class="ryo-search-history">
							{#each searchHistory as value, i (value)}
								<button type="button" onclick={() => searchAgain(value)}><b>{String(i + 1).padStart(2, '0')}</b><span>{value}</span><em>↗</em></button>
							{/each}
						</div>
					{:else}
						<div class="ryo-search-idle-empty"><strong>{t('No search history yet.')}</strong><p>{t('Use the field above or press')} <kbd>Ctrl K</kbd> {t('from anywhere.')}</p></div>
					{/if}
				</div>

				<div class="ryo-search-idle-column">
					<div class="ryo-search-idle-head"><span>{t('// RECENT LISTENING')}</span><b>{recentListening.length ? `${recentListening.length} ITEMS` : 'READY'}</b></div>
					{#if recentListening.length}
						<div class="ryo-search-recent-listening">
							{#each recentListening as item, i (item.id)}
								<button type="button" onclick={() => openItem(item)}>
									<span>{#if item.thumbnail}<img src={thumb(item.thumbnail, 160)} alt="" loading="lazy" decoding="async" />{:else}音{/if}</span>
									<div><b>{item.title}</b><small>{item.subtitle ?? item.kind}</small></div><em>{String(i + 1).padStart(2, '0')}</em>
								</button>
							{/each}
						</div>
					{:else}
						<div class="ryo-search-index-plate"><span>検索</span><strong>DISCOVERY INDEX</strong><p>{t('Search songs, artists, albums and playlists without leaving the keyboard.')}</p><div><b>/</b> PAGE SEARCH <i>·</i> <b>CTRL K</b> COMMAND</div></div>
					{/if}
				</div>
			</section>
		{:else if !sections.length}
			<div class="ryo-search-none">
				<span>検索</span><div><small>{t('// DISCOVERY / EMPTY')}</small><strong>No results for “{searched}”.</strong><p>{t('Try a shorter title, the artist name, or a different spelling.')}</p><button type="button" onclick={() => { query = ''; res = null; songs = []; searched = ''; }}>{t('CLEAR QUERY')}</button></div>
			</div>
		{:else}
			<div class="content-in flex flex-col gap-8">
				{#if quickItems.length && activeResult}
					<section class="ryo-search-workspace" aria-label={t('Search results inspector')}>
						<div class="ryo-search-quick">
							<div class="ryo-search-workspace-head"><span>{t('// RESULTS')}</span><b>↑↓ / J K · ENTER</b></div>
							<div class="ryo-search-quick-list" role="listbox" aria-label={t('Quick results')} data-ryo-own-scroll bind:this={quickListEl} onscroll={quickScroll} {@attach ownNestedVerticalScroll}>
								{#each quickItems as item, i (item.id)}
									<div
										role="option"
										tabindex="-1"
										aria-selected={activeResult.id === item.id}
										data-search-id={item.id}
										data-ctx={item.kind === 'song' ? 'track' : undefined}
										class="ryo-search-quick-row group/searchquick relative"
										class:active={activeResult.id === item.id}
										onpointerenter={() => (activeResultId = item.id)}
									>
										<button
											type="button"
											class="grid min-w-0 flex-1 grid-cols-[auto_auto_minmax(0,1fr)_auto] items-center gap-2 text-left"
											onfocus={() => (activeResultId = item.id)}
											onclick={() => { activeResultId = item.id; naturalAction(item); }}
										>
											<span class="ryo-search-quick-index">{String(i + 1).padStart(2, '0')}</span>
											<span class="ryo-search-quick-art">{#if item.thumbnail}<img src={thumb(item.thumbnail, 160)} alt="" loading="lazy" decoding="async" />{/if}</span>
											<span class="ryo-search-quick-copy"><strong>{item.title}</strong><small>{item.subtitle ?? item.kind}</small></span>
											<em>{item.kind.toUpperCase()}</em>
										</button>
										{#if item.kind === 'song'}
											<TrackMenu song={asSong(item)} onAdd={() => openAddToPlaylist(asSong(item))} triggerClass="mr-1 rounded-md p-1 text-muted-foreground  transition hover:bg-muted hover:text-foreground" />
										{/if}
									</div>
								{/each}
								<div class="ryo-search-quick-tail" aria-live="polite">
									{#if quickLoading}<span>{t('Loading more…')}</span>{:else if quickDone}<span>{t('No more results')}</span>{:else}<button type="button" onclick={loadNextQuickPage}>{t('Load more')}</button>{/if}
								</div>
							</div>
						</div>

						<aside class="ryo-search-inspector">
							<div class="ryo-search-inspector-art">
								{#if activeResult.thumbnail}<img src={thumb(activeResult.thumbnail, 480)} alt="" decoding="async" />{:else}<span>音</span>{/if}
								<div class="ryo-search-inspector-index">{t('RESULT ·')} {activeResult.kind.toUpperCase()}</div>
							</div>
							<div class="ryo-search-inspector-copy">
								<div class="ryo-search-inspector-eyebrow">{t('// SELECTED RESULT')}</div>
								<h2>{activeResult.title}</h2>
								<p>{activeResult.subtitle ?? activeResult.kind}</p>
								{#if activeResult.duration || activeResult.playCount}
									<div class="ryo-search-inspector-meta">{#if activeResult.duration}<span>{t('DURATION')} <b>{activeResult.duration}</b></span>{/if}{#if activeResult.playCount}<span>{t('PLAYS')} <b>{activeResult.playCount}</b></span>{/if}</div>
								{/if}
								<div class="ryo-search-inspector-actions">
									<button type="button" class="primary" onclick={() => naturalAction(activeResult)}>
										{t(activeResult.kind === 'song' ? 'Play' : 'Open')} <HugeiconsIcon icon={PlayIcon} altIcon={ArrowRight01Icon} showAlt={activeResult.kind !== 'song'} class="h-3.5 w-3.5" />
									</button>
									{#if activeResult.kind === 'song'}
										<TrackMenu song={asSong(activeResult)} onAdd={() => openAddToPlaylist(asSong(activeResult))} triggerClass="ryo-search-inspector-menu" />
									{:else}
										{#if activeResult.kind !== 'artist'}<button type="button" onclick={() => playSelection(activeResult)}><HugeiconsIcon icon={PlayIcon} class="h-3.5 w-3.5" /> {t('Play')}</button>{/if}
										<PlaylistMenu item={activeResult} triggerClass="ryo-search-inspector-menu" />
									{/if}
								</div>
							</div>
						</aside>
					</section>
				{/if}

				{#each sections as sec (sec.key)}
					<section>
						<div class="mb-3 flex items-center justify-between">
							<h2 class="ryo-search-section-title"><span>//</span>{t(sec.label)}</h2>
							{#if sec.more}
								<button
									class="ryo-show-more cursor-pointer"
									onclick={() => showMore(sec.key as 'songs' | 'albums' | 'artists' | 'playlists')}
								>
									{t('Show more')}
								</button>
							{/if}
						</div>
						{#if sec.list}
							{#each songRows.slice(0, sec.max) as song (song.video_id)}
								<TrackRow
									{song}
									showPlayCount
									onplay={() => playSong(song)}
									onAdd={() => openAddToPlaylist(song)}
								/>
							{/each}
						{:else}
							<Shelf items={sec.items.slice(0, sec.max)} />
						{/if}
					</section>
				{/each}
			</div>
		{/if}
	</div>
</div>
