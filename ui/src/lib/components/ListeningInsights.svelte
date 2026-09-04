<script lang="ts">
	import * as api from '$lib/api';
	import { appearance } from '$lib/theme.svelte';
	import { t } from '$lib/i18n.svelte';

	let period = $state<'day' | 'week' | 'month'>('week');
	let stats = $state<api.ListeningStats | null>(null);
	let loading = $state(false);
	let error = $state('');
	let request = 0;

	async function load(next = period) {
		period = next;
		const seq = ++request;
		loading = true;
		error = '';
		try {
			const value = await api.getListeningStats(next);
			if (seq === request) stats = value;
		} catch (e) {
			if (seq === request) error = String(e);
		} finally {
			if (seq === request) loading = false;
		}
	}

	$effect(() => {
		void load('week');
	});

	function duration(seconds = 0) {
		if (seconds < 60) return `${seconds}s`;
		const h = Math.floor(seconds / 3600);
		const m = Math.round((seconds % 3600) / 60);
		return h ? `${h}h ${m}m` : `${m}m`;
	}
</script>

<div class="space-y-4">
	<div class="flex flex-wrap items-center justify-between gap-3 rounded-2xl border border-border bg-muted/20 p-3">
		<div>
			<p class="text-xs font-semibold tracking-[0.16em] text-muted-foreground">{t('LISTENING / LOCAL')}</p>
			<p class="mt-1 text-sm text-foreground/90">{t("Private insights from this device's bounded play history.")}</p>
		</div>
		<div class="flex rounded-xl border border-border bg-muted/25 p-1">
			{#each [['day', 'Day'], ['week', 'Week'], ['month', 'Month']] as item (item[0])}
				<button
					type="button"
					class="rounded-lg px-3 py-1.5 text-xs font-medium transition-colors {period === item[0] ? 'bg-accent/15 text-foreground' : 'text-muted-foreground hover:text-foreground'}"
					onclick={() => load(item[0] as 'day' | 'week' | 'month')}
					>{t(item[1])}</button>
				{/each}
		</div>
	</div>

	{#if loading && !stats}
		<div class="rounded-2xl border border-border p-8 text-center text-sm text-muted-foreground">{t('Reading listening history…')}</div>
	{:else if error}
		<div class="rounded-2xl border border-red-400/15 p-5 text-sm text-muted-foreground">
			<p>{t('Could not build insights.')}</p>
			<button type="button" class="mt-2 text-foreground hover:underline" onclick={() => load()}>{t('Try again')}</button>
		</div>
	{:else if stats}
		<div class="grid gap-3 sm:grid-cols-2">
			<div class="rounded-2xl border border-border bg-muted/15 p-5">
				<span class="text-[11px] tracking-[0.18em] text-muted-foreground">{t('PLAYS')}</span>
				<strong class="mt-2 block font-heading text-4xl text-foreground">{stats.plays}</strong>
				<small class="text-muted-foreground">{t('recorded starts in this period')}</small>
			</div>
			<div class="rounded-2xl border border-border bg-muted/15 p-5">
				<span class="text-[11px] tracking-[0.18em] text-muted-foreground">{t('KNOWN DURATION')}</span>
				<strong class="mt-2 block font-heading text-4xl text-foreground">{duration(stats.knownDurationSeconds)}</strong>
				<small class="text-muted-foreground">{t('approximate from tracks with duration metadata')}</small>
			</div>
		</div>

		<div class="grid gap-4 lg:grid-cols-2">
			<section class="rounded-2xl border border-border bg-muted/15 p-4">
				<h3 class="mb-3 text-sm font-semibold">{t('Top artists')}</h3>
				{#if stats.topArtists.length}
					<div class="space-y-1">
						{#each stats.topArtists as row, i (row.name)}
							<div class="flex items-center gap-3 rounded-xl px-2 py-2 hover:bg-muted/35">
								<span class="w-5 text-right font-mono text-[11px] text-muted-foreground">{String(i + 1).padStart(2, '0')}</span>
								<span class="min-w-0 flex-1 truncate text-sm">{row.name}</span>
								<b class="text-xs text-muted-foreground">{row.plays}</b>
							</div>
						{/each}
					</div>
				{:else}<p class="text-sm text-muted-foreground">{t('Play some music and this fills itself in.')}</p>{/if}
			</section>
			<section class="rounded-2xl border border-border bg-muted/15 p-4">
				<h3 class="mb-3 text-sm font-semibold">{t('Top tracks')}</h3>
				{#if stats.topTracks.length}
					<div class="space-y-1">
						{#each stats.topTracks as row, i}
							<div class="flex items-center gap-3 rounded-xl px-2 py-2 hover:bg-muted/35">
								<span class="w-5 text-right font-mono text-[11px] text-muted-foreground">{String(i + 1).padStart(2, '0')}</span>
								<div class="min-w-0 flex-1"><p class="truncate text-sm">{row.title}</p><small class="block truncate text-muted-foreground">{row.artists}</small></div>
								<b class="text-xs text-muted-foreground">{row.plays}</b>
							</div>
						{/each}
					</div>
				{:else}<p class="text-sm text-muted-foreground">{t('Nothing recorded for this period yet.')}</p>{/if}
			</section>
		</div>
		{#if appearance.lowResourceMode}
			<p class="text-[11px] text-muted-foreground">{t('Low resource mode is active. Insights remain on-demand and run no background animation or polling.')}</p>
		{/if}
	{/if}
</div>
