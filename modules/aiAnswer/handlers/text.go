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
		sb.WriteString(fmt.Sprintf(" from %s: %s\n", m.Username, describe(m)))
	}
	// Текущее сообщение уже лежит в history: его записали в IsCalled, до того
	// как решили отвечать. Дописывать его отдельно значит показать модели один
	// и тот же ход дважды. Ветка ниже — на случай, когда записать не удалось и
	// история пуста.
	if len(history) == 0 && msg.From != nil {
		sb.WriteString(fmt.Sprintf(" from %s: %s", msg.From.UserName, msg.Text))
	}
	return sb.String()
}

// describe отдаёт текст сообщения, а для медиа без подписи — пометку о том, что
// это было. Иначе картинка превращается в пустой ход «from vasya: », и модель
// видит в разговоре дырку вместо события.
func describe(m store.ContextMessage) string {
	if m.Text != "" {
		return m.Text
	}
	switch m.MediaType {
	case "photo":
		return "[прислал картинку]"
	case "sticker":
		return "[прислал стикер]"
	default:
		return m.Text
	}
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
