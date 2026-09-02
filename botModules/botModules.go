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

// Типы полей, которые модуль может объявить в Registration.
const (
	FieldNumber = "number"
	FieldBool   = "bool"
	FieldSelect = "select"
	FieldText   = "text"
)

type Option struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// Field — одно поле формы настроек, как его описывает сам модуль.
//
// Options модуль считает в момент вызова, а не берёт из конфига: список персон
// у aiAnswer живёт в его же базе и меняется без перезапуска.
type Field struct {
	Key         string   `json:"key"`
	Label       string   `json:"label"`
	Description string   `json:"description,omitempty"`
	Type        string   `json:"type"`
	Min         *int     `json:"min,omitempty"`
	Max         *int     `json:"max,omitempty"`
	Options     []Option `json:"options,omitempty"`
	Default     any      `json:"default,omitempty"`
}

// Registration — то, чем модуль представляется движку на старте: порядок в
// очереди, человеческое имя для админки и описание своих настроек.
type Registration struct {
	Order       int     `json:"order"`
	Label       string  `json:"label"`
	Description string  `json:"description,omitempty"`
	Fields      []Field `json:"fields,omitempty"`
}

type BotModule interface {
	Register() Registration
	IsCalled(payload *Payload) bool
	Answer(payload *Payload) (RichAnswer, error)
}
