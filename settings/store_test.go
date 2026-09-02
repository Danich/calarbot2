package settings

import (
	"path/filepath"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestMetaRoundTrip(t *testing.T) {
	s := newTestStore(t)

	if _, ok, err := s.Meta("seeded"); err != nil || ok {
		t.Fatalf("Meta on empty store = ok %v, err %v; want false, nil", ok, err)
	}

	if err := s.SetMeta("seeded", "1"); err != nil {
		t.Fatalf("SetMeta: %v", err)
	}

	v, ok, err := s.Meta("seeded")
	if err != nil || !ok || v != "1" {
		t.Fatalf("Meta = %q, %v, %v; want \"1\", true, nil", v, ok, err)
	}
}

// WAL нужен потому, что в один файл теперь пишут три процесса: движок,
// aiAnswer и админка. Без него они встают в очередь на весь файл.
func TestNewEnablesWAL(t *testing.T) {
	s := newTestStore(t)

	var mode string
	if err := s.db.QueryRow(`PRAGMA journal_mode`).Scan(&mode); err != nil {
		t.Fatalf("PRAGMA journal_mode: %v", err)
	}
	if mode != "wal" {
		t.Fatalf("journal_mode = %q; want \"wal\"", mode)
	}
}
