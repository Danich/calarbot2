package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"calarbot2/modules/aiAnswer/handlers"
)

type mockVision struct{ desc string }

func (m *mockVision) DescribeImage(_ context.Context, _, _ string) (string, error) {
	return m.desc, nil
}

func TestVisionHandlerDescribe(t *testing.T) {
	// Fake Telegram getFile API
	tgServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":     true,
			"result": map[string]string{"file_path": "photos/test.jpg"},
		})
	}))
	defer tgServer.Close()

	h := handlers.NewVisionHandler(&mockVision{"a fluffy cat"}, "fake-token")
	h.BotAPIURL = tgServer.URL

	msg := &tgbotapi.Message{
		Text:  "what is this?",
		Photo: []tgbotapi.PhotoSize{{FileID: "file123", Width: 100, Height: 100}},
	}

	desc, err := h.Describe(context.Background(), msg)
	if err != nil {
		t.Fatalf("Describe: %v", err)
	}
	if desc != "a fluffy cat" {
		t.Errorf("got %q, want %q", desc, "a fluffy cat")
	}
}

func TestVisionHandlerNoPhoto(t *testing.T) {
	h := handlers.NewVisionHandler(&mockVision{"irrelevant"}, "fake-token")
	msg := &tgbotapi.Message{Text: "hello"}
	_, err := h.Describe(context.Background(), msg)
	if err == nil {
		t.Error("expected error for message with no photo")
	}
}
