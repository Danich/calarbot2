package handlers_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"calarbot2/modules/aiAnswer/handlers"
)

type mockVision struct {
	desc string
	err  error
}

func (m *mockVision) DescribeImage(_ context.Context, _, _ string) (string, error) {
	return m.desc, m.err
}

type mockPersonaLLM struct {
	response   string
	err        error
	lastUser   string
	lastSystem string
}

func (m *mockPersonaLLM) Complete(_ context.Context, system, user string) (string, error) {
	m.lastSystem = system
	m.lastUser = user
	return m.response, m.err
}

func TestVisionHandler_Describe_noPersona(t *testing.T) {
	h := handlers.NewVisionHandler(&mockVision{desc: "a fluffy cat"}, nil)
	msg := &tgbotapi.Message{Caption: "что это?"}

	got, err := h.Describe(context.Background(), "", msg, "https://cdn.telegram.org/file/photos/test.jpg")
	if err != nil {
		t.Fatalf("Describe: %v", err)
	}
	if got != "a fluffy cat" {
		t.Errorf("got %q, want %q", got, "a fluffy cat")
	}
}

func TestVisionHandler_Describe_withPersona(t *testing.T) {
	persona := &mockPersonaLLM{response: "arrr, a fluffy cat it be!"}
	h := handlers.NewVisionHandler(&mockVision{desc: "a fluffy cat"}, persona)

	msg := &tgbotapi.Message{Caption: "что это?"}
	got, err := h.Describe(context.Background(), "You are a pirate.", msg, "https://cdn.telegram.org/file/photos/test.jpg")
	if err != nil {
		t.Fatalf("Describe: %v", err)
	}
	if got != "arrr, a fluffy cat it be!" {
		t.Errorf("got %q, want persona-styled answer", got)
	}
	if persona.lastUser != "a fluffy cat" {
		t.Errorf("persona received user=%q, want raw description", persona.lastUser)
	}
	// Без инструкции персона отвечает на описание, а не пересказывает его.
	if !strings.HasPrefix(persona.lastSystem, "You are a pirate.") ||
		!strings.Contains(persona.lastSystem, "Перескажи") {
		t.Errorf("persona received system=%q, want the character prompt plus the rewrite instruction", persona.lastSystem)
	}
}

func TestVisionHandler_Describe_personaErrorFallback(t *testing.T) {
	persona := &mockPersonaLLM{err: errors.New("persona down")}
	h := handlers.NewVisionHandler(&mockVision{desc: "a fluffy cat"}, persona)

	msg := &tgbotapi.Message{}
	got, err := h.Describe(context.Background(), "sys", msg, "https://cdn.telegram.org/file/photos/test.jpg")
	if err != nil {
		t.Fatalf("Describe: %v", err)
	}
	if got != "a fluffy cat" {
		t.Errorf("got %q, want raw fallback on persona error", got)
	}
}

func TestVisionHandler_Describe_noPhotoURL(t *testing.T) {
	h := handlers.NewVisionHandler(&mockVision{desc: "irrelevant"}, nil)
	msg := &tgbotapi.Message{Text: "hello"}
	_, err := h.Describe(context.Background(), "", msg, "")
	if err == nil {
		t.Error("expected error when no photo URL provided")
	}
}
