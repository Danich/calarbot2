package settings

import (
	"database/sql"
	"errors"
	"fmt"
)

// ModuleEnabled: нет строки — выключен. Дефолта «включён» не существует ни у
// одного модуля, включение всегда явное.
func (s *Store) ModuleEnabled(chatID int64, module string) (bool, error) {
	var enabled int
	err := s.db.QueryRow(
		`SELECT enabled FROM chat_modules WHERE chat_id = ? AND module = ?`,
		chatID, module,
	).Scan(&enabled)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("select chat module: %w", err)
	}
	return enabled != 0, nil
}

func (s *Store) SetModuleEnabled(chatID int64, module string, enabled bool) error {
	v := 0
	if enabled {
		v = 1
	}
	_, err := s.db.Exec(
		`INSERT INTO chat_modules (chat_id, module, enabled) VALUES (?, ?, ?)
		 ON CONFLICT(chat_id, module) DO UPDATE SET enabled = excluded.enabled`,
		chatID, module, v,
	)
	return err
}
