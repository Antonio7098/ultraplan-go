package productstate

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

const DatabaseRelativePath = ".ultraplan/run-control.db"

var ErrNotFound = errors.New("product state not found")

type Item struct {
	Key     string
	Ordinal int
	Payload []byte
}

type Record struct {
	Kind          string
	Scope         string
	SchemaVersion int
	Header        []byte
	Items         []Item
}

type Store struct{ db *sql.DB }

var stores sync.Map

func Existing(root string) (*Store, bool, error) {
	path := filepath.Join(root, filepath.FromSlash(DatabaseRelativePath))
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	store, err := open(root)
	return store, err == nil, err
}

func Ensure(root string) (*Store, error) { return open(root) }

func open(root string) (*Store, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if cached, ok := stores.Load(root); ok {
		return cached.(*Store), nil
	}
	path := filepath.Join(root, filepath.FromSlash(DatabaseRelativePath))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	values := url.Values{}
	values.Set("_busy_timeout", strconv.FormatInt((5*time.Second).Milliseconds(), 10))
	values.Set("_foreign_keys", "on")
	values.Set("_journal_mode", "WAL")
	values.Set("_synchronous", "FULL")
	values.Set("_txlock", "immediate")
	dsn := (&url.URL{Scheme: "file", Path: filepath.ToSlash(path), RawQuery: values.Encode()}).String()
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(4)
	if err := createSchema(context.Background(), db); err != nil {
		_ = db.Close()
		return nil, err
	}
	store := &Store{db: db}
	actual, loaded := stores.LoadOrStore(root, store)
	if loaded {
		_ = db.Close()
		return actual.(*Store), nil
	}
	return store, nil
}

func createSchema(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS product_states (
  kind TEXT NOT NULL,
  scope TEXT NOT NULL,
  schema_version INTEGER NOT NULL,
  header_json BLOB NOT NULL,
  header_hash BLOB NOT NULL,
  updated_at TEXT NOT NULL,
  PRIMARY KEY (kind, scope)
);
CREATE TABLE IF NOT EXISTS product_state_items (
  kind TEXT NOT NULL,
  scope TEXT NOT NULL,
  item_key TEXT NOT NULL,
  ordinal INTEGER NOT NULL,
  payload_json BLOB NOT NULL,
  payload_hash BLOB NOT NULL,
  PRIMARY KEY (kind, scope, item_key),
  FOREIGN KEY (kind, scope) REFERENCES product_states(kind, scope) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS product_state_items_order ON product_state_items(kind, scope, ordinal);
`)
	return err
}

func (s *Store) Has(ctx context.Context, kind, scope string) (bool, error) {
	var found int
	err := s.db.QueryRowContext(ctx, `SELECT 1 FROM product_states WHERE kind = ? AND scope = ?`, kind, scope).Scan(&found)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func (s *Store) Load(ctx context.Context, kind, scope string) (Record, error) {
	record := Record{Kind: kind, Scope: scope}
	if err := s.db.QueryRowContext(ctx, `SELECT schema_version, header_json FROM product_states WHERE kind = ? AND scope = ?`, kind, scope).Scan(&record.SchemaVersion, &record.Header); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Record{}, ErrNotFound
		}
		return Record{}, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT item_key, ordinal, payload_json FROM product_state_items WHERE kind = ? AND scope = ? ORDER BY ordinal`, kind, scope)
	if err != nil {
		return Record{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var item Item
		if err := rows.Scan(&item.Key, &item.Ordinal, &item.Payload); err != nil {
			return Record{}, err
		}
		record.Items = append(record.Items, item)
	}
	return record, rows.Err()
}

func (s *Store) Save(ctx context.Context, record Record) error {
	if record.Kind == "" || record.Scope == "" || record.SchemaVersion < 1 || len(record.Header) == 0 {
		return errors.New("invalid product state record")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	headerHash := sha256.Sum256(record.Header)
	if _, err := tx.ExecContext(ctx, `INSERT INTO product_states(kind, scope, schema_version, header_json, header_hash, updated_at)
VALUES(?, ?, ?, ?, ?, ?) ON CONFLICT(kind, scope) DO UPDATE SET schema_version=excluded.schema_version,
header_json=excluded.header_json, header_hash=excluded.header_hash, updated_at=excluded.updated_at
WHERE product_states.header_hash <> excluded.header_hash OR product_states.schema_version <> excluded.schema_version`,
		record.Kind, record.Scope, record.SchemaVersion, record.Header, headerHash[:], time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return err
	}
	keep := make(map[string]bool, len(record.Items))
	for _, item := range record.Items {
		if item.Key == "" {
			return fmt.Errorf("product state item key is required")
		}
		keep[item.Key] = true
		hash := sha256.Sum256(item.Payload)
		if _, err := tx.ExecContext(ctx, `INSERT INTO product_state_items(kind, scope, item_key, ordinal, payload_json, payload_hash)
VALUES(?, ?, ?, ?, ?, ?) ON CONFLICT(kind, scope,item_key) DO UPDATE SET ordinal=excluded.ordinal,
payload_json=excluded.payload_json, payload_hash=excluded.payload_hash
WHERE product_state_items.payload_hash <> excluded.payload_hash OR product_state_items.ordinal <> excluded.ordinal`,
			record.Kind, record.Scope, item.Key, item.Ordinal, item.Payload, hash[:]); err != nil {
			return err
		}
	}
	rows, err := tx.QueryContext(ctx, `SELECT item_key FROM product_state_items WHERE kind = ? AND scope = ?`, record.Kind, record.Scope)
	if err != nil {
		return err
	}
	var stale []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			_ = rows.Close()
			return err
		}
		if !keep[key] {
			stale = append(stale, key)
		}
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, key := range stale {
		if _, err := tx.ExecContext(ctx, `DELETE FROM product_state_items WHERE kind=? AND scope=? AND item_key=?`, record.Kind, record.Scope, key); err != nil {
			return err
		}
	}
	return tx.Commit()
}
