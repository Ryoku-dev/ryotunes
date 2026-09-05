//! Live SoundCloud smoke test (network test policy). NOT in the default run — it hits the real
//! guest api-v2, discovers a client_id, and exercises the whole path end to end:
//!   cargo test -p ryotunes-soundcloud -- --ignored --nocapture
//!
//! It searches "klingande messiah", then fetches the track, its stream URL, its waveform, the
//! artist, the artist's albums, and a chart — printing one line of evidence for each.

use ryotunes_soundcloud::{ChartKind, SoundCloud};

/// Host portion of a URL, for terse evidence output.
fn host_of(url: &str) -> &str {
    url.split("://").nth(1).and_then(|rest| rest.split('/').next()).unwrap_or(url)
}

#[tokio::test]
#[ignore = "hits the live SoundCloud api-v2; run with --ignored"]
async fn live_smoke() {
    let cache = std::env::temp_dir().join("ryotunes-soundcloud-smoke");
    let sc = SoundCloud::new(cache);

    // 1. Search.
    let page = sc.search_tracks("klingande messiah", 0).await.expect("search_tracks");
    assert!(!page.items.is_empty(), "search returned no tracks");
    let hit = page.items[0].clone();
    eprintln!("track id: {} — \"{}\" by {}", hit.id, hit.title, hit.user.username);

    // 2. Fetch the track by id.
    let track = sc.track(hit.id).await.expect("track");
    assert_eq!(track.id, hit.id, "track(id) returned a different track");
    assert!(!track.transcodings.is_empty(), "track has no transcodings");

    // 3. Stream URL — an HLS m3u8 on a SoundCloud CDN.
    let stream = sc.stream_url(&track).await.expect("stream_url");
    let host = host_of(&stream);
    eprintln!("stream url host: {host}");
    assert!(
        host.contains("sndcdn") || host.contains("soundcloud"),
        "stream host is not a SoundCloud CDN: {stream}"
    );
    assert!(stream.contains(".m3u8"), "stream is not an m3u8: {stream}");

    // 4. Waveform — 240 buckets, 0..=100.
    let wave = sc.waveform(&track).await.expect("waveform");
    eprintln!("waveform samples: {} (max {})", wave.len(), wave.iter().copied().max().unwrap_or(0));
    assert_eq!(wave.len(), 240, "waveform not downsampled to 240");
    assert!(wave.iter().all(|&s| s <= 100), "waveform sample out of 0..=100");

    // 5. The artist.
    let user = sc.user(track.user.id).await.expect("user");
    eprintln!(
        "user: {} — {} followers, {} tracks",
        user.username, user.followers, user.track_count
    );
    assert_eq!(user.id, track.user.id);

    // 6. The artist's albums.
    let albums = sc.user_albums(user.id).await.expect("user_albums");
    eprintln!("albums: {}", albums.len());
    match albums.first() {
        Some(a) => eprintln!("first album: \"{}\" (is_album={})", a.title, a.is_album),
        None => eprintln!("first album: <this artist has no albums>"),
    }

    // 7. A chart.
    let chart = sc.charts(ChartKind::Top, "soundcloud:genres:all-music").await.expect("charts");
    assert!(!chart.is_empty(), "chart came back empty");
    eprintln!("chart tracks: {}", chart.len());
    eprintln!("first chart track: \"{}\" by {}", chart[0].title, chart[0].user.username);
}
