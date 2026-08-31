package lore_test

import (
	"context"
	"strings"
	"testing"

	"calarbot2/modules/aiAnswer/lore"
	"calarbot2/modules/aiAnswer/store"
)

func TestCompactReturnsASingleLine(t *testing.T) {
	llm := &stubLLM{reply: "за первую неделю облазил три этажа и подружился с @vasya"}
	got, err := lore.NewCompactor(llm).Compact(context.Background(), "canon", []store.LoreRecord{
		{Text: "нашёл оливье"}, {Text: "@vasya обещал схему этажей"},
	})
	if err != nil {
		t.Fatalf("Compact: %v", err)
	}
	if !strings.Contains(got, "три этажа") {
		t.Errorf("summary = %q", got)
	}
	if strings.Contains(got, "\n") {
		t.Error("summary must be a single line")
	}
}

func TestCompactPassesRecordsToTheModel(t *testing.T) {
	llm := &stubLLM{reply: "сводка"}
	_, _ = lore.NewCompactor(llm).Compact(context.Background(), "canon", []store.LoreRecord{{Text: "нашёл оливье"}})
	if !strings.Contains(llm.user, "нашёл оливье") {
		t.Error("records missing from the compaction prompt")
	}
}

// Бесплатные модели любят добавить хвост после ответа и префикс "- " перед
// ним — сводка должна остаться одной строкой в пределах лимита независимо
// от того, что дописала модель.
func TestCompactTrimsMultilineDashedAndOverlongReplies(t *testing.T) {
	long := strings.Repeat("я", lore.EventMaxRunes+50)
	llm := &stubLLM{reply: "- " + long + "\nнадеюсь, помог!"}
	got, err := lore.NewCompactor(llm).Compact(context.Background(), "canon", []store.LoreRecord{{Text: "нашёл оливье"}})
	if err != nil {
		t.Fatalf("Compact: %v", err)
	}
	if strings.Contains(got, "\n") {
		t.Error("summary must be a single line")
	}
	if strings.HasPrefix(got, "-") {
		t.Errorf("leading dash must be stripped: %q", got)
	}
	if r := []rune(got); len(r) > lore.EventMaxRunes {
		t.Errorf("summary length = %d runes, want at most %d", len(r), lore.EventMaxRunes)
	}
}
