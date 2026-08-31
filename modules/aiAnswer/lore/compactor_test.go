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
