<script lang="ts">
	import { goto } from '$app/navigation';
	import { untrack } from 'svelte';
	import * as Dialog from '$lib/components/ui/dialog';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { HugeiconsIcon } from '@hugeicons/svelte';
	import { Link01Icon, Copy01Icon, ArrowRight01Icon } from '@hugeicons/core-free-icons';
	import { hrefFor } from '$lib/browse';
	import { parseYtLink } from '$lib/ytlink';
	import { startRadio, toast, ui } from '$lib/player.svelte';
	import { t } from '$lib/i18n.svelte';

	let url = $state('');
	const target = $derived(parseYtLink(url));

	// When the dialog opens, opportunistically adopt a supported music link already on the
	// clipboard. Clipboard denial is intentionally silent; the explicit Paste button still reports it.
	$effect(() => {
		if (!ui.linkOpen || url.trim()) return;
		untrack(() => {
			navigator.clipboard?.readText().then((value) => {
				const candidate = value?.trim() ?? '';
				if (ui.linkOpen && candidate && parseYtLink(candidate)) url = candidate;
			}).catch(() => {});
		});
	});

	async function paste() {
		try {
			const value = await navigator.clipboard.readText();
			if (value) url = value.trim();
		} catch {
			toast.error(t('Clipboard access was not available'));
		}
	}

	function submit(e: Event) {
		e.preventDefault();
		const resolved = parseYtLink(url);
		if (!resolved) {
			toast.error(t('That is not a YouTube Music link'));
			return;
		}
		ui.linkOpen = false;
		url = '';
		if (resolved.kind === 'song') startRadio('song', resolved.id);
		else goto(hrefFor({ ...resolved, title: '' }));
	}
</script>

<Dialog.Root bind:open={ui.linkOpen}>
	<Dialog.Content class="ryo-overlay-sheet ryo-link-sheet overflow-hidden p-0 sm:max-w-[560px]">
		<header class="ryo-overlay-head">
			<div class="ryo-overlay-eyebrow"><span>—</span><b>力</b><strong>LINK / RESOLVE</strong><i></i><em>LINK-01</em></div>
			<Dialog.Title>{t('Open a music link')}</Dialog.Title>
			<Dialog.Description>{t('Resolve a YouTube Music song, album, artist or playlist without leaving the instrument.')}</Dialog.Description>
		</header>

		<form class="ryo-link-body" onsubmit={submit}>
			<label class="ryo-overlay-field">
				<span>ADDRESS</span>
				<small>{t('Paste a music.youtube.com or youtu.be address.')}</small>
				<div><HugeiconsIcon icon={Link01Icon} class="h-4 w-4" /><Input bind:value={url} placeholder="https://music.youtube.com/…" autofocus spellcheck={false} /><button type="button" onclick={paste} title={t('Paste from clipboard')}><HugeiconsIcon icon={Copy01Icon} class="h-4 w-4" /><span>{t('PASTE')}</span></button></div>
			</label>

			<div class="ryo-link-readout" aria-live="polite">
				<div><span>STATE</span><strong>{url.trim() ? (target ? 'RESOLVED' : 'WAITING') : 'READY'}</strong></div>
				<div><span>TYPE</span><strong>{target?.kind?.toUpperCase() ?? '—'}</strong></div>
				<div><span>ID</span><strong>{target?.id ? target.id.slice(0, 18) : '—'}</strong></div>
			</div>

			<div class="ryo-overlay-note"><span>ACCEPTS</span><strong>SONG · ALBUM · ARTIST · PLAYLIST</strong></div>

			<footer class="ryo-overlay-actions">
				<Button type="button" variant="outline" onclick={() => (ui.linkOpen = false)}>{t('CANCEL')}</Button>
				<Button type="submit" disabled={!target}>{t('OPEN')} <HugeiconsIcon icon={ArrowRight01Icon} class="h-3.5 w-3.5" /></Button>
			</footer>
		</form>
	</Dialog.Content>
</Dialog.Root>
