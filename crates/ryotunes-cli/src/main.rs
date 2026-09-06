//! `ryotunes-cli <method> [json-params]`, `ryotunes-cli events [names...]`, or the local,
//! socket-free `ryotunes-cli skin <check|list|show ...>` (see skin.rs).

use std::io::{BufRead, BufReader, Write};
use std::os::unix::net::UnixStream;

use ryotunes_protocol::{Outgoing, Request};

mod skin;

fn parse(args: &[String]) -> Result<(Option<std::path::PathBuf>, Request, bool), String> {
    let mut socket = None;
    let mut rest = args.to_vec();
    if rest.first().map(String::as_str) == Some("--socket") {
        rest.remove(0);
        socket = Some(rest.first().ok_or("--socket needs a path")?.into());
        rest.remove(0);
    }
    let method = rest
        .first()
        .ok_or("usage: ryotunes-cli [--socket PATH] <method> [json] | events [name...]")?
        .clone();
    if method == "events" {
        let names: Vec<String> = rest[1..].to_vec();
        let params = if names.is_empty() {
            serde_json::json!({ "events": ["*"] })
        } else {
            serde_json::json!({ "events": names })
        };
        return Ok((socket, Request { id: 1, method: "subscribe".into(), params }, true));
    }
    let params = match rest.get(1) {
        Some(raw) => serde_json::from_str(raw).map_err(|e| format!("params: {e}"))?,
        None => serde_json::Value::Null,
    };
    Ok((socket, Request { id: 1, method, params }, false))
}

fn main() {
    let args: Vec<String> = std::env::args().skip(1).collect();
    // `skin` is a local toolbox: it never touches the daemon socket.
    if args.first().map(String::as_str) == Some("skin") {
        std::process::exit(skin::run(&args[1..]));
    }
    let (socket, req, follow) = match parse(&args) {
        Ok(v) => v,
        Err(e) => {
            eprintln!("{e}");
            std::process::exit(2);
        }
    };
    let path = socket.unwrap_or_else(ryotunes_protocol::socket_path);
    let mut stream = match UnixStream::connect(&path) {
        Ok(s) => s,
        Err(e) => {
            eprintln!("ryotunesd not reachable at {}: {e}", path.display());
            std::process::exit(3);
        }
    };
    let mut line = serde_json::to_string(&req).unwrap();
    line.push('\n');
    stream.write_all(line.as_bytes()).unwrap();
    let reader = BufReader::new(stream.try_clone().unwrap());
    for l in reader.lines() {
        let l = match l {
            Ok(l) => l,
            Err(_) => break,
        };
        match serde_json::from_str::<Outgoing>(&l) {
            Ok(Outgoing::Response(r)) => {
                if let Some(e) = r.error {
                    println!("{}", serde_json::to_string(&e).unwrap());
                    std::process::exit(1);
                }
                if !follow {
                    println!("{}", r.result.unwrap_or(serde_json::Value::Null));
                    return;
                }
            }
            Ok(Outgoing::Event(ev)) => {
                if follow {
                    println!("{}", serde_json::to_string(&ev).unwrap());
                }
            }
            Err(_) => eprintln!("unparsable line: {l}"),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn parses_method_with_and_without_params() {
        let (_, r, follow) = parse(&["seek".into(), r#"{"position":12}"#.into()]).unwrap();
        assert_eq!(r.method, "seek");
        assert_eq!(r.params["position"], 12);
        assert!(!follow);
        let (_, r, _) = parse(&["hello".into()]).unwrap();
        assert!(r.params.is_null());
    }

    #[test]
    fn events_subscribes_and_follows() {
        let (_, r, follow) = parse(&["events".into(), "position".into()]).unwrap();
        assert_eq!(r.method, "subscribe");
        assert_eq!(r.params["events"][0], "position");
        assert!(follow);
    }

    #[test]
    fn socket_override_is_consumed_first() {
        let (s, r, _) = parse(&["--socket".into(), "/tmp/x.sock".into(), "hello".into()]).unwrap();
        assert_eq!(s.unwrap(), std::path::PathBuf::from("/tmp/x.sock"));
        assert_eq!(r.method, "hello");
    }
}
