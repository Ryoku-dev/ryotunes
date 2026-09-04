<script module lang="ts">
	// Module scope, so returning to the library (back from an album you opened, or via the sidebar)
	// keeps the tab you were on instead of snapping to All.
	let lastTab = 'all';
</script>

<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { HugeiconsIcon } from '@hugeicons/svelte';
	import {
		Add01Icon,
		CloudSyncIcon,
		DashboardSquare02Icon,
		DriveIcon,
		MusicNote01Icon,
		MusicNoteSquare02Icon,
		Playlist02Icon,
		SquareStackIcon,
		UserSharingIcon
	} from '@hugeicons/core-free-icons';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import * as Dialog from '$lib/components/ui/dialog';
	import * as Tabs from '$lib/components/ui/tabs';
	import * as Tooltip from '$lib/components/ui/tooltip';
	import LibrarySongs from '$lib/components/LibrarySongs.svelte';
	import LocalMusic from '$lib/components/LocalMusic.svelte';
	import ListeningInsights from '$lib/components/ListeningInsights.svelte';
	import MediaCard from '$lib/components/MediaCard.svelte';
	import ErrorState from '$lib/components/ErrorState.svelte';
	import RyokuPageHeader from '$lib/components/RyokuPageHeader.svelte';
	import type { BrowseItem } from '$lib/api';
	import * as api from '$lib/api';
	import {
		auth,
		personal,
		toast,
		library,
		loadLibrary,
		loadLibraryExtras,
		createLibraryPlaylist,
		syncSavedToYouTube
	} from '$lib/player.svelte';
	import { mergeSaved, unsynced } from '$lib/personal';
	import { reveal } from '$lib/reveal.svelte';
	import { t } from '$lib/i18n.svelte';

	let dialogOpen = $state(false);
	let newTitle = $state('');
	let busy = $state(false);
	// `?tab=local` so anything that sends you back here (an album whose files were deleted) lands
	// on the tab you came from instead of a sign-in prompt.
	let tab = $state(page.url.searchParams.get('tab') ?? lastTab);
	$effect(() => {
		lastTab = tab;
	});

	// Everything here lives in the shared `library` store, so a revisit renders the cached grid
	// immediately and the forced refresh below swaps in fresh data behind it. What was saved on this
	// machine merges in per tab (`mergeSaved`), which is the whole library when signed out.
	const playlists = $derived(mergeSaved(personal, library.items, 'playlist'));
	const albums = $derived(mergeSaved(personal, library.albums, 'album'));
	const artists = $derived(mergeSaved(personal, library.artists, 'artist'));
	const all = $derived([...playlists, ...albums, ...artists]);
	// One per tab rather than one shared instance reset on switch: an `$effect` reset lands
	// after the render it is meant to govern, so switching tabs would build the new tab's grid
	// against the old tab's count and immediately tear the excess back down. A tab keeping its
	// own depth also means coming back to one lands where you left it.
	const rvAll = reveal();
	const rvPlaylists = reveal();
	const rvAlbums = reveal();
	const rvArtists = reveal();
	const loading = $derived((library.loading || library.extrasLoading) && !all.length);
	const error = $derived(library.error ?? library.extrasError);
	// Only the empty states differ: signed out there is no account library to be missing yet.
	const signedOut = $derived(!auth.account?.signedIn);
	// What the sync button has left to push. Synced rows stay in the local library (they are what
	// the user still has after signing out), so counting all of `personal.saved` would nag forever.
	const toSync = $derived(unsynced(personal));
	const libraryReadout = $derived([
		`PLAYLISTS|${playlists.length}`,
		`ALBUMS|${albums.length}`,
		`ARTISTS|${artists.length}`
	]);
	const savedTotal = $derived(playlists.length + albums.length + artists.length);

	onMount(load);

	function load() {
		loadLibrary(true);
		loadLibraryExtras(true);
	}

	let syncing = $state(false);
	async function sync() {
		if (syncing) return;
		syncing = true;
		const n = toSync.length;
		try {
			const { synced, failed } = await syncSavedToYouTube();
			if (failed && synced) toast(`Synced ${synced} of ${n}. ${failed} failed, still saved here.`);
			else if (failed) toast.error(`Nothing synced. ${failed} failed, still saved here.`);
			else toast.success(`Synced ${synced} to YouTube Music`);
		} catch (e) {
			toast.error(String(e));
		} finally {
			syncing = false;
		}
	}

	let importing = $state(false);
	async function importPlaylist() {
		if (importing || !auth.account?.signedIn) return;
		importing = true;
		try {
			const transfer = await api.importPlaylistFile();
			if (!transfer) return;
			if (!transfer.items.length) throw new Error(t('That playlist file has no tracks.'));
			const playlistId = await api.createPlaylist(transfer.title.trim() || 'Imported playlist');
			let added = 0;
			for (const song of transfer.items) {
				try { if (await api.addToPlaylist(playlistId, song.video_id)) added++; } catch {}
			}
			await loadLibrary(true);
			toast.success(`Imported ${added} ${added === 1 ? 'song' : 'songs'}`);
		} catch (e) {
			toast.error(String(e));
		} finally { importing = false; }
	}

	async function createNew() {
		const title = newTitle.trim();
		if (!title || busy) return;
		busy = true;
		try {
			await createLibraryPlaylist(title);
			toast.success(`Created "${title}"`);
			newTitle = '';
			dialogOpen = false;
		} catch (e) {
			toast.error(String(e));
		} finally {
			busy = false;
		}
	}
</script>

{#snippet grid(items: BrowseItem[], empty: string, rv: ReturnType<typeof reveal>)}
	{#if items.length}
		<div class="card-grid content-in">
			{#each items.slice(0, rv.count(items.length)) as item (item.kind + item.id)}
				<MediaCard {item} />
			{/each}
		</div>
		
		{#if rv.more(items.length)}<div {@attach rv.sentinel}></div>{/if}
	{:else}
		<div class="ryo-library-empty">
			<div><span>{t('// COLLECTION / EMPTY')}</span><b>蔵</b></div>
			<strong>{t('Nothing in this shelf yet.')}</strong>
			<p>{empty}</p>
		</div>
	{/if}
{/snippet}

<div class="ryo-route-page">
	<RyokuPageHeader
		eyebrow="MUSIC / COLLECTION"
		title={t('Library')}
		blurb={t('Your saved music, local files and playlists — one collection, arranged like an instrument sheet.')}
		artMode="library"
		code="LIBRARY · INDEX"
		artTitle="収蔵"
		artSub="COLLECTION"
		tate="音を集める"
		seal="蔵"
		readout={libraryReadout}
	/>

	<div class="ryo-library-index" aria-label={t('Library summary')}>
		<div><span>01</span><small>{t('PLAYLISTS')}</small><strong>{playlists.length}</strong></div>
		<div><span>02</span><small>{t('ALBUMS')}</small><strong>{albums.length}</strong></div>
		<div><span>03</span><small>{t('ARTISTS')}</small><strong>{artists.length}</strong></div>
		<div><span>04</span><small>{t('SAVED')}</small><strong>{savedTotal}</strong></div>
		<p>{signedOut ? t('Local collection · sign in to merge your YouTube Music library.') : t('Account collection · local music remains available in its own lane.')}</p>
	</div>

	<div class="ryo-page-toolbar ryo-library-toolbar">
		<div class="ryo-page-toolbar-spacer"></div>
		<div class="flex items-center gap-2">
			{#if auth.account?.signedIn}
				{#if toSync.length}
					<Tooltip.Provider delayDuration={150}>
						<Tooltip.Root>
							<Tooltip.Trigger>
								{#snippet child({ props })}
									<Button
										{...props}
										variant="outline"
										size="icon-sm"
										onclick={sync}
										disabled={syncing}
										aria-label="Sync {toSync.length} saved items to YouTube Music"
									>
										<span class="relative">
											<HugeiconsIcon
												icon={CloudSyncIcon}
												class="h-4 w-4 {syncing ? 'animate-pulse' : ''}"
											/>
											<span
												class="absolute -right-2 -top-1.5 min-w-3.5 rounded-full bg-accent px-[3px] text-[9px] font-semibold leading-[0.875rem] text-accent-foreground ring-[1.5px] ring-background"
											>
												{toSync.length}
											</span>
										</span>
									</Button>
								{/snippet}
							</Tooltip.Trigger>
							<Tooltip.Content side="bottom">
								{syncing
									? t('Adding them to YouTube Music…')
									: `Add the ${toSync.length} saved on this device to your YouTube Music library`}
							</Tooltip.Content>
						</Tooltip.Root>
					</Tooltip.Provider>
				{/if}
				<Button variant="outline" size="sm" onclick={importPlaylist} disabled={importing}>
					{importing ? t('Importing…') : t('Import playlist')}
				</Button>
			{/if}
			<Button variant="outline" size="sm" class="gap-2" onclick={() => (dialogOpen = true)}>
				<HugeiconsIcon icon={Add01Icon} class="h-4 w-4" /> {t('New playlist')}
			</Button>
		</div>
	</div>

	<div class="ryo-library-body ryo-library-tab-{tab}">
	<Dialog.Root bind:open={dialogOpen}>
		<Dialog.Content class="ryo-overlay-sheet sm:max-w-md">
			<Dialog.Header>
				<Dialog.Title>{t('New playlist')}</Dialog.Title>
				<Dialog.Description>
					{signedOut
						? t('This playlist will be stored on this device. Sign in later without losing it.')
						: t('Give your YouTube Music playlist a name to get started.')}
				</Dialog.Description>
			</Dialog.Header>
			<form
				class="flex flex-col gap-4"
				onsubmit={(e) => {
					e.preventDefault();
					createNew();
				}}
			>
				<Input bind:value={newTitle} placeholder={t('Playlist name')} autofocus />
				<Dialog.Footer>
					<Button type="button" variant="outline" onclick={() => (dialogOpen = false)}>
						{t('Cancel')}
					</Button>
					<Button type="submit" disabled={busy || !newTitle.trim()}>
						{busy ? t('Creating…') : t('Create')}
					</Button>
				</Dialog.Footer>
			</form>
		</Dialog.Content>
	</Dialog.Root>

	
	<Tabs.Root bind:value={tab}>
		<Tabs.List class="mb-4">
			<Tabs.Trigger value="all">
				<HugeiconsIcon icon={SquareStackIcon} class="h-4 w-4" /> {t('All')}
			</Tabs.Trigger>
			<Tabs.Trigger value="playlists">
				<HugeiconsIcon icon={Playlist02Icon} class="h-4 w-4" /> {t('Playlists')}
			</Tabs.Trigger>
			<Tabs.Trigger value="albums">
				<HugeiconsIcon icon={MusicNoteSquare02Icon} class="h-4 w-4" /> {t('Albums')}
			</Tabs.Trigger>
			<Tabs.Trigger value="artists">
				<HugeiconsIcon icon={UserSharingIcon} class="h-4 w-4" /> {t('Artists')}
			</Tabs.Trigger>
			<Tabs.Trigger value="songs">
				<HugeiconsIcon icon={MusicNote01Icon} class="h-4 w-4" /> {t('Songs')}
			</Tabs.Trigger>
			<Tabs.Trigger value="local">
				<HugeiconsIcon icon={DriveIcon} class="h-4 w-4" /> {t('Local')}
			</Tabs.Trigger>
			<Tabs.Trigger value="insights">
				<HugeiconsIcon icon={DashboardSquare02Icon} class="h-4 w-4" /> {t('Insights')}
			</Tabs.Trigger>
		</Tabs.List>
		
		
		<Tabs.Content value="songs">
			{#if tab === 'songs'}
				{#if signedOut}
					<p class="text-sm text-muted-foreground">{t('Sign in to see the songs saved in your YouTube Music library. Music on this machine is in the Local tab.')}</p>
				{:else}
					<LibrarySongs />
				{/if}
			{/if}
		</Tabs.Content>
		<Tabs.Content value="local">{#if tab === 'local'}<LocalMusic />{/if}</Tabs.Content>
		<Tabs.Content value="insights">{#if tab === 'insights'}<ListeningInsights />{/if}</Tabs.Content>
		{#if tab !== 'local' && tab !== 'songs' && tab !== 'insights'}
			{#if loading}
				<div class="ryo-library-loading" aria-live="polite">
					<div><span>{t('// COLLECTION / INDEXING')}</span><b>{t('LOCAL + ACCOUNT')}</b></div>
					<strong>{t('Loading your collection.')}</strong>
					<p>{t('Resolving playlists, albums and artists without blocking local music.')}</p>
					<section aria-hidden="true">
						{#each Array(8) as _, i (i)}
							<i style="--w:{42 + ((i * 19) % 48)}%"></i>
						{/each}
					</section>
				</div>
			{:else if error && !all.length}
				<ErrorState message={error} onRetry={load} />
			{:else}
				<Tabs.Content value="all">
					{#if tab === 'all'}
						{@render grid(
							all,
							signedOut
								? t('Nothing saved yet. Open a playlist or album and hit Save to library, or sign in for the one on your account.')
								: t('Your library is empty.'),
							rvAll
						)}
					{/if}
				</Tabs.Content>
				<Tabs.Content value="playlists">
					{#if tab === 'playlists'}
						{@render grid(
							playlists,
							signedOut
								? t('No playlists yet. Use New playlist above to create one on this device.')
								: t('No playlists yet. Use New playlist above, or save one from YouTube Music.'),
							rvPlaylists
						)}
					{/if}
				</Tabs.Content>
				<Tabs.Content value="albums">
					{#if tab === 'albums'}
						{@render grid(
							albums,
							t('No saved albums yet. Open an album and hit Save to library.'),
							rvAlbums
						)}
					{/if}
				</Tabs.Content>
				<Tabs.Content value="artists">
					{#if tab === 'artists'}
						{@render grid(
							artists,
							signedOut
								? t('No artists yet. Save one from its page to keep it here.')
								: t('No artists yet. They show up once you save their songs or albums.'),
							rvArtists
						)}
					{/if}
				</Tabs.Content>
			{/if}
		{/if}
	</Tabs.Root>
	</div>
</div>
