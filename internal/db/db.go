package db

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

type DB struct {
	sql *sql.DB
}

func Open(path string) (*DB, error) {
	sqldb, err := sql.Open("sqlite", "file:"+path+"?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// SQLite supports only one concurrent writer
	sqldb.SetMaxOpenConns(1)

	d := &DB{sql: sqldb}
	if err := d.migrate(); err != nil {
		sqldb.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return d, nil
}

func (d *DB) migrate() error {
	_, err := d.sql.Exec(`
		CREATE TABLE IF NOT EXISTS links (
			keyword    TEXT PRIMARY KEY,
			url        TEXT NOT NULL,
			title      TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL DEFAULT (datetime('now')),
			ip         TEXT NOT NULL DEFAULT '',
			clicks     INTEGER NOT NULL DEFAULT 0
		);

		CREATE TABLE IF NOT EXISTS clicks (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			keyword    TEXT NOT NULL REFERENCES links(keyword) ON DELETE CASCADE,
			clicked_at TEXT NOT NULL DEFAULT (datetime('now')),
			referrer   TEXT NOT NULL DEFAULT '',
			user_agent TEXT NOT NULL DEFAULT '',
			ip         TEXT NOT NULL DEFAULT ''
		);

		-- Auto-incrementing counter for keyword generation
		CREATE TABLE IF NOT EXISTS counter (
			id       INTEGER PRIMARY KEY CHECK (id = 1),
			next_val INTEGER NOT NULL DEFAULT 1
		);
		INSERT OR IGNORE INTO counter (id, next_val) VALUES (1, 1);

		CREATE TABLE IF NOT EXISTS categories (
			id   INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE
		);
	`)
	if err != nil {
		return err
	}
	// Idempotent: add category_id to links if not yet present.
	_, _ = d.sql.Exec(`ALTER TABLE links ADD COLUMN category_id INTEGER REFERENCES categories(id) ON DELETE SET NULL`)
	return nil
}

func (d *DB) Close() error {
	return d.sql.Close()
}
