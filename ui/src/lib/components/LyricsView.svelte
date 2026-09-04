<script module lang="ts">
	const lyricScrollMemory = new Map<string, number>();
</script>

<script lang="ts">
	import * as api from '$lib/api';
	import { playback, setPlaybackPosition } from '$lib/player.svelte';
	import { appearance } from '$lib/theme.svelte';
	import { ryokuWheelScroll } from '$lib/ryoku-scroll';
	import { HugeiconsIcon } from '@hugeicons/svelte';
	import { t } from '$lib/i18n.svelte';
	import { RefreshIcon, MusicNote01Icon } from '@hugeicons/core-free-icons';

	// `expanded` only sizes the type and centres the column. The owner of the extra room (the side
	// panel, or the now-playing view) decides how much there is. Toggling it must not remount this
	// component, or the lyrics refetch and the scroll position is lost.
	// `compact` is the mini-player: a ~220px column with no room for the source footer or a
	// scrollbar. It only shrinks the type and chrome; the sync/auto-scroll logic is identical.
	let { expanded = false, compact = false }: { expanded?: boolean; compact?: boolean } =
		$props();

	/** "3:21" / "1:02:03" → seconds. */
	function durationSecs(d?: string): number | undefined {
		if (!d) return undefined;
		const parts = d.split(':').map(Number);
		if (!parts.length || parts.some(Number.isNaN)) return undefined;
		return parts.reduce((a, b) => a * 60 + b, 0);
	}

	let lyrics = $state<api.Lyrics | null>(null);
	let loading = $state(true);
	let scroller: HTMLElement | undefined = $state();

	// Per-track manual correction for provider timing. Positive = lyrics appear later. Stored locally
	// because it is a listening preference, not YouTube metadata, and capped so stale ids cannot
	// grow localStorage forever.
	const OFFSET_KEY = 'ryotunes:lyrics-offsets-v1';
	let offsetMs = $state(0);
	function readOffsets(): Record<string, number> {
		try {
			const v = JSON.parse(localStorage.getItem(OFFSET_KEY) ?? '{}');
			return v && typeof v === 'object' ? v : {};
		} catch {
			return {};
		}
	}
	function setOffset(next: number) {
		offsetMs = Math.max(-5000, Math.min(5000, Math.round(next / 100) * 100));
		const id = playback.now?.videoId;
		if (!id) return;
		const map = readOffsets();
		if (offsetMs === 0) delete map[id];
		else map[id] = offsetMs;
		const entries = Object.entries(map).slice(-200);
		try { localStorage.setItem(OFFSET_KEY, JSON.stringify(Object.fromEntries(entries))); } catch {}
	}
	function formatOffset(ms: number) {
		if (!ms) return '0.0s';
		return `${ms > 0 ? '+' : ''}${(ms / 1000).toFixed(1)}s`;
	}

	// videoId of the fetch whose result is (or will be) shown — guards stale responses.
	let requested = '';

	async function requestLyrics(force = false) {
		const now = playback.now;
		if (!now) {
			requested = '';
			lyrics = null;
			loading = false;
			return;
		}
		if (!force && now.videoId === requested) return;
		const id = (requested = now.videoId);
		loading = true;
		lyrics = null;
		const album = playback.queue.items[playback.queue.currentIndex]?.album;
		try {
			const l = await api.getLyrics({
				videoId: id,
				title: now.title,
				artists: now.artists,
				album: album ?? undefined,
				duration: durationSecs(now.duration)
			});
			if (requested !== id) return;
			lyrics = l;
			hasScrolled = false;
		} catch {
			if (requested !== id) return;
			lyrics = null;
		} finally {
			if (requested === id) loading = false;
		}
	}

	$effect(() => {
		playback.now?.videoId;
		void requestLyrics();
	});

	$effect(() => {
		const id = playback.now?.videoId;
		offsetMs = id ? Number(readOffsets()[id] ?? 0) : 0;
	});


	// Last synced line whose cue has passed. Build the timed index only when lyrics change, then
	// binary-search it as the local clock advances; a long lyrics document no longer gets linearly
	// rescanned 30 times a second during word-synced playback.
	const timedLines = $derived(
		lyrics?.synced
			? lyrics.lines.flatMap((line, index) =>
					line.time_ms === undefined ? [] : [{ index, time: line.time_ms }]
				)
			: []
	);
	const activeIndex = $derived.by(() => {
		const currentMs = posMs;
		let lo = 0;
		let hi = timedLines.length - 1;
		let answer = -1;
		while (lo <= hi) {
			const mid = (lo + hi) >> 1;
			const cue = timedLines[mid]!;
			if (cue.time <= currentMs) {
				answer = cue.index;
				lo = mid + 1;
			} else {
				hi = mid - 1;
			}
		}
		return answer;
	});

	// Auto-scroll pauses while the user is scrolling (wheel/touch/scrollbar), resumes after 3s.
	// Tracked via input events, not `scroll`, so our own smooth scrolls don't trip it.
	let userScrollUntil = 0;
	let hasScrolled = false;
	let manualScroll = $state(false);
	let manualTimer: ReturnType<typeof setTimeout> | undefined;
	// A panel close/reopen must not throw away the user's manual lyric position. Synced auto-follow
	// takes over again after the normal quiet period, so this does not create a second scroll owner.
	$effect(() => {
		// Compact mini lyrics always follow the active line. Reusing the full lyrics panel's
		// manual scroll memory can reopen the mini with the current cue clipped above the viewport.
		if (compact) return;
		const id = playback.now?.videoId;
		if (!id || !scroller) return;
		const remembered = lyricScrollMemory.get(id);
		if (remembered === undefined) return;
		requestAnimationFrame(() => {
			if (!scroller || playback.now?.videoId !== id) return;
			scroller.scrollTop = remembered;
			userScrollUntil = Date.now() + 5000;
			manualScroll = true;
		});
	});

	function onUserScroll() {
		const id = playback.now?.videoId;
		if (!compact && id && scroller) lyricScrollMemory.set(id, scroller.scrollTop);
		// The mini is glanceable: a short manual pause is enough, then auto-follow resumes.
		const pauseMs = compact ? 1400 : 5000;
		userScrollUntil = Date.now() + pauseMs;
		manualScroll = true;
		clearTimeout(manualTimer);
		manualTimer = setTimeout(() => (manualScroll = false), pauseMs);
	}
	function scrollLineIntoView(index: number, behavior: ScrollBehavior) {
		if (!scroller) return;
		const line = scroller.querySelector<HTMLElement>(`[data-line="${index}"]`);
		if (!line) return;
		const scrollRect = scroller.getBoundingClientRect();
		const lineRect = line.getBoundingClientRect();
		const lineTop = scroller.scrollTop + (lineRect.top - scrollRect.top);
		const top = lineTop - (scroller.clientHeight - lineRect.height) / 2;
		scroller.scrollTo({ top: Math.max(0, top), behavior });
	}

	function returnToCurrent() {
		clearTimeout(manualTimer);
		manualScroll = false;
		userScrollUntil = 0;
		if (activeIndex < 0) return;
		scrollLineIntoView(activeIndex, appearance.lowResourceMode ? 'auto' : 'smooth');
		hasScrolled = true;
	}

	let wasExpanded: boolean | undefined;

	$effect(() => {
		const i = activeIndex;
		// Re-centre after the layout width/font changes, and jump rather than glide across it.
		// (Also fires on the first run, where both values are already at their defaults.)
		if (expanded !== wasExpanded) {
			wasExpanded = expanded;
			hasScrolled = false;
			userScrollUntil = 0;
			manualScroll = false;
		}
		if (i < 0 || !scroller || Date.now() < userScrollUntil) return;
		// Opening mid-song jumps straight to the line; after that, glide.
		scrollLineIntoView(
			i,
			appearance.lowResourceMode ? 'auto' : (hasScrolled ? 'smooth' : 'auto')
		);
		hasScrolled = true;
	});

	function seekTo(line: api.LyricLine) {
		if (line.time_ms === undefined) return;
		const secs = Math.max(0, (line.time_ms + offsetMs) / 1000);
		setPlaybackPosition(secs);
		userScrollUntil = 0; // jump the view along with the seek
		api.seek(secs);
	}

	// Line timing follows the shared ~4 Hz transport. Word timing gets a small local clock only while
	// this lyrics view is mounted and visible; 15 Hz is enough with the short CSS interpolation on
	// the active word and avoids a renderer/compositor wakeup on every animation frame.
	let interpolatedPosSecs = $state(playback.position);
	const hasWordTiming = $derived(
		!!lyrics?.synced && lyrics.lines.some((line) => !!line.words?.length)
	);

	$effect(() => {
		const pos = playback.position;
		const animateWords = hasWordTiming;
		if (playback.paused || !animateWords || document.visibilityState !== 'visible') {
			interpolatedPosSecs = pos;
			return;
		}
		const base = pos;
		const baseAt = performance.now();
		interpolatedPosSecs = pos;
		const interval = compact ? 100 : appearance.lowResourceMode ? 125 : 67;
		const timer = window.setInterval(() => {
			interpolatedPosSecs = base + ((performance.now() - baseAt) / 1000) * playback.speed;
		}, interval);
		return () => window.clearInterval(timer);
	});

	// Offset the cue clock (rather than rewriting provider data) so word timing + translations remain intact.
	const posMs = $derived(interpolatedPosSecs * 1000 - offsetMs);

	function getWordProgress(word: api.LyricWord, currentMs: number): number {
		if (currentMs <= word.start_ms) return 0;
		if (currentMs >= word.end_ms) return 1;
		const dur = word.end_ms - word.start_ms;
		if (dur <= 0) return 1;
		return (currentMs - word.start_ms) / dur;
	}
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -- handlers only detect scroll intent -->
<div
	bind:this={scroller}
	onwheel={onUserScroll}
	ontouchmove={onUserScroll}
	onpointerdown={onUserScroll}
	{@attach ryokuWheelScroll}
	style="background:var(--ryo-paper);color:var(--ryo-ink);"
	class="ryo-lyrics-scroller {compact ? 'ryo-lyrics-scroller-compact' : ''} min-h-0 flex-1 overflow-x-hidden overflow-y-auto {compact
		? 'px-2 [scrollbar-width:none] [&::-webkit-scrollbar]:hidden'
		: expanded
			? 'px-10 py-6'
			: 'px-5 py-6'}"
>
	{#if manualScroll && lyrics?.synced && activeIndex >= 0 && !compact}
		<button type="button" class="ryo-lyrics-return" onclick={returnToCurrent}>{t('CURRENT LINE')}</button>
	{/if}
	{#if loading}
		<div class="ryo-lyrics-resolving" aria-live="polite">
			<div class="ryo-lyrics-resolving-head">
				<span>// RESOLVING LYRICS</span>
				<b>LOCAL → PROVIDERS</b>
			</div>
			<div class="ryo-lyrics-resolving-meta">
				<strong>{playback.now?.title ?? t('CURRENT TRACK')}</strong>
				<small>{playback.now?.artists ?? t('Waiting for metadata')}</small>
			</div>
			<div class="ryo-lyrics-ruled" aria-hidden="true">
				{#each { length: 7 } as _, i (i)}
					<span style="--w:{62 + ((i * 13) % 32)}%"></span>
				{/each}
			</div>
			<div class="ryo-lyrics-resolving-foot"><i></i><span>SEARCHING SYNCED + PLAIN TEXT SOURCES</span></div>
		</div>
	{:else if lyrics?.instrumental}
		<div class="ryo-lyrics-empty">
			<div class="ryo-lyrics-empty-mark"><HugeiconsIcon icon={MusicNote01Icon} class="h-5 w-5" /></div>
			<span>// LYRICS / INSTRUMENTAL</span>
			<strong>{t('Instrumental track.')}</strong>
			<p>{t('There are no vocal lines to follow for this recording.')}</p>
		</div>
	{:else if lyrics && lyrics.synced}
		
		<div class="ryo-lyrics-lines {expanded ? 'mx-auto max-w-3xl' : ''}">
			{#each lyrics.lines as line, i (i)}
				{@const isActive = i === activeIndex}
				{@const isPast = i < activeIndex}
				<button
					data-line={i}
					onclick={() => seekTo(line)}
					class="block w-full origin-left cursor-pointer text-left font-heading font-bold leading-snug transition-[color,transform] duration-[170ms] ease-out hover:text-foreground
						{expanded ? 'py-3 text-3xl' : compact ? 'py-1 text-sm' : 'py-2 text-xl'}
						{isActive
						? 'scale-[1.04] text-foreground'
						: isPast
							? 'text-muted-foreground/40'
							: 'text-muted-foreground/70'}"
				>
					{#if line.words && line.words.length > 0}
						
						<span class="inline-flex flex-wrap items-baseline">
							{#each line.words as word, wIdx (wIdx)}
								{@const isWordEnd = word.text.endsWith(' ')}
								{@const cleanText = word.text.trimEnd()}
								{#if isActive}
									{@const progress = getWordProgress(word, posMs)}
									{@const pct = Math.round(Math.min(1, Math.max(0, progress)) * 100)}
									{@const isCurrentWord = progress > 0 && progress < 1}
									
									<span
										class="inline-block bg-clip-text text-transparent [-webkit-text-fill-color:transparent] transition-transform duration-100 ease-out {isWordEnd ? 'mr-[0.26em]' : ''} {isCurrentWord
											? 'scale-[1.03]'
											: ''}"
										style="background-image: linear-gradient(90deg, var(--foreground) {pct}%, var(--muted-foreground) {pct}%)"
									>
										{cleanText}
									</span>
								{:else}
									<span class="inline-block {isWordEnd ? 'mr-[0.26em]' : ''} {isPast ? 'text-muted-foreground/40' : 'text-muted-foreground/70'}">
										{cleanText}
									</span>
								{/if}
							{/each}
						</span>
					{:else}
						<span>{line.text || '♪'}</span>
					{/if}

					
					{#if line.translation}
						<p class="mt-1 text-sm font-normal italic tracking-wide opacity-80 transition-opacity">
							{line.translation}
						</p>
					{/if}
				</button>
			{/each}
		</div>
	{:else if lyrics}
		<div
			class="space-y-2 leading-relaxed text-foreground/90 {expanded
				? 'mx-auto max-w-3xl text-xl'
				: compact
					? 'text-xs'
					: 'text-[15px]'}"
		>
			{#each lyrics.lines as line, i (i)}
				{#if line.text}
					<div>
						<p>{line.text}</p>
						{#if line.translation}
							<p class="text-xs italic text-muted-foreground">{line.translation}</p>
						{/if}
					</div>
				{:else}
					<div class="h-4"></div>
				{/if}
			{/each}
		</div>
	{:else}
		<div class="ryo-lyrics-empty">
			<div class="ryo-lyrics-empty-mark">詞</div>
			<span>// LYRICS / EMPTY</span>
			<strong>{t('No lyrics found for this track.')}</strong>
			<p>{playback.now?.title ?? t('Current track')} · {playback.now?.artists ?? t('Unknown artist')}</p>
			<div class="ryo-lyrics-empty-readout"><b>SOURCE</b><em>PROVIDER CHAIN</em><b>STATE</b><em>EMPTY</em></div>
			<button type="button" onclick={() => requestLyrics(true)}><HugeiconsIcon icon={RefreshIcon} class="h-3.5 w-3.5" /> {t('Retry lookup')}</button>
		</div>
	{/if}
</div>
{#if lyrics && !loading && !compact}
	<div class="ryo-lyrics-footer flex items-center justify-between gap-3 border-t text-xs text-muted-foreground" style="background:var(--ryo-paper);color:var(--ryo-ink-muted);border-color:var(--ryo-line);">
		<span>{lyrics.source.startsWith('Source:') ? lyrics.source : `Lyrics from ${lyrics.source}`}</span>
		{#if lyrics.synced}
			<div class="ryo-lyrics-timing flex items-center gap-1" title={t('Adjust lyric timing for this track')}>
				<span class="mr-1 opacity-70">{t('Timing')} {formatOffset(offsetMs)}</span>
				<button type="button" class="rounded-md px-1.5 py-0.5 hover:bg-muted hover:text-foreground" onclick={() => setOffset(offsetMs - 500)}>−0.5</button>
				<button type="button" class="rounded-md px-1.5 py-0.5 hover:bg-muted hover:text-foreground" onclick={() => setOffset(0)}>{t('Reset')}</button>
				<button type="button" class="rounded-md px-1.5 py-0.5 hover:bg-muted hover:text-foreground" onclick={() => setOffset(offsetMs + 500)}>+0.5</button>
			</div>
		{/if}
	</div>
{/if}

