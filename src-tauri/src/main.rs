#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

use std::{fs, path::PathBuf, process::{Child, Command, Stdio}, sync::Mutex};
use tauri::{Manager, RunEvent};

struct BackendProcess(Mutex<Option<Child>>);

fn backend_binary() -> PathBuf {
    let directory = std::env::current_exe().ok().and_then(|path| path.parent().map(PathBuf::from)).unwrap_or_default();
    let bundled = directory.join("enm-server");
    if bundled.exists() { return bundled; }
    directory.join("enm-server-aarch64-apple-darwin")
}

fn main() {
    let app = tauri::Builder::default()
        .setup(|app| {
            let data_dir = app.path().app_data_dir().map_err(|error| error.to_string())?;
            fs::create_dir_all(&data_dir)?;
            let mut command = Command::new(backend_binary());
            command
                .env("ENM_DATABASE_PATH", data_dir.join("enm.db"))
                .env("ENM_SWITCH_ADDRESS", std::env::var("ENM_SWITCH_ADDRESS").unwrap_or_else(|_| "192.168.250.55".into()))
                .env("ENM_SWITCH_USERNAME", std::env::var("ENM_SWITCH_USERNAME").unwrap_or_else(|_| "admin".into()))
                .stdin(Stdio::null()).stdout(Stdio::null()).stderr(Stdio::null());
            if let Ok(password) = std::env::var("ENM_SWITCH_PASSWORD") { command.env("ENM_SWITCH_PASSWORD", password); }
            if let Ok(targets) = std::env::var("ENM_SWITCHES_JSON") { command.env("ENM_SWITCHES_JSON", targets); }
            let child = command.spawn().map_err(|error| format!("Backend konnte nicht gestartet werden: {error}"))?;
            app.manage(BackendProcess(Mutex::new(Some(child))));
            Ok(())
        })
        .build(tauri::generate_context!())
        .expect("failed to build Event Network Manager");

    app.run(|handle, event| {
        if matches!(event, RunEvent::Exit | RunEvent::ExitRequested { .. }) {
            if let Some(state) = handle.try_state::<BackendProcess>() {
                if let Ok(mut child) = state.0.lock() {
                    if let Some(mut process) = child.take() { let _ = process.kill(); let _ = process.wait(); }
                }
            }
        }
    });
}
