//! Live Navidrome checks. Not in the default run, because they need a real server and account.
//!
//! Read-only on purpose: nothing is starred, scrobbled or written, so the run leaves the library
//! exactly as it was found.
//!
//!   NAVIDROME_TEST_SERVER=https://music.example.com \
//!   NAVIDROME_TEST_USER=admin \
//!   NAVIDROME_TEST_PASSWORD=secret \
//!     cargo test -p ryotunes-navidrome --test live_smoke -- --ignored --nocapture

use ryotunes_navidrome::{AlbumList, Navidrome};

#[tokio::test]
#[ignore]
async fn sign_in_and_read_the_catalogue() {
    let (Ok(server), Ok(user), Ok(password)) = (
        std::env::var("NAVIDROME_TEST_SERVER"),
        std::env::var("NAVIDROME_TEST_USER"),
        std::env::var("NAVIDROME_TEST_PASSWORD"),
    ) else {
        eprintln!("set NAVIDROME_TEST_SERVER, NAVIDROME_TEST_USER and NAVIDROME_TEST_PASSWORD");
        return;
    };

    let navidrome = Navidrome::new();
    let auth = navidrome
        .sign_in(&server, &user, &password)
        .await
        .expect("the server accepted the sign-in");
    assert!(navidrome.signed_in());
    assert_eq!(auth.base_url, server.trim().trim_end_matches('/'));
    assert_eq!(auth.username, user.trim());
    eprintln!("signed in to {} as {}", auth.base_url, auth.username);
    eprintln!("extensions = {:?}", navidrome.capabilities().extensions);

    navidrome.ping().await.expect("the session still answers a ping");

    let artists = navidrome.artists().await.expect("getArtists answered");
    eprintln!("artists = {}", artists.len());

    let albums = navidrome
        .albums(AlbumList::AlphabeticalByArtist, 0, 10)
        .await
        .expect("getAlbumList2 answered");
    eprintln!("albums = {}", albums.len());

    let playlists = navidrome.playlists().await.expect("getPlaylists answered");
    eprintln!("playlists = {}", playlists.len());

    let starred = navidrome.starred().await.expect("getStarred2 answered");
    eprintln!(
        "starred = {} songs, {} albums, {} artists",
        starred.songs.len(),
        starred.albums.len(),
        starred.artists.len()
    );

    let results = navidrome.search("a", 5).await.expect("search3 answered");
    eprintln!(
        "search = {} songs, {} albums, {} artists",
        results.songs.len(),
        results.albums.len(),
        results.artists.len()
    );

    // The album page is the one call that carries songs, so it is the only way to reach a stream
    // URL without writing anything.
    let album_id = albums
        .first()
        .map(|album| album.id.clone())
        .or_else(|| results.albums.first().map(|album| album.id.clone()));
    let Some(album_id) = album_id else {
        eprintln!("no albums on this server; the catalogue calls above are all that ran");
        return;
    };
    let album = navidrome.album(&album_id).await.expect("getAlbum answered");
    assert_eq!(album.id, album_id);
    eprintln!("album = {} ({} songs)", album.title(), album.song.len());

    if let Some(song) = album.song.first() {
        let url = navidrome.stream_url(&song.id).expect("a stream url builds");
        assert!(url.contains("/rest/stream.view"));
        assert!(url.contains("format=raw"), "{url}");
        let response = reqwest::get(&url).await.expect("the stream endpoint answered");
        eprintln!(
            "stream = {} {}",
            response.status(),
            response
                .headers()
                .get(reqwest::header::CONTENT_TYPE)
                .and_then(|value| value.to_str().ok())
                .unwrap_or("no content type")
        );
        assert!(response.status().is_success(), "{}", response.status());

        let lyrics = navidrome.lyrics(&song.id).await.expect("lyrics call answered");
        eprintln!("lyrics = {:?}", lyrics.map(|lyrics| lyrics.line.len()));
    }

    if let Some(playlist) = playlists.first() {
        let page = navidrome.playlist(&playlist.id).await.expect("getPlaylist answered");
        assert_eq!(page.id, playlist.id);
        assert_eq!(page.song_ids().len(), page.entry.len());
        eprintln!("playlist = {} ({} songs)", page.name, page.entry.len());
    }
}
