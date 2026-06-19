package handlers_test

import (
	"context"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"calarbot2/modules/aiAnswer/handlers"
)

type mockImageGen struct{ url string }

func (m *mockImageGen) GenerateImage(_ context.Context, _ string) (string, error) {
	return m.url, nil
}

func TestImageGenHandlerGenerate(t *testing.T) {
	h := handlers.NewImageGenHandler(&mockImageGen{"https://example.com/img.jpg"})
	msg := &tgbotapi.Message{Text: "нарисуй кота"}
	url, err := h.Generate(context.Background(), msg)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if url != "https://example.com/img.jpg" {
		t.Errorf("got %q, want %q", url, "https://example.com/img.jpg")
	}
}

func TestImageGenHandlerEmptyPrompt(t *testing.T) {
	h := handlers.NewImageGenHandler(&mockImageGen{})
	msg := &tgbotapi.Message{Text: ""}
	_, err := h.Generate(context.Background(), msg)
	if err == nil {
		t.Error("expected error for empty prompt")
	}
}
