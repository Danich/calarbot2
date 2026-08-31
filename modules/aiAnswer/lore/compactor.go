package lore

import (
	"context"
	"fmt"
	"strings"

	"calarbot2/modules/aiAnswer/store"
)

const (
	// CompactThreshold и CompactBatch — усушка уровня 0 (события). Уровень
	// дешёвый и частый: 40/20 держат его крупными, редкими пачками.
	CompactThreshold = 40
	CompactBatch     = 20

	// CompactThresholdHigh и CompactBatchHigh — усушка уровней 1 и выше
	// (сводки, главы, ...). Тот же принцип "правило одно на любом уровне", но
	// с меньшими числами: без этого LoreForPrompt тащит в промпт до 40 живых
	// записей на КАЖДОМ уровне сразу — при паре активных уровней это уже
	// несколько тысяч лишних токенов на каждый ответ, а не бюджет из спеки.
	// 10/5 держат каждый уровень не толще ~11 живых записей и сохраняют
	// логарифмический рост: события копятся, сводки вчетверо медленнее,
	// главы вчетверо медленнее сводок.
	CompactThresholdHigh = 10
	CompactBatchHigh     = 5
)

const compactSystem = `Ты сжимаешь дневник персонажа.

Тебе дают несколько записей подряд. Верни ОДНУ строку до 200 знаков: что
произошло за этот отрезок, в прошедшем времени, от первого лица. Сохрани имена
и то, что важно для отношений с людьми. Не добавляй ничего от себя и не пиши
ничего, кроме этой строки.`

type Compactor struct {
	client Completer
}

func NewCompactor(c Completer) *Compactor {
	return &Compactor{client: c}
}

func (c *Compactor) Compact(ctx context.Context, canon string, records []store.LoreRecord) (string, error) {
	var sb strings.Builder
	sb.WriteString("Персонаж:\n")
	sb.WriteString(canon)
	sb.WriteString("\n\nЗаписи:\n")
	for _, r := range records {
		sb.WriteString("- ")
		sb.WriteString(r.Text)
		sb.WriteString("\n")
	}
	raw, err := c.client.Complete(ctx, compactSystem, sb.String())
	if err != nil {
		return "", fmt.Errorf("compact: %w", err)
	}
	line := strings.TrimSpace(raw)
	if i := strings.Index(line, "\n"); i >= 0 {
		line = strings.TrimSpace(line[:i])
	}
	line = strings.TrimSpace(strings.TrimPrefix(line, "-"))
	if r := []rune(line); len(r) > EventMaxRunes {
		line = string(r[:EventMaxRunes])
	}
	if line == "" {
		return "", fmt.Errorf("compact: empty summary")
	}
	return line, nil
}
