package handlers

import (
	"context"
	"fmt"
	"log"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type VisionClient interface {
	DescribeImage(ctx context.Context, fileURL, prompt string) (string, error)
}

type VisionHandler struct {
	client    VisionClient
	persona   LLMClient
	sysPrompt string
}

func NewVisionHandler(client VisionClient, persona LLMClient, sysPrompt string) *VisionHandler {
	return &VisionHandler{client: client, persona: persona, sysPrompt: sysPrompt}
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
	raw, err := h.client.DescribeImage(ctx, photoURL, prompt)
	if err != nil {
		return "", err
	}
	if h.persona == nil {
		return raw, nil
	}
	styled, err := h.persona.Complete(ctx, h.sysPrompt, raw)
	if err != nil {
		log.Printf("vision persona wrap error: %v", err)
		return raw, nil
	}
	return styled, nil
}
