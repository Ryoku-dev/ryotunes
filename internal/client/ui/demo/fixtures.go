package demo

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"time"

	"github.com/ryoku-dev/ryotunes/internal/client/state"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

type fixtures struct {
	snapshot protocol.Snapshot
	tracks   []model.Track
	cards    []model.BrowseItem
	feed     model.Feed
	playlist model.PlaylistPage
	album    model.AlbumPage
	artist   model.ArtistPage
	lyrics   model.Lyrics
	radio    []model.RadioStation
	logs     model.LogsResult
	stats    model.ListeningStats
}

func makeFixtures(directory string) (fixtures, error) {
	artwork, err := writeArtwork(directory)
	if err != nil {
		return fixtures{}, err
	}
	tracks := make([]model.Track, 12)
	titles := []string{"Stone in the water", "Night train", "Soft voltage", "Aperture", "Borrowed light", "Still room", "Northbound", "Paper satellite", "Low tide", "Signal path", "Warm circuit", "After image"}
	artists := []string{"Kite Assembly", "Mira North", "Glass Harbour", "Low Weather"}
	for index := range tracks {
		album := []string{"Mineral lines", "Distance studies", "Quiet machines"}[index%3]
		duration := fmt.Sprintf("%d:%02d", 3+index%3, (index*13+17)%60)
		thumbnail := artwork[index%len(artwork)]
		albumID := fmt.Sprintf("album-%d", index%3+1)
		artistID := fmt.Sprintf("artist-%d", index%len(artists)+1)
		tracks[index] = model.Track{
			VideoID: fmt.Sprintf("demo-track-%02d", index+1), Title: titles[index], Artists: artists[index%len(artists)],
			ArtistID: &artistID, Album: &album, AlbumID: &albumID, Duration: &duration, Thumbnail: &thumbnail,
			ArtistRuns: []model.Artist{{Text: artists[index%len(artists)], ID: &artistID}}, Explicit: index == 7,
		}
	}
	cards := make([]model.BrowseItem, 32)
	kinds := []string{"album", "artist", "playlist", "song"}
	for index := range cards {
		subtitle := artists[index%len(artists)]
		thumbnail := artwork[index%len(artwork)]
		cards[index] = model.BrowseItem{Kind: kinds[index%len(kinds)], ID: fmt.Sprintf("demo-%s-%02d", kinds[index%len(kinds)], index+1), Title: titles[index%len(titles)], Subtitle: &subtitle, Thumbnail: &thumbnail}
	}
	shelves := make([]model.Shelf, 4)
	for index, title := range []string{"Continue listening", "Picked for you", "Familiar artists", "Forgotten favourites"} {
		more := fmt.Sprintf("demo-shelf-%d", index+1)
		shelves[index] = model.Shelf{Title: title, Items: append([]model.BrowseItem(nil), cards[index*8:index*8+8]...), MoreBrowseID: &more}
	}
	feed := model.Feed{Chips: []model.FeedChip{{Title: "Energise", Params: "energy"}, {Title: "Focus", Params: "focus"}, {Title: "Unwind", Params: "unwind"}}, Sections: shelves}

	queue := model.QueueSnapshot{Items: append([]model.Track(nil), tracks...), CurrentIndex: 0, PlayedFrom: 0, Repeat: model.RepeatOff}
	queue.Items[1].Queued = true
	queue.Items[2].Queued = true
	now := model.PlaybackTrack{VideoID: tracks[0].VideoID, Title: tracks[0].Title, Artists: tracks[0].Artists, ArtistID: tracks[0].ArtistID, ArtistRuns: tracks[0].ArtistRuns, Thumbnail: tracks[0].Thumbnail, Duration: tracks[0].Duration, StreamClient: "demo"}
	settings := model.SettingsSnapshot{
		"volume": "68", "proxy": "", "quality": "AUTO", "enable_history": "true", "discord_rpc": "false",
		"close_to_tray": "true", "autostart": "false", "autoplay": "true", "prevent_duplicates": "true",
		"lyrics_boidu": "true", "lyrics_romanize": "false", "low_resource_mode": "false",
		"disabled_stream_clients": "", "ui_scale": "110", "discord_presence_name": "Ryotunes",
	}
	name, handle, email, channel := "Nero", "@nero", "nero@example.invalid", "demo-channel"
	account := model.Account{SignedIn: true, Name: &name, Handle: &handle, Email: &email, ChannelID: &channel}
	nowUnix := time.Now().Unix()
	completedPath := filepath.Join(directory, "Stone in the water.opus")
	collection, collectionKind := "Mineral lines", "album"
	finishedAt := nowUnix - 7000
	downloads := model.DownloadSnapshot{Jobs: []model.DownloadEntry{
		{ID: "demo-download-1", VideoID: tracks[3].VideoID, Title: tracks[3].Title, Artists: tracks[3].Artists, Thumbnail: *tracks[3].Thumbnail, Status: model.DownloadStatusDownloading, Progress: 62, Source: "YouTube", Collection: &collection, CollectionKind: &collectionKind, CreatedAt: nowUnix - 90},
		{ID: "demo-download-2", VideoID: tracks[0].VideoID, Title: tracks[0].Title, Artists: tracks[0].Artists, Thumbnail: *tracks[0].Thumbnail, Status: model.DownloadStatusCompleted, Progress: 100, Source: "YouTube", FilePath: &completedPath, CreatedAt: nowUnix - 7200, FinishedAt: &finishedAt},
	}, ActiveCount: 1}

	title, subtitle, description, privacy, cover := "Mineral lines", "Kite Assembly · 12 songs", "A patient record made from room tone, wire and low light.", "PRIVATE", artwork[0]
	playlist := model.PlaylistPage{Title: &title, Subtitle: &subtitle, Thumbnail: &artwork[0], Description: &description, Privacy: &privacy, Cover: &cover, Items: append([]model.Track(nil), tracks...), Owned: true}
	album := playlist
	artistName, subscribers, topID := "Kite Assembly", "128K subscribers", "demo-top-songs"
	artist := model.ArtistPage{Name: &artistName, Thumbnail: &artwork[2], Description: &description, Subscribers: &subscribers, ChannelID: "demo-artist", TopSongs: append([]model.Track(nil), tracks[:5]...), TopSongsID: &topID, Sections: []model.ArtistSection{{Title: "Albums", Items: append([]model.BrowseItem(nil), cards[:8]...)}}}

	lyrics := model.Lyrics{Source: "demo", Synced: true}
	for index, line := range []string{"Stone in the water", "A line beneath the blue", "Every quiet signal", "Finds another room", "Hold the borrowed light", "Let the night train through", "Nothing has to hurry", "Nothing has to prove"} {
		start := uint64(index * 12000)
		end := start + 11000
		lyrics.Lines = append(lyrics.Lines, model.LyricsLine{TimeMS: &start, EndTimeMS: &end, Text: line})
	}

	radio := make([]model.RadioStation, 10)
	for index := range radio {
		radio[index] = model.RadioStation{StationUUID: fmt.Sprintf("demo-radio-%d", index+1), Name: fmt.Sprintf("Signal %02d", index+1), Country: []string{"Japan", "Iceland", "Canada"}[index%3], Tags: []string{"ambient", "jazz", "electronic"}[index%3], Codec: "AAC", Bitrate: int64(128 + index%3*64), Favicon: artwork[index%len(artwork)]}
	}
	logs := model.LogsResult{Entries: []model.LogEntry{{Timestamp: time.Now().Format(time.RFC3339), Level: "info", Source: "demo", Message: "Demo daemon ready"}}, Total: 1, Path: filepath.Join(directory, "demo.log")}
	stats := model.ListeningStats{Period: model.ListeningWeek, Plays: 42, KnownDurationSeconds: 9820, TopArtists: []model.ArtistPlayCount{{Name: "Kite Assembly", Plays: 14}}, TopTracks: []model.TrackPlayCount{{Title: tracks[0].Title, Artists: tracks[0].Artists, Plays: 6}}}
	manual := true
	personalState := state.EmptyPersonal()
	personalState.Picks = []state.Pick{
		{BrowseItem: cards[2], LastUsedAt: time.Now().Add(-2 * time.Hour).UnixMilli(), Manual: &manual},
		{BrowseItem: cards[6], LastUsedAt: time.Now().Add(-24 * time.Hour).UnixMilli(), Manual: &manual},
	}
	personalState.Saved = append(personalState.Saved, cards[0], cards[5])
	personalState.Pins = []string{cards[2].ID, cards[6].ID}
	recentItem := model.BrowseItem{Kind: "song", ID: tracks[0].VideoID, Title: tracks[0].Title, Subtitle: &tracks[0].Artists, Thumbnail: tracks[0].Thumbnail}
	personalState.Recent[tracks[0].VideoID] = state.RecentItem{BrowseItem: recentItem, ArtistID: tracks[0].ArtistID, At: time.Now().Add(-40 * time.Minute).UnixMilli()}
	if tracks[0].ArtistID != nil {
		personalState.Artists[*tracks[0].ArtistID] = state.ArtistStat{Name: tracks[0].Artists, Count: 7, LastPlayedAt: time.Now().Add(-40 * time.Minute).UnixMilli()}
	}
	personalState.Home.Order = []string{"shortcuts", "continue-listening", "picked-for-you"}
	personal, err := state.MarshalPersonal(personalState)
	if err != nil {
		return fixtures{}, err
	}
	snapshot := protocol.Snapshot{
		Playback: model.PlaybackSnapshot{Now: &now, Position: 74, Duration: 247, Volume: 68}, Queue: queue, Settings: settings,
		Auth: account, AudioFx: model.DefaultAudioFx(), Provider: model.ProviderYouTube, Downloads: downloads, Personal: personal,
		LT: model.LTState{Status: "idle", ServerURL: "wss://sync.ryotunes.invalid", Users: []model.LTUser{}, Queue: []model.LTTrack{}},
	}
	return fixtures{snapshot: snapshot, tracks: tracks, cards: cards, feed: feed, playlist: playlist, album: album, artist: artist, lyrics: lyrics, radio: radio, logs: logs, stats: stats}, nil
}

func writeArtwork(directory string) ([]string, error) {
	colors := []color.NRGBA{{R: 72, G: 92, B: 108, A: 255}, {R: 142, G: 105, B: 72, A: 255}, {R: 79, G: 111, B: 91, A: 255}, {R: 96, G: 83, B: 123, A: 255}, {R: 128, G: 73, B: 82, A: 255}, {R: 61, G: 104, B: 121, A: 255}, {R: 126, G: 116, B: 73, A: 255}, {R: 82, G: 82, B: 86, A: 255}}
	urls := make([]string, len(colors))
	for index, fill := range colors {
		path := filepath.Join(directory, fmt.Sprintf("art-%02d.png", index+1))
		file, err := os.Create(path)
		if err != nil {
			return nil, err
		}
		image := image.NewNRGBA(image.Rect(0, 0, 192, 192))
		for offset := range image.Pix {
			if offset%4 == 0 {
				image.Pix[offset], image.Pix[offset+1], image.Pix[offset+2], image.Pix[offset+3] = fill.R, fill.G, fill.B, fill.A
			}
		}
		if err := png.Encode(file, image); err != nil {
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
