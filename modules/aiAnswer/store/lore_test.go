package store_test

import (
	"fmt"
	"testing"

	"calarbot2/modules/aiAnswer/store"
)

// saveN кладёт n сообщений в чат и возвращает стор, готовый к проверкам.
func saveN(t *testing.T, chatID int64, n int) *store.Store {
	t.Helper()
	s := newTestStore(t)
	for i := 0; i < n; i++ {
		if err := s.SaveMessage(msg(chatID, 1, "alice", fmt.Sprintf("m%d", i))); err != nil {
			t.Fatalf("SaveMessage: %v", err)
		}
	}
	return s
}

// Новая персона начинает с чистого листа: иначе новорождённый персонаж
// переварит историю чата с начала времён и получит чужую жизнь.
func TestEnsureLoreCursorStartsAtNow(t *testing.T) {
	s := saveN(t, 100, 30)
	if err := s.EnsureLoreCursor(100, 7); err != nil {
		t.Fatalf("EnsureLoreCursor: %v", err)
	}
	batch, err := s.RipeMessages(100, 7, 10, 50)
	if err != nil {
		t.Fatalf("RipeMessages: %v", err)
	}
	if len(batch.Messages) != 0 {
		t.Errorf("got %d ripe messages, want 0 for a fresh persona", len(batch.Messages))
	}
}

func TestRipeMessagesExcludeTheWindow(t *testing.T) {
	s := saveN(t, 100, 30)
	// Курсор в нуле: имитируем персону, которая живёт в чате с самого начала.
	if err := s.EnsureLoreCursorAt(100, 7, 0); err != nil {
		t.Fatalf("EnsureLoreCursorAt: %v", err)
	}
	batch, err := s.RipeMessages(100, 7, 10, 50)
	if err != nil {
		t.Fatalf("RipeMessages: %v", err)
	}
	if len(batch.Messages) != 20 {
		t.Fatalf("ripe = %d, want 20 (30 saved minus a window of 10)", len(batch.Messages))
	}
	if batch.Messages[0].Text != "m0" {
		t.Errorf("first ripe = %q, want the oldest message", batch.Messages[0].Text)
	}
}

func TestRipeMessagesRespectLimit(t *testing.T) {
	s := saveN(t, 100, 100)
	s.EnsureLoreCursorAt(100, 7, 0)
	batch, _ := s.RipeMessages(100, 7, 10, 50)
	if len(batch.Messages) != 50 {
		t.Errorf("ripe = %d, want the cap of 50", len(batch.Messages))
	}
}

func TestAppendLoreAdvancesCursor(t *testing.T) {
	s := saveN(t, 100, 30)
	s.EnsureLoreCursorAt(100, 7, 0)
	if err := s.AppendLore(100, 7, []string{"нашёл оливье"}, 20); err != nil {
		t.Fatalf("AppendLore: %v", err)
	}
	batch, _ := s.RipeMessages(100, 7, 10, 50)
	if len(batch.Messages) != 0 {
		t.Errorf("%d messages still ripe after digesting up to id 20", len(batch.Messages))
	}
}

// Опоздавший конкурент, переваривший ту же пачку, не должен ни откатывать
// курсор, ни задваивать события: это и есть защита от параллельных извлечений.
func TestAppendLoreIgnoresAlreadyDigestedBatch(t *testing.T) {
	s := saveN(t, 100, 30)
	s.EnsureLoreCursorAt(100, 7, 0)
	s.AppendLore(100, 7, []string{"нашёл оливье"}, 20)

	if err := s.AppendLore(100, 7, []string{"тот же ход, второй заход"}, 20); err != nil {
		t.Fatalf("AppendLore: %v", err)
	}
	got, _ := s.LoreForPrompt(100, 7, 20)
	if len(got) != 1 {
		t.Errorf("lore records = %d, want 1: the batch was digested twice", len(got))
	}
}

func TestLoreIsolatedByPersona(t *testing.T) {
	s := saveN(t, 100, 30)
	s.EnsureLoreCursorAt(100, 7, 0)
	s.EnsureLoreCursorAt(100, 8, 0)
	s.AppendLore(100, 7, []string{"событие Мамкина"}, 20)

	other, err := s.LoreForPrompt(100, 8, 20)
	if err != nil {
		t.Fatalf("LoreForPrompt: %v", err)
	}
	if len(other) != 0 {
		t.Errorf("persona 8 sees %d records of persona 7", len(other))
	}
}

func TestLoreForPromptLimitsEvents(t *testing.T) {
	s := saveN(t, 100, 5)
	s.EnsureLoreCursorAt(100, 7, 0)
	for i := 0; i < 30; i++ {
		s.AppendLore(100, 7, []string{fmt.Sprintf("e%d", i)}, int64(i+1))
	}
	got, _ := s.LoreForPrompt(100, 7, 20)
	if len(got) != 20 {
		t.Fatalf("got %d records, want 20", len(got))
	}
	if got[len(got)-1].Text != "e29" {
		t.Errorf("last record = %q, want the newest event", got[len(got)-1].Text)
	}
}
