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
		Queue01Icon,
		Mic01Icon,
		VolumeHighIcon,
		VolumeMute02Icon,
		FavouriteIcon,
		Add01Icon,
		InfinityIcon,
		MinimizeScreenIcon,
		MusicNote01Icon,
		ArrowUp01Icon,
		ArrowDown01Icon,
		Loading03Icon
	} from '@hugeicons/core-free-icons';
	import { fade } from 'svelte/transition';
	import { Button } from '$lib/components/ui/button';
	import * as api from '$lib/api';
	import {
		np,
		playback,
		setPlaybackPosition,
		commitVolume,
		cycleRepeat,
		dragVolume,
		openAddToPlaylist,
		openMiniPlayer,
		toggleMute,
		toggleNowPlayingLike,
		wheelVolume
	} from '$lib/player.svelte';
	import { thumb } from '$lib/thumb';
	import ArtistLine from './ArtistLine.svelte';
	import Marquee from './Marquee.svelte';
	import TrackMenu from './TrackMenu.svelte';
	import { t } from '$lib/i18n.svelte';

	let {
		onToggleQueue,
		queueOpen,
		onToggleLyrics,
		lyricsOpen
	}: {
		onToggleQueue: () => void;
		queueOpen: boolean;
		onToggleLyrics: () => void;
		lyricsOpen: boolean;
	} = $props();

	// Pop the heart once when the user favourites (not when un-favouriting). Reset on animation end
	// so the next like can replay it.
	let justLiked = $state(false);

	function toggleLike() {
		if (playback.rating !== 'like') justLiked = true;
		toggleNowPlayingLike();
	}

	const fmt = (secs: number) => {
		if (!secs || secs < 0) return '0:00';
		const t = Math.floor(secs);
		const h = Math.floor(t / 3600);
		const m = Math.floor((t % 3600) / 60);
		const s = t % 60;
		const mm = h ? m.toString().padStart(2, '0') : `${m}`;
		return `${h ? `${h}:` : ''}${mm}:${s.toString().padStart(2, '0')}`;
	};

	const shuffleOn = $derived(playback.queue.shuffle ?? false);
	const repeat = $derived(playback.queue.repeat ?? 'off');
	const isRadioNow = $derived(api.isRadioId(playback.now?.videoId));
	const hasYouTubeTrack = $derived(
		!!playback.now &&
			!api.isLocalId(playback.now.videoId) &&
			!api.isRadioId(playback.now.videoId)
	);

	// The current track was appended by autoplay → show the subtle ∞ badge next to the title.
	// Matched against the now-playing videoId so a transient queue/now-playing mismatch (mid
	// gapless advance) can't flash the badge on the wrong song.
	const autoplayTrack = $derived.by(() => {
		const cur = playback.queue.items[playback.queue.currentIndex];
		return !!cur?.autoplay && cur.video_id === playback.now?.videoId;
	});

	// The ⋮ menu needs the full SongItem — NowPlaying carries no album_id. Take it from the queue
	// row, matched on videoId so a mid-advance mismatch can't point the menu at the wrong song.
	const currentSong = $derived.by(() => {
		const cur = playback.queue.items[playback.queue.currentIndex];
		return cur?.video_id === playback.now?.videoId ? cur : null;
	});

	// Seek: while dragging, hold a local value so incoming mpv position ticks can't yank the thumb
	// back under the pointer; only invoke the (expensive) seek on release.
	let seekDrag = $state<number | null>(null);
	const shownPosition = $derived(seekDrag ?? playback.position);
	const seekPct = $derived(playback.duration > 0 ? Math.min(100, Math.max(0, (shownPosition / playback.duration) * 100)) : 0);
	const SEEK_WAVE = 'M 0 5 Q 1.250 1 2.500 5 Q 3.750 9 5.000 5 Q 6.250 1 7.500 5 Q 8.750 9 10.000 5 Q 11.250 1 12.500 5 Q 13.750 9 15.000 5 Q 16.250 1 17.500 5 Q 18.750 9 20.000 5 Q 21.250 1 22.500 5 Q 23.750 9 25.000 5 Q 26.250 1 27.500 5 Q 28.750 9 30.000 5 Q 31.250 1 32.500 5 Q 33.750 9 35.000 5 Q 36.250 1 37.500 5 Q 38.750 9 40.000 5 Q 41.250 1 42.500 5 Q 43.750 9 45.000 5 Q 46.250 1 47.500 5 Q 48.750 9 50.000 5 Q 51.250 1 52.500 5 Q 53.750 9 55.000 5 Q 56.250 1 57.500 5 Q 58.750 9 60.000 5 Q 61.250 1 62.500 5 Q 63.750 9 65.000 5 Q 66.250 1 67.500 5 Q 68.750 9 70.000 5 Q 71.250 1 72.500 5 Q 73.750 9 75.000 5 Q 76.250 1 77.500 5 Q 78.750 9 80.000 5 Q 81.250 1 82.500 5 Q 83.750 9 85.000 5 Q 86.250 1 87.500 5 Q 88.750 9 90.000 5 Q 91.250 1 92.500 5 Q 93.750 9 95.000 5 Q 96.250 1 97.500 5 Q 98.750 9 100.000 5';

	function onSeekInput(e: Event) {
		seekDrag = Number((e.target as HTMLInputElement).value);
	}
	function onSeekCommit(e: Event) {
		const v = Number((e.target as HTMLInputElement).value);
		setPlaybackPosition(v);
		seekDrag = null;
		api.seek(v);
	}

	const onVolume = (e: Event) => dragVolume(Number((e.target as HTMLInputElement).value));
	const onVolumeCommit = (e: Event) => commitVolume(Number((e.target as HTMLInputElement).value));

	const isControl = (t: EventTarget | null) =>
		!!(t as HTMLElement | null)?.closest?.('button, a, input, [role="button"]');

	// Dragging a slider past its end and releasing outside it retargets the click at the bar (the
	// click lands on the common ancestor of press and release), which used to toggle the view.
	// So judge by where the press started, not where the release happened.
	let pressedControl = false;

	// Anywhere on the bar that isn't a control opens (or closes) the now-playing view: the bar is
	// what's left of it once it's minimised, so it's the way back in. Deliberately no pointer
	// cursor, because this is the whole bar, not a button, and every real button keeps its own click.
	function onBarClick(e: MouseEvent) {
		if (pressedControl || isControl(e.target)) return;
		np.open = !np.open;
	}
</script>


<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions, a11y_no_noninteractive_element_interactions -->
<footer
	onpointerdown={(e) => (pressedControl = isControl(e.target))}
	onclick={onBarClick}
	class="ryo-playerbar flex items-center gap-2 border-t bg-background px-2 py-2.5 sm:gap-4 sm:px-4 sm:py-3"
	class:ryo-playerbar-live={!playback.paused}
>
	
	<div class="flex min-w-0 flex-1 items-center gap-3" data-ctx>
		{#key playback.now?.videoId}
			{#if playback.now?.thumbnail}
				<img
					src={thumb(playback.now.thumbnail, 120)}
					alt=""
					style="max-width:none"
					class="h-12 w-12 shrink-0 rounded-md object-cover"
					in:fade={{ duration: 170 }}
				/>
			{:else}
				<div
					class="flex h-12 w-12 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground/50"
				>
					<HugeiconsIcon icon={MusicNote01Icon} class="h-5 w-5" />
				</div>
			{/if}
		{/key}
		<div class="min-w-0">
			<div class="flex items-center gap-1.5">
				<Marquee
					text={playback.now?.title ?? t('Nothing playing')}
					class="text-sm font-medium"
				/>
				{#if playback.pendingVideoId}
					<span class="ryo-player-resolving" title={t('Resolving stream')}><HugeiconsIcon icon={Loading03Icon} class="h-3 w-3 animate-spin" /> {t('RESOLVING')}</span>
				{/if}
				{#if autoplayTrack}
					<span
						class="shrink-0 text-muted-foreground"
						title={t('Playing similar music (Autoplay)')}
						in:fade={{ duration: 170 }}
					>
						<HugeiconsIcon icon={InfinityIcon} class="h-3.5 w-3.5" />
					</span>
				{/if}
			</div>
			<ArtistLine
				runs={playback.now?.artistRuns}
				text={playback.now?.artists ?? ''}
				marquee
				class="block max-w-full text-xs text-muted-foreground"
			/>
		</div>
		{#if playback.now}
			<div class="flex items-center">
				
				{#if hasYouTubeTrack}
					<Button
						variant="ghost"
						size="icon-sm"
						class="hidden lg:inline-flex"
						onclick={toggleLike}
						aria-label={t('Like')}
					>
						<span
							class="inline-flex"
							class:animate-heart-pop={justLiked}
							onanimationend={() => (justLiked = false)}
						>
							<HugeiconsIcon
								icon={FavouriteIcon}
								class="h-4 w-4 {playback.rating === 'like' ? 'fill-current text-primary' : 'text-muted-foreground'}"
							/>
						</span>
					</Button>
					<Button
						variant="ghost"
						size="icon-sm"
						class="hidden lg:inline-flex"
						onclick={() => {
							const now = playback.now!;
							openAddToPlaylist({
								video_id: now.videoId,
								title: now.title,
								artists: now.artists,
								artist_id: now.artistId,
								thumbnail: now.thumbnail,
								duration: now.duration
							});
						}}
						aria-label={t('Add to playlist')}
					>
						<HugeiconsIcon icon={Add01Icon} class="h-4 w-4 text-muted-foreground" />
					</Button>
				{/if}
				{#if currentSong}
					<TrackMenu
						song={currentSong}
						linksOnly
						onAdd={() => openAddToPlaylist(currentSong!)}
						triggerClass="inline-flex size-8 cursor-pointer items-center justify-center rounded-md text-muted-foreground transition hover:bg-muted hover:text-foreground"
					/>
				{/if}
			</div>
		{/if}
	</div>

	
	<div class="ryo-playerbar-center flex flex-[1.5] flex-col items-center">
		<div class="flex items-center gap-1">
			<Button
				variant="ghost"
				size="icon-sm"
				onclick={() => api.toggleShuffle()}
				aria-label={t('Shuffle')}
				aria-pressed={shuffleOn}
			>
				<HugeiconsIcon
					icon={ShuffleIcon}
					class="h-4 w-4 {shuffleOn ? 'text-primary' : 'text-muted-foreground'}"
				/>
			</Button>
			<Button variant="ghost" size="icon-sm" onclick={() => api.prevTrack()} aria-label={t('Previous')}>
				<HugeiconsIcon icon={PreviousIcon} class="h-5 w-5" />
			</Button>
			<Button
				variant="default"
				size="icon"
				class="ryo-transport-primary rounded-md"
				onclick={() => api.togglePause()}
				aria-label={t('Play/pause')}
			>
				
			<HugeiconsIcon
				icon={PauseIcon}
				altIcon={PlayIcon}
				showAlt={playback.paused}
				class="h-5 w-5"
			/>
			</Button>
			<Button variant="ghost" size="icon-sm" onclick={() => api.nextTrack()} aria-label={t('Next')}>
				<HugeiconsIcon icon={NextIcon} class="h-5 w-5" />
			</Button>
			<Button
				variant="ghost"
				size="icon-sm"
				onclick={cycleRepeat}
				aria-label={`${t('Repeat')}: ${repeat}`}
				aria-pressed={repeat !== 'off'}
			>
				
				<HugeiconsIcon
					icon={RepeatIcon}
					altIcon={RepeatOne01Icon}
					showAlt={repeat === 'one'}
					class="h-4 w-4 {repeat !== 'off' ? 'text-primary' : 'text-muted-foreground'}"
				/>
			</Button>
		</div>
		<div class="ryo-wave-seek-row flex w-full max-w-md items-center gap-2 text-xs text-muted-foreground">
			<span class="tabular-nums">{fmt(shownPosition)}</span>
			<div class="ryo-wave-seek flex-1" style="--pct:{seekPct}%">
				<svg viewBox="0 0 100 10" preserveAspectRatio="none" aria-hidden="true">
					<line class="ryo-wave-seek-rest" x1={seekPct} y1="5" x2="100" y2="5" />
					<path class="ryo-wave-seek-played" d={SEEK_WAVE} />
				</svg>
				<input
					type="range"
					class="ryo-wave-seek-input"
					min="0"
					max={playback.duration || 0}
					value={shownPosition}
					oninput={onSeekInput}
					onchange={onSeekCommit}
					aria-label={t('Seek')}
				/>
			</div>
			<span class="tabular-nums">{fmt(playback.duration)}</span>
		</div>
	</div>

	
	<div class="flex flex-1 items-center justify-end gap-2">
		
		<div class="hidden items-center gap-1 md:flex">
			<Button
				variant="ghost"
				size="icon-sm"
				class="text-muted-foreground"
				onclick={toggleMute}
				aria-label={t(playback.volume === 0 ? 'Unmute' : 'Mute')}
			>
				
				<HugeiconsIcon
					icon={VolumeHighIcon}
					altIcon={VolumeMute02Icon}
					showAlt={playback.volume === 0}
					class="h-4 w-4"
				/>
			</Button>
			<input
				type="range"
				class="range w-24"
				style="--pct:{playback.volume}%"
				min="0"
				max="100"
				value={playback.volume}
				oninput={onVolume}
				onchange={onVolumeCommit}
				onwheel={wheelVolume}
				aria-label={t('Volume')}
			/>
		</div>
		
		<div class="flex items-center gap-0.5">
			<Button variant="ghost" size="icon-sm" onclick={openMiniPlayer} aria-label={t('Mini player')}>
				<HugeiconsIcon icon={MinimizeScreenIcon} class="h-5 w-5" />
			</Button>
			<Button
				variant={lyricsOpen ? 'secondary' : 'ghost'}
				size="icon-sm"
				onclick={onToggleLyrics}
				aria-label={t(isRadioNow ? 'Lyrics unavailable for live radio' : 'Toggle lyrics')}
				disabled={isRadioNow}
			>
				<HugeiconsIcon icon={Mic01Icon} class="h-5 w-5" />
			</Button>
			<Button
				variant={queueOpen ? 'secondary' : 'ghost'}
				size="icon-sm"
				onclick={onToggleQueue}
				aria-label={t('Toggle queue')}
			>
				<HugeiconsIcon icon={Queue01Icon} class="h-5 w-5" />
			</Button>
			
			<Button
				variant="ghost"
				size="icon-sm"
				onclick={() => (np.open = !np.open)}
				aria-label={t(np.open ? 'Minimise player' : 'Open player')}
				aria-expanded={np.open}
			>
				
				<HugeiconsIcon
					icon={ArrowUp01Icon}
					altIcon={ArrowDown01Icon}
					showAlt={np.open}
					class="h-5 w-5"
				/>
			</Button>
		</div>
	</div>
</footer>
