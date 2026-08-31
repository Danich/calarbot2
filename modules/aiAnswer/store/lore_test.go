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

// RipeMessages — самый хитрый SQL в ветке (вложенный MIN(id) поверх окна плюс
// два COALESCE), и это единственное место, где потеря сообщения означала бы
// навсегда забытый кусок биографии. Прогоняем ~200 сообщений через повторные
// циклы "созрело — переварили" при нескольких размерах окна и проверяем, что
// на круг набор переваренных сообщений — это ровно всё, что старше окна, без
// пропусков и без повторов.
func TestRipeMessagesNoGapsAcrossRepeatedCycles(t *testing.T) {
	const total = 200
	for _, window := range []int{5, 10, 20} {
		t.Run(fmt.Sprintf("window=%d", window), func(t *testing.T) {
			s := saveN(t, 100, total)
			s.EnsureLoreCursorAt(100, 7, 0)

			seen := make(map[string]int)
			var order []string
			for {
				// limit меньше общего числа сообщений: без этого один вызов
				// переварил бы всё разом и тест не отличил бы "нет пропусков
				// внутри цикла" от "нет пропусков между циклами".
				batch, err := s.RipeMessages(100, 7, window, 7)
				if err != nil {
					t.Fatalf("RipeMessages: %v", err)
				}
				if len(batch.Messages) == 0 {
					break
				}
				for _, m := range batch.Messages {
					seen[m.Text]++
					order = append(order, m.Text)
				}
				if err := s.AppendLore(100, 7, nil, batch.LastID); err != nil {
					t.Fatalf("AppendLore: %v", err)
				}
			}

			want := total - window
			if len(order) != want {
				t.Fatalf("digested %d messages, want %d (total %d minus window %d)", len(order), want, total, window)
			}
			for text, n := range seen {
				if n != 1 {
					t.Errorf("message %q digested %d times, want exactly once", text, n)
				}
			}
			for i := 0; i < want; i++ {
				wantText := fmt.Sprintf("m%d", i)
				if _, ok := seen[wantText]; !ok {
					t.Errorf("message %q never digested (gap)", wantText)
				}
			}
			for i := want; i < total; i++ {
				stillWindowed := fmt.Sprintf("m%d", i)
				if _, ok := seen[stillWindowed]; ok {
					t.Errorf("message %q inside the window got digested", stillWindowed)
				}
			}
		})
	}
}

// Меньше сообщений, чем окно: всё ещё в окне, переваривать нечего — не
// ошибка и не пропуск, а законный пустой результат.
func TestRipeMessagesFewerThanWindow(t *testing.T) {
	s := saveN(t, 100, 3)
	s.EnsureLoreCursorAt(100, 7, 0)
	batch, err := s.RipeMessages(100, 7, 10, 50)
	if err != nil {
		t.Fatalf("RipeMessages: %v", err)
	}
	if len(batch.Messages) != 0 {
		t.Errorf("got %d ripe messages, want 0 when the chat is smaller than the window", len(batch.Messages))
	}
}

// Пустой чат — вырожденный случай того же правила: нет сообщений вообще,
// значит нечему быть ни в окне, ни созревшим.
func TestRipeMessagesEmptyChat(t *testing.T) {
	s := newTestStore(t)
	s.EnsureLoreCursorAt(100, 7, 0)
	batch, err := s.RipeMessages(100, 7, 10, 50)
	if err != nil {
		t.Fatalf("RipeMessages: %v", err)
	}
	if len(batch.Messages) != 0 {
		t.Errorf("got %d ripe messages, want 0 for an empty chat", len(batch.Messages))
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

func TestCompactCandidatesWaitForThreshold(t *testing.T) {
	s := saveN(t, 100, 5)
	s.EnsureLoreCursorAt(100, 7, 0)
	for i := 0; i < 39; i++ {
		s.AppendLore(100, 7, []string{fmt.Sprintf("e%d", i)}, int64(i+1))
	}
	got, err := s.CompactCandidates(100, 7, 0, 40, 20)
	if err != nil {
		t.Fatalf("CompactCandidates: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d candidates below the threshold, want 0", len(got))
	}
}

func TestCompactCandidatesTakeTheOldest(t *testing.T) {
	s := saveN(t, 100, 5)
	s.EnsureLoreCursorAt(100, 7, 0)
	for i := 0; i < 41; i++ {
		s.AppendLore(100, 7, []string{fmt.Sprintf("e%d", i)}, int64(i+1))
	}
	got, _ := s.CompactCandidates(100, 7, 0, 40, 20)
	if len(got) != 20 {
		t.Fatalf("got %d candidates, want 20", len(got))
	}
	if got[0].Text != "e0" {
		t.Errorf("first candidate = %q, want the oldest event", got[0].Text)
	}
}

// Схлопнутое не удаляется, но и в промпт больше не идёт.
func TestApplyCompactionCoversOriginals(t *testing.T) {
	s := saveN(t, 100, 5)
	s.EnsureLoreCursorAt(100, 7, 0)
	for i := 0; i < 41; i++ {
		s.AppendLore(100, 7, []string{fmt.Sprintf("e%d", i)}, int64(i+1))
	}
	cands, _ := s.CompactCandidates(100, 7, 0, 40, 20)
	ids := make([]int64, 0, len(cands))
	for _, c := range cands {
		ids = append(ids, c.ID)
	}
	if err := s.ApplyCompaction(100, 7, 0, ids, "первые недели прошли в коридорах"); err != nil {
		t.Fatalf("ApplyCompaction: %v", err)
	}

	got, _ := s.LoreForPrompt(100, 7, 100)
	if len(got) != 22 {
		t.Fatalf("prompt records = %d, want 21 events + 1 summary", len(got))
	}
	if got[0].Level != 1 || got[0].Text != "первые недели прошли в коридорах" {
		t.Errorf("first record = %+v, want the summary", got[0])
	}
	for _, r := range got {
		if r.Text == "e0" {
			t.Error("covered event still reaches the prompt")
		}
	}
}
