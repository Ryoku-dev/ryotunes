<script lang="ts">
	import { onMount } from 'svelte';
	import { HugeiconsIcon } from '@hugeicons/svelte';
	import { MusicNote01Icon, PlayIcon, Search01Icon } from '@hugeicons/core-free-icons';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import RyokuPageHeader from '$lib/components/RyokuPageHeader.svelte';
	import * as api from '$lib/api';
	import type { RadioStation } from '$lib/api';
	import { toast } from '$lib/player.svelte';
	import { t } from '$lib/i18n.svelte';

	const PAGE = 36;
	let input = $state('');
	let query = $state('');
	let stations = $state<RadioStation[]>([]);
	let loading = $state(false);
	let loadingMore = $state(false);
	let error = $state<string | null>(null);
	let hasMore = $state(true);
	let playing = $state('');
	let generation = 0;

	const readout = $derived([
		`STATIONS|${stations.length}`,
		`MODE|${query ? 'SEARCH' : 'TOP'}`,
		'STREAM|LIVE'
	]);

	onMount(() => {
		load(true);
	});

	async function load(reset: boolean) {
		if ((reset && loading) || (!reset && (loadingMore || !hasMore))) return;
		const myGeneration = reset ? ++generation : generation;
		const offset = reset ? 0 : stations.length;
		if (reset) {
			loading = true;
			error = null;
		} else {
			loadingMore = true;
		}
		try {
			const rows = await api.radioStations(query, offset, PAGE);
			if (myGeneration !== generation) return;
			if (reset) {
				stations = rows;
			} else {
				const seen = new Set(stations.map((station) => station.stationUuid));
				stations = [...stations, ...rows.filter((station) => !seen.has(station.stationUuid))];
			}
			hasMore = rows.length === PAGE;
		} catch (e) {
			if (myGeneration === generation) error = String(e);
		} finally {
			if (myGeneration === generation) {
				loading = false;
				loadingMore = false;
			}
		}
	}

	function search() {
		query = input.trim().replace(/\s+/g, ' ');
		load(true);
	}

	function clearSearch() {
		input = '';
		query = '';
		load(true);
	}

	async function playStation(station: RadioStation) {
		if (playing) return;
		playing = station.stationUuid;
		try {
			await api.playRadioStation(station);
			toast.success(`Playing ${station.name}`);
		} catch (e) {
			toast.error(String(e));
		} finally {
			playing = '';
		}
	}

	function detail(station: RadioStation) {
		const parts = [
			station.countryCode || station.country,
			station.codec,
			station.bitrate && station.bitrate > 0 ? `${station.bitrate} kbps` : ''
		].filter(Boolean);
		return parts.join(' · ') || t('Live stream');
	}

	function stationTags(station: RadioStation) {
		return (station.tags ?? '')
			.split(',')
			.map((tag) => tag.trim())
			.filter(Boolean)
			.slice(0, 3);
	}

	async function openHomepage(station: RadioStation) {
		if (!station.homepage) return;
		try {
			await api.openExternal(station.homepage);
		} catch (e) {
			toast.error(String(e));
		}
	}
</script>

<div class="ryo-route-page">
	<RyokuPageHeader
		eyebrow="MUSIC / AIRWAVES"
		title={t('Radio')}
		blurb={t("Live stations from around the world, played through Ryotunes' native audio engine.")}
		code="RADIO · DIRECTORY"
		artTitle="電波"
		artSub="LIVE RADIO"
		tate="世界を聴く"
		seal="波"
		{readout}
	/>

	<section class="radio-console">
		<div class="radio-console-copy">
			<span>{t('// RADIO BROWSER / LIVE DIRECTORY')}</span>
			<strong>{query ? `Results for “${query}”` : t('Popular stations')}</strong>
			<p>{t('Station discovery is demand-driven: Ryotunes only contacts the directory when this page is opened, searched or extended.')}</p>
		</div>
		<form
			class="radio-search"
			onsubmit={(event) => {
				event.preventDefault();
				search();
			}}
		>
			<div class="radio-search-box">
				<HugeiconsIcon icon={Search01Icon} class="h-4 w-4" />
				<Input
					bind:value={input}
					placeholder={t('Search stations by name…')}
					aria-label={t('Search internet radio stations')}
				/>
			</div>
			<Button type="submit" disabled={loading}>{t('Search')}</Button>
			{#if query}
				<Button type="button" variant="outline" onclick={clearSearch} disabled={loading}>{t('Top stations')}</Button>
			{/if}
		</form>
	</section>

	{#if loading}
		<div class="radio-loading" aria-live="polite">
			<span>{t('// TUNING')}</span>
			<strong>{t('Finding live stations.')}</strong>
			<p>{t('Trying available Radio Browser mirrors without blocking the player.')}</p>
		</div>
	{:else if error && !stations.length}
		<div class="radio-error" role="alert">
			<span>{t('// SIGNAL LOST')}</span>
			<strong>{t('Radio directory unavailable.')}</strong>
			<p>{error}</p>
			<Button variant="outline" onclick={() => load(true)}>{t('Try again')}</Button>
		</div>
	{:else if !stations.length}
		<div class="radio-empty">
			<span>{t('// NO MATCH')}</span>
			<strong>{t('No stations found.')}</strong>
			<p>{t('Try a shorter station name or return to the popular directory.')}</p>
			{#if query}<Button variant="outline" onclick={clearSearch}>{t('Top stations')}</Button>{/if}
		</div>
	{:else}
		<div class="radio-grid">
			{#each stations as station (station.stationUuid)}
				<article class="radio-card">
					<div class="radio-art">
						{#if station.favicon}
							<img
								src={station.favicon}
								alt=""
								loading="lazy"
								decoding="async"
								onerror={(event) => ((event.currentTarget as HTMLImageElement).style.display = 'none')}
							/>
						{/if}
						<HugeiconsIcon icon={MusicNote01Icon} class="radio-art-fallback" />
						<span>{t('LIVE')}</span>
					</div>
					<div class="radio-copy">
						<h2 title={station.name}>{station.name}</h2>
						<p>{detail(station)}</p>
						{#if stationTags(station).length}
							<div class="radio-tags" aria-label={t('Station tags')}>
								{#each stationTags(station) as tag}
									<span>{tag}</span>
								{/each}
							</div>
						{/if}
					</div>
					<div class="radio-actions">
						<Button
							size="sm"
							class="gap-2"
							disabled={!!playing}
							onclick={() => playStation(station)}
							aria-label={`Play ${station.name}`}
						>
							<HugeiconsIcon icon={PlayIcon} class="h-4 w-4" />
							{t(playing === station.stationUuid ? 'Tuning…' : 'Play')}
						</Button>
						{#if station.homepage}
							<Button variant="ghost" size="sm" onclick={() => openHomepage(station)}>{t('Site')}</Button>
						{/if}
					</div>
				</article>
			{/each}
		</div>

		<div class="radio-more">
			{#if error}
				<p role="status">{error}</p>
			{/if}
			{#if hasMore}
				<Button variant="outline" disabled={loadingMore} onclick={() => load(false)}>
					{t(loadingMore ? 'Finding more…' : 'Load more stations')}
				</Button>
			{:else}
				<span>{t('// END OF THIS SIGNAL SET')}</span>
			{/if}
		</div>
	{/if}

	<footer class="radio-attribution">
		<span>{t('DIRECTORY')}</span>
		<strong>Radio Browser</strong>
		<p>{t('Station metadata and live stream endpoints are provided by the community-run Radio Browser network.')}</p>
		<Button variant="ghost" size="sm" onclick={() => api.openExternal('https://www.radio-browser.info/')}>
			{t('About the directory')}
		</Button>
	</footer>
</div>

<style>
	.radio-console {
		display: grid;
		grid-template-columns: minmax(0, 1fr) minmax(22rem, 0.8fr);
		gap: 1.5rem;
		align-items: end;
		padding: 1.1rem 1.25rem;
		margin-bottom: 1.25rem;
		border: 1px solid hsl(var(--border));
		background: hsl(var(--card));
	}
	.radio-console-copy {
		min-width: 0;
	}
	.radio-console-copy > span,
	.radio-loading > span,
	.radio-error > span,
	.radio-empty > span,
	.radio-attribution > span,
	.radio-more > span {
		display: block;
		margin-bottom: 0.4rem;
		font-family: var(--font-mono);
		font-size: 0.68rem;
		letter-spacing: 0.12em;
		color: hsl(var(--muted-foreground));
	}
	.radio-console-copy strong,
	.radio-loading strong,
	.radio-error strong,
	.radio-empty strong,
	.radio-attribution strong {
		display: block;
		font-family: var(--font-heading);
		font-size: 1.1rem;
	}
	.radio-console-copy p,
	.radio-loading p,
	.radio-error p,
	.radio-empty p,
	.radio-attribution p {
		margin-top: 0.35rem;
		font-size: 0.78rem;
		line-height: 1.55;
		color: hsl(var(--muted-foreground));
	}
	.radio-search {
		display: flex;
		gap: 0.55rem;
		align-items: center;
	}
	.radio-search-box {
		display: flex;
		min-width: 0;
		flex: 1;
		align-items: center;
		gap: 0.45rem;
		padding-left: 0.65rem;
		border: 1px solid hsl(var(--border));
		background: hsl(var(--background));
	}
	.radio-search-box :global(input) {
		border: 0;
		box-shadow: none;
		background: transparent;
	}
	.radio-grid {
		display: grid;
		grid-template-columns: repeat(3, minmax(0, 1fr));
		gap: 0.75rem;
	}
	.radio-card {
		display: grid;
		grid-template-columns: 4.75rem minmax(0, 1fr);
		grid-template-areas:
			"art copy"
			"art actions";
		gap: 0.55rem 0.8rem;
		min-width: 0;
		padding: 0.8rem;
		border: 1px solid hsl(var(--border));
		background: hsl(var(--card));
	}
	.radio-art {
		grid-area: art;
		position: relative;
		display: flex;
		width: 4.75rem;
		height: 4.75rem;
		align-items: center;
		justify-content: center;
		overflow: hidden;
		border: 1px solid hsl(var(--border));
		background: hsl(var(--muted));
	}
	.radio-art img {
		position: absolute;
		inset: 0;
		z-index: 1;
		width: 100%;
		height: 100%;
		object-fit: cover;
		background: hsl(var(--muted));
	}
	:global(.radio-art-fallback) {
		width: 1.25rem;
		height: 1.25rem;
		color: hsl(var(--muted-foreground));
	}
	.radio-art > span {
		position: absolute;
		z-index: 2;
		right: 0.25rem;
		bottom: 0.25rem;
		padding: 0.12rem 0.25rem;
		background: hsl(var(--background) / 0.9);
		font-family: var(--font-mono);
		font-size: 0.55rem;
		letter-spacing: 0.12em;
	}
	.radio-copy {
		grid-area: copy;
		min-width: 0;
	}
	.radio-copy h2 {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		font-size: 0.9rem;
		font-weight: 650;
	}
	.radio-copy p {
		margin-top: 0.18rem;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		font-size: 0.7rem;
		color: hsl(var(--muted-foreground));
	}
	.radio-tags {
		display: flex;
		gap: 0.3rem;
		margin-top: 0.45rem;
		overflow: hidden;
	}
	.radio-tags span {
		max-width: 7.5rem;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		padding: 0.14rem 0.35rem;
		border: 1px solid hsl(var(--border));
		font-size: 0.61rem;
		color: hsl(var(--muted-foreground));
	}
	.radio-actions {
		grid-area: actions;
		display: flex;
		gap: 0.35rem;
		align-items: end;
	}
	.radio-loading,
	.radio-error,
	.radio-empty {
		padding: 2rem 1.25rem;
		border: 1px solid hsl(var(--border));
		background: hsl(var(--card));
	}
	.radio-error :global(button),
	.radio-empty :global(button) {
		margin-top: 0.9rem;
	}
	.radio-more {
		display: flex;
		min-height: 4.5rem;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		gap: 0.5rem;
	}
	.radio-more p {
		font-size: 0.72rem;
		color: hsl(var(--destructive));
	}
	.radio-attribution {
		display: grid;
		grid-template-columns: auto auto minmax(0, 1fr) auto;
		gap: 0.75rem;
		align-items: center;
		margin-top: 0.5rem;
		padding: 0.8rem 1rem;
		border-top: 1px solid hsl(var(--border));
	}
	.radio-attribution > span {
		margin: 0;
	}
	.radio-attribution > p {
		margin: 0;
	}
	@media (max-width: 1280px) {
		.radio-grid {
			grid-template-columns: repeat(2, minmax(0, 1fr));
		}
	}
	@media (max-width: 960px) {
		.radio-console {
			grid-template-columns: 1fr;
		}
		.radio-grid {
			grid-template-columns: 1fr;
		}
		.radio-attribution {
			grid-template-columns: 1fr;
		}
	}
</style>
