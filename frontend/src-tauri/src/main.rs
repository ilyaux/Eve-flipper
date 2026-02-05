// Prevents additional console window on Windows in release
#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

use tauri_plugin_shell::ShellExt;

#[cfg(windows)]
fn show_error(msg: &str) {
    use windows_sys::Win32::UI::WindowsAndMessaging::{MessageBoxW, MB_ICONERROR};
    let msg_wide: Vec<u16> = msg.encode_utf16().chain(Some(0)).collect();
    let title_wide: Vec<u16> = "EVE Flipper\0".encode_utf16().collect();
    unsafe {
        MessageBoxW(
            std::ptr::null_mut(),
            msg_wide.as_ptr(),
            title_wide.as_ptr(),
            MB_ICONERROR,
        );
    }
}

#[cfg(not(windows))]
fn show_error(msg: &str) {
    eprintln!("EVE Flipper error: {}", msg);
}

fn main() {
    tauri::Builder::default()
        .plugin(tauri_plugin_shell::init())
        .setup(|app| {
            let sidecar = match app.shell().sidecar("eve-flipper-backend") {
                Ok(s) => s,
                Err(e) => {
                    let msg = format!("Backend binary not found. Run from the folder that contains eve-flipper-backend.exe.\n\n{:?}", e);
                    show_error(&msg);
                    std::process::exit(1);
                }
            };
            let (mut _rx, _child) = match sidecar.args(["--port", "13370"]).spawn() {
                Ok(p) => p,
                Err(e) => {
                    let msg = format!("Failed to start backend server.\n\n{:?}", e);
                    show_error(&msg);
                    std::process::exit(1);
                }
            };
            Ok(())
        })
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
}
