package handlers_test

import (
	"context"
	"testing"

	"calarbot2/modules/aiAnswer/handlers"
)

type mockImageGen struct{ img []byte }

func (m *mockImageGen) GenerateImage(_ context.Context, _ string) ([]byte, error) {
	return m.img, nil
}

func TestImageGenHandlerGenerate(t *testing.T) {
	h := handlers.NewImageGenHandler(&mockImageGen{[]byte("PNGDATA")})
	answer, err := h.Generate(context.Background(), "нарисуй кота")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if string(answer.Photo) != "PNGDATA" {
		t.Errorf("Photo: got %q, want %q", answer.Photo, "PNGDATA")
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
