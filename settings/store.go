// Package settings хранит то, чем управляет админка: список чатов, включённость
// модулей по чатам и их настройки. Секреты и дефолты по-прежнему живут в yaml —
// строка здесь означает явно принятое решение, а её отсутствие «не трогали».
package settings

import (
	"database/sql"
	"errors"
	"fmt"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

// New открывает базу в режиме, пригодном для нескольких процессов сразу.
//
// busy_timeout задаётся на соединении, а не на базе, поэтому его мало включить
// один раз где-то ещё: без него второй писатель получит SQLITE_BUSY вместо
// того, чтобы подождать.
func New(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	// left_at, а не left: LEFT — зарезервированное слово в SQLite.
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS chats (
			id         INTEGER PRIMARY KEY,
			type       TEXT NOT NULL,
			title      TEXT NOT NULL DEFAULT '',
			username   TEXT NOT NULL DEFAULT '',
			first_seen INTEGER NOT NULL,
			last_seen  INTEGER NOT NULL,
			left_at    INTEGER
		);
		CREATE TABLE IF NOT EXISTS chat_modules (
			chat_id INTEGER NOT NULL,
			module  TEXT    NOT NULL,
			enabled INTEGER NOT NULL,
			PRIMARY KEY (chat_id, module)
		);
		CREATE TABLE IF NOT EXISTS chat_module_settings (
			chat_id INTEGER NOT NULL,
			module  TEXT    NOT NULL,
			key     TEXT    NOT NULL,
			value   TEXT    NOT NULL,
			PRIMARY KEY (chat_id, module, key)
		);
		CREATE TABLE IF NOT EXISTS settings_meta (
			key   TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);
	`)
	return err
}

func (s *Store) Meta(key string) (string, bool, error) {
	var v string
	err := s.db.QueryRow(`SELECT value FROM settings_meta WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("select meta: %w", err)
	}
	return v, true, nil
}

func (s *Store) SetMeta(key, value string) error {
	_, err := s.db.Exec(
		`INSERT INTO settings_meta (key, value) VALUES (?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		key, value,
	)
	return err
}
