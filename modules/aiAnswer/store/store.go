package store

import (
	"database/sql"
	"fmt"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

type ContextMessage struct {
	Username  string
	Text      string
	MediaType string
}

func New(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
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

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS messages (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			chat_id    INTEGER NOT NULL,
			user_id    INTEGER NOT NULL,
			username   TEXT,
			text       TEXT,
			media_type TEXT,
			ts         INTEGER NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_messages_chat ON messages(chat_id, ts);
		CREATE TABLE IF NOT EXISTS meta (
			key   TEXT PRIMARY KEY,
			value TEXT
		);
	`)
	return err
}

func (s *Store) SaveMessage(msg *tgbotapi.Message) error {
	mediaType := ""
	if msg.Photo != nil {
		mediaType = "photo"
	} else if msg.Sticker != nil {
		mediaType = "sticker"
	}
	username := ""
	var userID int64
	if msg.From != nil {
		username = msg.From.UserName
		userID = msg.From.ID
	}
	_, err := s.db.Exec(
		`INSERT INTO messages (chat_id, user_id, username, text, media_type, ts) VALUES (?, ?, ?, ?, ?, ?)`,
		msg.Chat.ID, userID, username, msg.Text, mediaType, msg.Date,
	)
	return err
}

func (s *Store) GetContext(chatID int64, limit int) ([]ContextMessage, error) {
	rows, err := s.db.Query(
		`SELECT username, text, media_type FROM messages WHERE chat_id = ? ORDER BY ts DESC LIMIT ?`,
		chatID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var msgs []ContextMessage
	for rows.Next() {
		var m ContextMessage
		if err := rows.Scan(&m.Username, &m.Text, &m.MediaType); err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Reverse to chronological order (oldest first)
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}
	return msgs, nil
}

func (s *Store) GetMeta(key string) (string, bool, error) {
	var value string
	err := s.db.QueryRow(`SELECT value FROM meta WHERE key = ?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return value, true, nil
}

func (s *Store) SetMeta(key, value string) error {
	_, err := s.db.Exec(
		`INSERT INTO meta(key, value) VALUES(?, ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`,
		key, value,
	)
	return err
}

func (s *Store) Close() error {
	return s.db.Close()
}
