//! Audible gapless check: `cargo run -p player --example play -- <fileA> <fileB>`
//! Plays two supplied audio URLs/files gaplessly and prints player events.

use std::collections::HashMap;

use player::{Player, PlayerEvent};

#[tokio::main]
async fn main() {
    let mut args = std::env::args().skip(1);
    let a = args.next().expect("usage: cargo run -p player --example play -- <track-a> <track-b>");
    let b = args.next().expect("usage: cargo run -p player --example play -- <track-a> <track-b>");

    let cache = std::env::temp_dir().join("ryotunes-player-example");
    std::fs::create_dir_all(&cache).ok();

    let mut p = Player::new(cache.to_str().unwrap()).expect("player");
    let mut events = p.take_events().unwrap();

    p.load(&a, &HashMap::new(), None, "A", &[]).expect("load A");
    p.enqueue(&b, "B").expect("enqueue B");
    p.play().expect("play");

    let mut ended = 0;
    let deadline = tokio::time::Instant::now() + std::time::Duration::from_secs(30);
    while tokio::time::Instant::now() < deadline {
        match tokio::time::timeout(std::time::Duration::from_secs(2), events.recv()).await {
            Ok(Some(PlayerEvent::TrackEnded)) => {
                ended += 1;
                println!("track ended ({ended}/2)");
                if ended >= 2 {
                    println!("OK: both tracks played gaplessly");
                    return;
                }
            }
            Ok(Some(ev)) => println!("event: {ev:?}"),
            Ok(None) => break,
            Err(_) => {}
        }
    }
    eprintln!("did not observe 2 track endings");
    std::process::exit(1);
}
