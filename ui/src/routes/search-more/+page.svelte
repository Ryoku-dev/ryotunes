<script lang="ts">
	import { page } from '$app/state';
	import TrackRow from '$lib/components/TrackRow.svelte';
	import TrackRowSkeleton from '$lib/components/TrackRowSkeleton.svelte';
	import MediaCard from '$lib/components/MediaCard.svelte';
	import MediaCardSkeleton from '$lib/components/MediaCardSkeleton.svelte';
	import ErrorState from '$lib/components/ErrorState.svelte';
	import * as api from '$lib/api';
	import type { BrowseItem, SongItem } from '$lib/api';
	import { openAddToPlaylist, playSong } from '$lib/player.svelte';
	import { getCached, putCached } from '$lib/pagecache';
	import { t } from '$lib/i18n.svelte';

	type MoreResult = { songs: SongItem[]; cards: BrowseItem[]; continuation?: string };

	let songs = $state<SongItem[]>([]);
	let cards = $state<BrowseItem[]>([]);
	let continuation = $state<string | undefined>();
	let loading = $state(true);
	let loadingMore = $state(false);
	let error = $state<string | null>(null);
	let loadedKey = '';

	const q = $derived(page.url.searchParams.get('q') ?? '');
	const cat = $derived(page.url.searchParams.get('cat') ?? 'songs');
	const label = $derived(t({ songs: 'Songs', albums: 'Albums', artists: 'Artists', playlists: 'Playlists' }[cat] ?? 'Results'));
	const done = $derived(!continuation);

	function uniqueSongs(base: SongItem[], next: SongItem[]) {
		const seen = new Set(base.map((song) => song.video_id));
		return [...base, ...next.filter((song) => {
			if (song.is_video || seen.has(song.video_id)) return false;
			seen.add(song.video_id);
			return true;
		})];
	}
	function uniqueCards(base: BrowseItem[], next: BrowseItem[]) {
		const seen = new Set(base.map((item) => `${item.kind}:${item.id}`));
		return [...base, ...next.filter((item) => {
			const key = `${item.kind}:${item.id}`;
			if (seen.has(key)) return false;
			seen.add(key);
			return true;
		})];
	}

	async function load(query: string, category: string) {
		const key = `searchmore:${category}:${query}`;
		loadedKey = key;
		const hit = getCached<MoreResult>(key);
		if (hit) {
			songs = hit.songs;
			cards = hit.cards;
			continuation = hit.continuation;
			loading = false;
		} else {
			loading = true;
			songs = [];
			cards = [];
			continuation = undefined;
		}
		error = null;
		try {
			let fresh: MoreResult;
			if (category === 'songs') {
				const result = await api.searchPage(query);
				fresh = { songs: result.items.filter((song) => !song.is_video), cards: [], continuation: result.continuation };
			} else {
				const result = await api.searchCardsPage(query, category as 'albums' | 'artists' | 'playlists');
				fresh = { songs: [], cards: result.items, continuation: result.continuation };
			}
			if (query !== q || category !== cat || loadedKey !== key) return;
			songs = fresh.songs;
			cards = fresh.cards;
			continuation = fresh.continuation;
			putCached(key, fresh);
		} catch (e) {
			if (query !== q || category !== cat || loadedKey !== key) return;
			if (!hit) error = String(e);
		} finally {
			if (query === q && category === cat && loadedKey === key) loading = false;
		}
	}

	async function loadMore() {
		const token = continuation;
		if (!token || loading || loadingMore || !q) return;
		const key = loadedKey;
		loadingMore = true;
		try {
			if (cat === 'songs') {
				const next = await api.searchPageMore(token);
				if (loadedKey !== key) return;
				songs = uniqueSongs(songs, next.items);
				continuation = next.continuation;
			} else {
				const next = await api.searchCardsMore(token);
				if (loadedKey !== key) return;
				cards = uniqueCards(cards, next.items);
				continuation = next.continuation;
			}
			putCached(key, { songs, cards, continuation });
		} catch (e) {
			if (loadedKey === key) error = String(e);
		} finally {
			if (loadedKey === key) loadingMore = false;
		}
	}

	function moreSentinel(el: HTMLElement) {
		const observer = new IntersectionObserver((entries) => {
			if (entries.some((entry) => entry.isIntersecting)) void loadMore();
		}, { rootMargin: '600px 0px' });
		observer.observe(el);
		return () => observer.disconnect();
	}

	$effect(() => {
		if (q) void load(q, cat);
	});
</script>

<div class="p-6">
	<h1 class="mb-1 font-heading text-2xl font-bold">{label}</h1>
	<p class="mb-6 text-sm text-muted-foreground">{t('Results for')} “{q}”</p>

	{#if loading}
		{#if cat === 'songs'}
			{#each Array(10) as _, i (i)}<TrackRowSkeleton />{/each}
		{:else}
			<div class="card-grid">{#each Array(12) as _, i (i)}<MediaCardSkeleton />{/each}</div>
		{/if}
	{:else if error && !songs.length && !cards.length}
		<ErrorState message={error} onRetry={() => load(q, cat)} />
	{:else if cat === 'songs'}
		<div class="content-in">
			{#each songs as song (song.video_id)}
				<TrackRow {song} showPlayCount onplay={() => playSong(song)} onAdd={() => openAddToPlaylist(song)} />
			{:else}<p class="text-sm text-muted-foreground">{t('Nothing found.')}</p>{/each}
		</div>
	{:else if cards.length}
		<div class="card-grid content-in">
			{#each cards as item (`${item.kind}:${item.id}`)}<MediaCard {item} />{/each}
		</div>
	{:else}
		<p class="text-sm text-muted-foreground">{t('Nothing found.')}</p>
	{/if}

	{#if !loading && (songs.length || cards.length)}
		<div class="ryo-search-pagination-state" {@attach moreSentinel}>
			{#if loadingMore}// LOADING MORE RESULTS{:else if done}// END OF RESULTS{:else}// MORE RESULTS READY{/if}
		</div>
	{/if}
	{#if error && (songs.length || cards.length)}<div class="mt-3"><ErrorState message={error} onRetry={loadMore} /></div>{/if}
</div>
