package handlers

import (
	"context"
	"fmt"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"calarbot2/modules/aiAnswer/store"
)

type LLMClient interface {
	Complete(ctx context.Context, system, user string) (string, error)
}

type TextHandler struct {
	client       LLMClient
	systemPrompt string
}

func NewTextHandler(client LLMClient, systemPrompt string) *TextHandler {
	return &TextHandler{client: client, systemPrompt: systemPrompt}
}

func buildContextPrompt(chatTitle string, history []store.ContextMessage, msg *tgbotapi.Message) string {
	var sb strings.Builder
	sb.WriteString("Last messages in chat ")
	sb.WriteString(chatTitle)
	sb.WriteString(":\n")
	for _, m := range history {
		sb.WriteString(fmt.Sprintf(" from %s: %s\n", m.Username, m.Text))
	}
	if msg.From != nil {
		sb.WriteString(fmt.Sprintf(" from %s: %s", msg.From.UserName, msg.Text))
	}
	return sb.String()
}

func chatTitle(msg *tgbotapi.Message) string {
	if msg.Chat != nil && msg.Chat.Title != "" {
		return msg.Chat.Title
	}
	return "Unknown"
}

func (h *TextHandler) Chat(ctx context.Context, msg *tgbotapi.Message, history []store.ContextMessage) (string, error) {
	return h.client.Complete(ctx, h.systemPrompt, buildContextPrompt(chatTitle(msg), history, msg))
}

func (h *TextHandler) Answer(ctx context.Context, msg *tgbotapi.Message, history []store.ContextMessage) (string, error) {
	return h.client.Complete(ctx, h.systemPrompt, buildContextPrompt(chatTitle(msg), history, msg))
}

func (h *TextHandler) Translate(ctx context.Context, msg *tgbotapi.Message, _ []store.ContextMessage) (string, error) {
	return h.client.Complete(ctx,
		"You are a translator. Detect the source language and translate to Russian if not Russian, or to English otherwise. Reply with only the translated text.",
		msg.Text,
	)
}
