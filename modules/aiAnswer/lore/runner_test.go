package lore_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"calarbot2/modules/aiAnswer/lore"
	"calarbot2/modules/aiAnswer/store"
)

func runnerStore(t *testing.T, chatID int64, n int) *store.Store {
	t.Helper()
	s, err := store.New(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	for i := 0; i < n; i++ {
		s.SaveMessage(&tgbotapi.Message{
			Chat: &tgbotapi.Chat{ID: chatID},
			From: &tgbotapi.User{ID: 1, UserName: "alice"},
			Text: fmt.Sprintf("m%d", i),
			Date: int(time.Now().Unix()),
		})
	}
	return s
}

func TestRunnerWaitsForBatchMin(t *testing.T) {
	s := runnerStore(t, 100, 12) // окно 10 ⇒ созрело всего 2
	llm := &stubLLM{reply: "- что-то было"}
	s.EnsureLoreCursorAt(100, 7, 0)

	if err := lore.NewRunner(s, lore.NewExtractor(llm), 10).Run(context.Background(), 100, 7, "canon"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if llm.user != "" {
		t.Error("model called before the batch was ripe enough")
	}
}

func TestRunnerWritesEventsAndAdvancesCursor(t *testing.T) {
	s := runnerStore(t, 100, 40)
	s.EnsureLoreCursorAt(100, 7, 0)
	llm := &stubLLM{reply: "- нашёл оливье"}

	r := lore.NewRunner(s, lore.NewExtractor(llm), 10)
	if err := r.Run(context.Background(), 100, 7, "canon"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	got, _ := s.LoreForPrompt(100, 7, 20)
	if len(got) != 1 || got[0].Text != "нашёл оливье" {
		t.Fatalf("lore = %#v", got)
	}
	batch, _ := s.RipeMessages(100, 7, 10, 50)
	if len(batch.Messages) != 0 {
		t.Errorf("%d messages still ripe after a successful run", len(batch.Messages))
	}
}

// Ошибка модели не двигает курсор: пачка должна доехать следующим разом.
func TestRunnerKeepsCursorOnModelError(t *testing.T) {
	s := runnerStore(t, 100, 40)
	s.EnsureLoreCursorAt(100, 7, 0)
	llm := &stubLLM{err: errors.New("model down")}

	r := lore.NewRunner(s, lore.NewExtractor(llm), 10)
	if err := r.Run(context.Background(), 100, 7, "canon"); err == nil {
		t.Fatal("want an error")
	}
	batch, _ := s.RipeMessages(100, 7, 10, 50)
	if len(batch.Messages) == 0 {
		t.Error("cursor advanced despite the model failing")
	}
}

// NONE — валидный ответ, а не сбой: курсор обязан двинуться, иначе пачка
// застрянет навсегда.
func TestRunnerAdvancesCursorOnNone(t *testing.T) {
	s := runnerStore(t, 100, 40)
	s.EnsureLoreCursorAt(100, 7, 0)
	llm := &stubLLM{reply: "NONE"}

	r := lore.NewRunner(s, lore.NewExtractor(llm), 10)
	if err := r.Run(context.Background(), 100, 7, "canon"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	batch, _ := s.RipeMessages(100, 7, 10, 50)
	if len(batch.Messages) != 0 {
		t.Errorf("%d messages still ripe after NONE", len(batch.Messages))
	}
}

func TestRunnerNeverExceedsBatchMax(t *testing.T) {
	s := runnerStore(t, 100, 200)
	s.EnsureLoreCursorAt(100, 7, 0)
	llm := &stubLLM{reply: "NONE"}

	r := lore.NewRunner(s, lore.NewExtractor(llm), 10)
	if err := r.Run(context.Background(), 100, 7, "canon"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if lines := countLines(llm.user); lines > lore.BatchMax+10 {
		t.Errorf("prompt carried %d lines, batch cap is %d", lines, lore.BatchMax)
	}
}

func countLines(s string) int {
	n := 0
	for _, c := range s {
		if c == '\n' {
			n++
		}
	}
	return n
}
