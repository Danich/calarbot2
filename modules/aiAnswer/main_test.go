package main

import (
	"database/sql"
	"strings"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"calarbot2/botModules"
	"calarbot2/common"
	"calarbot2/modules/aiAnswer/store"
)

func TestModuleOrder(t *testing.T) {
	m := NewModule(42, AIConfig{BotUsername: "testbot", AnswerLevel: 500})
	if m.Register().Order != 42 {
		t.Errorf("Register().Order = %d, want 42", m.Register().Order)
	}
}

func TestModuleIsCalledNilMessage(t *testing.T) {
	m := NewModule(0, AIConfig{BotUsername: "testbot", AnswerLevel: 500})
	if m.IsCalled(&botModules.Payload{}) {
		t.Error("IsCalled(nil) should return false")
	}
}

func TestModuleIsCalledReplyToBot(t *testing.T) {
	m := NewModule(0, AIConfig{
		BotUsername: "testbot",
		AnswerLevel: DiceSize + 100,
		ReplyWeight: DiceSize + 200,
	})
	msg := &tgbotapi.Message{
		Text: "reply",
		Chat: &tgbotapi.Chat{ID: 1},
		From: &tgbotapi.User{ID: 1},
		ReplyToMessage: &tgbotapi.Message{
			From: &tgbotapi.User{UserName: "testbot"},
		},
	}
	if !m.IsCalled(&botModules.Payload{Msg: msg}) {
		t.Error("IsCalled with reply to bot should return true")
	}
}

func TestModuleIsCalledMentionBot(t *testing.T) {
	m := NewModule(0, AIConfig{
		BotUsername: "testbot",
		AnswerLevel: DiceSize + 100,
		CallWeight:  DiceSize + 200,
	})
	msg := &tgbotapi.Message{
		Text: "Hello @testbot",
		Chat: &tgbotapi.Chat{ID: 1},
		From: &tgbotapi.User{ID: 1},
		Entities: []tgbotapi.MessageEntity{
			{Type: "mention", Offset: 6, Length: 8},
		},
	}
	if !m.IsCalled(&botModules.Payload{Msg: msg}) {
		t.Error("IsCalled with mention should return true")
	}
}

func TestModuleIsCalledDirectReplyAlwaysTrue(t *testing.T) {
	// Direct reply to bot is always true regardless of AnswerLevel
	m := NewModule(0, AIConfig{
		BotUsername: "testbot",
		AnswerLevel: DiceSize + 999,
		ReplyWeight: 0,
	})
	msg := &tgbotapi.Message{
		Text: "reply",
		Chat: &tgbotapi.Chat{ID: 1},
		From: &tgbotapi.User{ID: 1},
		ReplyToMessage: &tgbotapi.Message{
			From: &tgbotapi.User{UserName: "testbot"},
		},
	}
	if !m.IsCalled(&botModules.Payload{Msg: msg}) {
		t.Error("IsCalled with direct reply to bot should always return true")
	}
}

func TestModuleIsCalledDirectMentionAlwaysTrue(t *testing.T) {
	// Direct @mention is always true regardless of AnswerLevel
	m := NewModule(0, AIConfig{
		BotUsername: "testbot",
		AnswerLevel: DiceSize + 999,
		CallWeight:  0,
	})
	msg := &tgbotapi.Message{
		Text: "Hello @testbot",
		Chat: &tgbotapi.Chat{ID: 1},
		From: &tgbotapi.User{ID: 1},
		Entities: []tgbotapi.MessageEntity{
			{Type: "mention", Offset: 6, Length: 8},
		},
	}
	if !m.IsCalled(&botModules.Payload{Msg: msg}) {
		t.Error("IsCalled with @mention should always return true (direct address)")
	}
}

func TestSystemPromptCarriesPersonaAndLore(t *testing.T) {
	path := t.TempDir() + "/test.db"
	s, err := store.New(path)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	p, _, _ := s.UpsertConfigPersona("mamkin", "mamkin", "ты Мамкин")
	s.EnsureLoreCursorAt(100, p.ID, 0)
	s.AppendLore(100, p.ID, []string{"нашёл оливье"}, 1)
	s.Close()

	m := NewModule(1, AIConfig{
		SystemPrompt:   "ты Мамкин",
		DefaultPersona: "mamkin",
		LoreWindow:     20,
		SQLitePath:     path,
	})
	defer m.cancelRefresh()

	_, system := m.systemPromptFor(100, "mamkin")
	if !strings.Contains(system, "ты Мамкин") {
		t.Error("canon missing from the system prompt")
	}
	if !strings.Contains(system, "нашёл оливье") {
		t.Error("lore missing from the system prompt")
	}
}

func TestSystemPromptFallsBackWithoutStore(t *testing.T) {
	m := NewModule(1, AIConfig{SystemPrompt: "ты Мамкин"})
	defer m.cancelRefresh()

	if _, system := m.systemPromptFor(100, m.config.DefaultPersona); system != "ты Мамкин" {
		t.Errorf("system = %q, want the config prompt verbatim", system)
	}
}

// Закрытый стор ломает самый первый запрос systemPromptFor — ResolvePersona.
// Промпт должен откатиться на конфиг целиком, без обрезка канона.
func TestSystemPromptFallsBackWhenResolvePersonaFails(t *testing.T) {
	path := t.TempDir() + "/test.db"
	m := NewModule(1, AIConfig{
		SystemPrompt:   "ты Мамкин",
		DefaultPersona: "mamkin",
		LoreWindow:     20,
		SQLitePath:     path,
	})
	defer m.cancelRefresh()

	if err := m.store.Close(); err != nil {
		t.Fatalf("store.Close: %v", err)
	}

	p, system := m.systemPromptFor(100, "mamkin")
	if system != "ты Мамкин" {
		t.Errorf("system = %q, want the config prompt verbatim on ResolvePersona failure", system)
	}
	if p != (store.Persona{}) {
		t.Errorf("persona = %+v, want the zero value on ResolvePersona failure", p)
	}
}

// Персона резолвится успешно, а запрос лора — нет: канон персоны должен
// остаться, теряется только блок воспоминаний. Это отличает этот откат от
// провала ResolvePersona, где теряется всё, включая персону.
func TestSystemPromptKeepsCanonWhenLoreQueryFails(t *testing.T) {
	path := t.TempDir() + "/test.db"
	m := NewModule(1, AIConfig{
		SystemPrompt:   "ты Мамкин",
		DefaultPersona: "mamkin",
		LoreWindow:     20,
		SQLitePath:     path,
	})
	defer m.cancelRefresh()

	// NewModule уже засеяла персону "mamkin" в m.store. Ломаем только таблицу
	// lore отдельным соединением к тому же файлу, оставляя personas рабочей —
	// иначе не отличить этот откат от провала ResolvePersona.
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	if _, err := raw.Exec("DROP TABLE lore"); err != nil {
		t.Fatalf("drop lore table: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("raw.Close: %v", err)
	}

	p, system := m.systemPromptFor(100, "mamkin")
	if p.Key != "mamkin" {
		t.Fatalf("persona = %+v, want the resolved persona kept despite lore failure", p)
	}
	if system != "ты Мамкин" {
		t.Errorf("system = %q, want the persona canon verbatim (no lore block) when lore lookup fails", system)
	}
}

func TestRegisterDeclaresConfigValuesAsDefaults(t *testing.T) {
	m := &Module{order: 100, config: AIConfig{
		AnswerLevel: 990, CallWeight: 700, ReplyWeight: 400,
		ContextSize: 10, DefaultPersona: "mamkin",
	}}

	reg := m.Register()

	if reg.Order != 100 || reg.Label != "AI-ответ" {
		t.Errorf("Registration = %+v; want order 100, label AI-ответ", reg)
	}

	byKey := map[string]botModules.Field{}
	for _, f := range reg.Fields {
		byKey[f.Key] = f
	}

	// Сегодняшняя настройка обязана стать дефолтом нового канала — это и есть
	// весь механизм «прикопать нынешние значения», отдельного нет.
	for key, want := range map[string]any{
		"answer_level": 990, "call_weight": 700, "reply_weight": 400, "context_size": 10,
	} {
		f, ok := byKey[key]
		if !ok {
			t.Fatalf("Register did not declare %s", key)
		}
		if f.Default != want {
			t.Errorf("%s default = %v; want %v", key, f.Default, want)
		}
		if f.Type != botModules.FieldNumber {
			t.Errorf("%s type = %q; want number", key, f.Type)
		}
	}

	if byKey["persona"].Default != "mamkin" {
		t.Errorf("persona default = %v; want mamkin", byKey["persona"].Default)
	}
	if byKey["persona"].Type != botModules.FieldSelect {
		t.Errorf("persona type = %q; want select", byKey["persona"].Type)
	}
}

// Веса — это бросок d1000, значения вне диапазона осмысленного смысла не имеют,
// и админка обязана узнать границы от модуля, а не угадать их.
func TestRegisterBoundsTheWeights(t *testing.T) {
	m := &Module{config: AIConfig{}}

	for _, f := range m.Register().Fields {
		switch f.Key {
		case "answer_level", "call_weight", "reply_weight":
			if f.Min == nil || *f.Min != 0 || f.Max == nil || *f.Max != 1000 {
				t.Errorf("%s bounds = %v..%v; want 0..1000", f.Key, f.Min, f.Max)
			}
		case "context_size":
			if f.Min == nil || *f.Min != 0 {
				t.Errorf("context_size min = %v; want 0", f.Min)
			}
		}
	}
}

func TestExtractMentions(t *testing.T) {
	tests := []struct {
		name     string
		msg      *tgbotapi.Message
		expected []string
	}{
		{
			name: "single mention",
			msg: &tgbotapi.Message{
				Text:     "Hello @testbot",
				Entities: []tgbotapi.MessageEntity{{Type: "mention", Offset: 6, Length: 8}},
			},
			expected: []string{"@testbot"},
		},
		{
			name:     "no mentions",
			msg:      &tgbotapi.Message{Text: "Hello world", Entities: []tgbotapi.MessageEntity{}},
			expected: []string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mentions := common.ExtractMentions(tt.msg)
			if len(mentions) != len(tt.expected) {
				t.Errorf("got %d mentions, want %d", len(mentions), len(tt.expected))
				return
			}
			for i, m := range mentions {
				if m != tt.expected[i] {
					t.Errorf("mentions[%d] = %q, want %q", i, m, tt.expected[i])
				}
			}
		})
	}
}
