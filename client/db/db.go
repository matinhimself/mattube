package db

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

func Open(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path+"?_journal=WAL&_timeout=5000&_foreign_keys=on")
	if err != nil {
		return nil, fmt.Errorf("open sqlite %s: %w", path, err)
	}
	db.SetMaxOpenConns(1) // SQLite WAL supports 1 writer
	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return db, nil
}

func migrate(db *sql.DB) error {
	_, err := db.Exec(`
	CREATE TABLE IF NOT EXISTS users (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		username    TEXT    NOT NULL UNIQUE,
		password    TEXT    NOT NULL,
		is_admin    INTEGER NOT NULL DEFAULT 0,
		created_at  TEXT    NOT NULL DEFAULT (datetime('now')),
		last_login  TEXT
	);

	CREATE TABLE IF NOT EXISTS sessions (
		token       TEXT    PRIMARY KEY,
		user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		expires_at  TEXT    NOT NULL,
		created_at  TEXT    NOT NULL DEFAULT (datetime('now'))
	);

	CREATE INDEX IF NOT EXISTS idx_sessions_expires ON sessions(expires_at);

	CREATE TABLE IF NOT EXISTS settings (
		key         TEXT PRIMARY KEY,
		value       TEXT NOT NULL,
		updated_at  TEXT NOT NULL DEFAULT (datetime('now'))
	);
	`)
	return err
}

func GetSetting(db *sql.DB, key string) (string, error) {
	var value string
	err := db.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&value)
	if err != nil {
		return "", err
	}
	return value, nil
}

func SetSetting(db *sql.DB, key, value string) error {
	_, err := db.Exec(
		`INSERT INTO settings(key, value, updated_at) VALUES(?, ?, datetime('now'))
		 ON CONFLICT(key) DO UPDATE SET value=excluded.value, updated_at=excluded.updated_at`,
		key, value,
	)
	return err
}
