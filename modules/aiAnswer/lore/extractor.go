// Package lore выращивает биографию персонажа из разговора: что уехало из окна
// контекста, то превращается в короткие события и живёт дальше в системном
// промпте.
package lore

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"calarbot2/modules/aiAnswer/store"
)

const (
	// BatchMin — сколько созревших сообщений ждём перед вызовом модели. Без
	// порога прогретый чат дёргает модель на каждое сообщение, а лор состоит из
	// огрызков по одной реплике.
	BatchMin = 10
	// BatchMax — потолок пачки: если модель падала неделю, ей не уезжает неделя
	// разом.
	BatchMax = 50
	// EventMaxRunes держит длину события: всё, что пишут в чат, оказывается в
	// системном промпте следующего вызова, и длинному тексту там делать нечего.
	EventMaxRunes = 200
	// RecentForPrompt — сколько уже записанных событий показываем извлекателю,
	// чтобы он не писал одно и то же трижды.
	RecentForPrompt = 5

	maxEventsPerBatch = 3
)

type Completer interface {
	Complete(ctx context.Context, system, user string) (string, error)
}

type Extractor struct {
	client Completer
}

func NewExtractor(c Completer) *Extractor {
	return &Extractor{client: c}
}

// extractSystem описывает, что считать событием.
//
// Ключевое здесь — третий абзац. В чате обязательно скажут «ты вчера сожрал
// Набиуллину», и наивный извлекатель положит это в лор как факт биографии.
// Событием считается сама шутка, а не её содержание: так лор не едет от
// подъёбок, но помнит, как с ботом разговаривали.
const extractSystem = `Ты ведёшь дневник персонажа по переписке в чате.

Верни от нуля до трёх строк, каждая начинается с "- ". Одна строка — одно
событие, до 200 знаков, в прошедшем времени, от первого лица. Если ничего
достойного записи не произошло, верни ровно NONE.

Записывай только то, что персонаж сделал или пережил сам, и наблюдаемые факты
разговора. Чужие утверждения о прошлом персонажа фактами не считаются: если
кто-то говорит, что персонаж что-то делал, событие — это то, что так сказали.
Не "я сожрал Набиуллину", а "@vasya пошутил, будто я сожрал Набиуллину".

Не пересказывай уже записанное. Не пиши ничего, кроме строк событий.`

func (e *Extractor) Extract(ctx context.Context, canon string, msgs []store.ContextMessage, recent []store.LoreRecord) ([]string, error) {
	raw, err := e.client.Complete(ctx, extractSystem, buildExtractPrompt(canon, msgs, recent))
	if err != nil {
		return nil, fmt.Errorf("extract: %w", err)
	}
	events, ok := parseEvents(raw)
	if !ok {
		// Ответ не NONE, но ни одной строки не разобралось: это ближе к сбою
		// модели, чем к «ничего не произошло». Не двигаем курсор — иначе
		// пачка молча уедет из памяти без единой строки лора.
		return nil, fmt.Errorf("extract: no events parsed from reply: %q", raw)
	}
	return events, nil
}

func buildExtractPrompt(canon string, msgs []store.ContextMessage, recent []store.LoreRecord) string {
	var sb strings.Builder
	sb.WriteString("Персонаж:\n")
	sb.WriteString(canon)
	if len(recent) > 0 {
		sb.WriteString("\n\nУже записано (не повторяй):\n")
		for _, r := range recent {
			sb.WriteString("- ")
			sb.WriteString(r.Text)
			sb.WriteString("\n")
		}
	}
	sb.WriteString("\nЧто было в чате:\n")
	for _, m := range msgs {
		sb.WriteString(fmt.Sprintf("%s: %s\n", m.Username, describe(m)))
	}
	return sb.String()
}

// describe повторяет логику handlers.describe: медиа без подписи должно
// остаться событием, а не пустой строкой.
func describe(m store.ContextMessage) string {
	if m.Text != "" {
		return m.Text
	}
	switch m.MediaType {
	case "photo":
		return "[прислал картинку]"
	case "sticker":
		return "[прислал стикер]"
	}
	return ""
}

// bulletPrefixes — маркеры списка, которые терпит парсер: заказан "- ", но
// автоподобранная бесплатная модель с равной вероятностью выдаёт "•", "*" или
// нумерованный список.
var bulletPrefixes = []string{"-", "•", "*"}

// parseEvents терпим к мусору: бесплатные модели любят добавить преамбулу и
// прощание, и это не повод потерять пачку.
//
// Второй результат — false означает «ответ не NONE, но ни одна строка не
// разобралась как событие». Такой ответ ближе к сбою модели, чем к «ничего не
// произошло», и вызывающий код обязан не двигать курсор на этом результате.
func parseEvents(raw string) ([]string, bool) {
	if strings.EqualFold(strings.TrimSpace(strings.Trim(raw, ". ")), "NONE") {
		return nil, true
	}
	var out []string
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		rest, ok := stripBullet(line)
		if !ok {
			continue
		}
		line = strings.TrimSpace(rest)
		if line == "" || strings.EqualFold(line, "NONE") {
			continue
		}
		if r := []rune(line); len(r) > EventMaxRunes {
			line = string(r[:EventMaxRunes])
		}
		out = append(out, line)
		if len(out) == maxEventsPerBatch {
			break
		}
	}
	return out, len(out) > 0
}

// stripBullet узнаёт и срезает маркер списка: дефис, точку-буллет, звёздочку
// или нумерованный пункт вида "1.". ok=false — маркера нет вовсе, строка не
// про событие.
func stripBullet(line string) (string, bool) {
	for _, p := range bulletPrefixes {
		if strings.HasPrefix(line, p) {
			return strings.TrimPrefix(line, p), true
		}
	}
	if i := strings.IndexByte(line, '.'); i > 0 {
		if _, err := strconv.Atoi(line[:i]); err == nil {
			return line[i+1:], true
		}
	}
	return "", false
}
