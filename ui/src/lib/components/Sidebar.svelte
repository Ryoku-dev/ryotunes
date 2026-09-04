<script lang="ts">
	import { page } from '$app/state';
	import { HugeiconsIcon } from '@hugeicons/svelte';
	import {
		Home01Icon,
		Search01Icon,
		LibraryIcon,
		Settings01Icon,
		Add01Icon,
		PinIcon,
		MusicNote01Icon,
		Radio02Icon,
		ListRestartIcon,
		SquareArrowLeft01Icon,
		SquareArrowRight01Icon
	} from '@hugeicons/core-free-icons';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import * as Dialog from '$lib/components/ui/dialog';
	import { ON_REPEAT_ID, isSmartPlaylistId, type BrowseItem } from '$lib/api';
	import { thumb } from '$lib/thumb';
	import PlaylistMenu from './PlaylistMenu.svelte';
	import SmartPlaylistArt from './SmartPlaylistArt.svelte';
	import {
		auth,
		library,
		personal,
		ui,
		createLibraryPlaylist,
		toggleSidebar,
		toast
	} from '$lib/player.svelte';
	import { mergeSaved, orderLibrary } from '$lib/personal';
	import { normalizeSearchText } from '$lib/localsearch';
	import { t } from '$lib/i18n.svelte';

	const discoverNav = [
		{ href: '/', label: t('Home'), icon: Home01Icon, kana: '聴' },
		{ href: '/search', label: t('Search'), icon: Search01Icon, kana: '探' },
		{ href: '/radio', label: t('Radio'), icon: Radio02Icon, kana: '波' }
	];
	const collectionNav = { href: '/library', label: t('Library'), icon: LibraryIcon, kana: '蔵' };
	const isActive = (href: string) =>
		href === '/' ? page.url.pathname === '/' : page.url.pathname.startsWith(href);

	// Pinned first (in pin order), then everything else by last played. Derived here rather than in
	// the shared `library` store so the Library page keeps YouTube's own ordering. Playlists saved
	// on this machine sit in the same list: signed out they are the only ones there.
	const playlists = $derived(
		orderLibrary(mergeSaved(personal, library.items, 'playlist'), personal)
	);
	// How many of the leading rows are pinned — a rule under the last one explains the split.
	const pinnedCount = $derived(playlists.filter((p) => personal.pins.includes(p.id)).length);
	let playlistFilter = $state('');
	const visiblePlaylists = $derived.by(() => {
		const q = normalizeSearchText(playlistFilter);
		if (!q) return playlists;
		return playlists.filter((p) => normalizeSearchText(`${p.title} ${p.subtitle ?? ''}`).includes(q));
	});

	// YTM's library subtitle is "Owner • 20 tracks" and the rail is too narrow for both, so keep the
	// count and drop the rest. Subtitles without a number (albums: "Album • Artist") stay whole.
	const rowSubtitle = (s?: string) =>
		s
			?.split('•')
			.map((p) => p.trim())
			.filter((p) => /\d/.test(p))
			.at(-1) ?? s;

	const playlistHref = (item: BrowseItem) =>
		item.kind === 'album'
			? `/album/${encodeURIComponent(item.id)}`
			: item.kind === 'artist'
				? `/artist/${encodeURIComponent(item.id)}`
				: `/playlist/${encodeURIComponent(item.id)}`;

	// New-playlist dialog (mirrors the Library page).
	let dialogOpen = $state(false);
	let newTitle = $state('');
	let creating = $state(false);
	async function createNew() {
		const title = newTitle.trim();
		if (!title || creating) return;
		creating = true;
		try {
			await createLibraryPlaylist(title);
			toast.success(`${t('Created')} "${title}"`);
			newTitle = '';
			dialogOpen = false;
		} catch (e) {
			toast.error(String(e));
		} finally {
			creating = false;
		}
	}

	// Account lives in the titlebar now — see AccountMenu.svelte.

	// Manual collapse is a large-screen preference: below lg the rail is already collapsed by the
	// breakpoint, so the button is hidden there and `wide()` has nothing to drop. Every expanded
	// style is an `lg:` class, so collapsing is just not emitting them. The flag lives in `ui`
	// because the overlays that offset by the sidebar's width read it too.
	const collapsed = $derived(ui.sidebarCollapsed);
	const wide = (cls: string) => (collapsed ? '' : cls);
</script>

<aside
	class="ryo-sidebar flex h-full w-16 shrink-0 flex-col border-r bg-sidebar text-sidebar-foreground {wide(
		'lg:w-[16.75rem]'
	)}"
>
	<div class="ryo-rail-head">
		{#if !collapsed}
			<div class="ryo-rail-masthead hidden lg:block">
				<div class="ryo-rail-brand-row">
					<span class="ryo-rail-brand-mark">力</span>
					<div class="min-w-0 flex-1">
						<div class="ryo-rail-brand-name">RYOTUNES</div>
						<div class="ryo-rail-brand-sub">RYOKU // MUSIC</div>
					</div>
					<Button variant="ghost" size="icon-sm" onclick={toggleSidebar} aria-label={t('Collapse sidebar')} class="ryo-rail-collapse">
						<HugeiconsIcon icon={SquareArrowLeft01Icon} class="h-4 w-4" />
					</Button>
				</div>
				<span class="ryo-rail-triple">///</span>
			</div>
		{:else}
			<Button variant="ghost" size="icon-sm" class="hidden lg:inline-flex" onclick={toggleSidebar} aria-label={t('Expand sidebar')}>
				<HugeiconsIcon icon={SquareArrowRight01Icon} class="h-4 w-4" />
			</Button>
		{/if}
	</div>


	<nav class="ryo-sidebar-nav">
		<div class="ryo-rail-group-head hidden {wide('lg:flex')}">
			<span>01</span><strong>DISCOVER</strong><i></i><b>聴</b>
		</div>
		{#each discoverNav as n (n.href)}
			<a
				href={n.href}
				title={n.label}
				class="group relative flex items-center justify-center gap-3 rounded-md px-3 py-2 text-sm font-medium {wide('lg:justify-start')} {isActive(n.href) ? 'bg-primary text-primary-foreground' : 'text-sidebar-foreground/70 hover:bg-sidebar-accent/50 hover:text-sidebar-foreground'}"
			>
				<HugeiconsIcon icon={n.icon} class="h-5 w-5 shrink-0" />
				<span class="hidden flex-1 font-medium {wide('lg:inline')}">{isActive(n.href) ? '// ' : ''}{n.label}</span>
				<span class="hidden ryo-nav-kana {wide('lg:inline')}">{n.kana}</span>
			</a>
		{/each}

		<div class="ryo-rail-group-head mt-2 hidden {wide('lg:flex')}">
			<span>02</span><strong>COLLECTION</strong><i></i><b>蔵</b>
		</div>
		<a
			href={collectionNav.href}
			title={collectionNav.label}
			class="group relative flex items-center justify-center gap-3 rounded-md px-3 py-2 text-sm font-medium {wide('lg:justify-start')} {isActive(collectionNav.href) ? 'bg-primary text-primary-foreground' : 'text-sidebar-foreground/70 hover:bg-sidebar-accent/50 hover:text-sidebar-foreground'}"
		>
			<HugeiconsIcon icon={collectionNav.icon} class="h-5 w-5 shrink-0" />
			<span class="hidden flex-1 font-medium {wide('lg:inline')}">{isActive(collectionNav.href) ? '// ' : ''}{collectionNav.label}</span>
			<span class="hidden ryo-nav-kana {wide('lg:inline')}">{collectionNav.kana}</span>
		</a>

		<div class="ryo-rail-group-head mt-2 hidden {wide('lg:flex')}">
			<span>03</span><strong>SYSTEM</strong><i></i><b>設定</b>
		</div>
		<button
			onclick={() => (ui.settingsOpen = true)}
			title={t('Settings')}
			class="group flex items-center justify-center gap-3 rounded-md px-3 py-2 text-sm font-medium {ui.settingsOpen ? `bg-primary text-primary-foreground` : `text-sidebar-foreground/70 hover:bg-sidebar-accent/50 hover:text-sidebar-foreground`} {wide(`lg:justify-start`)}"
		>
			<HugeiconsIcon icon={Settings01Icon} class="h-5 w-5 shrink-0" />
			<span class="hidden flex-1 text-left font-medium {wide('lg:inline')}">{ui.settingsOpen ? '// ' : ''}{t('Settings')}</span>
			<span class="hidden ryo-nav-kana {wide('lg:inline')}">設</span>
		</button>
	</nav>

	
	<div class="ryo-rail-library hidden min-h-0 flex-1 flex-col {wide('lg:flex')}">
			<div class="ryo-rail-group-head ryo-rail-playlists-head"><span>04</span><strong>PLAYLISTS</strong><i></i><b>列</b></div>
			
			<Button
				variant="outline"
				size="sm"
				class="mb-2 w-full gap-2"
				onclick={() => (dialogOpen = true)}
			>
				<HugeiconsIcon icon={Add01Icon} class="h-4 w-4" /> {t('New playlist')}
			</Button>
			{#if playlists.length > 8}
				<div class="ryo-rail-playlist-filter">
					<Input bind:value={playlistFilter} aria-label={t('Filter playlists')} placeholder={t('Filter playlists…')} />
				</div>
			{/if}
			<div class="ryo-rail-playlist-scroll min-h-0 flex-1 overflow-y-auto" data-ryo-own-scroll>
				{#each visiblePlaylists as pl, i (pl.id)}
					
					<div class="group/row relative" data-ctx>
						<a
							href={playlistHref(pl)}
							title={pl.title}
							class="flex items-center gap-2.5 rounded-lg py-1.5 pl-2 pr-9 transition-colors hover:bg-sidebar-accent/50"
						>
							<div
								class="relative h-10 w-10 shrink-0 overflow-hidden bg-muted {pl.kind === 'artist'
									? 'rounded-full'
									: 'rounded-md'}"
							>
								{#if isSmartPlaylistId(pl.id)}
									<SmartPlaylistArt id={pl.id} />
								{:else if pl.thumbnail}
									<img src={thumb(pl.thumbnail, 96)} alt="" class="h-full w-full object-cover" loading="lazy" decoding="async" />
								{:else}
									<div class="flex h-full w-full items-center justify-center text-muted-foreground/50"><HugeiconsIcon icon={MusicNote01Icon} class="h-4 w-4" /></div>
								{/if}
							</div>
							{#if personal.pins.includes(pl.id)}
								<span
									class="absolute left-9 top-0.5 flex h-4 w-4 items-center justify-center rounded-full bg-primary text-primary-foreground shadow"
								>
									<HugeiconsIcon icon={PinIcon} class="h-2.5 w-2.5" />
								</span>
							{/if}
							<div class="min-w-0 flex-1">
								<div class="truncate text-[13px] font-medium">{pl.title}</div>
								{#if pl.subtitle}
									<div class="truncate text-xs text-muted-foreground">{rowSubtitle(pl.subtitle)}</div>
								{/if}
							</div>
						</a>
						<PlaylistMenu item={pl} />
					</div>
					{#if !playlistFilter.trim() && pinnedCount && i === pinnedCount - 1}
						<div class="mx-3 my-1.5 h-px bg-border"></div>
					{/if}
				{:else}
					{#if library.loading}
						<p class="px-3 py-1.5 text-xs text-muted-foreground">{t('Loading…')}</p>
					{:else if playlistFilter.trim()}
						<p class="px-3 py-3 text-xs text-muted-foreground">{t('No playlists match')} “{playlistFilter.trim()}”.</p>
					{/if}
				{/each}
			</div>
		</div>

		<Dialog.Root bind:open={dialogOpen}>
			<Dialog.Content class="ryo-overlay-sheet sm:max-w-md">
				<Dialog.Header>
					<Dialog.Title>{t('New playlist')}</Dialog.Title>
					<Dialog.Description>
						{t(auth.account?.signedIn
							? 'Give your YouTube Music playlist a name to get started.'
							: 'This playlist will be stored on this device. Sign in later without losing it.')}
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
						<Button type="button" variant="outline" onclick={() => (dialogOpen = false)}>{t('Cancel')}</Button>
						<Button type="submit" disabled={creating || !newTitle.trim()}>
							{t(creating ? 'Creating…' : 'Create')}
						</Button>
					</Dialog.Footer>
				</form>
			</Dialog.Content>
		</Dialog.Root>

	<div class="ryo-rail-foot hidden {wide('lg:block')}">
		<div class="ryo-rail-foot-rule"></div>
		<div class="ryo-rail-edition"><span>RYOKU</span><b>// MUSIC</b><em>LIVE</em></div>
		<div class="ryo-rail-barcode"></div>
		<div class="ryo-rail-barcode-label">RYOTUNES · DESKTOP</div>
	</div>

</aside>
