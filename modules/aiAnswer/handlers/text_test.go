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
	h := handlers.NewTextHandler(llm, "you are a bot")

	history := []store.ContextMessage{
		{Username: "alice", Text: "hi"},
		{Username: "bob", Text: "hello"},
	}
	msg := chatMsg("TestChat", "charlie", "hey")

	got, err := h.Chat(context.Background(), msg, history)
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
	h := handlers.NewTextHandler(llm, "you are a bot")

	msg := chatMsg("", "alice", "Bonjour le monde")
	got, err := h.Translate(context.Background(), msg, nil)
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
