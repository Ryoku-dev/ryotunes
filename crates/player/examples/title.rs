//! Media-title probe: `cargo run -p player --example title -- <file> [title]`
//! Loads a file with an awkward per-file title (spaces, commas, quotes, non-ASCII) and asserts
//! that mpv reports it back verbatim.

use std::collections::HashMap;

use player::Player;

fn main() {
    let a = std::env::args().nth(1).expect("usage: cargo run -p player --example title -- <file>");
    let cache = std::env::temp_dir().join("ryotunes-player-example");
    std::fs::create_dir_all(&cache).ok();
    let p = Player::new(cache.to_str().unwrap()).expect("player");
    let title = std::env::args()
        .nth(2)
        .unwrap_or_else(|| "Ünïcode Artist, \"quoted\" – A Title".to_owned());
    p.load(&a, &HashMap::new(), None, &title, &[]).expect("load");
    let mut got = String::new();
    for _ in 0..40 {
        std::thread::sleep(std::time::Duration::from_millis(100));
        got = p.media_title().unwrap_or_default();
        if !got.is_empty() {
            break;
        }
    }
    println!("want: {title}");
    println!("got:  {got}");
    assert_eq!(got, title);
}
