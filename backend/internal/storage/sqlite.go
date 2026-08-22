package storage

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"event-network-manager/backend/internal/domain"
	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schema string

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	if path == "" {
		path = filepath.Join("data", "enm.db")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("initialize sqlite database: %w", err)
	}
	store := &Store{db: db}
	if err := store.ensureDefaultRoles(context.Background()); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("initialize role profiles: %w", err)
	}
	return store, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) Save(ctx context.Context, snapshot domain.Snapshot) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	placeholderAddress := "snapshot://" + snapshot.SwitchID
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO switches(id,name,model,address,last_seen_at) VALUES(?,?,?,?,?)
		ON CONFLICT(id) DO NOTHING`, snapshot.SwitchID, snapshot.SwitchID, "unknown", placeholderAddress, snapshot.CreatedAt.UTC().Format(time.RFC3339Nano)); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO snapshots(id,switch_id,created_at,configuration) VALUES(?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET configuration=excluded.configuration`,
		snapshot.ID, snapshot.SwitchID, snapshot.CreatedAt.UTC().Format(time.RFC3339Nano), []byte(snapshot.Configuration)); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) List(ctx context.Context, switchID string) ([]domain.Snapshot, error) {
	query := `SELECT id,switch_id,created_at,configuration FROM snapshots`
	args := []any{}
	if switchID != "" {
		query += ` WHERE switch_id=?`
		args = append(args, switchID)
	}
	query += ` ORDER BY created_at DESC`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []domain.Snapshot{}
	for rows.Next() {
		var snapshot domain.Snapshot
		var createdAt string
		var configuration []byte
		if err := rows.Scan(&snapshot.ID, &snapshot.SwitchID, &createdAt, &configuration); err != nil {
			return nil, err
		}
		snapshot.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		snapshot.Configuration = string(configuration)
		snapshot.SizeBytes = len(configuration)
		result = append(result, snapshot)
	}
	return result, rows.Err()
}

func (s *Store) UpsertSwitches(ctx context.Context, switches []domain.Switch) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, sw := range switches {
		if sw.Address == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO switches(id,name,model,address,last_seen_at) VALUES(?,?,?,?,?)
			ON CONFLICT(id) DO UPDATE SET name=excluded.name,model=excluded.model,address=excluded.address,last_seen_at=excluded.last_seen_at`,
			sw.ID, sw.Name, sw.Model, sw.Address, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) TrustedHostKey(ctx context.Context, address string) (string, string, []byte, bool, error) {
	var algorithm, fingerprint string
	var publicKey []byte
	err := s.db.QueryRowContext(ctx, `SELECT algorithm,fingerprint,public_key FROM trusted_host_keys WHERE address=?`, address).Scan(&algorithm, &fingerprint, &publicKey)
	if err == sql.ErrNoRows {
		return "", "", nil, false, nil
	}
	if err != nil {
		return "", "", nil, false, err
	}
	return algorithm, fingerprint, publicKey, true, nil
}

func (s *Store) TrustHostKey(ctx context.Context, address, algorithm, fingerprint string, publicKey []byte) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO trusted_host_keys(address,algorithm,fingerprint,public_key,trusted_at) VALUES(?,?,?,?,?)
		ON CONFLICT(address) DO UPDATE SET algorithm=excluded.algorithm,fingerprint=excluded.fingerprint,public_key=excluded.public_key,trusted_at=excluded.trusted_at`,
		address, algorithm, fingerprint, publicKey, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

func (s *Store) SaveRollbackCommands(ctx context.Context, snapshotID string, commands []string) error {
	encoded, err := json.Marshal(commands)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO applied_changes(snapshot_id,rollback_commands,created_at) VALUES(?,?,?) ON CONFLICT(snapshot_id) DO UPDATE SET rollback_commands=excluded.rollback_commands`, snapshotID, string(encoded), time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

func (s *Store) RollbackCommands(ctx context.Context, snapshotID string) ([]string, bool, error) {
	var encoded string
	err := s.db.QueryRowContext(ctx, `SELECT rollback_commands FROM applied_changes WHERE snapshot_id=?`, snapshotID).Scan(&encoded)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var commands []string
	if err := json.Unmarshal([]byte(encoded), &commands); err != nil {
		return nil, false, err
	}
	return commands, true, nil
}

func (s *Store) ensureDefaultRoles(ctx context.Context) error {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM role_profiles`).Scan(&count); err != nil {
		return err
	}
	if count != 0 {
		return nil
	}
	return s.SaveRoleProfiles(ctx, domain.DefaultRoleProfiles())
}

func (s *Store) RoleProfiles(ctx context.Context) ([]domain.RoleProfile, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT profile_json FROM role_profiles ORDER BY rowid`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	profiles := []domain.RoleProfile{}
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var profile domain.RoleProfile
		if err := json.Unmarshal([]byte(raw), &profile); err != nil {
			return nil, err
		}
		profiles = append(profiles, profile)
	}
	return profiles, rows.Err()
}

func (s *Store) SaveRoleProfiles(ctx context.Context, profiles []domain.RoleProfile) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM role_profiles`); err != nil {
		return err
	}
	for _, profile := range profiles {
		raw, err := json.Marshal(profile)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO role_profiles(id,profile_json) VALUES(?,?)`, profile.ID, string(raw)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) PortSettings(ctx context.Context, switchID string) ([]domain.PortSetting, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT switch_id,port_index,display_name,role_id FROM port_settings WHERE switch_id=? ORDER BY port_index`, switchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	settings := []domain.PortSetting{}
	for rows.Next() {
		var setting domain.PortSetting
		if err := rows.Scan(&setting.SwitchID, &setting.PortIndex, &setting.DisplayName, &setting.RoleID); err != nil {
			return nil, err
		}
		settings = append(settings, setting)
	}
	return settings, rows.Err()
}

func (s *Store) SavePortSettings(ctx context.Context, settings []domain.PortSetting) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, setting := range settings {
		if _, err := tx.ExecContext(ctx, `INSERT INTO port_settings(switch_id,port_index,display_name,role_id) VALUES(?,?,?,?) ON CONFLICT(switch_id,port_index) DO UPDATE SET display_name=excluded.display_name,role_id=excluded.role_id`, setting.SwitchID, setting.PortIndex, setting.DisplayName, setting.RoleID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) SwitchDisplayName(ctx context.Context, switchID string) (string, bool, error) {
	var name string
	err := s.db.QueryRowContext(ctx, `SELECT display_name FROM switch_settings WHERE switch_id=?`, switchID).Scan(&name)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	return name, err == nil, err
}

func (s *Store) SaveSwitchDisplayName(ctx context.Context, switchID, name string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO switch_settings(switch_id,display_name) VALUES(?,?) ON CONFLICT(switch_id) DO UPDATE SET display_name=excluded.display_name`, switchID, name)
	return err
}

func (s *Store) SaveRoleSettingRollback(ctx context.Context, snapshotID string, settings []domain.PortSetting) error {
	raw, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO role_setting_rollbacks(snapshot_id,settings_json) VALUES(?,?) ON CONFLICT(snapshot_id) DO UPDATE SET settings_json=excluded.settings_json`, snapshotID, string(raw))
	return err
}

func (s *Store) RestoreRoleSettings(ctx context.Context, snapshotID string) error {
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT settings_json FROM role_setting_rollbacks WHERE snapshot_id=?`, snapshotID).Scan(&raw)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	var settings []domain.PortSetting
	if err := json.Unmarshal([]byte(raw), &settings); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, setting := range settings {
		if _, err := tx.ExecContext(ctx, `DELETE FROM port_settings WHERE switch_id=? AND port_index=?`, setting.SwitchID, setting.PortIndex); err != nil {
			return err
		}
		if setting.DisplayName != "" || setting.RoleID != "" {
			if _, err := tx.ExecContext(ctx, `INSERT INTO port_settings(switch_id,port_index,display_name,role_id) VALUES(?,?,?,?)`, setting.SwitchID, setting.PortIndex, setting.DisplayName, setting.RoleID); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}
