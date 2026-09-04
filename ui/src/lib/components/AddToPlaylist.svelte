<script lang="ts">
	import { fade } from 'svelte/transition';
	import { HugeiconsIcon } from '@hugeicons/svelte';
	import { Cancel01Icon } from '@hugeicons/core-free-icons';
	import * as api from '$lib/api';
	import type { BrowseItem } from '$lib/api';
	import { thumb } from '$lib/thumb';
	import {
		ui,
		toast,
		bumpLibraryTrackCount,
		createLibraryPlaylist,
		notePlaylistAdd,
		noteSavedIn
	} from '$lib/player.svelte';
	import { t } from '$lib/i18n.svelte';

	let playlists = $state<BrowseItem[]>([]);
	let loading = $state(false);
	let creating = $state(false);
	let newTitle = $state('');
	let showCreate = $state(false);

	// Fetch the library playlists fresh each time the picker opens (cheap; picks up new playlists).
	// On Repeat and Liked Music are dropped: On Repeat is built from local play counts, and Liked
	// Music takes likes rather than playlist edits (YouTube 400s the add). The command boundary
	// refuses both too, but a target you can tap and can't use is the bug.
	$effect(() => {
		if (ui.addSongs) {
			loading = true;
			api
				.getLibrary()
				.then(
					(p) =>
						(playlists = p.filter(
							(i) => !api.isSmartPlaylistId(i.id) && i.id !== api.LIKED_MUSIC_ID
						))
				)
				.catch((e) => toast.error(String(e)))
				.finally(() => (loading = false));
		}
	});

	function close() {
		ui.addSongs = null;
		showCreate = false;
		newTitle = '';
	}

	async function pick(pl: BrowseItem) {
		const songs = ui.addSongs;
		close();
		if (!songs?.length) return;
		try {
			// Sequential — a whole album is a handful of requests; don't hammer the API in parallel.
			// YouTube refuses a track the playlist already holds, so only the ones it accepted get
			// counted and drawn: an optimistic row for a refused add is a row that can never be
			// removed (no setVideoId behind it) until the app restarts.
			const added: typeof songs = [];
			for (const song of songs) {
				const accepted = api.isLocalPlaylistId(pl.id)
					? await api.addToLocalPlaylist(pl.id, song)
					: await api.addToPlaylist(pl.id, song.video_id);
				if (accepted) added.push(song);
			}
			const dupes = songs.length - added.length;
			// Every song, not just the accepted ones: a refusal means the playlist already holds it,
			// so its "saved" mark is right either way.
			noteSavedIn(pl.id, songs.map((s) => s.video_id));
			if (added.length) {
				bumpLibraryTrackCount(pl.id, added.length);
				notePlaylistAdd(pl.id, added);
			}
			if (!added.length) {
				toast(dupes > 1 ? `All ${dupes} are already in ${pl.title}` : `Already in ${pl.title}`);
			} else if (dupes) {
				toast.success(`Added ${added.length} to ${pl.title} (${dupes} already there)`);
			} else {
				toast.success(
					added.length > 1 ? `Added ${added.length} songs to ${pl.title}` : `Added to ${pl.title}`
				);
			}
		} catch (e) {
			toast.error(String(e));
		}
	}

	async function createAndPick() {
		const title = newTitle.trim();
		if (!title || creating) return;
		creating = true;
		try {
			const playlist = await createLibraryPlaylist(title);
			playlists = [playlist, ...playlists.filter((p) => p.id !== playlist.id)];
			newTitle = '';
			showCreate = false;
			await pick(playlist);
		} catch (e) {
			toast.error(String(e));
		} finally {
			creating = false;
		}
	}
</script>

<svelte:window
	onkeydown={(e) => {
		if (ui.addSongs && e.key === 'Escape') close();
	}}
/>

{#if ui.addSongs}
	<div
		transition:fade={{ duration: 90 }}
		class="ryo-overlay-backdrop fixed inset-0 z-50 flex items-center justify-center p-4"
	>
		<div
			class="ryo-overlay-sheet w-full max-w-sm p-4"
		>
			<div class="mb-3 flex items-center justify-between">
				<h2 class="font-heading text-base font-semibold">{t('Add to playlist')}</h2>
				<button
					class="flex h-8 w-8 shrink-0 items-center justify-center rounded-full text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
					onclick={close}
					aria-label={t('Close')}
				>
					<HugeiconsIcon icon={Cancel01Icon} class="h-4 w-4" />
				</button>
			</div>
			<div class="mb-2">
				{#if showCreate}
					<form
						class="flex gap-2"
						onsubmit={(e) => {
							e.preventDefault();
							createAndPick();
						}}
					>
						<input
							class="min-w-0 flex-1 rounded-md border border-border bg-background px-3 py-2 text-sm outline-none focus:border-foreground/40"
							bind:value={newTitle}
							maxlength="150"
							placeholder={t('Playlist name')}
						/>
						<button
							type="submit"
							class="rounded-md bg-foreground px-3 py-2 text-sm font-medium text-background disabled:opacity-50"
							disabled={creating || !newTitle.trim()}
						>
							{t(creating ? 'Creating…' : 'Create + add')}
						</button>
					</form>
				{:else}
					<button
						class="w-full rounded-md border border-border px-3 py-2 text-left text-sm font-medium hover:bg-accent/10"
						onclick={() => (showCreate = true)}
					>
						{t('+ New playlist')}
					</button>
				{/if}
			</div>
			{#if loading}
				<p class="p-2 text-sm text-muted-foreground">{t('Loading…')}</p>
			{:else if playlists.length}
				<div class="max-h-80 overflow-y-auto">
					{#each playlists as pl (pl.id)}
						<button
							class="flex w-full items-center gap-3 rounded-lg p-2 text-left hover:bg-accent/10"
							onclick={() => pick(pl)}
						>
							{#if pl.thumbnail}
								<img src={thumb(pl.thumbnail, 96)} alt="" class="h-10 w-10 rounded-md object-cover" />
							{:else}
								<div class="h-10 w-10 rounded-md bg-muted"></div>
							{/if}
							<div class="min-w-0">
								<div class="truncate text-sm font-medium">{pl.title}</div>
								{#if pl.subtitle}
									<div class="truncate text-xs text-muted-foreground">{pl.subtitle}</div>
								{/if}
							</div>
						</button>
					{/each}
				</div>
			{:else}
				<p class="p-2 text-sm text-muted-foreground">
					{t('No playlists yet. Create one above and these songs will be added immediately.')}
				</p>
			{/if}
		</div>
	</div>
{/if}
