PRAGMA journal_mode = WAL;
PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS switches (
  id TEXT PRIMARY KEY, name TEXT NOT NULL, model TEXT NOT NULL,
  address TEXT NOT NULL UNIQUE, last_seen_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS snapshots (
  id TEXT PRIMARY KEY, switch_id TEXT NOT NULL, created_at TEXT NOT NULL,
  configuration BLOB NOT NULL,
  FOREIGN KEY (switch_id) REFERENCES switches(id)
);
CREATE TABLE IF NOT EXISTS settings (
  key TEXT PRIMARY KEY, value TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS trusted_host_keys (
  address TEXT PRIMARY KEY,
  algorithm TEXT NOT NULL,
  fingerprint TEXT NOT NULL,
  public_key BLOB NOT NULL,
  trusted_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS applied_changes (
  snapshot_id TEXT PRIMARY KEY,
  rollback_commands TEXT NOT NULL,
  created_at TEXT NOT NULL,
  FOREIGN KEY (snapshot_id) REFERENCES snapshots(id)
);
CREATE TABLE IF NOT EXISTS role_profiles (
  id TEXT PRIMARY KEY,
  profile_json TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS port_settings (
  switch_id TEXT NOT NULL,
  port_index INTEGER NOT NULL,
  display_name TEXT NOT NULL,
  role_id TEXT NOT NULL,
  PRIMARY KEY (switch_id, port_index)
);
CREATE TABLE IF NOT EXISTS switch_settings (
  switch_id TEXT PRIMARY KEY,
  display_name TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS role_setting_rollbacks (
  snapshot_id TEXT PRIMARY KEY,
  settings_json TEXT NOT NULL
);
-- Credentials are intentionally excluded and stored in macOS Keychain.
