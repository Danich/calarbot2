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
	CompactCandidates(chatID, personaID int64, level, threshold, batch int) ([]store.LoreRecord, error)
	ApplyCompaction(chatID, personaID int64, level int, ids []int64, summary string) error
}

type Runner struct {
	store  Storage
	ex     *Extractor
	cp     *Compactor
	window int

	// running держит по одному извлечению на (чат, персона): без него частые
	// сообщения запускают несколько заходов на одну и ту же пачку.
	running sync.Map
}

func NewRunner(s Storage, ex *Extractor, cp *Compactor, window int) *Runner {
	return &Runner{store: s, ex: ex, cp: cp, window: window}
}

// Maybe запускает извлечение в фоне и сразу возвращается: ответ в чате не ждёт
// памяти.
func (r *Runner) Maybe(chatID, personaID int64, canon string) {
	key := [2]int64{chatID, personaID}
	if _, busy := r.running.LoadOrStore(key, true); busy {
		return
	}
	go func() {
		// Порядок defer важен: Delete должен освободить ключ и тогда, когда
		// Run паникует — иначе чат навсегда выпадает из роста лора.
		defer r.running.Delete(key)
		defer func() {
			if p := recover(); p != nil {
				// Это единственная неприсмотренная фоновая горутина на горячем
				// пути сообщений: паника здесь не должна ронять процесс и
				// обрывать ответы во всех остальных чатах.
				log.Printf("lore: panic: chat=%d persona=%d: %v", chatID, personaID, p)
			}
		}()
		if err := r.Run(context.Background(), chatID, personaID, canon); err != nil {
			log.Printf("lore: chat=%d persona=%d: %v", chatID, personaID, err)
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
		// Новых сообщений мало, но лор мог перевалить порог и раньше: усушка
		// не должна ждать, пока чат снова оживёт и подкинет свежую пачку.
		if err := r.compact(ctx, chatID, personaID, canon); err != nil {
			log.Printf("lore compaction: %v", err)
		}
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
	// Усушка идёт снизу вверх: схлопнутые события могут переполнить уровень
	// сводок, и тогда сводки схлопнутся в главу тем же оператором.
	if err := r.compact(ctx, chatID, personaID, canon); err != nil {
		log.Printf("lore compaction: %v", err)
	}
	return nil
}

const maxCompactLevels = 5

func (r *Runner) compact(ctx context.Context, chatID, personaID int64, canon string) error {
	for level := 0; level < maxCompactLevels; level++ {
		cands, err := r.store.CompactCandidates(chatID, personaID, level, CompactThreshold, CompactBatch)
		if err != nil {
			return err
		}
		if len(cands) == 0 {
			return nil
		}
		summary, err := r.cp.Compact(ctx, canon, cands)
		if err != nil {
			return err
		}
		ids := make([]int64, 0, len(cands))
		for _, c := range cands {
			ids = append(ids, c.ID)
		}
		if err := r.store.ApplyCompaction(chatID, personaID, level, ids, summary); err != nil {
			return err
		}
		log.Printf("lore: chat=%d persona=%d compacted %d records of level %d into %q",
			chatID, personaID, len(ids), level, summary)
	}
	return nil
}
