package settings

import "fmt"

type Chat struct {
	ID        int64
	Type      string
	Title     string
	Username  string
	FirstSeen int64
	LastSeen  int64
	LeftAt    *int64
}

// UpsertChat записывает чат, увиденный движком.
//
// first_seen не двигается, а left_at сбрасывается: апдейт из чата означает, что
// бот там снова есть, независимо от того, выходил ли он оттуда через админку.
func (s *Store) UpsertChat(c Chat) error {
	_, err := s.db.Exec(
		`INSERT INTO chats (id, type, title, username, first_seen, last_seen, left_at)
		 VALUES (?, ?, ?, ?, ?, ?, NULL)
		 ON CONFLICT(id) DO UPDATE SET
		     type      = excluded.type,
		     title     = excluded.title,
		     username  = excluded.username,
		     last_seen = excluded.last_seen,
		     left_at   = NULL`,
		c.ID, c.Type, c.Title, c.Username, c.FirstSeen, c.LastSeen,
	)
	return err
}

// ListChats отдаёт чаты, где бот сейчас есть, в одном запросе. Личку от групп
// отделяет вью: разделение по типу нужно ровно в одном месте, и делать ради
// него второй проход по базе незачем.
func (s *Store) ListChats() ([]Chat, error) {
	rows, err := s.db.Query(
		`SELECT id, type, title, username, first_seen, last_seen, left_at
		 FROM chats WHERE left_at IS NULL ORDER BY last_seen DESC`)
	if err != nil {
		return nil, fmt.Errorf("select chats: %w", err)
	}
	defer rows.Close()

	var out []Chat
	for rows.Next() {
		var c Chat
		if err := rows.Scan(&c.ID, &c.Type, &c.Title, &c.Username, &c.FirstSeen, &c.LastSeen, &c.LeftAt); err != nil {
			return nil, fmt.Errorf("scan chat: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) MarkLeft(chatID, ts int64) error {
	_, err := s.db.Exec(`UPDATE chats SET left_at = ? WHERE id = ?`, ts, chatID)
	return err
}
