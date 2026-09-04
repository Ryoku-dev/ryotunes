<script lang="ts">
	import * as Dialog from '$lib/components/ui/dialog';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { HugeiconsIcon } from '@hugeicons/svelte';
	import {
		Copy01Icon,
		Logout01Icon,
		Tick02Icon,
		Cancel01Icon,
		UserRemove01Icon,
		Exchange01Icon,
		CrownIcon,
		RefreshIcon
	} from '@hugeicons/core-free-icons';
	import * as api from '$lib/api';
	import { copyText } from '$lib/clipboard';
	import { auth, ui, toast } from '$lib/player.svelte';
	import { lt } from '$lib/lt.svelte';
	import { thumb } from '$lib/thumb';
	import { t } from '$lib/i18n.svelte';

	let mode = $state<'join' | 'host'>('join');
	let name = $state('');
	let serverUrl = $state('');
	let inviteInput = $state('');
	let busy = $state(false);

	// Seed inputs when the modal opens: remembered name + the persisted server URL (host mode).
	$effect(() => {
		if (ui.ltOpen) {
			const remembered = localStorage.getItem('lt_name')?.trim() ?? '';
			name = remembered || auth.account?.name?.trim() || '';
			serverUrl = lt.serverUrl;
		}
	});

	const inRoom = $derived(lt.role !== 'none');
	const isHost = $derived(lt.role === 'host');
	// The only thing worth showing or sending: the bare room code is useless to a guest who doesn't
	// already know the server URL, and every self-hosted server has a different one.
	const invite = $derived(makeInvite(lt.serverUrl, lt.roomCode ?? ''));
	// Sitting between "asked to join" and "in the room" — show a waiting state, block re-sends.
	const waiting = $derived(lt.requesting && lt.role === 'none');

	function rememberName() {
		localStorage.setItem('lt_name', name.trim());
	}

	// An invite bundles the server + code so a guest only pastes one thing. `RYO~<base64(server|code)>`.
	function makeInvite(server: string, code: string): string {
		return 'RYO~' + btoa(`${server}|${code}`);
	}
	function parseInvite(raw: string): { server: string; code: string } | null {
		const s = raw.trim();
		if (s.startsWith('RYO~')) {
			try {
				const [server, code] = atob(s.slice(5)).split('|');
				return { server: server ?? '', code: (code ?? '').toUpperCase() };
			} catch {
				return null;
			}
		}
		// A bare room code — reuse whatever server we last connected to.
		return { server: '', code: s.toUpperCase() };
	}

	async function host() {
		if (!name.trim()) return toast.error(t('Enter a name first'));
		const u = serverUrl.trim();
		if (!u) return toast.error(t('Enter your sync server URL'));
		busy = true;
		try {
			if (!/^wss?:\/\//i.test(u)) throw new Error(t('Use a ws:// or wss:// sync server address'));
			if (u !== lt.serverUrl) await api.ltSetServerUrl(u);
			rememberName();
			await api.ltCreateRoom(name.trim());
		} catch (e) {
			toast.error(String(e));
		} finally {
			busy = false;
		}
	}

	async function join(e?: Event) {
		e?.preventDefault();
		if (!name.trim()) return toast.error(t('Enter a name first'));
		const parsed = parseInvite(inviteInput);
		if (!parsed || !parsed.code) return toast.error(t('Paste the invite code your friend sent'));
		const server = parsed.server || lt.serverUrl;
		if (!server) return toast.error(t('Paste the full invite from the host, it carries the server address'));
		busy = true;
		try {
			if (!/^wss?:\/\//i.test(server)) throw new Error(t('The invite does not contain a valid sync server'));
			if (server !== lt.serverUrl) await api.ltSetServerUrl(server);
			rememberName();
			await api.ltJoinRoom(parsed.code, name.trim());
		} catch (e) {
			toast.error(String(e));
		} finally {
			busy = false;
		}
	}

	async function leave() {
		await api.ltLeave();
	}

	function copyInvite() {
		copyText(invite).then(
			() => toast.success(t('Invite copied, send it to a friend')),
			() => toast.error(t('Could not copy the invite'))
		);
	}
</script>

<Dialog.Root bind:open={ui.ltOpen}>
	<Dialog.Content class="ryo-overlay-sheet ryo-listen-sheet overflow-hidden p-0 sm:max-w-[680px]">
		<header class="ryo-overlay-head ryo-listen-titlebar">
			<div class="ryo-overlay-eyebrow"><span>—</span><b>力</b><strong>{t('SESSION / SYNC')}</strong><i></i><em>LT-01</em></div>
			<Dialog.Title>{t('Listen Together')}</Dialog.Title>
			<Dialog.Description>{t('Share one queue and one playback clock without turning music into a meeting.')}</Dialog.Description>
		</header>

		{#if waiting}
			<div class="ryo-listen-wait" aria-live="polite">
				<div class="ryo-listen-wait-mark"><i></i><i></i><i></i><i></i><i></i></div>
				<span>{t('// SESSION / HANDSHAKE')}</span>
				<strong>{t(lt.status === 'connecting' ? 'Connecting to the sync transport.' : 'Waiting for host approval.')}</strong>
				<p>{t('The music engine stays local until the session is accepted.')}</p>
				<div><b>{t('STATE')}</b><em>{lt.status.toUpperCase()}</em><b>{t('ROLE')}</b><em>{t('GUEST')}</em></div>
				<Button variant="outline" size="sm" onclick={leave}>{t('Cancel request')}</Button>
			</div>
		{:else if !inRoom}
			<div class="ryo-listen-setup">
				<div class="ryo-listen-seg" role="tablist" aria-label={t('Session mode')}>
					<button type="button" role="tab" aria-selected={mode === 'join'} class:active={mode === 'join'} onclick={() => (mode = 'join')}><span>01</span> {t('JOIN')}</button>
					<button type="button" role="tab" aria-selected={mode === 'host'} class:active={mode === 'host'} onclick={() => (mode = 'host')}><span>02</span> {t('HOST')}</button>
				</div>

				{#if mode === 'join'}
					<form class="ryo-listen-form" onsubmit={join}>
						<label><span>{t('INVITE')}</span><small>{t('Paste the code your friend sent. It already carries the server address.')}</small><Input bind:value={inviteInput} placeholder="RYO~…" /></label>
						<label><span>{t('NAME')}</span><small>{t('Shown to the other listeners in this room.')}</small><Input bind:value={name} placeholder={t('Your name')} /></label>
						<div class="ryo-listen-readout"><div><span>{t('ROLE')}</span><strong>{t('GUEST')}</strong></div><div><span>{t('TRANSPORT')}</span><strong>WEBSOCKET</strong></div><div><span>{t('QUEUE')}</span><strong>{t('FOLLOW HOST')}</strong></div></div>
						<div class="ryo-listen-actions"><Button type="submit" disabled={busy}>{t(busy ? 'CONNECTING…' : 'JOIN SESSION')}</Button></div>
					</form>
				{:else}
					<div class="ryo-listen-form">
						<label><span>{t('SYNC SERVER')}</span><small>{t('Your self-hosted WebSocket endpoint. It is remembered on this machine.')}</small><Input bind:value={serverUrl} placeholder="wss://relay.example.org/ws" /></label>
						<label><span>{t('NAME')}</span><small>{t('The host identity other listeners will see.')}</small><Input bind:value={name} placeholder={t('Your name')} /></label>
						<div class="ryo-listen-readout"><div><span>{t('ROLE')}</span><strong>{t('HOST')}</strong></div><div><span>{t('TRANSPORT')}</span><strong>WEBSOCKET</strong></div><div><span>{t('CONTROL')}</span><strong>LOCAL</strong></div></div>
						<div class="ryo-listen-actions"><Button onclick={host} disabled={busy}>{t(busy ? 'STARTING…' : 'START SESSION')}</Button></div>
					</div>
				{/if}
			</div>
		{:else}
			<div class="ryo-listen-room">
				<section class="ryo-listen-room-state">
					<div class="ryo-listen-room-head"><span>{t('// LIVE SESSION')}</span><b>{t(isHost ? 'HOST' : 'GUEST')} · {lt.status.toUpperCase()}</b></div>
					<div class="ryo-listen-invite"><span>{invite}</span><button type="button" onclick={copyInvite}><HugeiconsIcon icon={Copy01Icon} class="h-3.5 w-3.5" /> {t('COPY INVITE')}</button></div>
					{#if lt.currentTrack}
						<div class="ryo-listen-track">
							{#if lt.currentTrack.thumbnail}<img src={thumb(lt.currentTrack.thumbnail, 192)} alt="" decoding="async" />{/if}
							<div><span>{t('NOW PLAYING')}</span><strong>{lt.currentTrack.title}</strong><small>{lt.currentTrack.artist}</small></div>
						</div>
					{/if}
				</section>

				{#if isHost && lt.pendingJoins.length}
					<section class="ryo-listen-block">
						<div class="ryo-listen-block-head"><span>{t('// JOIN REQUESTS')}</span><b>{lt.pendingJoins.length}</b></div>
						{#each lt.pendingJoins as p (p.userId)}
							<div class="ryo-listen-person"><span class="ryo-listen-person-mark">?</span><strong>{p.username}</strong><div></div><button title={t('Approve')} onclick={() => api.ltApproveJoin(p.userId)}><HugeiconsIcon icon={Tick02Icon} class="h-4 w-4" /></button><button title={t('Reject')} onclick={() => api.ltRejectJoin(p.userId)}><HugeiconsIcon icon={Cancel01Icon} class="h-4 w-4" /></button></div>
						{/each}
					</section>
				{/if}

				<section class="ryo-listen-block">
					<div class="ryo-listen-block-head"><span>{t('// LISTENERS')}</span><b>{lt.users.length}</b></div>
					{#each lt.users as u (u.user_id)}
						<div class="ryo-listen-person" class:offline={!u.is_connected}>
							<span class="ryo-listen-person-mark">{u.is_connected ? '●' : '○'}</span>
							<strong>{u.username}{u.user_id === lt.myId ? t(' · YOU') : ''}</strong>
							<small>{t(u.is_host ? 'HOST' : 'LISTENER')}</small><div></div>
							{#if u.is_host}<HugeiconsIcon icon={CrownIcon} class="h-3.5 w-3.5" />{/if}
							{#if isHost && u.user_id !== lt.myId}
								<button title={t('Make host')} onclick={() => api.ltTransferHost(u.user_id)}><HugeiconsIcon icon={Exchange01Icon} class="h-4 w-4" /></button>
								<button title={t('Remove')} onclick={() => api.ltKick(u.user_id)}><HugeiconsIcon icon={UserRemove01Icon} class="h-4 w-4" /></button>
							{/if}
						</div>
					{/each}
				</section>

				{#if isHost && lt.suggestions.length}
					<section class="ryo-listen-block">
						<div class="ryo-listen-block-head"><span>{t('// SUGGESTIONS')}</span><b>{lt.suggestions.length}</b></div>
						{#each lt.suggestions as s (s.id)}
							<div class="ryo-listen-suggestion"><div><strong>{s.track.title}</strong><small>{s.track.artist} · {t('from')} {s.from_username}</small></div><button title={t('Accept')} onclick={() => api.ltApproveSuggestion(s.id)}><HugeiconsIcon icon={Tick02Icon} class="h-4 w-4" /></button><button title={t('Reject')} onclick={() => api.ltRejectSuggestion(s.id)}><HugeiconsIcon icon={Cancel01Icon} class="h-4 w-4" /></button></div>
						{/each}
					</section>
				{/if}

				<footer class="ryo-listen-footer">
					<div><span>{t('SYNC')}</span><strong>{lt.status.toUpperCase()}</strong></div>
					{#if !isHost}<Button variant="outline" size="sm" onclick={() => api.ltRequestSync()}><HugeiconsIcon icon={RefreshIcon} class="h-4 w-4" /> {t('RE-SYNC')}</Button>{/if}
					<Button variant="outline" size="sm" onclick={leave}><HugeiconsIcon icon={Logout01Icon} class="h-4 w-4" /> {t('LEAVE')}</Button>
				</footer>
			</div>
		{/if}
	</Dialog.Content>
</Dialog.Root>
