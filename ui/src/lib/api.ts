// The UI's only door to Rust. UI state contract — commands in, events out. The UI never
// touches YouTube; everything here is a Tauri command or event payload.
import { invoke } from '@tauri-apps/api/core';
import { listen, type UnlistenFn } from '@tauri-apps/api/event';

/** How the signed-in user rated a track (innertube `Rating`). The three states are mutually
 *  exclusive: liking a disliked track clears the dislike, and vice versa. */
export type Rating = 'like' | 'dislike' | 'indifferent';

/** One run of an artist line: its text, plus a channel id when that run links an artist. */
export interface ArtistRun {
	text: string;
	id?: string;
}

export interface SongItem {
	video_id: string;
	title: string;
	artists: string;
	/** Primary artist's channel browseId (`UC…`), when linked — makes the artist name navigable. */
	artist_id?: string;
	/** The artist line run by run — a collab links each name to its own page. Empty/absent when
	 * nothing is linked; render plain `artists` then. */
	artist_runs?: ArtistRun[];
	album?: string;
	/** The album's browseId (`MPRE…`), when linked — makes the album navigable. */
	album_id?: string;
	duration?: string;
	/** Play count as YouTube abbreviates it ("53M"). Album, artist and search rows. */
	play_count?: string;
	thumbnail?: string;
	/** Item id within a playlist when this account may remove that row. */
	set_video_id?: string;
	added_by?: string;
	added_by_avatar?: string;
	/** The signed-in user's rating (absent when the response didn't say — same as 'indifferent'). */
	rating?: Rating;
	/** Listen Together: name of the guest who added this queue item (session adds only). */
	queued_by?: string;
	/** Queued to play next ("Play next", or a guest's session add) — the "Next in queue" block. */
	queued?: boolean;
	/** Appended by "Add to queue" — its own block at the tail of the queue. */
	queued_end?: boolean;
	/** The album/playlist either block was added from, for its heading in the queue panel. */
	queued_from?: string;
	/** Appended by autoplay radio continuation — drives the queue's "Autoplay" divider + badge. */
	autoplay?: boolean;
	/** YouTube flags the track explicit. Browse/search rows only: `/next` carries no badge, so a
	 *  radio- or autoplay-appended track arrives without it. */
	explicit?: boolean;
	/** This row links a music video rather than the audio track. */
	is_video?: boolean;
	/** Privately-owned YouTube Music upload; requires an authenticated stream client. */
	is_upload?: boolean;
}

export interface NowPlaying {
	videoId: string;
	title: string;
	artists: string;
	artistId?: string;
	/** The artist line run by run — links each artist of a collab separately. */
	artistRuns?: ArtistRun[];
	thumbnail?: string;
	duration?: string;
	streamClient: string;
	/** The user's rating of the track (null if unknown). */
	rating?: Rating | null;
}

export type RepeatMode = 'off' | 'all' | 'one';

export interface QueueState {
	items: SongItem[];
	currentIndex: number;
	/** Start of the previously-played run: `items[playedFrom..currentIndex]` has actually been
	 *  heard. Not `0..currentIndex`: a playlist opened at track 7 has six untouched tracks first. */
	playedFrom?: number;
	shuffle?: boolean;
	repeat?: RepeatMode;
	/** What seeded the queue (playlist/album title, "<song> Radio") — the "Next from" header. */
	sourceName?: string | null;
}

export interface Account {
	signedIn: boolean;
	name?: string | null;
	handle?: string | null;
	email?: string | null;
	thumbnail?: string | null;
	channelId?: string | null;
	canSwitch?: boolean;
	/** The cookie authenticated, but a multi-channel login is not complete until one is chosen. */
	selectionRequired?: boolean;
}

export interface AccountIdentity {
	/** Opaque, process-local selector. Raw delegated/data-sync ids stay in Rust. */
	selectionKey: string;
	name: string;
	handle?: string | null;
	email?: string | null;
	thumbnail?: string | null;
	channelId?: string | null;
	selected: boolean;
}

export interface BrowseItem {
	kind: 'song' | 'playlist' | 'album' | 'artist';
	/** videoId (song) or browseId (playlist/album/artist). */
	id: string;
	title: string;
	subtitle?: string;
	thumbnail?: string;
	/** "3:47" — song items from a list-style shelf only (card shelves don't carry one). */
	duration?: string;
	/** Song cards only: the artist line run by run, so a card that gets played keeps its links. */
	artistRuns?: ArtistRun[];
	/** Play count as YouTube abbreviates it ("2.5B") — search song rows only. */
	playCount?: string;
	/** YouTube flags this track/album explicit. */
	explicit?: boolean;
	/** Song card is a privately-owned upload. */
	isUpload?: boolean;
}

export interface HomeSection {
	title: string;
	/** Header navigation resolves to an artist channel; display as a mixed recommendation shelf. */
	titleIsArtist?: boolean;
	items: BrowseItem[];
	moreBrowseId?: string;
	moreParams?: string;
}
/** A mood/genre filter chip above the home feed; `params` re-fetches home filtered to it. */
export interface HomeChip {
	title: string;
	params: string;
}
export interface HomePage {
	chips: HomeChip[];
	sections: HomeSection[];
	continuation?: string;
}

/**
 * The On Repeat auto-playlist's synthetic browseId (mirrors `ON_REPEAT_ID` in state.rs). It routes
 * like any other playlist; the only thing the UI does differently is draw an icon cover, because
 * a playlist built from local play counts has no artwork of its own.
 */
export const ON_REPEAT_ID = 'RYOTUNES_ON_REPEAT';
export const RECENTLY_PLAYED_ID = 'RYOTUNES_RECENTLY_PLAYED';
export const REDISCOVER_ID = 'RYOTUNES_REDISCOVER';
export const SMART_PLAYLIST_IDS = new Set([ON_REPEAT_ID, RECENTLY_PLAYED_ID, REDISCOVER_ID]);
export const isSmartPlaylistId = (id: string) => SMART_PLAYLIST_IDS.has(id);

/**
 * Liked Music's browseId. YouTube edits this one through the rating endpoint, not `edit_playlist`,
 * so it is never an add/remove/rename target: liking the song is the edit.
 */
export const LIKED_MUSIC_ID = 'VLLM';

/**
 * YouTube Music's own Library ▸ Songs, despite the name: the songs saved to the account's library.
 * It browses like a playlist (no header, no sort menu), so `getPlaylist` reads it and the Library
 * page's Songs tab pages through it with `getPlaylistMore`.
 */
export const LIBRARY_SONGS_ID = 'FEmusic_liked_videos';

/**
 * Local music (Rust `local.rs`). A file on disk is a song whose `video_id` is `LOCAL:<path>`, and
 * an album of them is a browseId `LOCALALBUM:<key>` — so local items ride every existing surface
 * (cards, queue, Shortcuts, the album page) and play with no network.
 */
export const LOCAL_SONG_PREFIX = 'LOCAL:';
export const LOCAL_ALBUM_PREFIX = 'LOCALALBUM:';
/** An artist on this disk. Renders through the album route: same page, no YouTube channel. */
export const LOCAL_ARTIST_PREFIX = 'LOCALARTIST:';
export const LOCAL_PLAYLIST_PREFIX = 'RYOTUNES_LOCAL_PLAYLIST:';
export const RADIO_ID_PREFIX = 'RYOTUNES_RADIO:';
export const isLocalPlaylistId = (id: string | undefined | null): boolean =>
	!!id && id.startsWith(LOCAL_PLAYLIST_PREFIX);
export const isRadioId = (id: string | undefined | null): boolean =>
	!!id && id.startsWith(RADIO_ID_PREFIX);
export const isLocalId = (id: string | undefined | null): boolean =>
	!!id &&
	(id.startsWith(LOCAL_SONG_PREFIX) ||
		id.startsWith(LOCAL_ALBUM_PREFIX) ||
		id.startsWith(LOCAL_ARTIST_PREFIX));

export interface LocalLibrary {
	/** Watched folders, as absolute paths. */
	folders: string[];
	albums: BrowseItem[];
	artists: BrowseItem[];
	songs: SongItem[];
	/** Song/album/artist ids that were in the library but are gone from disk since the last scan. */
	removed: string[];
}
export interface RadioStation {
	stationUuid: string;
	name: string;
	streamUrl: string;
	url?: string;
	homepage?: string;
	favicon?: string;
	country?: string;
	countryCode?: string;
	tags?: string;
	codec?: string;
	bitrate?: number;
	votes?: number;
	clickcount?: number;
}


/** The orders YouTube itself can put a playlist in — everything in `SortKey` but our own `plays`. */
export type ServerSort = 'default' | 'newest' | 'oldest' | 'title' | 'artist' | 'album' | 'top';

export interface SortMenu {
	/** The order YouTube has this list in right now, when it is one we have a name for. */
	selected?: ServerSort;
	/**
	 * The choice is a write, so storing it makes YouTube Music and every other client follow.
	 * Playlists you own only: elsewhere the menu is view-only (Liked Music remembers the last order
	 * asked for anyway, someone else's playlist does not).
	 */
	editable: boolean;
}

export interface PlaylistPage {
	title?: string;
	subtitle?: string;
	thumbnail?: string;
	/** The playlist's own blurb, which the edit dialog prefills its description with. */
	description?: string;
	/** `PUBLIC` / `PRIVATE` / `UNLISTED`. Only playlists you own report it. */
	privacy?: string;
	/** Custom artwork picked on this machine; falls back to `thumbnail` when unset. */
	cover?: string;
	items: SongItem[];
	continuation?: string;
	/** True only when the signed-in user owns this playlist (rename/delete allowed). */
	owned: boolean;
	/** Collaboration is enabled; per-row remove permission still comes from set_video_id. */
	collaborative?: boolean;
	/** Absent on lists YouTube will not reorder: albums, its own radio mixes, On Repeat. */
	sortMenu?: SortMenu;
}
export interface PlaylistContinuation {
	items: SongItem[];
	continuation?: string;
}

export interface ArtistCarousel {
	title: string;
	items: BrowseItem[];
	moreBrowseId?: string;
	moreParams?: string;
}
export interface SearchResults {
	top: BrowseItem[];
	songs: BrowseItem[];
	albums: BrowseItem[];
	artists: BrowseItem[];
	playlists: BrowseItem[];
	continuation?: string;
}
export interface SongSearchPage { items: SongItem[]; continuation?: string; }
export interface SearchCardPage { items: BrowseItem[]; continuation?: string; }

export interface AlbumPage {
	title?: string;
	artist?: string;
	artistId?: string;
	/** The artist line run by run — links each artist of a collaborative album separately. */
	artistRuns?: ArtistRun[];
	artistThumbnail?: string;
	subtitle?: string;
	secondSubtitle?: string;
	description?: string;
	thumbnail?: string;
	items: SongItem[];
	continuation?: string;
	/** The album itself is flagged explicit (the header wears the badge, not just some tracks). */
	explicit?: boolean;
	/** The album's audio playlist id (`OLAK5uy_…`) — autoplay's radio seed, and the save target. */
	playlistId?: string;
	/** Already saved to the signed-in user's library. */
	inLibrary: boolean;
	/** Card shelves under the tracks: other versions, more from the artist, related releases. */
	sections?: ArtistCarousel[];
}

export interface ArtistPage {
	name?: string;
	thumbnail?: string;
	description?: string;
	subscribers?: string;
	monthlyListeners?: string;
	channelId: string;
	subscribed: boolean;
	topSongs: SongItem[];
	/** `VL…` playlist of all the artist's top songs, behind the shelf's "See all". */
	topSongsId?: string;
	sections: ArtistCarousel[];
}

// --- commands (UI state) -----------------------------------------------------------------
export const search = (query: string) => invoke<SongItem[]>('search', { query });
export const searchPage = (query: string) => invoke<SongSearchPage>('search_page', { query });
export const searchPageMore = (token: string) => invoke<SongSearchPage>('search_page_more', { token });
/** Unfiltered search → categorized sections. */
export const searchAll = (query: string) => invoke<SearchResults>('search_all', { query });
export const searchAllMore = (token: string) => invoke<SearchResults>('search_all_more', { token });
/** Filtered "Show more" card search for one category (albums / artists / playlists). */
export const searchCards = (query: string, category: 'albums' | 'artists' | 'playlists') =>
	invoke<BrowseItem[]>('search_cards', { query, category });
export const searchCardsPage = (query: string, category: 'albums' | 'artists' | 'playlists') =>
	invoke<SearchCardPage>('search_cards_page', { query, category });
export const searchCardsMore = (token: string) => invoke<SearchCardPage>('search_cards_more', { token });
export const play = (item: SongItem) => invoke<void>('play', { item });
/** Warm the stream cache without changing playback. Used on settled search/hover intent. */
export const prefetchStream = (videoId: string, isUpload = false) =>
	invoke<void>('prefetch_stream', { videoId, isUpload });
export const playIndex = (index: number) => invoke<void>('play_index', { index });
/** Remove an upcoming track from the queue (host/local only — guests are add-only). */
export const removeFromQueue = (index: number) => invoke<void>('remove_from_queue', { index });
/** Drag-to-reorder: move the upcoming queue item at `from` to index `to` (both past the playing
 * track — the history and the playing row don't move). */
export const moveInQueue = (from: number, to: number) =>
	invoke<void>('move_in_queue', { from, to });
/**
 * "Play next": insert tracks at the front of the "Next in queue" block, behind any earlier
 * "Play next" adds. `from` is the album/playlist they came from — it heads the block in the panel.
 */
export const playNext = (items: SongItem[], from?: string) =>
	invoke<void>('play_next', { items, from });
/**
 * "Add to queue": the tracks go at the *back* of the same block — after everything already queued
 * by hand, ahead of the playing context and anything the app generated behind it.
 * `continuation` is the source page's next-page token — the backend walks the rest of a long
 * playlist into the queue in the background.
 */
export const addToQueue = (items: SongItem[], from?: string, continuation?: string) =>
	invoke<void>('add_to_queue', { items, from, continuation });
/** Clear every upcoming manually-queued track (the "Next in queue" section). */
export const clearQueued = () => invoke<void>('clear_queued');
export const nextTrack = () => invoke<void>('next_track');
export const prevTrack = () => invoke<void>('prev_track');
export const toggleShuffle = () => invoke<void>('toggle_shuffle');
export const setRepeat = (mode: RepeatMode) => invoke<void>('set_repeat', { mode });
/** One-shot: let the current song finish, then stop instead of advancing/autoplaying. */
export const setStopAfterCurrent = (enabled: boolean) =>
	invoke<void>('set_stop_after_current', { enabled });
export const togglePause = () => invoke<void>('toggle_pause');
export const seek = (position: number) => invoke<void>('seek', { position });
export const setVolume = (volume: number) => invoke<void>('set_volume', { volume });
/** Tempo (0.25–2.0) + pitch (−12..=12 semitones). Not persisted: resets on restart. */
export const setPlaybackParams = (speed: number, semitones: number) =>
	invoke<void>('set_playback_params', { speed, semitones });
export const getQueue = () => invoke<QueueState>('get_queue');
/** What the event stream already reported, for a webview that started after it did. */
export interface PlaybackSnapshot {
	now: NowPlaying | null;
	paused: boolean;
	position: number;
	duration: number;
	/** The level restored from last run (or the one another window already set). */
	volume: number;
	stopAfterCurrent?: boolean;
}
export const getPlayback = () => invoke<PlaybackSnapshot>('get_playback');

// --- settings (UI state) -----------------------------------------------------------------
export const getSettings = () => invoke<Record<string, string>>('get_settings');
export const setSetting = (key: string, value: string) =>
	invoke<void>('set_setting', { key, value });
export interface DiscordStatus { enabled: boolean; status: 'disabled' | 'connecting' | 'connected' | 'unavailable'; }
export const discordStatus = () => invoke<DiscordStatus>('discord_status');
/** Streamable client keys for the "disabled clients" setting. */
export const getStreamClients = () => invoke<string[]>('get_stream_clients');
/** Wipe both cache tiers (URL cache + mpv on-disk audio cache). */
export const clearCaches = () => invoke<void>('clear_caches');
/** Open an http(s) link in the real browser, never in the webview itself. */
export const openExternal = (url: string) => invoke<void>('open_external', { url });

// --- updates (self-update; shared Rust updater) --------------------------------------------
/**
 * A version check against the project's GitHub releases. `latestVersion` is a bare semver
 * (e.g. "1.0.1"), or null when no eligible release is published yet — that is not an error, only a
 * network failure is. `available` is true when a newer eligible release exists; `canInstall` is
 * true only where an in-app install is supported (a managed Arch/x86_64 package), with
 * `unsupportedReason` explaining why not otherwise. `notes` is plain release text; `releaseUrl` is
 * the changelog page.
 */
export interface UpdateInfo {
	currentVersion: string;
	latestVersion: string | null;
	available: boolean;
	releaseUrl: string;
	notes: string;
	canInstall: boolean;
	unsupportedReason: string | null;
}
export const checkForUpdates = () => invoke<UpdateInfo>('check_for_updates');
/**
 * Download, verify and install the requested release. The backend rechecks it against the exact
 * latest eligible version before fetching anything, installs only known release assets after a hash
 * check, and needs administrator approval (pkexec). `restartRequired` is true when the whole app
 * must be quit and reopened to reload the daemon and client. A network, hash or privilege failure
 * is a real error.
 */
export interface UpdateInstallResult {
	version: string;
	restartRequired: boolean;
}
export const installUpdate = (version: string) =>
	invoke<UpdateInstallResult>('install_update', { version });

// --- personal store (Home Shortcuts, sidebar pins, play recency; see personal.ts) -------------
/** The stored blob, wrapped as `{ personal }`; `personal` is `{}` when nothing is saved yet. */
export const getPersonal = () => invoke<{ personal: unknown }>('get_personal');
/** Replace the stored blob. The backend rejects a non-object or a blob over 1 MiB. */
export const setPersonal = (personal: unknown) => invoke<void>('set_personal', { personal });

// --- auth (authentication flow) ---------------------------------------------------------------------
export const getAccount = () => invoke<Account>('get_account');
export const getAccountIdentities = () =>
	invoke<AccountIdentity[]>('get_account_identities');
export const switchAccount = (selectionKey: string) =>
	invoke<Account>('switch_account', { selectionKey });
export const signOut = () => invoke<void>('sign_out');
/** Open the in-app Google sign-in webview (authentication flow Path A). Result arrives via onAuthChanged. */
export const loginWebview = () => invoke<void>('login_webview');

// --- window readiness / mini player -------------------------------------------------------------
/** Reveal a hidden WebView only after its Svelte tree has mounted. Native lifecycle owns the swap. */
export const frontendReady = (label: 'main' | 'mini') => invoke<void>('frontend_ready', { label });

// --- mini player (Rust mini.rs) ---------------------------------------------------------------
/** Hide the app to the tray and open the floating widget (a second window running this same SPA). */
export const openMini = () => invoke<void>('open_mini');
/** Close the widget and bring the app back. */
export const closeMini = () => invoke<void>('close_mini');

// --- browse / library (browse parser) ---------------------------------------------------------
/** `params` is a `HomeChip.params` token — omit for the unfiltered feed. */
export const getHome = (params?: string) => invoke<HomePage>('get_home', { params });
export const getHomeMore = (token: string) => invoke<HomePage>('get_home_more', { token });
export const getLibrary = () => invoke<BrowseItem[]>('get_library');
export const getLibraryAlbums = () => invoke<BrowseItem[]>('get_library_albums');
export const getLibraryArtists = () => invoke<BrowseItem[]>('get_library_artists');
/**
 * `sort` asks YouTube to order the tracks; omit it to get whatever order the account already has
 * the list in, which is the one a fresh visit wants (it is what YouTube Music would show).
 */
export const getPlaylist = (id: string, sort?: ServerSort, desc?: boolean) =>
	invoke<PlaylistPage>('get_playlist', { id, sort, desc });
/**
 * Store a sort order on a playlist, so YouTube Music and every other client show it the same way.
 * Only for a list whose `sortMenu.editable` is true.
 */
export const setPlaylistSort = (playlistId: string, sort: ServerSort) =>
	invoke<void>('set_playlist_sort', { playlistId, sort });
export const getPlaylistMore = (token: string) =>
	invoke<PlaylistContinuation>('get_playlist_more', { token });
/**
 * videoId → the ids of the playlists you own that hold it. Read straight from local SQLite, so it
 * answers instantly and is empty until `syncPlaylistIndex` has filled it in at least once.
 */
export const playlistIndex = () => invoke<Record<string, string[]>>('playlist_index');
/**
 * Re-walk your own playlists and answer with the rebuilt map. Skips the crawl while the stored one
 * is still inside its window, so calling this on every launch is cheap.
 */
export const syncPlaylistIndex = () => invoke<Record<string, string[]>>('sync_playlist_index');
/**
 * videoId → times played, from the local listening history. Same trailing window On Repeat uses
 * (a month): the history table is pruned to it, so there is no older data. A videoId that isn't in
 * the map has not been played inside the window.
 */
export const getPlayCounts = () => invoke<Record<string, number>>('play_counts');
export interface ListeningStats {
	period: 'day' | 'week' | 'month';
	plays: number;
	knownDurationSeconds: number;
	topArtists: { name: string; plays: number }[];
	topTracks: { title: string; artists: string; plays: number }[];
}
export const getListeningStats = (period: 'day' | 'week' | 'month') =>
	invoke<ListeningStats>('listening_stats', { period });
/**
 * `start`: the clicked track index, or `null` for "just play it" (random opener under shuffle).
 * `sourceId`: the page's playlist/album playlist id — makes autoplay continue with that
 * context's radio (omit to fall back to song radio seeded from the queue's last track).
 * `sourceName`: the page title, for the queue panel's "Next from" header.
 * `shuffle`: turn shuffle on for this queue — pass items in their real order, Rust shuffles.
 */
export const playPlaylist = (
	items: SongItem[],
	start: number | null,
	sourceId?: string,
	sourceName?: string,
	shuffle?: boolean,
	continuation?: string
) => invoke<void>('play_playlist', { items, start, sourceId, sourceName, shuffle, continuation });
/**
 * Start a radio: an endless YouTube-generated queue seeded on this item. `id` is the videoId
 * (song) or browseId/playlistId (everything else) — Rust resolves it to a radio playlist, so the
 * UI never builds one. `name` titles the queue ("<name> Radio").
 *
 * A song radio on the track that's already playing splices in behind it (no re-buffer); every
 * other case replaces the queue. Rejects when YouTube has no radio for the item.
 */
export const startRadio = (kind: 'song' | 'artist' | 'album' | 'playlist', id: string, name?: string) =>
	invoke<void>('start_radio', { kind, id, name });
/** Internet Radio directory. Rust handles mirror discovery, timeouts and validation. */
export const radioStations = (query = '', offset = 0, limit = 36) =>
	invoke<RadioStation[]>('radio_stations', { query, offset, limit });
export const playRadioStation = (station: RadioStation) =>
	invoke<void>('play_radio_station', { stationUuid: station.stationUuid });
export interface PlaylistTransfer { title: string; items: SongItem[]; }
export const exportPlaylistFile = (title: string, items: SongItem[]) =>
	invoke<boolean>('export_playlist_file', { title, items });
export const importPlaylistFile = () =>
	invoke<PlaylistTransfer | null>('import_playlist_file');
export const getAlbum = (id: string) => invoke<AlbumPage>('get_album', { id });
export const getArtist = (id: string) => invoke<ArtistPage>('get_artist', { id });
export const getBrowseGrid = (id: string, params?: string) =>
	invoke<BrowseItem[]>('get_browse_grid', { id, params });

// --- local music (local.rs) ------------------------------------------------------------------
/** Rescan the watched folders. Cheap when nothing changed (one stat per file). */
export const getLocalLibrary = () => invoke<LocalLibrary>('get_local_library');
export const addLocalFolder = () => invoke<LocalLibrary | null>('add_local_folder');
export const removeLocalFolder = (path: string) =>
	invoke<LocalLibrary>('remove_local_folder', { path });

// --- write actions (write API ✎) ----------------------------------------------------------
/** Like, dislike, or clear the rating. YouTube's three states are mutually exclusive, so a dislike
 *  un-likes in the same call. */
export const rate = (videoId: string, rating: Rating) => invoke<void>('rate', { videoId, rating });
/** `false` = the playlist already had this track, so YouTube added nothing. */
export const addToPlaylist = (playlistId: string, videoId: string) =>
	invoke<boolean>('add_to_playlist', { playlistId, videoId });
export const addToLocalPlaylist = (playlistId: string, item: SongItem) =>
	invoke<boolean>('add_to_local_playlist', { playlistId, item });
export const removeFromPlaylist = (playlistId: string, videoId: string, setVideoId: string) =>
	invoke<void>('remove_from_playlist', { playlistId, videoId, setVideoId });
export const createPlaylist = (title: string) => invoke<string>('create_playlist', { title });
/** Name / description / visibility, from the "Edit playlist" dialog. Leave a field out and
 *  YouTube is never told about it, so an untouched one can't be overwritten. */
export const editPlaylistDetails = (
	playlistId: string,
	changes: { name?: string; description?: string; public?: boolean }
) => invoke<void>('edit_playlist_details', { playlistId, ...changes });
/** Native picker/clear path for playlist artwork. WebKit never supplies a local filesystem path. */
export const setPlaylistCover = (playlistId: string, pick: boolean) =>
	invoke<{ cover?: string; thumbnail?: string } | null>('set_playlist_cover', { playlistId, pick });
export const deletePlaylist = (playlistId: string) =>
	invoke<void>('delete_playlist', { playlistId });
export const subscribe = (channelId: string, subscribed: boolean) =>
	invoke<void>('subscribe', { channelId, subscribed });
/** Save an album to the library (or remove it). `playlistId` is `AlbumPage.playlistId`. */
export const setAlbumSaved = (playlistId: string, saved: boolean) =>
	invoke<void>('set_album_saved', { playlistId, saved });

// --- events (UI state). Each returns an unlisten fn; call it on component teardown. --------
export const onNowPlaying = (cb: (n: NowPlaying) => void): Promise<UnlistenFn> =>
	listen<NowPlaying>('now-playing', (e) => cb(e.payload));
/** Backend correction for a playing track whose source row omitted or had stale likeStatus. */
export const onRating = (cb: (videoId: string, rating: Rating) => void): Promise<UnlistenFn> =>
	listen<{ videoId: string; rating: Rating }>('rating', (e) =>
		cb(e.payload.videoId, e.payload.rating)
	);
export const onQueueChanged = (cb: (q: QueueState) => void): Promise<UnlistenFn> =>
	listen<QueueState>('queue-changed', (e) => cb(e.payload));
/**
 * The queue moved but its track list did not: only the play pointer and the flags changed.
 * Emitted instead of `queue-changed` on every advance and skip, because the full item list is
 * megabytes on a big playlist and a Tauri event delivers its payload as JavaScript *source*.
 * `current` carries the playing row so a metadata backfill (duration, artists) still lands.
 */
export interface QueueIndex {
	currentIndex: number;
	playedFrom?: number;
	shuffle?: boolean;
	repeat?: RepeatMode;
	sourceName?: string | null;
	current: SongItem | null;
}

export const onQueueIndex = (cb: (q: QueueIndex) => void): Promise<UnlistenFn> =>
	listen<QueueIndex>('queue-index', (e) => cb(e.payload));
export const onPosition = (cb: (p: number) => void): Promise<UnlistenFn> =>
	listen<{ position: number }>('position', (e) => cb(e.payload.position));
export const onDuration = (cb: (d: number) => void): Promise<UnlistenFn> =>
	listen<{ duration: number }>('duration', (e) => cb(e.payload.duration));
/** Echo of every `set_volume`, so a second window's slider can't drift from what you hear. */
export const onVolume = (cb: (v: number) => void): Promise<UnlistenFn> =>
	listen<number>('volume', (e) => cb(e.payload));
export const onPlaybackState = (cb: (s: 'playing' | 'paused') => void): Promise<UnlistenFn> =>
	listen<'playing' | 'paused'>('playback-state', (e) => cb(e.payload));
export const onStopAfterCurrent = (cb: (enabled: boolean) => void): Promise<UnlistenFn> =>
	listen<boolean>('stop-after-current', (e) => cb(e.payload));
export const onPlaybackError = (cb: (msg: string) => void): Promise<UnlistenFn> =>
	listen<{ message: string }>('playback-error', (e) => cb(e.payload.message));
export const onPlaybackNotice = (cb: (msg: string) => void): Promise<UnlistenFn> =>
	listen<{ message: string }>('playback-notice', (e) => cb(e.payload.message));
/** Custom playlist artwork applied here but refused by YouTube Music (it syncs in the background,
 *  so the failure lands long after the picker closed). */
export const onCoverError = (cb: (msg: string) => void): Promise<UnlistenFn> =>
	listen<{ message: string }>('cover-error', (e) => cb(e.payload.message));
export const onAuthChanged = (cb: (a: Account) => void): Promise<UnlistenFn> =>
	listen<Account>('auth-changed', (e) => cb(e.payload));
export const onAccountSelectionRequired = (cb: () => void): Promise<UnlistenFn> =>
	listen('account-selection-required', () => cb());
/**
 * Local music disappeared from disk. Fired when a play attempt finds nothing there, carrying the
 * song (and album, if that emptied it) so every view holding those ids can drop them at once.
 */
export const onLocalChanged = (cb: (removed: string[]) => void): Promise<UnlistenFn> =>
	listen<{ removed: string[] }>('local-changed', (e) => cb(e.payload.removed));
export const onLoginError = (cb: (msg: string) => void): Promise<UnlistenFn> =>
	listen<string>('login-error', (e) => cb(e.payload));
export const onLoginDone = (cb: () => void): Promise<UnlistenFn> =>
	listen('login-done', () => cb());

// --- Ryoku live design tokens ---------------------------------------------------------------
export interface RyokuThemeTokens {
	detected: boolean;
	paper: string;
	paperLift: string;
	panel: string;
	card: string;
	sidebar: string;
	player: string;
	ink: string;
	inkDim: string;
	bone: string;
	inkOnBone: string;
	primary: string;
	onPrimary: string;
	primaryContainer: string;
	onPrimaryContainer: string;
	secondary: string;
	onSecondary: string;
	secondaryContainer: string;
	onSecondaryContainer: string;
	tertiary: string;
	onTertiary: string;
	tertiaryContainer: string;
	onTertiaryContainer: string;
	outline: string;
	outlineVariant: string;
	light: boolean;
	motionScale: number;
	reduceMotion: boolean;
	source: 'named' | 'wallpaper' | 'signature';
}
export const ryokuThemeTokens = () => invoke<RyokuThemeTokens>('ryoku_theme_tokens');
export const onRyokuThemeChanged = (cb: (tokens: RyokuThemeTokens) => void): Promise<UnlistenFn> =>
	listen<RyokuThemeTokens>('ryoku-theme-changed', (e) => cb(e.payload));

// --- lyrics ---------------------------------------------------------------------------------
export interface LyricWord {
	text: string;
	start_ms: number;
	end_ms: number;
}
export interface LyricLine {
	/** Start cue in milliseconds; present ⇔ the line is synced. */
	time_ms?: number;
	end_time_ms?: number;
	text: string;
	words?: LyricWord[];
	translation?: string;
}
export interface Lyrics {
	/** Attribution for the panel footer ("LRCLIB", "Source: Musixmatch", …). */
	source: string;
	synced: boolean;
	instrumental: boolean;
	lines: LyricLine[];
}
/** Cached on the Rust side (provider chain: LRCLIB → YT Music). `null` = none found. */
export const getLyrics = (args: {
	videoId: string;
	title: string;
	artists: string;
	album?: string;
	duration?: number;
}) => invoke<Lyrics | null>('get_lyrics', args);

// --- Last.fm scrobbling ---------------------------------------------------------------------
export interface LastfmState {
	configured: boolean;
	connected: boolean;
	username?: string | null;
	/** Set when a connect attempt failed (timeout, network, rejected) — show it as a toast. */
	error?: string | null;
}
export const lastfmStatus = () => invoke<LastfmState>('lastfm_status');
/** Opens the browser auth flow; the outcome arrives via onLastfmState, not this promise. */
export const lastfmConnect = () => invoke<void>('lastfm_connect');
/** Also cancels an in-flight connect (the auth poll checks and bails). */
export const lastfmDisconnect = () => invoke<void>('lastfm_disconnect');
export const onLastfmState = (cb: (s: LastfmState) => void): Promise<UnlistenFn> =>
	listen<LastfmState>('lastfm-state', (e) => cb(e.payload));

// --- Listen Together (session protocol) -----------------------------------------------------------
export interface LtUser {
	user_id: string;
	username: string;
	is_host: boolean;
	is_connected: boolean;
}
export interface LtTrack {
	id: string;
	title: string;
	artist: string;
	thumbnail?: string | null;
	duration_ms: number;
	/** Name of the guest who added this track to the session queue. */
	queued_by?: string | null;
}
export interface LtPendingJoin {
	userId: string;
	username: string;
}
export interface LtSuggestion {
	id: string;
	from_user_id: string;
	from_username: string;
	track: LtTrack;
}
export interface LtState {
	status: 'disconnected' | 'connecting' | 'connected';
	role: 'none' | 'host' | 'guest';
	/** Asked to create/join and awaiting the room (host approval) — show a waiting state. */
	requesting: boolean;
	roomCode: string | null;
	myId: string | null;
	serverUrl: string;
	users: LtUser[];
	currentTrack: LtTrack | null;
	queue: LtTrack[];
	pendingJoins: LtPendingJoin[];
	suggestions: LtSuggestion[];
}

export const ltGetState = () => invoke<LtState>('lt_get_state');
export const ltSetServerUrl = (url: string) => invoke<void>('lt_set_server_url', { url });
export const ltCreateRoom = (username: string) => invoke<void>('lt_create_room', { username });
export const ltJoinRoom = (code: string, username: string) =>
	invoke<void>('lt_join_room', { code, username });
export const ltLeave = () => invoke<void>('lt_leave');
export const ltApproveJoin = (userId: string) => invoke<void>('lt_approve_join', { userId });
export const ltRejectJoin = (userId: string) => invoke<void>('lt_reject_join', { userId });
export const ltKick = (userId: string) => invoke<void>('lt_kick', { userId });
export const ltTransferHost = (userId: string) => invoke<void>('lt_transfer_host', { userId });
export const ltApproveSuggestion = (id: string) => invoke<void>('lt_approve_suggestion', { id });
export const ltRejectSuggestion = (id: string) => invoke<void>('lt_reject_suggestion', { id });
export const ltRequestSync = () => invoke<void>('lt_request_sync');

export const onLtState = (cb: (s: LtState) => void): Promise<UnlistenFn> =>
	listen<LtState>('lt-state', (e) => cb(e.payload));
export const onLtNotice = (cb: (msg: string) => void): Promise<UnlistenFn> =>
	listen<string>('lt-notice', (e) => cb(e.payload));
