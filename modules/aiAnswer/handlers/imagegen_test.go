package handlers_test

import (
	"context"
	"testing"

	"calarbot2/modules/aiAnswer/handlers"
)

type mockImageGen struct{ url string }

func (m *mockImageGen) GenerateImage(_ context.Context, _ string) (string, error) {
	return m.url, nil
}

func TestImageGenHandlerGenerate(t *testing.T) {
	h := handlers.NewImageGenHandler(&mockImageGen{"https://example.com/img.jpg"})
	answer, err := h.Generate(context.Background(), "нарисуй кота")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if answer.PhotoURL != "https://example.com/img.jpg" {
		t.Errorf("PhotoURL: got %q, want %q", answer.PhotoURL, "https://example.com/img.jpg")
	}
	if answer.Text != "" {
		t.Errorf("Text: got %q, want %q", answer.Text, "")
	}
}

func TestImageGenHandlerEmptyPrompt(t *testing.T) {
	h := handlers.NewImageGenHandler(&mockImageGen{})
	_, err := h.Generate(context.Background(), "")
	if err == nil {
		t.Error("expected error for empty prompt")
	}
}
