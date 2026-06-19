package botModules

import tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

type Payload struct {
	Msg   *tgbotapi.Message
	Extra map[string]interface{}
}

type RichAnswer struct {
	Text     string
	PhotoURL string
}

type BotModule interface {
	Order() int
	IsCalled(msg *tgbotapi.Message) bool
	Answer(payload *Payload) (RichAnswer, error)
}
