<script lang="ts">
	// "Edit playlist" on a playlist you own: name, description, visibility and a cover of your own.
	//
	// The three text/visibility fields are one write, sent on Save and only for what actually
	// changed. The cover applies the moment a file is picked: it is stored on this machine (so it
	// draws instantly and offline) and uploaded to YouTube Music behind the picker.
	import { HugeiconsIcon } from '@hugeicons/svelte';
	import { ImageAdd02Icon, Delete02Icon } from '@hugeicons/core-free-icons';
	import * as Dialog from '$lib/components/ui/dialog';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Switch } from '$lib/components/ui/switch';
	import * as api from '$lib/api';
	import { thumb } from '$lib/thumb';
	import { toast } from '$lib/player.svelte';
	import { t } from '$lib/i18n.svelte';

	/** What the page shows while YouTube catches up, and what it puts back if the write fails. */
	type Edit = { title?: string; description?: string; privacy?: string; cover?: string };

	let {
		open = $bindable(false),
		id,
		title,
		description,
		privacy,
		cover,
		fallback,
		local = false,
		onchange
	}: {
		open: boolean;
		id: string;
		title?: string;
		description?: string;
		privacy?: string;
		/** Custom artwork already stored for this playlist. */
		cover?: string;
		/** YouTube's own artwork, shown when there is no custom one. */
		fallback?: string;
		/** Device playlist: name + local artwork only; no YouTube description/privacy write. */
		local?: boolean;
		onchange: (patch: Edit) => void;
	} = $props();

	let draftName = $state('');
	let draftDescription = $state('');
	let isPublic = $state(false);
	let saving = $state(false);
	let removing = $state(false);
	let coverBusy = $state(false);

	// Fill the form from the page each time it opens, so a cancelled edit leaves nothing behind.
	// Guarded on `open` before anything else is read: while closed, the props aren't tracked, so a
	// mid-edit optimistic update on the page can't reach in and rewrite the draft.
	$effect(() => {
		if (!open) return;
		draftName = title ?? '';
		draftDescription = description ?? '';
		isPublic = privacy === 'PUBLIC';
	});

	const preview = $derived(thumb(cover ?? fallback, 400));

	async function pickCover() {
		await storeCover(true);
	}

	// Rust owns the picker, so WebKit never sees or chooses a filesystem path.
	async function storeCover(pick: boolean) {
		if (coverBusy) return;
		coverBusy = true;
		removing = !pick;
		try {
			const result = await api.setPlaylistCover(id, pick);
			if (!result) return;
			const { cover: saved, thumbnail } = result;
			onchange({ cover: saved ?? undefined, ...(thumbnail ? { thumbnail } : {}) });
		} catch (e) {
			toast.error(String(e));
		} finally {
			coverBusy = false;
			removing = false;
		}
	}

	async function save() {
		if (saving) return;
		const name = draftName.trim();
		const changes: { name?: string; description?: string; public?: boolean } = {};
		if (name && name !== title) changes.name = name;
		if (!local && draftDescription !== (description ?? '')) changes.description = draftDescription;
		if (!local && isPublic !== (privacy === 'PUBLIC')) changes.public = isPublic;
		if (!Object.keys(changes).length) {
			open = false;
			return;
		}
		const before: Edit = { title, description, privacy };
		saving = true;
		onchange(
			local
				? { title: changes.name ?? title }
				: {
						title: changes.name ?? title,
						description: changes.description ?? description,
						privacy:
							changes.public === undefined ? privacy : changes.public ? 'PUBLIC' : 'PRIVATE'
					}
		);
		open = false;
		try {
			await api.editPlaylistDetails(id, changes);
			toast.success(t('Playlist updated'));
		} catch (e) {
			onchange(before);
			toast.error(String(e));
		} finally {
			saving = false;
		}
	}
</script>

<Dialog.Root bind:open>
	<Dialog.Content class="ryo-overlay-sheet sm:max-w-xl">
		<Dialog.Header>
			<Dialog.Title>{t('Edit playlist')}</Dialog.Title>
			<Dialog.Description>
				{t(local
					? 'Change the name or artwork stored for this playlist on this device.'
					: 'Change how this playlist looks and who can see it.')}
			</Dialog.Description>
		</Dialog.Header>
		<form
			class="flex flex-col gap-4"
			onsubmit={(e) => {
				e.preventDefault();
				save();
			}}
		>
			<div class="flex gap-4">
				<div class="flex shrink-0 flex-col items-center gap-1.5">
					<button
						type="button"
						class="group relative h-32 w-32 cursor-pointer overflow-hidden rounded-xl border bg-muted"
						onclick={pickCover}
						aria-label={t('Change cover art')}
						disabled={coverBusy}
					>
						{#if preview}
							<img src={preview} alt="" class="h-full w-full object-cover" />
						{/if}
						
						<span
							class="absolute inset-0 flex flex-col items-center justify-center gap-1 bg-black/60 text-xs font-medium text-white transition group-hover:opacity-100 group-focus-visible:opacity-100 {preview
								? 'opacity-0'
								: 'opacity-100'}"
						>
							<HugeiconsIcon icon={ImageAdd02Icon} class="h-6 w-6" />
							{t('Choose image')}
						</span>
					</button>
					{#if cover}
						<Button
							type="button"
							variant="ghost"
							size="sm"
							class="gap-1.5 text-xs text-muted-foreground"
							onclick={() => storeCover(false)}
							disabled={removing}
						>
							<HugeiconsIcon icon={Delete02Icon} class="h-3.5 w-3.5" />
							{t(removing ? 'Removing…' : 'Remove')}
						</Button>
					{/if}
				</div>
				<div class="flex min-w-0 flex-1 flex-col gap-3">
					<Input bind:value={draftName} placeholder={t('Playlist name')} aria-label={t('Playlist name')} />
					{#if !local}
						<textarea
							bind:value={draftDescription}
							placeholder={t('Description')}
							aria-label={t('Playlist description')}
							rows="4"
							class="w-full flex-1 resize-none rounded-2xl border border-input bg-input/30 px-3 py-2 text-sm outline-none transition-colors placeholder:text-muted-foreground focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50"
						></textarea>
					{:else}
						<p class="rounded-2xl border px-3 py-2 text-xs leading-relaxed text-muted-foreground">
							{t('Device playlists stay available without a Google account. Their songs, name and artwork are kept locally and are never uploaded automatically.')}
						</p>
					{/if}
				</div>
			</div>
			{#if !local}
				<div class="flex items-center justify-between gap-4 rounded-2xl border px-3 py-2.5">
					<div class="min-w-0">
						<div class="text-sm font-medium">{t('Public')}</div>
						<p class="text-xs text-muted-foreground">
							{t(isPublic
								? 'Anyone can find this playlist on YouTube Music.'
								: 'Only you can see this playlist.')}
						</p>
					</div>
					<Switch bind:checked={isPublic} aria-label={t('Public playlist')} />
				</div>
			{/if}
			<p class="text-xs text-muted-foreground">
				{t(local
					? 'Artwork is copied into Ryotunes storage on this device. Square JPEG or PNG works best.'
					: 'Artwork applies here at once and uploads to YouTube Music in the background. Square JPEG or PNG works best.')}
			</p>
			<Dialog.Footer>
				<Button type="button" variant="outline" onclick={() => (open = false)}>{t('Cancel')}</Button>
				<Button type="submit" disabled={saving || !draftName.trim()}>
					{t(saving ? 'Saving…' : 'Save')}
				</Button>
			</Dialog.Footer>
		</form>
	</Dialog.Content>
</Dialog.Root>
