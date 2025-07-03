package main

import (
	"fmt"
	"os"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"calarbot2/botModules"
)

type Module struct {
	order int
}

func (m Module) Order() int                          { return m.order }
func (m Module) IsCalled(msg *tgbotapi.Message) bool { return true }
func (m Module) Answer(msg *botModules.Payload) (string, error) {
	return msg.Msg.Text, nil
}

func main() {
	order := 1000
	if len(os.Args) > 1 {
		_, _ = fmt.Sscanf(os.Args[1], "%d", &order)
	}
	module := Module{order: order}
	err := botModules.ServeModule(module, ":8080")
	if err != nil {
		fmt.Println("Error starting server:", err)
	}
}
