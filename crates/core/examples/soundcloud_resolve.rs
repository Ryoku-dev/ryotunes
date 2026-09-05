//! Live proof of the SoundCloud resolve path plus the `soundcloud_bridge` artist-page shape.
//!
//! Run (hits the network — needs internet):
//!   `cargo run -p ryotunes-core --example soundcloud_resolve`
//!
//! It resolves `sc:track:580008897` to an HLS stream URL (the same call `AppState::resolve` makes
//! for a `sc:track:` id) and prints it, then builds `soundcloud_bridge::artist_page` for the user
//! "Klingande" and prints the JSON keys the client binds to.

use ryotunes_core::soundcloud_bridge;
use ryotunes_soundcloud::SoundCloud;

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let cache = std::env::temp_dir().join("ryotunes-sc-example");
    let sc = SoundCloud::new(cache);

    // 1) Resolve sc:track:580008897 -> HLS m3u8 stream URL (mpv plays it directly).
    let track = sc.track(580_008_897).await?;
    let url = sc.stream_url(&track).await?;
    println!("track  : {} — {}", track.title, track.user.username);
    println!("stream : {url}");
    assert!(url.starts_with("https://"), "stream url should be a plain https m3u8");

    // 2) soundcloud_bridge::artist_page(user Klingande) — the get_artist shape for sc:user: ids.
    let users = sc.search_users("Klingande").await?;
    let user = users.into_iter().next().ok_or("no user found for Klingande")?;
    println!("user   : {} (sc:user:{}), {} followers", user.username, user.id, user.followers);
    let uid = user.id;
    let (tracks, albums, playlists, likes) = tokio::join!(
        sc.user_tracks(uid, 0),
        sc.user_albums(uid),
        sc.user_playlists(uid),
        sc.user_likes(uid),
    );
    let tracks = tracks.map(|p| p.items).unwrap_or_default();
    let page = soundcloud_bridge::artist_page(
        &user,
        &tracks,
        &albums.unwrap_or_default(),
        &playlists.unwrap_or_default(),
        &likes.unwrap_or_default(),
    );
    let json = serde_json::to_value(&page)?;
    let obj = json.as_object().expect("artist_page serializes to an object");
    let keys: Vec<&str> = obj.keys().map(String::as_str).collect();
    println!("get_artist(sc:user:) keys: {keys:?}");
    println!("{}", serde_json::to_string_pretty(&json)?);
    Ok(())
}
