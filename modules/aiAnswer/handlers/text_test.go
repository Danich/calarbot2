package handlers_test

import (
	"context"
	"strings"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"calarbot2/modules/aiAnswer/handlers"
	"calarbot2/modules/aiAnswer/store"
)

type mockLLM struct {
	capturedSystem string
	capturedUser   string
	response       string
}

func (m *mockLLM) Complete(_ context.Context, system, user string) (string, error) {
	m.capturedSystem = system
	m.capturedUser = user
	return m.response, nil
}

func chatMsg(chatTitle, username, text string) *tgbotapi.Message {
	return &tgbotapi.Message{
		Chat: &tgbotapi.Chat{Title: chatTitle},
		From: &tgbotapi.User{UserName: username},
		Text: text,
	}
}

func TestTextHandlerChatIncludesHistory(t *testing.T) {
	llm := &mockLLM{response: "reply"}
	h := handlers.NewTextHandler(llm)

	history := []store.ContextMessage{
		{Username: "alice", Text: "hi"},
		{Username: "bob", Text: "hello"},
	}
	msg := chatMsg("TestChat", "charlie", "hey")

	got, err := h.Chat(context.Background(), "you are a bot", msg, history)
	if err != nil || got != "reply" {
		t.Fatalf("Chat() = %q, %v", got, err)
	}
	if llm.capturedSystem != "you are a bot" {
		t.Errorf("system prompt = %q, want %q", llm.capturedSystem, "you are a bot")
	}
	if !strings.Contains(llm.capturedUser, "alice") || !strings.Contains(llm.capturedUser, "hi") {
		t.Errorf("user message missing history: %q", llm.capturedUser)
	}
	if !strings.Contains(llm.capturedUser, "TestChat") {
		t.Errorf("user message missing chat name: %q", llm.capturedUser)
	}
}

func TestTextHandlerTranslateUsesTranslationPrompt(t *testing.T) {
	llm := &mockLLM{response: "translated text"}
	h := handlers.NewTextHandler(llm)

	msg := chatMsg("", "alice", "Bonjour le monde")
	got, err := h.Translate(context.Background(), "you are a bot", msg, nil)
	if err != nil || got != "translated text" {
		t.Fatalf("Translate() = %q, %v", got, err)
	}
	if !strings.Contains(strings.ToLower(llm.capturedSystem), "translat") {
		t.Errorf("translation system prompt missing 'translat': %q", llm.capturedSystem)
	}
	if !strings.Contains(llm.capturedUser, "Bonjour le monde") {
		t.Errorf("user message missing original text: %q", llm.capturedUser)
	}
}

// Текущее сообщение уже лежит в истории — его записали до решения отвечать.
// Дописывать его отдельно значит показать модели один и тот же ход дважды.
func TestChatDoesNotRepeatTheCurrentMessage(t *testing.T) {
	client := &mockLLM{response: "ок"}
	h := handlers.NewTextHandler(client)

	msg := &tgbotapi.Message{
		Chat: &tgbotapi.Chat{Title: "чат"},
		From: &tgbotapi.User{UserName: "vasya"},
		Text: "как дела",
	}
	history := []store.ContextMessage{
		{Username: "petya", Text: "привет"},
		{Username: "vasya", Text: "как дела"},
	}
	if _, err := h.Chat(context.Background(), "sys", msg, history); err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(client.capturedUser, "как дела"); n != 1 {
		t.Fatalf("сообщение встречается в промпте %d раз, ожидался один:\n%s", n, client.capturedUser)
	}
}

// Записать не удалось, история пуста — модель всё равно должна увидеть, на что
// отвечает.
func TestChatFallsBackToTheMessageWhenHistoryIsEmpty(t *testing.T) {
	client := &mockLLM{response: "ок"}
	h := handlers.NewTextHandler(client)

	msg := &tgbotapi.Message{
		Chat: &tgbotapi.Chat{Title: "чат"},
		From: &tgbotapi.User{UserName: "vasya"},
		Text: "как дела",
	}
	if _, err := h.Chat(context.Background(), "sys", msg, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(client.capturedUser, "как дела") {
		t.Fatalf("сообщение потерялось:\n%s", client.capturedUser)
	}
}

// Картинка без подписи не должна превращаться в пустой ход.
func TestMediaWithoutTextIsDescribed(t *testing.T) {
	client := &mockLLM{response: "ок"}
	h := handlers.NewTextHandler(client)

	msg := &tgbotapi.Message{Chat: &tgbotapi.Chat{Title: "чат"}, From: &tgbotapi.User{UserName: "vasya"}}
	history := []store.ContextMessage{{Username: "petya", MediaType: "photo"}}
	if _, err := h.Chat(context.Background(), "sys", msg, history); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(client.capturedUser, "картинку") {
		t.Fatalf("медиа осталось пустым ходом:\n%s", client.capturedUser)
	}
}
