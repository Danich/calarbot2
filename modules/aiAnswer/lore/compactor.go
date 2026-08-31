package lore

import (
	"context"
	"fmt"
	"strings"

	"calarbot2/modules/aiAnswer/store"
)

const (
	// CompactThreshold и CompactBatch задают усушку: правило одно и работает на
	// любом уровне, поэтому память растёт логарифмически — события копятся,
	// сводки вдесятеро медленнее, главы вдесятеро медленнее сводок.
	CompactThreshold = 40
	CompactBatch     = 20
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
