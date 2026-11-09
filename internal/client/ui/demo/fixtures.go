package demo

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ryoku-dev/ryotunes/internal/client/state"
	"github.com/ryoku-dev/ryotunes/internal/eq"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

// Catalogue IDs are bare YouTube Music identifiers used by demo detail routes.
const (
	ArtistID   = "UCkA2M8xJQ6nR5vT9sP3wL7q"
	AlbumID    = "MPREb_KiteMineralLines"
	PlaylistID = "PL_KiteNightSignals"
)

// SpectrumLevels returns the fixed 64-band demo analyser frame.
func SpectrumLevels() []float32 {
	shape := [...]float32{.18, .24, .33, .46, .61, .76, .9, .72, .55, .41, .3, .22, .28, .39, .58, .8}
	levels := make([]float32, 64)
	for index := range levels {
		level := shape[index%len(shape)] - float32(index/16)*.035
		levels[index] = max(.08, level)
	}
	return levels
}

type fixtures struct {
	snapshot         protocol.Snapshot
	tracks           []model.Track
	cards            []model.BrowseItem
	library          []model.BrowseItem
	feed             model.Feed
	focusFeed        model.Feed
	playlist         model.PlaylistPage
	album            model.AlbumPage
	artist           model.ArtistPage
	lyrics           model.Lyrics
	radio            []model.RadioStation
	logs             model.LogsResult
	stats            model.ListeningStats
	equalizerPresets []model.EQPreset
}

func makeFixtures(directory string, playing bool) (fixtures, error) {
	artwork, err := writeArtwork(directory)
	if err != nil {
		return fixtures{}, err
	}

	tracks := make([]model.Track, 12)
	titles := []string{"Stone in the water", "Night train", "Soft voltage", "Aperture", "Borrowed light", "Still room", "Northbound", "Paper satellite", "Low tide", "Signal path", "Warm circuit", "After image"}
	artists := []string{"Kite Assembly", "Mira North", "Glass Harbour", "Low Weather"}
	for index := range tracks {
		albumName := []string{"Mineral lines", "Distance studies", "Quiet machines"}[index%3]
		duration := fmt.Sprintf("%d:%02d", 3+index%3, (index*13+17)%60)
		if index == 0 {
			duration = "3:18"
		}
		thumbnail := artwork[index%len(artwork)]
		albumID := fmt.Sprintf("album-%d", index%3+1)
		artistID := fmt.Sprintf("artist-%d", index%len(artists)+1)
		plays := fmt.Sprintf("%d.%dM", 1+index/4, 2+index%7)
		rating := model.RatingIndifferent
		if index == 0 {
			rating = model.RatingLike
		}
		tracks[index] = model.Track{
			VideoID: fmt.Sprintf("demo-track-%02d", index+1), Title: titles[index], Artists: artists[index%len(artists)],
			ArtistID: &artistID, Album: &albumName, AlbumID: &albumID, Duration: &duration, Thumbnail: &thumbnail,
			ArtistRuns: []model.Artist{{Text: artists[index%len(artists)], ID: &artistID}}, Explicit: index == 7,
			PlayCount: &plays, Plays: &plays, Rating: &rating,
		}
	}

	cards := make([]model.BrowseItem, 32)
	kinds := []string{"album", "artist", "playlist", "song"}
	for index := range cards {
		kind := kinds[index%len(kinds)]
		subtitle := artists[index%len(artists)]
		thumbnail := artwork[index%len(artwork)]
		title := fmt.Sprintf("%s %d", map[string]string{"album": "Record", "artist": "Artist", "playlist": "Mix", "song": "Track"}[kind], index/4+1)
		cards[index] = model.BrowseItem{
			Kind: kind, ID: fmt.Sprintf("demo-%s-%02d", kind, index+1), Title: title,
			Subtitle: &subtitle, Thumbnail: &thumbnail, Explicit: kind == "song" && index%8 == 7,
		}
	}
	shelves := make([]model.Shelf, 3)
	for index, title := range []string{"New from your artists", "Deep focus", "Evening records"} {
		more := fmt.Sprintf("demo-shelf-%d", index+1)
		shelves[index] = model.Shelf{Title: title, Items: append([]model.BrowseItem(nil), cards[index*8:index*8+8]...), MoreBrowseID: &more}
	}
	feed := model.Feed{
		Chips:    []model.FeedChip{{Title: "Energise", Params: "energy"}, {Title: "Focus", Params: "focus"}, {Title: "Unwind", Params: "unwind"}},
		Sections: shelves,
	}
	focusFeed := model.Feed{
		Chips: feed.Chips,
		Sections: []model.Shelf{
			{Title: "Focus rotation", Items: append([]model.BrowseItem(nil), cards[16:24]...)},
			{Title: "Quiet signals", Items: append([]model.BrowseItem(nil), cards[24:32]...)},
		},
	}

	likedSubtitle := "128 songs"
	liked := model.BrowseItem{Kind: "playlist", ID: "RYOTUNES_LIKED", Title: "Liked songs", Subtitle: &likedSubtitle, Thumbnail: &artwork[7]}
	library := []model.BrowseItem{liked}
	library = append(library, cardsOfKind(cards, "playlist")[:8]...)

	sourceName := "Mineral lines"
	queue := model.QueueSnapshot{
		Items: append([]model.Track(nil), tracks[:11]...), CurrentIndex: 0, PlayedFrom: 0,
		Repeat: model.RepeatOff, SourceName: &sourceName,
	}
	for index := 1; index <= 6; index++ {
		queue.Items[index].Queued = true
		queue.Items[index].QueuedFrom = &sourceName
	}
	for index := 7; index < len(queue.Items); index++ {
		queue.Items[index].Autoplay = true
	}
	now := model.PlaybackTrack{
		VideoID: tracks[0].VideoID, Title: tracks[0].Title, Artists: tracks[0].Artists,
		ArtistID: tracks[0].ArtistID, ArtistRuns: tracks[0].ArtistRuns, Thumbnail: tracks[0].Thumbnail,
		Duration: tracks[0].Duration, StreamClient: "Web Remixed", Rating: tracks[0].Rating,
		Plays: tracks[0].Plays, Genre: new("Alternative"),
	}
	settings := model.SettingsSnapshot{
		"volume": "68", "proxy": "", "quality": "AUTO", "enable_history": "true", "discord_rpc": "true",
		"close_to_tray": "true", "autostart": "true", "autoplay": "true", "prevent_duplicates": "true",
		"lyrics_boidu": "true", "lyrics_romanize": "true", "low_resource_mode": "false",
		"disabled_stream_clients": "", "ui_scale": "100", "discord_presence_name": "Ryotunes",
	}
	name, handle, email, channel := "Nero", "@nero", "nero@example.invalid", "demo-channel"
	account := model.Account{SignedIn: true, Name: &name, Handle: &handle, Email: &email, ChannelID: &channel}

	nowUnix := time.Now().Unix()
	downloadSettings := model.DownloadSettings{Path: filepath.Join(directory, "Music"), Format: model.DownloadOpus, Workers: 2, OrganizeByArtist: true, EmbedMetadata: true}
	if err := os.MkdirAll(downloadSettings.Path, 0o700); err != nil {
		return fixtures{}, err
	}
	collection, collectionKind := "Mineral lines", "album"
	downloads := model.DownloadSnapshot{ActiveCount: 2}
	for index, progress := range []int{62, 28} {
		track := tracks[index+3]
		downloads.Jobs = append(downloads.Jobs, model.DownloadEntry{
			ID: fmt.Sprintf("demo-download-active-%d", index+1), VideoID: track.VideoID, Title: track.Title,
			Artists: track.Artists, Thumbnail: *track.Thumbnail, Status: model.DownloadStatusDownloading,
			Progress: progress, Source: "YouTube Music", Collection: &collection, CollectionKind: &collectionKind,
			CreatedAt: nowUnix - int64(90+index*45), Settings: downloadSettings,
		})
	}
	for index := range 6 {
		track := tracks[index]
		path := filepath.Join(downloadSettings.Path, fmt.Sprintf("%02d %s.opus", index+1, track.Title))
		file, createErr := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
		if createErr != nil {
			return fixtures{}, createErr
		}
		size := int64(4+index) * 1024 * 1024
		if truncateErr := file.Truncate(size); truncateErr != nil {
			_ = file.Close()
			return fixtures{}, truncateErr
		}
		if closeErr := file.Close(); closeErr != nil {
			return fixtures{}, closeErr
		}
		finishedAt := nowUnix - int64(3600+index*900)
		downloads.Jobs = append(downloads.Jobs, model.DownloadEntry{
			ID: fmt.Sprintf("demo-download-saved-%d", index+1), VideoID: track.VideoID, Title: track.Title,
			Artists: track.Artists, Thumbnail: *track.Thumbnail, Status: model.DownloadStatusCompleted,
			Progress: 100, Source: "YouTube Music", FilePath: &path, CreatedAt: finishedAt - 120,
			FinishedAt: &finishedAt, Settings: downloadSettings,
		})
	}
	failure := "The source stopped responding. Retry when the connection is stable."
	failedTrack := tracks[8]
	downloads.Jobs = append(downloads.Jobs, model.DownloadEntry{
		ID: "demo-download-failed", VideoID: failedTrack.VideoID, Title: failedTrack.Title,
		Artists: failedTrack.Artists, Thumbnail: *failedTrack.Thumbnail, Status: model.DownloadStatusFailed,
		Progress: 41, Error: &failure, Source: "YouTube Music", CreatedAt: nowUnix - 500, Settings: downloadSettings,
	})

	description, privacy := "A patient record made from room tone, wire and low light.", "PRIVATE"
	selectedSort := model.PlaylistSortNewest
	playlistTitle, playlistSubtitle := "Night signals", "12 songs · 46 min"
	playlist := model.PlaylistPage{
		Title: &playlistTitle, Subtitle: &playlistSubtitle, Thumbnail: &artwork[1], Description: &description,
		Privacy: &privacy, Cover: &artwork[1], Items: append([]model.Track(nil), tracks...), Owned: true,
		SortMenu: &model.PlaylistSortMenu{Selected: &selectedSort, Editable: true},
	}
	albumTitle, albumSubtitle := "Mineral lines", "Kite Assembly · 2026 · 12 songs · 46 min"
	album := model.AlbumPage{
		Title: &albumTitle, Subtitle: &albumSubtitle, Thumbnail: &artwork[0], Description: &description,
		Cover: &artwork[0], Items: append([]model.Track(nil), tracks...),
	}
	artistName, subscribers, listeners, topID, radioID := "Kite Assembly", "128K subscribers", "482K monthly listeners", "demo-top-songs", "demo-artist-radio"
	sectionTitles := []string{"Albums", "Singles and EPs", "Videos", "Featured on", "Fans also like"}
	sectionKinds := []string{"album", "album", "song", "playlist", "artist"}
	artistSections := make([]model.ArtistSection, len(sectionTitles))
	for index := range artistSections {
		items := cardsOfKind(cards, sectionKinds[index])
		more := fmt.Sprintf("demo-artist-section-%d", index+1)
		artistSections[index] = model.ArtistSection{Title: sectionTitles[index], Items: append([]model.BrowseItem(nil), items...), MoreBrowseID: &more}
	}
	artist := model.ArtistPage{
		Name: &artistName, Thumbnail: &artwork[2], Description: &description, Subscribers: &subscribers,
		MonthlyListeners: &listeners, ChannelID: ArtistID, Subscribed: false, RadioPlaylistID: &radioID,
		TopSongs: append([]model.Track(nil), tracks[:5]...), TopSongsID: &topID, Sections: artistSections,
	}

	lyrics := model.Lyrics{Source: "YouTube Music", Synced: true}
	lyricText := []string{
		"Stone in the water", "A line beneath the blue", "夜汽車 carries the quiet home",
		"Every quiet signal finds another room", "Hold the borrowed light",
		"Let the night train through every open window until the shoreline answers in copper",
		"Nothing has to hurry when the copper sky turns", "Nothing has to prove",
		"Turn toward the shoreline", "Hear the copper ring", "Carry what is quiet", "Leave the current singing",
	}
	for index, line := range lyricText {
		start := uint64(50_000 + index*9_000)
		end := start + 8_000
		entry := model.LyricsLine{
			TimeMS: new(start), EndTimeMS: new(end), Text: line,
			Romanized: new(fmt.Sprintf("Reading %02d", index+1)),
		}
		if index%3 == 0 {
			fields := strings.Fields(line)
			span := end - start - 500
			for wordIndex, word := range fields {
				wordStart := start + span*uint64(wordIndex)/uint64(len(fields))
				wordEnd := start + span*uint64(wordIndex+1)/uint64(len(fields))
				entry.Words = append(entry.Words, model.LyricsWord{Text: word, StartMS: wordStart, EndMS: wordEnd})
			}
		}
		if index == 4 {
			entry.Translation = new("Keep the light we were lent")
		}
		lyrics.Lines = append(lyrics.Lines, entry)
	}

	radio := make([]model.RadioStation, 12)
	countries := []string{"Japan", "Iceland", "Canada", "Germany"}
	codecs := []string{"AAC", "MP3", "OGG"}
	tags := []string{"ambient", "jazz", "electronic", "classical"}
	for index := range radio {
		radio[index] = model.RadioStation{
			StationUUID: fmt.Sprintf("demo-radio-%d", index+1), Name: fmt.Sprintf("Signal %02d", index+1),
			StreamURL: "https://radio.invalid/stream", URL: "https://radio.invalid/stream",
			Homepage: "https://radio.invalid", Country: countries[index%len(countries)], CountryCode: []string{"JP", "IS", "CA", "DE"}[index%4],
			Tags: tags[index%len(tags)], Codec: codecs[index%len(codecs)], Bitrate: int64(96 + index%4*32),
			Votes: int64(90 + index*13), ClickCount: int64(500 + index*71), Favicon: artwork[index%len(artwork)],
		}
	}
	logEntries := []model.LogEntry{
		{Timestamp: time.Now().Add(-3 * time.Minute).Format(time.RFC3339), Level: "info", Source: "daemon", Target: "player", Message: "Playback restored"},
		{Timestamp: time.Now().Add(-2 * time.Minute).Format(time.RFC3339), Level: "warn", Source: "daemon", Target: "provider:youtube", Message: "Stream fallback selected"},
		{Timestamp: time.Now().Add(-time.Minute).Format(time.RFC3339), Level: "error", Source: "client", Target: "art", Message: "One cover was unavailable"},
	}
	logs := model.LogsResult{
		Entries: logEntries, Total: len(logEntries), Errors: 1, Warnings: 1, Path: filepath.Join(directory, "demo.log"),
		FileBytes: 18432, EngineVersion: "mpv 0.40.0", MPRIS: true,
	}
	stats := model.ListeningStats{
		Period: model.ListeningWeek, Plays: 42, KnownDurationSeconds: 9820,
		TopArtists: []model.ArtistPlayCount{{Name: "Kite Assembly", Plays: 14}},
		TopTracks:  []model.TrackPlayCount{{Title: tracks[0].Title, Artists: tracks[0].Artists, Plays: 6}},
	}

	manual := true
	personalState := state.EmptyPersonal()
	for index := 0; index < 4; index++ {
		personalState.Picks = append(personalState.Picks, state.Pick{
			BrowseItem: cards[index*4+2], LastUsedAt: time.Now().Add(-time.Duration(index+1) * time.Hour).UnixMilli(), Manual: &manual,
		})
	}
	personalState.Saved = append(personalState.Saved, library...)
	personalState.Pins = []string{library[1].ID, library[2].ID, library[3].ID}
	for index := 0; index < 4; index++ {
		track := tracks[index]
		item := model.BrowseItem{Kind: "song", ID: track.VideoID, Title: track.Title, Subtitle: &track.Artists, Thumbnail: track.Thumbnail}
		personalState.Recent[track.VideoID] = state.RecentItem{BrowseItem: item, ArtistID: track.ArtistID, At: time.Now().Add(-time.Duration(index+1) * 20 * time.Minute).UnixMilli()}
	}
	for index, artistName := range artists {
		artistID := fmt.Sprintf("artist-%d", index+1)
		personalState.Artists[artistID] = state.ArtistStat{Name: artistName, Count: 7 - index, LastPlayedAt: time.Now().Add(-time.Duration(index+1) * time.Hour).UnixMilli()}
	}
	personalState.Home.Order = []string{"shortcuts", "continue-listening", "picked-for-you"}
	personal, err := state.MarshalPersonal(personalState)
	if err != nil {
		return fixtures{}, err
	}
	premium := true
	spotifyName, soundcloudName, tidalName, tidalQuality := "Nero on Spotify", "nero", "Nero", "LOSSLESS"
	equalizer := model.Equalizer{
		Enabled: true, Preamp: -2.5, Preset: "Mineral lift",
		Bands: []model.EQBand{
			{Type: model.EQHighPass, Frequency: 32, Q: .71, Enabled: true},
			{Type: model.EQLowShelf, Frequency: 90, Gain: 3.5, Q: .8, Enabled: true},
			{Type: model.EQPeak, Frequency: 280, Gain: -2, Q: 1.2, Enabled: true},
			{Type: model.EQPeak, Frequency: 1200, Gain: 2.8, Q: 1.4, Enabled: true},
			{Type: model.EQPeak, Frequency: 4200, Gain: 3.2, Q: 1.1, Enabled: true},
			{Type: model.EQHighShelf, Frequency: 10500, Gain: -1.5, Q: .7, Enabled: true},
		},
	}
	equalizerPresets := eq.Builtins()
	equalizerPresets = append(equalizerPresets, model.EQPreset{
		Name: "Mineral lift", Preamp: equalizer.Preamp, Bands: append([]model.EQBand(nil), equalizer.Bands...),
	})
	snapshot := protocol.Snapshot{
		Playback: model.PlaybackSnapshot{Now: &now, Paused: !playing, Position: 105, Duration: 198, Volume: 68},
		Queue:    queue, Settings: settings, Auth: account, AudioFx: model.DefaultAudioFx(),
		Equalizer: equalizer, Provider: model.ProviderYouTube, Downloads: downloads, Personal: personal,
		Spotify:    model.SpotifyStatus{SignedIn: true, Stored: true, Name: &spotifyName, Premium: &premium},
		SoundCloud: model.SoundCloudStatus{SignedIn: true, Name: &soundcloudName},
		Tidal:      model.TidalStatus{SignedIn: true, Name: &tidalName, Quality: &tidalQuality, HasCredentials: true},
		LT:         model.LTState{Status: "idle", ServerURL: "wss://sync.ryotunes.invalid", Users: []model.LTUser{}, Queue: []model.LTTrack{}},
	}
	return fixtures{
		snapshot: snapshot, tracks: tracks, cards: cards, library: library, feed: feed, focusFeed: focusFeed,
		playlist: playlist, album: album, artist: artist, lyrics: lyrics, radio: radio, logs: logs, stats: stats,
		equalizerPresets: equalizerPresets,
	}, nil
}

func writeArtwork(directory string) ([]string, error) {
	grounds := []color.NRGBA{
		{R: 90, G: 95, B: 120, A: 255}, {R: 105, G: 120, B: 95, A: 255},
		{R: 120, G: 120, B: 90, A: 255}, {R: 105, G: 95, B: 120, A: 255},
		{R: 120, G: 95, B: 115, A: 255}, {R: 75, G: 95, B: 75, A: 255},
		{R: 105, G: 85, B: 105, A: 255}, {R: 100, G: 80, B: 100, A: 255},
	}
	vibrants := []color.NRGBA{
		{R: 239, G: 93, B: 72, A: 255}, {R: 76, G: 184, B: 218, A: 255},
		{R: 238, G: 167, B: 58, A: 255}, {R: 93, G: 206, B: 161, A: 255},
		{R: 92, G: 174, B: 232, A: 255}, {R: 235, G: 104, B: 176, A: 255},
		{R: 129, G: 91, B: 224, A: 255}, {R: 222, G: 118, B: 62, A: 255},
	}
	urls := make([]string, len(grounds))
	for index, ground := range grounds {
		path := filepath.Join(directory, fmt.Sprintf("art-%02d.png", index+1))
		file, err := os.Create(path)
		if err != nil {
			return nil, err
		}
		cover := image.NewNRGBA(image.Rect(0, 0, 192, 192))
		for y := range 192 {
			for x := range 192 {
				fill := ground
				if x >= 80 && x < 176 && y >= 32 && y < 152 && (x-80)+(y-32) >= 20 {
					fill = vibrants[index]
				}
				cover.SetNRGBA(x, y, fill)
			}
		}
		if err := png.Encode(file, cover); err != nil {
			_ = file.Close()
			return nil, err
		}
		if err := file.Close(); err != nil {
			return nil, err
		}
		urls[index] = "file://" + path
	}
	return urls, nil
}
