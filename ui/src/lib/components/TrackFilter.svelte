<script module lang="ts">
	import type { SongItem } from '$lib/api';
	import { filterSongItems } from '$lib/localsearch';

	/** Shared ranked search over title, artist, album and relevant metadata. */
	export function filterTracks<T extends SongItem>(items: T[], query: string): T[] {
		return filterSongItems(items, query);
	}
</script>

<script lang="ts">
	import { HugeiconsIcon } from '@hugeicons/svelte';
	import { SearchList01Icon, Cancel01Icon } from '@hugeicons/core-free-icons';
	import { t } from '$lib/i18n.svelte';

	let {
		value = $bindable(''),
		placeholder = t('Search this list'),
		compact = false
	}: { value?: string; placeholder?: string; compact?: boolean } = $props();

	let input: HTMLInputElement;
	function clear() {
		value = '';
		queueMicrotask(() => input?.focus());
	}
</script>

<div class="ryo-track-filter" class:compact>
	<HugeiconsIcon icon={SearchList01Icon} strokeWidth={2.2} class="ryo-track-filter-icon" />
	<input
		bind:this={input}
		bind:value
		{placeholder}
		aria-label={placeholder}
		autocomplete="off"
		spellcheck="false"
		class="ryo-track-filter-input" data-ryo-escape-owner
		onkeydown={(e) => {
			if (e.key !== 'Escape') return;
			e.preventDefault();
			e.stopPropagation();
			if (value) clear();
			else input?.blur();
		}}
	/>
	<button
		type="button"
		class="ryo-track-filter-clear"
		class:visible={!!value}
		onclick={clear}
		aria-label={t('Clear search')}
		tabindex={value ? 0 : -1}
	>
		<HugeiconsIcon icon={Cancel01Icon} strokeWidth={2.2} class="h-3.5 w-3.5" />
	</button>
</div>
