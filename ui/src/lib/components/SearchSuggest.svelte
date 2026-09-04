<script lang="ts">
	// The search field plus its typeahead preview: type, wait briefly, get a handful of real results
	// under the input. Runs the same `search_all` the search page runs and writes the same page-cache
	// key, so submitting a previewed query paints from cache instead of searching twice.
	//
	// Must live inside a <form>: Enter with nothing highlighted, and the "All results" row, fall
	// through to that form's onsubmit, which is where each caller decides what a full search means
	// (run it in place, or navigate to /search).
	import { HugeiconsIcon } from '@hugeicons/svelte';
	import { Search01Icon, MusicNote01Icon, UserIcon } from '@hugeicons/core-free-icons';
	import { Input } from '$lib/components/ui/input';
	import ExplicitIcon from './ExplicitIcon.svelte';
	import type { BrowseItem } from '$lib/api';
	import { asSong, openItem, searchPreviewPage } from '$lib/browse';
	import { MOD } from '$lib/shortcuts';
	import { thumb } from '$lib/thumb';
	import { openAddToPlaylist } from '$lib/player.svelte';
	import TrackMenu from './TrackMenu.svelte';
	import { createSearchPager, nextSearchPage, searchPagerDone, type SearchPagerState } from '$lib/search-pager';
	import { ownNestedVerticalScroll } from '$lib/ryoku-scroll';
	import { t } from '$lib/i18n.svelte';

	let {
		value = $bindable(''),
		placeholder = 'Search',
		inputClass = '',
		/** Panel geometry. Default matches the field; a narrow field wants its own width. */
		panelClass = 'left-0 right-0',
		onpick
	}: {
		value?: string;
		placeholder?: string;
		inputClass?: string;
		panelClass?: string;
		/** Fired after a row is taken (played or navigated) — for callers that dismiss themselves. */
		onpick?: () => void;
	} = $props();

	let open = $state(false);
	let items = $state<BrowseItem[]>([]);
	let loading = $state(false);
	let loadingMore = $state(false);
	let pager = $state<SearchPagerState>(createSearchPager());
	let active = $state(-1); // keyboard-highlighted row, -1 = none (Enter submits the form)
	let loadedFor = ''; // query `items` belongs to, so a stale response can't land
	let debounce: ReturnType<typeof setTimeout> | undefined;

	const KIND = { song: t('Song'), album: t('Album'), artist: t('Artist'), playlist: t('Playlist') };

	async function load(q: string) {
		loadedFor = q;
		active = -1;
		loading = true;
		loadingMore = false;
		try {
			const first = await searchPreviewPage(q);
			if (loadedFor === q) {
				items = first.items;
				pager = createSearchPager({ mixedContinuation: first.continuation });
			}
		} catch {
			if (loadedFor === q) items = [];
		} finally {
			if (loadedFor === q) loading = false;
		}
	}

	async function loadMore() {
		const q = loadedFor;
		if (!q || loading || loadingMore || searchPagerDone(pager)) return;
		loadingMore = true;
		try {
			const batch = await nextSearchPage(q, pager);
			if (loadedFor !== q) return;
			const seen = new Set(items.map((item) => `${item.kind}:${item.id}`));
			const unique = batch.filter((item) => {
				const key = `${item.kind}:${item.id}`;
				if (seen.has(key)) return false;
				seen.add(key);
				return true;
			});
			items = [...items, ...unique];
		} catch {
			// The full-search row stays available even if one continuation request fails.
		} finally {
			if (loadedFor === q) loadingMore = false;
		}
	}

	function panelScroll(e: Event) {
		const el = e.currentTarget as HTMLElement;
		if (el.scrollTop + el.clientHeight >= el.scrollHeight - 96) void loadMore();
	}

	// Reads the element, not `value`: the binding lands on this same event and the order of the two
	// listeners is not ours to assume.
	function onType(e: Event & { currentTarget: HTMLInputElement }) {
		clearTimeout(debounce);
		const q = e.currentTarget.value.trim().replace(/\s+/g, ' ');
		if (q.length < 2) {
			close();
			return;
		}
		open = true;
		if (q !== loadedFor) {
			// Loading starts now, not when the timer fires: otherwise the empty panel reads as
			// "no results" for the whole debounce, on every query.
			items = [];
			loading = true;
		}
		debounce = setTimeout(() => load(q), 150);
	}

	function close() {
		clearTimeout(debounce);
		open = false;
		loading = false;
		loadingMore = false;
		active = -1;
	}

	function submitAll(e: MouseEvent & { currentTarget: HTMLButtonElement }) {
		e.preventDefault();
		const form = e.currentTarget.form;
		close();
		// Closing removes the dropdown. requestSubmit on the captured form makes navigation explicit
		// instead of relying on the default click after its submit button has left the DOM.
		form?.requestSubmit();
	}

	function choose(item: BrowseItem) {
		close();
		openItem(item); // a song plays, everything else opens its page
		onpick?.();
	}

	function onKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape') {
			e.preventDefault();
			e.stopPropagation();
			if (open) close();
			else (e.currentTarget as HTMLElement).blur();
		} else if (e.key === 'Enter') {
			// Only a highlighted row is ours; a bare Enter is the caller's form submit.
			if (active >= 0 && items[active]) {
				e.preventDefault();
				choose(items[active]);
			} else {
				close();
			}
		} else if ((e.key === 'ArrowDown' || e.key === 'ArrowUp') && items.length) {
			e.preventDefault();
			open = true;
			const n = items.length;
			active = e.key === 'ArrowDown' ? (active + 1) % n : (active <= 0 ? n : active) - 1;
		}
	}
</script>


<div
	class="relative w-full min-w-0"
	onfocusout={(e) => {
		if (!e.currentTarget.contains(e.relatedTarget as Node | null)) close();
	}}
>
	<Input
		data-ryo-escape-owner
		bind:value
		placeholder={t(placeholder)}
		class="ryo-unified-search-input pr-16 {inputClass}"
		autocomplete="off"
		role="combobox"
		aria-expanded={open}
		aria-controls="search-suggest"
		oninput={onType}
		onkeydown={onKeydown}
		onfocus={() => {
			if (items.length && value.trim() === loadedFor) open = true;
		}}
	/>
	
	{#if !value}
		<kbd
			class="pointer-events-none absolute right-2.5 top-1/2 -translate-y-1/2 rounded border bg-muted px-1.5 py-0.5 font-mono text-[0.625rem] font-medium tracking-wide text-muted-foreground"
		>
			{MOD}K
		</kbd>
	{/if}
	{#if open}
		<div
			id="search-suggest"
			role="listbox"
			aria-label={t('Search preview')}
			data-ryo-own-scroll
			class="ryo-search-suggest absolute top-full z-[80] mt-2 overflow-y-auto border {panelClass}"
			onscroll={panelScroll}
			{@attach ownNestedVerticalScroll}
		>
			{#if loading && !items.length}
				<div class="ryo-typeahead-resolver" aria-live="polite">
					<div class="ryo-typeahead-resolver-head"><span>{t('// SEARCH / RESOLVE')}</span><b>{t('FETCHING')}</b></div>
					<div class="ryo-typeahead-resolver-meta"><span>{t('QUERY')}</span><strong>{value.trim()}</strong><span>{t('SOURCE')}</span><strong>YOUTUBE MUSIC</strong></div>
					<div class="ryo-typeahead-resolver-lines" aria-hidden="true">
						{#each [84, 67, 92, 58] as width, i (i)}<i style={`--w:${width}%`}><b>{String(i + 1).padStart(2, '0')}</b></i>{/each}
					</div>
				</div>
			{:else if !items.length}
				<div class="px-4 py-3 text-sm text-muted-foreground">{t('Nothing quick for that.')}</div>
			{:else}
				{#each items as item, i (item.id)}
					{@const hero = i === 0}
					<div
						role="option"
						tabindex="-1"
						aria-selected={i === active}
						data-ctx={item.kind === 'song' ? 'track' : undefined}
						class="ryo-search-row group/searchrow relative flex w-full items-center {i === active ? 'ryo-search-row-active' : ''} {hero ? 'border-b' : ''}"
						onmouseenter={() => (active = i)}
					>
						<button
							type="button"
							class="flex min-w-0 flex-1 cursor-pointer items-center gap-3 px-3 pr-10 text-left {hero ? 'py-2.5' : 'py-1.5'}"
							onmousedown={(e) => e.preventDefault()}
							onclick={() => choose(item)}
						>
							{#if item.thumbnail}
								<img src={thumb(item.thumbnail, hero ? 128 : 96)} alt="" loading="lazy" decoding="async" class="shrink-0 object-cover {item.kind === 'artist' ? 'rounded-full' : 'rounded-md'} {hero ? 'h-12 w-12' : 'h-10 w-10'}" />
							{:else}
								<div class="flex shrink-0 items-center justify-center bg-muted text-muted-foreground/50 {item.kind === 'artist' ? 'rounded-full' : 'rounded-md'} {hero ? 'h-12 w-12' : 'h-10 w-10'}">
									<HugeiconsIcon icon={item.kind === 'artist' ? UserIcon : MusicNote01Icon} class="h-5 w-5" />
								</div>
							{/if}
							<div class="min-w-0 flex-1">
								<div class="truncate {hero ? 'font-semibold' : 'text-sm'}">{item.title}</div>
								<div class="flex items-center gap-1 text-xs text-muted-foreground">
									{#if item.explicit}<ExplicitIcon class="h-3 w-3 shrink-0" />{/if}
									<span class="truncate">{KIND[item.kind]}{item.subtitle ? ` • ${item.subtitle}` : ''}</span>
								</div>
							</div>
							{#if hero}<span class="shrink-0 rounded-full bg-primary/10 px-2 py-0.5 text-[0.625rem] font-semibold uppercase tracking-wide text-primary">{t('Top result')}</span>{/if}
						</button>
						{#if item.kind === 'song'}
							<TrackMenu
								song={asSong(item)}
								onAdd={() => openAddToPlaylist(asSong(item))}
								triggerClass="absolute right-2 top-1/2 -translate-y-1/2 rounded-md p-1.5 text-muted-foreground transition hover:bg-muted hover:text-foreground"
							/>
						{/if}
					</div>
				{/each}
			{/if}
			{#if loadingMore}
				<div class="ryo-search-more-state" role="status">{t('// LOADING MORE RESULTS')}</div>
			{:else if items.length && searchPagerDone(pager)}
				<div class="ryo-search-more-state" data-end>{t('// END OF QUICK RESULTS')}</div>
			{/if}
			
			<button
				type="submit"
				class="ryo-search-all flex w-full cursor-pointer items-center gap-2 border-t px-3 py-2 text-left text-xs font-medium"
				onmousedown={(e) => e.preventDefault()}
				onmouseenter={() => (active = -1)}
				onclick={submitAll}
			>
				<HugeiconsIcon icon={Search01Icon} class="h-3.5 w-3.5" />
				{t('All results for')} “{value.trim()}”
			</button>
		</div>
	{/if}
</div>
