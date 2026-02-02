// Prevents additional console window on Windows in release
#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

use tauri_plugin_shell::ShellExt;

fn main() {
    tauri::Builder::default()
        .plugin(tauri_plugin_shell::init())
        .setup(|app| {
            // Spawn Go backend as sidecar
            let sidecar = app
                .shell()
                .sidecar("eve-flipper-backend")
                .expect("failed to find sidecar binary");

            let (mut _rx, _child) = sidecar
                .args(["--port", "13370"])
                .spawn()
                .expect("failed to spawn Go backend sidecar");

            Ok(())
        })
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
}
