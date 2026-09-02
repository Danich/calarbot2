package settings

import "fmt"

// Values отдаёт только явно выставленные значения. Наложить их на дефолты
// модуля — дело Resolve.
func (s *Store) Values(chatID int64, module string) (map[string]string, error) {
	rows, err := s.db.Query(
		`SELECT key, value FROM chat_module_settings WHERE chat_id = ? AND module = ?`,
		chatID, module,
	)
	if err != nil {
		return nil, fmt.Errorf("select module settings: %w", err)
	}
	defer rows.Close()

	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, fmt.Errorf("scan module setting: %w", err)
		}
		out[k] = v
	}
	return out, rows.Err()
}

func (s *Store) SetValue(chatID int64, module, key, value string) error {
	_, err := s.db.Exec(
		`INSERT INTO chat_module_settings (chat_id, module, key, value) VALUES (?, ?, ?, ?)
		 ON CONFLICT(chat_id, module, key) DO UPDATE SET value = excluded.value`,
		chatID, module, key, value,
	)
	return err
}

func (s *Store) DeleteValue(chatID int64, module, key string) error {
	_, err := s.db.Exec(
		`DELETE FROM chat_module_settings WHERE chat_id = ? AND module = ? AND key = ?`,
		chatID, module, key,
	)
	return err
}
