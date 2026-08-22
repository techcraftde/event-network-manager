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
	return &Store{db: db}, nil
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
