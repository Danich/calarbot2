package store

import (
	"fmt"
	"time"
)

type LoreRecord struct {
	ID    int64
	Level int
	Text  string
	TS    int64
}

// RipeBatch — созревшие сообщения и id самого свежего из них: именно туда
// переедет курсор, когда пачку переварят.
type RipeBatch struct {
	Messages []ContextMessage
	LastID   int64
}

// EnsureLoreCursor ставит курсор новой персоны на «сейчас».
func (s *Store) EnsureLoreCursor(chatID, personaID int64) error {
	_, err := s.db.Exec(`
		INSERT INTO lore_cursor (chat_id, persona_id, last_message_id)
		SELECT ?, ?, COALESCE((SELECT MAX(id) FROM messages WHERE chat_id = ?), 0)
		WHERE NOT EXISTS (
			SELECT 1 FROM lore_cursor WHERE chat_id = ? AND persona_id = ?
		)`, chatID, personaID, chatID, chatID, personaID)
	return err
}

// EnsureLoreCursorAt — тот же засев, но с явной точки. Нужен тестам и усушке.
func (s *Store) EnsureLoreCursorAt(chatID, personaID, at int64) error {
	_, err := s.db.Exec(`
		INSERT INTO lore_cursor (chat_id, persona_id, last_message_id)
		SELECT ?, ?, ?
		WHERE NOT EXISTS (
			SELECT 1 FROM lore_cursor WHERE chat_id = ? AND persona_id = ?
		)`, chatID, personaID, at, chatID, personaID)
	return err
}

// RipeMessages отдаёт сообщения, которые уже уехали из окна контекста и ещё не
// переварены.
//
// Пока сообщение в окне, модель его и так видит: положить его ещё и в лор —
// заплатить дважды и получить в промпте эхо. Нижняя граница — минимальный id
// среди последних windowSize сообщений чата.
func (s *Store) RipeMessages(chatID, personaID int64, windowSize, limit int) (RipeBatch, error) {
	rows, err := s.db.Query(`
		SELECT id, username, text, media_type FROM messages
		WHERE chat_id = ?
		  AND id > COALESCE(
		        (SELECT last_message_id FROM lore_cursor WHERE chat_id = ? AND persona_id = ?), 0)
		  AND id < COALESCE(
		        (SELECT MIN(id) FROM (
		             SELECT id FROM messages WHERE chat_id = ? ORDER BY id DESC LIMIT ?)), 0)
		ORDER BY id ASC
		LIMIT ?`, chatID, chatID, personaID, chatID, windowSize, limit)
	if err != nil {
		return RipeBatch{}, fmt.Errorf("select ripe messages: %w", err)
	}
	defer rows.Close()

	var batch RipeBatch
	for rows.Next() {
		var id int64
		var m ContextMessage
		if err := rows.Scan(&id, &m.Username, &m.Text, &m.MediaType); err != nil {
			return RipeBatch{}, err
		}
		batch.Messages = append(batch.Messages, m)
		batch.LastID = id
	}
	return batch, rows.Err()
}

// AppendLore пишет события и двигает курсор одной транзакцией: разрыв между
// этими двумя действиями означал бы либо дубли, либо потерянные сообщения.
//
// Проверка курсора внутри той же транзакции — защита от параллельных
// извлечений: если пачку уже переварили, второй заход не пишет ничего, а не
// задваивает события.
func (s *Store) AppendLore(chatID, personaID int64, events []string, cursor int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback()

	var current int64
	if err := tx.QueryRow(
		`SELECT last_message_id FROM lore_cursor WHERE chat_id = ? AND persona_id = ?`,
		chatID, personaID,
	).Scan(&current); err != nil {
		return fmt.Errorf("read cursor: %w", err)
	}
	if cursor <= current {
		return tx.Commit()
	}

	now := time.Now().Unix()
	for _, e := range events {
		if _, err := tx.Exec(
			`INSERT INTO lore (chat_id, persona_id, level, text, ts) VALUES (?, ?, 0, ?, ?)`,
			chatID, personaID, e, now,
		); err != nil {
			return fmt.Errorf("insert lore: %w", err)
		}
	}
	if _, err := tx.Exec(
		`UPDATE lore_cursor SET last_message_id = ? WHERE chat_id = ? AND persona_id = ?`,
		cursor, chatID, personaID,
	); err != nil {
		return fmt.Errorf("advance cursor: %w", err)
	}
	return tx.Commit()
}

// RecentLore — последние живые события, чтобы извлекатель не писал одно и то же
// трижды.
func (s *Store) RecentLore(chatID, personaID int64, limit int) ([]LoreRecord, error) {
	return s.queryLore(`
		SELECT id, level, text, ts FROM lore
		WHERE chat_id = ? AND persona_id = ? AND level = 0 AND covered_by IS NULL
		ORDER BY id DESC LIMIT ?`, chatID, personaID, limit)
}

// LoreForPrompt отдаёт то, что поедет в системный промпт: сначала все живые
// сводки и главы, потом последние eventLimit событий — хронологически.
func (s *Store) LoreForPrompt(chatID, personaID int64, eventLimit int) ([]LoreRecord, error) {
	summaries, err := s.queryLore(`
		SELECT id, level, text, ts FROM lore
		WHERE chat_id = ? AND persona_id = ? AND level > 0 AND covered_by IS NULL
		ORDER BY level DESC, id ASC LIMIT ?`, chatID, personaID, 1000)
	if err != nil {
		return nil, err
	}
	events, err := s.queryLore(`
		SELECT id, level, text, ts FROM lore
		WHERE chat_id = ? AND persona_id = ? AND level = 0 AND covered_by IS NULL
		ORDER BY id DESC LIMIT ?`, chatID, personaID, eventLimit)
	if err != nil {
		return nil, err
	}
	reverse(events)
	return append(summaries, events...), nil
}

func (s *Store) queryLore(query string, args ...any) ([]LoreRecord, error) {
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("select lore: %w", err)
	}
	defer rows.Close()

	var out []LoreRecord
	for rows.Next() {
		var r LoreRecord
		if err := rows.Scan(&r.ID, &r.Level, &r.Text, &r.TS); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func reverse(rs []LoreRecord) {
	for i, j := 0, len(rs)-1; i < j; i, j = i+1, j-1 {
		rs[i], rs[j] = rs[j], rs[i]
	}
}
