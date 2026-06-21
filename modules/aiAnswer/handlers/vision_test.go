package handlers_test

import (
	"context"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"calarbot2/modules/aiAnswer/handlers"
)

type mockVision struct{ desc string }

func (m *mockVision) DescribeImage(_ context.Context, _, _ string) (string, error) {
	return m.desc, nil
}

func TestVisionHandlerDescribe(t *testing.T) {
	h := handlers.NewVisionHandler(&mockVision{"a fluffy cat"})
	msg := &tgbotapi.Message{
		Caption: "что это?",
		Photo:   []tgbotapi.PhotoSize{{FileID: "file123", Width: 100, Height: 100}},
	}

	desc, err := h.Describe(context.Background(), msg, "https://cdn.telegram.org/file/photos/test.jpg")
	if err != nil {
		t.Fatalf("Describe: %v", err)
	}
	if desc != "a fluffy cat" {
		t.Errorf("got %q, want %q", desc, "a fluffy cat")
	}
}

func TestVisionHandlerNoPhotoURL(t *testing.T) {
	h := handlers.NewVisionHandler(&mockVision{"irrelevant"})
	msg := &tgbotapi.Message{Text: "hello"}
	_, err := h.Describe(context.Background(), msg, "")
	if err == nil {
		t.Error("expected error when no photo URL provided")
	}
}
