package lore

import (
	"context"
	"log"
	"sync"

	"calarbot2/modules/aiAnswer/store"
)

// Storage — та часть стора, которой пользуется рост лора.
type Storage interface {
	EnsureLoreCursor(chatID, personaID int64) error
	RipeMessages(chatID, personaID int64, windowSize, limit int) (store.RipeBatch, error)
	RecentLore(chatID, personaID int64, limit int) ([]store.LoreRecord, error)
	AppendLore(chatID, personaID int64, events []string, cursor int64) error
}

type Runner struct {
	store  Storage
	ex     *Extractor
	window int

	// running держит по одному извлечению на (чат, персона): без него частые
	// сообщения запускают несколько заходов на одну и ту же пачку.
	running sync.Map
}

func NewRunner(s Storage, ex *Extractor, window int) *Runner {
	return &Runner{store: s, ex: ex, window: window}
}

// Maybe запускает извлечение в фоне и сразу возвращается: ответ в чате не ждёт
// памяти.
func (r *Runner) Maybe(chatID, personaID int64, canon string) {
	key := [2]int64{chatID, personaID}
	if _, busy := r.running.LoadOrStore(key, true); busy {
		return
	}
	go func() {
		defer r.running.Delete(key)
		if err := r.Run(context.Background(), chatID, personaID, canon); err != nil {
			log.Printf("lore: %v", err)
		}
	}()
}

// Run переваривает одну пачку. Возвращает nil, когда переваривать нечего.
func (r *Runner) Run(ctx context.Context, chatID, personaID int64, canon string) error {
	if err := r.store.EnsureLoreCursor(chatID, personaID); err != nil {
		return err
	}
	batch, err := r.store.RipeMessages(chatID, personaID, r.window, BatchMax)
	if err != nil {
		return err
	}
	if len(batch.Messages) < BatchMin {
		return nil
	}
	recent, err := r.store.RecentLore(chatID, personaID, RecentForPrompt)
	if err != nil {
		return err
	}
	events, err := r.ex.Extract(ctx, canon, batch.Messages, recent)
	if err != nil {
		// Курсор не двигаем: пачка доедет следующим разом.
		return err
	}
	if err := r.store.AppendLore(chatID, personaID, events, batch.LastID); err != nil {
		return err
	}
	for _, e := range events {
		log.Printf("lore: chat=%d persona=%d + %q", chatID, personaID, e)
	}
	return nil
}
