package lore_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"calarbot2/modules/aiAnswer/lore"
	"calarbot2/modules/aiAnswer/store"
)

// syncBuffer — приёмник для log.SetOutput в тесте с фоновой горутиной: сама
// стандартная log.Logger сериализует запись через свой мьютекс, но наше
// собственное чтение String() идёт мимо него, и bytes.Buffer без замка на
// это не рассчитан.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

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

	if err := lore.NewRunner(s, lore.NewExtractor(llm), lore.NewCompactor(llm), 10).Run(context.Background(), 100, 7, "canon"); err != nil {
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

	r := lore.NewRunner(s, lore.NewExtractor(llm), lore.NewCompactor(llm), 10)
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

	r := lore.NewRunner(s, lore.NewExtractor(llm), lore.NewCompactor(llm), 10)
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

	r := lore.NewRunner(s, lore.NewExtractor(llm), lore.NewCompactor(llm), 10)
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

	r := lore.NewRunner(s, lore.NewExtractor(llm), lore.NewCompactor(llm), 10)
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

// signalingStore оборачивает стор и сообщает о каждом завершённом AppendLore.
//
// "Модель ответила" — это ещё не "горутина закончила": после Complete() Run()
// ещё пишет в базу и снимает guard. Без этого сигнала тест может решить, что
// пора завершаться, и t.Cleanup закроет базу, пока фоновая горутина всё ещё
// дописывает лор — гонка с закрытием, а не с самим guard.
type signalingStore struct {
	*store.Store
	appended chan error
}

func (s *signalingStore) AppendLore(chatID, personaID int64, events []string, cursor int64) error {
	err := s.Store.AppendLore(chatID, personaID, events, cursor)
	s.appended <- err
	return err
}

// panicOnceLLM падает на первом вызове и работает как обычно на следующих:
// нужен, чтобы проверить, что паника в фоновой горутине Maybe не роняет
// процесс и не оставляет guard занятым навсегда.
type panicOnceLLM struct {
	calls   int32
	started chan struct{}
}

func (p *panicOnceLLM) Complete(_ context.Context, _, _ string) (string, error) {
	n := atomic.AddInt32(&p.calls, 1)
	p.started <- struct{}{}
	if n == 1 {
		panic("boom")
	}
	return "- цел и невредим", nil
}

// Если бы это упало без recover(), паника пришла бы из горутины Maybe и
// уронила бы весь процесс go test, а не только этот тест — значит, сам факт,
// что тест дошёл до конца, уже часть проверки.
func TestMaybeRecoversFromPanicAndReleasesGuard(t *testing.T) {
	raw := runnerStore(t, 100, 40)
	raw.EnsureLoreCursorAt(100, 7, 0)
	s := &signalingStore{Store: raw, appended: make(chan error, 4)}
	llm := &panicOnceLLM{started: make(chan struct{}, 4)}
	r := lore.NewRunner(s, lore.NewExtractor(llm), lore.NewCompactor(llm), 10)

	logBuf := &syncBuffer{}
	orig := log.Writer()
	log.SetOutput(logBuf)
	defer log.SetOutput(orig)

	r.Maybe(100, 7, "canon")
	select {
	case <-llm.started:
	case <-time.After(2 * time.Second):
		t.Fatal("model was never called")
	}

	// Первый заход паникует и до AppendLore не доходит. guard освобождается,
	// когда паника размотается, а второй Maybe должен снова дойти до модели
	// и на этот раз дописать лор. Ждём именно записи, а не просто вызова
	// модели: иначе тест может вернуться раньше, чем горутина допишет базу,
	// и столкнётся с t.Cleanup, который её закрывает.
	deadline := time.Now().Add(2 * time.Second)
	appended := false
	for !appended {
		r.Maybe(100, 7, "canon")
		select {
		case err := <-s.appended:
			if err != nil {
				t.Fatalf("AppendLore: %v", err)
			}
			appended = true
		case <-time.After(2 * time.Millisecond):
		}
		if !appended && time.Now().After(deadline) {
			t.Fatal("guard never released after a panic; a later Maybe call was skipped forever")
		}
	}

	if !strings.Contains(logBuf.String(), "panic") {
		t.Error("the recovered panic was not logged")
	}
}

// blockingLLM держит первый вызов на канале, которым управляет тест: так
// момент, когда второй Maybe накладывается на первый, известен точно, без
// гадания по времени.
type blockingLLM struct {
	calls   int32
	release chan struct{}
}

func (b *blockingLLM) Complete(_ context.Context, _, _ string) (string, error) {
	n := atomic.AddInt32(&b.calls, 1)
	if n == 1 {
		<-b.release
	}
	return "- ок", nil
}

func TestMaybeCollapsesOverlappingCalls(t *testing.T) {
	// 120 сообщений при окне 10 и BatchMax 50: первый заход съедает только
	// первые 50 созревших (RipeMessages режет пачку по BatchMax) и двигает
	// курсор только до них, так что для второго захода остаётся ещё под 60
	// созревших — есть что переваривать, и второй Maybe не выйдет пустым по
	// нехватке материала, а действительно дойдёт до модели второй раз.
	raw := runnerStore(t, 100, 120)
	raw.EnsureLoreCursorAt(100, 7, 0)
	s := &signalingStore{Store: raw, appended: make(chan error, 4)}
	llm := &blockingLLM{release: make(chan struct{})}
	r := lore.NewRunner(s, lore.NewExtractor(llm), lore.NewCompactor(llm), 10)

	// LoadOrStore в Maybe отрабатывает синхронно до запуска горутины, так что
	// guard уже занят к моменту, когда первый Maybe возвращает управление —
	// второй вызов гарантированно застаёт его занятым, без гонки.
	r.Maybe(100, 7, "canon")
	r.Maybe(100, 7, "canon")

	// Дать первому заходу дойти до модели: без этого calls мог бы остаться
	// нулевым по случайности планировщика, и проверка ничего бы не значила.
	deadline := time.Now().Add(2 * time.Second)
	for atomic.LoadInt32(&llm.calls) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("model was never called")
		}
		time.Sleep(2 * time.Millisecond)
	}

	if n := atomic.LoadInt32(&llm.calls); n != 1 {
		t.Fatalf("model called %d times for two overlapping Maybe calls, want 1 (the guard should have skipped the second)", n)
	}

	close(llm.release) // отпускаем заблокированный первый заход

	// Дождаться, пока первый заход реально допишет лор (а не просто дозвонится
	// до модели) — только тогда guard гарантированно свободен.
	select {
	case err := <-s.appended:
		if err != nil {
			t.Fatalf("AppendLore (first run): %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("first run never finished writing")
	}

	// Более поздний Maybe должен снова дойти до модели и дописать лор —
	// иначе застрявший guard теряет пачки навсегда. Ждём именно записи, а не
	// просто вызова модели, чтобы не столкнуться с t.Cleanup, закрывающим
	// стор раньше, чем горутина закончит.
	deadline = time.Now().Add(2 * time.Second)
	done := false
	for !done {
		r.Maybe(100, 7, "canon")
		select {
		case err := <-s.appended:
			if err != nil {
				t.Fatalf("AppendLore (second run): %v", err)
			}
			done = true
		case <-time.After(2 * time.Millisecond):
		}
		if !done && time.Now().After(deadline) {
			t.Fatal("guard never released after Run finished; a later Maybe call was skipped")
		}
	}

	if n := atomic.LoadInt32(&llm.calls); n != 2 {
		t.Fatalf("model called %d times overall, want exactly 2 (one per successful Run)", n)
	}
}

func TestRunnerCompactsWhenEventsPileUp(t *testing.T) {
	s := runnerStore(t, 100, 40)
	s.EnsureLoreCursorAt(100, 7, 0)
	for i := 0; i < 41; i++ {
		s.AppendLore(100, 7, []string{fmt.Sprintf("e%d", i)}, int64(i+1))
	}
	llm := &stubLLM{reply: "сводка недели"}

	r := lore.NewRunner(s, lore.NewExtractor(llm), lore.NewCompactor(llm), 10)
	if err := r.Run(context.Background(), 100, 7, "canon"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	got, _ := s.LoreForPrompt(100, 7, 100)
	if got[0].Level != 1 {
		t.Errorf("first record = %+v, want a level-1 summary", got[0])
	}
	if len(got) > 41 {
		t.Errorf("prompt records = %d, compaction did not shrink anything", len(got))
	}
}
