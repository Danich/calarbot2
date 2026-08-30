package lore_test

import (
	"context"
	"strings"
	"testing"

	"calarbot2/modules/aiAnswer/lore"
	"calarbot2/modules/aiAnswer/store"
)

type stubLLM struct {
	reply  string
	err    error
	system string
	user   string
}

func (s *stubLLM) Complete(_ context.Context, system, user string) (string, error) {
	s.system, s.user = system, user
	return s.reply, s.err
}

func msgs(texts ...string) []store.ContextMessage {
	out := make([]store.ContextMessage, 0, len(texts))
	for _, t := range texts {
		out = append(out, store.ContextMessage{Username: "alice", Text: t})
	}
	return out
}

func TestExtractParsesDashedLines(t *testing.T) {
	llm := &stubLLM{reply: "- нашёл оливье в холодильнике\n- @vasya обещал показать схему этажей"}
	got, err := lore.NewExtractor(llm).Extract(context.Background(), "canon", msgs("привет"), nil)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if len(got) != 2 || got[0] != "нашёл оливье в холодильнике" {
		t.Errorf("got %#v", got)
	}
}

func TestExtractTreatsNoneAsEmpty(t *testing.T) {
	llm := &stubLLM{reply: "NONE"}
	got, err := lore.NewExtractor(llm).Extract(context.Background(), "canon", msgs("привет"), nil)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %#v, want nothing", got)
	}
}

// Бесплатные модели любят добавить преамбулу — она не должна попадать в лор.
func TestExtractIgnoresChatter(t *testing.T) {
	llm := &stubLLM{reply: "Конечно! Вот что произошло:\n- съел чужой контейнер\nНадеюсь, помог!"}
	got, _ := lore.NewExtractor(llm).Extract(context.Background(), "canon", msgs("привет"), nil)
	if len(got) != 1 || got[0] != "съел чужой контейнер" {
		t.Errorf("got %#v", got)
	}
}

func TestExtractCapsEventCountAndLength(t *testing.T) {
	long := strings.Repeat("я", 500)
	llm := &stubLLM{reply: "- a\n- b\n- c\n- d\n- " + long}
	got, _ := lore.NewExtractor(llm).Extract(context.Background(), "canon", msgs("привет"), nil)
	if len(got) != 3 {
		t.Fatalf("got %d events, want at most 3", len(got))
	}
	for _, e := range got {
		if len([]rune(e)) > lore.EventMaxRunes {
			t.Errorf("event longer than the cap: %d runes", len([]rune(e)))
		}
	}
}

func TestExtractPassesCanonAndRecentLore(t *testing.T) {
	llm := &stubLLM{reply: "NONE"}
	recent := []store.LoreRecord{{Text: "уже записано"}}
	_, _ = lore.NewExtractor(llm).Extract(context.Background(), "я Мамкин", msgs("привет"), recent)
	if !strings.Contains(llm.user, "я Мамкин") {
		t.Error("canon missing from the extractor prompt")
	}
	if !strings.Contains(llm.user, "уже записано") {
		t.Error("recent lore missing from the extractor prompt")
	}
	if !strings.Contains(llm.user, "привет") {
		t.Error("batch missing from the extractor prompt")
	}
}

func TestExtractPropagatesModelError(t *testing.T) {
	llm := &stubLLM{err: context.DeadlineExceeded}
	if _, err := lore.NewExtractor(llm).Extract(context.Background(), "canon", msgs("привет"), nil); err == nil {
		t.Error("want the model error to propagate so the cursor stays put")
	}
}
