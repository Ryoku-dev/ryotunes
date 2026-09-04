<script lang="ts">
	import { MOD } from '$lib/shortcuts';
	import { goto } from '$app/navigation';
	import { HugeiconsIcon } from '@hugeicons/svelte';
	import { Search01Icon } from '@hugeicons/core-free-icons';
	import SearchSuggest from '$lib/components/SearchSuggest.svelte';
	import RyokuMusicDeck from '$lib/components/RyokuMusicDeck.svelte';
	import { auth } from '$lib/player.svelte';
	import { t } from '$lib/i18n.svelte';

	const hour = new Date().getHours();
	const daypart = hour < 5 ? t('Good night') : hour < 12 ? t('Good morning') : hour < 18 ? t('Good afternoon') : t('Good evening');
	let searchQuery = $state('');
	function accountFirstName(name?: string | null) {
		const raw = name?.trim().split(/\s+/)[0] ?? '';
		if (!raw) return '';
		if (raw.length > 20) return `${raw.slice(0, 19)}…`;
		return raw === raw.toUpperCase() && /[A-Z]/.test(raw)
			? raw.charAt(0) + raw.slice(1).toLowerCase()
			: raw;
	}
	const firstName = $derived(accountFirstName(auth.account?.name));

	function goSearch() {
		const q = searchQuery.trim();
		if (!q) return;
		goto(`/search?${new URLSearchParams({ q }).toString()}`);
	}
</script>

<section class="ryo-home-head ryo-home-header">
	<div class="ryo-home-copy ryo-home-intro">
		<div class="ryo-eyebrow ryo-home-eyebrow">
			<span class="ryo-eyebrow-rule"></span>
			<span class="ryo-jp-mark">力</span>
			<span>{t('HOME / LISTEN')}</span>
			<i></i>
			<b>01</b>
		</div>

		<h1 class="ryo-page-title ryo-home-greeting">{daypart}{firstName ? `, ${firstName}` : ''}</h1>
		<p class="ryo-page-caption ryo-home-caption">{t('Pick up where you left off, or find the next thing worth hearing.')}</p>

		<form class="ryo-home-search ryo-home-searchbox" onsubmit={(e) => { e.preventDefault(); goSearch(); }}>
			<HugeiconsIcon icon={Search01Icon} class="ryo-search-icon" />
			<SearchSuggest
				bind:value={searchQuery}
				placeholder="Search tracks, albums, artists…"
				inputClass="ryo-search-input"
				panelClass="ryo-home-search-panel left-0"
			/>
		</form>

		<div class="ryo-home-shortcuts" aria-label={t('Keyboard shortcuts')}>
			<span><kbd>{MOD}K</kbd> {t('command search')}</span>
			<i></i>
			<span><kbd>SPACE</kbd> {t('play / pause')}</span>
		</div>
	</div>

	<div class="ryo-home-deck-wrap">
		<RyokuMusicDeck />
	</div>
</section>
