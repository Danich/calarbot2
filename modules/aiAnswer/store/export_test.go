package store

// SetPersonaSourceForTesting подменяет source персоны напрямую, в обход
// UpsertConfigPersona: так тесты проверяют границу владения (config/admin),
// не завися от того, как именно source меняется в проде. Файл собирается
// только тестами и не попадает в публичный API пакета.
func (s *Store) SetPersonaSourceForTesting(id int64, source string) error {
	_, err := s.db.Exec(`UPDATE personas SET source = ? WHERE id = ?`, source, id)
	return err
}
