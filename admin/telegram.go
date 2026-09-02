package main

import (
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Leaver — та часть телеграма, которой пользуется панель. Интерфейсом, чтобы
// хендлеры тестировались без сети, как Sender в notify.
type Leaver interface {
	LeaveChat(chatID int64) error
}

type BotLeaver struct {
	API *tgbotapi.BotAPI
}

func (b *BotLeaver) LeaveChat(chatID int64) error {
	_, err := b.API.Request(tgbotapi.LeaveChatConfig{ChatID: chatID})
	return err
}
