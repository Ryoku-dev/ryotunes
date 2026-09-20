// Share the Linux launcher with the Arch/Tauri package without building WebKit UI assets.
#[cfg(target_os = "linux")]
#[path = "../../../src-tauri/src/native.rs"]
mod native;

#[cfg(target_os = "linux")]
fn main() {
    native::launch();
}

#[cfg(not(target_os = "linux"))]
fn main() {
    eprintln!("The native Quickshell launcher is only available on Linux.");
    std::process::exit(1);
}
