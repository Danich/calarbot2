package handlers

import (
	"context"
	"fmt"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type VisionClient interface {
	DescribeImage(ctx context.Context, fileURL, prompt string) (string, error)
}

type VisionHandler struct {
	client VisionClient
}

func NewVisionHandler(client VisionClient) *VisionHandler {
	return &VisionHandler{client: client}
}

func (h *VisionHandler) Describe(ctx context.Context, msg *tgbotapi.Message, photoURL string) (string, error) {
	if photoURL == "" {
		return "", fmt.Errorf("no photo URL provided")
	}

	prompt := msg.Text
	if prompt == "" {
		prompt = msg.Caption
	}
	if prompt == "" {
		prompt = "Describe this image in detail."
	}
	return h.client.DescribeImage(ctx, photoURL, prompt)
}
