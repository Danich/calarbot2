package botModules

import tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

type Payload struct {
	Msg   *tgbotapi.Message
	Extra map[string]interface{}
}

type RichAnswer struct {
	Text string
	// PhotoURL — картинка, за которой телеграм сходит сам.
	PhotoURL string
	// Photo — готовые байты картинки, когда ссылки не существует: провайдеры
	// генерации отдают base64, а не URL, и хостить это негде.
	//
	// json маршалит []byte в base64 и обратно, так что по проводу между
	// модулем и движком оно едет без отдельной возни.
	Photo []byte
}

type BotModule interface {
	Order() int
	IsCalled(msg *tgbotapi.Message) bool
	Answer(payload *Payload) (RichAnswer, error)
}
