package main

import (
	"log"
	"os"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"calarbot2/botModules"
)

var includeModules = map[string]botModules.ModuleClient{
	"simpleReply": {"http://simpleReply:8080"},
	"skazka":      {"http://skazka:8080"},
	"sber":        {"http://sber:8080"},
}

type Bot struct {
	BotAPI  *tgbotapi.BotAPI
	Flags   map[string]bool
	Modules []*botModules.BotModule
}

func readToken(filename string) (string, error) {
	token, err := os.ReadFile(filename)

	return strings.Trim(string(token), "\n"), err
}

func (b *Bot) InitBot() {
	token, err := readToken("/.tgtoken")
	if err != nil {
		log.Panic(err)
	}

	b.BotAPI, err = tgbotapi.NewBotAPI(token)
	if err != nil {
		log.Panic(err)
	}

	b.BotAPI.Debug = true

	log.Printf("Authorized on account %s", b.BotAPI.Self.UserName)
}

func (b *Bot) RunBot() {
	bot := b.BotAPI

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := bot.GetUpdatesChan(u)

	for update := range updates {
		if update.Message != nil { // If we got a message
			log.Printf("[%s] %s", update.Message.From.UserName, update.Message.Text)

			// Find the module that should handle this message
			payload := &botModules.Payload{Msg: update.Message, Extra: nil}
			var moduleFound bool
			var answer string
			var err error

			for moduleName, client := range includeModules {
				isCalled, err := client.IsCalled(payload)
				if err != nil {
					log.Printf("Error checking if module %s is called: %v", moduleName, err)
					continue
				}

				if isCalled {
					log.Printf("Module %s will handle the message", moduleName)
					answer, err = client.Answer(payload)
					if err != nil {
						log.Printf("Error in module %s: %v", moduleName, err)
						answer = "An error occurred while processing your request."
					}
					moduleFound = true
					break
				}
			}

			// If no module was found to handle the message, use simpleReply as fallback
			if !moduleFound {
				log.Printf("No module found to handle the message, using simpleReply as fallback")
				client := includeModules["simpleReply"]
				answer, err = client.Answer(payload)
				if err != nil {
					log.Printf("Error in fallback module: %v", err)
					answer = "An error occurred while processing your request."
				}
			}

			// Only send a response if there's something to say
			if answer != "" {
				msg := tgbotapi.NewMessage(update.Message.Chat.ID, answer)
				msg.ReplyToMessageID = update.Message.MessageID

				_, err = bot.Send(msg)
				if err != nil {
					log.Printf("Error sending message: %v", err)
				}
			}
		}
	}
}
