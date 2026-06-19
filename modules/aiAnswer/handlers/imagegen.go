package handlers

import (
	"context"
	"fmt"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type ImageGenClient interface {
	GenerateImage(ctx context.Context, prompt string) (string, error)
}

type ImageGenHandler struct {
	client ImageGenClient
}

func NewImageGenHandler(client ImageGenClient) *ImageGenHandler {
	return &ImageGenHandler{client: client}
}

// Generate returns the URL of the generated image.
func (h *ImageGenHandler) Generate(ctx context.Context, msg *tgbotapi.Message) (string, error) {
	if msg.Text == "" {
		return "", fmt.Errorf("empty prompt: no text in message")
	}
	return h.client.GenerateImage(ctx, msg.Text)
}
