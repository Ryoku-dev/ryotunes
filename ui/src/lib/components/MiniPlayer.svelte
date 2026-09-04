<script lang="ts">
	import { HugeiconsIcon } from '@hugeicons/svelte';
	import {
		PreviousIcon,
		NextIcon,
		PlayIcon,
		PauseIcon,
		ShuffleIcon,
		RepeatIcon,
		RepeatOne01Icon,
		FavouriteIcon,
		MaximizeScreenIcon,
		VolumeHighIcon,
		VolumeMute02Icon,
		Queue01Icon,
		Mic01Icon,
		MusicNote01Icon
	} from '@hugeicons/core-free-icons';
	import * as api from '$lib/api';
	import {
		playback,
		setPlaybackPosition,
		commitVolume,
		cycleRepeat,
		dragVolume,
		toggleMute,
		toggleNowPlayingLike,
		wheelVolume
	} from '$lib/player.svelte';
	import { thumb } from '$lib/thumb';
	import { artworkAccent } from '$lib/artcolor';
	import ArtworkImage from './ArtworkImage.svelte';
	import LyricsView from './LyricsView.svelte';
	import QueueList from './QueueList.svelte';
	import Marquee from './Marquee.svelte';
	import { t } from '$lib/i18n.svelte';

	type MiniView = 'now' | 'lyrics' | 'queue';
	let view = $state<MiniView>('now');
	const now = $derived(playback.now);
	const shuffleOn = $derived(playback.queue.shuffle ?? false);
	const repeat = $derived(playback.queue.repeat ?? 'off');
	const likeable = $derived(!!now && !api.isLocalId(now.videoId));
	const next = $derived(playback.queue.items[playback.queue.currentIndex + 1] ?? null);

	// Accent extraction stays one-shot per track and uses a modest thumbnail. Large visible artwork
	// itself goes through ArtworkImage's bounded decode path below.
	const accentArt = $derived(now?.thumbnail ? thumb(now.thumbnail, 192) : '');
	let accent = $state<string | null>(null);
	let accentRun = 0;
	$effect(() => {
		const url = accentArt;
		const run = ++accentRun;
		accent = null;
		if (!url) return;
		artworkAccent(url).then((value) => {
			if (run === accentRun) accent = value;
		});
	});

	let seekDrag = $state<number | null>(null);
	const shownPosition = $derived(seekDrag ?? playback.position);
	const progress = $derived(playback.duration > 0 ? Math.min(100, Math.max(0, shownPosition / playback.duration * 100)) : 0);
	const fmt = (secs: number) => {
		if (!Number.isFinite(secs) || secs <= 0) return '0:00';
		const total = Math.floor(secs);
		return `${Math.floor(total / 60)}:${String(total % 60).padStart(2, '0')}`;
	};
	function onSeekInput(e: Event) { seekDrag = Number((e.currentTarget as HTMLInputElement).value); }
	function onSeekCommit(e: Event) {
		const value = Number((e.currentTarget as HTMLInputElement).value);
		setPlaybackPosition(value);
		seekDrag = null;
		void api.seek(value);
	}

	let volDragging = $state(false);
	let justLiked = $state(false);
	function toggleLike() {
		if (playback.rating !== 'like') justLiked = true;
		toggleNowPlayingLike();
	}
</script>

<svelte:window onpointerup={() => (volDragging = false)} />

<div
	data-tauri-drag-region="deep"
	class="ryo-mini-v2"
	style={accent ? `--mini-accent:${accent}` : undefined}
>
	<div class="ryo-mini-v2-glow" aria-hidden="true"></div>
	<section class="ryo-mini-v2-art" aria-label={t('Current artwork')}>
		<ArtworkImage source={now?.thumbnail} size={480} previewSize={120} className="ryo-mini-v2-art-image" />
		<span class="ryo-mini-v2-live">// LIVE</span>
		<div class="ryo-mini-v2-art-copy">
			<strong>{now?.title ?? t('Nothing playing')}</strong>
			<span>{now?.artists ?? t('Ryotunes is ready')}</span>
		</div>
	</section>

	<section class="ryo-mini-v2-main">
		<header class="ryo-mini-v2-head">
			<div class="ryo-mini-v2-brand"><span class="ryo-mini-v2-rule"></span><b>力 RYOTUNES</b></div>
			<nav class="ryo-mini-v2-tabs" aria-label={t('Mini player view')}>
				<button class:active={view === 'now'} aria-pressed={view === 'now'} aria-label={t('Now playing')} title={t('Now playing')} onclick={() => (view = 'now')}><HugeiconsIcon icon={MusicNote01Icon} class="h-4 w-4" /></button>
				<button class:active={view === 'lyrics'} aria-pressed={view === 'lyrics'} aria-label={t('Lyrics')} title={t('Lyrics')} onclick={() => (view = 'lyrics')}><HugeiconsIcon icon={Mic01Icon} class="h-4 w-4" /></button>
				<button class:active={view === 'queue'} aria-pressed={view === 'queue'} aria-label={t('Queue')} title={t('Queue')} onclick={() => (view = 'queue')}><HugeiconsIcon icon={Queue01Icon} class="h-4 w-4" /></button>
			</nav>
			<div class="ryo-mini-v2-head-actions">
				{#if likeable}
					<button class:active={playback.rating === 'like'} onclick={toggleLike} aria-label={t(playback.rating === 'like' ? 'Remove from liked songs' : 'Like track')}>
						<span class:animate-heart-pop={justLiked} onanimationend={() => (justLiked = false)}><HugeiconsIcon icon={FavouriteIcon} class="h-4 w-4" /></span>
					</button>
				{/if}
				<!-- This is deliberately an explicit “open full app” action. The window manager close\n				     path remains separate in Rust and never calls it. -->
				<button onclick={() => api.closeMini().catch(() => {})} title={t('Open full Ryotunes')} aria-label={t('Open full Ryotunes')}><HugeiconsIcon icon={MaximizeScreenIcon} class="h-4 w-4" /></button>
			</div>
		</header>

		<div class="ryo-mini-v2-body">
			{#if view === 'now'}
				<div class="ryo-mini-v2-now">
					<div class="ryo-mini-v2-track">
						<Marquee text={now?.title ?? t('Nothing playing')} class="ryo-mini-v2-title" />
						<Marquee text={now?.artists ?? t('Ryotunes is ready')} class="ryo-mini-v2-artist" />
					</div>
					<div class="ryo-mini-v2-seek">
						<span>{fmt(shownPosition)}</span>
						<input
							type="range"
							class="range ryo-mini-v2-range"
							style="--pct:{progress}%"
							min="0"
							max={playback.duration || 0}
							value={shownPosition}
							oninput={onSeekInput}
							onchange={onSeekCommit}
							aria-label={t('Seek')}
						/>
						<span>{fmt(playback.duration)}</span>
					</div>
					<div class="ryo-mini-v2-next" title={next ? `${t('Up next')}: ${next.title}` : t('Queue is empty')}>
						<HugeiconsIcon icon={Queue01Icon} class="h-3.5 w-3.5" />
						<span>{next ? `${t('Next')} · ${next.title}` : `${t('Queue')} · ${t('End')}`}</span>
					</div>
				</div>
			{:else if view === 'lyrics'}
				<div class="ryo-mini-v2-panel ryo-mini-v2-lyrics" aria-label={t('Lyrics')}>
					<div class="ryo-mini-v2-panel-label"><span>// LYRICS</span><b>AUTO FOLLOW</b></div>
					<LyricsView compact />
				</div>
			{:else}
				<div class="ryo-mini-v2-panel ryo-mini-v2-queue" aria-label={t('Queue')}>
					<div class="ryo-mini-v2-panel-label"><span>// QUEUE</span><b>{Math.max(0, playback.queue.items.length - playback.queue.currentIndex - 1)} NEXT</b></div>
					<QueueList compact showMenus={false} />
				</div>
			{/if}
		</div>

		<footer class="ryo-mini-v2-controls">
			<div class="ryo-mini-v2-transport">
				<button class:active={shuffleOn} onclick={() => api.toggleShuffle()} aria-label={t('Shuffle')}><HugeiconsIcon icon={ShuffleIcon} class="h-4 w-4" /></button>
				<button onclick={() => api.prevTrack()} aria-label={t('Previous')}><HugeiconsIcon icon={PreviousIcon} class="h-4 w-4" /></button>
				<button class="primary" onclick={() => api.togglePause()} aria-label={t(playback.paused ? 'Play' : 'Pause')}>
					<HugeiconsIcon icon={PauseIcon} altIcon={PlayIcon} showAlt={playback.paused} class="h-4 w-4" />
				</button>
				<button onclick={() => api.nextTrack()} aria-label={t('Next')}><HugeiconsIcon icon={NextIcon} class="h-4 w-4" /></button>
				<button class:active={repeat !== 'off'} onclick={cycleRepeat} aria-label={`${t('Repeat')}: ${repeat}`}>
					<HugeiconsIcon icon={RepeatIcon} altIcon={RepeatOne01Icon} showAlt={repeat === 'one'} class="h-4 w-4" />
				</button>
			</div>

			<div class="ryo-mini-v2-volume" role="group" aria-label={t('Volume')}>
				<button onclick={toggleMute} aria-label={t(playback.volume === 0 ? 'Unmute' : 'Mute')}>
					<HugeiconsIcon icon={VolumeHighIcon} altIcon={VolumeMute02Icon} showAlt={playback.volume === 0} class="h-4 w-4" />
				</button>
				<input
					type="range"
					class="range ryo-mini-v2-volume-range"
					style="--pct:{playback.volume}%"
					min="0" max="100" value={playback.volume}
					onpointerdown={() => (volDragging = true)}
					oninput={(e) => dragVolume(Number(e.currentTarget.value))}
					onchange={(e) => commitVolume(Number(e.currentTarget.value))}
					onwheel={wheelVolume}
					aria-label={t('Volume')}
				/>
			</div>
		</footer>
	</section>
</div>

<style>
	.ryo-mini-v2 {
		position:relative; display:grid; grid-template-columns:232px minmax(0,1fr); width:100vw; height:100vh;
		overflow:hidden; border:1px solid color-mix(in srgb,var(--ryo-ink) 14%,transparent); border-radius:16px;
		background:color-mix(in srgb,var(--ryo-paper) 97%,var(--mini-accent,var(--ryo-system-accent)) 3%); color:var(--ryo-ink); user-select:none;
		box-shadow:0 28px 72px rgb(0 0 0 / .28),0 2px 12px rgb(0 0 0 / .12); isolation:isolate;
	}
	.ryo-mini-v2::before { content:""; position:absolute; inset:0; z-index:-1; pointer-events:none; background:linear-gradient(125deg,color-mix(in srgb,var(--mini-accent,var(--ryo-system-accent)) 6%,transparent),transparent 50%); }
	.ryo-mini-v2::after { content:""; position:absolute; inset:0; pointer-events:none; border-radius:inherit; box-shadow:inset 0 1px color-mix(in srgb,var(--ryo-ink) 7%,transparent); }
	.ryo-mini-v2-glow { position:absolute; inset:-110px 10% auto -55px; height:285px; z-index:-1; background:radial-gradient(ellipse,color-mix(in srgb,var(--mini-accent,var(--ryo-system-accent)) 11%,transparent),transparent 70%); opacity:.68; }
	.ryo-mini-v2-art { position:relative; overflow:hidden; background:var(--ryo-panel); }
	:global(.ryo-mini-v2-art-image) { width:100%; height:100%; }
	.ryo-mini-v2-art::before { content:""; position:absolute; inset:0; z-index:1; pointer-events:none; box-shadow:inset -1px 0 color-mix(in srgb,var(--ryo-ink) 12%,transparent); }
	.ryo-mini-v2-art::after { content:""; position:absolute; inset:0; pointer-events:none; background:linear-gradient(180deg,rgb(0 0 0/.01) 30%,rgb(0 0 0/.72) 100%); z-index:1; }
	.ryo-mini-v2-live { position:absolute; left:16px; top:15px; z-index:2; padding:5px 8px; border:1px solid rgb(255 255 255/.18); border-radius:999px; background:rgb(7 7 8/.48); color:rgb(255 255 255/.88); font:650 8px/1 "DM Sans Variable",sans-serif; letter-spacing:1.2px; backdrop-filter:blur(8px); }
	.ryo-mini-v2-art-copy { position:absolute; z-index:2; inset:auto 17px 18px; min-width:0; color:white; text-shadow:0 2px 16px rgb(0 0 0/.55); }
	.ryo-mini-v2-art-copy strong,.ryo-mini-v2-art-copy span { display:block; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }
	.ryo-mini-v2-art-copy strong { font-size:14px; line-height:1.25; font-weight:650; letter-spacing:-.01em; }
	.ryo-mini-v2-art-copy span { margin-top:5px; color:rgb(255 255 255/.70); font-size:10px; }
	.ryo-mini-v2-main { min-width:0; min-height:0; display:grid; grid-template-rows:54px minmax(0,1fr) 66px; padding:11px 18px 13px 19px; }
	.ryo-mini-v2-head { display:grid; grid-template-columns:auto minmax(150px,1fr) auto; align-items:center; gap:14px; min-width:0; border-bottom:1px solid var(--ryo-line-soft); padding-bottom:9px; }
	.ryo-mini-v2-brand { display:flex; align-items:center; gap:8px; color:var(--ryo-ink-muted); font:700 8px/1 "DM Sans Variable",sans-serif; letter-spacing:1.35px; white-space:nowrap; }
	.ryo-mini-v2-brand b { color:var(--ryo-ink-dim); font-weight:700; }
	.ryo-mini-v2-rule { width:17px; height:1px; background:var(--ryo-ink-dim); opacity:.58; }
	.ryo-mini-v2-tabs { justify-self:center; display:flex; align-items:center; gap:7px; padding:3px; border:1px solid var(--ryo-line-soft); border-radius:12px; background:color-mix(in srgb,var(--ryo-panel) 78%,transparent); }
	.ryo-mini-v2-tabs button { position:relative; display:grid; place-items:center; width:38px; min-width:38px; height:34px; padding:0; border-radius:9px; border:0; color:var(--ryo-ink-faint); }
	.ryo-mini-v2-tabs button :global(svg) { width:16px; height:16px; opacity:.78; }
	.ryo-mini-v2-tabs button.active { background:var(--ryo-paper-lift); color:var(--ryo-ink); box-shadow:inset 0 0 0 1px var(--ryo-line),0 2px 8px rgb(0 0 0/.06); }
	.ryo-mini-v2-tabs button.active::after { content:""; position:absolute; left:14px; right:14px; bottom:3px; height:2px; border-radius:2px; background:color-mix(in srgb,var(--mini-accent,var(--ryo-system-accent)) 58%,var(--ryo-ink)); opacity:.78; }
	.ryo-mini-v2-head-actions,.ryo-mini-v2-transport,.ryo-mini-v2-volume { display:flex; align-items:center; gap:6px; }
	.ryo-mini-v2 button { display:grid; grid-auto-flow:column; place-items:center; min-width:32px; height:32px; padding:0; border:1px solid transparent; border-radius:9px; color:var(--ryo-ink-muted); cursor:pointer; transition:background-color 110ms,border-color 110ms,color 110ms,transform 90ms,opacity 110ms; }
	.ryo-mini-v2 button:hover,.ryo-mini-v2 button:focus-visible { outline:none; border-color:var(--ryo-line); background:var(--ryo-tint10); color:var(--ryo-ink); }
	.ryo-mini-v2 button:active { transform:scale(.97); }
	.ryo-mini-v2 button.active { color:var(--ryo-bone); }
	.ryo-mini-v2-body { min-height:0; min-width:0; overflow:hidden; }
	.ryo-mini-v2-now { display:grid; height:100%; min-height:0; grid-template-rows:minmax(0,1fr) 36px 30px; }
	.ryo-mini-v2-track { min-width:0; align-self:center; padding:12px 4px 5px; }
	:global(.ryo-mini-v2-title) { color:var(--ryo-ink); font-family:"Fraunces",serif; font-size:29px; line-height:1.04; font-weight:430; letter-spacing:-.028em; }
	:global(.ryo-mini-v2-artist) { margin-top:8px; color:var(--ryo-ink-muted); font-size:11px; line-height:1.25; font-weight:520; }
	.ryo-mini-v2-seek { display:grid; grid-template-columns:36px minmax(0,1fr) 36px; align-items:center; gap:10px; color:var(--ryo-ink-faint); font:600 8px/1 "DM Sans Variable",sans-serif; font-variant-numeric:tabular-nums; }
	.ryo-mini-v2-seek span:last-child { text-align:right; }
	.ryo-mini-v2-range,.ryo-mini-v2-volume-range { outline:none !important; box-shadow:none !important; }
	.ryo-mini-v2-next { min-width:0; display:flex; align-items:center; gap:7px; color:var(--ryo-ink-faint); font:700 8px/1 "DM Sans Variable",sans-serif; letter-spacing:.55px; }
	.ryo-mini-v2-next span { min-width:0; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }
	.ryo-mini-v2-panel { height:100%; min-height:0; display:flex; flex-direction:column; overflow:hidden; padding:8px 0 3px; }
	.ryo-mini-v2-panel-label { flex:none; height:27px; display:flex; align-items:center; justify-content:space-between; padding:0 5px; color:var(--ryo-ink-faint); font:700 8px/1 "DM Sans Variable",sans-serif; letter-spacing:.85px; }
	.ryo-mini-v2-panel-label b { font-weight:700; opacity:.68; }
	.ryo-mini-v2-lyrics :global(.ryo-lyrics-scroller) { scrollbar-width:none; padding-inline:5px; }
	.ryo-mini-v2-lyrics :global(.ryo-lyrics-scroller-compact) { padding-block:54px !important; scroll-padding-block:54px; }
	.ryo-mini-v2-lyrics :global(.ryo-lyrics-scroller::-webkit-scrollbar) { display:none; }
	.ryo-mini-v2-queue { background:transparent; }
	.ryo-mini-v2-queue :global(.ryo-queue-compact) { background:transparent !important; padding-inline:0; }
	.ryo-mini-v2-controls { display:flex; align-items:center; justify-content:space-between; gap:14px; border-top:1px solid var(--ryo-line-soft); padding-top:11px; }
	.ryo-mini-v2-transport { gap:7px; }
	.ryo-mini-v2-transport .primary { width:42px; height:42px; border-radius:12px; background:var(--ryo-ink); color:var(--ryo-paper); border-color:var(--ryo-ink); box-shadow:0 7px 20px rgb(0 0 0/.15); }
	.ryo-mini-v2-transport .primary:hover,.ryo-mini-v2-transport .primary:focus-visible { background:var(--ryo-ink); color:var(--ryo-paper); transform:translateY(-1px); }
	.ryo-mini-v2-volume { width:132px; }
	.ryo-mini-v2-volume-range { min-width:0; width:92px; }
	@media (prefers-reduced-motion: reduce) { .ryo-mini-v2 button { transition:none; } }
</style>
